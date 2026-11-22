package api

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/cloak"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func newServer(t *testing.T, overrides map[string]any) (*Server, *config.Settings) {
	t.Helper()
	if overrides == nil {
		overrides = map[string]any{}
	}
	overrides["profiles"] = map[string]any{"dir": filepath.Join(t.TempDir(), "profiles")}
	st, err := config.LoadSettings("", overrides, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, "1.2.3")
	t.Cleanup(s.Close)
	return s, st
}

// do sends a request and returns the status and the body.
func do(t *testing.T, s *Server, method, path, body string, header ...string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func expect(t *testing.T, s *Server, method, path, body string, status int, want string) {
	t.Helper()
	code, got := do(t, s, method, path, body)
	if code != status || got != want {
		t.Errorf("%s %s %s: got %d %s, want %d %s", method, path, body, code, got, status, want)
	}
}

func TestServiceRoutes(t *testing.T) {
	s, _ := newServer(t, nil)
	expect(t, s, "GET", "/health", "", 200, `{"status":"ok"}`)
	expect(t, s, "GET", "/ready", "", 200, `{"status":"ready"}`)
	expect(t, s, "GET", "/api/v1/info", "", 200,
		`{"name":"autobrowser-client","version":"1.2.3","api":{"host":"127.0.0.1","port":8080}}`)
	expect(t, s, "GET", "/api/v1/sessions", "", 200, `{"success":true,"sessions":[]}`)
	expect(t, s, "GET", "/api/v1/session/nope", "", 404,
		`{"success":false,"error":{"code":"SESSION_NOT_FOUND","message":"Session not found: nope"}}`)
	expect(t, s, "POST", "/api/v1/session/nope/stop", "", 404,
		`{"success":false,"error":{"code":"SESSION_NOT_FOUND","message":"Session not found: nope"}}`)
	expect(t, s, "POST", "/api/v1/session/nope/script", `{"script":{"steps":[]}}`, 404,
		`{"success":false,"error":{"code":"SESSION_NOT_FOUND","message":"Session not found: nope"}}`)
}

func TestProfiles(t *testing.T) {
	s, st := newServer(t, nil)
	expect(t, s, "GET", "/api/v1/profiles", "", 200, `{"success":true,"profiles":[]}`)
	dir := filepath.Join(st.Profiles.Dir, "test_prof")
	expect(t, s, "POST", "/api/v1/profile/create", `{"name":" test_prof "}`, 200,
		`{"success":true,"profile":"test_prof","path":"`+dir+`"}`)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("profile dir: %v", err)
	}
	expect(t, s, "POST", "/api/v1/profile/create", `{"name":"test_prof"}`, 409,
		`{"detail":"Profile 'test_prof' already exists"}`)
	for _, name := range []string{"../escape", ".hidden", ""} {
		expect(t, s, "POST", "/api/v1/profile/create", `{"name":"`+name+`"}`, 400, `{"detail":"Invalid profile name"}`)
	}
	expect(t, s, "POST", "/api/v1/profile/create", `{"name":"alpha"}`, 200,
		`{"success":true,"profile":"alpha","path":"`+filepath.Join(st.Profiles.Dir, "alpha")+`"}`)
	expect(t, s, "GET", "/api/v1/profiles", "", 200, `{"success":true,"profiles":["alpha","test_prof"]}`)
}

func TestFastAPIErrors(t *testing.T) {
	s, _ := newServer(t, nil)
	expect(t, s, "GET", "/nope", "", 404, `{"detail":"Not Found"}`)
	expect(t, s, "POST", "/health", "", 405, `{"detail":"Method Not Allowed"}`)
	expect(t, s, "POST", "/api/v1/profile/create", "", 422,
		`{"detail":[{"type":"missing","loc":["body"],"msg":"Field required","input":null}]}`)
	expect(t, s, "POST", "/api/v1/profile/create", "{bad", 422,
		`{"detail":[{"type":"model_attributes_type","loc":["body"],"msg":"Input should be a valid dictionary or object to extract fields from","input":"{bad"}]}`)
	expect(t, s, "POST", "/api/v1/profile/create", `{"name":5}`, 422,
		`{"detail":[{"type":"string_type","loc":["body","name"],"msg":"Input should be a valid string","input":5}]}`)
	expect(t, s, "POST", "/api/v1/session/x/pages/p/history", `{"direction":"up"}`, 422,
		`{"detail":[{"type":"string_pattern_mismatch","loc":["body","direction"],"msg":"String should match pattern '^(back|forward|reload)$'","input":"up","ctx":{"pattern":"^(back|forward|reload)$"}}]}`)
	expect(t, s, "POST", "/api/v1/session/x/pages/p/elements/click", `{"locator":{},"timeout":"5x"}`, 422,
		`{"detail":[{"type":"missing","loc":["body","locator","value"],"msg":"Field required","input":{}},`+
			`{"type":"int_parsing","loc":["body","timeout"],"msg":"Input should be a valid integer, unable to parse string as an integer","input":"5x"}]}`)
	expect(t, s, "GET", "/api/v1/session/x/pages/p/snapshot?timeout=a", "", 422,
		`{"detail":[{"type":"int_parsing","loc":["query","timeout"],"msg":"Input should be a valid integer, unable to parse string as an integer","input":"a"}]}`)
	expect(t, s, "GET", "/api/v1/session/x/pages/p/logs/bogus", "", 400,
		`{"success":false,"error":{"code":"INVALID_ARGUMENT","message":"kind must be console, network or dialogs"}}`)
	expect(t, s, "POST", "/api/v1/session/x/pages/p/elements/upload", `{"locator":{"value":"#f"},"files":["/nope"]}`, 400,
		`{"success":false,"error":{"code":"INVALID_ARGUMENT","message":"Files not found: /nope"}}`)
}

func TestDocs(t *testing.T) {
	s, _ := newServer(t, nil)
	code, body := do(t, s, "GET", "/openapi.json", "")
	want, _ := os.ReadFile("openapi.json")
	if code != 200 || body != string(want) {
		t.Fatalf("openapi.json: %d", code)
	}
	v, err := omap.ParseJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := v.(*omap.Map).Get("paths")
	if _, ok := paths.(*omap.Map).Get("/health"); !ok {
		t.Fatal("no /health in openapi paths")
	}
	for _, p := range []string{"/docs", "/docs/oauth2-redirect", "/redoc"} {
		if code, body := do(t, s, "GET", p, ""); code != 200 || !strings.Contains(body, "<html") {
			t.Errorf("%s: %d", p, code)
		}
	}
}

func TestTokenAuth(t *testing.T) {
	s, _ := newServer(t, map[string]any{"api": map[string]any{"token": "test-secret"}})
	unauthorized := `{"success":false,"error":{"code":"UNAUTHORIZED","message":"Invalid or missing token"}}`
	for _, p := range []string{"/health", "/ready", "/docs", "/openapi.json"} {
		if code, _ := do(t, s, "GET", p, ""); code != 200 {
			t.Errorf("%s without token: %d", p, code)
		}
	}
	expect(t, s, "GET", "/api/v1/sessions", "", 401, unauthorized)
	expect(t, s, "GET", "/redoc", "", 401, unauthorized)
	if code, _ := do(t, s, "GET", "/api/v1/sessions", "", "Authorization", "Bearer test-secret"); code != 200 {
		t.Errorf("right token: %d", code)
	}
	if code, _ := do(t, s, "GET", "/api/v1/sessions", "", "Authorization", "Bearer wrong"); code != 401 {
		t.Errorf("wrong token: %d", code)
	}
}

// --- with a browser ---

const scriptPage = "data:text/html,<title>T</title><h1>Hello</h1>"

func browserSession(t *testing.T, s *Server) string {
	t.Helper()
	if os.Getenv(cloak.BinaryEnv) == "" {
		if m, _ := filepath.Glob(filepath.Join(cloak.CacheDir(), "chromium-*")); len(m) == 0 {
			t.Skip("cloakbrowser binary not available")
		}
	}
	code, body := do(t, s, "POST", "/api/v1/browser/start", `{"urls":["`+scriptPage+`"],"headless":true}`)
	if code != 200 {
		t.Fatalf("start: %d %s", code, body)
	}
	v, _ := omap.ParseJSON([]byte(body))
	sid, _ := v.(*omap.Map).Get("sessionId")
	return sid.(string)
}

func jsonGet(t *testing.T, body string, path ...any) any {
	t.Helper()
	v, err := omap.ParseJSON([]byte(body))
	if err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	for _, k := range path {
		switch k := k.(type) {
		case string:
			v, _ = v.(*omap.Map).Get(k)
		case int:
			v = v.([]any)[k]
		}
	}
	return v
}

func TestRunScript(t *testing.T) {
	s, _ := newServer(t, nil)
	sid := browserSession(t, s)
	url := "/api/v1/session/" + sid + "/script"
	code, body := do(t, s, "POST", url, `{"script":{"steps":[{"extract":"h1","as":"heading"},{"set":{"n":"${k}"}}]},"vars":{"k":5}}`)
	if code != 200 || jsonGet(t, body, "success") != true || jsonGet(t, body, "sessionId") != sid ||
		omap.Marshal(jsonGet(t, body, "vars"), -1) != `{"k":5,"heading":"Hello","n":5}` ||
		jsonGet(t, body, "steps", 1, "action") != "set" || jsonGet(t, body, "durationMs") == nil {
		t.Fatalf("script: %d %s", code, body)
	}
	if _, body := do(t, s, "POST", url, `{"script":"steps:\n  - assert: {text: Hello}\n"}`); jsonGet(t, body, "success") != true {
		t.Fatalf("yaml script: %s", body)
	}
	code, body = do(t, s, "POST", url, `{"script":{"steps":[{"fail":"boom"}]}}`)
	if code != 200 || jsonGet(t, body, "success") != false || jsonGet(t, body, "steps", 0, "error") != "boom" {
		t.Fatalf("failing script: %d %s", code, body)
	}
	for _, script := range []string{`{"steps":[{"bogus":1}]}`, `"steps: ["`, `"- not a mapping"`} {
		code, body := do(t, s, "POST", url, `{"script":`+script+`}`)
		if code != 400 || jsonGet(t, body, "error", "code") != "SCRIPT_VALIDATION_ERROR" {
			t.Errorf("invalid script %s: %d %s", script, code, body)
		}
	}
}

const chainPage = "data:text/html,<input id=q value=old><select id=s><option value=a>A</option><option value=b>B</option></select>" +
	`<button class=b onclick=\"document.title='clicked'\">1</button><button class=b>2</button>`

func TestActionChain(t *testing.T) {
	s, _ := newServer(t, nil)
	sid := browserSession(t, s)
	_, body := do(t, s, "GET", "/api/v1/session/"+sid+"/pages", "")
	pageID := jsonGet(t, body, "pages").(*omap.Map).Keys()[0]
	base := "/api/v1/session/" + sid + "/pages/" + pageID
	if code, body := do(t, s, "POST", base+"/navigate", `{"url":"`+chainPage+`"}`); code != 200 {
		t.Fatalf("navigate: %s", body)
	}
	steps := `[{"action":"find","locator":{"type":"css","value":"#q"}},
		{"action":"fill","locator":{"type":"css","value":"#q"},"value":"${not_a_var}","clearBefore":true},
		{"action":"select","locator":{"type":"css","value":"#s"},"option":"b"},
		{"action":"click","locator":{"type":"css","value":".b"}},
		{"action":"wait","locator":{"type":"css","value":"#q"},"state":"attached"},
		{"action":"wait","locator":{"type":"css","value":"#q"},"state":"visible"},
		{"action":"wait"},
		{"action":"mouse-move-random","endX":10,"endY":10,"steps":3,"durationMs":10},
		{"action":"bogus","stopOnError":false},
		{"action":"click","locator":{"type":"css","value":"#missing"},"timeout":300},
		{"action":"press","key":"Tab"}]`
	code, body := do(t, s, "POST", base+"/actions/run", `{"steps":`+steps+`}`)
	if code != 200 || jsonGet(t, body, "success") != false {
		t.Fatalf("chain: %d %s", code, body)
	}
	results := jsonGet(t, body, "results").([]any)
	var actions, oks []string
	for _, r := range results {
		a, _ := r.(*omap.Map).Get("action")
		ok, _ := r.(*omap.Map).Get("success")
		actions = append(actions, a.(string))
		oks = append(oks, omap.Marshal(ok, -1))
	}
	if strings.Join(actions, ",") != "find,fill,select,click,wait,wait,wait,mouse-move-random,bogus,click" ||
		strings.Join(oks, ",") != "true,true,true,true,true,true,true,true,false,false" {
		t.Fatalf("chain results: %s", body)
	}
	if omap.Marshal(jsonGet(t, body, "results", 0, "data", "found"), -1) != "1" ||
		omap.Marshal(jsonGet(t, body, "results", 2, "data"), -1) != `{"selected":["b"]}` ||
		omap.Marshal(jsonGet(t, body, "results", 7, "data"), -1) != `{"points":4}` ||
		jsonGet(t, body, "results", 8, "error") != "Unknown action: bogus" ||
		jsonGet(t, body, "results", 0, "durationMs") == nil {
		t.Fatalf("chain data: %s", body)
	}
	_, body = do(t, s, "POST", base+"/evaluate",
		`{"expression":"() => [document.title, document.querySelector('#q').value, document.querySelector('#s').value]"}`)
	if got := omap.Marshal(jsonGet(t, body, "result", "result"), -1); got != `["clicked","${not_a_var}","b"]` {
		t.Fatalf("page state: %s", body)
	}
}
