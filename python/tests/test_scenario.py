"""Unit tests for scenario loading and validation."""

from __future__ import annotations

import re
import tempfile
from pathlib import Path

import pytest
import yaml

from autobrowser_client.errors import ScriptValidationError
from autobrowser_client.scenario import ScriptDefinition, ScriptStep, load_script


def _write_yaml(data: dict, suffix: str = ".yaml") -> Path:
    f = tempfile.NamedTemporaryFile(suffix=suffix, mode="w", delete=False)
    yaml.dump(data, f)
    f.close()
    return Path(f.name)


def test_minimal_valid_script():
    data = {"steps": [{"action": "open", "url": "https://example.com"}]}
    path = _write_yaml(data)
    try:
        script = load_script(path)
        assert len(script.steps) == 1
        assert script.steps[0].action == "open"
        assert script.steps[0].url == "https://example.com"
        assert script.stop_on_error is True
    finally:
        path.unlink()


def test_full_script():
    data = {
        "session": {"headless": False, "profile": "test"},
        "steps": [
            {"action": "open", "url": "https://example.com"},
            {"action": "wait", "timeout": 1000},
            {"action": "find", "locator": {"type": "css", "value": "h1"}},
            {"action": "click", "locator": {"type": "id", "value": "btn"}},
            {"action": "fill", "locator": {"type": "name", "value": "q"}, "value": "hello"},
            {"action": "press", "key": "Enter"},
            {"action": "scroll", "deltaY": 100},
            {"action": "get-content", "format": "html"},
            {"action": "get-cookies"},
            {"action": "get-headers"},
            {"action": "save", "file": "/tmp/out.json"},
        ],
        "output": {"file": "/tmp/result.json"},
        "stopOnError": False,
    }
    path = _write_yaml(data)
    try:
        script = load_script(path)
        assert len(script.steps) == 11
        assert script.stop_on_error is False
        assert script.session.profile == "test"
    finally:
        path.unlink()


def test_open_requires_url():
    data = {"steps": [{"action": "open"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_click_requires_locator():
    data = {"steps": [{"action": "click"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_fill_requires_value():
    data = {"steps": [{"action": "fill", "locator": {"type": "css", "value": "#input"}}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_press_requires_key():
    data = {"steps": [{"action": "press"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_scroll_requires_delta():
    data = {"steps": [{"action": "scroll"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_save_requires_file():
    data = {"steps": [{"action": "save"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_invalid_action_name():
    data = {"steps": [{"action": "fly"}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_invalid_locator_type():
    data = {"steps": [{"action": "find", "locator": {"type": "regex", "value": ".*"}}]}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


def test_missing_file():
    with pytest.raises(ScriptValidationError, match="does not exist"):
        load_script("/nonexistent/script.yaml")


def test_unsupported_format():
    f = tempfile.NamedTemporaryFile(suffix=".txt", mode="w", delete=False)
    f.write("hello")
    f.close()
    try:
        with pytest.raises(ScriptValidationError, match="Unsupported"):
            load_script(f.name)
    finally:
        Path(f.name).unlink()


def test_non_object_root():
    f = tempfile.NamedTemporaryFile(suffix=".yaml", mode="w", delete=False)
    f.write("- item1\n- item2\n")
    f.close()
    try:
        with pytest.raises(ScriptValidationError, match="object"):
            load_script(f.name)
    finally:
        Path(f.name).unlink()


def test_extra_fields_rejected():
    data = {"steps": [{"action": "open", "url": "https://x.com"}], "extra": True}
    path = _write_yaml(data)
    try:
        with pytest.raises(ScriptValidationError):
            load_script(path)
    finally:
        path.unlink()


# --- new actions: validation ---


@pytest.mark.parametrize(
    "step",
    [
        {"action": "hover"},
        {"action": "extract"},
        {"action": "select", "locator": "#s"},
        {"action": "select", "locator": "#s", "value": "a", "options": ["b"]},
        {"action": "upload", "locator": "#f"},
        {"action": "evaluate"},
        {"action": "history"},
        {"action": "history", "direction": "sideways"},
        {"action": "wait-text"},
        {"action": "mouse-move", "x": 1},
        {"action": "mouse-random", "endX": 1},
    ],
)
def test_new_action_validation_errors(step):
    with pytest.raises(ValueError):
        ScriptStep.model_validate(step)


def test_new_action_fields():
    s = ScriptStep.model_validate(
        {"action": "extract", "locator": "a", "attribute": "href", "all": True}
    )
    assert s.all_ is True and s.attribute == "href"
    s = ScriptStep.model_validate(
        {"action": "mouse-random", "startX": 1, "startY": 2, "endX": 3, "endY": 4, "durationMs": 10}
    )
    assert (s.start_x, s.end_y, s.duration_ms) == (1, 4, 10)


def test_args_only_for_evaluate():
    with pytest.raises(ValueError, match="'args' is not supported by 'click'"):
        ScriptStep.model_validate({"action": "click", "locator": "#a", "args": [1]})
    s = ScriptStep.model_validate({"evaluate": "(a) => a", "args": {"k": [1, 2]}})
    assert s.args == {"k": [1, 2]}
    assert ScriptStep.model_validate({"evaluate": "1", "args": None}).args is None


@pytest.mark.parametrize(
    "step,message",
    [
        ({"action": "click", "locator": "#a", "any": [{"text": "x"}]}, "'any' is not supported by 'click'"),
        ({"action": "wait", "any": []}, "'wait' with 'any' requires at least one condition"),
        (
            {"action": "wait", "any": [{"text": "x"}], "waitUntil": "load"},
            "'wait' with 'any' cannot be combined with 'waitUntil'",
        ),
        (
            {"action": "wait", "any": [{"text": "x", "timeout": 5}]},
            "'wait' alternatives cannot have 'timeout'; use the step 'timeout'",
        ),
        ({"action": "wait", "timeout": 5, "as": "g"}, "'as' is not supported by 'wait'"),
        ({"action": "wait", "any": [{}]}, "condition needs exactly one of"),
        ({"wait": "bogus"}, "wait shorthand expects int (ms), load-state string or {any: [...]}, got: 'bogus'"),
        ({"wait": {"any": [{"text": "x"}], "timeout": 5}}, "wait shorthand expects int (ms)"),
        ({"wait": {"any": [{"text": "x"}]}, "any": [{"text": "y"}]}, "shorthand conflict"),
    ],
)
def test_wait_any_validation_errors(step, message):
    with pytest.raises(ValueError, match=re.escape(message)):
        ScriptStep.model_validate(step)


def test_wait_any_valid():
    s = ScriptStep.model_validate({"wait": {"any": [{"visible": "#a"}, {"text": "x"}]}, "timeout": 500, "as": "g"})
    assert s.action == "wait" and len(s.any_) == 2 and s.timeout == 500 and s.as_name == "g"
    canon = ScriptStep.model_validate({"action": "wait", "any": [{"text": "x"}]})
    assert canon.any_[0].text == "x"
