package omap

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Repr formats a value like Python's repr(): None, True, 'text', [1, 2], {'a': 1}.
func Repr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return reprString(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return FormatFloat(t)
	case *List:
		return Repr(t.Items)
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = Repr(x)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		items := make([]any, len(t))
		for i, x := range t {
			items[i] = x
		}
		return Repr(items)
	case *Map:
		parts := make([]string, 0, t.Len())
		for _, k := range t.keys {
			parts = append(parts, reprString(k)+": "+Repr(t.vals[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = reprString(k) + ": " + Repr(t[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

// Str is Python's str(): like Repr but strings stay unquoted.
func Str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return Repr(v)
}

func reprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r == '\n':
			sb.WriteString(`\n`)
		case r == '\r':
			sb.WriteString(`\r`)
		case r == '\t':
			sb.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&sb, `\x%02x`, r)
		case r > 0x7f && !unicode.IsPrint(r):
			if r > 0xffff {
				fmt.Fprintf(&sb, `\U%08x`, r)
			} else {
				fmt.Fprintf(&sb, `\u%04x`, r)
			}
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}
