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
- `GET /api/v1/session/{id}` - get session status
- `POST /api/v1/session/{id}/stop` - stop active session

#### Profile management

- `POST /api/v1/profile/create` - create profile

#### Page data

- `GET /api/v1/session/{id}/cookies` - get cookies
- `GET /api/v1/session/{id}/headers` - get request and response headers
- `GET /api/v1/session/{id}/content` - get page content

#### DOM interaction

- `POST /api/v1/session/{id}/pages/{pageId}/elements/find`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/find-all`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/click`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/fill`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/select`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/check`
- `POST /api/v1/session/{id}/pages/{pageId}/forms/submit`

#### Mouse and keyboard

- `POST /api/v1/session/{id}/pages/{pageId}/mouse/move`
- `POST /api/v1/session/{id}/pages/{pageId}/mouse/scroll`
- `POST /api/v1/session/{id}/pages/{pageId}/mouse/move-random`
- `POST /api/v1/session/{id}/pages/{pageId}/keyboard/press`

#### Action chains

- `POST /api/v1/session/{id}/pages/{pageId}/actions/run`

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

Minimum shared error catalog:

- `INVALID_ARGUMENT`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `SESSION_NOT_FOUND`
- `PAGE_NOT_FOUND`
- `PROFILE_NOT_FOUND`
- `PROFILE_LOCKED`
- `PROXY_UNAVAILABLE`
- `BROWSER_START_FAILED`
- `TIMEOUT`
- `ELEMENT_NOT_FOUND`
- `ELEMENT_NOT_INTERACTABLE`
- `AMBIGUOUS_ELEMENT_MATCH`
- `CONTENT_TOO_LARGE`
- `INTERNAL_ERROR`

Recommended HTTP status mapping:

- `400` bad request
- `401` unauthorized
- `403` forbidden
- `404` not found
- `409` conflict
- `422` validation error
- `500` internal error
- `502` upstream or browser start error
- `504` timeout

### OpenAPI

The implementation should expose an OpenAPI specification and interactive documentation generated from the API models.

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
- `GET /api/v1/session/{id}` - получение статуса сессии
- `POST /api/v1/session/{id}/stop` - остановка активной сессии

#### Управление профилями

- `POST /api/v1/profile/create` - создание профиля

#### Данные страницы

- `GET /api/v1/session/{id}/cookies` - получение cookies
- `GET /api/v1/session/{id}/headers` - получение заголовков запроса и ответа
- `GET /api/v1/session/{id}/content` - получение содержимого страницы

#### Работа с DOM

- `POST /api/v1/session/{id}/pages/{pageId}/elements/find`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/find-all`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/click`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/fill`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/select`
- `POST /api/v1/session/{id}/pages/{pageId}/elements/check`
- `POST /api/v1/session/{id}/pages/{pageId}/forms/submit`

#### Мышь и клавиатура

- `POST /api/v1/session/{id}/pages/{pageId}/mouse/move`
- `POST /api/v1/session/{id}/pages/{pageId}/mouse/scroll`
- `POST /api/v1/session/{id}/pages/{pageId}/mouse/move-random`
- `POST /api/v1/session/{id}/pages/{pageId}/keyboard/press`

#### Цепочки действий

- `POST /api/v1/session/{id}/pages/{pageId}/actions/run`

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

Минимальный общий каталог ошибок:

- `INVALID_ARGUMENT`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `SESSION_NOT_FOUND`
- `PAGE_NOT_FOUND`
- `PROFILE_NOT_FOUND`
- `PROFILE_LOCKED`
- `PROXY_UNAVAILABLE`
- `BROWSER_START_FAILED`
- `TIMEOUT`
- `ELEMENT_NOT_FOUND`
- `ELEMENT_NOT_INTERACTABLE`
- `AMBIGUOUS_ELEMENT_MATCH`
- `CONTENT_TOO_LARGE`
- `INTERNAL_ERROR`

Рекомендуемое соответствие HTTP-статусов:

- `400` bad request
- `401` unauthorized
- `403` forbidden
- `404` not found
- `409` conflict
- `422` validation error
- `500` internal error
- `502` upstream or browser start error
- `504` timeout

### OpenAPI

Реализация должна предоставлять OpenAPI-спецификацию и интерактивную документацию, генерируемую из моделей API.
