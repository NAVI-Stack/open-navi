"""``navi`` Python SDK — governed read surface (Phase 1, reads only).

This module exposes exactly **one** governed operation:

    async def query_context(*, run_id, purpose, scope) -> Context

It is a thin typed client over the kernel's ``POST /api/context/query`` endpoint
(``internal/contextread``). The kernel decides and mediates every read; the SDK
neither writes, mutates, schedules, nor invokes anything. Per the Language-Layer
Contract §3.1/§6, this package holds **no** DB/store handle and **no** privileged
connector — it only speaks HTTP to the gateway, over loopback by default.

``purpose`` and ``scope`` are *required keyword arguments*: there is no way to
call ``query_context`` without them (omitting either is a ``TypeError``, never a
silent default). This mirrors Contract §4 — "reads are governed too": a read must
declare why it needs context and how much.

Transport is stdlib-only (``urllib.request`` run in a worker thread), keeping the
dependency footprint at zero, consistent with ``python/intake/worker.py``.
"""
from __future__ import annotations

import asyncio
import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Dict, List, Optional

from . import _schema
from .errors import ContextQueryRejected, NaviError, RunNotFound, TransportError

# Default gateway base URL. Matches the CLI client (cmd/navi/main.go), and is
# overridable with NAVI_GATEWAY_URL. Loopback by default: the kernel grants
# loopback callers owner-like access, so no auth header is required locally.
DEFAULT_BASE_URL = "http://localhost:6284"
_CONTEXT_QUERY_PATH = "/api/context/query"


class Purpose(str, Enum):
    """Declared reason a caller needs context.

    Mirrors the kernel's enumeration (``internal/contextread`` ``Purpose``). The
    kernel is authoritative: it rejects any purpose it does not recognize. These
    constants exist only for caller ergonomics; ``query_context`` also accepts a
    plain ``str``.
    """

    EVAL_SCORING = "eval_scoring"


class Scope(str, Enum):
    """Bound on what a read may return. Mirrors ``internal/contextread`` ``Scope``."""

    CURRENT_RUN_SUMMARY = "current_run_summary"


@dataclass
class Provenance:
    """Provenance markers stamped on every governed read result.

    Lets consumers (prompts, logs, eval artifacts) trace where context came from
    and what was stripped. Mirrors ``contextread.Provenance``.
    """

    source: str = ""
    kernel_mediated: bool = False
    run_id: str = ""
    purpose: str = ""
    scope: str = ""
    redactions: List[str] = field(default_factory=list)
    generated_at: str = ""

    @classmethod
    def from_dict(cls, raw: Dict[str, Any]) -> "Provenance":
        raw = raw or {}
        return cls(
            source=raw.get("source", ""),
            kernel_mediated=bool(raw.get("kernel_mediated", False)),
            run_id=raw.get("run_id", ""),
            purpose=raw.get("purpose", ""),
            scope=raw.get("scope", ""),
            redactions=list(raw.get("redactions") or []),
            generated_at=raw.get("generated_at", ""),
        )


def _coerce_enum(enum_cls, value):
    """Map a wire string onto a generated enum, tolerating unknown values.

    The kernel is the source of truth for these enums; if it ever emits a value
    this (generated) client doesn't know, we return the raw string rather than
    crashing the read.
    """
    if value in (None, ""):
        return value
    try:
        return enum_cls(value)
    except ValueError:
        return value


@dataclass
class RunSummary:
    """Typed view of a ``current_run_summary`` context block.

    Enum-typed fields use the **T1-generated** governed enums
    (``schema/python/navi_schema/governed.py``) so the SDK never re-declares
    governed vocabulary. Redacted fields (failure reason, affected entities) are
    surfaced only as presence/count, exactly as the kernel returns them.
    """

    run_id: str = ""
    command_type: Any = ""  # _schema.CommandType
    outcome: Any = ""  # _schema.ExecutionOutcomeOutcome
    failure_class: Any = ""  # _schema.FailureClass
    retryable: bool = False
    started_at: str = ""
    ended_at: Optional[str] = None
    duration_ms: Optional[int] = None
    skill_ids: List[str] = field(default_factory=list)
    connector_ids: List[str] = field(default_factory=list)
    llm_provider: str = ""
    llm_model: str = ""
    llm_task_class: str = ""
    llm_complexity: str = ""
    workspace_id: str = ""
    boundary_crossing: bool = False
    approval_required: bool = False
    approval_outcome: Any = ""  # _schema.ApprovalOutcome
    has_failure_reason: bool = False
    affected_entity_count: int = 0

    @classmethod
    def from_context(cls, ctx: Dict[str, Any]) -> "RunSummary":
        ctx = ctx or {}
        return cls(
            run_id=ctx.get("run_id", ""),
            command_type=_coerce_enum(_schema.CommandType, ctx.get("command_type", "")),
            outcome=_coerce_enum(_schema.ExecutionOutcomeOutcome, ctx.get("outcome", "")),
            failure_class=_coerce_enum(_schema.FailureClass, ctx.get("failure_class", "")),
            retryable=bool(ctx.get("retryable", False)),
            started_at=ctx.get("started_at", ""),
            ended_at=ctx.get("ended_at"),
            duration_ms=ctx.get("duration_ms"),
            skill_ids=list(ctx.get("skill_ids") or []),
            connector_ids=list(ctx.get("connector_ids") or []),
            llm_provider=ctx.get("llm_provider", ""),
            llm_model=ctx.get("llm_model", ""),
            llm_task_class=ctx.get("llm_task_class", ""),
            llm_complexity=ctx.get("llm_complexity", ""),
            workspace_id=ctx.get("workspace_id", ""),
            boundary_crossing=bool(ctx.get("boundary_crossing", False)),
            approval_required=bool(ctx.get("approval_required", False)),
            approval_outcome=_coerce_enum(_schema.ApprovalOutcome, ctx.get("approval_outcome", "")),
            has_failure_reason=bool(ctx.get("has_failure_reason", False)),
            affected_entity_count=int(ctx.get("affected_entity_count", 0) or 0),
        )


@dataclass
class Context:
    """Redacted, scope-bounded, provenance-tagged result of a governed read.

    ``context`` is the raw least-context block as the kernel returned it; its
    shape varies by ``scope``. For the ``current_run_summary`` scope, call
    :meth:`run_summary` for a view typed against the generated enums.
    """

    run_id: str = ""
    purpose: str = ""
    scope: str = ""
    context: Dict[str, Any] = field(default_factory=dict)
    provenance: Provenance = field(default_factory=Provenance)
    generated_at: str = ""

    @classmethod
    def from_dict(cls, raw: Dict[str, Any]) -> "Context":
        raw = raw or {}
        return cls(
            run_id=raw.get("run_id", ""),
            purpose=raw.get("purpose", ""),
            scope=raw.get("scope", ""),
            context=dict(raw.get("context") or {}),
            provenance=Provenance.from_dict(raw.get("provenance") or {}),
            generated_at=raw.get("generated_at", ""),
        )

    def run_summary(self) -> RunSummary:
        """Parse the context block into a generated-enum-typed :class:`RunSummary`.

        Only meaningful for ``scope == "current_run_summary"``.
        """
        if self.scope != Scope.CURRENT_RUN_SUMMARY.value:
            raise NaviError(
                "run_summary() is only valid for scope=%r, got %r"
                % (Scope.CURRENT_RUN_SUMMARY.value, self.scope)
            )
        return RunSummary.from_context(self.context)


class Client:
    """Thin typed client for the kernel's governed read surface.

    Construct once and reuse. ``base_url`` defaults to ``NAVI_GATEWAY_URL`` or
    :data:`DEFAULT_BASE_URL` (loopback). An ``api_key`` is only needed for a
    non-loopback gateway; loopback callers are granted owner-like access by the
    gateway and need no header.
    """

    def __init__(
        self,
        base_url: Optional[str] = None,
        *,
        api_key: Optional[str] = None,
        timeout: float = 30.0,
    ) -> None:
        self.base_url = (base_url or os.getenv("NAVI_GATEWAY_URL") or DEFAULT_BASE_URL).rstrip("/")
        # Optional: only used off-loopback. Falls back to the connector shared
        # secret env the gateway already understands.
        self.api_key = api_key or os.getenv("NAVI_GATEWAY_SHARED_SECRET")
        self.timeout = timeout

    async def query_context(self, *, run_id: str, purpose: str, scope: str) -> Context:
        """Governed, read-only context fetch.

        ``purpose`` and ``scope`` are required keyword arguments — the only way
        the kernel will adjudicate the read. The kernel enforces purpose/scope
        validity, redaction, least-context, provenance, and audit; this method
        just relays the request and types the answer.

        :raises ContextQueryRejected: the kernel refused by policy (400-class).
        :raises RunNotFound: the run does not exist (404).
        :raises TransportError: the request never reached a governed answer.
        """
        purpose_val = purpose.value if isinstance(purpose, Enum) else purpose
        scope_val = scope.value if isinstance(scope, Enum) else scope
        body = {"run_id": run_id, "purpose": purpose_val, "scope": scope_val}
        raw = await asyncio.to_thread(self._post, _CONTEXT_QUERY_PATH, body)
        return Context.from_dict(raw)

    # --- transport (stdlib only) -------------------------------------------

    def _post(self, path: str, body: Dict[str, Any]) -> Dict[str, Any]:
        url = self.base_url + path
        data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(url, data=data, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Accept", "application/json")
        if self.api_key:
            req.add_header("X-API-Key", self.api_key)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                payload = resp.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            self._raise_for_status(exc, body.get("run_id", ""))
            raise  # unreachable; _raise_for_status always raises
        except urllib.error.URLError as exc:
            raise TransportError("navi: could not reach gateway at %s: %s" % (url, exc.reason)) from exc
        try:
            return json.loads(payload)
        except json.JSONDecodeError as exc:
            raise TransportError("navi: malformed response from gateway: %s" % exc) from exc

    @staticmethod
    def _raise_for_status(exc: "urllib.error.HTTPError", run_id: str) -> None:
        try:
            detail = json.loads(exc.read().decode("utf-8"))
        except Exception:  # noqa: BLE001 - best-effort error parsing
            detail = {}
        message = _extract_error_message(detail) or (exc.reason or "request failed")
        if exc.code == 400:
            raise ContextQueryRejected(message, status=exc.code)
        if exc.code == 404:
            raise RunNotFound(message, run_id=run_id)
        raise TransportError("navi: gateway returned %s: %s" % (exc.code, message))


def _extract_error_message(detail: Any) -> str:
    """Pull a human message out of the gateway's error body.

    The gateway uses two shapes: ``{"error": "msg"}`` (replyError) and
    ``{"error": {"code", "message", "details"}}`` (replyErrorAPI).
    """
    if not isinstance(detail, dict):
        return ""
    err = detail.get("error")
    if isinstance(err, str):
        return err
    if isinstance(err, dict):
        return err.get("message", "") or err.get("code", "")
    return ""


# Module-level convenience: a single governed read without managing a Client.
async def query_context(
    *,
    run_id: str,
    purpose: str,
    scope: str,
    base_url: Optional[str] = None,
    api_key: Optional[str] = None,
) -> Context:
    """Governed, read-only context fetch using a transient :class:`Client`.

    ``purpose`` and ``scope`` are required keyword arguments. See
    :meth:`Client.query_context`.
    """
    client = Client(base_url, api_key=api_key)
    return await client.query_context(run_id=run_id, purpose=purpose, scope=scope)
