# Configuration / Конфигурация

## English

### Purpose

This document describes how `autobrowser-client` should be configured for CLI usage, HTTP API usage, Docker runtime, and script execution.

### Configuration sources

The project should support these configuration sources:

- CLI arguments
- environment variables
- configuration file in `YAML` or `JSON`
- built-in default values

Priority order:

1. CLI arguments
2. environment variables
3. configuration file
4. default values

### Supported configuration areas

The configuration should cover at least:

- browser launch mode
- profile storage
- HTTP API host and port
- timeouts
- logging
- proxy settings
- output limits
- authentication token for API
- session lifetime and limits

### Recommended configuration file

Example `config.yaml`:

```yaml
browser:
  headless: true
  launchArgs: []
  userAgent: Mozilla/5.0

profiles:
  dir: /data/profiles
  reuse: true

api:
  host: 127.0.0.1
  port: 8080
  token: ""
  maxSessions: 10

timeouts:
  startupMs: 30000
  actionMs: 10000
  navigationMs: 30000
  idleSessionMs: 300000

proxy:
  enabled: false
  scheme: socks5
  host: 127.0.0.1
  port: 1080
  username: ""
  password: ""
  socks2httpBin: ""       # path to socks2http binary, auto-detected via PATH if empty
  localPort: 0            # 0 = auto-select free port

logging:
  level: INFO
  file: ""
  maskSecrets: true

output:
  maxContentBytes: 1048576
  contentFormat: html
```

### Environment variables

All supported environment variables:

```env
AUTOBROWSER_HEADLESS=true
AUTOBROWSER_USER_AGENT=Mozilla/5.0
AUTOBROWSER_PROFILE_DIR=/data/profiles
AUTOBROWSER_API_HOST=127.0.0.1
AUTOBROWSER_API_PORT=8080
AUTOBROWSER_API_TOKEN=
AUTOBROWSER_API_MAX_SESSIONS=10
AUTOBROWSER_LOG_LEVEL=INFO
AUTOBROWSER_LOG_FILE=
AUTOBROWSER_PROXY_ENABLED=false
AUTOBROWSER_PROXY_SCHEME=socks5
AUTOBROWSER_PROXY_HOST=127.0.0.1
AUTOBROWSER_PROXY_PORT=1080
AUTOBROWSER_PROXY_USERNAME=
AUTOBROWSER_PROXY_PASSWORD=
AUTOBROWSER_SOCKS2HTTP_BIN=
AUTOBROWSER_PROXY_LOCAL_PORT=0
AUTOBROWSER_TIMEOUT_STARTUP_MS=30000
AUTOBROWSER_TIMEOUT_ACTION_MS=10000
AUTOBROWSER_TIMEOUT_NAVIGATION_MS=30000
AUTOBROWSER_SESSION_IDLE_MS=300000
AUTOBROWSER_MAX_CONTENT_BYTES=1048576
AUTOBROWSER_CONTENT_FORMAT=html
```

### CLI override examples

Examples of runtime override:

```bash
autobrowser run --url https://example.com --headless --timeout 10000
```

```bash
autobrowser run-script --file scenario.yaml --profile test1
```

### Rules for secrets

- API tokens, proxy passwords, and sensitive headers must not be printed in full in logs.
- Secret values may be passed through environment variables.
- Example files should use empty placeholders instead of real secrets.

### Validation requirements

Configuration validation should check:

- valid host and port values
- positive timeout values
- existing profile directory or ability to create it
- valid logging level
- consistent proxy settings
- content size limits

### Docker notes

When running in Docker:

- configuration should be passable through environment variables;
- profile storage should be mounted as a volume;
- API host and port should be configurable;
- the default container base is `Ubuntu 24.04`.

## Русский

### Назначение

Этот документ описывает, как должна настраиваться система `autobrowser-client` для CLI, HTTP API, Docker и выполнения сценариев.

### Источники конфигурации

Проект должен поддерживать следующие источники конфигурации:

- аргументы CLI
- переменные окружения
- конфигурационный файл в формате `YAML` или `JSON`
- встроенные значения по умолчанию

Порядок приоритета:

1. аргументы CLI
2. переменные окружения
3. конфигурационный файл
4. значения по умолчанию

### Поддерживаемые области конфигурации

Конфигурация должна покрывать как минимум:

- режим запуска браузера
- хранение профилей
- host и port HTTP API
- таймауты
- логирование
- параметры прокси
- ограничения вывода
- токен аутентификации API
- время жизни и лимиты сессий

### Рекомендуемый конфигурационный файл

Пример `config.yaml`:

```yaml
browser:
  headless: true
  launchArgs: []
  userAgent: Mozilla/5.0

profiles:
  dir: /data/profiles
  reuse: true

api:
  host: 127.0.0.1
  port: 8080
  token: ""
  maxSessions: 10

timeouts:
  startupMs: 30000
  actionMs: 10000
  navigationMs: 30000
  idleSessionMs: 300000

proxy:
  enabled: false
  scheme: socks5
  host: 127.0.0.1
  port: 1080
  username: ""
  password: ""
  socks2httpBin: ""       # путь к бинарнику socks2http, автоопределение через PATH если пусто
  localPort: 0            # 0 = автовыбор свободного порта

logging:
  level: INFO
  file: ""
  maskSecrets: true

output:
  maxContentBytes: 1048576
  contentFormat: html
```

### Переменные окружения

Все поддерживаемые переменные окружения:

```env
AUTOBROWSER_HEADLESS=true
AUTOBROWSER_USER_AGENT=Mozilla/5.0
AUTOBROWSER_PROFILE_DIR=/data/profiles
AUTOBROWSER_API_HOST=127.0.0.1
AUTOBROWSER_API_PORT=8080
AUTOBROWSER_API_TOKEN=
AUTOBROWSER_API_MAX_SESSIONS=10
AUTOBROWSER_LOG_LEVEL=INFO
AUTOBROWSER_LOG_FILE=
AUTOBROWSER_PROXY_ENABLED=false
AUTOBROWSER_PROXY_SCHEME=socks5
AUTOBROWSER_PROXY_HOST=127.0.0.1
AUTOBROWSER_PROXY_PORT=1080
AUTOBROWSER_PROXY_USERNAME=
AUTOBROWSER_PROXY_PASSWORD=
AUTOBROWSER_SOCKS2HTTP_BIN=
AUTOBROWSER_PROXY_LOCAL_PORT=0
AUTOBROWSER_TIMEOUT_STARTUP_MS=30000
AUTOBROWSER_TIMEOUT_ACTION_MS=10000
AUTOBROWSER_TIMEOUT_NAVIGATION_MS=30000
AUTOBROWSER_SESSION_IDLE_MS=300000
AUTOBROWSER_MAX_CONTENT_BYTES=1048576
AUTOBROWSER_CONTENT_FORMAT=html
```

### Примеры переопределения через CLI

Примеры переопределения параметров во время запуска:

```bash
autobrowser run --url https://example.com --headless --timeout 10000
```

```bash
autobrowser run-script --file scenario.yaml --profile test1
```

### Правила для секретов

- Токены API, пароли прокси и чувствительные заголовки не должны выводиться в логах полностью.
- Секретные значения могут передаваться через переменные окружения.
- В примерах и шаблонах должны использоваться пустые значения или заглушки вместо реальных секретов.

### Требования к валидации

Проверка конфигурации должна контролировать:

- корректность host и port;
- положительные значения таймаутов;
- существование директории профилей или возможность ее создать;
- допустимый уровень логирования;
- согласованность настроек прокси;
- ограничения на размер сохраняемого содержимого.

### Примечания для Docker

При запуске в Docker:

- конфигурация должна передаваться через переменные окружения;
- хранилище профилей должно подключаться как volume;
- host и port API должны настраиваться;
- базовый образ контейнера по умолчанию - `Ubuntu 24.04`.
