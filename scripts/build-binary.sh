#!/usr/bin/env sh
# Build a single-file autobrowser binary with PyInstaller into dist/.
set -eu
cd "$(dirname "$0")/.."
python -m pip install --quiet ".[cloak]" "pyinstaller>=6.11,<7"
python -m PyInstaller --noconfirm --clean --onefile --name autobrowser \
  --collect-all patchright \
  --collect-all cloakbrowser \
  --collect-all geoip2 \
  --collect-all maxminddb \
  --collect-all autobrowser_client \
  --add-data "skills/autobrowser/SKILL.md:autobrowser_client/skill" \
  --collect-submodules uvicorn \
  --collect-submodules mcp \
  autobrowser_client/main.py
