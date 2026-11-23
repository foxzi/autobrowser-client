"""MCP server end to end: a real MCP client talks to `autobrowser mcp` over stdio."""

from __future__ import annotations

import json
import os
import re
import sys

import anyio
import pytest
from mcp import ClientSession, StdioServerParameters, stdio_client

PAGE = (
    "data:text/html,<title>T</title><input id=q>"
    "<button onclick=\"document.body.insertAdjacentHTML('beforeend','<p>clicked</p>')\">Go</button>"
    "<div id=box><b>boxed</b> text 0123456789</div><div style='height:3000px'></div>"
)


@pytest.mark.anyio
async def test_mcp_browser_flow() -> None:
    params = StdioServerParameters(
        command=sys.executable,
        args=["-m", "autobrowser_client.main", "mcp", "--headless"],
        env={**os.environ, "AUTOBROWSER_LOG_LEVEL": "WARNING"},
    )
    async with (
        stdio_client(params) as (read, write),
        ClientSession(read, write) as client,
    ):
        await client.initialize()
        tools = {t.name for t in (await client.list_tools()).tools}
        assert {"browser_navigate", "browser_snapshot", "browser_click", "browser_cookies", "browser_scroll"} <= tools

        async def call(name: str, /, **args: object):
            res = await client.call_tool(name, args)
            return res

        res = await call("browser_navigate", url=PAGE)
        assert not res.is_error, res.content

        snap = (await call("browser_snapshot")).content[0].text
        ref = re.search(r'button "Go" \[ref=(e\d+)\]', snap).group(1)

        res = await call("browser_click", target=ref)
        assert not res.is_error, res.content
        res = await call("browser_wait_for", text="clicked")
        assert not res.is_error, res.content

        res = await call("browser_type", target="#q", by="css", text="hello")
        assert not res.is_error, res.content
        res = await call(
            "browser_evaluate", expression="() => document.querySelector('#q').value"
        )
        assert res.content[0].text == '"hello"'

        shot = await call("browser_screenshot")
        assert shot.content[0].type == "image"

        # failures reach the model as tool errors with a message
        res = await call("browser_click", target="#missing", by="css")
        assert res.is_error
        assert "Timeout" in res.content[0].text
        assert ": :" not in res.content[0].text

        # cookies: set, get, clear by name, clear all
        cookie = {"name": "a", "value": "1", "domain": "example.com", "path": "/"}
        res = await call("browser_cookies", action="set", cookies=[cookie, {**cookie, "name": "b"}])
        assert not res.is_error, res.content
        got = json.loads(res.content[0].text)
        assert {(c["name"], c["value"]) for c in got} == {("a", "1"), ("b", "1")}
        got = json.loads((await call("browser_cookies")).content[0].text)
        assert len(got) == 2
        res = await call("browser_cookies", action="clear", name="a")
        assert [c["name"] for c in json.loads(res.content[0].text)] == ["b"]
        res = await call("browser_cookies", action="clear")
        assert json.loads(res.content[0].text) == []
        res = await call("browser_cookies", action="set")
        assert res.is_error
        assert "cookies is required to set cookies" in res.content[0].text
        res = await call("browser_cookies", action="set", cookies=[{"name": "x", "value": "1"}])
        assert res.is_error
        assert "Cookie should have a url or a domain/path pair" in res.content[0].text

        # scroll by wheel delta
        async def scroll_y() -> object:
            res = await call("browser_evaluate", expression="() => window.scrollY")
            return json.loads(res.content[0].text)

        assert await scroll_y() == 0
        res = await call("browser_scroll", delta_y=500)
        assert not res.is_error, res.content
        assert json.loads(res.content[0].text)["title"] == "T"
        for _ in range(20):
            if await scroll_y() > 0:
                break
            await anyio.sleep(0.1)
        assert await scroll_y() > 0

        # text and snapshot of one element, max_chars
        res = await call("browser_get_text", target="#box", by="css")
        assert res.content[0].text == "boxed text 0123456789"
        res = await call("browser_get_text", target="#box", by="css", max_chars=5)
        assert res.content[0].text == "boxed\n... [truncated, 16 more chars]"
        res = await call("browser_snapshot", target="#box", by="css")
        assert not res.is_error, res.content
        sub = res.content[0].text
        assert "boxed" in sub
        assert "button \"Go\"" not in sub
        subref = re.search(r"\[ref=(e\d+)\]", sub).group(1)
        res = await call("browser_click", target=subref)
        assert not res.is_error, res.content
        res = await call("browser_snapshot", max_chars=10)
        assert res.content[0].text.endswith("more chars]")
        assert len(res.content[0].text.split("\n... [truncated")[0]) == 10

        res = await call("browser_close")
        assert not res.is_error


@pytest.mark.anyio
async def test_mcp_run_script(tmp_path) -> None:
    params = StdioServerParameters(
        command=sys.executable,
        args=["-m", "autobrowser_client.main", "mcp", "--headless"],
        env={**os.environ, "AUTOBROWSER_LOG_LEVEL": "WARNING"},
        cwd=tmp_path,
    )
    (tmp_path / "s.yaml").write_text(
        "steps:\n  - open: \"${url}\"\n  - extract: h1\n    as: heading\n"
    )
    async with (
        stdio_client(params) as (read, write),
        ClientSession(read, write) as client,
    ):
        await client.initialize()
        page = "data:text/html,<h1>Hello</h1>"

        res = await client.call_tool(
            "browser_run_script",
            {"script": 'steps:\n  - open: "${url}"\n  - extract: h1\n    as: heading\n',
             "vars": {"url": page}},
        )
        assert not res.is_error, res.content
        data = json.loads(res.content[0].text)
        assert data["success"] is True
        assert data["vars"]["heading"] == "Hello"
        assert [s["action"] for s in data["steps"]] == ["open", "extract"]

        res = await client.call_tool(
            "browser_run_script", {"file": "s.yaml", "vars": {"url": page}}
        )
        assert not res.is_error, res.content
        assert json.loads(res.content[0].text)["vars"]["heading"] == "Hello"

        # a failing step is reported in the result, not as a tool error
        res = await client.call_tool(
            "browser_run_script", {"script": "steps:\n  - fail: boom\n"}
        )
        assert not res.is_error
        data = json.loads(res.content[0].text)
        assert data["success"] is False
        assert data["steps"][0]["error"] == "boom"

        # bad arguments and invalid scripts are tool errors
        res = await client.call_tool("browser_run_script", {})
        assert res.is_error
        res = await client.call_tool("browser_run_script", {"script": "steps:\n  - bogus: 1\n"})
        assert res.is_error
        assert "SCRIPT_VALIDATION_ERROR" in res.content[0].text

        await client.call_tool("browser_close", {})
