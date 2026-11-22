# autobrowser (Go)

Порт Python-клиента `autobrowser` на Go (см. `../python`). Поддерживается только движок
cloakbrowser (чистый CDP через go-rod, без Node и patchright). Статус: этап 1 из 5
(ядро, сценарии, базовые действия). Статический бинарник без внешних зависимостей.

## Сборка

```bash
cd go && CGO_ENABLED=0 go build -o autobrowser ./cmd/autobrowser
```

## Использование

```bash
autobrowser version
autobrowser show-config -c config.yaml
autobrowser run-script -f file.yaml [--var K=V] [-c config.yaml] [-p profile] \
    [--proxy URL] [--viewport WxH] [-o result.json] [--preserve-session] [--artifacts DIR]
```

`--var` можно повторять. JSON-результат выводится в stdout или в файл `-o`.

## Поиск бинарника браузера

Chromium из cloakbrowser берётся из `CLOAKBROWSER_BINARY_PATH`, иначе из самого нового
`~/.cloakbrowser/chromium-*/chrome`. Автоматической загрузки пока нет: установите бинарник
через Python-клиент или задайте переменную.

## Паритет с Python-клиентом

| Возможность | Python | Go |
|---|---|---|
| Конфиг-файл, env, значения по умолчанию, show-config | готово | готово |
| YAML-сценарии, vars, шаблоны `${}`, include | готово | готово |
| if / try / forEach / repeat / while, retry, optional, stopOnError | готово | готово |
| assert / set / fail, JSON-результат | готово | готово |
| Профили, lock, preserveSession (`tabs.json`) | готово | готово |
| Действия: open, wait, find, find-all, click, dblclick, hover, focus, fill, clear, check, uncheck, type, press, scroll, screenshot, get-content, save, evaluate, history, wait-text, extract | готово | готово |
| CLI: version, show-config, run-script | готово | готово |
| Прокси | готово (relay с авторизацией) | частично (http/socks5 без логина и пароля) |
| Локаторы | готово | частично (`text` приближает get_by_text, `ref` не поддержан) |
| Regex в условиях | готово | частично (RE2: без lookahead и обратных ссылок) |
| Headless, viewport, userAgent | готово | частично (мобильная эмуляция только подменой UA) |
| Движок patchright | готово | нет |
| geoip | готово | нет |
| Инъекция JS-отпечатка | готово | нет (`session.fingerprint` игнорируется с предупреждением) |
| Действия: get-cookies, get-headers, solve-challenge, browse-tabs, submit, select, upload, read, mouse-move, mouse-scroll-to, mouse-random | готово | нет |
| Загрузка бинарника | готово | нет |
| MCP-сервер | готово | нет |
| HTTP API | готово | нет |
| Остальные CLI-команды | готово | нет |
| Релизные бинарники | готово | нет |

## Живое сравнение Python и Go

Примеры `conditions`, `try-catch`, `extract-loop`, `screenshot` и собственный сценарий с формой
дали одинаковые результаты в обоих клиентах (без учёта `durationMs` и `sessionId`). `simple.yaml`
отличается, потому что в Go нет `get-headers` и `get-cookies`.

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
