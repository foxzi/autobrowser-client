# MCP server / MCP-сервер

## English

`autobrowser mcp` runs a Model Context Protocol server over stdio. An AI agent (Claude Code, Claude Desktop, Cursor and others) gets a stealth browser as a set of `browser_*` tools.

### Install: single binary

Each release has a self-contained `autobrowser` executable for Linux (amd64, arm64), macOS (arm64) and Windows (amd64): Python and dependencies are inside. Download it from [Releases](https://github.com/foxzi/autobrowser-client/releases/latest), make it executable and use its full path as the MCP command:

```bash
curl -fL https://github.com/foxzi/autobrowser-client/releases/latest/download/autobrowser-linux-amd64 -o ~/.local/bin/autobrowser
chmod +x ~/.local/bin/autobrowser
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser ~/.local/bin/autobrowser install
claude mcp add autobrowser -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser -- ~/.local/bin/autobrowser mcp
```

Step-by-step guide for an agent: [install-agent.md](install-agent.md). The binary unpacks itself into a temporary directory on every start, so the first response takes a few seconds.

To build it yourself: `./scripts/build-binary.sh` (needs Python 3.11+, result in `dist/autobrowser`).

### Install: uv

You only need [uv](https://docs.astral.sh/uv/getting-started/installation/). uv downloads Python and all dependencies by itself, isolated from the system.

The recommended engine is cloakbrowser. It downloads its own Chromium into `~/.cloakbrowser` on the first start, with no root and no extra steps.

Claude Code:

```bash
claude mcp add autobrowser \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- uvx --from "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client" autobrowser mcp
```

Any client with a JSON config (`mcpServers`):

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "uvx",
      "args": [
        "--from", "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client",
        "autobrowser", "mcp"
      ],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

To pin a release, add the tag to the URL: `git+https://github.com/foxzi/autobrowser-client@v0.1.0`. See [releases.md](releases.md).

The first start downloads dependencies and the browser, which takes about a minute. To do it in advance, run the same `uvx --from ... autobrowser install` command with the same environment.

The patchright engine (the default when `AUTOBROWSER_BROWSER_ENGINE` is not set) needs Google Chrome. Install it yourself, or run `autobrowser install`, which on Linux installs a system package and may ask for sudo.

### Options

```
autobrowser mcp [--headless/--headed] [--profile NAME] [--proxy socks5://user:pass@host:port] [--config FILE]
```

- `--headed` shows the browser window. It is also needed for a few Cloudflare-protected sites.
- `--profile` keeps cookies and storage between runs in `.profiles/NAME`. The path is relative to the working directory, and `AUTOBROWSER_PROFILE_DIR` changes it.
- Every setting from [`configuration.md`](configuration.md) also works as an environment variable in the `env` block.

### Tools

There is one browser per server process. It starts on the first call and closes with the server. `page_id` is optional everywhere and defaults to the first tab.

| Tool | Purpose |
|---|---|
| `browser_navigate` | open a URL |
| `browser_history` | back / forward / reload |
| `browser_snapshot` | accessibility tree with `[ref=eN]` marks |
| `browser_screenshot` | PNG of the viewport, the full page or one element |
| `browser_click`, `browser_hover` | act on an element |
| `browser_type` | fill an input, with optional Enter |
| `browser_press_key` | press a key |
| `browser_select_option` | choose in a `<select>` |
| `browser_file_upload` | attach local files |
| `browser_evaluate` | run JS in the page |
| `browser_wait_for` | wait for text to appear or disappear |
| `browser_get_text` | visible page text |
| `browser_tabs` | list / new / close tabs |
| `browser_logs` | console, network and dialogs log |
| `browser_handle_dialogs` | accept or dismiss alert/confirm/prompt |
| `browser_solve_challenge` | wait through a Cloudflare challenge |
| `browser_close` | close the browser |

Element tools take `target` and `by`. By default `target` is a ref from the latest snapshot (`e12`). With `by` set to `css`, `xpath`, `text`, `id`, `name`, `class` or `attribute` it is a locator value.

Failures come back as tool errors with the browser's message, for example a timeout while waiting for an element.

### Skill

[`skills/autobrowser/SKILL.md`](../skills/autobrowser/SKILL.md) teaches the agent how to use the tools: the snapshot/ref loop, waits, Cloudflare handling, tabs, dialogs and safety rules. For Claude Code, install it for all projects:

```bash
mkdir -p ~/.claude/skills/autobrowser
curl -fsSL https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md \
  -o ~/.claude/skills/autobrowser/SKILL.md
```

or copy the `skills/autobrowser` folder to `.claude/skills/` of a project. Other agents can take the file as a system prompt or rules file.

### Security

The agent controls a real browser. `browser_file_upload` can read any local file the server process can read, and `browser_evaluate` runs arbitrary JS in the page. Connect the server only to agents you trust.

## Русский

`autobrowser mcp` запускает сервер Model Context Protocol через stdio. AI-агент (Claude Code, Claude Desktop, Cursor и другие) получает стелс-браузер в виде набора инструментов `browser_*`.

### Установка: один бинарь

В каждом релизе есть самодостаточный исполняемый файл `autobrowser` для Linux (amd64, arm64), macOS (arm64) и Windows (amd64): Python и зависимости внутри. Скачайте его со страницы [Releases](https://github.com/foxzi/autobrowser-client/releases/latest), сделайте исполняемым и укажите полный путь как команду MCP:

```bash
curl -fL https://github.com/foxzi/autobrowser-client/releases/latest/download/autobrowser-linux-amd64 -o ~/.local/bin/autobrowser
chmod +x ~/.local/bin/autobrowser
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser ~/.local/bin/autobrowser install
claude mcp add autobrowser -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser -- ~/.local/bin/autobrowser mcp
```

Пошаговая инструкция для агента: [install-agent.md](install-agent.md). Бинарь при каждом запуске распаковывается во временный каталог, поэтому первый ответ занимает несколько секунд.

Собрать самому: `./scripts/build-binary.sh` (нужен Python 3.11+, результат в `dist/autobrowser`).

### Установка: uv

Нужен только [uv](https://docs.astral.sh/uv/getting-started/installation/). Он сам скачает Python и все зависимости и изолирует их от системы.

Рекомендуемый движок: cloakbrowser. При первом запуске он сам скачивает свой Chromium в `~/.cloakbrowser`, без root и без дополнительных шагов.

Claude Code:

```bash
claude mcp add autobrowser \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- uvx --from "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client" autobrowser mcp
```

Любой клиент с JSON-конфигом (`mcpServers`):

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "uvx",
      "args": [
        "--from", "autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client",
        "autobrowser", "mcp"
      ],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

Чтобы зафиксировать релиз, добавьте тег к URL: `git+https://github.com/foxzi/autobrowser-client@v0.1.0`. Подробнее в [releases.md](releases.md).

Первый запуск скачивает зависимости и браузер, это занимает около минуты. Чтобы сделать это заранее, выполните ту же команду `uvx --from ... autobrowser install` с теми же переменными окружения.

Движку patchright (по умолчанию, если `AUTOBROWSER_BROWSER_ENGINE` не задан) нужен Google Chrome. Установите его сами или выполните `autobrowser install`: на Linux команда ставит системный пакет и может запросить sudo.

### Параметры

```
autobrowser mcp [--headless/--headed] [--profile NAME] [--proxy socks5://user:pass@host:port] [--config FILE]
```

- `--headed` показывает окно браузера. Это нужно и для некоторых сайтов за Cloudflare.
- `--profile` сохраняет cookies и storage между запусками в `.profiles/NAME`. Путь считается от рабочей директории, изменить его можно через `AUTOBROWSER_PROFILE_DIR`.
- Любую настройку из [`configuration.md`](configuration.md) можно задать переменной окружения в блоке `env`.

### Инструменты

На один процесс сервера приходится один браузер. Он запускается при первом вызове и закрывается вместе с сервером. `page_id` везде необязателен, по умолчанию используется первая вкладка.

| Инструмент | Назначение |
|---|---|
| `browser_navigate` | открыть URL |
| `browser_history` | назад / вперёд / перезагрузка |
| `browser_snapshot` | дерево доступности с метками `[ref=eN]` |
| `browser_screenshot` | PNG области просмотра, всей страницы или элемента |
| `browser_click`, `browser_hover` | действие с элементом |
| `browser_type` | заполнить поле, по желанию нажать Enter |
| `browser_press_key` | нажать клавишу |
| `browser_select_option` | выбрать в `<select>` |
| `browser_file_upload` | прикрепить локальные файлы |
| `browser_evaluate` | выполнить JS на странице |
| `browser_wait_for` | дождаться появления или исчезновения текста |
| `browser_get_text` | видимый текст страницы |
| `browser_tabs` | список, новая вкладка, закрыть вкладку |
| `browser_logs` | журнал консоли, сети и диалогов |
| `browser_handle_dialogs` | принимать или отклонять alert/confirm/prompt |
| `browser_solve_challenge` | дождаться прохождения проверки Cloudflare |
| `browser_close` | закрыть браузер |

Инструменты для элементов принимают `target` и `by`. По умолчанию `target` — это ref из последнего snapshot (`e12`). Если `by` равен `css`, `xpath`, `text`, `id`, `name`, `class` или `attribute`, то `target` — значение локатора.

Ошибки возвращаются как ошибки инструмента с сообщением браузера, например таймаут ожидания элемента.

### Skill

[`skills/autobrowser/SKILL.md`](../skills/autobrowser/SKILL.md) объясняет агенту, как пользоваться инструментами: цикл snapshot/ref, ожидания, Cloudflare, вкладки, диалоги и правила безопасности. Для Claude Code установите его для всех проектов:

```bash
mkdir -p ~/.claude/skills/autobrowser
curl -fsSL https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md \
  -o ~/.claude/skills/autobrowser/SKILL.md
```

или скопируйте папку `skills/autobrowser` в `.claude/skills/` проекта. Другим агентам файл можно подключить как системный промпт или файл правил.

### Безопасность

Агент управляет настоящим браузером. `browser_file_upload` может прочитать любой локальный файл, доступный процессу сервера, а `browser_evaluate` выполняет произвольный JS на странице. Подключайте сервер только к агентам, которым доверяете.
