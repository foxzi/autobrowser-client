"""Tests for string-form locator shorthand parsing."""

import pytest
from autobrowser_client.scenario import Locator, ScriptStep


@pytest.mark.parametrize(
    "raw,exp_type,exp_value",
    [
        ("css=button", "css", "button"),
        ("xpath=//div", "xpath", "//div"),
        ("name=username", "name", "username"),
        ("id=submit", "id", "submit"),
        ("class=btn", "class", "btn"),
        ("text=Click me", "text", "Click me"),
        ("attribute=data-id", "attribute", "data-id"),
        ("//div[@id='x']", "xpath", "//div[@id='x']"),
        ("/html/body", "xpath", "/html/body"),
        ("#submit", "id", "submit"),
        (".btn-primary", "class", "btn-primary"),
        ("button.primary", "css", "button.primary"),
        ("button[type=submit]", "css", "button[type=submit]"),
    ],
)
def test_string_form(raw, exp_type, exp_value):
    loc = Locator.model_validate(raw)
    assert loc.type == exp_type
    assert loc.value == exp_value


def test_dict_form_still_works():
    loc = Locator(type="css", value="x")
    assert loc.type == "css"
    assert loc.value == "x"


def test_dict_model_validate():
    loc = Locator.model_validate({"type": "css", "value": "x"})
    assert loc.type == "css"
    assert loc.value == "x"


def test_script_step_end_to_end():
    step = ScriptStep.model_validate({"action": "click", "locator": "css=button"})
    assert step.locator is not None
    assert step.locator.type == "css"
    assert step.locator.value == "button"
