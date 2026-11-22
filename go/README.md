# autobrowser (Go)

Go port of the Python `autobrowser` client (see `../python`). It supports only the
cloakbrowser engine (plain CDP via go-rod, no Node, no patchright). Status: stage 1 of 5
(core, scenarios, basic actions). Static binary, no runtime dependencies.

## Build

```bash
cd go && CGO_ENABLED=0 go build -o autobrowser ./cmd/autobrowser
```

## Usage

```bash
autobrowser version
autobrowser show-config -c config.yaml
autobrowser run-script -f file.yaml [--var K=V] [-c config.yaml] [-p profile] \
    [--proxy URL] [--viewport WxH] [-o result.json] [--preserve-session] [--artifacts DIR]
```

`--var` is repeatable. The result JSON goes to stdout or to the `-o` file.

## Browser binary lookup

The cloakbrowser Chromium is taken from `CLOAKBROWSER_BINARY_PATH`, otherwise from the newest
`~/.cloakbrowser/chromium-*/chrome`. There is no automatic download yet: install the binary
with the Python client or set the variable.

## Parity with the Python client

| Feature | Python | Go |
|---|---|---|
| Config file, env, defaults, show-config | done | done |
| Scenario YAML, vars, `${}` templating, include | done | done |
| if / try / forEach / repeat / while, retry, optional, stopOnError | done | done |
| assert / set / fail, result JSON | done | done |
| Profiles, lock, preserveSession (`tabs.json`) | done | done |
| Actions: open, wait, find, find-all, click, dblclick, hover, focus, fill, clear, check, uncheck, type, press, scroll, screenshot, get-content, save, evaluate, history, wait-text, extract | done | done |
| CLI: version, show-config, run-script | done | done |
| Proxy | done (relay with auth) | partial (http/socks5 without credentials) |
| Locators | done | partial (`text` approximates get_by_text, `ref` unsupported) |
| Regex conditions | done | partial (RE2: no lookahead or backreferences) |
| Headless, viewport, userAgent | done | partial (mobile emulation via UA override only) |
| patchright engine | done | missing |
| geoip | done | missing |
| JS fingerprint injection | done | missing (`session.fingerprint` ignored with a warning) |
| Actions: get-cookies, get-headers, solve-challenge, browse-tabs, submit, select, upload, read, mouse-move, mouse-scroll-to, mouse-random | done | missing |
| Binary download | done | missing |
| MCP server | done | missing |
| HTTP API | done | missing |
| Other CLI commands | done | missing |
| Release binaries | done | missing |

## Python vs Go live comparison

The examples `conditions`, `try-catch`, `extract-loop`, `screenshot` and a custom form scenario
gave identical results in both clients (ignoring `durationMs` and `sessionId`). `simple.yaml`
differs because `get-headers` and `get-cookies` are missing in Go.

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
