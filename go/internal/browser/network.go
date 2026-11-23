package browser

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

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
	logs    map[string][]any // console, network, dialogs: the last logLimit items each
	// onNav gets the main-frame navigation responses while set (solve-challenge).
	onNav func(headers *omap.Map, status int)
}

// logLimit is the deque maxlen of the per-page logs.
const logLimit = 500

// listen enables the Network, Log and Page domains of the tab and records
// responses, console entries and dialogs like _attach_listeners; the
// goroutine ends with the browser connection.
func (t *tab) listen() {
	t.net.pending = map[proto.NetworkRequestID]*proto.NetworkRequestWillBeSent{}
	t.net.logs = map[string][]any{"console": {}, "network": {}, "dialogs": {}}
	for _, c := range []interface{ Call(proto.Client) error }{proto.NetworkEnable{}, proto.LogEnable{}, proto.PageEnable{}} {
		if err := c.Call(t.page); err != nil {
			slog.Warn("Failed to enable events for " + t.id + ": " + err.Error())
			return
		}
	}
	events := t.page.Event()
	go func() {
		for msg := range events {
			var req proto.NetworkRequestWillBeSent
			var resp proto.NetworkResponseReceived
			var failed proto.NetworkLoadingFailed
			var entry proto.LogEntryAdded
			var dialog proto.PageJavascriptDialogOpening
			switch {
			case msg.Load(&req):
				t.onRequest(&req)
			case msg.Load(&resp):
				t.onResponse(&resp)
			case msg.Load(&failed):
				t.onFailed(&failed)
			case msg.Load(&entry):
				if entry.Entry.Source != proto.LogLogEntrySourceWorker {
					t.log("console", omap.New("type", string(entry.Entry.Level), "text", entry.Entry.Text, "time", now()))
				}
			case msg.Load(&dialog):
				t.onDialog(&dialog)
			}
		}
	}()
}

// Playwright ignores data: URLs; a redirect is the response of the previous request.
func (t *tab) onRequest(e *proto.NetworkRequestWillBeSent) {
	if strings.HasPrefix(e.Request.URL, "data:") {
		return
	}
	t.net.mu.Lock()
	prev := t.net.pending[e.RequestID]
	t.net.pending[e.RequestID] = e
	t.net.mu.Unlock()
	if prev != nil && e.RedirectResponse != nil {
		t.respond(prev, e.RedirectResponse)
	}
}

func (t *tab) onResponse(e *proto.NetworkResponseReceived) {
	t.net.mu.Lock()
	req := t.net.pending[e.RequestID]
	delete(t.net.pending, e.RequestID)
	t.net.mu.Unlock()
	if req != nil {
		t.respond(req, e.Response)
	}
}

func (t *tab) onFailed(e *proto.NetworkLoadingFailed) {
	t.net.mu.Lock()
	req := t.net.pending[e.RequestID]
	delete(t.net.pending, e.RequestID)
	t.net.mu.Unlock()
	if req != nil {
		t.log("network", omap.New("method", req.Request.Method, "url", req.Request.URL,
			"resourceType", resourceType(req), "status", nil, "failure", e.ErrorText))
	}
}

// respond is the "response" event: the network log, and the headers of documents.
func (t *tab) respond(req *proto.NetworkRequestWillBeSent, resp *proto.NetworkResponse) {
	if req.Type == proto.NetworkResourceTypeDocument {
		t.record(req, resp)
	}
	t.log("network", omap.New("method", req.Request.Method, "url", req.Request.URL,
		"resourceType", resourceType(req), "status", resp.Status, "failure", nil))
}

// resourceType is request.resource_type of Playwright: the lower-case CDP type.
func resourceType(req *proto.NetworkRequestWillBeSent) string {
	if req.Type == "" {
		return "other"
	}
	return strings.ToLower(string(req.Type))
}

// onDialog answers a JS dialog by the dialog policy of the session and logs it.
func (t *tab) onDialog(e *proto.PageJavascriptDialogOpening) {
	accept, promptText := t.s.dialogPolicy()
	t.log("dialogs", omap.New("type", string(e.Type), "message", e.Message, "accepted", accept, "time", now()))
	// a map, as rod omits an empty promptText and a prompt is answered with ""
	params := map[string]any{"accept": accept}
	if accept && e.Type == proto.PageDialogTypePrompt {
		params["promptText"] = ""
		if promptText != nil {
			params["promptText"] = *promptText
		}
	}
	go func() {
		if _, err := t.page.Call(context.Background(), string(t.page.SessionID), "Page.handleJavaScriptDialog", params); err != nil {
			slog.Debug("Failed to answer a dialog: " + err.Error())
		}
	}()
}

func (t *tab) log(kind string, item *omap.Map) {
	t.net.mu.Lock()
	defer t.net.mu.Unlock()
	items := append(t.net.logs[kind], item)
	if len(items) > logLimit {
		items = items[len(items)-logLimit:]
	}
	t.net.logs[kind] = items
}

// now is time.time(): seconds since the epoch.
func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

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
	return s.CookiesFor(nil)
}

// CookiesFor is context.cookies(urls): only the cookies that affect one of
// the URLs, all of them without URLs.
func (s *Session) CookiesFor(urls []string) ([]any, error) {
	if s.ctx == nil {
		return nil, apperr.New(apperr.BrowserStartFailed, "Browser context is not initialized")
	}
	cookies, err := s.ctx.GetCookies()
	if err != nil {
		return nil, err
	}
	var parsed []*url.URL
	for _, u := range urls {
		p, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, p)
	}
	out := []any{}
	for _, c := range cookies {
		if len(parsed) > 0 && !cookieMatches(c, parsed) {
			continue
		}
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

// cookieMatches is filterCookies of Playwright.
func cookieMatches(c *proto.NetworkCookie, urls []*url.URL) bool {
	domain := c.Domain
	if !strings.HasPrefix(domain, ".") {
		domain = "." + domain
	}
	for _, u := range urls {
		host := u.Hostname()
		if !strings.HasSuffix("."+host, domain) {
			continue
		}
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, c.Path) {
			continue
		}
		local := host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "::1" || loopback.MatchString(host)
		if u.Scheme != "https" && !local && c.Secure {
			continue
		}
		return true
	}
	return false
}

var loopback = regexp.MustCompile(`^127\.\d+\.\d+\.\d+$`)

// AddCookies is context.add_cookies: each cookie needs a url or a domain and
// path; a url gives the domain, the path and secure.
func (s *Session) AddCookies(cookies []*omap.Map) error {
	if s.ctx == nil {
		return apperr.New(apperr.BrowserStartFailed, "Browser context is not initialized")
	}
	var params []*proto.NetworkCookieParam
	for _, c := range cookies {
		get := func(k string) any { v, _ := c.Get(k); return v }
		str := func(k string) string { v, _ := get(k).(string); return v }
		flag := func(k string) bool { v, _ := get(k).(bool); return v }
		p := &proto.NetworkCookieParam{Name: str("name"), Value: str("value"), Domain: str("domain"),
			Path: str("path"), HTTPOnly: flag("httpOnly"), Secure: flag("secure"),
			SameSite: proto.NetworkCookieSameSite(str("sameSite"))}
		rawURL := str("url")
		if rawURL == "" && (p.Domain == "" || p.Path == "") {
			return errors.New("BrowserContext.add_cookies: Cookie should have a url or a domain/path pair")
		}
		if rawURL != "" && p.Domain != "" {
			return errors.New("BrowserContext.add_cookies: Cookie should have either url or domain")
		}
		if rawURL != "" && p.Path != "" {
			return errors.New("BrowserContext.add_cookies: Cookie should have either url or path")
		}
		if rawURL != "" {
			u, err := url.Parse(rawURL)
			if err != nil {
				return err
			}
			p.Domain, p.Secure = u.Hostname(), u.Scheme == "https"
			p.Path = u.EscapedPath()[:strings.LastIndex(u.EscapedPath(), "/")+1]
		}
		switch e := get("expires").(type) {
		case int:
			p.Expires = proto.TimeSinceEpoch(e)
		case float64:
			p.Expires = proto.TimeSinceEpoch(e)
		}
		params = append(params, p)
	}
	return proto.StorageSetCookies{Cookies: params, BrowserContextID: s.ctx.BrowserContextID}.Call(s.ctx)
}

// ClearCookies is context.clear_cookies: all cookies, or with a filter only
// those whose name, domain and path equal the given ones, by expiring them.
func (s *Session) ClearCookies(name, domain, path *string) error {
	if s.ctx == nil {
		return apperr.New(apperr.BrowserStartFailed, "Browser context is not initialized")
	}
	if name == nil && domain == nil && path == nil {
		return proto.StorageClearCookies{BrowserContextID: s.ctx.BrowserContextID}.Call(s.ctx)
	}
	cookies, err := s.ctx.GetCookies()
	if err != nil {
		return err
	}
	eq := func(want *string, v string) bool { return want == nil || *want == v }
	var expire []*proto.NetworkCookieParam
	for _, c := range cookies {
		if eq(name, c.Name) && eq(domain, c.Domain) && eq(path, c.Path) {
			// Playwright sends expires 0, which the omitempty field drops:
			// one second after the epoch is as much in the past.
			expire = append(expire, &proto.NetworkCookieParam{Name: c.Name, Value: c.Value, Domain: c.Domain,
				Path: c.Path, HTTPOnly: c.HTTPOnly, Secure: c.Secure, SameSite: c.SameSite,
				Expires: 1, PartitionKey: c.PartitionKey})
		}
	}
	if len(expire) == 0 {
		return nil
	}
	return proto.StorageSetCookies{Cookies: expire, BrowserContextID: s.ctx.BrowserContextID}.Call(s.ctx)
}

// jsNumber is how a JS number reaches Python through JSON: whole numbers are ints.
func jsNumber(f float64) any {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return int(f)
	}
	return f
}
