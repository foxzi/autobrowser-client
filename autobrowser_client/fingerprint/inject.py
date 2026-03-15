"""Build the fingerprint injection JS with config baked in."""
from __future__ import annotations

import json
from pathlib import Path

from autobrowser_client.fingerprint.config import FingerprintConfig

_JS_PATH = Path(__file__).parent / "inject.js"
_JS_CACHE: str | None = None


def _load_js() -> str:
    global _JS_CACHE
    if _JS_CACHE is None:
        _JS_CACHE = _JS_PATH.read_text(encoding="utf-8")
    return _JS_CACHE


def build_injection_script(fp: FingerprintConfig) -> str:
    """Return a full JS string that sets __fpConfig and runs the spoof."""
    config_json = json.dumps(fp.to_js_config(), ensure_ascii=False)
    js_body = _load_js()
    return f"window.__fpConfig = {config_json};\n{js_body}"
