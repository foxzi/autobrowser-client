package browser

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// selectJS is Playwright's InjectedScript.selectOptions for string options
// (matched by value or label); "error:..." results are retried.
const selectJS = `function(want) {
	if (this.nodeName.toLowerCase() !== 'select') return 'error:notselect';
	const norm = s => s.replace(/\s+/g, ' ').trim();
	const options = [...this.options];
	const selected = [];
	let remaining = want.slice();
	for (const option of options) {
		const filter = w => w === option.value || w === option.label || norm(w) === norm(option.label);
		if (!remaining.some(filter)) continue;
		if (option.disabled || option.closest('optgroup:disabled')) return 'error:optionnotenabled';
		selected.push(option);
		if (this.multiple) {
			remaining = remaining.filter(w => !filter(w));
		} else {
			remaining = [];
			break;
		}
	}
	if (remaining.length) return 'error:optionsnotfound';
	this.value = undefined;
	selected.forEach(o => o.selected = true);
	this.dispatchEvent(new Event('input', {bubbles: true, composed: true}));
	this.dispatchEvent(new Event('change', {bubbles: true}));
	return selected.map(o => o.value);
}`

// readJS is read_element: input_value and is_checked are null where Playwright throws.
const readJS = `function() {
	const tag = this.nodeName;
	const role = this.getAttribute('role');
	let checked = null;
	if (tag === 'INPUT' && ['checkbox', 'radio'].includes(this.type)) checked = this.checked;
	else if (['checkbox', 'radio', 'switch', 'menuitemcheckbox', 'menuitemradio', 'option', 'treeitem'].includes(role)
		&& ['true', 'false'].includes(this.getAttribute('aria-checked'))) checked = this.getAttribute('aria-checked') === 'true';
	return {
		text: this.innerText,
		innerHTML: this.innerHTML,
		enabled: !this.matches(':disabled') && !this.closest('[aria-disabled="true"]'),
		checked,
		value: ['INPUT', 'TEXTAREA', 'SELECT'].includes(tag) ? this.value : null,
	};
}`

// firstLoc is resolve_locator(...).first: these actions ignore index.
func (t *tab) firstLoc(step *scenario.Step) (*loc, error) {
	if step.Locator == nil {
		return nil, need(step, "locator")
	}
	l, err := t.locator(*step.Locator, nil)
	if err != nil {
		return nil, err
	}
	l.repr += ".first"
	return l, nil
}

// submit calls form.submit() on the closest form of the element.
func (l *loc) submit(ms int) error {
	el, err := l.waitFor("wait_for", "attached", ms)
	if err != nil {
		return err
	}
	_, err = el.Eval(`function() { const f = this.closest('form'); if (f) f.submit(); }`)
	return err
}

// selectOption is Locator.select_option with values or labels.
func (l *loc) selectOption(options []string, ms int) ([]any, error) {
	start := time.Now()
	if _, err := l.waitFor("wait_for", "attached", ms); err != nil {
		return nil, err
	}
	el, err := l.actionable("select_option", ms)
	if err != nil {
		return nil, err
	}
	var out []any
	ok, err := poll(max(ms-int(time.Since(start).Milliseconds()), 1), func() (bool, error) {
		r, err := el.Eval(selectJS, options)
		if err != nil {
			return false, err
		}
		if s, isErr := r.Value.Val().(string); s == "error:notselect" {
			return false, fmt.Errorf("Locator.select_option: Error: Element is not a <select> element\nCall log:\n  - waiting for %s\n", l.repr)
		} else if isErr {
			return false, nil // an "error:..." result: retry
		}
		for _, v := range r.Value.Arr() {
			out = append(out, v.Str())
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, l.timeoutErr("select_option", ms, "\n  - did not find some options")
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}

// read is read_element.
func (l *loc) read(ms int) (*omap.Map, error) {
	el, err := l.waitFor("wait_for", "attached", ms)
	if err != nil {
		return nil, err
	}
	r, err := el.Eval(readJS)
	if err != nil {
		return nil, err
	}
	m := r.Value.Map()
	nullable := func(k string) any {
		if m[k].Nil() {
			return nil
		}
		return m[k].Val()
	}
	return omap.New(
		"text", m["text"].Str(),
		"innerHTML", m["innerHTML"].Str(),
		"visible", l.visible(el),
		"enabled", m["enabled"].Bool(),
		"checked", nullable("checked"),
		"value", nullable("value"),
	), nil
}

// upload is upload_files: set the files of a file input, or of the chooser
// opened by clicking the element.
func (l *loc) upload(files []string, ms int) (*omap.Map, error) {
	// Playwright reads the files only when setting them, so a missing file
	// fails after the chooser wait, like in Python.
	resolve := func() ([]string, error) {
		paths := make([]string, len(files))
		for i, f := range files {
			if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("[Errno 2] No such file or directory: '%s'", f)
			}
			paths[i], _ = filepath.Abs(f)
		}
		return paths, nil
	}
	el, err := l.waitFor("evaluate", "attached", ms)
	if err != nil {
		return nil, err
	}
	r, err := el.Eval(`function() { return this.tagName === 'INPUT' && this.type === 'file' }`)
	if err != nil {
		return nil, err
	}
	if r.Value.Bool() {
		paths, err := resolve()
		if err != nil {
			return nil, err
		}
		if err := el.SetFiles(paths); err != nil {
			return nil, err
		}
		return omap.New("files", toAny(files)), nil
	}
	p := l.t.page
	if err := (proto.PageSetInterceptFileChooserDialog{Enabled: true}).Call(p); err != nil {
		return nil, err
	}
	defer func() { _ = proto.PageSetInterceptFileChooserDialog{Enabled: false}.Call(p) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
	defer cancel()
	var e proto.PageFileChooserOpened
	wait := p.Context(ctx).WaitEvent(&e)
	if err := l.pointer("click", ms); err != nil {
		return nil, err
	}
	wait()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("Timeout %dms exceeded while waiting for event \"filechooser\"\n"+
			"=========================== logs ===========================\n"+
			"waiting for event \"filechooser\"\n"+
			"============================================================", ms)
	}
	paths, err := resolve()
	if err != nil {
		return nil, err
	}
	if err := (proto.DOMSetFileInputFiles{Files: paths, BackendNodeID: e.BackendNodeID}).Call(p); err != nil {
		return nil, err
	}
	return omap.New("files", toAny(files)), nil
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// mouseRandom is mouse_move_random: a cubic bezier from start to end with
// gaussian jitter on the control points and on every step (smaller near the
// ends). It returns the number of points.
func (t *tab) mouseRandom(sx, sy, ex, ey float64, steps, durationMs int, jitter float64, seed *int) (int, error) {
	src := rand.NewSource(time.Now().UnixNano())
	if seed != nil {
		src = rand.NewSource(int64(*seed))
	}
	rng := rand.New(src)
	gauss := func(sigma float64) float64 { return rng.NormFloat64() * sigma }

	c1x := sx + (ex-sx)/3 + gauss(jitter*5)
	c1y := sy + (ey-sy)/3 + gauss(jitter*5)
	c2x := sx + 2*(ex-sx)/3 + gauss(jitter*5)
	c2y := sy + 2*(ey-sy)/3 + gauss(jitter*5)

	n := max(steps, 1)
	delay := time.Duration(float64(durationMs) / float64(n) * float64(time.Millisecond))
	for i := 0; i <= steps; i++ {
		tt := float64(i) / float64(n)
		u := 1 - tt
		x := math.Pow(u, 3)*sx + 3*u*u*tt*c1x + 3*u*tt*tt*c2x + math.Pow(tt, 3)*ex
		y := math.Pow(u, 3)*sy + 3*u*u*tt*c1y + 3*u*tt*tt*c2y + math.Pow(tt, 3)*ey
		edge := 4 * tt * (1 - tt) // 0 at the ends, 1 in the middle
		x += gauss(jitter * edge)
		y += gauss(jitter * edge)
		if err := t.page.Mouse.MoveTo(proto.Point{X: x, Y: y}); err != nil {
			return 0, err
		}
		if i < steps {
			time.Sleep(delay)
		}
	}
	return steps + 1, nil
}

// browseTabs visits every open tab: bring it to front, wait for load and
// scroll down a few times with random pauses.
func (s *Session) browseTabs(dy, ms int) *omap.Map {
	s.sync()
	s.mu.Lock()
	tabs := append([]*tab(nil), s.pages...)
	s.mu.Unlock()
	visited := []any{}
	for _, t := range tabs {
		u, _ := t.url()
		if u == "about:blank" || u == "chrome://newtab/" {
			continue
		}
		err := func() error {
			if err := (proto.PageBringToFront{}).Call(t.page); err != nil {
				return err
			}
			if err := t.waitState("load", ms); err != nil {
				return err
			}
			for range 2 + rand.Intn(4) {
				if err := t.page.Mouse.Scroll(0, float64(dy), 1); err != nil {
					return err
				}
				time.Sleep(time.Duration(500+rand.Intn(1501)) * time.Millisecond)
			}
			return nil
		}()
		if err != nil {
			slog.Warn(fmt.Sprintf("browse-tabs: failed on %s (%s): %s", t.id, u, err))
			visited = append(visited, omap.New("page", t.id, "url", u, "error", err.Error()))
			continue
		}
		visited = append(visited, omap.New("page", t.id, "url", u))
	}
	return omap.New("visited", visited, "count", len(visited))
}
