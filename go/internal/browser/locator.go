package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// locatorJS returns the elements of a locator as an array, in document order.
// css and xpath are native; text is Playwright's get_by_text: a case-insensitive,
// whitespace-normalised substring match on the smallest matching elements.
const locatorJS = `(kind, value) => {
	if (kind === 'xpath') {
		const r = document.evaluate(value, document, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, null);
		const out = [];
		for (let i = 0; i < r.snapshotLength; i++) out.push(r.snapshotItem(i));
		return out.filter(n => n.nodeType === 1);
	}
	if (kind === 'text') {
		const norm = s => s.replace(/\s+/g, ' ').trim().toLowerCase();
		const want = norm(value);
		const has = el => norm(el.textContent || '').includes(want);
		const out = [];
		const walk = root => {
			for (const el of root.querySelectorAll('*')) {
				if (['SCRIPT', 'STYLE', 'NOSCRIPT', 'HEAD', 'TITLE', 'META', 'LINK'].includes(el.tagName)) continue;
				if (!has(el)) continue;
				if ([...el.children].some(c => has(c))) continue;
				out.push(el);
			}
		};
		walk(document);
		return out;
	}
	return Array.from(document.querySelectorAll(value));
}`

// visibleJS is Playwright's visibility: a non-empty box and not visibility:hidden.
const visibleJS = `function() {
	const r = this.getBoundingClientRect();
	const st = getComputedStyle(this);
	return r.width > 0 && r.height > 0 && st.visibility !== 'hidden';
}`

// selector turns a locator into the (kind, value) pair of locatorJS.
func selector(loc scenario.Locator) (kind, value, repr string, err error) {
	switch loc.Type {
	case "xpath":
		return "xpath", loc.Value, fmt.Sprintf("locator(%q)", "xpath="+loc.Value), nil
	case "text":
		return "text", loc.Value, fmt.Sprintf("get_by_text(%q)", loc.Value), nil
	case "ref":
		return "ref", loc.Value, fmt.Sprintf("locator(%q)", "aria-ref="+loc.Value), nil
	}
	css := loc.Value
	switch loc.Type {
	case "id":
		css = "#" + loc.Value
	case "name":
		css = "[name='" + loc.Value + "']"
	case "class":
		css = "." + loc.Value
	case "attribute":
		css = "[" + loc.Value + "]"
	}
	return "css", css, fmt.Sprintf("locator(%q)", css), nil
}

// loc is a locator bound to a tab; index >= 0 is nth(index).
type loc struct {
	t     *tab
	kind  string
	value string
	repr  string
	index *int
}

func (t *tab) locator(l scenario.Locator, index *int) (*loc, error) {
	kind, value, repr, err := selector(l)
	if err != nil {
		return nil, err
	}
	if index != nil {
		repr += fmt.Sprintf(".nth(%d)", *index)
	}
	return &loc{t: t, kind: kind, value: value, repr: repr, index: index}, nil
}

// all returns the current matches.
func (l *loc) all() (rod.Elements, error) {
	var els rod.Elements
	var err error
	if l.kind == "ref" {
		els, err = l.t.resolveRef(l.value, l.index)
	} else {
		els, err = l.t.page.ElementsByJS(rod.Eval(locatorJS, l.kind, l.value))
	}
	if err != nil {
		return nil, err
	}
	if l.index != nil {
		if *l.index < 0 || *l.index >= len(els) {
			return nil, nil
		}
		return rod.Elements{els[*l.index]}, nil
	}
	return els, nil
}

func (l *loc) count() (int, error) {
	els, err := l.all()
	return len(els), err
}

func (l *loc) first() (*rod.Element, error) {
	els, err := l.all()
	if err != nil || len(els) == 0 {
		return nil, err
	}
	return els[0], nil
}

func (l *loc) visible(el *rod.Element) bool {
	res, err := el.Eval(visibleJS)
	return err == nil && res.Value.Bool()
}

// timeoutErr is the Playwright text of an expired wait.
func (l *loc) timeoutErr(api string, ms int, suffix string) error {
	return fmt.Errorf("Locator.%s: Timeout %dms exceeded.\nCall log:\n  - waiting for %s%s\n", api, ms, l.repr, suffix)
}

// poll runs fn every 100 ms until it reports done or the timeout expires.
func poll(ms int, fn func() (bool, error)) (bool, error) {
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	for {
		done, err := fn()
		if err != nil {
			return false, err
		}
		if done {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitFor is Locator.wait_for on the first match; state is attached, visible or hidden.
func (l *loc) waitFor(api, state string, ms int) (*rod.Element, error) {
	var found *rod.Element
	ok, err := poll(ms, func() (bool, error) {
		el, err := l.first()
		var bad invalidSelector
		if errors.As(err, &bad) {
			return false, fmt.Errorf("Locator.%s: %s\nCall log:\n  - waiting for %s\n", api, bad, l.repr)
		}
		if err != nil {
			return false, err
		}
		switch state {
		case "attached":
			found = el
			return el != nil, nil
		case "visible":
			found = el
			return el != nil && l.visible(el), nil
		}
		return el == nil || !l.visible(el), nil // hidden: detached counts too
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		suffix := ""
		if state != "attached" {
			suffix = " to be " + state
		}
		return nil, l.timeoutErr(api, ms, suffix)
	}
	return found, nil
}

// actionable waits for the first match, then for it to be visible, scrolls to
// it and returns it ready for input.
func (l *loc) actionable(api string, ms int) (*rod.Element, error) {
	start := time.Now()
	el, err := l.waitFor(api, "attached", ms)
	if err != nil {
		return nil, err
	}
	left := max(ms-int(time.Since(start).Milliseconds()), 1)
	ok, err := poll(left, func() (bool, error) { return l.visible(el), nil })
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, l.timeoutErr(api, ms, "\n  - element is not visible")
	}
	if err := el.ScrollIntoView(); err != nil {
		return nil, err
	}
	return el, nil
}

// toValue converts a JS result (raw JSON) to an ordered Go value.
func toValue(raw []byte) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	return omap.ParseJSON(raw)
}

func jsonArg(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
