package browser

import (
	"sync"

	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// navStatus records the status of the last main-frame document response
// while fn runs: the status of Playwright's navigation response (nil when
// there was none, e.g. data: URLs or nothing to go back to).
func (t *tab) navStatus(fn func() error) (any, error) {
	var mu sync.Mutex
	var status any
	t.setNavHook(func(_ *omap.Map, code int) {
		mu.Lock()
		status = code
		mu.Unlock()
	})
	err := fn()
	t.setNavHook(nil)
	mu.Lock()
	defer mu.Unlock()
	return status, err
}

// pageResult is {url, title, status} of actions.navigate and actions.history.
func (t *tab) pageResult(status any) (*omap.Map, error) {
	u, err := t.url()
	if err != nil {
		return nil, err
	}
	title, err := t.Title()
	if err != nil {
		return nil, err
	}
	return omap.New("url", u, "title", title, "status", status), nil
}

// Navigate is actions.navigate (wait_until "load").
func (s *Session) Navigate(pageID, url string, timeoutMs int) (*omap.Map, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	status, err := t.navStatus(func() error { return t.goto_(url, timeoutMs) })
	if err != nil {
		return nil, err
	}
	return t.pageResult(status)
}

// History is actions.history: back, forward or reload.
func (s *Session) History(pageID, direction string, timeoutMs int) (*omap.Map, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	status, err := t.navStatus(func() error { return t.history(direction, timeoutMs) })
	if err != nil {
		return nil, err
	}
	return t.pageResult(status)
}

// Screenshot is actions.take_screenshot as PNG bytes: the first element of
// locator when it is set, else the viewport or the full page.
func (s *Session) Screenshot(pageID string, fullPage bool, locator *scenario.Locator, timeoutMs int) ([]byte, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	if locator == nil {
		return t.page.Screenshot(fullPage, nil)
	}
	l, err := t.locator(*locator, nil)
	if err != nil {
		return nil, err
	}
	el, err := l.actionable("screenshot", timeoutMs)
	if err != nil {
		return nil, err
	}
	return el.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
}

// Do runs one step action like ExecuteStep but returns its data and the raw
// error (an *apperr.Error keeps its code).
func (s *Session) Do(step *scenario.Step, pageID string) (any, error) {
	return s.run(step, pageID)
}
