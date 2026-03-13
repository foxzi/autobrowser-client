"""Manage socks2http relay subprocess."""
from __future__ import annotations

import logging
import shutil
import socket
import subprocess
import time
from pathlib import Path

from autobrowser_client.config import ProxySettings
from autobrowser_client.errors import AppError, ErrorCode

logger = logging.getLogger(__name__)


class ProxyRelayError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.INTERNAL_ERROR, message)


def _find_free_port() -> int:
    """Find an available local TCP port."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _check_port_open(host: str, port: int, timeout: float = 5.0) -> bool:
    """Wait until a TCP port is accepting connections."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.5):
                return True
        except OSError:
            time.sleep(0.1)
    return False


class ProxyRelay:
    """Manages a socks2http subprocess that converts SOCKS5 proxy to local HTTP proxy."""

    def __init__(self, settings: ProxySettings) -> None:
        self.settings = settings
        self._process: subprocess.Popen | None = None  # type: ignore[type-arg]
        self.local_host: str = "127.0.0.1"
        self.local_port: int = settings.local_port or 0

    @property
    def http_proxy_url(self) -> str:
        """Return the local HTTP proxy URL for browser to use."""
        return f"http://{self.local_host}:{self.local_port}"

    def start(self) -> None:
        """Start socks2http relay subprocess."""
        proxy_url = self.settings.proxy_url()
        if not proxy_url:
            raise ProxyRelayError("Proxy host is not configured")

        bin_path = self.settings.socks2http_bin
        if not Path(bin_path).is_file():
            resolved = shutil.which("socks2http")
            if resolved:
                bin_path = resolved
            else:
                raise ProxyRelayError(f"socks2http binary not found: {bin_path}")

        if self.local_port == 0:
            self.local_port = _find_free_port()

        cmd = [
            bin_path,
            "--listen-host", self.local_host,
            "--listen-port", str(self.local_port),
            "--proxy-url", proxy_url,
            "--log-level", "WARNING",
        ]

        logger.info(
            "Starting socks2http relay: %s:%d -> %s",
            self.local_host,
            self.local_port,
            self.settings.proxy_url(),
        )

        self._process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

        if not _check_port_open(self.local_host, self.local_port, timeout=5.0):
            self.stop()
            raise ProxyRelayError(
                f"socks2http failed to start on {self.local_host}:{self.local_port}"
            )

        logger.info("socks2http relay is ready on %s:%d", self.local_host, self.local_port)

    def stop(self) -> None:
        """Stop the socks2http subprocess."""
        if self._process is None:
            return
        logger.info("Stopping socks2http relay (pid=%d)", self._process.pid)
        try:
            self._process.terminate()
            self._process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self._process.kill()
            self._process.wait(timeout=3)
        except Exception:
            pass
        self._process = None

    @property
    def is_running(self) -> bool:
        return self._process is not None and self._process.poll() is None
