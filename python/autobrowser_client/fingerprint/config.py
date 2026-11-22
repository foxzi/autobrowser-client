"""Fingerprint configuration: model, generation, persistence."""
from __future__ import annotations

import json
import logging
import random
from pathlib import Path
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field

logger = logging.getLogger(__name__)

FINGERPRINT_FILE = "fingerprint.json"

Platform = Literal["windows", "linux", "android"]

# WebGL renderer/vendor pairs: Windows (ANGLE + Direct3D11)
WEBGL_PROFILES_WINDOWS = [
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
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Direct3D11 vs_5_0 ps_5_0, D3D11)",
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

# WebGL renderer/vendor pairs: Linux (ANGLE + OpenGL / Vulkan, Mesa drivers)
WEBGL_PROFILES_LINUX = [
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 SUPER, OpenGL 4.5)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060, OpenGL 4.5)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce RTX 2060, OpenGL 4.5)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Mesa Intel(R) UHD Graphics 630 (CFL GT2), OpenGL 4.6)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Mesa Intel(R) Iris(R) Xe Graphics (ADL GT2), OpenGL 4.6)",
    },
    {
        "vendor": "Google Inc. (Intel)",
        "renderer": "ANGLE (Intel, Mesa Intel(R) UHD Graphics 620 (WHL GT2), OpenGL 4.6)",
    },
    {
        "vendor": "Google Inc. (AMD)",
        "renderer": "ANGLE (AMD, AMD Radeon RX 580 (radeonsi, polaris10, LLVM 15.0.7, DRM 3.49, 6.1.0), OpenGL 4.6)",
    },
    {
        "vendor": "Google Inc. (AMD)",
        "renderer": "ANGLE (AMD, AMD Radeon RX 5700 XT (radeonsi, navi10, LLVM 15.0.7, DRM 3.49, 6.1.0), OpenGL 4.6)",
    },
    {
        "vendor": "Google Inc. (NVIDIA)",
        "renderer": "ANGLE (NVIDIA, NVIDIA GeForce GTX 1050 Ti, OpenGL 4.5)",
    },
]

# WebGL renderer/vendor pairs: Android (mobile GPUs)
WEBGL_PROFILES_ANDROID = [
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
        "vendor": "Qualcomm",
        "renderer": "Adreno (TM) 620",
    },
    {
        "vendor": "Qualcomm",
        "renderer": "Adreno (TM) 619",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G78",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G77",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G710",
    },
    {
        "vendor": "ARM",
        "renderer": "Mali-G510",
    },
]

_GPU_POOLS: dict[str, list[dict[str, str]]] = {
    "windows": WEBGL_PROFILES_WINDOWS,
    "linux": WEBGL_PROFILES_LINUX,
    "android": WEBGL_PROFILES_ANDROID,
}


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


def generate_fingerprint(
    seed: int | None = None,
    platform: Platform = "windows",
    *,
    mobile: bool = False,
) -> FingerprintConfig:
    """Generate a random fingerprint config.

    Args:
        seed: Fixed seed value. Random if not provided.
        platform: Target platform for GPU profile selection.
        mobile: Deprecated, use platform="android" instead.
    """
    # Backward compat: mobile=True -> android
    if mobile:
        platform = "android"

    if seed is None:
        seed = random.randint(100000, 999999999)

    rng = random.Random(seed)
    pool = _GPU_POOLS.get(platform, WEBGL_PROFILES_WINDOWS)
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


def load_fingerprint(
    profile_dir: Path,
    platform: Platform = "windows",
    *,
    mobile: bool = False,
) -> FingerprintConfig:
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

    fp = generate_fingerprint(platform=platform, mobile=mobile)
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
