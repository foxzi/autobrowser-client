"""scenario.schema.json stays in sync with the examples (editor support only)."""

from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml

jsonschema = pytest.importorskip("jsonschema")

ROOT = Path(__file__).resolve().parent.parent
SCHEMA = json.loads((ROOT / "scenario.schema.json").read_text())
EXAMPLES = sorted((ROOT / "examples").glob("*.yaml"))


def test_schema_is_valid():
    jsonschema.Draft202012Validator.check_schema(SCHEMA)


@pytest.mark.parametrize("path", EXAMPLES, ids=lambda p: p.name)
def test_example_matches_schema(path: Path):
    jsonschema.validate(yaml.safe_load(path.read_text()), SCHEMA)


@pytest.mark.parametrize(
    "script",
    [
        {"steps": [{"action": "nope"}]},
        {"steps": [{"action": "open"}]},
        {"steps": [{"try": [{"click": "#a"}]}]},
        {"unknown": 1, "steps": []},
    ],
)
def test_wrong_script_rejected(script: dict):
    with pytest.raises(jsonschema.ValidationError):
        jsonschema.validate(script, SCHEMA)
