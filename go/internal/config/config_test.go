package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

func load(t *testing.T, path string, overrides map[string]any, env map[string]string) *Settings {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	s, err := LoadSettings(path, overrides, env)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	s := load(t, "", nil, nil)
	if !s.Browser.Headless || s.Browser.Engine != "patchright" || s.API.Port != 8080 || s.API.Host != "127.0.0.1" ||
		s.Timeouts.ActionMs != 10000 || s.Timeouts.NavigationMs != 30000 || s.Proxy.Enabled ||
		s.Output.MaxContentBytes != 1048576 || s.Browser.Viewport.Width != 1280 || s.Browser.Viewport.Height != 720 ||
		s.Profiles.Dir != ".profiles" || s.Timeouts.IdleSessionMs != 300000 || s.API.MaxSessions != 10 ||
		s.Logging.Level != "INFO" || !s.Logging.MaskSecrets || s.Output.ContentFormat != "html" || s.Proxy.Scheme != "socks5" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}

func TestEnvOverride(t *testing.T) {
	s := load(t, "", nil, map[string]string{
		"AUTOBROWSER_API_PORT":  "9090",
		"AUTOBROWSER_HEADLESS":  "false",
		"AUTOBROWSER_LOG_LEVEL": "DEBUG",
		"AUTOBROWSER_API_TOKEN": "secret123",
	})
	if s.API.Port != 9090 || s.Browser.Headless || s.Logging.Level != "DEBUG" || *s.API.Token != "secret123" {
		t.Fatalf("%+v", s)
	}
}

func TestEnvBoolParsing(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "TRUE": true, " yes ": true, "on": true, "0": false, "no": false, "anything": false} {
		s := load(t, "", nil, map[string]string{"AUTOBROWSER_GEOIP": v})
		if s.Browser.Geoip != want {
			t.Errorf("%q: got %v", v, s.Browser.Geoip)
		}
	}
}

func TestEnvInvalidInt(t *testing.T) {
	if _, err := LoadSettings("", nil, map[string]string{"AUTOBROWSER_API_PORT": "abc"}); err == nil {
		t.Fatal("expected error")
	}
	s := load(t, "", nil, map[string]string{"AUTOBROWSER_API_PORT": " 1_000 "})
	if s.API.Port != 1000 {
		t.Fatal(s.API.Port)
	}
}

func TestConfigFileYAML(t *testing.T) {
	p := writeFile(t, "c.yaml", "browser:\n  headless: false\napi:\n  port: 7777\n")
	s := load(t, p, nil, nil)
	if s.Browser.Headless || s.API.Port != 7777 {
		t.Fatalf("%+v", s)
	}
}

func TestConfigFileEmptyYAML(t *testing.T) {
	load(t, writeFile(t, "c.yml", ""), nil, nil)
}

func TestConfigFileJSON(t *testing.T) {
	p := writeFile(t, "c.json", `{"api": {"host": "0.0.0.0", "maxSessions": 5}}`)
	s := load(t, p, nil, nil)
	if s.API.Host != "0.0.0.0" || s.API.MaxSessions != 5 {
		t.Fatalf("%+v", s)
	}
}

func TestConfigFileErrors(t *testing.T) {
	_, err := LoadSettings("/nonexistent/config.yaml", nil, map[string]string{})
	if e, ok := err.(*apperr.Error); !ok || e.Code != apperr.ConfigErr || !strings.Contains(e.Message, "does not exist") {
		t.Fatal(err)
	}
	_, err = LoadSettings(writeFile(t, "c.toml", "foo = 1"), nil, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "Unsupported") {
		t.Fatal(err)
	}
	_, err = LoadSettings(writeFile(t, "c.yaml", "- a\n- b\n"), nil, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "object at the root") {
		t.Fatal(err)
	}
}

func TestPrecedence(t *testing.T) {
	p := writeFile(t, "c.yaml", "api: {port: 1111, host: file}\n")
	s := load(t, p, map[string]any{"api": map[string]any{"port": 1234}}, map[string]string{"AUTOBROWSER_API_PORT": "9090"})
	if s.API.Port != 1234 || s.API.Host != "file" {
		t.Fatalf("%+v", s.API)
	}
	s = load(t, p, nil, map[string]string{"AUTOBROWSER_API_PORT": "9090"})
	if s.API.Port != 9090 {
		t.Fatal(s.API.Port)
	}
}

func TestDeepMerge(t *testing.T) {
	got := deepMerge(
		map[string]any{"a": map[string]any{"b": 1, "c": 2}, "d": 3, "l": []any{1, 2}},
		map[string]any{"a": map[string]any{"b": 10, "e": 5}, "l": []any{9}})
	want := map[string]any{"a": map[string]any{"b": 10, "c": 2, "e": 5}, "d": 3, "l": []any{9}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func str(s string) *string { return &s }
func num(i int) *int       { return &i }

func TestProxyURL(t *testing.T) {
	cases := []struct {
		p    ProxySettings
		want string
	}{
		{ProxySettings{Scheme: "socks5", Host: str("h"), Port: num(1080), Username: str("user"), Password: str("pass")}, "socks5://user:pass@h:1080"},
		{ProxySettings{Scheme: "http", Host: str("h"), Port: num(8080)}, "http://h:8080"},
		{ProxySettings{Scheme: "socks5", Host: str("h")}, "socks5://h:1080"},
		{ProxySettings{Scheme: "http", Host: str("h"), Username: str("u")}, "http://u@h:8080"},
		{ProxySettings{Scheme: "socks5"}, ""},
	}
	for _, c := range cases {
		if got := c.p.ProxyURL(); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestInvalidConfigs(t *testing.T) {
	bad := []map[string]any{
		{"browser": map[string]any{"unknown_field": true}},
		{"unknown": map[string]any{}},
		{"browser": map[string]any{"engine": "firefox"}},
		{"browser": map[string]any{"viewport": map[string]any{"width": 0}}},
		{"browser": map[string]any{"launch_args": []any{}}},
		{"api": map[string]any{"port": 70000}},
		{"api": map[string]any{"maxSessions": 0}},
		{"timeouts": map[string]any{"actionMs": -1}},
		{"proxy": map[string]any{"port": 0}},
		{"logging": map[string]any{"level": "info"}},
		{"output": map[string]any{"contentFormat": "pdf"}},
		{"browser": "x"},
		{"profiles": map[string]any{"dir": 5}},
		{"browser": map[string]any{"launchArgs": []any{1}}},
	}
	for i, o := range bad {
		_, err := LoadSettings("", o, map[string]string{})
		e, ok := err.(*apperr.Error)
		if !ok || e.Code != apperr.ConfigErr {
			t.Errorf("case %d: expected ConfigError, got %v", i, err)
		}
	}
}

func TestLaxCoercion(t *testing.T) {
	p := writeFile(t, "c.yaml", "browser: {headless: 'no'}\napi: {port: '9000'}\ntimeouts: {actionMs: 5.0}\n")
	s := load(t, p, nil, nil)
	if s.Browser.Headless || s.API.Port != 9000 || s.Timeouts.ActionMs != 5 {
		t.Fatalf("%+v", s)
	}
}

func TestFingerprintPlatform(t *testing.T) {
	s := load(t, "", nil, map[string]string{"AUTOBROWSER_FINGERPRINT_PLATFORM": "linux"})
	if s.Browser.FingerprintPlatform == nil || *s.Browser.FingerprintPlatform != "linux" {
		t.Fatal("not linux")
	}
	if load(t, "", nil, nil).Browser.FingerprintPlatform != nil {
		t.Fatal("expected nil")
	}
	if _, err := LoadSettings("", nil, map[string]string{"AUTOBROWSER_FINGERPRINT_PLATFORM": "android"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSafeDumpMasksSecrets(t *testing.T) {
	s := load(t, "", map[string]any{"proxy": map[string]any{"password": "topsecret", "host": "h"}},
		map[string]string{"AUTOBROWSER_API_TOKEN": "abc"})
	out := omap.Marshal(s.SafeDump(), 2)
	if strings.Contains(out, "topsecret") || !strings.Contains(out, `"password": "tops***"`) || !strings.Contains(out, `"token": "***"`) {
		t.Fatal(out)
	}
}

func TestSafeDumpDefaults(t *testing.T) {
	want := `{
  "browser": {
    "headless": true,
    "engine": "patchright",
    "geoip": false,
    "launchArgs": [],
    "viewport": {
      "width": 1280,
      "height": 720
    }
  },
  "profiles": {
    "dir": ".profiles"
  },
  "api": {
    "host": "127.0.0.1",
    "port": 8080,
    "maxSessions": 10
  },
  "timeouts": {
    "startupMs": 30000,
    "actionMs": 10000,
    "navigationMs": 30000,
    "idleSessionMs": 300000
  },
  "proxy": {
    "enabled": false,
    "scheme": "socks5",
    "socks2httpBin": "",
    "localPort": 0
  },
  "logging": {
    "level": "INFO",
    "maskSecrets": true
  },
  "output": {
    "maxContentBytes": 1048576,
    "contentFormat": "html"
  }
}`
	if got := omap.Marshal(load(t, "", nil, nil).SafeDump(), 2); got != want {
		t.Fatal(got)
	}
}

func TestValidateEngine(t *testing.T) {
	s := load(t, "", nil, nil)
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "cloakbrowser") {
		t.Fatal(err)
	}
	s = load(t, "", nil, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}
