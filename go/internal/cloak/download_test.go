package cloak

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tarGz builds a tar.gz from name -> content ("dir/" names are directories).
func tarGz(t *testing.T, files [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		h := &tar.Header{Name: f[0], Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(len(f[1]))}
		if strings.HasSuffix(f[0], "/") {
			h = &tar.Header{Name: f[0], Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f[1]))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractTarFlattensAndChmods(t *testing.T) {
	cache := isolate(t)
	dest := filepath.Join(cache, "chromium-x")
	bin := filepath.Join(dest, "chrome")
	archive := writeTemp(t, "a.tar.gz", tarGz(t, [][2]string{
		{"wrap/", ""}, {"wrap/chrome", "bin"}, {"wrap/lib/x.so", "so"},
	}))
	if err := extractArchive(archive, dest, bin); err != nil {
		t.Fatal(err)
	}
	if !isExecutable(bin) && runtime.GOOS != "windows" {
		t.Fatal("binary is not executable")
	}
	if _, err := os.Stat(filepath.Join(dest, "lib", "x.so")); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(dest + ".*"); len(left) != 0 {
		t.Fatalf("staging leftovers: %v", left)
	}
}

func TestExtractKeepsAppBundle(t *testing.T) {
	cache := isolate(t)
	dest := filepath.Join(cache, "chromium-x")
	bin := filepath.Join(dest, "Chromium.app", "Contents", "MacOS", "Chromium")
	archive := writeTemp(t, "a.tar.gz", tarGz(t, [][2]string{{"Chromium.app/Contents/MacOS/Chromium", "bin"}}))
	if err := extractArchive(archive, dest, bin); err != nil {
		t.Fatal(err)
	}
	if !exists(bin) {
		t.Fatal(".app bundle was flattened")
	}
}

func TestExtractZip(t *testing.T) {
	cache := isolate(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("wrap/chrome.exe")
	w.Write([]byte("bin"))
	zw.Close()
	archive := writeTemp(t, "a.zip", buf.Bytes())
	dest := filepath.Join(cache, "chromium-x")
	if err := extractArchive(archive, dest, filepath.Join(dest, "chrome.exe")); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dest, "chrome.exe")) {
		t.Fatal("zip entry was not extracted and flattened")
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	cache := isolate(t)
	archive := writeTemp(t, "a.tar.gz", tarGz(t, [][2]string{{"../evil", "x"}}))
	dest := filepath.Join(cache, "chromium-x")
	if err := extractArchive(archive, dest, filepath.Join(dest, "chrome")); err == nil {
		t.Fatal("path traversal was not rejected")
	}
	if exists(filepath.Join(cache, "evil")) {
		t.Fatal("file escaped the staging directory")
	}
}

func TestExtractKeepsCompleteInstall(t *testing.T) {
	cache := isolate(t)
	dest := filepath.Join(cache, "chromium-x")
	bin := filepath.Join(dest, "chrome")
	os.MkdirAll(dest, 0o755)
	os.WriteFile(bin, []byte("old"), 0o755)
	archive := writeTemp(t, "a.tar.gz", tarGz(t, [][2]string{{"chrome", "new"}}))
	if err := extractArchive(archive, dest, bin); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(bin); string(data) != "old" {
		t.Fatalf("complete install was replaced: %q", data)
	}
}

func TestExtractReplacesIncompleteInstall(t *testing.T) {
	cache := isolate(t)
	dest := filepath.Join(cache, "chromium-x")
	bin := filepath.Join(dest, "chrome")
	os.MkdirAll(dest, 0o755)
	os.WriteFile(filepath.Join(dest, "junk"), []byte("x"), 0o644)
	archive := writeTemp(t, "a.tar.gz", tarGz(t, [][2]string{{"chrome", "new"}}))
	if err := extractArchive(archive, dest, bin); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(bin); string(data) != "new" || exists(filepath.Join(dest, "junk")) {
		t.Fatal("incomplete install was not replaced")
	}
	if left, _ := filepath.Glob(dest + ".*"); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
}

func TestParseChecksums(t *testing.T) {
	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("B", 64)
	got := parseChecksums("version=1.2.3.4.5\n\n" + h1 + "  one.tar.gz\n" + h2 + " *two.zip\nshort  x\n" + h1 + "\n")
	want := map[string]string{"one.tar.gz": h1, "two.zip": strings.ToLower(h2)}
	if len(got) != len(want) || got["one.tar.gz"] != want["one.tar.gz"] || got["two.zip"] != want["two.zip"] {
		t.Fatalf("parseChecksums = %v", got)
	}
	if v := parseManifestVersion("a\n  version= 1.2.3.4.5 \nb"); v != "1.2.3.4.5" {
		t.Fatalf("parseManifestVersion = %q", v)
	}
	if v := parseManifestVersion(h1 + "  x"); v != "" {
		t.Fatalf("parseManifestVersion without line = %q", v)
	}
}

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := signingPubKeys
	signingPubKeys = []string{"not base64!", base64.StdEncoding.EncodeToString(pub)}
	t.Cleanup(func() { signingPubKeys = old })
	return priv
}

func TestVerifySignature(t *testing.T) {
	priv := testKey(t)
	manifest := []byte("version=1\n")
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, manifest))
	if err := verifySignature(manifest, []byte(sig+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := verifySignature([]byte("version=2\n"), []byte(sig)); err == nil ||
		!strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("tampered manifest accepted: %v", err)
	}
	if err := verifySignature(manifest, []byte("***")); err == nil ||
		!strings.Contains(err.Error(), "Malformed SHA256SUMS.sig") {
		t.Fatalf("malformed signature: %v", err)
	}
	if err := verifySignature(manifest, []byte(base64.StdEncoding.EncodeToString([]byte("short")))); err == nil {
		t.Fatal("wrong length signature accepted")
	}
}

func TestPinnedKeyIsValidEd25519Key(t *testing.T) {
	pub, err := base64.StdEncoding.DecodeString(signingPubKeys[0])
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("pinned key is not a 32-byte base64 key: %v", err)
	}
}

// release is a fake download origin serving one version.
type release struct {
	version  string // version the archive is requested for
	declared string // version written into the manifest
	archive  []byte
	priv     ed25519.PrivateKey
	sumsHash string // overrides the archive hash in the manifest when set
	noSums   bool
	hits     map[string]int
}

func (r *release) server(t *testing.T) *httptest.Server {
	t.Helper()
	name, _ := archiveName()
	r.hits = map[string]int{}
	prefix := "/chromium-v" + r.version + "/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits[strings.TrimPrefix(req.URL.Path, prefix)]++
		sum := sha256.Sum256(r.archive)
		hash := hex.EncodeToString(sum[:])
		if r.sumsHash != "" {
			hash = r.sumsHash
		}
		manifest := fmt.Sprintf("version=%s\n%s  %s\n", r.declared, hash, name)
		switch req.URL.Path {
		case prefix + name:
			w.Write(r.archive)
		case prefix + "SHA256SUMS":
			if r.noSums {
				http.NotFound(w, req)
				return
			}
			w.Write([]byte(manifest))
		case prefix + "SHA256SUMS.sig":
			if r.priv == nil {
				http.NotFound(w, req)
				return
			}
			w.Write([]byte(base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, []byte(manifest)))))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRelease(t *testing.T) *release {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test archives are tar.gz")
	}
	tag, err := platformVersion()
	if err != nil {
		t.Skip(err)
	}
	rel, _ := filepath.Rel(binaryDir(tag), binaryPath(tag))
	return &release{
		version:  tag,
		declared: tag,
		archive:  tarGz(t, [][2]string{{"wrap/" + filepath.ToSlash(rel), "binary"}}),
		priv:     testKey(t),
	}
}

func useOrigins(t *testing.T, primary, github string) {
	t.Helper()
	oldP, oldG := officialBase, githubBase
	officialBase, githubBase = primary, github
	t.Cleanup(func() { officialBase, githubBase = oldP, oldG })
}

func TestEnsureBinaryDownloadsAndVerifies(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	srv := r.server(t)
	useOrigins(t, srv.URL, "http://127.0.0.1:1")

	path, err := EnsureBinary()
	if err != nil {
		t.Fatal(err)
	}
	if path != binaryPath(r.version) || !isExecutable(path) {
		t.Fatalf("path = %s", path)
	}
	// second call uses the cache
	total := func() (n int) {
		for _, c := range r.hits {
			n += c
		}
		return
	}
	before := total()
	if _, err := EnsureBinary(); err != nil {
		t.Fatal(err)
	}
	if total() != before {
		t.Fatal("cached binary was downloaded again")
	}
}

func TestEnsureBinaryFallsBackToGitHub(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	good := r.server(t)
	bad := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(bad.Close)
	useOrigins(t, bad.URL, good.URL)

	if _, err := EnsureBinary(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureBinaryRefusesBadVerification(t *testing.T) {
	for name, tc := range map[string]struct {
		mod  func(r *release)
		want string
	}{
		"bad checksum":     {func(r *release) { r.sumsHash = strings.Repeat("0", 64) }, "Checksum verification failed"},
		"older version":    {func(r *release) { r.declared = "1.0.0.0.1" }, "Version mismatch in signed SHA256SUMS"},
		"no manifest":      {func(r *release) { r.noSums = true }, "Could not fetch a signed SHA256SUMS"},
		"unsigned release": {func(r *release) { r.priv = nil }, "Could not fetch a signed SHA256SUMS"},
	} {
		t.Run(name, func(t *testing.T) {
			cache := isolate(t)
			r := newRelease(t)
			tc.mod(r)
			srv := r.server(t)
			useOrigins(t, srv.URL, "http://127.0.0.1:1")
			_, err := EnsureBinary()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if exists(binaryDir(r.version)) {
				t.Fatalf("unverified binary was installed in %s", cache)
			}
		})
	}
}

func TestEnsureBinaryWrongSignatureKey(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	_, other, _ := ed25519.GenerateKey(nil)
	r.priv = other
	srv := r.server(t)
	useOrigins(t, srv.URL, "http://127.0.0.1:1")
	if _, err := EnsureBinary(); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureBinaryCustomURL(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	r.noSums = true
	srv := r.server(t)
	t.Setenv("CLOAKBROWSER_DOWNLOAD_URL", srv.URL)
	if _, err := EnsureBinary(); err != nil {
		t.Fatalf("custom URL without SHA256SUMS must install: %v", err)
	}
}

func TestEnsureBinaryCustomURLChecksum(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	r.sumsHash = strings.Repeat("0", 64)
	srv := r.server(t)
	t.Setenv("CLOAKBROWSER_DOWNLOAD_URL", srv.URL)
	if _, err := EnsureBinary(); err == nil || !strings.Contains(err.Error(), "Checksum verification failed") {
		t.Fatalf("error = %v", err)
	}
	t.Setenv("CLOAKBROWSER_SKIP_CHECKSUM", "true")
	if _, err := EnsureBinary(); err != nil {
		t.Fatalf("CLOAKBROWSER_SKIP_CHECKSUM must skip verification: %v", err)
	}
}

func TestEnsureBinaryCustomURLNoFallback(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	good := r.server(t)
	bad := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(bad.Close)
	useOrigins(t, "unused", good.URL)
	t.Setenv("CLOAKBROWSER_DOWNLOAD_URL", bad.URL)
	if _, err := EnsureBinary(); err == nil {
		t.Fatal("custom URL must not fall back to GitHub")
	}
}

func TestEnsureBinaryOverride(t *testing.T) {
	isolate(t)
	p := writeTemp(t, "mychrome", []byte("x"))
	t.Setenv(BinaryEnv, p)
	if got, err := EnsureBinary(); err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}
	t.Setenv(BinaryEnv, p+".missing")
	want := fmt.Sprintf("CLOAKBROWSER_BINARY_PATH set to '%s.missing' but file does not exist", p)
	if _, err := EnsureBinary(); err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestEnsureBinaryFromCache(t *testing.T) {
	isolate(t)
	base, err := platformVersion()
	if err != nil {
		t.Skip(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("exec bits")
	}
	p := fakeBinary(t, base, 0o755)
	if got, err := EnsureBinary(); err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}

	// effective (marker) version is not usable: fall back to the platform one
	tag, _ := platformTag()
	newer := "999.0.0.0.1"
	fakeBinary(t, newer, 0o644)
	os.WriteFile(filepath.Join(CacheDir(), "latest_version_"+tag), []byte(newer), 0o644)
	if got, err := EnsureBinary(); err != nil || got != p {
		t.Fatalf("fallback: got %q, %v", got, err)
	}

	// pinned version in cache
	pinned := "148.0.7778.215.2"
	pp := fakeBinary(t, pinned, 0o755)
	t.Setenv("CLOAKBROWSER_VERSION", pinned)
	if got, err := EnsureBinary(); err != nil || got != pp {
		t.Fatalf("pinned: got %q, %v", got, err)
	}

	t.Setenv("CLOAKBROWSER_VERSION", "bogus")
	if _, err := EnsureBinary(); err == nil || !strings.Contains(err.Error(), "Invalid browser version pin") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureBinaryDownloadsPinnedVersion(t *testing.T) {
	isolate(t)
	r := newRelease(t)
	r.version, r.declared = "148.0.7778.215.2", "148.0.7778.215.2"
	rel, _ := filepath.Rel(binaryDir(r.version), binaryPath(r.version))
	r.archive = tarGz(t, [][2]string{{filepath.ToSlash(rel), "binary"}})
	srv := r.server(t)
	useOrigins(t, srv.URL, "http://127.0.0.1:1")
	t.Setenv("CLOAKBROWSER_VERSION", r.version)
	got, err := EnsureBinary()
	if err != nil || got != binaryPath(r.version) || !isExecutable(got) {
		t.Fatalf("got %q, %v", got, err)
	}
}
