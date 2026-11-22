package cloak

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
)

const (
	geoipDBFilename       = "GeoLite2-City.mmdb"
	geoipUpdateInterval   = 30 * 24 * time.Hour
	defaultGeoipTimeout   = 20.0
	geoipTimeoutEnv       = "CLOAKBROWSER_GEOIP_TIMEOUT_SECONDS"
	echoRequestTimeout    = 10 * time.Second
	geoipDownloadIdleWait = 300 * time.Second
)

// geoipDBURL is the P3TERX mirror of MaxMind GeoLite2-City, no license key needed
// (a variable for tests).
var geoipDBURL = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb"

// ipEchoURLs are the exit-IP echo services, tried in order.
var ipEchoURLs = []string{
	"https://api.ipify.org",
	"https://checkip.amazonaws.com",
	"https://ifconfig.me/ip",
}

// geoipDownloadLock serializes database downloads within the process.
var geoipDownloadLock sync.Mutex

// countryLocale is COUNTRY_LOCALE_MAP: country ISO code to BCP 47 locale.
var countryLocale = map[string]string{
	"US": "en-US", "GB": "en-GB", "AU": "en-AU", "CA": "en-CA", "NZ": "en-NZ",
	"IE": "en-IE", "ZA": "en-ZA", "SG": "en-SG",
	"DE": "de-DE", "AT": "de-AT", "CH": "de-CH",
	"FR": "fr-FR", "BE": "fr-BE",
	"ES": "es-ES", "MX": "es-MX", "AR": "es-AR", "CO": "es-CO", "CL": "es-CL",
	"BR": "pt-BR", "PT": "pt-PT",
	"IT": "it-IT", "NL": "nl-NL",
	"JP": "ja-JP", "KR": "ko-KR", "CN": "zh-CN", "TW": "zh-TW", "HK": "zh-HK",
	"RU": "ru-RU", "UA": "uk-UA", "PL": "pl-PL", "CZ": "cs-CZ", "RO": "ro-RO",
	"IL": "he-IL", "TR": "tr-TR", "SA": "ar-SA", "AE": "ar-AE", "EG": "ar-EG",
	"IN": "hi-IN", "ID": "id-ID", "PH": "en-PH",
	"TH": "th-TH", "VN": "vi-VN", "MY": "ms-MY",
	"SE": "sv-SE", "NO": "nb-NO", "DK": "da-DK", "FI": "fi-FI",
	"GR": "el-GR", "HU": "hu-HU", "BG": "bg-BG",
	// Extended coverage — common residential/mobile proxy exits
	"SI": "sl-SI", "SK": "sk-SK", "HR": "hr-HR", "RS": "sr-RS", "LT": "lt-LT",
	"LV": "lv-LV", "EE": "et-EE", "IS": "is-IS", "LU": "fr-LU", "MT": "en-MT",
	"CY": "el-CY", "MD": "ro-MD", "BY": "ru-BY", "GE": "ka-GE", "AL": "sq-AL",
	"MK": "mk-MK", "BA": "bs-BA",
	"PE": "es-PE", "VE": "es-VE", "EC": "es-EC", "UY": "es-UY", "CR": "es-CR",
	"DO": "es-DO", "GT": "es-GT", "BO": "es-BO", "PY": "es-PY",
	"PK": "en-PK", "BD": "bn-BD", "LK": "si-LK", "KZ": "ru-KZ", "IR": "fa-IR",
	"IQ": "ar-IQ", "JO": "ar-JO", "LB": "ar-LB", "KW": "ar-KW", "QA": "ar-QA",
	"OM": "ar-OM", "BH": "ar-BH",
	"NG": "en-NG", "KE": "en-KE", "MA": "fr-MA", "DZ": "ar-DZ", "TN": "ar-TN",
	"GH": "en-GH",
	"AM": "hy-AM", "AZ": "az-AZ", "UZ": "uz-UZ", "KG": "ky-KG", "TJ": "tg-TJ",
	"TM": "tk-TM",
	"ME": "sr-ME", "XK": "sq-XK", "LI": "de-LI", "MC": "fr-MC", "AD": "ca-AD",
	"MM": "my-MM", "KH": "km-KH", "LA": "lo-LA", "MN": "mn-MN", "BN": "ms-BN",
	"MO": "zh-MO",
	"YE": "ar-YE", "SY": "ar-SY", "PS": "ar-PS", "LY": "ar-LY",
	"ET": "am-ET", "TZ": "sw-TZ", "UG": "en-UG", "SN": "fr-SN", "CI": "fr-CI",
	"CM": "fr-CM", "AO": "pt-AO", "MZ": "pt-MZ", "ZM": "en-ZM", "ZW": "en-ZW",
	"HN": "es-HN", "NI": "es-NI", "SV": "es-SV", "PA": "es-PA", "JM": "en-JM",
	"TT": "en-TT", "PR": "es-PR",
	"SC": "en-SC",
}

// ResolveProxyGeo is cloakbrowser.geoip.resolve_proxy_geo: the timezone and
// locale of the exit IP seen through proxyURL, or of the machine's own public IP
// when proxyURL is empty. The timezone is "" when the database has none.
func ResolveProxyGeo(proxyURL string) (timezone, locale string, err error) {
	// Ensure the DB first: the download must not be bounded by the resolution
	// timeout (a first-use ~70 MB fetch legitimately outlasts it).
	dbPath := ensureGeoipDB()

	timeout := geoipTimeoutSeconds()
	var deadline time.Time // zero: no timeout
	if timeout > 0 {
		deadline = time.Now().Add(time.Duration(timeout * float64(time.Second)))
	}
	expired := func() bool { return !deadline.IsZero() && !time.Now().Before(deadline) }

	ip := resolveExitIP(proxyURL, deadline)
	// Hostname fallback only applies to a proxy. A private address (a local
	// forwarding proxy, a LAN gateway) is never the exit.
	if ip == "" && proxyURL != "" && !expired() {
		ip = resolveProxyIP(proxyURL)
		if ip != "" && isPrivateIP(ip) {
			slog.Debug(fmt.Sprintf("Proxy host resolves to private address %s, not using it as the exit", ip))
			ip = ""
		}
	}
	if ip == "" || expired() {
		if !deadline.IsZero() && expired() {
			return "", "", fmt.Errorf("GeoIP resolution timed out after %.1fs", timeout)
		}
		return "", "", errors.New("GeoIP resolution failed: could not discover the egress IP")
	}

	if dbPath == "" {
		return "", "", errors.New("GeoIP resolution failed: GeoIP database is unavailable")
	}

	timezone, country, err := lookupCity(dbPath, ip)
	if err != nil {
		return "", "", fmt.Errorf("GeoIP lookup failed for %s: %s", ip, err)
	}
	locale = countryLocale[country]
	if locale == "" {
		// Map gap, not a lookup failure: do not abort the launch over it.
		slog.Warn(fmt.Sprintf("GeoIP: no locale for country %s (IP %s), using en-US", pyStr(country), ip))
		locale = "en-US"
	}
	slog.Debug(fmt.Sprintf("GeoIP: %s -> tz=%s, country=%s, locale=%s", ip, pyStr(timezone), pyStr(country), locale))
	return timezone, locale, nil
}

// pyStr formats an empty value like Python's None in log lines.
func pyStr(s string) string {
	if s == "" {
		return "None"
	}
	return s
}

// resolveGeo is a variable so tests can replace the network lookup.
var resolveGeo = ResolveProxyGeo

// ResolveGeoIP is browser.service._resolve_geoip: the timezone and locale of the
// exit IP (through proxyURL, or the own IP when empty); failures are start errors.
func ResolveGeoIP(proxyURL string) (timezone, locale string, err error) {
	timezone, locale, err = resolveGeo(proxyURL)
	if err != nil {
		return "", "", apperr.New(apperr.BrowserStartFailed, fmt.Sprintf("browser.geoip failed: %s", err))
	}
	slog.Info(fmt.Sprintf("GeoIP: timezone=%s locale=%s", pyStr(timezone), locale))
	return timezone, locale, nil
}

// lookupCity returns the time zone and country ISO code of ip.
func lookupCity(dbPath, ip string) (timezone, country string, err error) {
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", "", err
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Location struct {
			TimeZone string `maxminddb:"time_zone"`
		} `maxminddb:"location"`
	}
	network, ok, err := db.LookupNetwork(net.IP(addr.AsSlice()), &rec)
	if err != nil {
		return "", "", err
	}
	if !ok || network == nil {
		return "", "", fmt.Errorf("The address %s is not in the database.", ip)
	}
	return rec.Location.TimeZone, rec.Country.ISOCode, nil
}

// resolveProxyIP extracts the proxy host from the URL and resolves it to an IP.
func resolveProxyIP(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	if _, err := netip.ParseAddr(host); err == nil {
		return host
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		slog.Warn(fmt.Sprintf("Failed to resolve proxy hostname: %s", err))
		return ""
	}
	if len(addrs) == 0 {
		return ""
	}
	slog.Debug(fmt.Sprintf("Resolved proxy %s -> %s", host, addrs[0]))
	return addrs[0]
}

// notGlobal lists the ranges Python's ipaddress treats as not globally
// reachable (is_private plus the CGNAT 100.64/10 block).
var notGlobal = func() []netip.Prefix {
	var out []netip.Prefix
	for _, c := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/29", "192.0.0.170/31", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
		"::1/128", "::/128", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "fc00::/7", "fe80::/10",
	} {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}()

// isPrivateIP is _is_private_ip: not is_global (is_private misses CGNAT, e.g. Tailscale).
func isPrivateIP(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	for _, p := range notGlobal {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// geoipTimeoutSeconds is _get_geoip_timeout_seconds: 20 s by default, 0 means none.
func geoipTimeoutSeconds() float64 {
	raw := os.Getenv(geoipTimeoutEnv)
	if raw == "" {
		return defaultGeoipTimeout
	}
	t, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(t) || math.IsInf(t, 0) {
		slog.Warn(fmt.Sprintf("Invalid %s='%s'; using %.1fs", geoipTimeoutEnv, raw, defaultGeoipTimeout))
		return defaultGeoipTimeout
	}
	return math.Max(t, 0)
}

// resolveExitIP is _resolve_exit_ip: the egress IP via the echo services,
// through proxyURL when given, otherwise directly. It returns "" when none
// answers or the deadline (zero: none) passes.
func resolveExitIP(proxyURL string, deadline time.Time) string {
	client := httpClient(0)
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			slog.Warn("Failed to discover exit IP through proxy")
			return ""
		}
		client.Transport.(*http.Transport).Proxy = http.ProxyURL(pu)
	}
	// httpx.get does not follow redirects: any non-2xx answer is a failure.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, u := range ipEchoURLs {
		timeout := echoRequestTimeout
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ""
			}
			timeout = min(timeout, remaining)
		}
		client.Timeout = timeout
		if ip, ok := fetchIP(client, u); ok {
			slog.Debug(fmt.Sprintf("Exit IP via %s: %s", u, ip))
			return ip
		}
	}
	slog.Warn("Failed to discover exit IP through proxy")
	return ""
}

func fetchIP(client *http.Client, u string) (string, bool) {
	resp, err := client.Get(u)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if err != nil {
		return "", false
	}
	ip := strings.TrimSpace(string(body))
	if addr, err := netip.ParseAddr(ip); err != nil || addr.Zone() != "" {
		return "", false
	}
	return ip, true
}

func geoipDBPath() string {
	return filepath.Join(CacheDir(), "geoip", geoipDBFilename)
}

// ensureGeoipDB returns the path of GeoLite2-City.mmdb, downloading it on first
// use; "" when the download failed.
func ensureGeoipDB() string {
	path := geoipDBPath()
	if exists(path) {
		maybeTriggerGeoipUpdate(path)
		return path
	}
	// Only one launch fetches the shared file, the rest wait and reuse it.
	geoipDownloadLock.Lock()
	defer geoipDownloadLock.Unlock()
	if exists(path) {
		return path
	}
	if err := downloadGeoipDB(path); err != nil {
		slog.Warn(fmt.Sprintf("Failed to download GeoIP database: %s", err))
		return ""
	}
	return path
}

// downloadGeoipDB is _download_geoip_db: an atomic download of the database.
func downloadGeoipDB(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	info("Downloading GeoIP database (~70 MB) ...")
	tmp, err := os.CreateTemp(filepath.Dir(dest), "*.tmp")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err := fetchToFile(geoipDBURL, tmp.Name(), geoipDownloadIdleWait, func(pct, _, _ int64) {
		info("GeoIP download: %d %%", pct)
	}); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return err
	}
	info("GeoIP database ready: %s", dest)
	return nil
}

// maybeTriggerGeoipUpdate re-downloads the database in the background when it
// is older than 30 days, unless a download is already running.
func maybeTriggerGeoipUpdate(path string) {
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) < geoipUpdateInterval {
		return
	}
	go func() {
		if !geoipDownloadLock.TryLock() {
			return
		}
		defer geoipDownloadLock.Unlock()
		if err := downloadGeoipDB(path); err != nil {
			slog.Debug("Background GeoIP update failed: " + err.Error())
		}
	}()
}
