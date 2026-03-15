# Runtime: Profiles, Proxy, Storage / Профили, прокси, хранение данных

## English

### Profiles

Browser profiles store user data (cookies, localStorage, session storage) between runs.

#### Configuration

```yaml
profiles:
  dir: ".profiles"
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `dir` | `.profiles` | Base directory for profile storage |

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

#### Session preservation (tab save/restore)

When `preserveSession: true` is set in a scenario or `--preserve-session` is passed on the CLI, the browser:

- **On stop:** saves all open tab URLs to `tabs.json` in the profile directory.
- **On start:** restores tabs from `tabs.json` (if no explicit `urls` are provided).

Without `preserveSession`, `tabs.json` is neither read nor written. The file remains on disk as a backup until the next run with `preserveSession: true`.

**How it works:**

1. Before session stop (with `preserveSession`), all non-blank page URLs are written to `{profile_dir}/tabs.json`.
2. On next start (with `preserveSession`), if no `urls` are passed and `tabs.json` exists, the saved URLs are opened.
3. Without `preserveSession`, the file is not touched — it stays as a backup.
4. Additionally, `--restore-last-session` Chromium flag is always added for persistent contexts, allowing Chromium's own session restore as a fallback.
5. All pages from the context (including Chromium-restored ones) are registered and tracked.

**Priority of tab sources (when `preserveSession: true`):**

1. Explicit `urls` parameter (highest priority)
2. `tabs.json` from profile directory
3. Chromium's built-in session restore (`--restore-last-session`)
4. Single blank page (fallback)

**Example scenario with session preservation:**

```yaml
session:
  headless: false
  profile: my-profile
  preserveSession: true

steps:
  - action: open
    url: "https://example.com"
  - action: open
    url: "https://example.org"
```

After this scenario completes, `tabs.json` in the profile directory will contain:

```json
[
  "https://example.com",
  "https://example.org"
]
```

The next run with `profile: my-profile` (without explicit URLs) will restore both tabs.

**CLI usage:**

```bash
autobrowser run-script --file warmup.yaml --preserve-session
```

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

### Platform spoofing

When a non-Linux User-Agent is used, the browser automatically adjusts JavaScript platform properties via CDP to match the UA string:

| UA contains | `navigator.platform` | Client Hints platform | Mobile context |
|-------------|---------------------|-----------------------|----------------|
| Windows | `Win32` | `Windows` | No |
| macOS | `MacIntel` | `macOS` | No |
| Android | `Linux armv8l` | `Android` | Yes (`is_mobile`, `has_touch`) |
| Linux | No spoofing | No spoofing | No |

Spoofed properties:
- `navigator.platform`
- `navigator.userAgentData.platform` (Client Hints)
- `Sec-CH-UA-Platform` header
- Chrome version and brand list in `userAgentData`

For Android UAs, the device model is extracted (e.g. `SM-S918B`, `Pixel 7 Pro`) and included in Client Hints metadata.

Mobile User-Agents automatically enable `is_mobile: true` and `has_touch: true` on the browser context, which affects viewport behavior and touch event emulation.

### Fingerprint spoofing

When `fingerprint: true` is set in a scenario, the browser injects JavaScript that modifies browser APIs to produce a unique but consistent fingerprint for each profile.

#### Spoofed APIs

| API | Spoofing method |
|-----|-----------------|
| Canvas (`getImageData`) | Adds deterministic noise to pixel data based on seed |
| WebGL (`RENDERER` / `VENDOR`) | Overrides GPU strings with values from a pool |
| AudioContext (`getFloatFrequencyData`) | Adds micro-noise to frequency data |
| ClientRects (`getBoundingClientRect`) | Adds sub-pixel noise to element coordinates |
| Fonts (`measureText`) | Adds micro-noise to text width measurements |

#### How it works

1. On first run with `fingerprint: true`, a `fingerprint.json` is generated in the profile directory with a random seed, GPU strings, and per-API toggles.
2. On subsequent runs, the same `fingerprint.json` is loaded, ensuring the profile always produces the same fingerprint.
3. The fingerprint config is baked into a JavaScript injection script and executed via `context.add_init_script()` before any page content loads.
4. The injection uses a seeded PRNG (mulberry32) so all noise is deterministic — same seed always produces the same modifications.

#### Fingerprint config file

The `fingerprint.json` file is stored in the profile directory alongside `tabs.json`:

```json
{
  "seed": 1234567890,
  "canvas": true,
  "webgl": true,
  "webglVendor": "Google Inc. (NVIDIA)",
  "webglRenderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
  "audio": true,
  "clientRects": true,
  "fonts": true
}
```

#### CLI command

Generate a fingerprint config for a profile directory:

```bash
autobrowser generate-fingerprint --profile work/my-profile/profile
autobrowser generate-fingerprint --profile work/my-profile/profile --platform linux
autobrowser generate-fingerprint --profile work/my-profile/profile --platform android
autobrowser generate-fingerprint --profile work/my-profile/profile --seed 42
autobrowser generate-fingerprint --profile work/my-profile/profile --force  # overwrite existing
```

Available platforms: `windows` (default), `linux`, `android`. Each platform uses a dedicated GPU profile pool with realistic vendor/renderer strings for that OS.

#### Scenario overrides

Individual fingerprint fields can be overridden in the scenario file:

```yaml
session:
  fingerprint:
    seed: 12345
    canvas: true
    webgl: false
```

Overrides are merged on top of the existing `fingerprint.json` (or a freshly generated one). The merged result is used for the session but is **not** saved back to disk.

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
```

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `dir` | `.profiles` | Базовая директория для хранения профилей |

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

#### Сохранение сессии (сохранение/восстановление вкладок)

Когда в сценарии указано `preserveSession: true` или передан CLI-флаг `--preserve-session`, браузер:

- **При остановке:** сохраняет URL всех открытых вкладок в файл `tabs.json` в директории профиля.
- **При запуске:** восстанавливает вкладки из `tabs.json` (если не указаны явные `urls`).

Без `preserveSession` файл `tabs.json` не читается и не записывается. Файл остаётся на диске как резервная копия до следующего запуска с `preserveSession: true`.

**Как это работает:**

1. Перед остановкой сессии (с `preserveSession`) все URL непустых страниц записываются в `{profile_dir}/tabs.json`.
2. При следующем запуске (с `preserveSession`), если `urls` не переданы и `tabs.json` существует, сохранённые URL открываются.
3. Без `preserveSession` файл не затрагивается — остаётся как резервная копия.
4. Дополнительно для persistent-контекстов всегда добавляется флаг Chromium `--restore-last-session`, позволяющий встроенное восстановление сессии как запасной механизм.
5. Все страницы из контекста (включая восстановленные Chromium) регистрируются и отслеживаются.

**Приоритет источников вкладок (при `preserveSession: true`):**

1. Явный параметр `urls` (высший приоритет)
2. `tabs.json` из директории профиля
3. Встроенное восстановление Chromium (`--restore-last-session`)
4. Одна пустая страница (запасной вариант)

**Пример сценария с сохранением сессии:**

```yaml
session:
  headless: false
  profile: my-profile
  preserveSession: true

steps:
  - action: open
    url: "https://example.com"
  - action: open
    url: "https://example.org"
```

После выполнения этого сценария `tabs.json` в директории профиля будет содержать:

```json
[
  "https://example.com",
  "https://example.org"
]
```

Следующий запуск с `profile: my-profile` (без явных URL) восстановит обе вкладки.

**Использование через CLI:**

```bash
autobrowser run-script --file warmup.yaml --preserve-session
```

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

### Подмена платформы (platform spoofing)

При использовании User-Agent, отличного от Linux, браузер автоматически подменяет JavaScript-свойства платформы через CDP для соответствия UA-строке:

| UA содержит | `navigator.platform` | Client Hints platform | Мобильный контекст |
|-------------|---------------------|-----------------------|--------------------|
| Windows | `Win32` | `Windows` | Нет |
| macOS | `MacIntel` | `macOS` | Нет |
| Android | `Linux armv8l` | `Android` | Да (`is_mobile`, `has_touch`) |
| Linux | Без подмены | Без подмены | Нет |

Подменяемые свойства:
- `navigator.platform`
- `navigator.userAgentData.platform` (Client Hints)
- Заголовок `Sec-CH-UA-Platform`
- Версия Chrome и список брендов в `userAgentData`

Для Android UA модель устройства извлекается автоматически (например, `SM-S918B`, `Pixel 7 Pro`) и включается в метаданные Client Hints.

Мобильные User-Agent автоматически включают `is_mobile: true` и `has_touch: true` в контексте браузера, что влияет на поведение viewport и эмуляцию тач-событий.

### Подмена отпечатка браузера (fingerprint spoofing)

При указании `fingerprint: true` в сценарии браузер инжектирует JavaScript, который модифицирует API браузера для создания уникального, но постоянного отпечатка для каждого профиля.

#### Подменяемые API

| API | Метод подмены |
|-----|---------------|
| Canvas (`getImageData`) | Добавление детерминированного шума к пиксельным данным на основе seed |
| WebGL (`RENDERER` / `VENDOR`) | Подмена строк GPU значениями из пула |
| AudioContext (`getFloatFrequencyData`) | Добавление микро-шума к частотным данным |
| ClientRects (`getBoundingClientRect`) | Добавление субпиксельного шума к координатам элементов |
| Fonts (`measureText`) | Добавление микро-шума к измерениям ширины текста |

#### Как это работает

1. При первом запуске с `fingerprint: true` в директории профиля создаётся `fingerprint.json` со случайным seed, строками GPU и флагами для каждого API.
2. При последующих запусках загружается тот же `fingerprint.json`, обеспечивая постоянство отпечатка профиля.
3. Конфиг отпечатка встраивается в JavaScript-скрипт инъекции и выполняется через `context.add_init_script()` до загрузки любого контента страницы.
4. Инъекция использует ГПСЧ с seed (mulberry32), поэтому весь шум детерминирован — один и тот же seed всегда даёт одинаковые модификации.

#### Файл конфига отпечатка

Файл `fingerprint.json` хранится в директории профиля рядом с `tabs.json`:

```json
{
  "seed": 1234567890,
  "canvas": true,
  "webgl": true,
  "webglVendor": "Google Inc. (NVIDIA)",
  "webglRenderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
  "audio": true,
  "clientRects": true,
  "fonts": true
}
```

#### CLI-команда

Генерация конфига отпечатка для директории профиля:

```bash
autobrowser generate-fingerprint --profile work/my-profile/profile
autobrowser generate-fingerprint --profile work/my-profile/profile --platform linux
autobrowser generate-fingerprint --profile work/my-profile/profile --platform android
autobrowser generate-fingerprint --profile work/my-profile/profile --seed 42
autobrowser generate-fingerprint --profile work/my-profile/profile --force  # перезаписать существующий
```

Доступные платформы: `windows` (по умолчанию), `linux`, `android`. Каждая платформа использует свой пул GPU-профилей с реалистичными строками vendor/renderer для данной ОС.

#### Переопределения в сценарии

Отдельные поля отпечатка можно переопределить в файле сценария:

```yaml
session:
  fingerprint:
    seed: 12345
    canvas: true
    webgl: false
```

Переопределения объединяются поверх существующего `fingerprint.json` (или только что сгенерированного). Результат используется для сессии, но **не** сохраняется обратно на диск.

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
