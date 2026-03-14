# Examples / Примеры сценариев

## RU

Примеры YAML-сценариев для `autobrowser run-script`. Каждый файл — готовый к запуску сценарий.

### Установка

```bash
# Локально
pip install -e .

# Или через Docker
docker compose build
```

### simple.yaml — Проверка антидетекта

Открывает [Scrapfly Automation Detector](https://scrapfly.io/web-scraping-tools/automation-detector) и собирает headers, cookies и текстовое содержимое страницы. Работает в headless-режиме.

```bash
autobrowser run-script --file examples/simple.yaml
```

С сохранением результата в файл:

```bash
autobrowser run-script --file examples/simple.yaml --output /tmp/simple.json
```

### login.yaml — Авторизация на сайте

Шаблон сценария авторизации: открывает страницу логина, заполняет поля username/password, нажимает submit, ждёт загрузки и сохраняет HTML. Использует профиль для сохранения сессии.

> Замените URL, селекторы и учётные данные на реальные перед запуском.

```bash
autobrowser run-script --file examples/login.yaml --profile mysite
```

### screenshot.yaml — Скриншот страницы

Открывает [Scrapfly Automation Detector](https://scrapfly.io/web-scraping-tools/automation-detector), ждёт полной загрузки и сохраняет полноразмерный скриншот страницы в `output/scrapfly_detector.png`.

```bash
autobrowser run-script --file examples/screenshot.yaml
```

### yandex_warmup.yaml — Прогрев профиля Яндекса

Имитирует поведение реального пользователя в headful-режиме (с видимым окном браузера):
1. Заходит на ya.ru, вводит поисковый запрос посимвольно
2. Ждёт 15 секунд — если появится капча, можно решить вручную
3. Кликает по второму результату поиска
4. Скроллит страницу результата
5. Переходит на dzen.ru
6. Кликает по 3-й новости в ленте, скроллит

Записывает cookies и данные в профиль. Запускается перед основным поиском.

```bash
autobrowser run-script --file examples/yandex_warmup.yaml --profile yandex
```

### yandex_search.yaml — Поиск в Яндексе

Заходит на ya.ru, вводит поисковый запрос посимвольно (`action: type`, `delay: 80ms`) и получает текстовое содержимое страницы результатов. Работает в headful-режиме с профилем.

Посимвольный ввод (`type` вместо `fill`) нужен для обхода детектора ботов Яндекса — мгновенное заполнение поля вызывает капчу.

**Перед первым запуском прогрейте профиль:**

```bash
autobrowser run-script --file examples/yandex_warmup.yaml --profile yandex
```

**Запуск поиска:**

```bash
autobrowser run-script --file examples/yandex_search.yaml --profile yandex --output /tmp/yandex.json
```

### Общие параметры CLI

| Параметр | Описание |
|----------|----------|
| `--file` | Путь к YAML/JSON сценарию |
| `--profile` | Имя профиля браузера (сохраняет cookies и данные между запусками) |
| `--proxy` | Прокси: `socks5://user:pass@host:port` (переопределяет `session.proxy` из сценария) |
| `--viewport` | Размер окна: `WIDTHxHEIGHT`, например `1920x1080` (переопределяет `session.viewport`) |
| `--output` | Файл для сохранения результатов в JSON |

> Прокси также можно указать в секции `session` сценария: `proxy: socks5://user:pass@host:port`. CLI-флаг `--proxy` имеет приоритет.

### Доступные действия в сценариях

| Действие | Описание | Обязательные поля |
|----------|----------|-------------------|
| `open` | Открыть URL | `url` |
| `wait` | Ждать (таймаут или событие) | `timeout` или `waitUntil` |
| `find` | Найти элемент | `locator` |
| `click` | Кликнуть по элементу | `locator` (опционально `index`) |
| `fill` | Мгновенно заполнить поле | `locator`, `value` |
| `type` | Посимвольный ввод (имитация человека) | `locator`, `value` (опционально `delay`) |
| `press` | Нажать клавишу | `key` |
| `scroll` | Прокрутить страницу | `deltaX` или `deltaY` |
| `screenshot` | Сделать скриншот страницы | `file` (опционально `fullPage`) |
| `get-content` | Получить содержимое страницы | — |
| `get-cookies` | Получить cookies | — |
| `get-headers` | Получить HTTP-заголовки | — |
| `save` | Сохранить содержимое в файл | `file` |

---

## EN

YAML scenario examples for `autobrowser run-script`. Each file is a ready-to-run scenario.

### Installation

```bash
# Local
pip install -e .

# Or via Docker
docker compose build
```

### simple.yaml — Anti-detection check

Opens [Scrapfly Automation Detector](https://scrapfly.io/web-scraping-tools/automation-detector) and collects headers, cookies and text content. Runs in headless mode.

```bash
autobrowser run-script --file examples/simple.yaml
```

Save result to file:

```bash
autobrowser run-script --file examples/simple.yaml --output /tmp/simple.json
```

### login.yaml — Website authentication

Template for login automation: opens login page, fills username/password fields, clicks submit, waits for load and saves HTML. Uses profile to persist session.

> Replace URL, selectors and credentials with real values before running.

```bash
autobrowser run-script --file examples/login.yaml --profile mysite
```

### screenshot.yaml — Page screenshot

Opens [Scrapfly Automation Detector](https://scrapfly.io/web-scraping-tools/automation-detector), waits for full load and saves a full-page screenshot to `output/scrapfly_detector.png`.

```bash
autobrowser run-script --file examples/screenshot.yaml
```

### yandex_warmup.yaml — Yandex profile warmup

Simulates real user behavior in headful mode (visible browser window):
1. Opens ya.ru, types a search query character by character
2. Waits 15 seconds — if a captcha appears, it can be solved manually
3. Clicks the second search result
4. Scrolls the result page
5. Navigates to dzen.ru
6. Clicks the 3rd news article in the feed, scrolls it

Saves cookies and data to the profile. Run before the main search.

```bash
autobrowser run-script --file examples/yandex_warmup.yaml --profile yandex
```

### yandex_search.yaml — Yandex search

Navigates to ya.ru, types a search query character by character (`action: type`, `delay: 80ms`) and retrieves the text content of the search results page. Runs in headful mode with a profile.

Character-by-character input (`type` instead of `fill`) is required to bypass Yandex bot detection — instant field filling triggers a captcha.

**Warm up the profile before first run:**

```bash
autobrowser run-script --file examples/yandex_warmup.yaml --profile yandex
```

**Run the search:**

```bash
autobrowser run-script --file examples/yandex_search.yaml --profile yandex --output /tmp/yandex.json
```

### Common CLI parameters

| Parameter | Description |
|-----------|-------------|
| `--file` | Path to YAML/JSON scenario |
| `--profile` | Browser profile name (persists cookies and data between runs) |
| `--proxy` | Proxy: `socks5://user:pass@host:port` (overrides `session.proxy` from scenario) |
| `--viewport` | Viewport size: `WIDTHxHEIGHT`, e.g. `1920x1080` (overrides `session.viewport`) |
| `--output` | File to save JSON results |

> Proxy can also be set in the scenario `session` section: `proxy: socks5://user:pass@host:port`. The `--proxy` CLI flag takes priority.

### Available scenario actions

| Action | Description | Required fields |
|--------|-------------|-----------------|
| `open` | Navigate to URL | `url` |
| `wait` | Wait (timeout or event) | `timeout` or `waitUntil` |
| `find` | Find element | `locator` |
| `click` | Click element | `locator` (optional `index`) |
| `fill` | Instantly fill a field | `locator`, `value` |
| `type` | Type character by character (human-like) | `locator`, `value` (optional `delay`) |
| `press` | Press a key | `key` |
| `scroll` | Scroll page | `deltaX` or `deltaY` |
| `screenshot` | Take a page screenshot | `file` (optional `fullPage`) |
| `get-content` | Get page content | — |
| `get-cookies` | Get cookies | — |
| `get-headers` | Get HTTP headers | — |
| `save` | Save content to file | `file` |
