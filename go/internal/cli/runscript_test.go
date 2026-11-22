package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadPlan(t *testing.T, yaml string, args ...string) (*runPlan, error) {
	t.Helper()
	path := writeScript(t, yaml)
	o, err := parseRunScript(&env{args: append([]string{"-f", path}, args...)})
	if err != nil {
		t.Fatal(err)
	}
	script, err := scenario.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return plan(o, script)
}

const base = "steps:\n  - open: https://example.com\n"

func TestRunScriptFlags(t *testing.T) {
	path := writeScript(t, base)
	o, err := parseRunScript(&env{args: []string{"--file", path, "-c", "c.yaml", "-p", "work", "--proxy", "http://h:1",
		"--viewport", "800x600", "-o", "out.json", "--preserve-session", "--var", "a=1", "--var", "b=x=y", "--artifacts", "art"}})
	if err != nil {
		t.Fatal(err)
	}
	want := runScriptOpts{file: path, config: "c.yaml", profile: "work", proxy: "http://h:1", viewport: "800x600",
		output: "out.json", artifacts: "art", preserve: true, vars: []string{"a=1", "b=x=y"}}
	if !reflect.DeepEqual(*o, want) {
		t.Errorf("got %+v", *o)
	}
}

func TestRunScriptUsageErrors(t *testing.T) {
	path := writeScript(t, base)
	cases := map[string][]string{
		"missing --file": {"run-script"},
		"no such file":   {"run-script", "-f", "/nonexistent.yaml"},
		"directory":      {"run-script", "-f", t.TempDir()},
		"extra arg":      {"run-script", "-f", path, "x"},
		"unknown flag":   {"run-script", "-f", path, "--bogus"},
		"bad var":        {"run-script", "-f", path, "--var", "novalue"},
		"empty var name": {"run-script", "-f", path, "--var", "=1"},
		"bad viewport":   {"run-script", "-f", path, "--viewport", "big"},
		"bad viewport 2": {"run-script", "-f", path, "--viewport", "1x2x3"},
	}
	for name, args := range cases {
		if code, _, _ := run(args...); code != 2 {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}

func TestRunScriptVarsAndViewport(t *testing.T) {
	p, err := loadPlan(t, base, "--var", "a=1", "--var", "a=2", "--var", "b=x=y")
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := p.vars.Get("a"); a != "2" {
		t.Errorf("a=%v", a)
	}
	if b, _ := p.vars.Get("b"); b != "x=y" {
		t.Errorf("b=%v", b)
	}
	vp, err := parseViewport("1920X1080")
	if err != nil || vp["width"] != 1920 || vp["height"] != 1080 {
		t.Errorf("viewport %v %v", vp, err)
	}
}

func TestRunScriptPrecedence(t *testing.T) {
	yaml := "session:\n  profile: sp\n  proxy: socks5://sh:1\n  viewport: {width: 100, height: 200}\n" +
		"  headless: false\n  userAgent: UA\n  preserveSession: true\n" + base
	// scenario only
	p, err := loadPlan(t, yaml)
	if err != nil {
		t.Fatal(err)
	}
	vp := p.overrides["browser"].(map[string]any)["viewport"].(map[string]any)
	if vp["width"] != 100 || vp["height"] != 200 {
		t.Errorf("viewport %v", vp)
	}
	if p.overrides["proxy"].(map[string]any)["host"] != "sh" || p.profile != "sp" || !p.preserve {
		t.Errorf("scenario values lost: %+v", p)
	}
	if *p.browserOpts.Headless || p.browserOpts.UserAgent != "UA" || p.browserOpts.Profile != "sp" {
		t.Errorf("browser options %+v", p.browserOpts)
	}
	// flags win
	p, err = loadPlan(t, yaml, "-p", "fp", "--proxy", "http://fh:2", "--viewport", "3x4")
	if err != nil {
		t.Fatal(err)
	}
	vp = p.overrides["browser"].(map[string]any)["viewport"].(map[string]any)
	px := p.overrides["proxy"].(map[string]any)
	if vp["width"] != 3 || px["host"] != "fh" || px["scheme"] != "http" || px["port"] != 2 || p.profile != "fp" {
		t.Errorf("flags lost: %v %v %s", vp, px, p.profile)
	}
	// defaults: nothing overridden, headless true
	p, err = loadPlan(t, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.overrides) != 0 || !*p.browserOpts.Headless || p.preserve || p.profile != "" {
		t.Errorf("defaults: %+v", p)
	}
	// --preserve-session turns it on without the scenario
	if p, _ = loadPlan(t, base, "--preserve-session"); !p.preserve {
		t.Error("preserve flag ignored")
	}
}

func TestRunScriptArtifactsAndOutputPaths(t *testing.T) {
	if _, err := loadPlan(t, base, "--artifacts", "/abs"); err == nil {
		t.Error("absolute artifacts path accepted")
	}
	if _, err := loadPlan(t, base, "--artifacts", "../up"); err == nil {
		t.Error("artifacts traversal accepted")
	}
	if _, err := loadPlan(t, base+"output:\n  file: ../out.json\n"); err == nil {
		t.Error("output.file traversal accepted")
	}
	p, err := loadPlan(t, "errorArtifacts: fromscript\n"+base)
	if err != nil || p.artifactsDir != "fromscript" {
		t.Errorf("%v %+v", err, p)
	}
	if p, _ = loadPlan(t, "errorArtifacts: fromscript\n"+base, "--artifacts", "cli"); p.artifactsDir != "cli" {
		t.Errorf("flag should win: %s", p.artifactsDir)
	}
}

func TestApplyProxyURL(t *testing.T) {
	got := map[string]any{}
	if err := applyProxyURL("socks5://us%40er:p:w@Host.Example:1080/x", got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"enabled": true, "scheme": "socks5", "host": "host.example", "port": 1080,
		"username": "us%40er", "password": "p:w"}
	if !reflect.DeepEqual(got["proxy"], want) {
		t.Errorf("got %v", got["proxy"])
	}
	got = map[string]any{}
	if err := applyProxyURL("//h", got); err != nil {
		t.Fatal(err)
	}
	if px := got["proxy"].(map[string]any); px["scheme"] != "socks5" || px["host"] != "h" || px["port"] != nil {
		t.Errorf("got %v", px)
	}
	if err := applyProxyURL("http://h:abc", map[string]any{}); err == nil {
		t.Error("bad port accepted")
	}
	// the overrides must be accepted by config
	s, err := config.LoadSettings("", map[string]any{"proxy": got["proxy"]}, map[string]string{"X": "y"})
	if err != nil || !s.Proxy.Enabled || *s.Proxy.Host != "h" {
		t.Errorf("%v %+v", err, s)
	}
}

func TestRunScriptUnsupportedEngine(t *testing.T) {
	t.Setenv("AUTOBROWSER_BROWSER_ENGINE", "patchright")
	code, out, errText := run("run-script", "-f", writeScript(t, base))
	if code != 1 || out != "" || !strings.Contains(errText, `"CONFIG_ERROR"`) || !strings.Contains(errText, "cloakbrowser") {
		t.Errorf("%d %q %q", code, out, errText)
	}
}

func TestRunScriptInvalidScenario(t *testing.T) {
	code, _, errText := run("run-script", "-f", writeScript(t, "steps: []\nbogus: 1\n"))
	if code != 1 || !strings.Contains(errText, "SCRIPT_VALIDATION_ERROR") {
		t.Errorf("%d %q", code, errText)
	}
}
