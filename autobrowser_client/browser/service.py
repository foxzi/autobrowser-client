from __future__ import annotations

import json
import logging
import re
import threading
import time
import uuid
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from enum import StrEnum
from pathlib import Path
from typing import Any

TABS_FILE = "tabs.json"

from patchright.sync_api import sync_playwright, Playwright, Browser, BrowserContext, Page, Request, Response

from autobrowser_client.config import Settings
from autobrowser_client.errors import BrowserStartError, PageNotFoundError, SessionNotFoundError
from autobrowser_client.fingerprint.config import (
    FingerprintConfig, Platform, load_fingerprint, merge_fingerprint,
)
from autobrowser_client.fingerprint.inject import build_injection_script
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
        self._profile_dir: Path | None = None
        self._next_page = 0
        # Per-page event logs for agents: console, network, dialogs
        self._logs: dict[str, dict[str, deque[dict[str, Any]]]] = {}
        # How to answer JS dialogs (alert/confirm/prompt); applied immediately
        self.dialog_policy: dict[str, Any] = {"accept": False, "promptText": None}
        # Playwright sync objects are bound to the thread that created them,
        # so every browser call of this session runs on one worker thread.
        self._executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix=session_id)
        self._thread_id: int | None = None

    def call(self, fn: Any, *args: Any, **kwargs: Any) -> Any:
        """Run fn on the session thread (inline when already on it)."""
        if threading.get_ident() == self._thread_id:
            return fn(*args, **kwargs)
        return self._executor.submit(self._run_owned, fn, args, kwargs).result()

    def _run_owned(self, fn: Any, args: tuple, kwargs: dict) -> Any:
        self._thread_id = threading.get_ident()
        return fn(*args, **kwargs)

    def _headful_user_agent(self) -> str | None:
        """Real Chrome UA without the "HeadlessChrome" token Cloudflare blocks."""
        try:
            browser = self._playwright.chromium.launch(channel="chrome", headless=True)
            try:
                ua = browser.new_page().evaluate("() => navigator.userAgent")
            finally:
                browser.close()
        except Exception as exc:
            logger.warning("Failed to detect browser user agent: %s", exc)
            return None
        return ua.replace("HeadlessChrome/", "Chrome/")

    @property
    def context(self) -> BrowserContext:
        if self._context is None:
            raise BrowserStartError("Browser context is not initialized")
        return self._context

    def start(self, **kwargs: Any) -> None:
        try:
            self.call(self._start, **kwargs)
        except Exception:
            self._executor.shutdown(wait=False)
            raise

    def _start(
        self,
        urls: list[str] | None = None,
        profile: str | None = None,
        user_agent: str | None = None,
        headless: bool | None = None,
        viewport: dict[str, int] | None = None,
        preserve_session: bool = False,
        fingerprint: FingerprintConfig | bool | dict[str, Any] | None = None,
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

            if not ua and is_headless and self.settings.browser.engine == "patchright":
                ua = self._headful_user_agent()

            # Detect mobile UA to set is_mobile/has_touch on context
            platform_info = _detect_platform_from_ua(ua) if ua else None
            is_mobile_ua = platform_info["mobile"] if platform_info else False

            if self.settings.browser.engine == "cloakbrowser":
                try:
                    from cloakbrowser.download import ensure_binary as _cb_ensure_binary
                    from cloakbrowser.browser import build_args as _cb_build_args
                    from cloakbrowser.config import IGNORE_DEFAULT_ARGS as _CB_IGNORE_DEFAULT_ARGS
                except ImportError as exc:
                    raise BrowserStartError(
                        "cloakbrowser engine requested but 'cloakbrowser' package is not installed. "
                        "Install with: pip install cloakbrowser"
                    ) from exc

                binary_path = _cb_ensure_binary()
                fp_platform = self.settings.browser.fingerprint_platform
                if fp_platform:
                    launch_args.append(f"--fingerprint-platform={fp_platform}")
                chrome_args = _cb_build_args(
                    stealth_args=True,
                    extra_args=launch_args,
                    headless=is_headless,
                )

                if profile:
                    profile_dir = Path(self.settings.profiles.dir) / profile
                    profile_dir.mkdir(parents=True, exist_ok=True)
                    self._profile_dir = profile_dir

                    self._profile_lock = ProfileLock(profile_dir)
                    self._profile_lock.acquire()

                    if "--restore-last-session" not in launch_args:
                        launch_args.append("--restore-last-session")

                    logger.info("Using profile directory: %s", profile_dir)

                    ctx_kwargs: dict[str, Any] = {
                        "executable_path": binary_path,
                        "headless": is_headless,
                        "args": chrome_args,
                        "ignore_default_args": _CB_IGNORE_DEFAULT_ARGS,
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
                    launch_kwargs: dict[str, Any] = {
                        "executable_path": binary_path,
                        "headless": is_headless,
                        "args": chrome_args,
                        "ignore_default_args": _CB_IGNORE_DEFAULT_ARGS,
                    }
                    if proxy_server:
                        launch_kwargs["proxy"] = proxy_server
                    self._browser = self._playwright.chromium.launch(**launch_kwargs)

                    ctx_opts: dict[str, Any] = {"viewport": vp}
                    if ua:
                        ctx_opts["user_agent"] = ua
                    if is_mobile_ua:
                        ctx_opts["is_mobile"] = True
                        ctx_opts["has_touch"] = True
                    self._context = self._browser.new_context(**ctx_opts)
            else:
                if profile:
                    profile_dir = Path(self.settings.profiles.dir) / profile
                    profile_dir.mkdir(parents=True, exist_ok=True)
                    self._profile_dir = profile_dir

                    self._profile_lock = ProfileLock(profile_dir)
                    self._profile_lock.acquire()

                    # Help Chromium restore tabs from previous session
                    if "--restore-last-session" not in launch_args:
                        launch_args.append("--restore-last-session")

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

            # Determine platform for fingerprint GPU pool selection
            fp_platform: Platform = "windows"
            if is_mobile_ua:
                fp_platform = "android"
            elif platform_info and platform_info.get("platform") == "Linux x86_64":
                fp_platform = "linux"
            elif ua and "Linux" in ua and "Android" not in ua:
                fp_platform = "linux"

            # Apply fingerprint spoofing
            fp_config = self._resolve_fingerprint(fingerprint, fp_platform)
            if fp_config:
                script = build_injection_script(fp_config)
                self._context.add_init_script(script)
                logger.info("Fingerprint spoofing enabled (seed=%d)", fp_config.seed)

            # Determine URLs to open: explicit > saved tabs (if preserveSession) > blank
            effective_urls = list(urls) if urls else []
            if not effective_urls and preserve_session and self._profile_dir:
                effective_urls = self._load_saved_tabs()

            if effective_urls:
                for url in effective_urls:
                    page = self._context.new_page()
                    page_id = f"page_{len(self._pages)}"
                    self._pages[page_id] = page
                    self._attach_listeners(page_id, page)
                    try:
                        page.goto(url, timeout=self.settings.timeouts.navigation_ms)
                    except Exception:
                        logger.warning("Failed to restore tab %s: %s", page_id, url)
                    logger.info("Opened %s as %s", url, page_id)
            elif self._context.pages:
                # Register all existing pages (e.g. restored by Chromium)
                for i, existing_page in enumerate(self._context.pages):
                    page_id = f"page_{i}"
                    self._pages[page_id] = existing_page
                    self._attach_listeners(page_id, existing_page)
                logger.info("Registered %d existing page(s)", len(self._context.pages))
            else:
                page = self._context.new_page()
                self._pages["page_0"] = page
                self._attach_listeners("page_0", page)

            # Apply platform spoof if UA indicates a different OS
            if ua:
                for page in self._pages.values():
                    _apply_platform_spoof(self._context, page, ua)

                # Auto-apply spoof to any new pages opened later
                def _on_new_page(new_page: Page) -> None:
                    _apply_platform_spoof(self._context, new_page, ua)  # type: ignore[arg-type]

                self._context.on("page", _on_new_page)

            # Track tabs opened later (popups, target=_blank, new_tab)
            self._next_page = len(self._pages)
            self._context.on("page", self._register_page)
            for page_id, page in self._pages.items():
                page.on("close", lambda _p, pid=page_id: self._pages.pop(pid, None))

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

    def stop(self, preserve_session: bool = False) -> None:
        if self.state in {SessionState.STOPPED, SessionState.STOPPING, SessionState.FAILED}:
            return
        try:
            self.call(self._stop, preserve_session)
        finally:
            self._executor.shutdown(wait=False)

    def _stop(self, preserve_session: bool = False) -> None:
        if self.state in {SessionState.STOPPED, SessionState.STOPPING}:
            return
        self.state = SessionState.STOPPING
        logger.info("Stopping session %s", self.session_id)
        if preserve_session:
            self.save_tabs()
        self._cleanup()
        self.state = SessionState.STOPPED
        self.stopped_at = time.time()
        logger.info("Session %s stopped", self.session_id)

    def save_tabs(self) -> None:
        """Save open tab URLs to tabs.json in the profile directory."""
        if not self._profile_dir or not self._pages:
            return
        tab_urls = []
        for page in self._pages.values():
            try:
                url = page.url
                if url and url not in {"about:blank", "chrome://newtab/"}:
                    tab_urls.append(url)
            except Exception:
                pass
        tabs_path = self._profile_dir / TABS_FILE
        if tab_urls:
            tabs_path.write_text(
                json.dumps(tab_urls, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )
            logger.info("Saved %d tab(s) to %s", len(tab_urls), tabs_path)
        elif tabs_path.exists():
            tabs_path.unlink()
            logger.info("Removed empty %s", tabs_path)

    def _load_saved_tabs(self) -> list[str]:
        """Load previously saved tab URLs from tabs.json."""
        if not self._profile_dir:
            return []
        tabs_path = self._profile_dir / TABS_FILE
        if not tabs_path.exists():
            return []
        try:
            data = json.loads(tabs_path.read_text(encoding="utf-8"))
            if isinstance(data, list):
                urls = [u for u in data if isinstance(u, str) and u]
                logger.info("Loaded %d saved tab(s) from %s", len(urls), tabs_path)
                return urls
        except Exception as exc:
            logger.warning("Failed to load %s: %s", tabs_path, exc)
        return []

    def _resolve_fingerprint(
        self,
        fingerprint: FingerprintConfig | bool | dict[str, Any] | None,
        platform: Platform = "windows",
    ) -> FingerprintConfig | None:
        """Resolve fingerprint config from argument + profile file.

        Returns None if fingerprinting should be disabled.
        """
        # Explicitly disabled
        if fingerprint is False:
            return None

        # Already a FingerprintConfig object
        if isinstance(fingerprint, FingerprintConfig):
            return fingerprint

        # Load base from profile if available
        base: FingerprintConfig | None = None
        if self._profile_dir:
            base = load_fingerprint(self._profile_dir, platform=platform)

        # fingerprint=True or None with profile -> use base
        if fingerprint is True or fingerprint is None:
            return base

        # fingerprint is a dict of overrides
        if isinstance(fingerprint, dict):
            if base:
                return merge_fingerprint(base, fingerprint)
            # No profile — need at least a seed to generate
            from autobrowser_client.fingerprint.config import generate_fingerprint
            base = generate_fingerprint(
                seed=fingerprint.get("seed"),
                platform=platform,
            )
            return merge_fingerprint(base, fingerprint)

        return base

    def get_page(self, page_id: str | None = None) -> Page:
        if not self._pages:
            raise BrowserStartError("No pages available in session")
        if page_id is None:
            return next(iter(self._pages.values()))
        if page_id not in self._pages:
            raise PageNotFoundError(page_id)
        return self._pages[page_id]

    def _register_page(self, page: Page) -> str:
        for page_id, known in self._pages.items():
            if known is page:
                return page_id
        page_id = f"page_{self._next_page}"
        self._next_page += 1
        self._pages[page_id] = page
        self._attach_listeners(page_id, page)
        page.on("close", lambda _p: self._pages.pop(page_id, None))
        logger.info("Registered new tab %s", page_id)
        return page_id

    def new_tab(self, url: str | None = None) -> str:
        page_id = self._register_page(self.context.new_page())
        if url:
            self._pages[page_id].goto(url, timeout=self.settings.timeouts.navigation_ms)
        return page_id

    def close_tab(self, page_id: str) -> None:
        self.get_page(page_id).close()
        self._pages.pop(page_id, None)

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

        logs = self._logs.setdefault(page_id, {k: deque(maxlen=500) for k in ("console", "network", "dialogs")})

        def on_console(msg: Any) -> None:
            logs["console"].append({"type": msg.type, "text": msg.text, "time": time.time()})

        def on_net(response: Response) -> None:
            req = response.request
            logs["network"].append({
                "method": req.method, "url": req.url, "resourceType": req.resource_type,
                "status": response.status, "failure": None,
            })

        def on_failed(req: Request) -> None:
            logs["network"].append({
                "method": req.method, "url": req.url, "resourceType": req.resource_type,
                "status": None, "failure": req.failure,
            })

        def on_dialog(dialog: Any) -> None:
            policy = self.dialog_policy
            logs["dialogs"].append({
                "type": dialog.type, "message": dialog.message,
                "accepted": policy["accept"], "time": time.time(),
            })
            if policy["accept"]:
                dialog.accept(policy["promptText"] or "") if dialog.type == "prompt" else dialog.accept()
            else:
                dialog.dismiss()

        page.on("console", on_console)
        page.on("response", on_net)
        page.on("requestfailed", on_failed)
        page.on("dialog", on_dialog)

    def get_log(self, page_id: str, kind: str, clear: bool = False) -> list[dict[str, Any]]:
        self.get_page(page_id)
        log = self._logs.get(page_id, {}).get(kind)
        if log is None:
            return []
        items = list(log)
        if clear:
            log.clear()
        return items

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
        self._lock = threading.Lock()
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
            with self._lock:
                sessions = list(self._sessions.items())
            for session_id, session in sessions:
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
                    with self._lock:
                        self._sessions.pop(session_id, None)

    def create_session(
        self,
        urls: list[str] | None = None,
        profile: str | None = None,
        user_agent: str | None = None,
        headless: bool | None = None,
        viewport: dict[str, int] | None = None,
        preserve_session: bool = False,
        fingerprint: FingerprintConfig | bool | dict[str, Any] | None = None,
    ) -> BrowserSession:
        with self._lock:
            max_sessions = self.settings.api.max_sessions
            active = sum(1 for s in self._sessions.values() if s.state == SessionState.RUNNING)
            if active >= max_sessions:
                raise BrowserStartError(f"Maximum active sessions reached: {max_sessions}")

            session_id = f"sess_{uuid.uuid4().hex[:12]}"
            session = BrowserSession(session_id, self.settings)
            self._sessions[session_id] = session
        session.start(
            urls=urls, profile=profile, user_agent=user_agent,
            headless=headless, viewport=viewport,
            preserve_session=preserve_session,
            fingerprint=fingerprint,
        )
        return session

    def get_session(self, session_id: str) -> BrowserSession:
        with self._lock:
            session = self._sessions.get(session_id)
        if session is None:
            raise SessionNotFoundError(session_id)
        session.touch()
        return session

    def stop_session(self, session_id: str) -> BrowserSession:
        session = self.get_session(session_id)
        session.stop()
        with self._lock:
            self._sessions.pop(session_id, None)
        return session

    def list_sessions(self) -> list[dict[str, Any]]:
        with self._lock:
            sessions = list(self._sessions.values())
        return [s.info() for s in sessions]

    def stop_all(self) -> None:
        self._reaper_stop.set()
        if self._reaper_thread:
            self._reaper_thread.join(timeout=5)
        with self._lock:
            sessions = list(self._sessions.values())
        for session in sessions:
            try:
                session.stop()
            except Exception:
                pass
