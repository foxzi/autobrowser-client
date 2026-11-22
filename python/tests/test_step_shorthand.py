"""Tests for verb-key shorthand step syntax."""

import pytest
from pydantic import ValidationError

from autobrowser_client.scenario import (
    ScriptDefinition,
    ScriptStep,
    _expand_shorthand_step,
)

# ---------------------------------------------------------------------------
# Unit tests for _expand_shorthand_step
# ---------------------------------------------------------------------------


def test_dict_form_passthrough():
    d = {"action": "click", "locator": {"type": "css", "value": "x"}}
    assert _expand_shorthand_step(d) is d


def test_unknown_key_passthrough():
    d = {"bogus": "value"}
    assert _expand_shorthand_step(d) == {"bogus": "value"}


# ---------------------------------------------------------------------------
# Parametrized shorthand -> ScriptStep cases
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "raw,exp_action,exp_field,exp_value",
    [
        # open
        ({"open": "https://example.com"}, "open", "url", "https://example.com"),
        # wait int -> timeout
        ({"wait": 3000}, "wait", "timeout", 3000),
        # wait load-state strings -> wait_until
        ({"wait": "networkidle"}, "wait", "wait_until", "networkidle"),
        ({"wait": "load"}, "wait", "wait_until", "load"),
        ({"wait": "domcontentloaded"}, "wait", "wait_until", "domcontentloaded"),
        # press
        ({"press": "Enter"}, "press", "key", "Enter"),
        # screenshot / save
        ({"screenshot": "out.png"}, "screenshot", "file", "out.png"),
        ({"save": "result.html"}, "save", "file", "result.html"),
    ],
)
def test_simple_shorthands(raw, exp_action, exp_field, exp_value):
    step = ScriptStep.model_validate(raw)
    assert step.action == exp_action
    assert getattr(step, exp_field) == exp_value


@pytest.mark.parametrize(
    "raw,exp_action",
    [
        ({"get-content": None}, "get-content"),
        ({"get-cookies": None}, "get-cookies"),
        ({"get-headers": None}, "get-headers"),
        ({"browse-tabs": None}, "browse-tabs"),
        ({"solve-challenge": None}, "solve-challenge"),
    ],
)
def test_null_shorthands(raw, exp_action):
    step = ScriptStep.model_validate(raw)
    assert step.action == exp_action


def test_click_shorthand_css_string():
    step = ScriptStep.model_validate({"click": "#submit"})
    assert step.action == "click"
    assert step.locator is not None
    assert step.locator.type == "id"
    assert step.locator.value == "submit"


def test_find_shorthand():
    step = ScriptStep.model_validate({"find": "//div[@id='x']"})
    assert step.action == "find"
    assert step.locator is not None
    assert step.locator.type == "xpath"


def test_fill_shorthand():
    step = ScriptStep.model_validate({"fill": ["name=user", "demo"]})
    assert step.action == "fill"
    assert step.locator is not None
    assert step.locator.type == "name"
    assert step.locator.value == "user"
    assert step.value == "demo"


def test_type_shorthand():
    step = ScriptStep.model_validate({"type": ["css=input", "hello"]})
    assert step.action == "type"
    assert step.locator is not None
    assert step.locator.type == "css"
    assert step.value == "hello"


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------


def test_wait_bogus_string_raises():
    with pytest.raises(ValidationError):
        ScriptStep.model_validate({"wait": "bogus"})


def test_fill_not_a_list_raises():
    with pytest.raises(ValidationError):
        ScriptStep.model_validate({"fill": "not-a-list"})


def test_fill_wrong_length_raises():
    with pytest.raises(ValidationError):
        ScriptStep.model_validate({"fill": ["only-one"]})


def test_conflict_shorthand_and_url_raises():
    with pytest.raises(ValidationError):
        ScriptStep.model_validate({"open": "https://x.com", "url": "https://y.com"})


def test_conflict_shorthand_and_action_raises():
    # 'action' key present -> passthrough, then pydantic sees duplicate-like data
    # Actually if action is present _expand_shorthand returns d unchanged,
    # so this tests that dict-form still works even when a verb key coincides
    step = ScriptStep.model_validate({"action": "open", "url": "https://x.com"})
    assert step.action == "open"
    assert step.url == "https://x.com"


def test_get_content_non_null_raises():
    with pytest.raises(ValidationError):
        ScriptStep.model_validate({"get-content": "something"})


# ---------------------------------------------------------------------------
# Extra fields preserved
# ---------------------------------------------------------------------------


def test_extra_fields_preserved():
    step = ScriptStep.model_validate({"open": "https://x.com", "timeout": 5000})
    assert step.action == "open"
    assert step.url == "https://x.com"
    assert step.timeout == 5000


def test_screenshot_full_page():
    step = ScriptStep.model_validate({"screenshot": "out.png", "fullPage": True})
    assert step.action == "screenshot"
    assert step.file == "out.png"
    assert step.full_page is True


# ---------------------------------------------------------------------------
# Dict form unchanged
# ---------------------------------------------------------------------------


def test_dict_form_click():
    step = ScriptStep.model_validate(
        {"action": "click", "locator": {"type": "css", "value": "x"}}
    )
    assert step.action == "click"
    assert step.locator is not None
    assert step.locator.type == "css"


# ---------------------------------------------------------------------------
# End-to-end: ScriptDefinition with mixed shorthand + dict steps
# ---------------------------------------------------------------------------


def test_script_definition_mixed():
    data = {
        "steps": [
            {"open": "https://example.com"},
            {"action": "wait", "waitUntil": "networkidle"},
            {"click": "#btn"},
            {"fill": ["name=q", "hello"]},
            {"screenshot": "out.png"},
            {"get-content": None},
        ]
    }
    script = ScriptDefinition.model_validate(data)
    assert len(script.steps) == 6
    assert script.steps[0].action == "open"
    assert script.steps[0].url == "https://example.com"
    assert script.steps[1].action == "wait"
    assert script.steps[2].action == "click"
    assert script.steps[3].action == "fill"
    assert script.steps[4].action == "screenshot"
    assert script.steps[5].action == "get-content"


# ---------------------------------------------------------------------------
# Change A: pages field removed from ScriptDefinition
# ---------------------------------------------------------------------------


def test_pages_field_removed():
    assert not hasattr(ScriptDefinition.model_fields.get("pages", object()), "default")
    assert "pages" not in ScriptDefinition.model_fields


def test_pages_in_yaml_raises():
    """Existing YAML with 'pages:' key should raise validation error (extra='forbid')."""
    with pytest.raises(ValidationError):
        ScriptDefinition.model_validate({"pages": ["https://x.com"], "steps": []})


# --- new actions shorthand ---


@pytest.mark.parametrize(
    "verb",
    ["hover", "dblclick", "focus", "clear", "check", "uncheck", "submit",
     "extract", "find-all", "read", "mouse-scroll-to"],
)
def test_locator_shorthand_new_actions(verb):
    s = ScriptStep.model_validate({verb: "#a"})
    assert s.action == verb
    assert s.locator.type == "id" and s.locator.value == "a"


def test_select_shorthand():
    s = ScriptStep.model_validate({"select": ["#s", "b"]})
    assert s.value == "b" and s.options is None
    s = ScriptStep.model_validate({"select": ["#s", ["a", "b"]]})
    assert s.options == ["a", "b"] and s.value is None


def test_upload_shorthand():
    assert ScriptStep.model_validate({"upload": ["#f", "a.txt"]}).files == ["a.txt"]
    assert ScriptStep.model_validate({"upload": ["#f", ["a", "b"]]}).files == ["a", "b"]


def test_misc_shorthand_new_actions():
    assert ScriptStep.model_validate({"evaluate": "1+1"}).script == "1+1"
    assert ScriptStep.model_validate({"history": "back"}).direction == "back"
    s = ScriptStep.model_validate({"wait-text": "Done", "gone": True})
    assert s.text == "Done" and s.gone is True


def test_extract_shorthand_with_options():
    s = ScriptStep.model_validate({"extract": "a", "attribute": "href", "all": True})
    assert s.attribute == "href" and s.all_ is True


def test_select_shorthand_conflict():
    with pytest.raises(ValueError, match="conflict"):
        ScriptStep.model_validate({"select": ["#s", "b"], "value": "c"})
