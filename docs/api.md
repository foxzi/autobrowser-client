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
  "sessionId": "sess_123",
  "pageId": "page_1",
  "action": "click",
  "durationMs": 5007,
  "error": {
    "code": "ELEMENT_NOT_FOUND",
    "message": "Element was not found for the provided locator",
    "details": {}
  }
}
```

### Authentication

- API authentication is optional.
- When enabled, requests should provide a configured token.
- Health and readiness endpoints may remain unauthenticated.

Recommended header:

```http
Authorization: Bearer <token>
```

### Core endpoints

#### Service endpoints

- `GET /health` - liveness check
- `GET /ready` - readiness check
- `GET /api/v1/info` - API metadata and current configuration summary

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
{ "startX": 0, "startY": 0, "endX": 500, "endY": 300, "steps": 25, "durationMs": 300, "jitter": 3.0 }
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

#### Wait options

Common wait fields:

- `timeout`
- `waitUntil`

Supported `waitUntil` values:

- `domcontentloaded`
- `load`
- `networkidle`
- `visible`
- `hidden`

### Example requests

#### Start browser session

```json
{
  "headless": true,
  "urls": ["https://example.com"],
  "profile": "test1",
  "userAgent": "Mozilla/5.0",
  "viewport": {"width": 1920, "height": 1080},
  "timeout": 10000
}
```

#### Find element

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  },
  "timeout": 5000,
  "waitUntil": "visible"
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

### Error codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_ARGUMENT` | 400 | Bad request or invalid parameter |
| `CONFIG_ERROR` | 400 | Configuration error |
| `UNAUTHORIZED` | 401 | Missing or invalid token |
| `SESSION_NOT_FOUND` | 404 | Session ID not found |
| `PAGE_NOT_FOUND` | 404 | Page ID not found |
| `PROFILE_NOT_FOUND` | 404 | Profile does not exist |
| `PROFILE_LOCKED` | 409 | Profile is locked by another session |
| `NOT_IMPLEMENTED` | 501 | Feature not implemented |
| `BROWSER_START_FAILED` | 502 | Browser failed to start |
| `TIMEOUT` | 504 | Operation timed out |
| `INTERNAL_ERROR` | 500 | Unexpected server error |

### OpenAPI

Interactive documentation is available at `/docs` (Swagger UI) and `/openapi.json` when the server is running. These paths bypass token authentication.

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
  "sessionId": "sess_123",
  "pageId": "page_1",
  "action": "click",
  "durationMs": 5007,
  "error": {
    "code": "ELEMENT_NOT_FOUND",
    "message": "Element was not found for the provided locator",
    "details": {}
  }
}
```

### Аутентификация

- Аутентификация API является опциональной.
- При включенной защите запросы должны передавать настроенный токен.
- Endpoints `health` и `ready` могут оставаться без аутентификации.

Рекомендуемый заголовок:

```http
Authorization: Bearer <token>
```

### Основные endpoints

#### Сервисные endpoints

- `GET /health` - проверка живости
- `GET /ready` - проверка готовности
- `GET /api/v1/info` - метаданные API и краткая сводка текущей конфигурации

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
{ "startX": 0, "startY": 0, "endX": 500, "endY": 300, "steps": 25, "durationMs": 300, "jitter": 3.0 }
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

#### Параметры ожидания

Общие поля ожидания:

- `timeout`
- `waitUntil`

Поддерживаемые значения `waitUntil`:

- `domcontentloaded`
- `load`
- `networkidle`
- `visible`
- `hidden`

### Примеры запросов

#### Запуск браузерной сессии

```json
{
  "headless": true,
  "urls": ["https://example.com"],
  "profile": "test1",
  "userAgent": "Mozilla/5.0",
  "timeout": 10000
}
```

#### Поиск элемента

```json
{
  "locator": {
    "type": "css",
    "value": "button[type='submit']"
  },
  "timeout": 5000,
  "waitUntil": "visible"
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

### Коды ошибок

| Код | HTTP-статус | Описание |
|-----|-------------|----------|
| `INVALID_ARGUMENT` | 400 | Некорректный запрос или параметр |
| `CONFIG_ERROR` | 400 | Ошибка конфигурации |
| `UNAUTHORIZED` | 401 | Отсутствующий или недействительный токен |
| `SESSION_NOT_FOUND` | 404 | Сессия не найдена |
| `PAGE_NOT_FOUND` | 404 | Страница не найдена |
| `PROFILE_NOT_FOUND` | 404 | Профиль не существует |
| `PROFILE_LOCKED` | 409 | Профиль заблокирован другой сессией |
| `NOT_IMPLEMENTED` | 501 | Функция не реализована |
| `BROWSER_START_FAILED` | 502 | Браузер не удалось запустить |
| `TIMEOUT` | 504 | Таймаут операции |
| `INTERNAL_ERROR` | 500 | Непредвиденная ошибка сервера |

### OpenAPI

Интерактивная документация доступна по адресу `/docs` (Swagger UI) и `/openapi.json` при запущенном сервере. Эти пути не требуют токена аутентификации.
