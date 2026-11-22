package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func TestMaskHeaders(t *testing.T) {
	h := omap.New("Authorization", "Bearer 123456789", "cookie", "a=1", "x", "plain")
	got := omap.Marshal(maskHeaders(h), 0)
	if got != `{"Authorization": "Bearer 1***", "cookie": "****", "x": "plain"}` {
		t.Errorf("mask: %s", got)
	}
}

func TestJSNumber(t *testing.T) {
	if v := jsNumber(-1); v != -1 {
		t.Errorf("-1: %#v", v)
	}
	if v := jsNumber(1.5); v != 1.5 {
		t.Errorf("1.5: %#v", v)
	}
}

// runner runs steps on a fresh session and fails the test on a failed step.
func runner(t *testing.T, s *Session) (run func(action string, kv ...any) *engine.StepResult, ok func(*engine.StepResult) string) {
	run = func(action string, kv ...any) *engine.StepResult {
		t.Helper()
		return s.ExecuteStep(step(t, action, kv...), "")
	}
	ok = func(res *engine.StepResult) string {
		t.Helper()
		if !res.Success {
			t.Fatalf("%s failed: %s", res.Action, res.Error)
		}
		if res.Data == nil {
			return ""
		}
		return omap.Marshal(res.Data, 0)
	}
	return run, ok
}

func TestCookiesAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r" {
			http.Redirect(w, r, "/p#frag", http.StatusFound)
			return
		}
		w.Header().Add("Set-Cookie", "sid=abcdefghijkl; Path=/")
		w.Header().Add("Set-Cookie", "b=1; Path=/; HttpOnly; SameSite=Strict")
		w.Header().Set("X-Zed", "1")
		fmt.Fprint(w, "<title>t</title>")
	}))
	defer srv.Close()
	s := startSession(t)
	run, ok := runner(t, s)

	ok(run("open", "url", "data:text/html,<p>x</p>"))
	if got := ok(run("get-headers")); got != `{"headers": []}` {
		t.Errorf("data: URL must not be recorded: %s", got)
	}
	ok(run("open", "url", srv.URL+"/r"))
	got := ok(run("get-cookies"))
	want := `{"cookies": [{"name": "sid", "value": "abcdefghijkl", "domain": "127.0.0.1", "path": "/", "expires": -1, ` +
		`"httpOnly": false, "secure": false, "sameSite": "Lax"}, {"name": "b", "value": "1", "domain": "127.0.0.1", ` +
		`"path": "/", "expires": -1, "httpOnly": true, "secure": false, "sameSite": "Strict"}]}`
	if got != want {
		t.Errorf("cookies:\n%s\n%s", got, want)
	}
	res := run("get-headers")
	ok(res)
	list, _ := res.Data.(*omap.Map).Get("headers")
	entries := list.([]any)
	if len(entries) != 2 {
		t.Fatalf("headers: %s", omap.Marshal(list, 0))
	}
	first, second := entries[0].(*omap.Map), entries[1].(*omap.Map)
	if u, _ := first.Get("url"); u != srv.URL+"/r" {
		t.Errorf("redirect url %v", u)
	}
	if st, _ := first.Get("statusCode"); st != 302 {
		t.Errorf("redirect status %v", st)
	}
	if u, _ := second.Get("url"); u != srv.URL+"/p" {
		t.Errorf("url %v", u)
	}
	resp, _ := second.Get("response")
	if v, _ := resp.(*omap.Map).Get("x-zed"); v != "1" {
		t.Errorf("response headers %s", omap.Marshal(resp, 0))
	}
	req, _ := second.Get("request")
	if v, _ := req.(*omap.Map).Get("user-agent"); !strings.Contains(fmt.Sprint(v), "Mozilla") {
		t.Errorf("request headers %s", omap.Marshal(req, 0))
	}
	if h := s.Headers("page_9"); len(h) != 0 {
		t.Errorf("unknown page: %v", h)
	}
}

const formPage = `<html><body>
<form id=f action="/done"><input name=q value=v1><button id=b type=button>b</button></form>
<select id=s multiple><option value=a>A</option><option value=b>Bee</option><option value=c>C</option></select>
<select id=one><option value=x>X</option><option value=y>Y</option></select>
<input type=file id=up multiple>
<button id=pick onclick="document.getElementById('up').click()">pick</button>
<input type=checkbox id=cb checked>
<div id=d>Hello <b>world</b></div>
<div id=hid style="display:none">h</div>
<div style="height:3000px"></div><p id=far>far</p>
</body></html>`

func css(v string) *omap.Map { return omap.New("type", "css", "value", v) }

// Expected values come from the Python client (patchright) on the same page.
func TestFormActions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/done" {
			fmt.Fprint(w, "done "+r.URL.RawQuery)
			return
		}
		fmt.Fprint(w, formPage)
	}))
	defer srv.Close()
	dir := t.TempDir()
	up := filepath.Join(dir, "up.txt")
	other := filepath.Join(dir, "page.html")
	for _, f := range []string{up, other} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := startSession(t)
	run, ok := runner(t, s)
	fails := func(res *engine.StepResult, want string) {
		t.Helper()
		if res.Success || res.Error != want {
			t.Errorf("%s: %v\n got %q\nwant %q", res.Action, res.Success, res.Error, want)
		}
	}
	ok(run("open", "url", srv.URL))

	cases := []struct {
		res  *engine.StepResult
		want string
	}{
		{run("select", "locator", css("#s"), "options", []any{"a", "Bee"}), `{"selected": ["a", "b"]}`},
		{run("select", "locator", css("#one"), "value", "Y"), `{"selected": ["y"]}`},
		{run("read", "locator", css("#d")), `{"text": "Hello world", "innerHTML": "Hello <b>world</b>", "visible": true, "enabled": true, "checked": null, "value": null}`},
		{run("read", "locator", css("#cb")), `{"text": "", "innerHTML": "", "visible": true, "enabled": true, "checked": true, "value": "on"}`},
		{run("read", "locator", css("[name=q]")), `{"text": "", "innerHTML": "", "visible": true, "enabled": true, "checked": null, "value": "v1"}`},
		{run("read", "locator", css("#hid")), `{"text": "h", "innerHTML": "h", "visible": false, "enabled": true, "checked": null, "value": null}`},
		{run("upload", "locator", css("#up"), "files", []any{up}), `{"files": ["` + up + `"]}`},
		{run("evaluate", "script", "document.getElementById('up').files[0].name"), `{"result": "up.txt"}`},
		{run("upload", "locator", css("#pick"), "files", []any{up, other}), `{"files": ["` + up + `", "` + other + `"]}`},
		{run("evaluate", "script", "document.getElementById('up').files.length"), `{"result": 2}`},
		{run("mouse-move", "x", 10, "y", 20, "steps", 3), ``},
		{run("mouse-random", "endX", 100, "endY", 100, "seed", 1, "steps", 5), `{"points": 6}`},
		{run("mouse-scroll-to", "locator", css("#far")), ``},
		{run("evaluate", "script", "scrollY > 0"), `{"result": true}`},
	}
	for i, c := range cases {
		if got := ok(c.res); got != c.want {
			t.Errorf("case %d %s:\n got %s\nwant %s", i, c.res.Action, got, c.want)
		}
	}

	waitErr := func(api, sel string) string {
		return "Locator." + api + ": Timeout 1000ms exceeded.\nCall log:\n  - waiting for locator(\"" + sel + "\").first\n"
	}
	fails(run("select", "locator", css("#one"), "value", "zz", "timeout", 1000),
		waitErr("select_option", "#one")[:len(waitErr("select_option", "#one"))-1]+"\n  - did not find some options\n")
	fails(run("select", "locator", css("#d"), "value", "zz", "timeout", 1000),
		"Locator.select_option: Error: Element is not a <select> element\nCall log:\n  - waiting for locator(\"#d\").first\n")
	fails(run("select", "locator", css("#nope"), "value", "zz", "timeout", 1000), waitErr("wait_for", "#nope"))
	fails(run("read", "locator", css("#nope"), "timeout", 1000), waitErr("wait_for", "#nope"))
	missing := filepath.Join(dir, "missing.txt")
	fails(run("upload", "locator", css("#up"), "files", []any{missing}), "[Errno 2] No such file or directory: '"+missing+"'")
	fails(run("upload", "locator", css("#nope"), "files", []any{up}, "timeout", 1000), waitErr("evaluate", "#nope"))
	fails(run("upload", "locator", css("#d"), "files", []any{up}, "timeout", 1000),
		"Timeout 1000ms exceeded while waiting for event \"filechooser\"\n=========================== logs ===========================\n"+
			"waiting for event \"filechooser\"\n============================================================")
	fails(run("mouse-scroll-to", "locator", css("#nope"), "timeout", 1000), waitErr("scroll_into_view_if_needed", "#nope"))
	fails(run("submit", "locator", css("#nope"), "timeout", 1000), waitErr("wait_for", "#nope"))

	ok(run("submit", "locator", css("#b")))
	time.Sleep(200 * time.Millisecond)
	ok(run("wait", "waitUntil", "load"))
	ok(run("wait-text", "text", "done q=v1"))

	tb, err := s.newTab()
	if err != nil {
		t.Fatal(err)
	}
	if err := tb.goto_(srv.URL+"/done?x=1", 10000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.newTab(); err != nil { // about:blank is skipped
		t.Fatal(err)
	}
	got := ok(run("browse-tabs", "deltaY", 100))
	want := `{"visited": [{"page": "page_0", "url": "` + srv.URL + `/done?q=v1"}, {"page": "page_1", "url": "` + srv.URL + `/done?x=1"}], "count": 2}`
	if got != want {
		t.Errorf("browse-tabs:\n got %s\nwant %s", got, want)
	}
}
