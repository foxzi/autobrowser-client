from __future__ import annotations

import json
from pathlib import Path
from typing import Annotated, Any

import typer

from autobrowser_client import __version__
from autobrowser_client.browser.executor import execute_script
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import Settings, load_settings
from autobrowser_client.errors import AppError, NotImplementedAppError
from autobrowser_client.logging_utils import configure_logging
from autobrowser_client.scenario import load_script

app = typer.Typer(help="CLI for autobrowser-client")


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


@app.command("version")
def version() -> None:
    typer.echo(__version__)


@app.command("show-config")
def show_config(
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
) -> None:
    settings = _settings_from_options(config=config)
    typer.echo(json.dumps(settings.safe_dump(), indent=2, ensure_ascii=False))


@app.command("validate-script")
def validate_script(
    file: Annotated[Path, typer.Option("--file", exists=True, dir_okay=False)],
) -> None:
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


@app.command("generate-fingerprint")
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
    """Generate fingerprint.json for a browser profile."""
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


@app.command("run-script")
def run_script(
    file: Annotated[Path, typer.Option("--file", exists=True, dir_okay=False)],
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
    profile: Annotated[str | None, typer.Option("--profile")] = None,
    proxy: Annotated[
        str | None,
        typer.Option("--proxy", help="Proxy URL: socks5://user:pass@host:port"),
    ] = None,
    viewport: Annotated[
        str | None,
        typer.Option("--viewport", help="Viewport size: WIDTHxHEIGHT (e.g. 1920x1080)"),
    ] = None,
    output_file: Annotated[
        Path | None, typer.Option("--output", help="Save result to file")
    ] = None,
    preserve_session: Annotated[
        bool, typer.Option("--preserve-session", help="Save open tabs for next session")
    ] = False,
) -> None:
    script = load_script(file)
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
        results = session.call(
            execute_script,
            session,
            script.steps,
            stop_on_error=script.stop_on_error,
        )

        all_ok = all(r["success"] for r in results)
        output = {
            "success": all_ok,
            "sessionId": session.session_id,
            "profile": effective_profile,
            "steps": results,
        }

        if script.output.file:
            Path(script.output.file).write_text(
                json.dumps(output, indent=2, ensure_ascii=False),
                encoding="utf-8",
            )

        text = json.dumps(output, indent=2, ensure_ascii=False)

        if output_file:
            output_file.write_text(text, encoding="utf-8")
            typer.echo(f"Result saved to {output_file}")
        else:
            typer.echo(text)

        raise typer.Exit(code=0 if all_ok else 1)

    finally:
        session.stop(preserve_session=effective_preserve)


@app.command("run")
def run(
    url: Annotated[list[str], typer.Option("--url")],
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
    profile: Annotated[str | None, typer.Option("--profile")] = None,
    profile_dir: Annotated[
        Path | None, typer.Option("--profile-dir", exists=False, file_okay=False)
    ] = None,
    user_agent: Annotated[str | None, typer.Option("--user-agent")] = None,
    headless: Annotated[bool | None, typer.Option("--headless/--headful")] = None,
    timeout: Annotated[int | None, typer.Option("--timeout")] = None,
    proxy: Annotated[
        str | None,
        typer.Option("--proxy", help="Proxy URL: socks5://user:pass@host:port"),
    ] = None,
    viewport_str: Annotated[
        str | None,
        typer.Option("--viewport", help="Viewport size: WIDTHxHEIGHT (e.g. 1920x1080)"),
    ] = None,
    output_file: Annotated[
        Path | None, typer.Option("--output", help="Save result to file")
    ] = None,
    content_format: Annotated[
        str, typer.Option("--format", help="Content format: html or text")
    ] = "html",
) -> None:
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


@app.command("serve")
def serve(
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
    host: Annotated[str, typer.Option("--host")] = "127.0.0.1",
    port: Annotated[int, typer.Option("--port")] = 8080,
) -> None:
    import uvicorn

    from autobrowser_client.api.app import create_app

    settings = _settings_from_options(config=config)
    api_app = create_app(settings)
    uvicorn.run(api_app, host=host, port=port)


@app.command("mcp")
def mcp(
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
    profile: Annotated[
        str | None, typer.Option("--profile", help="Persistent profile name")
    ] = None,
    headless: Annotated[bool | None, typer.Option("--headless/--headed")] = None,
    proxy: Annotated[
        str | None, typer.Option("--proxy", help="e.g. socks5://user:pass@host:port")
    ] = None,
) -> None:
    """Run the MCP server over stdio."""
    from autobrowser_client.mcp_server import run_server

    settings = _settings_from_options(config=config, headless=headless, proxy=proxy)
    run_server(settings, profile)


@app.command("install")
def install(
    config: Annotated[
        Path | None, typer.Option("--config", exists=False, dir_okay=False)
    ] = None,
) -> None:
    """Download the browser for the configured engine."""
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
