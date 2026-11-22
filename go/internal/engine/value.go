package engine

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

var (
	exactRe = regexp.MustCompile(`^\$\{([^}]+)\}$`)
	tokenRe = regexp.MustCompile(`\$\$\{|\$\{([^}]+)\}`)
)

// TemplateError means a ${...} could not be substituted into a step; the
// step (not the script) fails with its text.
type TemplateError struct {
	Msg       string
	Undefined bool
}

func (e *TemplateError) Error() string { return e.Msg }

func undefined(path string) error {
	return &TemplateError{Msg: "undefined variable: " + path, Undefined: true}
}

func mapGet(v any, key string) (any, bool) {
	switch t := v.(type) {
	case *omap.Map:
		return t.Get(key)
	case map[string]any:
		x, ok := t[key]
		return x, ok
	}
	return nil, false
}

func listItems(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case *omap.List:
		return t.Items, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// lookup resolves "name", "a.b.0" or "env.NAME" to a value.
func lookup(path string, vars *omap.Map) (any, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "env.") {
		if v, ok := os.LookupEnv(path[4:]); ok {
			return v, nil
		}
		return nil, undefined(path)
	}
	parts := strings.Split(path, ".")
	value, ok := vars.Get(parts[0])
	if !ok {
		return nil, undefined(path)
	}
	for _, part := range parts[1:] {
		if x, ok := mapGet(value, part); ok {
			value = x
			continue
		}
		if items, ok := listItems(value); ok && isDigits(part) {
			if i, err := strconv.Atoi(part); err == nil && i < len(items) {
				value = items[i]
				continue
			}
		}
		return nil, undefined(path)
	}
	return value, nil
}

// Substitute replaces ${...} in every string of a nested value. A string
// that is exactly one ${...} keeps the type of the variable; $${ is a literal ${.
func Substitute(value any, vars *omap.Map) (any, error) {
	switch t := value.(type) {
	case string:
		if m := exactRe.FindStringSubmatch(t); m != nil {
			return lookup(m[1], vars)
		}
		var sb strings.Builder
		last := 0
		for _, loc := range tokenRe.FindAllStringSubmatchIndex(t, -1) {
			sb.WriteString(t[last:loc[0]])
			last = loc[1]
			if loc[2] < 0 {
				sb.WriteString("${")
				continue
			}
			v, err := lookup(t[loc[2]:loc[3]], vars)
			if err != nil {
				return nil, err
			}
			sb.WriteString(omap.Str(v))
		}
		sb.WriteString(t[last:])
		return sb.String(), nil
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			v, err := Substitute(x, vars)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case *omap.Map:
		out := omap.New()
		for _, k := range t.Keys() {
			x, _ := t.Get(k)
			v, err := Substitute(x, vars)
			if err != nil {
				return nil, err
			}
			out.Set(k, v)
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			v, err := Substitute(x, vars)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	}
	return value, nil
}

func numeric(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func dictEntries(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case *omap.Map:
		m := make(map[string]any, t.Len())
		for _, k := range t.Keys() {
			m[k], _ = t.Get(k)
		}
		return m, true
	case map[string]any:
		return t, true
	}
	return nil, false
}

// pyEq is Python's == for scenario values (1 == 1.0 == True, dicts ignore order).
func pyEq(a, b any) bool {
	if af, ok := numeric(a); ok {
		bf, ok := numeric(b)
		return ok && af == bf
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	if la, ok := listItems(a); ok {
		lb, ok := listItems(b)
		if !ok || len(la) != len(lb) {
			return false
		}
		for i := range la {
			if !pyEq(la[i], lb[i]) {
				return false
			}
		}
		return true
	}
	if da, ok := dictEntries(a); ok {
		db, ok := dictEntries(b)
		if !ok || len(da) != len(db) {
			return false
		}
		for k, v := range da {
			w, ok := db[k]
			if !ok || !pyEq(v, w) {
				return false
			}
		}
		return true
	}
	return false
}

func pyTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int, int64, float64:
		f, _ := numeric(t)
		return f != 0
	}
	if items, ok := listItems(v); ok {
		return len(items) > 0
	}
	if d, ok := dictEntries(v); ok {
		return len(d) > 0
	}
	return true
}

// pyTypeName is type(v).__name__.
func pyTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case int, int64:
		return "int"
	case float64:
		return "float"
	}
	if _, ok := listItems(v); ok {
		return "list"
	}
	if _, ok := dictEntries(v); ok {
		return "dict"
	}
	return fmt.Sprintf("%T", v)
}

// pyInt is int(v); ok is false where Python raises.
func pyInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, false
		}
		return int(t), true
	case string:
		s := strings.ReplaceAll(strings.TrimSpace(t), "_", "")
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	return 0, false
}
