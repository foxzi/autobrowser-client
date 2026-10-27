"""Built-in proxy relay against fake upstream SOCKS5 and HTTP proxies."""

from __future__ import annotations

import base64
import http.server
import socket
import socketserver
import struct
import threading
import urllib.request

import pytest

from autobrowser_client.config import ProxySettings
from autobrowser_client.proxy import relay as relay_mod
from autobrowser_client.proxy.builtin import _pipe, _read_head, _recv_exact
from autobrowser_client.proxy.relay import ProxyRelay


def _serve(server: socketserver.BaseServer) -> socketserver.BaseServer:
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


class _Target(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        body = f"ok {self.path}".encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args) -> None:
        pass


class _Socks5(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        c = self.request
        _, n = _recv_exact(c, 2)
        assert 2 in _recv_exact(c, n)
        c.sendall(b"\x05\x02")
        _, ulen = _recv_exact(c, 2)
        user = _recv_exact(c, ulen)
        plen = _recv_exact(c, 1)[0]
        pwd = _recv_exact(c, plen)
        if (user, pwd) != (b"u", b"p"):
            c.sendall(b"\x01\x01")
            return
        c.sendall(b"\x01\x00")
        _, _, _, atyp = _recv_exact(c, 4)
        assert atyp == 3
        host = _recv_exact(c, _recv_exact(c, 1)[0]).decode()
        port = struct.unpack(">H", _recv_exact(c, 2))[0]
        up = socket.create_connection((host, port))
        c.sendall(b"\x05\x00\x00\x01" + b"\x00" * 6)
        _pipe(c, up)
        up.close()


class _HttpProxy(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        c = self.request
        head, rest = _read_head(c)
        expected = base64.b64encode(b"u:p")
        if expected not in head:
            c.sendall(b"HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
            return
        method, target, _ = head.split(b"\r\n", 1)[0].split(b" ", 2)
        if method == b"CONNECT":
            host, port = target.decode().rsplit(":", 1)
            up = socket.create_connection((host, int(port)))
            c.sendall(b"HTTP/1.1 200 OK\r\n\r\n")
        else:
            host_port = target.decode().split("/")[2]
            host, port = host_port.rsplit(":", 1)
            up = socket.create_connection((host, int(port)))
            up.sendall(head + b"\r\n\r\n" + rest)
        _pipe(c, up)
        up.close()


class _TS(socketserver.ThreadingTCPServer):
    daemon_threads = True
    allow_reuse_address = True


@pytest.fixture
def target():
    srv = _serve(http.server.ThreadingHTTPServer(("127.0.0.1", 0), _Target))
    yield srv.server_address[1]
    srv.shutdown()


@pytest.fixture(params=["socks5", "http"])
def upstream(request):
    handler = _Socks5 if request.param == "socks5" else _HttpProxy
    srv = _serve(_TS(("127.0.0.1", 0), handler))
    yield request.param, srv.server_address[1]
    srv.shutdown()


@pytest.fixture
def no_socks2http(monkeypatch):
    monkeypatch.setattr(relay_mod.shutil, "which", lambda _: None)


def _relay(scheme: str, port: int, password: str = "p") -> ProxyRelay:
    settings = ProxySettings.model_validate(
        {
            "enabled": True,
            "scheme": scheme,
            "host": "127.0.0.1",
            "port": port,
            "username": "u",
            "password": password,
        }
    )
    r = ProxyRelay(settings)
    r.start()
    return r


def test_plain_http(no_socks2http, upstream, target):
    r = _relay(*upstream)
    try:
        assert r.is_running
        opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({"http": r.http_proxy_url})
        )
        body = opener.open(f"http://localhost:{target}/a?b=1", timeout=5).read()
        assert body.startswith(b"ok ") and body.endswith(b"/a?b=1")
    finally:
        r.stop()
    assert not r.is_running


def test_connect_tunnel(no_socks2http, upstream, target):
    r = _relay(*upstream)
    try:
        with socket.create_connection(("127.0.0.1", r.local_port), timeout=5) as s:
            s.sendall(
                f"CONNECT localhost:{target} HTTP/1.1\r\nHost: localhost:{target}\r\n\r\n".encode()
            )
            head, _ = _read_head(s)
            assert head.startswith(b"HTTP/1.1 200")
            s.sendall(
                b"GET /t HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
            )
            data = b""
            while chunk := s.recv(4096):
                data += chunk
            assert data.endswith(b"ok /t")
    finally:
        r.stop()


def test_bad_credentials_give_502(no_socks2http, upstream, target):
    r = _relay(*upstream, password="wrong")
    try:
        with socket.create_connection(("127.0.0.1", r.local_port), timeout=5) as s:
            s.sendall(f"CONNECT localhost:{target} HTTP/1.1\r\n\r\n".encode())
            head, _ = _read_head(s)
            assert head.startswith(b"HTTP/1.1 502")
    finally:
        r.stop()
