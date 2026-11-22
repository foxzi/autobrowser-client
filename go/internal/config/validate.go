package config

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// decoder converts a merged config map into typed values with pydantic-like
// lax coercion (numeric strings to int, "yes" to bool) and collects errors.
type decoder struct {
	errs []string
}

func (d *decoder) fail(loc []string, msg string) {
	d.errs = append(d.errs, strings.Join(loc, ".")+"\n  "+msg)
}

func sub(loc []string, key string) []string {
	return append(append([]string(nil), loc...), key)
}

// obj returns the section map; a missing key gives an empty map.
func (d *decoder) obj(loc []string, parent map[string]any, key string, allowed ...string) map[string]any {
	l := sub(loc, key)
	v, ok := parent[key]
	if !ok {
		return map[string]any{}
	}
	m, ok := v.(map[string]any)
	if !ok {
		d.fail(l, "Input should be a valid dictionary or object to extract fields from")
		return map[string]any{}
	}
	d.extra(l, m, allowed...)
	return m
}

func (d *decoder) extra(loc []string, m map[string]any, allowed ...string) {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var unknown []string
	for k := range m {
		if !ok[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		d.fail(sub(loc, k), "Extra inputs are not permitted")
	}
}

func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case int:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case float64:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "0", "off", "f", "false", "n", "no":
			return false, true
		case "1", "on", "t", "true", "y", "yes":
			return true, true
		}
	}
	return false, false
}

func (d *decoder) boolean(loc []string, m map[string]any, key string, def bool) bool {
	v, ok := m[key]
	if !ok {
		return def
	}
	b, ok := asBool(v)
	if !ok {
		d.fail(sub(loc, key), "Input should be a valid boolean, unable to interpret input")
		return def
	}
	return b
}

func asInt(v any) (int, string) {
	switch t := v.(type) {
	case int:
		return t, ""
	case float64:
		if t == math.Trunc(t) && !math.IsInf(t, 0) {
			return int(t), ""
		}
		return 0, "Input should be a valid integer, got a number with a fractional part"
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return i, ""
		}
		return 0, "Input should be a valid integer, unable to parse string as an integer"
	}
	return 0, "Input should be a valid integer"
}

// intBounds are inclusive limits; gt0 means strictly greater than zero.
type intBounds struct {
	min, max int
	gt0      bool
}

func (d *decoder) integer(loc []string, m map[string]any, key string, def int, b intBounds) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	i, msg := asInt(v)
	if msg == "" {
		msg = b.check(i)
	}
	if msg != "" {
		d.fail(sub(loc, key), msg)
		return def
	}
	return i
}

func (d *decoder) optInt(loc []string, m map[string]any, key string, b intBounds) *int {
	if v, ok := m[key]; !ok || v == nil {
		return nil
	}
	i := d.integer(loc, m, key, 0, b)
	return &i
}

func (b intBounds) check(i int) string {
	switch {
	case b.gt0 && i <= 0:
		return "Input should be greater than 0"
	case !b.gt0 && i < b.min:
		return fmt.Sprintf("Input should be greater than or equal to %d", b.min)
	case b.max != 0 && i > b.max:
		return fmt.Sprintf("Input should be less than or equal to %d", b.max)
	}
	return ""
}

func (d *decoder) str(loc []string, m map[string]any, key, def string) string {
	v, ok := m[key]
	if !ok {
		return def
	}
	s, ok := v.(string)
	if !ok {
		d.fail(sub(loc, key), "Input should be a valid string")
		return def
	}
	return s
}

func (d *decoder) optStr(loc []string, m map[string]any, key string) *string {
	if v, ok := m[key]; !ok || v == nil {
		return nil
	}
	s := d.str(loc, m, key, "")
	return &s
}

func literalMsg(choices []string) string {
	q := make([]string, len(choices))
	for i, c := range choices {
		q[i] = "'" + c + "'"
	}
	if len(q) == 1 {
		return "Input should be " + q[0]
	}
	return "Input should be " + strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}

func (d *decoder) literal(loc []string, m map[string]any, key, def string, choices ...string) string {
	v, ok := m[key]
	if !ok {
		return def
	}
	if s, ok := v.(string); ok {
		for _, c := range choices {
			if s == c {
				return s
			}
		}
	}
	d.fail(sub(loc, key), literalMsg(choices))
	return def
}

func (d *decoder) optLiteral(loc []string, m map[string]any, key string, choices ...string) *string {
	if v, ok := m[key]; !ok || v == nil {
		return nil
	}
	s := d.literal(loc, m, key, "", choices...)
	if s == "" {
		return nil
	}
	return &s
}

func (d *decoder) strList(loc []string, m map[string]any, key string) []string {
	out := []string{}
	v, ok := m[key]
	if !ok {
		return out
	}
	switch t := v.(type) {
	case []string:
		return append(out, t...)
	case []any:
		for i, item := range t {
			s, ok := item.(string)
			if !ok {
				d.fail(sub(sub(loc, key), strconv.Itoa(i)), "Input should be a valid string")
				continue
			}
			out = append(out, s)
		}
		return out
	}
	d.fail(sub(loc, key), "Input should be a valid list")
	return out
}
