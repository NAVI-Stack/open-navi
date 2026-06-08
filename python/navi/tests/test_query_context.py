"""Tests for the ``navi`` Python SDK governed read surface (T4).

Covered:
  * required-kwarg enforcement — ``purpose``/``scope`` cannot be omitted, and
    nothing may be passed positionally (a ``TypeError``, not a silent default);
  * happy path against a real loopback HTTP stub (the actual transport, not a
    monkeypatch), parsed into the generated-enum-typed view;
  * policy rejection (400) and run-not-found (404) mapping;
  * the package exposes **no** effect method;
  * the package imports nothing forbidden (no store/DB, no connector, no Go
    internals).

Stdlib only: ``unittest`` + ``http.server``. Run from the repo's ``python/``
directory (or anywhere — the test inserts it on ``sys.path``):

    python -m unittest navi.tests.test_query_context
"""
from __future__ import annotations

import asyncio
import inspect
import json
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

# Make ``navi`` importable regardless of where the test is launched from.
_PYTHON_DIR = Path(__file__).resolve().parents[2]
if str(_PYTHON_DIR) not in sys.path:
    sys.path.insert(0, str(_PYTHON_DIR))

import navi  # noqa: E402
from navi import Client, ContextQueryRejected, RunNotFound, query_context  # noqa: E402

# A representative current_run_summary response, shaped exactly like
# contextread.Response (internal/contextread/contextread.go).
_OK_RESPONSE = {
    "run_id": "run_abc",
    "purpose": "eval_scoring",
    "scope": "current_run_summary",
    "context": {
        "run_id": "run_abc",
        "command_type": "invoke",
        "outcome": "succeeded",
        "failure_class": "",
        "retryable": False,
        "started_at": "2026-06-03T10:00:00Z",
        "ended_at": "2026-06-03T10:00:05Z",
        "duration_ms": 5000,
        "skill_ids": ["summarize"],
        "connector_ids": [],
        "llm_provider": "anthropic",
        "llm_model": "claude-opus-4-8",
        "workspace_id": "ws_1",
        "boundary_crossing": False,
        "approval_required": False,
        "approval_outcome": "na",
        "has_failure_reason": False,
        "affected_entity_count": 2,
    },
    "provenance": {
        "source": "navi.kernel.contextread",
        "kernel_mediated": True,
        "run_id": "run_abc",
        "purpose": "eval_scoring",
        "scope": "current_run_summary",
        "redactions": ["affected_entities"],
        "generated_at": "2026-06-03T10:00:06Z",
    },
    "generated_at": "2026-06-03T10:00:06Z",
}


class _StubGateway:
    """A throwaway loopback HTTP server mimicking POST /api/context/query."""

    def __init__(self, status=200, payload=None):
        self.status = status
        self.payload = payload if payload is not None else _OK_RESPONSE
        self.last_request = None
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):  # noqa: N802 - stdlib signature
                length = int(self.headers.get("Content-Length", 0))
                raw = self.rfile.read(length).decode("utf-8") if length else "{}"
                outer.last_request = {
                    "path": self.path,
                    "body": json.loads(raw),
                    "content_type": self.headers.get("Content-Type"),
                }
                body = json.dumps(outer.payload).encode("utf-8")
                self.send_response(outer.status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):  # silence test server logging
                pass

        # Bind to loopback only — exercises the loopback transport path.
        self._server = HTTPServer(("127.0.0.1", 0), Handler)

    @property
    def base_url(self):
        host, port = self._server.server_address
        return "http://%s:%d" % (host, port)

    def __enter__(self):
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)
        self._thread.start()
        return self

    def __exit__(self, *exc):
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=2)


class RequiredKwargTests(unittest.TestCase):
    """`purpose` and `scope` are required — calling without them is a TypeError."""

    def test_missing_purpose_and_scope_raises_type_error(self):
        with self.assertRaises(TypeError):
            asyncio.run(query_context(run_id="run_abc"))  # type: ignore[call-arg]

    def test_missing_scope_raises_type_error(self):
        with self.assertRaises(TypeError):
            asyncio.run(query_context(run_id="run_abc", purpose="eval_scoring"))  # type: ignore[call-arg]

    def test_missing_purpose_raises_type_error(self):
        with self.assertRaises(TypeError):
            asyncio.run(query_context(run_id="run_abc", scope="current_run_summary"))  # type: ignore[call-arg]

    def test_arguments_are_keyword_only(self):
        # No positional form may exist: signature must mark run_id/purpose/scope
        # as KEYWORD_ONLY so a positional call is a static/TypeError, not a
        # silent default.
        sig = inspect.signature(query_context)
        for name in ("run_id", "purpose", "scope"):
            self.assertEqual(
                sig.parameters[name].kind,
                inspect.Parameter.KEYWORD_ONLY,
                "%s must be keyword-only" % name,
            )
        method_sig = inspect.signature(Client.query_context)
        for name in ("run_id", "purpose", "scope"):
            self.assertEqual(
                method_sig.parameters[name].kind,
                inspect.Parameter.KEYWORD_ONLY,
                "Client.query_context %s must be keyword-only" % name,
            )


class HappyPathTests(unittest.TestCase):
    def test_loopback_happy_path_returns_typed_redacted_context(self):
        with _StubGateway() as gw:
            ctx = asyncio.run(
                query_context(
                    run_id="run_abc",
                    purpose="eval_scoring",
                    scope="current_run_summary",
                    base_url=gw.base_url,
                )
            )
            # Request was well-formed and hit the governed endpoint.
            self.assertEqual(gw.last_request["path"], "/api/context/query")
            self.assertEqual(
                gw.last_request["body"],
                {"run_id": "run_abc", "purpose": "eval_scoring", "scope": "current_run_summary"},
            )

        # Envelope parsed.
        self.assertEqual(ctx.run_id, "run_abc")
        self.assertEqual(ctx.purpose, "eval_scoring")
        self.assertEqual(ctx.scope, "current_run_summary")
        # Provenance present and tagged.
        self.assertTrue(ctx.provenance.kernel_mediated)
        self.assertEqual(ctx.provenance.source, "navi.kernel.contextread")
        self.assertIn("affected_entities", ctx.provenance.redactions)

        # Typed view uses the T1-generated enums.
        summary = ctx.run_summary()
        self.assertEqual(summary.command_type, navi.CommandType.INVOKE)
        self.assertEqual(summary.outcome, navi.ExecutionOutcomeOutcome.EXECUTION_OUTCOME_SUCCEEDED)
        # Redaction surfaced as a count, never the raw entities.
        self.assertEqual(summary.affected_entity_count, 2)
        self.assertNotIn("affected_entities", ctx.context)

    def test_enum_purpose_and_scope_are_accepted(self):
        with _StubGateway() as gw:
            asyncio.run(
                query_context(
                    run_id="run_abc",
                    purpose=navi.Purpose.EVAL_SCORING,
                    scope=navi.Scope.CURRENT_RUN_SUMMARY,
                    base_url=gw.base_url,
                )
            )
            # Enum members serialize to their string value on the wire.
            self.assertEqual(gw.last_request["body"]["purpose"], "eval_scoring")
            self.assertEqual(gw.last_request["body"]["scope"], "current_run_summary")


class ErrorMappingTests(unittest.TestCase):
    def test_rejection_maps_to_context_query_rejected(self):
        with _StubGateway(status=400, payload={"error": "contextread: unknown purpose"}) as gw:
            with self.assertRaises(ContextQueryRejected) as caught:
                asyncio.run(
                    query_context(
                        run_id="run_abc",
                        purpose="nope",
                        scope="current_run_summary",
                        base_url=gw.base_url,
                    )
                )
        self.assertIn("unknown purpose", str(caught.exception))
        self.assertEqual(caught.exception.status, 400)

    def test_run_not_found_maps_to_run_not_found(self):
        payload = {"error": {"code": "NOT_FOUND", "message": "run not found", "details": None}}
        with _StubGateway(status=404, payload=payload) as gw:
            with self.assertRaises(RunNotFound) as caught:
                asyncio.run(
                    query_context(
                        run_id="ghost",
                        purpose="eval_scoring",
                        scope="current_run_summary",
                        base_url=gw.base_url,
                    )
                )
        self.assertEqual(caught.exception.run_id, "ghost")
        self.assertIn("run not found", str(caught.exception))


class ReadOnlySurfaceTests(unittest.TestCase):
    """The package must expose no effect method and no forbidden import."""

    EFFECT_NAMES = {
        "create", "update", "delete", "propose", "schedule", "invoke",
        "send", "acquire", "delegate", "compose", "write", "mutate",
        "execute", "resolve_proposal",
    }

    def test_no_effect_methods_on_package(self):
        for name in dir(navi):
            self.assertNotIn(
                name.lower(), self.EFFECT_NAMES,
                "navi package must expose no effect method, found %r" % name,
            )

    def test_public_surface_is_just_query_context(self):
        # The only governed *operation* exported is query_context. Everything
        # else in __all__ is a type, enum, error, or config constant.
        ops = [
            n for n in navi.__all__
            if callable(getattr(navi, n)) and not isinstance(getattr(navi, n), type)
        ]
        self.assertEqual(ops, ["query_context"])

    def test_client_has_no_effect_methods(self):
        public = {n for n in dir(Client) if not n.startswith("_")}
        self.assertEqual(public, {"query_context"})

    def test_no_forbidden_imports(self):
        pkg_dir = Path(navi.__file__).resolve().parent
        forbidden = ("sqlite3", "internal/store", "internal/connectors", "connectors")
        for src in pkg_dir.glob("*.py"):
            text = src.read_text(encoding="utf-8")
            for token in forbidden:
                self.assertNotIn(
                    token, text,
                    "forbidden reference %r found in %s" % (token, src.name),
                )


if __name__ == "__main__":
    unittest.main()
