"""Scenario step executor for BrowserSession."""
from __future__ import annotations

import json
import logging
import random
import time
from pathlib import Path
from typing import Any

from autobrowser_client.browser import actions
from autobrowser_client.browser.actions import resolve_locator
from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.errors import AppError, ErrorCode
from autobrowser_client.scenario import Locator, ScriptDefinition, ScriptStep

logger = logging.getLogger(__name__)


def _validate_output_path(file_path: str) -> Path:
    """Validate that a file path is safe (no path traversal, no absolute paths)."""
    p = Path(file_path)
    if p.is_absolute():
        raise AppError(ErrorCode.INVALID_ARGUMENT, f"Absolute paths are not allowed: {file_path}")
    try:
        resolved = p.resolve()
        cwd = Path.cwd().resolve()
        if not resolved.is_relative_to(cwd):
            raise AppError(ErrorCode.INVALID_ARGUMENT, f"Path traversal is not allowed: {file_path}")
    except (OSError, ValueError) as exc:
        raise AppError(ErrorCode.INVALID_ARGUMENT, f"Invalid file path: {file_path}") from exc
    return p


class StepResult:
    def __init__(self, action: str, success: bool) -> None:
        self.action = action
        self.success = success
        self.duration_ms: float = 0
        self.data: Any = None
        self.error: str | None = None

    def to_dict(self) -> dict[str, Any]:
        d: dict[str, Any] = {
            "action": self.action,
            "success": self.success,
            "durationMs": round(self.duration_ms, 1),
        }
        if self.data is not None:
            d["data"] = self.data
        if self.error is not None:
            d["error"] = self.error
        return d


def _resolve_locator(
    session: BrowserSession,
    locator: Locator,
    page_id: str | None = None,
    index: int | None = None,
) -> Any:
    """Convert a Locator model to a Playwright locator object."""
    page = session.get_page(page_id)
    loc = resolve_locator(page, locator.type, locator.value)
    if index is not None:
        loc = loc.nth(index)
    return loc


_CF_CHALLENGE_HOST = "challenges.cloudflare.com"
_CF_CHECK_JS = "() => typeof window._cf_chl_opt !== 'undefined'"
_NAV_STATUS_JS = "() => performance.getEntriesByType('navigation')[0]?.responseStatus"


def _is_cf_challenge(page: Any, state: dict[str, Any]) -> bool:
    """True while the page shows a Cloudflare challenge (or is navigating)."""
    if state["mitigated"]:
        return True
    try:
        # Page globals are only visible outside patchright's isolated world
        if page.evaluate(_CF_CHECK_JS, isolated_context=False):
            return True
    except Exception:
        return True  # context destroyed by navigation: check again later
    return any(_CF_CHALLENGE_HOST in f.url for f in page.frames)


def _click_turnstile(page: Any) -> bool:
    """Click the Turnstile checkbox if its iframe is visible."""
    for frame in page.frames:
        if _CF_CHALLENGE_HOST not in frame.url:
            continue
        try:
            box = frame.frame_element().bounding_box()
        except Exception:
            continue
        if box and box["width"] > 0:
            page.mouse.click(box["x"] + 30, box["y"] + box["height"] / 2)
            return True
    return False


def solve_cf_challenge(page: Any, timeout_ms: int) -> dict[str, Any]:
    """Wait for a Cloudflare challenge to pass, clicking Turnstile when shown.

    Returns once the page has been challenge-free for 3 consecutive seconds.
    Fails if the page after the challenge answers with HTTP >= 400.
    """
    # Cloudflare marks challenge responses with "cf-mitigated: challenge"
    state: dict[str, Any] = {"mitigated": False, "status": None}
    try:
        state["status"] = page.evaluate(_NAV_STATUS_JS) or None
    except Exception:
        pass

    def on_response(resp: Any) -> None:
        if resp.request.is_navigation_request() and resp.frame == page.main_frame:
            state["mitigated"] = resp.headers.get("cf-mitigated") == "challenge"
            state["status"] = resp.status

    page.on("response", on_response)
    try:
        return _wait_cf_challenge(page, timeout_ms, state)
    finally:
        page.remove_listener("response", on_response)


def _wait_cf_challenge(page: Any, timeout_ms: int, state: dict[str, Any]) -> dict[str, Any]:
    t0 = time.monotonic()
    deadline = t0 + timeout_ms / 1000
    last_click = 0.0
    clicks = 0
    seen = False
    ok_streak = 0
    while time.monotonic() < deadline:
        if _is_cf_challenge(page, state):
            seen = True
            ok_streak = 0
            now = time.monotonic()
            # Give the non-interactive check a few seconds before clicking
            if now - t0 > 4 and now - last_click > 8 and _click_turnstile(page):
                last_click = now
                clicks += 1
                logger.info("Clicked Cloudflare Turnstile checkbox (%d)", clicks)
        else:
            ok_streak += 1
            if ok_streak >= 3:
                status = state["status"]
                if status is not None and status >= 400:
                    reason = "after challenge" if seen else "without challenge"
                    raise AppError(
                        ErrorCode.INTERNAL_ERROR,
                        f"Page blocked {reason}: HTTP {status} at {page.url}",
                    )
                return {"challenge": seen, "clicks": clicks, "url": page.url}
        page.wait_for_timeout(1000)
    raise AppError(
        ErrorCode.TIMEOUT,
        f"Cloudflare challenge not passed in {timeout_ms} ms (clicks: {clicks})",
    )


def execute_step(session: BrowserSession, step: ScriptStep, page_id: str | None = None) -> StepResult:
    """Execute a single script step against a browser session."""
    result = StepResult(step.action, success=True)
    t0 = time.monotonic()

    try:
        effective_page_id = step.page or page_id
        page = session.get_page(effective_page_id)
        timeout = step.timeout or session.settings.timeouts.action_ms

        if step.action == "open":
            if not step.url:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "open action requires 'url'")
            page.goto(step.url, timeout=session.settings.timeouts.navigation_ms)
            result.data = {"url": page.url}

        elif step.action == "wait":
            if step.wait_until == "networkidle":
                page.wait_for_load_state("networkidle", timeout=timeout)
            elif step.wait_until == "load":
                page.wait_for_load_state("load", timeout=timeout)
            elif step.wait_until == "domcontentloaded":
                page.wait_for_load_state("domcontentloaded", timeout=timeout)
            elif step.wait_until in {"visible", "hidden"}:
                loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
                loc.wait_for(state=step.wait_until, timeout=timeout)
            elif step.timeout:
                page.wait_for_timeout(step.timeout)
            else:
                page.wait_for_load_state("load", timeout=timeout)

        elif step.action == "find":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "find action requires 'locator'")
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            loc.wait_for(state="attached", timeout=timeout)
            count = loc.count()
            result.data = {"found": count}

        elif step.action == "click":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "click action requires 'locator'")
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            loc.click(timeout=timeout)

        elif step.action == "fill":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "fill action requires 'locator'")
            if step.value is None:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "fill action requires 'value'")
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            loc.fill(step.value, timeout=timeout)

        elif step.action == "type":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "type action requires 'locator'")
            if step.value is None:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "type action requires 'value'")
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            loc.click(timeout=timeout)
            delay = 50 if step.delay is None else step.delay
            for ch in step.value:
                page.keyboard.type(ch)
                page.wait_for_timeout(random.uniform(delay * 0.5, delay * 1.5))

        elif step.action == "press":
            if not step.key:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "press action requires 'key'")
            if step.locator:
                loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
                loc.press(step.key, timeout=timeout)
            else:
                page.keyboard.press(step.key)

        elif step.action == "scroll":
            dx = step.delta_x or 0
            dy = step.delta_y or 0
            page.mouse.wheel(dx, dy)

        elif step.action == "screenshot":
            if not step.file:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "screenshot action requires 'file'")
            out = _validate_output_path(step.file)
            out.parent.mkdir(parents=True, exist_ok=True)
            page.screenshot(path=str(out), full_page=step.full_page)
            size = out.stat().st_size
            result.data = {"file": step.file, "fullPage": step.full_page, "bytes": size}

        elif step.action == "get-content":
            fmt = step.format or "html"
            content = session.get_content(effective_page_id, fmt=fmt)
            result.data = {"format": fmt, "length": len(content), "content": content}

        elif step.action == "get-cookies":
            cookies = session.get_cookies()
            result.data = {"cookies": cookies}

        elif step.action == "get-headers":
            headers = session.get_headers(effective_page_id)
            result.data = {"headers": headers}

        elif step.action == "save":
            if not step.file:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "save action requires 'file'")
            out = _validate_output_path(step.file)
            if step.value is not None:
                content = step.value
            else:
                content = session.get_content(effective_page_id, fmt=step.format or "html")
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_text(content, encoding="utf-8")
            result.data = {"file": step.file, "bytes": len(content.encode("utf-8"))}

        elif step.action == "solve-challenge":
            result.data = solve_cf_challenge(page, step.timeout or 60000)

        elif step.action == "browse-tabs":
            pages = session.list_pages()
            scroll_dy = step.delta_y or 400
            visited = []
            for pid, url in pages.items():
                if url in {"about:blank", "chrome://newtab/"}:
                    continue
                try:
                    p = session.get_page(pid)
                    p.bring_to_front()
                    p.wait_for_load_state("load", timeout=timeout)
                    # Scroll down a few times
                    for _ in range(random.randint(2, 5)):
                        p.mouse.wheel(0, scroll_dy)
                        p.wait_for_timeout(random.randint(500, 2000))
                    visited.append({"page": pid, "url": url})
                except Exception as exc:
                    logger.warning("browse-tabs: failed on %s (%s): %s", pid, url, exc)
                    visited.append({"page": pid, "url": url, "error": str(exc)})
            result.data = {"visited": visited, "count": len(visited)}

        elif step.action in {"hover", "dblclick", "focus", "clear", "check", "uncheck"}:
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            if step.action == "dblclick":
                loc.dblclick(timeout=timeout)
            elif step.action == "check":
                loc.check(timeout=timeout)
            elif step.action == "uncheck":
                loc.uncheck(timeout=timeout)
            else:
                getattr(loc, step.action)(timeout=timeout)

        elif step.action == "submit":
            actions.submit_form(session, effective_page_id, step.locator.type, step.locator.value, timeout)

        elif step.action == "select":
            option = step.options if step.options else step.value
            selected = actions.select_option(
                session, effective_page_id, step.locator.type, step.locator.value, option, timeout
            )
            result.data = {"selected": selected}

        elif step.action == "upload":
            result.data = actions.upload_files(
                session, effective_page_id, step.locator.type, step.locator.value, step.files, timeout
            )

        elif step.action == "evaluate":
            result.data = actions.evaluate(session, effective_page_id, step.script)

        elif step.action == "history":
            nav = actions.history(
                session, effective_page_id, step.direction,
                step.timeout or session.settings.timeouts.navigation_ms,
            )
            result.data = {"url": nav["url"]}

        elif step.action == "wait-text":
            actions.wait_for_text(session, effective_page_id, step.text, step.gone, timeout)

        elif step.action == "extract":
            loc = _resolve_locator(session, step.locator, effective_page_id, step.index)
            loc.first.wait_for(state="attached", timeout=timeout)
            if step.all_:
                if step.attribute:
                    value = loc.evaluate_all("(els, a) => els.map(e => e.getAttribute(a))", step.attribute)
                else:
                    value = loc.evaluate_all("els => els.map(e => e.innerText)")
            elif step.attribute:
                value = loc.first.get_attribute(step.attribute, timeout=timeout)
            else:
                value = loc.first.inner_text(timeout=timeout)
            result.data = {"value": value}

        elif step.action == "find-all":
            result.data = actions.find_all_elements(
                session, effective_page_id, step.locator.type, step.locator.value, timeout
            )

        elif step.action == "read":
            result.data = actions.read_element(
                session, effective_page_id, step.locator.type, step.locator.value, timeout
            )

        elif step.action == "mouse-move":
            actions.mouse_move(session, effective_page_id, step.x, step.y, step.move_steps or 1)

        elif step.action == "mouse-scroll-to":
            actions.mouse_scroll_to_element(
                session, effective_page_id, step.locator.type, step.locator.value, timeout
            )

        elif step.action == "mouse-random":
            points = actions.mouse_move_random(
                session, effective_page_id,
                step.start_x, step.start_y, step.end_x, step.end_y,
                steps=step.move_steps or 25,
                duration_ms=300 if step.duration_ms is None else step.duration_ms,
                jitter=3.0 if step.jitter is None else step.jitter,
                seed=step.seed,
            )
            result.data = {"points": len(points)}

        else:
            result.success = False
            result.error = f"Unknown action: {step.action}"

    except Exception as exc:
        result.success = False
        # patchright prefixes messages with an empty API name: ": Timeout ..."
        result.error = str(exc).removeprefix(": ")
        logger.warning("Step '%s' failed: %s", step.action, exc)

    result.duration_ms = (time.monotonic() - t0) * 1000
    return result


def execute_script(
    session: BrowserSession,
    steps: list[ScriptStep],
    stop_on_error: bool = True,
    page_id: str | None = None,
) -> list[dict[str, Any]]:
    """Execute a list of script steps, returning results (see engine.run_script)."""
    from autobrowser_client.browser.engine import run_script

    script = ScriptDefinition(steps=steps, stopOnError=stop_on_error)
    return run_script(session, script, page_id)["steps"]
