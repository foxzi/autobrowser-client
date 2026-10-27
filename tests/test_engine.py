"""Scenario engine tests with a fake session (no real browser)."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from autobrowser_client.browser import engine
from autobrowser_client.browser.engine import UndefinedVariable, run_script, substitute
from autobrowser_client.browser.executor import StepResult
from autobrowser_client.scenario import ScriptDefinition


def test_substitute_types_and_interpolation():
    v = {"n": 5, "items": ["a", "b"], "user": {"name": "bob", "tags": ["x", "y"]}}
    assert substitute("${n}", v) == 5
    assert substitute("${items}", v) == ["a", "b"]
    assert substitute("n=${n}!", v) == "n=5!"
    assert substitute("${user.name}", v) == "bob"
    assert substitute("${user.tags.1}", v) == "y"
    assert substitute("${items.0}-${ items.1 }", v) == "a-b"
    assert substitute({"k": ["${n}", 7]}, v) == {"k": [5, 7]}
    assert substitute(3, v) == 3


def test_substitute_env_and_escape(monkeypatch):
    monkeypatch.setenv("AB_TEST_VAR", "secret")
    assert substitute("${env.AB_TEST_VAR}", {}) == "secret"
    assert substitute("$${x} and ${a}", {"a": 1}) == "${x} and 1"
    assert substitute("$${x}", {}) == "${x}"


@pytest.mark.parametrize("text", ["${nope}", "${user.age}", "${items.5}", "${env.AB_NOT_SET_XYZ}", "a${nope}b"])
def test_substitute_undefined(text):
    with pytest.raises(UndefinedVariable, match="undefined variable"):
        substitute(text, {"user": {}, "items": []})


@pytest.fixture()
def session():
    s = MagicMock()
    s.settings.timeouts.action_ms = 1000
    s.settings.timeouts.navigation_ms = 2000
    return s


@pytest.fixture()
def calls(monkeypatch):
    """Replace execute_step: records concrete steps, results from a per-action queue."""
    class Log(list):
        queue: dict[str, list] = {}

    log = Log()
    queue = log.queue = {}

    def fake(session, step, page_id=None):
        log.append(step)
        r = StepResult(step.action, success=True)
        data = queue.get(step.action, [None])
        data = data.pop(0) if len(data) > 1 else data[0]
        if isinstance(data, Exception):
            r.success = False
            r.error = str(data)
        else:
            r.data = data
        return r

    monkeypatch.setattr(engine, "execute_step", fake)
    return log


def _script(steps, **root):
    return ScriptDefinition.model_validate({"steps": steps, **root})


def test_vars_defaults_and_override(session, calls):
    script = _script([{"open": "https://${host}/${path}"}], vars={"host": "a.com", "path": "x"})
    out = run_script(session, script, vars={"host": "b.org"})
    assert out["success"]
    assert calls[0].url == "https://b.org/x"
    assert out["vars"] == {"host": "b.org", "path": "x"}


def test_set_keeps_types_and_chains(session, calls):
    script = _script([
        {"set": {"n": 3, "names": ["a"]}},
        {"set": {"title": "t-${n}", "again": "${names}"}},
        {"open": "u/${title}"},
    ])
    out = run_script(session, script)
    assert out["vars"] == {"n": 3, "names": ["a"], "title": "t-3", "again": ["a"]}
    assert out["steps"][1]["data"] == {"vars": {"title": "t-3", "again": ["a"]}}
    assert calls[0].url == "u/t-3"


def test_as_stores_value(session, calls):
    calls.queue["extract"] = [{"value": "42"}]
    calls.queue["evaluate"] = [{"result": [1, 2]}]
    calls.queue["read"] = [{"text": "hi"}]
    script = _script([
        {"extract": "h1", "as": "title"},
        {"evaluate": "1", "as": "res"},
        {"read": "h1", "as": "whole"},
        {"click": "#${title}"},
    ])
    out = run_script(session, script)
    assert out["vars"] == {"title": "42", "res": [1, 2], "whole": {"text": "hi"}}
    assert calls[-1].locator.value == "#42"


def test_undefined_variable_is_step_error(session, calls):
    out = run_script(session, _script([{"open": "${missing}"}, {"open": "b"}]))
    assert out["success"] is False
    assert len(out["steps"]) == 1
    assert out["steps"][0]["error"] == "undefined variable: missing"
    assert calls == []


def test_non_string_number_coerced_for_text_field(session, calls):
    run_script(session, _script([{"fill": ["#q", "${n}"]}], vars={"n": 7}))
    assert calls[0].value == "7"


def test_as_validation():
    with pytest.raises(ValueError):
        _script([{"click": "a", "as": "x"}])
    with pytest.raises(ValueError):
        _script([{"extract": "a", "as": "1bad"}])
    with pytest.raises(ValueError):
        _script([{"set": {}}])


def _cond(data: dict, session, variables=None):
    from autobrowser_client.scenario import Condition

    return engine.check_condition(session, Condition.model_validate(data), variables or {})


def test_condition_page_checks(session):
    page = session.get_page.return_value
    page.locator.return_value.count.return_value = 2
    page.locator.return_value.first.is_visible.return_value = False
    page.inner_text.return_value = "Hello world"
    page.url = "https://a.com/x?y=1"
    page.title.return_value = "My Title"
    assert _cond({"exists": "#a"}, session)
    assert not _cond({"visible": "#a"}, session)
    assert _cond({"text": "world"}, session) and not _cond({"text": "bye"}, session)
    assert _cond({"url": "a.com/x"}, session)
    assert _cond({"urlMatches": r"x\?y=\d"}, session) and not _cond({"urlMatches": r"^http:"}, session)
    assert _cond({"title": "Title"}, session)
    page.locator.return_value.count.return_value = 0
    assert not _cond({"exists": "#a"}, session)


def test_condition_never_raises_for_page_errors(session):
    session.get_page.return_value.locator.return_value.count.side_effect = RuntimeError("boom")
    assert _cond({"exists": "#a"}, session) is False
    session.get_page.side_effect = RuntimeError("no page")
    assert _cond({"text": "x"}, session) is False


def test_condition_var(session):
    v = {"a": 5, "s": "hello", "l": ["x"], "z": 0, "u": {"k": "v"}}
    assert _cond({"var": "a"}, session, v) and not _cond({"var": "z"}, session, v)
    assert not _cond({"var": "undefined"}, session, v)
    assert _cond({"var": "a", "equals": 5}, session, v) and not _cond({"var": "a", "equals": 6}, session, v)
    assert _cond({"var": "s", "contains": "ell"}, session, v)
    assert _cond({"var": "l", "contains": "x"}, session, v)
    assert _cond({"var": "s", "matches": "^h.l+o$"}, session, v)
    assert _cond({"var": "u.k", "equals": "v"}, session, v)
    assert not _cond({"var": "undefined", "equals": 1}, session, v)


def test_condition_combinators(session):
    v = {"a": 1}
    t, f = {"var": "a"}, {"var": "nope"}
    assert _cond({"not": f}, session, v) and not _cond({"not": t}, session, v)
    assert _cond({"all": [t, t]}, session, v) and not _cond({"all": [t, f]}, session, v)
    assert _cond({"any": [f, t]}, session, v) and not _cond({"any": [f, f]}, session, v)


def test_condition_timeout_polls(session, monkeypatch):
    page = session.get_page.return_value
    counts = iter([0, 0, 1])
    page.locator.return_value.count.side_effect = lambda: next(counts)
    sleeps = []
    monkeypatch.setattr(engine, "_sleep", sleeps.append)
    assert _cond({"exists": "#a", "timeout": 5000}, session)
    assert sleeps == [250, 250]
    page.locator.return_value.count.side_effect = None
    page.locator.return_value.count.return_value = 0
    assert not _cond({"exists": "#a", "timeout": 30}, session)


@pytest.mark.parametrize("data", [{}, {"exists": "a", "text": "b"}, {"equals": 1}, {"not": {"var": "a"}, "timeout": 5}])
def test_condition_validation(data):
    from autobrowser_client.scenario import Condition

    with pytest.raises(ValueError):
        Condition.model_validate(data)


def test_assert_step(session, calls):
    ok = run_script(session, _script([{"assert": {"var": "a", "equals": 1}}], vars={"a": 1}))
    assert ok["success"] and ok["steps"][0]["data"] == {"condition": True}
    bad = run_script(session, _script([{"assert": {"var": "a", "equals": "${b}"}, "message": "a is ${a}"}], vars={"a": 1, "b": 2}))
    assert bad["success"] is False and bad["steps"][0]["error"] == "a is 1"
    bad = run_script(session, _script([{"assert": {"var": "a", "equals": 2}}], vars={"a": 1}))
    assert bad["steps"][0]["error"].startswith("assertion failed: ")
