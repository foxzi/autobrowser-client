package cli

import (
	"fmt"
	"io"
)

// Command groups, in help order (Typer panels).
const (
	groupScenarios = "Scenarios"
	groupAgents    = "AI agents"
	groupSetup     = "Setup"
	groupInfo      = "Info"
)

var groupOrder = []string{groupScenarios, groupAgents, groupSetup, groupInfo}

// option describes one option or argument for the help text.
type option struct {
	short, long, meta, help string
}

func (o option) names() string {
	n := o.long
	if o.short != "" {
		n = o.short + ", " + o.long
		if o.long == "" {
			n = o.short
		}
	}
	if o.meta != "" {
		n += " " + o.meta
	}
	return n
}

var helpOption = option{"-h", "--help", "", "Show this message and exit."}

const rootEnv = `Environment

  AUTOBROWSER_BROWSER_ENGINE   cloakbrowser (the only engine in the Go version)
  AUTOBROWSER_HEADLESS         true / false
  AUTOBROWSER_PROFILE_DIR      directory of persistent profiles
  AUTOBROWSER_PROXY_*          ENABLED, SCHEME, HOST, PORT, USERNAME, PASSWORD
  AUTOBROWSER_LOG_LEVEL        DEBUG, INFO, WARNING, ERROR
  AUTOBROWSER_NO_AUTO_UPDATE   1 turns off the binary self-update

Docs: https://github.com/foxzi/autobrowser-client
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: autobrowser [OPTIONS] COMMAND [ARGS]...

autobrowser: a stealth browser for scripts and AI agents.

Options:
  -V, --version  Show the version and exit.
  -h, --help     Show this message and exit.
`)
	for _, g := range groupOrder {
		fmt.Fprintf(w, "\n%s:\n", g)
		for _, c := range commands {
			if c.group == g {
				fmt.Fprintf(w, "  %-20s %s\n", c.name, c.help)
			}
		}
	}
	fmt.Fprintf(w, "\nExamples\n\n  autobrowser run-script --file examples/simple.yaml   run a scenario\n"+
		"  autobrowser help run-script                          help for a command\n\n%s", rootEnv)
}

// printCommandHelp is the help of one command (Usage, description, options).
func printCommandHelp(w io.Writer, c *command) {
	usage := c.usage
	if usage == "" {
		usage = "[OPTIONS]"
	}
	fmt.Fprintf(w, "Usage: autobrowser %s %s\n\n%s\n", c.name, usage, c.help)
	if c.long != "" {
		fmt.Fprintf(w, "\n%s\n", c.long)
	}
	section := func(title string, opts []option) {
		if len(opts) == 0 {
			return
		}
		width := 0
		for _, o := range opts {
			width = max(width, len(o.names()))
		}
		fmt.Fprintf(w, "\n%s:\n", title)
		for _, o := range opts {
			fmt.Fprintf(w, "  %-*s  %s\n", width, o.names(), o.help)
		}
	}
	section("Arguments", c.args)
	section("Options", append(append([]option(nil), c.opts...), helpOption))
}

func findCommand(name string) *command {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

func helpCmd(e *env) error {
	if len(e.args) == 0 {
		printUsage(e.stdout)
		return nil
	}
	c := findCommand(e.args[0])
	if c == nil {
		return &usageError{"Invalid value for COMMAND: No such command: " + e.args[0]}
	}
	printCommandHelp(e.stdout, c)
	return nil
}

// wantsHelp reports whether args ask for the command help (-h / --help).
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// hint is the usage line of an error message, as Typer prints it.
func hint(c *command) string {
	usage := c.usage
	if usage == "" {
		usage = "[OPTIONS]"
	}
	return fmt.Sprintf("Usage: autobrowser %s %s\nTry 'autobrowser %s -h' for help.\n\n", c.name, usage, c.name)
}
