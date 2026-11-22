---
name: autobrowser
description: Drive a stealth browser through the autobrowser MCP server (browser_* tools) - open pages, read them, click, fill forms, upload files, take screenshots, pass Cloudflare challenges, search the web (Google, Bing, DuckDuckGo) and run multi-step YAML scenarios. Use when a task needs a real browser or a web search, when a site blocks plain HTTP requests or shows a Cloudflare / Turnstile check, or when the user mentions autobrowser.
---

# autobrowser

The `autobrowser` MCP server gives one stealth Chromium (patchright or cloakbrowser) as `browser_*` tools. The browser starts on the first call and keeps cookies, tabs and state until `browser_close` or the server stops.

Other MCP servers (for example Playwright MCP) also have `browser_*` tools with the same names. Use only the tools of the `autobrowser` server: clients usually prefix them with the server name, such as `mcp__autobrowser__browser_navigate`.

If the autobrowser tools are missing, the server is not connected. Tell the user to install it by following https://github.com/foxzi/autobrowser-client/blob/main/docs/install-agent.md

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

## Many steps at once

For a known multi-step flow (login, pagination, scraping a list) use `browser_run_script(script=...)` instead of many single calls. It runs on the current browser session (the scenario `session` block is ignored) and returns `{success, steps, vars}`; read the failed step's `error` and fix the scenario. Use the single tools when the next step depends on what you see.

```yaml
vars: {base: "https://quotes.toscrape.com"}
steps:
  - open: "${base}"
  - if: {exists: "#cookie-accept"}
    then: [{click: "#cookie-accept"}]
  - action: find-all
    locator: "div.quote span.text"
    as: quotes
  - while: {exists: "li.next a"}
    max: 20                     # exceeding max is an error
    do:
      - click: "li.next a"
      - wait-text: Quotes
  - try:
      - action: click
        locator: "text=Login"
        retry: 2
    catch:
      - fail: "login link missing: ${error.message}"
  - assert: {url: "/login"}
```

- Locators: `"css"`, `"xpath=//a"`, `"text=Next"`, or `{type, value}`.
- Actions: `open`, `history`, `click`, `dblclick`, `hover`, `focus`, `fill`, `type`, `clear`, `press`, `check`/`uncheck`, `select`, `submit`, `upload`, `wait` (ms or element), `wait-text`, `find`, `scroll`, `evaluate`, `extract`, `find-all`, `read`, `get-content`, `get-cookies`, `get-headers`, `screenshot`, `save`, `solve-challenge`, `browse-tabs`, `mouse-move`, `mouse-scroll-to`, `mouse-random`, `set`, `assert`, `fail`.
- Variables: root `vars`, `${name}`, `${item.url}`, `${env.NAME}`; `as: name` stores a result (plain name, no `${...}`; overwritten each loop iteration); `set` assigns.
- Conditions (`if`, `while`, `assert`): `exists`, `visible`, `text`, `url`, `urlMatches`, `title`, `var` with `equals`/`contains`/`matches`, combined by `not`/`all`/`any`; `timeout: ms` waits until true.
- Blocks: `if/then/else`, `try/catch/finally` (`error` in catch), `forEach/as/do` (`index` too), `repeat/do`, `while/max/do`, `include`.
- Step options: `name`, `retry`, `retryDelay`, `optional`, `stopOnError`.
- Full syntax: https://github.com/foxzi/autobrowser-client/blob/main/docs/scripts.md
- Templates, JS and pitfalls in depth: https://github.com/foxzi/autobrowser-client/blob/main/docs/scenario-guide.md

## Writing scenarios

Build the scenario from what you saw on the page: first explore with `browser_navigate` + `browser_snapshot`, then turn the known steps into YAML. Pass changing input through `vars` of `browser_run_script` (`vars={"query": "..."}`, values are strings) instead of editing the script.

Templates `${...}`:

- Substituted in step fields, conditions, `forEach`/`repeat` and **inside `evaluate` scripts before the JS runs**.
- `"${obj}"` as the whole string keeps the type (list, dict, number); `"page ${n}"` inside text turns it into text.
- An undefined variable fails the step (`undefined variable: name`). A step with `optional: true` that failed does not set its `as:` variable.
- A dict or list in a text field (`value: "${results}"`) is a step error: build the text in JS and store it in a field.

JavaScript in `evaluate`:

- `script` can be an expression, statements, `() => ...` or `async () => ...`; the function gets no arguments. Use `script: |` for long code.
- A JS template literal `` `${x}` `` is taken as a scenario variable. Write `$${x}` or join strings with `+`.
- Values inserted into JS are raw text, not escaped: a quote in user text breaks the script. Put user text into the page with `type`/`fill` (`value: "${query}"`) instead of into JS.
- Return one JSON-like object with everything you need (`as: results`), then use `${results.items}`, `${results.text}`. DOM nodes, `Map`, `Set` do not come back; return `innerText`, `href`, arrays.
- After a step that navigates (`press: Enter`, a submit click) wait first (`action: wait` with `waitUntil: visible` and a locator of the new page, or `wait: load`); otherwise JS fails with `Execution context was destroyed`.
- Polling loops in JS need their own deadline: `while (Date.now() - t0 < 20000 && !done()) await new Promise(r => setTimeout(r, 500));`.

Files and results:

- `screenshot`, `save`, `get-content` take a `file` path relative to the server working directory; absolute paths and `..` are rejected.
- `save` with `value` writes that text (`value: "${results.text}"`, autobrowser 0.5.3+; older versions ignore `value` and save the page); without `value` it saves the page.
- Put the screenshot in `finally` so it is taken even when a step fails.
- Read the outcome from `steps` (every step with `success`, `error`, `data`) and `vars` of the result.

YAML: quote values containing `: `, `{`, `[` or words like `yes`, `no`, `on`, `null`.

## Web search

To search the web, open the results page directly instead of typing on the home page:

- Google: `https://www.google.com/search?q=<url-encoded query>&hl=en` (add `&num=20` for more results).
- DuckDuckGo without JS: `https://html.duckduckgo.com/html/?q=<query>` - simple markup, rarely blocks.
- Bing: `https://www.bing.com/search?q=<query>`.

Then extract results in one `browser_evaluate` call instead of a snapshot:

```js
() => [...document.querySelectorAll('#search a:has(h3)')].slice(0, 10)
  .map(a => ({title: a.querySelector('h3').innerText, url: a.href}))
```

DuckDuckGo HTML: `a.result__a` (title and link), `.result__snippet` (snippet). Bing: `#b_results h2 a`. Selectors change; if the list is empty, take a `browser_snapshot` and adjust.

Result links are redirect wrappers (`google.com/goto?url=...`, `bing.com/ck/a?...`, `duckduckgo.com/l/?uddg=...`). For DuckDuckGo the real URL is `new URL(a.href).searchParams.get('uddg')`; for others `browser_navigate` to the link and take the final `url` from the reply.

- Google captcha: URL contains `/sorry/` or the page has `#captcha-form` / a reCAPTCHA iframe. Do not retry in a loop; report it, switch to DuckDuckGo, or suggest a proxy or a logged-in profile (`--profile NAME` of the MCP server).
- Cookie consent (EU IPs) blocks the page: click "Accept all" / "Reject all" found in the snapshot, then continue.
- To read a found page, `browser_navigate` to its URL and use `browser_get_text`; open several results in tabs only when needed.

Repeated search as one scenario (results, captcha flag and a text file):

```yaml
steps:
  - try:
      - open: https://www.google.com/
      - action: evaluate                 # cookie consent, if shown
        script: |
          async () => {
            await new Promise(r => setTimeout(r, 1500));
            const b = [...document.querySelectorAll('button')]
              .find(x => /accept all|hyväksy kaikki|alle akzeptieren|tout accepter|принять все/i.test(x.innerText));
            if (b) { b.click(); await new Promise(r => setTimeout(r, 1500)); }
          }
      - action: type
        locator: "textarea[name='q'], input[name='q']"
        value: "${query}"                # user text goes to the page, not into JS
      - press: Enter
      - action: wait                     # results or captcha, whichever comes first
        waitUntil: visible
        locator: "#search a h3, #captcha-form, iframe[src*='recaptcha']"
        index: 0
        timeout: 20000
        optional: true
      - action: evaluate
        as: results
        script: |
          () => {
            const items = [...document.querySelectorAll('#search a:has(h3)')].slice(0, 10)
              .map(a => ({title: a.querySelector('h3').innerText, url: a.href}));
            return {
              captcha: /\/sorry\//.test(location.href) || !!document.querySelector('#captcha-form'),
              items,
              text: items.map((x, i) => (i + 1) + '. ' + x.title + '\n   ' + x.url).join('\n') + '\n',
            };
          }
      - action: save
        file: output/search.txt
        value: "${results.text}"
    finally:
      - action: screenshot
        file: output/search.png
```

Call it with `browser_run_script(script=..., vars={"query": "..."})` and read `vars.results`. The full version with a login check is `examples/google-search.yaml` in the repository.

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

- `browser_logs(kind="console")` - browser-level messages only (failed resources, security warnings). Page `console.log` and uncaught errors are not captured by design (stealth); to read page state use `browser_evaluate`.
- `browser_logs(kind="network")` - failed requests and HTTP statuses (API 401/500, blocked resources).
- Tool errors carry the browser message (e.g. timeout waiting for element): take a fresh snapshot and retry with a correct ref or a `by` locator rather than repeating the same call.
- `browser_close` then any call gives a fresh browser if the state is broken.

## Etiquette

- The browser is real and stateful: logins persist in the profile. Do not submit purchases, payments, messages or deletions without explicit user confirmation.
- Do not upload files or run `browser_evaluate` code the user did not ask for.
- Keep request rates human-like; do not hammer a site in tight loops.
