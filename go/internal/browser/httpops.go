package browser

import (
	"errors"
	"fmt"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// firstOf is resolve_locator(...).first on a tab of the session.
func (s *Session) firstOf(pageID string, locator scenario.Locator) (*tab, *loc, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, nil, err
	}
	l, err := t.locator(locator, nil)
	if err != nil {
		return nil, nil, err
	}
	l.repr += ".first"
	return t, l, nil
}

// WaitFor is actions.wait_for_element: the first match reaches state
// (attached, detached, visible or hidden).
func (s *Session) WaitFor(pageID string, locator scenario.Locator, state string, ms int) error {
	switch state {
	case "attached", "visible", "hidden":
		_, l, err := s.firstOf(pageID, locator)
		if err != nil {
			return err
		}
		_, err = l.waitFor("wait_for", state, ms)
		return err
	case "detached":
		_, l, err := s.firstOf(pageID, locator)
		if err != nil {
			return err
		}
		ok, err := poll(ms, func() (bool, error) {
			el, err := l.first()
			return el == nil, err
		})
		if err != nil {
			return err
		}
		if !ok {
			return l.timeoutErr("wait_for", ms, " to be detached")
		}
		return nil
	}
	return errors.New("Locator.wait_for: state: expected one of (attached|detached|visible|hidden)")
}

// MoveToElement is actions.mouse_move_to_element: moves the mouse to the
// centre of the first match and returns that point.
func (s *Session) MoveToElement(pageID string, locator scenario.Locator, ms int) (*omap.Map, error) {
	t, l, err := s.firstOf(pageID, locator)
	if err != nil {
		return nil, err
	}
	el, err := l.waitFor("bounding_box", "attached", ms)
	if err != nil {
		return nil, err
	}
	var box *proto.DOMRect
	if shape, err := el.Shape(); err == nil {
		box = shape.Box()
	}
	if box == nil {
		return nil, errors.New("Element has no bounding box (not visible?)")
	}
	cx, cy := box.X+box.Width/2, box.Y+box.Height/2
	if err := t.page.Mouse.MoveTo(proto.Point{X: cx, Y: cy}); err != nil {
		return nil, err
	}
	return omap.New("x", cx, "y", cy), nil
}

// Wheel is actions.mouse_scroll.
func (s *Session) Wheel(pageID string, dx, dy float64) error {
	t, err := s.tab(pageID)
	if err != nil {
		return err
	}
	return t.page.Mouse.Scroll(dx, dy, 1)
}

// Evaluate is actions.evaluate: the main world, or an isolated world when
// isolated is set (page globals are not visible there).
func (s *Session) Evaluate(pageID, expr string, isolated bool) (any, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	if !isolated {
		return t.evaluate(expr)
	}
	world, err := proto.PageCreateIsolatedWorld{FrameID: t.page.FrameID, WorldName: "utility"}.Call(t.page)
	if err != nil {
		return nil, err
	}
	res, err := proto.RuntimeCallFunctionOn{
		FunctionDeclaration: `(expr) => {
			const v = (0, eval)(expr);
			return typeof v === 'function' ? v() : v;
		}`,
		Arguments:          []*proto.RuntimeCallArgument{{Value: gson.New(expr)}},
		ExecutionContextID: world.ExecutionContextID,
		AwaitPromise:       true,
		ReturnByValue:      true,
	}.Call(t.page)
	if err != nil {
		return nil, err
	}
	if d := res.ExceptionDetails; d != nil {
		msg := d.Text
		if d.Exception != nil {
			msg = exceptionMessage(d.Exception)
		}
		return nil, fmt.Errorf("Page.evaluate: %s", msg)
	}
	return toValue([]byte(res.Result.Value.JSON("", "")))
}

// exceptionMessage is Playwright's getExceptionMessage: description || String(value).
func exceptionMessage(exc *proto.RuntimeRemoteObject) string {
	if exc.Description != "" {
		return exc.Description
	}
	if s, ok := exc.Value.Val().(string); ok {
		return s
	}
	return exc.Value.JSON("", "")
}
