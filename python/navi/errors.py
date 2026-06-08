"""Error types for the ``navi`` Python SDK.

The SDK is a thin typed client over the kernel's governed read surface
(Language-Layer Contract §3.2: "the API exposes governed operations, not raw
backend access"). These errors map the gateway's HTTP responses onto a small,
honest exception hierarchy. There is deliberately no "retry the effect" or
"bypass governance" surface here — the SDK only reads.
"""
from __future__ import annotations

from typing import Optional


class NaviError(Exception):
    """Base class for every error raised by the ``navi`` SDK."""


class TransportError(NaviError):
    """The request never produced a governed answer.

    Connection refused, timeout, malformed body, or any non-HTTP failure. This
    is distinct from a kernel *rejection*: the kernel did not decline the read,
    it never adjudicated it.
    """


class ContextQueryRejected(NaviError):
    """The kernel refused the read by policy (HTTP 400-class).

    Raised for a missing/unknown ``purpose`` or ``scope`` or a disallowed
    (purpose, scope) pairing. The governance decision lives in Go
    (``internal/contextread``); the SDK only relays it.
    """

    def __init__(self, message: str, *, status: int = 400) -> None:
        super().__init__(message)
        self.status = status


class RunNotFound(NaviError):
    """The requested ``run_id`` does not exist (HTTP 404)."""

    def __init__(self, message: str, *, run_id: Optional[str] = None) -> None:
        super().__init__(message)
        self.run_id = run_id
