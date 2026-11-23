// Package proxy ports autobrowser_client/proxy: a local HTTP proxy that
// forwards to an upstream SOCKS5 or HTTP proxy with credentials, because
// Chromium cannot authenticate to SOCKS5 proxies.
package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
)

// relayError is ProxyRelayError.
func relayError(format string, args ...any) *apperr.Error {
	return apperr.New(apperr.InternalErr, fmt.Sprintf(format, args...))
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// checkPortOpen waits until a TCP port is accepting connections.
func checkPortOpen(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
			c.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// startTimeout is how long Start waits for socks2http to accept connections.
var startTimeout = 5 * time.Second

// Relay manages the local proxy: an external socks2http process when one is
// configured or found in PATH, otherwise the built-in relay.
type Relay struct {
	settings  config.ProxySettings
	localHost string
	localPort int
	cmd       *exec.Cmd
	exited    chan struct{}
	builtin   *builtinRelay
}

// NewRelay is ProxyRelay(settings).
func NewRelay(settings config.ProxySettings) *Relay {
	return &Relay{settings: settings, localHost: "127.0.0.1", localPort: settings.LocalPort}
}

// HTTPProxyURL is the local HTTP proxy URL for the browser.
func (r *Relay) HTTPProxyURL() string {
	return fmt.Sprintf("http://%s:%d", r.localHost, r.localPort)
}

// LocalPort is the port the relay listens on (known after Start).
func (r *Relay) LocalPort() int { return r.localPort }

// Start launches socks2http (settings.Socks2httpBin or PATH) or the built-in relay.
func (r *Relay) Start() error {
	proxyURL := r.settings.ProxyURL()
	if proxyURL == "" {
		return relayError("Proxy host is not configured")
	}

	bin := r.settings.Socks2httpBin
	if bin != "" {
		if st, err := os.Stat(bin); err != nil || st.IsDir() {
			return relayError("socks2http binary not found: %s", bin)
		}
	} else if p, err := exec.LookPath("socks2http"); err == nil {
		bin = p
	}
	if bin == "" {
		return r.startBuiltin()
	}

	if r.localPort == 0 {
		port, err := findFreePort()
		if err != nil {
			return relayError("%s", err)
		}
		r.localPort = port
	}

	slog.Info(fmt.Sprintf("Starting socks2http relay: %s:%d -> %s", r.localHost, r.localPort, r.maskedProxyURL()))
	cmd := exec.Command(bin,
		"--listen-host", r.localHost,
		"--listen-port", strconv.Itoa(r.localPort),
		"--proxy-url", proxyURL,
		"--log-level", "WARNING")
	if err := cmd.Start(); err != nil {
		return relayError("%s", err)
	}
	r.cmd = cmd
	r.exited = make(chan struct{})
	go func() {
		cmd.Wait()
		close(r.exited)
	}()

	if !checkPortOpen(r.localHost, r.localPort, startTimeout) {
		r.Stop()
		return relayError("socks2http failed to start on %s:%d", r.localHost, r.localPort)
	}
	slog.Info(fmt.Sprintf("socks2http relay is ready on %s:%d", r.localHost, r.localPort))
	return nil
}

// maskedProxyURL is the upstream URL with the password hidden for logs.
func (r *Relay) maskedProxyURL() string {
	s := r.settings
	if password(s) != "" {
		stars := "***"
		s.Password = &stars
	}
	return s.ProxyURL()
}

func (r *Relay) startBuiltin() error {
	b, err := newBuiltinRelay(r.settings, r.localHost, r.localPort)
	if err != nil {
		return relayError("built-in proxy relay failed to start: %s", err)
	}
	r.localPort = b.port
	b.start()
	r.builtin = b
	port := "None"
	if r.settings.Port != nil {
		port = strconv.Itoa(*r.settings.Port)
	}
	host := "None"
	if r.settings.Host != nil {
		host = *r.settings.Host
	}
	slog.Info(fmt.Sprintf("Built-in proxy relay is ready on %s:%d -> %s:%s", r.localHost, r.localPort, host, port))
	return nil
}

// Stop stops the relay. It is safe to call more than once.
func (r *Relay) Stop() {
	if r.builtin != nil {
		r.builtin.stop()
		r.builtin = nil
	}
	if r.cmd == nil {
		return
	}
	slog.Info(fmt.Sprintf("Stopping socks2http relay (pid=%d)", r.cmd.Process.Pid))
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		r.cmd.Process.Kill() // SIGTERM is not deliverable on Windows
	}
	select {
	case <-r.exited:
	case <-time.After(5 * time.Second):
		r.cmd.Process.Kill()
		select {
		case <-r.exited:
		case <-time.After(3 * time.Second):
		}
	}
	r.cmd = nil
}

// IsRunning reports whether the relay is up.
func (r *Relay) IsRunning() bool {
	if r.builtin != nil {
		return r.builtin.isRunning()
	}
	if r.cmd == nil {
		return false
	}
	select {
	case <-r.exited:
		return false
	default:
		return true
	}
}
