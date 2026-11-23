package proxy

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/config"
)

// serve accepts connections on a loopback port and runs handler on each.
func serve(t *testing.T, handler func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				handler(c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func mustRead(c net.Conn, n int) []byte {
	b, err := recvExact(c, n)
	if err != nil {
		panic(err)
	}
	return b
}

// fakeSocks5 requires user/password auth u:p and connects to any target.
func fakeSocks5(c net.Conn) {
	n := mustRead(c, 2)[1]
	hasAuth := false
	for _, m := range mustRead(c, int(n)) {
		hasAuth = hasAuth || m == 2
	}
	if !hasAuth {
		return
	}
	c.Write([]byte{5, 2})
	user := mustRead(c, int(mustRead(c, 2)[1]))
	pwd := mustRead(c, int(mustRead(c, 1)[0]))
	if string(user) != "u" || string(pwd) != "p" {
		c.Write([]byte{1, 1})
		return
	}
	c.Write([]byte{1, 0})
	if mustRead(c, 4)[3] != 3 {
		return
	}
	host := string(mustRead(c, int(mustRead(c, 1)[0])))
	port := binary.BigEndian.Uint16(mustRead(c, 2))
	up, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		return
	}
	defer up.Close()
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	pipe(c, up)
}

// fakeHTTPProxy requires Basic u:p and forwards CONNECT and absolute-URI requests.
func fakeHTTPProxy(c net.Conn) {
	head, rest, err := readHead(c)
	if err != nil {
		return
	}
	if !strings.Contains(string(head), base64.StdEncoding.EncodeToString([]byte("u:p"))) {
		c.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
		return
	}
	first, _, _ := strings.Cut(string(head), "\r\n")
	parts := strings.SplitN(first, " ", 3)
	var up net.Conn
	if parts[0] == "CONNECT" {
		if up, err = net.Dial("tcp", parts[1]); err != nil {
			return
		}
		c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	} else {
		u, _ := url.Parse(parts[1])
		if up, err = net.Dial("tcp", u.Host); err != nil {
			return
		}
		up.Write(append([]byte(string(head)+"\r\n\r\n"), rest...))
	}
	defer up.Close()
	pipe(c, up)
}

func target(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok %s", r.URL.RequestURI())
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

func startRelay(t *testing.T, scheme string, port int, pwd string) *Relay {
	t.Helper()
	host, user := "127.0.0.1", "u"
	r := NewRelay(config.ProxySettings{
		Enabled: true, Scheme: scheme, Host: &host, Port: &port, Username: &user, Password: &pwd,
	})
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	return r
}

func forEachUpstream(t *testing.T, fn func(t *testing.T, scheme string, port int)) {
	for scheme, h := range map[string]func(net.Conn){"socks5": fakeSocks5, "http": fakeHTTPProxy} {
		t.Run(scheme, func(t *testing.T) { fn(t, scheme, serve(t, h)) })
	}
}

func TestPlainHTTP(t *testing.T) {
	tp := target(t)
	forEachUpstream(t, func(t *testing.T, scheme string, port int) {
		r := startRelay(t, scheme, port, "p")
		if !r.IsRunning() {
			t.Fatal("relay is not running")
		}
		proxyURL, _ := url.Parse(r.HTTPProxyURL())
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/a?b=1", tp))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "ok /a?b=1" {
			t.Fatalf("body = %q", body)
		}
		r.Stop()
		if r.IsRunning() {
			t.Fatal("relay is still running after Stop")
		}
	})
}

func TestConnectTunnel(t *testing.T) {
	tp := target(t)
	forEachUpstream(t, func(t *testing.T, scheme string, port int) {
		r := startRelay(t, scheme, port, "p")
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", r.LocalPort()), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(c, "CONNECT localhost:%d HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", tp, tp)
		head, _, err := readHead(c)
		if err != nil || !strings.HasPrefix(string(head), "HTTP/1.1 200") {
			t.Fatalf("head = %q, err = %v", head, err)
		}
		c.Write([]byte("GET /t HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
		data, _ := io.ReadAll(c)
		if !strings.HasSuffix(string(data), "ok /t") {
			t.Fatalf("data = %q", data)
		}
	})
}

func TestBadCredentialsGive502(t *testing.T) {
	tp := target(t)
	forEachUpstream(t, func(t *testing.T, scheme string, port int) {
		r := startRelay(t, scheme, port, "wrong")
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", r.LocalPort()), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(c, "CONNECT localhost:%d HTTP/1.1\r\n\r\n", tp)
		head, _, err := readHead(c)
		if err != nil || !strings.HasPrefix(string(head), "HTTP/1.1 502") {
			t.Fatalf("head = %q, err = %v", head, err)
		}
	})
}

func TestNonProxyRequestGives400(t *testing.T) {
	r := startRelay(t, "http", serve(t, fakeHTTPProxy), "p")
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", r.LocalPort()), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	c.Write([]byte("GET /plain HTTP/1.1\r\nHost: x\r\n\r\n"))
	head, _, err := readHead(c)
	if err != nil || !strings.HasPrefix(string(head), "HTTP/1.1 400") {
		t.Fatalf("head = %q, err = %v", head, err)
	}
}

func TestRawPathQuery(t *testing.T) {
	for in, want := range map[string]string{
		"http://h":            "/",
		"http://h?x=1":        "/?x=1",
		"http://h:80/a%20b?c": "/a%20b?c",
		"http://h/a?":         "/a",
		"http://h/a#frag":     "/a",
	} {
		if got := rawPathQuery(in); got != want {
			t.Errorf("rawPathQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStartErrors(t *testing.T) {
	host := "127.0.0.1"
	check := func(s config.ProxySettings, want string) {
		t.Helper()
		err := NewRelay(s).Start()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Start() error = %v, want %q", err, want)
		}
	}
	check(config.ProxySettings{Scheme: "socks5"}, "Proxy host is not configured")
	check(config.ProxySettings{Scheme: "socks5", Host: &host, Socks2httpBin: "/nonexistent/socks2http"},
		"socks2http binary not found: /nonexistent/socks2http")
	t.Setenv("PATH", t.TempDir())
	check(config.ProxySettings{Scheme: "https", Host: &host},
		"built-in proxy relay failed to start: Unsupported proxy scheme for built-in relay: https")
}

func TestSocks2httpNotListeningFails(t *testing.T) {
	script := filepath.Join(t.TempDir(), "socks2http")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	host := "127.0.0.1"
	r := NewRelay(config.ProxySettings{Scheme: "socks5", Host: &host, Socks2httpBin: script})
	old := startTimeout
	startTimeout = 300 * time.Millisecond
	defer func() { startTimeout = old }()
	err := r.Start()
	if err == nil || !strings.Contains(err.Error(), "socks2http failed to start on 127.0.0.1:") {
		t.Fatalf("Start() error = %v", err)
	}
	if r.IsRunning() {
		t.Fatal("process still running after failed start")
	}
}
