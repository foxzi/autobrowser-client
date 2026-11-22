// Package cli is the autobrowser command line (port of cli/app.py).
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
	"github.com/foxzi/autobrowser-client/go/internal/logging"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// Version is the version of the Python package this port follows.
const Version = "0.5.4"

// env is what a command may use: output streams and the arguments after its name.
type env struct {
	stdout, stderr io.Writer
	args           []string
}

type command struct {
	name, help string
	run        func(*env) error
}

// commands is in help order; new commands (run-script, ...) are added here.
var commands []command

func init() {
	commands = []command{
		{"run-script", "Run a YAML/JSON scenario and print the step results as JSON.", runScript},
		{"show-config", "Print the effective configuration as JSON (secrets masked).", showConfig},
		{"version", "Show the version.", versionCmd},
		{"help", "Show help for autobrowser or one of its commands.", helpCmd},
	}
}

// usageError is a wrong command line: exit code 2 like Typer.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	switch args[0] {
	case "--version", "-V":
		fmt.Fprintln(stdout, Version)
		return 0
	case "--help", "-h":
		printUsage(stdout)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			e.args = args[1:]
			return exitCode(c.run(e), stderr)
		}
	}
	fmt.Fprintf(stderr, "Error: No such command '%s'.\n", args[0])
	return 2
}

func exitCode(err error, stderr io.Writer) int {
	var ae *apperr.Error
	var ue *usageError
	var es *exitStatus
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	case errors.As(err, &es):
		return es.code
	case errors.As(err, &ue):
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

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "autobrowser: a stealth browser for scripts and AI agents.")
	fmt.Fprintln(w, "\nUsage: autobrowser [--version|-V] COMMAND [OPTIONS]\n\nCommands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-14s %s\n", c.name, c.help)
	}
}

// newFlags makes a flag set for a command; a parse error is a usage error.
func newFlags(e *env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet("autobrowser "+name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return &usageError{err.Error()}
	}
	if fs.NArg() > 0 {
		return &usageError{"Got unexpected extra argument (" + fs.Arg(0) + ")"}
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

func helpCmd(e *env) error {
	if len(e.args) == 0 {
		printUsage(e.stdout)
		return nil
	}
	for _, c := range commands {
		if c.name == e.args[0] {
			c.run(&env{stdout: e.stdout, stderr: e.stderr, args: []string{"-h"}})
			return nil
		}
	}
	return &usageError{"Invalid value for COMMAND: No such command: " + e.args[0]}
}
