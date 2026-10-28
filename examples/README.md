# Examples / Примеры сценариев

---

## EN

YAML scenario examples for `autobrowser run-script`. Each file is a ready-to-run scenario.

### File list

| File | Description |
|---|---|
| `simple.yaml` | Open a page, collect headers, cookies, content |
| `screenshot.yaml` | Full-page screenshot (bot detection test page) |
| `login.yaml` | Fill and submit a form (httpbin.org demo form) |
| `browse.yaml` | Restore saved tabs and scroll each one |
| `fingerprint_com.yaml` | Test fingerprint.com bot detection |
| `amiunique.yaml` | Check browser fingerprint uniqueness |
| `yandex_search.yaml` | Search on Yandex with character-by-character input |
| `yandex_warmup.yaml` | Warm up a Yandex profile (headful, manual captcha OK) |
| `proxy.yaml` | Route traffic through a proxy server |
| `cloakbrowser.yaml` | Run with the cloakbrowser engine via env var |
| `cloudflare.yaml` | Pass a Cloudflare challenge with `solve-challenge` (cloakbrowser + `AUTOBROWSER_FINGERPRINT_PLATFORM=linux`) |
| `with-profile.yaml` | Persist cookies and session data across runs |
| `conditions.yaml` | `if` and `assert`: act on a banner only when it exists |
| `try-catch.yaml` | `try` / `catch` / `finally` and `optional` steps |
| `speedtest.yaml` | speedtest.net speed test: results from `evaluate`, screenshot in `finally` |
| `google-search.yaml` | Google search in a saved profile: top results, login and captcha flags, screenshot |
| `extract-loop.yaml` | `forEach` + `extract` + variables (quotes.toscrape.com) |

### Running scenarios

Three equivalent ways to run any scenario:

**1. Local venv**

```bash
autobrowser run-script --file examples/simple.yaml
```

**2. Docker (direct)**

```bash
docker run --rm \
  -v $(pwd)/examples:/scenarios \
  -v $(pwd)/output:/app/output \
  autobrowser-client:dev run-script --file /scenarios/simple.yaml
```

**3. Docker Compose**

```bash
docker compose run --rm \
  -v $(pwd)/examples:/scenarios \
  -v $(pwd)/output:/app/output \
  cli run-script --file /scenarios/simple.yaml
```

Build the image first if needed:

```bash
docker compose build
```

### Environment variables

| Variable | Description | Example |
|---|---|---|
| `AUTOBROWSER_BROWSER_ENGINE` | Browser engine: `patchright` (default) or `cloakbrowser` | `patchright` |
| `AUTOBROWSER_FINGERPRINT_PLATFORM` | cloakbrowser OS fingerprint: `windows` (default), `linux`, `macos` | `linux` |
| `AUTOBROWSER_PROFILE_DIR` | Root directory for named profiles | `.profiles` |
| `AUTOBROWSER_LOG_LEVEL` | Log verbosity: `DEBUG`, `INFO`, `WARNING`, `ERROR` | `INFO` |

Pass an env var to a single run:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run-script --file examples/cloakbrowser.yaml
```

Or via Docker:

```bash
docker run --rm \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -v $(pwd)/examples:/scenarios \
  autobrowser-client:dev run-script --file /scenarios/cloakbrowser.yaml
```

### CLI overrides

These flags take priority over values in the scenario file:

```bash
autobrowser run-script --file examples/proxy.yaml \
  --proxy socks5://user:pass@host:1080 \
  --viewport 1280x720 \
  --profile myprofile \
  --output /tmp/result.json
```

### How to add a new scenario

1. Create a `.yaml` file in `examples/` following `scenario.schema.json`.
2. Validate: `autobrowser validate-script --file examples/your.yaml`
3. Run: `autobrowser run-script --file examples/your.yaml`
4. Add a row to the file list above.

Minimal template:

```yaml
session:
  headless: true

steps:
  - action: open
    url: https://example.com

  - action: screenshot
    file: output/result.png
```

---

## RU

Примеры YAML-сценариев для `autobrowser run-script`. Каждый файл — готовый к запуску сценарий.

### Список файлов

| Файл | Описание |
|---|---|
| `simple.yaml` | Открыть страницу, собрать headers, cookies, контент |
| `screenshot.yaml` | Полноразмерный скриншот (тестовая страница bot detection) |
| `login.yaml` | Заполнение и отправка формы (демо-форма httpbin.org) |
| `browse.yaml` | Восстановить сохранённые вкладки и прокрутить каждую |
| `fingerprint_com.yaml` | Тест bot detection fingerprint.com |
| `amiunique.yaml` | Проверить уникальность отпечатка браузера |
| `yandex_search.yaml` | Поиск в Яндексе с посимвольным вводом |
| `yandex_warmup.yaml` | Прогрев профиля Яндекса (headful, капчу решить вручную) |
| `proxy.yaml` | Маршрутизация трафика через прокси |
| `cloakbrowser.yaml` | Запуск с движком cloakbrowser через env-переменную |
| `cloudflare.yaml` | Прохождение Cloudflare challenge шагом `solve-challenge` (cloakbrowser + `AUTOBROWSER_FINGERPRINT_PLATFORM=linux`) |
| `with-profile.yaml` | Сохранение cookies и данных сессии между запусками |
| `conditions.yaml` | `if` и `assert`: действие с баннером только если он есть |
| `try-catch.yaml` | `try` / `catch` / `finally` и `optional` шаги |
| `speedtest.yaml` | Тест скорости speedtest.net: результат через `evaluate`, скриншот в `finally` |
| `google-search.yaml` | Поиск в Google в сохранённом профиле: топ результатов, флаги входа и капчи, скриншот |
| `extract-loop.yaml` | `forEach` + `extract` + переменные (quotes.toscrape.com) |

### Способы запуска

Три равнозначных способа запустить любой сценарий:

**1. Локально через venv**

```bash
autobrowser run-script --file examples/simple.yaml
```

**2. Docker напрямую**

```bash
docker run --rm \
  -v $(pwd)/examples:/scenarios \
  -v $(pwd)/output:/app/output \
  autobrowser-client:dev run-script --file /scenarios/simple.yaml
```

**3. Docker Compose**

```bash
docker compose run --rm \
  -v $(pwd)/examples:/scenarios \
  -v $(pwd)/output:/app/output \
  cli run-script --file /scenarios/simple.yaml
```

Сборка образа перед первым запуском:

```bash
docker compose build
```

### Переменные окружения

| Переменная | Описание | Пример значения |
|---|---|---|
| `AUTOBROWSER_BROWSER_ENGINE` | Движок браузера: `patchright` (по умолчанию) или `cloakbrowser` | `patchright` |
| `AUTOBROWSER_FINGERPRINT_PLATFORM` | ОС-отпечаток cloakbrowser: `windows` (по умолчанию), `linux`, `macos` | `linux` |
| `AUTOBROWSER_PROFILE_DIR` | Корневая директория именованных профилей | `.profiles` |
| `AUTOBROWSER_LOG_LEVEL` | Уровень логирования: `DEBUG`, `INFO`, `WARNING`, `ERROR` | `INFO` |

Передать переменную для одного запуска:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser run-script --file examples/cloakbrowser.yaml
```

Или через Docker:

```bash
docker run --rm \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -v $(pwd)/examples:/scenarios \
  autobrowser-client:dev run-script --file /scenarios/cloakbrowser.yaml
```

### Переопределение через CLI

Флаги CLI имеют приоритет над значениями в файле сценария:

```bash
autobrowser run-script --file examples/proxy.yaml \
  --proxy socks5://user:pass@host:1080 \
  --viewport 1280x720 \
  --profile myprofile \
  --output /tmp/result.json
```

### Как добавить новый сценарий

1. Создайте `.yaml` файл в `examples/` по формату `scenario.schema.json`.
2. Валидация: `autobrowser validate-script --file examples/your.yaml`
3. Запуск: `autobrowser run-script --file examples/your.yaml`
4. Добавьте строку в таблицу файлов выше.

Минимальный шаблон:

```yaml
session:
  headless: true

steps:
  - action: open
    url: https://example.com

  - action: screenshot
    file: output/result.png
```
