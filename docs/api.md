# HTTP API / HTTP API

## English

### Purpose

`autobrowser-client` exposes an HTTP API for starting browser sessions, managing profiles, extracting page data, and performing interactive browser actions.

The API is intended for remote or service-to-service control when plain CLI execution is not enough.

### Base path and versioning

- Base prefix: `/api/v1`
- Health endpoint: `/health`
- Readiness endpoint: `/ready`

Backward-incompatible changes should only be introduced in a new API version.

### General response format

Successful response shape:

```json
{
  "success": true,
  "sessionId": "sess_123",
  "pageId": "page_1",
  "action": "click",
  "durationMs": 124,
  "result": {}
}
```

Error response shape:

```json
{
  "success": false,
  "error": {
    "code": "TIMEOUT",
    "message": "Locator.click: Timeout 10000ms exceeded. ..."
  }
}
```

Request body validation errors use the FastAPI format: HTTP 422 with `{"detail": [...]}`. `POST /api/v1/profile/create` answers an invalid or existing name with HTTP 400 / 409 and `{"detail": "..."}`.

### Authentication

- API authentication is optional: it is on when `api.token` / `AUTOBROWSER_API_TOKEN` is set.
- Every request then needs the header below, otherwise HTTP 401 with code `UNAUTHORIZED`.
- `/health`, `/ready`, `/docs` and `/openapi.json` stay open.

Header:

```http
Authorization: Bearer <token>
```

### Core endpoints

#### Service endpoints

- `GET /health` - liveness check
- `GET /ready` - readiness check
- `GET /api/v1/info` - name, version, API host and port

#### Session management

- `POST /api/v1/browser/start` - start browser session
- `GET /api/v1/sessions` - list all active sessions
- `GET /api/v1/session/{id}` - get session status
- `POST /api/v1/session/{id}/stop` - stop active session

#### Profile management

- `POST /api/v1/profile/create` - create profile
- `GET /api/v1/profiles` - list all profiles

#### Page data

- `GET /api/v1/session/{id}/cookies` - get cookies
- `GET /api/v1/session/{id}/headers` - get request and response headers
- `GET /api/v1/session/{id}/content` - get page content (query param `fmt`: `html` or `text`)

`headers` and `content` take an optional query param `page_id` (default: the first tab).

#### DOM interaction

- `POST .../elements/find` - find element (body: `ElementRequest`)
- `POST .../elements/find-all` - find all matching elements (body: `ElementRequest`)
- `POST .../elements/read` - read element details: text, innerHTML, visible, enabled, checked, value (body: `ElementRequest`)
- `POST .../elements/click` - click element (body: `ClickRequest`, optional `double: true`)
- `POST .../elements/hover` - hover over element (body: `ElementRequest`)
- `POST .../elements/focus` - focus element (body: `ElementRequest`)
- `POST .../elements/fill` - fill input (body: `FillRequest`, optional `clearBefore: true`)
- `POST .../elements/clear` - clear input (body: `ElementRequest`)
- `POST .../elements/select` - select option (body: `SelectRequest`, `option: string | string[]`)
- `POST .../elements/check` - check/uncheck checkbox (body: `CheckRequest`, `checked: bool`)
- `POST .../elements/wait` - wait for element or navigation (body: `WaitRequest`)
- `POST .../forms/submit` - submit form (body: `ElementRequest`)

> All DOM paths are prefixed with `/api/v1/session/{id}/pages/{pageId}`.

#### Mouse and keyboard

- `POST .../mouse/move` - move to coordinates (body: `MouseMoveRequest`)
- `POST .../mouse/move-to` - move to element by locator (body: `ElementRequest`)
- `POST .../mouse/scroll` - scroll by delta (body: `ScrollRequest`)
- `POST .../mouse/scroll-to` - scroll to element (body: `ElementRequest`)
- `POST .../mouse/move-random` - human-like random movement (body: `MouseRandomRequest`)
- `POST .../keyboard/press` - press key (body: `PressRequest`)

> All mouse/keyboard paths are prefixed with `/api/v1/session/{id}/pages/{pageId}`.

#### Action chains

- `POST /api/v1/session/{id}/pages/{pageId}/actions/run` - run a sequence of actions
- `POST /api/v1/session/{id}/script` - run a scenario (variables, conditions, loops) on the session; see "Run script"

#### Pages and agent operations

- `GET /api/v1/session/{id}/pages` - list open tabs (tabs opened by the page, e.g. popups, are registered automatically)
- `POST /api/v1/session/{id}/pages` - open a new tab (body: `{"url": "..."}`, optional)
- `DELETE /api/v1/session/{id}/pages/{pageId}` - close a tab
- `GET .../snapshot` - accessibility tree with `[ref=eN]` marks (query param `timeout`, default 10000); pass a ref as locator `{"type": "ref", "value": "e2"}`
- `POST .../screenshot` - PNG in base64 (body: `fullPage`, optional `locator` for one element)
- `POST .../navigate` - go to URL (body: `url`, `waitUntil`, `timeout`)
- `POST .../history` - `direction`: `back`, `forward` or `reload`
- `POST .../evaluate` - run JS expression or function source (body: `expression`, `isolated`, default main world)
- `POST .../wait-text` - wait for text to appear, or to disappear with `gone: true`
- `POST .../elements/upload` - attach local files (body: `locator`, `files`: server-side paths); works on `input[type=file]` or on an element that opens a file chooser
- `GET .../logs/{kind}` - event log of the tab, `kind`: `console`, `network` or `dialogs`; `?clear=true` empties it after reading (last 500 entries kept)
- `POST /api/v1/session/{id}/dialog-policy` - how to answer `alert`/`confirm`/`prompt` (body: `accept`, `promptText`); default is dismiss, every dialog is recorded in `logs/dialogs`

> Console log contains only browser-level messages (failed resources, security warnings). Patchright does not deliver page `console.log` and uncaught errors by design, to stay undetectable; the Go binary does not record them either (the CDP `Runtime` domain stays off).

> Refs are valid until the page changes; take a new snapshot after navigation or DOM updates.

### Request models

#### ElementRequest

```json
{ "locator": { "type": "css", "value": "..." }, "timeout": 10000 }
```

#### ClickRequest

```json
{ "locator": { "type": "css", "value": "..." }, "timeout": 10000, "double": false }
```

#### FillRequest

```json
{ "locator": { "type": "css", "value": "..." }, "value": "text", "clearBefore": false, "timeout": 10000 }
```

#### SelectRequest

```json
{ "locator": { "type": "css", "value": "..." }, "option": "value", "timeout": 10000 }
```

#### CheckRequest

```json
{ "locator": { "type": "css", "value": "..." }, "checked": true, "timeout": 10000 }
```

#### WaitRequest

```json
{ "locator": { "type": "css", "value": "..." }, "state": "visible", "timeout": 10000 }
```

Without locator (wait for navigation):

```json
{ "waitUntil": "load", "timeout": 10000 }
```

#### MouseMoveRequest

```json
{ "x": 100, "y": 200, "steps": 1 }
```

#### ScrollRequest

```json
{ "deltaX": 0, "deltaY": 500 }
```

#### MouseRandomRequest

```json
{ "startX": 0, "startY": 0, "endX": 500, "endY": 300, "steps": 25, "durationMs": 300, "jitter": 3.0, "seed": null }
```

#### PressRequest

```json
{ "key": "Enter" }
```

#### ActionChainRequest

```json
{ "steps": [ { "action": "click", "locator": { "type": "css", "value": "..." } } ], "timeout": 10000 }
```

### Shared request objects

#### Locator

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  }
}
```

Supported `type` values:

- `css`
- `xpath`
- `id`
- `name`
- `class`
- `text`
- `attribute`
- `ref` - ref from `snapshot` (e.g. `e2`)

#### Wait options

`WaitRequest` with a `locator` waits for the element `state`: `attached`, `detached`, `visible` (default) or `hidden`.
Without a locator it waits for the page load state `waitUntil`: `load` (default), `domcontentloaded` or `networkidle`.
`navigate` takes the same `waitUntil` values plus `commit` (the Go binary ignores it and always waits for `load`). `timeout` is in milliseconds.

### Example requests

#### Start browser session

Note: the browser engine (`patchright` or `cloakbrowser`) is a process-wide setting configured via config file or `AUTOBROWSER_BROWSER_ENGINE` env var. It is not a per-session or per-request parameter.

```json
{
  "headless": true,
  "urls": ["https://example.com"],
  "profile": "test1",
  "userAgent": "Mozilla/5.0",
  "viewport": {"width": 1920, "height": 1080}
}
```

#### Find element

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  },
  "timeout": 5000
}
```

#### Fill input

```json
{
  "locator": {
    "type": "name",
    "value": "email"
  },
  "value": "user@example.com",
  "clearBefore": true,
  "timeout": 5000
}
```

#### Run action chain

```json
{
  "steps": [
    {
      "action": "find",
      "locator": {
        "type": "id",
        "value": "login"
      }
    },
    {
      "action": "fill",
      "locator": {
        "type": "name",
        "value": "username"
      },
      "value": "demo-user"
    }
  ]
}
```

#### Run script

`POST /api/v1/session/{id}/script` runs a scenario in the format of [`scripts.md`](scripts.md) on an existing session. The scenario `session` block is ignored.

```json
{
  "script": {
    "steps": [
      {"open": "https://example.com"},
      {"action": "extract", "locator": "p", "as": "intro"},
      {"assert": {"var": "intro", "contains": "documentation"}}
    ]
  },
  "vars": {"key": "value"},
  "pageId": "page_1"
}
```

- `script` - an object, or a YAML string (`include` paths are relative to the server working directory);
- `vars` - optional, overrides the scenario `vars`;
- `pageId` - optional, default is the first tab.

Response: `{"success", "sessionId", "steps", "vars", "durationMs"}`. A failing step gives HTTP 200 with `success: false` and the error inside `steps`; an invalid script gives HTTP 400 with `SCRIPT_VALIDATION_ERROR`.

### Error codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_ARGUMENT` | 400 | Bad request or invalid parameter |
| `CONFIG_ERROR` | 400 | Configuration error |
| `SCRIPT_VALIDATION_ERROR` | 400 | Invalid scenario |
| `UNAUTHORIZED` | 401 | Missing or invalid token |
| `SESSION_NOT_FOUND` | 404 | Session ID not found |
| `PAGE_NOT_FOUND` | 404 | Page ID not found |
| `ACTION_FAILED` | 422 | Browser action failed (JS error, element not found, etc.) |
| `NOT_IMPLEMENTED` | 501 | Feature not implemented |
| `BROWSER_START_FAILED` | 502 | Browser failed to start, profile locked by another process, session limit (`api.maxSessions`) reached |
| `TIMEOUT` | 504 | Operation timed out |
| `INTERNAL_ERROR` | 500 | Unexpected server error; any other code is also returned with 500 |

### OpenAPI

Interactive documentation is available at `/docs` (Swagger UI), `/redoc` and `/openapi.json` when the server is running. `/docs` and `/openapi.json` bypass token authentication. The Go binary (`autobrowser serve`) serves the same routes and the same `openapi.json`; see [known differences](../go/README.md#known-differences).

## Русский

### Назначение

`autobrowser-client` предоставляет HTTP API для запуска браузерных сессий, управления профилями, получения данных страницы и выполнения интерактивных действий в браузере.

API предназначен для удаленного управления или взаимодействия между сервисами, когда обычного запуска через CLI недостаточно.

### Базовый путь и версионирование

- Базовый префикс: `/api/v1`
- Endpoint проверки живости: `/health`
- Endpoint проверки готовности: `/ready`

Несовместимые с предыдущими версиями изменения должны выноситься только в новую версию API.

### Общий формат ответа

Форма успешного ответа:

```json
{
  "success": true,
  "sessionId": "sess_123",
  "pageId": "page_1",
  "action": "click",
  "durationMs": 124,
  "result": {}
}
```

Форма ответа с ошибкой:

```json
{
  "success": false,
  "error": {
    "code": "TIMEOUT",
    "message": "Locator.click: Timeout 10000ms exceeded. ..."
  }
}
```

Ошибки валидации тела запроса - в формате FastAPI: HTTP 422 и `{"detail": [...]}`. `POST /api/v1/profile/create` на некорректное или уже существующее имя отвечает HTTP 400 / 409 и `{"detail": "..."}`.

### Аутентификация

- Аутентификация API опциональна: включается, если задан `api.token` / `AUTOBROWSER_API_TOKEN`.
- Тогда каждый запрос должен передавать заголовок ниже, иначе HTTP 401 с кодом `UNAUTHORIZED`.
- `/health`, `/ready`, `/docs` и `/openapi.json` остаются открытыми.

Заголовок:

```http
Authorization: Bearer <token>
```

### Основные endpoints

#### Сервисные endpoints

- `GET /health` - проверка живости
- `GET /ready` - проверка готовности
- `GET /api/v1/info` - имя, версия, хост и порт API

#### Управление сессиями

- `POST /api/v1/browser/start` - запуск браузерной сессии
- `GET /api/v1/sessions` - список всех активных сессий
- `GET /api/v1/session/{id}` - получение статуса сессии
- `POST /api/v1/session/{id}/stop` - остановка активной сессии

#### Управление профилями

- `POST /api/v1/profile/create` - создание профиля
- `GET /api/v1/profiles` - список всех профилей

#### Данные страницы

- `GET /api/v1/session/{id}/cookies` - получение cookies
- `GET /api/v1/session/{id}/headers` - получение заголовков запроса и ответа
- `GET /api/v1/session/{id}/content` - получение содержимого страницы (query-параметр `fmt`: `html` или `text`)

`headers` и `content` принимают необязательный query-параметр `page_id` (по умолчанию первая вкладка).

#### Работа с DOM

- `POST .../elements/find` - поиск элемента (тело: `ElementRequest`)
- `POST .../elements/find-all` - поиск всех совпадающих элементов (тело: `ElementRequest`)
- `POST .../elements/read` - чтение элемента: text, innerHTML, visible, enabled, checked, value (тело: `ElementRequest`)
- `POST .../elements/click` - клик по элементу (тело: `ClickRequest`, необязательное `double: true`)
- `POST .../elements/hover` - наведение на элемент (тело: `ElementRequest`)
- `POST .../elements/focus` - фокус на элемент (тело: `ElementRequest`)
- `POST .../elements/fill` - заполнение поля (тело: `FillRequest`, необязательное `clearBefore: true`)
- `POST .../elements/clear` - очистка поля (тело: `ElementRequest`)
- `POST .../elements/select` - выбор опции (тело: `SelectRequest`, `option: string | string[]`)
- `POST .../elements/check` - отметка/снятие чекбокса (тело: `CheckRequest`, `checked: bool`)
- `POST .../elements/wait` - ожидание элемента или навигации (тело: `WaitRequest`)
- `POST .../forms/submit` - отправка формы (тело: `ElementRequest`)

> Все DOM-пути начинаются с `/api/v1/session/{id}/pages/{pageId}`.

#### Мышь и клавиатура

- `POST .../mouse/move` - перемещение к координатам (тело: `MouseMoveRequest`)
- `POST .../mouse/move-to` - перемещение к элементу по локатору (тело: `ElementRequest`)
- `POST .../mouse/scroll` - прокрутка по дельте (тело: `ScrollRequest`)
- `POST .../mouse/scroll-to` - прокрутка к элементу (тело: `ElementRequest`)
- `POST .../mouse/move-random` - рандомное движение мыши (тело: `MouseRandomRequest`)
- `POST .../keyboard/press` - нажатие клавиши (тело: `PressRequest`)

> Все пути мыши/клавиатуры начинаются с `/api/v1/session/{id}/pages/{pageId}`.

#### Цепочки действий

- `POST /api/v1/session/{id}/pages/{pageId}/actions/run` - запуск последовательности действий
- `POST /api/v1/session/{id}/script` - запуск сценария (переменные, условия, циклы) в сессии; см. «Запуск сценария»

#### Вкладки и операции для агентов

- `GET /api/v1/session/{id}/pages` - список открытых вкладок (вкладки, открытые страницей, например popup, регистрируются автоматически)
- `POST /api/v1/session/{id}/pages` - открыть новую вкладку (тело: `{"url": "..."}`, необязательно)
- `DELETE /api/v1/session/{id}/pages/{pageId}` - закрыть вкладку
- `GET .../snapshot` - дерево доступности с метками `[ref=eN]` (query-параметр `timeout`, по умолчанию 10000); ref передается как локатор `{"type": "ref", "value": "e2"}`
- `POST .../screenshot` - PNG в base64 (тело: `fullPage`, необязательный `locator` для одного элемента)
- `POST .../navigate` - переход по URL (тело: `url`, `waitUntil`, `timeout`)
- `POST .../history` - `direction`: `back`, `forward` или `reload`
- `POST .../evaluate` - выполнить JS-выражение или функцию (тело: `expression`, `isolated`, по умолчанию main world)
- `POST .../wait-text` - ждать появления текста, или исчезновения при `gone: true`
- `POST .../elements/upload` - прикрепить локальные файлы (тело: `locator`, `files`: пути на сервере); работает с `input[type=file]` и с элементом, который открывает выбор файла
- `GET .../logs/{kind}` - журнал событий вкладки, `kind`: `console`, `network` или `dialogs`; `?clear=true` очищает его после чтения (хранятся последние 500 записей)
- `POST /api/v1/session/{id}/dialog-policy` - как отвечать на `alert`/`confirm`/`prompt` (тело: `accept`, `promptText`); по умолчанию dismiss, каждый диалог записывается в `logs/dialogs`

> Журнал console содержит только сообщения уровня браузера (ошибки загрузки ресурсов, предупреждения безопасности). Patchright намеренно не передает `console.log` страницы и необработанные ошибки, чтобы не выдавать автоматизацию; Go-бинарь их тоже не записывает (домен CDP `Runtime` выключен).

> Ref действительны, пока страница не изменилась; после навигации или обновления DOM сделайте новый snapshot.

### Модели запросов

#### ElementRequest

```json
{ "locator": { "type": "css", "value": "..." }, "timeout": 10000 }
```

#### ClickRequest

```json
{ "locator": { "type": "css", "value": "..." }, "timeout": 10000, "double": false }
```

#### FillRequest

```json
{ "locator": { "type": "css", "value": "..." }, "value": "text", "clearBefore": false, "timeout": 10000 }
```

#### SelectRequest

```json
{ "locator": { "type": "css", "value": "..." }, "option": "value", "timeout": 10000 }
```

#### CheckRequest

```json
{ "locator": { "type": "css", "value": "..." }, "checked": true, "timeout": 10000 }
```

#### WaitRequest

```json
{ "locator": { "type": "css", "value": "..." }, "state": "visible", "timeout": 10000 }
```

Без локатора (ожидание навигации):

```json
{ "waitUntil": "load", "timeout": 10000 }
```

#### MouseMoveRequest

```json
{ "x": 100, "y": 200, "steps": 1 }
```

#### ScrollRequest

```json
{ "deltaX": 0, "deltaY": 500 }
```

#### MouseRandomRequest

```json
{ "startX": 0, "startY": 0, "endX": 500, "endY": 300, "steps": 25, "durationMs": 300, "jitter": 3.0, "seed": null }
```

#### PressRequest

```json
{ "key": "Enter" }
```

#### ActionChainRequest

```json
{ "steps": [ { "action": "click", "locator": { "type": "css", "value": "..." } } ], "timeout": 10000 }
```

### Общие объекты запроса

#### Locator

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  }
}
```

Поддерживаемые значения `type`:

- `css`
- `xpath`
- `id`
- `name`
- `class`
- `text`
- `attribute`
- `ref` - ref из `snapshot` (например `e2`)

#### Параметры ожидания

`WaitRequest` с `locator` ждёт состояние элемента `state`: `attached`, `detached`, `visible` (по умолчанию) или `hidden`.
Без локатора он ждёт состояние загрузки страницы `waitUntil`: `load` (по умолчанию), `domcontentloaded` или `networkidle`.
`navigate` принимает те же значения `waitUntil` и ещё `commit` (Go-бинарь его игнорирует и всегда ждёт `load`). `timeout` - в миллисекундах.

### Примеры запросов

#### Запуск браузерной сессии

Примечание: движок браузера (`patchright` или `cloakbrowser`) — это настройка уровня процесса, задаётся через конфиг-файл или переменную окружения `AUTOBROWSER_BROWSER_ENGINE`. Изменить его на уровне сессии или API-запроса нельзя.

```json
{
  "headless": true,
  "urls": ["https://example.com"],
  "profile": "test1",
  "userAgent": "Mozilla/5.0",
  "viewport": {"width": 1920, "height": 1080}
}
```

#### Поиск элемента

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  },
  "timeout": 5000
}
```

#### Заполнение поля

```json
{
  "locator": {
    "type": "name",
    "value": "email"
  },
  "value": "user@example.com",
  "clearBefore": true,
  "timeout": 5000
}
```

#### Запуск цепочки действий

```json
{
  "steps": [
    {
      "action": "find",
      "locator": {
        "type": "id",
        "value": "login"
      }
    },
    {
      "action": "fill",
      "locator": {
        "type": "name",
        "value": "username"
      },
      "value": "demo-user"
    }
  ]
}
```

#### Запуск сценария

`POST /api/v1/session/{id}/script` выполняет сценарий формата [`scripts.md`](scripts.md) в существующей сессии. Блок `session` сценария игнорируется.

```json
{
  "script": {
    "steps": [
      {"open": "https://example.com"},
      {"action": "extract", "locator": "p", "as": "intro"},
      {"assert": {"var": "intro", "contains": "documentation"}}
    ]
  },
  "vars": {"key": "value"},
  "pageId": "page_1"
}
```

- `script` - объект или строка YAML (пути `include` задаются относительно рабочего каталога сервера);
- `vars` - необязательно, переопределяет `vars` сценария;
- `pageId` - необязательно, по умолчанию первая вкладка.

Ответ: `{"success", "sessionId", "steps", "vars", "durationMs"}`. Упавший шаг даёт HTTP 200 с `success: false` и ошибкой внутри `steps`; некорректный сценарий - HTTP 400 с `SCRIPT_VALIDATION_ERROR`.

### Коды ошибок

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `INVALID_ARGUMENT` | 400 | Некорректный запрос или параметр |
| `CONFIG_ERROR` | 400 | Ошибка конфигурации |
| `SCRIPT_VALIDATION_ERROR` | 400 | Некорректный сценарий |
| `UNAUTHORIZED` | 401 | Отсутствующий или недействительный токен |
| `SESSION_NOT_FOUND` | 404 | Сессия не найдена |
| `PAGE_NOT_FOUND` | 404 | Страница не найдена |
| `ACTION_FAILED` | 422 | Ошибка действия в браузере (ошибка JS, элемент не найден и т.п.) |
| `NOT_IMPLEMENTED` | 501 | Функция не реализована |
| `BROWSER_START_FAILED` | 502 | Браузер не удалось запустить, профиль занят другим процессом, достигнут лимит сессий (`api.maxSessions`) |
| `TIMEOUT` | 504 | Таймаут операции |
| `INTERNAL_ERROR` | 500 | Непредвиденная ошибка сервера; любой другой код тоже возвращается с 500 |

### OpenAPI

Интерактивная документация доступна по адресам `/docs` (Swagger UI), `/redoc` и `/openapi.json` при запущенном сервере. `/docs` и `/openapi.json` не требуют токена. Go-бинарь (`autobrowser serve`) отдаёт те же маршруты и тот же `openapi.json`; см. [известные отличия](../go/README.ru.md#известные-отличия).
