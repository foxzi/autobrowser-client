# autobrowser-client

A stealth browser for scripts and AI agents. It runs Chromium through Playwright-compatible stealth engines (`patchright` by default, optional `cloakbrowser`) and adds what plain Playwright lacks: persistent profiles, consistent fingerprints, authenticated SOCKS5 proxies, Cloudflare challenge handling, YAML scenarios, an HTTP API and an MCP server. Ships as a single binary that updates itself.

Стелс-браузер для сценариев и AI-агентов. Запускает Chromium через совместимые с Playwright стелс-движки (`patchright` по умолчанию, опционально `cloakbrowser`) и добавляет то, чего нет в обычном Playwright: постоянные профили, стабильные отпечатки, SOCKS5-прокси с авторизацией, прохождение проверок Cloudflare, YAML-сценарии, HTTP API и MCP-сервер. Поставляется одним бинарём, который обновляется сам.

## Quick install for AI agents / Быстрая установка через AI-агента

Give your agent this link / Дайте агенту эту ссылку:

```
Install autobrowser by following https://github.com/foxzi/autobrowser-client/blob/main/docs/install-agent.md
```

## Why not just Playwright / Чем отличается от Playwright

| | Playwright / Playwright MCP | autobrowser |
|---|---|---|
| Automation detection | Default build is visible to bot checks (`navigator.webdriver`, CDP traces) | Stealth engines: `patchright` (patched driver, system Chrome) or `cloakbrowser` (patches at the C++ level) |
| Cloudflare / Turnstile | Not handled | `solve-challenge` step and `browser_solve_challenge` tool wait for the challenge and click Turnstile |
| Fingerprint | Same as the host | Per-profile Canvas, WebGL, Audio, ClientRects and font noise, deterministic by seed; platform properties and Client Hints follow the User-Agent |
| Profiles | You manage `user_data_dir` yourself | Named persistent profiles with cookies, storage, fingerprint and optional restore of open tabs |
| Proxy | Chromium cannot use SOCKS5 with a login and password | SOCKS5/HTTP with auth through a built-in local relay; credentials masked in logs |
| No-code automation | Write code | YAML/JSON scenarios with shorthand steps, validation without a browser |
| Remote control | Library only | HTTP API with sessions, tokens, limits and idle timeout |
| AI agents | Playwright MCP | MCP server with `browser_*` tools on top of the stealth engine, plus a ready skill for the agent |
| Human-like input | Manual | Typing with delays, random mouse movement, tab browsing with scrolling |
| Delivery | Python/Node package and browsers | Single binary for Linux, macOS and Windows with self-update; Docker images |

| | Playwright / Playwright MCP | autobrowser |
|---|---|---|
| Детект автоматизации | Обычная сборка видна проверкам на ботов (`navigator.webdriver`, следы CDP) | Стелс-движки: `patchright` (пропатченный драйвер, системный Chrome) или `cloakbrowser` (патчи на уровне C++) |
| Cloudflare / Turnstile | Не обрабатывается | Шаг `solve-challenge` и инструмент `browser_solve_challenge` ждут проверку и нажимают Turnstile |
| Отпечаток | Как у хоста | Свой для каждого профиля шум Canvas, WebGL, Audio, ClientRects и шрифтов, стабильный по seed; платформа и Client Hints подстраиваются под User-Agent |
| Профили | `user_data_dir` нужно вести самому | Именованные постоянные профили: cookies, storage, отпечаток и восстановление открытых вкладок |
| Прокси | Chromium не умеет SOCKS5 с логином и паролем | SOCKS5/HTTP с авторизацией через встроенный локальный relay; пароли скрыты в логах |
| Автоматизация без кода | Нужно писать код | YAML/JSON-сценарии с короткой записью шагов и проверкой без запуска браузера |
| Удалённое управление | Только библиотека | HTTP API с сессиями, токеном, лимитами и таймаутом простоя |
| AI-агенты | Playwright MCP | MCP-сервер с инструментами `browser_*` поверх стелс-движка и готовый skill для агента |
| Человекоподобный ввод | Вручную | Набор текста с задержками, случайные движения мыши, обход вкладок с прокруткой |
| Поставка | Пакет Python/Node и браузеры | Один бинарь для Linux, macOS и Windows с самообновлением; Docker-образы |

## Features

### Stealth engines
- `patchright` (default): patched Playwright driver running the system Google Chrome.
- `cloakbrowser`: its own Chromium build with source-level patches, downloaded by `autobrowser install`.
- Both engines support every CLI command, API endpoint and MCP tool; switch with `AUTOBROWSER_BROWSER_ENGINE` or `browser.engine` in the config.
- `fingerprintPlatform` / `AUTOBROWSER_FINGERPRINT_PLATFORM` picks the platform cloakbrowser presents.

### Profiles and sessions
- Named profiles keep cookies, localStorage and session data between runs.
- `--preserve-session` saves open tabs to `tabs.json` and restores them on the next start.
- Sessions close themselves after `timeouts.idleSessionMs` of inactivity (5 minutes by default).

### Fingerprints
- `fingerprint: true` creates `fingerprint.json` in the profile: Canvas, WebGL vendor/renderer, AudioContext, ClientRects and font metrics get seeded noise, so a profile looks the same on every run and different from other profiles.
- GPU strings are chosen to match the User-Agent (Windows, Linux, Android).
- A non-Linux User-Agent also changes `navigator.platform`, `userAgentData` and `Sec-CH-UA-Platform`; Android UAs turn on mobile and touch mode.
- `autobrowser generate-fingerprint` creates a fingerprint ahead of time; scenarios can override single fields.

### Proxy
- `socks5://user:pass@host:port` or HTTP proxies, per run (`--proxy`), per scenario or in the config.
- A local relay is started for the session and stopped with it, which makes authenticated SOCKS5 work in Chromium: `socks2http` if it is installed, otherwise a built-in Python relay (nothing to install).
- `geoip` / `AUTOBROWSER_GEOIP` sets the browser timezone and language from the exit IP, so they match the proxy country.
- Proxy passwords and sensitive headers (`authorization`, `cookie`, `set-cookie`, `proxy-authorization`) are masked in output and logs.

### YAML scenarios
- Steps: `open`, `wait`, `click`, `fill`, `type`, `press`, `scroll`, `find`, `screenshot`, `get-content`, `get-cookies`, `get-headers`, `browse-tabs`, `solve-challenge`.
- Short form: `- open: https://example.com`, `- click: "text=Sign in"`; locators by CSS, XPath, text, id, name, class or attribute.
- More actions: `hover`, `select`, `upload`, `evaluate`, `extract`, `wait-text`, `history`, `set`, `assert`, `fail` and others.
- Variables `${name}` / `${env.NAME}` (`vars:` in the file, `--var NAME=VALUE`), conditions, and blocks `if`, `try`/`catch`/`finally`, `forEach`, `repeat`, `while`, `include`; step options `retry`, `optional`; `--artifacts DIR` saves a screenshot and HTML of failed steps.
- `autobrowser validate-script` checks a scenario without a browser; the result is JSON with per-step status, exit code 1 on failure.
- The same scenarios run over MCP (`browser_run_script`) and HTTP (`POST /api/v1/session/{id}/script`).

### HTTP API
- `autobrowser serve`: sessions, tabs, navigation, history, snapshots, screenshots, element actions (find, read, click, hover, fill, select, check, upload, submit), mouse and keyboard, JavaScript evaluation, console/network logs, dialog policy, profiles.
- Bearer token (`AUTOBROWSER_API_TOKEN`), session limit, health and readiness checks.

### MCP server for AI agents
- `autobrowser mcp` (stdio) gives the agent 18 tools: navigate, history, snapshot, screenshot, click, hover, type, press key, select, file upload, evaluate, wait for text, get text, tabs, logs, dialogs, solve challenge, close.
- The snapshot returns `ref` ids for elements, so the agent clicks by reference instead of guessing selectors.
- `autobrowser install-skill` installs a skill that teaches the agent the snapshot/ref loop, waits, Cloudflare handling and safety rules.

### Delivery
- Single binary with no Python required; releases for Linux amd64/arm64, macOS arm64 and Windows amd64 with sha256 checksums.
- Self-update: `mcp` and `serve` check for a new release once a day; `autobrowser update` updates right away, `AUTOBROWSER_NO_AUTO_UPDATE=1` turns it off.
- Docker images for the CLI and the API; also installable as a Python package.

## Возможности

### Стелс-движки
- `patchright` (по умолчанию): пропатченный драйвер Playwright с системным Google Chrome.
- `cloakbrowser`: собственная сборка Chromium с патчами в исходниках, скачивается командой `autobrowser install`.
- Оба движка поддерживают все команды CLI, эндпоинты API и инструменты MCP; переключаются через `AUTOBROWSER_BROWSER_ENGINE` или `browser.engine` в конфиге.
- `fingerprintPlatform` / `AUTOBROWSER_FINGERPRINT_PLATFORM` задаёт платформу, которую показывает cloakbrowser.

### Профили и сессии
- Именованные профили хранят cookies, localStorage и данные сессии между запусками.
- `--preserve-session` сохраняет открытые вкладки в `tabs.json` и восстанавливает их при следующем запуске.
- Сессия закрывается сама после `timeouts.idleSessionMs` простоя (по умолчанию 5 минут).

### Отпечатки
- `fingerprint: true` создаёт в профиле `fingerprint.json`: Canvas, WebGL vendor/renderer, AudioContext, ClientRects и метрики шрифтов получают шум от seed, поэтому профиль одинаков при каждом запуске и отличается от других профилей.
- Строки GPU подбираются под User-Agent (Windows, Linux, Android).
- User-Agent не для Linux меняет `navigator.platform`, `userAgentData` и `Sec-CH-UA-Platform`; Android UA включает мобильный режим и touch.
- `autobrowser generate-fingerprint` создаёт отпечаток заранее; в сценарии можно переопределить отдельные поля.

### Прокси
- `socks5://user:pass@host:port` или HTTP-прокси: на запуск (`--proxy`), в сценарии или в конфиге.
- На время сессии запускается локальный relay, благодаря ему SOCKS5 с авторизацией работает в Chromium: `socks2http`, если он установлен, иначе встроенный relay на Python (ничего ставить не нужно).
- `geoip` / `AUTOBROWSER_GEOIP` задаёт часовой пояс и язык браузера по выходному IP, чтобы они совпадали со страной прокси.
- Пароли прокси и чувствительные заголовки (`authorization`, `cookie`, `set-cookie`, `proxy-authorization`) скрываются в выводе и логах.

### YAML-сценарии
- Шаги: `open`, `wait`, `click`, `fill`, `type`, `press`, `scroll`, `find`, `screenshot`, `get-content`, `get-cookies`, `get-headers`, `browse-tabs`, `solve-challenge`.
- Короткая запись: `- open: https://example.com`, `- click: "text=Sign in"`; локаторы по CSS, XPath, тексту, id, name, class или атрибуту.
- Другие действия: `hover`, `select`, `upload`, `evaluate`, `extract`, `wait-text`, `history`, `set`, `assert`, `fail` и др.
- Переменные `${name}` / `${env.NAME}` (`vars:` в файле, `--var NAME=VALUE`), условия и блоки `if`, `try`/`catch`/`finally`, `forEach`, `repeat`, `while`, `include`; опции шага `retry`, `optional`; `--artifacts DIR` сохраняет скриншот и HTML упавших шагов.
- Те же сценарии выполняются через MCP (`browser_run_script`) и HTTP (`POST /api/v1/session/{id}/script`).
- `autobrowser validate-script` проверяет сценарий без браузера; результат в JSON со статусом каждого шага, при ошибке код выхода 1.

### HTTP API
- `autobrowser serve`: сессии, вкладки, навигация, история, снапшоты, скриншоты, действия с элементами (find, read, click, hover, fill, select, check, upload, submit), мышь и клавиатура, выполнение JavaScript, логи консоли и сети, политика диалогов, профили.
- Bearer-токен (`AUTOBROWSER_API_TOKEN`), лимит сессий, проверки health и ready.

### MCP-сервер для AI-агентов
- `autobrowser mcp` (stdio) даёт агенту 18 инструментов: navigate, history, snapshot, screenshot, click, hover, type, press key, select, file upload, evaluate, wait for text, get text, tabs, logs, dialogs, solve challenge, close.
- Снапшот возвращает `ref` элементов, и агент кликает по ссылке, а не подбирает селекторы.
- `autobrowser install-skill` ставит skill, который учит агента циклу snapshot/ref, ожиданиям, работе с Cloudflare и правилам безопасности.

### Поставка
- Один бинарь без Python; релизы для Linux amd64/arm64, macOS arm64 и Windows amd64 с контрольными суммами sha256.
- Самообновление: `mcp` и `serve` раз в сутки проверяют новый релиз; `autobrowser update` обновляет сразу, `AUTOBROWSER_NO_AUTO_UPDATE=1` отключает.
- Docker-образы для CLI и API; также ставится как Python-пакет.

## Documentation

- Technical specification / Техническое задание: [`tz.md`](tz.md)
- MCP server for AI agents / MCP-сервер для AI-агентов: [`docs/mcp.md`](docs/mcp.md)
- Agent skill / Skill для агента: [`skills/autobrowser/SKILL.md`](skills/autobrowser/SKILL.md)
- Install via an AI agent / Установка через AI-агента: [`docs/install-agent.md`](docs/install-agent.md)
- HTTP API / HTTP API: [`docs/api.md`](docs/api.md)
- Configuration / Конфигурация: [`docs/configuration.md`](docs/configuration.md)
- CLI scripts / CLI-сценарии: [`docs/scripts.md`](docs/scripts.md)
- Scenario guide: variables, templates, JS, files / Руководство по сценариям: переменные, шаблоны, JS, файлы: [`docs/scenario-guide.md`](docs/scenario-guide.md)
- Runtime (profiles, proxy, fingerprints, storage) / Рантайм: [`docs/runtime.md`](docs/runtime.md)
- Troubleshooting (Cloudflare, proxy, engines) / Решение проблем: [`docs/troubleshooting.md`](docs/troubleshooting.md)
- Releases and versions / Релизы и версии: [`docs/releases.md`](docs/releases.md)
- Examples / Примеры: [`examples/README.md`](examples/README.md)

## CLI commands

| Command | Description |
|---------|-------------|
| `autobrowser version` | Show version |
| `autobrowser show-config` | Show effective configuration |
| `autobrowser validate-script --file scenario.yaml` | Validate scenario without running |
| `autobrowser run-script --file scenario.yaml` | Run YAML/JSON scenario |
| `autobrowser run --url https://example.com` | Open URL and collect data |
| `autobrowser serve` | Start HTTP API server |
| `autobrowser mcp` | Start MCP server (stdio) for AI agents |
| `autobrowser install` | Download the browser for the configured engine |
| `autobrowser install-skill [--dir DIR]` | Install the agent skill |
| `autobrowser update [--check]` | Update the binary to the latest release |
| `autobrowser generate-fingerprint --profile DIR` | Generate fingerprint.json for a profile |

Common flags: `--config`, `--profile`, `--proxy`, `--viewport`, `--output`, `--headless/--headful`, `--timeout`, `--preserve-session`.

Help: `autobrowser -h`, `autobrowser help <command>` or `autobrowser <command> -h` (examples and environment variables included). `autobrowser -V` prints the version.

## Browser engine

Two stealth engines are supported. Default is `patchright`.

| Engine | Stealth method | Chromium | Install |
|---|---|---|---|
| `patchright` (default) | JS injection patches, uses system Chrome (`channel="chrome"`) | system | built-in |
| `cloakbrowser` | C++ level patches, own binary (auto-downloaded on first use) | bundled | optional |

Switch to cloakbrowser:

```bash
# Install optional dependency
pip install "autobrowser-client[cloak]"
# or: pip install cloakbrowser

# Via env var
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run --url https://example.com
```

Or in `config.yaml`:

```yaml
browser:
  engine: cloakbrowser
```

Both engines support profiles, proxy, fingerprint injection, viewport, user_agent, all CLI commands, and all API endpoints.

## Quick start

### Binary

Download the binary for your platform from [Releases](https://github.com/foxzi/autobrowser-client/releases/latest), then:

```bash
autobrowser install                               # download the browser
autobrowser run --url https://example.com --format text
autobrowser run-script --file examples/simple.yaml
```

### Docker (recommended for servers)

```bash
docker compose build cli
docker compose run --rm cli version
docker compose run --rm cli show-config
docker compose run --rm cli run-script --file examples/simple.yaml
```

### Local venv (dev mode)

```bash
python3 -m venv .venv
.venv/bin/pip install -e .
.venv/bin/autobrowser version
.venv/bin/autobrowser show-config
.venv/bin/autobrowser run-script --file examples/simple.yaml
```

### HTTP API server

```bash
.venv/bin/autobrowser serve --host 0.0.0.0 --port 8080
# or via Docker
docker compose up api
```

### MCP server

```bash
claude mcp add autobrowser -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser -- autobrowser mcp
autobrowser install-skill
```

See [`docs/mcp.md`](docs/mcp.md) for other clients.

## Script example (shorthand syntax)

Scenarios support a compact shorthand: use the action name as the YAML key.

```yaml
steps:
  - open: https://example.com/login
  - wait: networkidle
  - fill: ["name=username", "demo-user"]
  - fill: ["name=password", "demo-password"]
  - click: "button[type='submit']"
  - wait: networkidle
  - screenshot: login.png
    fullPage: true
  - get-content: ~

output:
  file: result.html
```

Cloudflare-protected site with a proxy and a profile:

```yaml
session:
  profile: news
  proxy: socks5://user:pass@127.0.0.1:1080
  fingerprint: true
steps:
  - open: https://example.com
  - solve-challenge: ~
  - get-content: ~
```

See [`docs/scripts.md`](docs/scripts.md) for full syntax reference including locator shorthands and all available actions, and [`examples/`](examples/) for ready scenarios.
