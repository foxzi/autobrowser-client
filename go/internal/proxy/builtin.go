package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/config"
)

const (
	connectTimeout = 30 * time.Second
	pipeIdle       = 300 * time.Second
	maxHead        = 65536
)

// recvExact reads exactly n bytes.
func recvExact(c net.Conn, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(c, buf); err != nil {
		return nil, errors.New("upstream closed connection")
	}
	return buf, nil
}

// readHead reads until the end of the HTTP headers and returns the head and
// the rest of the buffer.
func readHead(c net.Conn) (head, rest []byte, err error) {
	var buf []byte
	chunk := make([]byte, 4096)
	for !bytes.Contains(buf, []byte("\r\n\r\n")) {
		n, rerr := c.Read(chunk)
		if n == 0 && rerr != nil {
			return nil, nil, errors.New("connection closed before headers")
		}
		buf = append(buf, chunk[:n]...)
		if len(buf) > maxHead {
			return nil, nil, errors.New("headers too large")
		}
	}
	head, rest, _ = bytes.Cut(buf, []byte("\r\n\r\n"))
	return head, rest, nil
}

// upstreamAddr is the upstream proxy address with Python's default ports.
func upstreamAddr(s config.ProxySettings, defPort int) string {
	host := ""
	if s.Host != nil {
		host = *s.Host
	}
	port := defPort
	if s.Port != nil && *s.Port != 0 {
		port = *s.Port
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func username(s config.ProxySettings) string {
	if s.Username == nil {
		return ""
	}
	return *s.Username
}

func password(s config.ProxySettings) string {
	if s.Password == nil {
		return ""
	}
	return *s.Password
}

func dialUpstream(s config.ProxySettings, defPort int) (net.Conn, error) {
	return net.DialTimeout("tcp", upstreamAddr(s, defPort), connectTimeout)
}

func socks5Connect(s config.ProxySettings, host string, port int) (_ net.Conn, err error) {
	conn, err := dialUpstream(s, 1080)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	conn.SetDeadline(time.Now().Add(connectTimeout))

	methods := []byte{0}
	if username(s) != "" {
		methods = []byte{0, 2}
	}
	if _, err = conn.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
		return nil, err
	}
	resp, err := recvExact(conn, 2)
	if err != nil {
		return nil, err
	}
	if resp[0] != 5 {
		return nil, fmt.Errorf("not a SOCKS5 proxy")
	}
	switch resp[1] {
	case 2:
		user, pwd := []byte(username(s)), []byte(password(s))
		if len(user) > 255 || len(pwd) > 255 {
			return nil, errors.New("SOCKS5 credentials are longer than 255 bytes")
		}
		msg := append([]byte{1, byte(len(user))}, user...)
		msg = append(append(msg, byte(len(pwd))), pwd...)
		if _, err = conn.Write(msg); err != nil {
			return nil, err
		}
		if resp, err = recvExact(conn, 2); err != nil {
			return nil, err
		}
		if resp[1] != 0 {
			return nil, fmt.Errorf("SOCKS5 authentication failed")
		}
	case 0:
	default:
		return nil, fmt.Errorf("SOCKS5 proxy rejected auth methods")
	}

	// Python encodes the name as idna; browsers always send ASCII names.
	if len(host) > 255 || strings.ContainsFunc(host, func(r rune) bool { return r > 127 }) {
		return nil, errors.New("host name is not a plain ASCII name of at most 255 bytes")
	}
	req := append([]byte{5, 1, 0, 3, byte(len(host))}, host...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err = conn.Write(req); err != nil {
		return nil, err
	}
	if resp, err = recvExact(conn, 4); err != nil {
		return nil, err
	}
	if resp[1] != 0 {
		return nil, fmt.Errorf("SOCKS5 connect failed, code %d", resp[1])
	}
	addrLen := 0
	switch resp[3] {
	case 1:
		addrLen = 4
	case 4:
		addrLen = 16
	default:
		b, err := recvExact(conn, 1)
		if err != nil {
			return nil, err
		}
		addrLen = int(b[0])
	}
	if _, err = recvExact(conn, addrLen+2); err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

// proxyAuthHeader is the Proxy-Authorization line with a trailing CRLF, or "".
func proxyAuthHeader(s config.ProxySettings) string {
	if username(s) == "" {
		return ""
	}
	token := base64.StdEncoding.EncodeToString([]byte(username(s) + ":" + password(s)))
	return "Proxy-Authorization: Basic " + token + "\r\n"
}

func httpConnect(s config.ProxySettings, host string, port int) (_ net.Conn, err error) {
	conn, err := dialUpstream(s, 8080)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	conn.SetDeadline(time.Now().Add(connectTimeout))

	target := net.JoinHostPort(host, strconv.Itoa(port))
	if strings.Contains(host, ":") {
		target = host + ":" + strconv.Itoa(port) // Python does not bracket IPv6
	}
	req := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n" + proxyAuthHeader(s) + "\r\n"
	if _, err = conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	head, _, err := readHead(conn)
	if err != nil {
		return nil, err
	}
	first, _, _ := strings.Cut(string(head), "\r\n")
	if f := strings.Fields(first); len(f) < 2 || f[1] != "200" {
		return nil, fmt.Errorf("upstream CONNECT failed: %s", first)
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

// pipe copies both ways until either side closes or both are idle for pipeIdle.
func pipe(a, b net.Conn) {
	touch := func() {
		t := time.Now().Add(pipeIdle)
		a.SetDeadline(t)
		b.SetDeadline(t)
	}
	touch()
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		buf := make([]byte, 65536)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
				touch()
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}
	go cp(b, a)
	go cp(a, b)
	<-done
	a.Close()
	b.Close()
	<-done
}

// stripHeaders drops header lines whose name is in names (lower case).
func stripHeaders(lines []string, names ...string) []string {
	var out []string
	for _, ln := range lines {
		name, _, _ := strings.Cut(ln, ":")
		drop := false
		for _, n := range names {
			if strings.ToLower(strings.TrimSpace(name)) == n {
				drop = true
			}
		}
		if !drop {
			out = append(out, ln)
		}
	}
	return out
}

// builtinRelay is the relay server running in the background (BuiltinRelay).
type builtinRelay struct {
	settings config.ProxySettings
	ln       net.Listener
	port     int
	done     chan struct{}
}

func newBuiltinRelay(s config.ProxySettings, host string, port int) (*builtinRelay, error) {
	if s.Scheme != "socks5" && s.Scheme != "http" {
		return nil, fmt.Errorf("Unsupported proxy scheme for built-in relay: %s", s.Scheme)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	return &builtinRelay{settings: s, ln: ln, port: ln.Addr().(*net.TCPAddr).Port, done: make(chan struct{})}, nil
}

func (b *builtinRelay) start() {
	go func() {
		defer close(b.done)
		var wg sync.WaitGroup
		for {
			c, err := b.ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.handle(c)
			}()
		}
	}()
}

func (b *builtinRelay) stop() {
	b.ln.Close()
	<-b.done
}

func (b *builtinRelay) isRunning() bool {
	select {
	case <-b.done:
		return false
	default:
		return true
	}
}

func (b *builtinRelay) open(host string, port int) (net.Conn, error) {
	if b.settings.Scheme == "socks5" {
		return socks5Connect(b.settings, host, port)
	}
	return httpConnect(b.settings, host, port)
}

func (b *builtinRelay) handle(client net.Conn) {
	defer client.Close()
	var upstream net.Conn
	piped := false
	err := func() error {
		head, rest, err := readHead(client)
		if err != nil {
			return err
		}
		lines := strings.Split(string(head), "\r\n")
		parts := strings.SplitN(lines[0], " ", 3)
		if len(parts) != 3 {
			return fmt.Errorf("malformed request line: %q", lines[0])
		}
		method, target, version := parts[0], parts[1], parts[2]
		s := b.settings

		if method == "CONNECT" {
			i := strings.LastIndex(target, ":")
			host, portStr := "", target
			if i >= 0 {
				host, portStr = target[:i], target[i+1:]
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid port in %q", target)
			}
			if upstream, err = b.open(strings.Trim(host, "[]"), port); err != nil {
				return err
			}
			if _, err = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
				return err
			}
			if len(rest) > 0 {
				if _, err = upstream.Write(rest); err != nil {
					return err
				}
			}
		} else {
			u, err := url.Parse(target)
			if err != nil {
				return err
			}
			if !strings.EqualFold(u.Scheme, "http") || u.Hostname() == "" {
				client.Write([]byte("HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n"))
				return nil
			}
			headers := stripHeaders(lines[1:], "proxy-connection", "connection", "proxy-authorization")
			headers = append(headers, "Connection: close")
			port := 80
			if p := u.Port(); p != "" {
				port, _ = strconv.Atoi(p)
			}
			var first string
			if s.Scheme == "socks5" {
				if upstream, err = socks5Connect(s, strings.ToLower(u.Hostname()), port); err != nil {
					return err
				}
				first = method + " " + rawPathQuery(target) + " " + version
			} else {
				if upstream, err = dialUpstream(s, 8080); err != nil {
					return err
				}
				upstream.SetDeadline(time.Time{})
				if auth := strings.TrimRight(proxyAuthHeader(s), "\r\n"); auth != "" {
					headers = append(headers, auth)
				}
				first = lines[0]
			}
			msg := strings.Join(append([]string{first}, headers...), "\r\n") + "\r\n\r\n"
			if _, err = upstream.Write(append([]byte(msg), rest...)); err != nil {
				return err
			}
		}
		piped = true
		pipe(client, upstream)
		return nil
	}()
	if upstream != nil {
		upstream.Close()
	}
	if err != nil && !piped {
		slog.Warn(fmt.Sprintf("Proxy relay request failed: %s", err))
		client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n"))
	}
}

// rawPathQuery is the undecoded path and query of an absolute http URL, "/" for an empty path.
func rawPathQuery(target string) string {
	rest := target[strings.Index(target, "://")+3:]
	i := strings.IndexAny(rest, "/?#")
	if i < 0 {
		return "/"
	}
	rest = rest[i:]
	rest, _, _ = strings.Cut(rest, "#")
	if strings.Count(rest, "?") == 1 {
		rest = strings.TrimSuffix(rest, "?")
	}
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return rest
}
