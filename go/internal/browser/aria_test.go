package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// TestAriaSnapshot compares Snapshot with the output of the Python client on
// the same pages (testdata/aria/expected.json, made by testdata/aria/gen.py).
func TestAriaSnapshot(t *testing.T) {
	raw, err := os.ReadFile("testdata/aria/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata/aria")))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	port := host[strings.LastIndex(host, ":")+1:]

	s := startSession(t)
	snap := func(name string, loc ...string) {
		t.Helper()
		var res *Snapshot
		var err error
		if loc != nil {
			res, err = s.ElementSnapshot("page_0", scenario.Locator{Type: loc[0], Value: loc[1]}, 10000)
		} else {
			res, err = s.Snapshot("page_0", 10000)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := strings.ReplaceAll(res.Snapshot, host, "HOST")
		got = strings.ReplaceAll(got, "localhost:"+port, "LOCALHOST")
		if got != want[name] {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", name, got, want[name])
		}
	}
	run := func(action string, kv ...any) {
		t.Helper()
		if res := s.ExecuteStep(step(t, action, kv...), ""); !res.Success {
			t.Fatalf("%s: %s", action, res.Error)
		}
	}
	ref := func(v string) *omap.Map { return omap.New("type", "ref", "value", v) }

	for _, page := range []string{"basic", "frames"} {
		run("open", "url", srv.URL+"/"+page+".html")
		snap(page)
	}
	run("click", "locator", ref("f3e3"))
	run("fill", "locator", ref("f2e3"), "value", "typed")
	snap("frames-after")
	// Element snapshots: a main frame element, an element inside a frame.
	snap("element-h1", "css", "h1")
	snap("element-frame", "ref", "f3e1")
	run("click", "locator", ref("f1e2"))
	run("fill", "locator", ref("f3e4"), "value", "el")

	run("open", "url", srv.URL+"/dynamic.html")
	snap("dynamic-1")
	run("click", "locator", ref("f5e2"))
	run("fill", "locator", ref("f5e3"), "value", "hello")
	snap("dynamic-2")

	res := s.ExecuteStep(step(t, "click", "locator", ref("e99"), "timeout", 500), "")
	if want := "Locator.click: Timeout 500ms exceeded.\nCall log:\n  - waiting for locator(\"aria-ref=e99\")\n"; res.Error != want {
		t.Errorf("missing ref: %q", res.Error)
	}
	res = s.ExecuteStep(step(t, "click", "locator", ref("f9e1"), "index", 0), "")
	if want := "Locator.click: Invalid frame in aria-ref selector \"aria-ref=f9e1 >> nth=0\"\nCall log:\n  - waiting for locator(\"aria-ref=f9e1\").first\n"; res.Error != want {
		t.Errorf("bad frame: %q", res.Error)
	}
}

func TestAriaSnapshotBlank(t *testing.T) {
	s := startSession(t)
	if res, err := s.Snapshot("page_0", 10000); err != nil || *res != (Snapshot{URL: "about:blank"}) {
		t.Errorf("about:blank: %+v %v", res, err)
	}
	if res := s.ExecuteStep(step(t, "open", "url", "data:text/html,<button>B</button>"), ""); !res.Success {
		t.Fatal(res.Error)
	}
	// Python gives e2 here too: the page keeps the ref counter of about:blank.
	if res, err := s.Snapshot("page_0", 10000); err != nil || res.Snapshot != `- button "B" [ref=e2]` {
		t.Errorf("data url: %+v %v", res, err)
	}
}
