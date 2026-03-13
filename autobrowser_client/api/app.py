from __future__ import annotations

import asyncio
import logging
import time
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from autobrowser_client import __version__
from autobrowser_client.browser import actions
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import Settings
from autobrowser_client.errors import AppError, BrowserStartError, SessionNotFoundError

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Request models
# ---------------------------------------------------------------------------

class StartRequest(BaseModel):
    urls: list[str] = Field(default_factory=list)
    profile: str | None = None
    user_agent: str | None = Field(default=None, alias="userAgent")
    headless: bool | None = None
    timeout: int | None = None


class LocatorBody(BaseModel):
    type: str = "css"
    value: str


class ElementRequest(BaseModel):
    locator: LocatorBody
    timeout: int = 10000


class ClickRequest(BaseModel):
    locator: LocatorBody
    timeout: int = 10000
    double: bool = False


class FillRequest(BaseModel):
    locator: LocatorBody
    value: str
    clear_before: bool = Field(default=False, alias="clearBefore")
    timeout: int = 10000


class SelectRequest(BaseModel):
    locator: LocatorBody
    option: str | list[str]
    timeout: int = 10000


class CheckRequest(BaseModel):
    locator: LocatorBody
    checked: bool = True
    timeout: int = 10000


class WaitRequest(BaseModel):
    locator: LocatorBody | None = None
    state: str = "visible"
    wait_until: str | None = Field(default=None, alias="waitUntil")
    timeout: int = 10000


class PressRequest(BaseModel):
    key: str


class MouseMoveRequest(BaseModel):
    x: float
    y: float
    steps: int = 1


class ScrollRequest(BaseModel):
    delta_x: float = Field(default=0, alias="deltaX")
    delta_y: float = Field(default=0, alias="deltaY")


class MouseRandomRequest(BaseModel):
    start_x: float = Field(alias="startX")
    start_y: float = Field(alias="startY")
    end_x: float = Field(alias="endX")
    end_y: float = Field(alias="endY")
    steps: int = 25
    duration_ms: int = Field(default=300, alias="durationMs")
    jitter: float = 3.0
    seed: int | None = None


class ActionChainRequest(BaseModel):
    steps: list[dict[str, Any]]
    timeout: int = 10000


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _error_response(exc: AppError, status: int) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={
            "success": False,
            "error": {"code": exc.code, "message": exc.message},
        },
    )


def _timed(fn: Any, *args: Any, **kwargs: Any) -> tuple[Any, float]:
    """Run sync function, return (result, duration_ms)."""
    t0 = time.monotonic()
    result = fn(*args, **kwargs)
    return result, round((time.monotonic() - t0) * 1000, 1)


# ---------------------------------------------------------------------------
# App factory
# ---------------------------------------------------------------------------

def create_app(settings: Settings) -> FastAPI:
    app = FastAPI(
        title="autobrowser-client",
        version=__version__,
        description="HTTP API for browser automation with patchright.",
        openapi_tags=[
            {"name": "health", "description": "Health and readiness probes"},
            {"name": "info", "description": "Server information"},
            {"name": "profiles", "description": "Profile management"},
            {"name": "sessions", "description": "Browser session lifecycle"},
            {"name": "page-data", "description": "Cookies, headers, page content"},
            {"name": "dom", "description": "DOM element interaction"},
            {"name": "mouse", "description": "Mouse control"},
            {"name": "keyboard", "description": "Keyboard control"},
            {"name": "actions", "description": "Action chains"},
        ],
    )
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

    # --- Optional token auth middleware ---
    if settings.api.token:
        from starlette.middleware.base import BaseHTTPMiddleware
        from starlette.responses import JSONResponse as StarletteJSON

        class TokenAuthMiddleware(BaseHTTPMiddleware):
            async def dispatch(self, request: Request, call_next: Any) -> Any:
                if request.url.path in ("/health", "/ready", "/docs", "/openapi.json"):
                    return await call_next(request)
                auth = request.headers.get("authorization", "")
                expected = f"Bearer {settings.api.token}"
                if auth != expected:
                    return StarletteJSON(
                        status_code=401,
                        content={"success": False, "error": {"code": "UNAUTHORIZED", "message": "Invalid or missing token"}},
                    )
                return await call_next(request)

        app.add_middleware(TokenAuthMiddleware)

    @app.on_event("shutdown")
    def shutdown() -> None:
        manager.stop_all()

    # =======================================================================
    # Service endpoints
    # =======================================================================

    @app.get("/health", tags=["health"])
    async def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/ready", tags=["health"])
    async def ready() -> dict[str, str]:
        return {"status": "ready"}

    @app.get("/api/v1/info", tags=["info"])
    async def info() -> dict[str, object]:
        return {
            "name": "autobrowser-client",
            "version": __version__,
            "api": {"host": settings.api.host, "port": settings.api.port},
        }

    # =======================================================================
    # Profile management
    # =======================================================================

    @app.post("/api/v1/profile/create", tags=["profiles"])
    async def create_profile(body: dict[str, Any]) -> dict[str, Any]:
        name = body.get("name")
        if not name or not isinstance(name, str):
            raise HTTPException(status_code=400, detail="Field 'name' is required")
        name = name.strip()
        if not name or "/" in name or "\\" in name or name.startswith("."):
            raise HTTPException(status_code=400, detail="Invalid profile name")
        profile_dir = Path(settings.profiles.dir) / name
        if profile_dir.exists():
            raise HTTPException(
                status_code=409, detail=f"Profile '{name}' already exists"
            )
        profile_dir.mkdir(parents=True, exist_ok=True)
        logger.info("Created profile: %s", name)
        return {"success": True, "profile": name, "path": str(profile_dir)}

    @app.get("/api/v1/profiles", tags=["profiles"])
    async def list_profiles() -> dict[str, Any]:
        profiles_dir = Path(settings.profiles.dir)
        profiles: list[str] = []
        if profiles_dir.is_dir():
            profiles = sorted(
                d.name for d in profiles_dir.iterdir() if d.is_dir()
            )
        return {"success": True, "profiles": profiles}

    # =======================================================================
    # Session management
    # =======================================================================

    @app.post("/api/v1/browser/start", tags=["sessions"])
    async def browser_start(body: StartRequest) -> dict[str, Any]:
        session = await asyncio.to_thread(
            manager.create_session,
            urls=body.urls or None,
            profile=body.profile,
            user_agent=body.user_agent,
            headless=body.headless,
        )
        return {"success": True, **session.info()}

    @app.get("/api/v1/sessions", tags=["sessions"])
    async def list_sessions() -> dict[str, Any]:
        return {"success": True, "sessions": manager.list_sessions()}

    @app.get("/api/v1/session/{session_id}", tags=["sessions"])
    async def get_session(session_id: str) -> dict[str, Any]:
        return {"success": True, **manager.get_session(session_id).info()}

    @app.post("/api/v1/session/{session_id}/stop", tags=["sessions"])
    async def stop_session(session_id: str) -> dict[str, Any]:
        session = await asyncio.to_thread(manager.stop_session, session_id)
        return {"success": True, **session.info()}

    # =======================================================================
    # Page data
    # =======================================================================

    @app.get("/api/v1/session/{session_id}/cookies", tags=["page-data"])
    async def get_cookies(session_id: str) -> dict[str, Any]:
        session = manager.get_session(session_id)
        cookies = await asyncio.to_thread(session.get_cookies)
        return {"success": True, "sessionId": session_id, "cookies": cookies}

    @app.get("/api/v1/session/{session_id}/headers", tags=["page-data"])
    async def get_headers(session_id: str, page_id: str | None = None) -> dict[str, Any]:
        session = manager.get_session(session_id)
        return {"success": True, "sessionId": session_id, "headers": session.get_headers(page_id)}

    @app.get("/api/v1/session/{session_id}/content", tags=["page-data"])
    async def get_content(session_id: str, page_id: str | None = None, fmt: str = "html") -> dict[str, Any]:
        session = manager.get_session(session_id)
        content = await asyncio.to_thread(session.get_content, page_id, fmt)
        return {"success": True, "sessionId": session_id, "content": content}

    # =======================================================================
    # DOM interaction
    # =======================================================================

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/find", tags=["dom"])
    async def api_find(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.find_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "find", "durationMs": ms, "result": data}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/find-all", tags=["dom"])
    async def api_find_all(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.find_all_elements,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "find-all", "durationMs": ms, "result": data}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/read", tags=["dom"])
    async def api_read(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.read_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "read", "durationMs": ms, "result": data}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/click", tags=["dom"])
    async def api_click(session_id: str, page_id: str, body: ClickRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.click_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout, body.double,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "click", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/hover", tags=["dom"])
    async def api_hover(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.hover_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "hover", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/focus", tags=["dom"])
    async def api_focus(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.focus_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "focus", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/fill", tags=["dom"])
    async def api_fill(session_id: str, page_id: str, body: FillRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.fill_element,
            session, page_id, body.locator.type, body.locator.value,
            body.value, body.clear_before, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "fill", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/clear", tags=["dom"])
    async def api_clear(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.clear_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "clear", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/select", tags=["dom"])
    async def api_select(session_id: str, page_id: str, body: SelectRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.select_option,
            session, page_id, body.locator.type, body.locator.value, body.option, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "select", "durationMs": ms, "result": {"selected": data}}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/check", tags=["dom"])
    async def api_check(session_id: str, page_id: str, body: CheckRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        action_name = "check" if body.checked else "uncheck"
        _, ms = await asyncio.to_thread(
            _timed, actions.check_element,
            session, page_id, body.locator.type, body.locator.value, body.checked, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": action_name, "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/forms/submit", tags=["dom"])
    async def api_submit(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.submit_form,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "submit", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/elements/wait", tags=["dom"])
    async def api_wait(session_id: str, page_id: str, body: WaitRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        if body.locator:
            _, ms = await asyncio.to_thread(
                _timed, actions.wait_for_element,
                session, page_id, body.locator.type, body.locator.value, body.state, body.timeout,
            )
        else:
            _, ms = await asyncio.to_thread(
                _timed, actions.wait_for_navigation,
                session, page_id, None, body.wait_until or "load", body.timeout,
            )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "wait", "durationMs": ms}

    # =======================================================================
    # Mouse and keyboard
    # =======================================================================

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/mouse/move", tags=["mouse"])
    async def api_mouse_move(session_id: str, page_id: str, body: MouseMoveRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.mouse_move,
            session, page_id, body.x, body.y, body.steps,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "mouse-move", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/mouse/move-to", tags=["mouse"])
    async def api_mouse_move_to(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.mouse_move_to_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "mouse-move-to", "durationMs": ms, "result": data}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/mouse/scroll", tags=["mouse"])
    async def api_scroll(session_id: str, page_id: str, body: ScrollRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.mouse_scroll,
            session, page_id, body.delta_x, body.delta_y,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "scroll", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/mouse/scroll-to", tags=["mouse"])
    async def api_scroll_to(session_id: str, page_id: str, body: ElementRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.mouse_scroll_to_element,
            session, page_id, body.locator.type, body.locator.value, body.timeout,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "scroll-to", "durationMs": ms}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/mouse/move-random", tags=["mouse"])
    async def api_mouse_random(session_id: str, page_id: str, body: MouseRandomRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        data, ms = await asyncio.to_thread(
            _timed, actions.mouse_move_random,
            session, page_id,
            body.start_x, body.start_y, body.end_x, body.end_y,
            body.steps, body.duration_ms, body.jitter, body.seed,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "mouse-move-random", "durationMs": ms, "result": {"points": len(data)}}

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/keyboard/press", tags=["keyboard"])
    async def api_press(session_id: str, page_id: str, body: PressRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        _, ms = await asyncio.to_thread(
            _timed, actions.press_key,
            session, page_id, body.key,
        )
        return {"success": True, "sessionId": session_id, "pageId": page_id, "action": "press", "durationMs": ms}

    # =======================================================================
    # Action chains
    # =======================================================================

    @app.post("/api/v1/session/{session_id}/pages/{page_id}/actions/run", tags=["actions"])
    async def api_action_chain(session_id: str, page_id: str, body: ActionChainRequest) -> dict[str, Any]:
        session = manager.get_session(session_id)
        results, ms = await asyncio.to_thread(
            _timed, actions.run_action_chain,
            session, page_id, body.steps, body.timeout,
        )
        all_ok = all(r["success"] for r in results)
        return {
            "success": all_ok,
            "sessionId": session_id,
            "pageId": page_id,
            "action": "chain",
            "durationMs": ms,
            "results": results,
        }

    return app
