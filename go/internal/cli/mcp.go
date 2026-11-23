package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/foxzi/autobrowser-client/go/internal/mcp"
	"github.com/foxzi/autobrowser-client/go/internal/updater"
)

var mcpCommand = command{name: "mcp", group: groupAgents, run: mcpCmd,
	help: "Run the MCP server over stdio for AI agents.",
	long: "Gives the agent browser_* tools: navigate, snapshot, click, type, tabs and more.\n" +
		"Register it in the client, e.g. Claude Code:\n\n" +
		"claude mcp add autobrowser -- autobrowser mcp",
	opts: []option{
		{"-c", "--config", "<file>", "YAML/JSON config file."},
		{"-p", "--profile", "<str>", "Persistent profile name."},
		{"", "--headless/--headed", "", "Hide or show the browser window."},
		{"", "--proxy", "<str>", "Proxy URL: socks5://user:pass@host:port."},
	}}

func mcpCmd(e *env) error {
	fs := newFlags(e, "mcp")
	cfg := configFlag(fs)
	var profile, proxy string
	var headless *bool
	fs.StringVar(&profile, "profile", "", "Persistent profile name.")
	fs.StringVar(&profile, "p", "", "Persistent profile name (short).")
	fs.BoolFunc("headless", "Hide the browser window.", func(string) error { v := true; headless = &v; return nil })
	fs.BoolFunc("headed", "Show the browser window.", func(string) error { v := false; headless = &v; return nil })
	fs.StringVar(&proxy, "proxy", "", "Proxy URL: socks5://user:pass@host:port.")
	if err := parse(fs, e.args); err != nil {
		return err
	}
	overrides := map[string]any{}
	if headless != nil {
		overrides["browser"] = map[string]any{"headless": *headless}
	}
	if proxy != "" {
		if err := applyProxyURL(proxy, overrides); err != nil {
			return err
		}
	}
	settings, err := settingsFromOptions(*cfg, overrides)
	if err != nil {
		return err
	}
	updater.StartAutoUpdate(Version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = mcp.New(settings, profile).Run(ctx, &sdk.StdioTransport{})
	if ctx.Err() != nil {
		return nil
	}
	return err
}
