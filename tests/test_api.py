"""Integration tests for HTTP API (no browser required for most)."""

from __future__ import annotations

import shutil
from pathlib import Path

import pytest
from httpx import ASGITransport, AsyncClient

from autobrowser_client.api.app import create_app
from autobrowser_client.config import load_settings


@pytest.fixture()
def settings(tmp_path: Path):
    return load_settings(
        env={},
        overrides={"profiles": {"dir": str(tmp_path / "profiles")}},
    )


@pytest.fixture()
def app(settings):
    return create_app(settings)


@pytest.fixture()
async def client(app):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac


@pytest.mark.anyio
async def test_health(client: AsyncClient):
    r = await client.get("/health")
    assert r.status_code == 200
    assert r.json() == {"status": "ok"}


@pytest.mark.anyio
async def test_ready(client: AsyncClient):
    r = await client.get("/ready")
    assert r.status_code == 200
    assert r.json() == {"status": "ready"}


@pytest.mark.anyio
async def test_info(client: AsyncClient):
    r = await client.get("/api/v1/info")
    assert r.status_code == 200
    data = r.json()
    assert data["name"] == "autobrowser-client"
    assert "version" in data


@pytest.mark.anyio
async def test_sessions_empty(client: AsyncClient):
    r = await client.get("/api/v1/sessions")
    assert r.status_code == 200
    data = r.json()
    assert data["success"] is True
    assert data["sessions"] == []


@pytest.mark.anyio
async def test_session_not_found(client: AsyncClient):
    r = await client.get("/api/v1/session/nonexistent")
    assert r.status_code == 404
    data = r.json()
    assert data["success"] is False
    assert data["error"]["code"] == "SESSION_NOT_FOUND"


@pytest.mark.anyio
async def test_stop_nonexistent_session(client: AsyncClient):
    r = await client.post("/api/v1/session/nonexistent/stop")
    assert r.status_code == 404


@pytest.mark.anyio
async def test_create_profile(client: AsyncClient, settings):
    r = await client.post("/api/v1/profile/create", json={"name": "test_prof"})
    assert r.status_code == 200
    data = r.json()
    assert data["success"] is True
    assert data["profile"] == "test_prof"
    assert Path(settings.profiles.dir, "test_prof").is_dir()


@pytest.mark.anyio
async def test_create_duplicate_profile(client: AsyncClient, settings):
    Path(settings.profiles.dir, "dup").mkdir(parents=True)
    r = await client.post("/api/v1/profile/create", json={"name": "dup"})
    assert r.status_code == 409


@pytest.mark.anyio
async def test_create_profile_invalid_name(client: AsyncClient):
    r = await client.post("/api/v1/profile/create", json={"name": "../escape"})
    assert r.status_code == 400

    r = await client.post("/api/v1/profile/create", json={"name": ".hidden"})
    assert r.status_code == 400

    r = await client.post("/api/v1/profile/create", json={"name": ""})
    assert r.status_code == 400


@pytest.mark.anyio
async def test_list_profiles(client: AsyncClient, settings):
    profiles_dir = Path(settings.profiles.dir)
    profiles_dir.mkdir(parents=True, exist_ok=True)
    (profiles_dir / "alpha").mkdir()
    (profiles_dir / "beta").mkdir()

    r = await client.get("/api/v1/profiles")
    assert r.status_code == 200
    data = r.json()
    assert data["success"] is True
    assert "alpha" in data["profiles"]
    assert "beta" in data["profiles"]


@pytest.mark.anyio
async def test_list_profiles_empty(client: AsyncClient):
    r = await client.get("/api/v1/profiles")
    assert r.status_code == 200
    data = r.json()
    assert data["profiles"] == []


@pytest.mark.anyio
async def test_openapi_json(client: AsyncClient):
    r = await client.get("/openapi.json")
    assert r.status_code == 200
    data = r.json()
    assert "paths" in data
    assert "/health" in data["paths"]


@pytest.mark.anyio
async def test_token_auth():
    """When token is set, requests without it are rejected."""
    settings = load_settings(
        env={},
        overrides={"api": {"token": "test-secret"}},
    )
    app = create_app(settings)
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # Health should work without auth
        r = await client.get("/health")
        assert r.status_code == 200

        # API endpoint should fail without auth
        r = await client.get("/api/v1/sessions")
        assert r.status_code == 401

        # With correct token
        r = await client.get(
            "/api/v1/sessions",
            headers={"Authorization": "Bearer test-secret"},
        )
        assert r.status_code == 200

        # With wrong token
        r = await client.get(
            "/api/v1/sessions",
            headers={"Authorization": "Bearer wrong"},
        )
        assert r.status_code == 401


SCRIPT_PAGE = "data:text/html,<title>T</title><h1>Hello</h1>"


@pytest.fixture()
async def browser_session(client: AsyncClient):
    r = await client.post("/api/v1/browser/start", json={"urls": [SCRIPT_PAGE], "headless": True})
    if r.status_code != 200:
        pytest.skip(f"browser unavailable: {r.text}")
    sid = r.json()["sessionId"]
    yield sid
    await client.post(f"/api/v1/session/{sid}/stop")


@pytest.mark.anyio
async def test_run_script_object_and_yaml(client: AsyncClient, browser_session: str):
    url = f"/api/v1/session/{browser_session}/script"
    obj = {"steps": [{"extract": "h1", "as": "heading"}, {"set": {"n": "${k}"}}]}
    r = await client.post(url, json={"script": obj, "vars": {"k": 5}})
    assert r.status_code == 200
    data = r.json()
    assert data["success"] is True
    assert data["sessionId"] == browser_session
    assert data["vars"] == {"k": 5, "heading": "Hello", "n": 5}
    assert [s["action"] for s in data["steps"]] == ["extract", "set"]
    assert "durationMs" in data

    r = await client.post(url, json={"script": "steps:\n  - assert: {text: Hello}\n"})
    assert r.json()["success"] is True


@pytest.mark.anyio
async def test_run_script_failure_is_200_with_error(client: AsyncClient, browser_session: str):
    r = await client.post(
        f"/api/v1/session/{browser_session}/script", json={"script": {"steps": [{"fail": "boom"}]}}
    )
    assert r.status_code == 200
    data = r.json()
    assert data["success"] is False
    assert data["steps"][0]["error"] == "boom"


@pytest.mark.anyio
async def test_run_script_invalid_script(client: AsyncClient, browser_session: str):
    url = f"/api/v1/session/{browser_session}/script"
    for script in ({"steps": [{"bogus": 1}]}, "steps: [", "- not a mapping"):
        r = await client.post(url, json={"script": script})
        assert r.status_code == 400, script
        assert r.json()["error"]["code"] == "SCRIPT_VALIDATION_ERROR"


@pytest.mark.anyio
async def test_run_script_session_not_found(client: AsyncClient):
    r = await client.post("/api/v1/session/nope/script", json={"script": {"steps": []}})
    assert r.status_code == 404
