package scenario

import (
	"regexp"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

var actionNames = []string{
	"open", "wait", "find", "click", "fill", "type", "press", "scroll", "screenshot", "get-content",
	"get-cookies", "get-headers", "save", "browse-tabs", "solve-challenge", "hover", "dblclick", "focus",
	"clear", "check", "uncheck", "submit", "select", "upload", "evaluate", "history", "wait-text",
	"extract", "find-all", "read", "mouse-move", "mouse-scroll-to", "mouse-random", "set", "assert", "fail",
}

var (
	knownActions = setOf(actionNames...)
	waitStates   = setOf("load", "domcontentloaded", "networkidle")
	// value actions can store their result in a variable with "as"
	valueActions = setOf("extract", "evaluate", "read", "find-all", "get-content", "get-cookies", "get-headers")
	// shorthand value must be null
	shorthandNull = setOf("get-content", "get-cookies", "get-headers", "browse-tabs", "solve-challenge",
		"mouse-move", "mouse-random")
	// shorthand value is the locator
	locatorActions = setOf("find", "click", "hover", "dblclick", "focus", "clear", "check", "uncheck",
		"submit", "extract", "find-all", "read", "mouse-scroll-to")
	nameRe = regexp.MustCompile(`^[A-Za-z_][\p{L}\p{N}_]*$`)
)

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

func isInt(v any) bool {
	_, ok := v.(int)
	return ok
}

// expandShorthand turns {open: url} style steps into canonical {action: open, url: ...}.
func expandShorthand(d *omap.Map) (*omap.Map, error) {
	if d.Has("action") {
		return d, nil
	}
	verb := ""
	for _, k := range d.Keys() {
		if knownActions[k] {
			verb = k
			break
		}
	}
	if verb == "" {
		return d, nil
	}
	value, _ := d.Get(verb)
	result := omap.New()
	for _, k := range d.Keys() {
		if k != verb {
			v, _ := d.Get(k)
			result.Set(k, v)
		}
	}
	result.Set("action", verb)

	set := func(field string, v any) error {
		if result.Has(field) {
			return shorthandErr("shorthand conflict: cannot specify both shorthand value and explicit '" + field + "'")
		}
		result.Set(field, v)
		return nil
	}
	pair := func(what string) ([]any, error) {
		list, ok := value.([]any)
		if !ok || len(list) != 2 {
			return nil, shorthandErr("'" + verb + "' shorthand expects a 2-element list " + what + ", got: " + omap.Repr(value))
		}
		return list, nil
	}

	switch {
	case verb == "open":
		return result, set("url", value)
	case verb == "wait":
		if isInt(value) {
			return result, set("timeout", value)
		}
		if s, ok := value.(string); ok && waitStates[s] {
			return result, set("waitUntil", value)
		}
		return nil, shorthandErr("wait shorthand expects int (ms) or load-state string, got: " + omap.Repr(value))
	case locatorActions[verb]:
		return result, set("locator", value)
	case verb == "fill" || verb == "type":
		list, err := pair("[locator, value]")
		if err != nil {
			return nil, err
		}
		if err := set("locator", list[0]); err != nil {
			return nil, err
		}
		return result, set("value", list[1])
	case verb == "press":
		return result, set("key", value)
	case verb == "select":
		list, err := pair("[locator, option]")
		if err != nil {
			return nil, err
		}
		if err := set("locator", list[0]); err != nil {
			return nil, err
		}
		if _, isList := list[1].([]any); isList {
			return result, set("options", list[1])
		}
		return result, set("value", list[1])
	case verb == "upload":
		list, err := pair("[locator, file(s)]")
		if err != nil {
			return nil, err
		}
		if err := set("locator", list[0]); err != nil {
			return nil, err
		}
		if _, isList := list[1].([]any); isList {
			return result, set("files", list[1])
		}
		return result, set("files", []any{list[1]})
	case verb == "evaluate":
		return result, set("script", value)
	case verb == "history":
		return result, set("direction", value)
	case verb == "wait-text":
		return result, set("text", value)
	case verb == "fail":
		return result, set("message", value)
	case verb == "assert":
		return result, set("condition", value)
	case verb == "set":
		return result, set("vars", value)
	case verb == "screenshot" || verb == "save":
		return result, set("file", value)
	case shorthandNull[verb]:
		if value != nil {
			return nil, shorthandErr("'" + verb + "' shorthand expects null value, got: " + omap.Repr(value))
		}
	}
	return result, nil
}

type shorthandErr string

func (e shorthandErr) Error() string { return string(e) }

// ParseStep validates one step mapping (shorthand allowed). The substituted
// form of a step goes through here again.
func ParseStep(raw any) (*Step, error) {
	p := &parser{}
	s := parseStep(p, nil, raw)
	if len(p.errs) > 0 {
		return nil, &ValidationError{Model: "ScriptStep", Errors: p.errs}
	}
	return s, nil
}

func parseStep(p *parser, loc []string, v any) *Step {
	m, ok := v.(*omap.Map)
	if !ok {
		p.add(loc, "Input should be a valid dictionary or instance of ScriptStep")
		return nil
	}
	expanded, err := expandShorthand(m)
	if err != nil {
		p.valueError(loc, err.Error())
		return nil
	}
	o := obj{p: p, loc: loc, m: expanded, coerce: true}
	start := len(p.errs)
	o.forbidExtra("action", "page", "url", "locator", "value", "key", "format", "waitUntil", "timeout",
		"deltaX", "deltaY", "delay", "index", "fullPage", "file", "stopOnError", "options", "files", "script",
		"direction", "text", "gone", "attribute", "all", "x", "y", "steps", "startX", "startY", "endX", "endY",
		"durationMs", "jitter", "seed", "as", "vars", "condition", "message", "name", "retry", "retryDelay",
		"optional")
	zero := 0
	s := &Step{}
	o.required("action")
	if v, ok := o.m.Get("action"); ok && v == nil {
		o.fail("action", "Input should be a valid string")
	}
	if a := o.literal("action", actionNames...); a != nil {
		s.Action = *a
	}
	s.Page = o.strOr("page")
	s.URL = o.strOr("url")
	s.Locator = o.locator("locator")
	s.Value = o.str("value")
	s.Key = o.strOr("key")
	if f := o.literal("format", "html", "text"); f != nil {
		s.Format = *f
	}
	if w := o.literal("waitUntil", "domcontentloaded", "load", "networkidle", "visible", "hidden"); w != nil {
		s.WaitUntil = *w
	}
	s.Timeout = o.integer("timeout", nil)
	s.DeltaX = o.integer("deltaX", nil)
	s.DeltaY = o.integer("deltaY", nil)
	s.Delay = o.integer("delay", nil)
	s.Index = o.integer("index", nil)
	s.FullPage = o.boolOr("fullPage", false)
	s.File = o.strOr("file")
	s.StopOnError = o.optBool("stopOnError")
	s.Options = o.strList("options")
	s.Files = o.strList("files")
	s.Script = o.strOr("script")
	if d := o.literal("direction", "back", "forward", "reload"); d != nil {
		s.Direction = *d
	}
	s.Text = o.strOr("text")
	s.Gone = o.boolOr("gone", false)
	s.Attribute = o.strOr("attribute")
	s.All = o.boolOr("all", false)
	s.X = o.float("x")
	s.Y = o.float("y")
	s.MoveSteps = o.integer("steps", nil)
	s.StartX = o.floatOr("startX", 0)
	s.StartY = o.floatOr("startY", 0)
	s.EndX = o.float("endX")
	s.EndY = o.float("endY")
	s.DurationMs = o.integer("durationMs", nil)
	s.Jitter = o.float("jitter")
	s.Seed = o.integer("seed", nil)
	s.AsName = o.str("as")
	s.Vars = o.dict("vars")
	if cv, ok := o.get("condition"); ok {
		s.Condition = parseCondition(p, join(loc, "condition"), cv)
	}
	s.Message = o.strOr("message")
	s.Name = o.strOr("name")
	s.Retry = o.intOr("retry", 0, &zero)
	s.RetryDelay = o.intOr("retryDelay", 1000, &zero)
	s.Optional = o.boolOr("optional", false)
	if len(p.errs) > start {
		return nil
	}
	if msg := s.validatePayload(); msg != "" {
		p.valueError(loc, msg)
		return nil
	}
	s.Raw = expanded.Copy()
	if s.Locator != nil {
		s.Raw.Set("locator", s.Locator.Dump())
	}
	if s.Condition != nil {
		s.Raw.Set("condition", s.Condition.Raw)
	}
	return s
}

// validatePayload is validate_action_payload; it returns the error text or "".
func (s *Step) validatePayload() string {
	a := s.Action
	switch {
	case a == "open" && s.URL == "":
		return "'open' step requires 'url'"
	case (locatorActions[a] || a == "fill" || a == "type" || a == "select" || a == "upload") && s.Locator == nil:
		return "'" + a + "' step requires 'locator'"
	case a == "select" && (s.Value == nil) == (len(s.Options) == 0):
		return "'select' step requires exactly one of 'value' or 'options'"
	case a == "upload" && len(s.Files) == 0:
		return "'upload' step requires 'files'"
	case a == "evaluate" && s.Script == "":
		return "'evaluate' step requires 'script'"
	case a == "history" && s.Direction == "":
		return "'history' step requires 'direction'"
	case a == "wait-text" && s.Text == "":
		return "'wait-text' step requires 'text'"
	case a == "mouse-move" && (s.X == nil || s.Y == nil):
		return "'mouse-move' step requires 'x' and 'y'"
	case a == "mouse-random" && (s.EndX == nil || s.EndY == nil):
		return "'mouse-random' step requires 'endX' and 'endY'"
	case (a == "fill" || a == "type") && s.Value == nil:
		return "'" + a + "' step requires 'value'"
	case a == "wait" && (s.WaitUntil == "visible" || s.WaitUntil == "hidden") && s.Locator == nil:
		return "'wait' with waitUntil '" + s.WaitUntil + "' requires 'locator'"
	case a == "press" && s.Key == "":
		return "'press' step requires 'key'"
	case a == "scroll" && s.DeltaX == nil && s.DeltaY == nil:
		return "'scroll' step requires 'deltaX' or 'deltaY'"
	case (a == "save" || a == "screenshot") && s.File == "":
		return "'" + a + "' step requires 'file'"
	case a == "set" && (s.Vars == nil || s.Vars.Len() == 0):
		return "'set' step requires 'vars'"
	case a == "fail" && s.Message == "":
		return "'fail' step requires 'message'"
	case a == "assert" && s.Condition == nil:
		return "'assert' step requires 'condition'"
	}
	if s.AsName != nil {
		if !valueActions[a] {
			return "'as' is not supported by '" + a + "'"
		}
		if !nameRe.MatchString(*s.AsName) {
			return "invalid variable name: " + omap.Repr(*s.AsName)
		}
	}
	return ""
}
