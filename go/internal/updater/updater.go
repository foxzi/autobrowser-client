// Package updater ports autobrowser_client/updater.py: self-update of the
// binary from the GitHub releases of the project. The Go binary uses its own
// assets, autobrowser-go-<os>-<arch>[.exe] with a .sha256 sibling.
package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RepoURL is the GitHub repository whose releases are used.
var RepoURL = "https://github.com/foxzi/autobrowser-client"

const checkInterval = 24 * time.Hour

var (
	executable = os.Executable
	// download returns the body of a URL; the response of a failed status is an error.
	download = func(url string, timeout time.Duration) ([]byte, error) {
		resp, err := (&http.Client{Timeout: timeout}).Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP Error %d: %s (%s)", resp.StatusCode, http.StatusText(resp.StatusCode), url)
		}
		return io.ReadAll(resp.Body)
	}
)

func parse(version string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(strings.TrimLeft(version, "v"), ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid literal for int() with base 10: '%s'", p)
		}
		out = append(out, n)
	}
	return out, nil
}

// AssetName is the release asset of this platform, e.g. autobrowser-go-linux-arm64.
func AssetName() (string, error) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("Unsupported platform: %s", runtime.GOOS)
	}
	arch := "amd64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	name := "autobrowser-go-" + runtime.GOOS + "-" + arch
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name, nil
}

// Latest is latest_version: /releases/latest redirects to /releases/tag/vX.Y.Z,
// which needs no API token and has no rate limit.
func Latest() (string, error) {
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(RepoURL + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	u := strings.TrimRight(resp.Request.URL.String(), "/")
	return strings.TrimLeft(u[strings.LastIndex(u, "/")+1:], "v"), nil
}

// IsNewer reports whether version is newer than current.
func IsNewer(current, version string) (bool, error) {
	v, err := parse(version)
	if err != nil {
		return false, err
	}
	c, err := parse(current)
	if err != nil {
		return false, err
	}
	for i := 0; i < max(len(v), len(c)); i++ {
		if i >= len(v) {
			return false, nil
		}
		if i >= len(c) {
			return true, nil
		}
		if v[i] != c[i] {
			return v[i] > c[i], nil
		}
	}
	return false, nil
}

// Install is install_version: downloads the release binary, verifies its
// sha256 and replaces the running executable. It returns the executable path.
func Install(version string) (string, error) {
	name, err := AssetName()
	if err != nil {
		return "", err
	}
	base := fmt.Sprintf("%s/releases/download/v%s/%s", RepoURL, version, name)
	data, err := download(base, 300*time.Second)
	if err != nil {
		return "", err
	}
	sum, err := download(base+".sha256", 300*time.Second)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	if fields := strings.Fields(string(sum)); len(fields) == 0 || hex.EncodeToString(digest[:]) != fields[0] {
		return "", fmt.Errorf("Checksum mismatch for %s v%s", name, version)
	}

	exe, err := executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		// A running exe cannot be overwritten on Windows, but it can be renamed
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, exe); err != nil {
		return "", err
	}
	return exe, nil
}

// CleanupOld removes the .old binary left by a previous update on Windows.
func CleanupOld() {
	if exe, err := executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			_ = os.Remove(exe + ".old")
		}
	}
}

func stampFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "autobrowser", "last-update-check")
}

func autoUpdate(current string) {
	// never break the running command
	stamp := stampFile()
	if err := os.MkdirAll(filepath.Dir(stamp), 0o777); err == nil {
		_ = os.WriteFile(stamp, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o666)
	}
	version, err := Latest()
	if err == nil {
		var newer bool
		if newer, err = IsNewer(current, version); err == nil && newer {
			var path string
			if path, err = Install(version); err == nil {
				slog.Info(fmt.Sprintf("Updated %s to v%s; restart to use it", path, version))
			}
		}
	}
	if err != nil {
		slog.Warn(fmt.Sprintf("Auto-update failed: %s", err))
	}
}

// StartAutoUpdate checks for a new release at most once a day in a background
// goroutine; disabled by AUTOBROWSER_NO_AUTO_UPDATE=1.
func StartAutoUpdate(current string) {
	CleanupOld()
	if os.Getenv("AUTOBROWSER_NO_AUTO_UPDATE") == "1" {
		return
	}
	var last int64
	if data, err := os.ReadFile(stampFile()); err == nil {
		last, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}
	if time.Since(time.Unix(last, 0)) < checkInterval {
		return
	}
	go autoUpdate(current)
}
