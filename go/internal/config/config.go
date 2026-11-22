// Package config ports autobrowser_client/config.py: settings, defaults,
// file and AUTOBROWSER_* loading, and the masked dump used by show-config.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

type ViewportSettings struct {
	Width  int
	Height int
}

type BrowserSettings struct {
	Headless            bool
	Engine              string
	FingerprintPlatform *string
	Geoip               bool
	LaunchArgs          []string
	UserAgent           *string
	Viewport            ViewportSettings
}

type ProfilesSettings struct {
	Dir string
}

type APISettings struct {
	Host        string
	Port        int
	Token       *string
	MaxSessions int
}

type TimeoutsSettings struct {
	StartupMs     int
	ActionMs      int
	NavigationMs  int
	IdleSessionMs int
}

type ProxySettings struct {
	Enabled       bool
	Scheme        string
	Host          *string
	Port          *int
	Username      *string
	Password      *string
	Socks2httpBin string
	LocalPort     int
}

type LoggingSettings struct {
	Level       string
	File        *string
	MaskSecrets bool
}

type OutputSettings struct {
	MaxContentBytes int
	ContentFormat   string
}

type Settings struct {
	Browser  BrowserSettings
	Profiles ProfilesSettings
	API      APISettings
	Timeouts TimeoutsSettings
	Proxy    ProxySettings
	Logging  LoggingSettings
	Output   OutputSettings
}

// ProxyURL builds the upstream proxy URL for socks2http; "" means no host
// (Python returns None).
func (p ProxySettings) ProxyURL() string {
	if p.Host == nil || *p.Host == "" {
		return ""
	}
	auth := ""
	if p.Username != nil && *p.Username != "" {
		auth = *p.Username
		if p.Password != nil && *p.Password != "" {
			auth += ":" + *p.Password
		}
		auth += "@"
	}
	port := 8080
	if p.Scheme == "socks5" {
		port = 1080
	}
	if p.Port != nil && *p.Port != 0 {
		port = *p.Port
	}
	return fmt.Sprintf("%s://%s%s:%d", p.Scheme, auth, *p.Host, port)
}

// Validate checks what the Go build cannot run. It is called at launch time
// (not by LoadSettings), so show-config works for any engine like in Python.
func (s *Settings) Validate() error {
	if s.Browser.Engine != "cloakbrowser" {
		return apperr.Config(fmt.Sprintf(
			"Go version supports only browser.engine=cloakbrowser (got %q); "+
				"set AUTOBROWSER_BROWSER_ENGINE=cloakbrowser or browser.engine in the config", s.Browser.Engine))
	}
	return nil
}

func newSettings(root map[string]any) (*Settings, error) {
	d := &decoder{}
	s := &Settings{}
	d.extra(nil, root, "browser", "profiles", "api", "timeouts", "proxy", "logging", "output")

	b := d.obj(nil, root, "browser", "headless", "engine", "fingerprintPlatform", "geoip", "launchArgs", "userAgent", "viewport")
	bl := []string{"browser"}
	s.Browser.Headless = d.boolean(bl, b, "headless", true)
	s.Browser.Engine = d.literal(bl, b, "engine", "patchright", "patchright", "cloakbrowser")
	s.Browser.FingerprintPlatform = d.optLiteral(bl, b, "fingerprintPlatform", "windows", "linux", "macos")
	s.Browser.Geoip = d.boolean(bl, b, "geoip", false)
	s.Browser.LaunchArgs = d.strList(bl, b, "launchArgs")
	s.Browser.UserAgent = d.optStr(bl, b, "userAgent")
	vp := d.obj(bl, b, "viewport", "width", "height")
	vl := []string{"browser", "viewport"}
	s.Browser.Viewport.Width = d.integer(vl, vp, "width", 1280, intBounds{gt0: true})
	s.Browser.Viewport.Height = d.integer(vl, vp, "height", 720, intBounds{gt0: true})

	p := d.obj(nil, root, "profiles", "dir")
	s.Profiles.Dir = d.str([]string{"profiles"}, p, "dir", ".profiles")

	a := d.obj(nil, root, "api", "host", "port", "token", "maxSessions")
	al := []string{"api"}
	s.API.Host = d.str(al, a, "host", "127.0.0.1")
	s.API.Port = d.integer(al, a, "port", 8080, intBounds{min: 1, max: 65535})
	s.API.Token = d.optStr(al, a, "token")
	s.API.MaxSessions = d.integer(al, a, "maxSessions", 10, intBounds{min: 1})

	t := d.obj(nil, root, "timeouts", "startupMs", "actionMs", "navigationMs", "idleSessionMs")
	tl := []string{"timeouts"}
	s.Timeouts.StartupMs = d.integer(tl, t, "startupMs", 30000, intBounds{})
	s.Timeouts.ActionMs = d.integer(tl, t, "actionMs", 10000, intBounds{})
	s.Timeouts.NavigationMs = d.integer(tl, t, "navigationMs", 30000, intBounds{})
	s.Timeouts.IdleSessionMs = d.integer(tl, t, "idleSessionMs", 300000, intBounds{})

	x := d.obj(nil, root, "proxy", "enabled", "scheme", "host", "port", "username", "password", "socks2httpBin", "localPort")
	xl := []string{"proxy"}
	s.Proxy.Enabled = d.boolean(xl, x, "enabled", false)
	s.Proxy.Scheme = d.str(xl, x, "scheme", "socks5")
	s.Proxy.Host = d.optStr(xl, x, "host")
	s.Proxy.Port = d.optInt(xl, x, "port", intBounds{min: 1, max: 65535})
	s.Proxy.Username = d.optStr(xl, x, "username")
	s.Proxy.Password = d.optStr(xl, x, "password")
	s.Proxy.Socks2httpBin = d.str(xl, x, "socks2httpBin", "")
	s.Proxy.LocalPort = d.integer(xl, x, "localPort", 0, intBounds{min: 0, max: 65535})

	lg := d.obj(nil, root, "logging", "level", "file", "maskSecrets")
	ll := []string{"logging"}
	s.Logging.Level = d.literal(ll, lg, "level", "INFO", "DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL")
	s.Logging.File = d.optStr(ll, lg, "file")
	s.Logging.MaskSecrets = d.boolean(ll, lg, "maskSecrets", true)

	o := d.obj(nil, root, "output", "maxContentBytes", "contentFormat")
	ol := []string{"output"}
	s.Output.MaxContentBytes = d.integer(ol, o, "maxContentBytes", 1048576, intBounds{})
	s.Output.ContentFormat = d.literal(ol, o, "contentFormat", "html", "html", "text")

	if len(d.errs) > 0 {
		noun := "errors"
		if len(d.errs) == 1 {
			noun = "error"
		}
		return nil, apperr.Config(fmt.Sprintf("%d validation %s for Settings\n%s", len(d.errs), noun, strings.Join(d.errs, "\n")))
	}
	return s, nil
}

// SafeDump returns the settings keyed by their camelCase names in the order
// of Python's model_dump (unset optional values omitted), with secrets masked.
func (s *Settings) SafeDump() *omap.Map {
	put := func(m *omap.Map, key string, v *string) {
		if v != nil {
			m.Set(key, *v)
		}
	}
	browser := omap.New("headless", s.Browser.Headless, "engine", s.Browser.Engine)
	put(browser, "fingerprintPlatform", s.Browser.FingerprintPlatform)
	browser.Set("geoip", s.Browser.Geoip)
	args := make([]any, len(s.Browser.LaunchArgs))
	for i, a := range s.Browser.LaunchArgs {
		args[i] = a
	}
	browser.Set("launchArgs", args)
	put(browser, "userAgent", s.Browser.UserAgent)
	browser.Set("viewport", omap.New("width", s.Browser.Viewport.Width, "height", s.Browser.Viewport.Height))

	api := omap.New("host", s.API.Host, "port", s.API.Port)
	put(api, "token", s.API.Token)
	api.Set("maxSessions", s.API.MaxSessions)

	proxy := omap.New("enabled", s.Proxy.Enabled, "scheme", s.Proxy.Scheme)
	put(proxy, "host", s.Proxy.Host)
	if s.Proxy.Port != nil {
		proxy.Set("port", *s.Proxy.Port)
	}
	put(proxy, "username", s.Proxy.Username)
	put(proxy, "password", s.Proxy.Password)
	proxy.Set("socks2httpBin", s.Proxy.Socks2httpBin)
	proxy.Set("localPort", s.Proxy.LocalPort)

	logging := omap.New("level", s.Logging.Level)
	put(logging, "file", s.Logging.File)
	logging.Set("maskSecrets", s.Logging.MaskSecrets)

	return maskMapping(omap.New(
		"browser", browser,
		"profiles", omap.New("dir", s.Profiles.Dir),
		"api", api,
		"timeouts", omap.New("startupMs", s.Timeouts.StartupMs, "actionMs", s.Timeouts.ActionMs,
			"navigationMs", s.Timeouts.NavigationMs, "idleSessionMs", s.Timeouts.IdleSessionMs),
		"proxy", proxy,
		"logging", logging,
		"output", omap.New("maxContentBytes", s.Output.MaxContentBytes, "contentFormat", s.Output.ContentFormat),
	))
}

var sensitiveKeys = map[string]bool{"authorization": true, "cookie": true, "password": true, "token": true, "proxy_password": true}

func maskValue(v string) string {
	r := []rune(v)
	if len(r) <= 4 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "***"
}

// maskMapping ports logging_utils.mask_mapping.
func maskMapping(m *omap.Map) *omap.Map {
	out := omap.New()
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		if sub, ok := v.(*omap.Map); ok {
			out.Set(k, maskMapping(sub))
			continue
		}
		if s, ok := v.(string); ok && sensitiveKeys[strings.ToLower(k)] {
			out.Set(k, maskValue(s))
			continue
		}
		out.Set(k, v)
	}
	return out
}

func deepMerge(base, updates map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range updates {
		if uv, ok := v.(map[string]any); ok {
			if bv, ok := result[k].(map[string]any); ok {
				result[k] = deepMerge(bv, uv)
				continue
			}
		}
		result[k] = v
	}
	return result
}

func loadFile(path string) (map[string]any, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, apperr.Config("Configuration file does not exist: " + path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data any
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		data, err = omap.ParseYAML(raw)
		if err != nil {
			return nil, apperr.Config("Invalid YAML in " + path + ": " + err.Error())
		}
		if falsy(data) {
			data = omap.New()
		}
	case ".json":
		data, err = omap.ParseJSON(raw)
		if err != nil {
			return nil, apperr.Config("Invalid JSON in " + path + ": " + err.Error())
		}
	default:
		return nil, apperr.Config("Unsupported config file format: " + filepath.Ext(path))
	}
	m, ok := data.(*omap.Map)
	if !ok {
		return nil, apperr.Config("Configuration file must contain an object at the root")
	}
	return omap.Plain(m).(map[string]any), nil
}

func falsy(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case bool:
		return !t
	case int:
		return t == 0
	case float64:
		return t == 0
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case *omap.Map:
		return t.Len() == 0
	}
	return false
}

// pyInt mimics int(str): surrounding whitespace, a sign and single
// underscores between digits are accepted.
func pyInt(s string) (int, error) {
	t := strings.TrimSpace(s)
	clean := t
	if strings.Contains(t, "_") {
		body := strings.TrimLeft(t, "+-")
		if strings.HasPrefix(body, "_") || strings.HasSuffix(body, "_") || strings.Contains(body, "__") {
			return 0, fmt.Errorf("invalid literal for int() with base 10: %q", s)
		}
		clean = strings.ReplaceAll(t, "_", "")
	}
	n, err := strconv.Atoi(clean)
	if err != nil {
		return 0, fmt.Errorf("invalid literal for int() with base 10: '%s'", s)
	}
	return n, nil
}

type envMapping struct {
	key  string
	path []string
	kind string
}

var envMappings = []envMapping{
	{"AUTOBROWSER_HEADLESS", []string{"browser", "headless"}, "bool"},
	{"AUTOBROWSER_BROWSER_ENGINE", []string{"browser", "engine"}, "str"},
	{"AUTOBROWSER_FINGERPRINT_PLATFORM", []string{"browser", "fingerprintPlatform"}, "str"},
	{"AUTOBROWSER_GEOIP", []string{"browser", "geoip"}, "bool"},
	{"AUTOBROWSER_USER_AGENT", []string{"browser", "userAgent"}, "str"},
	{"AUTOBROWSER_VIEWPORT_WIDTH", []string{"browser", "viewport", "width"}, "int"},
	{"AUTOBROWSER_VIEWPORT_HEIGHT", []string{"browser", "viewport", "height"}, "int"},
	{"AUTOBROWSER_PROFILE_DIR", []string{"profiles", "dir"}, "str"},
	{"AUTOBROWSER_API_HOST", []string{"api", "host"}, "str"},
	{"AUTOBROWSER_API_PORT", []string{"api", "port"}, "int"},
	{"AUTOBROWSER_API_TOKEN", []string{"api", "token"}, "str"},
	{"AUTOBROWSER_API_MAX_SESSIONS", []string{"api", "maxSessions"}, "int"},
	{"AUTOBROWSER_LOG_LEVEL", []string{"logging", "level"}, "str"},
	{"AUTOBROWSER_LOG_FILE", []string{"logging", "file"}, "str"},
	{"AUTOBROWSER_PROXY_ENABLED", []string{"proxy", "enabled"}, "bool"},
	{"AUTOBROWSER_PROXY_SCHEME", []string{"proxy", "scheme"}, "str"},
	{"AUTOBROWSER_PROXY_HOST", []string{"proxy", "host"}, "str"},
	{"AUTOBROWSER_PROXY_PORT", []string{"proxy", "port"}, "int"},
	{"AUTOBROWSER_PROXY_USERNAME", []string{"proxy", "username"}, "str"},
	{"AUTOBROWSER_PROXY_PASSWORD", []string{"proxy", "password"}, "str"},
	{"AUTOBROWSER_SOCKS2HTTP_BIN", []string{"proxy", "socks2httpBin"}, "str"},
	{"AUTOBROWSER_PROXY_LOCAL_PORT", []string{"proxy", "localPort"}, "int"},
	{"AUTOBROWSER_TIMEOUT_STARTUP_MS", []string{"timeouts", "startupMs"}, "int"},
	{"AUTOBROWSER_TIMEOUT_ACTION_MS", []string{"timeouts", "actionMs"}, "int"},
	{"AUTOBROWSER_TIMEOUT_NAVIGATION_MS", []string{"timeouts", "navigationMs"}, "int"},
	{"AUTOBROWSER_SESSION_IDLE_MS", []string{"timeouts", "idleSessionMs"}, "int"},
	{"AUTOBROWSER_MAX_CONTENT_BYTES", []string{"output", "maxContentBytes"}, "int"},
	{"AUTOBROWSER_CONTENT_FORMAT", []string{"output", "contentFormat"}, "str"},
}

func envOverrides(env map[string]string) (map[string]any, error) {
	result := map[string]any{}
	for _, m := range envMappings {
		raw, ok := env[m.key]
		if !ok {
			continue
		}
		var value any = raw
		switch m.kind {
		case "bool":
			switch strings.ToLower(strings.TrimSpace(raw)) {
			case "1", "true", "yes", "on":
				value = true
			default:
				value = false
			}
		case "int":
			n, err := pyInt(raw)
			if err != nil {
				return nil, apperr.Config(m.key + ": " + err.Error())
			}
			value = n
		}
		cursor := result
		for _, part := range m.path[:len(m.path)-1] {
			next, ok := cursor[part].(map[string]any)
			if !ok {
				next = map[string]any{}
				cursor[part] = next
			}
			cursor = next
		}
		cursor[m.path[len(m.path)-1]] = value
	}
	return result, nil
}

// LoadSettings merges defaults < config file < AUTOBROWSER_* env < overrides.
// A nil env means the process environment (an empty non-nil map means none;
// Python treats {} like os.environ).
func LoadSettings(configPath string, overrides map[string]any, env map[string]string) (*Settings, error) {
	data := map[string]any{}
	if configPath != "" {
		file, err := loadFile(configPath)
		if err != nil {
			return nil, err
		}
		data = deepMerge(data, file)
	}
	if env == nil {
		env = map[string]string{}
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
	}
	envData, err := envOverrides(env)
	if err != nil {
		return nil, err
	}
	data = deepMerge(data, envData)
	if overrides != nil {
		data = deepMerge(data, overrides)
	}
	return newSettings(data)
}
