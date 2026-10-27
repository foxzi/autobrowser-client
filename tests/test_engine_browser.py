"""Scenario engine against a real headless browser."""

from __future__ import annotations

from pathlib import Path

import pytest

from autobrowser_client.browser.engine import run_script
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import load_settings
from autobrowser_client.scenario import parse_script

PAGE = (
    "data:text/html,<title>T</title><h1>Hello</h1><p id=cookie>We use cookies</p>"
    "<a class=l href='/x'>A</a><a class=l href='/y'>B</a>"
)


@pytest.fixture()
def run(tmp_path: Path):
    settings = load_settings(env={}, overrides={"profiles": {"dir": str(tmp_path / "profiles")}})
    manager = SessionManager(settings)
    try:
        session = manager.create_session(urls=[PAGE], headless=True)
    except Exception as exc:
        pytest.skip(f"browser unavailable: {exc}")

    def _run(text: str, **kw):
        script = parse_script(text, tmp_path)
        return session.call(run_script, session, script, **kw)

    yield _run
    session.stop()


def test_if_exists_and_assert_text(run):
    out = run(
        """
steps:
  - if: {exists: "#cookie"}
    then: [{set: {banner: yes}}]
    else: [{set: {banner: no}}]
  - if: {exists: "#nope"}
    then: [{fail: should not run}]
  - assert: {text: Hello}
  - assert: {not: {visible: "#nope"}}
"""
    )
    assert out["success"], out
    assert out["steps"][0]["branch"] == "then" and out["steps"][1]["branch"] is None
    assert out["vars"]["banner"] is True


def test_try_catch_with_fail(run):
    out = run(
        """
steps:
  - try:
      - fail: custom ${who}
    catch:
      - set: {caught: "${error.message}"}
    finally:
      - set: {done: true}
"""
        , vars={"who": "boom"},
    )
    assert out["success"], out
    assert out["vars"]["caught"] == "custom boom" and out["vars"]["done"] is True


def test_for_each_extract_as(run):
    out = run(
        """
steps:
  - extract: a.l
    attribute: href
    all: true
    as: links
  - forEach: "${links}"
    as: link
    do:
      - set: {last: "${index}:${link}"}
  - extract: h1
    as: title
  - assert: {var: title, equals: Hello}
  - assert: {var: last, equals: "1:/y"}
  - assert: {text: nope, timeout: 300}
    message: expected failure
    optional: true
"""
    )
    assert out["success"], out
    assert out["vars"]["links"] == ["/x", "/y"]
    assert len(out["steps"][1]["iterations"]) == 2
    assert out["steps"][-1]["optional"] is True and out["steps"][-1]["error"] == "expected failure"


def test_failure_artifacts(run, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    out = run("steps:\n  - click: '#missing'\n    timeout: 300\n", artifacts_dir="art")
    assert not out["success"]
    art = out["steps"][0]["artifacts"]
    assert (tmp_path / art["screenshot"]).stat().st_size > 0
    assert "Hello" in (tmp_path / art["html"]).read_text()
