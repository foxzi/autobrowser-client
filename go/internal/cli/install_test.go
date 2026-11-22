package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/cloak"
)

func TestInstallCloakbrowser(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "chrome")
	os.WriteFile(bin, nil, 0o755)
	t.Setenv(cloak.BinaryEnv, bin)
	t.Setenv("AUTOBROWSER_BROWSER_ENGINE", "cloakbrowser")
	if code, out, errText := run("install"); code != 0 || out != bin+"\n" {
		t.Errorf("%d %q %q", code, out, errText)
	}
	t.Setenv(cloak.BinaryEnv, filepath.Join(t.TempDir(), "none"))
	if code, out, errText := run("install"); code != 1 || out != "" || !strings.Contains(errText, "does not exist") {
		t.Errorf("missing binary: %d %q %q", code, out, errText)
	}
}

func TestInstallPatchright(t *testing.T) {
	t.Setenv("AUTOBROWSER_BROWSER_ENGINE", "patchright")
	code, out, errText := run("install")
	if code != 1 || out != "" || !strings.HasPrefix(errText, `{"success": false, "error": {"code": "CONFIG_ERROR"`) ||
		!strings.Contains(errText, "supports only browser.engine=cloakbrowser") {
		t.Errorf("%d %q %q", code, out, errText)
	}
}

func TestInstallUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown flag": {"install", "--bogus"},
		"extra arg":    {"install", "x"},
		"no config":    {"install", "-c"},
	} {
		if code, _, _ := run(args...); code != 2 {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}
