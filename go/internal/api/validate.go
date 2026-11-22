package api

import (
	"regexp"

	"github.com/foxzi/autobrowser-client/go/internal/lax"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

type kind int

const (
	kStr kind = iota
	kInt
	kFloat
	kBool
	kDict
	kStrList
	kDictList
	kStrOrStrList // str | list[str]
	kDictOrStr    // dict[str, Any] | str
	kModel
)

// field is one field of a pydantic request model, under its JSON name (alias).
type field struct {
	name     string
	kind     kind
	required bool
	nullable bool // "X | None"
	def      any
	pattern  string
	model    []field // kModel
}

// validationError is a FastAPI RequestValidationError: 422 {"detail": [...]}.
type validationError []any

func (e validationError) Error() string { return "validation error" }

func verr(typ string, loc []any, msg string, input any) *omap.Map {
	return omap.New("type", typ, "loc", loc, "msg", msg, "input", input)
}

func at(loc []any, k any) []any { return append(append([]any{}, loc...), k) }

// validateModel checks a JSON value against the fields like pydantic in lax
// mode: unknown keys are ignored, defaults filled in, values coerced.
func validateModel(v any, fields []field, loc []any) (*omap.Map, []any) {
	obj, ok := v.(*omap.Map)
	if !ok {
		return nil, []any{verr("model_attributes_type", loc,
			"Input should be a valid dictionary or object to extract fields from", v)}
	}
	out := omap.New()
	var errs []any
	for _, f := range fields {
		raw, ok := obj.Get(f.name)
		if !ok {
			if f.required {
				errs = append(errs, verr("missing", at(loc, f.name), "Field required", obj))
			} else {
				out.Set(f.name, f.def)
			}
			continue
		}
		val, fe := validateField(raw, f, at(loc, f.name))
		errs = append(errs, fe...)
		out.Set(f.name, val)
	}
	return out, errs
}

func validateField(v any, f field, loc []any) (any, []any) {
	if v == nil && f.nullable {
		return nil, nil
	}
	switch f.kind {
	case kStr:
		s, ok := v.(string)
		if !ok {
			return nil, []any{verr("string_type", loc, "Input should be a valid string", v)}
		}
		if f.pattern != "" && !regexp.MustCompile(f.pattern).MatchString(s) {
			e := verr("string_pattern_mismatch", loc, "String should match pattern '"+f.pattern+"'", v)
			e.Set("ctx", omap.New("pattern", f.pattern))
			return nil, []any{e}
		}
		return s, nil
	case kInt:
		if b, ok := v.(bool); ok {
			if b {
				return 1, nil
			}
			return 0, nil
		}
		n, msg := lax.Int(v)
		if msg != "" {
			typ := "int_type"
			switch msg {
			case "Input should be a valid integer, got a number with a fractional part":
				typ = "int_from_float"
			case "Input should be a valid integer, unable to parse string as an integer":
				typ = "int_parsing"
			}
			return nil, []any{verr(typ, loc, msg, v)}
		}
		return n, nil
	case kFloat:
		if b, ok := v.(bool); ok {
			if b {
				return 1.0, nil
			}
			return 0.0, nil
		}
		x, ok := lax.Float(v)
		if !ok {
			if _, isStr := v.(string); isStr {
				return nil, []any{verr("float_parsing", loc, "Input should be a valid number, unable to parse string as a number", v)}
			}
			return nil, []any{verr("float_type", loc, "Input should be a valid number", v)}
		}
		return x, nil
	case kBool:
		b, ok := lax.Bool(v)
		if !ok {
			switch v.(type) {
			case string, int, float64:
				return nil, []any{verr("bool_parsing", loc, "Input should be a valid boolean, unable to interpret input", v)}
			}
			return nil, []any{verr("bool_type", loc, "Input should be a valid boolean", v)}
		}
		return b, nil
	case kDict:
		if _, ok := v.(*omap.Map); !ok {
			return nil, []any{verr("dict_type", loc, "Input should be a valid dictionary", v)}
		}
		return v, nil
	case kStrList, kDictList:
		list, ok := v.([]any)
		if !ok {
			return nil, []any{verr("list_type", loc, "Input should be a valid list", v)}
		}
		item := field{kind: kStr}
		if f.kind == kDictList {
			item.kind = kDict
		}
		var errs []any
		for i, x := range list {
			_, fe := validateField(x, item, at(loc, i))
			errs = append(errs, fe...)
		}
		return list, errs
	case kStrOrStrList:
		if s, ok := v.(string); ok {
			return s, nil
		}
		_, e1 := validateField(v, field{kind: kStr}, at(loc, "str"))
		list, e2 := validateField(v, field{kind: kStrList}, at(loc, "list[str]"))
		if e2 == nil {
			return list, nil
		}
		return nil, append(e1, e2...)
	case kDictOrStr:
		if _, ok := v.(*omap.Map); ok {
			return v, nil
		}
		if s, ok := v.(string); ok {
			return s, nil
		}
		_, e1 := validateField(v, field{kind: kDict}, at(loc, "dict[str,any]"))
		_, e2 := validateField(v, field{kind: kStr}, at(loc, "str"))
		return nil, append(e1, e2...)
	case kModel:
		return validateModel(v, f.model, loc)
	}
	return v, nil
}

// queryParam validates one query parameter (all of them are optional).
func queryParam(raw string, present bool, f field) (any, []any) {
	if !present {
		return f.def, nil
	}
	return validateField(raw, f, []any{"query", f.name})
}
