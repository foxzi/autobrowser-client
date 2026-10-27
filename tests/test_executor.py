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


# --- new actions (fake session) ---


def _page(session):
    return session.get_page.return_value


def test_simple_locator_actions(session):
    loc = _page(session).locator.return_value
    for action in ["hover", "dblclick", "focus", "clear", "check", "uncheck"]:
        assert execute_step(session, _step({action: "#a"})).success
    loc.hover.assert_called_once_with(timeout=1000)
    loc.dblclick.assert_called_once_with(timeout=1000)
    loc.focus.assert_called_once_with(timeout=1000)
    loc.clear.assert_called_once_with(timeout=1000)
    loc.check.assert_called_once_with(timeout=1000)
    loc.uncheck.assert_called_once_with(timeout=1000)


def test_select_single_and_multi(session):
    loc = _page(session).locator.return_value.first
    loc.select_option.return_value = ["b"]
    r = execute_step(session, _step({"select": ["#s", "b"]}))
    assert r.data == {"selected": ["b"]}
    loc.select_option.assert_called_with(["b"], timeout=1000)
    execute_step(session, _step({"action": "select", "locator": "#s", "options": ["a", "b"]}))
    loc.select_option.assert_called_with(["a", "b"], timeout=1000)


def test_evaluate_and_history(session):
    page = _page(session)
    page.evaluate.return_value = 42
    assert execute_step(session, _step({"evaluate": "1+41"})).data == {"result": 42}
    page.url = "http://x/"
    page.go_back.return_value = None
    r = execute_step(session, _step({"history": "back"}))
    assert r.data == {"url": "http://x/"}
    page.go_back.assert_called_once_with(timeout=2000, wait_until="commit")


def test_wait_text_gone(session):
    r = execute_step(session, _step({"wait-text": "Loading", "gone": True}))
    assert r.success
    _page(session).get_by_text.assert_called_once_with("Loading")
    _page(session).get_by_text.return_value.first.wait_for.assert_called_once_with(
        state="hidden", timeout=1000
    )


def test_extract_text_attribute_all(session):
    loc = _page(session).locator.return_value
    loc.first.inner_text.return_value = "hi"
    loc.first.get_attribute.return_value = None
    loc.evaluate_all.return_value = ["a", "b"]
    assert execute_step(session, _step({"extract": "h1"})).data == {"value": "hi"}
    r = execute_step(session, _step({"extract": "a", "attribute": "href"}))
    assert r.data == {"value": None}
    loc.first.get_attribute.assert_called_once_with("href", timeout=1000)
    r = execute_step(session, _step({"extract": "a", "attribute": "href", "all": True}))
    assert r.data == {"value": ["a", "b"]}
    assert loc.evaluate_all.call_args.args[1] == "href"


def test_extract_missing_element_fails(session):
    loc = _page(session).locator.return_value
    loc.first.wait_for.side_effect = RuntimeError("timeout")
    r = execute_step(session, _step({"extract": "h1"}))
    assert not r.success and "timeout" in r.error


def test_find_all_and_read(session):
    loc = _page(session).locator.return_value
    loc.count.return_value = 1
    loc.nth.return_value.inner_text.return_value = "t"
    loc.nth.return_value.is_visible.return_value = True
    r = execute_step(session, _step({"find-all": "li"}))
    assert r.data == {"found": 1, "elements": [{"index": 0, "text": "t", "visible": True}]}
    loc.first.inner_text.return_value = "txt"
    assert execute_step(session, _step({"read": "li"})).data["text"] == "txt"


def test_mouse_actions(session):
    page = _page(session)
    assert execute_step(session, _step({"action": "mouse-move", "x": 5, "y": 6, "steps": 3})).success
    page.mouse.move.assert_called_once_with(5, 6, steps=3)
    assert execute_step(session, _step({"mouse-scroll-to": "#a"})).success
    page.locator.return_value.first.scroll_into_view_if_needed.assert_called_once_with(timeout=1000)
    r = execute_step(
        session,
        _step({"action": "mouse-random", "endX": 100, "endY": 50, "steps": 2, "durationMs": 0, "seed": 1}),
    )
    assert r.data == {"points": 3}


def test_submit_and_upload(session):
    loc = _page(session).locator.return_value.first
    assert execute_step(session, _step({"submit": "#f"})).success
    assert "closest('form')" in loc.evaluate.call_args.args[0]
    loc.evaluate.return_value = True
    r = execute_step(session, _step({"upload": ["#f", "a.txt"]}))
    assert r.data == {"files": ["a.txt"]}
    loc.set_input_files.assert_called_once_with(["a.txt"], timeout=1000)


def test_error_message_has_no_empty_api_prefix(session):
    loc = _page(session).locator.return_value
    loc.first.wait_for.side_effect = RuntimeError(": Timeout 300ms exceeded.")
    r = execute_step(session, _step({"extract": "h1"}))
    assert r.error == "Timeout 300ms exceeded."
