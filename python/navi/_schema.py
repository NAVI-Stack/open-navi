"""Loader for the T1-generated governed contracts (Go → Python).

The ``navi`` SDK types its responses against the generated module at
``schema/python/navi_schema/governed.py`` (Language-Layer Contract §7:
governed DTOs are *generated*, not hand-written). We must not re-declare those
enums here, or they would drift from the kernel.

We load ``governed.py`` *by file path* rather than ``import navi_schema`` on
purpose. The ``navi_schema`` package ``__init__`` pulls in ``models.py``, which
depends on Pydantic. ``governed.py`` itself is stdlib-only (dataclasses + enum),
so loading it directly keeps the SDK's dependency footprint at zero — consistent
with ``python/intake/worker.py`` — while still consuming the generated truth.
"""
from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

# python/navi/_schema.py  →  parents[2] is the repo root.
#   parents[0] = python/navi   parents[1] = python   parents[2] = <repo root>
_GENERATED = (
    Path(__file__).resolve().parents[2]
    / "schema"
    / "python"
    / "navi_schema"
    / "governed.py"
)


def _load_governed() -> ModuleType:
    if not _GENERATED.exists():
        raise ImportError(
            "navi SDK: generated governed types not found at %s; "
            "run `make generate-python` first" % _GENERATED
        )
    spec = importlib.util.spec_from_file_location("navi_schema_governed", _GENERATED)
    if spec is None or spec.loader is None:  # pragma: no cover - defensive
        raise ImportError("navi SDK: could not load generated governed types")
    module = importlib.util.module_from_spec(spec)
    # Register before exec: dataclasses resolves field types via
    # sys.modules[cls.__module__], so the module must be discoverable while its
    # own @dataclass decorators run.
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


governed = _load_governed()

# Re-exported generated enums the read surface actually returns. Sourced from the
# kernel, never hand-declared.
CommandType = governed.CommandType
ExecutionOutcomeOutcome = governed.ExecutionOutcomeOutcome
FailureClass = governed.FailureClass
ApprovalOutcome = governed.ApprovalOutcome

__all__ = [
    "governed",
    "CommandType",
    "ExecutionOutcomeOutcome",
    "FailureClass",
    "ApprovalOutcome",
]
