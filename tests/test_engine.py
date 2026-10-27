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


@pytest.fixture()
def no_sleep(monkeypatch):
    sleeps: list = []
    monkeypatch.setattr(engine, "_sleep", sleeps.append)
    return sleeps


def test_retry_attempts_and_delay(session, calls, no_sleep):
    calls.queue["click"] = [RuntimeError("e1"), RuntimeError("e2"), None]
    out = run_script(session, _script([{"click": "#a", "retry": 2, "retryDelay": 10, "name": "go"}]))
    step = out["steps"][0]
    assert out["success"] and step["attempts"] == 3 and step["name"] == "go"
    assert len(calls) == 3 and no_sleep == [10, 10]


def test_retry_exhausted_fails(session, calls, no_sleep):
    calls.queue["click"] = [RuntimeError("nope")]
    out = run_script(session, _script([{"click": "#a", "retry": 1}, {"open": "x"}]))
    assert not out["success"] and len(out["steps"]) == 1
    assert out["steps"][0]["attempts"] == 2 and out["steps"][0]["error"] == "nope"
    assert no_sleep == [1000]


def test_optional_failure_recorded_and_ignored(session, calls):
    calls.queue["click"] = [RuntimeError("nope")]
    out = run_script(session, _script([{"click": "#a", "optional": True}, {"open": "x"}]))
    first = out["steps"][0]
    assert out["success"] and len(out["steps"]) == 2
    assert first["success"] is True and first["optional"] is True and first["error"] == "nope"


def test_stop_on_error_false_records_and_continues(session, calls):
    calls.queue["click"] = [RuntimeError("nope")]
    out = run_script(session, _script([{"click": "#a", "stopOnError": False}, {"open": "x"}]))
    assert out["success"] is False and len(out["steps"]) == 2
    out = run_script(session, _script([{"click": "#a"}, {"open": "x"}], stopOnError=False))
    assert out["success"] is False and len(out["steps"]) == 2


def test_error_artifacts(session, calls, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    page = session.get_page.return_value
    page.screenshot.side_effect = lambda path: open(path, "wb").write(b"png")
    page.content.return_value = "<html></html>"
    calls.queue["click"] = [RuntimeError("nope")]
    script = _script([{"click": "#a", "stopOnError": False}, {"click": "#b", "stopOnError": False}], errorArtifacts="art")
    out = run_script(session, script)
    assert out["steps"][0]["artifacts"] == {"screenshot": "art/001-click.png", "html": "art/001-click.html"}
    assert out["steps"][1]["artifacts"]["screenshot"] == "art/002-click.png"
    assert (tmp_path / "art" / "001-click.html").read_text() == "<html></html>"


def test_error_artifacts_errors_not_raised(session, calls, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    session.get_page.return_value.screenshot.side_effect = RuntimeError("closed")
    calls.queue["click"] = [RuntimeError("nope")]
    out = run_script(session, _script([{"click": "#a"}]), artifacts_dir="art")
    assert "artifacts" not in out["steps"][0] and out["steps"][0]["error"] == "nope"


def test_artifacts_dir_must_be_inside_cwd(session, calls):
    from autobrowser_client.errors import AppError

    with pytest.raises(AppError):
        run_script(session, _script([{"open": "x"}]), artifacts_dir="/tmp/art")


# --- control blocks -------------------------------------------------------


def _ok(r):
    return r["success"]


def _fail_queue(calls, action="click", msg="nope"):
    calls.queue[action] = [RuntimeError(msg)]


def test_if_branches(session, calls):
    script = _script([
        {"if": {"var": "a"}, "then": [{"open": "then"}], "else": [{"open": "else"}]},
        {"if": {"var": "nope"}, "then": [{"open": "x"}]},
        {"if": {"var": "a"}, "then": [], "name": "n"},
    ], vars={"a": 1})
    out = run_script(session, script)
    s = out["steps"]
    assert out["success"] and [c.url for c in calls] == ["then"]
    assert (s[0]["condition"], s[0]["branch"], len(s[0]["steps"])) == (True, "then", 1)
    assert (s[1]["condition"], s[1]["branch"], s[1]["steps"]) == (False, None, [])
    assert s[2]["name"] == "n"


def test_if_else_branch_taken(session, calls):
    script = _script([{"if": {"var": "a"}, "then": [{"open": "t"}], "else": [{"open": "e"}]}])
    out = run_script(session, script, vars={"a": 0})
    assert out["steps"][0]["branch"] == "else" and calls[0].url == "e"


def test_if_condition_substituted_and_undefined(session, calls):
    out = run_script(session, _script([{"if": {"var": "a", "equals": "${b}"}, "then": [{"open": "t"}]}]), vars={"a": 1, "b": 1})
    assert out["steps"][0]["condition"] is True
    out = run_script(session, _script([{"if": {"var": "a", "equals": "${zz}"}, "then": []}]), vars={"a": 1})
    assert not out["success"] and out["steps"][0]["error"] == "undefined variable: zz"


def test_try_catch_handles_failure(session, calls):
    _fail_queue(calls)
    script = _script([
        {"try": [{"open": "a"}, {"click": "#x"}, {"open": "skipped"}],
         "catch": [{"open": "c/${error.message}/${error.action}"}],
         "finally": [{"open": "f"}]},
        {"open": "after"},
    ])
    out = run_script(session, script)
    assert out["success"]
    assert [c.url for c in calls if c.action == "open"] == ["a", "c/nope/click", "f", "after"]
    node = out["steps"][0]
    assert node["success"] is True
    assert node["steps"][1]["handled"] is True and node["steps"][1]["success"] is False
    assert len(node["catch"]) == 1 and len(node["finally"]) == 1
    assert out["vars"]["error"] == {"message": "nope", "action": "click", "name": None}


def test_try_without_catch_propagates_after_finally(session, calls):
    _fail_queue(calls)
    script = _script([
        {"try": [{"click": "#x"}], "finally": [{"open": "f"}]},
        {"open": "after"},
    ])
    out = run_script(session, script)
    assert not out["success"] and len(out["steps"]) == 1
    node = out["steps"][0]
    assert node["success"] is False and len(node["finally"]) == 1
    assert [c.url for c in calls if c.action == "open"] == ["f"]


def test_failure_in_catch_propagates_after_finally(session, calls):
    script = _script([
        {"try": [{"fail": "one"}], "catch": [{"fail": "two"}], "finally": [{"open": "f"}]},
    ])
    out = run_script(session, script)
    node = out["steps"][0]
    assert not out["success"] and not node["success"]
    assert node["catch"][0]["error"] == "two" and node["finally"][0]["success"]
    assert node["steps"][0]["handled"] is True


def test_nested_try_outer_catches_inner_propagation(session, calls):
    script = _script([
        {"try": [{"try": [{"fail": "deep"}], "finally": [{"open": "inner-f"}]}],
         "catch": [{"open": "outer-c"}]},
    ])
    out = run_script(session, script)
    outer = out["steps"][0]
    inner = outer["steps"][0]
    assert out["success"] and outer["success"] and inner["success"] is False
    assert inner["handled"] is True and inner["steps"][0]["handled"] is True
    assert [c.url for c in calls] == ["inner-f", "outer-c"]


def test_try_does_not_catch_recorded_failure(session, calls):
    _fail_queue(calls)
    script = _script([{"try": [{"click": "#x", "stopOnError": False}, {"open": "next"}], "catch": [{"open": "c"}]}])
    out = run_script(session, script)
    assert not out["success"] and [c.url for c in calls if c.action == "open"] == ["next"]
    assert out["steps"][0]["success"] is False


def test_optional_inside_try_does_not_trigger_catch(session, calls):
    _fail_queue(calls)
    script = _script([{"try": [{"click": "#x", "optional": True}], "catch": [{"open": "c"}]}])
    out = run_script(session, script)
    assert out["success"] and not any(c.action == "open" for c in calls)


def test_fail_action_and_top_level_stop(session, calls):
    out = run_script(session, _script([{"fail": "bad ${a}"}, {"open": "x"}]), vars={"a": 1})
    assert not out["success"] and len(out["steps"]) == 1
    assert out["steps"][0]["error"] == "bad 1" and calls == []


def test_failure_in_nested_block_stops_script(session, calls):
    script = _script([
        {"repeat": 3, "do": [{"open": "a"}, {"fail": "x"}]},
        {"open": "never"},
    ])
    out = run_script(session, script)
    assert not out["success"] and len(calls) == 1
    node = out["steps"][0]
    assert node["success"] is False and len(node["iterations"]) == 1


def test_for_each_items_index_and_literal(session, calls):
    script = _script([
        {"forEach": ["a", "b"], "as": "x", "do": [{"open": "${index}:${x}"}]},
        {"set": {"urls": ["u1", "u2"]}},
        {"forEach": "${urls}", "do": [{"open": "${item}"}]},
    ])
    out = run_script(session, script)
    assert [c.url for c in calls] == ["0:a", "1:b", "u1", "u2"]
    assert len(out["steps"][0]["iterations"]) == 2 and out["success"]


def test_for_each_not_a_list(session, calls):
    out = run_script(session, _script([{"forEach": "${v}", "do": []}]), vars={"v": "str"})
    assert not out["success"] and "expects a list" in out["steps"][0]["error"]


def test_repeat(session, calls):
    out = run_script(session, _script([{"repeat": 3, "do": [{"open": "u${index}"}]}]))
    assert [c.url for c in calls] == ["u0", "u1", "u2"] and len(out["steps"][0]["iterations"]) == 3
    out = run_script(session, _script([{"repeat": "${n}", "do": []}]), vars={"n": "2"})
    assert len(out["steps"][0]["iterations"]) == 2


def test_while_runs_until_false_and_max(session, calls):
    script = _script([
        {"while": {"var": "go"}, "max": 5, "do": [{"open": "x"}, {"if": {"var": "index", "equals": 1}, "then": [{"set": {"go": False}}]}]},
    ], vars={"go": True})
    out = run_script(session, script)
    assert out["success"] and len(out["steps"][0]["iterations"]) == 2

    out = run_script(session, _script([{"while": {"var": "go"}, "max": 3, "do": [{"open": "x"}]}], vars={"go": True}))
    assert not out["success"] and len(out["steps"][0]["iterations"]) == 3
    assert out["steps"][0]["error"] == "while exceeded max 3 iterations"


def test_while_exact_max_is_ok(session, calls):
    script = _script([
        {"while": {"var": "go"}, "max": 2, "do": [{"if": {"var": "index", "equals": 1}, "then": [{"set": {"go": False}}]}]},
    ], vars={"go": True})
    assert run_script(session, script)["success"]


def test_block_validation():
    for bad in (
        {"try": []},
        {"forEach": [1], "as": "1x", "do": []},
        {"if": {"var": "a"}},
        {"while": {"var": "a"}, "max": 0, "do": []},
        {"repeat": 2, "do": [], "bogus": 1},
    ):
        with pytest.raises(ValueError):
            _script([bad])


def test_execute_script_compat(session, calls):
    from autobrowser_client.browser.executor import execute_script
    from autobrowser_client.scenario import ScriptStep

    calls.queue["click"] = [RuntimeError("nope")]
    steps = [ScriptStep.model_validate({"open": "a"}), ScriptStep.model_validate({"click": "#x"}), ScriptStep.model_validate({"open": "b"})]
    results = execute_script(session, steps)
    assert [r["success"] for r in results] == [True, False]
    assert len(execute_script(session, steps, stop_on_error=False)) == 3


def test_locator_shorthand_parsed_after_substitution(session, calls):
    script = _script([
        {"click": "${sel}"},
        {"action": "find", "locator": "${sel2}"},
        {"click": "#${idv}"},
    ], vars={"sel": "xpath=//a", "sel2": "text=Next", "idv": "go"})
    assert script.steps[0].locator.value == "${sel}"
    run_script(session, script)
    assert (calls[0].locator.type, calls[0].locator.value) == ("xpath", "//a")
    assert (calls[1].locator.type, calls[1].locator.value) == ("text", "Next")
    assert (calls[2].locator.type, calls[2].locator.value) == ("id", "go")


def test_locator_shorthand_in_condition(session, calls):
    script = _script([{"assert": {"exists": "${sel}"}}], vars={"sel": "xpath=//a"})
    session.get_page.return_value.locator.return_value.count.return_value = 1
    assert run_script(session, script)["success"]
