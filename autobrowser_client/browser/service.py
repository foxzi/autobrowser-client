from __future__ import annotations

from autobrowser_client.errors import NotImplementedAppError


class BrowserService:
    def run(self) -> None:
        raise NotImplementedAppError("Browser runtime is not implemented yet")
