package omap

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// FormatFloat formats a float like Python's repr(): 1.0, 0.5, 1e+16.
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64)
	exp, _ := strconv.Atoi(e[strings.IndexByte(e, 'e')+1:])
	if exp < -4 || exp >= 16 {
		return e
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsRune(s, '.') {
		s += ".0"
	}
	return s
}

// Marshal writes v like Python's json.dumps(v, ensure_ascii=False, indent=indent).
// indent 0 gives the compact form with ", " and ": " separators.
func Marshal(v any, indent int) string {
	var sb strings.Builder
	encode(&sb, v, indent, 0)
	return sb.String()
}

func quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&sb, `\u%04x`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func encode(sb *strings.Builder, v any, indent, level int) {
	switch t := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		sb.WriteString(strconv.FormatBool(t))
	case string:
		sb.WriteString(quote(t))
	case int:
		sb.WriteString(strconv.Itoa(t))
	case int64:
		sb.WriteString(strconv.FormatInt(t, 10))
	case float64:
		switch {
		case math.IsNaN(t):
			sb.WriteString("NaN")
		case math.IsInf(t, 1):
			sb.WriteString("Infinity")
		case math.IsInf(t, -1):
			sb.WriteString("-Infinity")
		default:
			sb.WriteString(FormatFloat(t))
		}
	case *Map:
		encodeObject(sb, t.keys, func(k string) any { return t.vals[k] }, indent, level)
	case *List:
		encode(sb, t.Items, indent, level)
	case []any:
		encodeArray(sb, len(t), func(i int) any { return t[i] }, indent, level)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		encodeObject(sb, keys, func(k string) any { return t[k] }, indent, level)
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			encodeArray(sb, rv.Len(), func(i int) any { return rv.Index(i).Interface() }, indent, level)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			sb.WriteString(strconv.FormatInt(rv.Int(), 10))
		case reflect.Float32:
			encode(sb, rv.Float(), indent, level)
		case reflect.Map:
			m := map[string]any{}
			for _, k := range rv.MapKeys() {
				m[fmt.Sprint(k.Interface())] = rv.MapIndex(k).Interface()
			}
			encode(sb, m, indent, level)
		default:
			sb.WriteString(quote(fmt.Sprint(v)))
		}
	}
}

func newline(sb *strings.Builder, indent, level int) {
	sb.WriteByte('\n')
	sb.WriteString(strings.Repeat(" ", indent*level))
}

func encodeObject(sb *strings.Builder, keys []string, get func(string) any, indent, level int) {
	if len(keys) == 0 {
		sb.WriteString("{}")
		return
	}
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
			if indent == 0 {
				sb.WriteByte(' ')
			}
		}
		if indent > 0 {
			newline(sb, indent, level+1)
		}
		sb.WriteString(quote(k))
		sb.WriteString(": ")
		encode(sb, get(k), indent, level+1)
	}
	if indent > 0 {
		newline(sb, indent, level)
	}
	sb.WriteByte('}')
}

func encodeArray(sb *strings.Builder, n int, get func(int) any, indent, level int) {
	if n == 0 {
		sb.WriteString("[]")
		return
	}
	sb.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
			if indent == 0 {
				sb.WriteByte(' ')
			}
		}
		if indent > 0 {
			newline(sb, indent, level+1)
		}
		encode(sb, get(i), indent, level+1)
	}
	if indent > 0 {
		newline(sb, indent, level)
	}
	sb.WriteByte(']')
}
