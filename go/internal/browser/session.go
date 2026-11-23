package browser

import (
	"strings"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// State is SessionState.
type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

// dialogPolicy is how JS dialogs are answered; dismissed by default.
type dialogPolicy struct {
	accept     bool
	promptText *string
}

// ID is the session id ("" for sessions made by Start).
func (s *Session) ID() string { return s.id }

// State is the current state of the session.
func (s *Session) State() State {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	return s.state
}

// Call runs fn while no other call of the session runs, like
// BrowserSession.call runs everything on the session thread.
func (s *Session) Call(fn func() error) error {
	s.callMu.Lock()
	defer s.callMu.Unlock()
	return fn()
}

// Touch updates the last activity time.
func (s *Session) Touch() {
	s.stMu.Lock()
	s.lastActivity = time.Now()
	s.stMu.Unlock()
}

func (s *Session) idleFor() time.Duration {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	return time.Since(s.lastActivity)
}

// SetDialogPolicy sets how future dialogs are answered.
func (s *Session) SetDialogPolicy(accept bool, promptText *string) {
	s.stMu.Lock()
	s.dialog = dialogPolicy{accept: accept, promptText: promptText}
	s.stMu.Unlock()
}

func (s *Session) dialogPolicy() (bool, *string) {
	s.stMu.Lock()
	defer s.stMu.Unlock()
	return s.dialog.accept, s.dialog.promptText
}

// DialogPolicy is the dialog_policy dict: accept and promptText.
func (s *Session) DialogPolicy() *omap.Map {
	accept, text := s.dialogPolicy()
	var promptText any
	if text != nil {
		promptText = *text
	}
	return omap.New("accept", accept, "promptText", promptText)
}

// ListPages is list_pages: page id to URL, in the order the tabs were opened.
func (s *Session) ListPages() *omap.Map {
	out := omap.New()
	if s.root == nil {
		return out
	}
	s.sync()
	s.mu.Lock()
	pages := append([]*tab(nil), s.pages...)
	s.mu.Unlock()
	for _, t := range pages {
		u, _ := t.url()
		out.Set(t.id, u)
	}
	return out
}

// NewTab is new_tab: opens a tab, navigates it when url is set, returns its id.
func (s *Session) NewTab(url string) (string, error) {
	if s.ctx == nil {
		return "", apperr.New(apperr.BrowserStartFailed, "Browser context is not initialized")
	}
	t, err := s.newTab()
	if err != nil {
		return "", err
	}
	if url != "" {
		if err := t.goto_(url, s.settingsT.NavigationMs); err != nil {
			return t.id, err
		}
	}
	return t.id, nil
}

// CloseTab is close_tab.
func (s *Session) CloseTab(pageID string) error {
	t, err := s.tab(pageID)
	if err != nil {
		return err
	}
	if err := t.page.Close(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.pages {
		if p == t {
			s.pages = append(s.pages[:i], s.pages[i+1:]...)
			break
		}
	}
	return nil
}

// Log is get_log: the console, network or dialogs log of a tab.
func (s *Session) Log(pageID, kind string, clear bool) ([]any, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	// Python looks the log up by the page_id argument, so None gives nothing
	if pageID == "" {
		return []any{}, nil
	}
	t.net.mu.Lock()
	defer t.net.mu.Unlock()
	items, ok := t.net.logs[kind]
	if !ok {
		return []any{}, nil
	}
	out := append([]any{}, items...)
	if clear {
		t.net.logs[kind] = []any{}
	}
	return out, nil
}

// Content is get_content: HTML, or the body text for fmt "text", cut to
// output.maxContentBytes without splitting a character.
func (s *Session) Content(pageID, fmt string) (string, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return "", err
	}
	var content string
	if fmt == "text" {
		content, err = t.InnerText()
	} else {
		content, err = t.Content()
	}
	if err != nil {
		return "", err
	}
	if max := s.output.MaxContentBytes; max > 0 && len(content) > max {
		content = strings.ToValidUTF8(content[:max], "")
	}
	return content, nil
}

// ElementText is actions.element_text: the innerText of the first match, cut
// like Content.
func (s *Session) ElementText(pageID string, l scenario.Locator, timeoutMs int) (string, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return "", err
	}
	lc, err := t.locator(l, nil)
	if err != nil {
		return "", err
	}
	lc.repr += ".first"
	el, err := lc.waitFor("inner_text", "attached", timeoutMs)
	if err != nil {
		return "", err
	}
	v, err := elementValue(el, "")
	if err != nil {
		return "", err
	}
	text := v.(string)
	if max := s.output.MaxContentBytes; max > 0 && len(text) > max {
		text = strings.ToValidUTF8(text[:max], "")
	}
	return text, nil
}

// Info is BrowserSession.info.
func (s *Session) Info() *omap.Map {
	s.stMu.Lock()
	var stoppedAt, errText any
	if !s.stoppedAt.IsZero() {
		stoppedAt = epoch(s.stoppedAt)
	}
	if s.err != "" {
		errText = s.err
	}
	m := omap.New("sessionId", s.id, "state", string(s.state), "startedAt", epoch(s.startedAt),
		"stoppedAt", stoppedAt, "error", errText)
	s.stMu.Unlock()
	m.Set("pages", s.ListPages())
	if s.relay != nil && s.relay.IsRunning() {
		m.Set("proxy", s.relay.HTTPProxyURL())
	}
	return m
}

func epoch(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }
