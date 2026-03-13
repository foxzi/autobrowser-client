from __future__ import annotations

from fastapi import FastAPI

from autobrowser_client import __version__
from autobrowser_client.config import Settings


def create_app(settings: Settings) -> FastAPI:
    app = FastAPI(title="autobrowser-client", version=__version__)

    @app.get("/health")
    async def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/ready")
    async def ready() -> dict[str, str]:
        return {"status": "ready"}

    @app.get("/api/v1/info")
    async def info() -> dict[str, object]:
        return {
            "name": "autobrowser-client",
            "version": __version__,
            "api": {
                "host": settings.api.host,
                "port": settings.api.port,
            },
        }

    return app
