from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, Literal

import yaml
from pydantic import BaseModel, ConfigDict, Field

from autobrowser_client.errors import ConfigError
from autobrowser_client.logging_utils import mask_mapping


class ViewportSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    width: int = Field(default=1280, gt=0)
    height: int = Field(default=720, gt=0)


class BrowserSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    headless: bool = True
    engine: Literal["patchright", "cloakbrowser"] = "patchright"
    # cloakbrowser only: OS fingerprint (--fingerprint-platform); None keeps cloakbrowser default
    fingerprint_platform: Literal["windows", "linux", "macos"] | None = Field(
        default=None, alias="fingerprintPlatform"
    )
    launch_args: list[str] = Field(default_factory=list, alias="launchArgs")
    user_agent: str | None = Field(default=None, alias="userAgent")
    viewport: ViewportSettings = Field(default_factory=ViewportSettings)


class ProfilesSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    dir: str = ".profiles"


class ApiSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    host: str = "127.0.0.1"
    port: int = Field(default=8080, ge=1, le=65535)
    token: str | None = None
    max_sessions: int = Field(default=10, ge=1, alias="maxSessions")


class TimeoutsSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    startup_ms: int = Field(default=30000, ge=0, alias="startupMs")
    action_ms: int = Field(default=10000, ge=0, alias="actionMs")
    navigation_ms: int = Field(default=30000, ge=0, alias="navigationMs")
    idle_session_ms: int = Field(default=300000, ge=0, alias="idleSessionMs")


class ProxySettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    enabled: bool = False
    scheme: str = "socks5"
    host: str | None = None
    port: int | None = Field(default=None, ge=1, le=65535)
    username: str | None = None
    password: str | None = None
    socks2http_bin: str = Field(
        default="",
        alias="socks2httpBin",
    )
    local_port: int = Field(default=0, ge=0, le=65535, alias="localPort")

    def proxy_url(self) -> str | None:
        """Build upstream proxy URL for socks2http."""
        if not self.host:
            return None
        auth = ""
        if self.username:
            auth = self.username
            if self.password:
                auth += f":{self.password}"
            auth += "@"
        p = self.port or (1080 if self.scheme == "socks5" else 8080)
        return f"{self.scheme}://{auth}{self.host}:{p}"


LogLevel = Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]


class LoggingSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    level: LogLevel = "INFO"
    file: str | None = None
    mask_secrets: bool = Field(default=True, alias="maskSecrets")


ContentFormat = Literal["html", "text"]


class OutputSettings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    max_content_bytes: int = Field(default=1048576, ge=0, alias="maxContentBytes")
    content_format: ContentFormat = Field(default="html", alias="contentFormat")


class Settings(BaseModel):
    model_config = ConfigDict(extra="forbid")

    browser: BrowserSettings = Field(default_factory=BrowserSettings)
    profiles: ProfilesSettings = Field(default_factory=ProfilesSettings)
    api: ApiSettings = Field(default_factory=ApiSettings)
    timeouts: TimeoutsSettings = Field(default_factory=TimeoutsSettings)
    proxy: ProxySettings = Field(default_factory=ProxySettings)
    logging: LoggingSettings = Field(default_factory=LoggingSettings)
    output: OutputSettings = Field(default_factory=OutputSettings)

    def safe_dump(self) -> dict[str, Any]:
        return mask_mapping(self.model_dump(mode="json", by_alias=True, exclude_none=True))


def _deep_merge(base: dict[str, Any], updates: dict[str, Any]) -> dict[str, Any]:
    result = dict(base)
    for key, value in updates.items():
        if isinstance(value, dict) and isinstance(result.get(key), dict):
            result[key] = _deep_merge(result[key], value)
        else:
            result[key] = value
    return result


def _parse_bool(value: str) -> bool:
    normalized = value.strip().lower()
    return normalized in {"1", "true", "yes", "on"}


def _load_file(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise ConfigError(f"Configuration file does not exist: {path}")

    raw = path.read_text(encoding="utf-8")
    if path.suffix in {".yaml", ".yml"}:
        data = yaml.safe_load(raw) or {}
    elif path.suffix == ".json":
        data = json.loads(raw)
    else:
        raise ConfigError(f"Unsupported config file format: {path.suffix}")

    if not isinstance(data, dict):
        raise ConfigError("Configuration file must contain an object at the root")
    return data


def _env_overrides(env: dict[str, str]) -> dict[str, Any]:
    result: dict[str, Any] = {}

    mappings: list[tuple[str, tuple[str, ...], str]] = [
        ("AUTOBROWSER_HEADLESS", ("browser", "headless"), "bool"),
        ("AUTOBROWSER_BROWSER_ENGINE", ("browser", "engine"), "str"),
        ("AUTOBROWSER_FINGERPRINT_PLATFORM", ("browser", "fingerprintPlatform"), "str"),
        ("AUTOBROWSER_USER_AGENT", ("browser", "userAgent"), "str"),
        ("AUTOBROWSER_VIEWPORT_WIDTH", ("browser", "viewport", "width"), "int"),
        ("AUTOBROWSER_VIEWPORT_HEIGHT", ("browser", "viewport", "height"), "int"),
        ("AUTOBROWSER_PROFILE_DIR", ("profiles", "dir"), "str"),
        ("AUTOBROWSER_API_HOST", ("api", "host"), "str"),
        ("AUTOBROWSER_API_PORT", ("api", "port"), "int"),
        ("AUTOBROWSER_API_TOKEN", ("api", "token"), "str"),
        ("AUTOBROWSER_API_MAX_SESSIONS", ("api", "maxSessions"), "int"),
        ("AUTOBROWSER_LOG_LEVEL", ("logging", "level"), "str"),
        ("AUTOBROWSER_LOG_FILE", ("logging", "file"), "str"),
        ("AUTOBROWSER_PROXY_ENABLED", ("proxy", "enabled"), "bool"),
        ("AUTOBROWSER_PROXY_SCHEME", ("proxy", "scheme"), "str"),
        ("AUTOBROWSER_PROXY_HOST", ("proxy", "host"), "str"),
        ("AUTOBROWSER_PROXY_PORT", ("proxy", "port"), "int"),
        ("AUTOBROWSER_PROXY_USERNAME", ("proxy", "username"), "str"),
        ("AUTOBROWSER_PROXY_PASSWORD", ("proxy", "password"), "str"),
        ("AUTOBROWSER_SOCKS2HTTP_BIN", ("proxy", "socks2httpBin"), "str"),
        ("AUTOBROWSER_PROXY_LOCAL_PORT", ("proxy", "localPort"), "int"),
        ("AUTOBROWSER_TIMEOUT_STARTUP_MS", ("timeouts", "startupMs"), "int"),
        ("AUTOBROWSER_TIMEOUT_ACTION_MS", ("timeouts", "actionMs"), "int"),
        ("AUTOBROWSER_TIMEOUT_NAVIGATION_MS", ("timeouts", "navigationMs"), "int"),
        ("AUTOBROWSER_SESSION_IDLE_MS", ("timeouts", "idleSessionMs"), "int"),
        ("AUTOBROWSER_MAX_CONTENT_BYTES", ("output", "maxContentBytes"), "int"),
        ("AUTOBROWSER_CONTENT_FORMAT", ("output", "contentFormat"), "str"),
    ]

    for env_key, path, kind in mappings:
        if env_key not in env:
            continue
        value: Any = env[env_key]
        if kind == "bool":
            value = _parse_bool(value)
        elif kind == "int":
            value = int(value)

        cursor = result
        for part in path[:-1]:
            cursor = cursor.setdefault(part, {})
        cursor[path[-1]] = value

    return result


def load_settings(
    config_path: str | Path | None = None,
    overrides: dict[str, Any] | None = None,
    env: dict[str, str] | None = None,
) -> Settings:
    data: dict[str, Any] = {}
    if config_path:
        data = _deep_merge(data, _load_file(Path(config_path)))

    data = _deep_merge(data, _env_overrides(env or dict(os.environ)))
    if overrides:
        data = _deep_merge(data, overrides)

    try:
        return Settings.model_validate(data)
    except Exception as exc:
        raise ConfigError(str(exc)) from exc
