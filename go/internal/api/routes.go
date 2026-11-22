package api

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// Request models (the pydantic classes of app.py).
var (
	locatorModel = []field{{name: "type", kind: kStr, def: "css"}, {name: "value", kind: kStr, required: true}}
	locatorField = field{name: "locator", kind: kModel, required: true, model: locatorModel}
	timeout10s   = field{name: "timeout", kind: kInt, def: 10000}
	timeout30s   = field{name: "timeout", kind: kInt, def: 30000}

	startModel = []field{
		{name: "urls", kind: kStrList, def: []any{}},
		{name: "profile", kind: kStr, nullable: true},
		{name: "userAgent", kind: kStr, nullable: true},
		{name: "headless", kind: kBool, nullable: true},
		{name: "viewport", kind: kModel, nullable: true, model: []field{
			{name: "width", kind: kInt, required: true}, {name: "height", kind: kInt, required: true}}},
		{name: "timeout", kind: kInt, nullable: true},
	}
	elementModel = []field{locatorField, timeout10s}
	clickModel   = []field{locatorField, timeout10s, {name: "double", kind: kBool, def: false}}
	fillModel    = []field{locatorField, {name: "value", kind: kStr, required: true},
		{name: "clearBefore", kind: kBool, def: false}, timeout10s}
	selectModel = []field{locatorField, {name: "option", kind: kStrOrStrList, required: true}, timeout10s}
	checkModel  = []field{locatorField, {name: "checked", kind: kBool, def: true}, timeout10s}
	waitModel   = []field{
		{name: "locator", kind: kModel, nullable: true, model: locatorModel},
		{name: "state", kind: kStr, def: "visible"},
		{name: "waitUntil", kind: kStr, nullable: true},
		timeout10s,
	}
	pressModel     = []field{{name: "key", kind: kStr, required: true}}
	mouseMoveModel = []field{{name: "x", kind: kFloat, required: true}, {name: "y", kind: kFloat, required: true},
		{name: "steps", kind: kInt, def: 1}}
	scrollModel      = []field{{name: "deltaX", kind: kFloat, def: 0.0}, {name: "deltaY", kind: kFloat, def: 0.0}}
	mouseRandomModel = []field{
		{name: "startX", kind: kFloat, required: true}, {name: "startY", kind: kFloat, required: true},
		{name: "endX", kind: kFloat, required: true}, {name: "endY", kind: kFloat, required: true},
		{name: "steps", kind: kInt, def: 25}, {name: "durationMs", kind: kInt, def: 300},
		{name: "jitter", kind: kFloat, def: 3.0}, {name: "seed", kind: kInt, nullable: true},
	}
	chainModel  = []field{{name: "steps", kind: kDictList, required: true}, timeout10s}
	scriptModel = []field{{name: "script", kind: kDictOrStr, required: true},
		{name: "vars", kind: kDict, nullable: true}, {name: "pageId", kind: kStr, nullable: true}}
	navigateModel = []field{{name: "url", kind: kStr, required: true},
		{name: "waitUntil", kind: kStr, def: "load"}, timeout30s}
	historyModel = []field{{name: "direction", kind: kStr, required: true, pattern: "^(back|forward|reload)$"},
		timeout30s}
	evaluateModel   = []field{{name: "expression", kind: kStr, required: true}, {name: "isolated", kind: kBool, def: false}}
	screenshotModel = []field{{name: "fullPage", kind: kBool, def: false},
		{name: "locator", kind: kModel, nullable: true, model: locatorModel}, timeout10s}
	waitTextModel = []field{{name: "text", kind: kStr, required: true}, {name: "gone", kind: kBool, def: false},
		timeout10s}
	uploadModel = []field{locatorField, {name: "files", kind: kStrList, required: true}, timeout10s}
	dialogModel = []field{{name: "accept", kind: kBool, required: true},
		{name: "promptText", kind: kStr, nullable: true}}
	newTabModel  = []field{{name: "url", kind: kStr, nullable: true}}
	profileModel = []field{{name: "name", kind: kStr, required: true}}

	pageIDQuery = field{name: "page_id", kind: kStr, nullable: true}
)

const sessionPath = "/api/v1/session/{session_id}"
const pagePath = sessionPath + "/pages/{page_id}"

func (s *Server) table() []route {
	el := func(path string, body []field, fn handler) route { return s.add("POST", pagePath+path, body, nil, fn) }
	return []route{
		{method: "GET", parts: []string{"", "openapi.json"}, raw: serveOpenAPI},
		{method: "GET", parts: []string{"", "docs"}, raw: serveHTML(swaggerHTML)},
		{method: "GET", parts: []string{"", "docs", "oauth2-redirect"}, raw: serveHTML(oauth2RedirectHTML)},
		{method: "GET", parts: []string{"", "redoc"}, raw: serveHTML(redocHTML)},
		s.add("GET", "/health", nil, nil, func(*request) (any, error) { return omap.New("status", "ok"), nil }),
		s.add("GET", "/ready", nil, nil, func(*request) (any, error) { return omap.New("status", "ready"), nil }),
		s.add("GET", "/api/v1/info", nil, nil, s.info),
		s.add("POST", "/api/v1/profile/create", profileModel, nil, s.createProfile),
		s.add("GET", "/api/v1/profiles", nil, nil, s.listProfiles),
		s.add("POST", "/api/v1/browser/start", startModel, nil, s.browserStart),
		s.add("GET", "/api/v1/sessions", nil, nil, func(*request) (any, error) {
			return omap.New("success", true, "sessions", s.manager.List()), nil
		}),
		s.add("GET", sessionPath, nil, nil, s.getSession),
		s.add("POST", sessionPath+"/stop", nil, nil, s.stopSession),
		s.add("GET", sessionPath+"/cookies", nil, nil, s.cookies),
		s.add("GET", sessionPath+"/headers", nil, []field{pageIDQuery}, s.headers),
		s.add("GET", sessionPath+"/content", nil, []field{pageIDQuery, {name: "fmt", kind: kStr, def: "html"}}, s.content),
		el("/elements/find", elementModel, s.find),
		el("/elements/find-all", elementModel, s.stepOp("find-all", "find-all", true)),
		el("/elements/read", elementModel, s.stepOp("read", "read", true)),
		el("/elements/click", clickModel, s.click),
		el("/elements/hover", elementModel, s.stepOp("hover", "hover", false)),
		el("/elements/focus", elementModel, s.stepOp("focus", "focus", false)),
		el("/elements/fill", fillModel, s.fill),
		el("/elements/clear", elementModel, s.stepOp("clear", "clear", false)),
		el("/elements/select", selectModel, s.selectOpt),
		el("/elements/check", checkModel, s.check),
		el("/forms/submit", elementModel, s.stepOp("submit", "submit", false)),
		el("/elements/wait", waitModel, s.wait),
		el("/mouse/move", mouseMoveModel, s.mouseMove),
		el("/mouse/move-to", elementModel, s.moveTo),
		el("/mouse/scroll", scrollModel, s.scroll),
		el("/mouse/scroll-to", elementModel, s.stepOp("mouse-scroll-to", "scroll-to", false)),
		el("/mouse/move-random", mouseRandomModel, s.moveRandom),
		el("/keyboard/press", pressModel, s.press),
		el("/actions/run", chainModel, s.actionChain),
		s.add("POST", sessionPath+"/script", scriptModel, nil, s.runScript),
		s.add("GET", sessionPath+"/pages", nil, nil, s.listPages),
		s.add("POST", sessionPath+"/pages", newTabModel, nil, s.newTab),
		s.add("DELETE", pagePath, nil, nil, s.closeTab),
		s.add("GET", pagePath+"/snapshot", nil, []field{{name: "timeout", kind: kInt, def: 10000}}, s.snapshot),
		el("/screenshot", screenshotModel, s.screenshot),
		el("/navigate", navigateModel, s.navigate),
		el("/history", historyModel, s.history),
		el("/evaluate", evaluateModel, s.evaluate),
		el("/wait-text", waitTextModel, s.waitText),
		el("/elements/upload", uploadModel, s.upload),
		s.add("GET", pagePath+"/logs/{kind}", nil, []field{{name: "clear", kind: kBool, def: false}}, s.logs),
		s.add("POST", sessionPath+"/dialog-policy", dialogModel, nil, s.dialogPolicy),
	}
}

// --- body accessors (values are already validated) ---

func (r *request) str(k string) string {
	v, _ := r.body.Get(k)
	s, _ := v.(string)
	return s
}

func (r *request) num(k string) int {
	v, _ := r.body.Get(k)
	n, _ := v.(int)
	return n
}

func (r *request) float(k string) float64 {
	v, _ := r.body.Get(k)
	f, _ := v.(float64)
	return f
}

func (r *request) flag(k string) bool {
	v, _ := r.body.Get(k)
	b, _ := v.(bool)
	return b
}

func (r *request) locator(k string) *scenario.Locator {
	v, _ := r.body.Get(k)
	m, ok := v.(*omap.Map)
	if !ok {
		return nil
	}
	t, _ := m.Get("type")
	val, _ := m.Get("value")
	return &scenario.Locator{Type: t.(string), Value: val.(string)}
}

func (r *request) strs(k string) []string {
	v, _ := r.body.Get(k)
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func ms(t0 time.Time) float64 {
	d := float64(time.Since(t0).Microseconds()) / 1000
	return float64(int64(d*10+0.5)) / 10
}

// --- service, profiles, sessions ---

func (s *Server) info(*request) (any, error) {
	return omap.New("name", "autobrowser-client", "version", s.version,
		"api", omap.New("host", s.settings.API.Host, "port", s.settings.API.Port)), nil
}

func (s *Server) createProfile(r *request) (any, error) {
	name := strings.TrimSpace(r.str("name"))
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return nil, &httpError{400, "Invalid profile name"}
	}
	dir := filepath.Join(s.settings.Profiles.Dir, name)
	if _, err := os.Stat(dir); err == nil {
		return nil, &httpError{409, fmt.Sprintf("Profile '%s' already exists", name)}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, &internalError{err.Error()}
	}
	slog.Info("Created profile: " + name)
	return omap.New("success", true, "profile", name, "path", dir), nil
}

func (s *Server) listProfiles(*request) (any, error) {
	profiles := []any{}
	entries, _ := os.ReadDir(s.settings.Profiles.Dir)
	var names []string
	for _, e := range entries {
		if st, err := os.Stat(filepath.Join(s.settings.Profiles.Dir, e.Name())); err == nil && st.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		profiles = append(profiles, n)
	}
	return omap.New("success", true, "profiles", profiles), nil
}

func withInfo(sess *browser.Session) *omap.Map {
	m := omap.New("success", true)
	info := sess.Info()
	for _, k := range info.Keys() {
		v, _ := info.Get(k)
		m.Set(k, v)
	}
	return m
}

func (s *Server) browserStart(r *request) (any, error) {
	opts := browser.Options{URLs: r.strs("urls"), Profile: r.str("profile"), UserAgent: r.str("userAgent")}
	if v, _ := r.body.Get("headless"); v != nil {
		h := v.(bool)
		opts.Headless = &h
	}
	if v, _ := r.body.Get("viewport"); v != nil {
		w, _ := v.(*omap.Map).Get("width")
		h, _ := v.(*omap.Map).Get("height")
		opts.Viewport = &config.ViewportSettings{Width: w.(int), Height: h.(int)}
	}
	sess, err := s.manager.Create(opts)
	if err != nil {
		return nil, err
	}
	return withInfo(sess), nil
}

func (s *Server) getSession(r *request) (any, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	return withInfo(sess), nil
}

func (s *Server) stopSession(r *request) (any, error) {
	sess, err := s.manager.Stop(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	return withInfo(sess), nil
}

// call runs fn on the session (session.call): one call at a time.
func (s *Server) call(r *request, fn func(*browser.Session) error) (*browser.Session, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	return sess, sess.Call(func() error { return fn(sess) })
}

func (s *Server) cookies(r *request) (any, error) {
	var cookies []any
	_, err := s.call(r, func(sess *browser.Session) (err error) {
		cookies, err = sess.Cookies()
		return err
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "cookies", cookies), nil
}

func (r *request) queryStr(k string) string {
	v, _ := r.query.Get(k)
	s, _ := v.(string)
	return s
}

func (s *Server) headers(r *request) (any, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"],
		"headers", sess.Headers(r.queryStr("page_id"))), nil
}

func (s *Server) content(r *request) (any, error) {
	var content string
	_, err := s.call(r, func(sess *browser.Session) (err error) {
		content, err = sess.Content(r.queryStr("page_id"), r.queryStr("fmt"))
		return err
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "content", content), nil
}

// --- page operations ---

// pageOp runs fn on the page and answers with the action, its duration and,
// unless withResult is false, its result (_page_op and the element routes).
func (s *Server) pageOp(r *request, action string, withResult bool, fn func(*browser.Session, string) (any, error)) (any, error) {
	pageID := r.params["page_id"]
	var data any
	var took float64
	_, err := s.call(r, func(sess *browser.Session) (err error) {
		t0 := time.Now()
		data, err = fn(sess, pageID)
		took = ms(t0)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := omap.New("success", true, "sessionId", r.params["session_id"], "pageId", pageID,
		"action", action, "durationMs", took)
	if withResult {
		out.Set("result", data)
	}
	return out, nil
}

// step runs one scenario step on the page with the error as raised.
func step(sess *browser.Session, pageID string, st *scenario.Step) (any, error) {
	if _, err := sess.Page(pageID); err != nil {
		return nil, err
	}
	res := sess.ExecuteStep(st, pageID)
	if !res.Success {
		return nil, stepError(res.Error)
	}
	return res.Data, nil
}

type stepError string

func (e stepError) Error() string { return string(e) }

func intp(n int) *int { return &n }

var first = intp(0)

// firstMatch are the step actions that take index 0 to act on the first
// match (the engine resolves a locator strictly); the others use .first.
var firstMatch = map[string]bool{"click": true, "dblclick": true, "hover": true, "focus": true,
	"clear": true, "check": true, "uncheck": true}

// stepOp is an element route done by one step.
func (s *Server) stepOp(stepAction, action string, withResult bool) handler {
	return func(r *request) (any, error) {
		return s.pageOp(r, action, withResult, func(sess *browser.Session, pid string) (any, error) {
			st := &scenario.Step{Action: stepAction, Locator: r.locator("locator"), Timeout: intp(r.num("timeout"))}
			if firstMatch[stepAction] {
				st.Index = first
			}
			return step(sess, pid, st)
		})
	}
}

func (s *Server) find(r *request) (any, error) {
	return s.pageOp(r, "find", true, func(sess *browser.Session, pid string) (any, error) {
		loc := r.locator("locator")
		data, err := step(sess, pid, &scenario.Step{Action: "find", Locator: loc, Timeout: intp(r.num("timeout"))})
		if err != nil {
			return nil, err
		}
		found, _ := data.(*omap.Map).Get("found")
		return omap.New("found", found, "locator", omap.New("type", loc.Type, "value", loc.Value)), nil
	})
}

func (s *Server) click(r *request) (any, error) {
	act := "click"
	if r.flag("double") {
		act = "dblclick"
	}
	return s.pageOp(r, "click", false, func(sess *browser.Session, pid string) (any, error) {
		return step(sess, pid, &scenario.Step{Action: act, Locator: r.locator("locator"), Index: first,
			Timeout: intp(r.num("timeout"))})
	})
}

func (s *Server) fill(r *request) (any, error) {
	return s.pageOp(r, "fill", false, func(sess *browser.Session, pid string) (any, error) {
		to := intp(r.num("timeout"))
		if r.flag("clearBefore") {
			if _, err := step(sess, pid, &scenario.Step{Action: "clear", Locator: r.locator("locator"),
				Index: first, Timeout: to}); err != nil {
				return nil, err
			}
		}
		v := r.str("value")
		return step(sess, pid, &scenario.Step{Action: "fill", Locator: r.locator("locator"), Index: first,
			Value: &v, Timeout: to})
	})
}

func (s *Server) selectOpt(r *request) (any, error) {
	return s.pageOp(r, "select", true, func(sess *browser.Session, pid string) (any, error) {
		st := &scenario.Step{Action: "select", Locator: r.locator("locator"), Timeout: intp(r.num("timeout"))}
		if v, _ := r.body.Get("option"); v != nil {
			if one, ok := v.(string); ok {
				st.Options = []string{one}
			} else {
				st.Options = r.strs("option")
			}
		}
		if st.Options == nil {
			st.Options = []string{}
		}
		return step(sess, pid, st)
	})
}

func (s *Server) check(r *request) (any, error) {
	act := "uncheck"
	if r.flag("checked") {
		act = "check"
	}
	return s.pageOp(r, act, false, func(sess *browser.Session, pid string) (any, error) {
		return step(sess, pid, &scenario.Step{Action: act, Locator: r.locator("locator"), Index: first,
			Timeout: intp(r.num("timeout"))})
	})
}

var loadStates = map[string]bool{"load": true, "domcontentloaded": true, "networkidle": true}

func (s *Server) wait(r *request) (any, error) {
	return s.pageOp(r, "wait", false, func(sess *browser.Session, pid string) (any, error) {
		if loc := r.locator("locator"); loc != nil {
			return nil, sess.WaitFor(pid, *loc, r.str("state"), r.num("timeout"))
		}
		until := r.str("waitUntil")
		if until == "" {
			until = "load"
		}
		if !loadStates[until] {
			return nil, fmt.Errorf("Page.wait_for_load_state: state: expected one of (load|domcontentloaded|networkidle)")
		}
		return step(sess, pid, &scenario.Step{Action: "wait", WaitUntil: until, Timeout: intp(r.num("timeout"))})
	})
}

func (s *Server) mouseMove(r *request) (any, error) {
	return s.pageOp(r, "mouse-move", false, func(sess *browser.Session, pid string) (any, error) {
		x, y := r.float("x"), r.float("y")
		return step(sess, pid, &scenario.Step{Action: "mouse-move", X: &x, Y: &y, MoveSteps: intp(r.num("steps"))})
	})
}

func (s *Server) moveTo(r *request) (any, error) {
	return s.pageOp(r, "mouse-move-to", true, func(sess *browser.Session, pid string) (any, error) {
		res, err := sess.MoveToElement(pid, *r.locator("locator"), r.num("timeout"))
		if err != nil && err.Error() == "Element has no bounding box (not visible?)" {
			return nil, &internalError{err.Error()} // a ValueError in Python
		}
		return res, err
	})
}

func (s *Server) scroll(r *request) (any, error) {
	return s.pageOp(r, "scroll", false, func(sess *browser.Session, pid string) (any, error) {
		return nil, sess.Wheel(pid, r.float("deltaX"), r.float("deltaY"))
	})
}

func (s *Server) moveRandom(r *request) (any, error) {
	return s.pageOp(r, "mouse-move-random", true, func(sess *browser.Session, pid string) (any, error) {
		ex, ey := r.float("endX"), r.float("endY")
		st := &scenario.Step{Action: "mouse-random", StartX: r.float("startX"), StartY: r.float("startY"),
			EndX: &ex, EndY: &ey, MoveSteps: intp(r.num("steps")), DurationMs: intp(r.num("durationMs"))}
		j := r.float("jitter")
		st.Jitter = &j
		if v, _ := r.body.Get("seed"); v != nil {
			st.Seed = intp(v.(int))
		}
		return step(sess, pid, st)
	})
}

func (s *Server) press(r *request) (any, error) {
	return s.pageOp(r, "press", false, func(sess *browser.Session, pid string) (any, error) {
		return step(sess, pid, &scenario.Step{Action: "press", Key: r.str("key")})
	})
}

func (s *Server) runScript(r *request) (any, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	script, _ := r.body.Get("script")
	cwd, _ := os.Getwd()
	sc, err := scenario.Parse(script, cwd)
	if err != nil {
		return nil, err
	}
	opts := engine.Options{PageID: r.str("pageId")}
	if v, _ := r.body.Get("vars"); v != nil {
		opts.Vars = v.(*omap.Map)
	}
	var res *engine.Result
	var took float64
	err = sess.Call(func() (err error) {
		t0 := time.Now()
		res, err = engine.Run(sess, sc, opts)
		took = ms(t0)
		return err
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", res.Success, "sessionId", r.params["session_id"], "steps", res.Steps,
		"vars", res.Vars, "durationMs", took), nil
}

func (s *Server) listPages(r *request) (any, error) {
	var pages *omap.Map
	_, err := s.call(r, func(sess *browser.Session) error {
		pages = sess.ListPages()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "pages", pages), nil
}

func (s *Server) newTab(r *request) (any, error) {
	var pageID string
	_, err := s.call(r, func(sess *browser.Session) (err error) {
		pageID, err = sess.NewTab(r.str("url"))
		return err
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "pageId", pageID), nil
}

func (s *Server) closeTab(r *request) (any, error) {
	_, err := s.call(r, func(sess *browser.Session) error { return sess.CloseTab(r.params["page_id"]) })
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "pageId", r.params["page_id"]), nil
}

func (s *Server) snapshot(r *request) (any, error) {
	timeout, _ := r.query.Get("timeout")
	return s.pageOp(r, "snapshot", true, func(sess *browser.Session, pid string) (any, error) {
		snap, err := sess.Snapshot(pid, timeout.(int))
		if err != nil {
			return nil, err
		}
		return omap.New("url", snap.URL, "title", snap.Title, "snapshot", snap.Snapshot, "truncated", snap.Truncated), nil
	})
}

func (s *Server) screenshot(r *request) (any, error) {
	return s.pageOp(r, "screenshot", true, func(sess *browser.Session, pid string) (any, error) {
		loc := r.locator("locator")
		if loc != nil && (loc.Type == "" || loc.Value == "") {
			loc = nil // Python takes the page when type or value is empty
		}
		png, err := sess.Screenshot(pid, r.flag("fullPage"), loc, r.num("timeout"))
		if err != nil {
			return nil, err
		}
		return omap.New("mimeType", "image/png", "data", base64.StdEncoding.EncodeToString(png)), nil
	})
}

func (s *Server) navigate(r *request) (any, error) {
	return s.pageOp(r, "navigate", true, func(sess *browser.Session, pid string) (any, error) {
		return sess.Navigate(pid, r.str("url"), r.num("timeout"))
	})
}

func (s *Server) history(r *request) (any, error) {
	dir := r.str("direction")
	return s.pageOp(r, dir, true, func(sess *browser.Session, pid string) (any, error) {
		return sess.History(pid, dir, r.num("timeout"))
	})
}

func (s *Server) evaluate(r *request) (any, error) {
	return s.pageOp(r, "evaluate", true, func(sess *browser.Session, pid string) (any, error) {
		v, err := sess.Evaluate(pid, r.str("expression"), r.flag("isolated"))
		if err != nil {
			return nil, err
		}
		return omap.New("result", v), nil
	})
}

func (s *Server) waitText(r *request) (any, error) {
	return s.pageOp(r, "wait-text", true, func(sess *browser.Session, pid string) (any, error) {
		if _, err := step(sess, pid, &scenario.Step{Action: "wait-text", Text: r.str("text"), Gone: r.flag("gone"),
			Timeout: intp(r.num("timeout"))}); err != nil {
			return nil, err
		}
		return omap.New("text", r.str("text"), "gone", r.flag("gone")), nil
	})
}

func (s *Server) upload(r *request) (any, error) {
	files := r.strs("files")
	var missing []string
	for _, f := range files {
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return nil, apperr.Invalid("Files not found: " + strings.Join(missing, ", "))
	}
	return s.pageOp(r, "upload", true, func(sess *browser.Session, pid string) (any, error) {
		if _, err := step(sess, pid, &scenario.Step{Action: "upload", Locator: r.locator("locator"), Files: files,
			Timeout: intp(r.num("timeout"))}); err != nil {
			return nil, err
		}
		out := make([]any, len(files))
		for i, f := range files {
			out[i] = f
		}
		return omap.New("files", out), nil
	})
}

func (s *Server) logs(r *request) (any, error) {
	kind := r.params["kind"]
	if kind != "console" && kind != "network" && kind != "dialogs" {
		return nil, apperr.Invalid("kind must be console, network or dialogs")
	}
	clear, _ := r.query.Get("clear")
	var items []any
	_, err := s.call(r, func(sess *browser.Session) (err error) {
		items, err = sess.Log(r.params["page_id"], kind, clear.(bool))
		return err
	})
	if err != nil {
		return nil, err
	}
	return omap.New("success", true, "sessionId", r.params["session_id"], "pageId", r.params["page_id"],
		"kind", kind, "items", items), nil
}

func (s *Server) dialogPolicy(r *request) (any, error) {
	sess, err := s.manager.Get(r.params["session_id"])
	if err != nil {
		return nil, err
	}
	var text *string
	if v, _ := r.body.Get("promptText"); v != nil {
		t := v.(string)
		text = &t
	}
	sess.SetDialogPolicy(r.flag("accept"), text)
	return omap.New("success", true, "sessionId", r.params["session_id"], "dialogPolicy", sess.DialogPolicy()), nil
}
