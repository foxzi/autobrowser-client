"""MCP server end to end: a real MCP client talks to `autobrowser mcp` over stdio."""

from __future__ import annotations

import os
import re
import sys

import pytest
from mcp import ClientSession, StdioServerParameters, stdio_client

PAGE = (
    "data:text/html,<title>T</title><input id=q>"
    "<button onclick=\"document.body.insertAdjacentHTML('beforeend','<p>clicked</p>')\">Go</button>"
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
        assert {"browser_navigate", "browser_snapshot", "browser_click"} <= tools

        async def call(name: str, **args: object):
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

        res = await call("browser_close")
        assert not res.is_error
