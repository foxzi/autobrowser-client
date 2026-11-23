# autobrowser (Go)

Порт Python-клиента `autobrowser` на Go (см. `../python`). Поддерживается только движок
cloakbrowser (чистый CDP через go-rod, без Node и patchright). Статический бинарник без
внешних зависимостей. Порт соответствует Python-клиенту, кроме
[известных отличий](#известные-отличия) ниже.

## Сборка

```bash
cd go && CGO_ENABLED=0 go build -o autobrowser ./cmd/autobrowser
```

Релизные бинарники называются `autobrowser-go-<os>-<arch>[.exe]` (linux amd64/arm64, darwin arm64,
windows amd64), рядом лежит `.sha256`; `autobrowser update` берёт именно их.

## Использование

Движок по умолчанию в конфиге - `patchright`, Go-версия его не запускает. Укажите движок:

```bash
export AUTOBROWSER_BROWSER_ENGINE=cloakbrowser   # или browser.engine в конфиге
autobrowser install                              # скачать браузер
autobrowser run-script -f file.yaml [--var K=V] [-c config.yaml] [-p profile] \
    [--proxy URL] [--viewport WxH] [-o result.json] [--preserve-session] [--artifacts DIR]
autobrowser mcp                                  # MCP-сервер через stdio
autobrowser serve                                # HTTP API
```

Команды: `version`, `show-config`, `validate-script`, `run-script`, `run`, `serve`, `mcp`,
`install`, `install-skill`, `update [--check]`, `generate-fingerprint`, `help`.
`--var` можно повторять. JSON-результат выводится в stdout или в файл `-o`.

## Поиск бинарника браузера

Chromium из cloakbrowser ищется и, если его нет, скачивается так же, как в cloakbrowser
`ensure_binary` (`internal/cloak`): `CLOAKBROWSER_BINARY_PATH`, `CLOAKBROWSER_VERSION`,
`CLOAKBROWSER_CACHE_DIR` (по умолчанию `~/.cloakbrowser`) работают так же. Загрузка проверяется
по подписанному Ed25519 файлу SHA256SUMS до распаковки.

## Паритет с Python-клиентом

| Возможность | Python | Go |
|---|---|---|
| Конфиг-файл, env, значения по умолчанию, show-config | готово | готово |
| YAML-сценарии, vars, шаблоны `${}`, include | готово | готово |
| if / try / forEach / repeat / while, retry, optional, stopOnError | готово | готово |
| assert / set / fail, JSON-результат | готово | готово |
| Профили, lock, preserveSession (`tabs.json`) | готово | готово |
| Действия: open, wait, find, find-all, click, dblclick, hover, focus, fill, clear, check, uncheck, type, press, scroll, screenshot, get-content, save, evaluate, history, wait-text, extract | готово | готово |
| Действия: get-cookies, get-headers, solve-challenge, browse-tabs, submit, select, upload, read, mouse-move, mouse-scroll-to, mouse-random | готово | готово |
| Локаторы (css, xpath, text, id, name, class, attribute, `ref`) | готово | готово (`text` приближает `get_by_text`) |
| Снимок доступности с ref (`page_snapshot`, локатор `ref`) | готово | готово (инжектируемый скрипт Playwright лежит в `internal/browser/injected`, обновляется через его `extract.sh`) |
| Regex в условиях | готово | готово (RE2: без lookahead и обратных ссылок) |
| Headless, viewport, userAgent, мобильная эмуляция | готово | готово |
| Движок cloakbrowser | готово | готово |
| Движок patchright | готово | не поддерживается (ошибка при запуске) |
| Прокси (http, socks5, с логином и паролем) | готово (relay) | готово (`socks2http`, если установлен, иначе встроенный relay) |
| geoip (часовой пояс и локаль по IP выхода) | готово | готово (`internal/cloak`, GeoLite2) |
| Инъекция JS-отпечатка, `generate-fingerprint` | готово | готово |
| Загрузка бинарника браузера | готово | готово |
| Менеджер сессий (сессии, вкладки, политика диалогов, логи console/network/dialogs, закрытие по простою) | готово | готово (`internal/browser/manager.go`, `session.go`) |
| MCP-сервер (`autobrowser mcp`, включая `browser_run_script`) | готово | готово (`internal/mcp`, официальный go-sdk; схемы инструментов те же, что в Python) |
| HTTP API (`autobrowser serve`, OpenAPI, `/docs`) | готово | готово (`internal/api`, net/http) |
| CLI: version, show-config, validate-script, run-script, run, serve, mcp, install, install-skill, update, generate-fingerprint, help | готово | готово |
| Самообновление (`update`, фоновая проверка в `mcp` и `serve`) | готово | готово (Go-ассеты) |
| Релизные бинарники | готово (`autobrowser-<os>-<arch>`) | готово (`autobrowser-go-<os>-<arch>`) |

## Известные отличия

Где Go-клиент не может или не стал повторять Python дословно. Формы JSON-результатов, ключи
конфига, переменные окружения и коды выхода совпадают, если не сказано иное.

### Движок

- Только `cloakbrowser`. `browser.engine=patchright` при запуске даёт `CONFIG_ERROR`
  (`show-config` работает с любым движком); `install` не выполняет `patchright install chrome`.
- Браузер, упавший при запуске (например, `headless: false` без X-сервера), даёт
  `BROWSER_START_FAILED "browser exited during startup: <stderr chrome>"` вместо текста Playwright.
- Ошибки CDP вне портированных текстов - обычные ошибки протокола без `Call log:` Playwright.

### Действия и локаторы

- Локатор `text` приближает `get_by_text`; `select` не переходит от `<label>` к элементу.
- Проверка готовности элемента - только «в DOM и виден» (без enabled / stable). Ошибки `select`,
  `read` и `mouse-scroll-to` содержат короткий call log, а не полный лог повторов Playwright.
- `read` приближает `is_enabled` через `:disabled` и предков с `aria-disabled`.
- `upload` нескольких файлов в поле без `multiple` не отклоняется.
- `get-headers` берёт Document-ответы главного фрейма из событий CDP; имена заголовков
  отсортированы, Python сохраняет порядок получения.
- `mouse-random` использует тот же алгоритм с `math/rand` Go, поэтому один seed даёт другой путь
  (вывод шага тот же).
- `solve-challenge` находит iframe вне процесса через `Target.getTargets`; точка клика та же.
- Regex в условиях - RE2; некорректный regex делает проверку ложной.
- У неуспешного шага всегда есть ключ `error` (пустая строка без текста); Python опускает его для `None`.
- Ошибки валидации сценария (`run-script`, `validate-script`, MCP `browser_run_script`, API
  `SCRIPT_VALIDATION_ERROR`) используют пути Go вида `steps.0.action` без хвоста pydantic
  `[type=..., input_value=...]` и строки "For further information"; Python печатает
  `steps.0.step.action` с ними. Коды ошибок и коды выхода совпадают.

### Снимок страницы

- Фреймы внутри межсайтового (out-of-process) iframe получают префикс `fN` при первом снимке,
  поэтому нумерация может отличаться от Playwright, если таких фреймов несколько.
- График повторов снимка (0, 1, 2, 4, 8 с) лишь приближает Playwright.

### Сеть и логи

- Вызовы `console.log` / `console.error` на странице не записываются: домен CDP `Runtime`
  выключен ради скрытности. В лог попадают только сообщения браузера (например, ошибки загрузки
  ресурсов), сетевые записи и диалоги. Записи воркеров пропускаются.
- Ключи записей логов console, network и dialogs те же, что в Python.

### Прокси и загрузка cloakbrowser

- Строка лога relay скрывает пароль upstream (`***`); Python печатает его открытым текстом.
- Relay отвечает 502 на не-ASCII имена upstream-хостов (браузеры всё равно шлют punycode) и при
  ошибках после начала туннеля просто закрывает сокеты. Подпроцесс `socks2http` получает
  SIGTERM, через 5 с - kill; его вывод отбрасывается.
- Загрузка Pro / по ключу лицензии и `CLOAKBROWSER_RELEASE_CHANNEL` не поддерживаются
  (заданный `CLOAKBROWSER_LICENSE_KEY` только пишет предупреждение).
- Нет приветственного баннера и фоновой проверки новой сборки cloakbrowser;
  маркеры `latest_version*`, записанные Python, учитываются.
- Распаковка архива отклоняет записи, выходящие за целевой каталог.
- Таймаут загрузки - «нет данных 60 с» (база geoip: 300 с), а не пофазные таймауты httpx.
- geoip: SOCKS5-прокси работают сразу (Python нужен `cloakbrowser[geoip]`); ошибка выглядит как
  `browser.geoip failed: <err>` без подсказки про pip; поиск IP выхода без прокси учитывает
  `HTTP_PROXY` / `HTTPS_PROXY`, но не `ALL_PROXY`; пустой часовой пояс в базе - `""`.

### Отпечаток

- Генерация в точности повторяет `random.Random(seed).choice` Python; отличается только случайный
  seed (без `--seed`), он случаен в обоих. Seed - int64 (в Python любой размер).
- `fingerprint.json` читается строго по типам (без приведения pydantic для `"true"` или `"123"`);
  некорректный файл создаётся заново.

### Вывод CLI

- Help, usage и ошибки - обычный текст (без рамок Rich); формулировки следуют Typer. Длинные
  опции с одним дефисом (`-config`) тоже принимаются. Без аргументов help идёт в stdout, код 2.
- `update` берёт ассеты `autobrowser-go-<os>-<arch>` (`darwin`, не `macos`); ошибка обновления,
  кроме несовпадения контрольной суммы, печатается обычным текстом с кодом 1 (Python: traceback).
- `serve` печатает одну строку `Serving the API on http://HOST:PORT` вместо логов uvicorn.
- `run --profile-dir` передаётся как введён (Python нормализует через `pathlib.Path`).
- Версия задаётся при сборке (`-ldflags -X`), по умолчанию `0.5.4`.

### MCP

- Capabilities в `initialize`: `{"prompts":{},"resources":{},"tools":{}}` (go-sdk не выводит
  false-поля); в списках есть `ttlMs` / `cacheScope: "public"` от go-sdk (Python: `"private"`).
- В результатах инструментов нет `isError: false` (go-sdk); при ошибке по-прежнему `isError: true`.
- Проверка аргументов повторяет нужное подмножество lax-режима pydantic с его текстами ошибок;
  целые больше int64 дают `int_parsing`.

### API

- `/docs`, `/docs/oauth2-redirect` и `/redoc` отдают статическую копию HTML, снятую с
  FastAPI 0.162.0. Access-лога нет.
- Заголовок `Authorization` с не-ASCII символами даёт 401 (Python: 500 из `hmac.compare_digest`).


## Прототип (go/cmd/cdpprobe)

Исходный одноразовый эксперимент теперь лежит в `go/cmd/cdpprobe`. Пути ниже указаны относительно него.

Одноразовый прототип: статический бинарник на Go (go-rod, чистый CDP, без Node и patchright)
управляет Chromium из cloakbrowser. Вопрос: проходит ли он проверки на ботов?

### Сборка и запуск

```bash
cd cmd/cdpprobe
CGO_ENABLED=0 go build -ldflags="-s -w" -o go-cdp .
./run.sh out-win -platform windows          # все сайты, один вариант
./run.sh out-x -url https://example.com/    # один URL
./runall.sh                                 # 4 варианта: windows/linux x stealth вкл/выкл
./cfloop.sh                                 # только Cloudflare, по 3 прогона на вариант
```

`run.sh` монтирует бинарник в `parsers-autobrowser:latest` и запускает от пользователя хоста.
Флаги: `-bin -platform windows|linux -headless -stealth -out -url -runtime-enable`.
По каждому сайту сохраняются `<name>.png` (вся страница), `<name>.html`, `<name>.json`
(итоговый URL, заголовок, `navigator.webdriver`, UA, результат Cloudflare).

Флаги запуска: стандартные флаги rod без `--enable-automation`, плюс
`--no-sandbox --fingerprint=<случайное> --fingerprint-platform=<платформа>`.

### Результаты (2026-11-22, headless, в Docker)

| Сайт | windows | linux | windows + stealth | linux + stealth |
|---|---|---|---|---|
| browserscan bot-detection (вкл. CDP) | Normal | Normal | Normal | Normal |
| BotD | не бот | не бот | не бот | не бот |
| creepjs headless / stealth | 0% / 0% | 0% / 0% | 0% / 80% | 0% / 80% |
| creepjs like-headless | 25% | 44% | 25% | 44% |
| navigator.webdriver | false | false | false | false |
| sannysoft | ERR_TIMED_OUT | ERR_TIMED_OUT | ERR_TIMED_OUT | ERR_TIMED_OUT |
| areyouheadless | сайт отдаёт 502 | 502 | 502 | 502 |
| criptonoticias (Cloudflare), прошёл/прогонов | 4/4 | 3/4 | 3/4 | 1/4 |

Замечания:

- sannysoft не открылся; Python-клиент (`autobrowser run-script`) в тот же день
  тоже получил таймаут, значит проблема в сайте или сети. areyouheadless отдаёт nginx 502.
- go-rod/stealth вредит: creepjs его видит (`hasIframeProxy`), Cloudflare блокирует чаще.
  Незаметность даёт сам бинарник cloakbrowser, а не клиент.
- Блокировки Cloudflare заканчиваются на чекбоксе Turnstile; один клик
  по виджету за 30 с не помог.
- rod по умолчанию эмулирует устройство (Mac, UA Chrome/114) на каждой странице. Это нужно
  отключать через `NoDefaultDevice()`, иначе UA противоречит отпечатку.

### Риск Runtime.enable

go-rod не вызывает `Runtime.enable` явно (использует `Page.enable`, `DOM.enable`,
`Runtime.evaluate`/`callFunctionOn`). Проверка CDP на browserscan показала Normal.
Встроенная ловушка (getter `stack` у Error + `console.debug`) показала «не обнаружено»
даже с `-runtime-enable`, то есть на Chrome 146 она не работает и ничего не доказывает.
Поэтому риск **не проверен**, известно только «browserscan не обнаружил».

### Размер и запуск

| | go-cdp | PyInstaller `autobrowser` |
|---|---|---|
| размер бинарника | 11 МБ | 77 МБ |
| `-h` / `--help` | 0,004 с | ~1,0 с |

Запуск Chromium через go-rod: ~110 мс до готового CDP-соединения.
