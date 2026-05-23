# CLI Scripts / CLI-сценарии

## English

### Purpose

CLI scripts provide a simple declarative way to run browser automation without calling the HTTP API directly and without composing long CLI command lines.

They are intended for simple, repeatable scenarios such as:

- opening one or more pages;
- waiting for page load or DOM state changes;
- finding and interacting with elements;
- collecting page content, cookies, headers, text, or attributes;
- saving results to stdout or a file.

### Supported format

Scripts must be provided as `YAML` or `JSON` files.

Recommended command:

```bash
autobrowser run-script --file scenario.yaml
```

Validation without browser start:

```bash
autobrowser validate-script --file scenario.yaml
```

### Design rules

The script format is intentionally limited:

- declarative structure only;
- no loops;
- no conditional branches;
- no user-defined functions;
- one ordered list of steps.

This keeps the format predictable, easy to validate, and suitable for simple automation tasks.

### Top-level structure

A script may contain the following sections:

- `session` - browser session parameters;
- `steps` - ordered actions to execute;
- `output` - final result handling;
- `stopOnError` - stop on first failed step or continue.

Minimal example:

```yaml
session:
  headless: true
  profile: test1
  userAgent: Mozilla/5.0
  timeout: 10000

steps:
  - action: open
    url: https://example.com/login

  - action: fill
    locator:
      type: name
      value: username
    value: demo-user

  - action: fill
    locator:
      type: name
      value: password
    value: demo-password

  - action: click
    locator:
      type: css
      value: button[type='submit']

  - action: wait
    waitUntil: networkidle

  - action: get-content
    format: html

output:
  file: result.html
```

### `session`

Typical fields:

- `headless`
- `profile`
- `profileDir`
- `userAgent`
- `proxy` — proxy URL, e.g. `socks5://user:pass@host:port`
- `viewport` — browser viewport size, e.g. `{width: 1920, height: 1080}`
- `timeout`
- `preserveSession` — save open tabs to `tabs.json` on stop, restore them on next start (default: `false`)
- `fingerprint` — enable browser fingerprint spoofing (default: `null` / disabled). Accepts `true`, `false`, or an object with overrides.

Example:

```yaml
session:
  headless: true
  profile: test1
  profileDir: /data/profiles
  userAgent: Mozilla/5.0
  proxy: socks5://user:pass@host:port
  viewport:
    width: 1920
    height: 1080
  timeout: 10000
  preserveSession: true
  fingerprint: true
```

> The CLI flags `--proxy`, `--viewport`, and `--preserve-session` override the values set in the scenario file.

> When an Android User-Agent is specified, the browser context is automatically configured with `is_mobile: true` and `has_touch: true`, enabling mobile viewport behavior and touch event emulation.

#### Fingerprint overrides

The `fingerprint` field accepts three forms:

```yaml
# Enable with auto-generated config (stored in profile/fingerprint.json)
fingerprint: true

# Disable
fingerprint: false

# Override specific fields
fingerprint:
  seed: 12345
  canvas: true
  webgl: true
  webglVendor: "Google Inc. (NVIDIA)"
  webglRenderer: "ANGLE (NVIDIA, GeForce RTX 3060)"
  audio: true
  clientRects: true
  fonts: true
```

Available override fields: `seed`, `canvas`, `webgl`, `webglVendor`, `webglRenderer`, `audio`, `clientRects`, `fonts`.

### `steps`

Each step must include `action` and action-specific fields.

Supported actions:

| Action | Description | Required fields | Optional fields |
|--------|-------------|-----------------|-----------------|
| `open` | Navigate to URL | `url` | `waitUntil`, `timeout` |
| `wait` | Wait for event or timeout | `waitUntil` or `timeout` | `locator` (for `visible`/`hidden`) |
| `find` | Find element(s) | `locator` | `index`, `timeout` |
| `click` | Click element | `locator` | `index`, `timeout` |
| `fill` | Instantly fill a field | `locator`, `value` | `index`, `timeout` |
| `type` | Type character by character (human-like) | `locator`, `value` | `delay` (ms, default 50), `index`, `timeout` |
| `press` | Press a key | `key` | — |
| `scroll` | Scroll page | `deltaX` or `deltaY` | — |
| `screenshot` | Take a page screenshot | `file` | `fullPage` (default false) |
| `get-content` | Get page content | — | `format` (`html` or `text`) |
| `get-cookies` | Get cookies | — | — |
| `get-headers` | Get HTTP headers | — | — |
| `save` | Save content to file | `file` | `format` (`html` or `text`) |
| `browse-tabs` | Bring each tab to front, wait, scroll | — | `deltaY` (scroll amount, default 400), `timeout` |

All steps support optional `page` field to target a specific tab (e.g. `page: page_1`). Default is the first page.

All steps support optional `stopOnError` to override the global `stopOnError` setting.

### Locator format

DOM-related steps should use a unified locator object compatible with the HTTP API:

```yaml
locator:
  type: css
  value: button[type='submit']
```

Supported locator types:

- `css`
- `xpath`
- `id`
- `name`
- `class`
- `text`
- `attribute`

### Shorthand locator strings / Сокращённая запись локатора

**English**

Instead of the dict form you can pass the locator as a plain string. Parsing rules:

1. `type=value` — if the prefix before the first `=` is a known type, it is used directly.  
   Example: `"css=button[type=submit]"`, `"name=username"`, `"xpath=//div"`.
2. String starting with `//` or `/` → `xpath`.
3. String matching `^#[a-zA-Z_][\w-]*$` → `id` (leading `#` stripped).
4. String matching `^\.[a-zA-Z_][\w-]*$` → `class` (leading `.` stripped).
5. Anything else → `css`.

```yaml
# equivalent forms:
- action: click
  locator: "css=button[type=submit]"

- action: click
  locator: "#submit-btn"

- action: click
  locator: "//div[@id='x']"

# dict form still works:
- action: click
  locator:
    type: css
    value: button[type=submit]
```

---

**Русский**

Вместо объектной формы локатор можно указать строкой. Правила разбора:

1. `тип=значение` — если префикс до первого `=` является известным типом, он используется напрямую.  
   Пример: `"css=button[type=submit]"`, `"name=username"`, `"xpath=//div"`.
2. Строка начинается с `//` или `/` → `xpath`.
3. Строка соответствует `^#[a-zA-Z_][\w-]*$` → `id` (символ `#` удаляется).
4. Строка соответствует `^\.[a-zA-Z_][\w-]*$` → `class` (символ `.` удаляется).
5. Всё остальное → `css`.

Объектная форма по-прежнему работает без изменений.

### Validation

Scripts should be validated against JSON Schema before execution.

Validation must check:

- required fields;
- allowed action names;
- allowed locator types;
- allowed `waitUntil` values;
- field types and enum values;
- unexpected properties.

Validation errors should include:

- exit code;
- path to the invalid field;
- short human-readable error message.

### Execution result

Script execution result should include:

- overall `success` flag;
- total duration;
- per-step result;
- diagnostics for failed steps;
- output destination details when applicable.

## Русский

### Назначение

CLI-сценарии дают простой декларативный способ запускать автоматизацию браузера без прямого вызова HTTP API и без длинных команд с большим числом флагов.

Они предназначены для простых повторяемых сценариев, например:

- открыть одну или несколько страниц;
- дождаться загрузки страницы или изменения состояния DOM;
- найти элемент и выполнить действие;
- получить содержимое страницы, cookies, headers, текст или атрибуты;
- сохранить результат в stdout или файл.

### Поддерживаемый формат

Сценарии должны передаваться в виде файлов `YAML` или `JSON`.

Рекомендуемая команда запуска:

```bash
autobrowser run-script --file scenario.yaml
```

Проверка сценария без запуска браузера:

```bash
autobrowser validate-script --file scenario.yaml
```

### Правила проектирования

Формат сценария намеренно ограничен:

- только декларативная структура;
- без циклов;
- без условных ветвлений;
- без пользовательских функций;
- только один упорядоченный список шагов.

Это делает формат предсказуемым, удобным для валидации и подходящим для простых задач автоматизации.

### Структура верхнего уровня

Сценарий может содержать следующие секции:

- `session` - параметры браузерной сессии;
- `steps` - упорядоченные действия для выполнения;
- `output` - правила обработки итогового результата;
- `stopOnError` - остановка на первой ошибке или продолжение выполнения.

Минимальный пример:

```yaml
session:
  headless: true
  profile: test1
  userAgent: Mozilla/5.0
  timeout: 10000

steps:
  - action: open
    url: https://example.com/login

  - action: fill
    locator:
      type: name
      value: username
    value: demo-user

  - action: fill
    locator:
      type: name
      value: password
    value: demo-password

  - action: click
    locator:
      type: css
      value: button[type='submit']

  - action: wait
    waitUntil: networkidle

  - action: get-content
    format: html

output:
  file: result.html
```

### `session`

Типовые поля:

- `headless`
- `profile`
- `profileDir`
- `userAgent`
- `proxy` — URL прокси, например `socks5://user:pass@host:port`
- `viewport` — размер окна браузера, например `{width: 1920, height: 1080}`
- `timeout`
- `preserveSession` — сохранять открытые вкладки в `tabs.json` при остановке, восстанавливать при следующем запуске (по умолчанию: `false`)
- `fingerprint` — включить подмену отпечатка браузера (по умолчанию: `null` / выключено). Принимает `true`, `false` или объект с переопределениями.

Пример:

```yaml
session:
  headless: true
  profile: test1
  profileDir: /data/profiles
  userAgent: Mozilla/5.0
  proxy: socks5://user:pass@host:port
  viewport:
    width: 1920
    height: 1080
  timeout: 10000
  preserveSession: true
  fingerprint: true
```

> CLI-флаги `--proxy`, `--viewport` и `--preserve-session` имеют приоритет над значениями из файла сценария.

> При указании Android User-Agent контекст браузера автоматически настраивается с `is_mobile: true` и `has_touch: true`, включая мобильное поведение viewport и эмуляцию тач-событий.

#### Переопределение отпечатка

Поле `fingerprint` принимает три формы:

```yaml
# Включить с автогенерацией конфига (сохраняется в profile/fingerprint.json)
fingerprint: true

# Выключить
fingerprint: false

# Переопределить конкретные поля
fingerprint:
  seed: 12345
  canvas: true
  webgl: true
  webglVendor: "Google Inc. (NVIDIA)"
  webglRenderer: "ANGLE (NVIDIA, GeForce RTX 3060)"
  audio: true
  clientRects: true
  fonts: true
```

Доступные поля для переопределения: `seed`, `canvas`, `webgl`, `webglVendor`, `webglRenderer`, `audio`, `clientRects`, `fonts`.

### `steps`

Каждый шаг должен содержать поле `action` и набор параметров, зависящих от действия.

Поддерживаемые действия:

| Действие | Описание | Обязательные поля | Необязательные поля |
|----------|----------|-------------------|---------------------|
| `open` | Открыть URL | `url` | `waitUntil`, `timeout` |
| `wait` | Ждать событие или таймаут | `waitUntil` или `timeout` | `locator` (для `visible`/`hidden`) |
| `find` | Найти элемент(ы) | `locator` | `index`, `timeout` |
| `click` | Кликнуть по элементу | `locator` | `index`, `timeout` |
| `fill` | Мгновенно заполнить поле | `locator`, `value` | `index`, `timeout` |
| `type` | Посимвольный ввод (имитация человека) | `locator`, `value` | `delay` (мс, по умолчанию 50), `index`, `timeout` |
| `press` | Нажать клавишу | `key` | — |
| `scroll` | Прокрутить страницу | `deltaX` или `deltaY` | — |
| `screenshot` | Сделать скриншот страницы | `file` | `fullPage` (по умолчанию false) |
| `get-content` | Получить содержимое страницы | — | `format` (`html` или `text`) |
| `get-cookies` | Получить cookies | — | — |
| `get-headers` | Получить HTTP-заголовки | — | — |
| `save` | Сохранить содержимое в файл | `file` | `format` (`html` или `text`) |
| `browse-tabs` | Переключить каждый таб, подождать, прокрутить | — | `deltaY` (величина прокрутки, по умолчанию 400), `timeout` |

Каждый шаг поддерживает необязательное поле `page` для указания конкретной вкладки (например `page: page_1`). По умолчанию используется первая страница.

Каждый шаг поддерживает необязательное поле `stopOnError` для переопределения глобальной настройки `stopOnError`.

### Формат локатора

Шаги, работающие с DOM, должны использовать единый объект локатора, совместимый с HTTP API:

```yaml
locator:
  type: css
  value: button[type='submit']
```

Поддерживаемые типы локаторов:

- `css`
- `xpath`
- `id`
- `name`
- `class`
- `text`
- `attribute`

### Валидация

Сценарии должны проверяться по JSON Schema до выполнения.

Валидация должна проверять:

- обязательные поля;
- допустимые имена действий;
- допустимые типы локаторов;
- допустимые значения `waitUntil`;
- типы полей и значения перечислений;
- неожиданные свойства.

Ошибки валидации должны содержать:

- код завершения;
- путь к некорректному полю;
- короткое понятное описание ошибки.

### Результат выполнения

Результат выполнения сценария должен включать:

- общий признак `success`;
- общую длительность;
- результат по каждому шагу;
- диагностические сведения по ошибочным шагам;
- сведения о месте вывода результата, если оно задано.
