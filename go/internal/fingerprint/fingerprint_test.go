package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// Expected renderers come from the Python client (random.Random(seed).choice).
var pythonGPU = []struct {
	platform string
	seed     int
	renderer string
}{
	{"windows", 0, "ANGLE (AMD, AMD Radeon RX 5700 XT Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 1, "ANGLE (NVIDIA, NVIDIA GeForce GTX 1050 Ti Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 42, "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 123456, "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 999999999, "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 4294967296, "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", 12345678901234, "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"windows", -5, "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"linux", 0, "ANGLE (AMD, AMD Radeon RX 580 (radeonsi, polaris10, LLVM 15.0.7, DRM 3.49, 6.1.0), OpenGL 4.6)"},
	{"linux", 1, "ANGLE (NVIDIA, NVIDIA GeForce RTX 2060, OpenGL 4.5)"},
	{"linux", 42, "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060, OpenGL 4.5)"},
	{"linux", 123456, "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)"},
	{"linux", 999999999, "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)"},
	{"linux", 4294967296, "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060, OpenGL 4.5)"},
	{"linux", 12345678901234, "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER, OpenGL 4.5)"},
	{"linux", -5, "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)"},
	{"android", 0, "Mali-G77"},
	{"android", 1, "Adreno (TM) 640"},
	{"android", 42, "Adreno (TM) 660"},
	{"android", 123456, "Adreno (TM) 619"},
	{"android", 999999999, "Adreno (TM) 619"},
	{"android", 4294967296, "Adreno (TM) 660"},
	{"android", 12345678901234, "Adreno (TM) 730"},
	{"android", -5, "Adreno (TM) 619"},
}

func TestGenerateMatchesPython(t *testing.T) {
	for _, tc := range pythonGPU {
		seed := tc.seed
		c := Generate(&seed, Platform(tc.platform))
		if c.WebglRenderer != tc.renderer || c.Seed != tc.seed {
			t.Errorf("%s seed %d: %q, want %q", tc.platform, tc.seed, c.WebglRenderer, tc.renderer)
		}
	}
}

func TestGenerateRandomSeed(t *testing.T) {
	c := Generate(nil, "windows")
	if c.Seed < 100000 || c.Seed > 999999999 || !c.Canvas || !c.Webgl || !c.Audio || !c.ClientRects || !c.Fonts {
		t.Errorf("%+v", c)
	}
	seed := 1
	if Generate(&seed, "nope").WebglRenderer != Generate(&seed, "windows").WebglRenderer {
		t.Error("unknown platform must use the windows pool")
	}
}

func TestSaveFormatAndInitScript(t *testing.T) {
	seed := 77
	c := Generate(&seed, "linux")
	dir := t.TempDir()
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, File))
	want := `{
  "seed": 77,
  "canvas": true,
  "webgl": true,
  "webglVendor": "Google Inc. (Intel)",
  "webglRenderer": "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)",
  "audio": true,
  "clientRects": true,
  "fonts": true
}`
	if string(data) != want {
		t.Errorf("fingerprint.json:\n%s", data)
	}
	script := InitScript(c)
	head := `window.__fpConfig = {"seed": 77, "canvas": true, "webgl": true, "webglVendor": "Google Inc. (Intel)", "webglRenderer": "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)", "audio": true, "clientRects": true, "fonts": true};` + "\n"
	if !strings.HasPrefix(script, head) || !strings.HasSuffix(script, injectJS) || len(injectJS) < 1000 {
		t.Errorf("init script: %.300s", script)
	}
}

func TestLoadGeneratesAndReuses(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, File)); err != nil {
		t.Fatal("fingerprint.json not saved")
	}
	again, err := Load(dir, "windows")
	if err != nil || *again != *c {
		t.Errorf("%+v %v, want %+v", again, err, c)
	}
}

func TestLoadRegeneratesInvalid(t *testing.T) {
	for _, bad := range []string{
		`not json`,
		`[]`,
		`{"seed": 1, "webglVendor": "v"}`,
		`{"seed": "1", "webglVendor": "v", "webglRenderer": "r"}`,
		`{"seed": 1, "webglVendor": "v", "webglRenderer": "r", "extra": 1}`,
		`{"seed": 1, "webglVendor": "v", "webglRenderer": "r", "canvas": "yes"}`,
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, File), []byte(bad), 0o644)
		c, err := Load(dir, "windows")
		if err != nil || c.WebglRenderer == "r" {
			t.Errorf("%s: %+v %v", bad, c, err)
		}
		if data, _ := os.ReadFile(filepath.Join(dir, File)); string(data) == bad {
			t.Errorf("%s: file not rewritten", bad)
		}
	}
}

func TestLoadKeepsPartialFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, File), []byte(`{"seed": 5.0, "webglVendor": "v", "webglRenderer": "r", "audio": false}`), 0o644)
	c, err := Load(dir, "windows")
	want := Config{Seed: 5, Canvas: true, Webgl: true, WebglVendor: "v", WebglRenderer: "r", Audio: false, ClientRects: true, Fonts: true}
	if err != nil || *c != want {
		t.Errorf("%+v %v", c, err)
	}
}

func TestMergeAndResolve(t *testing.T) {
	seed, off, vendor := 9, false, "V"
	o := &scenario.FingerprintOverrides{Seed: &seed, Canvas: &off, WebglVendor: &vendor}

	// no profile, overrides: generated from the seed, then merged
	c, err := Resolve("", o, "linux")
	if err != nil {
		t.Fatal(err)
	}
	g := Generate(&seed, "linux")
	if c.Seed != 9 || c.Canvas || c.WebglVendor != "V" || c.WebglRenderer != g.WebglRenderer || !c.Fonts {
		t.Errorf("%+v", c)
	}

	// profile: base from fingerprint.json, overrides on top
	dir := t.TempDir()
	base, _ := Load(dir, "windows")
	c, _ = Resolve(dir, o, "linux")
	if c.WebglRenderer != base.WebglRenderer || c.Seed != 9 {
		t.Errorf("%+v base %+v", c, base)
	}
	if c, _ = Resolve(dir, true, "linux"); *c != *base {
		t.Errorf("true: %+v", c)
	}
	if c, _ = Resolve(dir, nil, "linux"); *c != *base {
		t.Errorf("nil: %+v", c)
	}

	// disabled, no profile, explicit config
	if c, _ = Resolve(dir, false, "linux"); c != nil {
		t.Error("false must disable")
	}
	if c, _ = Resolve("", nil, "linux"); c != nil {
		t.Error("no profile and no overrides: nothing")
	}
	if c, _ = Resolve("", base, "linux"); c != base {
		t.Error("*Config is used as is")
	}
}

func TestJSConfigOrder(t *testing.T) {
	m := (&Config{}).JSConfig()
	want := "seed canvas webgl webglVendor webglRenderer audio clientRects fonts"
	if strings.Join(m.Keys(), " ") != want {
		t.Error(m.Keys())
	}
}
