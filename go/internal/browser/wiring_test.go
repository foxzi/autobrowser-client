package browser

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/fingerprint"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

func TestBuildArgsGeoAndFontMetrics(t *testing.T) {
	args := BuildArgs([]string{"--lang=de", "--fingerprint-windows-font-metrics", "--enable-features=A"}, true, "Europe/Berlin", "de-DE")
	joined := strings.Join(args, " ")
	for _, w := range []string{"--fingerprint-timezone=Europe/Berlin", "--lang=de-DE", "--fingerprint-locale=de-DE", "--enable-features=A,MediaRouter"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %s in %s", w, joined)
		}
	}
	if strings.Contains(joined, "--lang=de ") || strings.Contains(BuildArgs(nil, true, "", "")[0], "timezone") {
		t.Errorf("unexpected args: %s", joined)
	}
	if got := strings.Join(BuildArgs([]string{"--fingerprint-windows-font-metrics"}, true, "", ""), " "); !strings.HasSuffix(got, "--enable-features=MediaRouter") {
		t.Errorf("font metrics alone: %s", got)
	}
}

func TestFingerprintPlatform(t *testing.T) {
	for ua, want := range map[string]string{
		"":                                "windows",
		"Mozilla/5.0 (X11; Linux x86_64)": "linux",
		"Mozilla/5.0 (Linux; Android 13)": "windows", // mobile flag decides android
	} {
		if got := fingerprintPlatform(ua, false); string(got) != want {
			t.Errorf("%q: %s", ua, got)
		}
	}
	if fingerprintPlatform("x", true) != "android" {
		t.Error("mobile must be android")
	}
}

func startWith(t *testing.T, edit func(*config.Settings), opts Options) *Session {
	t.Helper()
	requireBrowser(t)
	st, err := config.LoadSettings("", nil, map[string]string{"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"})
	if err != nil {
		t.Fatal(err)
	}
	st.Browser.Headless = true
	if edit != nil {
		edit(st)
	}
	s, err := Start(st, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(false) })
	return s
}

// The fingerprint script runs in the first tab and in popups before page scripts.
func TestFingerprintInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<script>
const gl = document.createElement('canvas').getContext('webgl');
const ext = gl && gl.getExtension('WEBGL_debug_renderer_info');
window.renderer = ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : 'none';
</script><a id="pop" href="/p" target="_blank">pop</a>`)
	}))
	defer srv.Close()
	seed := 12345
	s := startWith(t, nil, Options{Fingerprint: &scenario.FingerprintOverrides{Seed: &seed}})
	run, ok := runner(t, s)
	ok(run("open", "url", srv.URL))
	want := fmt.Sprintf(`{"result": %q}`, fingerprint.Generate(&seed, "windows").WebglRenderer)
	if got := ok(run("evaluate", "script", "window.renderer")); got != want {
		t.Errorf("first tab: %s, want %s", got, want)
	}
	ok(run("click", "locator", omap.New("type", "id", "value", "pop")))
	var got string
	for i := 0; i < 50; i++ {
		if r := s.ExecuteStep(step(t, "evaluate", "script", "window.renderer"), "page_1"); r.Success {
			if got = omap.Marshal(r.Data, 0); got != `{"result": null}` {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got != want {
		t.Errorf("popup: %s, want %s", got, want)
	}
}

// An Android user agent turns on mobile metrics and touch, in new tabs too.
func TestMobileEmulation(t *testing.T) {
	s := startWith(t, func(st *config.Settings) {
		ua := "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36"
		st.Browser.UserAgent = &ua
		st.Browser.Viewport = config.ViewportSettings{Width: 412, Height: 915}
	}, Options{})
	run, ok := runner(t, s)
	// innerWidth is 980: a page without a viewport meta tag gets the mobile
	// layout width, as in Python
	script := "[navigator.maxTouchPoints > 0, matchMedia('(pointer: coarse)').matches, innerWidth, screen.orientation.type, navigator.userAgent.includes('Android')]"
	want := `{"result": [true, true, 981, "portrait-primary", true]}`
	if got := ok(run("evaluate", "script", script)); got != want {
		t.Errorf("first tab: %s, want %s", got, want)
	}
	if _, err := s.newTab(); err != nil {
		t.Fatal(err)
	}
	if got := ok(s.ExecuteStep(step(t, "evaluate", "script", script), "page_1")); got != want {
		t.Errorf("new tab: %s, want %s", got, want)
	}
}

// With a proxy the browser goes through the relay, which adds the upstream
// credentials; the relay stops with the session.
func TestProxyRelay(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.test" || r.Header.Get("Proxy-Authorization") == "" {
			http.Error(w, "not proxied", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, "<title>via upstream</title>")
	}))
	defer upstream.Close()
	port := upstream.Listener.Addr().(*net.TCPAddr).Port
	host, user, pwd := "127.0.0.1", "u", "p"
	s := startWith(t, func(st *config.Settings) {
		st.Proxy = config.ProxySettings{Enabled: true, Scheme: "http", Host: &host, Port: &port, Username: &user, Password: &pwd, LocalPort: 0}
	}, Options{})
	run, ok := runner(t, s)
	ok(run("open", "url", "http://example.test/"))
	if got := ok(run("evaluate", "script", "document.title")); got != `{"result": "via upstream"}` {
		t.Errorf("title: %s", got)
	}
	relayURL := s.relay.HTTPProxyURL()
	s.Close(false)
	if c, err := net.Dial("tcp", strings.TrimPrefix(relayURL, "http://")); err == nil {
		c.Close()
		t.Errorf("relay %s still listens after Close", relayURL)
	}
}

// A profile session uses the fingerprint.json of the profile.
func TestProfileFingerprint(t *testing.T) {
	dir := t.TempDir()
	s := startWith(t, func(st *config.Settings) { st.Profiles.Dir = dir }, Options{Profile: "p1"})
	fp, err := fingerprint.Load(filepath.Join(dir, "p1"), "windows")
	if err != nil {
		t.Fatal(err)
	}
	run, ok := runner(t, s)
	ok(run("open", "url", "data:text/html,<title>x</title>"))
	script := "(() => { const gl = document.createElement('canvas').getContext('webgl'); return gl.getParameter(gl.getExtension('WEBGL_debug_renderer_info').UNMASKED_RENDERER_WEBGL) })()"
	if got, want := ok(run("evaluate", "script", script)), fmt.Sprintf(`{"result": %q}`, fp.WebglRenderer); got != want {
		t.Errorf("renderer: %s, want %s", got, want)
	}
}
