#!/usr/bin/env python3
"""Structured patch preview and apply entrypoint for NAVI Programmer."""

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


SKILL_ID = "navi-programmer.patch-apply"
DEFAULT_MAX_FILE_BYTES = 1024 * 1024
MAX_DIFF_LINES = 800
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
            "preview_patch": preview_patch,
            "apply_patch": apply_patch,
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


def preview_patch(args: dict[str, Any]) -> dict[str, Any]:
    return process_patch(args, write=False)


def apply_patch(args: dict[str, Any]) -> dict[str, Any]:
    return process_patch(args, write=not bool(args.get("dry_run", False)))


def process_patch(args: dict[str, Any], write: bool) -> dict[str, Any]:
    root = resolve_root(args)
    operations = args.get("operations")
    if not isinstance(operations, list) or not operations:
        raise SkillError("invalid_input", "operations must be a non-empty array")
    if len(operations) > 100:
        raise SkillError("too_many_operations", "operations must contain 100 items or fewer")
    encoding = str(args.get("encoding") or "utf-8")
    allow_hidden = bool(args.get("allow_hidden", False))
    max_file_bytes = clamp_int(args.get("max_file_bytes"), DEFAULT_MAX_FILE_BYTES, 1, DEFAULT_MAX_FILE_BYTES)

    before_by_path: dict[Path, str] = {}
    after_by_path: dict[Path, str] = {}
    parent_create: set[Path] = set()
    operation_results: list[dict[str, Any]] = []

    for index, raw_operation in enumerate(operations):
        if not isinstance(raw_operation, dict):
            raise SkillError("invalid_operation", f"operation {index} must be an object")
        result = apply_operation(
            root,
            raw_operation,
            index,
            encoding,
            allow_hidden,
            max_file_bytes,
            before_by_path,
            after_by_path,
            parent_create,
        )
        operation_results.append(result)

    changed_files = build_changed_files(root, before_by_path, after_by_path)
    changed = len(changed_files) > 0
    diff_summary = build_combined_diff(root, before_by_path, after_by_path)
    dry_run = not write

    if write and changed:
        written: list[str] = []
        try:
            for target, after in after_by_path.items():
                before = before_by_path[target]
                if before == after:
                    continue
                if target.parent in parent_create and not target.parent.exists():
                    target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(encode_text(after, encoding))
                written.append(relative_path(root, target))
        except OSError as exc:
            return {
                "outcome": "partial" if written else "failed",
                "root": str(root),
                "changed": len(written) > 0,
                "dry_run": False,
                "changed_files": changed_files,
                "operation_results": operation_results,
                "diff_summary": diff_summary,
                "failure_class": "write_failed",
                "failure_reason": str(exc),
                "written_files": written,
                "created_at": utc_now(),
            }

    return {
        "outcome": "clean",
        "root": str(root),
        "changed": changed,
        "dry_run": dry_run,
        "changed_files": changed_files,
        "operation_results": operation_results,
        "diff_summary": diff_summary,
        "failure_class": "",
        "created_at": utc_now(),
    }


def apply_operation(
    root: Path,
    operation: dict[str, Any],
    index: int,
    encoding: str,
    allow_hidden: bool,
    max_file_bytes: int,
    before_by_path: dict[Path, str],
    after_by_path: dict[Path, str],
    parent_create: set[Path],
) -> dict[str, Any]:
    op = str(operation.get("op") or operation.get("operation") or "")
    if not op:
        raise SkillError("invalid_operation", f"operation {index} is missing op")
    path_value = operation.get("path")
    if not path_value:
        raise SkillError("invalid_operation", f"operation {index} is missing path")
    target = resolve_scoped_path(root, path_value)
    assert_visible_path(root, target, allow_hidden or bool(operation.get("allow_hidden", False)))
    actual_exists = target.exists()
    existed_before_operation = actual_exists or target in after_by_path
    before = load_current_text(root, target, encoding, max_file_bytes, before_by_path, after_by_path)
    exists = existed_before_operation

    expected_sha = str(operation.get("expected_sha256") or "")
    if expected_sha and sha256_text(before, encoding) != expected_sha:
        raise SkillError("hash_mismatch", f"operation {index} expected_sha256 does not match: {relative_path(root, target)}")

    replacements = 0
    if op == "create_file":
        if exists and not bool(operation.get("allow_overwrite", False)):
            raise SkillError("file_exists", f"operation {index} target exists: {relative_path(root, target)}")
        if actual_exists and not expected_sha:
            raise SkillError("precondition_required", f"operation {index} overwrite requires expected_sha256")
        next_text = require_text(operation.get("content"), "content")
        mark_parent_create(root, target, operation, parent_create)
        replacements = 1
    elif op == "replace_file":
        if not exists and not bool(operation.get("create_if_missing", False)):
            raise SkillError("file_not_found", f"operation {index} target missing: {relative_path(root, target)}")
        if actual_exists and not expected_sha:
            raise SkillError("precondition_required", f"operation {index} replace_file requires expected_sha256")
        next_text = require_text(operation.get("content"), "content")
        mark_parent_create(root, target, operation, parent_create)
        replacements = 1
    elif op == "append_text":
        ensure_existing_file(root, target, exists, index)
        if actual_exists and not expected_sha:
            raise SkillError("precondition_required", f"operation {index} append_text requires expected_sha256")
        text = require_text(operation.get("text"), "text")
        next_text = before + text
        replacements = 1
    elif op == "prepend_text":
        ensure_existing_file(root, target, exists, index)
        if actual_exists and not expected_sha:
            raise SkillError("precondition_required", f"operation {index} prepend_text requires expected_sha256")
        text = require_text(operation.get("text"), "text")
        next_text = text + before
        replacements = 1
    elif op == "replace_text":
        ensure_existing_file(root, target, exists, index)
        old_text = require_text(operation.get("old_text"), "old_text")
        new_text = require_text(operation.get("new_text"), "new_text")
        next_text, replacements = replace_text(before, old_text, new_text, operation, index, target, root)
    elif op in {"insert_after", "insert_before"}:
        ensure_existing_file(root, target, exists, index)
        anchor = require_text(operation.get("anchor"), "anchor")
        text = require_text(operation.get("text"), "text")
        next_text, replacements = insert_text(before, anchor, text, operation, index, target, root, after=(op == "insert_after"))
    else:
        raise SkillError("unsupported_operation", f"operation {index} has unsupported op: {op}")

    if b"\x00" in encode_text(next_text, encoding):
        raise SkillError("binary_file", f"operation {index} produced null bytes: {relative_path(root, target)}")
    after_by_path[target] = next_text
    return {
        "index": index,
        "path": relative_path(root, target),
        "op": op,
        "changed": before != next_text,
        "replacements": replacements,
        "before_sha256": sha256_text(before, encoding) if exists else "",
        "after_sha256": sha256_text(next_text, encoding),
    }


def load_current_text(
    root: Path,
    target: Path,
    encoding: str,
    max_file_bytes: int,
    before_by_path: dict[Path, str],
    after_by_path: dict[Path, str],
) -> str:
    if target in after_by_path:
        return after_by_path[target]
    if target in before_by_path:
        return before_by_path[target]
    if not target.exists():
        before_by_path[target] = ""
        return ""
    if not target.is_file():
        raise SkillError("not_file", f"path is not a file: {relative_path(root, target)}")
    data = target.read_bytes()
    if len(data) > max_file_bytes:
        raise SkillError("file_too_large", f"file exceeds {max_file_bytes} bytes: {relative_path(root, target)}")
    if is_binary(data[: min(len(data), TEXT_SAMPLE_BYTES)]):
        raise SkillError("binary_file", f"refusing to patch binary file: {relative_path(root, target)}")
    text = decode_text(data, encoding)
    before_by_path[target] = text
    return text


def replace_text(
    before: str,
    old_text: str,
    new_text: str,
    operation: dict[str, Any],
    index: int,
    target: Path,
    root: Path,
) -> tuple[str, int]:
    if old_text == "":
        raise SkillError("invalid_operation", f"operation {index} old_text must not be empty")
    occurrence = str(operation.get("occurrence") or "first")
    if occurrence == "first":
        if old_text not in before:
            raise SkillError("anchor_not_found", f"operation {index} old_text not found: {relative_path(root, target)}")
        return before.replace(old_text, new_text, 1), 1
    if occurrence == "last":
        pos = before.rfind(old_text)
        if pos < 0:
            raise SkillError("anchor_not_found", f"operation {index} old_text not found: {relative_path(root, target)}")
        return before[:pos] + new_text + before[pos + len(old_text) :], 1
    if occurrence == "all":
        count = before.count(old_text)
        if count == 0:
            raise SkillError("anchor_not_found", f"operation {index} old_text not found: {relative_path(root, target)}")
        max_replacements = operation.get("max_replacements")
        if max_replacements is not None and count > int(max_replacements):
            raise SkillError("too_many_replacements", f"operation {index} would replace {count} occurrences")
        return before.replace(old_text, new_text), count
    raise SkillError("invalid_operation", f"operation {index} occurrence must be first, last, or all")


def insert_text(
    before: str,
    anchor: str,
    text: str,
    operation: dict[str, Any],
    index: int,
    target: Path,
    root: Path,
    *,
    after: bool,
) -> tuple[str, int]:
    if anchor == "":
        raise SkillError("invalid_operation", f"operation {index} anchor must not be empty")
    occurrence = str(operation.get("occurrence") or "first")
    if occurrence == "first":
        pos = before.find(anchor)
    elif occurrence == "last":
        pos = before.rfind(anchor)
    else:
        raise SkillError("invalid_operation", f"operation {index} occurrence must be first or last")
    if pos < 0:
        raise SkillError("anchor_not_found", f"operation {index} anchor not found: {relative_path(root, target)}")
    insert_at = pos + len(anchor) if after else pos
    return before[:insert_at] + text + before[insert_at:], 1


def mark_parent_create(root: Path, target: Path, operation: dict[str, Any], parent_create: set[Path]) -> None:
    parent = target.parent
    if parent.exists():
        if not parent.is_dir():
            raise SkillError("parent_not_directory", f"parent is not a directory: {relative_path(root, parent)}")
        return
    if not bool(operation.get("create_parent_dirs", False)):
        raise SkillError("parent_not_found", f"parent directory does not exist: {relative_path(root, parent)}")
    parent_create.add(parent)


def ensure_existing_file(root: Path, target: Path, exists: bool, index: int) -> None:
    if not exists:
        raise SkillError("file_not_found", f"operation {index} target missing: {relative_path(root, target)}")
    if target.exists() and not target.is_file():
        raise SkillError("not_file", f"operation {index} target is not a file: {relative_path(root, target)}")


def build_changed_files(root: Path, before_by_path: dict[Path, str], after_by_path: dict[Path, str]) -> list[dict[str, Any]]:
    changed: list[dict[str, Any]] = []
    for target in sorted(after_by_path, key=lambda item: relative_path(root, item)):
        before = before_by_path[target]
        after = after_by_path[target]
        if before == after:
            continue
        action = "create" if before == "" and not target.exists() else "patch"
        changed.append(
            {
                "path": relative_path(root, target),
                "action": action,
                "before_sha256": sha256_text(before) if target.exists() else "",
                "after_sha256": sha256_text(after),
                "bytes_before": len(before.encode("utf-8")),
                "bytes_after": len(after.encode("utf-8")),
            }
        )
    return changed


def build_combined_diff(root: Path, before_by_path: dict[Path, str], after_by_path: dict[Path, str]) -> dict[str, Any]:
    all_lines: list[str] = []
    additions = 0
    deletions = 0
    for target in sorted(after_by_path, key=lambda item: relative_path(root, item)):
        before = before_by_path[target]
        after = after_by_path[target]
        if before == after:
            continue
        path = relative_path(root, target)
        diff_lines = list(
            difflib.unified_diff(
                before.splitlines(),
                after.splitlines(),
                fromfile=f"a/{path}",
                tofile=f"b/{path}",
                lineterm="",
            )
        )
        additions += sum(1 for line in diff_lines if line.startswith("+") and not line.startswith("+++"))
        deletions += sum(1 for line in diff_lines if line.startswith("-") and not line.startswith("---"))
        all_lines.extend(diff_lines)
    truncated = len(all_lines) > MAX_DIFF_LINES
    return {
        "additions": additions,
        "deletions": deletions,
        "unified_diff": ("\n".join(all_lines[:MAX_DIFF_LINES]) + "\n") if all_lines else "",
        "truncated": truncated,
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


def encode_text(value: str, encoding: str = "utf-8") -> bytes:
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


def is_binary(sample: bytes) -> bool:
    if b"\x00" in sample:
        return True
    if not sample:
        return False
    control = sum(1 for byte in sample if byte < 32 and byte not in (9, 10, 13))
    return control / len(sample) > 0.30


def sha256_text(value: str, encoding: str = "utf-8") -> str:
    return hashlib.sha256(value.encode(encoding)).hexdigest()


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
