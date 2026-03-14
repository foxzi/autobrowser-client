from __future__ import annotations

import logging
import re
import threading
import time
import uuid
from enum import StrEnum
from pathlib import Path
from typing import Any

from patchright.sync_api import sync_playwright, Playwright, Browser, BrowserContext, Page, Request, Response

from autobrowser_client.config import Settings
from autobrowser_client.errors import BrowserStartError, SessionNotFoundError
from autobrowser_client.profiles.manager import ProfileLock
from autobrowser_client.proxy.relay import ProxyRelay

logger = logging.getLogger(__name__)


def _detect_platform_from_ua(user_agent: str) -> dict[str, Any] | None:
    """Detect OS from User-Agent string and return CDP userAgentMetadata params.

    Returns None if the UA matches the real OS (Linux) — no spoofing needed.
    """
    ua = user_agent.lower()

    # Extract Chrome version from UA
    chrome_match = re.search(r"chrome/(\d+\.\d+\.\d+\.\d+)", ua)
    chrome_ver = chrome_match.group(1) if chrome_match else "130.0.0.0"
    chrome_major = chrome_ver.split(".")[0]

    brands = [
        {"brand": "Chromium", "version": chrome_ver},
        {"brand": "Google Chrome", "version": chrome_ver},
        {"brand": "Not/A)Brand", "version": "24.0.0.0"},
    ]

    if "windows nt" in ua:
        # Extract Windows version (e.g. "Windows NT 10.0")
        nt_match = re.search(r"windows nt (\d+\.\d+)", ua)
        nt_ver = nt_match.group(1) if nt_match else "10.0"
        return {
            "platform": "Win32",
            "mobile": False,
            "userAgentMetadata": {
                "platform": "Windows",
                "platformVersion": f"{nt_ver}.0",
                "architecture": "x86",
                "model": "",
                "mobile": False,
                "fullVersionList": brands,
                "fullVersion": chrome_ver,
                "bitness": "64",
                "wow64": False,
            },
        }

    if "macintosh" in ua or "mac os x" in ua:
        # Extract macOS version (e.g. "Mac OS X 10_15_7")
        mac_match = re.search(r"mac os x (\d+[_\.]\d+[_\.]\d+)", ua)
        mac_ver = mac_match.group(1).replace("_", ".") if mac_match else "14.0.0"
        return {
            "platform": "MacIntel",
            "mobile": False,
            "userAgentMetadata": {
                "platform": "macOS",
                "platformVersion": mac_ver,
                "architecture": "arm",
                "model": "",
                "mobile": False,
                "fullVersionList": brands,
                "fullVersion": chrome_ver,
                "bitness": "64",
                "wow64": False,
            },
        }

    if "android" in ua:
        # Extract Android version (e.g. "Android 14" or "Android 13")
        android_match = re.search(r"android (\d+(?:\.\d+)*)", ua)
        android_ver = android_match.group(1) if android_match else "14"
        # Extract device model (e.g. "SM-S918B" from "SM-S918B Build/")
        model_match = re.search(r";\s*([^;)]+?)\s*(?:Build|MIUI|\))", user_agent)
        model = model_match.group(1).strip() if model_match else ""
        return {
            "platform": "Linux armv8l",
            "mobile": True,
            "userAgentMetadata": {
                "platform": "Android",
                "platformVersion": android_ver,
                "architecture": "arm",
                "model": model,
                "mobile": True,
                "fullVersionList": brands,
                "fullVersion": chrome_ver,
                "bitness": "64",
                "wow64": False,
            },
        }

    # Linux UA on Linux host — no spoofing needed
    return None


def _apply_platform_spoof(context: BrowserContext, page: Page, user_agent: str) -> None:
    """Apply CDP platform override if UA indicates a different OS."""
    platform_data = _detect_platform_from_ua(user_agent)
    if platform_data is None:
        return

    try:
        cdp = context.new_cdp_session(page)
        cdp.send("Emulation.setUserAgentOverride", {
            "userAgent": user_agent,
            "platform": platform_data["platform"],
            "userAgentMetadata": platform_data["userAgentMetadata"],
        })
        logger.debug("Applied platform spoof: %s", platform_data["platform"])
    except Exception as exc:
        logger.warning("Failed to apply platform spoof: %s", exc)


class SessionState(StrEnum):
    STARTING = "starting"
    RUNNING = "running"
    STOPPING = "stopping"
    STOPPED = "stopped"
    FAILED = "failed"


_SENSITIVE_HEADERS = {"authorization", "cookie", "set-cookie", "proxy-authorization"}


def _mask_headers(headers: dict[str, str]) -> dict[str, str]:
    masked: dict[str, str] = {}
    for k, v in headers.items():
        if k.lower() in _SENSITIVE_HEADERS:
            masked[k] = v[:8] + "***" if len(v) > 8 else "****"
        else:
            masked[k] = v
    return masked


class HeadersEntry:
    """Captured request/response headers for a single navigation."""

    def __init__(self, url: str) -> None:
        self.url = url
        self.request_headers: dict[str, str] = {}
        self.response_headers: dict[str, str] = {}
        self.status_code: int | None = None

    def to_dict(self, mask: bool = True) -> dict[str, Any]:
        req = _mask_headers(self.request_headers) if mask else self.request_headers
        resp = _mask_headers(self.response_headers) if mask else self.response_headers
        return {
            "url": self.url,
            "request": req,
            "response": resp,
            "statusCode": self.status_code,
        }


class BrowserSession:
    def __init__(self, session_id: str, settings: Settings) -> None:
        self.session_id = session_id
        self.state = SessionState.STARTING
        self.settings = settings
        self.started_at: float = time.time()
        self.stopped_at: float | None = None
        self.last_activity: float = time.time()
        self.error: str | None = None

        self._playwright: Playwright | None = None
        self._browser: Browser | None = None
        self._context: BrowserContext | None = None
        self._pages: dict[str, Page] = {}
        self._headers: dict[str, list[HeadersEntry]] = {}  # page_id -> headers list
        self._proxy_relay: ProxyRelay | None = None
        self._profile_lock: ProfileLock | None = None

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
        viewport: dict[str, int] | None = None,
    ) -> None:
        try:
            # Start proxy relay if configured
            proxy_server: dict[str, str] | None = None
            if self.settings.proxy.enabled:
                self._proxy_relay = ProxyRelay(self.settings.proxy)
                self._proxy_relay.start()
                proxy_server = {"server": self._proxy_relay.http_proxy_url}
                logger.info("Using proxy relay: %s", self._proxy_relay.http_proxy_url)

            self._playwright = sync_playwright().start()

            is_headless = headless if headless is not None else self.settings.browser.headless
            ua = user_agent or self.settings.browser.user_agent
            vp = viewport or {
                "width": self.settings.browser.viewport.width,
                "height": self.settings.browser.viewport.height,
            }

            launch_args = list(self.settings.browser.launch_args)

            # Detect mobile UA to set is_mobile/has_touch on context
            platform_info = _detect_platform_from_ua(ua) if ua else None
            is_mobile_ua = platform_info["mobile"] if platform_info else False

            if profile:
                profile_dir = Path(self.settings.profiles.dir) / profile
                profile_dir.mkdir(parents=True, exist_ok=True)

                self._profile_lock = ProfileLock(profile_dir)
                self._profile_lock.acquire()

                logger.info("Using profile directory: %s", profile_dir)

                ctx_kwargs: dict[str, Any] = {
                    "channel": "chrome",
                    "headless": is_headless,
                    "args": launch_args or None,
                    "viewport": vp,
                }
                if ua:
                    ctx_kwargs["user_agent"] = ua
                if proxy_server:
                    ctx_kwargs["proxy"] = proxy_server
                if is_mobile_ua:
                    ctx_kwargs["is_mobile"] = True
                    ctx_kwargs["has_touch"] = True

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
                if proxy_server:
                    browser_kwargs["proxy"] = proxy_server
                self._browser = self._playwright.chromium.launch(**browser_kwargs)

                ctx_opts: dict[str, Any] = {"viewport": vp}
                if ua:
                    ctx_opts["user_agent"] = ua
                if is_mobile_ua:
                    ctx_opts["is_mobile"] = True
                    ctx_opts["has_touch"] = True
                self._context = self._browser.new_context(**ctx_opts)

            if urls:
                for url in urls:
                    page = self._context.new_page()
                    page_id = f"page_{len(self._pages)}"
                    self._pages[page_id] = page
                    self._attach_listeners(page_id, page)
                    page.goto(url, timeout=self.settings.timeouts.navigation_ms)
                    logger.info("Opened %s as %s", url, page_id)
            elif self._context.pages:
                self._pages["page_0"] = self._context.pages[0]
                self._attach_listeners("page_0", self._context.pages[0])
            else:
                page = self._context.new_page()
                self._pages["page_0"] = page
                self._attach_listeners("page_0", page)

            # Apply platform spoof if UA indicates a different OS
            if ua:
                for page in self._pages.values():
                    _apply_platform_spoof(self._context, page, ua)

            self.state = SessionState.RUNNING
            logger.info("Session %s is running", self.session_id)

        except Exception as exc:
            self.state = SessionState.FAILED
            self.error = str(exc)
            self.stopped_at = time.time()
            logger.error("Session %s failed to start: %s", self.session_id, exc)
            self._cleanup()
            raise BrowserStartError(str(exc)) from exc

    def touch(self) -> None:
        """Update last activity timestamp."""
        self.last_activity = time.time()

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

    def _attach_listeners(self, page_id: str, page: Page) -> None:
        entries = self._headers.setdefault(page_id, [])

        def on_response(response: Response) -> None:
            request = response.request
            if request.resource_type == "document":
                entry = HeadersEntry(request.url)
                entry.request_headers = dict(request.headers)
                entry.response_headers = dict(response.headers)
                entry.status_code = response.status
                entries.append(entry)

        page.on("response", on_response)

    def get_cookies(self) -> list[dict[str, Any]]:
        return self.context.cookies()

    def get_headers(self, page_id: str | None = None) -> list[dict[str, Any]]:
        pid = page_id or next(iter(self._pages), None)
        if pid is None:
            return []
        return [e.to_dict() for e in self._headers.get(pid, [])]

    def get_content(self, page_id: str | None = None, fmt: str = "html") -> str:
        page = self.get_page(page_id)
        if fmt == "text":
            content = page.inner_text("body")
        else:
            content = page.content()
        max_bytes = self.settings.output.max_content_bytes
        if max_bytes and len(content.encode("utf-8")) > max_bytes:
            content = content.encode("utf-8")[:max_bytes].decode("utf-8", errors="ignore")
        return content

    def info(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "sessionId": self.session_id,
            "state": self.state,
            "startedAt": self.started_at,
            "stoppedAt": self.stopped_at,
            "error": self.error,
            "pages": self.list_pages(),
        }
        if self._proxy_relay and self._proxy_relay.is_running:
            result["proxy"] = self._proxy_relay.http_proxy_url
        return result

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
        try:
            if self._proxy_relay:
                self._proxy_relay.stop()
        except Exception:
            pass
        try:
            if self._profile_lock:
                self._profile_lock.release()
        except Exception:
            pass
        self._context = None
        self._browser = None
        self._playwright = None
        self._proxy_relay = None
        self._profile_lock = None
        self._pages.clear()


class SessionManager:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self._sessions: dict[str, BrowserSession] = {}
        self._reaper_stop = threading.Event()
        self._reaper_thread: threading.Thread | None = None
        idle_ms = self.settings.timeouts.idle_session_ms
        if idle_ms > 0:
            self._reaper_thread = threading.Thread(
                target=self._reap_idle_sessions,
                daemon=True,
            )
            self._reaper_thread.start()

    def _reap_idle_sessions(self) -> None:
        idle_sec = self.settings.timeouts.idle_session_ms / 1000.0
        while not self._reaper_stop.wait(timeout=min(idle_sec / 2, 30)):
            now = time.time()
            for session in list(self._sessions.values()):
                if session.state != SessionState.RUNNING:
                    continue
                if now - session.last_activity > idle_sec:
                    logger.info(
                        "Reaping idle session %s (idle %.0fs)",
                        session.session_id,
                        now - session.last_activity,
                    )
                    try:
                        session.stop()
                    except Exception:
                        pass

    def create_session(
        self,
        urls: list[str] | None = None,
        profile: str | None = None,
        user_agent: str | None = None,
        headless: bool | None = None,
        viewport: dict[str, int] | None = None,
    ) -> BrowserSession:
        max_sessions = self.settings.api.max_sessions
        active = sum(1 for s in self._sessions.values() if s.state == SessionState.RUNNING)
        if active >= max_sessions:
            raise BrowserStartError(f"Maximum active sessions reached: {max_sessions}")

        session_id = f"sess_{uuid.uuid4().hex[:12]}"
        session = BrowserSession(session_id, self.settings)
        self._sessions[session_id] = session
        session.start(urls=urls, profile=profile, user_agent=user_agent, headless=headless, viewport=viewport)
        return session

    def get_session(self, session_id: str) -> BrowserSession:
        session = self._sessions.get(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)
        session.touch()
        return session

    def stop_session(self, session_id: str) -> BrowserSession:
        session = self.get_session(session_id)
        session.stop()
        return session

    def list_sessions(self) -> list[dict[str, Any]]:
        return [s.info() for s in self._sessions.values()]

    def stop_all(self) -> None:
        self._reaper_stop.set()
        if self._reaper_thread:
            self._reaper_thread.join(timeout=5)
        for session in self._sessions.values():
            try:
                session.stop()
            except Exception:
                pass
