package mcp

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// fieldError is one pydantic validation error of the tool arguments.
type fieldError struct {
	loc, msg, typ string
	input         any
}

// validateArgs checks args against the input schema like the pydantic model
// Python builds from the tool signature (lax mode: "yes" is a bool, "5" an
// int), stores the coerced values and fills in the defaults. The error text
// is the one pydantic gives.
func validateArgs(tool string, schema, args *omap.Map) error {
	props, _ := schema.Get("properties")
	required := map[string]bool{}
	if req, ok := schema.Get("required"); ok {
		for _, r := range req.([]any) {
			required[r.(string)] = true
		}
	}
	var errs []fieldError
	input := args.Copy() // the arguments as sent, for "missing" errors
	for _, key := range props.(*omap.Map).Keys() {
		p, _ := props.(*omap.Map).Get(key)
		prop := p.(*omap.Map)
		v, ok := args.Get(key)
		if !ok {
			if required[key] {
				errs = append(errs, fieldError{key, "Field required", "missing", input})
			} else {
				def, _ := prop.Get("default")
				args.Set(key, def)
			}
			continue
		}
		v, fe := checkValue(key, prop, v)
		errs = append(errs, fe...)
		args.Set(key, v)
	}
	if len(errs) == 0 {
		return nil
	}
	plural := "error"
	if len(errs) > 1 {
		plural = "errors"
	}
	lines := []string{fmt.Sprintf("%d validation %s for %sArguments", len(errs), plural, tool)}
	for _, e := range errs {
		lines = append(lines, e.loc, fmt.Sprintf("  %s [type=%s, input_value=%s, input_type=%s]",
			e.msg, e.typ, truncRepr(e.input), pyType(e.input)),
			"    For further information visit https://errors.pydantic.dev/2.14/v/"+e.typ)
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

// truncRepr is pydantic's input_value: repr() cut in the middle past 50 chars.
func truncRepr(v any) string {
	r := []rune(omap.Repr(v))
	if len(r) > 50 {
		return string(r[:25]) + "..." + string(r[len(r)-24:])
	}
	return string(r)
}

// pyType is the Python type name of a JSON value (input_type).
func pyType(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case string:
		return "str"
	case bool:
		return "bool"
	case int:
		return "int"
	case float64:
		return "float"
	case []any:
		return "list"
	}
	return "dict"
}

var boolStrings = map[string]bool{
	"0": false, "off": false, "f": false, "false": false, "n": false, "no": false,
	"1": true, "on": true, "t": true, "true": true, "y": true, "yes": true,
}

// checkValue validates one value and returns it coerced to the schema type.
func checkValue(loc string, prop *omap.Map, v any) (any, []fieldError) {
	if anyOf, ok := prop.Get("anyOf"); ok {
		if v == nil {
			return nil, nil
		}
		prop = anyOf.([]any)[0].(*omap.Map) // the non-null member
	}
	bad := func(typ, msg string) (any, []fieldError) { return v, []fieldError{{loc, msg, typ, v}} }
	if enum, ok := prop.Get("enum"); ok {
		var names []string
		for _, e := range enum.([]any) {
			if e == v {
				return v, nil
			}
			names = append(names, "'"+e.(string)+"'")
		}
		msg := strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
		return bad("literal_error", "Input should be "+msg)
	}
	typ, _ := prop.Get("type")
	switch typ {
	case "string":
		if _, ok := v.(string); !ok {
			return bad("string_type", "Input should be a valid string")
		}
	case "boolean":
		switch t := v.(type) {
		case bool:
		case int:
			if t != 0 && t != 1 {
				return bad("bool_parsing", "Input should be a valid boolean, unable to interpret input")
			}
			return t == 1, nil
		case float64:
			if t != 0 && t != 1 {
				return bad("bool_type", "Input should be a valid boolean")
			}
			return t == 1, nil
		case string:
			b, ok := boolStrings[strings.ToLower(t)]
			if !ok {
				return bad("bool_parsing", "Input should be a valid boolean, unable to interpret input")
			}
			return b, nil
		default:
			return bad("bool_type", "Input should be a valid boolean")
		}
	case "integer":
		switch t := v.(type) {
		case int:
		case bool:
			if t {
				return 1, nil
			}
			return 0, nil
		case float64:
			if math.Abs(t) >= 1<<63 {
				return bad("int_parsing_size", "Unable to parse input string as an integer, exceeded maximum size")
			}
			if t != math.Trunc(t) {
				return bad("int_from_float", "Input should be a valid integer, got a number with a fractional part")
			}
			return int(t), nil
		case string:
			s := strings.TrimSpace(t)
			if i := strings.IndexByte(s, '.'); i > 0 && strings.Trim(s[i+1:], "0") == "" {
				s = s[:i]
			}
			n, err := strconv.Atoi(strings.ReplaceAll(s, "_", ""))
			if err != nil {
				return bad("int_parsing", "Input should be a valid integer, unable to parse string as an integer")
			}
			return n, nil
		default:
			return bad("int_type", "Input should be a valid integer")
		}
	case "array":
		list, ok := v.([]any)
		if !ok {
			return bad("list_type", "Input should be a valid list")
		}
		items, _ := prop.Get("items")
		var errs []fieldError
		for i, item := range list {
			_, fe := checkValue(fmt.Sprintf("%s.%d", loc, i), items.(*omap.Map), item)
			errs = append(errs, fe...)
		}
		return v, errs
	case "object":
		m, ok := v.(*omap.Map)
		if !ok {
			return bad("dict_type", "Input should be a valid dictionary")
		}
		items, _ := prop.Get("additionalProperties")
		var errs []fieldError
		for _, k := range m.Keys() {
			item, _ := m.Get(k)
			_, fe := checkValue(loc+"."+k, items.(*omap.Map), item)
			errs = append(errs, fe...)
		}
		return v, errs
	}
	return v, nil
}
