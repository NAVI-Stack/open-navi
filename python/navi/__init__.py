"""``navi`` — Python SDK for NAVI's governed kernel surfaces.

Phase 1 exposes exactly **one** governed operation, a read:

    import asyncio
    from navi import query_context

    ctx = asyncio.run(query_context(
        run_id="run_123",
        purpose="eval_scoring",
        scope="current_run_summary",
    ))

The SDK is a thin typed HTTP client over the Go kernel's governed read surface
(``POST /api/context/query``). The kernel decides and mediates; the SDK never
writes, mutates, schedules, invokes, or proposes. There are **no effect methods**
in this package (no ``create``/``update``/``propose``/``schedule``/``invoke``/
``send``/``delete``) — those are Phase 2 and must go through the Go governor.

By construction this package imports no DB/store handle, no privileged connector,
and no Go internals. It speaks only HTTP to the gateway, over loopback by default
(Language-Layer Contract §3, §4, §6).
"""
from __future__ import annotations

from .client import (
    DEFAULT_BASE_URL,
    Client,
    Context,
    Provenance,
    Purpose,
    RunSummary,
    Scope,
    query_context,
)
from .errors import ContextQueryRejected, NaviError, RunNotFound, TransportError

# T1-generated governed enums the read surface returns, re-exported so callers
# can compare against typed values (e.g. ``navi.CommandType.INVOKE``) without
# reaching into the generated module. Generated, never hand-declared.
from ._schema import (
    ApprovalOutcome,
    CommandType,
    ExecutionOutcomeOutcome,
    FailureClass,
)

__all__ = [
    # The single governed operation.
    "query_context",
    "Client",
    # Response types.
    "Context",
    "Provenance",
    "RunSummary",
    # Enumerated purpose/scope (mirror the kernel; the kernel is authoritative).
    "Purpose",
    "Scope",
    # Re-exported generated governed enums.
    "CommandType",
    "ExecutionOutcomeOutcome",
    "FailureClass",
    "ApprovalOutcome",
    # Errors.
    "NaviError",
    "ContextQueryRejected",
    "RunNotFound",
    "TransportError",
    # Config.
    "DEFAULT_BASE_URL",
]
