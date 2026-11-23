package browser

import (
	"net/url"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// css locators support Playwright's :has-text() pseudo-class
// (examples/amiunique.yaml).
func TestCSSHasText(t *testing.T) {
	s := startSession(t)
	run, ok := runner(t, s)
	html := `<div role="status"><button>Accept all</button><button>Reject</button></div>` +
		`<div><button>accept</button></div><p>x <b>ACCEPT</b></p>`
	ok(run("open", "url", "data:text/html,"+url.PathEscape(html)))
	for sel, want := range map[string]string{
		`[role='status'] button:has-text('ACCEPT')`: `{"found": 1}`,
		`button:has-text("accept")`:                 `{"found": 2}`,
		`div > button:has-text( 'Reject' ), b`:      `{"found": 2}`,
		`:has-text("accept"):not(body):not(html)`:   `{"found": 6}`,
		`div:has-text("reject") + div button`:       `{"found": 1}`,
		`button:has-text("Accept all") ~ button`:    `{"found": 1}`,
	} {
		got := ok(run("find", "locator", omap.New("type", "css", "value", sel), "timeout", 1000))
		if got != want {
			t.Errorf("%s: got %s, want %s", sel, got, want)
		}
	}
}
