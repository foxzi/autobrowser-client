package browser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

const (
	cfChallengeHost = "challenges.cloudflare.com"
	cfCheckJS       = `() => typeof window._cf_chl_opt !== 'undefined'`
	navStatusJS     = `() => performance.getEntriesByType('navigation')[0]?.responseStatus`
)

// cfNow is the clock of the challenge loop (replaced in tests).
var cfNow = time.Now

type cfFrame struct{ id, url string }

type cfBox struct{ x, y, width, height float64 }

// cfPage is what the challenge loop needs from a page.
type cfPage interface {
	cfCheck() (bool, error) // cfCheckJS in the main world
	frames() []cfFrame
	frameBox(f cfFrame) (*cfBox, error) // box of the frame's owner element
	click(x, y float64) error
	pageURL() string
	sleep(ms int)
}

// cfState is updated by the response listener: cf-mitigated and the HTTP status
// of the last main-frame navigation response.
type cfState struct {
	mu        sync.Mutex
	mitigated bool
	status    *int
}

func (st *cfState) onNav(headers *omap.Map, status int) {
	v, _ := headers.Get("cf-mitigated")
	st.mu.Lock()
	st.mitigated = v == "challenge"
	st.status = &status
	st.mu.Unlock()
}

// isCFChallenge is _is_cf_challenge: true while the page shows a challenge (or is navigating).
func isCFChallenge(p cfPage, st *cfState) bool {
	st.mu.Lock()
	mitigated := st.mitigated
	st.mu.Unlock()
	if mitigated {
		return true
	}
	ok, err := p.cfCheck()
	if err != nil || ok {
		return true // an error is a context destroyed by navigation: check again later
	}
	for _, f := range p.frames() {
		if strings.Contains(f.url, cfChallengeHost) {
			return true
		}
	}
	return false
}

// clickTurnstile is _click_turnstile: click the checkbox if its iframe is visible.
func clickTurnstile(p cfPage) bool {
	for _, f := range p.frames() {
		if !strings.Contains(f.url, cfChallengeHost) {
			continue
		}
		box, err := p.frameBox(f)
		if err != nil {
			continue
		}
		if box != nil && box.width > 0 {
			_ = p.click(box.x+30, box.y+box.height/2)
			return true
		}
	}
	return false
}

// SolveChallenge is solve_cf_challenge: wait for a Cloudflare challenge to pass,
// clicking Turnstile when shown. It returns once the page has been
// challenge-free for 3 consecutive seconds and fails if the page after the
// challenge answers with HTTP >= 400.
func (s *Session) SolveChallenge(pageID string, timeoutMs int) (*omap.Map, error) {
	t, err := s.tab(pageID)
	if err != nil {
		return nil, err
	}
	st := &cfState{}
	if res, err := t.page.Eval(navStatusJS); err == nil && res.Value.Int() != 0 {
		st.status = ptr(res.Value.Int())
	}
	t.setNavHook(st.onNav)
	defer t.setNavHook(nil)
	return waitCFChallenge(t, timeoutMs, st)
}

func waitCFChallenge(p cfPage, timeoutMs int, st *cfState) (*omap.Map, error) {
	t0 := cfNow()
	deadline := t0.Add(time.Duration(timeoutMs) * time.Millisecond)
	var lastClick time.Time
	clicks := 0
	seen := false
	okStreak := 0
	for cfNow().Before(deadline) {
		if isCFChallenge(p, st) {
			seen = true
			okStreak = 0
			now := cfNow()
			// Give the non-interactive check a few seconds before clicking
			if now.Sub(t0) > 4*time.Second && (lastClick.IsZero() || now.Sub(lastClick) > 8*time.Second) && clickTurnstile(p) {
				lastClick = now
				clicks++
				slog.Info(fmt.Sprintf("Clicked Cloudflare Turnstile checkbox (%d)", clicks))
			}
		} else {
			okStreak++
			if okStreak >= 3 {
				st.mu.Lock()
				status := st.status
				st.mu.Unlock()
				if status != nil && *status >= 400 {
					reason := "without challenge"
					if seen {
						reason = "after challenge"
					}
					return nil, apperr.New(apperr.InternalErr,
						fmt.Sprintf("Page blocked %s: HTTP %d at %s", reason, *status, p.pageURL()))
				}
				return omap.New("challenge", seen, "clicks", clicks, "url", p.pageURL()), nil
			}
		}
		p.sleep(1000)
	}
	return nil, apperr.New(apperr.Timeout, fmt.Sprintf("Cloudflare challenge not passed in %d ms (clicks: %d)", timeoutMs, clicks))
}

func (t *tab) cfCheck() (bool, error) {
	res, err := t.page.Eval(cfCheckJS)
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// frames is page.frames: the frame tree of the page plus its out-of-process
// iframes, which are separate targets and missing from that tree.
func (t *tab) frames() []cfFrame {
	tree, err := proto.PageGetFrameTree{}.Call(t.page)
	if err != nil {
		return nil
	}
	var out []cfFrame
	ids := map[string]bool{}
	var walk func(n *proto.PageFrameTree)
	walk = func(n *proto.PageFrameTree) {
		out = append(out, cfFrame{id: string(n.Frame.ID), url: n.Frame.URL + n.Frame.URLFragment})
		ids[string(n.Frame.ID)] = true
		for _, c := range n.ChildFrames {
			walk(c)
		}
	}
	walk(tree.FrameTree)
	// rod's TargetInfo has no parentFrameId, so read the raw reply
	raw, err := t.page.Call(t.page.GetContext(), string(t.page.SessionID), "Target.getTargets", map[string]any{})
	if err != nil {
		return out
	}
	var res struct {
		TargetInfos []struct {
			TargetID      string `json:"targetId"`
			Type          string `json:"type"`
			URL           string `json:"url"`
			ParentFrameID string `json:"parentFrameId"`
		} `json:"targetInfos"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return out
	}
	for added := true; added; {
		added = false
		for _, ti := range res.TargetInfos {
			if ti.Type == "iframe" && !ids[ti.TargetID] && ids[ti.ParentFrameID] {
				out = append(out, cfFrame{id: ti.TargetID, url: ti.URL})
				ids[ti.TargetID] = true
				added = true
			}
		}
	}
	return out
}

// frameBox is frame.frame_element().bounding_box(): the border box of the
// owner element, found even inside closed shadow roots.
func (t *tab) frameBox(f cfFrame) (*cfBox, error) {
	owner, err := proto.DOMGetFrameOwner{FrameID: proto.PageFrameID(f.id)}.Call(t.page)
	if err != nil {
		return nil, err
	}
	bm, err := proto.DOMGetBoxModel{BackendNodeID: owner.BackendNodeID}.Call(t.page)
	if err != nil {
		return nil, nil // not rendered: no box
	}
	q := bm.Model.Border
	if len(q) < 8 {
		return nil, nil
	}
	minX, maxX, minY, maxY := q[0], q[0], q[1], q[1]
	for i := 0; i < 8; i += 2 {
		minX, maxX = min(minX, q[i]), max(maxX, q[i])
		minY, maxY = min(minY, q[i+1]), max(maxY, q[i+1])
	}
	return &cfBox{x: minX, y: minY, width: maxX - minX, height: maxY - minY}, nil
}

func (t *tab) click(x, y float64) error {
	if err := t.page.Mouse.MoveTo(proto.Point{X: x, Y: y}); err != nil {
		return err
	}
	return t.page.Mouse.Click(proto.InputMouseButtonLeft, 1)
}

func (t *tab) pageURL() string {
	u, _ := t.url()
	return u
}

func (t *tab) sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }
