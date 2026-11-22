// Package api is the HTTP API (port of api/app.py, a FastAPI app): the same
// routes, JSON shapes, status codes and error bodies.
package api

import (
	"crypto/subtle"
	_ "embed"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// openapiJSON is a byte-identical copy of python/openapi.json (CI diffs them).
//
//go:embed openapi.json
var openapiJSON []byte

// The /docs, /docs/oauth2-redirect and /redoc pages as FastAPI renders them.
var (
	//go:embed swagger.html
	swaggerHTML string
	//go:embed oauth2-redirect.html
	oauth2RedirectHTML string
	//go:embed redoc.html
	redocHTML string
)

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openapiJSON)
}

func serveHTML(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	}
}

// httpError is fastapi.HTTPException: {"detail": ...}.
type httpError struct {
	status int
	detail string
}

func (e *httpError) Error() string { return e.detail }

// internalError is an exception no handler catches: Starlette answers 500.
type internalError struct{ msg string }

func (e *internalError) Error() string { return e.msg }

// statusOf is the AppError status map of app_error_handler.
var statusOf = map[apperr.Code]int{
	apperr.SessionNotFound:     404,
	apperr.PageNotFound:        404,
	apperr.BrowserStartFailed:  502,
	apperr.Timeout:             504,
	apperr.ConfigErr:           400,
	apperr.InvalidArgument:     400,
	apperr.ScriptValidationErr: 400,
	apperr.NotImplemented:      501,
}

var timeoutRe = regexp.MustCompile(`Timeout \d+ms exceeded`)

// request is what a handler gets: path parameters, validated body and query.
type request struct {
	r      *http.Request
	params map[string]string
	body   *omap.Map
	query  *omap.Map
}

type handler func(*request) (any, error)

type route struct {
	method string
	parts  []string // "{name}" is a parameter
	body   []field  // nil: the route takes no body
	query  []field
	fn     handler
	raw    http.HandlerFunc // routes that do not answer JSON
}

// Server is the API app; Close stops every browser session (lifespan end).
type Server struct {
	settings *config.Settings
	version  string
	manager  *browser.Manager
	routes   []route
}

// New is create_app.
func New(settings *config.Settings, version string) *Server {
	s := &Server{settings: settings, version: version, manager: browser.NewManager(settings)}
	s.routes = s.table()
	return s
}

// Close stops all sessions.
func (s *Server) Close() { s.manager.StopAll() }

func (s *Server) add(method, path string, body, query []field, fn handler) route {
	return route{method: method, parts: strings.Split(path, "/"), body: body, query: query, fn: fn}
}

func (rt *route) match(path string) map[string]string {
	parts := strings.Split(path, "/")
	if len(parts) != len(rt.parts) {
		return nil
	}
	params := map[string]string{}
	for i, p := range rt.parts {
		if strings.HasPrefix(p, "{") {
			if parts[i] == "" {
				return nil
			}
			params[p[1:len(p)-1]] = parts[i]
		} else if p != parts[i] {
			return nil
		}
	}
	return params
}

// allows reports whether a route of method serves the request method;
// Starlette adds HEAD to GET routes.
func (rt *route) allows(method string) bool {
	return rt.method == method || (rt.method == "GET" && method == "HEAD")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body := omap.Marshal(v, -1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeAppError(w http.ResponseWriter, status int, code apperr.Code, message string) {
	writeJSON(w, status, omap.New("success", false, "error", omap.New("code", string(code), "message", message)))
}

// writeError answers like the exception handlers of the app.
func writeError(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	var he *httpError
	var ve validationError
	var ie *internalError
	switch {
	case errors.As(err, &ae):
		status, ok := statusOf[ae.Code]
		if !ok {
			status = 500
		}
		writeAppError(w, status, ae.Code, ae.Message)
	case errors.As(err, &he):
		writeJSON(w, he.status, omap.New("detail", he.detail))
	case errors.As(err, &ve):
		writeJSON(w, 422, omap.New("detail", []any(ve)))
	case errors.As(err, &ie):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "Internal Server Error")
	default:
		// browser errors (PlaywrightError): timeouts 504, the rest 422
		msg := strings.TrimPrefix(err.Error(), ": ")
		if timeoutRe.MatchString(msg) {
			writeAppError(w, 504, apperr.Timeout, msg)
		} else {
			writeAppError(w, 422, apperr.ActionFailed, msg)
		}
	}
}

// authExempt are the paths TokenAuthMiddleware lets through.
var authExempt = map[string]bool{"/health": true, "/ready": true, "/docs": true, "/openapi.json": true}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tok := s.settings.API.Token; tok != nil && *tok != "" && !authExempt[r.URL.Path] {
		got := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(got), []byte("Bearer "+*tok)) != 1 {
			writeAppError(w, 401, "UNAUTHORIZED", "Invalid or missing token")
			return
		}
	}
	path := r.URL.Path
	var partial *route
	for i := range s.routes {
		rt := &s.routes[i]
		params := rt.match(path)
		if params == nil {
			continue
		}
		if rt.allows(r.Method) {
			s.handle(w, r, rt, params)
			return
		}
		if partial == nil {
			partial = rt
		}
	}
	if partial != nil {
		allow := partial.method
		if allow == "GET" {
			allow = "GET, HEAD"
		}
		w.Header().Set("Allow", allow)
		writeJSON(w, 405, omap.New("detail", "Method Not Allowed"))
		return
	}
	// redirect_slashes: the other form of the path matches a route
	if path != "/" {
		other := path + "/"
		if strings.HasSuffix(path, "/") {
			other = strings.TrimRight(path, "/")
		}
		for i := range s.routes {
			if s.routes[i].match(other) != nil {
				u := *r.URL
				u.Path, u.RawPath = other, ""
				u.Scheme, u.Host = "http", r.Host
				if r.TLS != nil {
					u.Scheme = "https"
				}
				w.Header().Set("Location", u.String())
				w.WriteHeader(307)
				return
			}
		}
	}
	writeJSON(w, 404, omap.New("detail", "Not Found"))
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request, rt *route, params map[string]string) {
	if rt.raw != nil {
		rt.raw(w, r)
		return
	}
	req := &request{r: r, params: params, query: omap.New()}
	var errs []any
	q := r.URL.Query()
	for _, f := range rt.query {
		_, present := q[f.name]
		v, fe := queryParam(q.Get(f.name), present, f)
		errs = append(errs, fe...)
		req.query.Set(f.name, v)
	}
	if rt.body != nil {
		body, fe := readBody(r, rt.body)
		errs = append(errs, fe...)
		req.body = body
	}
	if len(errs) > 0 {
		writeError(w, validationError(errs))
		return
	}
	out, err := rt.fn(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

// readBody parses and validates the JSON body like FastAPI: an empty body is
// "missing", a body that is not JSON is validated as a string.
func readBody(r *http.Request, fields []field) (*omap.Map, []any) {
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 {
		return nil, []any{verr("missing", []any{"body"}, "Field required", nil)}
	}
	var v any = string(data)
	if isJSON(r.Header.Get("Content-Type")) {
		// invalid JSON is validated as the raw string, like FastAPI does here
		if parsed, err := omap.ParseJSON(data); err == nil {
			v = parsed
		}
	}
	return validateModel(v, fields, []any{"body"})
}

func isJSON(contentType string) bool {
	if contentType == "" {
		return true
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	sub, ok := strings.CutPrefix(mt, "application/")
	return ok && (sub == "json" || strings.HasSuffix(sub, "+json"))
}
