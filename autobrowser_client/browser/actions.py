"""Interactive browser actions for API endpoints."""
from __future__ import annotations

import logging
import random
import base64
import time
from typing import Any, Literal

from patchright.sync_api import Page, Locator as PwLocator

from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.scenario import LocatorType

logger = logging.getLogger(__name__)


def resolve_locator(page: Page, loc_type: LocatorType, value: str) -> PwLocator:
    """Convert locator type + value to a Playwright locator object."""
    if loc_type == "css":
        return page.locator(value)
    elif loc_type == "xpath":
        return page.locator(f"xpath={value}")
    elif loc_type == "id":
        return page.locator(f"#{value}")
    elif loc_type == "name":
        return page.locator(f"[name='{value}']")
    elif loc_type == "class":
        return page.locator(f".{value}")
    elif loc_type == "text":
        return page.get_by_text(value)
    elif loc_type == "attribute":
        return page.locator(f"[{value}]")
    elif loc_type == "ref":
        # Ref from the latest snapshot(), e.g. "e12"
        return page.get_by_ref(value)
    return page.locator(value)


# ---------------------------------------------------------------------------
# Page-level operations for agents
# ---------------------------------------------------------------------------

def _truncate(session: BrowserSession, text: str) -> tuple[str, bool]:
    max_bytes = session.settings.output.max_content_bytes
    data = text.encode("utf-8")
    if max_bytes and len(data) > max_bytes:
        return data[:max_bytes].decode("utf-8", errors="ignore"), True
    return text, False


def page_snapshot(session: BrowserSession, page_id: str, timeout: int = 10000) -> dict[str, Any]:
    """Accessibility tree with [ref=eN] marks; use refs as locator type "ref"."""
    page = session.get_page(page_id)
    text, truncated = _truncate(session, page.aria_snapshot(mode="ai", timeout=timeout))
    return {"url": page.url, "title": page.title(), "snapshot": text, "truncated": truncated}


def take_screenshot(
    session: BrowserSession,
    page_id: str,
    full_page: bool = False,
    loc_type: LocatorType | None = None,
    value: str | None = None,
    timeout: int = 10000,
) -> dict[str, Any]:
    """PNG screenshot of the page or one element, base64-encoded."""
    page = session.get_page(page_id)
    if loc_type and value:
        png = resolve_locator(page, loc_type, value).first.screenshot(timeout=timeout)
    else:
        png = page.screenshot(full_page=full_page, timeout=timeout)
    return {"mimeType": "image/png", "data": base64.b64encode(png).decode("ascii")}


def navigate(
    session: BrowserSession,
    page_id: str,
    url: str,
    wait_until: str = "load",
    timeout: int = 30000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    resp = page.goto(url, wait_until=wait_until, timeout=timeout)  # type: ignore[arg-type]
    return {"url": page.url, "title": page.title(), "status": resp.status if resp else None}


def history(
    session: BrowserSession,
    page_id: str,
    direction: Literal["back", "forward", "reload"],
    timeout: int = 30000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    # "commit": pages restored from bfcache never fire "load".
    if direction == "back":
        resp = page.go_back(timeout=timeout, wait_until="commit")
    elif direction == "forward":
        resp = page.go_forward(timeout=timeout, wait_until="commit")
    else:
        resp = page.reload(timeout=timeout)
    return {"url": page.url, "title": page.title(), "status": resp.status if resp else None}


def evaluate(
    session: BrowserSession,
    page_id: str,
    expression: str,
    isolated: bool = False,
) -> dict[str, Any]:
    """Run JS (expression or function source). Main world by default so page globals are visible."""
    page = session.get_page(page_id)
    return {"result": page.evaluate(expression, isolated_context=isolated)}


def wait_for_text(
    session: BrowserSession,
    page_id: str,
    text: str,
    gone: bool = False,
    timeout: int = 10000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    page.get_by_text(text).first.wait_for(state="hidden" if gone else "visible", timeout=timeout)
    return {"text": text, "gone": gone}


# ---------------------------------------------------------------------------
# Element finding and reading
# ---------------------------------------------------------------------------

def find_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value)
    loc.first.wait_for(state="attached", timeout=timeout)
    count = loc.count()
    return {"found": count, "locator": {"type": loc_type, "value": value}}


def find_all_elements(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value)
    loc.first.wait_for(state="attached", timeout=timeout)
    count = loc.count()
    elements = []
    for i in range(count):
        el = loc.nth(i)
        elements.append({
            "index": i,
            "text": el.inner_text(),
            "visible": el.is_visible(),
        })
    return {"found": count, "elements": elements}


def read_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> dict[str, Any]:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.wait_for(state="attached", timeout=timeout)
    result: dict[str, Any] = {
        "text": loc.inner_text(),
        "innerHTML": loc.inner_html(),
        "visible": loc.is_visible(),
        "enabled": loc.is_enabled(),
        "checked": None,
        "value": None,
    }
    try:
        result["value"] = loc.input_value()
    except Exception:
        pass
    try:
        result["checked"] = loc.is_checked()
    except Exception:
        pass
    return result


# ---------------------------------------------------------------------------
# Element actions
# ---------------------------------------------------------------------------

def click_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
    double: bool = False,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    if double:
        loc.dblclick(timeout=timeout)
    else:
        loc.click(timeout=timeout)


def hover_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.hover(timeout=timeout)


def focus_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.wait_for(state="attached", timeout=timeout)
    loc.focus()


def press_key(
    session: BrowserSession,
    page_id: str,
    key: str,
) -> None:
    page = session.get_page(page_id)
    page.keyboard.press(key)


# ---------------------------------------------------------------------------
# Form actions
# ---------------------------------------------------------------------------

def fill_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    text: str,
    clear_before: bool = False,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    if clear_before:
        loc.clear(timeout=timeout)
    loc.fill(text, timeout=timeout)


def upload_files(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    files: list[str],
    timeout: int = 10000,
) -> dict[str, Any]:
    """Attach local files to a file input, or to the chooser opened by clicking the element."""
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    is_input = loc.evaluate("e => e.tagName === 'INPUT' && e.type === 'file'", timeout=timeout)
    if is_input:
        loc.set_input_files(files, timeout=timeout)
    else:
        with page.expect_file_chooser(timeout=timeout) as chooser:
            loc.click(timeout=timeout)
        chooser.value.set_files(files, timeout=timeout)
    return {"files": files}


def clear_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.clear(timeout=timeout)


def select_option(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    option: str | list[str],
    timeout: int = 10000,
) -> list[str]:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.wait_for(state="attached", timeout=timeout)
    if isinstance(option, str):
        option = [option]
    return loc.select_option(option, timeout=timeout)


def check_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    checked: bool = True,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    if checked:
        loc.check(timeout=timeout)
    else:
        loc.uncheck(timeout=timeout)


def submit_form(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.wait_for(state="attached", timeout=timeout)
    loc.evaluate("el => { const f = el.closest('form'); if(f) f.submit(); }")


# ---------------------------------------------------------------------------
# Wait actions
# ---------------------------------------------------------------------------

def wait_for_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    state: str = "visible",
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.wait_for(state=state, timeout=timeout)


def wait_for_navigation(
    session: BrowserSession,
    page_id: str,
    url: str | None = None,
    wait_until: str = "load",
    timeout: int = 30000,
) -> dict[str, str]:
    page = session.get_page(page_id)
    page.wait_for_load_state(wait_until, timeout=timeout)
    return {"url": page.url}


def wait_for_network_idle(
    session: BrowserSession,
    page_id: str,
    timeout: int = 30000,
) -> None:
    page = session.get_page(page_id)
    page.wait_for_load_state("networkidle", timeout=timeout)


# ---------------------------------------------------------------------------
# Mouse
# ---------------------------------------------------------------------------

def mouse_move(
    session: BrowserSession,
    page_id: str,
    x: float,
    y: float,
    steps: int = 1,
) -> None:
    page = session.get_page(page_id)
    page.mouse.move(x, y, steps=steps)


def mouse_move_to_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> dict[str, float]:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    box = loc.bounding_box(timeout=timeout)
    if box is None:
        raise ValueError("Element has no bounding box (not visible?)")
    cx = box["x"] + box["width"] / 2
    cy = box["y"] + box["height"] / 2
    page.mouse.move(cx, cy)
    return {"x": cx, "y": cy}


def mouse_scroll(
    session: BrowserSession,
    page_id: str,
    delta_x: float = 0,
    delta_y: float = 0,
) -> None:
    page = session.get_page(page_id)
    page.mouse.wheel(delta_x, delta_y)


def mouse_scroll_to_element(
    session: BrowserSession,
    page_id: str,
    loc_type: LocatorType,
    value: str,
    timeout: int = 10000,
) -> None:
    page = session.get_page(page_id)
    loc = resolve_locator(page, loc_type, value).first
    loc.scroll_into_view_if_needed(timeout=timeout)


def mouse_move_random(
    session: BrowserSession,
    page_id: str,
    start_x: float,
    start_y: float,
    end_x: float,
    end_y: float,
    steps: int = 25,
    duration_ms: int = 300,
    jitter: float = 3.0,
    seed: int | None = None,
) -> list[dict[str, float]]:
    """Move mouse along a bezier-like random trajectory."""
    rng = random.Random(seed)
    page = session.get_page(page_id)

    # Generate control points for a cubic bezier with jitter
    ctrl1_x = start_x + (end_x - start_x) / 3 + rng.gauss(0, jitter * 5)
    ctrl1_y = start_y + (end_y - start_y) / 3 + rng.gauss(0, jitter * 5)
    ctrl2_x = start_x + 2 * (end_x - start_x) / 3 + rng.gauss(0, jitter * 5)
    ctrl2_y = start_y + 2 * (end_y - start_y) / 3 + rng.gauss(0, jitter * 5)

    points: list[dict[str, float]] = []
    delay_per_step = duration_ms / max(steps, 1) / 1000.0

    for i in range(steps + 1):
        t = i / max(steps, 1)
        # Cubic bezier
        u = 1 - t
        x = (u**3 * start_x
             + 3 * u**2 * t * ctrl1_x
             + 3 * u * t**2 * ctrl2_x
             + t**3 * end_x)
        y = (u**3 * start_y
             + 3 * u**2 * t * ctrl1_y
             + 3 * u * t**2 * ctrl2_y
             + t**3 * end_y)

        # Add per-step jitter (smaller near endpoints)
        edge_factor = 4 * t * (1 - t)  # 0 at endpoints, 1 at midpoint
        x += rng.gauss(0, jitter * edge_factor)
        y += rng.gauss(0, jitter * edge_factor)

        page.mouse.move(x, y)
        points.append({"x": round(x, 1), "y": round(y, 1)})
        if i < steps:
            time.sleep(delay_per_step)

    return points


# ---------------------------------------------------------------------------
# Action chains
# ---------------------------------------------------------------------------

_CHAIN_LOCATOR_ACTIONS = {
    "find-all", "read", "click", "dblclick", "hover", "focus", "clear", "check", "uncheck", "submit",
}
# Locator actions that act on the first match (the engine resolves a locator strictly)
_CHAIN_FIRST_MATCH = {"click", "dblclick", "hover", "focus", "clear", "check", "uncheck"}


def _escape_vars(value: Any) -> Any:
    """Chain strings are literal: neutralize ``${...}`` so the engine does not substitute them."""
    if isinstance(value, str):
        return value.replace("${", "$${")
    if isinstance(value, list):
        return [_escape_vars(v) for v in value]
    if isinstance(value, dict):
        return {k: _escape_vars(v) for k, v in value.items()}
    return value


def _chain_steps(step: dict[str, Any], timeout: int) -> list[dict[str, Any]] | None:
    """Engine steps equivalent to a legacy chain step; None if it keeps the legacy code path."""
    action = step.get("action", "")
    loc = step.get("locator") or {}
    locator = {"type": loc.get("type", "css"), "value": loc.get("value", "")}
    base: dict[str, Any] = {"timeout": timeout, "stopOnError": True}

    if action == "wait":
        if locator["value"]:
            state = step.get("state", "visible")
            if state not in {"visible", "hidden"}:  # attached/detached are not engine wait modes
                return None
            return [{**base, "action": "wait", "locator": locator, "waitUntil": state, "index": 0}]
        until = step.get("waitUntil", "load")
        if until not in {"load", "domcontentloaded", "networkidle"}:
            return None
        return [{**base, "action": "wait", "waitUntil": until}]
    if action in _CHAIN_LOCATOR_ACTIONS:
        extra = {"index": 0} if action in _CHAIN_FIRST_MATCH else {}
        return [{**base, "action": action, "locator": locator, **extra}]
    if action == "fill":
        fill = {**base, "action": "fill", "locator": locator, "value": step.get("value", ""), "index": 0}
        if step.get("clearBefore", False):
            return [{**base, "action": "clear", "locator": locator, "index": 0}, fill]
        return [fill]
    if action == "select":
        option = step.get("option", [])
        pick = {"value": option} if isinstance(option, str) else {"options": option}
        return [{**base, "action": "select", "locator": locator, **pick}]
    if action == "press":
        return [{**base, "action": "press", "key": step.get("key", "Enter")}]
    if action == "scroll":
        return [{**base, "action": "scroll", "deltaX": step.get("deltaX", 0), "deltaY": step.get("deltaY", 0)}]
    if action == "mouse-move":
        return [{**base, "action": "mouse-move", "x": step.get("x", 0), "y": step.get("y", 0),
                 "steps": step.get("steps", 1)}]
    if action == "mouse-move-random":
        keys = ("startX", "startY", "endX", "endY", "steps", "durationMs", "jitter", "seed")
        given = {k: step[k] for k in keys if k in step}
        return [{**base, "action": "mouse-random", "endX": 0, "endY": 0, **given}]
    return None


def _legacy_step(
    session: BrowserSession, page_id: str, step: dict[str, Any], timeout: int, result: dict[str, Any]
) -> None:
    """Chain steps the engine cannot express: find, wait for attached/detached, unknown actions."""
    action = step.get("action", "")
    loc = step.get("locator") or {}
    loc_type = loc.get("type", "css")
    loc_value = loc.get("value", "")
    if action == "find":
        result["data"] = find_element(session, page_id, loc_type, loc_value, timeout)
    elif action == "wait" and loc_value:
        wait_for_element(session, page_id, loc_type, loc_value, step.get("state", "visible"), timeout)
    elif action == "wait":
        wait_for_navigation(session, page_id, wait_until=step.get("waitUntil", "load"), timeout=timeout)
    else:
        result["success"] = False
        result["error"] = f"Unknown action: {action}"


def run_action_chain(
    session: BrowserSession,
    page_id: str,
    steps: list[dict[str, Any]],
    default_timeout: int = 10000,
) -> list[dict[str, Any]]:
    """Execute a sequence of actions through the scenario engine, returning flat per-step results."""
    from autobrowser_client.browser.engine import run_script  # engine imports this module
    from autobrowser_client.scenario import ScriptDefinition

    results: list[dict[str, Any]] = []
    for step in steps:
        action = step.get("action", "")
        timeout = step.get("timeout", default_timeout)
        t0 = time.monotonic()
        result: dict[str, Any] = {"action": action, "success": True}
        try:
            engine_steps = _chain_steps(step, timeout)
            if engine_steps is None:
                _legacy_step(session, page_id, step, timeout, result)
            else:
                script = ScriptDefinition.model_validate({"steps": _escape_vars(engine_steps)})
                nodes = run_script(session, script, page_id)["steps"]
                node = next((n for n in nodes if not n["success"]), nodes[-1])
                result["success"] = node["success"]
                for key in ("data", "error"):
                    if key in node:
                        result[key] = node[key]
        except Exception as exc:
            result["success"] = False
            result["error"] = str(exc)

        result["durationMs"] = round((time.monotonic() - t0) * 1000, 1)
        results.append(result)

        if not result["success"] and step.get("stopOnError", True):
            break

    return results
