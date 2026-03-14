# autobrowser-client

Python client for launching and controlling a browser through CLI scripts and HTTP API using patchright.

Проект предназначен для запуска браузера с профилями, прокси, пользовательским `User-Agent`, получением cookies, headers и content, а также для интерактивного браузинга через CLI и HTTP API.

## Documentation

- Technical specification / Техническое задание: [`tz.md`](tz.md)
- HTTP API / HTTP API: [`docs/api.md`](docs/api.md)
- Configuration / Конфигурация: [`docs/configuration.md`](docs/configuration.md)
- CLI scripts / CLI-сценарии: [`docs/scripts.md`](docs/scripts.md)
- Runtime (profiles, proxy, storage) / Рантайм: [`docs/runtime.md`](docs/runtime.md)
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

Common flags: `--config`, `--profile`, `--proxy`, `--viewport`, `--output`, `--headless/--headful`, `--timeout`.

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
