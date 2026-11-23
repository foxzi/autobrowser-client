package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// A click or an element press that submits a form waits until the navigation
// commits, so a following "wait load" sees the new page (examples/login.yaml).
func TestClickWaitsForNavigation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodPost {
			time.Sleep(500 * time.Millisecond)
			fmt.Fprint(w, `<p>submitted</p>`)
			return
		}
		fmt.Fprint(w, `<form method="post" action="/done"><input name="q"><button>Go</button></form>`)
	}))
	defer srv.Close()
	s := startSession(t)
	run, ok := runner(t, s)
	css := func(v string) *omap.Map { return omap.New("type", "css", "value", v) }
	for _, act := range [][]any{
		{"click", "locator", css("button")},
		{"press", "locator", css("input"), "key", "Enter"},
	} {
		ok(run("open", "url", srv.URL))
		ok(run(act[0].(string), act[1:]...))
		ok(run("wait", "waitUntil", "load"))
		if got := ok(run("get-content", "format", "text")); !strings.Contains(got, "submitted") {
			t.Fatalf("%s: content after wait load: %s", act[0], got)
		}
	}
}
