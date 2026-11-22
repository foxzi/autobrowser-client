package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-V"}} {
		if code, out, _ := run(args...); code != 0 || out != Version+"\n" {
			t.Errorf("%v: %d %q", args, code, out)
		}
	}
}

func TestShowConfig(t *testing.T) {
	for _, k := range []string{"ENGINE", "HEADLESS"} {
		t.Setenv("AUTOBROWSER_BROWSER_"+k, "")
		os.Unsetenv("AUTOBROWSER_BROWSER_" + k)
	}
	t.Setenv("AUTOBROWSER_HEADLESS", "")
	os.Unsetenv("AUTOBROWSER_HEADLESS")
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte("api: {port: 9999}\n"), 0o644)
	code, out, _ := run("show-config", "--config", path)
	if code != 0 || !strings.HasPrefix(out, "{\n  \"browser\": {") || !strings.Contains(out, `"port": 9999`) {
		t.Fatalf("%d %s", code, out)
	}
	code, _, errText := run("show-config", "-c", filepath.Join(t.TempDir(), "none.yaml"))
	if code != 1 || !strings.HasPrefix(errText, `{"success": false, "error": {"code": "CONFIG_ERROR"`) {
		t.Fatalf("%d %s", code, errText)
	}
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := run("nope"); code != 2 {
		t.Errorf("unknown command: %d", code)
	}
	if code, _, _ := run("show-config", "--bogus"); code != 2 {
		t.Errorf("bad flag: %d", code)
	}
	if code, _, _ := run("version", "extra"); code != 2 {
		t.Errorf("extra arg: %d", code)
	}
}
