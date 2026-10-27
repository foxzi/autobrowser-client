"""include resolution and parse_script."""

from __future__ import annotations

from pathlib import Path

import pytest

from autobrowser_client.errors import ScriptValidationError
from autobrowser_client.scenario import ScriptStep, load_script, parse_script


def _write(path: Path, text: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")
    return path


def test_include_relative_flattens_and_merges_vars(tmp_path):
    _write(tmp_path / "common" / "login.yaml", "vars: {user: lib, extra: 1}\nsteps:\n  - open: https://x/login\n")
    main = _write(
        tmp_path / "main.yaml",
        "vars: {user: me}\nsteps:\n  - include: common/login.yaml\n  - try:\n      - include: common/login.yaml\n    catch: []\n",
    )
    script = load_script(main)
    assert script.vars == {"user": "me", "extra": 1}
    assert isinstance(script.steps[0], ScriptStep) and script.steps[0].url == "https://x/login"
    assert script.steps[1].try_[0].url == "https://x/login"


def test_nested_include_is_relative_to_including_file(tmp_path):
    _write(tmp_path / "a" / "b" / "leaf.yaml", "steps: [{open: leaf}]")
    _write(tmp_path / "a" / "mid.yaml", "steps: [{include: b/leaf.yaml}]")
    main = _write(tmp_path / "main.yaml", "steps: [{include: a/mid.yaml}]")
    assert load_script(main).steps[0].url == "leaf"


def test_include_cycle(tmp_path):
    _write(tmp_path / "a.yaml", "steps: [{include: b.yaml}]")
    _write(tmp_path / "b.yaml", "steps: [{include: a.yaml}]")
    with pytest.raises(ScriptValidationError, match="cycle"):
        load_script(tmp_path / "a.yaml")


def test_include_escape_and_absolute_rejected(tmp_path):
    _write(tmp_path / "outside.yaml", "steps: [{open: x}]")
    main = _write(tmp_path / "sub" / "main.yaml", "steps: [{include: ../outside.yaml}]")
    with pytest.raises(ScriptValidationError, match="escapes"):
        load_script(main)
    abs_main = _write(tmp_path / "abs.yaml", f"steps: [{{include: {tmp_path / 'outside.yaml'}}}]")
    with pytest.raises(ScriptValidationError, match="Absolute"):
        load_script(abs_main)


def test_include_missing_and_extra_keys(tmp_path):
    main = _write(tmp_path / "m.yaml", "steps: [{include: nope.yaml}]")
    with pytest.raises(ScriptValidationError, match="does not exist"):
        load_script(main)
    main = _write(tmp_path / "m2.yaml", "steps: [{include: a.yaml, name: x}]")
    with pytest.raises(ScriptValidationError, match="no other keys"):
        load_script(main)


def test_parse_script_dict_text_and_base_dir(tmp_path):
    _write(tmp_path / "inc.yaml", "steps: [{open: inc}]")
    assert parse_script({"steps": [{"open": "a"}]}, tmp_path).steps[0].url == "a"
    assert parse_script("steps:\n  - include: inc.yaml\n", tmp_path).steps[0].url == "inc"
    with pytest.raises(ScriptValidationError):
        parse_script("- just a list", tmp_path)
    with pytest.raises(ScriptValidationError):
        parse_script({"steps": [{"action": "open"}]}, tmp_path)
