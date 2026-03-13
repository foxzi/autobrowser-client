from __future__ import annotations

import asyncio
import logging
import time
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from autobrowser_client import __version__
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import Settings
from autobrowser_client.errors import AppError, BrowserStartError, SessionNotFoundError

logger = logging.getLogger(__name__)


class StartRequest(BaseModel):
    urls: list[str] = Field(default_factory=list)
    profile: str | None = None
    user_agent: str | None = Field(default=None, alias="userAgent")
    headless: bool | None = None
    timeout: int | None = None


def _error_response(exc: AppError, status: int) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={
            "success": False,
            "error": {"code": exc.code, "message": exc.message},
        },
    )


def create_app(settings: Settings) -> FastAPI:
    app = FastAPI(title="autobrowser-client", version=__version__)
    manager = SessionManager(settings)

    @app.exception_handler(AppError)
    async def app_error_handler(_request: Request, exc: AppError) -> JSONResponse:
        status_map: dict[str, int] = {
            "SESSION_NOT_FOUND": 404,
            "BROWSER_START_FAILED": 502,
            "TIMEOUT": 504,
            "CONFIG_ERROR": 400,
            "INVALID_ARGUMENT": 400,
            "NOT_IMPLEMENTED": 501,
        }
        status = status_map.get(exc.code, 500)
        return _error_response(exc, status)

    @app.on_event("shutdown")
    def shutdown() -> None:
        manager.stop_all()

    # --- Service endpoints ---

    @app.get("/health")
    async def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/ready")
    async def ready() -> dict[str, str]:
        return {"status": "ready"}

    @app.get("/api/v1/info")
    async def info() -> dict[str, object]:
        return {
            "name": "autobrowser-client",
            "version": __version__,
            "api": {
                "host": settings.api.host,
                "port": settings.api.port,
            },
        }

    # --- Session management ---

    @app.post("/api/v1/browser/start")
    async def browser_start(body: StartRequest) -> dict[str, Any]:
        session = await asyncio.to_thread(
            manager.create_session,
            urls=body.urls or None,
            profile=body.profile,
            user_agent=body.user_agent,
            headless=body.headless,
        )
        return {
            "success": True,
            **session.info(),
        }

    @app.get("/api/v1/sessions")
    async def list_sessions() -> dict[str, Any]:
        sessions = manager.list_sessions()
        return {"success": True, "sessions": sessions}

    @app.get("/api/v1/session/{session_id}")
    async def get_session(session_id: str) -> dict[str, Any]:
        session = manager.get_session(session_id)
        return {"success": True, **session.info()}

    @app.post("/api/v1/session/{session_id}/stop")
    async def stop_session(session_id: str) -> dict[str, Any]:
        session = await asyncio.to_thread(manager.stop_session, session_id)
        return {"success": True, **session.info()}

    # --- Page data ---

    @app.get("/api/v1/session/{session_id}/cookies")
    async def get_cookies(session_id: str) -> dict[str, Any]:
        session = manager.get_session(session_id)
        cookies = await asyncio.to_thread(session.get_cookies)
        return {"success": True, "sessionId": session_id, "cookies": cookies}

    @app.get("/api/v1/session/{session_id}/headers")
    async def get_headers(session_id: str, page_id: str | None = None) -> dict[str, Any]:
        session = manager.get_session(session_id)
        headers = session.get_headers(page_id)
        return {"success": True, "sessionId": session_id, "headers": headers}

    @app.get("/api/v1/session/{session_id}/content")
    async def get_content(
        session_id: str,
        page_id: str | None = None,
        fmt: str = "html",
    ) -> dict[str, Any]:
        session = manager.get_session(session_id)
        content = await asyncio.to_thread(session.get_content, page_id, fmt)
        return {"success": True, "sessionId": session_id, "content": content}

    return app
