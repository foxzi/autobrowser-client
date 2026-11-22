package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

const testPage = `<!doctype html><title>Test page</title>
<h1 id="h">Hello World</h1>
<input id="name" name="who"> <input type="checkbox" id="cb">
<button id="go" onclick="document.getElementById('out').textContent='clicked '+document.getElementById('name').value">Go</button>
<div id="out"></div>
<ul><li class="it">one</li><li class="it">two</li></ul>
<a id="lnk" href="/next" data-x="42">next</a>
<div id="hidden" style="display:none">secret</div>`

func startSession(t *testing.T) *Session {
	t.Helper()
	if _, err := FindBinary(); err != nil {
		t.Skip("cloakbrowser binary not available: ", err)
	}
	st, err := config.LoadSettings("", nil, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	if err != nil {
		t.Fatal(err)
	}
	st.Browser.Headless = true
	s, err := Start(st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(false) })
	return s
}

func step(t *testing.T, action string, kv ...any) *scenario.Step {
	t.Helper()
	m := omap.New(append([]any{"action", action}, kv...)...)
	s, err := scenario.ParseStep(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionSteps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testPage)
	}))
	defer srv.Close()
	deadSrv := httptest.NewServer(nil)
	dead := deadSrv.URL
	deadSrv.Close()
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)

	s := startSession(t)
	loc := func(typ, v string) *omap.Map { return omap.New("type", typ, "value", v) }
	ok := func(res *engine.StepResult) *engine.StepResult {
		t.Helper()
		if !res.Success {
			t.Fatalf("%s failed: %s", res.Action, res.Error)
		}
		return res
	}
	run := func(action string, kv ...any) *engine.StepResult {
		return s.ExecuteStep(step(t, action, kv...), "")
	}

	ok(run("open", "url", srv.URL))
	if r := ok(run("find", "locator", loc("css", "li.it"))); omap.Marshal(r.Data, 0) != `{"found": 2}` {
		t.Errorf("find: %s", omap.Marshal(r.Data, 0))
	}
	ok(run("fill", "locator", loc("id", "name"), "value", "bob"))
	ok(run("click", "locator", loc("id", "go")))
	ok(run("wait-text", "text", "clicked bob"))
	r := ok(run("extract", "locator", loc("css", "#out")))
	if got := omap.Marshal(r.Data, 0); got != `{"value": "clicked bob"}` {
		t.Errorf("extract: %s", got)
	}
	r = ok(run("extract", "locator", loc("css", ".it"), "all", true))
	if got := omap.Marshal(r.Data, 0); got != `{"value": ["one", "two"]}` {
		t.Errorf("extract all: %s", got)
	}
	r = ok(run("extract", "locator", loc("id", "lnk"), "attribute", "data-x"))
	if got := omap.Marshal(r.Data, 0); got != `{"value": "42"}` {
		t.Errorf("attr: %s", got)
	}
	ok(run("click", "locator", loc("text", "two"), "index", 0))
	ok(run("type", "locator", loc("name", "who"), "value", "ab", "delay", 1))
	ok(run("press", "key", "Control+a"))
	ok(run("press", "key", "Backspace"))
	r = ok(run("evaluate", "script", `() => document.getElementById('name').value`))
	if got := omap.Marshal(r.Data, 0); got != `{"result": ""}` {
		t.Errorf("evaluate after typing: %s", got)
	}
	r = ok(run("evaluate", "script", `({a: 1, b: [1, "x"]})`))
	if got := omap.Marshal(r.Data, 0); got != `{"result": {"a": 1, "b": [1, "x"]}}` {
		t.Errorf("evaluate: %s", got)
	}
	ok(run("check", "locator", loc("id", "cb")))
	r = ok(run("evaluate", "script", `document.getElementById('cb').checked`))
	if got := omap.Marshal(r.Data, 0); got != `{"result": true}` {
		t.Errorf("check: %s", got)
	}
	ok(run("scroll", "deltaY", 100))
	ok(run("screenshot", "file", "out/s.png"))
	if st, err := os.Stat(filepath.Join(dir, "out/s.png")); err != nil || st.Size() == 0 {
		t.Errorf("screenshot file: %v", err)
	}
	r = ok(run("get-content", "format", "text"))
	if !strings.Contains(omap.Marshal(r.Data, 0), "Hello World") {
		t.Errorf("get-content: %s", omap.Marshal(r.Data, 0))
	}
	ok(run("save", "file", "page.html"))
	ok(run("wait", "waitUntil", "networkidle"))
	ok(run("wait", "waitUntil", "hidden", "locator", loc("id", "hidden"), "timeout", 500))
	ok(run("history", "direction", "reload"))

	res := run("click", "locator", loc("css", "#nope"), "timeout", 300)
	want := "Locator.click: Timeout 300ms exceeded.\nCall log:\n  - waiting for locator(\"#nope\")"
	if res.Success || res.Error != want {
		t.Errorf("missing click: %v %q", res.Success, res.Error)
	}
	p, err := s.Page("")
	if err != nil {
		t.Fatal(err)
	}
	if title, _ := p.Title(); title != "Test page" {
		t.Errorf("title %q", title)
	}
	if n, _ := p.Count(scenario.Locator{Type: "css", Value: ".it"}); n != 2 {
		t.Errorf("count %d", n)
	}
	if v, _ := p.IsVisible(scenario.Locator{Type: "id", Value: "hidden"}); v {
		t.Error("hidden element reported visible")
	}
	if res := run("open", "url", dead); res.Success || !strings.Contains(res.Error, "ERR_CONNECTION_REFUSED") {
		t.Errorf("open refused: %v %q", res.Success, res.Error)
	}

	if _, err := s.Page("page_9"); err == nil || !strings.Contains(err.Error(), "Page not found: page_9") {
		t.Errorf("page_9: %v", err)
	}
}

func TestProfileLockAndTabs(t *testing.T) {
	if _, err := FindBinary(); err != nil {
		t.Skip("cloakbrowser binary not available")
	}
	st, _ := config.LoadSettings("", nil, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	st.Browser.Headless = true
	st.Profiles.Dir = t.TempDir()
	s, err := Start(st, Options{Profile: "p1", PreserveSession: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(st, Options{Profile: "p1"}); err == nil || !strings.Contains(err.Error(), "Profile 'p1' is locked by PID") {
		t.Errorf("second start: %v", err)
	}
	if r := s.ExecuteStep(step(t, "open", "url", "data:text/html,<title>x</title>hi"), ""); !r.Success {
		t.Fatal(r.Error)
	}
	s.Close(true)
	data, err := os.ReadFile(filepath.Join(st.Profiles.Dir, "p1", TabsFile))
	if err != nil || !strings.Contains(string(data), "data:text/html") {
		t.Errorf("tabs.json: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(st.Profiles.Dir, "p1", "profile.lock")); err == nil {
		t.Error("lock file left behind")
	}
}

func TestCommandLine(t *testing.T) {
	args := chromeCommandLine([]string{"--x"}, true, "http://p:1", "/d", false)
	joined := strings.Join(args, " ")
	for _, w := range []string{"--headless", "--no-sandbox", "--proxy-server=http://p:1", "--x", "--user-data-dir=/d", "--no-startup-window"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %s in %s", w, joined)
		}
	}
	if strings.Contains(joined, "about:blank") {
		t.Error("unexpected about:blank")
	}
}

func TestDetectPlatformFromUA(t *testing.T) {
	if detectPlatformFromUA("Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0.0.0") != nil {
		t.Error("linux must not spoof")
	}
	w := detectPlatformFromUA("Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.6099.71")
	if w == nil || w.platform != "Win32" || w.meta.Platform != "Windows" || w.meta.FullVersion != "120.0.6099.71" {
		t.Errorf("windows: %+v", w)
	}
	a := detectPlatformFromUA("Mozilla/5.0 (Linux; Android 13; Pixel 7 Build/TQ3A) Chrome/120.0.0.0 Mobile")
	if a == nil || !a.mobile || a.meta.Model != "Pixel 7" {
		t.Errorf("android: %+v", a)
	}
}
