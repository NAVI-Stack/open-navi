#!/usr/bin/env python3
"""Scoped file mutation entrypoint for NAVI Programmer."""

from __future__ import annotations

import datetime as _dt
import difflib
import hashlib
import json
import os
from pathlib import Path
import sys
import time
from typing import Any


SKILL_ID = "navi-programmer.file-mutate"
DEFAULT_MAX_CONTENT_BYTES = 1024 * 1024
MAX_DIFF_LINES = 400
TEXT_SAMPLE_BYTES = 4096
DENIED_NAMES = {
    ".git",
    ".hg",
    ".svn",
    ".navi",
    ".venv",
    "venv",
    "node_modules",
    "__pycache__",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    "dist",
    "build",
    "coverage",
}


class SkillError(Exception):
    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


def main() -> int:
    started = time.perf_counter()
    interface = ""
    try:
        payload = json.load(sys.stdin)
        if not isinstance(payload, dict):
            raise SkillError("invalid_input", "expected a JSON object payload")
        interface = str(payload.get("interface") or "")
        args = payload.get("arguments") or {}
        if not interface:
            raise SkillError("invalid_input", "missing interface")
        if not isinstance(args, dict):
            raise SkillError("invalid_input", "arguments must be an object")
        handlers = {
            "create_file": create_file,
            "write_file": write_file,
        }
        handler = handlers.get(interface)
        if handler is None:
            raise SkillError("unknown_interface", f"unsupported interface: {interface}")
        output = handler(args)
        emit("success", started, interface, output=output)
        return 0
    except SkillError as exc:
        emit("error", started, interface, error={"type": exc.code, "message": exc.message})
        return 0
    except json.JSONDecodeError as exc:
        emit("error", started, interface, error={"type": "invalid_json", "message": str(exc)})
        return 0
    except Exception as exc:
        emit("error", started, interface, error={"type": exc.__class__.__name__, "message": str(exc)})
        return 0


def emit(
    status: str,
    started: float,
    interface: str,
    *,
    output: Any | None = None,
    error: dict[str, str] | None = None,
) -> None:
    result: dict[str, Any] = {
        "status": status,
        "duration_ms": int((time.perf_counter() - started) * 1000),
        "metadata": {
            "skill_id": SKILL_ID,
            "interface": interface,
            "invocation_id": os.environ.get("NAVI_SKILL_INVOCATION_ID", ""),
            "python_version": ".".join(str(part) for part in sys.version_info[:3]),
        },
    }
    if output is not None:
        result["output"] = output
    if error is not None:
        result["error"] = error
    print(json.dumps(result, ensure_ascii=True, separators=(",", ":")))


def create_file(args: dict[str, Any]) -> dict[str, Any]:
    return mutate_file(args, mode="create")


def write_file(args: dict[str, Any]) -> dict[str, Any]:
    return mutate_file(args, mode="write")


def mutate_file(args: dict[str, Any], mode: str) -> dict[str, Any]:
    root = resolve_root(args)
    path_value = args.get("path")
    if not path_value:
        raise SkillError("invalid_input", "path is required")
    target = resolve_scoped_path(root, path_value)
    allow_hidden = bool(args.get("allow_hidden", False))
    assert_visible_path(root, target, allow_hidden)
    encoding = str(args.get("encoding") or "utf-8")
    max_content_bytes = clamp_int(args.get("max_content_bytes"), DEFAULT_MAX_CONTENT_BYTES, 1, DEFAULT_MAX_CONTENT_BYTES)
    new_content = require_text(args.get("content"), "content")
    new_bytes = encode_text(new_content, encoding)
    if len(new_bytes) > max_content_bytes:
        raise SkillError("content_too_large", f"content exceeds {max_content_bytes} bytes")
    if b"\x00" in new_bytes:
        raise SkillError("binary_file", "content contains null bytes")

    parent = target.parent
    parent_exists = parent.exists()
    if not parent_exists and not bool(args.get("create_parent_dirs", False)):
        raise SkillError("parent_not_found", f"parent directory does not exist: {relative_path(root, parent)}")
    if parent_exists and not parent.is_dir():
        raise SkillError("parent_not_directory", f"parent is not a directory: {relative_path(root, parent)}")

    exists = target.exists()
    if exists and not target.is_file():
        raise SkillError("not_file", f"path is not a file: {relative_path(root, target)}")

    before_bytes = b""
    before_content = ""
    if exists:
        before_bytes = target.read_bytes()
        if is_binary(before_bytes[: min(len(before_bytes), TEXT_SAMPLE_BYTES)]):
            raise SkillError("binary_file", f"refusing to mutate binary file: {relative_path(root, target)}")
        before_content = decode_text(before_bytes, encoding)

    expected_sha = str(args.get("expected_sha256") or "")
    has_content_precondition = "expected_content" in args
    if mode == "create" and exists and not bool(args.get("allow_overwrite", False)):
        raise SkillError("file_exists", f"file already exists: {relative_path(root, target)}")
    if mode == "create" and exists and not expected_sha:
        raise SkillError("precondition_required", "overwriting through create_file requires expected_sha256")
    if mode == "write" and not exists and not bool(args.get("create_if_missing", False)):
        raise SkillError("file_not_found", f"file does not exist: {relative_path(root, target)}")
    if mode == "write" and exists and not expected_sha and not has_content_precondition:
        raise SkillError("precondition_required", "write_file on an existing file requires expected_sha256 or expected_content")
    if expected_sha and sha256_hex(before_bytes) != expected_sha:
        raise SkillError("hash_mismatch", f"expected_sha256 does not match: {relative_path(root, target)}")
    if has_content_precondition and args.get("expected_content") != before_content:
        raise SkillError("content_mismatch", f"expected_content does not match: {relative_path(root, target)}")

    changed = before_bytes != new_bytes
    action = "create" if not exists else "replace"
    dry_run = bool(args.get("dry_run", False))
    diff = build_diff(relative_path(root, target), before_content, new_content)
    changed_file = {
        "path": relative_path(root, target),
        "action": action if changed else "noop",
        "before_sha256": sha256_hex(before_bytes) if exists else "",
        "after_sha256": sha256_hex(new_bytes),
        "bytes_before": len(before_bytes),
        "bytes_after": len(new_bytes),
        "dry_run": dry_run,
    }

    if changed and not dry_run:
        try:
            if not parent.exists():
                parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(new_bytes)
        except OSError as exc:
            raise SkillError("write_failed", str(exc))

    return {
        "outcome": "clean",
        "root": str(root),
        "changed": changed,
        "dry_run": dry_run,
        "changed_files": [changed_file] if changed else [],
        "diff_summary": diff,
        "failure_class": "",
        "created_at": utc_now(),
    }


def resolve_root(args: dict[str, Any]) -> Path:
    raw = args.get("root") or args.get("repo_root") or os.environ.get("NAVI_WORKSPACE_DIR") or os.getcwd()
    root = Path(str(raw)).expanduser().resolve()
    if not root.exists():
        raise SkillError("root_not_found", f"root does not exist: {root}")
    if not root.is_dir():
        raise SkillError("root_not_directory", f"root is not a directory: {root}")
    return root


def resolve_scoped_path(root: Path, path_value: Any) -> Path:
    raw = str(path_value or "")
    candidate = Path(raw).expanduser()
    if not candidate.is_absolute():
        candidate = root / candidate
    resolved = candidate.resolve()
    if not is_relative_to(resolved, root):
        raise SkillError("path_out_of_scope", f"path escapes root: {raw}")
    return resolved


def assert_visible_path(root: Path, target: Path, allow_hidden: bool) -> None:
    rel = relative_path(root, target)
    for part in Path(rel).parts:
        if part in DENIED_NAMES:
            raise SkillError("path_denied", f"path is denied: {rel}")
        if part.startswith(".") and not allow_hidden:
            raise SkillError("path_denied", f"hidden path requires allow_hidden: {rel}")


def require_text(value: Any, name: str) -> str:
    if not isinstance(value, str):
        raise SkillError("invalid_input", f"{name} must be a string")
    return value


def encode_text(value: str, encoding: str) -> bytes:
    try:
        return value.encode(encoding)
    except LookupError:
        raise SkillError("invalid_encoding", f"unknown encoding: {encoding}")
    except UnicodeEncodeError as exc:
        raise SkillError("encode_error", str(exc))


def decode_text(value: bytes, encoding: str) -> str:
    try:
        return value.decode(encoding)
    except LookupError:
        raise SkillError("invalid_encoding", f"unknown encoding: {encoding}")
    except UnicodeDecodeError as exc:
        raise SkillError("decode_error", str(exc))


def build_diff(path: str, before: str, after: str) -> dict[str, Any]:
    lines = list(
        difflib.unified_diff(
            before.splitlines(),
            after.splitlines(),
            fromfile=f"a/{path}",
            tofile=f"b/{path}",
            lineterm="",
        )
    )
    additions = sum(1 for line in lines if line.startswith("+") and not line.startswith("+++"))
    deletions = sum(1 for line in lines if line.startswith("-") and not line.startswith("---"))
    truncated = len(lines) > MAX_DIFF_LINES
    return {
        "additions": additions,
        "deletions": deletions,
        "unified_diff": ("\n".join(lines[:MAX_DIFF_LINES]) + "\n") if lines else "",
        "truncated": truncated,
    }


def is_binary(sample: bytes) -> bool:
    if b"\x00" in sample:
        return True
    if not sample:
        return False
    control = sum(1 for byte in sample if byte < 32 and byte not in (9, 10, 13))
    return control / len(sample) > 0.30


def sha256_hex(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def clamp_int(value: Any, default: int, minimum: int, maximum: int) -> int:
    if value is None:
        parsed = default
    else:
        try:
            parsed = int(value)
        except (TypeError, ValueError):
            raise SkillError("invalid_input", f"expected integer, got {value!r}")
    return max(minimum, min(maximum, parsed))


def relative_path(root: Path, target: Path) -> str:
    try:
        rel = target.resolve().relative_to(root)
    except ValueError:
        return str(target)
    value = rel.as_posix()
    return value if value else "."


def is_relative_to(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def utc_now() -> str:
    return _dt.datetime.now(tz=_dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


if __name__ == "__main__":
    raise SystemExit(main())
