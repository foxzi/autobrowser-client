"""Scenario step executor for BrowserSession."""
from __future__ import annotations

import json
import logging
import time
from pathlib import Path
from typing import Any

from autobrowser_client.browser.actions import resolve_locator
from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.errors import AppError, ErrorCode
from autobrowser_client.scenario import ScriptStep, Locator

logger = logging.getLogger(__name__)


def _validate_output_path(file_path: str) -> Path:
    """Validate that a file path is safe (no path traversal, no absolute paths)."""
    p = Path(file_path)
    if p.is_absolute():
        raise AppError(ErrorCode.INVALID_ARGUMENT, f"Absolute paths are not allowed: {file_path}")
    try:
        resolved = p.resolve()
        cwd = Path.cwd().resolve()
        if not str(resolved).startswith(str(cwd)):
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


def execute_step(session: BrowserSession, step: ScriptStep, page_id: str | None = None) -> StepResult:
    """Execute a single script step against a browser session."""
    result = StepResult(step.action, success=True)
    t0 = time.monotonic()

    try:
        page = session.get_page(page_id)
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
                if step.locator:
                    loc = _resolve_locator(session, step.locator, page_id, step.index)
                    if step.wait_until == "visible":
                        loc.wait_for(state="visible", timeout=timeout)
                    else:
                        loc.wait_for(state="hidden", timeout=timeout)
                else:
                    page.wait_for_timeout(timeout)
            elif step.timeout:
                page.wait_for_timeout(step.timeout)
            else:
                page.wait_for_load_state("load", timeout=timeout)

        elif step.action == "find":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "find action requires 'locator'")
            loc = _resolve_locator(session, step.locator, page_id, step.index)
            loc.wait_for(state="attached", timeout=timeout)
            count = loc.count()
            result.data = {"found": count}

        elif step.action == "click":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "click action requires 'locator'")
            loc = _resolve_locator(session, step.locator, page_id, step.index)
            loc.click(timeout=timeout)

        elif step.action == "fill":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "fill action requires 'locator'")
            if step.value is None:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "fill action requires 'value'")
            loc = _resolve_locator(session, step.locator, page_id, step.index)
            loc.fill(step.value, timeout=timeout)

        elif step.action == "type":
            if not step.locator:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "type action requires 'locator'")
            if step.value is None:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "type action requires 'value'")
            loc = _resolve_locator(session, step.locator, page_id, step.index)
            loc.click(timeout=timeout)
            delay = step.delay or 50
            page.keyboard.type(step.value, delay=delay)

        elif step.action == "press":
            if not step.key:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "press action requires 'key'")
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
            content = session.get_content(page_id, fmt=fmt)
            result.data = {"format": fmt, "length": len(content), "content": content}

        elif step.action == "get-cookies":
            cookies = session.get_cookies()
            result.data = {"cookies": cookies}

        elif step.action == "get-headers":
            headers = session.get_headers(page_id)
            result.data = {"headers": headers}

        elif step.action == "save":
            if not step.file:
                raise AppError(ErrorCode.INVALID_ARGUMENT, "save action requires 'file'")
            out = _validate_output_path(step.file)
            fmt = step.format or "html"
            content = session.get_content(page_id, fmt=fmt)
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_text(content, encoding="utf-8")
            result.data = {"file": step.file, "bytes": len(content.encode("utf-8"))}

        else:
            result.success = False
            result.error = f"Unknown action: {step.action}"

    except Exception as exc:
        result.success = False
        result.error = str(exc)
        logger.warning("Step '%s' failed: %s", step.action, exc)

    result.duration_ms = (time.monotonic() - t0) * 1000
    return result


def execute_script(
    session: BrowserSession,
    steps: list[ScriptStep],
    stop_on_error: bool = True,
    page_id: str | None = None,
) -> list[dict[str, Any]]:
    """Execute a full list of script steps, returning results."""
    results: list[dict[str, Any]] = []
    for i, step in enumerate(steps):
        logger.info("Executing step %d/%d: %s", i + 1, len(steps), step.action)
        step_result = execute_step(session, step, page_id)
        results.append(step_result.to_dict())

        if not step_result.success:
            should_stop = stop_on_error
            if step.stop_on_error is not None:
                should_stop = step.stop_on_error
            if should_stop:
                logger.error("Stopping script at step %d due to error: %s", i + 1, step_result.error)
                break

    return results
