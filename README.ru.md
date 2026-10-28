# autobrowser-client

[English](README.md) | Русский

Стелс-браузер для сценариев и AI-агентов. Запускает Chromium через совместимые с Playwright стелс-движки (`patchright` по умолчанию, опционально `cloakbrowser`) и добавляет то, чего нет в обычном Playwright: постоянные профили, стабильные отпечатки, SOCKS5-прокси с авторизацией, прохождение проверок Cloudflare, YAML-сценарии, HTTP API и MCP-сервер. Поставляется одним бинарём, который обновляется сам.

## Быстрая установка через AI-агента

Дайте агенту эту ссылку:

```
Install autobrowser by following https://github.com/foxzi/autobrowser-client/blob/main/docs/install-agent.md
```

## Чем отличается от Playwright

| | Playwright / Playwright MCP | autobrowser |
|---|---|---|
| Детект автоматизации | Обычная сборка видна проверкам на ботов (`navigator.webdriver`, следы CDP) | Стелс-движки: `patchright` (пропатченный драйвер, системный Chrome) или `cloakbrowser` (патчи на уровне C++) |
| Cloudflare / Turnstile | Не обрабатывается | Шаг `solve-challenge` и инструмент `browser_solve_challenge` ждут проверку и нажимают Turnstile |
| Отпечаток | Как у хоста | Свой для каждого профиля шум Canvas, WebGL, Audio, ClientRects и шрифтов, стабильный по seed; платформа и Client Hints подстраиваются под User-Agent |
| Профили | `user_data_dir` нужно вести самому | Именованные постоянные профили: cookies, storage, отпечаток и восстановление открытых вкладок |
| Прокси | Chromium не умеет SOCKS5 с логином и паролем | SOCKS5/HTTP с авторизацией через встроенный локальный relay; пароли скрыты в логах |
| Автоматизация без кода | Нужно писать код | YAML/JSON-сценарии с короткой записью шагов и проверкой без запуска браузера |
| Удалённое управление | Только библиотека | HTTP API с сессиями, токеном, лимитами и таймаутом простоя |
| AI-агенты | Playwright MCP | MCP-сервер с инструментами `browser_*` поверх стелс-движка и готовый skill для агента |
| Человекоподобный ввод | Вручную | Набор текста с задержками, случайные движения мыши, обход вкладок с прокруткой |
| Поставка | Пакет Python/Node и браузеры | Один бинарь для Linux, macOS и Windows с самообновлением; Docker-образы |

## Возможности

### Стелс-движки
- `patchright` (по умолчанию): пропатченный драйвер Playwright с системным Google Chrome.
- `cloakbrowser`: собственная сборка Chromium с патчами в исходниках, скачивается командой `autobrowser install`.
- Оба движка поддерживают все команды CLI, эндпоинты API и инструменты MCP; переключаются через `AUTOBROWSER_BROWSER_ENGINE` или `browser.engine` в конфиге.
- `fingerprintPlatform` / `AUTOBROWSER_FINGERPRINT_PLATFORM` задаёт платформу, которую показывает cloakbrowser.

### Профили и сессии
- Именованные профили хранят cookies, localStorage и данные сессии между запусками.
- `--preserve-session` сохраняет открытые вкладки в `tabs.json` и восстанавливает их при следующем запуске.
- Сессия закрывается сама после `timeouts.idleSessionMs` простоя (по умолчанию 5 минут).

### Отпечатки
- `fingerprint: true` создаёт в профиле `fingerprint.json`: Canvas, WebGL vendor/renderer, AudioContext, ClientRects и метрики шрифтов получают шум от seed, поэтому профиль одинаков при каждом запуске и отличается от других профилей.
- Строки GPU подбираются под User-Agent (Windows, Linux, Android).
- User-Agent не для Linux меняет `navigator.platform`, `userAgentData` и `Sec-CH-UA-Platform`; Android UA включает мобильный режим и touch.
- `autobrowser generate-fingerprint` создаёт отпечаток заранее; в сценарии можно переопределить отдельные поля.

### Прокси
- `socks5://user:pass@host:port` или HTTP-прокси: на запуск (`--proxy`), в сценарии или в конфиге.
- На время сессии запускается локальный relay, благодаря ему SOCKS5 с авторизацией работает в Chromium: `socks2http`, если он установлен, иначе встроенный relay на Python (ничего ставить не нужно).
- `geoip` / `AUTOBROWSER_GEOIP` задаёт часовой пояс и язык браузера по выходному IP, чтобы они совпадали со страной прокси.
- Пароли прокси и чувствительные заголовки (`authorization`, `cookie`, `set-cookie`, `proxy-authorization`) скрываются в выводе и логах.

### YAML-сценарии
- Шаги: `open`, `wait`, `click`, `fill`, `type`, `press`, `scroll`, `find`, `screenshot`, `get-content`, `get-cookies`, `get-headers`, `browse-tabs`, `solve-challenge`.
- Короткая запись: `- open: https://example.com`, `- click: "text=Sign in"`; локаторы по CSS, XPath, тексту, id, name, class или атрибуту.
- Другие действия: `hover`, `select`, `upload`, `evaluate`, `extract`, `wait-text`, `history`, `set`, `assert`, `fail` и др.
- Переменные `${name}` / `${env.NAME}` (`vars:` в файле, `--var NAME=VALUE`), условия и блоки `if`, `try`/`catch`/`finally`, `forEach`, `repeat`, `while`, `include`; опции шага `retry`, `optional`; `--artifacts DIR` сохраняет скриншот и HTML упавших шагов.
- Те же сценарии выполняются через MCP (`browser_run_script`) и HTTP (`POST /api/v1/session/{id}/script`).
- `autobrowser validate-script` проверяет сценарий без браузера; результат в JSON со статусом каждого шага, при ошибке код выхода 1.

### HTTP API
- `autobrowser serve`: сессии, вкладки, навигация, история, снапшоты, скриншоты, действия с элементами (find, read, click, hover, fill, select, check, upload, submit), мышь и клавиатура, выполнение JavaScript, логи консоли и сети, политика диалогов, профили.
- Bearer-токен (`AUTOBROWSER_API_TOKEN`), лимит сессий, проверки health и ready.

### MCP-сервер для AI-агентов
- `autobrowser mcp` (stdio) даёт агенту 18 инструментов: navigate, history, snapshot, screenshot, click, hover, type, press key, select, file upload, evaluate, wait for text, get text, tabs, logs, dialogs, solve challenge, close.
- Снапшот возвращает `ref` элементов, и агент кликает по ссылке, а не подбирает селекторы.
- `autobrowser install-skill` ставит skill, который учит агента циклу snapshot/ref, ожиданиям, работе с Cloudflare и правилам безопасности.

### Поставка
- Один бинарь без Python; релизы для Linux amd64/arm64, macOS arm64 и Windows amd64 с контрольными суммами sha256.
- Самообновление: `mcp` и `serve` раз в сутки проверяют новый релиз; `autobrowser update` обновляет сразу, `AUTOBROWSER_NO_AUTO_UPDATE=1` отключает.
- Docker-образы для CLI и API; также ставится как Python-пакет.

## Документация

- Техническое задание: [`tz.md`](tz.md)
- MCP-сервер для AI-агентов: [`docs/mcp.md`](docs/mcp.md)
- Skill для агента: [`skills/autobrowser/SKILL.md`](skills/autobrowser/SKILL.md)
- Установка через AI-агента: [`docs/install-agent.md`](docs/install-agent.md)
- HTTP API: [`docs/api.md`](docs/api.md)
- Конфигурация: [`docs/configuration.md`](docs/configuration.md)
- CLI-сценарии: [`docs/scripts.md`](docs/scripts.md)
- Руководство по сценариям: переменные, шаблоны, JS, файлы: [`docs/scenario-guide.md`](docs/scenario-guide.md)
- Рантайм (профили, прокси, отпечатки, хранилище): [`docs/runtime.md`](docs/runtime.md)
- Решение проблем (Cloudflare, прокси, движки): [`docs/troubleshooting.md`](docs/troubleshooting.md)
- Релизы и версии: [`docs/releases.md`](docs/releases.md)
- Примеры: [`examples/README.md`](examples/README.md)

## Команды CLI

| Команда | Описание |
|---------|----------|
| `autobrowser version` | Показать версию |
| `autobrowser show-config` | Показать действующую конфигурацию |
| `autobrowser validate-script --file scenario.yaml` | Проверить сценарий без запуска |
| `autobrowser run-script --file scenario.yaml` | Выполнить YAML/JSON-сценарий |
| `autobrowser run --url https://example.com` | Открыть URL и собрать данные |
| `autobrowser serve` | Запустить HTTP API-сервер |
| `autobrowser mcp` | Запустить MCP-сервер (stdio) для AI-агентов |
| `autobrowser install` | Скачать браузер для выбранного движка |
| `autobrowser install-skill [--dir DIR]` | Установить skill для агента |
| `autobrowser update [--check]` | Обновить бинарь до последнего релиза |
| `autobrowser generate-fingerprint --profile DIR` | Создать fingerprint.json для профиля |

Общие флаги: `--config`, `--profile`, `--proxy`, `--viewport`, `--output`, `--headless/--headful`, `--timeout`, `--preserve-session`.

Справка: `autobrowser -h`, `autobrowser help <command>` или `autobrowser <command> -h` (с примерами и переменными окружения). `autobrowser -V` выводит версию.

## Движок браузера

Поддерживаются два стелс-движка. По умолчанию используется `patchright`.

| Движок | Метод маскировки | Chromium | Установка |
|---|---|---|---|
| `patchright` (по умолчанию) | Патчи через JS-инъекции, использует системный Chrome (`channel="chrome"`) | системный | встроен |
| `cloakbrowser` | Патчи на уровне C++, собственный бинарь (скачивается автоматически при первом использовании) | в комплекте | опционально |

Переключение на cloakbrowser:

```bash
# Install optional dependency
pip install "autobrowser-client[cloak]"
# or: pip install cloakbrowser

# Via env var
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run --url https://example.com
```

Или в `config.yaml`:

```yaml
browser:
  engine: cloakbrowser
```

Оба движка поддерживают профили, прокси, внедрение отпечатков, viewport, user_agent, все команды CLI и все эндпоинты API.

## Быстрый старт

### Бинарь

Скачайте бинарь для своей платформы из [Releases](https://github.com/foxzi/autobrowser-client/releases/latest), затем:

```bash
autobrowser install                               # download the browser
autobrowser run --url https://example.com --format text
autobrowser run-script --file examples/simple.yaml
```

### Docker (рекомендуется для серверов)

```bash
docker compose build cli
docker compose run --rm cli version
docker compose run --rm cli show-config
docker compose run --rm cli run-script --file examples/simple.yaml
```

### Локальный venv (режим разработки)

```bash
python3 -m venv .venv
.venv/bin/pip install -e .
.venv/bin/autobrowser version
.venv/bin/autobrowser show-config
.venv/bin/autobrowser run-script --file examples/simple.yaml
```

### HTTP API-сервер

```bash
.venv/bin/autobrowser serve --host 0.0.0.0 --port 8080
# or via Docker
docker compose up api
```

### MCP-сервер

```bash
claude mcp add autobrowser -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser -- autobrowser mcp
autobrowser install-skill
```

Другие клиенты описаны в [`docs/mcp.md`](docs/mcp.md).

## Пример сценария (короткая запись)

Сценарии поддерживают компактную короткую запись: имя действия используется как ключ YAML.

```yaml
steps:
  - open: https://example.com/login
  - wait: networkidle
  - fill: ["name=username", "demo-user"]
  - fill: ["name=password", "demo-password"]
  - click: "button[type='submit']"
  - wait: networkidle
  - screenshot: login.png
    fullPage: true
  - get-content: ~

output:
  file: result.html
```

Сайт под защитой Cloudflare, с прокси и профилем:

```yaml
session:
  profile: news
  proxy: socks5://user:pass@127.0.0.1:1080
  fingerprint: true
steps:
  - open: https://example.com
  - solve-challenge: ~
  - get-content: ~
```

Полный справочник по синтаксису, включая короткие формы локаторов и все доступные действия, смотрите в [`docs/scripts.md`](docs/scripts.md), а готовые сценарии - в [`examples/`](examples/).
