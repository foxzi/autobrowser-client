# Install autobrowser with an AI agent / Установка autobrowser через AI-агента

> **AI agent:** follow the [English](#english) section step by step. The Russian section is the same text for humans.
>
> **Человеку:** дайте агенту ссылку на этот файл и попросите: «Установи autobrowser по этой инструкции». Перевод — в разделе [Русский](#русский).

## English

You are installing autobrowser: an MCP server that gives you a stealth browser as `browser_*` tools, plus a skill that explains how to use them. autobrowser is a single executable file, no Python or other tools are needed. Run the commands yourself. Ask the user only where a step says so.

Placeholders used below:

- `RELEASE` = `https://github.com/foxzi/autobrowser-client/releases/latest/download`
- `SKILL_URL` = `https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md`
- `BIN` = the full path of the installed binary from step 1

### Step 1. Download the binary

Pick the file for the OS and CPU (`uname -sm` on Linux / macOS):

| Platform | File | `BIN` |
|---|---|---|
| Linux x86_64 | `autobrowser-linux-amd64` | `~/.local/bin/autobrowser` |
| Linux aarch64 | `autobrowser-linux-arm64` | `~/.local/bin/autobrowser` |
| macOS Apple Silicon (arm64) | `autobrowser-macos-arm64` | `~/.local/bin/autobrowser` |
| Windows x64 | `autobrowser-windows-amd64.exe` | `%LOCALAPPDATA%\autobrowser\autobrowser.exe` |

For other platforms (for example macOS on Intel) there is no binary: stop and point the user to the uv install in https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md.

Linux / macOS (replace `FILE`):

```bash
mkdir -p ~/.local/bin
curl -fL RELEASE/FILE -o ~/.local/bin/autobrowser
chmod +x ~/.local/bin/autobrowser
~/.local/bin/autobrowser version
```

Windows (PowerShell):

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\autobrowser" | Out-Null
Invoke-WebRequest RELEASE/autobrowser-windows-amd64.exe -OutFile "$env:LOCALAPPDATA\autobrowser\autobrowser.exe"
& "$env:LOCALAPPDATA\autobrowser\autobrowser.exe" version
```

The `version` command must print a version number. Optionally verify the checksum: `RELEASE/FILE.sha256`.

### Step 2. Download the browser

Linux / macOS:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser BIN install
```

Windows (PowerShell):

```powershell
$env:AUTOBROWSER_BROWSER_ENGINE="cloakbrowser"; & "BIN" install
```

It takes about a minute and prints the path of the downloaded Chromium (stored in `~/.cloakbrowser`). No root is needed. If it fails, show the error to the user and stop.

### Step 3. Register the MCP server

Detect which client you run in and use the matching block. Always use the full path `BIN` (with `~` expanded) as the command: MCP clients may not have `~/.local/bin` on `PATH`. If the user named a proxy (`socks5://...` or `http://...`), append `--proxy <url>` after `mcp` in the arguments.

**Claude Code** (the `claude` command exists):

```bash
claude mcp add autobrowser --scope user \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- BIN mcp
```

If it reports that `autobrowser` already exists, run `claude mcp remove autobrowser --scope user` and repeat.

**Codex CLI** - add to `~/.codex/config.toml`:

```toml
[mcp_servers.autobrowser]
command = "BIN"
args = ["mcp"]
env = { AUTOBROWSER_BROWSER_ENGINE = "cloakbrowser" }
```

**Clients with a JSON `mcpServers` config** - Cursor (`~/.cursor/mcp.json`), Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows) and others. Merge this entry into `mcpServers`, keep the existing entries:

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "BIN",
      "args": ["mcp"],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

In JSON on Windows escape backslashes in the path (`C:\\Users\\...`).

For any other client, find where it keeps MCP servers and add a stdio server with the same command, arguments and environment. If the client has no field for environment variables, use `env` as the command (Linux / macOS): `env AUTOBROWSER_BROWSER_ENGINE=cloakbrowser BIN mcp`. If you cannot find where it keeps MCP servers, give the user the JSON above.

### Step 4. Install the skill

The skill is built into the binary. `BIN install-skill` writes it to `~/.claude/skills/autobrowser/SKILL.md` and prints the path.

**Claude Code:** run `BIN install-skill`.

**Other clients with a skills directory:** run `BIN install-skill --dir <skills directory>`, for example `--dir ~/.config/nemo/skills` for NemoAgent. The file goes to `<skills directory>/autobrowser/SKILL.md`.

**Clients without skills:** if the client has a rules / instructions file, add the content of the skill there (`BIN install-skill --dir <temp dir>` gives you the file, or download `SKILL_URL`). Otherwise skip this step and tell the user.

### Step 5. Verify and report

1. Claude Code: `claude mcp list` must show `autobrowser` as connected. Other clients: check the client's MCP status if it has one.
2. The new tools appear only after the client restarts or reloads MCP servers. If `browser_*` tools are not available to you yet, ask the user to restart the client and then run the check below.
3. Check: `browser_navigate(url="https://example.com")` must return `status` 200 and title "Example Domain". Then `browser_close()`.

Report to the user: what was installed, where the binary and the config are, and whether the check passed. Mention:

- To update, download the binary again (step 1).
- `--headed` after `mcp` shows the browser window; `--profile NAME` keeps cookies between runs.
- Full docs: https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md
- Problems with Cloudflare, proxies or engines: https://github.com/foxzi/autobrowser-client/blob/main/docs/troubleshooting.md

## Русский

Инструкция для агента: установить autobrowser — MCP-сервер, который даёт агенту скрытный браузер в виде инструментов `browser_*`, и skill с правилами работы с ними. autobrowser — один исполняемый файл, Python и другие инструменты не нужны. Агент сам выполняет команды и задаёт вопросы пользователю только там, где это указано.

Обозначения:

- `RELEASE` = `https://github.com/foxzi/autobrowser-client/releases/latest/download`
- `SKILL_URL` = `https://raw.githubusercontent.com/foxzi/autobrowser-client/main/skills/autobrowser/SKILL.md`
- `BIN` = полный путь к установленному файлу из шага 1

### Шаг 1. Скачать бинарь

Выбрать файл по ОС и процессору (`uname -sm` на Linux / macOS):

| Платформа | Файл | `BIN` |
|---|---|---|
| Linux x86_64 | `autobrowser-linux-amd64` | `~/.local/bin/autobrowser` |
| Linux aarch64 | `autobrowser-linux-arm64` | `~/.local/bin/autobrowser` |
| macOS Apple Silicon (arm64) | `autobrowser-macos-arm64` | `~/.local/bin/autobrowser` |
| Windows x64 | `autobrowser-windows-amd64.exe` | `%LOCALAPPDATA%\autobrowser\autobrowser.exe` |

Для других платформ (например, macOS на Intel) бинаря нет: остановиться и отправить пользователя к установке через uv в https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md.

Linux / macOS (подставить `FILE`):

```bash
mkdir -p ~/.local/bin
curl -fL RELEASE/FILE -o ~/.local/bin/autobrowser
chmod +x ~/.local/bin/autobrowser
~/.local/bin/autobrowser version
```

Windows (PowerShell):

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\autobrowser" | Out-Null
Invoke-WebRequest RELEASE/autobrowser-windows-amd64.exe -OutFile "$env:LOCALAPPDATA\autobrowser\autobrowser.exe"
& "$env:LOCALAPPDATA\autobrowser\autobrowser.exe" version
```

Команда `version` должна напечатать номер версии. Контрольная сумма при желании: `RELEASE/FILE.sha256`.

### Шаг 2. Скачать браузер

Linux / macOS:

```bash
AUTOBROWSER_BROWSER_ENGINE=cloakbrowser BIN install
```

Windows (PowerShell):

```powershell
$env:AUTOBROWSER_BROWSER_ENGINE="cloakbrowser"; & "BIN" install
```

Занимает около минуты и печатает путь к скачанному Chromium (хранится в `~/.cloakbrowser`). Root не нужен. При ошибке — показать её пользователю и остановиться.

### Шаг 3. Подключить MCP-сервер

Определить, в каком клиенте работает агент, и взять нужный блок. Командой всегда указывать полный путь `BIN` (с раскрытым `~`): у MCP-клиента `~/.local/bin` может не быть в `PATH`. Если пользователь назвал прокси (`socks5://...` или `http://...`), добавить `--proxy <url>` после `mcp` в аргументах.

**Claude Code** (есть команда `claude`):

```bash
claude mcp add autobrowser --scope user \
  -e AUTOBROWSER_BROWSER_ENGINE=cloakbrowser \
  -- BIN mcp
```

Если сервер `autobrowser` уже есть — выполнить `claude mcp remove autobrowser --scope user` и повторить.

**Codex CLI** — добавить в `~/.codex/config.toml`:

```toml
[mcp_servers.autobrowser]
command = "BIN"
args = ["mcp"]
env = { AUTOBROWSER_BROWSER_ENGINE = "cloakbrowser" }
```

**Клиенты с JSON-конфигом `mcpServers`** — Cursor (`~/.cursor/mcp.json`), Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json` на macOS, `%APPDATA%\Claude\claude_desktop_config.json` на Windows) и другие. Добавить запись в `mcpServers`, сохранив существующие:

```json
{
  "mcpServers": {
    "autobrowser": {
      "command": "BIN",
      "args": ["mcp"],
      "env": {"AUTOBROWSER_BROWSER_ENGINE": "cloakbrowser"}
    }
  }
}
```

В JSON на Windows обратные слэши в пути экранируются (`C:\\Users\\...`).

Для другого клиента — найти, где он хранит MCP-серверы, и добавить stdio-сервер с той же командой, аргументами и окружением. Если у клиента нет поля для переменных окружения — сделать командой `env` (Linux / macOS): `env AUTOBROWSER_BROWSER_ENGINE=cloakbrowser BIN mcp`. Если не удалось найти, где он хранит MCP-серверы, — отдать пользователю JSON выше.

### Шаг 4. Установить skill

Skill встроен в бинарь. `BIN install-skill` записывает его в `~/.claude/skills/autobrowser/SKILL.md` и печатает путь.

**Claude Code:** выполнить `BIN install-skill`.

**Другие клиенты с папкой skills:** выполнить `BIN install-skill --dir <папка skills>`, например `--dir ~/.config/nemo/skills` для NemoAgent. Файл попадёт в `<папка skills>/autobrowser/SKILL.md`.

**Клиенты без skills:** если у клиента есть файл правил / инструкций — добавить туда содержимое skill (файл даст `BIN install-skill --dir <временная папка>` или можно скачать `SKILL_URL`). Иначе пропустить шаг и сказать об этом пользователю.

### Шаг 5. Проверить и отчитаться

1. Claude Code: `claude mcp list` должен показать `autobrowser` как подключённый. Другие клиенты — посмотреть статус MCP, если клиент его показывает.
2. Новые инструменты появляются только после перезапуска клиента или перезагрузки MCP-серверов. Если `browser_*` ещё недоступны — попросить пользователя перезапустить клиент и затем выполнить проверку.
3. Проверка: `browser_navigate(url="https://example.com")` должен вернуть `status` 200 и заголовок "Example Domain". Затем `browser_close()`.

Отчитаться пользователю: что установлено, где лежат бинарь и конфиг, прошла ли проверка. Упомянуть:

- Для обновления — скачать бинарь заново (шаг 1).
- `--headed` после `mcp` показывает окно браузера; `--profile NAME` сохраняет cookies между запусками.
- Полная документация: https://github.com/foxzi/autobrowser-client/blob/main/docs/mcp.md
- Проблемы с Cloudflare, прокси или движками: https://github.com/foxzi/autobrowser-client/blob/main/docs/troubleshooting.md
