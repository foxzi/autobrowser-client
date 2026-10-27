"""New scenario actions against a real headless browser."""

from __future__ import annotations

from pathlib import Path

import pytest

from autobrowser_client.browser.executor import execute_step
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import load_settings
from autobrowser_client.scenario import ScriptStep

PAGE = (
    "data:text/html,<title>T</title><h1>Hello</h1>"
    "<a href='/x'>A</a><a href='/y'>B</a>"
    "<input id=c type=checkbox><select id=s><option>a</option><option>b</option></select>"
)


@pytest.fixture()
def run(tmp_path: Path):
    settings = load_settings(env={}, overrides={"profiles": {"dir": str(tmp_path / "profiles")}})
    manager = SessionManager(settings)
    try:
        session = manager.create_session(urls=[PAGE], headless=True)
    except Exception as exc:
        pytest.skip(f"browser unavailable: {exc}")

    def _run(data: dict):
        return session.call(execute_step, session, ScriptStep.model_validate(data)).to_dict()

    yield _run
    session.stop()


def test_extract_select_check_evaluate(run):
    assert run({"extract": "h1"})["data"] == {"value": "Hello"}
    assert run({"extract": "a", "attribute": "href", "all": True})["data"] == {"value": ["/x", "/y"]}
    assert run({"extract": "a", "attribute": "nope"})["data"] == {"value": None}
    assert run({"select": ["#s", "b"]})["data"] == {"selected": ["b"]}
    assert run({"check": "#c"})["success"]
    assert run({"evaluate": "document.getElementById('c').checked"})["data"] == {"result": True}
    assert run({"uncheck": "#c"})["success"]
    assert run({"evaluate": "document.getElementById('c').checked"})["data"] == {"result": False}
    assert run({"wait-text": "Hello"})["success"]
    assert run({"find-all": "a"})["data"]["found"] == 2
    assert run({"wait-text": "nope", "timeout": 300})["success"] is False
