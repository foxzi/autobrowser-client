"""Scenario engine: variables, conditions, control blocks and run_script."""
from __future__ import annotations

import logging
import os
import re
import time
from typing import Any

from pydantic import BaseModel

from autobrowser_client.browser.executor import StepResult, execute_step
from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.scenario import ScriptDefinition, ScriptStep

logger = logging.getLogger(__name__)

_EXACT_RE = re.compile(r"\$\{([^}]+)\}")
_TOKEN_RE = re.compile(r"\$\$\{|\$\{([^}]+)\}")


class UndefinedVariable(Exception):
    pass


def _lookup(path: str, variables: dict[str, Any]) -> Any:
    """Resolve ``name``, ``a.b.0`` or ``env.NAME`` to a value."""
    path = path.strip()
    if path.startswith("env."):
        if path[4:] not in os.environ:
            raise UndefinedVariable(f"undefined variable: {path}")
        return os.environ[path[4:]]
    head, *rest = path.split(".")
    if head not in variables:
        raise UndefinedVariable(f"undefined variable: {path}")
    value = variables[head]
    for part in rest:
        if isinstance(value, dict) and part in value:
            value = value[part]
        elif isinstance(value, list) and part.isdigit() and int(part) < len(value):
            value = value[int(part)]
        else:
            raise UndefinedVariable(f"undefined variable: {path}")
    return value


def substitute(value: Any, variables: dict[str, Any]) -> Any:
    """Replace ``${...}`` in every string of a nested value.

    A string that is exactly one ``${...}`` keeps the type of the variable.
    ``$${`` is a literal ``${``.
    """
    if isinstance(value, str):
        m = _EXACT_RE.fullmatch(value)
        if m:
            return _lookup(m.group(1), variables)

        def repl(match: re.Match[str]) -> str:
            if match.group(1) is None:
                return "${"
            return str(_lookup(match.group(1), variables))

        return _TOKEN_RE.sub(repl, value)
    if isinstance(value, list):
        return [substitute(v, variables) for v in value]
    if isinstance(value, dict):
        return {k: substitute(v, variables) for k, v in value.items()}
    return value


def _substitute_model(model: BaseModel, variables: dict[str, Any]) -> Any:
    data = substitute(model.model_dump(by_alias=True, exclude_unset=True), variables)
    return type(model).model_validate(data)


class _Context:
    def __init__(
        self,
        session: BrowserSession,
        variables: dict[str, Any],
        stop_on_error: bool = True,
        page_id: str | None = None,
    ) -> None:
        self.session = session
        self.vars = variables
        self.stop_on_error = stop_on_error
        self.page_id = page_id
        self.failed = False


def _stored_value(step: ScriptStep, data: Any) -> Any:
    if step.action == "extract":
        return data["value"]
    if step.action == "evaluate":
        return data["result"]
    return data


def _attempt(step: ScriptStep, ctx: _Context) -> StepResult:
    """Substitute variables and run one step once."""
    try:
        concrete = _substitute_model(step, ctx.vars)
    except UndefinedVariable as exc:
        result = StepResult(step.action, success=False)
        result.error = str(exc)
        return result

    if concrete.action == "set":
        t0 = time.monotonic()
        ctx.vars.update(concrete.vars)
        result = StepResult("set", success=True)
        result.data = {"vars": concrete.vars}
        result.duration_ms = (time.monotonic() - t0) * 1000
        return result

    result = execute_step(ctx.session, concrete, ctx.page_id)
    if result.success and concrete.as_name:
        ctx.vars[concrete.as_name] = _stored_value(concrete, result.data)
    return result


def run_script(
    session: BrowserSession,
    script: ScriptDefinition,
    page_id: str | None = None,
    vars: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Run a scenario on an open session; returns success, step results and variables."""
    ctx = _Context(session, {**script.vars, **(vars or {})}, script.stop_on_error, page_id)
    results: list[dict[str, Any]] = []
    for i, step in enumerate(script.steps):
        logger.info("Executing step %d/%d: %s", i + 1, len(script.steps), step.action)
        result = _attempt(step, ctx)
        results.append(result.to_dict())
        if not result.success:
            ctx.failed = True
            stop = ctx.stop_on_error if step.stop_on_error is None else step.stop_on_error
            if stop:
                logger.error("Stopping script at step %d due to error: %s", i + 1, result.error)
                break
    return {"success": not ctx.failed, "steps": results, "vars": ctx.vars}
