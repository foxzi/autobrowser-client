// Package cloak ports the parts of the Python cloakbrowser package (0.6.5) that
// autobrowser relies on: locating and downloading the stealth Chromium binary
// (config.py, download.py) and resolving the exit-IP timezone and locale
// (geoip.py). Pro/license-key downloads are not supported.
package cloak

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	// BinaryEnv overrides the browser binary path.
	BinaryEnv = "CLOAKBROWSER_BINARY_PATH"

	chromiumVersion = "146.0.7680.177.5"

	githubDownloadBase = "https://github.com/CloakHQ/cloakbrowser/releases/download"
)

// platformVersions is PLATFORM_CHROMIUM_VERSIONS.
var platformVersions = map[string]string{
	"linux-x64":    "146.0.7680.177.5",
	"linux-arm64":  "146.0.7680.177.3",
	"darwin-arm64": "145.0.7632.109.2",
	"darwin-x64":   "145.0.7632.109.2",
	"windows-x64":  "146.0.7680.177.5",
}

// versionPin is _VERSION_PIN_RE: four or five numeric components.
var versionPin = regexp.MustCompile(`^[0-9]+(\.[0-9]+){3,4}$`)

// platformTag is get_platform_tag.
func platformTag() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux-x64", nil
	case "linux/arm64":
		return "linux-arm64", nil
	case "darwin/arm64":
		return "darwin-arm64", nil
	case "darwin/amd64":
		return "darwin-x64", nil
	case "windows/amd64":
		return "windows-x64", nil
	}
	return "", fmt.Errorf("Unsupported platform: %s %s. Supported: Linux-x86_64, Linux-aarch64, "+
		"Darwin-arm64, Darwin-x86_64, Windows-AMD64, Windows-x86_64", runtime.GOOS, runtime.GOARCH)
}

// platformVersion is get_chromium_version.
func platformVersion() (string, error) {
	tag, err := platformTag()
	if err != nil {
		return "", err
	}
	return platformVersions[tag], nil
}

// CacheDir is get_cache_dir: CLOAKBROWSER_CACHE_DIR or ~/.cloakbrowser.
func CacheDir() string {
	if d := os.Getenv("CLOAKBROWSER_CACHE_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cloakbrowser")
}

func binaryDir(version string) string {
	return filepath.Join(CacheDir(), "chromium-"+version)
}

// binaryPath is get_binary_path for the free build of version.
func binaryPath(version string) string {
	dir := binaryDir(version)
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Chromium.app", "Contents", "MacOS", "Chromium")
	case "windows":
		return filepath.Join(dir, "chrome.exe")
	}
	return filepath.Join(dir, "chrome")
}

// normalizeRequestedVersion is normalize_requested_version for the
// CLOAKBROWSER_VERSION env var: "" means no pin.
func normalizeRequestedVersion() (string, error) {
	v := strings.TrimSpace(os.Getenv("CLOAKBROWSER_VERSION"))
	if v == "" {
		return "", nil
	}
	if !versionPin.MatchString(v) {
		return "", errors.New("Invalid browser version pin. Use a full numeric Chromium version, " +
			"e.g. '148.0.7778.215.2'.")
	}
	return v, nil
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

// versionNewer is _version_newer; unparsable versions are never newer.
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// effectiveVersion is get_effective_version (free tier): the newest auto-updated
// version named by a latest_version marker whose binary exists, else the
// platform default.
func effectiveVersion() (string, error) {
	base, err := platformVersion()
	if err != nil {
		return "", err
	}
	tag, _ := platformTag()
	for _, name := range []string{"latest_version_" + tag, "latest_version"} {
		data, err := os.ReadFile(filepath.Join(CacheDir(), name))
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(data)); v != "" && versionNewer(v, base) && exists(binaryPath(v)) {
			return v, nil
		}
	}
	return base, nil
}

func archiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func archiveName() (string, error) {
	tag, err := platformTag()
	if err != nil {
		return "", err
	}
	return "cloakbrowser-" + tag + archiveExt(), nil
}

// downloadBase is DOWNLOAD_BASE_URL; Python reads it at import, Go on each call.
func downloadBase() string {
	if u := os.Getenv("CLOAKBROWSER_DOWNLOAD_URL"); u != "" {
		return u
	}
	return officialBase
}

// officialBase is the primary download origin (a variable for tests).
var officialBase = "https://cloakbrowser.dev"

// githubBase is the GitHub Releases fallback origin (a variable for tests).
var githubBase = githubDownloadBase

func downloadURL(version string) (string, error) {
	name, err := archiveName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/chromium-v%s/%s", downloadBase(), version, name), nil
}

func fallbackDownloadURL(version string) (string, error) {
	name, err := archiveName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/chromium-v%s/%s", githubBase, version, name), nil
}
