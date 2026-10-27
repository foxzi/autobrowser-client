"""Agent-oriented page operations against a real headless browser."""

from __future__ import annotations

import time
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from autobrowser_client.api.app import create_app
from autobrowser_client.config import load_settings

PAGE = (
    "data:text/html,<title>T</title>"
    "<button onclick=\"document.body.insertAdjacentHTML('beforeend','<p>clicked</p>')\">Go</button>"
    "<button onclick=\"window.open('about:blank')\">Pop</button>"
)


@pytest.fixture()
def session(tmp_path: Path):
    settings = load_settings(
        env={}, overrides={"profiles": {"dir": str(tmp_path / "profiles")}}
    )
    with TestClient(create_app(settings)) as client:
        resp = client.post(
            "/api/v1/browser/start", json={"urls": [PAGE], "headless": True}
        )
        if resp.status_code != 200:
            pytest.skip(f"browser unavailable: {resp.text[:200]}")
        sid = resp.json()["sessionId"]
        yield client, f"/api/v1/session/{sid}"
        client.post(f"/api/v1/session/{sid}/stop")


def test_snapshot_ref_click_and_wait_text(session):
    client, base = session
    snap = client.get(f"{base}/pages/page_0/snapshot").json()["result"]
    assert snap["title"] == "T"
    assert 'button "Go" [ref=e2]' in snap["snapshot"]

    click = client.post(
        f"{base}/pages/page_0/elements/click",
        json={"locator": {"type": "ref", "value": "e2"}},
    )
    assert click.json()["success"]
    assert client.post(
        f"{base}/pages/page_0/wait-text", json={"text": "clicked"}
    ).json()["success"]


def test_evaluate_screenshot_and_errors(session):
    client, base = session
    assert client.post(
        f"{base}/pages/page_0/evaluate", json={"expression": "document.title"}
    ).json()["result"] == {"result": "T"}

    shot = client.post(f"{base}/pages/page_0/screenshot", json={}).json()["result"]
    assert shot["mimeType"] == "image/png" and shot["data"]

    bad_js = client.post(f"{base}/pages/page_0/evaluate", json={"expression": "nope()"})
    assert (
        bad_js.status_code == 422 and bad_js.json()["error"]["code"] == "ACTION_FAILED"
    )

    missing = client.post(
        f"{base}/pages/page_0/elements/click",
        json={"locator": {"type": "ref", "value": "e99"}, "timeout": 500},
    )
    assert missing.status_code == 504 and missing.json()["error"]["code"] == "TIMEOUT"

    assert (
        client.post(f"{base}/pages/nope/evaluate", json={"expression": "1"}).status_code
        == 404
    )


def test_tabs_popup_new_and_close(session):
    client, base = session
    client.post(
        f"{base}/pages/page_0/elements/click",
        json={"locator": {"type": "text", "value": "Pop"}},
    )
    time.sleep(0.5)
    assert "page_1" in client.get(f"{base}/pages").json()["pages"]

    new_id = client.post(f"{base}/pages", json={}).json()["pageId"]
    assert new_id == "page_2"
    client.delete(f"{base}/pages/{new_id}")
    assert new_id not in client.get(f"{base}/pages").json()["pages"]


def test_dialog_policy_and_logs(session):
    client, base = session
    client.post(f"{base}/pages/page_0/evaluate", json={"expression": "document.title = String(confirm('q'))"})
    assert client.post(f"{base}/pages/page_0/evaluate", json={"expression": "document.title"}).json()["result"]["result"] == "false"

    client.post(f"{base}/dialog-policy", json={"accept": True, "promptText": "hi"})
    client.post(f"{base}/pages/page_0/evaluate", json={"expression": "document.title = prompt('name')"})
    assert client.post(f"{base}/pages/page_0/evaluate", json={"expression": "document.title"}).json()["result"]["result"] == "hi"

    dialogs = client.get(f"{base}/pages/page_0/logs/dialogs?clear=true").json()["items"]
    assert [(d["type"], d["accepted"]) for d in dialogs] == [("confirm", False), ("prompt", True)]
    assert client.get(f"{base}/pages/page_0/logs/dialogs").json()["items"] == []

    client.post(f"{base}/pages/page_0/evaluate", json={"expression": "fetch('http://127.0.0.1:9/x').catch(() => 0)"})
    time.sleep(0.3)
    net = client.get(f"{base}/pages/page_0/logs/network").json()["items"]
    assert any(n["failure"] for n in net)
    assert client.get(f"{base}/pages/page_0/logs/bogus").status_code == 400
