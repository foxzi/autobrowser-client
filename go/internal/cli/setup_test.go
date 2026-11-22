package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateScript(t *testing.T) {
	path := writeScript(t, "steps:\n  - action: open\n    url: https://example.com\nstopOnError: false\n")
	code, out, _ := run("validate-script", "--file", path)
	if code != 0 || out != "{\n  \"success\": true,\n  \"steps\": 1,\n  \"stopOnError\": false\n}\n" {
		t.Fatalf("%d %q", code, out)
	}

	code, _, errText := run("validate-script", "-f", writeScript(t, "steps:\n  - action: open\n"))
	if code != 1 || !strings.Contains(errText, `"code": "SCRIPT_VALIDATION_ERROR"`) ||
		!strings.Contains(errText, "requires 'url'") || strings.Contains(errText, "goroutine") {
		t.Fatalf("%d %s", code, errText)
	}
	if code, _, _ := run("validate-script", "-f", writeScript(t, "steps:\n  - action: teleport\n")); code == 0 {
		t.Error("invalid action accepted")
	}
	if code, _, e := run("validate-script", "--file", "/nonexistent/script.yaml"); code != 2 || !strings.Contains(e, "does not exist") {
		t.Errorf("missing file: %d %s", code, e)
	}
	if code, _, e := run("validate-script"); code != 2 || !strings.Contains(e, "Missing option '--file' / '-f'.") {
		t.Errorf("no --file: %d %s", code, e)
	}
}

func TestGenerateFingerprint(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "work")
	code, out, _ := run("generate-fingerprint", "--profile", profile, "--platform", "linux", "--seed", "42")
	want := "Created profile directory: " + profile + `
{
  "seed": 42,
  "canvas": true,
  "webgl": true,
  "webglVendor": "Google Inc. (NVIDIA)",
  "webglRenderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060, OpenGL 4.5)",
  "audio": true,
  "clientRects": true,
  "fonts": true
}
`
	if code != 0 || out != want {
		t.Fatalf("%d %q", code, out)
	}
	saved, _ := os.ReadFile(filepath.Join(profile, "fingerprint.json"))
	if string(saved)+"\n" != strings.TrimPrefix(out, "Created profile directory: "+profile+"\n") {
		t.Errorf("saved file differs from stdout: %s", saved)
	}

	// exists: refused without --force
	code, out, _ = run("generate-fingerprint", "--profile", profile, "--seed", "43")
	path := filepath.Join(profile, "fingerprint.json")
	if code != 1 || out != "fingerprint.json already exists at "+path+"\nUse --force to overwrite\n" {
		t.Errorf("%d %q", code, out)
	}
	code, out, _ = run("generate-fingerprint", "--profile", profile, "--seed", "43", "--force")
	if code != 0 || !strings.Contains(out, `"seed": 43`) {
		t.Errorf("%d %q", code, out)
	}

	if code, out, _ = run("generate-fingerprint", "--profile", profile, "--platform", "mac"); code != 1 ||
		out != "Unknown platform: mac. Use: windows, linux, android\n" {
		t.Errorf("%d %q", code, out)
	}
	if code, _, e := run("generate-fingerprint", "--profile", profile, "--seed", "x"); code != 2 || !strings.Contains(e, "not a valid int") {
		t.Errorf("%d %s", code, e)
	}
	if code, _, e := run("generate-fingerprint"); code != 2 || !strings.Contains(e, "Missing option '--profile'.") {
		t.Errorf("%d %s", code, e)
	}
}

func TestInstallSkill(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := run("install-skill", "--dir", dir)
	target := filepath.Join(dir, "autobrowser", "SKILL.md")
	if code != 0 || out != target+"\n" {
		t.Fatalf("%d %q", code, out)
	}
	got, _ := os.ReadFile(target)
	if !strings.HasPrefix(string(got), "---\nname: autobrowser") {
		t.Errorf("unexpected skill: %.80s", got)
	}

	// default directory is ~/.claude/skills
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, out, _ = run("install-skill"); out != filepath.Join(home, ".claude", "skills", "autobrowser", "SKILL.md")+"\n" {
		t.Errorf("default dir: %q", out)
	}
}

func TestHelp(t *testing.T) {
	code, out, _ := run()
	if code != 2 || !strings.HasPrefix(out, "Usage: autobrowser [OPTIONS] COMMAND [ARGS]...") {
		t.Errorf("no args: %d %.60s", code, out)
	}
	_, all, _ := run("help")
	for _, c := range commands {
		if !strings.Contains(all, "  "+c.name+" ") {
			t.Errorf("help does not list %s", c.name)
		}
		code, out, _ := run("help", c.name)
		_, out2, _ := run(c.name, "--help")
		if code != 0 || out != out2 || !strings.HasPrefix(out, "Usage: autobrowser "+c.name+" ") ||
			!strings.Contains(out, c.help) || !strings.Contains(out, "-h, --help") {
			t.Errorf("help %s: %d %s", c.name, code, out)
		}
	}
	_, out, _ = run("help", "generate-fingerprint")
	if !strings.Contains(out, "--platform <str>  Target platform: windows, linux, android [default: windows]") {
		t.Error(out)
	}
	if code, _, e := run("help", "nope"); code != 2 || !strings.Contains(e, "Invalid value for COMMAND: No such command: nope") {
		t.Errorf("%d %s", code, e)
	}
}
