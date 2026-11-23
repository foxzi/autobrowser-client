package cloak

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
)

// mmdbEncode helpers: just enough of the MaxMind DB data format for string maps.
func mmdbString(s string) []byte {
	return append([]byte{byte(2<<5 | len(s))}, s...)
}

func mmdbMap(kv ...string) []byte {
	out := []byte{byte(7<<5 | len(kv)/2)}
	for _, s := range kv {
		out = append(out, mmdbString(s)...)
	}
	return out
}

func mmdbUint(typ int, v uint64) []byte {
	var b []byte
	for ; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	return append([]byte{byte(typ<<5 | len(b))}, b...)
}

// mmdbRecord is a city record: a country ISO code and a time zone.
type mmdbRecord struct{ country, tz string }

func (r mmdbRecord) encode() []byte {
	var kv []string
	var out []byte
	if r.country != "" {
		out = append(out, mmdbString("country")...)
		out = append(out, mmdbMap("iso_code", r.country)...)
		kv = append(kv, "country")
	}
	if r.tz != "" {
		out = append(out, mmdbString("location")...)
		out = append(out, mmdbMap("time_zone", r.tz)...)
		kv = append(kv, "location")
	}
	return append([]byte{byte(7<<5 | len(kv))}, out...)
}

// writeMMDB writes a small IPv4 database with the given /24 networks ("a.b.c" -> record).
func writeMMDB(t *testing.T, path string, nets map[[3]byte]mmdbRecord) {
	t.Helper()
	type node [2]int // -1 empty, otherwise node index (>= 0) or -2-dataOffset
	nodes := []node{{-1, -1}}
	var data []byte
	for prefix, rec := range nets {
		off := len(data)
		data = append(data, rec.encode()...)
		cur := 0
		for i := 0; i < 24; i++ {
			bit := int(prefix[i/8]>>(7-i%8)) & 1
			if i == 23 {
				nodes[cur][bit] = -2 - off
				break
			}
			if nodes[cur][bit] < 0 {
				nodes = append(nodes, node{-1, -1})
				nodes[cur][bit] = len(nodes) - 1
			}
			cur = nodes[cur][bit]
		}
	}
	count := len(nodes)
	rec := func(v int) []byte {
		switch {
		case v == -1:
			v = count
		case v < -1:
			v = count + 16 + (-2 - v)
		}
		return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		buf.Write(rec(n[0]))
		buf.Write(rec(n[1]))
	}
	buf.Write(make([]byte, 16))
	buf.Write(data)
	buf.WriteString("\xab\xcd\xefMaxMind.com")
	buf.Write(mmdbMapRaw(
		"binary_format_major_version", mmdbUint(5, 2),
		"binary_format_minor_version", mmdbUint(5, 0),
		"build_epoch", append([]byte{byte(0<<5 | 1), 2}, 1),
		"database_type", mmdbString("GeoLite2-City"),
		"ip_version", mmdbUint(5, 4),
		"node_count", mmdbUint(6, uint64(count)),
		"record_size", mmdbUint(5, 24),
	))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mmdbMapRaw encodes a map from alternating keys and already encoded values.
func mmdbMapRaw(kv ...any) []byte {
	out := []byte{byte(7<<5 | len(kv)/2)}
	for i := 0; i < len(kv); i += 2 {
		out = append(out, mmdbString(kv[i].(string))...)
		out = append(out, kv[i+1].([]byte)...)
	}
	return out
}

func testDB(t *testing.T) {
	t.Helper()
	writeMMDB(t, geoipDBPath(), map[[3]byte]mmdbRecord{
		{81, 2, 69}:    {"GB", "Europe/London"},
		{91, 198, 174}: {"FI", "Europe/Helsinki"},
		{203, 0, 113}:  {"ZZ", "Etc/UTC"},
		{198, 51, 100}: {"", ""},
	})
}

func echoServer(t *testing.T, body string, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		fmt.Fprintln(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func useEcho(t *testing.T, urls ...string) {
	t.Helper()
	old := ipEchoURLs
	ipEchoURLs = urls
	t.Cleanup(func() { ipEchoURLs = old })
}

func TestResolveProxyGeoDirect(t *testing.T) {
	isolate(t)
	testDB(t)
	bad := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(bad.Close)
	junk := echoServer(t, "not an ip", 0)
	ok := echoServer(t, "91.198.174.5", 0)
	useEcho(t, bad.URL, junk.URL, ok.URL) // the first two services must be skipped

	tz, locale, err := ResolveProxyGeo("")
	if err != nil || tz != "Europe/Helsinki" || locale != "fi-FI" {
		t.Fatalf("got %q, %q, %v", tz, locale, err)
	}
}

func TestResolveProxyGeoThroughProxy(t *testing.T) {
	isolate(t)
	testDB(t)
	// An HTTP proxy that answers every request with the exit IP.
	proxy := echoServer(t, "81.2.69.10", 0)
	useEcho(t, "http://echo.invalid/ip")

	tz, locale, err := ResolveProxyGeo(proxy.URL)
	if err != nil || tz != "Europe/London" || locale != "en-GB" {
		t.Fatalf("got %q, %q, %v", tz, locale, err)
	}
}

func TestResolveProxyGeoUnknownCountryUsesEnUS(t *testing.T) {
	isolate(t)
	testDB(t)
	useEcho(t, echoServer(t, "203.0.113.7", 0).URL)
	tz, locale, err := ResolveProxyGeo("")
	if err != nil || tz != "Etc/UTC" || locale != "en-US" {
		t.Fatalf("got %q, %q, %v", tz, locale, err)
	}
	// no country and no time zone in the record
	useEcho(t, echoServer(t, "198.51.100.1", 0).URL)
	tz, locale, err = ResolveProxyGeo("")
	if err != nil || tz != "" || locale != "en-US" {
		t.Fatalf("empty record: got %q, %q, %v", tz, locale, err)
	}
}

func TestResolveProxyGeoErrors(t *testing.T) {
	isolate(t)
	testDB(t)
	bad := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(bad.Close)

	useEcho(t, bad.URL)
	if _, _, err := ResolveProxyGeo(""); err == nil ||
		err.Error() != "GeoIP resolution failed: could not discover the egress IP" {
		t.Fatalf("no echo service: %v", err)
	}
	// a proxy on a private address is never used as the exit
	if _, _, err := ResolveProxyGeo("http://127.0.0.1:1"); err == nil ||
		!strings.Contains(err.Error(), "could not discover the egress IP") {
		t.Fatalf("private proxy host: %v", err)
	}

	useEcho(t, echoServer(t, "9.9.9.9", 0).URL)
	if _, _, err := ResolveProxyGeo(""); err == nil ||
		err.Error() != "GeoIP lookup failed for 9.9.9.9: The address 9.9.9.9 is not in the database." {
		t.Fatalf("unknown address: %v", err)
	}

	t.Setenv(geoipTimeoutEnv, "0.3")
	useEcho(t, echoServer(t, "81.2.69.1", 2*time.Second).URL)
	start := time.Now()
	if _, _, err := ResolveProxyGeo(""); err == nil || err.Error() != "GeoIP resolution timed out after 0.3s" {
		t.Fatalf("timeout: %v", err)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("timeout took %s", time.Since(start))
	}
}

func TestResolveProxyGeoDatabaseUnavailable(t *testing.T) {
	isolate(t)
	oldURL := geoipDBURL
	geoipDBURL = "http://127.0.0.1:1/GeoLite2-City.mmdb"
	t.Cleanup(func() { geoipDBURL = oldURL })
	useEcho(t, echoServer(t, "81.2.69.1", 0).URL)
	if _, _, err := ResolveProxyGeo(""); err == nil ||
		err.Error() != "GeoIP resolution failed: GeoIP database is unavailable" {
		t.Fatalf("got %v", err)
	}
}

func TestEnsureGeoipDBDownloads(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "src.mmdb")
	writeMMDB(t, src, map[[3]byte]mmdbRecord{{81, 2, 69}: {"GB", "Europe/London"}})
	data, _ := os.ReadFile(src)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	oldURL := geoipDBURL
	geoipDBURL = srv.URL
	t.Cleanup(func() { geoipDBURL = oldURL })

	want := filepath.Join(CacheDir(), "geoip", "GeoLite2-City.mmdb")
	if got := ensureGeoipDB(); got != want {
		t.Fatalf("ensureGeoipDB = %q, want %q", got, want)
	}
	if got := ensureGeoipDB(); got != want || hits != 1 {
		t.Fatalf("second call: %q, %d downloads", got, hits)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(want), "*.tmp")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
	if _, _, err := lookupCity(want, "81.2.69.1"); err != nil {
		t.Fatal(err)
	}
}

func TestStaleGeoipDBIsRefreshedInBackground(t *testing.T) {
	isolate(t)
	testDB(t)
	path := geoipDBPath()
	old := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(path, old, old)
	fresh := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case fresh <- struct{}{}:
		default:
		}
		w.Write([]byte("new"))
	}))
	t.Cleanup(srv.Close)
	oldURL := geoipDBURL
	geoipDBURL = srv.URL
	t.Cleanup(func() { geoipDBURL = oldURL })

	if got := ensureGeoipDB(); got != path {
		t.Fatalf("stale DB must still be returned: %q", got)
	}
	select {
	case <-fresh:
	case <-time.After(3 * time.Second):
		t.Fatal("no background refresh")
	}
	// wait for the background download to finish before the temp dir is removed
	geoipDownloadLock.Lock()
	geoipDownloadLock.Unlock()
}

func TestGeoipTimeoutSeconds(t *testing.T) {
	isolate(t)
	for env, want := range map[string]float64{"": 20, "5": 5, "0.5": 0.5, "0": 0, "-3": 0, "abc": 20, "nan": 20, "inf": 20} {
		t.Setenv(geoipTimeoutEnv, env)
		if got := geoipTimeoutSeconds(); got != want {
			t.Errorf("%s=%q: got %v, want %v", geoipTimeoutEnv, env, got, want)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"10.1.2.3": true, "192.168.0.1": true, "172.16.5.5": true, "127.0.0.1": true, "169.254.1.1": true,
		"100.64.0.1": true, "::1": true, "fc00::1": true, "fe80::1": true, "::ffff:10.0.0.1": true,
		"8.8.8.8": false, "81.2.69.1": false, "2606:4700::1": false, "100.128.0.1": false, "bogus": false,
	} {
		if got := isPrivateIP(ip); got != want {
			t.Errorf("isPrivateIP(%q) = %v, want %v", ip, got, want)
		}
	}
}

func TestResolveProxyIP(t *testing.T) {
	for in, want := range map[string]string{
		"socks5://u:p@1.2.3.4:1080": "1.2.3.4",
		"http://[2001:db8::1]:3128": "2001:db8::1",
		"http://localhost:8080":     "",
		"not a url":                 "",
		"http://":                   "",
	} {
		got := resolveProxyIP(in)
		if in == "http://localhost:8080" {
			if got != "127.0.0.1" && got != "::1" {
				t.Errorf("resolveProxyIP(%q) = %q", in, got)
			}
			continue
		}
		if got != want {
			t.Errorf("resolveProxyIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountryLocaleMap(t *testing.T) {
	if len(countryLocale) != 133 {
		t.Fatalf("map has %d entries, Python has 133", len(countryLocale))
	}
	for cc, want := range map[string]string{"US": "en-US", "DE": "de-DE", "RU": "ru-RU", "FI": "fi-FI", "SC": "en-SC", "KZ": "ru-KZ"} {
		if countryLocale[cc] != want {
			t.Errorf("%s = %q, want %q", cc, countryLocale[cc], want)
		}
	}
}

// Ported from python/tests/test_geoip.py.
func TestResolveGeoIPPassesProxy(t *testing.T) {
	var calls []string
	old := resolveGeo
	resolveGeo = func(p string) (string, string, error) {
		calls = append(calls, p)
		return "Europe/Helsinki", "fi-FI", nil
	}
	t.Cleanup(func() { resolveGeo = old })
	tz, locale, err := ResolveGeoIP("http://127.0.0.1:9")
	if err != nil || tz != "Europe/Helsinki" || locale != "fi-FI" {
		t.Fatalf("got %q, %q, %v", tz, locale, err)
	}
	if len(calls) != 1 || calls[0] != "http://127.0.0.1:9" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestResolveGeoIPFailureIsStartError(t *testing.T) {
	old := resolveGeo
	resolveGeo = func(string) (string, string, error) {
		return "", "", fmt.Errorf("could not discover the egress IP")
	}
	t.Cleanup(func() { resolveGeo = old })
	_, _, err := ResolveGeoIP("")
	ae, ok := err.(*apperr.Error)
	if !ok || ae.Code != apperr.BrowserStartFailed || !strings.Contains(ae.Message, "egress IP") ||
		!strings.HasPrefix(ae.Message, "browser.geoip failed: ") {
		t.Fatalf("error = %#v", err)
	}
}
