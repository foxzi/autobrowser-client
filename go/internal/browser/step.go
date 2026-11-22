package browser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// ExecuteStep is executor.execute_step for the actions the Go version supports.
func (s *Session) ExecuteStep(step *scenario.Step, pageID string) *engine.StepResult {
	res := &engine.StepResult{Action: step.Action, Success: true}
	t0 := time.Now()
	data, err := s.run(step, pageID)
	if err != nil {
		res.Success = false
		res.Error = strings.TrimPrefix(err.Error(), ": ")
		slog.Warn(fmt.Sprintf("Step '%s' failed: %s", step.Action, err))
	} else {
		res.Data = data
	}
	res.DurationMs = float64(time.Since(t0).Microseconds()) / 1000
	return res
}

func need(step *scenario.Step, what string) error {
	return apperr.Invalid(fmt.Sprintf("%s action requires '%s'", step.Action, what))
}

func (s *Session) run(step *scenario.Step, pageID string) (any, error) {
	eff := pageID
	if step.Page != "" {
		eff = step.Page
	}
	t, err := s.tab(eff)
	if err != nil {
		return nil, err
	}
	timeout := s.settingsT.ActionMs
	if step.Timeout != nil && *step.Timeout != 0 {
		timeout = *step.Timeout
	}
	locate := func() (*loc, error) {
		if step.Locator == nil {
			return nil, need(step, "locator")
		}
		return t.locator(*step.Locator, step.Index)
	}

	switch step.Action {
	case "open":
		if step.URL == "" {
			return nil, apperr.Invalid("open action requires 'url'")
		}
		if err := t.goto_(step.URL, s.settingsT.NavigationMs); err != nil {
			return nil, err
		}
		u, _ := t.url()
		return omap.New("url", u), nil

	case "wait":
		switch {
		case step.WaitUntil == "networkidle":
			return nil, t.waitNetworkIdle(timeout)
		case step.WaitUntil == "load" || step.WaitUntil == "domcontentloaded":
			return nil, t.waitState(step.WaitUntil, timeout)
		case step.WaitUntil == "visible" || step.WaitUntil == "hidden":
			l, err := locate()
			if err != nil {
				return nil, err
			}
			_, err = l.waitFor("wait_for", step.WaitUntil, timeout)
			return nil, err
		case step.Timeout != nil && *step.Timeout != 0:
			time.Sleep(time.Duration(*step.Timeout) * time.Millisecond)
			return nil, nil
		}
		return nil, t.waitState("load", timeout)

	case "find":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		if _, err := l.waitFor("wait_for", "attached", timeout); err != nil {
			return nil, err
		}
		n, err := l.count()
		if err != nil {
			return nil, err
		}
		return omap.New("found", n), nil

	case "click", "dblclick", "hover":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		api := map[string]string{"click": "click", "dblclick": "dblclick", "hover": "hover"}[step.Action]
		return nil, l.pointer(api, timeout)

	case "focus":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		el, err := l.waitFor("focus", "attached", timeout)
		if err != nil {
			return nil, err
		}
		return nil, el.Focus()

	case "fill", "clear":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		value := ""
		if step.Action == "fill" {
			if step.Value == nil {
				return nil, need(step, "value")
			}
			value = *step.Value
		}
		return nil, l.fill(step.Action, value, timeout)

	case "check", "uncheck":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		return nil, l.setChecked(step.Action, step.Action == "check", timeout)

	case "type":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		if step.Value == nil {
			return nil, need(step, "value")
		}
		if err := l.pointer("click", timeout); err != nil {
			return nil, err
		}
		delay := 50
		if step.Delay != nil {
			delay = *step.Delay
		}
		for _, ch := range *step.Value {
			if err := t.typeRune(ch); err != nil {
				return nil, err
			}
			d := float64(delay) * (0.5 + rand.Float64())
			time.Sleep(time.Duration(d * float64(time.Millisecond)))
		}
		return nil, nil

	case "press":
		if step.Key == "" {
			return nil, need(step, "key")
		}
		if step.Locator != nil {
			l, err := locate()
			if err != nil {
				return nil, err
			}
			el, err := l.waitFor("press", "attached", timeout)
			if err != nil {
				return nil, err
			}
			if err := el.Focus(); err != nil {
				return nil, err
			}
		}
		return nil, t.press(step.Key)

	case "scroll":
		dx, dy := 0, 0
		if step.DeltaX != nil {
			dx = *step.DeltaX
		}
		if step.DeltaY != nil {
			dy = *step.DeltaY
		}
		return nil, t.page.Mouse.Scroll(float64(dx), float64(dy), 1)

	case "screenshot":
		if step.File == "" {
			return nil, need(step, "file")
		}
		out, err := engine.ValidateOutputPath(step.File)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		if err := t.screenshot(out, step.FullPage); err != nil {
			return nil, err
		}
		st, err := os.Stat(out)
		if err != nil {
			return nil, err
		}
		return omap.New("file", step.File, "fullPage", step.FullPage, "bytes", int(st.Size())), nil

	case "get-content":
		format := step.Format
		if format == "" {
			format = "html"
		}
		content, err := s.content(t, format)
		if err != nil {
			return nil, err
		}
		return omap.New("format", format, "length", utf8.RuneCountInString(content), "content", content), nil

	case "save":
		if step.File == "" {
			return nil, need(step, "file")
		}
		out, err := engine.ValidateOutputPath(step.File)
		if err != nil {
			return nil, err
		}
		var content string
		if step.Value != nil {
			content = *step.Value
		} else {
			format := step.Format
			if format == "" {
				format = "html"
			}
			if content, err = s.content(t, format); err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
			return nil, err
		}
		return omap.New("file", step.File, "bytes", len(content)), nil

	case "evaluate":
		v, err := t.evaluate(step.Script)
		if err != nil {
			return nil, err
		}
		return omap.New("result", v), nil

	case "history":
		ms := s.settingsT.NavigationMs
		if step.Timeout != nil && *step.Timeout != 0 {
			ms = *step.Timeout
		}
		if err := t.history(step.Direction, ms); err != nil {
			return nil, err
		}
		u, _ := t.url()
		return omap.New("url", u), nil

	case "wait-text":
		l, err := t.locator(scenario.Locator{Type: "text", Value: step.Text}, nil)
		if err != nil {
			return nil, err
		}
		state := "visible"
		if step.Gone {
			state = "hidden"
		}
		_, err = l.waitFor("wait_for", state, timeout)
		return nil, err

	case "extract":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		v, err := l.extract(step.All, step.Attribute, timeout)
		if err != nil {
			return nil, err
		}
		return omap.New("value", v), nil

	case "find-all":
		l, err := locate()
		if err != nil {
			return nil, err
		}
		return l.findAll(timeout)
	}
	return nil, fmt.Errorf("Unknown action: %s", step.Action)
}

// content is BrowserSession.get_content.
func (s *Session) content(t *tab, format string) (string, error) {
	var c string
	var err error
	if format == "text" {
		c, err = t.InnerText()
	} else {
		c, err = t.Content()
	}
	if err != nil {
		return "", err
	}
	if max := s.output.MaxContentBytes; max > 0 && len(c) > max {
		c = strings.ToValidUTF8(c[:max], "")
	}
	return c, nil
}

// withTimeout bounds the element operations; an expired bound is reported in
// the Playwright form.
func (l *loc) withTimeout(api string, ms int, el *rod.Element, fn func(*rod.Element) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
	defer cancel()
	err := fn(el.Context(ctx))
	if err != nil && (errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil) {
		return l.timeoutErr(api, ms, "")
	}
	return err
}

// pointer is click, dblclick and hover: wait until visible, scroll, then use the mouse.
func (l *loc) pointer(api string, ms int) error {
	start := time.Now()
	el, err := l.actionable(api, ms)
	if err != nil {
		return err
	}
	left := max(ms-int(time.Since(start).Milliseconds()), 1)
	return l.withTimeout(api, left, el, func(e *rod.Element) error {
		switch api {
		case "hover":
			return e.Hover()
		case "dblclick":
			return e.Click(proto.InputMouseButtonLeft, 2)
		}
		return e.Click(proto.InputMouseButtonLeft, 1)
	})
}

// fill selects the field content and replaces it.
func (l *loc) fill(api, value string, ms int) error {
	el, err := l.actionable(api, ms)
	if err != nil {
		return err
	}
	if err := el.Focus(); err != nil {
		return err
	}
	_, err = el.Eval(`function() {
		if (this.select) this.select();
		else if (this.isContentEditable) document.execCommand('selectAll', false);
	}`)
	if err != nil {
		return err
	}
	if value == "" {
		_, err = el.Eval(`function() {
			if ('value' in this) this.value = ''; else this.textContent = '';
			this.dispatchEvent(new Event('input', {bubbles: true}));
			this.dispatchEvent(new Event('change', {bubbles: true}));
		}`)
		return err
	}
	return l.t.page.InsertText(value)
}

func (l *loc) setChecked(api string, want bool, ms int) error {
	el, err := l.actionable(api, ms)
	if err != nil {
		return err
	}
	state := func() bool {
		r, err := el.Eval(`function() { return !!this.checked }`)
		return err == nil && r.Value.Bool()
	}
	if state() == want {
		return nil
	}
	if err := l.withTimeout(api, ms, el, func(e *rod.Element) error {
		return e.Click(proto.InputMouseButtonLeft, 1)
	}); err != nil {
		return err
	}
	if state() != want {
		return fmt.Errorf("Locator.%s: Clicking the checkbox did not change its state", api)
	}
	return nil
}

func (t *tab) typeRune(r rune) error {
	if r == '\n' {
		r = '\r'
	}
	if k := rodKey(r); k != 0 {
		return t.page.Keyboard.Type(k)
	}
	return t.page.InsertText(string(r))
}

// press holds the modifiers of "Control+a" while the last key goes down and up.
func (t *tab) press(spec string) error {
	keys, err := parseKey(spec)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := t.page.Keyboard.Press(k); err != nil {
			return err
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if err := t.page.Keyboard.Release(keys[i]); err != nil {
			return err
		}
	}
	return nil
}

// history is go_back / go_forward / reload.
func (t *tab) history(direction string, ms int) error {
	if direction == "reload" {
		if err := (proto.PageReload{}).Call(t.page); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		return t.waitState("load", ms)
	}
	h, err := proto.PageGetNavigationHistory{}.Call(t.page)
	if err != nil {
		return err
	}
	i := h.CurrentIndex - 1
	if direction == "forward" {
		i = h.CurrentIndex + 1
	}
	if i < 0 || i >= len(h.Entries) {
		return nil // Playwright returns null: nothing to go to
	}
	if err := (proto.PageNavigateToHistoryEntry{EntryID: h.Entries[i].ID}).Call(t.page); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (l *loc) extract(all bool, attr string, ms int) (any, error) {
	if _, err := l.waitFor("wait_for", "attached", ms); err != nil {
		return nil, err
	}
	els, err := l.all()
	if err != nil {
		return nil, err
	}
	if all {
		out := []any{}
		for _, el := range els {
			v, err := elementValue(el, attr)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	if len(els) == 0 {
		return nil, l.timeoutErr("inner_text", ms, "")
	}
	return elementValue(els[0], attr)
}

func elementValue(el *rod.Element, attr string) (any, error) {
	if attr != "" {
		r, err := el.Eval(`function(a) { return this.getAttribute(a) }`, attr)
		if err != nil {
			return nil, err
		}
		if r.Value.Nil() {
			return nil, nil
		}
		return r.Value.Str(), nil
	}
	r, err := el.Eval(`function() { return this.innerText }`)
	if err != nil {
		return nil, err
	}
	return r.Value.Str(), nil
}

func (l *loc) findAll(ms int) (any, error) {
	if _, err := l.waitFor("wait_for", "attached", ms); err != nil {
		return nil, err
	}
	els, err := l.all()
	if err != nil {
		return nil, err
	}
	list := []any{}
	for i, el := range els {
		text, err := elementValue(el, "")
		if err != nil {
			return nil, err
		}
		list = append(list, omap.New("index", i, "text", text, "visible", l.visible(el)))
	}
	return omap.New("found", len(els), "elements", list), nil
}
