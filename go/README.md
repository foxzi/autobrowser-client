# autobrowser (Go)

Go port of the Python `autobrowser` client (see `../python`). It supports only the
cloakbrowser engine (plain CDP via go-rod, no Node, no patchright). Static binary, no
runtime dependencies. The port is at parity with the Python client except for the
[known differences](#known-differences) below.

## Build

```bash
cd go && CGO_ENABLED=0 go build -o autobrowser ./cmd/autobrowser
```

Release binaries are named `autobrowser-go-<os>-<arch>[.exe]` (linux amd64/arm64, darwin arm64,
windows amd64) with a `.sha256` next to each; `autobrowser update` picks them.

## Usage

The default engine in the config is `patchright`, which the Go build does not run. Set the engine:

```bash
export AUTOBROWSER_BROWSER_ENGINE=cloakbrowser   # or browser.engine in the config file
autobrowser install                              # download the browser
autobrowser run-script -f file.yaml [--var K=V] [-c config.yaml] [-p profile] \
    [--proxy URL] [--viewport WxH] [-o result.json] [--preserve-session] [--artifacts DIR]
autobrowser mcp                                  # MCP server over stdio
autobrowser serve                                # HTTP API
```

Commands: `version`, `show-config`, `validate-script`, `run-script`, `run`, `serve`, `mcp`,
`install`, `install-skill`, `update [--check]`, `generate-fingerprint`, `help`.
`--var` is repeatable. The result JSON goes to stdout or to the `-o` file.

## Browser binary lookup

The cloakbrowser Chromium is found and, when missing, downloaded like cloakbrowser
`ensure_binary` does (`internal/cloak`): `CLOAKBROWSER_BINARY_PATH`, `CLOAKBROWSER_VERSION`,
`CLOAKBROWSER_CACHE_DIR` (default `~/.cloakbrowser`) work the same. The download is verified
against an Ed25519-signed SHA256SUMS before it is extracted.

## Parity with the Python client

| Feature | Python | Go |
|---|---|---|
| Config file, env, defaults, show-config | done | done |
| Scenario YAML, vars, `${}` templating, include | done | done |
| if / try / forEach / repeat / while, retry, optional, stopOnError | done | done |
| assert / set / fail, result JSON | done | done |
| Profiles, lock, preserveSession (`tabs.json`) | done | done |
| Actions: open, wait, find, find-all, click, dblclick, hover, focus, fill, clear, check, uncheck, type, press, scroll, screenshot, get-content, save, evaluate, history, wait-text, extract | done | done |
| Actions: get-cookies, get-headers, solve-challenge, browse-tabs, submit, select, upload, read, mouse-move, mouse-scroll-to, mouse-random | done | done |
| Locators (css, xpath, text, id, name, class, attribute, `ref`) | done | done (`text` approximates `get_by_text`) |
| Accessibility snapshot with refs (`page_snapshot`, locator `ref`) | done | done (Playwright injected script vendored in `internal/browser/injected`, regenerate with its `extract.sh`) |
| Regex conditions | done | done (RE2: no lookahead or backreferences) |
| Headless, viewport, userAgent, mobile emulation | done | done |
| cloakbrowser engine | done | done |
| patchright engine | done | not supported (error at launch) |
| Proxy (http, socks5, with login and password) | done (relay) | done (`socks2http` if installed, otherwise a built-in relay) |
| geoip (timezone and locale from the exit IP) | done | done (`internal/cloak`, GeoLite2) |
| JS fingerprint injection, `generate-fingerprint` | done | done |
| Browser binary download | done | done |
| Session manager (sessions, tabs, dialog policy, console/network/dialog logs, idle reap) | done | done (`internal/browser/manager.go`, `session.go`) |
| MCP server (`autobrowser mcp`, `browser_run_script` included) | done | done (`internal/mcp`, official go-sdk; tool schemas are the Python ones) |
| HTTP API (`autobrowser serve`, OpenAPI, `/docs`) | done | done (`internal/api`, net/http) |
| CLI: version, show-config, validate-script, run-script, run, serve, mcp, install, install-skill, update, generate-fingerprint, help | done | done |
| Self-update (`update`, background check in `mcp` and `serve`) | done | done (Go assets) |
| Release binaries | done (`autobrowser-<os>-<arch>`) | done (`autobrowser-go-<os>-<arch>`) |

## Known differences

Where the Go client cannot or does not match Python exactly. Result JSON shapes, config keys,
environment variables and exit codes are the same unless listed here.

### Engine

- Only `cloakbrowser`. `browser.engine=patchright` fails at launch with a `CONFIG_ERROR`
  (`show-config` works for any engine); `install` does not run `patchright install chrome`.
- A browser that dies at launch (for example `headless: false` without an X server) gives
  `BROWSER_START_FAILED "browser exited during startup: <chrome stderr>"` instead of Playwright's text.
- CDP errors outside the ported texts are plain protocol errors without Playwright's `Call log:`.

### Actions and locators

- `text` locators approximate `get_by_text`; `select` does not follow `<label>` to its control.
- Actionability is "attached and visible" only (no enabled / stable checks). `select`,
  `read` and `mouse-scroll-to` errors carry a short call log, not Playwright's retry log.
- `read` approximates `is_enabled` with `:disabled` and `aria-disabled` ancestors.
- `upload` with several files on a non-`multiple` input is not rejected.
- `get-headers` takes the Document responses of the main frame from CDP events; header names are
  sorted, Python keeps the received order.
- `mouse-random` uses the same algorithm with Go's `math/rand`, so a seed gives another path
  (the step output is the same).
- `solve-challenge` finds out-of-process iframes through `Target.getTargets`; the click point is the same.
- `evaluate` converts objects with `JSON.stringify`: `Infinity` and `NaN` give `null`, a `Date` gives
  its ISO string (`1970-01-01T00:00:00.000Z`), a DOM element `{}` (Python: `Infinity`,
  `1970-01-01 00:00:00+00:00`, `ref: <Node>`).
- Regex conditions use RE2; an invalid regex makes the check false.
- A failed step always has an `error` key (empty string without text); Python omits it for `None`.
- Scenario validation errors (`run-script`, `validate-script`, MCP `browser_run_script`, API
  `SCRIPT_VALIDATION_ERROR`) use Go paths such as `steps.0.action` without pydantic's
  `[type=..., input_value=...]` trailer and "For further information" line; Python prints
  `steps.0.step.action` with them. Error codes and exit codes are the same.

### Snapshot

- Frames nested in an out-of-process (cross-site) iframe get their `fN` prefix when first
  snapshotted, so numbering can differ from Playwright when several such frames appear.
- The snapshot retry schedule (0, 1, 2, 4, 8 s) approximates Playwright's.

### Network, logs

- Page `console.log` / `console.error` calls are not recorded: the CDP `Runtime` domain stays
  off for stealth. Only browser-level console messages (such as failed resource loads),
  network entries and dialogs are logged. Worker entries are skipped.
- Entry keys of the console, network and dialog logs are the same as in Python.

### Proxy and cloakbrowser download

- The relay log line masks the upstream password (`***`); Python prints it in clear text.
- The relay rejects non-ASCII upstream hosts with 502 (browsers send punycode anyway) and just
  closes the sockets on errors after the tunnel started. The `socks2http` subprocess gets
  SIGTERM, then kill after 5 s; its output is discarded.
- The Pro / license-key download and `CLOAKBROWSER_RELEASE_CHANNEL` are not supported
  (a set `CLOAKBROWSER_LICENSE_KEY` only logs a warning).
- No welcome banner and no background check for a newer cloakbrowser build;
  `latest_version*` markers written by Python are still honoured.
- Archive extraction refuses entries that escape the target directory.
- Download timeout is "no data for 60 s" (geoip database: 300 s), not httpx's per-phase timeouts.
- geoip: SOCKS5 proxies work natively (Python needs `cloakbrowser[geoip]`); the error reads
  `browser.geoip failed: <err>` without the pip hint; the exit-IP lookup without a proxy honours
  `HTTP_PROXY` / `HTTPS_PROXY` but not `ALL_PROXY`; an empty timezone in the database is `""`.

### Fingerprint

- Generation reproduces Python's `random.Random(seed).choice` exactly; only the random seed
  (no `--seed`) differs, as it is random in both. Seeds are int64 (Python: any size).
- `fingerprint.json` is loaded strictly by type (no pydantic coercion of `"true"` or `"123"`);
  an invalid file is regenerated.

### CLI output

- Help, usage and error output is plain text (no Rich boxes); texts follow Typer. Single-dash
  long options (`-config`) are accepted too. Without arguments the help goes to stdout, exit code 2.
- `update` uses the `autobrowser-go-<os>-<arch>` assets (`darwin`, not `macos`); an updater error
  other than a checksum mismatch prints the plain error and exits 1 (Python: traceback).
- `serve` prints one line `Serving the API on http://HOST:PORT` instead of uvicorn's log lines.
- `run --profile-dir` is passed as typed (Python normalizes it through `pathlib.Path`).
- The version is set at build time (`-ldflags -X`), default `0.5.4`.

### MCP

- `initialize` capabilities are `{"prompts":{},"resources":{},"tools":{}}` (go-sdk omits false
  fields); list results carry go-sdk's `ttlMs` / `cacheScope: "public"` (Python: `"private"`).
- Tool results omit `isError: false` (go-sdk); failed tools still return `isError: true`.
- Argument validation reimplements the pydantic lax subset the tools need with pydantic's error
  texts; integers beyond int64 fail with `int_parsing`.

### API

- `/docs`, `/docs/oauth2-redirect` and `/redoc` serve a static copy of the HTML captured from
  FastAPI 0.162.0. There is no access log.
- A non-ASCII `Authorization` header gets 401 (Python: 500 from `hmac.compare_digest`).


## Prototype (go/cmd/cdpprobe)

The original throwaway experiment now lives in `go/cmd/cdpprobe`. Paths below are relative to it.

Throwaway prototype: a static Go binary (go-rod, plain CDP, no Node, no patchright)
drives the cloakbrowser Chromium binary. Question: does it still pass bot checks?

### Build and run

```bash
cd cmd/cdpprobe
CGO_ENABLED=0 go build -ldflags="-s -w" -o go-cdp .
./run.sh out-win -platform windows          # all sites, one variant
./run.sh out-x -url https://example.com/    # single URL
./runall.sh                                 # 4 variants: windows/linux x stealth off/on
./cfloop.sh                                 # Cloudflare only, 3 runs per variant
```

`run.sh` mounts the binary into `parsers-autobrowser:latest` and runs it as the host user.
Flags: `-bin -platform windows|linux -headless -stealth -out -url -runtime-enable`.
Each site produces `<name>.png` (full page), `<name>.html`, `<name>.json`
(final URL, title, `navigator.webdriver`, UA, Cloudflare result).

Launch flags: rod defaults minus `--enable-automation`, plus
`--no-sandbox --fingerprint=<rand> --fingerprint-platform=<platform>`.

### Results (2026-11-22, headless, inside Docker)

| Site | windows | linux | windows + stealth | linux + stealth |
|---|---|---|---|---|
| browserscan bot-detection (incl. CDP) | Normal | Normal | Normal | Normal |
| BotD | not a bot | not a bot | not a bot | not a bot |
| creepjs headless / stealth | 0% / 0% | 0% / 0% | 0% / 80% | 0% / 80% |
| creepjs like-headless | 25% | 44% | 25% | 44% |
| navigator.webdriver | false | false | false | false |
| sannysoft | ERR_TIMED_OUT | ERR_TIMED_OUT | ERR_TIMED_OUT | ERR_TIMED_OUT |
| areyouheadless | site returns 502 | 502 | 502 | 502 |
| criptonoticias (Cloudflare), passed/runs | 4/4 | 3/4 | 3/4 | 1/4 |

Notes:

- sannysoft did not load; the Python client (`autobrowser run-script`) timed out
  on it the same day, so it is a site/network issue. areyouheadless returns nginx 502.
- go-rod/stealth hurts: creepjs flags it (`hasIframeProxy`), Cloudflare blocks more often.
  The stealth comes from the cloakbrowser binary itself, not from the client.
- Cloudflare blocks, when they happen, end on the Turnstile checkbox; one click
  on the widget did not help within 30 s.
- rod applies a default device (Mac, Chrome/114 UA) to every page. It must be disabled
  with `NoDefaultDevice()`, otherwise the UA contradicts the fingerprint.

### Runtime.enable risk

go-rod does not call `Runtime.enable` explicitly (it uses `Page.enable`, `DOM.enable`,
`Runtime.evaluate`/`callFunctionOn`). browserscan's CDP check reported Normal.
The bundled probe (Error `stack` getter + `console.debug`) reported "not detected"
even with `-runtime-enable`, so that probe does not work on Chrome 146 and proves nothing.
The risk is therefore **not verified**, only "not detected by browserscan".

### Size and startup

| | go-cdp | PyInstaller `autobrowser` |
|---|---|---|
| binary size | 11 MB | 77 MB |
| `-h` / `--help` | 0.004 s | ~1.0 s |

Chromium launch through go-rod: ~110 ms to a ready CDP connection.
