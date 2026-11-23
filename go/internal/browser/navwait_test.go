package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// Waits keep polling while the page navigates (Playwright retries when the
// execution context is destroyed).
func TestWaitSurvivesNavigation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n >= 60 {
			fmt.Fprint(w, `<p id="done">finished</p>`)
			return
		}
		fmt.Fprintf(w, `<p>step %d</p><script>setTimeout(() => location.href = '/?n=%d', 5)</script>`, n, n+1)
	}))
	defer srv.Close()
	s := startSession(t)
	run, ok := runner(t, s)
	ok(run("open", "url", srv.URL))
	ok(run("wait-text", "text", "finished", "timeout", 10000))
	ok(run("open", "url", srv.URL))
	ok(run("wait", "waitUntil", "visible", "locator", omap.New("type", "id", "value", "done"), "timeout", 10000))
	ok(run("open", "url", srv.URL))
	ok(run("find", "locator", omap.New("type", "css", "value", "#done"), "timeout", 10000))
}
