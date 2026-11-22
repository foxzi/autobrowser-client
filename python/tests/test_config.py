"""Unit tests for configuration loading and defaults."""

from __future__ import annotations

import json
import tempfile
from pathlib import Path

import pytest

from autobrowser_client.config import Settings, load_settings, _deep_merge, _env_overrides
from autobrowser_client.errors import ConfigError


def test_defaults():
    s = load_settings(env={})
    assert s.browser.headless is True
    assert s.api.port == 8080
    assert s.api.host == "127.0.0.1"
    assert s.timeouts.action_ms == 10000
    assert s.timeouts.navigation_ms == 30000
    assert s.proxy.enabled is False
    assert s.output.max_content_bytes == 1048576


def test_env_override():
    env = {
        "AUTOBROWSER_API_PORT": "9090",
        "AUTOBROWSER_HEADLESS": "false",
        "AUTOBROWSER_LOG_LEVEL": "DEBUG",
        "AUTOBROWSER_API_TOKEN": "secret123",
    }
    s = load_settings(env=env)
    assert s.api.port == 9090
    assert s.browser.headless is False
    assert s.logging.level == "DEBUG"
    assert s.api.token == "secret123"


def test_config_file_yaml():
    data = {"browser": {"headless": False}, "api": {"port": 7777}}
    with tempfile.NamedTemporaryFile(suffix=".yaml", mode="w", delete=False) as f:
        import yaml
        yaml.dump(data, f)
        f.flush()
        s = load_settings(config_path=f.name, env={})
    assert s.browser.headless is False
    assert s.api.port == 7777
    Path(f.name).unlink()


def test_config_file_json():
    data = {"api": {"host": "0.0.0.0", "maxSessions": 5}}
    with tempfile.NamedTemporaryFile(suffix=".json", mode="w", delete=False) as f:
        json.dump(data, f)
        f.flush()
        s = load_settings(config_path=f.name, env={})
    assert s.api.host == "0.0.0.0"
    assert s.api.max_sessions == 5
    Path(f.name).unlink()


def test_config_file_missing():
    with pytest.raises(ConfigError, match="does not exist"):
        load_settings(config_path="/nonexistent/config.yaml", env={})


def test_config_file_unsupported_format():
    with tempfile.NamedTemporaryFile(suffix=".toml", mode="w", delete=False) as f:
        f.write("foo = 1")
        f.flush()
        with pytest.raises(ConfigError, match="Unsupported"):
            load_settings(config_path=f.name, env={})
    Path(f.name).unlink()


def test_overrides_take_precedence():
    env = {"AUTOBROWSER_API_PORT": "9090"}
    overrides = {"api": {"port": 1234}}
    s = load_settings(env=env, overrides=overrides)
    assert s.api.port == 1234


def test_deep_merge():
    base = {"a": {"b": 1, "c": 2}, "d": 3}
    updates = {"a": {"b": 10, "e": 5}}
    result = _deep_merge(base, updates)
    assert result == {"a": {"b": 10, "c": 2, "e": 5}, "d": 3}


def test_proxy_url():
    s = load_settings(
        env={},
        overrides={
            "proxy": {
                "enabled": True,
                "scheme": "socks5",
                "host": "proxy.example.com",
                "port": 1080,
                "username": "user",
                "password": "pass",
            }
        },
    )
    assert s.proxy.proxy_url() == "socks5://user:pass@proxy.example.com:1080"


def test_proxy_url_no_auth():
    s = load_settings(
        env={},
        overrides={
            "proxy": {"host": "proxy.example.com", "port": 8080, "scheme": "http"}
        },
    )
    assert s.proxy.proxy_url() == "http://proxy.example.com:8080"


def test_proxy_url_none_when_no_host():
    s = load_settings(env={})
    assert s.proxy.proxy_url() is None


def test_invalid_config_extra_field():
    with pytest.raises(ConfigError):
        load_settings(env={}, overrides={"browser": {"unknown_field": True}})


def test_safe_dump_masks_secrets():
    s = load_settings(
        env={},
        overrides={"proxy": {"password": "topsecret", "host": "h"}},
    )
    dump = s.safe_dump()
    assert "topsecret" not in json.dumps(dump)


def test_fingerprint_platform_env():
    settings = load_settings(env={"AUTOBROWSER_FINGERPRINT_PLATFORM": "linux"})
    assert settings.browser.fingerprint_platform == "linux"
    assert load_settings(env={}).browser.fingerprint_platform is None


def test_fingerprint_platform_invalid():
    with pytest.raises(ConfigError):
        load_settings(env={"AUTOBROWSER_FINGERPRINT_PLATFORM": "android"})


def test_geoip_env():
    assert load_settings(env={}).browser.geoip is False
    assert load_settings(env={"AUTOBROWSER_GEOIP": "true"}).browser.geoip is True
