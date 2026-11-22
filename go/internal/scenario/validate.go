package scenario

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/lax"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// FieldError is one validation problem; Loc is the dotted path of the field.
type FieldError struct {
	Loc []string
	Msg string
}

// ValidationError collects the problems of a scenario, step or condition.
type ValidationError struct {
	Model  string
	Errors []FieldError
}

// Details joins the errors as "loc: msg; loc: msg" (the form used for steps
// that become invalid after variable substitution).
func (e *ValidationError) Details() string {
	parts := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		parts[i] = strings.Join(fe.Loc, ".") + ": " + fe.Msg
	}
	return strings.Join(parts, "; ")
}

// Error formats like pydantic: "N validation errors for Model" then loc and message lines.
func (e *ValidationError) Error() string {
	noun := "errors"
	if len(e.Errors) == 1 {
		noun = "error"
	}
	lines := []string{fmt.Sprintf("%d validation %s for %s", len(e.Errors), noun, e.Model)}
	for _, fe := range e.Errors {
		lines = append(lines, strings.Join(fe.Loc, "."), "  "+fe.Msg)
	}
	return strings.Join(lines, "\n")
}

type parser struct {
	errs []FieldError
}

func (p *parser) add(loc []string, msg string) {
	p.errs = append(p.errs, FieldError{Loc: append([]string(nil), loc...), Msg: msg})
}

func (p *parser) valueError(loc []string, msg string) { p.add(loc, "Value error, "+msg) }

func join(loc []string, parts ...string) []string {
	return append(append([]string(nil), loc...), parts...)
}

// obj reads typed fields of one mapping and reports problems under loc.
type obj struct {
	p      *parser
	loc    []string
	m      *omap.Map
	coerce bool // coerce_numbers_to_str
}

func (p *parser) object(loc []string, v any, model string, coerce bool) (obj, bool) {
	m, ok := v.(*omap.Map)
	if !ok {
		p.add(loc, "Input should be a valid dictionary or instance of "+model)
		return obj{}, false
	}
	return obj{p: p, loc: loc, m: m, coerce: coerce}, true
}

// get returns the value when the key is present and not null.
func (o obj) get(key string) (any, bool) {
	v, ok := o.m.Get(key)
	return v, ok && v != nil
}

func (o obj) fail(key, msg string) { o.p.add(join(o.loc, key), msg) }

func (o obj) forbidExtra(allowed ...string) {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for _, k := range o.m.Keys() {
		if !ok[k] {
			o.fail(k, "Extra inputs are not permitted")
		}
	}
}

func (o obj) required(key string) {
	if !o.m.Has(key) {
		o.fail(key, "Field required")
	}
}

func toStr(v any, coerce bool) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case int:
		if coerce {
			return strconv.Itoa(t), true
		}
	case float64:
		if coerce {
			return omap.FormatFloat(t), true
		}
	}
	return "", false
}

// str reads a str|None field.
func (o obj) str(key string) *string {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	s, ok := toStr(v, o.coerce)
	if !ok {
		o.fail(key, "Input should be a valid string")
		return nil
	}
	return &s
}

func (o obj) strOr(key string) string {
	if s := o.str(key); s != nil {
		return *s
	}
	return ""
}

// integer reads an int field with a minimum; nil when absent or null.
func (o obj) integer(key string, min *int) *int {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	n, msg := lax.Int(v)
	if msg == "" && min != nil && n < *min {
		msg = fmt.Sprintf("Input should be greater than or equal to %d", *min)
	}
	if msg != "" {
		o.fail(key, msg)
		return nil
	}
	return &n
}

// intOr reads a non-nullable int field; explicit null is an error.
func (o obj) intOr(key string, def int, min *int) int {
	if v, ok := o.m.Get(key); ok && v == nil {
		o.fail(key, "Input should be a valid integer")
		return def
	}
	if n := o.integer(key, min); n != nil {
		return *n
	}
	return def
}

func (o obj) float(key string) *float64 {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	f, ok := lax.Float(v)
	if !ok {
		o.fail(key, "Input should be a valid number")
		return nil
	}
	return &f
}

func (o obj) floatOr(key string, def float64) float64 {
	if f := o.float(key); f != nil {
		return *f
	}
	return def
}

func (o obj) optBool(key string) *bool {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	b, ok := lax.Bool(v)
	if !ok {
		o.fail(key, "Input should be a valid boolean, unable to interpret input")
		return nil
	}
	return &b
}

// boolOr reads a non-nullable bool field; explicit null is an error.
func (o obj) boolOr(key string, def bool) bool {
	if v, ok := o.m.Get(key); ok && v == nil {
		o.fail(key, "Input should be a valid boolean")
		return def
	}
	if b := o.optBool(key); b != nil {
		return *b
	}
	return def
}

func (o obj) literal(key string, choices ...string) *string {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	if s, ok := v.(string); ok {
		for _, c := range choices {
			if s == c {
				return &s
			}
		}
	}
	q := make([]string, len(choices))
	for i, c := range choices {
		q[i] = "'" + c + "'"
	}
	msg := "Input should be " + q[0]
	if len(q) > 1 {
		msg = "Input should be " + strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
	}
	o.fail(key, msg)
	return nil
}

// strList reads a list[str]|None field.
func (o obj) strList(key string) []string {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		o.fail(key, "Input should be a valid list")
		return nil
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := toStr(item, o.coerce)
		if !ok {
			o.p.add(join(o.loc, key, strconv.Itoa(i)), "Input should be a valid string")
			continue
		}
		out = append(out, s)
	}
	return out
}

func (o obj) dict(key string) *omap.Map {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	m, ok := v.(*omap.Map)
	if !ok {
		o.fail(key, "Input should be a valid dictionary")
		return nil
	}
	return m
}
