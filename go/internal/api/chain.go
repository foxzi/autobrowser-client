package api

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// chainLocatorActions are the chain actions that are one engine step with a locator.
var chainLocatorActions = map[string]bool{"find-all": true, "read": true, "click": true, "dblclick": true,
	"hover": true, "focus": true, "clear": true, "check": true, "uncheck": true, "submit": true}

// escapeVars makes chain strings literal: the engine must not substitute ${...}.
func escapeVars(v any) any {
	switch t := v.(type) {
	case string:
		return strings.ReplaceAll(t, "${", "$${")
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = escapeVars(x)
		}
		return out
	case *omap.Map:
		out := omap.New()
		for _, k := range t.Keys() {
			x, _ := t.Get(k)
			out.Set(k, escapeVars(x))
		}
		return out
	}
	return v
}

func get(m *omap.Map, k string, def any) any {
	if v, ok := m.Get(k); ok {
		return v
	}
	return def
}

// truthy is Python's bool(v) for JSON values.
func truthy(v any) bool {
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

// pyStr is str(v) for the JSON values a chain step may hold.
func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return t
	}
	return omap.Marshal(v, 0)
}

// chainLocator is the {type, value} locator of a chain step.
func chainLocator(st *omap.Map) *omap.Map {
	loc, _ := st.Get("locator")
	m, ok := loc.(*omap.Map)
	if !ok {
		m = omap.New()
	}
	return omap.New("type", get(m, "type", "css"), "value", get(m, "value", ""))
}

// chainSteps are the engine steps of a chain step (_chain_steps); nil keeps
// the legacy code path.
func chainSteps(st *omap.Map, timeout any) []any {
	action := get(st, "action", "")
	locator := chainLocator(st)
	base := func(kv ...any) *omap.Map {
		return omap.New(append([]any{"timeout", timeout, "stopOnError", true}, kv...)...)
	}
	name, _ := action.(string)
	switch {
	case name == "wait":
		if v, _ := locator.Get("value"); truthy(v) {
			state := get(st, "state", "visible")
			if state != "visible" && state != "hidden" {
				return nil
			}
			return []any{base("action", "wait", "locator", locator, "waitUntil", state, "index", 0)}
		}
		until := get(st, "waitUntil", "load")
		if until != "load" && until != "domcontentloaded" && until != "networkidle" {
			return nil
		}
		return []any{base("action", "wait", "waitUntil", until)}
	case chainLocatorActions[name]:
		if firstMatch[name] {
			return []any{base("action", name, "locator", locator, "index", 0)}
		}
		return []any{base("action", name, "locator", locator)}
	case name == "fill":
		fill := base("action", "fill", "locator", locator, "value", get(st, "value", ""), "index", 0)
		if truthy(get(st, "clearBefore", false)) {
			return []any{base("action", "clear", "locator", locator, "index", 0), fill}
		}
		return []any{fill}
	case name == "select":
		option := get(st, "option", []any{})
		if _, ok := option.(string); ok {
			return []any{base("action", "select", "locator", locator, "value", option)}
		}
		return []any{base("action", "select", "locator", locator, "options", option)}
	case name == "press":
		return []any{base("action", "press", "key", get(st, "key", "Enter"))}
	case name == "scroll":
		return []any{base("action", "scroll", "deltaX", get(st, "deltaX", 0), "deltaY", get(st, "deltaY", 0))}
	case name == "mouse-move":
		return []any{base("action", "mouse-move", "x", get(st, "x", 0), "y", get(st, "y", 0),
			"steps", get(st, "steps", 1))}
	case name == "mouse-move-random":
		m := base("action", "mouse-random", "endX", 0, "endY", 0)
		for _, k := range []string{"startX", "startY", "endX", "endY", "steps", "durationMs", "jitter", "seed"} {
			if v, ok := st.Get(k); ok {
				m.Set(k, v)
			}
		}
		return []any{m}
	}
	return nil
}

// legacyStep runs the chain steps the engine cannot express: find, wait for
// attached/detached or another load state, unknown actions.
func legacyStep(sess *browser.Session, pageID string, st *omap.Map, timeout any, result *omap.Map) error {
	action := get(st, "action", "")
	locator := chainLocator(st)
	typ, _ := locator.Get("type")
	value, _ := locator.Get("value")
	msec, ok := timeout.(int)
	if !ok {
		return fmt.Errorf("timeout must be an integer, got %s", pyStr(timeout))
	}
	loc := scenario.Locator{Type: pyStr(typ), Value: pyStr(value)}
	switch {
	case action == "find":
		data, err := step(sess, pageID, &scenario.Step{Action: "find", Locator: &loc, Timeout: &msec})
		if err != nil {
			return err
		}
		found, _ := data.(*omap.Map).Get("found")
		result.Set("data", omap.New("found", found, "locator", locator))
	case action == "wait" && truthy(value):
		return sess.WaitFor(pageID, loc, pyStr(get(st, "state", "visible")), msec)
	case action == "wait":
		// only load states the engine lacks get here: Playwright rejects them
		return fmt.Errorf("Page.wait_for_load_state: state: expected one of (load|domcontentloaded|networkidle)")
	default:
		result.Set("success", false)
		result.Set("error", "Unknown action: "+pyStr(action))
	}
	return nil
}

// runChain is run_action_chain: each step goes through the scenario engine
// and gives a flat result.
func runChain(sess *browser.Session, pageID string, steps []any, defaultTimeout int) []any {
	results := []any{}
	cwd, _ := os.Getwd()
	for _, raw := range steps {
		st := raw.(*omap.Map)
		action := get(st, "action", "")
		timeout := get(st, "timeout", defaultTimeout)
		t0 := time.Now()
		result := omap.New("action", action, "success", true)
		var err error
		if engineSteps := chainSteps(st, timeout); engineSteps == nil {
			err = legacyStep(sess, pageID, st, timeout, result)
		} else {
			var script *scenario.Script
			script, err = scenario.Parse(omap.New("steps", escapeVars(engineSteps)), cwd)
			if err == nil {
				var res *engine.Result
				res, err = engine.Run(sess, script, engine.Options{PageID: pageID})
				if err == nil {
					node := res.Steps[len(res.Steps)-1].(*omap.Map)
					for _, n := range res.Steps {
						if ok, _ := n.(*omap.Map).Get("success"); ok != true {
							node = n.(*omap.Map)
							break
						}
					}
					ok, _ := node.Get("success")
					result.Set("success", ok)
					for _, k := range []string{"data", "error"} {
						if v, has := node.Get(k); has {
							result.Set(k, v)
						}
					}
				}
			}
		}
		if err != nil {
			result.Set("success", false)
			result.Set("error", strings.TrimPrefix(err.Error(), ": "))
		}
		result.Set("durationMs", ms(t0))
		results = append(results, result)
		if ok, _ := result.Get("success"); ok != true && truthy(get(st, "stopOnError", true)) {
			break
		}
	}
	return results
}

func (s *Server) actionChain(r *request) (any, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	steps, _ := r.body.Get("steps")
	pageID := r.params["page_id"]
	var results []any
	var took float64
	_ = sess.Call(func() error {
		t0 := time.Now()
		results = runChain(sess, pageID, steps.([]any), r.num("timeout"))
		took = ms(t0)
		return nil
	})
	allOK := true
	for _, x := range results {
		if ok, _ := x.(*omap.Map).Get("success"); ok != true {
			allOK = false
		}
	}
	return omap.New("success", allOK, "sessionId", r.params["session_id"], "pageId", pageID,
		"action", "chain", "durationMs", took, "results", results), nil
}
