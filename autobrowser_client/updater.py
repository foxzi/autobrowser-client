"""Self-update of the single-file binary from GitHub releases."""

from __future__ import annotations

import hashlib
import logging
import os
import platform
import sys
import threading
import time
import urllib.request
from pathlib import Path

from autobrowser_client import __version__

REPO_URL = "https://github.com/foxzi/autobrowser-client"
CHECK_INTERVAL_S = 24 * 3600
STAMP_FILE = Path("~/.cache/autobrowser/last-update-check").expanduser()

logger = logging.getLogger(__name__)


def is_frozen() -> bool:
    return bool(getattr(sys, "frozen", False))


def _parse(version: str) -> tuple[int, ...]:
    return tuple(int(p) for p in version.lstrip("v").split("."))


def asset_name() -> str:
    system = {"Linux": "linux", "Darwin": "macos", "Windows": "windows"}[
        platform.system()
    ]
    machine = platform.machine().lower()
    arch = "arm64" if machine in ("arm64", "aarch64") else "amd64"
    return f"autobrowser-{system}-{arch}" + (".exe" if system == "windows" else "")


def latest_version() -> str:
    # /releases/latest redirects to /releases/tag/vX.Y.Z; no API rate limit
    with urllib.request.urlopen(f"{REPO_URL}/releases/latest", timeout=15) as resp:
        return resp.geturl().rstrip("/").rsplit("/", 1)[-1].lstrip("v")


def is_newer(version: str) -> bool:
    return _parse(version) > _parse(__version__)


def _download(url: str) -> bytes:
    with urllib.request.urlopen(url, timeout=300) as resp:
        return resp.read()


def install_version(version: str) -> Path:
    """Download the release binary, verify sha256 and replace the running one."""
    if not is_frozen():
        raise RuntimeError(
            "Self-update works only for the binary; use pip/uv to upgrade"
        )
    name = asset_name()
    base = f"{REPO_URL}/releases/download/v{version}/{name}"
    data = _download(base)
    expected = _download(base + ".sha256").decode().split()[0]
    if hashlib.sha256(data).hexdigest() != expected:
        raise RuntimeError(f"Checksum mismatch for {name} v{version}")

    exe = Path(sys.executable).resolve()
    tmp = exe.with_name(exe.name + ".new")
    tmp.write_bytes(data)
    tmp.chmod(0o755)
    if os.name == "nt":
        # A running exe cannot be overwritten on Windows, but it can be renamed
        old = exe.with_name(exe.name + ".old")
        old.unlink(missing_ok=True)
        os.replace(exe, old)
    os.replace(tmp, exe)
    return exe


def cleanup_old() -> None:
    if is_frozen():
        exe = Path(sys.executable).resolve()
        try:
            exe.with_name(exe.name + ".old").unlink(missing_ok=True)
        except OSError:
            pass


def _auto_update() -> None:
    try:
        STAMP_FILE.parent.mkdir(parents=True, exist_ok=True)
        STAMP_FILE.write_text(str(int(time.time())))
        version = latest_version()
        if is_newer(version):
            path = install_version(version)
            logger.info("Updated %s to v%s; restart to use it", path, version)
    except Exception as exc:  # never break the running command
        logger.warning("Auto-update failed: %s", exc)


def start_auto_update() -> None:
    """Check for a new release at most once a day in a background thread.

    Only for the binary; disabled by AUTOBROWSER_NO_AUTO_UPDATE=1.
    """
    cleanup_old()
    if not is_frozen() or os.environ.get("AUTOBROWSER_NO_AUTO_UPDATE") == "1":
        return
    try:
        last = int(STAMP_FILE.read_text())
    except (OSError, ValueError):
        last = 0
    if time.time() - last < CHECK_INTERVAL_S:
        return
    threading.Thread(target=_auto_update, daemon=True).start()
