package browser

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

var _ engine.Page = (*tab)(nil)

// Page is engine.Browser.Page.
func (s *Session) Page(pageID string) (engine.Page, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *tab) Count(l scenario.Locator) (int, error) {
	lc, err := t.locator(l, nil)
	if err != nil {
		return 0, err
	}
	return lc.count()
}

func (t *tab) IsVisible(l scenario.Locator) (bool, error) {
	lc, err := t.locator(l, nil)
	if err != nil {
		return false, err
	}
	el, err := lc.first()
	if err != nil || el == nil {
		return false, err
	}
	return lc.visible(el), nil
}

func (t *tab) InnerText() (string, error) {
	res, err := t.page.Eval(`() => document.body ? document.body.innerText : ''`)
	if err != nil {
		return "", err
	}
	return res.Value.Str(), nil
}

func (t *tab) URL() (string, error) { return t.url() }

// Title is document.title, like Playwright (the target title falls back to the URL).
func (t *tab) Title() (string, error) {
	res, err := t.page.Eval(`() => document.title`)
	if err != nil {
		return "", err
	}
	return res.Value.Str(), nil
}

// Content is Playwright's page.content(): the doctype plus the document HTML.
func (t *tab) Content() (string, error) {
	res, err := t.page.Eval(`() => {
		let html = '';
		if (document.doctype) html = new XMLSerializer().serializeToString(document.doctype);
		if (document.documentElement) html += document.documentElement.outerHTML;
		return html;
	}`)
	if err != nil {
		return "", err
	}
	return res.Value.Str(), nil
}

func (t *tab) Screenshot(path string) error { return t.screenshot(path, false) }

func (t *tab) screenshot(path string, fullPage bool) error {
	png, err := t.page.Screenshot(fullPage, nil)
	if err != nil {
		return err
	}
	return os.WriteFile(path, png, 0o644)
}

// readyState polls document.readyState for load / domcontentloaded.
func (t *tab) waitState(state string, ms int) error {
	ok, err := poll(ms, func() (bool, error) {
		res, err := t.page.Eval(`() => document.readyState`)
		if err != nil {
			return false, nil // context destroyed by a navigation
		}
		rs := res.Value.Str()
		return rs == "complete" || (state == "domcontentloaded" && rs != "loading"), nil
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Page.wait_for_load_state: Timeout %dms exceeded.", ms)
	}
	return nil
}

// waitNetworkIdle waits for the load state and 500 ms without requests.
func (t *tab) waitNetworkIdle(ms int) error {
	start := time.Now()
	if err := t.waitState("load", ms); err != nil {
		return err
	}
	left := time.Duration(ms)*time.Millisecond - time.Since(start)
	done := make(chan struct{})
	wait := t.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	go func() { wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(left):
		return fmt.Errorf("Page.wait_for_load_state: Timeout %dms exceeded.", ms)
	}
}

// goto_ is Page.goto: navigate and wait for the load event.
func (t *tab) goto_(url string, ms int) error {
	start := time.Now()
	nav := proto.PageNavigate{URL: url}
	res, err := nav.Call(t.page)
	if err != nil {
		return err
	}
	if res.ErrorText != "" {
		return fmt.Errorf("Page.goto: %s at %s", res.ErrorText, url)
	}
	left := ms - int(time.Since(start).Milliseconds())
	if err := t.waitState("load", max(left, 1)); err != nil {
		return fmt.Errorf("Page.goto: Timeout %dms exceeded.", ms)
	}
	return nil
}

// evaluate runs an expression or function source; a function is called.
func (t *tab) evaluate(expr string) (any, error) {
	// Objects come back as JSON text: CDP values lose the key order.
	res, err := t.page.Evaluate(rod.Eval(`async (expr) => {
		let v = (0, eval)(expr);
		v = await (typeof v === 'function' ? v() : v);
		return v !== null && typeof v === 'object' ? [JSON.stringify(v)] : v;
	}`, expr).ByPromise())
	if ee := (*rod.EvalError)(nil); errors.As(err, &ee) {
		return nil, fmt.Errorf("Page.evaluate: %s", exceptionMessage(ee.Exception))
	}
	if err != nil {
		return nil, fmt.Errorf("Page.evaluate: %s", err)
	}
	if a, ok := res.Value.Val().([]any); ok && len(a) == 1 {
		if j, ok := a[0].(string); ok {
			return toValue([]byte(j))
		}
	}
	return toValue([]byte(res.Value.JSON("", "")))
}
