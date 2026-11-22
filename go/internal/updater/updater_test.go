package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	for v, want := range map[string]bool{"0.10.0": true, "v0.3.1": true, "0.3.0": false, "0.2.9": false, "0.3": false, "0.3.0.1": true} {
		if got, err := IsNewer("0.3.0", v); err != nil || got != want {
			t.Errorf("IsNewer(0.3.0, %s) = %v, %v", v, got, err)
		}
	}
	if _, err := IsNewer("0.3.0", "0.4.0-rc1"); err == nil {
		t.Error("non numeric version accepted")
	}
}

func TestAssetName(t *testing.T) {
	name, err := AssetName()
	want := "autobrowser-go-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if err != nil || name != want {
		t.Errorf("%q %v, want %q", name, err, want)
	}
}

// fakeRelease serves data and a sha256 file for any download URL and points
// the updater at a temp executable.
func fakeRelease(t *testing.T, data []byte, digest string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "autobrowser")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldDownload := executable, download
	t.Cleanup(func() { executable, download = oldExe, oldDownload })
	executable = func() (string, error) { return exe, nil }
	download = func(url string, _ time.Duration) ([]byte, error) {
		if strings.HasSuffix(url, ".sha256") {
			return []byte(digest + "  autobrowser\n"), nil
		}
		if !strings.Contains(url, "/releases/download/v9.9.9/autobrowser-go-") {
			t.Errorf("unexpected url %s", url)
		}
		return data, nil
	}
	return exe
}

func TestInstallReplacesBinary(t *testing.T) {
	sum := sha256.Sum256([]byte("new"))
	exe := fakeRelease(t, []byte("new"), hex.EncodeToString(sum[:]))
	got, err := Install("9.9.9")
	want, _ := filepath.EvalSymlinks(exe)
	if err != nil || got != want {
		t.Fatalf("%q %v, want %q", got, err, want)
	}
	if data, _ := os.ReadFile(exe); string(data) != "new" {
		t.Errorf("binary: %q", data)
	}
	if st, _ := os.Stat(exe); runtime.GOOS != "windows" && st.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", st.Mode())
	}
}

func TestInstallRejectsBadChecksum(t *testing.T) {
	exe := fakeRelease(t, []byte("new"), strings.Repeat("0", 64))
	if _, err := Install("9.9.9"); err == nil || !strings.Contains(err.Error(), "Checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(exe); string(data) != "old" {
		t.Errorf("binary replaced: %q", data)
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error(".new left behind")
	}
}

func TestLatestFollowsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v1.2.3", http.StatusFound)
			return
		}
		fmt.Fprint(w, "page")
	}))
	defer srv.Close()
	old := RepoURL
	RepoURL = srv.URL
	defer func() { RepoURL = old }()
	if v, err := Latest(); err != nil || v != "1.2.3" {
		t.Errorf("%q %v", v, err)
	}
}

func TestDownloadStatusError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := download(srv.URL+"/x", time.Second); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
}

func TestStartAutoUpdateRespectsOptOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AUTOBROWSER_NO_AUTO_UPDATE", "1")
	StartAutoUpdate("0.0.1")
	if _, err := os.Stat(stampFile()); err == nil {
		t.Error("stamp written although auto-update is disabled")
	}
	// checked less than a day ago: no goroutine, stamp untouched
	t.Setenv("AUTOBROWSER_NO_AUTO_UPDATE", "")
	os.MkdirAll(filepath.Dir(stampFile()), 0o755)
	stamp := fmt.Sprint(time.Now().Unix())
	os.WriteFile(stampFile(), []byte(stamp), 0o644)
	StartAutoUpdate("0.0.1")
	time.Sleep(50 * time.Millisecond)
	if data, _ := os.ReadFile(stampFile()); string(data) != stamp {
		t.Errorf("stamp changed: %s", data)
	}
}
