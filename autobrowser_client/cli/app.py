from __future__ import annotations

import json
from pathlib import Path
from typing import Annotated, Any

import typer

from autobrowser_client import __version__
from autobrowser_client.browser.engine import run_script as run_scenario
from autobrowser_client.browser.executor import _validate_output_path
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import Settings, load_settings
from autobrowser_client.errors import AppError, NotImplementedAppError
from autobrowser_client.logging_utils import configure_logging
from autobrowser_client.scenario import load_script
from autobrowser_client.updater import start_auto_update

APP_HELP = """[bold]autobrowser[/bold]: a stealth browser for scripts and AI agents.

Runs Chromium through [cyan]patchright[/cyan] or [cyan]cloakbrowser[/cyan] with persistent profiles,
fingerprints and proxies. Use it from YAML scenarios, an HTTP API or as an MCP server.
"""

APP_EPILOG = """[bold]Examples[/bold]

  [green]autobrowser install[/green]                                  download the browser
  [green]autobrowser run --url https://example.com[/green]            open a page, print JSON
  [green]autobrowser run-script --file examples/simple.yaml[/green]   run a scenario
  [green]autobrowser mcp[/green]                                      MCP server for agents
  [green]autobrowser help run-script[/green]                          help for a command

[bold]Environment[/bold]

  [cyan]AUTOBROWSER_BROWSER_ENGINE[/cyan]   patchright (default) or cloakbrowser
  [cyan]AUTOBROWSER_HEADLESS[/cyan]         true / false
  [cyan]AUTOBROWSER_PROFILE_DIR[/cyan]      directory of persistent profiles
  [cyan]AUTOBROWSER_PROXY_*[/cyan]          ENABLED, SCHEME, HOST, PORT, USERNAME, PASSWORD
  [cyan]AUTOBROWSER_LOG_LEVEL[/cyan]        DEBUG, INFO, WARNING, ERROR
  [cyan]AUTOBROWSER_NO_AUTO_UPDATE[/cyan]   1 turns off the binary self-update

Docs: [blue]https://github.com/foxzi/autobrowser-client[/blue]
"""

SCENARIOS = "Scenarios"
AGENTS = "AI agents"
SETUP = "Setup"
INFO = "Info"

app = typer.Typer(
    name="autobrowser",
    help=APP_HELP,
    epilog=APP_EPILOG,
    rich_markup_mode="rich",
    no_args_is_help=True,
    context_settings={"help_option_names": ["-h", "--help"]},
)

ConfigOpt = Annotated[
    Path | None,
    typer.Option(
        "--config", "-c", dir_okay=False, help="YAML/JSON config file.", show_default=False
    ),
]
ProxyOpt = Annotated[
    str | None,
    typer.Option("--proxy", help="Proxy URL: [cyan]socks5://user:pass@host:port[/cyan]."),
]
ViewportOpt = Annotated[
    str | None,
    typer.Option("--viewport", help="Viewport size WIDTHxHEIGHT, e.g. [cyan]1920x1080[/cyan]."),
]
OutputOpt = Annotated[
    Path | None, typer.Option("--output", "-o", help="Save the JSON result to a file.")
]


def _version_callback(value: bool) -> None:
    if value:
        typer.echo(__version__)
        raise typer.Exit()


@app.callback()
def _root(
    _version: Annotated[
        bool,
        typer.Option(
            "--version",
            "-V",
            callback=_version_callback,
            is_eager=True,
            help="Show the version and exit.",
        ),
    ] = False,
) -> None:
    pass


@app.command("help", rich_help_panel=INFO)
def help_cmd(
    ctx: typer.Context,
    command: Annotated[
        list[str] | None, typer.Argument(help="Command to describe, e.g. run-script.")
    ] = None,
) -> None:
    """Show help for autobrowser or one of its commands.

    [green]autobrowser help[/green]
    [green]autobrowser help mcp[/green]
    """
    target = ctx.find_root()
    for name in command or []:
        group = target.command
        sub = group.get_command(target, name) if hasattr(group, "get_command") else None
        if sub is None:
            raise typer.BadParameter(f"No such command: {name}", param_hint="COMMAND")
        target = sub.make_context(name, [], parent=target, resilient_parsing=True)
    text = target.command.get_help(target)
    if text:
        typer.echo(text)


def _parse_vars(items: list[str]) -> dict[str, str]:
    """Parse repeated NAME=VALUE options into a dict."""
    result: dict[str, str] = {}
    for item in items:
        name, sep, value = item.partition("=")
        if not sep or not name:
            raise typer.BadParameter(f"Invalid variable: '{item}', expected NAME=VALUE")
        result[name] = value
    return result


def _parse_viewport(viewport_str: str) -> dict[str, int]:
    """Parse viewport string like '1920x1080' into {width, height}."""
    parts = viewport_str.lower().split("x")
    if len(parts) != 2:
        raise typer.BadParameter(
            f"Invalid viewport format: '{viewport_str}', expected WIDTHxHEIGHT (e.g. 1920x1080)"
        )
    try:
        return {"width": int(parts[0]), "height": int(parts[1])}
    except ValueError:
        raise typer.BadParameter(
            f"Invalid viewport format: '{viewport_str}', expected WIDTHxHEIGHT (e.g. 1920x1080)"
        )


def _settings_from_options(
    config: Path | None,
    headless: bool | None = None,
    profile_dir: Path | None = None,
    user_agent: str | None = None,
    timeout: int | None = None,
    proxy: str | None = None,
    viewport: dict[str, int] | None = None,
) -> Settings:
    overrides: dict[str, Any] = {}
    if headless is not None:
        overrides.setdefault("browser", {})["headless"] = headless
    if profile_dir is not None:
        overrides.setdefault("profiles", {})["dir"] = str(profile_dir)
    if user_agent is not None:
        overrides.setdefault("browser", {})["userAgent"] = user_agent
    if timeout is not None:
        overrides.setdefault("timeouts", {})["actionMs"] = timeout
        overrides.setdefault("timeouts", {})["navigationMs"] = timeout
    if proxy is not None:
        _apply_proxy_url(proxy, overrides)
    if viewport is not None:
        overrides.setdefault("browser", {})["viewport"] = viewport

    settings = load_settings(config_path=config, overrides=overrides or None)
    configure_logging(settings.logging.level, settings.logging.file)
    return settings


def _apply_proxy_url(proxy_url: str, overrides: dict[str, Any]) -> None:
    """Parse proxy URL like socks5://user:pass@host:port into overrides."""
    from urllib.parse import urlparse

    parsed = urlparse(proxy_url)
    proxy_cfg = overrides.setdefault("proxy", {})
    proxy_cfg["enabled"] = True
    proxy_cfg["scheme"] = parsed.scheme or "socks5"
    proxy_cfg["host"] = parsed.hostname
    if parsed.port:
        proxy_cfg["port"] = parsed.port
    if parsed.username:
        proxy_cfg["username"] = parsed.username
    if parsed.password:
        proxy_cfg["password"] = parsed.password


@app.command("version", rich_help_panel=INFO)
def version() -> None:
    """Show the version."""
    typer.echo(__version__)


@app.command("update", rich_help_panel=SETUP)
def update(
    check: Annotated[
        bool, typer.Option("--check", help="Only report whether an update exists")
    ] = False,
) -> None:
    """Update the binary to the latest GitHub release.

    The new binary is checked against its [cyan].sha256[/cyan] and replaces the current one.
    [green]mcp[/green] and [green]serve[/green] also check for updates once a day in the background;
    set [cyan]AUTOBROWSER_NO_AUTO_UPDATE=1[/cyan] to turn that off. Not for pip/uv installs.
    """
    from autobrowser_client import updater

    latest = updater.latest_version()
    if not updater.is_newer(latest):
        typer.echo(f"Up to date: {__version__}")
        return
    if check:
        typer.echo(f"Update available: {__version__} -> {latest}")
        return
    try:
        path = updater.install_version(latest)
    except RuntimeError as exc:
        typer.echo(str(exc), err=True)
        raise typer.Exit(code=1) from exc
    typer.echo(f"Updated {path}: {__version__} -> {latest}")


@app.command("show-config", rich_help_panel=INFO)
def show_config(config: ConfigOpt = None) -> None:
    """Print the effective configuration as JSON (secrets masked).

    Defaults, then the [cyan]--config[/cyan] file, then [cyan]AUTOBROWSER_*[/cyan] variables.
    """
    settings = _settings_from_options(config=config)
    typer.echo(json.dumps(settings.safe_dump(), indent=2, ensure_ascii=False))


@app.command("validate-script", rich_help_panel=SCENARIOS)
def validate_script(
    file: Annotated[
        Path,
        typer.Option("--file", "-f", exists=True, dir_okay=False, help="Scenario file."),
    ],
) -> None:
    """Check a YAML/JSON scenario without starting a browser.

    [green]autobrowser validate-script -f examples/login.yaml[/green]
    """
    script = load_script(file)
    typer.echo(
        json.dumps(
            {
                "success": True,
                "steps": len(script.steps),
                "stopOnError": script.stop_on_error,
            },
            indent=2,
            ensure_ascii=False,
        )
    )


@app.command("generate-fingerprint", rich_help_panel=SETUP)
def generate_fingerprint_cmd(
    profile: Annotated[str, typer.Option("--profile", help="Profile directory path")],
    platform: Annotated[
        str, typer.Option("--platform", help="Target platform: windows, linux, android")
    ] = "windows",
    seed: Annotated[int | None, typer.Option("--seed", help="Fixed seed value")] = None,
    force: Annotated[
        bool, typer.Option("--force", help="Overwrite existing fingerprint.json")
    ] = False,
) -> None:
    """Generate fingerprint.json for a browser profile.

    [green]autobrowser generate-fingerprint --profile profiles/work --platform linux[/green]
    """
    from autobrowser_client.fingerprint.config import (
        FINGERPRINT_FILE,
        Platform,
        generate_fingerprint,
        save_fingerprint,
    )

    if platform not in ("windows", "linux", "android"):
        typer.echo(f"Unknown platform: {platform}. Use: windows, linux, android")
        raise typer.Exit(code=1)

    profile_dir = Path(profile)
    if not profile_dir.exists():
        profile_dir.mkdir(parents=True, exist_ok=True)
        typer.echo(f"Created profile directory: {profile_dir}")

    fp_path = profile_dir / FINGERPRINT_FILE
    if fp_path.exists() and not force:
        typer.echo(f"fingerprint.json already exists at {fp_path}")
        typer.echo("Use --force to overwrite")
        raise typer.Exit(code=1)

    fp = generate_fingerprint(seed=seed, platform=platform)  # type: ignore[arg-type]
    save_fingerprint(profile_dir, fp)

    typer.echo(json.dumps(fp.to_js_config(), indent=2, ensure_ascii=False))


@app.command("run-script", rich_help_panel=SCENARIOS)
def run_script(
    file: Annotated[
        Path,
        typer.Option("--file", "-f", exists=True, dir_okay=False, help="Scenario file."),
    ],
    config: ConfigOpt = None,
    profile: Annotated[
        str | None, typer.Option("--profile", "-p", help="Persistent profile name.")
    ] = None,
    proxy: ProxyOpt = None,
    viewport: ViewportOpt = None,
    output_file: OutputOpt = None,
    preserve_session: Annotated[
        bool,
        typer.Option("--preserve-session", help="Save open tabs for the next session."),
    ] = False,
    var: Annotated[
        list[str] | None,
        typer.Option("--var", help="Scenario variable NAME=VALUE; repeat for more."),
    ] = None,
    artifacts: Annotated[
        Path | None,
        typer.Option("--artifacts", help="Directory for screenshot and HTML of failed steps."),
    ] = None,
) -> None:
    """Run a YAML/JSON scenario and print the step results as JSON.

    Command-line options override the [cyan]session[/cyan] block of the scenario.
    Exit code is 0 when all steps succeed, 1 otherwise.

    [green]autobrowser run-script -f examples/simple.yaml[/green]
    [green]autobrowser run-script -f login.yaml -p work --proxy socks5://127.0.0.1:1080[/green]
    [green]autobrowser run-script -f login.yaml --var user=me --var password=secret[/green]
    """
    script = load_script(file)
    variables = _parse_vars(var or [])
    artifacts_dir = str(artifacts) if artifacts else script.error_artifacts
    if artifacts_dir:
        _validate_output_path(artifacts_dir)
    if script.output.file:
        _validate_output_path(script.output.file)
    effective_proxy = proxy or script.session.proxy

    # Viewport: CLI flag > scenario > config default
    effective_viewport: dict[str, int] | None = None
    if viewport:
        effective_viewport = _parse_viewport(viewport)
    elif script.session.viewport:
        effective_viewport = {
            "width": script.session.viewport.width,
            "height": script.session.viewport.height,
        }

    settings = _settings_from_options(
        config=config, proxy=effective_proxy, viewport=effective_viewport
    )
    effective_profile = profile or script.session.profile
    headless = script.session.headless
    user_agent = script.session.user_agent
    effective_preserve = preserve_session or script.session.preserve_session

    # Resolve fingerprint: scenario value can be bool, dict of overrides, or None
    fp_arg: bool | dict | None = None
    fp_raw = script.session.fingerprint
    if isinstance(fp_raw, bool):
        fp_arg = fp_raw
    elif fp_raw is not None:
        # FingerprintOverrides model -> dict of non-None fields
        fp_arg = {
            k: v for k, v in fp_raw.model_dump(by_alias=True).items() if v is not None
        }
        if not fp_arg:
            fp_arg = True  # fingerprint: {} means enable

    manager = SessionManager(settings)
    session = manager.create_session(
        urls=None,
        profile=effective_profile,
        user_agent=user_agent,
        headless=headless,
        preserve_session=effective_preserve,
        fingerprint=fp_arg,
    )

    try:
        result = session.call(
            run_scenario, session, script, vars=variables, artifacts_dir=artifacts_dir
        )

        all_ok = result["success"]
        output = {
            "success": all_ok,
            "sessionId": session.session_id,
            "profile": effective_profile,
            "steps": result["steps"],
            "vars": result["vars"],
        }

        if script.output.file:
            Path(script.output.file).write_text(
                json.dumps(output, indent=2, ensure_ascii=False, default=str),
                encoding="utf-8",
            )

        text = json.dumps(output, indent=2, ensure_ascii=False, default=str)

        if output_file:
            output_file.write_text(text, encoding="utf-8")
            typer.echo(f"Result saved to {output_file}")
        else:
            typer.echo(text)

        raise typer.Exit(code=0 if all_ok else 1)

    finally:
        session.stop(preserve_session=effective_preserve)


@app.command("run", rich_help_panel=SCENARIOS)
def run(
    url: Annotated[
        list[str], typer.Option("--url", "-u", help="URL to open; repeat for more tabs.")
    ],
    config: ConfigOpt = None,
    profile: Annotated[
        str | None, typer.Option("--profile", "-p", help="Persistent profile name.")
    ] = None,
    profile_dir: Annotated[
        Path | None,
        typer.Option("--profile-dir", file_okay=False, help="Directory of profiles."),
    ] = None,
    user_agent: Annotated[
        str | None, typer.Option("--user-agent", help="Override the User-Agent.")
    ] = None,
    headless: Annotated[
        bool | None,
        typer.Option("--headless/--headful", help="Hide or show the browser window."),
    ] = None,
    timeout: Annotated[
        int | None, typer.Option("--timeout", help="Navigation and action timeout, ms.")
    ] = None,
    proxy: ProxyOpt = None,
    viewport_str: ViewportOpt = None,
    output_file: OutputOpt = None,
    content_format: Annotated[
        str, typer.Option("--format", help="Page content format: html or text.")
    ] = "html",
) -> None:
    """Open URLs and print cookies, headers and page content as JSON.

    [green]autobrowser run -u https://example.com --format text[/green]
    [green]autobrowser run -u https://a.com -u https://b.com -o result.json[/green]
    """
    vp = _parse_viewport(viewport_str) if viewport_str else None
    settings = _settings_from_options(
        config=config,
        headless=headless,
        profile_dir=profile_dir,
        user_agent=user_agent,
        timeout=timeout,
        proxy=proxy,
        viewport=vp,
    )

    manager = SessionManager(settings)
    session = manager.create_session(
        urls=url,
        profile=profile,
        user_agent=user_agent,
        headless=headless,
    )

    try:

        def collect() -> dict[str, Any]:
            pages_data: dict[str, Any] = {}
            for page_id in session.list_pages():
                pages_data[page_id] = {
                    "url": session.get_page(page_id).url,
                    "headers": session.get_headers(page_id),
                    "content": session.get_content(page_id, fmt=content_format),
                }
            return {
                "success": True,
                "sessionId": session.session_id,
                "cookies": session.get_cookies(),
                "pages": pages_data,
            }

        result = session.call(collect)

        output = json.dumps(result, indent=2, ensure_ascii=False)

        if output_file:
            output_file.write_text(output, encoding="utf-8")
            typer.echo(f"Result saved to {output_file}")
        else:
            typer.echo(output)

    finally:
        session.stop()


@app.command("serve", rich_help_panel=AGENTS)
def serve(
    config: ConfigOpt = None,
    host: Annotated[str, typer.Option("--host", help="Address to listen on.")] = "127.0.0.1",
    port: Annotated[int, typer.Option("--port", help="Port to listen on.")] = 8080,
) -> None:
    """Start the HTTP API server (sessions, pages, scenarios).

    Set [cyan]AUTOBROWSER_API_TOKEN[/cyan] to require a bearer token.

    [green]autobrowser serve --host 0.0.0.0 --port 8080[/green]
    """
    import uvicorn

    from autobrowser_client.api.app import create_app

    settings = _settings_from_options(config=config)
    start_auto_update()
    api_app = create_app(settings)
    uvicorn.run(api_app, host=host, port=port)


@app.command("mcp", rich_help_panel=AGENTS)
def mcp(
    config: ConfigOpt = None,
    profile: Annotated[
        str | None, typer.Option("--profile", "-p", help="Persistent profile name.")
    ] = None,
    headless: Annotated[
        bool | None,
        typer.Option("--headless/--headed", help="Hide or show the browser window."),
    ] = None,
    proxy: ProxyOpt = None,
) -> None:
    """Run the MCP server over stdio for AI agents.

    Gives the agent [cyan]browser_*[/cyan] tools: navigate, snapshot, click, type, tabs and more.
    Register it in the client, e.g. Claude Code:

    [green]claude mcp add autobrowser -- autobrowser mcp[/green]
    """
    from autobrowser_client.mcp_server import run_server

    settings = _settings_from_options(config=config, headless=headless, proxy=proxy)
    start_auto_update()
    run_server(settings, profile)


@app.command("install-skill", rich_help_panel=AGENTS)
def install_skill(
    skills_dir: Annotated[
        Path,
        typer.Option("--dir", help="Skills directory of the agent client."),
    ] = Path("~/.claude/skills"),
) -> None:
    """Install the agent skill into DIR/autobrowser/SKILL.md.

    [green]autobrowser install-skill[/green]
    [green]autobrowser install-skill --dir ~/.config/nemo/skills[/green]
    """
    package = Path(__file__).resolve().parents[1]
    # Bundled into the wheel and the binary; fall back to the repo copy in a checkout
    source = package / "skill" / "SKILL.md"
    if not source.exists():
        source = package.parent / "skills" / "autobrowser" / "SKILL.md"
    target = skills_dir.expanduser() / "autobrowser" / "SKILL.md"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(source.read_text(encoding="utf-8"), encoding="utf-8")
    typer.echo(target)


@app.command("install", rich_help_panel=SETUP)
def install(config: ConfigOpt = None) -> None:
    """Download the browser for the configured engine.

    patchright installs Google Chrome (on Linux it may ask for sudo),
    cloakbrowser downloads its own Chromium build.

    [green]AUTOBROWSER_BROWSER_ENGINE=cloakbrowser autobrowser install[/green]
    """
    import subprocess

    settings = _settings_from_options(config=config)
    if settings.browser.engine == "cloakbrowser":
        from cloakbrowser.download import ensure_binary

        typer.echo(ensure_binary())
    else:
        # Google Chrome; on Linux this installs a system package and may ask for sudo.
        # Call the driver directly: in a frozen binary sys.executable is not Python.
        from patchright._impl._driver import compute_driver_executable, get_driver_env

        driver, cli = compute_driver_executable()
        raise typer.Exit(
            subprocess.call([driver, cli, "install", "chrome"], env=get_driver_env())
        )


# Panels are shown in the order of their first command
_PANELS = [SCENARIOS, AGENTS, SETUP, INFO]
app.registered_commands.sort(key=lambda c: _PANELS.index(c.rich_help_panel))


def main() -> None:
    try:
        app()
    except NotImplementedAppError as exc:
        typer.echo(str(exc), err=True)
        raise typer.Exit(code=2) from exc
    except AppError as exc:
        typer.echo(
            json.dumps(
                {
                    "success": False,
                    "error": {
                        "code": exc.code,
                        "message": exc.message,
                    },
                },
                ensure_ascii=False,
            ),
            err=True,
        )
        raise typer.Exit(code=1) from exc
