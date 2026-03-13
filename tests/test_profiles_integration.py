"""Integration tests for profiles and proxy configuration."""

from __future__ import annotations

from pathlib import Path

import pytest

from autobrowser_client.config import ProxySettings, load_settings
from autobrowser_client.errors import AppError, ErrorCode
from autobrowser_client.profiles.manager import ProfileLock, check_profile_exists
from autobrowser_client.proxy.relay import ProxyRelay, ProxyRelayError, _find_free_port


class TestProfileIntegration:
    """Tests for profile creation and session management."""

    def test_profile_dir_created_on_start(self, tmp_path: Path):
        """Profile directory is created when session starts with --profile."""
        profiles_dir = tmp_path / "profiles"
        profiles_dir.mkdir()
        settings = load_settings(
            env={},
            overrides={"profiles": {"dir": str(profiles_dir)}},
        )
        # Simulate what BrowserSession.start does for profile setup
        profile_name = "new_profile"
        profile_dir = Path(settings.profiles.dir) / profile_name
        profile_dir.mkdir(parents=True, exist_ok=True)

        assert profile_dir.is_dir()
        assert check_profile_exists(str(profiles_dir), profile_name) == profile_dir

    def test_profile_lock_blocks_concurrent_use(self, tmp_path: Path):
        """Same profile cannot be used by two sessions simultaneously."""
        profile_dir = tmp_path / "shared_profile"
        profile_dir.mkdir()

        lock1 = ProfileLock(profile_dir)
        lock1.acquire()

        lock2 = ProfileLock(profile_dir)
        with pytest.raises(AppError):
            lock2.acquire()

        lock1.release()
        # Now lock2 should succeed
        lock2.acquire()
        lock2.release()

    def test_profile_reuse_preserves_data(self, tmp_path: Path):
        """Data written to profile directory persists between lock cycles."""
        profile_dir = tmp_path / "persistent"
        profile_dir.mkdir()

        # First session writes data
        lock1 = ProfileLock(profile_dir)
        lock1.acquire()
        (profile_dir / "cookies.json").write_text('{"session": "abc"}')
        lock1.release()

        # Second session reads it
        lock2 = ProfileLock(profile_dir)
        lock2.acquire()
        assert (profile_dir / "cookies.json").read_text() == '{"session": "abc"}'
        lock2.release()


class TestProxyConfig:
    """Tests for proxy configuration and relay setup."""

    def test_proxy_settings_from_env(self):
        env = {
            "AUTOBROWSER_PROXY_ENABLED": "true",
            "AUTOBROWSER_PROXY_HOST": "proxy.example.com",
            "AUTOBROWSER_PROXY_PORT": "1080",
            "AUTOBROWSER_PROXY_USERNAME": "user",
            "AUTOBROWSER_PROXY_PASSWORD": "pass",
        }
        s = load_settings(env=env)
        assert s.proxy.enabled is True
        assert s.proxy.host == "proxy.example.com"
        assert s.proxy.port == 1080
        assert s.proxy.proxy_url() == "socks5://user:pass@proxy.example.com:1080"

    def test_proxy_disabled_by_default(self):
        s = load_settings(env={})
        assert s.proxy.enabled is False
        assert s.proxy.proxy_url() is None

    def test_find_free_port(self):
        port = _find_free_port()
        assert isinstance(port, int)
        assert 1024 < port < 65536

    def test_proxy_relay_requires_host(self):
        settings = ProxySettings(enabled=True)
        relay = ProxyRelay(settings)
        with pytest.raises(ProxyRelayError, match="not configured"):
            relay.start()

    def test_proxy_relay_requires_binary(self, tmp_path: Path):
        settings = ProxySettings.model_validate({
            "enabled": True,
            "host": "proxy.example.com",
            "port": 1080,
            "socks2httpBin": str(tmp_path / "nonexistent_binary"),
        })
        relay = ProxyRelay(settings)
        with pytest.raises(ProxyRelayError, match="not found"):
            relay.start()

    def test_proxy_relay_http_url(self):
        settings = ProxySettings.model_validate(
            {"enabled": True, "host": "h", "localPort": 12345}
        )
        relay = ProxyRelay(settings)
        assert relay.http_proxy_url == "http://127.0.0.1:12345"

    def test_proxy_relay_stop_when_not_started(self):
        settings = ProxySettings(enabled=True, host="h")
        relay = ProxyRelay(settings)
        relay.stop()  # Should not raise

    def test_proxy_relay_is_not_running_initially(self):
        settings = ProxySettings(enabled=True, host="h")
        relay = ProxyRelay(settings)
        assert relay.is_running is False
