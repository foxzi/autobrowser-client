from __future__ import annotations

import logging
import time
import uuid
from enum import StrEnum
from pathlib import Path
from typing import Any

from patchright.sync_api import sync_playwright, Playwright, Browser, BrowserContext, Page

from autobrowser_client.config import Settings
from autobrowser_client.errors import BrowserStartError, SessionNotFoundError

logger = logging.getLogger(__name__)


class SessionState(StrEnum):
    STARTING = "starting"
    RUNNING = "running"
    STOPPING = "stopping"
    STOPPED = "stopped"
    FAILED = "failed"


class BrowserSession:
    def __init__(self, session_id: str, settings: Settings) -> None:
        self.session_id = session_id
        self.state = SessionState.STARTING
        self.settings = settings
        self.started_at: float = time.time()
        self.stopped_at: float | None = None
        self.error: str | None = None

        self._playwright: Playwright | None = None
        self._browser: Browser | None = None
        self._context: BrowserContext | None = None
        self._pages: dict[str, Page] = {}

    @property
    def context(self) -> BrowserContext:
        if self._context is None:
            raise BrowserStartError("Browser context is not initialized")
        return self._context

    def start(
        self,
        urls: list[str] | None = None,
        profile: str | None = None,
        user_agent: str | None = None,
        headless: bool | None = None,
    ) -> None:
        try:
            self._playwright = sync_playwright().start()

            is_headless = headless if headless is not None else self.settings.browser.headless
            ua = user_agent or self.settings.browser.user_agent

            launch_args = list(self.settings.browser.launch_args)

            if profile:
                profile_dir = Path(self.settings.profiles.dir) / profile
                profile_dir.mkdir(parents=True, exist_ok=True)
                logger.info("Using profile directory: %s", profile_dir)

                ctx_kwargs: dict[str, Any] = {
                    "channel": "chrome",
                    "headless": is_headless,
                    "args": launch_args or None,
                }
                if ua:
                    ctx_kwargs["user_agent"] = ua

                self._context = self._playwright.chromium.launch_persistent_context(
                    user_data_dir=str(profile_dir),
                    **ctx_kwargs,
                )
            else:
                browser_kwargs: dict[str, Any] = {
                    "channel": "chrome",
                    "headless": is_headless,
                    "args": launch_args or None,
                }
                self._browser = self._playwright.chromium.launch(**browser_kwargs)

                ctx_opts: dict[str, Any] = {}
                if ua:
                    ctx_opts["user_agent"] = ua
                self._context = self._browser.new_context(**ctx_opts)

            if urls:
                for url in urls:
                    page = self._context.new_page()
                    page.goto(url, timeout=self.settings.timeouts.navigation_ms)
                    page_id = f"page_{len(self._pages)}"
                    self._pages[page_id] = page
                    logger.info("Opened %s as %s", url, page_id)
            elif self._context.pages:
                self._pages["page_0"] = self._context.pages[0]

            self.state = SessionState.RUNNING
            logger.info("Session %s is running", self.session_id)

        except Exception as exc:
            self.state = SessionState.FAILED
            self.error = str(exc)
            self.stopped_at = time.time()
            logger.error("Session %s failed to start: %s", self.session_id, exc)
            self._cleanup()
            raise BrowserStartError(str(exc)) from exc

    def stop(self) -> None:
        if self.state in {SessionState.STOPPED, SessionState.STOPPING}:
            return
        self.state = SessionState.STOPPING
        logger.info("Stopping session %s", self.session_id)
        self._cleanup()
        self.state = SessionState.STOPPED
        self.stopped_at = time.time()
        logger.info("Session %s stopped", self.session_id)

    def get_page(self, page_id: str | None = None) -> Page:
        if not self._pages:
            raise BrowserStartError("No pages available in session")
        if page_id is None:
            return next(iter(self._pages.values()))
        if page_id not in self._pages:
            raise BrowserStartError(f"Page not found: {page_id}")
        return self._pages[page_id]

    def list_pages(self) -> dict[str, str]:
        return {pid: page.url for pid, page in self._pages.items()}

    def get_cookies(self) -> list[dict[str, Any]]:
        return self.context.cookies()

    def get_content(self, page_id: str | None = None, fmt: str = "html") -> str:
        page = self.get_page(page_id)
        if fmt == "text":
            return page.inner_text("body")
        return page.content()

    def info(self) -> dict[str, Any]:
        return {
            "sessionId": self.session_id,
            "state": self.state,
            "startedAt": self.started_at,
            "stoppedAt": self.stopped_at,
            "error": self.error,
            "pages": self.list_pages(),
        }

    def _cleanup(self) -> None:
        try:
            if self._context:
                self._context.close()
        except Exception:
            pass
        try:
            if self._browser:
                self._browser.close()
        except Exception:
            pass
        try:
            if self._playwright:
                self._playwright.stop()
        except Exception:
            pass
        self._context = None
        self._browser = None
        self._playwright = None
        self._pages.clear()


class SessionManager:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self._sessions: dict[str, BrowserSession] = {}

    def create_session(
        self,
        urls: list[str] | None = None,
        profile: str | None = None,
        user_agent: str | None = None,
        headless: bool | None = None,
    ) -> BrowserSession:
        max_sessions = self.settings.api.max_sessions
        active = sum(1 for s in self._sessions.values() if s.state == SessionState.RUNNING)
        if active >= max_sessions:
            raise BrowserStartError(f"Maximum active sessions reached: {max_sessions}")

        session_id = f"sess_{uuid.uuid4().hex[:12]}"
        session = BrowserSession(session_id, self.settings)
        self._sessions[session_id] = session
        session.start(urls=urls, profile=profile, user_agent=user_agent, headless=headless)
        return session

    def get_session(self, session_id: str) -> BrowserSession:
        session = self._sessions.get(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)
        return session

    def stop_session(self, session_id: str) -> BrowserSession:
        session = self.get_session(session_id)
        session.stop()
        return session

    def list_sessions(self) -> list[dict[str, Any]]:
        return [s.info() for s in self._sessions.values()]

    def stop_all(self) -> None:
        for session in self._sessions.values():
            try:
                session.stop()
            except Exception:
                pass
