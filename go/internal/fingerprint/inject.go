package fingerprint

import (
	_ "embed"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// injectJS is a byte-identical copy of python/autobrowser_client/fingerprint/inject.js
// (go:embed cannot reach ../python; CI diffs the two).
//
//go:embed inject.js
var injectJS string

// InitScript is build_injection_script: the JS that sets window.__fpConfig and
// runs the spoof. Add it as a script evaluated on every new document.
func InitScript(c *Config) string {
	return "window.__fpConfig = " + omap.Marshal(c.JSConfig(), 0) + ";\n" + injectJS
}
