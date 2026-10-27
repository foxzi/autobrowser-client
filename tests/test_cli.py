"""Integration tests for CLI commands (no browser required)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path

import pytest
import yaml


def run_cli(*args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, "-m", "autobrowser_client.main"] + list(args)
    return subprocess.run(cmd, capture_output=True, text=True, timeout=10, env=env)


class TestVersion:
    def test_version_output(self):
        r = run_cli("version")
        assert r.returncode == 0
        assert r.stdout.strip() == "0.1.0"


class TestShowConfig:
    def test_show_config_json(self):
        r = run_cli("show-config")
        assert r.returncode == 0
        data = json.loads(r.stdout)
        assert "browser" in data
        assert "api" in data
        assert "proxy" in data


class TestValidateScript:
    def test_valid_script(self, tmp_path: Path):
        script = {"steps": [{"action": "open", "url": "https://example.com"}]}
        path = tmp_path / "test.yaml"
        path.write_text(yaml.dump(script))
        r = run_cli("validate-script", "--file", str(path))
        assert r.returncode == 0
        data = json.loads(r.stdout)
        assert data["success"] is True
        assert data["steps"] == 1

    def test_invalid_script_missing_url(self, tmp_path: Path):
        script = {"steps": [{"action": "open"}]}
        path = tmp_path / "bad.yaml"
        path.write_text(yaml.dump(script))
        r = run_cli("validate-script", "--file", str(path))
        assert r.returncode == 1
        error = json.loads(r.stderr)["error"]
        assert error["code"] == "SCRIPT_VALIDATION_ERROR"
        assert "requires 'url'" in error["message"]
        assert "Traceback" not in r.stderr

    def test_missing_script_file(self):
        r = run_cli("validate-script", "--file", "/nonexistent/script.yaml")
        assert r.returncode != 0

    def test_invalid_action(self, tmp_path: Path):
        script = {"steps": [{"action": "teleport"}]}
        path = tmp_path / "bad2.yaml"
        path.write_text(yaml.dump(script))
        r = run_cli("validate-script", "--file", str(path))
        assert r.returncode != 0


class TestRunScriptOptions:
    def test_invalid_var(self, tmp_path: Path):
        path = tmp_path / "s.yaml"
        path.write_text("steps: [{open: 'https://example.com'}]")
        r = run_cli("run-script", "--file", str(path), "--var", "novalue")
        assert r.returncode != 0
        assert "NAME=VALUE" in r.stderr + r.stdout

    def test_artifacts_outside_cwd_rejected(self, tmp_path: Path):
        path = tmp_path / "s.yaml"
        path.write_text("steps: [{open: 'https://example.com'}]")
        r = run_cli("run-script", "--file", str(path), "--artifacts", "/tmp/art")
        assert r.returncode != 0
