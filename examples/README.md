# Examples / Примеры сценариев

## RU

Примеры YAML-сценариев для `autobrowser run-script`. Каждый файл — готовый к запуску сценарий.

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

### fingerprint_com.yaml — Тест fingerprint.com

Посещает [fingerprint.com/demo/](https://fingerprint.com/demo/), ждёт асинхронный анализ отпечатка браузера (visitor ID, smart signals) и сохраняет результат:
- `output/fingerprint_com.png` — полноразмерный скриншот
- `output/fingerprint_com.txt` — текст страницы с visitor ID, браузером, IP, bot detection, VPN detection, incognito detection, tampering score

Полезно для проверки, что patchright не определяется как бот. Ожидаемый результат: `bot: notDetected`, `tampering: false`, `antiDetectBrowser: false`.

```bash
autobrowser run-script --file examples/fingerprint_com.yaml
```

### amiunique.yaml — Уникальность отпечатка браузера

Посещает [amiunique.org/fingerprint](https://amiunique.org/fingerprint), принимает cookies (баннер), ждёт загрузки JS-атрибутов и сохраняет результат:
- `output/amiunique.png` — полноразмерный скриншот с таблицами отпечатков
- `output/amiunique.txt` — текст с HTTP-заголовками, JS-атрибутами и показателями уникальности

Показывает, насколько уникален отпечаток браузера среди всех посетителей. Включает данные по User-Agent, языкам, разрешению экрана, шрифтам, Canvas, WebGL и другим параметрам.

```bash
autobrowser run-script --file examples/amiunique.yaml
```

---

## EN

YAML scenario examples for `autobrowser run-script`. Each file is a ready-to-run scenario.

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

### fingerprint_com.yaml — Fingerprint.com test

Visits [fingerprint.com/demo/](https://fingerprint.com/demo/), waits for async browser fingerprint analysis (visitor ID, smart signals) and saves the result:
- `output/fingerprint_com.png` — full-page screenshot
- `output/fingerprint_com.txt` — page text with visitor ID, browser, IP, bot detection, VPN detection, incognito detection, tampering score

Useful for verifying that patchright is not detected as a bot. Expected result: `bot: notDetected`, `tampering: false`, `antiDetectBrowser: false`.

```bash
autobrowser run-script --file examples/fingerprint_com.yaml
```

### amiunique.yaml — Browser fingerprint uniqueness

Visits [amiunique.org/fingerprint](https://amiunique.org/fingerprint), accepts the cookie banner, waits for JS attributes to load and saves the result:
- `output/amiunique.png` — full-page screenshot with fingerprint tables
- `output/amiunique.txt` — text with HTTP headers, JS attributes and uniqueness metrics

Shows how unique the browser fingerprint is among all visitors. Includes data on User-Agent, languages, screen resolution, fonts, Canvas, WebGL and other parameters.

```bash
autobrowser run-script --file examples/amiunique.yaml
```
