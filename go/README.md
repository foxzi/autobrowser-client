# go-cdp experiment

Throwaway prototype: a static Go binary (go-rod, plain CDP, no Node, no patchright)
drives the cloakbrowser Chromium binary. Question: does it still pass bot checks?

## Build and run

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

## Results (2026-11-22, headless, inside Docker)

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

## Runtime.enable risk

go-rod does not call `Runtime.enable` explicitly (it uses `Page.enable`, `DOM.enable`,
`Runtime.evaluate`/`callFunctionOn`). browserscan's CDP check reported Normal.
The bundled probe (Error `stack` getter + `console.debug`) reported "not detected"
even with `-runtime-enable`, so that probe does not work on Chrome 146 and proves nothing.
The risk is therefore **not verified**, only "not detected by browserscan".

## Size and startup

| | go-cdp | PyInstaller `autobrowser` |
|---|---|---|
| binary size | 11 MB | 77 MB |
| `-h` / `--help` | 0.004 s | ~1.0 s |

Chromium launch through go-rod: ~110 ms to a ready CDP connection.
