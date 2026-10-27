from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Annotated, Any, Literal, Union, get_args

import yaml
from pydantic import BaseModel, ConfigDict, Discriminator, Field, Tag, model_validator

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
    "solve-challenge",
    "hover",
    "dblclick",
    "focus",
    "clear",
    "check",
    "uncheck",
    "submit",
    "select",
    "upload",
    "evaluate",
    "history",
    "wait-text",
    "extract",
    "find-all",
    "read",
    "mouse-move",
    "mouse-scroll-to",
    "mouse-random",
    "set",
    "assert",
    "fail",
]
LocatorType = Literal["css", "xpath", "id", "name", "class", "text", "attribute", "ref"]
WaitUntil = Literal["domcontentloaded", "load", "networkidle", "visible", "hidden"]


_LOCATOR_TYPES = {"css", "xpath", "id", "name", "class", "text", "attribute", "ref"}
_ID_RE = re.compile(r"^#[a-zA-Z_][\w-]*$")
_CLASS_RE = re.compile(r"^\.[a-zA-Z_][\w-]*$")
_WAIT_STATES = {"load", "domcontentloaded", "networkidle"}
_KNOWN_ACTIONS = set(get_args(ActionName))
_NAME_RE = re.compile(r"^[A-Za-z_]\w*$")
# Actions whose result can be stored in a variable with ``as``
_VALUE_ACTIONS = {"extract", "evaluate", "read", "find-all", "get-content", "get-cookies", "get-headers"}
# Actions whose shorthand value maps to a specific field
_SHORTHAND_NULL_ACTIONS = {
    "get-content", "get-cookies", "get-headers", "browse-tabs", "solve-challenge",
    "mouse-move", "mouse-random",
}
# Actions whose shorthand value is the locator
_LOCATOR_ACTIONS = {
    "find", "click", "hover", "dblclick", "focus", "clear", "check", "uncheck",
    "submit", "extract", "find-all", "read", "mouse-scroll-to",
}


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
    model_config = ConfigDict(extra="forbid", coerce_numbers_to_str=True)

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

    elif verb in _LOCATOR_ACTIONS:
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

    elif verb == "select":
        if not isinstance(value, list) or len(value) != 2:
            raise ValueError(f"'select' shorthand expects a 2-element list [locator, option], got: {value!r}")
        _check_conflict("locator")
        result["locator"] = value[0]
        if isinstance(value[1], list):
            _check_conflict("options")
            result["options"] = value[1]
        else:
            _check_conflict("value")
            result["value"] = value[1]

    elif verb == "upload":
        if not isinstance(value, list) or len(value) != 2:
            raise ValueError(f"'upload' shorthand expects a 2-element list [locator, file(s)], got: {value!r}")
        _check_conflict("locator")
        _check_conflict("files")
        result["locator"] = value[0]
        result["files"] = value[1] if isinstance(value[1], list) else [value[1]]

    elif verb == "evaluate":
        _check_conflict("script")
        result["script"] = value

    elif verb == "history":
        _check_conflict("direction")
        result["direction"] = value

    elif verb == "wait-text":
        _check_conflict("text")
        result["text"] = value

    elif verb == "fail":
        _check_conflict("message")
        result["message"] = value

    elif verb == "assert":
        _check_conflict("condition")
        result["condition"] = value

    elif verb == "set":
        _check_conflict("vars")
        result["vars"] = value

    elif verb in {"screenshot", "save"}:
        _check_conflict("file")
        result["file"] = value

    elif verb in _SHORTHAND_NULL_ACTIONS:
        if value is not None:
            raise ValueError(f"'{verb}' shorthand expects null value, got: {value!r}")

    return result


class Condition(BaseModel):
    """Check used by ``if``, ``while`` and ``assert``: exactly one condition key."""

    model_config = ConfigDict(extra="forbid", coerce_numbers_to_str=True)

    exists: Locator | None = None
    visible: Locator | None = None
    text: str | None = None
    url: str | None = None
    url_matches: str | None = Field(default=None, alias="urlMatches")
    title: str | None = None
    var: str | None = None
    equals: Any = None
    contains: str | None = None
    matches: str | None = None
    not_: Condition | None = Field(default=None, alias="not")
    all_: list[Condition] | None = Field(default=None, alias="all")
    any_: list[Condition] | None = Field(default=None, alias="any")
    timeout: int = 0

    @model_validator(mode="after")
    def _validate_keys(self) -> "Condition":
        keys = ("exists", "visible", "text", "url", "url_matches", "title", "var", "not_", "all_", "any_")
        if sum(getattr(self, k) is not None for k in keys) != 1:
            raise ValueError(
                "condition needs exactly one of: exists, visible, text, url, urlMatches, "
                "title, var, not, all, any"
            )
        has_var_opts = "equals" in self.model_fields_set or self.contains or self.matches
        if has_var_opts and self.var is None:
            raise ValueError("'equals', 'contains' and 'matches' require 'var'")
        if self.timeout and (self.not_ is not None or self.all_ is not None or self.any_ is not None):
            raise ValueError("'timeout' is only allowed on a leaf condition")
        return self


Condition.model_rebuild()


class ScriptStep(BaseModel):
    # numbers are coerced to str so that "${n}" may yield an int for a text field
    model_config = ConfigDict(extra="forbid", coerce_numbers_to_str=True)

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
    options: list[str] | None = None
    files: list[str] | None = None
    script: str | None = None
    direction: Literal["back", "forward", "reload"] | None = None
    text: str | None = None
    gone: bool = False
    attribute: str | None = None
    all_: bool = Field(default=False, alias="all")
    x: float | None = None
    y: float | None = None
    move_steps: int | None = Field(default=None, alias="steps")
    start_x: float = Field(default=0, alias="startX")
    start_y: float = Field(default=0, alias="startY")
    end_x: float | None = Field(default=None, alias="endX")
    end_y: float | None = Field(default=None, alias="endY")
    duration_ms: int | None = Field(default=None, alias="durationMs")
    jitter: float | None = None
    seed: int | None = None
    as_name: str | None = Field(default=None, alias="as")
    vars: dict[str, Any] | None = None
    condition: Condition | None = None
    message: str | None = None
    name: str | None = None
    retry: int = Field(default=0, ge=0)
    retry_delay: int = Field(default=1000, ge=0, alias="retryDelay")
    optional: bool = False

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
        if self.action in _LOCATOR_ACTIONS | {"fill", "type", "select", "upload"} and not self.locator:
            raise ValueError(f"'{self.action}' step requires 'locator'")
        if self.action == "select" and (self.value is None) == (not self.options):
            raise ValueError("'select' step requires exactly one of 'value' or 'options'")
        if self.action == "upload" and not self.files:
            raise ValueError("'upload' step requires 'files'")
        if self.action == "evaluate" and not self.script:
            raise ValueError("'evaluate' step requires 'script'")
        if self.action == "history" and not self.direction:
            raise ValueError("'history' step requires 'direction'")
        if self.action == "wait-text" and not self.text:
            raise ValueError("'wait-text' step requires 'text'")
        if self.action == "mouse-move" and (self.x is None or self.y is None):
            raise ValueError("'mouse-move' step requires 'x' and 'y'")
        if self.action == "mouse-random" and (self.end_x is None or self.end_y is None):
            raise ValueError("'mouse-random' step requires 'endX' and 'endY'")
        if self.action in {"fill", "type"} and self.value is None:
            raise ValueError(f"'{self.action}' step requires 'value'")
        if self.action == "wait" and self.wait_until in {"visible", "hidden"} and not self.locator:
            raise ValueError(f"'wait' with waitUntil '{self.wait_until}' requires 'locator'")
        if self.action == "press" and not self.key:
            raise ValueError("'press' step requires 'key'")
        if self.action == "scroll" and self.delta_x is None and self.delta_y is None:
            raise ValueError("'scroll' step requires 'deltaX' or 'deltaY'")
        if self.action in {"save", "screenshot"} and not self.file:
            raise ValueError(f"'{self.action}' step requires 'file'")
        if self.action == "set" and not self.vars:
            raise ValueError("'set' step requires 'vars'")
        if self.action == "fail" and not self.message:
            raise ValueError("'fail' step requires 'message'")
        if self.action == "assert" and self.condition is None:
            raise ValueError("'assert' step requires 'condition'")
        if self.as_name is not None:
            if self.action not in _VALUE_ACTIONS:
                raise ValueError(f"'as' is not supported by '{self.action}'")
            if not _NAME_RE.match(self.as_name):
                raise ValueError(f"invalid variable name: {self.as_name!r}")
        return self


class IfBlock(BaseModel):
    model_config = ConfigDict(extra="forbid")

    if_: Condition = Field(alias="if")
    then: list[Step]
    else_: list[Step] | None = Field(default=None, alias="else")
    name: str | None = None


class TryBlock(BaseModel):
    model_config = ConfigDict(extra="forbid")

    try_: list[Step] = Field(alias="try")
    catch: list[Step] | None = None
    finally_: list[Step] | None = Field(default=None, alias="finally")
    name: str | None = None

    @model_validator(mode="after")
    def _needs_catch_or_finally(self) -> "TryBlock":
        if self.catch is None and self.finally_ is None:
            raise ValueError("'try' requires 'catch' or 'finally'")
        return self


class ForEachBlock(BaseModel):
    model_config = ConfigDict(extra="forbid")

    for_each: list[Any] | str = Field(alias="forEach")
    as_name: str = Field(default="item", alias="as")
    do: list[Step]
    name: str | None = None

    @model_validator(mode="after")
    def _valid_name(self) -> "ForEachBlock":
        if not _NAME_RE.match(self.as_name):
            raise ValueError(f"invalid variable name: {self.as_name!r}")
        return self


class RepeatBlock(BaseModel):
    model_config = ConfigDict(extra="forbid")

    repeat: int | str
    do: list[Step]
    name: str | None = None


class WhileBlock(BaseModel):
    model_config = ConfigDict(extra="forbid")

    while_: Condition = Field(alias="while")
    max: int = Field(default=100, ge=1)
    do: list[Step]
    name: str | None = None


_BLOCK_KEYS = {
    "if": "if", "try": "try", "forEach": "forEach", "repeat": "repeat", "while": "while",
}
# Keys that hold nested step lists in a control block
_BLOCK_BODIES = ("then", "else", "try", "catch", "finally", "do")
_BLOCK_TYPES: dict[type, str] = {
    IfBlock: "if", TryBlock: "try", ForEachBlock: "forEach", RepeatBlock: "repeat", WhileBlock: "while",
}


def _step_kind(value: object) -> str:
    if isinstance(value, dict):
        return next((tag for key, tag in _BLOCK_KEYS.items() if key in value), "step")
    return _BLOCK_TYPES.get(type(value), "step")


Step = Annotated[
    Union[
        Annotated[IfBlock, Tag("if")],
        Annotated[TryBlock, Tag("try")],
        Annotated[ForEachBlock, Tag("forEach")],
        Annotated[RepeatBlock, Tag("repeat")],
        Annotated[WhileBlock, Tag("while")],
        Annotated[ScriptStep, Tag("step")],
    ],
    Discriminator(_step_kind),
]

for _block in _BLOCK_TYPES:
    _block.model_rebuild()


class ScriptDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    session: SessionConfig = Field(default_factory=SessionConfig)
    steps: list[Step]
    output: ScriptOutput = Field(default_factory=ScriptOutput)
    stop_on_error: bool = Field(default=True, alias="stopOnError")
    vars: dict[str, Any] = Field(default_factory=dict)
    error_artifacts: str | None = Field(default=None, alias="errorArtifacts")


def _read_data(path: Path) -> dict:
    if not path.exists():
        raise ScriptValidationError(f"Script file does not exist: {path}")

    raw = path.read_text(encoding="utf-8")
    if path.suffix in {".yaml", ".yml"}:
        data = yaml.safe_load(raw) or {}
    elif path.suffix == ".json":
        data = json.loads(raw)
    else:
        raise ScriptValidationError(f"Unsupported script format: {path.suffix}")

    if not isinstance(data, dict):
        raise ScriptValidationError("Script root must be an object")
    return data


def _load_include(item: dict, base_dir: Path, stack: list[Path], variables: dict) -> list:
    target = item["include"]
    if len(item) != 1:
        raise ScriptValidationError("'include' step takes no other keys")
    if not isinstance(target, str) or not target:
        raise ScriptValidationError(f"'include' expects a file path, got: {target!r}")
    if Path(target).is_absolute():
        raise ScriptValidationError(f"Absolute include paths are not allowed: {target}")
    path = (base_dir / target).resolve()
    if not path.is_relative_to(base_dir.resolve()):
        raise ScriptValidationError(f"Include path escapes the scenario directory: {target}")
    if path in stack:
        chain = " -> ".join(p.name for p in [*stack, path])
        raise ScriptValidationError(f"Include cycle: {chain}")

    data = _read_data(path)
    included_vars = data.get("vars")
    if isinstance(included_vars, dict):
        for key, value in included_vars.items():
            variables.setdefault(key, value)
    return _expand_includes(data.get("steps") or [], path.parent, [*stack, path], variables)


def _expand_includes(steps: list, base_dir: Path, stack: list[Path], variables: dict) -> list:
    """Replace ``include`` steps with the steps of the included file (recursively)."""
    result: list = []
    for item in steps:
        if isinstance(item, dict) and "include" in item:
            result.extend(_load_include(item, base_dir, stack, variables))
            continue
        if isinstance(item, dict):
            for key in _BLOCK_BODIES:
                if isinstance(item.get(key), list):
                    item[key] = _expand_includes(item[key], base_dir, stack, variables)
        result.append(item)
    return result


def _build_script(data: dict, base_dir: Path, stack: list[Path]) -> ScriptDefinition:
    if isinstance(data.get("steps"), list):
        variables = data.setdefault("vars", {})
        if not isinstance(variables, dict):
            variables = {}
        data["steps"] = _expand_includes(data["steps"], base_dir, stack, variables)
    try:
        return ScriptDefinition.model_validate(data)
    except Exception as exc:
        raise ScriptValidationError(str(exc)) from exc


def load_script(path: str | Path) -> ScriptDefinition:
    script_path = Path(path)
    data = _read_data(script_path)
    resolved = script_path.resolve()
    return _build_script(data, resolved.parent, [resolved])


def parse_script(data: dict | str, base_dir: str | Path) -> ScriptDefinition:
    """Build a scenario from an object or YAML text; ``include`` paths are relative to base_dir."""
    if isinstance(data, str):
        data = yaml.safe_load(data) or {}
    if not isinstance(data, dict):
        raise ScriptValidationError("Script root must be an object")
    return _build_script(data, Path(base_dir), [])
