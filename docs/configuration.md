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
  engine: patchright   # patchright (default) or cloakbrowser
  launchArgs: []
  userAgent: Mozilla/5.0
  viewport:
    width: 1280
    height: 720

profiles:
  dir: /data/profiles

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
AUTOBROWSER_BROWSER_ENGINE=patchright
AUTOBROWSER_USER_AGENT=Mozilla/5.0
AUTOBROWSER_VIEWPORT_WIDTH=1280
AUTOBROWSER_VIEWPORT_HEIGHT=720
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

### Browser engines

`browser.engine` selects the stealth engine. Allowed values: `patchright` (default) | `cloakbrowser`.

| Engine | Stealth method | Chromium | Hard dep |
|---|---|---|---|
| `patchright` | JS injection via init scripts, uses system Chrome (`channel="chrome"`) | system | yes |
| `cloakbrowser` | C++ level patches, bundled binary auto-downloaded on first use | bundled | no (optional) |

Both engines support profiles, proxy, fingerprint injection, viewport, userAgent, all CLI commands, and all API endpoints.

To use cloakbrowser, install the optional dependency first:

```bash
pip install "autobrowser-client[cloak]"
# or: pip install cloakbrowser
```

Engine is a process-wide setting (config file or env var `AUTOBROWSER_BROWSER_ENGINE`). It cannot be changed per session or per API request.

### Platform spoofing

When `userAgent` contains a non-Linux OS identifier (e.g. Windows or macOS), the browser automatically adjusts `navigator.platform`, `navigator.userAgentData.platform`, and Client Hints (Sec-CH-UA-Platform) to match. This prevents antibot systems from detecting a mismatch between the UA string and JavaScript platform properties.

Supported platforms:
- **Windows**: `navigator.platform` = `Win32`, Client Hints platform = `Windows`
- **macOS**: `navigator.platform` = `MacIntel`, Client Hints platform = `macOS`
- **Android**: `navigator.platform` = `Linux armv8l`, Client Hints platform = `Android`, device model extracted from UA
- **Linux**: no spoofing needed (native OS)

For mobile User-Agent strings (Android), the browser context is automatically configured with `is_mobile: true` and `has_touch: true`. This affects viewport behavior and enables touch event emulation.

Example — pretend to be a Windows machine:

```yaml
session:
  userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
```

Example — pretend to be an Android device:

```yaml
session:
  userAgent: "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Mobile Safari/537.36"
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

#### Browser engines in Docker

The Docker image ships with **both** engines pre-installed and their binaries pre-downloaded:

| Engine | Binary location | Enabled by default |
|---|---|---|
| `patchright` | `/opt/playwright-browsers/` | yes |
| `cloakbrowser` | `/home/browser/.cloakbrowser/` | available, opt-in |

The `socks2http` relay is also pre-installed at `/usr/local/bin/socks2http`. SOCKS5 proxy works out of the box — no additional setup needed.

Switch engines at runtime — no rebuild needed:

```bash
# use cloakbrowser
docker run --rm -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser ...

# use patchright (default)
docker run --rm autobrowser ...
```

To build a slim image without cloakbrowser:

```bash
docker build --build-arg INSTALL_CLOAKBROWSER=0 -t autobrowser-slim .
```

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
  engine: patchright   # patchright (default) или cloakbrowser
  launchArgs: []
  userAgent: Mozilla/5.0
  viewport:
    width: 1280
    height: 720

profiles:
  dir: /data/profiles

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
AUTOBROWSER_BROWSER_ENGINE=patchright
AUTOBROWSER_USER_AGENT=Mozilla/5.0
AUTOBROWSER_VIEWPORT_WIDTH=1280
AUTOBROWSER_VIEWPORT_HEIGHT=720
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

### Движки браузера

`browser.engine` выбирает движок. Допустимые значения: `patchright` (по умолчанию) | `cloakbrowser`.

| Движок | Метод стелс | Chromium | Обяз. зависимость |
|---|---|---|---|
| `patchright` | JS-инъекции через init-скрипты, системный Chrome (`channel="chrome"`) | системный | да |
| `cloakbrowser` | Патчи на уровне C++, бандлированный бинарник (автозагрузка при первом запуске) | встроенный | нет (опционально) |

Оба движка поддерживают профили, прокси, инъекцию fingerprint, viewport, userAgent, все CLI-команды и все API-endpoints.

Для использования cloakbrowser сначала установите опциональную зависимость:

```bash
pip install "autobrowser-client[cloak]"
# или: pip install cloakbrowser
```

Движок — это настройка уровня процесса (конфиг-файл или переменная окружения `AUTOBROWSER_BROWSER_ENGINE`). Его нельзя изменить на уровне сессии или API-запроса.

### Подмена платформы (platform spoofing)

Если `userAgent` содержит идентификатор другой ОС (например, Windows или macOS), браузер автоматически подменяет `navigator.platform`, `navigator.userAgentData.platform` и Client Hints (Sec-CH-UA-Platform), чтобы они соответствовали UA-строке. Это предотвращает обнаружение несоответствия антибот-системами.

Поддерживаемые платформы:
- **Windows**: `navigator.platform` = `Win32`, Client Hints = `Windows`
- **macOS**: `navigator.platform` = `MacIntel`, Client Hints = `macOS`
- **Android**: `navigator.platform` = `Linux armv8l`, Client Hints = `Android`, модель устройства извлекается из UA
- **Linux**: подмена не нужна (родная ОС)

Для мобильных User-Agent строк (Android) контекст браузера автоматически настраивается с `is_mobile: true` и `has_touch: true`. Это влияет на поведение viewport и включает эмуляцию тач-событий.

Пример — притвориться Windows-машиной:

```yaml
session:
  userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
```

Пример — притвориться Android-устройством:

```yaml
session:
  userAgent: "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Mobile Safari/537.36"
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

#### Движки браузера в Docker

Docker-образ поставляется с **обоими** движками: бинарники скачиваются при сборке образа.

| Движок | Расположение бинарника | По умолчанию |
|---|---|---|
| `patchright` | `/opt/playwright-browsers/` | да |
| `cloakbrowser` | `/home/browser/.cloakbrowser/` | доступен, включается явно |

Relay `socks2http` также предустановлен по пути `/usr/local/bin/socks2http`. SOCKS5-прокси работает из коробки без дополнительной настройки.

Переключить движок можно в рантайме без пересборки образа:

```bash
# использовать cloakbrowser
docker run --rm -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser ...

# использовать patchright (по умолчанию)
docker run --rm autobrowser ...
```

Для сборки компактного образа без cloakbrowser:

```bash
docker build --build-arg INSTALL_CLOAKBROWSER=0 -t autobrowser-slim .
```
