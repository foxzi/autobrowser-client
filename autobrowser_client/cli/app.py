from __future__ import annotations

import json
from pathlib import Path
from typing import Annotated, Any

import typer

from autobrowser_client import __version__
from autobrowser_client.browser.service import SessionManager
from autobrowser_client.config import Settings, load_settings
from autobrowser_client.errors import AppError, NotImplementedAppError
from autobrowser_client.logging_utils import configure_logging
from autobrowser_client.scenario import load_script


app = typer.Typer(help="CLI for autobrowser-client")


def _settings_from_options(
    config: Path | None,
    headless: bool | None = None,
    profile_dir: Path | None = None,
    user_agent: str | None = None,
    timeout: int | None = None,
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

    settings = load_settings(config_path=config, overrides=overrides or None)
    configure_logging(settings.logging.level, settings.logging.file)
    return settings


@app.command("version")
def version() -> None:
    typer.echo(__version__)


@app.command("show-config")
def show_config(
    config: Annotated[Path | None, typer.Option("--config", exists=False, dir_okay=False)] = None,
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


@app.command("run-script")
def run_script(
    file: Annotated[Path, typer.Option("--file", exists=True, dir_okay=False)],
    config: Annotated[Path | None, typer.Option("--config", exists=False, dir_okay=False)] = None,
    profile: Annotated[str | None, typer.Option("--profile")] = None,
) -> None:
    settings = _settings_from_options(config=config)
    script = load_script(file)
    effective_profile = profile or script.session.profile
    typer.echo(
        json.dumps(
            {
                "success": True,
                "mode": "dry-run",
                "profile": effective_profile,
                "steps": [step.model_dump(by_alias=True, exclude_none=True) for step in script.steps],
                "settings": settings.safe_dump(),
            },
            indent=2,
            ensure_ascii=False,
        )
    )
    raise typer.Exit(code=0)


@app.command("run")
def run(
    url: Annotated[list[str], typer.Option("--url")],
    config: Annotated[Path | None, typer.Option("--config", exists=False, dir_okay=False)] = None,
    profile: Annotated[str | None, typer.Option("--profile")] = None,
    profile_dir: Annotated[Path | None, typer.Option("--profile-dir", exists=False, file_okay=False)] = None,
    user_agent: Annotated[str | None, typer.Option("--user-agent")] = None,
    headless: Annotated[bool | None, typer.Option("--headless/--headful")] = None,
    timeout: Annotated[int | None, typer.Option("--timeout")] = None,
    output_file: Annotated[Path | None, typer.Option("--output", help="Save result to file")] = None,
    content_format: Annotated[str, typer.Option("--format", help="Content format: html or text")] = "html",
) -> None:
    settings = _settings_from_options(
        config=config,
        headless=headless,
        profile_dir=profile_dir,
        user_agent=user_agent,
        timeout=timeout,
    )

    manager = SessionManager(settings)
    session = manager.create_session(
        urls=url,
        profile=profile,
        user_agent=user_agent,
        headless=headless,
    )

    try:
        pages_data: dict[str, Any] = {}
        for page_id in session.list_pages():
            pages_data[page_id] = {
                "url": session.get_page(page_id).url,
                "headers": session.get_headers(page_id),
                "content": session.get_content(page_id, fmt=content_format),
            }

        result = {
            "success": True,
            "sessionId": session.session_id,
            "cookies": session.get_cookies(),
            "pages": pages_data,
        }

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
    config: Annotated[Path | None, typer.Option("--config", exists=False, dir_okay=False)] = None,
    host: Annotated[str, typer.Option("--host")] = "127.0.0.1",
    port: Annotated[int, typer.Option("--port")] = 8080,
) -> None:
    import uvicorn

    from autobrowser_client.api.app import create_app

    settings = _settings_from_options(config=config)
    api_app = create_app(settings)
    uvicorn.run(api_app, host=host, port=port)


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
