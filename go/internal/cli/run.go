package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/browser"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

var runCommand = command{name: "run", group: groupScenarios, run: runCmd,
	help: "Open URLs and print cookies, headers and page content as JSON.",
	long: "autobrowser run -u https://example.com --format text\n" +
		"autobrowser run -u https://a.com -u https://b.com -o result.json",
	opts: []option{
		{"-u", "--url", "<str>", "URL to open; repeat for more tabs. [required]"},
		{"-c", "--config", "<file>", "YAML/JSON config file."},
		{"-p", "--profile", "<str>", "Persistent profile name."},
		{"", "--profile-dir", "<directory>", "Directory of profiles."},
		{"", "--user-agent", "<str>", "Override the User-Agent."},
		{"", "--headless/--headful", "", "Hide or show the browser window."},
		{"", "--timeout", "<int>", "Navigation and action timeout, ms."},
		{"", "--proxy", "<str>", "Proxy URL: socks5://user:pass@host:port."},
		{"", "--viewport", "<str>", "Viewport size WIDTHxHEIGHT, e.g. 1920x1080."},
		{"-o", "--output", "<path>", "Save the JSON result to a file."},
		{"", "--format", "<str>", "Page content format: html or text. [default: html]"},
	}}

// runOpts are the parsed options of run; headless is nil unless given.
type runOpts struct {
	urls                                                         []string
	config, profile, profileDir, userAgent, proxy, viewport, out string
	format                                                       string
	headless                                                     *bool
	timeout                                                      *int
}

func parseRun(e *env) (*runOpts, error) {
	o := &runOpts{}
	var urls listFlag
	var timeout string
	fs := newFlags(e, "run")
	cfg := configFlag(fs)
	fs.Var(&urls, "url", "URL to open.")
	fs.Var(&urls, "u", "URL to open (short).")
	fs.StringVar(&o.profile, "profile", "", "Persistent profile name.")
	fs.StringVar(&o.profile, "p", "", "Persistent profile name (short).")
	fs.StringVar(&o.profileDir, "profile-dir", "", "Directory of profiles.")
	fs.StringVar(&o.userAgent, "user-agent", "", "Override the User-Agent.")
	setHeadless := func(v bool) func(string) error {
		return func(string) error { o.headless = &v; return nil }
	}
	fs.BoolFunc("headless", "Hide the browser window.", setHeadless(true))
	fs.BoolFunc("headful", "Show the browser window.", setHeadless(false))
	fs.StringVar(&timeout, "timeout", "", "Navigation and action timeout, ms.")
	fs.StringVar(&o.proxy, "proxy", "", "Proxy URL: socks5://user:pass@host:port.")
	fs.StringVar(&o.viewport, "viewport", "", "Viewport size WIDTHxHEIGHT, e.g. 1920x1080.")
	fs.StringVar(&o.out, "output", "", "Save the JSON result to a file.")
	fs.StringVar(&o.out, "o", "", "Save the JSON result to a file (short).")
	fs.StringVar(&o.format, "format", "html", "Page content format: html or text.")
	if err := parse(fs, e.args); err != nil {
		return nil, err
	}
	o.config, o.urls = *cfg, urls
	if len(o.urls) == 0 {
		return nil, &usageError{"Missing option '--url' / '-u'."}
	}
	if o.profileDir != "" {
		if st, err := os.Stat(o.profileDir); err == nil && !st.IsDir() {
			return nil, &usageError{fmt.Sprintf("Invalid value for '--profile-dir': Directory '%s' is a file.", o.profileDir)}
		}
	}
	if timeout != "" {
		n, err := strconv.Atoi(strings.TrimSpace(timeout))
		if err != nil {
			return nil, &usageError{fmt.Sprintf("Invalid value for '--timeout': '%s' is not a valid int.", timeout)}
		}
		o.timeout = &n
	}
	return o, nil
}

// overrides are the settings overrides of the options (_settings_from_options).
func (o *runOpts) overrides() (map[string]any, error) {
	ov := map[string]any{}
	b := map[string]any{}
	if o.headless != nil {
		b["headless"] = *o.headless
	}
	if o.profileDir != "" {
		ov["profiles"] = map[string]any{"dir": o.profileDir}
	}
	if o.userAgent != "" {
		b["userAgent"] = o.userAgent
	}
	if o.timeout != nil {
		ov["timeouts"] = map[string]any{"actionMs": *o.timeout, "navigationMs": *o.timeout}
	}
	if o.proxy != "" {
		if err := applyProxyURL(o.proxy, ov); err != nil {
			return nil, err
		}
	}
	if o.viewport != "" {
		vp, err := parseViewport(o.viewport)
		if err != nil {
			return nil, err
		}
		b["viewport"] = vp
	}
	if len(b) > 0 {
		ov["browser"] = b
	}
	return ov, nil
}

func runCmd(e *env) error {
	o, err := parseRun(e)
	if err != nil {
		return err
	}
	ov, err := o.overrides()
	if err != nil {
		return err
	}
	settings, err := settingsFromOptions(o.config, ov)
	if err != nil {
		return err
	}
	session, err := browser.Start(settings, browser.Options{URLs: o.urls, Profile: o.profile,
		UserAgent: o.userAgent, Headless: o.headless})
	if err != nil {
		return err
	}
	defer session.Close(false)

	list := session.ListPages()
	pages := omap.New()
	for _, id := range list.Keys() {
		url, _ := list.Get(id)
		content, err := session.Content(id, o.format)
		if err != nil {
			return err
		}
		pages.Set(id, omap.New("url", url, "headers", session.Headers(id), "content", content))
	}
	cookies, err := session.Cookies()
	if err != nil {
		return err
	}
	result := omap.New("success", true, "sessionId", newSessionID(), "cookies", cookies, "pages", pages)
	text := omap.Marshal(result, 2)
	if o.out != "" {
		if err := os.WriteFile(o.out, []byte(text), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "Result saved to %s\n", o.out)
	} else {
		fmt.Fprintln(e.stdout, text)
	}
	return nil
}
