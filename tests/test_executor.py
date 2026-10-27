"""Executor tests with a fake session (no real browser)."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from autobrowser_client.browser import executor
from autobrowser_client.browser.executor import _validate_output_path, execute_step
from autobrowser_client.errors import AppError
from autobrowser_client.scenario import ScriptStep


@pytest.fixture()
def session():
    s = MagicMock()
    s.settings.timeouts.action_ms = 1000
    s.settings.timeouts.navigation_ms = 2000
    return s


def _step(data: dict) -> ScriptStep:
    return ScriptStep.model_validate(data)


def test_output_path_sibling_prefix_rejected(tmp_path, monkeypatch):
    base = tmp_path / "work"
    (tmp_path / "work-evil").mkdir()
    base.mkdir()
    monkeypatch.chdir(base)
    with pytest.raises(AppError):
        _validate_output_path("../work-evil/x.txt")
    _validate_output_path("sub/x.txt")


def test_output_path_absolute_rejected():
    with pytest.raises(AppError):
        _validate_output_path("/tmp/x.txt")


def test_wait_visible_requires_locator():
    with pytest.raises(ValueError, match="requires 'locator'"):
        _step({"action": "wait", "waitUntil": "visible"})
    with pytest.raises(ValueError, match="requires 'locator'"):
        _step({"action": "wait", "waitUntil": "hidden"})


def test_wait_visible_with_locator(session):
    r = execute_step(session, _step({"action": "wait", "waitUntil": "visible", "locator": "#a"}))
    assert r.success
    session.get_page.return_value.locator.return_value.wait_for.assert_called_once_with(
        state="visible", timeout=1000
    )


def test_type_randomized_delay(session, monkeypatch):
    page = session.get_page.return_value
    monkeypatch.setattr(executor.random, "uniform", lambda a, b: (a, b))
    r = execute_step(session, _step({"type": ["#q", "ab"], "delay": 100}))
    assert r.success
    assert [c.args for c in page.keyboard.type.call_args_list] == [("a",), ("b",)]
    assert [c.args[0] for c in page.wait_for_timeout.call_args_list] == [(50.0, 150.0)] * 2


def test_press_with_locator(session):
    page = session.get_page.return_value
    r = execute_step(session, _step({"press": "Enter", "locator": "#q"}))
    assert r.success
    page.locator.return_value.press.assert_called_once_with("Enter", timeout=1000)
    page.keyboard.press.assert_not_called()


def test_press_without_locator(session):
    r = execute_step(session, _step({"press": "Enter"}))
    assert r.success
    session.get_page.return_value.keyboard.press.assert_called_once_with("Enter")
