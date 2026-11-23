package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/cloak"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func TestRunFlags(t *testing.T) {
	o, err := parseRun(&env{args: []string{"--url", "https://a", "-u", "https://b", "-c", "c.yaml", "-p", "work",
		"--profile-dir", "profs", "--user-agent", "UA", "--headless", "--headful", "--timeout", "500",
		"--proxy", "http://h:1", "--viewport", "800x600", "-o", "out.json", "--format", "text"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(o.urls, []string{"https://a", "https://b"}) || o.config != "c.yaml" || o.profile != "work" ||
		o.profileDir != "profs" || o.userAgent != "UA" || o.headless == nil || *o.headless || *o.timeout != 500 ||
		o.proxy != "http://h:1" || o.viewport != "800x600" || o.out != "out.json" || o.format != "text" {
		t.Errorf("got %+v", *o)
	}
	ov, err := o.overrides()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"browser":  map[string]any{"headless": false, "userAgent": "UA", "viewport": map[string]any{"width": 800, "height": 600}},
		"profiles": map[string]any{"dir": "profs"},
		"timeouts": map[string]any{"actionMs": 500, "navigationMs": 500},
		"proxy":    map[string]any{"enabled": true, "scheme": "http", "host": "h", "port": 1},
	}
	if !reflect.DeepEqual(ov, want) {
		t.Errorf("overrides %v", ov)
	}

	o, err = parseRun(&env{args: []string{"-u", "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if o.format != "html" || o.headless != nil || o.timeout != nil {
		t.Errorf("defaults %+v", *o)
	}
	if ov, _ := o.overrides(); len(ov) != 0 {
		t.Errorf("default overrides %v", ov)
	}
}

func TestRunUsageErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	os.WriteFile(file, nil, 0o644)
	cases := map[string]struct {
		args []string
		msg  string
	}{
		"missing --url": {[]string{"run"}, "Missing option '--url' / '-u'."},
		"no url value":  {[]string{"run", "-u"}, "Option '-u' requires an argument."},
		"bad timeout":   {[]string{"run", "-u", "x", "--timeout", "abc"}, "Invalid value for '--timeout': 'abc' is not a valid int."},
		"dir is file":   {[]string{"run", "-u", "x", "--profile-dir", file}, "Invalid value for '--profile-dir': Directory '" + file + "' is a file."},
		"bad viewport":  {[]string{"run", "-u", "x", "--viewport", "12"}, "Invalid value: Invalid viewport format: '12'"},
		"unknown flag":  {[]string{"run", "-u", "x", "--bogus"}, "No such option: --bogus"},
		"extra arg":     {[]string{"run", "-u", "x", "extra"}, "Got unexpected extra argument(s) (extra)"},
	}
	for name, c := range cases {
		code, out, errText := run(c.args...)
		if code != 2 || out != "" || !strings.Contains(errText, "Error: "+c.msg) {
			t.Errorf("%s: %d %q", name, code, errText)
		}
	}
}

func TestRunUnsupportedEngine(t *testing.T) {
	t.Setenv("AUTOBROWSER_BROWSER_ENGINE", "patchright")
	code, out, errText := run("run", "-u", "about:blank")
	if code != 1 || out != "" || !strings.Contains(errText, `"CONFIG_ERROR"`) || !strings.Contains(errText, "cloakbrowser") {
		t.Errorf("%d %q %q", code, out, errText)
	}
}

func TestRunBrowser(t *testing.T) {
	if os.Getenv(cloak.BinaryEnv) == "" {
		if m, _ := filepath.Glob(filepath.Join(cloak.CacheDir(), "chromium-*")); len(m) == 0 {
			t.Skip("cloakbrowser binary not available")
		}
	}
	t.Setenv("AUTOBROWSER_BROWSER_ENGINE", "cloakbrowser")
	out := filepath.Join(t.TempDir(), "r.json")
	code, stdout, errText := run("run", "-u", "data:text/html,<title>T</title><h1>Hello</h1>", "--headless",
		"--profile-dir", t.TempDir(), "--format", "text", "-o", out)
	if code != 0 || stdout != "Result saved to "+out+"\n" {
		t.Fatalf("%d %q %q", code, stdout, errText)
	}
	data, _ := os.ReadFile(out)
	v, err := omap.ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(*omap.Map)
	if !reflect.DeepEqual(m.Keys(), []string{"success", "sessionId", "cookies", "pages"}) {
		t.Fatalf("keys %v", m.Keys())
	}
	pages, _ := m.Get("pages")
	ids := pages.(*omap.Map).Keys()
	if len(ids) != 1 {
		t.Fatalf("pages %s", data)
	}
	p, _ := pages.(*omap.Map).Get(ids[0])
	pm := p.(*omap.Map)
	content, _ := pm.Get("content")
	if !reflect.DeepEqual(pm.Keys(), []string{"url", "headers", "content"}) || content != "Hello" {
		t.Errorf("page %s", data)
	}
}
