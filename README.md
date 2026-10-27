# autobrowser-client

Python client for launching and controlling a browser through CLI scripts and HTTP API using Playwright-compatible stealth engines (patchright by default, optional CloakBrowser).

Проект предназначен для запуска браузера с профилями, прокси, пользовательским `User-Agent`, получением cookies, headers и content, а также для интерактивного браузинга через CLI и HTTP API.

## Quick install for AI agents / Быстрая установка через AI-агента

Give your agent this link / Дайте агенту эту ссылку:

```
Install autobrowser by following https://github.com/foxzi/autobrowser-client/blob/main/docs/install-agent.md
```

## Documentation

- Technical specification / Техническое задание: [`tz.md`](tz.md)
- MCP server for AI agents / MCP-сервер для AI-агентов: [`docs/mcp.md`](docs/mcp.md)
- Agent skill / Skill для агента: [`skills/autobrowser/SKILL.md`](skills/autobrowser/SKILL.md)
- Install via an AI agent / Установка через AI-агента: [`docs/install-agent.md`](docs/install-agent.md)
- HTTP API / HTTP API: [`docs/api.md`](docs/api.md)
- Configuration / Конфигурация: [`docs/configuration.md`](docs/configuration.md)
- CLI scripts / CLI-сценарии: [`docs/scripts.md`](docs/scripts.md)
- Runtime (profiles, proxy, storage) / Рантайм: [`docs/runtime.md`](docs/runtime.md)
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

### Docker (recommended)

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

See [`docs/scripts.md`](docs/scripts.md) for full syntax reference including locator shorthands and all available actions.
