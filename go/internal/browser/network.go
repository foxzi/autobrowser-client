package browser

import (
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

var sensitiveHeaders = map[string]bool{"authorization": true, "cookie": true, "set-cookie": true, "proxy-authorization": true}

// maskHeaders is _mask_headers.
func maskHeaders(h *omap.Map) *omap.Map {
	out := omap.New()
	for _, k := range h.Keys() {
		v, _ := h.Get(k)
		if s, ok := v.(string); ok && sensitiveHeaders[strings.ToLower(k)] {
			if r := []rune(s); len(r) > 8 {
				v = string(r[:8]) + "***"
			} else {
				v = "****"
			}
		}
		out.Set(k, v)
	}
	return out
}

// headersEntry is HeadersEntry: the headers of one document response.
type headersEntry struct {
	url    string
	req    *omap.Map
	resp   *omap.Map
	status int
}

func (e *headersEntry) toMap() *omap.Map {
	return omap.New("url", e.url, "request", maskHeaders(e.req), "response", maskHeaders(e.resp), "statusCode", e.status)
}

// lowerHeaders gives Playwright's header object: lower-case names. CDP
// headers arrive as a Go map, so the names are sorted.
func lowerHeaders(h proto.NetworkHeaders) *omap.Map {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := omap.New()
	for _, k := range keys {
		out.Set(strings.ToLower(k), h[k].Str())
	}
	return out
}

// network is the per-tab state of the response listener.
type network struct {
	mu      sync.Mutex
	headers []*headersEntry
	pending map[proto.NetworkRequestID]*proto.NetworkRequestWillBeSent
	// onNav gets the main-frame navigation responses while set (solve-challenge).
	onNav func(headers *omap.Map, status int)
}

// listen enables the Network domain of the tab and records document responses
// like _attach_listeners; the goroutine ends with the browser connection.
func (t *tab) listen() {
	t.net.pending = map[proto.NetworkRequestID]*proto.NetworkRequestWillBeSent{}
	if err := (proto.NetworkEnable{}).Call(t.page); err != nil {
		slog.Warn("Failed to enable network events for " + t.id + ": " + err.Error())
		return
	}
	events := t.page.Event()
	go func() {
		for msg := range events {
			var req proto.NetworkRequestWillBeSent
			var resp proto.NetworkResponseReceived
			if msg.Load(&req) {
				t.onRequest(&req)
			} else if msg.Load(&resp) {
				t.onResponse(&resp)
			}
		}
	}()
}

// Playwright ignores data: URLs; a redirect is the response of the previous request.
func (t *tab) onRequest(e *proto.NetworkRequestWillBeSent) {
	if e.Type != proto.NetworkResourceTypeDocument || strings.HasPrefix(e.Request.URL, "data:") {
		return
	}
	t.net.mu.Lock()
	prev := t.net.pending[e.RequestID]
	t.net.pending[e.RequestID] = e
	t.net.mu.Unlock()
	if prev != nil && e.RedirectResponse != nil {
		t.record(prev, e.RedirectResponse)
	}
}

func (t *tab) onResponse(e *proto.NetworkResponseReceived) {
	if e.Type != proto.NetworkResourceTypeDocument {
		return
	}
	t.net.mu.Lock()
	req := t.net.pending[e.RequestID]
	delete(t.net.pending, e.RequestID)
	t.net.mu.Unlock()
	if req != nil {
		t.record(req, e.Response)
	}
}

func (t *tab) record(req *proto.NetworkRequestWillBeSent, resp *proto.NetworkResponse) {
	entry := &headersEntry{url: req.Request.URL, req: lowerHeaders(req.Request.Headers),
		resp: lowerHeaders(resp.Headers), status: resp.Status}
	t.net.mu.Lock()
	t.net.headers = append(t.net.headers, entry)
	hook := t.net.onNav
	t.net.mu.Unlock()
	isNav := string(req.RequestID) == string(req.LoaderID)
	if hook != nil && isNav && string(req.FrameID) == string(t.target) {
		hook(entry.resp, entry.status)
	}
}

func (t *tab) setNavHook(fn func(headers *omap.Map, status int)) {
	t.net.mu.Lock()
	t.net.onNav = fn
	t.net.mu.Unlock()
}

// Headers is BrowserSession.get_headers: the masked document headers of a
// tab (the first one when pageID is empty), oldest first.
func (s *Session) Headers(pageID string) []any {
	out := []any{}
	t, err := s.tab(pageID)
	if err != nil {
		return out
	}
	t.net.mu.Lock()
	defer t.net.mu.Unlock()
	for _, e := range t.net.headers {
		out = append(out, e.toMap())
	}
	return out
}

// Cookies is BrowserSession.get_cookies: context.cookies() of Playwright.
func (s *Session) Cookies() ([]any, error) {
	if s.ctx == nil {
		return nil, apperr.New(apperr.BrowserStartFailed, "Browser context is not initialized")
	}
	cookies, err := s.ctx.GetCookies()
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, c := range cookies {
		sameSite := string(c.SameSite)
		if sameSite == "" {
			sameSite = "Lax"
		}
		m := omap.New("name", c.Name, "value", c.Value, "domain", c.Domain, "path", c.Path,
			"expires", jsNumber(float64(c.Expires)), "httpOnly", c.HTTPOnly, "secure", c.Secure, "sameSite", sameSite)
		if c.PartitionKey != nil {
			m.Set("partitionKey", c.PartitionKey.TopLevelSite)
			m.Set("_crHasCrossSiteAncestor", c.PartitionKey.HasCrossSiteAncestor)
		}
		out = append(out, m)
	}
	return out, nil
}

// jsNumber is how a JS number reaches Python through JSON: whole numbers are ints.
func jsNumber(f float64) any {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return int(f)
	}
	return f
}
