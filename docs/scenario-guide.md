# Scenario guide: variables, templates, JS / Руководство по сценариям: переменные, шаблоны, JS

Practical companion to [scripts.md](scripts.md) (the reference of all actions and fields).
Every behaviour below was checked by running scenarios.

Практическое дополнение к [scripts.md](scripts.md) (справочник всех действий и полей).
Всё описанное ниже проверено запуском сценариев.

- [English](#english)
- [Русский](#русский)

---

## English

### 1. Running a scenario

```bash
autobrowser validate-script -f examples/google-search.yaml      # check without a browser
autobrowser run-script -f examples/google-search.yaml \
  -p youtube \                                 # saved profile (cookies, logins)
  --proxy socks5://127.0.0.1:1080 \            # overrides session.proxy
  --var "query=autobrowser github" \           # override vars (repeatable)
  --headed \                                   # show the window
  -o result.json                               # JSON result to a file instead of stdout
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run-script -f s.yaml   # choose the engine
```

The result is JSON `{success, sessionId, profile, steps, vars}`; the exit code is 1 when `success` is false.
Read a value from it: `jq '.vars.results.items' result.json`.

### 2. Variables: where they come from

| Source | Example | Notes |
|--------|---------|-------|
| `vars` in the file | `vars: {query: test}` | defaults, any YAML type |
| `--var NAME=VALUE` | `--var n=3` | overrides `vars`; the value is **always a string** (`"3"`, not `3`) |
| `vars` of an included file | `include: common/login.yaml` | added as defaults below the current ones |
| `as: name` on a step | `evaluate` ... `as: results` | stored only when the step succeeded |
| `set` step | `set: {page: 1}` | sets or overwrites variables |
| loops | `forEach` -> `item` (or `as:`), `index` | `index` is set by `forEach`, `repeat`, `while` |
| `try`/`catch` | `error` = `{message, action, name}` | set before `catch` runs |
| environment | `${env.HOME}` | read-only, not copied into `vars` of the result |

All variables are **global for the run**: there is no block scope. `item`, `index` and `error` stay
defined after their loop or `catch` and are overwritten by the next loop or `catch`.
Use different `as:` names for nested loops, otherwise the inner loop overwrites the outer `item`;
`index` is always the innermost loop's position.

What `as:` stores:

| Action | Stored value |
|--------|--------------|
| `extract` | `data.value` (string, or list with `all: true`) |
| `evaluate` | `data.result` (what the JS returned) |
| `read`, `find-all`, `get-content`, `get-cookies`, `get-headers` | the whole `data` |

If an `optional: true` step fails, its `as:` variable is **not** set. A later `${name}` then fails
with `undefined variable: name`; guard with `if: {var: name}` (an undefined variable is false there).

### 3. Templates `${...}`

Substitution happens right before a step runs, in every string field of the step (including
`script` of `evaluate`, `locator`, `file`, `value`, `url`, `message`), in conditions of `if`/`while`/`assert`,
in `forEach` and `repeat`, and in the values of `set`.
It does not happen in `as` (a plain name) and in the `include` path (resolved at load time).

| Syntax | Meaning |
|--------|---------|
| `${name}` | variable |
| `${a.b.c}` | key of a dict |
| `${rows.0}`, `${results.items.0.url}` | element of a list by index |
| `${env.NAME}` | environment variable (undefined if not set) |
| `$${` | literal `${` (no substitution) |

Two modes:

1. **The whole string is one template** (`"${obj}"`): the value keeps its type: list, dict, number,
   bool. This is how `forEach: "${links}"` gets a list and `set: {copy: "${obj}"}` copies a dict.
2. **Template inside text** (`"page ${n} of ${total}"`): the value is converted to text with
   Python `str()`. Strings and numbers look as expected, but a dict becomes
   `{'a': 1, 'b': True, 'c': None}` and a list `['x', 'y']`: Python syntax, **not JSON**.
   To put a structure into text, build the string in JS first (`JSON.stringify(...)`) and store it with `as:`.

Type pitfalls:

- A dict or list put as a whole into a field that expects text (`value: "${obj}"` of `save`/`type`)
  is a step error `invalid value after substitution: value: Input should be a valid string`.
  Like any step error it can be caught with `try`, skipped with `optional: true`, and it respects
  `stopOnError`. Numbers are fine (`value: "${n}"` writes `3`).
- `--var n=3` gives the string `"3"`: `if: {var: n, equals: 3}` is false, `equals: "3"` is true.
  `repeat: "${n}"` accepts both.
- `equals` compares with the YAML type: `equals: yes` is the boolean `true` (YAML 1.1), quote it: `"yes"`.
- Templates are not resolved by `validate-script`: an undefined variable shows up only at run time
  as a step error `undefined variable: name`.
- A locator string is parsed after substitution, so `click: "${target}"` with
  `--var "target=text=Next"` uses a text locator.

```yaml
vars:
  base: https://quotes.toscrape.com
  pages: [1, 2, 3]
steps:
  - forEach: "${pages}"                         # whole-string template -> list
    as: p                                       # index = 0, 1, 2
    do:
      - open: "${base}/page/${p}/"
      - action: extract
        locator: ".quote .text"
        all: true
        as: quotes
      - forEach: "${quotes}"                    # whole-string template -> list
        as: q
        do:
          - set: {last: "${q}"}
  - assert: {var: last}
```

### 4. JavaScript in `evaluate`

#### 4.1 What can be written in `script`

| Form | Example | Result |
|------|---------|--------|
| expression | `document.title + '!'` | value of the expression |
| several statements | `const a = 2; a * 2` | value of the last statement |
| function | `() => document.title` | return value |
| async function | `async () => { await new Promise(r => setTimeout(r, 500)); return 1 }` | awaited result |

- The function gets **no arguments**: pass data through templates (see 4.3).
- JS runs in the **main world** of the page: page globals (`window.jQuery`, app state) are visible,
  and the page sees changes you make.
- Long scripts: use a YAML block `script: |` (no quoting problems, line breaks kept).
- Put your own deadline into polling loops (as in
  `examples/google-search.yaml`: `while (Date.now() - t0 < 20000 && ...)`).

#### 4.2 What comes back

The result must be JSON-like. Checked conversions:

| JS value | `data.result` |
|----------|---------------|
| `undefined`, nothing returned | `null` |
| number, string, bool, plain object, array | as is |
| `Infinity` | `Infinity` |
| `Date` | string `1970-01-01 00:00:00+00:00` |
| `Map`, `Set` | `{}` (data lost) - convert with `Object.fromEntries(...)` / `[...set]` |
| DOM element | string `ref: <Node>` (useless) - return `el.innerText`, `el.href` etc. |

Return one object with everything you need, and store it with `as:`; then use `${results.items}`,
`${results.captcha}` in later steps.

#### 4.3 `${` inside JS: the most common trap

Templates are substituted in `script` **before** the JS runs. A JS template literal
`` `${x.title}` `` is therefore taken as a scenario variable and the step fails with
`undefined variable: x.title`. Options:

```js
// 1. escape: $${ becomes ${ and JS gets its template literal
() => { const i = 1; return `n=$${i + 1}`; }           // -> "n=2"
// 2. plain concatenation
items.map((x, i) => (i + 1) + '. ' + x.title).join('\n')
```

`$` alone is not special: `$('a')`, `$x(...)`, `/\$\{/` regexes work unchanged.

#### 4.4 Passing variables into JS

The value is inserted as raw text, **without escaping**:

```yaml
- action: evaluate
  script: "() => document.title.includes('${word}')"
```

- Works for controlled values (numbers, words, selectors without quotes).
- A value with a quote (`it's "quoted"`) breaks the JS: `SyntaxError`.
- A dict or list is inserted in Python syntax (`{'a': 1, 'b': True}`) and is not valid JS.
- Ways out:
  - keep user text out of JS: put it into the page with `type`/`fill` (`value: "${query}"`) and
    read it in JS from the DOM;
  - pass a URL-encoded value and decode it: `--var "q=it%27s"`, JS `decodeURIComponent('${q}')`;
  - for structures produced by a previous `evaluate`, return a JSON string there
    (`return JSON.stringify(obj)`, `as: obj_json`) and insert it as a JS literal:
    `() => { const o = ${obj_json}; return o.a; }` (JSON is valid JS).

#### 4.5 Navigation and timing

- If the page navigates while JS runs (a click or `press: Enter` started a load), the step fails with
  `Execution context was destroyed, most likely because of a navigation`. After an action that
  navigates, wait first: `wait: load`, or `action: wait` with `waitUntil: visible` and a `locator`
  of the element of the new page, then `evaluate`.
- `waitUntil: visible` with a comma-separated locator waits for whichever appears first
  (results **or** captcha); add `index: 0`, since several elements may match.
- Values read by JS from the page are a snapshot: re-run `evaluate` after the page changes.

### 5. Writing files

| Step | Writes |
|------|--------|
| `screenshot: out.png` (`fullPage: true` for the whole page) | PNG |
| `save` with `file` | page HTML (`format: html`, default) or page text (`format: text`) |
| `save` with `file` and `value` | the given text (since 0.5.3): `value: "${results.text}"` |
| `output.file` at the top level / `-o` | the whole JSON result |

- Paths are **relative to the working directory** and must stay inside it: absolute paths and `..`
  leading outside are rejected. Missing directories are created. Existing files are overwritten.
- The file name can be a template: `file: "output/${query}.txt"` (keep the value file-name safe).
- To save several lines, build the text in `evaluate`, e.g.
  `text: items.map((x, i) => (i + 1) + '. ' + x.title + '\n   ' + x.url).join('\n') + '\n'`.
- To save JSON, return `JSON.stringify(data, null, 2)` from `evaluate` and save that string.
- Put screenshots into `finally`: they are taken even when a step failed.

### 6. YAML details

- Quote locators that start with `#`: unquoted `#id` is a YAML comment.
- Quote values with `: `, `{`, `[`, or values like `yes`, `no`, `on`, `off`, `null`, `1.0`
  when they must stay strings.
- Prefer `"${...}"` in quotes; mandatory when the string starts with `{` or `[` or contains `: `.
- In double quotes `\n` is a newline and `\\` a backslash; inside `script: |` blocks everything is literal,
  so JS escapes (`'\n'`) reach JS as written.

### 7. Full example: search, results to a file, screenshot

`examples/google-search.yaml`, short version:

```yaml
session: {headless: true}
vars:
  query: playwright stealth browser
steps:
  - try:
      - open: https://www.google.com/
      - action: type
        locator: "textarea[name='q'], input[name='q']"
        value: "${query}"                       # text field: no escaping needed
      - press: Enter
      - action: wait                            # results or captcha, whichever comes first
        waitUntil: visible
        locator: "#search a h3, #captcha-form"
        index: 0
        timeout: 20000
        optional: true
      - action: evaluate
        as: results
        script: |
          () => {
            const items = [...document.querySelectorAll('#search a:has(h3)')].slice(0, 10)
              .map(a => ({title: a.querySelector('h3').innerText, url: a.href}));
            return {
              captcha: /\/sorry\//.test(location.href),
              items,
              // no JS template literal here: ${...} would be taken as a scenario variable
              text: items.map((x, i) => (i + 1) + '. ' + x.title + '\n   ' + x.url).join('\n') + '\n',
            };
          }
      - if: {var: results.captcha}
        then:
          - fail: "captcha for ${query}"
      - action: save
        file: output/google-search.txt
        value: "${results.text}"
    finally:
      - screenshot: output/google-search.png
```

### 8. Debugging

- `autobrowser validate-script -f s.yaml` first: catches field names (`type` needs `value`, not `text`).
- `--headed` to watch; `errorArtifacts: output/err` (or `--artifacts`) saves a screenshot and HTML
  of each failed step.
- Print intermediate values: `evaluate` with `as:` and look at `vars` in the result JSON.
- Step errors are in `steps[].error`; `name:` on steps makes them easy to find.

---

## Русский

### 1. Запуск сценария

```bash
autobrowser validate-script -f examples/google-search.yaml      # проверка без браузера
autobrowser run-script -f examples/google-search.yaml \
  -p youtube \                                 # сохранённый профиль (cookies, входы)
  --proxy socks5://127.0.0.1:1080 \            # важнее, чем session.proxy
  --var "query=autobrowser github" \           # переопределить vars (можно несколько раз)
  --headed \                                   # показать окно
  -o result.json                               # JSON-результат в файл вместо stdout
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run-script -f s.yaml   # выбор движка
```

Результат - JSON `{success, sessionId, profile, steps, vars}`; код выхода 1, если `success` false.
Достать значение: `jq '.vars.results.items' result.json`.

### 2. Переменные: откуда берутся

| Источник | Пример | Замечания |
|----------|--------|-----------|
| `vars` в файле | `vars: {query: test}` | значения по умолчанию, любой тип YAML |
| `--var NAME=VALUE` | `--var n=3` | переопределяет `vars`; значение **всегда строка** (`"3"`, а не `3`) |
| `vars` подключённого файла | `include: common/login.yaml` | добавляются как значения по умолчанию ниже текущих |
| `as: name` у шага | `evaluate` ... `as: results` | сохраняется только при успехе шага |
| шаг `set` | `set: {page: 1}` | задаёт или перезаписывает переменные |
| циклы | `forEach` -> `item` (или `as:`), `index` | `index` задают `forEach`, `repeat`, `while` |
| `try`/`catch` | `error` = `{message, action, name}` | задаётся перед запуском `catch` |
| окружение | `${env.HOME}` | только чтение, в `vars` результата не попадает |

Все переменные **глобальны на весь прогон**: областей видимости у блоков нет. `item`, `index` и
`error` остаются после своего цикла или `catch` и перезаписываются следующим циклом или `catch`.
Во вложенных циклах давайте разные имена через `as:`, иначе внутренний цикл затрёт внешний `item`;
`index` всегда относится к самому внутреннему циклу.

Что сохраняет `as:`:

| Действие | Сохраняется |
|----------|-------------|
| `extract` | `data.value` (строка, или список при `all: true`) |
| `evaluate` | `data.result` (то, что вернул JS) |
| `read`, `find-all`, `get-content`, `get-cookies`, `get-headers` | весь `data` |

Если шаг с `optional: true` упал, его переменная `as:` **не** задаётся. Тогда последующий `${name}`
падает с `undefined variable: name`; проверяйте через `if: {var: name}` (там неопределённая переменная = false).

### 3. Шаблоны `${...}`

Подстановка выполняется прямо перед запуском шага во всех строковых полях шага (включая `script`
у `evaluate`, `locator`, `file`, `value`, `url`, `message`), в условиях `if`/`while`/`assert`,
в `forEach` и `repeat`, в значениях `set`.
Не выполняется в `as` (простое имя) и в пути `include` (разбирается при загрузке).

| Синтаксис | Значение |
|-----------|----------|
| `${name}` | переменная |
| `${a.b.c}` | ключ словаря |
| `${rows.0}`, `${results.items.0.url}` | элемент списка по индексу |
| `${env.NAME}` | переменная окружения (если не задана - ошибка undefined) |
| `$${` | буквальный `${` (без подстановки) |

Два режима:

1. **Вся строка - один шаблон** (`"${obj}"`): значение сохраняет тип: список, словарь, число,
   bool. Так `forEach: "${links}"` получает список, а `set: {copy: "${obj}"}` копирует словарь.
2. **Шаблон внутри текста** (`"страница ${n} из ${total}"`): значение превращается в текст через
   Python `str()`. Строки и числа выглядят как ожидается, но словарь становится
   `{'a': 1, 'b': True, 'c': None}`, а список `['x', 'y']`: это синтаксис Python, **не JSON**.
   Чтобы вставить структуру в текст, соберите строку в JS (`JSON.stringify(...)`) и сохраните через `as:`.

Подводные камни с типами:

- Словарь или список целиком в поле, которое ждёт текст (`value: "${obj}"` у `save`/`type`),
  - ошибка шага `invalid value after substitution: value: Input should be a valid string`.
  Как любая ошибка шага, она ловится через `try`, пропускается при `optional: true` и учитывает
  `stopOnError`. Числа подходят (`value: "${n}"` запишет `3`).
- `--var n=3` даёт строку `"3"`: `if: {var: n, equals: 3}` ложно, `equals: "3"` истинно.
  `repeat: "${n}"` принимает оба варианта.
- `equals` сравнивает с типом из YAML: `equals: yes` - это булево `true` (YAML 1.1), пишите `"yes"`.
- `validate-script` шаблоны не раскрывает: неопределённая переменная проявится только при запуске
  ошибкой шага `undefined variable: name`.
- Строка-локатор разбирается после подстановки, поэтому `click: "${target}"` с
  `--var "target=text=Next"` использует текстовый локатор.

```yaml
vars:
  base: https://quotes.toscrape.com
  pages: [1, 2, 3]
steps:
  - forEach: "${pages}"                         # шаблон на всю строку -> список
    as: p                                       # index = 0, 1, 2
    do:
      - open: "${base}/page/${p}/"
      - action: extract
        locator: ".quote .text"
        all: true
        as: quotes
      - forEach: "${quotes}"                    # шаблон на всю строку -> список
        as: q
        do:
          - set: {last: "${q}"}
  - assert: {var: last}
```

### 4. JavaScript в `evaluate`

#### 4.1 Что можно писать в `script`

| Форма | Пример | Результат |
|-------|--------|-----------|
| выражение | `document.title + '!'` | значение выражения |
| несколько операторов | `const a = 2; a * 2` | значение последнего оператора |
| функция | `() => document.title` | возвращённое значение |
| async-функция | `async () => { await new Promise(r => setTimeout(r, 500)); return 1 }` | дожидается результата |

- Функция **не получает аргументов**: данные передаются через шаблоны (см. 4.4).
- JS выполняется в **основном мире** страницы: видны глобальные объекты страницы (`window.jQuery`,
  состояние приложения), и страница видит ваши изменения.
- Длинные скрипты пишите блоком YAML `script: |` (нет проблем с кавычками, переносы сохраняются).
- В циклах ожидания ставьте свой предел времени (как в
  `examples/google-search.yaml`: `while (Date.now() - t0 < 20000 && ...)`).

#### 4.2 Что возвращается

Результат должен быть JSON-подобным. Проверенные преобразования:

| Значение JS | `data.result` |
|-------------|---------------|
| `undefined`, ничего не возвращено | `null` |
| число, строка, bool, обычный объект, массив | как есть |
| `Infinity` | `Infinity` |
| `Date` | строка `1970-01-01 00:00:00+00:00` |
| `Map`, `Set` | `{}` (данные теряются) - преобразуйте `Object.fromEntries(...)` / `[...set]` |
| DOM-элемент | строка `ref: <Node>` (бесполезно) - возвращайте `el.innerText`, `el.href` и т.п. |

Возвращайте один объект со всем нужным и сохраняйте его через `as:`; дальше используйте
`${results.items}`, `${results.captcha}` в следующих шагах.

#### 4.3 `${` внутри JS: самая частая ловушка

Шаблоны подставляются в `script` **до** запуска JS. Поэтому JS-шаблонная строка
`` `${x.title}` `` воспринимается как переменная сценария, и шаг падает с
`undefined variable: x.title`. Варианты:

```js
// 1. экранирование: $${ превращается в ${, и JS получает свою шаблонную строку
() => { const i = 1; return `n=$${i + 1}`; }           // -> "n=2"
// 2. обычная конкатенация
items.map((x, i) => (i + 1) + '. ' + x.title).join('\n')
```

Одиночный `$` не особенный: `$('a')`, `$x(...)`, регулярки `/\$\{/` работают без изменений.

#### 4.4 Передача переменных в JS

Значение вставляется как сырой текст, **без экранирования**:

```yaml
- action: evaluate
  script: "() => document.title.includes('${word}')"
```

- Работает для контролируемых значений (числа, слова, селекторы без кавычек).
- Значение с кавычкой (`it's "quoted"`) ломает JS: `SyntaxError`.
- Словарь или список вставляется в синтаксисе Python (`{'a': 1, 'b': True}`) - это не JS.
- Выходы:
  - не пускать пользовательский текст в JS: ввести его на страницу через `type`/`fill`
    (`value: "${query}"`) и прочитать в JS из DOM;
  - передать значение в URL-кодировке и раскодировать: `--var "q=it%27s"`, в JS `decodeURIComponent('${q}')`;
  - для структур из предыдущего `evaluate` вернуть там JSON-строку
    (`return JSON.stringify(obj)`, `as: obj_json`) и вставить её как литерал JS:
    `() => { const o = ${obj_json}; return o.a; }` (JSON - валидный JS).

#### 4.5 Навигация и тайминги

- Если во время работы JS страница уходит на другой адрес (клик или `press: Enter` начали загрузку),
  шаг падает с `Execution context was destroyed, most likely because of a navigation`. После действия,
  вызывающего переход, сначала ждите: `wait: load` или `action: wait` с `waitUntil: visible` и
  `locator` элемента новой страницы, потом `evaluate`.
- `waitUntil: visible` с локатором через запятую ждёт то, что появится первым (результаты **или**
  капча); добавьте `index: 0`, так как совпасть может несколько элементов.
- Значения, прочитанные JS со страницы, - снимок: после изменений страницы запускайте `evaluate` заново.

### 5. Запись файлов

| Шаг | Что пишет |
|-----|-----------|
| `screenshot: out.png` (`fullPage: true` - вся страница) | PNG |
| `save` с `file` | HTML страницы (`format: html`, по умолчанию) или текст страницы (`format: text`) |
| `save` с `file` и `value` | заданный текст (с 0.5.3): `value: "${results.text}"` |
| `output.file` на верхнем уровне / `-o` | весь JSON-результат |

- Пути **относительно рабочего каталога** и должны оставаться внутри него: абсолютные пути и `..`
  за его пределы отклоняются. Недостающие каталоги создаются. Существующие файлы перезаписываются.
- Имя файла может быть шаблоном: `file: "output/${query}.txt"` (значение должно быть допустимым в имени файла).
- Для нескольких строк соберите текст в `evaluate`, например
  `text: items.map((x, i) => (i + 1) + '. ' + x.title + '\n   ' + x.url).join('\n') + '\n'`.
- Для JSON верните из `evaluate` `JSON.stringify(data, null, 2)` и сохраните эту строку.
- Скриншоты кладите в `finally`: они снимутся даже при упавшем шаге.

### 6. Тонкости YAML

- Локаторы, начинающиеся с `#`, берите в кавычки: без них `#id` - комментарий YAML.
- Берите в кавычки значения с `: `, `{`, `[`, а также `yes`, `no`, `on`, `off`, `null`, `1.0`,
  если они должны остаться строками.
- `"${...}"` лучше всегда в кавычках; обязательно, если строка начинается с `{` или `[` или содержит `: `.
- В двойных кавычках `\n` - перевод строки, `\\` - обратный слэш; внутри блока `script: |` всё
  буквально, и JS-экранирование (`'\n'`) доходит до JS как написано.

### 7. Полный пример: поиск, результаты в файл, скриншот

Сокращённая версия `examples/google-search.yaml` - см. английский раздел (п. 7), код тот же.
Главное в нём:

- запрос вводится через `type` с `value: "${query}"` - экранирование не нужно;
- ожидание «результаты или капча» одним шагом `wait` с локатором через запятую и `optional: true`;
- `evaluate` возвращает объект `{captcha, items, text}` в `results`, строка `text` собирается
  конкатенацией, без JS-шаблонных строк;
- `if: {var: results.captcha}` + `fail` для явной ошибки;
- `save` с `value: "${results.text}"` пишет `output/google-search.txt`;
- `screenshot` в `finally`.

### 8. Отладка

- Сначала `autobrowser validate-script -f s.yaml`: ловит ошибки в полях (у `type` поле `value`, а не `text`).
- `--headed`, чтобы смотреть; `errorArtifacts: output/err` (или `--artifacts`) сохраняет скриншот и
  HTML каждого упавшего шага.
- Промежуточные значения: `evaluate` с `as:` и поле `vars` в JSON-результате.
- Ошибки шагов в `steps[].error`; `name:` у шагов облегчает поиск.
