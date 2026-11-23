// Package browser launches the cloakbrowser Chromium over CDP (go-rod) and
// runs scenario steps on it. It ports browser/service.py (cloakbrowser branch),
// browser/executor.py and browser/actions.py.
package browser

import (
	"math/rand"
	"runtime"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/cloak"
)

// findBinary is cloakbrowser ensure_binary (downloads the browser when it is
// missing); every failure is a start error.
func findBinary() (string, error) {
	p, err := cloak.EnsureBinary()
	if err != nil {
		return "", apperr.New(apperr.BrowserStartFailed, err.Error())
	}
	return p, nil
}

// IgnoreDefaultArgs are the Playwright defaults cloakbrowser drops. Both are
// absent from the argument list built here, so nothing needs removing.
var IgnoreDefaultArgs = []string{"--enable-automation", "--enable-unsafe-swiftshader"}

// BuildArgs is cloakbrowser build_args(stealth_args=True, extra_args=extra,
// headless=headless, timezone=timezone, locale=locale): defaults < user args <
// timezone/locale, deduplicated by the text before "=". The --fingerprint seed
// is random per call.
func BuildArgs(extra []string, headless bool, timezone, locale string) []string {
	var keys []string
	seen := map[string]string{}
	set := func(arg string) {
		key, _, _ := strings.Cut(arg, "=")
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
		seen[key] = arg
	}
	fpPlatform := "windows"
	if runtime.GOOS == "darwin" {
		fpPlatform = "macos"
	}
	set("--no-sandbox")
	set("--fingerprint=" + strconv.Itoa(10000+rand.Intn(90000)))
	set("--fingerprint-platform=" + fpPlatform)
	if !headless || runtime.GOOS == "windows" {
		set("--ignore-gpu-blocklist")
	}
	for _, a := range extra {
		set(a)
	}
	if _, ok := seen["--fingerprint-windows-font-metrics"]; ok {
		var features []string
		if cur, ok := seen["--enable-features"]; ok {
			_, v, _ := strings.Cut(cur, "=")
			for _, f := range strings.Split(v, ",") {
				if f != "" {
					features = append(features, f)
				}
			}
		}
		if !contains(features, "MediaRouter") {
			set("--enable-features=" + strings.Join(append(features, "MediaRouter"), ","))
		}
	}
	if timezone != "" {
		set("--fingerprint-timezone=" + timezone)
	}
	if locale != "" {
		set("--lang=" + locale)
		set("--fingerprint-locale=" + locale)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
