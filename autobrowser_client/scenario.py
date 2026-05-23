from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Literal

import yaml
from pydantic import BaseModel, ConfigDict, Field, model_validator

from autobrowser_client.errors import ScriptValidationError


ActionName = Literal[
    "open",
    "wait",
    "find",
    "click",
    "fill",
    "type",
    "press",
    "scroll",
    "screenshot",
    "get-content",
    "get-cookies",
    "get-headers",
    "save",
    "browse-tabs",
]
LocatorType = Literal["css", "xpath", "id", "name", "class", "text", "attribute"]
WaitUntil = Literal["domcontentloaded", "load", "networkidle", "visible", "hidden"]


_LOCATOR_TYPES = {"css", "xpath", "id", "name", "class", "text", "attribute"}
_ID_RE = re.compile(r"^#[a-zA-Z_][\w-]*$")
_CLASS_RE = re.compile(r"^\.[a-zA-Z_][\w-]*$")


def _parse_locator_string(s: str) -> dict:
    eq = s.find("=")
    if eq != -1 and s[:eq] in _LOCATOR_TYPES:
        return {"type": s[:eq], "value": s[eq + 1:]}
    if s.startswith("//") or s.startswith("/"):
        return {"type": "xpath", "value": s}
    if _ID_RE.match(s):
        return {"type": "id", "value": s[1:]}
    if _CLASS_RE.match(s):
        return {"type": "class", "value": s[1:]}
    return {"type": "css", "value": s}


class Locator(BaseModel):
    model_config = ConfigDict(extra="forbid")

    type: LocatorType
    value: str

    @model_validator(mode="before")
    @classmethod
    def _coerce_string(cls, v: object) -> object:
        if isinstance(v, str):
            return _parse_locator_string(v)
        return v


class ViewportConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    width: int
    height: int


class FingerprintOverrides(BaseModel):
    """Optional fingerprint overrides in scenario."""

    model_config = ConfigDict(extra="forbid")

    seed: int | None = None
    canvas: bool | None = None
    webgl: bool | None = None
    webgl_vendor: str | None = Field(default=None, alias="webglVendor")
    webgl_renderer: str | None = Field(default=None, alias="webglRenderer")
    audio: bool | None = None
    client_rects: bool | None = Field(default=None, alias="clientRects")
    fonts: bool | None = None


class SessionConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    headless: bool = True
    profile: str | None = None
    profile_dir: str | None = Field(default=None, alias="profileDir")
    user_agent: str | None = Field(default=None, alias="userAgent")
    proxy: str | None = None
    viewport: ViewportConfig | None = None
    timeout: int | None = None
    preserve_session: bool = Field(default=False, alias="preserveSession")
    fingerprint: FingerprintOverrides | bool | None = None


class ScriptOutput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    file: str | None = None


class ScriptStep(BaseModel):
    model_config = ConfigDict(extra="forbid")

    action: ActionName
    page: str | None = None
    url: str | None = None
    locator: Locator | None = None
    value: str | None = None
    key: str | None = None
    format: Literal["html", "text"] | None = None
    wait_until: WaitUntil | None = Field(default=None, alias="waitUntil")
    timeout: int | None = None
    delta_x: int | None = Field(default=None, alias="deltaX")
    delta_y: int | None = Field(default=None, alias="deltaY")
    delay: int | None = None
    index: int | None = None
    full_page: bool = Field(default=False, alias="fullPage")
    file: str | None = None
    stop_on_error: bool | None = Field(default=None, alias="stopOnError")

    @model_validator(mode="after")
    def validate_action_payload(self) -> "ScriptStep":
        if self.action == "open" and not self.url:
            raise ValueError("'open' step requires 'url'")
        if self.action in {"find", "click", "fill", "type"} and not self.locator:
            raise ValueError(f"'{self.action}' step requires 'locator'")
        if self.action in {"fill", "type"} and self.value is None:
            raise ValueError(f"'{self.action}' step requires 'value'")
        if self.action == "press" and not self.key:
            raise ValueError("'press' step requires 'key'")
        if self.action == "scroll" and self.delta_x is None and self.delta_y is None:
            raise ValueError("'scroll' step requires 'deltaX' or 'deltaY'")
        if self.action in {"save", "screenshot"} and not self.file:
            raise ValueError(f"'{self.action}' step requires 'file'")
        return self


class ScriptDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session: SessionConfig = Field(default_factory=SessionConfig)
    pages: list[str] = Field(default_factory=list)
    steps: list[ScriptStep]
    output: ScriptOutput = Field(default_factory=ScriptOutput)
    stop_on_error: bool = Field(default=True, alias="stopOnError")


def load_script(path: str | Path) -> ScriptDefinition:
    script_path = Path(path)
    if not script_path.exists():
        raise ScriptValidationError(f"Script file does not exist: {script_path}")

    raw = script_path.read_text(encoding="utf-8")
    if script_path.suffix in {".yaml", ".yml"}:
        data = yaml.safe_load(raw) or {}
    elif script_path.suffix == ".json":
        data = json.loads(raw)
    else:
        raise ScriptValidationError(f"Unsupported script format: {script_path.suffix}")

    if not isinstance(data, dict):
        raise ScriptValidationError("Script root must be an object")

    try:
        return ScriptDefinition.model_validate(data)
    except Exception as exc:
        raise ScriptValidationError(str(exc)) from exc
