package cloak

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	installRenameAttempts   = 5
	transientRenameAttempts = 10
	transientRenameDelay    = 250 * time.Millisecond
)

// signingPubKeys is BINARY_SIGNING_PUBKEYS: base64 raw Ed25519 public keys (a
// variable for tests).
var signingPubKeys = []string{"MKFKwIhUcKWq5xTuNA0Ovg99njcDEcEJvmWYYhApvaU="}

func info(format string, args ...any) { slog.Info(fmt.Sprintf(format, args...)) }

// httpClient follows redirects, honours the proxy environment variables and
// uses a 10 s connect timeout like DOWNLOAD_TIMEOUT.
func httpClient(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	return &http.Client{Transport: t, Timeout: timeout}
}

// EnsureBinary is cloakbrowser.download.ensure_binary (free tier): it returns
// the path of the stealth Chromium, downloading it into the cache when needed.
// CLOAKBROWSER_BINARY_PATH skips the download. Pro license keys are ignored.
func EnsureBinary() (string, error) {
	if p := os.Getenv(BinaryEnv); p != "" {
		if !exists(p) {
			return "", fmt.Errorf("%s set to '%s' but file does not exist", BinaryEnv, p)
		}
		info("Using local binary override: %s", p)
		return p, nil
	}

	pinned, err := normalizeRequestedVersion()
	if err != nil {
		return "", err
	}
	if os.Getenv("CLOAKBROWSER_LICENSE_KEY") != "" && os.Getenv("CLOAKBROWSER_DOWNLOAD_URL") == "" {
		slog.Warn("CLOAKBROWSER_LICENSE_KEY is set, but Pro binaries are not supported by this client; using the free binary")
	}

	tag, err := platformTag()
	if err != nil {
		return "", err
	}

	if pinned != "" {
		path := binaryPath(pinned)
		if isExecutable(path) {
			slog.Debug(fmt.Sprintf("Pinned binary found in cache: %s (version %s)", path, pinned))
			return path, nil
		}
		info("Stealth Chromium %s not found. Downloading for %s...", pinned, tag)
		if err := downloadAndExtract(pinned); err != nil {
			return "", err
		}
		if !isExecutable(path) {
			return "", fmt.Errorf("Pinned download completed but binary not found at expected path: %s. "+
				"This may indicate a packaging issue. Please report at "+
				"https://github.com/CloakHQ/cloakbrowser/issues", path)
		}
		return path, nil
	}

	effective, err := effectiveVersion()
	if err != nil {
		return "", err
	}
	path := binaryPath(effective)
	if isExecutable(path) {
		slog.Debug(fmt.Sprintf("Binary found in cache: %s (version %s)", path, effective))
		return path, nil
	}

	platform, _ := platformVersion()
	if effective != platform {
		if fallback := binaryPath(platform); isExecutable(fallback) {
			slog.Debug("Binary found in cache: " + fallback)
			return fallback, nil
		}
	}

	info("Stealth Chromium %s not found. Downloading for %s...", platform, tag)
	if err := downloadAndExtract(platform); err != nil {
		return "", err
	}
	path = binaryPath(platform)
	if !exists(path) {
		return "", fmt.Errorf("Download completed but binary not found at expected path: %s. "+
			"This may indicate a packaging issue. Please report at "+
			"https://github.com/CloakHQ/cloakbrowser/issues", path)
	}
	return path, nil
}

// downloadAndExtract is _download_and_extract: primary origin, GitHub Releases
// fallback (not with a custom URL), verification, extraction.
func downloadAndExtract(version string) error {
	primary, err := downloadURL(version)
	if err != nil {
		return err
	}
	fallback, _ := fallbackDownloadURL(version)
	dir := binaryDir(version)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "cloakbrowser-*"+archiveExt())
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := downloadFile(primary, tmp.Name()); err != nil {
		if os.Getenv("CLOAKBROWSER_DOWNLOAD_URL") != "" {
			return err
		}
		slog.Warn(fmt.Sprintf("Primary download failed (%s), trying GitHub Releases...", err))
		if err := downloadFile(fallback, tmp.Name()); err != nil {
			return err
		}
	}
	if err := verifyDownloadChecksum(tmp.Name(), version); err != nil {
		return err
	}
	return extractArchive(tmp.Name(), dir, binaryPath(version))
}

// downloadFile is _download_file: it logs progress every 10% and gives up
// when no data arrives for 60 s.
func downloadFile(url, dest string) error {
	info("Downloading from %s", url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idle := time.AfterFunc(60*time.Second, cancel)
	defer idle.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient(0).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	total := max(resp.ContentLength, 0)
	var downloaded int64
	lastPct := int64(-1)
	buf := make([]byte, 8192)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			idle.Reset(60 * time.Second)
			if _, err := f.Write(buf[:n]); err != nil {
				slog.Error("Failed to write downloaded file: " + dest)
				return err
			}
			downloaded += int64(n)
			if total > 0 {
				if pct := downloaded * 100 / total; pct >= lastPct+10 {
					lastPct = pct
					info("Download progress: %d%% (%d/%d MB)", pct, downloaded>>20, total>>20)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	info("Download complete: %d MB", downloaded>>20)
	return nil
}

// verifyDownloadChecksum is _verify_download_checksum: the official origins
// require an Ed25519-signed SHA256SUMS; a custom CLOAKBROWSER_DOWNLOAD_URL gets
// the plain (skippable) same-origin checksum.
func verifyDownloadChecksum(file, version string) error {
	name, err := archiveName()
	if err != nil {
		return err
	}

	if os.Getenv("CLOAKBROWSER_DOWNLOAD_URL") != "" {
		if strings.EqualFold(os.Getenv("CLOAKBROWSER_SKIP_CHECKSUM"), "true") {
			slog.Warn("CLOAKBROWSER_SKIP_CHECKSUM set - skipping verification for custom download URL")
			return nil
		}
		sums := fetchChecksums(version)
		if sums == nil {
			slog.Warn("SHA256SUMS not available from custom URL - skipping checksum verification")
			return nil
		}
		expected, ok := sums[name]
		if !ok {
			slog.Warn(fmt.Sprintf("SHA256SUMS found but no entry for %s - skipping verification", name))
			return nil
		}
		return verifyChecksum(file, expected)
	}

	manifest, sig := fetchSignedManifest(version)
	if manifest == nil {
		return errors.New("Could not fetch a signed SHA256SUMS (SHA256SUMS + SHA256SUMS.sig) " +
			"for this release - refusing to use an unverified binary. " +
			"Retry, or report at https://github.com/CloakHQ/cloakbrowser/issues")
	}
	if err := verifySignature(manifest, sig); err != nil {
		return err
	}
	text := string(manifest)

	// The signature proves the manifest is genuine, not that it is for the
	// requested version: bind the version to refuse a forced downgrade.
	declared := parseManifestVersion(text)
	if declared != version {
		if declared == "" {
			declared = "none"
		}
		return fmt.Errorf("Version mismatch in signed SHA256SUMS: requested %s, "+
			"manifest declares %s. Refusing (possible downgrade).", version, declared)
	}
	expected, ok := parseChecksums(text)[name]
	if !ok {
		return fmt.Errorf("Signature-verified SHA256SUMS has no entry for %s - "+
			"cannot confirm binary integrity.", name)
	}
	return verifyChecksum(file, expected)
}

// parseManifestVersion reads the "version=<v>" line of a signed manifest.
func parseManifestVersion(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "version="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func httpGet(url string) ([]byte, error) {
	resp, err := httpClient(10 * time.Second).Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// fetchSignedManifest is _fetch_signed_manifest: SHA256SUMS and its signature
// from the same origin, primary first, then GitHub Releases. Nil when neither works.
func fetchSignedManifest(version string) (manifest, sig []byte) {
	for _, base := range []string{
		fmt.Sprintf("%s/chromium-v%s", downloadBase(), version),
		fmt.Sprintf("%s/chromium-v%s", githubBase, version),
	} {
		m, err := httpGet(base + "/SHA256SUMS")
		if err != nil {
			continue
		}
		s, err := httpGet(base + "/SHA256SUMS.sig")
		if err != nil {
			continue
		}
		return m, s
	}
	return nil, nil
}

// verifySignature checks a detached Ed25519 signature (base64) over the raw
// manifest against each pinned key.
func verifySignature(manifest, sigB64 []byte) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("Malformed SHA256SUMS.sig (not valid base64): %s", err)
	}
	for _, k := range signingPubKeys {
		pub, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(pub, manifest, sig) {
			info("SHA256SUMS signature verified: Ed25519 OK")
			return nil
		}
	}
	return errors.New("SHA256SUMS signature verification failed - no pinned key validated the " +
		"manifest. The binary's authenticity could not be confirmed. " +
		"Report at https://github.com/CloakHQ/cloakbrowser/issues")
}

// fetchChecksums is _fetch_checksums: the plain SHA256SUMS of a version, nil if unavailable.
func fetchChecksums(version string) map[string]string {
	urls := []string{fmt.Sprintf("%s/chromium-v%s/SHA256SUMS", downloadBase(), version)}
	if os.Getenv("CLOAKBROWSER_DOWNLOAD_URL") == "" {
		urls = append(urls, fmt.Sprintf("%s/chromium-v%s/SHA256SUMS", githubBase, version))
	}
	for _, u := range urls {
		if data, err := httpGet(u); err == nil {
			return parseChecksums(string(data))
		}
	}
	return nil
}

// parseChecksums parses "<64-hex sha256>  filename" lines, ignoring anything else.
func parseChecksums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := strings.ToLower(parts[0])
		if b, err := hex.DecodeString(hash); err != nil || len(b) != sha256.Size {
			continue
		}
		// Python splits once, so a file name keeps its inner spaces.
		_, name, _ := strings.Cut(strings.TrimSpace(line), parts[0])
		out[strings.TrimLeft(strings.TrimSpace(name), "*")] = hash
	}
	return out
}

func verifyChecksum(file, expected string) error {
	f, err := os.Open(file)
	if err != nil {
		slog.Error("Failed to read downloaded file for checksum: " + file)
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
		return fmt.Errorf("Checksum verification failed!\n  Expected: %s\n  Got:      %s\n"+
			"  File may be corrupted or tampered with. "+
			"Please retry or report at https://github.com/CloakHQ/cloakbrowser/issues", expected, actual)
	}
	info("Checksum verified: SHA-256 OK")
	return nil
}

func randomHex() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// extractArchive is _extract_archive: unpack into a private sibling directory,
// flatten a single top-level directory, chmod +x the binary, and rename the
// result into place, keeping a complete install a concurrent caller made.
func extractArchive(archive, dest, binary string) error {
	info("Extracting to %s", dest)
	staging := filepath.Join(filepath.Dir(dest), filepath.Base(dest)+".partial-"+randomHex())
	if err := os.Mkdir(staging, 0o755); err != nil {
		return err
	}
	var stale []string
	defer func() {
		os.RemoveAll(staging)
		for _, d := range stale {
			os.RemoveAll(d)
		}
	}()

	var err error
	if strings.HasSuffix(archive, ".zip") {
		err = extractZip(archive, staging)
	} else {
		err = extractTar(archive, staging)
	}
	if err != nil {
		return err
	}
	if err := flattenSingleSubdir(staging); err != nil {
		return err
	}

	rel, _ := filepath.Rel(dest, binary)
	if staged := filepath.Join(staging, rel); exists(staged) {
		makeExecutable(staged)
	}
	if runtime.GOOS == "darwin" {
		removeQuarantine(staging)
	}

	for attempt := 1; attempt <= installRenameAttempts; attempt++ {
		err := renameDirWithRetry(staging, dest)
		if err == nil {
			break
		}
		// dest is in the way: keep it if it is a complete install, replace an
		// incomplete leftover.
		if isExecutable(binary) {
			break
		}
		if attempt == installRenameAttempts {
			return err
		}
		aside := filepath.Join(filepath.Dir(dest), filepath.Base(dest)+".stale-"+randomHex())
		if err := renameDirWithRetry(dest, aside); err != nil {
			if os.IsNotExist(err) {
				continue // another caller moved it aside first
			}
			return err
		}
		stale = append(stale, aside)
	}

	if exists(binary) {
		info("Binary ready: %s", binary)
	}
	return nil
}

// renameDirWithRetry retries a rename that Windows transiently denies.
func renameDirWithRetry(src, dst string) error {
	for attempt := 1; ; attempt++ {
		err := os.Rename(src, dst)
		if err == nil || !os.IsPermission(err) || runtime.GOOS != "windows" ||
			attempt >= transientRenameAttempts || exists(dst) {
			return err
		}
		time.Sleep(transientRenameDelay)
	}
}

// safeJoin resolves an archive entry name below dir, refusing paths that escape it.
func safeJoin(dir, name string) (string, error) {
	p := filepath.Join(dir, filepath.FromSlash(name))
	if p != dir && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry escapes the target directory: %s", name)
	}
	return p, nil
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, mode.Perm()) // exact archive mode, not masked by umask
}

func extractTar(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		path, err := safeJoin(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o755)
		case tar.TypeReg:
			err = writeFile(path, tr, os.FileMode(h.Mode))
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				err = os.Symlink(h.Linkname, path)
			}
		case tar.TypeLink:
			var target string
			if target, err = safeJoin(dest, h.Linkname); err == nil {
				if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
					err = os.Link(target, path)
				}
			}
		}
		if err != nil {
			return err
		}
	}
}

func extractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		path, err := safeJoin(dest, zf.Name)
		if err != nil {
			return err
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFile(path, rc, zf.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// flattenSingleSubdir moves the contents of a single top-level directory up
// (never for a macOS .app bundle).
func flattenSingleSubdir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() || strings.HasSuffix(entries[0].Name(), ".app") {
		return nil
	}
	sub := filepath.Join(dir, entries[0].Name())
	slog.Debug("Flattening single subdirectory: " + entries[0].Name())
	items, err := os.ReadDir(sub)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := os.Rename(filepath.Join(sub, it.Name()), filepath.Join(dir, it.Name())); err != nil {
			slog.Error("Failed to flatten extracted directory: " + sub)
			return err
		}
	}
	return os.Remove(sub)
}

// makeExecutable is chmod +x, a no-op on Windows.
func makeExecutable(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if st, err := os.Stat(path); err == nil {
		os.Chmod(path, st.Mode().Perm()|0o111)
	}
}

// removeQuarantine drops macOS quarantine xattrs so Gatekeeper does not block the binary.
func removeQuarantine(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "xattr", "-cr", path).Run(); err != nil {
		slog.Debug("Failed to remove quarantine attributes: " + err.Error())
	}
}
