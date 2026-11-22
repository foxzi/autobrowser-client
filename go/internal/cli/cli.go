// Package cli is the autobrowser command line (port of cli/app.py).
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/logging"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// Version is the version of the Python package this port follows; release
// builds set it with -ldflags "-X .../internal/cli.Version=X.Y.Z".
var Version = "0.5.4"

// env is what a command may use: output streams and the arguments after its name.
type env struct {
	stdout, stderr io.Writer
	args           []string
}

type command struct {
	name, group string
	help, long  string   // one-line summary; description and examples
	usage       string   // text after the command name in "Usage:", default "[OPTIONS]"
	args, opts  []option // for help
	run         func(*env) error
}

// commands is in help order; new commands are added here.
var commands []command

func init() {
	configOpt := option{"-c", "--config", "<file>", "YAML/JSON config file."}
	commands = []command{
		{name: "validate-script", group: groupScenarios, run: validateScript,
			help: "Check a YAML/JSON scenario without starting a browser.",
			long: "autobrowser validate-script -f examples/login.yaml",
			opts: []option{{"-f", "--file", "<file>", "Scenario file. [required]"}}},
		{name: "run-script", group: groupScenarios, run: runScript,
			help: "Run a YAML/JSON scenario and print the step results as JSON.",
			long: "Command-line options override the session block of the scenario.\n" +
				"Exit code is 0 when all steps succeed, 1 otherwise.\n\n" +
				"autobrowser run-script -f examples/simple.yaml\n" +
				"autobrowser run-script -f login.yaml -p work --proxy socks5://127.0.0.1:1080\n" +
				"autobrowser run-script -f login.yaml --var user=me --var password=secret",
			opts: []option{
				{"-f", "--file", "<file>", "Scenario file. [required]"},
				configOpt,
				{"-p", "--profile", "<str>", "Persistent profile name."},
				{"", "--proxy", "<str>", "Proxy URL: socks5://user:pass@host:port."},
				{"", "--viewport", "<str>", "Viewport size WIDTHxHEIGHT, e.g. 1920x1080."},
				{"-o", "--output", "<path>", "Save the JSON result to a file."},
				{"", "--preserve-session", "", "Save open tabs for the next session."},
				{"", "--var", "<str>", "Scenario variable NAME=VALUE; repeat for more."},
				{"", "--artifacts", "<path>", "Directory for screenshot and HTML of failed steps."},
			}},
		serveCommand,
		{name: "install-skill", group: groupAgents, run: installSkill,
			help: "Install the agent skill into DIR/autobrowser/SKILL.md.",
			long: "autobrowser install-skill\nautobrowser install-skill --dir ~/.config/nemo/skills",
			opts: []option{{"", "--dir", "<path>", "Skills directory of the agent client. [default: ~/.claude/skills]"}}},
		{name: "update", group: groupSetup, run: update,
			help: "Update the binary to the latest GitHub release.",
			long: "The new binary is checked against its .sha256 and replaces the current one.\n" +
				"Set AUTOBROWSER_NO_AUTO_UPDATE=1 to turn off the background check of mcp and serve.\n" +
				"Not for builds from source.",
			opts: []option{{"", "--check", "", "Only report whether an update exists"}}},
		{name: "generate-fingerprint", group: groupSetup, run: generateFingerprint,
			help: "Generate fingerprint.json for a browser profile.",
			long: "autobrowser generate-fingerprint --profile profiles/work --platform linux",
			opts: []option{
				{"", "--profile", "<str>", "Profile directory path [required]"},
				{"", "--platform", "<str>", "Target platform: windows, linux, android [default: windows]"},
				{"", "--seed", "<int>", "Fixed seed value"},
				{"", "--force", "", "Overwrite existing fingerprint.json"},
			}},
		{name: "help", group: groupInfo, run: helpCmd, usage: "[OPTIONS] [command]...",
			help: "Show help for autobrowser or one of its commands.",
			long: "autobrowser help\nautobrowser help run-script",
			args: []option{{"", "command", "<str>", "Command to describe, e.g. run-script."}}},
		{name: "version", group: groupInfo, run: versionCmd, help: "Show the version."},
		{name: "show-config", group: groupInfo, run: showConfig,
			help: "Print the effective configuration as JSON (secrets masked).",
			long: "Defaults, then the --config file, then AUTOBROWSER_* variables.",
			opts: []option{configOpt}},
	}
}

// usageError is a wrong command line: exit code 2 like Typer.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 2
	}
	switch args[0] {
	case "--version", "-V":
		fmt.Fprintln(stdout, Version)
		return 0
	case "--help", "-h":
		printUsage(stdout)
		return 0
	}
	c := findCommand(args[0])
	if c == nil {
		fmt.Fprintf(stderr, "Usage: autobrowser [OPTIONS] COMMAND [ARGS]...\nTry 'autobrowser -h' for help.\n\nError: No such command '%s'.\n", args[0])
		return 2
	}
	if wantsHelp(args[1:]) {
		printCommandHelp(stdout, c)
		return 0
	}
	// without a configured logger Python shows only warnings
	_ = logging.Configure("WARNING", "")
	return exitCode(c.run(&env{stdout: stdout, stderr: stderr, args: args[1:]}), c, stderr)
}

func exitCode(err error, c *command, stderr io.Writer) int {
	var ae *apperr.Error
	var ue *usageError
	var es *exitStatus
	switch {
	case err == nil:
		return 0
	case errors.As(err, &es):
		return es.code
	case errors.As(err, &ue):
		fmt.Fprint(stderr, hint(c))
		fmt.Fprintln(stderr, "Error: "+ue.msg)
		return 2
	case errors.As(err, &ae):
		fmt.Fprintln(stderr, omap.Marshal(omap.New("success", false,
			"error", omap.New("code", string(ae.Code), "message", ae.Message)), 0))
		return 1
	}
	fmt.Fprintln(stderr, err)
	return 1
}

// newFlags makes a flag set for a command; errors are reported by the caller.
func newFlags(e *env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet("autobrowser "+name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// flagName is a flag name as typed: -c or --config.
func flagName(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

// parse parses the options; a parse error is a usage error worded like Click.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		msg := err.Error()
		if name, ok := strings.CutPrefix(msg, "flag provided but not defined: -"); ok {
			msg = "No such option: " + flagName(name)
		} else if name, ok := strings.CutPrefix(msg, "flag needs an argument: -"); ok {
			msg = "Option '" + flagName(name) + "' requires an argument."
		}
		return &usageError{msg}
	}
	if fs.NArg() > 0 {
		return &usageError{"Got unexpected extra argument(s) (" + strings.Join(fs.Args(), " ") + ")"}
	}
	return nil
}

// configFlag registers --config / -c.
func configFlag(fs *flag.FlagSet) *string {
	var path string
	fs.StringVar(&path, "config", "", "YAML/JSON config file.")
	fs.StringVar(&path, "c", "", "YAML/JSON config file (short).")
	return &path
}

// settingsFromOptions loads the settings and configures logging
// (_settings_from_options); overrides are the CLI option overrides.
func settingsFromOptions(configPath string, overrides map[string]any) (*config.Settings, error) {
	s, err := config.LoadSettings(configPath, overrides, nil)
	if err != nil {
		return nil, err
	}
	file := ""
	if s.Logging.File != nil {
		file = *s.Logging.File
	}
	if err := logging.Configure(s.Logging.Level, file); err != nil {
		return nil, err
	}
	return s, nil
}

func showConfig(e *env) error {
	fs := newFlags(e, "show-config")
	path := configFlag(fs)
	if err := parse(fs, e.args); err != nil {
		return err
	}
	s, err := settingsFromOptions(*path, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, omap.Marshal(s.SafeDump(), 2))
	return nil
}

func versionCmd(e *env) error {
	if err := parse(newFlags(e, "version"), e.args); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, Version)
	return nil
}
