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
