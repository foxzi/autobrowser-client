"""MCP server (stdio): one browser session per server process, started lazily."""

from __future__ import annotations

import base64
import json
import logging
from pathlib import Path
from typing import Annotated, Any, Literal

from mcp.server import MCPServer
from mcp.server.mcpserver import Image
from mcp.server.mcpserver.exceptions import ToolError
from pydantic import Field

from autobrowser_client.browser import actions
from autobrowser_client.browser.engine import run_script
from autobrowser_client.browser.executor import solve_cf_challenge
from autobrowser_client.browser.service import BrowserSession, SessionState
from autobrowser_client.config import Settings
from autobrowser_client.errors import AppError
from autobrowser_client.scenario import LocatorType, load_script, parse_script

logger = logging.getLogger(__name__)

INSTRUCTIONS = """\
Stealth browser (patchright / cloakbrowser) for sites with anti-bot protection.
Workflow: browser_navigate -> browser_snapshot -> act on elements by their ref
(e.g. target="e12") -> browser_snapshot again to see the result.
Refs come from the latest snapshot and go stale after the page changes.
Use other locators with by="css" | "xpath" | "text" | ... when needed.
On a Cloudflare "Just a moment" page call browser_solve_challenge.
page_id is optional everywhere: the first tab is used by default.
"""

Target = Annotated[
    str,
    Field(
        description="Element ref from browser_snapshot (e.g. e12), or a locator value for `by`"
    ),
]
By = Annotated[LocatorType, Field(description="Locator type of target; ref by default")]
PageId = Annotated[
    str | None, Field(description="Tab id from browser_tabs; first tab by default")
]


def create_server(settings: Settings, profile: str | None = None) -> MCPServer:
    server = MCPServer("autobrowser", instructions=INSTRUCTIONS)
    state: dict[str, BrowserSession | None] = {"session": None}
    action_ms = settings.timeouts.action_ms
    nav_ms = settings.timeouts.navigation_ms

    def session() -> BrowserSession:
        s = state["session"]
        if s is None or s.state != SessionState.RUNNING or not s.list_pages():
            if s is not None:
                s.stop()
            s = BrowserSession("mcp", settings)
            state["session"] = s
            try:
                s.start(profile=profile)
            except Exception as exc:
                state["session"] = None
                raise ToolError(f"Browser start failed: {exc}") from exc
        return s

    def run(fn: Any, *args: Any, **kwargs: Any) -> Any:
        """Run fn(session, ...) on the session thread; report failures to the model."""
        s = session()
        try:
            return s.call(fn, s, *args, **kwargs)
        except ToolError:
            raise
        except AppError as exc:
            raise ToolError(f"{exc.code}: {exc.message}") from exc
        except Exception as exc:
            raise ToolError(str(exc)[:2000]) from exc

    def page_id_of(s: BrowserSession, page_id: str | None) -> str:
        return page_id or next(iter(s.list_pages()))

    def page_info(s: BrowserSession, page_id: str | None) -> str:
        page = s.get_page(page_id)
        return json.dumps(
            {"pageId": page_id_of(s, page_id), "url": page.url, "title": page.title()}
        )

    def act(fn: Any, page_id: str | None, *args: Any, **kwargs: Any) -> str:
        def _do(s: BrowserSession) -> str:
            pid = page_id_of(s, page_id)
            fn(s, pid, *args, **kwargs)
            return page_info(s, pid)

        return run(_do)

    @server.tool()
    def browser_navigate(url: str, page_id: PageId = None) -> str:
        """Open a URL in the tab; starts the browser on first use."""
        return json.dumps(
            run(
                lambda s: actions.navigate(
                    s, page_id_of(s, page_id), url, timeout=nav_ms
                )
            )
        )

    @server.tool()
    def browser_history(
        direction: Literal["back", "forward", "reload"], page_id: PageId = None
    ) -> str:
        """Go back, forward, or reload the tab."""
        return json.dumps(
            run(
                lambda s: actions.history(
                    s, page_id_of(s, page_id), direction, timeout=nav_ms
                )
            )
        )

    @server.tool()
    def browser_snapshot(page_id: PageId = None) -> str:
        """Accessibility tree of the page with [ref=eN] marks to target elements."""
        data = run(
            lambda s: actions.page_snapshot(
                s, page_id_of(s, page_id), timeout=action_ms
            )
        )
        head = f"URL: {data['url']}\nTitle: {data['title']}\n"
        tail = "\n[truncated]" if data["truncated"] else ""
        return head + data["snapshot"] + tail

    @server.tool()
    def browser_screenshot(
        full_page: bool = False,
        target: Annotated[
            str | None,
            Field(description="Element ref or locator; whole viewport if empty"),
        ] = None,
        by: By = "ref",
        page_id: PageId = None,
    ) -> Image:
        """PNG screenshot of the viewport, the full page, or one element."""
        data = run(
            lambda s: actions.take_screenshot(
                s,
                page_id_of(s, page_id),
                full_page,
                by if target else None,
                target,
                timeout=action_ms,
            )
        )
        return Image(data=base64.b64decode(data["data"]), format="png")

    @server.tool()
    def browser_click(
        target: Target, by: By = "ref", double: bool = False, page_id: PageId = None
    ) -> str:
        """Click an element."""
        return act(
            actions.click_element, page_id, by, target, timeout=action_ms, double=double
        )

    @server.tool()
    def browser_hover(target: Target, by: By = "ref", page_id: PageId = None) -> str:
        """Move the mouse over an element."""
        return act(actions.hover_element, page_id, by, target, timeout=action_ms)

    @server.tool()
    def browser_type(
        target: Target,
        text: str,
        by: By = "ref",
        submit: Annotated[bool, Field(description="Press Enter after typing")] = False,
        page_id: PageId = None,
    ) -> str:
        """Replace the value of an input or textarea with text."""

        def _do(s: BrowserSession, pid: str) -> None:
            actions.fill_element(s, pid, by, target, text, timeout=action_ms)
            if submit:
                actions.resolve_locator(s.get_page(pid), by, target).first.press(
                    "Enter", timeout=action_ms
                )

        return act(_do, page_id)

    @server.tool()
    def browser_press_key(
        key: Annotated[
            str, Field(description="Key name, e.g. Enter, Escape, ArrowDown, Control+a")
        ],
        page_id: PageId = None,
    ) -> str:
        """Press a key on the focused element."""
        return act(actions.press_key, page_id, key)

    @server.tool()
    def browser_select_option(
        target: Target, values: list[str], by: By = "ref", page_id: PageId = None
    ) -> str:
        """Select options of a <select> by value or label."""
        return act(
            actions.select_option, page_id, by, target, values, timeout=action_ms
        )

    @server.tool()
    def browser_file_upload(
        target: Target,
        paths: Annotated[list[str], Field(description="Absolute paths of local files")],
        by: By = "ref",
        page_id: PageId = None,
    ) -> str:
        """Attach local files to a file input or to the chooser opened by clicking target."""
        return act(actions.upload_files, page_id, by, target, paths, timeout=action_ms)

    @server.tool()
    def browser_evaluate(
        expression: Annotated[
            str,
            Field(
                description="JS expression or function source, e.g. () => document.title"
            ),
        ],
        page_id: PageId = None,
    ) -> str:
        """Run JavaScript in the page and return the JSON result."""
        data = run(lambda s: actions.evaluate(s, page_id_of(s, page_id), expression))
        return json.dumps(data["result"], ensure_ascii=False, default=str)

    @server.tool()
    def browser_wait_for(
        text: str,
        gone: Annotated[
            bool, Field(description="Wait for the text to disappear instead")
        ] = False,
        timeout_ms: int = 10000,
        page_id: PageId = None,
    ) -> str:
        """Wait until text appears (or disappears) on the page."""
        return act(actions.wait_for_text, page_id, text, gone=gone, timeout=timeout_ms)

    @server.tool()
    def browser_get_text(page_id: PageId = None) -> str:
        """Visible text of the page body."""
        return run(lambda s: s.get_content(page_id_of(s, page_id), "text"))

    @server.tool()
    def browser_tabs(
        action: Literal["list", "new", "close"] = "list",
        url: Annotated[str | None, Field(description="URL for a new tab")] = None,
        page_id: Annotated[str | None, Field(description="Tab to close")] = None,
    ) -> str:
        """List, open, or close tabs. Popups opened by the page show up here."""

        def _do(s: BrowserSession) -> dict[str, str]:
            if action == "new":
                s.new_tab(url)
            elif action == "close":
                if not page_id:
                    raise ToolError("page_id is required to close a tab")
                s.close_tab(page_id)
            return s.list_pages()

        return json.dumps(run(_do))

    @server.tool()
    def browser_logs(
        kind: Literal["console", "network", "dialogs"],
        clear: bool = False,
        page_id: PageId = None,
    ) -> str:
        """Recent console messages, network responses/failures, or JS dialogs of the tab.

        Console has only browser-level messages (failed resources, security warnings):
        page console.log and uncaught errors are not delivered, to stay undetectable.
        """
        return json.dumps(
            run(lambda s: s.get_log(page_id_of(s, page_id), kind, clear)),
            ensure_ascii=False,
        )

    @server.tool()
    def browser_handle_dialogs(accept: bool, prompt_text: str | None = None) -> str:
        """Set how future alert/confirm/prompt dialogs are answered (dismissed by default)."""
        s = session()
        s.dialog_policy = {"accept": accept, "promptText": prompt_text}
        return json.dumps(s.dialog_policy)

    @server.tool()
    def browser_solve_challenge(timeout_ms: int = 60000, page_id: PageId = None) -> str:
        """Wait for a Cloudflare challenge to pass, clicking Turnstile when shown."""
        return json.dumps(
            run(lambda s: solve_cf_challenge(s.get_page(page_id), timeout_ms)),
            default=str,
        )

    @server.tool()
    def browser_run_script(
        script: Annotated[
            str | None, Field(description="Scenario as YAML text (JSON also works)")
        ] = None,
        file: Annotated[str | None, Field(description="Path of a scenario file")] = None,
        vars: Annotated[
            dict[str, str] | None, Field(description="Variables, override the scenario vars")
        ] = None,
        page_id: PageId = None,
    ) -> str:
        """Run a multi-step scenario (YAML) in one call: variables, if/try/forEach/while, retries.

        Pass exactly one of script or file. Runs on the current browser session: the
        scenario `session` block is ignored. `include` paths are relative to the file's
        directory, or to the working directory for inline script text.
        Returns JSON {success, steps, vars}.
        """
        if (script is None) == (file is None):
            raise ToolError("Pass exactly one of script or file")
        try:
            definition = (
                load_script(file) if file is not None else parse_script(script, Path.cwd())
            )
        except AppError as exc:
            raise ToolError(f"{exc.code}: {exc.message}") from exc
        except Exception as exc:  # unreadable file, invalid YAML/JSON
            raise ToolError(f"Invalid script: {exc}"[:2000]) from exc
        return json.dumps(
            run(lambda s: run_script(s, definition, page_id, vars)),
            ensure_ascii=False,
            default=str,
        )

    @server.tool()
    def browser_close() -> str:
        """Close the browser; the next tool call starts a fresh one."""
        s = state["session"]
        state["session"] = None
        if s is not None:
            s.stop()
        return "closed"

    server.close_browser = browser_close  # type: ignore[attr-defined]
    return server


def run_server(settings: Settings, profile: str | None = None) -> None:
    server = create_server(settings, profile)
    try:
        server.run("stdio")
    finally:
        server.close_browser()  # type: ignore[attr-defined]
