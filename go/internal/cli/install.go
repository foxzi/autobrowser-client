package cli

import (
	"fmt"

	"github.com/foxzi/autobrowser-client/go/internal/cloak"
)

var installCommand = command{name: "install", group: groupSetup, run: installCmd,
	help: "Download the browser for the configured engine.",
	long: "The Go version supports only cloakbrowser, which downloads its own Chromium build.\n\n" +
		"AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser install",
	opts: []option{{"-c", "--config", "<file>", "YAML/JSON config file."}}}

func installCmd(e *env) error {
	fs := newFlags(e, "install")
	cfg := configFlag(fs)
	if err := parse(fs, e.args); err != nil {
		return err
	}
	settings, err := settingsFromOptions(*cfg, nil)
	if err != nil {
		return err
	}
	// patchright (Google Chrome) is not supported: the same error as starting a browser
	if err := settings.Validate(); err != nil {
		return err
	}
	path, err := cloak.EnsureBinary()
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, path)
	return nil
}
