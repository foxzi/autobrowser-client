from __future__ import annotations

import logging
from pathlib import Path


SENSITIVE_KEYS = {"authorization", "cookie", "password", "token", "proxy_password"}


def mask_value(value: str, visible: int = 4) -> str:
    if len(value) <= visible:
        return "*" * len(value)
    return f"{value[:visible]}***"


def mask_mapping(data: dict[str, object]) -> dict[str, object]:
    masked: dict[str, object] = {}
    for key, value in data.items():
        if isinstance(value, dict):
            masked[key] = mask_mapping(value)
            continue
        if key.lower() in SENSITIVE_KEYS and isinstance(value, str):
            masked[key] = mask_value(value)
            continue
        masked[key] = value
    return masked


def configure_logging(level: str = "INFO", file_path: str | None = None) -> None:
    handlers: list[logging.Handler] = [logging.StreamHandler()]
    if file_path:
        path = Path(file_path)
        path.parent.mkdir(parents=True, exist_ok=True)
        handlers.append(logging.FileHandler(path))

    logging.basicConfig(
        level=getattr(logging, level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=handlers,
        force=True,
    )
