"""Unit tests for scenario loading and validation."""

from __future__ import annotations

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
        "pages": ["https://example.com"],
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
