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
- `pages` - optional predefined pages;
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
- `proxy`
- `timeout`
- extra browser launch arguments

Example:

```yaml
session:
  headless: true
  profile: test1
  profileDir: /data/profiles
  userAgent: Mozilla/5.0
  timeout: 10000
```

### `steps`

Each step must include `action` and action-specific fields.

Supported action groups:

- navigation: `open`, `wait`
- element search: `find`
- element interaction: `click`, `dblclick`, `hover`, `focus`, `fill`, `clear`, `select`, `check`, `uncheck`, `press`, `submit`
- mouse actions: `move`, `scroll`, `move-random`
- data extraction: `get-content`, `get-cookies`, `get-headers`, `get-attributes`, `get-text`
- output: `save`

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
- `pages` - необязательное описание заранее открываемых страниц;
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
- `proxy`
- `timeout`
- дополнительные параметры запуска браузера

Пример:

```yaml
session:
  headless: true
  profile: test1
  profileDir: /data/profiles
  userAgent: Mozilla/5.0
  timeout: 10000
```

### `steps`

Каждый шаг должен содержать поле `action` и набор параметров, зависящих от действия.

Поддерживаемые группы действий:

- навигация: `open`, `wait`
- поиск элемента: `find`
- действия над элементом: `click`, `dblclick`, `hover`, `focus`, `fill`, `clear`, `select`, `check`, `uncheck`, `press`, `submit`
- действия мышью: `move`, `scroll`, `move-random`
- получение данных: `get-content`, `get-cookies`, `get-headers`, `get-attributes`, `get-text`
- вывод результата: `save`

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
