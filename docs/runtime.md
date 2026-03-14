# Runtime: Profiles, Proxy, Storage / Профили, прокси, хранение данных

## English

### Profiles

Browser profiles store user data (cookies, localStorage, session storage) between runs.

#### Configuration

```yaml
profiles:
  dir: ".profiles"
  reuse: true
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `dir` | `.profiles` | Base directory for profile storage |
| `reuse` | `true` | Reuse existing profiles between runs |

Environment variable: `AUTOBROWSER_PROFILE_DIR`

#### Usage

**CLI:**

```bash
autobrowser run --url https://example.com --profile myprofile
```

**API:**

```json
POST /api/v1/browser/start
{
  "urls": ["https://example.com"],
  "profile": "myprofile"
}
```

When a profile name is specified, the browser uses `launch_persistent_context` with the profile directory. This preserves:

- Cookies
- localStorage and sessionStorage
- IndexedDB data
- Browser cache
- Saved passwords and form data

Profile data is stored in `{profiles.dir}/{profile_name}/`.

#### Profile lifecycle

1. If the profile directory does not exist, it is created automatically.
2. On subsequent runs with the same profile name, all stored data is restored.
3. Profile data persists until manually deleted from the filesystem.
4. When a profile is in use, a `profile.lock` file with the PID is created. Attempting to use a locked profile raises `ProfileLockedError`.
5. Stale lock files (where the PID no longer exists) are cleaned up automatically.

#### Profile management via API

- `POST /api/v1/profile/create` - create a new profile directory
- `GET /api/v1/profiles` - list all profiles

### Proxy

The project supports SOCKS5 and HTTP proxies via the `socks2http` relay.

#### Architecture

```
Browser --> patchright (proxy=http://127.0.0.1:PORT) --> socks2http --> upstream SOCKS5/HTTP --> target
```

`socks2http` is started as a subprocess that:
- Listens on a random free local port
- Converts HTTP proxy protocol to SOCKS5 upstream
- Shuts down automatically when the browser session ends

#### Configuration

```yaml
proxy:
  enabled: true
  scheme: socks5
  host: proxy.example.com
  port: 1080
  username: user
  password: pass
  socks2httpBin: /path/to/socks2http
  localPort: 0
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `enabled` | `false` | Enable proxy support |
| `scheme` | `socks5` | Upstream protocol (`socks5` or `http`) |
| `host` | - | Upstream proxy hostname |
| `port` | `1080`/`8080` | Upstream proxy port (default depends on scheme) |
| `username` | - | Proxy authentication username |
| `password` | - | Proxy authentication password |
| `socks2httpBin` | (auto) | Path to `socks2http` binary |
| `localPort` | `0` | Local relay port (`0` = auto-detect free port) |

Environment variables:

- `AUTOBROWSER_PROXY_ENABLED`
- `AUTOBROWSER_PROXY_SCHEME`
- `AUTOBROWSER_PROXY_HOST`
- `AUTOBROWSER_PROXY_PORT`
- `AUTOBROWSER_PROXY_USERNAME`
- `AUTOBROWSER_PROXY_PASSWORD`
- `AUTOBROWSER_SOCKS2HTTP_BIN`
- `AUTOBROWSER_PROXY_LOCAL_PORT`

#### CLI usage

```bash
autobrowser run --url https://example.com --proxy socks5://user:pass@host:1080
```

The `--proxy` flag accepts a URL in the format `scheme://user:pass@host:port`.

Proxy can also be specified directly in YAML scenario files:

```yaml
session:
  proxy: socks5://user:pass@host:port
```

> The CLI `--proxy` flag takes priority over the scenario value.

#### Proxy readiness check

Before starting the browser, the relay waits up to 5 seconds for the local port to accept connections. If the relay fails to start, a `ProxyRelayError` is raised.

#### Security

- Proxy credentials are masked in logs and `safe_dump()` output
- Password appears as `****` in configuration dumps

### Data storage

#### Content output

Page content is limited by `output.maxContentBytes` (default: 1 MB).

```yaml
output:
  maxContentBytes: 1048576
  contentFormat: html
```

Environment variables:

- `AUTOBROWSER_MAX_CONTENT_BYTES`
- `AUTOBROWSER_CONTENT_FORMAT`

#### Header masking

Sensitive HTTP headers (`authorization`, `cookie`, `set-cookie`, `proxy-authorization`) are automatically masked in API responses and CLI output.

#### Session idle timeout

Sessions are automatically terminated after `timeouts.idleSessionMs` (default: 300000 ms = 5 minutes) of inactivity. Every API request to a session resets the idle timer.

```yaml
timeouts:
  idleSessionMs: 300000
```

Environment variable: `AUTOBROWSER_SESSION_IDLE_MS`

---

## Русский

### Профили

Браузерные профили сохраняют пользовательские данные (cookies, localStorage, session storage) между запусками.

#### Конфигурация

```yaml
profiles:
  dir: ".profiles"
  reuse: true
```

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `dir` | `.profiles` | Базовая директория для хранения профилей |
| `reuse` | `true` | Повторное использование профилей между запусками |

Переменная окружения: `AUTOBROWSER_PROFILE_DIR`

#### Использование

**CLI:**

```bash
autobrowser run --url https://example.com --profile myprofile
```

**API:**

```json
POST /api/v1/browser/start
{
  "urls": ["https://example.com"],
  "profile": "myprofile"
}
```

При указании имени профиля браузер использует `launch_persistent_context` с директорией профиля. Это сохраняет:

- Cookies
- localStorage и sessionStorage
- Данные IndexedDB
- Кэш браузера
- Сохранённые пароли и данные форм

Данные профиля хранятся в `{profiles.dir}/{profile_name}/`.

#### Жизненный цикл профиля

1. Если директория профиля не существует, она создаётся автоматически.
2. При последующих запусках с тем же именем профиля все данные восстанавливаются.
3. Данные профиля сохраняются до ручного удаления из файловой системы.
4. При использовании профиля создаётся файл `profile.lock` с PID. Попытка использовать заблокированный профиль вызывает `ProfileLockedError`.
5. Устаревшие lock-файлы (PID которых больше не существует) очищаются автоматически.

#### Управление профилями через API

- `POST /api/v1/profile/create` - создание директории профиля
- `GET /api/v1/profiles` - список всех профилей

### Прокси

Проект поддерживает SOCKS5 и HTTP прокси через relay `socks2http`.

#### Архитектура

```
Browser --> patchright (proxy=http://127.0.0.1:PORT) --> socks2http --> upstream SOCKS5/HTTP --> target
```

`socks2http` запускается как подпроцесс, который:
- Слушает на случайном свободном локальном порту
- Конвертирует протокол HTTP-прокси в upstream SOCKS5
- Автоматически завершается при окончании браузерной сессии

#### Конфигурация

```yaml
proxy:
  enabled: true
  scheme: socks5
  host: proxy.example.com
  port: 1080
  username: user
  password: pass
  socks2httpBin: /path/to/socks2http
  localPort: 0
```

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `enabled` | `false` | Включить поддержку прокси |
| `scheme` | `socks5` | Протокол upstream (`socks5` или `http`) |
| `host` | - | Имя хоста upstream прокси |
| `port` | `1080`/`8080` | Порт upstream прокси (по умолчанию зависит от scheme) |
| `username` | - | Логин для аутентификации прокси |
| `password` | - | Пароль для аутентификации прокси |
| `socks2httpBin` | (авто) | Путь к бинарнику `socks2http` |
| `localPort` | `0` | Локальный порт relay (`0` = автовыбор свободного порта) |

Переменные окружения:

- `AUTOBROWSER_PROXY_ENABLED`
- `AUTOBROWSER_PROXY_SCHEME`
- `AUTOBROWSER_PROXY_HOST`
- `AUTOBROWSER_PROXY_PORT`
- `AUTOBROWSER_PROXY_USERNAME`
- `AUTOBROWSER_PROXY_PASSWORD`
- `AUTOBROWSER_SOCKS2HTTP_BIN`
- `AUTOBROWSER_PROXY_LOCAL_PORT`

#### Использование CLI

```bash
autobrowser run --url https://example.com --proxy socks5://user:pass@host:1080
```

Флаг `--proxy` принимает URL в формате `scheme://user:pass@host:port`.

Прокси также можно указать прямо в YAML-сценарии:

```yaml
session:
  proxy: socks5://user:pass@host:port
```

> CLI-флаг `--proxy` имеет приоритет над значением из сценария.

#### Проверка готовности прокси

Перед запуском браузера relay ожидает до 5 секунд готовности локального порта. Если relay не удаётся запустить, возникает ошибка `ProxyRelayError`.

#### Безопасность

- Учётные данные прокси маскируются в логах и выводе `safe_dump()`
- Пароль отображается как `****` в дампах конфигурации

### Хранение данных

#### Вывод содержимого

Содержимое страницы ограничено параметром `output.maxContentBytes` (по умолчанию: 1 МБ).

```yaml
output:
  maxContentBytes: 1048576
  contentFormat: html
```

Переменные окружения:

- `AUTOBROWSER_MAX_CONTENT_BYTES`
- `AUTOBROWSER_CONTENT_FORMAT`

#### Маскирование заголовков

Чувствительные HTTP-заголовки (`authorization`, `cookie`, `set-cookie`, `proxy-authorization`) автоматически маскируются в ответах API и выводе CLI.

#### Таймаут бездействия сессий

Сессии автоматически завершаются после `timeouts.idleSessionMs` (по умолчанию: 300000 мс = 5 минут) бездействия. Каждый API-запрос к сессии сбрасывает таймер.

```yaml
timeouts:
  idleSessionMs: 300000
```

Переменная окружения: `AUTOBROWSER_SESSION_IDLE_MS`
