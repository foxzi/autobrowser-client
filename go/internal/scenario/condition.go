package scenario

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

var (
	locatorTypes = []string{"css", "xpath", "id", "name", "class", "text", "attribute", "ref"}
	idRe         = regexp.MustCompile(`^#[a-zA-Z_][\p{L}\p{N}_-]*$`)
	classRe      = regexp.MustCompile(`^\.[a-zA-Z_][\p{L}\p{N}_-]*$`)
)

func isLocatorType(s string) bool {
	for _, t := range locatorTypes {
		if t == s {
			return true
		}
	}
	return false
}

// ParseLocatorString parses "css=a", "//xpath", "#id", ".class" or plain css.
func ParseLocatorString(s string) Locator {
	if eq := strings.Index(s, "="); eq != -1 && isLocatorType(s[:eq]) {
		return Locator{Type: s[:eq], Value: s[eq+1:]}
	}
	if strings.HasPrefix(s, "/") {
		return Locator{Type: "xpath", Value: s}
	}
	if idRe.MatchString(s) {
		return Locator{Type: "id", Value: s[1:]}
	}
	if classRe.MatchString(s) {
		return Locator{Type: "class", Value: s[1:]}
	}
	return Locator{Type: "css", Value: s}
}

func (o obj) locator(key string) *Locator {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	return parseLocator(o.p, join(o.loc, key), v)
}

func parseLocator(p *parser, loc []string, v any) *Locator {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, "${") {
			return &Locator{Type: "css", Value: t, Shorthand: true}
		}
		l := ParseLocatorString(t)
		return &l
	case *omap.Map:
		o := obj{p: p, loc: loc, m: t, coerce: true}
		if sh, _ := t.Get("shorthand"); isTruthy(sh) {
			raw, _ := t.Get("value")
			s, ok := toStr(raw, true)
			if !ok {
				o.fail("value", "Input should be a valid string")
				return nil
			}
			l := ParseLocatorString(s)
			return &l
		}
		o.forbidExtra("type", "value", "shorthand")
		o.required("type")
		o.required("value")
		typ := o.literal("type", locatorTypes...)
		val := o.str("value")
		if typ == nil || val == nil {
			if v, ok := t.Get("value"); ok && v == nil {
				o.fail("value", "Input should be a valid string")
			}
			return nil
		}
		return &Locator{Type: *typ, Value: *val, Shorthand: o.boolOr("shorthand", false)}
	}
	p.add(loc, "Input should be a valid dictionary or instance of Locator")
	return nil
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case int:
		return t != 0
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case *omap.Map:
		return t.Len() > 0
	}
	return true
}

func nonEmpty(s *string) bool { return s != nil && *s != "" }

// ParseCondition validates a condition mapping (used for substituted conditions).
func ParseCondition(raw any) (*Condition, error) {
	p := &parser{}
	c := parseCondition(p, nil, raw)
	if len(p.errs) > 0 {
		return nil, &ValidationError{Model: "Condition", Errors: p.errs}
	}
	return c, nil
}

func parseCondition(p *parser, loc []string, v any) *Condition {
	o, ok := p.object(loc, v, "Condition", true)
	if !ok {
		return nil
	}
	start := len(p.errs)
	o.forbidExtra("exists", "visible", "text", "url", "urlMatches", "title", "var", "equals",
		"contains", "matches", "not", "all", "any", "timeout")
	c := &Condition{}
	c.Exists = o.locator("exists")
	c.Visible = o.locator("visible")
	c.Text = o.str("text")
	c.URL = o.str("url")
	c.URLMatches = o.str("urlMatches")
	c.Title = o.str("title")
	c.Var = o.str("var")
	c.Equals, c.HasEquals = o.m.Get("equals")
	c.Contains = o.str("contains")
	c.Matches = o.str("matches")
	if nv, ok := o.get("not"); ok {
		c.Not = parseCondition(p, join(loc, "not"), nv)
	}
	c.All = conditionList(o, "all")
	c.Any = conditionList(o, "any")
	c.Timeout = o.intOr("timeout", 0, nil)
	if len(p.errs) > start {
		return nil
	}

	count := 0
	for _, present := range []bool{c.Exists != nil, c.Visible != nil, c.Text != nil, c.URL != nil,
		c.URLMatches != nil, c.Title != nil, c.Var != nil, c.Not != nil, c.All != nil, c.Any != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		p.valueError(loc, "condition needs exactly one of: exists, visible, text, url, urlMatches, "+
			"title, var, not, all, any")
		return nil
	}
	if (c.HasEquals || nonEmpty(c.Contains) || nonEmpty(c.Matches)) && c.Var == nil {
		p.valueError(loc, "'equals', 'contains' and 'matches' require 'var'")
		return nil
	}
	if c.Timeout != 0 && (c.Not != nil || c.All != nil || c.Any != nil) {
		p.valueError(loc, "'timeout' is only allowed on a leaf condition")
		return nil
	}
	c.Raw = c.dump(o.m)
	return c
}

func conditionList(o obj, key string) []*Condition {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		o.fail(key, "Input should be a valid list")
		return nil
	}
	out := make([]*Condition, 0, len(list))
	for i, item := range list {
		out = append(out, parseCondition(o.p, join(o.loc, key, fmt.Sprint(i)), item))
	}
	return out
}

// dump builds the canonical mapping of the keys present in the input, in field order.
func (c *Condition) dump(in *omap.Map) *omap.Map {
	out := omap.New()
	loc := func(key string, l *Locator) {
		if in.Has(key) {
			if l == nil {
				out.Set(key, nil)
			} else {
				out.Set(key, l.Dump())
			}
		}
	}
	str := func(key string, s *string) {
		if in.Has(key) {
			if s == nil {
				out.Set(key, nil)
			} else {
				out.Set(key, *s)
			}
		}
	}
	loc("exists", c.Exists)
	loc("visible", c.Visible)
	str("text", c.Text)
	str("url", c.URL)
	str("urlMatches", c.URLMatches)
	str("title", c.Title)
	str("var", c.Var)
	if c.HasEquals {
		out.Set("equals", c.Equals)
	}
	str("contains", c.Contains)
	str("matches", c.Matches)
	if c.Not != nil {
		out.Set("not", c.Not.Raw)
	}
	for _, g := range []struct {
		key  string
		list []*Condition
	}{{"all", c.All}, {"any", c.Any}} {
		if g.list != nil {
			items := make([]any, len(g.list))
			for i, sub := range g.list {
				items[i] = sub.Raw
			}
			out.Set(g.key, items)
		}
	}
	if in.Has("timeout") {
		out.Set("timeout", c.Timeout)
	}
	return out
}
