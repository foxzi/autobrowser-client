package browser

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

const agentPage = "data:text/html;charset=utf-8,<title>T</title>" +
	"<button onclick=\"window.open('about:blank')\">Pop</button><p>héllo</p>"

func startManager(t *testing.T) (*Manager, *Session) {
	t.Helper()
	requireBrowser(t)
	st, err := config.LoadSettings("", nil, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	if err != nil {
		t.Fatal(err)
	}
	st.Browser.Headless = true
	st.API.MaxSessions = 1
	m := NewManager(st)
	t.Cleanup(m.StopAll)
	s, err := m.Create(Options{URLs: []string{agentPage}})
	if err != nil {
		t.Fatal(err)
	}
	return m, s
}

func code(err error) apperr.Code {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func eval(t *testing.T, s *Session, expr string) any {
	t.Helper()
	r := s.ExecuteStep(step(t, "evaluate", "script", expr), "page_0")
	if !r.Success {
		t.Fatalf("evaluate %q: %v", expr, r.Error)
	}
	v, _ := r.Data.(*omap.Map).Get("result")
	return v
}

func TestManagerSessions(t *testing.T) {
	m, s := startManager(t)
	if !strings.HasPrefix(s.ID(), "sess_") || len(s.ID()) != 17 || s.State() != StateRunning {
		t.Fatalf("session %q state %q", s.ID(), s.State())
	}
	if _, err := m.Create(Options{}); code(err) != apperr.BrowserStartFailed ||
		err.Error() != "Maximum active sessions reached: 1" {
		t.Fatalf("max sessions: %v", err)
	}
	if _, err := m.Get("nope"); code(err) != apperr.SessionNotFound || err.Error() != "Session not found: nope" {
		t.Fatalf("get: %v", err)
	}
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("list: %v", list)
	}
	info := s.Info()
	if v, _ := info.Get("state"); v != "running" {
		t.Fatalf("info: %v", info)
	}
	if v, _ := info.Get("stoppedAt"); v != nil {
		t.Fatalf("stoppedAt: %v", v)
	}
	if _, err := m.Stop(s.ID()); err != nil {
		t.Fatal(err)
	}
	if s.State() != StateStopped || len(m.List()) != 0 {
		t.Fatalf("after stop: %q %v", s.State(), m.List())
	}
}

func TestTabsDialogsLogsContent(t *testing.T) {
	_, s := startManager(t)

	if r := s.ExecuteStep(step(t, "click", "locator", omap.New("type", "text", "value", "Pop")), "page_0"); !r.Success {
		t.Fatal(r.Error)
	}
	time.Sleep(500 * time.Millisecond)
	if _, ok := s.ListPages().Get("page_1"); !ok {
		t.Fatalf("popup not listed: %v", s.ListPages())
	}
	id, err := s.NewTab("")
	if err != nil || id != "page_2" {
		t.Fatalf("new tab %q: %v", id, err)
	}
	if err := s.CloseTab(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ListPages().Get(id); ok {
		t.Fatalf("closed tab listed: %v", s.ListPages())
	}

	eval(t, s, "document.title = String(confirm('q'))")
	if v := eval(t, s, "document.title"); v != "false" {
		t.Fatalf("confirm: %v", v)
	}
	text := "hi"
	s.SetDialogPolicy(true, &text)
	eval(t, s, "document.title = prompt('name')")
	if v := eval(t, s, "document.title"); v != "hi" {
		t.Fatalf("prompt: %v", v)
	}
	dialogs, err := s.Log("page_0", "dialogs", true)
	if err != nil || len(dialogs) != 2 {
		t.Fatalf("dialogs %v: %v", dialogs, err)
	}
	for i, want := range [][2]any{{"confirm", false}, {"prompt", true}} {
		d := dialogs[i].(*omap.Map)
		typ, _ := d.Get("type")
		acc, _ := d.Get("accepted")
		if typ != want[0] || acc != want[1] {
			t.Fatalf("dialog %d: %v", i, dialogs[i])
		}
	}
	if d, _ := s.Log("page_0", "dialogs", false); len(d) != 0 {
		t.Fatalf("not cleared: %v", d)
	}

	eval(t, s, "fetch('http://127.0.0.1:9/x').catch(() => 0)")
	time.Sleep(300 * time.Millisecond)
	net, _ := s.Log("page_0", "network", false)
	failed := false
	for _, n := range net {
		if f, _ := n.(*omap.Map).Get("failure"); f != nil {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("no failed request in %v", net)
	}
	console, _ := s.Log("page_0", "console", false)
	if len(console) == 0 || !strings.Contains(omap.Marshal(console, 0), `"type": "error", "text": "Failed to load resource: net::ERR_UNSAFE_PORT"`) {
		t.Fatalf("console: %s", omap.Marshal(console, 0))
	}
	if _, err := s.Log("page_9", "network", false); code(err) != apperr.PageNotFound {
		t.Fatalf("missing page: %v", err)
	}

	s.output.MaxContentBytes = 0
	full, err := s.Content("page_0", "text")
	if err != nil || !strings.Contains(full, "héllo") {
		t.Fatalf("content %q: %v", full, err)
	}
	cut := strings.Index(full, "é") + 1 // inside the two-byte character
	s.output.MaxContentBytes = cut
	if got, _ := s.Content("page_0", "text"); got != full[:cut-1] {
		t.Fatalf("cut content %q", got)
	}
}
