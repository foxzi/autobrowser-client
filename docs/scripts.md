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

- declarative structure only (no user-defined functions or embedded code, except the `evaluate` action);
- one ordered list of steps;
- flow control through blocks: `if`, `try`/`catch`/`finally`, `forEach`, `repeat`, `while`, `include` (see "Control blocks").

This keeps the format predictable and easy to validate.

### Top-level structure

A script may contain the following sections:

- `session` - browser session parameters;
- `steps` - ordered actions to execute;
- `output` - final result handling;
- `stopOnError` - stop on first failed step or continue;
- `vars` - variables (defaults), see "Variables";
- `errorArtifacts` - directory for screenshots and HTML of failed steps.

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
| `save` | Save page content, or `value` text, to a file | `file` | `format` (`html` or `text`), `value` (text to write instead of the page, e.g. `"${results.text}"`) |
| `browse-tabs` | Bring each tab to front, wait, scroll | — | `deltaY` (scroll amount, default 400), `timeout` |
| `solve-challenge` | Wait for a Cloudflare challenge to pass, clicking the Turnstile checkbox; fails on timeout or final HTTP >= 400 (error says `blocked without challenge` or `blocked after challenge`) | — | `timeout` (default 60000) |

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

### Shorthand step syntax / Сокращённая запись шагов

**English**

Instead of the full `{action: …, field: value}` dict form you can write the action name as the YAML key and pass its primary argument as the value. Both forms may coexist in the same file.

| Shorthand | Expands to |
|---|---|
| `- open: <url>` | `{action: open, url: <url>}` |
| `- wait: <ms>` (integer) | `{action: wait, timeout: <ms>}` |
| `- wait: load\|domcontentloaded\|networkidle` | `{action: wait, waitUntil: <state>}` |
| `- click: <locator-str>` | `{action: click, locator: <locator-str>}` |
| `- find: <locator-str>` | `{action: find, locator: <locator-str>}` |
| `- fill: [<locator>, <value>]` | `{action: fill, locator: <loc>, value: <val>}` |
| `- type: [<locator>, <value>]` | `{action: type, locator: <loc>, value: <val>}` |
| `- press: <key>` | `{action: press, key: <key>}` |
| `- screenshot: <file>` | `{action: screenshot, file: <file>}` |
| `- save: <file>` | `{action: save, file: <file>}` |
| `- get-content: ~` | `{action: get-content}` |
| `- get-cookies: ~` | `{action: get-cookies}` |
| `- get-headers: ~` | `{action: get-headers}` |
| `- browse-tabs: ~` | `{action: browse-tabs}` |
| `- solve-challenge: ~` | `{action: solve-challenge}` |

Extra step-level options (`timeout`, `stopOnError`, `fullPage`, etc.) can be added alongside the shorthand key:

```yaml
steps:
  # simple open
  - open: https://example.com

  # wait for network idle
  - wait: networkidle

  # wait 2 seconds
  - wait: 2000

  # click by CSS shorthand
  - click: "#submit-btn"

  # fill using string locator shorthand; both steps are equivalent
  - fill: ["name=username", "demo-user"]
  - action: fill
    locator: name=username
    value: demo-user

  # screenshot with extra option
  - screenshot: out.png
    fullPage: true

  # null-value actions
  - get-content: ~
  - get-cookies: ~
  - get-headers: ~
```

The `<locator-str>` value uses the same string parsing rules described in "Shorthand locator strings" above.

---

**Русский**

Вместо полной объектной формы `{action: …, field: value}` можно записать имя действия как ключ YAML, а его основной аргумент — как значение. Обе формы могут использоваться в одном файле.

| Сокращение | Разворачивается в |
|---|---|
| `- open: <url>` | `{action: open, url: <url>}` |
| `- wait: <мс>` (целое число) | `{action: wait, timeout: <мс>}` |
| `- wait: load\|domcontentloaded\|networkidle` | `{action: wait, waitUntil: <state>}` |
| `- click: <локатор>` | `{action: click, locator: <локатор>}` |
| `- find: <локатор>` | `{action: find, locator: <локатор>}` |
| `- fill: [<локатор>, <значение>]` | `{action: fill, locator: …, value: …}` |
| `- type: [<локатор>, <значение>]` | `{action: type, locator: …, value: …}` |
| `- press: <клавиша>` | `{action: press, key: <клавиша>}` |
| `- screenshot: <файл>` | `{action: screenshot, file: <файл>}` |
| `- save: <файл>` | `{action: save, file: <файл>}` |
| `- get-content: ~` | `{action: get-content}` |
| `- get-cookies: ~` | `{action: get-cookies}` |
| `- get-headers: ~` | `{action: get-headers}` |
| `- browse-tabs: ~` | `{action: browse-tabs}` |
| `- solve-challenge: ~` | `{action: solve-challenge}` |

Дополнительные поля шага (`timeout`, `stopOnError`, `fullPage` и т.д.) можно добавлять рядом с ключом сокращения. Объектная форма по-прежнему работает без изменений.

### Additional actions

| Action | Description | Fields | Result `data` |
|--------|-------------|--------|---------------|
| `hover`, `dblclick`, `focus`, `clear`, `check`, `uncheck`, `submit` | Act on an element | `locator` | - |
| `select` | Choose in a `<select>` | `locator`, `value` (one) or `options` (list) | `{selected}` |
| `upload` | Attach files to a file input | `locator`, `files` (list of paths) | `{files}` |
| `evaluate` | Run JS (expression or function source) | `script` | `{result}` |
| `history` | Navigate history | `direction`: `back`, `forward`, `reload` | `{url}` |
| `wait-text` | Wait for text in the page | `text`, `gone` (wait until it disappears, default false) | - |
| `extract` | Read text or an attribute | `locator`, `attribute` (default: inner text), `all` (list of all matches) | `{value}` |
| `find-all`, `read` | List matching elements / read an element | `locator` | as in the HTTP API |
| `mouse-move` | Move the mouse | `x`, `y`, `steps` | as in the HTTP API |
| `mouse-scroll-to` | Scroll an element into view | `locator` | as in the HTTP API |
| `mouse-random` | Human-like random mouse path | `endX`, `endY`; optional `startX`, `startY`, `steps`, `durationMs`, `jitter`, `seed` | as in the HTTP API |
| `set` | Set variables | `vars` (mapping) | `{vars}` |
| `assert` | Fail unless a condition holds | `condition`, `message` (optional) | `{condition: true}` |
| `fail` | Always fail with a message (catchable) | `message` | - |

`press` takes an optional `locator`: then the key is pressed on that element. `type` waits a random delay in `[delay*0.5, delay*1.5]` between characters. `wait` with `waitUntil: visible|hidden` requires a `locator`.

Shorthand forms: `hover`/`dblclick`/`focus`/`clear`/`check`/`uncheck`/`submit`/`extract`/`find-all`/`read: <locator>`; `select: [<locator>, <option or list>]`; `upload: [<locator>, <file or list>]`; `evaluate: <script>`; `history: <direction>`; `wait-text: <text>`; `set: {name: value}`; `assert: <condition>`; `fail: <message>`.

### Variables

Variables are defined in the root `vars` mapping (defaults) and overridden from the command line: `autobrowser run-script -f s.yaml --var user=me --var pass=secret`.

- `${name}` is substituted at execution time in every string field of a step: `url`, `value`, `key`, `file`, `script`, `message`, locator value, condition strings, items of `files`. Numeric fields (`timeout`) are not substituted.
- `${env.NAME}` reads the environment variable `NAME`. Dotted paths index dicts and lists: `${item.url}`, `${error.message}`, `${rows.0}`.
- A string that is exactly one `${...}` keeps the type of the value (list, dict, number); otherwise the value is interpolated as text.
- `$${` is a literal `${`. An undefined variable is a step error: `undefined variable: name`.
- Variables are global for the run. The final variables (without environment) are in `vars` of the result.
- A locator written as a string with `${...}` is parsed after substitution, so a variable can hold `xpath=//a` or `text=Next`.
- `as: name` stores the result of a value-producing action (`extract` -> value, `evaluate` -> result, `read`, `find-all`, `get-content`, `get-cookies`, `get-headers` -> the whole `data`) into the variable `name`. The name is a plain identifier (`[A-Za-z_][A-Za-z0-9_]*`), not a template: `as: first_${index}` is a validation error. Inside a loop the variable is overwritten on every iteration. `set` sets variables directly.

```yaml
vars:
  base: https://quotes.toscrape.com
steps:
  - open: "${base}"
  - action: extract
    locator: h1
    as: title
  - assert: {var: title, contains: Quotes}
```

### Conditions

Used by `if`, `while` and `assert`. A condition is a mapping with exactly one of:

| Key | True when |
|-----|-----------|
| `exists: <locator>` | the locator matches at least one element |
| `visible: <locator>` | the first match is visible |
| `text: <str>` | the page text contains the string |
| `url: <str>` / `urlMatches: <regex>` | the URL contains the string / matches the regex |
| `title: <str>` | the title contains the string |
| `var: <name>` | the variable is truthy (undefined = false); with `equals`, `contains` or `matches` it is compared |
| `not: <condition>` | the inner condition is false |
| `all: [..]` / `any: [..]` | all / any of the conditions are true |

A leaf condition accepts `timeout: <ms>`: it is polled every 250 ms until true or until the time is up. The default is `0`: one check, no waiting. Checking a condition never fails because an element is missing.

### Step options

Any action step accepts:

- `name` - a label copied into the result;
- `retry: N` - extra attempts after a failure (default 0), `retryDelay: ms` between them (default 1000); the result has `attempts`;
- `optional: true` - a failure is recorded (`success: true`, `optional: true`, `error`) and never stops the script;
- `stopOnError` - override the root setting for this step;
- `page` - target tab.

### Control blocks

A block is a step without `action`, recognised by its key. Blocks accept `name`.

```yaml
steps:
  - if: {exists: "#cookie-accept"}
    then:
      - click: "#cookie-accept"
    else:                      # optional
      - wait: 100

  - try:
      - click: "#maybe"
    catch:                     # optional; variable "error" = {message, action, name}
      - screenshot: failed.png
    finally:                   # optional, always runs
      - get-cookies: ~

  - forEach: ["a", "b"]        # or "${list_var}"
    as: item                   # default "item"; variable "index" is the 0-based position
    do:
      - open: "https://example.com/${item}"

  - repeat: 3                  # variable "index"
    do:
      - action: scroll
        deltaY: 400

  - while: {exists: "a.next"}
    max: 50                    # default 100; exceeding it is an error
    do:
      - click: "a.next"

  - include: common/login.yaml
```

`include` inserts the `steps` of another file at load time; its `vars` are added as defaults under the current ones. The path is relative to the including file and must stay inside its directory (absolute paths, paths escaping the directory and include cycles are validation errors). For inline scripts (MCP, HTTP API) the base directory is the working directory.

Failure semantics follow exceptions:

- A step that fails (after retries, not `optional`) with an effective `stopOnError: true` raises a failure; with `false` it is recorded and execution continues.
- `try` catches a failure raised inside it: `catch` runs with the `error` variable and the failure is marked `handled: true`. Without `catch` the failure propagates after `finally`. A failure inside `catch` or `finally` propagates.
- `fail` raises immediately and can be caught.
- An unhandled failure at top level stops the script. Script `success` is true when there is no unhandled failure and no recorded failure; handled failures do not count.

### Error artifacts

`errorArtifacts: <dir>` in the scenario, or `autobrowser run-script --artifacts DIR`: when a non-optional step fails (last attempt), a viewport screenshot and the page HTML are saved as `<dir>/<NNN>-<action>.png` and `.html` (the counter grows during the run). The step result gets `artifacts: {screenshot, html}`. The paths follow the same rules as other output files (inside the working directory). A failure to save artifacts is logged and never raised.

### Editor support

`scenario.schema.json` in the repository root is a JSON Schema (draft 2020-12) for scenario files: completion, hover docs and error highlighting for every action, shorthand step, condition and block. It is for editors only; `autobrowser validate-script` uses the pydantic models.

Per file (VS Code with the Red Hat YAML extension, Neovim/Helix with yaml-language-server), first line of the scenario:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/foxzi/autobrowser-client/main/scenario.schema.json
```

For a whole folder, in VS Code `settings.json`:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/foxzi/autobrowser-client/main/scenario.schema.json": ["examples/*.yaml", "scenarios/**/*.yaml"]
  }
}
```

JetBrains IDEs: Settings -> Languages & Frameworks -> Schemas and DTDs -> JSON Schema Mappings, add the file or URL and the file patterns. A local path (`./scenario.schema.json`) works instead of the URL inside this repository.

### Validation

`autobrowser validate-script -f s.yaml` loads the scenario without starting a browser. `run-script`, `browser_run_script` (MCP) and `POST /api/v1/session/{id}/script` run the same check before the first step, so an invalid scenario never runs partially.

The check uses the pydantic models of the client (not `scenario.schema.json`) and covers:

- action names, required fields of each action (`open` needs `url`, `click` needs `locator`, ...) and unknown keys;
- locator types and shorthand strings, `waitUntil` values, field types;
- shorthand steps, conditions (exactly one primary key) and blocks (`try` needs `catch` or `finally`, `as` only on value-producing actions);
- `include`: the file exists, stays inside the directory, no cycles.

`${...}` values are not resolved at this point: an undefined variable or a locator built from a variable fails only when the step runs.

On success it prints `{"success": true, "steps": N, "stopOnError": bool}` (exit code 0). On error it prints to stderr and exits with code 1:

```json
{"success": false, "error": {"code": "SCRIPT_VALIDATION_ERROR", "message": "1 validation error for ScriptDefinition\nsteps.0.step\n  Value error, 'open' step requires 'url' ..."}}
```

The message contains the path to the invalid field (`steps.0.step` - first step). MCP returns the same text as a tool error, the HTTP API as a 400 response with the same `code`.

### Execution result

`run-script` prints JSON `{success, sessionId, profile, steps, vars}` (`vars` - final variables); exit code 1 when `success` is false. Flat steps are `{action, success, durationMs, data, error}` plus `name`, `attempts`, `optional`, `handled`, `artifacts` when relevant. Blocks produce nested nodes:

- `if`: `{action: "if", success, condition: bool, branch: "then"|"else"|null, steps: [...]}`
- `try`: `{action: "try", success, steps: [...], catch: [...], finally: [...]}`
- `forEach` / `repeat` / `while`: `{action, success, iterations: [[...], ...]}`
- `include` is flattened at load time and has no result node.

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

- только декларативная структура (без пользовательских функций и встроенного кода, кроме действия `evaluate`);
- один упорядоченный список шагов;
- управление потоком через блоки: `if`, `try`/`catch`/`finally`, `forEach`, `repeat`, `while`, `include` (см. «Блоки управления»).

Это делает формат предсказуемым и удобным для валидации.

### Структура верхнего уровня

Сценарий может содержать следующие секции:

- `session` - параметры браузерной сессии;
- `steps` - упорядоченные действия для выполнения;
- `output` - правила обработки итогового результата;
- `stopOnError` - остановка на первой ошибке или продолжение выполнения;
- `vars` - переменные (значения по умолчанию), см. «Переменные»;
- `errorArtifacts` - каталог для скриншотов и HTML упавших шагов.

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
| `save` | Сохранить содержимое страницы или текст `value` в файл | `file` | `format` (`html` или `text`), `value` (текст вместо страницы, например `"${results.text}"`) |
| `browse-tabs` | Переключить каждый таб, подождать, прокрутить | — | `deltaY` (величина прокрутки, по умолчанию 400), `timeout` |
| `solve-challenge` | Дождаться прохождения Cloudflare challenge, кликая чекбокс Turnstile; ошибка по таймауту или при итоговом HTTP >= 400 (в тексте ошибки `blocked without challenge` или `blocked after challenge`) | — | `timeout` (по умолчанию 60000) |

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

### Дополнительные действия

| Действие | Описание | Поля | `data` результата |
|----------|----------|------|-------------------|
| `hover`, `dblclick`, `focus`, `clear`, `check`, `uncheck`, `submit` | Действие с элементом | `locator` | - |
| `select` | Выбор в `<select>` | `locator`, `value` (один) или `options` (список) | `{selected}` |
| `upload` | Прикрепить файлы к file input | `locator`, `files` (список путей) | `{files}` |
| `evaluate` | Выполнить JS (выражение или код функции) | `script` | `{result}` |
| `history` | Навигация по истории | `direction`: `back`, `forward`, `reload` | `{url}` |
| `wait-text` | Дождаться текста на странице | `text`, `gone` (ждать исчезновения, по умолчанию false) | - |
| `extract` | Прочитать текст или атрибут | `locator`, `attribute` (по умолчанию: текст), `all` (список всех совпадений) | `{value}` |
| `find-all`, `read` | Список подходящих элементов / чтение элемента | `locator` | как в HTTP API |
| `mouse-move` | Переместить мышь | `x`, `y`, `steps` | как в HTTP API |
| `mouse-scroll-to` | Прокрутить элемент в область видимости | `locator` | как в HTTP API |
| `mouse-random` | Случайная «человеческая» траектория мыши | `endX`, `endY`; необязательно `startX`, `startY`, `steps`, `durationMs`, `jitter`, `seed` | как в HTTP API |
| `set` | Задать переменные | `vars` (словарь) | `{vars}` |
| `assert` | Упасть, если условие не выполнено | `condition`, `message` (необязательно) | `{condition: true}` |
| `fail` | Всегда падает с сообщением (можно перехватить) | `message` | - |

У `press` есть необязательный `locator`: тогда клавиша нажимается на этом элементе. `type` ждёт случайную паузу в диапазоне `[delay*0.5, delay*1.5]` между символами. `wait` с `waitUntil: visible|hidden` требует `locator`.

Сокращённая запись: `hover`/`dblclick`/`focus`/`clear`/`check`/`uncheck`/`submit`/`extract`/`find-all`/`read: <локатор>`; `select: [<локатор>, <значение или список>]`; `upload: [<локатор>, <файл или список>]`; `evaluate: <скрипт>`; `history: <направление>`; `wait-text: <текст>`; `set: {имя: значение}`; `assert: <условие>`; `fail: <сообщение>`.

### Переменные

Переменные задаются в корневом `vars` (значения по умолчанию) и переопределяются из командной строки: `autobrowser run-script -f s.yaml --var user=me --var pass=secret`.

- `${name}` подставляется во время выполнения во все строковые поля шага: `url`, `value`, `key`, `file`, `script`, `message`, значение локатора, строки условий, элементы `files`. Числовые поля (`timeout`) не подставляются.
- `${env.NAME}` читает переменную окружения `NAME`. Точечные пути индексируют словари и списки: `${item.url}`, `${error.message}`, `${rows.0}`.
- Строка, состоящая ровно из одного `${...}`, сохраняет тип значения (список, словарь, число); иначе значение вставляется как текст.
- `$${` - буквальное `${`. Неопределённая переменная - ошибка шага: `undefined variable: name`.
- Переменные глобальны на весь запуск. Итоговые переменные (без окружения) попадают в `vars` результата.
- Локатор-строка с `${...}` разбирается после подстановки, поэтому переменная может содержать `xpath=//a` или `text=Next`.
- `as: name` сохраняет результат действия в переменную `name` (`extract` -> значение, `evaluate` -> результат, `read`, `find-all`, `get-content`, `get-cookies`, `get-headers` -> весь `data`). Имя - простой идентификатор (`[A-Za-z_][A-Za-z0-9_]*`), а не шаблон: `as: first_${index}` - ошибка валидации. В цикле переменная перезаписывается на каждой итерации. `set` задаёт переменные напрямую.

```yaml
vars:
  base: https://quotes.toscrape.com
steps:
  - open: "${base}"
  - action: extract
    locator: h1
    as: title
  - assert: {var: title, contains: Quotes}
```

### Условия

Используются в `if`, `while` и `assert`. Условие - словарь ровно с одним из ключей:

| Ключ | Истинно, когда |
|------|----------------|
| `exists: <локатор>` | локатор находит хотя бы один элемент |
| `visible: <локатор>` | первое совпадение видимо |
| `text: <строка>` | текст страницы содержит строку |
| `url: <строка>` / `urlMatches: <regex>` | URL содержит строку / соответствует regex |
| `title: <строка>` | заголовок содержит строку |
| `var: <имя>` | переменная истинна (не задана = ложь); с `equals`, `contains` или `matches` сравнивается |
| `not: <условие>` | вложенное условие ложно |
| `all: [..]` / `any: [..]` | все / любое из условий истинны |

У простого условия есть `timeout: <мс>`: оно проверяется каждые 250 мс, пока не станет истинным или не истечёт время. По умолчанию `0`: одна проверка без ожидания. Отсутствие элемента никогда не приводит к ошибке проверки.

### Опции шага

Любой шаг-действие принимает:

- `name` - метка, копируется в результат;
- `retry: N` - дополнительные попытки после ошибки (по умолчанию 0), `retryDelay: мс` между ними (по умолчанию 1000); в результате есть `attempts`;
- `optional: true` - ошибка записывается (`success: true`, `optional: true`, `error`) и никогда не останавливает сценарий;
- `stopOnError` - переопределить корневую настройку для шага;
- `page` - целевая вкладка.

### Блоки управления

Блок - шаг без `action`, определяется по ключу. Блоки принимают `name`.

```yaml
steps:
  - if: {exists: "#cookie-accept"}
    then:
      - click: "#cookie-accept"
    else:                      # необязательно
      - wait: 100

  - try:
      - click: "#maybe"
    catch:                     # необязательно; переменная "error" = {message, action, name}
      - screenshot: failed.png
    finally:                   # необязательно, выполняется всегда
      - get-cookies: ~

  - forEach: ["a", "b"]        # или "${list_var}"
    as: item                   # по умолчанию "item"; переменная "index" - позиция с нуля
    do:
      - open: "https://example.com/${item}"

  - repeat: 3                  # переменная "index"
    do:
      - action: scroll
        deltaY: 400

  - while: {exists: "a.next"}
    max: 50                    # по умолчанию 100; превышение - ошибка
    do:
      - click: "a.next"

  - include: common/login.yaml
```

`include` на этапе загрузки вставляет `steps` другого файла; его `vars` добавляются как значения по умолчанию под текущие. Путь задаётся относительно включающего файла и должен оставаться внутри его каталога (абсолютные пути, выход за каталог и циклы include - ошибки валидации). Для встроенных сценариев (MCP, HTTP API) базовый каталог - рабочий.

Семантика ошибок как у исключений:

- Шаг, упавший (после повторов, не `optional`) при действующем `stopOnError: true`, вызывает сбой; при `false` ошибка записывается и выполнение продолжается.
- `try` перехватывает сбой внутри себя: выполняется `catch` с переменной `error`, сбой помечается `handled: true`. Без `catch` сбой идёт дальше после `finally`. Сбой внутри `catch` или `finally` идёт дальше.
- `fail` падает сразу, его можно перехватить.
- Необработанный сбой на верхнем уровне останавливает сценарий. `success` сценария истинен, если нет необработанных и записанных сбоев; обработанные не считаются.

### Артефакты ошибок

`errorArtifacts: <каталог>` в сценарии или `autobrowser run-script --artifacts DIR`: при падении неопционального шага (последняя попытка) сохраняются скриншот области просмотра и HTML страницы как `<каталог>/<NNN>-<действие>.png` и `.html` (счётчик растёт в течение запуска). В результате шага появляется `artifacts: {screenshot, html}`. Пути подчиняются тем же правилам, что и другие выходные файлы (внутри рабочего каталога). Ошибка сохранения артефактов логируется и не пробрасывается.

### Поддержка в редакторах

`scenario.schema.json` в корне репозитория - JSON Schema (draft 2020-12) для файлов сценариев: автодополнение, подсказки и подсветка ошибок для всех действий, сокращённых шагов, условий и блоков. Схема нужна только редакторам; `autobrowser validate-script` проверяет по pydantic-моделям.

Для одного файла (VS Code с расширением Red Hat YAML, Neovim/Helix с yaml-language-server) первая строка сценария:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/foxzi/autobrowser-client/main/scenario.schema.json
```

Для папки целиком, в `settings.json` VS Code:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/foxzi/autobrowser-client/main/scenario.schema.json": ["examples/*.yaml", "scenarios/**/*.yaml"]
  }
}
```

JetBrains IDE: Settings -> Languages & Frameworks -> Schemas and DTDs -> JSON Schema Mappings, добавьте файл или URL и шаблоны файлов. Внутри этого репозитория вместо URL подходит локальный путь (`./scenario.schema.json`).

### Валидация

`autobrowser validate-script -f s.yaml` загружает сценарий без запуска браузера. `run-script`, `browser_run_script` (MCP) и `POST /api/v1/session/{id}/script` выполняют ту же проверку до первого шага, поэтому некорректный сценарий не выполняется частично.

Проверка идёт по pydantic-моделям клиента (не по `scenario.schema.json`) и охватывает:

- имена действий, обязательные поля каждого действия (`open` требует `url`, `click` - `locator`, ...) и неизвестные ключи;
- типы локаторов и сокращённые строки, значения `waitUntil`, типы полей;
- сокращённые шаги, условия (ровно один основной ключ) и блоки (`try` требует `catch` или `finally`, `as` только у действий, возвращающих значение);
- `include`: файл существует, не выходит за каталог, нет циклов.

Значения `${...}` на этом этапе не подставляются: неопределённая переменная или локатор из переменной дают ошибку только при выполнении шага.

При успехе печатается `{"success": true, "steps": N, "stopOnError": bool}` (код выхода 0). При ошибке JSON выводится в stderr, код выхода 1:

```json
{"success": false, "error": {"code": "SCRIPT_VALIDATION_ERROR", "message": "1 validation error for ScriptDefinition\nsteps.0.step\n  Value error, 'open' step requires 'url' ..."}}
```

Сообщение содержит путь к некорректному полю (`steps.0.step` - первый шаг). MCP возвращает тот же текст как ошибку инструмента, HTTP API - как ответ 400 с тем же `code`.

### Результат выполнения

`run-script` печатает JSON `{success, sessionId, profile, steps, vars}` (`vars` - итоговые переменные); код выхода 1, если `success` ложен. Плоские шаги: `{action, success, durationMs, data, error}` плюс `name`, `attempts`, `optional`, `handled`, `artifacts` при необходимости. Блоки дают вложенные узлы:

- `if`: `{action: "if", success, condition: bool, branch: "then"|"else"|null, steps: [...]}`
- `try`: `{action: "try", success, steps: [...], catch: [...], finally: [...]}`
- `forEach` / `repeat` / `while`: `{action, success, iterations: [[...], ...]}`
- `include` разворачивается при загрузке и узла результата не имеет.
