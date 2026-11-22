package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/engine"
	"github.com/foxzi/autobrowser-client/go/internal/profiles"
)

// Options of Start (the arguments of BrowserSession._start).
type Options struct {
	Profile   string
	UserAgent string
	// Headless overrides settings.browser.headless when not nil.
	Headless *bool
	// Viewport overrides settings.browser.viewport when not nil.
	Viewport        *config.ViewportSettings
	PreserveSession bool
	// Fingerprint is the scenario session.fingerprint; JS injection is not ported yet.
	Fingerprint any
}

// Session is a running browser: the Go BrowserSession for the cloakbrowser engine.
type Session struct {
	settings *config.Settings
	cmd      *exec.Cmd
	exited   chan struct{}
	root     *rod.Browser // the CDP connection
	ctx      *rod.Browser // where pages live: root for profiles, an incognito context otherwise
	tmpDir   string       // temporary user-data-dir, removed on Close

	lock       *profiles.Lock
	profileDir string

	ua        string
	spoof     *platformSpoof
	viewport  config.ViewportSettings
	mobile    bool
	mu        sync.Mutex
	pages     []*tab
	known     map[proto.TargetTargetID]bool // targets seen by sync, registered or deliberately not
	nextPage  int
	settingsT config.TimeoutsSettings
	output    config.OutputSettings
}

// playwrightSwitches mirrors chromiumSwitches() of the Playwright (patchright)
// bundled with the Python client, minus IGNORE_DEFAULT_ARGS.
func playwrightSwitches() []string {
	disabled := []string{
		"AvoidUnnecessaryBeforeUnloadCheckSync", "DestroyProfileOnBrowserClose", "DialMediaRouteProvider",
		"GlobalMediaControls", "HttpsUpgrades", "LensOverlay", "PaintHolding", "ThirdPartyStoragePartitioning",
		"BlockOriginHeaderModificationOnRedirect", "AvoidCorsURLLoaderRestartOnRedirect", "Translate",
		"AutoDeElevate", "OptimizationHints", "NetworkTimeServiceQuerying", "AimEnabled", "msForceBrowserSignIn",
		"msEdgeUpdateLaunchServicesPreferredVersion",
	}
	return []string{
		"--disable-field-trial-config",
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--no-default-browser-check",
		"--disable-dev-shm-usage",
		"--disable-edgeupdater",
		"--disable-features=" + strings.Join(disabled, ","),
		"--enable-features=CDPScreenshotNewSurface",
		"--disable-hang-monitor",
		"--disable-prompt-on-repost",
		"--disable-renderer-backgrounding",
		"--disable-updater-scheduler",
		"--force-color-profile=srgb",
		"--no-first-run",
		"--password-store=basic",
		"--use-mock-keychain",
		"--no-service-autorun",
		"--export-tagged-pdf",
		"--disable-search-engine-choice-screen",
		"--edge-skip-compat-layer-relaunch",
		"--disable-infobars",
		"--disable-search-engine-choice-screen",
		"--disable-sync",
		"--disable-blink-features=AutomationControlled",
	}
}

// chromeCommandLine is the whole argument list Playwright would pass:
// switches, headless flags, --no-sandbox, proxy, the cloakbrowser args, then
// the profile and the debugging port.
func chromeCommandLine(userArgs []string, headless bool, proxyServer, userDataDir string, persistent bool) []string {
	args := playwrightSwitches()
	if headless {
		args = append(args, "--headless", "--hide-scrollbars", "--mute-audio",
			"--blink-settings=primaryHoverType=2,availableHoverTypes=2,primaryPointerType=4,availablePointerTypes=4")
	}
	args = append(args, "--no-sandbox")
	if proxyServer != "" {
		args = append(args, "--proxy-server="+proxyServer, "--proxy-bypass-list=<-loopback>")
	}
	args = append(args, userArgs...)
	args = append(args, "--user-data-dir="+userDataDir, "--remote-debugging-port=0")
	if persistent {
		return append(args, "about:blank")
	}
	return append(args, "--no-startup-window")
}

func startFailed(format string, a ...any) error {
	return apperr.New(apperr.BrowserStartFailed, fmt.Sprintf(format, a...))
}

// proxyServer is the --proxy-server value. Python runs a local HTTP relay in
// front of the upstream proxy; Chromium talks to plain http/socks5 itself.
func proxyServer(p config.ProxySettings) (string, error) {
	if !p.Enabled {
		return "", nil
	}
	url := p.ProxyURL()
	if url == "" {
		return "", startFailed("Proxy host is not configured")
	}
	if (p.Username != nil && *p.Username != "") || (p.Password != nil && *p.Password != "") {
		return "", startFailed("proxy with credentials is not supported in Go yet (use a proxy without login)")
	}
	switch p.Scheme {
	case "http", "socks5":
		return url, nil
	}
	return "", startFailed("proxy scheme %q is not supported in Go yet (use http or socks5)", p.Scheme)
}

// Start launches the browser (BrowserSession._start, cloakbrowser branch).
func Start(settings *config.Settings, opts Options) (s *Session, err error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	s = &Session{settings: settings, known: map[proto.TargetTargetID]bool{},
		settingsT: settings.Timeouts, output: settings.Output}
	defer func() {
		if err != nil {
			slog.Error(fmt.Sprintf("Session failed to start: %s", err))
			s.cleanup()
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != apperr.BrowserStartFailed {
				err = apperr.New(apperr.BrowserStartFailed, err.Error())
			}
			s = nil
		}
	}()

	proxy, err := proxyServer(settings.Proxy)
	if err != nil {
		return s, err
	}
	headless := settings.Browser.Headless
	if opts.Headless != nil {
		headless = *opts.Headless
	}
	s.viewport = settings.Browser.Viewport
	if opts.Viewport != nil {
		s.viewport = *opts.Viewport
	}
	s.ua = opts.UserAgent
	if s.ua == "" && settings.Browser.UserAgent != nil {
		s.ua = *settings.Browser.UserAgent
	}
	launchArgs := append([]string(nil), settings.Browser.LaunchArgs...)
	if settings.Browser.Geoip {
		return s, startFailed("browser.geoip is not supported in Go yet " +
			"(set browser.geoip=false or AUTOBROWSER_GEOIP=false)")
	}
	s.spoof = detectPlatformFromUA(s.ua)
	s.mobile = s.spoof != nil && s.spoof.mobile
	if opts.Fingerprint != nil {
		slog.Warn("session.fingerprint is ignored: the Go version has no JS fingerprint injection yet")
	}

	bin, err := FindBinary()
	if err != nil {
		return s, err
	}
	if fp := settings.Browser.FingerprintPlatform; fp != nil && *fp != "" {
		launchArgs = append(launchArgs, "--fingerprint-platform="+*fp)
	}
	chromeArgs := BuildArgs(launchArgs, headless)

	userDataDir := ""
	if opts.Profile != "" {
		s.profileDir = filepath.Join(settings.Profiles.Dir, opts.Profile)
		if err := os.MkdirAll(s.profileDir, 0o755); err != nil {
			return s, err
		}
		s.lock = profiles.NewLock(s.profileDir)
		if err := s.lock.Acquire(); err != nil {
			return s, err
		}
		// Python appends --restore-last-session to launch_args after the
		// cloakbrowser args were built, so Chromium never gets it; same here.
		slog.Info("Using profile directory: " + s.profileDir)
		userDataDir = s.profileDir
	} else {
		if s.tmpDir, err = os.MkdirTemp("", "autobrowser_chromium_profile-"); err != nil {
			return s, err
		}
		userDataDir = s.tmpDir
	}

	args := chromeCommandLine(chromeArgs, headless, proxy, userDataDir, opts.Profile != "")
	wsURL, err := s.launch(bin, args)
	if err != nil {
		return s, err
	}
	s.root = rod.New().ControlURL(wsURL).NoDefaultDevice()
	if err := s.root.Connect(); err != nil {
		return s, err
	}
	if opts.Profile != "" {
		s.ctx = s.root
	} else if s.ctx, err = s.root.Incognito(); err != nil {
		return s, err
	}

	if err := s.openPages(opts); err != nil {
		return s, err
	}
	slog.Info("Session is running")
	return s, nil
}

var devtoolsRe = regexp.MustCompile(`DevTools listening on (ws://\S+)`)

// launch starts Chromium and returns its DevTools WebSocket URL.
func (s *Session) launch(bin string, args []string) (string, error) {
	cmd := exec.Command(bin, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", startFailed("failed to start %s: %v", bin, err)
	}
	s.cmd = cmd
	s.exited = make(chan struct{})
	urls := make(chan string, 1)
	var tail strings.Builder
	go func() {
		sc := bufio.NewScanner(stderr)
		sent := false
		for sc.Scan() {
			if m := devtoolsRe.FindStringSubmatch(sc.Text()); m != nil && !sent {
				urls <- m[1]
				sent = true
			} else if !sent && tail.Len() < 2000 {
				tail.WriteString(sc.Text() + "\n")
			}
		}
		_ = cmd.Wait()
		close(s.exited)
	}()
	select {
	case u := <-urls:
		return u, nil
	case <-s.exited:
		return "", startFailed("browser exited during startup: %s", strings.TrimSpace(tail.String()))
	case <-time.After(30 * time.Second):
		return "", startFailed("browser did not report a DevTools URL in 30000 ms")
	}
}

// openPages registers the first tabs: saved ones, existing ones, or a new blank tab.
func (s *Session) openPages(opts Options) error {
	var urls []string
	if opts.PreserveSession && s.profileDir != "" {
		urls = loadSavedTabs(s.profileDir)
	}
	existing, err := s.targets()
	if err != nil {
		return err
	}
	switch {
	case len(urls) > 0:
		for _, id := range existing { // Python leaves the initial blank tab unregistered
			s.known[id] = true
		}
		for _, u := range urls {
			t, err := s.newTab()
			if err != nil {
				return err
			}
			if err := t.goto_(u, s.settingsT.NavigationMs); err != nil {
				slog.Warn(fmt.Sprintf("Failed to restore tab %s: %s", t.id, u))
			}
			slog.Info(fmt.Sprintf("Opened %s as %s", u, t.id))
		}
	case len(existing) > 0:
		for _, id := range existing {
			if _, err := s.register(id); err != nil {
				return err
			}
		}
		slog.Info(fmt.Sprintf("Registered %d existing page(s)", len(existing)))
	default:
		if _, err := s.newTab(); err != nil {
			return err
		}
	}
	return nil
}

// Close stops the session; preserve saves the open tabs of a profile first.
func (s *Session) Close(preserve bool) {
	slog.Info("Stopping session")
	if preserve {
		s.saveTabs()
	}
	s.cleanup()
	slog.Info("Session stopped")
}

func (s *Session) cleanup() {
	if s.root != nil {
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			_ = s.root.Close()
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		select {
		case <-s.exited:
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
			<-s.exited
		}
	}
	if s.tmpDir != "" {
		_ = os.RemoveAll(s.tmpDir)
	}
	if s.lock != nil {
		s.lock.Release()
	}
	s.root, s.ctx, s.cmd, s.lock = nil, nil, nil, nil
	s.pages = nil
}

var _ engine.Browser = (*Session)(nil)

var _ = context.Background
