# autobrowser-client

Python client for launching and controlling a browser through CLI scripts and HTTP API using patchright.

Проект предназначен для запуска браузера с профилями, прокси, пользовательским `User-Agent`, получением cookies, headers и content, а также для интерактивного браузинга через CLI и HTTP API.

## Documentation

- Technical specification / Техническое задание: [`tz.md`](tz.md)
- HTTP API / HTTP API: [`docs/api.md`](docs/api.md)
- Configuration / Конфигурация: [`docs/configuration.md`](docs/configuration.md)
- CLI scripts / CLI-сценарии: [`docs/scripts.md`](docs/scripts.md)

## Quick start

```bash
docker compose build cli
docker compose run --rm cli version
docker compose run --rm cli show-config
```
