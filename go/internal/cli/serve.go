package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/foxzi/autobrowser-client/go/internal/api"
	"github.com/foxzi/autobrowser-client/go/internal/updater"
)

var serveCommand = command{name: "serve", group: groupAgents, run: serveCmd,
	help: "Start the HTTP API server (sessions, pages, scenarios).",
	long: "Set AUTOBROWSER_API_TOKEN to require a bearer token.\n\n" +
		"autobrowser serve --host 0.0.0.0 --port 8080",
	opts: []option{
		{"-c", "--config", "<file>", "YAML/JSON config file."},
		{"", "--host", "<str>", "Address to listen on. [default: 127.0.0.1]"},
		{"", "--port", "<int>", "Port to listen on. [default: 8080]"},
	}}

func serveCmd(e *env) error {
	fs := newFlags(e, "serve")
	cfg := configFlag(fs)
	host := fs.String("host", "127.0.0.1", "Address to listen on.")
	portArg := fs.String("port", "8080", "Port to listen on.")
	if err := parse(fs, e.args); err != nil {
		return err
	}
	port, err := strconv.Atoi(*portArg)
	if err != nil {
		return &usageError{fmt.Sprintf("Invalid value for '--port': '%s' is not a valid integer.", *portArg)}
	}
	settings, err := settingsFromOptions(*cfg, nil)
	if err != nil {
		return err
	}
	updater.StartAutoUpdate(Version)
	server := api.New(settings, Version)
	defer server.Close()

	ln, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: server}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	fmt.Fprintf(e.stderr, "Serving the API on http://%s (Press CTRL+C to quit)\n", ln.Addr())
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
