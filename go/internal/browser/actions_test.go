package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
