package browser

import (
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

func TestHTTPOps(t *testing.T) {
	s := startSession(t)
	page := `data:text/html,<body style="margin:0"><div id=box style="position:absolute;left:10px;top:20px;width:100px;height:50px"></div>` +
		`<p id=hid style="display:none">x</p><script>var pageVar = 7</script></body>`
	if _, err := s.Navigate("", page, 10000); err != nil {
		t.Fatal(err)
	}

	res, err := s.MoveToElement("", scenario.Locator{Type: "id", Value: "box"}, 5000)
	if err != nil || omap.Marshal(res, 0) != `{"x": 60.0, "y": 45.0}` {
		t.Fatalf("move-to: %v %v", res, err)
	}
	if _, err := s.MoveToElement("", scenario.Locator{Type: "id", Value: "hid"}, 1000); err == nil ||
		err.Error() != "Element has no bounding box (not visible?)" {
		t.Fatalf("move-to hidden: %v", err)
	}

	for _, state := range []string{"attached", "hidden"} {
		if err := s.WaitFor("", scenario.Locator{Type: "id", Value: "hid"}, state, 2000); err != nil {
			t.Fatalf("wait %s: %v", state, err)
		}
	}
	if err := s.WaitFor("", scenario.Locator{Type: "id", Value: "none"}, "detached", 2000); err != nil {
		t.Fatalf("wait detached: %v", err)
	}
	err = s.WaitFor("", scenario.Locator{Type: "id", Value: "box"}, "detached", 300)
	if err == nil || !strings.HasPrefix(err.Error(), "Locator.wait_for: Timeout 300ms exceeded.") ||
		!strings.Contains(err.Error(), `locator("#box").first to be detached`) {
		t.Fatalf("wait detached timeout: %v", err)
	}
	if err := s.WaitFor("", scenario.Locator{Type: "id", Value: "box"}, "bad", 300); err == nil {
		t.Fatal("bad state accepted")
	}

	if v, err := s.Evaluate("", "pageVar", false); err != nil || v != 7 {
		t.Fatalf("evaluate main: %v %v", v, err)
	}
	if v, err := s.Evaluate("", "typeof pageVar", true); err != nil || v != "undefined" {
		t.Fatalf("evaluate isolated: %v %v", v, err)
	}
	if v, err := s.Evaluate("", "() => document.title.length + 1", true); err != nil || v != 1 {
		t.Fatalf("evaluate isolated function: %v %v", v, err)
	}
	if _, err := s.Evaluate("", "nope()", true); err == nil || !strings.HasPrefix(err.Error(), "Page.evaluate: ReferenceError") {
		t.Fatalf("evaluate isolated error: %v", err)
	}
	if err := s.Wheel("", 0, 100.5); err != nil {
		t.Fatal(err)
	}
}
