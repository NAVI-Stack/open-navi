#!/usr/bin/env python3
"""Read-only repository inspection entrypoint for NAVI Programmer."""

from __future__ import annotations

import datetime as _dt
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from typing import Any


SKILL_ID = "navi-programmer.repo-inspect"
DEFAULT_EXCLUDED_NAMES = {
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
TEXT_SAMPLE_BYTES = 4096
PREVIEW_CHARS = 220


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
            "list_tree": list_tree,
            "read_file": read_file,
            "search_text": search_text,
            "repo_status": repo_status,
            "inspect_diff": inspect_diff,
        }
        handler = handlers.get(interface)
        if handler is None:
            raise SkillError("unknown_interface", f"unsupported interface: {interface}")

        output = handler(args)
        emit("success", started, interface, output=output)
        return 0
    except SkillError as exc:
        emit(
            "error",
            started,
            interface,
            error={"type": exc.code, "message": exc.message},
        )
        return 0
    except json.JSONDecodeError as exc:
        emit(
            "error",
            started,
            interface,
            error={"type": "invalid_json", "message": str(exc)},
        )
        return 0
    except Exception as exc:  # Defensive envelope: never print raw tracebacks.
        emit(
            "error",
            started,
            interface,
            error={"type": exc.__class__.__name__, "message": str(exc)},
        )
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


def list_tree(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    base = resolve_scoped_path(root, args.get("path") or ".")
    include_hidden = bool(args.get("include_hidden", False))
    assert_visible_path(root, base, include_hidden)
    if not base.exists():
        raise SkillError("file_not_found", f"path does not exist: {relative_path(root, base)}")
    if not base.is_dir():
        raise SkillError("not_directory", f"path is not a directory: {relative_path(root, base)}")

    recursive = bool(args.get("recursive", False))
    max_depth = clamp_int(args.get("max_depth", 2 if recursive else 1), 1, 8)
    max_entries = clamp_int(args.get("max_entries", 200), 1, 2000)

    entries: list[dict[str, Any]] = []
    excluded = 0
    truncated = False

    def visit(directory: Path, depth: int) -> None:
        nonlocal excluded, truncated
        if truncated:
            return
        children = sorted(
            directory.iterdir(),
            key=lambda item: (not item.is_dir(), item.name.lower()),
        )
        for child in children:
            if should_skip(root, child, include_hidden):
                excluded += 1
                continue
            entries.append(entry_for(root, child))
            if len(entries) >= max_entries:
                truncated = True
                return
            if recursive and depth < max_depth and child.is_dir() and not child.is_symlink():
                visit(child, depth + 1)
                if truncated:
                    return

    visit(base, 1)
    return {
        "root": str(root),
        "base_path": relative_path(root, base),
        "entries": entries,
        "truncated": truncated,
        "excluded_entries": excluded,
        "limits": {"max_depth": max_depth, "max_entries": max_entries},
    }


def read_file(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    path_value = args.get("path")
    if not path_value:
        raise SkillError("invalid_input", "path is required")
    target = resolve_scoped_path(root, path_value)
    include_hidden = bool(args.get("include_hidden", False))
    assert_visible_path(root, target, include_hidden)
    if not target.exists():
        raise SkillError("file_not_found", f"file does not exist: {relative_path(root, target)}")
    if not target.is_file():
        raise SkillError("not_file", f"path is not a file: {relative_path(root, target)}")

    max_bytes = clamp_int(args.get("max_bytes", 65536), 1, 1048576)
    data = target.read_bytes()
    binary = is_binary(data[: min(len(data), TEXT_SAMPLE_BYTES)])
    if binary:
        raise SkillError("binary_file", f"refusing to read binary file: {relative_path(root, target)}")

    truncated = len(data) > max_bytes
    selected = data[:max_bytes]
    encoding = str(args.get("encoding") or "utf-8")
    try:
        content = selected.decode(encoding)
    except LookupError:
        raise SkillError("invalid_encoding", f"unknown encoding: {encoding}")
    except UnicodeDecodeError as exc:
        raise SkillError("decode_error", str(exc))

    start_line = maybe_int(args.get("start_line"))
    end_line = maybe_int(args.get("end_line"))
    total_lines = None
    if start_line is not None or end_line is not None:
        lines = content.splitlines(keepends=True)
        total_lines = len(lines)
        start = start_line or 1
        end = end_line or total_lines
        if start < 1:
            raise SkillError("invalid_input", "start_line must be 1 or greater")
        if end < start:
            raise SkillError("invalid_input", "end_line must be greater than or equal to start_line")
        content = "".join(lines[start - 1 : end])

    return {
        "path": relative_path(root, target),
        "content": content,
        "truncated": truncated,
        "bytes_read": len(selected),
        "size_bytes": target.stat().st_size,
        "line_window": {"start_line": start_line, "end_line": end_line},
        "total_lines": total_lines,
    }


def search_text(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    query = str(args.get("query") or "")
    if query == "":
        raise SkillError("invalid_input", "query is required")
    base = resolve_scoped_path(root, args.get("path") or ".")
    include_hidden = bool(args.get("include_hidden", False))
    assert_visible_path(root, base, include_hidden)
    if not base.exists():
        raise SkillError("file_not_found", f"path does not exist: {relative_path(root, base)}")

    use_regex = bool(args.get("regex", False))
    case_sensitive = bool(args.get("case_sensitive", False))
    max_matches = clamp_int(args.get("max_matches", 100), 1, 1000)
    max_files = clamp_int(args.get("max_files", 500), 1, 5000)
    max_file_bytes = clamp_int(args.get("max_file_bytes", 524288), 1, 5242880)
    matcher = build_matcher(query, use_regex, case_sensitive)

    matches: list[dict[str, Any]] = []
    files_scanned = 0
    files_skipped = 0
    truncated = False

    for file_path in iter_files(root, base, include_hidden):
        if files_scanned >= max_files:
            truncated = True
            break
        try:
            size = file_path.stat().st_size
            if size > max_file_bytes:
                files_skipped += 1
                continue
            data = file_path.read_bytes()
            if is_binary(data[: min(len(data), TEXT_SAMPLE_BYTES)]):
                files_skipped += 1
                continue
            text = data.decode("utf-8")
        except (OSError, UnicodeDecodeError):
            files_skipped += 1
            continue

        files_scanned += 1
        for line_no, line in enumerate(text.splitlines(), start=1):
            column = matcher(line)
            if column is None:
                continue
            matches.append(
                {
                    "path": relative_path(root, file_path),
                    "line": line_no,
                    "column": column,
                    "preview": preview_line(line),
                }
            )
            if len(matches) >= max_matches:
                truncated = True
                break
        if truncated:
            break

    return {
        "root": str(root),
        "base_path": relative_path(root, base),
        "query": query,
        "regex": use_regex,
        "case_sensitive": case_sensitive,
        "matches": matches,
        "files_scanned": files_scanned,
        "files_skipped": files_skipped,
        "truncated": truncated,
        "limits": {
            "max_matches": max_matches,
            "max_files": max_files,
            "max_file_bytes": max_file_bytes,
        },
    }


def repo_status(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    base = str(root)
    available = True
    try:
        top = run_git(base, "rev-parse", "--show-toplevel")
    except FileNotFoundError:
        return {"git_available": False, "is_git_repo": False, "root": str(root)}
    except subprocess.CalledProcessError:
        return {"git_available": available, "is_git_repo": False, "root": str(root)}

    repo_root = top.stdout.strip() or str(root)
    branch_result = run_git_optional(base, "symbolic-ref", "--quiet", "--short", "HEAD")
    branch = branch_result.stdout.strip() if branch_result.returncode == 0 else ""
    head_result = run_git_optional(base, "rev-parse", "--short", "HEAD")
    head = head_result.stdout.strip() if head_result.returncode == 0 else ""
    status_result = run_git_optional(base, "status", "--short", "--branch")
    if status_result.returncode != 0:
        detail = status_result.stderr.strip() or status_result.stdout.strip() or "git status failed"
        raise SkillError("git_status_failed", detail)
    status = status_result.stdout.splitlines()
    changes = [line for line in status if not line.startswith("##")]
    return {
        "git_available": available,
        "is_git_repo": True,
        "root": str(root),
        "repo_root": repo_root,
        "branch": branch,
        "head": head,
        "dirty": len(changes) > 0,
        "status_lines": status,
    }


def inspect_diff(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    path_value = args.get("path")
    include_untracked = bool(args.get("include_untracked", True))
    staged = bool(args.get("staged", False))
    context_lines = clamp_int(args.get("context_lines", 3), 0, 20)
    max_bytes = clamp_int(args.get("max_bytes", 131072), 1, 1048576)

    target: Path | None = None
    rel_path = "."
    if path_value:
        target = resolve_scoped_path(root, path_value)
        assert_visible_path(root, target, include_hidden=False)
        rel_path = relative_path(root, target)
        if not target.exists():
            raise SkillError("file_not_found", f"path does not exist: {rel_path}")

    base = str(root)
    try:
        top = run_git(base, "rev-parse", "--show-toplevel")
    except FileNotFoundError:
        return {
            "git_available": False,
            "is_git_repo": False,
            "root": str(root),
            "path_filter": rel_path,
            "staged": staged,
        }
    except subprocess.CalledProcessError:
        return {
            "git_available": True,
            "is_git_repo": False,
            "root": str(root),
            "path_filter": rel_path,
            "staged": staged,
        }

    repo_root = top.stdout.strip() or str(root)
    git_args = ["diff", "--no-ext-diff", "--no-color", f"--unified={context_lines}"]
    if staged:
        git_args.append("--cached")
    if target is not None:
        git_args.extend(["--", rel_path])

    patch_result = run_git_optional(base, *git_args)
    if patch_result.returncode != 0:
        detail = patch_result.stderr.strip() or patch_result.stdout.strip() or "git diff failed"
        raise SkillError("git_diff_failed", detail)

    name_args = ["diff", "--name-status"]
    if staged:
        name_args.append("--cached")
    if target is not None:
        name_args.extend(["--", rel_path])
    names_result = run_git_optional(base, *name_args)
    if names_result.returncode != 0:
        detail = names_result.stderr.strip() or names_result.stdout.strip() or "git diff --name-status failed"
        raise SkillError("git_diff_failed", detail)

    files_changed = [parse_name_status_line(line) for line in names_result.stdout.splitlines() if line.strip()]
    untracked_files: list[str] = []
    if include_untracked and not staged:
        untracked_args = ["ls-files", "--others", "--exclude-standard"]
        if target is not None:
            untracked_args.extend(["--", rel_path])
        untracked_result = run_git_optional(base, *untracked_args)
        if untracked_result.returncode != 0:
            detail = (
                untracked_result.stderr.strip()
                or untracked_result.stdout.strip()
                or "git ls-files failed"
            )
            raise SkillError("git_diff_failed", detail)
        untracked_files = [line.strip() for line in untracked_result.stdout.splitlines() if line.strip()]
        for path in untracked_files:
            files_changed.append({"path": path, "status": "untracked"})

    patch = patch_result.stdout
    truncated = len(patch.encode("utf-8")) > max_bytes
    if truncated:
        patch = truncate_utf8(patch, max_bytes)

    return {
        "git_available": True,
        "is_git_repo": True,
        "root": str(root),
        "repo_root": repo_root,
        "path_filter": rel_path,
        "staged": staged,
        "include_untracked": include_untracked,
        "context_lines": context_lines,
        "files_changed": files_changed,
        "untracked_files": untracked_files,
        "patch": patch,
        "truncated": truncated,
        "bytes_read": len(patch.encode("utf-8")),
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
    raw = str(path_value or ".")
    candidate = Path(raw).expanduser()
    if not candidate.is_absolute():
        candidate = root / candidate
    resolved = candidate.resolve()
    if not is_relative_to(resolved, root):
        raise SkillError("path_out_of_scope", f"path escapes root: {raw}")
    return resolved


def assert_visible_path(root: Path, target: Path, include_hidden: bool) -> None:
    rel = relative_path(root, target)
    if rel == ".":
        return
    for part in Path(rel).parts:
        if denied_name(part, include_hidden):
            raise SkillError("path_denied", f"path is denied by default: {rel}")


def should_skip(root: Path, target: Path, include_hidden: bool) -> bool:
    rel = relative_path(root, target)
    if rel == ".":
        return False
    return any(denied_name(part, include_hidden) for part in Path(rel).parts)


def denied_name(name: str, include_hidden: bool) -> bool:
    if name in DEFAULT_EXCLUDED_NAMES:
        return True
    if include_hidden:
        return False
    return name.startswith(".")


def entry_for(root: Path, path: Path) -> dict[str, Any]:
    try:
        stat = path.lstat()
    except OSError:
        return {"path": relative_path(root, path), "type": "unknown"}
    if path.is_symlink():
        kind = "symlink"
    elif path.is_dir():
        kind = "directory"
    elif path.is_file():
        kind = "file"
    else:
        kind = "other"
    entry: dict[str, Any] = {
        "path": relative_path(root, path),
        "type": kind,
        "modified_utc": _dt.datetime.fromtimestamp(stat.st_mtime, tz=_dt.timezone.utc)
        .replace(microsecond=0)
        .isoformat()
        .replace("+00:00", "Z"),
    }
    if kind == "file":
        entry["size_bytes"] = stat.st_size
    return entry


def iter_files(root: Path, base: Path, include_hidden: bool):
    if base.is_file():
        if not should_skip(root, base, include_hidden):
            yield base
        return
    if not base.is_dir():
        raise SkillError("not_searchable", f"path is not searchable: {relative_path(root, base)}")
    for current, dir_names, file_names in os.walk(base):
        current_path = Path(current)
        dir_names[:] = [
            name for name in dir_names if not should_skip(root, current_path / name, include_hidden)
        ]
        for name in sorted(file_names):
            file_path = current_path / name
            if should_skip(root, file_path, include_hidden):
                continue
            yield file_path


def build_matcher(query: str, use_regex: bool, case_sensitive: bool):
    if use_regex:
        flags = 0 if case_sensitive else re.IGNORECASE
        try:
            pattern = re.compile(query, flags)
        except re.error as exc:
            raise SkillError("invalid_regex", str(exc))

        def regex_match(line: str) -> int | None:
            found = pattern.search(line)
            return None if found is None else found.start() + 1

        return regex_match

    needle = query if case_sensitive else query.lower()

    def substring_match(line: str) -> int | None:
        haystack = line if case_sensitive else line.lower()
        index = haystack.find(needle)
        return None if index < 0 else index + 1

    return substring_match


def run_git(cwd: str, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=5,
        check=True,
    )


def run_git_optional(cwd: str, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=5,
        check=False,
    )


def parse_name_status_line(line: str) -> dict[str, str]:
    parts = line.split("\t")
    status = parts[0].strip()
    path = parts[-1].strip() if len(parts) > 1 else ""
    return {"path": path, "status": status or "unknown"}


def is_binary(sample: bytes) -> bool:
    if b"\x00" in sample:
        return True
    if not sample:
        return False
    control = sum(1 for byte in sample if byte < 32 and byte not in (9, 10, 13))
    return control / len(sample) > 0.30


def preview_line(line: str) -> str:
    compact = line.strip()
    if len(compact) <= PREVIEW_CHARS:
        return compact
    return compact[: PREVIEW_CHARS - 3] + "..."


def truncate_utf8(value: str, max_bytes: int) -> str:
    encoded = value.encode("utf-8")
    if len(encoded) <= max_bytes:
        return value
    trimmed = encoded[:max_bytes]
    return trimmed.decode("utf-8", errors="ignore")


def clamp_int(value: Any, minimum: int, maximum: int) -> int:
    parsed = maybe_int(value)
    if parsed is None:
        parsed = minimum
    return max(minimum, min(maximum, parsed))


def maybe_int(value: Any) -> int | None:
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        raise SkillError("invalid_input", f"expected integer, got {value!r}")


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


if __name__ == "__main__":
    raise SystemExit(main())
