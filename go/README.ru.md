# Эксперимент go-cdp

Одноразовый прототип: статический бинарник на Go (go-rod, чистый CDP, без Node и patchright)
управляет Chromium из cloakbrowser. Вопрос: проходит ли он проверки на ботов?

## Сборка и запуск

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

## Результаты (2026-11-22, headless, в Docker)

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

## Риск Runtime.enable

go-rod не вызывает `Runtime.enable` явно (использует `Page.enable`, `DOM.enable`,
`Runtime.evaluate`/`callFunctionOn`). Проверка CDP на browserscan показала Normal.
Встроенная ловушка (getter `stack` у Error + `console.debug`) показала «не обнаружено»
даже с `-runtime-enable`, то есть на Chrome 146 она не работает и ничего не доказывает.
Поэтому риск **не проверен**, известно только «browserscan не обнаружил».

## Размер и запуск

| | go-cdp | PyInstaller `autobrowser` |
|---|---|---|
| размер бинарника | 11 МБ | 77 МБ |
| `-h` / `--help` | 0,004 с | ~1,0 с |

Запуск Chromium через go-rod: ~110 мс до готового CDP-соединения.
