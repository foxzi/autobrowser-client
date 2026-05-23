session:
  headless: true
  viewport:
    width: 1366
    height: 900

steps:
  - action: open
    url: https://bot.sannysoft.com/
    waitUntil: networkidle
    timeout: 30000

  - action: wait
    timeout: 3000

  - action: screenshot
    file: output/engine-compare/__ENGINE__/sannysoft.png
    fullPage: true

  - action: save
    file: output/engine-compare/__ENGINE__/sannysoft.html

  - action: open
    url: https://abrahamjuliot.github.io/creepjs/
    waitUntil: networkidle
    timeout: 45000

  - action: wait
    timeout: 8000

  - action: screenshot
    file: output/engine-compare/__ENGINE__/creepjs.png
    fullPage: true

  - action: open
    url: https://www.browserscan.net/bot-detection
    waitUntil: networkidle
    timeout: 30000

  - action: wait
    timeout: 5000

  - action: screenshot
    file: output/engine-compare/__ENGINE__/browserscan.png
    fullPage: true

  - action: open
    url: https://arh.antoinevastel.com/bots/areyouheadless
    waitUntil: networkidle
    timeout: 20000

  - action: wait
    timeout: 2000

  - action: screenshot
    file: output/engine-compare/__ENGINE__/areyouheadless.png
    fullPage: true

  - action: open
    url: https://fingerprintjs.github.io/BotD/main/
    waitUntil: networkidle
    timeout: 30000

  - action: wait
    timeout: 4000

  - action: screenshot
    file: output/engine-compare/__ENGINE__/botd.png
    fullPage: true

stopOnError: false
