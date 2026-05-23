from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Literal, get_args

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
_WAIT_STATES = {"load", "domcontentloaded", "networkidle"}
_KNOWN_ACTIONS = set(get_args(ActionName))
# Actions whose shorthand value maps to a specific field
_SHORTHAND_NULL_ACTIONS = {"get-content", "get-cookies", "get-headers", "browse-tabs"}


def _parse_locator_string(s: str) -> dict:
    eq = s.find("=")
    if eq != -1 and s[:eq] in _LOCATOR_TYPES:
        return {"type": s[:eq], "value": s[eq + 1 :]}
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


def _expand_shorthand_step(d: dict) -> dict:
    """Expand verb-key shorthand to canonical dict form.

    If ``d`` already contains ``"action"``, it is returned as-is.
    Otherwise the first key matching a known ActionName is treated as
    the action verb and its value is mapped to the appropriate field.
    """
    if "action" in d:
        return d

    # Find the shorthand verb key
    verb: str | None = None
    for key in d:
        if key in _KNOWN_ACTIONS:
            verb = key
            break

    if verb is None:
        return d  # let pydantic raise a normal validation error

    value = d[verb]
    # Build result starting with all OTHER keys (extra step options)
    result = {k: v for k, v in d.items() if k != verb}
    result["action"] = verb

    # Check for explicit conflict with the target field
    def _check_conflict(field: str) -> None:
        if field in result:
            raise ValueError(
                f"shorthand conflict: cannot specify both shorthand value and explicit '{field}'"
            )

    if verb == "open":
        _check_conflict("url")
        result["url"] = value

    elif verb == "wait":
        if isinstance(value, int):
            _check_conflict("timeout")
            result["timeout"] = value
        elif isinstance(value, str) and value in _WAIT_STATES:
            _check_conflict("waitUntil")
            result["waitUntil"] = value
        else:
            raise ValueError(
                f"wait shorthand expects int (ms) or load-state string, got: {value!r}"
            )

    elif verb in {"click", "find"}:
        _check_conflict("locator")
        result["locator"] = value

    elif verb in {"fill", "type"}:
        if not isinstance(value, list) or len(value) != 2:
            raise ValueError(
                f"'{verb}' shorthand expects a 2-element list [locator, value], got: {value!r}"
            )
        _check_conflict("locator")
        _check_conflict("value")
        result["locator"] = value[0]
        result["value"] = value[1]

    elif verb == "press":
        _check_conflict("key")
        result["key"] = value

    elif verb in {"screenshot", "save"}:
        _check_conflict("file")
        result["file"] = value

    elif verb in _SHORTHAND_NULL_ACTIONS:
        if value is not None:
            raise ValueError(f"'{verb}' shorthand expects null value, got: {value!r}")

    return result


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

    @model_validator(mode="before")
    @classmethod
    def _expand_shorthand(cls, data: object) -> object:
        if isinstance(data, dict):
            return _expand_shorthand_step(data)
        return data

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
