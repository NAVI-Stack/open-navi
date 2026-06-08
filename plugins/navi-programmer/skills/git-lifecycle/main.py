#!/usr/bin/env python3
"""Local Git lifecycle entrypoint for NAVI Programmer."""

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


SKILL_ID = "navi-programmer.git-lifecycle"
DEFAULT_MAX_DIFF_BYTES = 128 * 1024
MAX_DIFF_BYTES = 1024 * 1024
BRANCH_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$")
DENIED_PATH_PARTS = {
    ".git",
    ".hg",
    ".svn",
    ".navi",
    ".venv",
    "venv",
    "node_modules",
    "__pycache__",
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
            "inspect_status": inspect_status,
            "create_branch": create_branch,
            "create_commit": create_commit,
            "prepare_review": prepare_review,
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


def inspect_status(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    paths = resolve_pathspecs(repo, args.get("paths"))
    status = collect_status(repo, paths)
    include_diff = bool(args.get("include_diff", False))
    max_diff_bytes = clamp_int(args.get("max_diff_bytes"), DEFAULT_MAX_DIFF_BYTES, 1024, MAX_DIFF_BYTES)
    output = {
        **repo_summary(repo),
        **status,
        "remote_actions": {"push": False, "pull_request": False},
    }
    if include_diff:
        output["diff"] = collect_diff(repo, paths, max_diff_bytes)
    return output


def create_branch(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    branch_name = str(args.get("branch_name") or "").strip()
    if not branch_name:
        raise SkillError("invalid_input", "branch_name is required")
    validate_branch_name(repo, branch_name)
    checkout = bool(args.get("checkout", True))
    allow_existing = bool(args.get("allow_existing", False))
    allow_dirty = bool(args.get("allow_dirty", False))
    dry_run = bool(args.get("dry_run", False))
    start_point = str(args.get("start_point") or "").strip()
    status = collect_status(repo, [])
    if status["dirty"] and not allow_dirty:
        raise SkillError("dirty_worktree_blocked", "dirty worktree requires allow_dirty=true before branch creation")
    exists = branch_exists(repo, branch_name)
    if exists and not allow_existing:
        raise SkillError("branch_exists", f"branch already exists: {branch_name}")

    before = repo_summary(repo)
    would_run = ["git", "switch"]
    if not exists:
        would_run.append("-c")
    would_run.append(branch_name)
    if start_point and not exists:
        would_run.append(start_point)

    if not dry_run:
        if checkout:
            run_git(repo, *would_run[1:])
        elif not exists:
            args_for_branch = ["branch", branch_name]
            if start_point:
                args_for_branch.append(start_point)
            run_git(repo, *args_for_branch)
    after = repo_summary(repo)
    return {
        "outcome": "dry_run" if dry_run else "clean",
        "root": str(repo),
        "branch": branch_name,
        "created": (not exists) and not dry_run,
        "checked_out": checkout and not dry_run,
        "dry_run": dry_run,
        "command": would_run,
        "before": before,
        "after": after,
        "remote_actions": {"push": False, "pull_request": False},
        "created_at": utc_now(),
    }


def create_commit(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    message = str(args.get("message") or "").strip()
    if not message:
        raise SkillError("invalid_input", "message is required")
    if "\x00" in message:
        raise SkillError("invalid_input", "message contains null bytes")
    paths = resolve_pathspecs(repo, args.get("paths"))
    if not paths:
        raise SkillError("invalid_input", "paths must contain at least one explicit file path")
    expected_branch = str(args.get("expected_branch") or "").strip()
    current_branch = current_branch_name(repo)
    if expected_branch and current_branch != expected_branch:
        raise SkillError("branch_mismatch", f"expected branch {expected_branch}, got {current_branch}")
    dry_run = bool(args.get("dry_run", False))
    allow_untracked = bool(args.get("allow_untracked", False))
    selected = selected_status(repo, paths)
    if not selected["changed_files"]:
        raise SkillError("no_changes_selected", "selected paths have no changes to commit")
    if selected["untracked_files"] and not allow_untracked:
        raise SkillError("untracked_not_allowed", "untracked files require allow_untracked=true")

    if dry_run:
        return {
            "outcome": "dry_run",
            "root": str(repo),
            "branch": current_branch,
            "message": message,
            "paths": [str(path) for path in paths],
            "changed_files": selected["changed_files"],
            "commit": "",
            "dry_run": True,
            "remote_actions": {"push": False, "pull_request": False},
        }

    for path in paths:
        run_git(repo, "add", "--", str(path))
    commit_result = run_git_optional(repo, "commit", "-m", message, env=commit_identity_env(repo))
    if commit_result.returncode != 0:
        detail = commit_result.stderr.strip() or commit_result.stdout.strip() or "git commit failed"
        raise SkillError("commit_failed", detail)
    head = git_output(repo, "rev-parse", "--short", "HEAD")
    return {
        "outcome": "clean",
        "root": str(repo),
        "branch": current_branch_name(repo),
        "message": message,
        "paths": [str(path) for path in paths],
        "changed_files": selected["changed_files"],
        "commit": head,
        "dry_run": False,
        "remote_actions": {"push": False, "pull_request": False},
        "created_at": utc_now(),
    }


def prepare_review(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    paths = resolve_pathspecs(repo, args.get("paths"))
    status = collect_status(repo, paths)
    include_diff = bool(args.get("include_diff", True))
    max_diff_bytes = clamp_int(args.get("max_diff_bytes"), DEFAULT_MAX_DIFF_BYTES, 1024, MAX_DIFF_BYTES)
    title = str(args.get("title") or "").strip() or default_review_title(repo)
    summary = str(args.get("summary") or "").strip()
    validation_summary = str(args.get("validation_summary") or "").strip()
    diff = collect_diff(repo, paths, max_diff_bytes) if include_diff else {"included": False}
    handoff = build_handoff(title, summary, validation_summary, status, diff)
    return {
        **repo_summary(repo),
        "title": title,
        "summary": summary,
        "validation_summary": validation_summary,
        "status": status,
        "diff": diff,
        "handoff_markdown": handoff,
        "remote_actions": {
            "push": False,
            "pull_request": False,
            "blocked_reason": "remote actions are confirmation-gated and not performed by this skill",
        },
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


def resolve_repo(root: Path) -> Path:
    try:
        top = git_output(root, "rev-parse", "--show-toplevel")
    except SkillError:
        raise SkillError("not_git_repo", f"not a git repository: {root}")
    return Path(top).resolve()


def repo_summary(repo: Path) -> dict[str, Any]:
    branch = current_branch_name(repo)
    head = git_output_optional(repo, "rev-parse", "--short", "HEAD")
    upstream = git_output_optional(repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
    return {
        "repo_root": str(repo),
        "branch": branch,
        "head": head,
        "upstream": upstream,
    }


def collect_status(repo: Path, paths: list[Path]) -> dict[str, Any]:
    args = ["status", "--porcelain=v1", "--branch"]
    if paths:
        args.extend(["--", *[str(path) for path in paths]])
    lines = git_output(repo, *args).splitlines()
    branch_line = lines[0] if lines and lines[0].startswith("##") else ""
    entries = parse_status_lines(lines)
    staged = [item for item in entries if item["index"] != " " and item["index"] != "?"]
    unstaged = [item for item in entries if item["worktree"] != " " and item["index"] != "?"]
    untracked = [item for item in entries if item["index"] == "?" and item["worktree"] == "?"]
    return {
        "branch_line": branch_line,
        "dirty": len(entries) > 0,
        "status_lines": lines,
        "changed_files": entries,
        "staged_files": staged,
        "unstaged_files": unstaged,
        "untracked_files": untracked,
    }


def selected_status(repo: Path, paths: list[Path]) -> dict[str, Any]:
    return collect_status(repo, paths)


def parse_status_lines(lines: list[str]) -> list[dict[str, str]]:
    entries: list[dict[str, str]] = []
    for line in lines:
        if line.startswith("##") or len(line) < 3:
            continue
        index = line[0]
        worktree = line[1]
        path = line[3:]
        original_path = ""
        if " -> " in path:
            original_path, path = path.split(" -> ", 1)
        entries.append(
            {
                "path": path,
                "original_path": original_path,
                "index": index,
                "worktree": worktree,
                "status": (index + worktree).strip() or "modified",
            }
        )
    return entries


def collect_diff(repo: Path, paths: list[Path], max_bytes: int) -> dict[str, Any]:
    args = ["diff", "--no-ext-diff"]
    if paths:
        args.extend(["--", *[str(path) for path in paths]])
    unstaged = git_output(repo, *args)
    staged_args = ["diff", "--cached", "--no-ext-diff"]
    if paths:
        staged_args.extend(["--", *[str(path) for path in paths]])
    staged = git_output(repo, *staged_args)
    combined = ""
    if staged:
        combined += "## Staged diff\n\n" + staged
    if unstaged:
        if combined:
            combined += "\n"
        combined += "## Unstaged diff\n\n" + unstaged
    data = combined.encode("utf-8", errors="replace")
    truncated = len(data) > max_bytes
    text = data[:max_bytes].decode("utf-8", errors="replace")
    return {
        "included": True,
        "diff": text,
        "bytes": min(len(data), max_bytes),
        "truncated": truncated,
        "max_bytes": max_bytes,
    }


def build_handoff(
    title: str,
    summary: str,
    validation_summary: str,
    status: dict[str, Any],
    diff: dict[str, Any],
) -> str:
    changed = status.get("changed_files", [])
    lines = [
        f"# {title}",
        "",
        "## Summary",
        summary or "- No summary provided.",
        "",
        "## Validation",
        validation_summary or "- Validation evidence not provided.",
        "",
        "## Changed Files",
    ]
    if changed:
        lines.extend(f"- `{item['path']}` ({item['status']})" for item in changed)
    else:
        lines.append("- No changed files detected.")
    lines.extend(
        [
            "",
            "## Remote Actions",
            "- Push: not performed",
            "- Pull request: not performed",
        ]
    )
    if diff.get("included"):
        lines.extend(["", "## Diff", "```diff", diff.get("diff", ""), "```"])
    return "\n".join(lines)


def default_review_title(repo: Path) -> str:
    branch = current_branch_name(repo)
    return f"Review handoff for {branch or 'detached HEAD'}"


def resolve_pathspecs(repo: Path, value: Any) -> list[Path]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise SkillError("invalid_input", "paths must be an array")
    out: list[Path] = []
    for raw in value:
        if not isinstance(raw, str) or not raw.strip():
            raise SkillError("invalid_input", "paths must contain non-empty strings")
        candidate = Path(raw).expanduser()
        if candidate.is_absolute():
            resolved = candidate.resolve()
            if not is_relative_to(resolved, repo):
                raise SkillError("path_out_of_scope", f"path escapes repo: {raw}")
            rel = resolved.relative_to(repo)
        else:
            rel = Path(raw)
            resolved = (repo / rel).resolve()
            if not is_relative_to(resolved, repo):
                raise SkillError("path_out_of_scope", f"path escapes repo: {raw}")
        for part in rel.parts:
            if part in DENIED_PATH_PARTS:
                raise SkillError("path_denied", f"path is denied: {raw}")
        out.append(Path(rel.as_posix()))
    return out


def validate_branch_name(repo: Path, branch_name: str) -> None:
    if branch_name.startswith("-") or not BRANCH_RE.match(branch_name):
        raise SkillError("branch_invalid", f"invalid branch name: {branch_name}")
    result = run_git_optional(repo, "check-ref-format", "--branch", branch_name)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or "git check-ref-format failed"
        raise SkillError("branch_invalid", detail)


def branch_exists(repo: Path, branch_name: str) -> bool:
    result = run_git_optional(repo, "rev-parse", "--verify", "--quiet", f"refs/heads/{branch_name}")
    return result.returncode == 0


def current_branch_name(repo: Path) -> str:
    branch = git_output_optional(repo, "symbolic-ref", "--quiet", "--short", "HEAD")
    return branch or "HEAD"


def git_output(cwd: Path, *args: str) -> str:
    result = run_git_optional(cwd, *args)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"git {' '.join(args)} failed"
        raise SkillError("git_failed", detail)
    return result.stdout.strip()


def git_output_optional(cwd: Path, *args: str) -> str:
    result = run_git_optional(cwd, *args)
    if result.returncode != 0:
        return ""
    return result.stdout.strip()


def run_git(cwd: Path, *args: str) -> subprocess.CompletedProcess[str]:
    result = run_git_optional(cwd, *args)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"git {' '.join(args)} failed"
        raise SkillError("git_failed", detail)
    return result


def run_git_optional(
    cwd: Path, *args: str, env: dict[str, str] | None = None
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args],
        cwd=str(cwd),
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
        env=env,
    )


def commit_identity_env(repo: Path) -> dict[str, str] | None:
    """Return an environment carrying a fallback git identity when the repo has
    none configured, so autonomous commits inside a clean container do not fail
    with "Author identity unknown". A configured identity (repo or global) is
    respected (returns None). Overridable via NAVI_GIT_AUTHOR_NAME/EMAIL."""
    configured = run_git_optional(repo, "config", "user.email")
    if configured.returncode == 0 and configured.stdout.strip():
        return None
    name = os.environ.get("NAVI_GIT_AUTHOR_NAME", "NAVI Programmer")
    email = os.environ.get("NAVI_GIT_AUTHOR_EMAIL", "navi-programmer@navi.local")
    env = os.environ.copy()
    env.setdefault("GIT_AUTHOR_NAME", name)
    env.setdefault("GIT_AUTHOR_EMAIL", email)
    env.setdefault("GIT_COMMITTER_NAME", name)
    env.setdefault("GIT_COMMITTER_EMAIL", email)
    return env


def clamp_int(value: Any, default: int, minimum: int, maximum: int) -> int:
    if value is None:
        parsed = default
    else:
        try:
            parsed = int(value)
        except (TypeError, ValueError):
            raise SkillError("invalid_input", f"expected integer, got {value!r}")
    return max(minimum, min(maximum, parsed))


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
