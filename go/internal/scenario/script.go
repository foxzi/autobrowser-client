package scenario

import (
	"strconv"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// blockKeys are checked in this order to tell a control block from a step.
var blockKeys = []string{"if", "try", "forEach", "repeat", "while"}

func parseNode(p *parser, loc []string, v any) Node {
	if m, ok := v.(*omap.Map); ok {
		for _, key := range blockKeys {
			if !m.Has(key) {
				continue
			}
			switch key {
			case "if":
				return parseIf(p, loc, m)
			case "try":
				return parseTry(p, loc, m)
			case "forEach":
				return parseForEach(p, loc, m)
			case "repeat":
				return parseRepeat(p, loc, m)
			default:
				return parseWhile(p, loc, m)
			}
		}
	}
	if s := parseStep(p, loc, v); s != nil {
		return s
	}
	return nil
}

// nodeList parses a list of steps; a nil result with no error means "absent".
func (o obj) nodeList(key string, required bool) ([]Node, bool) {
	v, present := o.m.Get(key)
	if !present {
		if required {
			o.fail(key, "Field required")
		}
		return nil, false
	}
	if v == nil && !required {
		return nil, false
	}
	list, ok := v.([]any)
	if !ok {
		o.fail(key, "Input should be a valid list")
		return nil, false
	}
	return parseNodes(o.p, join(o.loc, key), list), true
}

func parseNodes(p *parser, loc []string, list []any) []Node {
	out := make([]Node, 0, len(list))
	for i, item := range list {
		out = append(out, parseNode(p, join(loc, strconv.Itoa(i)), item))
	}
	return out
}

func (o obj) condition(key string) *Condition {
	v, ok := o.m.Get(key)
	if !ok {
		o.fail(key, "Field required")
		return nil
	}
	return parseCondition(o.p, join(o.loc, key), v)
}

func parseIf(p *parser, loc []string, m *omap.Map) Node {
	o := obj{p: p, loc: loc, m: m}
	o.forbidExtra("if", "then", "else", "name")
	b := &IfBlock{If: o.condition("if")}
	b.Then, _ = o.nodeList("then", true)
	b.Else, b.HasElse = o.nodeList("else", false)
	b.Name = o.strOr("name")
	return b
}

func parseTry(p *parser, loc []string, m *omap.Map) Node {
	o := obj{p: p, loc: loc, m: m}
	start := len(p.errs)
	o.forbidExtra("try", "catch", "finally", "name")
	b := &TryBlock{}
	b.Try, _ = o.nodeList("try", true)
	b.Catch, b.HasCatch = o.nodeList("catch", false)
	b.Finally, b.HasFinally = o.nodeList("finally", false)
	b.Name = o.strOr("name")
	if len(p.errs) == start && !b.HasCatch && !b.HasFinally {
		p.valueError(loc, "'try' requires 'catch' or 'finally'")
	}
	return b
}

func parseForEach(p *parser, loc []string, m *omap.Map) Node {
	o := obj{p: p, loc: loc, m: m}
	start := len(p.errs)
	o.forbidExtra("forEach", "as", "do", "name")
	b := &ForEachBlock{AsName: "item"}
	switch t := func() any { v, _ := m.Get("forEach"); return v }().(type) {
	case []any, string:
		b.Items = t
	default:
		o.fail("forEach", "Input should be a valid list")
	}
	if v, ok := m.Get("as"); ok {
		if s, isStr := v.(string); isStr {
			b.AsName = s
		} else {
			o.fail("as", "Input should be a valid string")
		}
	}
	b.Do, _ = o.nodeList("do", true)
	b.Name = o.strOr("name")
	if len(p.errs) == start && !nameRe.MatchString(b.AsName) {
		p.valueError(loc, "invalid variable name: "+omap.Repr(b.AsName))
	}
	return b
}

func parseRepeat(p *parser, loc []string, m *omap.Map) Node {
	o := obj{p: p, loc: loc, m: m}
	o.forbidExtra("repeat", "do", "name")
	b := &RepeatBlock{}
	v, _ := m.Get("repeat")
	switch t := v.(type) {
	case int, string:
		b.Count = t
	case float64:
		if n, msg := intOf(t); msg == "" {
			b.Count = n
		} else {
			o.fail("repeat", msg)
		}
	default:
		o.fail("repeat", "Input should be a valid integer")
	}
	b.Do, _ = o.nodeList("do", true)
	b.Name = o.strOr("name")
	return b
}

func parseWhile(p *parser, loc []string, m *omap.Map) Node {
	o := obj{p: p, loc: loc, m: m}
	o.forbidExtra("while", "max", "do", "name")
	one := 1
	b := &WhileBlock{While: o.condition("while"), Max: o.intOr("max", 100, &one)}
	b.Do, _ = o.nodeList("do", true)
	b.Name = o.strOr("name")
	return b
}

func parseSession(p *parser, loc []string, v any) SessionConfig {
	s := SessionConfig{Headless: true}
	o, ok := p.object(loc, v, "SessionConfig", false)
	if !ok {
		return s
	}
	o.forbidExtra("headless", "profile", "profileDir", "userAgent", "proxy", "viewport", "timeout",
		"preserveSession", "fingerprint")
	s.Headless = o.boolOr("headless", true)
	s.Profile = o.strOr("profile")
	s.ProfileDir = o.strOr("profileDir")
	s.UserAgent = o.strOr("userAgent")
	s.Proxy = o.strOr("proxy")
	if vv, ok := o.get("viewport"); ok {
		if vo, ok := p.object(join(loc, "viewport"), vv, "ViewportConfig", false); ok {
			vo.forbidExtra("width", "height")
			vo.required("width")
			vo.required("height")
			w, h := vo.integer("width", nil), vo.integer("height", nil)
			if w != nil && h != nil {
				s.Viewport = &Viewport{Width: *w, Height: *h}
			}
		}
	}
	s.Timeout = o.integer("timeout", nil)
	s.PreserveSession = o.boolOr("preserveSession", false)
	switch t := func() any { v, _ := o.get("fingerprint"); return v }().(type) {
	case nil:
	case bool:
		s.Fingerprint = t
	default:
		fo, ok := p.object(join(loc, "fingerprint"), t, "FingerprintOverrides", false)
		if !ok {
			break
		}
		fo.forbidExtra("seed", "canvas", "webgl", "webglVendor", "webglRenderer", "audio", "clientRects", "fonts")
		s.Fingerprint = &FingerprintOverrides{
			Seed: fo.integer("seed", nil), Canvas: fo.optBool("canvas"), Webgl: fo.optBool("webgl"),
			WebglVendor: fo.str("webglVendor"), WebglRenderer: fo.str("webglRenderer"),
			Audio: fo.optBool("audio"), ClientRects: fo.optBool("clientRects"), Fonts: fo.optBool("fonts"),
		}
	}
	return s
}

// buildScript validates a root mapping (includes already expanded).
func buildScript(data *omap.Map) (*Script, error) {
	p := &parser{}
	o := obj{p: p, m: data}
	o.forbidExtra("session", "steps", "output", "stopOnError", "vars", "errorArtifacts")
	sc := &Script{StopOnError: true, Vars: omap.New()}
	if v, ok := data.Get("session"); ok {
		sc.Session = parseSession(p, []string{"session"}, v)
	} else {
		sc.Session = SessionConfig{Headless: true}
	}
	sc.Steps, _ = o.nodeList("steps", true)
	if v, ok := data.Get("output"); ok {
		if oo, ok := p.object([]string{"output"}, v, "ScriptOutput", false); ok {
			oo.forbidExtra("file")
			sc.OutputFile = oo.strOr("file")
		}
	}
	sc.StopOnError = o.boolOr("stopOnError", true)
	if v, ok := data.Get("vars"); ok {
		if vars, isMap := v.(*omap.Map); isMap {
			sc.Vars = vars
		} else {
			o.fail("vars", "Input should be a valid dictionary")
		}
	}
	sc.ErrorArtifacts = o.strOr("errorArtifacts")
	if len(p.errs) > 0 {
		return nil, &ValidationError{Model: "ScriptDefinition", Errors: p.errs}
	}
	return sc, nil
}
