package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/foxzi/autobrowser-client/go/internal/cloak"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func connect(t *testing.T, settings *config.Settings) *sdk.ClientSession {
	t.Helper()
	ct, st := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = New(settings, "").Run(ctx, st)
		close(done)
	}()
	c, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(); cancel(); <-done })
	return c
}

func defaultSettings(t *testing.T, env map[string]string) *config.Settings {
	t.Helper()
	st, err := config.LoadSettings("", nil, env)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// call returns the result text and isError.
func call(t *testing.T, c *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := c.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	switch ct := res.Content[0].(type) {
	case *sdk.TextContent:
		return ct.Text, res.IsError
	case *sdk.ImageContent:
		return ct.MIMEType + ":" + string(ct.Data[:4]), res.IsError
	}
	t.Fatalf("%s: unexpected content %T", name, res.Content[0])
	return "", false
}

func TestListTools(t *testing.T) {
	c := connect(t, defaultSettings(t, nil))
	if got := c.InitializeResult(); got.Instructions != instructions || got.ServerInfo.Name != "autobrowser" {
		t.Fatalf("%+v", got)
	}
	res, err := c.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var want []map[string]any
	if err := json.Unmarshal(toolsJSON, &want); err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(want) || len(want) != 19 {
		t.Fatalf("%d tools, want %d", len(res.Tools), len(want))
	}
	byName := map[string]*sdk.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	for _, w := range want {
		got, _ := json.Marshal(byName[w["name"].(string)])
		exp, _ := json.Marshal(w)
		if !bytes.Equal(got, exp) {
			t.Errorf("tool differs:\n%s\n%s", got, exp)
		}
	}
}

func TestErrors(t *testing.T) {
	c := connect(t, defaultSettings(t, nil))
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"browser_tabs", map[string]any{"action": "bogus"}, "Error executing tool browser_tabs: 1 validation error for browser_tabsArguments\n" +
			"action\n  Input should be 'list', 'new' or 'close' [type=literal_error, input_value='bogus', input_type=str]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/literal_error"},
		{"browser_click", map[string]any{"target": 5, "double": "x"}, "Error executing tool browser_click: 2 validation errors for browser_clickArguments\n" +
			"target\n  Input should be a valid string [type=string_type, input_value=5, input_type=int]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/string_type\n" +
			"double\n  Input should be a valid boolean, unable to interpret input [type=bool_parsing, input_value='x', input_type=str]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/bool_parsing"},
		{"browser_wait_for", map[string]any{"zzz": strings.Repeat("q", 80)}, "Error executing tool browser_wait_for: 1 validation error for browser_wait_forArguments\n" +
			"text\n  Field required [type=missing, input_value={'zzz': 'qqqqqqqqqqqqqqqq...qqqqqqqqqqqqqqqqqqqqqq'}, input_type=dict]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/missing"},
		{"browser_select_option", map[string]any{"target": "a", "values": []any{"a", 3}}, "Error executing tool browser_select_option: 1 validation error for browser_select_optionArguments\n" +
			"values.1\n  Input should be a valid string [type=string_type, input_value=3, input_type=int]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/string_type"},
		{"browser_run_script", map[string]any{"vars": map[string]any{"a": 1}, "script": "x"}, "Error executing tool browser_run_script: 1 validation error for browser_run_scriptArguments\n" +
			"vars.a\n  Input should be a valid string [type=string_type, input_value=1, input_type=int]\n" +
			"    For further information visit https://errors.pydantic.dev/2.14/v/string_type"},
		{"browser_run_script", map[string]any{}, "Error executing tool browser_run_script: Pass exactly one of script or file"},
		{"browser_run_script", map[string]any{"file": "/nonexistent.yaml"}, "Error executing tool browser_run_script: SCRIPT_VALIDATION_ERROR: Script file does not exist: /nonexistent.yaml"},
		{"nonexistent", map[string]any{}, "Unknown tool: nonexistent"},
	}
	for _, tc := range cases {
		got, isErr := call(t, c, tc.name, tc.args)
		if !isErr || got != tc.want {
			t.Errorf("%s %v:\n got %q\nwant %q", tc.name, tc.args, got, tc.want)
		}
	}
	if got, isErr := call(t, c, "browser_close", nil); isErr || got != "closed" {
		t.Fatal(got)
	}
}

func TestValidateCoercion(t *testing.T) {
	var schema *omap.Map
	var defs []json.RawMessage
	_ = json.Unmarshal(toolsJSON, &defs)
	for _, d := range defs {
		v, _ := omap.ParseJSON(d)
		if name, _ := v.(*omap.Map).Get("name"); name == "browser_wait_for" {
			s, _ := v.(*omap.Map).Get("inputSchema")
			schema = s.(*omap.Map)
		}
	}
	for _, tc := range []struct {
		gone, timeout any
		ok            bool
	}{
		{1, 5.0, true}, {"yes", "5", true}, {"TRUE", " 7 ", true}, {0.0, "1_000", true}, {false, true, true},
		{2, 5, false}, {1.5, 5, false}, {"No ", 5, false}, {nil, 5, false},
		{true, 5.5, false}, {true, "5.5", false}, {true, "x", false}, {true, nil, false},
	} {
		args := omap.New("text", "a", "gone", tc.gone, "timeout_ms", tc.timeout)
		err := validateArgs("browser_wait_for", schema, args)
		if (err == nil) != tc.ok {
			t.Errorf("%v %v: %v", tc.gone, tc.timeout, err)
		}
		if err == nil {
			if _, ok := args.Get("gone"); !ok {
				t.Error("no gone")
			}
			g, _ := args.Get("gone")
			n, _ := args.Get("timeout_ms")
			if _, ok := g.(bool); !ok {
				t.Errorf("gone %v not coerced", g)
			}
			if _, ok := n.(int); !ok {
				t.Errorf("timeout %v not coerced", n)
			}
		}
	}
	args := omap.New("text", "a")
	if err := validateArgs("browser_wait_for", schema, args); err != nil ||
		omap.Marshal(args, 0) != `{"text": "a", "gone": false, "timeout_ms": 10000, "page_id": null}` {
		t.Fatal(err, omap.Marshal(args, 0))
	}
}

func TestDumpsASCII(t *testing.T) {
	if got := dumpsASCII(omap.New("t", "é😀\"")); got != `{"t": "\u00e9\ud83d\ude00\""}` {
		t.Fatal(got)
	}
}

const testPage = `<!doctype html><title>T</title>
<input id="name"><button id="go" onclick="document.getElementById('out').textContent='hi '+document.getElementById('name').value">Go</button>
<div id="out"></div>`

func TestBrowserTools(t *testing.T) {
	if os.Getenv(cloak.BinaryEnv) == "" {
		if m, _ := filepath.Glob(filepath.Join(cloak.CacheDir(), "chromium-*")); len(m) == 0 {
			t.Skip("cloakbrowser binary not available")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testPage))
	}))
	defer srv.Close()
	st := defaultSettings(t, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	st.Browser.Headless = true
	c := connect(t, st)

	expect := func(name string, args map[string]any, want string) string {
		t.Helper()
		got, isErr := call(t, c, name, args)
		if isErr || (want != "" && got != want) {
			t.Fatalf("%s: %q (isError %v), want %q", name, got, isErr, want)
		}
		return got
	}
	info := `{"pageId": "page_0", "url": "` + srv.URL + `/", "title": "T"}`
	expect("browser_navigate", map[string]any{"url": srv.URL + "/"},
		`{"url": "`+srv.URL+`/", "title": "T", "status": 200}`)
	snap := expect("browser_snapshot", nil, "")
	if !strings.HasPrefix(snap, "URL: "+srv.URL+"/\nTitle: T\n") || !strings.Contains(snap, `button "Go" [ref=e`) {
		t.Fatal(snap)
	}
	expect("browser_type", map[string]any{"target": "#name", "by": "css", "text": "bob"}, info)
	ref := snap[strings.Index(snap, `button "Go" [ref=`)+len(`button "Go" [ref=`):]
	ref = ref[:strings.IndexByte(ref, ']')]
	expect("browser_click", map[string]any{"target": ref}, info)
	expect("browser_wait_for", map[string]any{"text": "hi bob"}, info)
	expect("browser_evaluate", map[string]any{"expression": "() => document.getElementById('out').textContent"}, `"hi bob"`)
	if got := expect("browser_get_text", nil, ""); !strings.Contains(got, "hi bob") {
		t.Fatal(got)
	}
	expect("browser_screenshot", nil, "image/png:\x89PNG")
	expect("browser_handle_dialogs", map[string]any{"accept": true}, `{"accept": true, "promptText": null}`)
	expect("browser_tabs", map[string]any{"action": "new"}, `{"page_0": "`+srv.URL+`/", "page_1": "about:blank"}`)
	expect("browser_tabs", map[string]any{"action": "close", "page_id": "page_1"}, `{"page_0": "`+srv.URL+`/"}`)
	if got, isErr := call(t, c, "browser_tabs", map[string]any{"action": "close"}); !isErr ||
		got != "Error executing tool browser_tabs: page_id is required to close a tab" {
		t.Fatal(got)
	}
	if got, isErr := call(t, c, "browser_click", map[string]any{"target": "x", "page_id": "nope"}); !isErr ||
		got != "Error executing tool browser_click: PAGE_NOT_FOUND: Page not found: nope" {
		t.Fatal(got)
	}
	expect("browser_logs", map[string]any{"kind": "dialogs"}, "[]")
	got := expect("browser_run_script", map[string]any{"script": "steps:\n  - action: evaluate\n    script: \"1 + ${n}\"\n", "vars": map[string]any{"n": "2"}}, "")
	if !strings.HasPrefix(got, `{"success": true, "steps": [{"action": "evaluate", "success": true`) ||
		!strings.HasSuffix(got, `"vars": {"n": "2"}}`) {
		t.Fatal(got)
	}
	expect("browser_close", nil, "closed")
	// the next call starts a fresh browser
	expect("browser_tabs", nil, `{"page_0": "about:blank"}`)
}
