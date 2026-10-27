# Install autobrowser with an AI agent / Установка autobrowser через AI-агента

> **AI agent:** follow the [English](#english) section step by step. The Russian section is the same text for humans.
>
> **Человеку:** дайте агенту ссылку на этот файл и попросите: «Установи autobrowser по этой инструкции». Перевод — в разделе [Русский](#русский).

## English

You are installing autobrowser: an MCP server that gives you a stealth browser as `browser_*` tools, plus a skill that explains how to use them. Run the commands yourself. Ask the user only where a step says so.

Placeholders used below:

- `SOURCE` = `autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client`
- `SKILL_URL` = `https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md`

### Step 1. Check uv and git

Run `uv --version` and `git --version`.

- If git is missing, stop and ask the user to install git.
- If uv is missing, tell the user you are going to install it from the official installer and run:
  - Linux / macOS: `curl -LsSf https://astral.sh/uv/install.sh | sh`
  - Windows (PowerShell): `powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`

  Then check `uvx --version`. If the shell does not find it, use the full path printed by the installer (usually `~/.local/bin/uvx`).

### Step 2. Download the package and the browser

Linux / macOS:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser uvx --from "SOURCE" autobrowser install
```

Windows (PowerShell):

```powershell
$env:AUTOBROWSER_BROWSER_ENGINE="cloakbrowser"; uvx --from "SOURCE" autobrowser install
```

It takes about a minute and prints the path of the downloaded Chromium (stored in `~/.cloakbrowser`). No root is needed. If it fails, show the error to the user and stop.

### Step 3. Register the MCP server

Detect which client you run in and use the matching block. If the user named a proxy (`socks5://...` or `http://...`), append `--proxy <url>` after `autobrowser mcp` in the arguments.

**Claude Code** (the `claude` command exists):

```bash
claude mcp add autobrowser --scope user \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- uvx --from "SOURCE" autobrowser mcp
```

If it reports that `autobrowser` already exists, run `claude mcp remove autobrowser --scope user` and repeat.

**Codex CLI** - add to `~/.codex/config.toml`:

```toml
[mcp_servers.autobrowser]
command = "uvx"
args = ["--from", "SOURCE", "autobrowser", "mcp"]
env = { AUTOBROWSER_BROWSER_ENGINE = "cloakbrowser" }
```

**Clients with a JSON `mcpServers` config** - Cursor (`~/.cursor/mcp.json`), Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows) and others. Merge this entry into `mcpServers`, keep the existing entries:

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "uvx",
      "args": ["--from", "SOURCE", "autobrowser", "mcp"],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

For any other client, find where it keeps MCP servers and add a stdio server with the same command, arguments and environment. If you cannot find it, give the user the JSON above.

Always replace `SOURCE` with the real value. If `uvx` is not on the client's `PATH`, use the full path to `uvx` as the command.

### Step 4. Install the skill

**Claude Code:**

```bash
mkdir -p ~/.claude/skills/autobrowser
curl -fsSL SKILL_URL -o ~/.claude/skills/autobrowser/SKILL.md
```

On Windows use `%USERPROFILE%\.claude\skills\autobrowser\SKILL.md` and `Invoke-WebRequest SKILL_URL -OutFile <path>`.

**Other clients:** if the client supports skills or a rules / instructions file, save the content of `SKILL_URL` there. Otherwise skip this step and tell the user.

### Step 5. Verify and report

1. Claude Code: `claude mcp list` must show `autobrowser` as connected. Other clients: check the client's MCP status if it has one.
2. The new tools appear only after the client restarts or reloads MCP servers. If `browser_*` tools are not available to you yet, ask the user to restart the client and then run the check below.
3. Check: `browser_navigate(url="https://example.com")` must return `status` 200 and title "Example Domain". Then `browser_close()`.

Report to the user: what was installed, where the config was written, and whether the check passed. Mention:

- `--headed` after `autobrowser mcp` shows the browser window; `--profile NAME` keeps cookies between runs.
- Full docs: https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md
- Problems with Cloudflare, proxies or engines: https://github.com/foxzi/autobrowser-client/blob/main/docs/troubleshooting.md

## Русский

Инструкция для агента: установить autobrowser — MCP-сервер, который даёт агенту скрытный браузер в виде инструментов `browser_*`, и skill с правилами работы с ними. Агент сам выполняет команды и задаёт вопросы пользователю только там, где это указано.

Обозначения:

- `SOURCE` = `autobrowser-client[cloak] @ git+https://github.com/foxzi/autobrowser-client`
- `SKILL_URL` = `https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md`

### Шаг 1. Проверить uv и git

Выполнить `uv --version` и `git --version`.

- Если нет git — остановиться и попросить пользователя установить git.
- Если нет uv — предупредить пользователя и поставить официальным установщиком:
  - Linux / macOS: `curl -LsSf https://astral.sh/uv/install.sh | sh`
  - Windows (PowerShell): `powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`

  Затем проверить `uvx --version`. Если оболочка его не находит — использовать полный путь из вывода установщика (обычно `~/.local/bin/uvx`).

### Шаг 2. Скачать пакет и браузер

Linux / macOS:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser uvx --from "SOURCE" autobrowser install
```

Windows (PowerShell):

```powershell
$env:AUTOBROWSER_BROWSER_ENGINE="cloakbrowser"; uvx --from "SOURCE" autobrowser install
```

Занимает около минуты и печатает путь к скачанному Chromium (лежит в `~/.cloakbrowser`). Права root не нужны. При ошибке — показать её пользователю и остановиться.

### Шаг 3. Подключить MCP-сервер

Определить, в каком клиенте работает агент, и взять нужный блок. Если пользователь указал прокси (`socks5://...` или `http://...`), добавить `--proxy <url>` после `autobrowser mcp` в аргументах.

**Claude Code** (есть команда `claude`):

```bash
claude mcp add autobrowser --scope user \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- uvx --from "SOURCE" autobrowser mcp
```

Если сервер `autobrowser` уже есть — выполнить `claude mcp remove autobrowser --scope user` и повторить.

**Codex CLI** — добавить в `~/.codex/config.toml`:

```toml
[mcp_servers.autobrowser]
command = "uvx"
args = ["--from", "SOURCE", "autobrowser", "mcp"]
env = { AUTOBROWSER_BROWSER_ENGINE = "cloakbrowser" }
```

**Клиенты с JSON-конфигом `mcpServers`** — Cursor (`~/.cursor/mcp.json`), Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json` на macOS, `%APPDATA%\Claude\claude_desktop_config.json` на Windows) и другие. Добавить запись в `mcpServers`, не трогая существующие:

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "uvx",
      "args": ["--from", "SOURCE", "autobrowser", "mcp"],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

Для другого клиента — найти, где он хранит MCP-серверы, и добавить stdio-сервер с той же командой, аргументами и окружением. Если не удалось — отдать пользователю JSON выше.

`SOURCE` всегда заменять на реальное значение. Если `uvx` нет в `PATH` клиента — указать полный путь к `uvx` в `command`.

### Шаг 4. Установить skill

**Claude Code:**

```bash
mkdir -p ~/.claude/skills/autobrowser
curl -fsSL SKILL_URL -o ~/.claude/skills/autobrowser/SKILL.md
```

На Windows — путь `%USERPROFILE%\.claude\skills\autobrowser\SKILL.md` и `Invoke-WebRequest SKILL_URL -OutFile <путь>`.

**Другие клиенты:** если клиент поддерживает skills или файл правил / инструкций — сохранить туда содержимое `SKILL_URL`. Иначе пропустить шаг и сказать об этом пользователю.

### Шаг 5. Проверить и отчитаться

1. Claude Code: `claude mcp list` должен показать `autobrowser` как подключённый. В других клиентах — посмотреть статус MCP, если он есть.
2. Новые инструменты появляются только после перезапуска клиента или перезагрузки MCP-серверов. Если инструментов `browser_*` ещё нет — попросить пользователя перезапустить клиент и затем выполнить проверку.
3. Проверка: `browser_navigate(url="https://example.com")` должен вернуть `status` 200 и заголовок "Example Domain". Затем `browser_close()`.

Сообщить пользователю: что установлено, куда записан конфиг, прошла ли проверка. Упомянуть:

- `--headed` после `autobrowser mcp` показывает окно браузера; `--profile NAME` сохраняет cookies между запусками.
- Полная документация: https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md
- Проблемы с Cloudflare, прокси и движками: https://github.com/foxzi/autobrowser-client/blob/main/docs/troubleshooting.md
