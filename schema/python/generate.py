#!/usr/bin/env python3
"""Generate NAVI's Python contracts.

This is the single Python codegen entrypoint. It emits two complementary outputs:

  1. navi_schema/models.py   — Pydantic V2 models from the canonical JSON Schema
                               definitions (schema/jsonschema/), via datamodel-codegen.
  2. navi_schema/governed.py — stdlib-only dataclass/enum contracts for the *governed*
                               types (CommandType, ValidationOutcome, ExecutionOutcome,
                               …), sourced directly from the canonical Go types by the
                               Go generator at schema/python/gen. datamodel-codegen
                               cannot source Go, so the governed contracts are produced
                               by that companion step — not a parallel generator, the
                               Go-sourcing half of this one flow (Language-Layer
                               Contract §7).

Usage:
    pip install -r requirements.txt
    python generate.py
"""

import shutil
import subprocess
import sys
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent.parent / "jsonschema"
OUTPUT_DIR = Path(__file__).resolve().parent / "navi_schema"
OUTPUT_FILE = OUTPUT_DIR / "models.py"
GOVERNED_FILE = OUTPUT_DIR / "governed.py"
REPO_ROOT = Path(__file__).resolve().parents[2]

# The entry-point schemas (order matters: enums first, then dependents).
SCHEMA_FILES = [
    "enums.json",
    "task.json",
    "claim.json",
    "agent.json",
    "event.json",
]


def generate_governed() -> None:
    """Emit governed.py from the canonical Go types via the Go generator."""
    go = shutil.which("go")
    if go is None:
        print(
            "WARNING: `go` not found on PATH; skipping governed contracts "
            "(governed.py). Install Go and re-run to regenerate them.",
            file=sys.stderr,
        )
        return
    cmd = [go, "run", "./schema/python/gen", "-root", ".", "-out", str(GOVERNED_FILE)]
    print(f"Running: {' '.join(cmd)} (cwd={REPO_ROOT})")
    result = subprocess.run(cmd, cwd=str(REPO_ROOT))
    if result.returncode != 0:
        sys.exit(result.returncode)
    print(f"Generated {GOVERNED_FILE}")


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    # Governed contracts (Go → stdlib Python) come first; they have no third-party deps.
    generate_governed()

    # Build a merged schema with all definitions in one pass.
    # datamodel-codegen can handle $ref across files when given an input dir.
    cmd = [
        sys.executable, "-m", "datamodel_code_generator",
        "--input", str(SCHEMA_DIR),
        "--input-file-type", "jsonschema",
        "--output", str(OUTPUT_FILE),
        "--output-model-type", "pydantic_v2.BaseModel",
        "--target-python-version", "3.11",
        "--use-annotated",
        "--field-constraints",
        "--collapse-root-models",
        "--use-default",
        "--strict-nullable",
        "--enum-field-as-literal", "all",
    ]
    print(f"Running: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print("STDERR:", result.stderr, file=sys.stderr)
        sys.exit(result.returncode)

    print(f"Generated {OUTPUT_FILE}")

    # Write __init__.py with re-exports for both generated modules. The governed
    # contracts are exposed as a submodule (navi_schema.governed) rather than star-
    # imported, so their names never collide with the Pydantic models.
    init_file = OUTPUT_DIR / "__init__.py"
    init_file.write_text(
        '"""navi_schema — generated NAVI contracts.\n'
        "\n"
        "models   : Pydantic V2 models from JSON Schema (schema/jsonschema/).\n"
        "governed : stdlib-only dataclass/enum governed contracts (Go → Python).\n"
        '"""\n'
        "from .models import *  # noqa: F401,F403\n"
        "from . import governed  # noqa: F401\n"
    )
    print(f"Wrote {init_file}")


if __name__ == "__main__":
    main()
