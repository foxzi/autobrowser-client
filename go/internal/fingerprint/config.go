// Package fingerprint ports autobrowser_client/fingerprint: the per-profile
// fingerprint config (generation, fingerprint.json) and the JS init script.
package fingerprint

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// File is the name of the fingerprint file inside a profile directory.
const File = "fingerprint.json"

// Platform selects the GPU pool: windows, linux or android.
type Platform string

// gpu is a WebGL vendor/renderer pair.
type gpu struct{ vendor, renderer string }

// GPU pools: Windows (ANGLE + Direct3D11).
var gpuWindows = []gpu{
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce GTX 1050 Ti Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 2060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (AMD)", "ANGLE (AMD, AMD Radeon RX 580 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (AMD)", "ANGLE (AMD, AMD Radeon RX 5700 XT Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)"},
}

// Linux (ANGLE + OpenGL, Mesa drivers).
var gpuLinux = []gpu{
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER, OpenGL 4.5)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060, OpenGL 4.5)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce RTX 2060, OpenGL 4.5)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Mesa Intel(R) UHD Graphics 630 (CFL GT2), OpenGL 4.6)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)"},
	{"Google Inc. (Intel)", "ANGLE (Intel, Mesa Intel(R) UHD Graphics 620 (WHL GT2), OpenGL 4.6)"},
	{"Google Inc. (AMD)", "ANGLE (AMD, AMD Radeon RX 580 (radeonsi, polaris10, LLVM 15.0.7, DRM 3.49, 6.1.0), OpenGL 4.6)"},
	{"Google Inc. (AMD)", "ANGLE (AMD, AMD Radeon RX 5700 XT (radeonsi, navi10, LLVM 15.0.7, DRM 3.49, 6.1.0), OpenGL 4.6)"},
	{"Google Inc. (NVIDIA)", "ANGLE (NVIDIA, NVIDIA GeForce GTX 1050 Ti, OpenGL 4.5)"},
}

// Android (mobile GPUs).
var gpuAndroid = []gpu{
	{"Qualcomm", "Adreno (TM) 730"},
	{"Qualcomm", "Adreno (TM) 660"},
	{"Qualcomm", "Adreno (TM) 640"},
	{"Qualcomm", "Adreno (TM) 620"},
	{"Qualcomm", "Adreno (TM) 619"},
	{"ARM", "Mali-G78"},
	{"ARM", "Mali-G77"},
	{"ARM", "Mali-G710"},
	{"ARM", "Mali-G510"},
}

// Config is the per-profile fingerprint spoofing configuration (FingerprintConfig).
type Config struct {
	Seed                       int
	Canvas, Webgl              bool
	WebglVendor, WebglRenderer string
	Audio, ClientRects, Fonts  bool
}

// JSConfig is to_js_config: the JSON form used in fingerprint.json and the init script.
func (c *Config) JSConfig() *omap.Map {
	return omap.New("seed", c.Seed, "canvas", c.Canvas, "webgl", c.Webgl,
		"webglVendor", c.WebglVendor, "webglRenderer", c.WebglRenderer,
		"audio", c.Audio, "clientRects", c.ClientRects, "fonts", c.Fonts)
}

// Generate is generate_fingerprint: the GPU is picked from the platform pool by
// the seed; a nil seed means a random one in 100000..999999999. An unknown
// platform uses the Windows pool.
func Generate(seed *int, platform Platform) *Config {
	n := randomSeed()
	if seed != nil {
		n = *seed
	}
	pool := gpuWindows
	switch platform {
	case "linux":
		pool = gpuLinux
	case "android":
		pool = gpuAndroid
	}
	g := pool[newMT(n).choice(len(pool))]
	return &Config{Seed: n, Canvas: true, Webgl: true, WebglVendor: g.vendor, WebglRenderer: g.renderer,
		Audio: true, ClientRects: true, Fonts: true}
}

// Merge is merge_fingerprint: the non-nil overrides replace the base values.
func Merge(base *Config, o *scenario.FingerprintOverrides) *Config {
	c := *base
	if o.Seed != nil {
		c.Seed = *o.Seed
	}
	setBool(&c.Canvas, o.Canvas)
	setBool(&c.Webgl, o.Webgl)
	if o.WebglVendor != nil {
		c.WebglVendor = *o.WebglVendor
	}
	if o.WebglRenderer != nil {
		c.WebglRenderer = *o.WebglRenderer
	}
	setBool(&c.Audio, o.Audio)
	setBool(&c.ClientRects, o.ClientRects)
	setBool(&c.Fonts, o.Fonts)
	return &c
}

func setBool(dst *bool, v *bool) {
	if v != nil {
		*dst = *v
	}
}

// Save is save_fingerprint: writes profileDir/fingerprint.json.
func Save(profileDir string, c *Config) error {
	path := filepath.Join(profileDir, File)
	if err := os.WriteFile(path, []byte(omap.Marshal(c.JSConfig(), 2)), 0o644); err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("Saved fingerprint to %s (seed=%d)", path, c.Seed))
	return nil
}

// Load is load_fingerprint: reads profileDir/fingerprint.json, or generates
// and saves a new one when it is missing or invalid.
func Load(profileDir string, platform Platform) (*Config, error) {
	path := filepath.Join(profileDir, File)
	if _, err := os.Stat(path); err == nil {
		c, err := read(path)
		if err == nil {
			slog.Info(fmt.Sprintf("Loaded fingerprint from %s (seed=%d)", path, c.Seed))
			return c, nil
		}
		slog.Warn(fmt.Sprintf("Failed to load %s, regenerating: %s", path, err))
	}
	c := Generate(nil, platform)
	return c, Save(profileDir, c)
}

// fields are the keys of fingerprint.json (extra keys are rejected, as in pydantic).
var fields = []string{"seed", "canvas", "webgl", "webglVendor", "webglRenderer", "audio", "clientRects", "fonts"}

func read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := omap.ParseJSON(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(*omap.Map)
	if !ok {
		return nil, fmt.Errorf("fingerprint must be an object")
	}
	for _, k := range m.Keys() {
		if !contains(fields, k) {
			return nil, fmt.Errorf("%s: unexpected field", k)
		}
	}
	// the boolean options are optional and default to true
	c := &Config{Canvas: true, Webgl: true, Audio: true, ClientRects: true, Fonts: true}
	for _, f := range []struct {
		key string
		dst *bool
	}{{"canvas", &c.Canvas}, {"webgl", &c.Webgl}, {"audio", &c.Audio}, {"clientRects", &c.ClientRects}, {"fonts", &c.Fonts}} {
		if raw, ok := m.Get(f.key); ok {
			b, isBool := raw.(bool)
			if !isBool {
				return nil, fmt.Errorf("%s: must be a boolean", f.key)
			}
			*f.dst = b
		}
	}
	for _, f := range []struct {
		key string
		dst *string
	}{{"webglVendor", &c.WebglVendor}, {"webglRenderer", &c.WebglRenderer}} {
		raw, ok := m.Get(f.key)
		s, isStr := raw.(string)
		if !ok || !isStr {
			return nil, fmt.Errorf("%s: must be a string", f.key)
		}
		*f.dst = s
	}
	raw, _ := m.Get("seed")
	switch n := raw.(type) {
	case int:
		c.Seed = n
	case float64:
		if n != float64(int(n)) {
			return nil, fmt.Errorf("seed: must be an integer")
		}
		c.Seed = int(n)
	default:
		return nil, fmt.Errorf("seed: must be an integer")
	}
	return c, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Resolve is SessionManager._resolve_fingerprint. arg is nil, a bool, a *Config
// or a *scenario.FingerprintOverrides; profileDir may be empty. A nil result
// means fingerprinting is disabled.
func Resolve(profileDir string, arg any, platform Platform) (*Config, error) {
	if b, ok := arg.(bool); ok && !b {
		return nil, nil
	}
	if c, ok := arg.(*Config); ok {
		return c, nil
	}
	var base *Config
	if profileDir != "" {
		var err error
		if base, err = Load(profileDir, platform); err != nil {
			return nil, err
		}
	}
	o, ok := arg.(*scenario.FingerprintOverrides)
	if !ok {
		return base, nil
	}
	if base == nil {
		base = Generate(o.Seed, platform)
	}
	return Merge(base, o), nil
}
