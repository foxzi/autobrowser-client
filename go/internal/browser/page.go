package browser

import (
	"fmt"
	"os"
	"strings"
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

func (t *tab) Title() (string, error) {
	info, err := t.page.Info()
	if err != nil {
		return "", err
	}
	return info.Title, nil
}

func (t *tab) Content() (string, error) { return t.page.HTML() }

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
	res, err := t.page.Evaluate(rod.Eval(`(expr) => {
		const v = (0, eval)(expr);
		return typeof v === 'function' ? v() : v;
	}`, expr).ByPromise())
	if err != nil {
		return nil, fmt.Errorf("Page.evaluate: %s", strings.TrimPrefix(err.Error(), "eval js error: "))
	}
	return toValue([]byte(res.Value.JSON("", "")))
}
