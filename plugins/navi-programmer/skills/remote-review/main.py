#!/usr/bin/env python3
"""Confirmation-gated remote review actions for NAVI Programmer."""

from __future__ import annotations

import datetime as _dt
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
from typing import Any


SKILL_ID = "navi-programmer.remote-review"
URL_RE = re.compile(r"https?://\S+")


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
            "push_branch": push_branch,
            "create_pull_request": create_pull_request,
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


def push_branch(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    branch = str(args.get("branch_name") or "").strip() or current_branch_name(repo)
    if not branch or branch == "HEAD":
        raise SkillError("branch_detached", "cannot push from detached HEAD without explicit branch_name")
    remote = str(args.get("remote") or "origin").strip() or "origin"
    ensure_remote_exists(repo, remote)
    set_upstream = bool(args.get("set_upstream", True))
    force_with_lease = bool(args.get("force_with_lease", False))
    dry_run = bool(args.get("dry_run", False))
    changed_files = normalize_changed_files(args.get("changed_files"))
    review_surface = build_review_surface(
        action="push_branch",
        repo=repo,
        title=str(args.get("title") or "").strip() or f"Push review for {branch}",
        summary=str(args.get("summary") or "").strip(),
        validation_summary=str(args.get("validation_summary") or "").strip(),
        changed_files=changed_files,
        review_handoff=str(args.get("review_handoff") or "").strip(),
        branch=branch,
        base_branch="",
    )
    confirmation = require_confirmation(args, "push_branch", review_surface)
    if confirmation is None:
        return blocked_confirmation_result("push_branch", review_surface)

    command = ["git", "push"]
    if dry_run:
        command.append("--dry-run")
    if force_with_lease:
        command.append("--force-with-lease")
    if set_upstream:
        command.extend(["--set-upstream", remote, branch])
    else:
        command.extend([remote, branch])

    result = run_command(repo, command, timeout=30)
    after = repo_summary(repo)
    return {
        "outcome": "dry_run" if dry_run else "clean",
        "repo_root": str(repo),
        "branch": branch,
        "remote": remote,
        "remote_url": remote_url(repo, remote),
        "pushed": not dry_run,
        "dry_run": dry_run,
        "command": command,
        "stdout": result.stdout.strip(),
        "stderr": result.stderr.strip(),
        "review_surface": review_surface,
        "remote_actions": remote_action_result(
            push=not dry_run,
            pull_request=False,
            confirmation=confirmation,
        ),
        "created_at": utc_now(),
        "after": after,
    }


def create_pull_request(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    repo = resolve_repo(root)
    head_branch = str(args.get("head_branch") or "").strip() or current_branch_name(repo)
    if not head_branch or head_branch == "HEAD":
        raise SkillError("branch_detached", "cannot create pull request from detached HEAD without explicit head_branch")
    base_branch = str(args.get("base_branch") or "").strip() or infer_default_base_branch(repo)
    title = str(args.get("title") or "").strip() or f"NAVI Programmer review for {head_branch}"
    summary = str(args.get("summary") or "").strip()
    validation_summary = str(args.get("validation_summary") or "").strip()
    review_handoff = str(args.get("review_handoff") or "").strip()
    dry_run = bool(args.get("dry_run", False))
    draft = bool(args.get("draft", True))
    changed_files = normalize_changed_files(args.get("changed_files"))
    body = build_pull_request_body(summary, validation_summary, changed_files, review_handoff, base_branch, head_branch)
    review_surface = build_review_surface(
        action="create_pull_request",
        repo=repo,
        title=title,
        summary=summary,
        validation_summary=validation_summary,
        changed_files=changed_files,
        review_handoff=review_handoff,
        branch=head_branch,
        base_branch=base_branch,
        body=body,
    )
    confirmation = require_confirmation(args, "create_pull_request", review_surface)
    if confirmation is None:
        return blocked_confirmation_result("create_pull_request", review_surface)

    gh_executable = str(args.get("gh_executable") or "gh").strip() or "gh"
    if shutil.which(gh_executable) is None and not Path(gh_executable).exists():
        raise SkillError("missing_dependency", "gh CLI is required for create_pull_request")

    command = [gh_executable, "pr", "create", "--title", title, "--body-file"]
    with tempfile.NamedTemporaryFile("w", encoding="utf-8", delete=False, suffix=".md") as handle:
        handle.write(body)
        body_file = handle.name
    try:
        command.append(body_file)
        if draft:
            command.append("--draft")
        if base_branch:
            command.extend(["--base", base_branch])
        command.extend(["--head", head_branch])
        if dry_run:
            return {
                "outcome": "dry_run",
                "repo_root": str(repo),
                "base_branch": base_branch,
                "head_branch": head_branch,
                "title": title,
                "body": body,
                "draft": draft,
                "dry_run": True,
                "command": command,
                "review_surface": review_surface,
                "remote_actions": remote_action_result(
                    push=False,
                    pull_request=False,
                    confirmation=confirmation,
                ),
            }

        result = run_command(repo, command, timeout=30)
    finally:
        try:
            os.unlink(body_file)
        except OSError:
            pass

    pr_url = extract_url(result.stdout) or extract_url(result.stderr)
    return {
        "outcome": "clean",
        "repo_root": str(repo),
        "base_branch": base_branch,
        "head_branch": head_branch,
        "title": title,
        "body": body,
        "draft": draft,
        "dry_run": False,
        "command": command,
        "stdout": result.stdout.strip(),
        "stderr": result.stderr.strip(),
        "pull_request_url": pr_url,
        "review_surface": review_surface,
        "remote_actions": remote_action_result(
            push=False,
            pull_request=True,
            confirmation=confirmation,
            pull_request_url=pr_url,
        ),
        "created_at": utc_now(),
    }


def require_confirmation(
    args: dict[str, Any],
    action: str,
    review_surface: dict[str, Any],
) -> dict[str, Any] | None:
    raw = args.get("confirmation")
    if raw is None:
        return None
    if not isinstance(raw, dict):
        raise SkillError("invalid_input", "confirmation must be an object")
    approved = bool(raw.get("approved", False))
    actor = str(raw.get("actor") or raw.get("approved_by") or "").strip()
    mode = str(raw.get("mode") or "explicit_confirmation").strip() or "explicit_confirmation"
    rationale = str(raw.get("rationale") or raw.get("reason") or "").strip()
    if not approved:
        return None
    if not actor:
        raise SkillError("invalid_input", "confirmed remote actions require confirmation.actor")
    if action == "create_pull_request" and not review_surface.get("body"):
        raise SkillError("invalid_input", "pull request review surface body is required")
    return {
        "approved": True,
        "actor": actor,
        "mode": mode,
        "rationale": rationale,
        "confirmed_at": str(raw.get("confirmed_at") or utc_now()),
    }


def blocked_confirmation_result(action: str, review_surface: dict[str, Any]) -> dict[str, Any]:
    return {
        "outcome": "blocked_requires_confirmation",
        "action": action,
        "review_surface": review_surface,
        "remote_actions": {
            "push": False,
            "pull_request": False,
            "confirmation_required": True,
            "blocked_reason": f"{action} requires explicit owner confirmation",
        },
        "next_step": f"Provide confirmation metadata before {action} can execute.",
        "created_at": utc_now(),
    }


def build_review_surface(
    *,
    action: str,
    repo: Path,
    title: str,
    summary: str,
    validation_summary: str,
    changed_files: list[str],
    review_handoff: str,
    branch: str,
    base_branch: str,
    body: str = "",
) -> dict[str, Any]:
    lines = [
        f"# {title}",
        "",
        "## Action",
        f"- Requested action: `{action}`",
        f"- Repo: `{repo}`",
        f"- Branch: `{branch}`",
    ]
    if base_branch:
        lines.append(f"- Base branch: `{base_branch}`")
    lines.extend(
        [
            "",
            "## Summary",
            summary or "- No summary provided.",
            "",
            "## Validation",
            validation_summary or "- Validation summary not provided.",
            "",
            "## Changed Files",
        ]
    )
    if changed_files:
        lines.extend(f"- `{path}`" for path in changed_files)
    else:
        lines.append("- Changed files were not supplied.")
    if review_handoff:
        lines.extend(["", "## Review Handoff", review_handoff])
    if body:
        lines.extend(["", "## Pull Request Body", body])
    return {
        "title": title,
        "summary": summary,
        "validation_summary": validation_summary,
        "changed_files": changed_files,
        "review_handoff": review_handoff,
        "markdown": "\n".join(lines),
        "body": body,
    }


def build_pull_request_body(
    summary: str,
    validation_summary: str,
    changed_files: list[str],
    review_handoff: str,
    base_branch: str,
    head_branch: str,
) -> str:
    lines = [
        "## Summary",
        summary or "- No summary provided.",
        "",
        "## Validation",
        validation_summary or "- Validation summary not provided.",
        "",
        "## Branches",
        f"- Base: `{base_branch or 'default'}`",
        f"- Head: `{head_branch}`",
        "",
        "## Changed Files",
    ]
    if changed_files:
        lines.extend(f"- `{path}`" for path in changed_files)
    else:
        lines.append("- Changed files were not supplied.")
    if review_handoff:
        lines.extend(["", "## Review Handoff", review_handoff])
    return "\n".join(lines)


def remote_action_result(
    *,
    push: bool,
    pull_request: bool,
    confirmation: dict[str, Any],
    pull_request_url: str = "",
) -> dict[str, Any]:
    return {
        "push": push,
        "pull_request": pull_request,
        "confirmation_required": True,
        "confirmation_mode": confirmation["mode"],
        "confirmed_by": confirmation["actor"],
        "confirmed_at": confirmation["confirmed_at"],
        "rationale": confirmation["rationale"],
        "pull_request_url": pull_request_url,
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
    result = run_command_optional(root, ["git", "rev-parse", "--show-toplevel"], timeout=10)
    if result.returncode != 0:
        raise SkillError("not_git_repo", f"not a git repository: {root}")
    return Path(result.stdout.strip()).resolve()


def current_branch_name(repo: Path) -> str:
    result = run_command_optional(repo, ["git", "symbolic-ref", "--quiet", "--short", "HEAD"], timeout=10)
    if result.returncode != 0:
        return "HEAD"
    return result.stdout.strip() or "HEAD"


def repo_summary(repo: Path) -> dict[str, Any]:
    return {
        "repo_root": str(repo),
        "branch": current_branch_name(repo),
        "head": git_output_optional(repo, "rev-parse", "--short", "HEAD"),
        "upstream": git_output_optional(repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"),
    }


def ensure_remote_exists(repo: Path, remote: str) -> None:
    result = run_command_optional(repo, ["git", "remote", "get-url", remote], timeout=10)
    if result.returncode != 0:
        raise SkillError("remote_not_found", f"git remote does not exist: {remote}")


def remote_url(repo: Path, remote: str) -> str:
    return git_output_optional(repo, "remote", "get-url", remote)


def infer_default_base_branch(repo: Path) -> str:
    remote_head = git_output_optional(repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
    if not remote_head or "/" not in remote_head:
        return ""
    return remote_head.rsplit("/", 1)[-1]


def normalize_changed_files(value: Any) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise SkillError("invalid_input", "changed_files must be an array")
    out: list[str] = []
    for item in value:
        if isinstance(item, str):
            text = item.strip()
        elif isinstance(item, dict):
            text = str(item.get("path") or "").strip()
        else:
            raise SkillError("invalid_input", "changed_files entries must be strings or objects with path")
        if text:
            out.append(text)
    return out


def extract_url(text: str) -> str:
    match = URL_RE.search(text or "")
    return match.group(0) if match else ""


def git_output_optional(repo: Path, *args: str) -> str:
    result = run_command_optional(repo, ["git", *args], timeout=10)
    if result.returncode != 0:
        return ""
    return result.stdout.strip()


def run_command(repo: Path, command: list[str], *, timeout: int) -> subprocess.CompletedProcess[str]:
    result = run_command_optional(repo, command, timeout=timeout)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"{command[0]} command failed"
        code = "remote_push_failed" if command[:2] == ["git", "push"] else "pull_request_failed"
        raise SkillError(code, detail)
    return result


def run_command_optional(repo: Path, command: list[str], *, timeout: int) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            command,
            cwd=str(repo),
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except FileNotFoundError as exc:
        raise SkillError("missing_dependency", str(exc))


def utc_now() -> str:
    return _dt.datetime.now(tz=_dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


if __name__ == "__main__":
    raise SystemExit(main())
