import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path


PROJECT_MARKERS = {
    "go.mod": "go",
    "package.json": "node",
    "pyproject.toml": "python",
    "requirements.txt": "python",
    "Cargo.toml": "rust",
    "pom.xml": "java",
    "build.gradle": "java",
    "deno.json": "deno",
}

SKIP_DIRS = {
    ".git",
    ".hg",
    ".svn",
    ".venv",
    "venv",
    "node_modules",
    "dist",
    "build",
    "target",
    "__pycache__",
}


def main() -> int:
    started = time.monotonic()
    try:
        request = json.load(sys.stdin)
        result = invoke(request, started)
        print(json.dumps({"jsonrpc": "2.0", "id": request.get("id"), "result": result}, separators=(",", ":")))
        return 0
    except Exception as exc:
        request_id = None
        try:
            request_id = locals().get("request", {}).get("id")
        except Exception:
            request_id = None
        print(json.dumps({
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -32000, "message": str(exc)},
        }, separators=(",", ":")))
        return 1


def invoke(request, started):
    params = request.get("params") or {}
    method = params.get("interface")
    workspace = safe_workspace(params.get("workspace"))
    data = params.get("input") or {}
    if method == "inspect_repo":
        output = inspect_repo(workspace, data)
    elif method == "detect_project":
        output = detect_project(workspace, data)
    elif method == "summarize_diff":
        output = summarize_diff(workspace, data)
    else:
        raise ValueError(f"unsupported interface: {method}")
    return {
        "status": "success",
        "output": output,
        "artifacts": [],
        "diff": None,
        "metrics": {"duration_ms": int((time.monotonic() - started) * 1000)},
    }


def safe_workspace(raw):
    workspace = Path(raw or os.environ.get("NAVI_WORKSPACE_DIR") or "/workspace").resolve()
    if not workspace.exists() or not workspace.is_dir():
        raise ValueError(f"workspace is unavailable: {workspace}")
    return workspace


def resolve_under(root, raw_path):
    candidate = Path(raw_path or ".")
    if not candidate.is_absolute():
        candidate = root / candidate
    resolved = candidate.resolve()
    try:
        resolved.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"path escapes workspace: {raw_path}") from exc
    return resolved


def inspect_repo(workspace, data):
    base = resolve_under(workspace, data.get("path") or ".")
    if not base.exists():
        raise ValueError(f"path does not exist: {data.get('path') or '.'}")
    max_entries = clamp_int(data.get("max_entries"), 200, 1, 2000)
    include_hidden = bool(data.get("include_hidden"))
    entries = []
    if base.is_file():
        entries.append(entry_for(workspace, base))
    else:
        for child in sorted(base.iterdir(), key=lambda item: item.name.lower()):
            if should_skip(child, include_hidden):
                continue
            entries.append(entry_for(workspace, child))
            if len(entries) >= max_entries:
                break
    project = detect_project(workspace, {"path": str(base.relative_to(workspace)) if base != workspace else "."})
    return {
        "workspace": str(workspace),
        "base_path": relpath(workspace, base),
        "entries": entries,
        "truncated": len(entries) >= max_entries,
        "project": project,
    }


def detect_project(workspace, data):
    base = resolve_under(workspace, data.get("path") or ".")
    if base.is_file():
        base = base.parent
    markers = []
    languages = set()
    for marker, language in PROJECT_MARKERS.items():
        path = find_upward(base, workspace, marker)
        if path is not None:
            markers.append({"path": relpath(workspace, path), "type": marker, "language": language})
            languages.add(language)
    package_manager = None
    for marker, manager in [("pnpm-lock.yaml", "pnpm"), ("yarn.lock", "yarn"), ("package-lock.json", "npm"), ("uv.lock", "uv"), ("poetry.lock", "poetry")]:
        if find_upward(base, workspace, marker) is not None:
            package_manager = manager
            break
    return {
        "workspace": str(workspace),
        "base_path": relpath(workspace, base),
        "languages": sorted(languages),
        "markers": markers,
        "package_manager": package_manager,
        "git": {"available": shutil.which("git") is not None, "present": (workspace / ".git").exists()},
    }


def summarize_diff(workspace, data):
    git = shutil.which("git")
    if git is None:
        return {
            "git_available": False,
            "is_git_repo": (workspace / ".git").exists(),
            "files_changed": [],
            "summary": "git is unavailable in the worker image",
            "patch": "",
            "truncated": False,
        }
    staged = bool(data.get("staged"))
    max_bytes = clamp_int(data.get("max_bytes"), 65536, 1, 1048576)
    target = resolve_under(workspace, data.get("path") or ".")
    path_arg = relpath(workspace, target)
    status_cmd = [git, "-C", str(workspace), "status", "--short", "--", path_arg]
    diff_cmd = [git, "-C", str(workspace), "diff", "--no-ext-diff"]
    if staged:
        diff_cmd.append("--staged")
    diff_cmd.extend(["--", path_arg])
    status = run_fixed(status_cmd)
    diff = run_fixed(diff_cmd)
    raw_patch = diff["stdout"]
    truncated = len(raw_patch.encode("utf-8", "replace")) > max_bytes
    patch = raw_patch.encode("utf-8", "replace")[:max_bytes].decode("utf-8", "replace") if truncated else raw_patch
    files = []
    for line in status["stdout"].splitlines():
        if not line.strip():
            continue
        files.append({"status": line[:2].strip(), "path": line[3:].strip()})
    return {
        "git_available": True,
        "is_git_repo": status["exit_code"] == 0,
        "staged": staged,
        "path_filter": path_arg,
        "files_changed": files,
        "summary": f"{len(files)} file(s) changed",
        "patch": patch,
        "truncated": truncated,
        "exit_code": diff["exit_code"],
        "stderr": diff["stderr"] or status["stderr"],
    }


def entry_for(root, path):
    stat = path.stat()
    return {
        "path": relpath(root, path),
        "name": path.name,
        "type": "dir" if path.is_dir() else "file",
        "size": stat.st_size if path.is_file() else None,
    }


def should_skip(path, include_hidden):
    if not include_hidden and path.name.startswith("."):
        return True
    return path.is_dir() and path.name in SKIP_DIRS


def find_upward(start, stop, filename):
    cur = start
    while True:
        candidate = cur / filename
        if candidate.exists():
            return candidate
        if cur == stop:
            return None
        cur = cur.parent


def run_fixed(cmd):
    completed = subprocess.run(cmd, cwd=str(Path(cmd[2]).resolve()), text=True, capture_output=True, timeout=30)
    return {"exit_code": completed.returncode, "stdout": completed.stdout, "stderr": completed.stderr}


def clamp_int(value, default, min_value, max_value):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        parsed = default
    return max(min_value, min(max_value, parsed))


def relpath(root, path):
    try:
        return str(path.relative_to(root)).replace("\\", "/") or "."
    except ValueError:
        return str(path)


if __name__ == "__main__":
    raise SystemExit(main())
