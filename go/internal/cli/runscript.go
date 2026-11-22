package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// exitStatus is a silent non-zero exit (typer.Exit).
type exitStatus struct{ code int }

func (e *exitStatus) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// listFlag is a repeatable string option.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// runScriptOpts are the parsed options of run-script.
type runScriptOpts struct {
	file, config, profile, proxy, viewport, output, artifacts string
	preserve                                                  bool
	vars                                                      []string
}

func parseRunScript(e *env) (*runScriptOpts, error) {
	o := &runScriptOpts{}
	fs := newFlags(e, "run-script")
	cfg := configFlag(fs)
	fs.StringVar(&o.file, "file", "", "Scenario file.")
	fs.StringVar(&o.file, "f", "", "Scenario file (short).")
	fs.StringVar(&o.profile, "profile", "", "Persistent profile name.")
	fs.StringVar(&o.profile, "p", "", "Persistent profile name (short).")
	fs.StringVar(&o.proxy, "proxy", "", "Proxy URL: socks5://user:pass@host:port.")
	fs.StringVar(&o.viewport, "viewport", "", "Viewport size WIDTHxHEIGHT, e.g. 1920x1080.")
	fs.StringVar(&o.output, "output", "", "Save the JSON result to a file.")
	fs.StringVar(&o.output, "o", "", "Save the JSON result to a file (short).")
	fs.BoolVar(&o.preserve, "preserve-session", false, "Save open tabs for the next session.")
	var vars listFlag
	fs.Var(&vars, "var", "Scenario variable NAME=VALUE; repeat for more.")
	fs.StringVar(&o.artifacts, "artifacts", "", "Directory for screenshot and HTML of failed steps.")
	if err := parse(fs, e.args); err != nil {
		return nil, err
	}
	o.config, o.vars = *cfg, vars
	if o.file == "" {
		return nil, &usageError{"Missing option '--file' / '-f'."}
	}
	st, err := os.Stat(o.file)
	switch {
	case err != nil:
		return nil, &usageError{fmt.Sprintf("Invalid value for '--file' / '-f': File '%s' does not exist.", o.file)}
	case st.IsDir():
		return nil, &usageError{fmt.Sprintf("Invalid value for '--file' / '-f': File '%s' is a directory.", o.file)}
	}
	return o, nil
}

// parseVars parses repeated NAME=VALUE options (_parse_vars).
func parseVars(items []string) (*omap.Map, error) {
	m := omap.New()
	for _, item := range items {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			return nil, &usageError{fmt.Sprintf("Invalid value: Invalid variable: '%s', expected NAME=VALUE", item)}
		}
		m.Set(name, value)
	}
	return m, nil
}

// parseViewport parses WIDTHxHEIGHT (_parse_viewport).
func parseViewport(s string) (map[string]any, error) {
	bad := &usageError{fmt.Sprintf(
		"Invalid value: Invalid viewport format: '%s', expected WIDTHxHEIGHT (e.g. 1920x1080)", s)}
	parts := strings.Split(strings.ToLower(s), "x")
	if len(parts) != 2 {
		return nil, bad
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return nil, bad
	}
	return map[string]any{"width": w, "height": h}, nil
}

var schemeRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):(.*)$`)

// applyProxyURL puts a proxy URL into the settings overrides (_apply_proxy_url,
// parsing like urllib.parse.urlparse: userinfo is not percent-decoded).
func applyProxyURL(proxyURL string, overrides map[string]any) error {
	scheme, rest := "", proxyURL
	if m := schemeRe.FindStringSubmatch(proxyURL); m != nil {
		if _, err := strconv.Atoi(m[2]); err != nil { // "host:1080" is not a scheme
			scheme, rest = strings.ToLower(m[1]), m[2]
		}
	}
	netloc := ""
	if strings.HasPrefix(rest, "//") {
		netloc = rest[2:]
		if i := strings.IndexAny(netloc, "/?#"); i >= 0 {
			netloc = netloc[:i]
		}
	}
	userinfo, hostinfo := "", netloc
	if i := strings.LastIndex(netloc, "@"); i >= 0 {
		userinfo, hostinfo = netloc[:i], netloc[i+1:]
	}
	host, port := hostinfo, ""
	if strings.HasPrefix(hostinfo, "[") {
		if i := strings.Index(hostinfo, "]"); i >= 0 {
			host = hostinfo[1:i]
			port = strings.TrimPrefix(hostinfo[i+1:], ":")
		}
	} else if i := strings.Index(hostinfo, ":"); i >= 0 {
		host, port = hostinfo[:i], hostinfo[i+1:]
	}
	p := map[string]any{"enabled": true, "scheme": scheme}
	if scheme == "" {
		p["scheme"] = "socks5"
	}
	if host != "" {
		p["host"] = strings.ToLower(host)
	} else {
		p["host"] = nil
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return fmt.Errorf("Port could not be cast to integer value as %q", port)
		}
		if n != 0 {
			p["port"] = n
		}
	}
	if userinfo != "" {
		user, pass, _ := strings.Cut(userinfo, ":")
		if user != "" {
			p["username"] = user
		}
		if pass != "" {
			p["password"] = pass
		}
	}
	overrides["proxy"] = p
	return nil
}

// runPlan is what run-script derives from the CLI flags and the scenario.
type runPlan struct {
	vars          *omap.Map
	artifactsDir  string
	overrides     map[string]any // settings overrides (proxy, viewport)
	profile       string
	preserve      bool
	browserOpts   browser.Options
	scriptOutFile string
}

// plan combines the flags with script.session: for proxy, viewport and
// profile a non-empty flag wins over the scenario, then the scenario over the
// config; preserveSession is flag OR scenario; headless, userAgent and
// fingerprint come only from the scenario (headless defaults to true there and
// so always beats the config).
func plan(o *runScriptOpts, script *scenario.Script) (*runPlan, error) {
	vars, err := parseVars(o.vars)
	if err != nil {
		return nil, err
	}
	p := &runPlan{vars: vars, artifactsDir: o.artifacts, overrides: map[string]any{}}
	if p.artifactsDir == "" {
		p.artifactsDir = script.ErrorArtifacts
	}
	if p.artifactsDir != "" {
		if _, err := engine.ValidateOutputPath(p.artifactsDir); err != nil {
			return nil, err
		}
	}
	if script.OutputFile != "" {
		if _, err := engine.ValidateOutputPath(script.OutputFile); err != nil {
			return nil, err
		}
		p.scriptOutFile = script.OutputFile
	}

	proxy := o.proxy
	if proxy == "" {
		proxy = script.Session.Proxy
	}
	var viewport map[string]any
	switch {
	case o.viewport != "":
		if viewport, err = parseViewport(o.viewport); err != nil {
			return nil, err
		}
	case script.Session.Viewport != nil:
		viewport = map[string]any{"width": script.Session.Viewport.Width, "height": script.Session.Viewport.Height}
	}
	if proxy != "" {
		if err := applyProxyURL(proxy, p.overrides); err != nil {
			return nil, err
		}
	}
	if viewport != nil {
		p.overrides["browser"] = map[string]any{"viewport": viewport}
	}

	p.profile = o.profile
	if p.profile == "" {
		p.profile = script.Session.Profile
	}
	p.preserve = o.preserve || script.Session.PreserveSession
	headless := script.Session.Headless
	p.browserOpts = browser.Options{
		Profile:         p.profile,
		UserAgent:       script.Session.UserAgent,
		Headless:        &headless,
		PreserveSession: p.preserve,
		Fingerprint:     fingerprintArg(script.Session.Fingerprint),
	}
	return p, nil
}

// fingerprintArg: an empty overrides block means "enabled".
func fingerprintArg(f any) any {
	if fo, ok := f.(*scenario.FingerprintOverrides); ok && *fo == (scenario.FingerprintOverrides{}) {
		return true
	}
	return f
}

func newSessionID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}

func runScript(e *env) error {
	o, err := parseRunScript(e)
	if err != nil {
		return err
	}
	script, err := scenario.Load(o.file)
	if err != nil {
		return err
	}
	p, err := plan(o, script)
	if err != nil {
		return err
	}
	settings, err := settingsFromOptions(o.config, p.overrides)
	if err != nil {
		return err
	}
	session, err := browser.Start(settings, p.browserOpts)
	if err != nil {
		return err
	}
	defer session.Close(p.preserve)

	res, err := engine.Run(session, script, engine.Options{Vars: p.vars, ArtifactsDir: p.artifactsDir})
	if err != nil {
		return err
	}
	var profile any
	if p.profile != "" {
		profile = p.profile
	}
	text := omap.Marshal(omap.New("success", res.Success, "sessionId", newSessionID(),
		"profile", profile, "steps", res.Steps, "vars", res.Vars), 2)
	if p.scriptOutFile != "" {
		if err := os.WriteFile(p.scriptOutFile, []byte(text), 0o644); err != nil {
			return err
		}
	}
	if o.output != "" {
		if err := os.WriteFile(o.output, []byte(text), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "Result saved to %s\n", o.output)
	} else {
		fmt.Fprintln(e.stdout, text)
	}
	if !res.Success {
		return &exitStatus{1}
	}
	return nil
}
