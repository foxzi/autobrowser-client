package cloak

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolate points the cloakbrowser env at an empty cache and returns it.
func isolate(t *testing.T) string {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("CLOAKBROWSER_CACHE_DIR", cache)
	for _, k := range []string{BinaryEnv, "CLOAKBROWSER_VERSION", "CLOAKBROWSER_DOWNLOAD_URL",
		"CLOAKBROWSER_LICENSE_KEY", "CLOAKBROWSER_SKIP_CHECKSUM"} {
		t.Setenv(k, "")
	}
	return cache
}

func fakeBinary(t *testing.T, version string, mode os.FileMode) string {
	t.Helper()
	p := binaryPath(version)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVersionNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"146.0.7680.177.6", "146.0.7680.177.5", true},
		{"146.0.7680.177.5", "146.0.7680.177.5", false},
		{"145.0.9999.0.1", "146.0.7680.177.5", false},
		{"147.0.0.0", "146.0.7680.177.5", true},
		{"146.0.7680.177.5.1", "146.0.7680.177.5", true},
		{"abc", "146.0.7680.177.5", false},
	} {
		if got := versionNewer(c.a, c.b); got != c.want {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizeRequestedVersion(t *testing.T) {
	isolate(t)
	if v, err := normalizeRequestedVersion(); v != "" || err != nil {
		t.Fatalf("unset: %q, %v", v, err)
	}
	t.Setenv("CLOAKBROWSER_VERSION", " 148.0.7778.215.2 ")
	if v, err := normalizeRequestedVersion(); v != "148.0.7778.215.2" || err != nil {
		t.Fatalf("valid pin: %q, %v", v, err)
	}
	for _, bad := range []string{"148", "148.0.1", "148.0.7778.215.2.1", "../x", "latest"} {
		t.Setenv("CLOAKBROWSER_VERSION", bad)
		if _, err := normalizeRequestedVersion(); err == nil {
			t.Errorf("pin %q accepted", bad)
		}
	}
}

func TestCacheDirAndBinaryPath(t *testing.T) {
	cache := isolate(t)
	if CacheDir() != cache {
		t.Fatalf("CacheDir() = %s", CacheDir())
	}
	t.Setenv("CLOAKBROWSER_CACHE_DIR", "")
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".cloakbrowser"); CacheDir() != want {
		t.Fatalf("default CacheDir() = %s, want %s", CacheDir(), want)
	}
	t.Setenv("CLOAKBROWSER_CACHE_DIR", cache)
	rel, _ := filepath.Rel(cache, binaryPath("1.2.3.4"))
	want := filepath.Join("chromium-1.2.3.4", "chrome")
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join("chromium-1.2.3.4", "Chromium.app", "Contents", "MacOS", "Chromium")
	case "windows":
		want = filepath.Join("chromium-1.2.3.4", "chrome.exe")
	}
	if rel != want {
		t.Fatalf("binaryPath = %s, want %s", rel, want)
	}
}

func TestEffectiveVersion(t *testing.T) {
	cache := isolate(t)
	base, err := platformVersion()
	if err != nil {
		t.Skip(err)
	}
	tag, _ := platformTag()
	newer := "999.0.0.0.1"
	marker := func(name, v string) {
		if err := os.WriteFile(filepath.Join(cache, name), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := func() string {
		v, err := effectiveVersion()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	if got() != base {
		t.Fatal("no marker: want the platform version")
	}
	marker("latest_version_"+tag, newer)
	if got() != base {
		t.Fatal("marker without a binary must be ignored")
	}
	fakeBinary(t, newer, 0o755)
	if got() != newer {
		t.Fatal("marker with a binary must win")
	}
	marker("latest_version_"+tag, "1.0.0.0") // older than base
	marker("latest_version", newer)
	if got() != newer {
		t.Fatal("legacy marker is the second choice")
	}
	marker("latest_version", "1.0.0.0")
	if got() != base {
		t.Fatal("older markers must be ignored")
	}
}

func TestDownloadURLs(t *testing.T) {
	isolate(t)
	tag, err := platformTag()
	if err != nil {
		t.Skip(err)
	}
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	name := "cloakbrowser-" + tag + ext
	if got, _ := archiveName(); got != name {
		t.Fatalf("archiveName = %s", got)
	}
	if got, _ := downloadURL("1.2.3.4.5"); got != "https://cloakbrowser.dev/chromium-v1.2.3.4.5/"+name {
		t.Fatalf("downloadURL = %s", got)
	}
	if got, _ := fallbackDownloadURL("1.2.3.4.5"); got !=
		"https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v1.2.3.4.5/"+name {
		t.Fatalf("fallbackDownloadURL = %s", got)
	}
	t.Setenv("CLOAKBROWSER_DOWNLOAD_URL", "http://mirror.local")
	if got, _ := downloadURL("1.2.3.4.5"); got != "http://mirror.local/chromium-v1.2.3.4.5/"+name {
		t.Fatalf("custom downloadURL = %s", got)
	}
}
