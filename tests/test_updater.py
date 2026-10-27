import hashlib
import sys

import pytest

from autobrowser_client import updater


def test_is_newer(monkeypatch):
    monkeypatch.setattr(updater, "__version__", "0.3.0")
    assert updater.is_newer("0.10.0")
    assert updater.is_newer("v0.3.1")
    assert not updater.is_newer("0.3.0")
    assert not updater.is_newer("0.2.9")


def test_asset_name(monkeypatch):
    monkeypatch.setattr(updater.platform, "system", lambda: "Linux")
    monkeypatch.setattr(updater.platform, "machine", lambda: "aarch64")
    assert updater.asset_name() == "autobrowser-linux-arm64"
    monkeypatch.setattr(updater.platform, "system", lambda: "Windows")
    monkeypatch.setattr(updater.platform, "machine", lambda: "AMD64")
    assert updater.asset_name() == "autobrowser-windows-amd64.exe"


def _fake_release(monkeypatch, tmp_path, data, digest):
    exe = tmp_path / "autobrowser"
    exe.write_bytes(b"old")
    monkeypatch.setattr(sys, "frozen", True, raising=False)
    monkeypatch.setattr(sys, "executable", str(exe))
    files = {"bin": data, "sha": f"{digest}  autobrowser\n".encode()}
    monkeypatch.setattr(
        updater,
        "_download",
        lambda url: files["sha"] if url.endswith(".sha256") else files["bin"],
    )
    return exe


def test_install_version_replaces_binary(monkeypatch, tmp_path):
    exe = _fake_release(
        monkeypatch, tmp_path, b"new", hashlib.sha256(b"new").hexdigest()
    )
    assert updater.install_version("9.9.9") == exe.resolve()
    assert exe.read_bytes() == b"new"


def test_install_version_rejects_bad_checksum(monkeypatch, tmp_path):
    exe = _fake_release(monkeypatch, tmp_path, b"new", "0" * 64)
    with pytest.raises(RuntimeError, match="Checksum"):
        updater.install_version("9.9.9")
    assert exe.read_bytes() == b"old"


def test_install_version_requires_binary(monkeypatch):
    monkeypatch.setattr(sys, "frozen", False, raising=False)
    with pytest.raises(RuntimeError, match="only for the binary"):
        updater.install_version("9.9.9")
