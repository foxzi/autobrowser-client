# Troubleshooting and recommendations / Решение проблем и рекомендации

## English

This document collects the hard cases met in practice (Cloudflare, proxies, engines) and how to deal with them. Every recommendation below was checked on real sites; results from a specific date and network may differ for you.

### 1. Diagnose first: what exactly does the site return

Before changing anything, find out which of these cases you have. Look at the main document response (status and headers), the page title and the frames.

| Signal | Meaning |
|---|---|
| `403`, `server: cloudflare`, no `cf-mitigated`, title `403 Forbidden` | Hard block by a WAF rule, decided on the request itself (usually User-Agent). No challenge will appear, retries do not help |
| `cf-mitigated: challenge`, title `Just a moment...`, text "Performing security verification" | Cloudflare challenge (JS / managed / Turnstile). Can be passed with `solve-challenge` |
| frame from `challenges.cloudflare.com` | Turnstile widget on the page; it may need a click |
| redirect to `?ki-cf-botcl=1` and then a challenge | Site-specific bot check that leads to a normal Cloudflare challenge |
| `200`, real page title | No protection triggered |

Useful checks:

- `https://httpbin.org/headers` - what the browser really sends (`User-Agent`, `Sec-Ch-Ua`).
- `https://httpbin.org/ip` - the exit IP (to make sure the proxy is used).
- A Python listener for document responses:

```python
def on_resp(r):
    if r.request.resource_type == "document":
        h = r.headers
        print(r.status, r.url, h.get("server"), h.get("cf-mitigated"), h.get("cf-ray"))
page.on("response", on_resp)
```

Comparing with `curl` or curl-impersonate is useful: if curl gets a challenge and the browser gets a plain 403, the browser itself (its fingerprint or headers) is the problem, not the IP.

### 2. Plain 403 without a challenge in headless patchright

**Symptom.** Article pages answer `403 Forbidden` right away, no `cf-mitigated`, no Turnstile frame. Changing the IP, the proxy or the profile does not help. `solve-challenge` fails with `Page blocked without challenge: HTTP 403`.

**Cause.** Headless Chrome sends `HeadlessChrome/<version>` in the User-Agent. Cloudflare WAF rules often block this token immediately.

**Fix.**

- Current version: nothing to do. When patchright runs headless and `userAgent` is not set, the session takes the real browser UA and replaces `HeadlessChrome` with `Chrome`. It costs one short extra browser launch on start.
- Older versions, or when you set the UA yourself: set a normal UA whose Chrome version matches the installed Chrome. Otherwise the UA disagrees with `Sec-Ch-Ua` and `navigator.userAgentData`, which is a bot signal.

```python
overrides = {"browser": {"headless": True,
    "userAgent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"}}
```

Environment variable: `AUTOBROWSER_USER_AGENT`. Check the version through `https://httpbin.org/headers`.

- Alternative: run with a window (`headless: false`, under Xvfb on a server). Headed patchright got the article with `200` and no challenge.

### 3. Cloudflare challenge (managed / Turnstile)

**Fix.** Add the `solve-challenge` step right after `open` (see `examples/cloudflare.yaml`):

```yaml
steps:
  - action: open
    url: https://www.criptonoticias.com/
  - action: solve-challenge
    timeout: 90000
```

In Python: `from autobrowser_client.browser.executor import solve_cf_challenge` and `solve_cf_challenge(page, 90000)` after `page.goto(...)`.

How it works:

- waits while the page shows a challenge (`cf-mitigated: challenge`, `window._cf_chl_opt`, or a `challenges.cloudflare.com` frame);
- after 4 s clicks the Turnstile checkbox, no more often than every 8 s;
- returns after 3 challenge-free seconds: `{"challenge": bool, "clicks": n, "url": ...}`.

Errors:

| Error | Meaning | What to do |
|---|---|---|
| `Page blocked without challenge: HTTP 4xx` | Hard block, see section 2 | Fix the UA or the engine; retries are useless |
| `Page blocked after challenge: HTTP 4xx` | The challenge passed but the page still refuses | Try another engine / fingerprint platform / exit IP |
| `Cloudflare challenge not passed in N ms (clicks: n)` | The challenge loops or does not solve | See section 4; try another IP or engine |

Notes:

- The same URL may show a challenge on one run and no challenge on the next. Typical runs needed 0 to 2 clicks, sometimes up to 5.
- Use a timeout of at least 60-90 s.
- Navigate with `wait_until="commit"` (or the `open` step) so that the challenge page does not make `goto` hang.

### 4. cloakbrowser: the challenge loops

**Symptom.** The checkbox passes, then the challenge appears again until the timeout.

**Cause.** The fingerprint OS does not match the host. On a desktop Linux host the default Windows fingerprint looped on criptonoticias.com; in the Docker image both platforms passed.

**Fix.** Set the fingerprint platform to the host OS:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser AUTOBROWSER_FINGERPRINT_PLATFORM=linux \
  autobrowser run-script --file examples/cloudflare.yaml
```

Config: `browser.fingerprintPlatform: linux` (`windows`, `linux`, `macos`; cloakbrowser only). Do not force `userAgent` with cloakbrowser: a UA that disagrees with the fingerprint is an easy bot signal.

### 5. Which engine to choose

| Situation | Recommendation |
|---|---|
| Default, most sites, Cloudflare with a challenge | patchright (headless UA is fixed automatically) |
| patchright is blocked after the challenge or loops | cloakbrowser with `AUTOBROWSER_FINGERPRINT_PLATFORM=linux` on a Linux host |
| Need a specific OS fingerprint (Windows / macOS) | cloakbrowser with `fingerprintPlatform`, preferably in Docker |
| Need a specific UA (mobile, other OS) | patchright with `userAgent`; non-Linux UAs get platform spoofing automatically (see `docs/configuration.md`) |

Neither engine passes every case every time. In tests on two Cloudflare sites, patchright passed all runs after the UA fix, while cloakbrowser through one proxy once failed with a timeout after 5 clicks. If one engine fails steadily, try the other.

### 6. SOCKS5 proxy and socks2http

Chromium cannot use a SOCKS5 proxy with authentication, so autobrowser starts a local HTTP relay (`socks2http`) that forwards to the upstream proxy.

**socks2http is optional.** The binary is searched in `proxy.socks2httpBin` / `AUTOBROWSER_SOCKS2HTTP_BIN`, then in `PATH`; if it is not found, the built-in Python relay is used (log line `Built-in proxy relay is ready`). If `socks2httpBin` is set explicitly but the file does not exist, the session fails with `socks2http binary not found`. In the Docker image the binary is at `/usr/local/bin/socks2http`. To use it outside Docker, download a release from https://github.com/foxzi/socks2http/releases, check the sha256 and put it in `PATH`:

```bash
curl -fsSL -o socks2http https://github.com/foxzi/socks2http/releases/download/v1.0.0/socks2http-linux-amd64
chmod +x socks2http && sudo mv socks2http /usr/local/bin/
```

**Check the proxy type before using it.** Some proxies speak only SOCKS5:

```bash
curl -s https://httpbin.org/ip                                 # direct IP
curl -s --socks5-hostname HOST:1080 https://httpbin.org/ip      # SOCKS5
curl -s -x http://HOST:1080 https://httpbin.org/ip              # HTTP (fails on a SOCKS5-only proxy)
```

**Pass the proxy:**

```bash
autobrowser run-script --file examples/proxy.yaml --proxy socks5://user:pass@HOST:1080
```

```python
"proxy": {"enabled": True, "scheme": "socks5", "host": "HOST", "port": 1080,
          "username": "", "password": ""}
```

Environment: `AUTOBROWSER_PROXY_ENABLED`, `AUTOBROWSER_PROXY_SCHEME`, `AUTOBROWSER_PROXY_HOST`, `AUTOBROWSER_PROXY_PORT`, `AUTOBROWSER_PROXY_USERNAME`, `AUTOBROWSER_PROXY_PASSWORD`. Verify the browser exit IP with `https://httpbin.org/ip` (`examples/proxy.yaml` does this).

In a scenario the proxy goes to `session.proxy`; the `--proxy` flag overrides it:

```yaml
session:
  proxy: "socks5://user:pass@HOST:1080"
```

**Timezone and language.** Without `AUTOBROWSER_GEOIP=true` the browser keeps the host timezone and language, which may not match the proxy country. See `browser.geoip` in [configuration.md](configuration.md).

**Not every site works through a proxy.** Through a SOCKS5 proxy the speedtest.net browser test (`examples/speedtest.yaml`) ends with `LATENCY TEST ERROR`, both headless and headful, while without the proxy it passes and curl through the same proxy reaches the Ookla servers. The cause is not established. If a site works directly but not through the proxy, compare both runs before changing the engine or fingerprint.

A different exit IP does not fix a plain 403 caused by the browser fingerprint (section 2).

### 7. Custom code: reading page globals in patchright

`page.evaluate()` in patchright runs in an isolated world by default and does not see variables set by page scripts (`window._cf_chl_opt` and similar always look `undefined`). To read them, pass `isolated_context=False`:

```python
page.evaluate("() => typeof window._cf_chl_opt", isolated_context=False)
```

DOM access (`document.title`, `querySelector`) works in both modes.

### 8. Checklist

1. Find out what the site returns (section 1).
2. Plain 403 without a challenge: check the UA through `httpbin.org/headers`; there must be no `HeadlessChrome`.
3. Challenge: add `solve-challenge` with a timeout of 60-90 s.
4. Loop on cloakbrowser: `AUTOBROWSER_FINGERPRINT_PLATFORM=linux`, or run in Docker.
5. Still blocked: switch the engine, then the exit IP.
6. Proxy: check its type with curl, check which relay started in the log, verify the exit IP.

## Русский

Здесь собраны сложные случаи, которые встречались на практике (Cloudflare, прокси, движки), и способы с ними справиться. Все рекомендации проверены на реальных сайтах, но в другой день и в другой сети результат может отличаться.

### 1. Сначала диагностика: что именно отдаёт сайт

Прежде чем что-то менять, определите, какой у вас случай. Смотрите на ответ основного документа (статус и заголовки), заголовок страницы и фреймы.

| Признак | Значение |
|---|---|
| `403`, `server: cloudflare`, нет `cf-mitigated`, заголовок `403 Forbidden` | Жёсткая блокировка правилом WAF по самому запросу (обычно по User-Agent). Проверки не будет, повторы не помогут |
| `cf-mitigated: challenge`, заголовок `Just a moment...`, текст "Performing security verification" | Проверка Cloudflare (JS / managed / Turnstile). Проходится шагом `solve-challenge` |
| фрейм с `challenges.cloudflare.com` | Виджет Turnstile на странице, может потребоваться клик |
| переход на `?ki-cf-botcl=1`, затем проверка | Проверка ботов на стороне сайта, ведущая к обычной проверке Cloudflare |
| `200`, настоящий заголовок страницы | Защита не сработала |

Полезные проверки:

- `https://httpbin.org/headers` - что браузер реально отправляет (`User-Agent`, `Sec-Ch-Ua`).
- `https://httpbin.org/ip` - внешний IP (убедиться, что прокси используется).
- Обработчик ответов документов на Python:

```python
def on_resp(r):
    if r.request.resource_type == "document":
        h = r.headers
        print(r.status, r.url, h.get("server"), h.get("cf-mitigated"), h.get("cf-ray"))
page.on("response", on_resp)
```

Полезно сравнить с `curl` или curl-impersonate: если curl получает проверку, а браузер получает обычный 403, проблема в самом браузере (его отпечатке или заголовках), а не в IP.

### 2. Обычный 403 без проверки у patchright без окна

**Признак.** Страницы статей сразу отвечают `403 Forbidden`, нет `cf-mitigated` и фрейма Turnstile. Смена IP, прокси или профиля не помогает. `solve-challenge` падает с `Page blocked without challenge: HTTP 403`.

**Причина.** Chrome без окна отправляет в User-Agent `HeadlessChrome/<версия>`. Правила WAF Cloudflare часто блокируют его сразу.

**Решение.**

- В текущей версии ничего делать не нужно. Если patchright запущен без окна и `userAgent` не задан, сессия берёт настоящий UA браузера и заменяет `HeadlessChrome` на `Chrome`. Это стоит одного короткого дополнительного запуска браузера при старте.
- В старых версиях или если вы задаёте UA сами: укажите обычный UA с версией установленного Chrome. Иначе UA расходится с `Sec-Ch-Ua` и `navigator.userAgentData`, а это признак бота.

```python
overrides = {"browser": {"headless": True,
    "userAgent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"}}
```

Переменная окружения: `AUTOBROWSER_USER_AGENT`. Версию проверяйте через `https://httpbin.org/headers`.

- Альтернатива: запуск с окном (`headless: false`, на сервере под Xvfb). patchright с окном получил статью с `200` и без проверки.

### 3. Проверка Cloudflare (managed / Turnstile)

**Решение.** Добавьте шаг `solve-challenge` сразу после `open` (см. `examples/cloudflare.yaml`):

```yaml
steps:
  - action: open
    url: https://www.criptonoticias.com/
  - action: solve-challenge
    timeout: 90000
```

В Python: `from autobrowser_client.browser.executor import solve_cf_challenge` и `solve_cf_challenge(page, 90000)` после `page.goto(...)`.

Как работает:

- ждёт, пока на странице есть проверка (`cf-mitigated: challenge`, `window._cf_chl_opt` или фрейм `challenges.cloudflare.com`);
- через 4 с кликает чекбокс Turnstile, не чаще раза в 8 с;
- возвращается после 3 секунд без проверки: `{"challenge": bool, "clicks": n, "url": ...}`.

Ошибки:

| Ошибка | Значение | Что делать |
|---|---|---|
| `Page blocked without challenge: HTTP 4xx` | Жёсткая блокировка, см. раздел 2 | Исправить UA или сменить движок; повторы бесполезны |
| `Page blocked after challenge: HTTP 4xx` | Проверка пройдена, но страница всё равно отказывает | Попробовать другой движок / платформу отпечатка / внешний IP |
| `Cloudflare challenge not passed in N ms (clicks: n)` | Проверка зацикливается или не решается | См. раздел 4; попробовать другой IP или движок |

Замечания:

- Один и тот же URL может в одном прогоне показать проверку, а в следующем нет. Обычно хватало 0-2 кликов, иногда до 5.
- Таймаут ставьте не меньше 60-90 с.
- Переходите с `wait_until="commit"` (или шагом `open`), чтобы страница проверки не подвешивала `goto`.

### 4. cloakbrowser: проверка зацикливается

**Признак.** Чекбокс проходит, затем проверка появляется снова, и так до таймаута.

**Причина.** ОС отпечатка не совпадает с хостом. На десктопном Linux-хосте Windows-отпечаток по умолчанию зацикливался на criptonoticias.com; в Docker-образе проходили обе платформы.

**Решение.** Задайте платформу отпечатка как у хоста:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser AUTOBROWSER_FINGERPRINT_PLATFORM=linux \
  autobrowser run-script --file examples/cloudflare.yaml
```

В конфиге: `browser.fingerprintPlatform: linux` (`windows`, `linux`, `macos`; только cloakbrowser). Не задавайте `userAgent` для cloakbrowser: UA, не совпадающий с отпечатком, легко выдаёт бота.

### 5. Какой движок выбрать

| Ситуация | Рекомендация |
|---|---|
| По умолчанию, большинство сайтов, Cloudflare с проверкой | patchright (UA без окна исправляется автоматически) |
| patchright блокируется после проверки или зацикливается | cloakbrowser с `AUTOBROWSER_FINGERPRINT_PLATFORM=linux` на Linux-хосте |
| Нужен отпечаток конкретной ОС (Windows / macOS) | cloakbrowser с `fingerprintPlatform`, лучше в Docker |
| Нужен конкретный UA (мобильный, другая ОС) | patchright с `userAgent`; для не-Linux UA подмена платформы включается автоматически (см. `docs/configuration.md`) |

Ни один движок не проходит все случаи каждый раз. В тестах на двух сайтах с Cloudflare patchright после исправления UA прошёл все прогоны, а cloakbrowser через один из прокси однажды упал по таймауту после 5 кликов. Если один движок стабильно не проходит, попробуйте другой.

### 6. SOCKS5-прокси и socks2http

Chromium не умеет работать с SOCKS5-прокси с авторизацией, поэтому autobrowser запускает локальный HTTP-релей (`socks2http`), который пересылает трафик на внешний прокси.

**socks2http не обязателен.** Программа ищется в `proxy.socks2httpBin` / `AUTOBROWSER_SOCKS2HTTP_BIN`, затем в `PATH`; если её нет, используется встроенный relay на Python (в логе `Built-in proxy relay is ready`). Если `socks2httpBin` задан явно, а файла нет, сессия падает с `socks2http binary not found`. В Docker-образе программа лежит в `/usr/local/bin/socks2http`. Чтобы использовать её без Docker, скачайте релиз с https://github.com/foxzi/socks2http/releases, проверьте sha256 и положите в `PATH`:

```bash
curl -fsSL -o socks2http https://github.com/foxzi/socks2http/releases/download/v1.0.0/socks2http-linux-amd64
chmod +x socks2http && sudo mv socks2http /usr/local/bin/
```

**Проверьте тип прокси до использования.** Некоторые прокси работают только как SOCKS5:

```bash
curl -s https://httpbin.org/ip                                 # прямой IP
curl -s --socks5-hostname HOST:1080 https://httpbin.org/ip      # SOCKS5
curl -s -x http://HOST:1080 https://httpbin.org/ip              # HTTP (не работает на SOCKS5-only прокси)
```

**Передача прокси:**

```bash
autobrowser run-script --file examples/proxy.yaml --proxy socks5://user:pass@HOST:1080
```

```python
"proxy": {"enabled": True, "scheme": "socks5", "host": "HOST", "port": 1080,
          "username": "", "password": ""}
```

Переменные окружения: `AUTOBROWSER_PROXY_ENABLED`, `AUTOBROWSER_PROXY_SCHEME`, `AUTOBROWSER_PROXY_HOST`, `AUTOBROWSER_PROXY_PORT`, `AUTOBROWSER_PROXY_USERNAME`, `AUTOBROWSER_PROXY_PASSWORD`. Внешний IP браузера проверяйте через `https://httpbin.org/ip` (это делает `examples/proxy.yaml`).

В сценарии прокси задаётся в `session.proxy`; флаг `--proxy` имеет приоритет:

```yaml
session:
  proxy: "socks5://user:pass@HOST:1080"
```

**Часовой пояс и язык.** Без `AUTOBROWSER_GEOIP=true` у браузера остаются часовой пояс и язык хоста, которые могут не совпадать со страной прокси. См. `browser.geoip` в [configuration.md](configuration.md).

**Не все сайты работают через прокси.** Через SOCKS5-прокси тест speedtest.net в браузере (`examples/speedtest.yaml`) завершается ошибкой `LATENCY TEST ERROR` и без окна, и с окном, а без прокси проходит; curl через тот же прокси до серверов Ookla доходит. Причина не установлена. Если сайт работает напрямую, но не через прокси, сравните оба запуска, прежде чем менять движок или отпечаток.

Другой внешний IP не исправит обычный 403, вызванный отпечатком браузера (раздел 2).

### 7. Свой код: чтение глобальных переменных страницы в patchright

`page.evaluate()` в patchright по умолчанию работает в изолированном контексте и не видит переменные, заданные скриптами страницы (`window._cf_chl_opt` и подобные всегда выглядят как `undefined`). Чтобы их прочитать, передайте `isolated_context=False`:

```python
page.evaluate("() => typeof window._cf_chl_opt", isolated_context=False)
```

Доступ к DOM (`document.title`, `querySelector`) работает в обоих режимах.

### 8. Чек-лист

1. Выясните, что отдаёт сайт (раздел 1).
2. Обычный 403 без проверки: проверьте UA через `httpbin.org/headers`, в нём не должно быть `HeadlessChrome`.
3. Проверка: добавьте `solve-challenge` с таймаутом 60-90 с.
4. Зацикливание на cloakbrowser: `AUTOBROWSER_FINGERPRINT_PLATFORM=linux` или запуск в Docker.
5. Всё ещё блокирует: смените движок, затем внешний IP.
6. Прокси: проверьте тип через curl, посмотрите в логе, какой relay запущен, проверьте внешний IP.
