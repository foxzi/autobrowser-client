package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/updater"
)

// releaseServer answers /releases/latest with a redirect to tag and 404 for
// everything else (no assets, so nothing can be installed).
func releaseServer(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	old := updater.RepoURL
	updater.RepoURL = srv.URL
	t.Cleanup(func() { updater.RepoURL = old })
}

func TestUpdateUpToDate(t *testing.T) {
	releaseServer(t, "v"+Version)
	for _, args := range [][]string{{"update"}, {"update", "--check"}} {
		if code, out, _ := run(args...); code != 0 || out != "Up to date: "+Version+"\n" {
			t.Errorf("%v: %d %q", args, code, out)
		}
	}
}

func TestUpdateCheck(t *testing.T) {
	releaseServer(t, "v99.0.0")
	code, out, _ := run("update", "--check")
	if code != 0 || out != "Update available: "+Version+" -> 99.0.0\n" {
		t.Errorf("%d %q", code, out)
	}
}

func TestUpdateInstallFailure(t *testing.T) {
	releaseServer(t, "v99.0.0")
	code, out, errText := run("update")
	if code != 1 || out != "" || !strings.Contains(errText, "404") {
		t.Errorf("%d %q %q", code, out, errText)
	}
}
