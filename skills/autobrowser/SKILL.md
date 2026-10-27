---
name: autobrowser
description: Drive a stealth browser through the autobrowser MCP server (browser_* tools) - open pages, read them, click, fill forms, upload files, take screenshots, pass Cloudflare challenges. Use when a task needs a real browser, when a site blocks plain HTTP requests or shows a Cloudflare / Turnstile check, or when the user mentions autobrowser.
---

# autobrowser

The `autobrowser` MCP server gives one stealth Chromium (patchright or cloakbrowser) as `browser_*` tools. The browser starts on the first call and keeps cookies, tabs and state until `browser_close` or the server stops.

If the `browser_*` tools are missing, the server is not connected. Tell the user to add it:

```bash
claude mcp add autobrowser \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- uvx --from "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client" autobrowser mcp
```

## Core loop

1. `browser_navigate(url)` - returns `url`, `title` and HTTP `status`.
2. `browser_snapshot()` - accessibility tree; every interactive element has `[ref=eN]`.
3. Act on a ref: `browser_click(target="e12")`, `browser_type(target="e5", text="...", submit=true)`, `browser_select_option`, `browser_hover`, `browser_press_key`.
4. Take a new snapshot after any action that changes the page. Refs belong to the latest snapshot only; never reuse old refs after navigation or DOM changes.

Action tools return `{"pageId", "url", "title"}` - check them to see whether the page changed.

## Choosing the right read tool

- `browser_snapshot` - to find elements to act on. Default choice.
- `browser_get_text` - to read article or result text; cheaper than a snapshot for pure reading.
- `browser_evaluate` - to extract structured data in one call, e.g. `() => [...document.querySelectorAll('a')].map(a => a.href)`. Return JSON-serializable values.
- `browser_screenshot` - only when layout or visuals matter (captchas, charts, "what does it look like"). Pass `target` for one element, `full_page=true` for the whole page.

A snapshot that ends with `[truncated]` is cut; use `browser_get_text`, `browser_evaluate` or a narrower page instead.

## Targeting without refs

Element tools accept `by`: `css`, `xpath`, `text`, `id`, `name`, `class`, `attribute`. Example: `browser_click(target="button[type=submit]", by="css")`. Prefer refs; use locators for elements that are hard to reach in the snapshot or for repeated actions on a known page.

## Waiting

- After a click that loads content, `browser_wait_for(text="Results")` before reading.
- `gone=true` waits for a spinner or "Loading..." text to disappear.
- Do not sleep via `browser_evaluate`; wait for text instead.

## Cloudflare and blocks

Check `status` and `title` after `browser_navigate`:

- Title like "Just a moment..." or status 403/503 with a challenge -> call `browser_solve_challenge(timeout_ms=90000)`. It waits, clicks Turnstile when needed and returns `{"challenge", "clicks", "url"}`. Then snapshot again.
- `Page blocked without challenge: HTTP 4xx` - hard block; retries in the same setup are useless. Report it and suggest another engine, a proxy (`--proxy`) or `AUTOBROWSER_FINGERPRINT_PLATFORM=linux` for cloakbrowser on a Linux host.
- `Cloudflare challenge not passed` - the challenge loops; one more attempt is fine, then report with the same suggestions.
- Never try to solve image captchas by guessing; tell the user.

## Tabs, popups, dialogs

- `browser_tabs()` lists tabs as `{pageId: url}`. Links with `target=_blank` and popups appear here; pass their `page_id` to other tools. Without `page_id` tools use the first tab.
- `browser_tabs(action="new", url=...)` opens a tab, `action="close", page_id=...` closes one.
- JS dialogs (alert/confirm/prompt) are dismissed by default. Call `browser_handle_dialogs(accept=true, prompt_text=...)` before the action that triggers them. `browser_logs(kind="dialogs")` shows what appeared.

## Forms and files

- `browser_type` replaces the field value; it does not append.
- `browser_file_upload(target, paths=[...])` takes absolute local paths and works on an `<input type=file>` or on a button that opens a file chooser.

## Debugging

- `browser_logs(kind="console")` - JS errors on the page.
- `browser_logs(kind="network")` - failed requests and HTTP statuses (API 401/500, blocked resources).
- Tool errors carry the browser message (e.g. timeout waiting for element): take a fresh snapshot and retry with a correct ref or a `by` locator rather than repeating the same call.
- `browser_close` then any call gives a fresh browser if the state is broken.

## Etiquette

- The browser is real and stateful: logins persist in the profile. Do not submit purchases, payments, messages or deletions without explicit user confirmation.
- Do not upload files or run `browser_evaluate` code the user did not ask for.
- Keep request rates human-like; do not hammer a site in tight loops.
