"""Fingerprint configuration: model, generation, persistence."""
from __future__ import annotations

import json
import logging
import random
from pathlib import Path
from typing import Any

from pydantic import BaseModel, ConfigDict, Field

logger = logging.getLogger(__name__)

FINGERPRINT_FILE = "fingerprint.json"

# Popular WebGL renderer/vendor pairs (desktop)
WEBGL_PROFILES = [
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce GTX 1050 Ti Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 2060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (AMD)",
        "renderer": "ANGLE (AMD, AMD Radeon RX 580 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (AMD)",
        "renderer": "ANGLE (AMD, AMD Radeon RX 5700 XT Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
    },
]

# Popular WebGL renderer/vendor pairs (mobile / Android)
WEBGL_PROFILES_MOBILE = [
    {
        "vendor": "Qualcomm",
        "renderer": "Adreno (TM) 730",
    },
    {
        "vendor": "Qualcomm",
        "renderer": "Adreno (TM) 660",
    },
    {
        "vendor": "Qualcomm",
        "renderer": "Adreno (TM) 640",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G78",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G77",
    },
]


class FingerprintConfig(BaseModel):
    """Per-profile fingerprint spoofing configuration."""

    model_config = ConfigDict(extra="forbid")

    seed: int = Field(description="Seed for deterministic noise generation")
    canvas: bool = Field(default=True, description="Add noise to Canvas API")
    webgl: bool = Field(default=True, description="Spoof WebGL renderer/vendor")
    webgl_vendor: str = Field(alias="webglVendor")
    webgl_renderer: str = Field(alias="webglRenderer")
    audio: bool = Field(default=True, description="Add noise to AudioContext")
    client_rects: bool = Field(default=True, alias="clientRects", description="Add noise to getBoundingClientRect")
    fonts: bool = Field(default=True, description="Add noise to font metrics")

    def to_js_config(self) -> dict[str, Any]:
        """Return config dict suitable for injection into JS."""
        return {
            "seed": self.seed,
            "canvas": self.canvas,
            "webgl": self.webgl,
            "webglVendor": self.webgl_vendor,
            "webglRenderer": self.webgl_renderer,
            "audio": self.audio,
            "clientRects": self.client_rects,
            "fonts": self.fonts,
        }


def generate_fingerprint(seed: int | None = None, mobile: bool = False) -> FingerprintConfig:
    """Generate a random fingerprint config.

    If seed is not provided, a random one is generated.
    """
    if seed is None:
        seed = random.randint(100000, 999999999)

    rng = random.Random(seed)
    pool = WEBGL_PROFILES_MOBILE if mobile else WEBGL_PROFILES
    gpu = rng.choice(pool)

    return FingerprintConfig.model_validate({
        "seed": seed,
        "canvas": True,
        "webgl": True,
        "webglVendor": gpu["vendor"],
        "webglRenderer": gpu["renderer"],
        "audio": True,
        "clientRects": True,
        "fonts": True,
    })


def load_fingerprint(profile_dir: Path, mobile: bool = False) -> FingerprintConfig:
    """Load fingerprint from profile dir, or generate and save if missing."""
    fp_path = profile_dir / FINGERPRINT_FILE
    if fp_path.exists():
        try:
            data = json.loads(fp_path.read_text(encoding="utf-8"))
            fp = FingerprintConfig.model_validate(data)
            logger.info("Loaded fingerprint from %s (seed=%d)", fp_path, fp.seed)
            return fp
        except Exception as exc:
            logger.warning("Failed to load %s, regenerating: %s", fp_path, exc)

    fp = generate_fingerprint(mobile=mobile)
    save_fingerprint(profile_dir, fp)
    return fp


def save_fingerprint(profile_dir: Path, fp: FingerprintConfig) -> None:
    """Save fingerprint config to profile directory."""
    fp_path = profile_dir / FINGERPRINT_FILE
    fp_path.write_text(
        json.dumps(fp.to_js_config(), indent=2, ensure_ascii=False),
        encoding="utf-8",
    )
    logger.info("Saved fingerprint to %s (seed=%d)", fp_path, fp.seed)


def merge_fingerprint(
    base: FingerprintConfig,
    overrides: dict[str, Any],
) -> FingerprintConfig:
    """Merge scenario overrides into a base fingerprint config."""
    data = base.to_js_config()
    data.update(overrides)
    return FingerprintConfig.model_validate(data)
