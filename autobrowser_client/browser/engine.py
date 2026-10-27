"""Scenario engine: variables, conditions, control blocks and run_script."""
from __future__ import annotations

import json
import logging
import os
import re
import time
from typing import Any

from pathlib import Path

from pydantic import BaseModel

from autobrowser_client.browser.actions import resolve_locator
from autobrowser_client.browser.executor import StepResult, _validate_output_path, execute_step
from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.scenario import (
    Condition,
    ForEachBlock,
    IfBlock,
    RepeatBlock,
    ScriptDefinition,
    ScriptStep,
    Step,
    TryBlock,
    WhileBlock,
)

logger = logging.getLogger(__name__)

_EXACT_RE = re.compile(r"\$\{([^}]+)\}")
_TOKEN_RE = re.compile(r"\$\$\{|\$\{([^}]+)\}")


class UndefinedVariable(Exception):
    pass


class StepFailure(Exception):
    """An unhandled step failure; propagates up to the nearest ``try`` block."""

    def __init__(self, message: str, action: str, name: str | None, node: dict[str, Any]) -> None:
        super().__init__(message)
        self.message = message
        self.action = action
        self.name = name
        self.nodes = [node]  # result nodes the failure passed through, marked handled by ``try``


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


def _sleep(ms: float) -> None:
    time.sleep(ms / 1000)


def _check_var(cond: Condition, variables: dict[str, Any]) -> bool:
    try:
        value = _lookup(cond.var, variables)
    except UndefinedVariable:
        return False
    checks = []
    if "equals" in cond.model_fields_set:
        checks.append(value == cond.equals)
    if cond.contains is not None:
        if isinstance(value, (list, dict)):
            checks.append(cond.contains in value)
        else:
            checks.append(cond.contains in str(value))
    if cond.matches is not None:
        checks.append(re.search(cond.matches, str(value)) is not None)
    return all(checks) if checks else bool(value)


def _check_leaf(session: BrowserSession, cond: Condition, variables: dict[str, Any], page_id: str | None) -> bool:
    if cond.var is not None:
        return _check_var(cond, variables)
    try:
        page = session.get_page(page_id)
        if cond.exists is not None:
            return resolve_locator(page, cond.exists.type, cond.exists.value).count() > 0
        if cond.visible is not None:
            return resolve_locator(page, cond.visible.type, cond.visible.value).first.is_visible()
        if cond.text is not None:
            return cond.text in page.inner_text("body")
        if cond.url is not None:
            return cond.url in page.url
        if cond.url_matches is not None:
            return re.search(cond.url_matches, page.url) is not None
        return cond.title in page.title()
    except Exception as exc:  # not found, page navigating, bad regex: the check is false
        logger.debug("condition check failed: %s", exc)
        return False


def check_condition(
    session: BrowserSession, cond: Condition, variables: dict[str, Any], page_id: str | None = None
) -> bool:
    """Evaluate a condition (variables already substituted). Never raises for "not found"."""
    if cond.not_ is not None:
        return not check_condition(session, cond.not_, variables, page_id)
    if cond.all_ is not None:
        return all(check_condition(session, c, variables, page_id) for c in cond.all_)
    if cond.any_ is not None:
        return any(check_condition(session, c, variables, page_id) for c in cond.any_)
    deadline = time.monotonic() + cond.timeout / 1000
    while not _check_leaf(session, cond, variables, page_id):
        if time.monotonic() >= deadline:
            return False
        _sleep(250)
    return True


class _Context:
    def __init__(
        self,
        session: BrowserSession,
        variables: dict[str, Any],
        stop_on_error: bool = True,
        page_id: str | None = None,
        artifacts_dir: str | None = None,
    ) -> None:
        self.session = session
        self.vars = variables
        self.stop_on_error = stop_on_error
        self.page_id = page_id
        self.failed = False
        self.artifacts_dir = artifacts_dir
        self.artifact_count = 0

    def save_artifacts(self, action: str) -> dict[str, str]:
        """Save a screenshot and the HTML of the page; errors are logged, never raised."""
        self.artifact_count += 1
        base = Path(self.artifacts_dir) / f"{self.artifact_count:03d}-{action}"
        saved: dict[str, str] = {}
        try:
            page = self.session.get_page(self.page_id)
            shot = _validate_output_path(f"{base}.png")
            shot.parent.mkdir(parents=True, exist_ok=True)
            page.screenshot(path=str(shot))
            saved["screenshot"] = str(shot)
            html = _validate_output_path(f"{base}.html")
            html.write_text(page.content(), encoding="utf-8")
            saved["html"] = str(html)
        except Exception as exc:
            logger.warning("Saving error artifacts failed: %s", exc)
        return saved


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

    if concrete.action == "fail":
        result = StepResult("fail", success=False)
        result.error = concrete.message
        return result

    if concrete.action == "assert":
        t0 = time.monotonic()
        ok = check_condition(ctx.session, concrete.condition, ctx.vars, ctx.page_id)
        result = StepResult("assert", success=ok)
        if ok:
            result.data = {"condition": True}
        else:
            what = json.dumps(concrete.condition.model_dump(by_alias=True, exclude_unset=True), ensure_ascii=False)
            result.error = concrete.message or f"assertion failed: {what}"
        result.duration_ms = (time.monotonic() - t0) * 1000
        return result

    result = execute_step(ctx.session, concrete, ctx.page_id)
    if result.success and concrete.as_name:
        ctx.vars[concrete.as_name] = _stored_value(concrete, result.data)
    return result


def _run_step(step: ScriptStep, ctx: _Context, out: list[dict[str, Any]]) -> None:
    """Run a step with retries; record its result in ``out`` and raise StepFailure when it stops the script."""
    attempts = 0
    while True:
        attempts += 1
        result = _attempt(step, ctx)
        if result.success or attempts > step.retry:
            break
        logger.info("Step '%s' failed (attempt %d): %s", step.action, attempts, result.error)
        _sleep(step.retry_delay)

    node = result.to_dict()
    if step.name:
        node["name"] = step.name
    if step.retry:
        node["attempts"] = attempts
    out.append(node)
    if result.success:
        return
    if step.optional:
        node["success"] = True
        node["optional"] = True
        return
    if ctx.artifacts_dir:
        artifacts = ctx.save_artifacts(step.action)
        if artifacts:
            node["artifacts"] = artifacts
    stop = ctx.stop_on_error if step.stop_on_error is None else step.stop_on_error
    if stop:
        raise StepFailure(result.error or "", step.action, step.name, node)
    ctx.failed = True


def _block_fail(node: dict[str, Any], kind: str, name: str | None, message: str) -> StepFailure:
    node["error"] = message
    return StepFailure(message, kind, name, node)


def _substitute_block(
    value: Any, ctx: _Context, node: dict[str, Any], kind: str, name: str | None
) -> Any:
    try:
        if isinstance(value, BaseModel):
            return _substitute_model(value, ctx.vars)
        return substitute(value, ctx.vars)
    except UndefinedVariable as exc:
        raise _block_fail(node, kind, name, str(exc)) from exc


def _run_if(block: IfBlock, ctx: _Context, node: dict[str, Any]) -> None:
    cond = _substitute_block(block.if_, ctx, node, "if", block.name)
    matched = check_condition(ctx.session, cond, ctx.vars, ctx.page_id)
    branch = block.then if matched else block.else_
    node["condition"] = matched
    node["branch"] = "then" if matched else ("else" if block.else_ is not None else None)
    node["steps"] = []
    _run_steps(branch or [], ctx, node["steps"])


def _run_try(block: TryBlock, ctx: _Context, node: dict[str, Any]) -> None:
    node["steps"] = []
    pending: StepFailure | None = None
    try:
        _run_steps(block.try_, ctx, node["steps"])
    except StepFailure as exc:
        pending = exc
        if block.catch is not None:
            for failed in exc.nodes:
                failed["handled"] = True
            ctx.vars["error"] = {"message": exc.message, "action": exc.action, "name": exc.name}
            node["catch"] = []
            pending = None
            try:
                _run_steps(block.catch, ctx, node["catch"])
            except StepFailure as catch_exc:
                pending = catch_exc
    if block.finally_ is not None:
        node["finally"] = []
        _run_steps(block.finally_, ctx, node["finally"])
    if pending is not None:
        raise pending


def _run_iteration(steps: list[Step], ctx: _Context, node: dict[str, Any], index: int) -> None:
    ctx.vars["index"] = index
    iteration: list[dict[str, Any]] = []
    node["iterations"].append(iteration)
    _run_steps(steps, ctx, iteration)


def _run_for_each(block: ForEachBlock, ctx: _Context, node: dict[str, Any]) -> None:
    items = _substitute_block(block.for_each, ctx, node, "forEach", block.name)
    if not isinstance(items, list):
        raise _block_fail(node, "forEach", block.name, f"forEach expects a list, got {type(items).__name__}")
    node["iterations"] = []
    for i, item in enumerate(items):
        ctx.vars[block.as_name] = item
        _run_iteration(block.do, ctx, node, i)


def _run_repeat(block: RepeatBlock, ctx: _Context, node: dict[str, Any]) -> None:
    raw = _substitute_block(block.repeat, ctx, node, "repeat", block.name)
    try:
        count = int(raw)
    except (TypeError, ValueError):
        count = -1
    if count < 0:
        raise _block_fail(node, "repeat", block.name, f"repeat expects a non-negative integer, got {raw!r}")
    node["iterations"] = []
    for i in range(count):
        _run_iteration(block.do, ctx, node, i)


def _run_while(block: WhileBlock, ctx: _Context, node: dict[str, Any]) -> None:
    node["iterations"] = []
    while True:
        cond = _substitute_block(block.while_, ctx, node, "while", block.name)
        if not check_condition(ctx.session, cond, ctx.vars, ctx.page_id):
            return
        if len(node["iterations"]) >= block.max:
            raise _block_fail(node, "while", block.name, f"while exceeded max {block.max} iterations")
        _run_iteration(block.do, ctx, node, len(node["iterations"]))


_BLOCK_RUNNERS = {
    IfBlock: ("if", _run_if),
    TryBlock: ("try", _run_try),
    ForEachBlock: ("forEach", _run_for_each),
    RepeatBlock: ("repeat", _run_repeat),
    WhileBlock: ("while", _run_while),
}


def _children_ok(node: dict[str, Any]) -> bool:
    """A block succeeds when all its results succeeded or were handled by a ``try``."""
    groups = [node.get("steps", []), node.get("catch", []), node.get("finally", [])]
    for iteration in node.get("iterations", []):
        groups.append(iteration)
    return all(r["success"] or r.get("handled") for group in groups for r in group)


def _run_block(block: Any, ctx: _Context, out: list[dict[str, Any]]) -> None:
    kind, runner = _BLOCK_RUNNERS[type(block)]
    node: dict[str, Any] = {"action": kind}
    if block.name:
        node["name"] = block.name
    node["success"] = True
    out.append(node)
    try:
        runner(block, ctx, node)
    except StepFailure as exc:
        node["success"] = False
        if all(n is not node for n in exc.nodes):
            exc.nodes.append(node)
        raise
    node["success"] = _children_ok(node)


def _run_steps(steps: list[Step], ctx: _Context, out: list[dict[str, Any]]) -> None:
    for step in steps:
        if isinstance(step, ScriptStep):
            logger.info("Executing step: %s", step.action)
            _run_step(step, ctx, out)
        else:
            _run_block(step, ctx, out)


def run_script(
    session: BrowserSession,
    script: ScriptDefinition,
    page_id: str | None = None,
    vars: dict[str, Any] | None = None,
    artifacts_dir: str | None = None,
) -> dict[str, Any]:
    """Run a scenario on an open session; returns success, step results and variables."""
    artifacts_dir = artifacts_dir or script.error_artifacts
    if artifacts_dir:
        _validate_output_path(artifacts_dir)
    ctx = _Context(session, {**script.vars, **(vars or {})}, script.stop_on_error, page_id, artifacts_dir)
    results: list[dict[str, Any]] = []
    try:
        _run_steps(script.steps, ctx, results)
    except StepFailure as exc:
        logger.error("Stopping script: %s", exc.message)
        ctx.failed = True
    return {"success": not ctx.failed, "steps": results, "vars": ctx.vars}
