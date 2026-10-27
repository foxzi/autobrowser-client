"""Built-in local HTTP proxy that forwards to an upstream SOCKS5 or HTTP proxy with auth.

Chromium cannot authenticate to SOCKS5 proxies, so the browser talks to this relay
over plain HTTP (CONNECT for HTTPS) and the relay does the upstream handshake.
"""

from __future__ import annotations

import base64
import logging
import select
import socket
import socketserver
import struct
import threading
from urllib.parse import urlsplit

from autobrowser_client.config import ProxySettings

logger = logging.getLogger(__name__)

CONNECT_TIMEOUT = 30.0
MAX_HEAD = 65536


class UpstreamError(Exception):
    pass


def _recv_exact(sock: socket.socket, n: int) -> bytes:
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise UpstreamError("upstream closed connection")
        buf += chunk
    return buf


def _read_head(sock: socket.socket) -> tuple[bytes, bytes]:
    """Read until the end of HTTP headers; return (head, rest of the buffer)."""
    buf = b""
    while b"\r\n\r\n" not in buf:
        chunk = sock.recv(4096)
        if not chunk:
            raise UpstreamError("connection closed before headers")
        buf += chunk
        if len(buf) > MAX_HEAD:
            raise UpstreamError("headers too large")
    head, rest = buf.split(b"\r\n\r\n", 1)
    return head, rest


def _socks5_connect(s: ProxySettings, host: str, port: int) -> socket.socket:
    sock = socket.create_connection((s.host, s.port or 1080), timeout=CONNECT_TIMEOUT)
    try:
        methods = b"\x00\x02" if s.username else b"\x00"
        sock.sendall(b"\x05" + bytes([len(methods)]) + methods)
        ver, method = _recv_exact(sock, 2)
        if ver != 5:
            raise UpstreamError("not a SOCKS5 proxy")
        if method == 2:
            user = (s.username or "").encode()
            pwd = (s.password or "").encode()
            sock.sendall(b"\x01" + bytes([len(user)]) + user + bytes([len(pwd)]) + pwd)
            if _recv_exact(sock, 2)[1] != 0:
                raise UpstreamError("SOCKS5 authentication failed")
        elif method != 0:
            raise UpstreamError("SOCKS5 proxy rejected auth methods")

        name = host.encode("idna")
        sock.sendall(
            b"\x05\x01\x00\x03" + bytes([len(name)]) + name + struct.pack(">H", port)
        )
        _, rep, _, atyp = _recv_exact(sock, 4)
        if rep != 0:
            raise UpstreamError(f"SOCKS5 connect failed, code {rep}")
        addr_len = {1: 4, 4: 16}.get(atyp) or _recv_exact(sock, 1)[0]
        _recv_exact(sock, addr_len + 2)
    except BaseException:
        sock.close()
        raise
    sock.settimeout(None)
    return sock


def _proxy_auth_header(s: ProxySettings) -> bytes:
    if not s.username:
        return b""
    token = base64.b64encode(f"{s.username}:{s.password or ''}".encode()).decode()
    return f"Proxy-Authorization: Basic {token}\r\n".encode()


def _http_connect(s: ProxySettings, host: str, port: int) -> socket.socket:
    sock = socket.create_connection((s.host, s.port or 8080), timeout=CONNECT_TIMEOUT)
    try:
        target = f"{host}:{port}"
        sock.sendall(
            f"CONNECT {target} HTTP/1.1\r\nHost: {target}\r\n".encode()
            + _proxy_auth_header(s)
            + b"\r\n"
        )
        head, _ = _read_head(sock)
        status = head.split(b"\r\n", 1)[0].split()
        if len(status) < 2 or status[1] != b"200":
            raise UpstreamError(
                f"upstream CONNECT failed: {head.splitlines()[0].decode(errors='replace')}"
            )
    except BaseException:
        sock.close()
        raise
    sock.settimeout(None)
    return sock


def _pipe(a: socket.socket, b: socket.socket) -> None:
    socks = [a, b]
    while True:
        readable, _, _ = select.select(socks, [], [], 300)
        if not readable:
            return
        for src in readable:
            data = src.recv(65536)
            if not data:
                return
            (b if src is a else a).sendall(data)


def _strip_headers(lines: list[bytes], names: set[bytes]) -> list[bytes]:
    return [ln for ln in lines if ln.split(b":", 1)[0].strip().lower() not in names]


class _Handler(socketserver.BaseRequestHandler):
    server: "_Server"  # type: ignore[assignment]

    def handle(self) -> None:
        client: socket.socket = self.request
        upstream = None
        try:
            head, rest = _read_head(client)
            lines = head.split(b"\r\n")
            method, target, version = lines[0].split(b" ", 2)
            s = self.server.settings

            if method == b"CONNECT":
                host, _, port = target.decode().rpartition(":")
                upstream = self.server.open(host.strip("[]"), int(port))
                client.sendall(b"HTTP/1.1 200 Connection established\r\n\r\n")
                if rest:
                    upstream.sendall(rest)
            else:
                url = urlsplit(target.decode())
                if url.scheme != "http" or not url.hostname:
                    client.sendall(
                        b"HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n"
                    )
                    return
                headers = _strip_headers(
                    lines[1:],
                    {b"proxy-connection", b"connection", b"proxy-authorization"},
                )
                headers.append(b"Connection: close")
                if s.scheme == "socks5":
                    upstream = _socks5_connect(s, url.hostname, url.port or 80)
                    path = url.path or "/"
                    if url.query:
                        path += "?" + url.query
                    first = b" ".join([method, path.encode(), version])
                else:
                    upstream = socket.create_connection(
                        (s.host, s.port or 8080), timeout=CONNECT_TIMEOUT
                    )
                    upstream.settimeout(None)
                    auth = _proxy_auth_header(s).rstrip(b"\r\n")
                    if auth:
                        headers.append(auth)
                    first = lines[0]
                upstream.sendall(b"\r\n".join([first, *headers]) + b"\r\n\r\n" + rest)
            _pipe(client, upstream)
        except (UpstreamError, OSError, ValueError) as e:
            logger.warning("Proxy relay request failed: %s", e)
            try:
                client.sendall(
                    b"HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n"
                )
            except OSError:
                pass
        finally:
            if upstream:
                upstream.close()


class _Server(socketserver.ThreadingTCPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, addr: tuple[str, int], settings: ProxySettings) -> None:
        self.settings = settings
        super().__init__(addr, _Handler)

    def open(self, host: str, port: int) -> socket.socket:
        if self.settings.scheme == "socks5":
            return _socks5_connect(self.settings, host, port)
        return _http_connect(self.settings, host, port)


class BuiltinRelay:
    """Runs the relay server in a background thread."""

    def __init__(self, settings: ProxySettings, host: str, port: int) -> None:
        if settings.scheme not in ("socks5", "http"):
            raise ValueError(
                f"Unsupported proxy scheme for built-in relay: {settings.scheme}"
            )
        self._server = _Server((host, port), settings)
        self.port = self._server.server_address[1]
        self._thread = threading.Thread(
            target=self._server.serve_forever, daemon=True, name="proxy-relay"
        )

    def start(self) -> None:
        self._thread.start()

    def stop(self) -> None:
        self._server.shutdown()
        self._server.server_close()

    @property
    def is_running(self) -> bool:
        return self._thread.is_alive()
