// Package browser launches the cloakbrowser Chromium over CDP (go-rod) and
// runs scenario steps on it. It ports browser/service.py (cloakbrowser branch),
// browser/executor.py and browser/actions.py.
package browser

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
)

// BinaryEnv overrides the browser binary path (like cloakbrowser).
const BinaryEnv = "CLOAKBROWSER_BINARY_PATH"

const chromiumVersion = "146.0.7680.177.5"

var versionPin = regexp.MustCompile(`^\d+(\.\d+)+$`)

// platformTag and the per-platform Chromium version follow cloakbrowser 0.6.5.
func platformTag() (tag, version string, ok bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux-x64", chromiumVersion, true
	case "linux/arm64":
		return "linux-arm64", "146.0.7680.177.3", true
	case "darwin/arm64":
		return "darwin-arm64", chromiumVersion, true
	case "darwin/amd64":
		return "darwin-x64", chromiumVersion, true
	case "windows/amd64":
		return "windows-x64", chromiumVersion, true
	}
	return "", "", false
}

func cacheDir() string {
	if d := os.Getenv("CLOAKBROWSER_CACHE_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cloakbrowser")
}

func binaryPath(version string) string {
	dir := filepath.Join(cacheDir(), "chromium-"+version)
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Chromium.app", "Contents", "MacOS", "Chromium")
	case "windows":
		return filepath.Join(dir, "chrome.exe")
	}
	return filepath.Join(dir, "chrome")
}

func versionTuple(v string) ([]int, bool) {
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func versionNewer(a, b string) bool {
	x, ok1 := versionTuple(a)
	y, ok2 := versionTuple(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return len(x) > len(y)
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0)
}

// FindBinary is cloakbrowser ensure_binary without the download: the
// CLOAKBROWSER_BINARY_PATH override, a CLOAKBROWSER_VERSION pin, the newest
// auto-updated build named by a latest_version marker, then the built-in version.
func FindBinary() (string, error) {
	if p := os.Getenv(BinaryEnv); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", apperr.New(apperr.BrowserStartFailed,
				fmt.Sprintf("%s points to a missing file: %s", BinaryEnv, p))
		}
		return p, nil
	}
	tag, base, ok := platformTag()
	if !ok {
		return "", apperr.New(apperr.BrowserStartFailed,
			fmt.Sprintf("cloakbrowser has no browser build for %s/%s", runtime.GOOS, runtime.GOARCH))
	}
	var tried []string
	try := func(version string) (string, bool) {
		p := binaryPath(version)
		tried = append(tried, p)
		return p, isExecutable(p)
	}
	if pin := strings.TrimSpace(os.Getenv("CLOAKBROWSER_VERSION")); pin != "" {
		if !versionPin.MatchString(pin) {
			return "", apperr.New(apperr.BrowserStartFailed,
				"Invalid CLOAKBROWSER_VERSION pin. Use a full numeric Chromium version, e.g. '148.0.7778.215.2'.")
		}
		if p, ok := try(pin); ok {
			return p, nil
		}
	} else {
		for _, name := range []string{"latest_version_" + tag, "latest_version"} {
			data, err := os.ReadFile(filepath.Join(cacheDir(), name))
			if v := strings.TrimSpace(string(data)); err == nil && v != "" && versionNewer(v, base) {
				if p := binaryPath(v); fileExists(p) {
					tried = append(tried, p)
					if isExecutable(p) {
						return p, nil
					}
				}
			}
		}
		if p, ok := try(base); ok {
			return p, nil
		}
	}
	return "", apperr.New(apperr.BrowserStartFailed, fmt.Sprintf(
		"cloakbrowser binary not found (looked at: %s). The Go version does not download the browser: "+
			"run `autobrowser install` or the Python cloakbrowser once to fetch it, "+
			"or set %s to the browser executable",
		strings.Join(tried, ", "), BinaryEnv))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// IgnoreDefaultArgs are the Playwright defaults cloakbrowser drops. Both are
// absent from the argument list built here, so nothing needs removing.
var IgnoreDefaultArgs = []string{"--enable-automation", "--enable-unsafe-swiftshader"}

// BuildArgs is cloakbrowser build_args(stealth_args=True, extra_args=extra,
// headless=headless): defaults < user args, deduplicated by the text before "=".
// The --fingerprint seed is random per call.
func BuildArgs(extra []string, headless bool) []string {
	var keys []string
	seen := map[string]string{}
	set := func(arg string) {
		key, _, _ := strings.Cut(arg, "=")
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
		seen[key] = arg
	}
	fpPlatform := "windows"
	if runtime.GOOS == "darwin" {
		fpPlatform = "macos"
	}
	set("--no-sandbox")
	set("--fingerprint=" + strconv.Itoa(10000+rand.Intn(90000)))
	set("--fingerprint-platform=" + fpPlatform)
	if !headless || runtime.GOOS == "windows" {
		set("--ignore-gpu-blocklist")
	}
	for _, a := range extra {
		set(a)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}
