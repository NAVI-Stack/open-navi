#!/usr/bin/env python3
"""Bounded validation command runner for NAVI Programmer."""

from __future__ import annotations

import datetime as _dt
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import time
from typing import Any


SKILL_ID = "navi-programmer.run-validation"
DEFAULT_TIMEOUT_MS = 30000
MAX_TIMEOUT_MS = 600000
DEFAULT_MAX_OUTPUT_BYTES = 131072
MAX_OUTPUT_BYTES = 5242880
DEFAULT_SANDBOX_PROFILE = "local-validation-network-disabled"

ALLOWED_EXECUTABLES = {
    "bun",
    "cargo",
    "dotnet",
    "eslint",
    "go",
    "gradle",
    "javac",
    "jest",
    "make",
    "mvn",
    "node",
    "npm",
    "pnpm",
    "poetry",
    "py",
    "pytest",
    "python",
    "python3",
    "ruff",
    "tsc",
    "uv",
    "vitest",
    "yarn",
}
BANNED_EXECUTABLES = {
    "bash",
    "cmd",
    "fish",
    "git",
    "npx",
    "powershell",
    "pwsh",
    "sh",
    "ssh",
    "zsh",
}
PACKAGE_MANAGERS = {"bun", "npm", "pnpm", "poetry", "yarn"}
NETWORKISH_PACKAGE_ACTIONS = {"add", "audit", "ci", "dlx", "get", "global", "install", "link", "publish", "remove", "uninstall", "update", "upgrade"}
VALIDATION_TARGET_NAMES = {
    "build",
    "check",
    "ci",
    "compile",
    "e2e",
    "format",
    "fmt",
    "lint",
    "mypy",
    "pytest",
    "ruff",
    "smoke",
    "static",
    "test",
    "tsc",
    "typecheck",
    "unit",
    "validate",
    "vet",
}
ALLOWED_PYTHON_MODULES = {
    "compileall",
    "mypy",
    "py_compile",
    "pytest",
    "ruff",
    "unittest",
}
ALLOWED_GO_ACTIONS = {"build", "test", "vet"}
ALLOWED_CARGO_ACTIONS = {"build", "check", "clippy", "fmt", "test"}
SHELL_OPERATOR_TOKENS = {"|", "||", "&", "&&", ";", "<", ">", ">>", "2>", "2>>"}
SECRET_NAME_RE = re.compile(r"(secret|token|password|passwd|credential|apikey|api_key|private|key)", re.IGNORECASE)
ENV_NAME_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


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
            "run_command": run_command,
            "record_not_run": record_not_run,
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
    except Exception as exc:  # Defensive envelope: command output belongs in the structured result.
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


def run_command(args: dict[str, Any]) -> dict[str, Any]:
    root = resolve_root(args)
    cwd = resolve_workdir(root, args.get("cwd") or ".")
    command = parse_command(args.get("command"))
    normalized_executable = validate_command_policy(command)
    timeout_ms = clamp_int(args.get("timeout_ms"), DEFAULT_TIMEOUT_MS, 1000, MAX_TIMEOUT_MS)
    max_output_bytes = clamp_int(args.get("max_output_bytes"), DEFAULT_MAX_OUTPUT_BYTES, 1024, MAX_OUTPUT_BYTES)
    expected_exit_codes = parse_expected_exit_codes(args.get("expected_exit_codes"))
    validation_kind = str(args.get("validation_kind") or "other")
    sandbox_profile = str(args.get("sandbox_profile") or DEFAULT_SANDBOX_PROFILE)
    env = build_env(args)
    executable_path = resolve_executable(command[0], env)
    dry_run = bool(args.get("dry_run", False))

    base_result = {
        "root": str(root),
        "cwd": relative_path(root, cwd),
        "command": command,
        "command_display": command_display(command),
        "validation_kind": validation_kind,
        "sandbox_profile": sandbox_profile,
        "policy": {
            "shell": False,
            "normalized_executable": normalized_executable,
            "network_access": "not_requested",
            "env_passthrough": sorted(parse_env_allowlist(args.get("env_allowlist"))),
        },
    }
    if dry_run:
        return {
            **base_result,
            "verdict": "not_run",
            "reason": "dry_run",
            "would_run": True,
            "exit_code": None,
            "duration_ms": 0,
            "stdout": "",
            "stderr": "",
            "stdout_truncated": False,
            "stderr_truncated": False,
            "timed_out": False,
        }

    started_at = _dt.datetime.now(tz=_dt.timezone.utc)
    started = time.perf_counter()
    try:
        completed = subprocess.run(
            [executable_path, *command[1:]],
            cwd=str(cwd),
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout_ms / 1000,
            shell=False,
            check=False,
        )
        duration_ms = int((time.perf_counter() - started) * 1000)
        stdout, stdout_truncated = truncate_text(completed.stdout or "", max_output_bytes)
        stderr, stderr_truncated = truncate_text(completed.stderr or "", max_output_bytes)
        verdict = classify_verdict(
            completed.returncode,
            expected_exit_codes,
            stdout_truncated=stdout_truncated,
            stderr_truncated=stderr_truncated,
        )
        return {
            **base_result,
            "verdict": verdict,
            "exit_code": completed.returncode,
            "duration_ms": duration_ms,
            "stdout": stdout,
            "stderr": stderr,
            "stdout_truncated": stdout_truncated,
            "stderr_truncated": stderr_truncated,
            "timed_out": False,
            "started_at": format_time(started_at),
            "ended_at": format_time(_dt.datetime.now(tz=_dt.timezone.utc)),
            "expected_exit_codes": expected_exit_codes,
        }
    except subprocess.TimeoutExpired as exc:
        duration_ms = int((time.perf_counter() - started) * 1000)
        stdout, stdout_truncated = truncate_text(to_text(exc.stdout), max_output_bytes)
        stderr, stderr_truncated = truncate_text(to_text(exc.stderr), max_output_bytes)
        return {
            **base_result,
            "verdict": "timed_out",
            "exit_code": None,
            "duration_ms": duration_ms,
            "stdout": stdout,
            "stderr": stderr,
            "stdout_truncated": stdout_truncated,
            "stderr_truncated": stderr_truncated,
            "timed_out": True,
            "started_at": format_time(started_at),
            "ended_at": format_time(_dt.datetime.now(tz=_dt.timezone.utc)),
            "timeout_ms": timeout_ms,
        }
    except OSError as exc:
        raise SkillError("command_launch_failed", str(exc))


def record_not_run(args: dict[str, Any]) -> dict[str, Any]:
    reason = str(args.get("reason") or "").strip()
    if not reason:
        raise SkillError("invalid_input", "reason is required")
    root = resolve_root(args)
    cwd = resolve_workdir(root, args.get("cwd") or ".")
    recommended = args.get("recommended_command")
    parsed_recommended = parse_command(recommended) if recommended else []
    return {
        "root": str(root),
        "cwd": relative_path(root, cwd),
        "verdict": "not_run",
        "validation_kind": str(args.get("validation_kind") or "other"),
        "reason": reason,
        "blocked_by": str(args.get("blocked_by") or ""),
        "recommended_command": parsed_recommended,
        "duration_ms": 0,
        "exit_code": None,
        "stdout": "",
        "stderr": "",
        "stdout_truncated": False,
        "stderr_truncated": False,
        "timed_out": False,
    }


def resolve_root(args: dict[str, Any]) -> Path:
    raw = args.get("root") or args.get("repo_root") or os.environ.get("NAVI_WORKSPACE_DIR") or os.getcwd()
    root = Path(str(raw)).expanduser().resolve()
    if not root.exists():
        raise SkillError("root_not_found", f"root does not exist: {root}")
    if not root.is_dir():
        raise SkillError("root_not_directory", f"root is not a directory: {root}")
    return root


def resolve_workdir(root: Path, path_value: Any) -> Path:
    raw = str(path_value or ".")
    candidate = Path(raw).expanduser()
    if not candidate.is_absolute():
        candidate = root / candidate
    resolved = candidate.resolve()
    if not is_relative_to(resolved, root):
        raise SkillError("path_out_of_scope", f"cwd escapes root: {raw}")
    if not resolved.exists():
        raise SkillError("cwd_not_found", f"cwd does not exist: {relative_path(root, resolved)}")
    if not resolved.is_dir():
        raise SkillError("cwd_not_directory", f"cwd is not a directory: {relative_path(root, resolved)}")
    return resolved


def parse_command(value: Any) -> list[str]:
    if isinstance(value, list):
        command = [str(part) for part in value if str(part) != ""]
    elif isinstance(value, str):
        if not value.strip():
            raise SkillError("invalid_input", "command is required")
        command = shlex.split(value, posix=os.name != "nt")
    else:
        raise SkillError("invalid_input", "command must be an argv array or string")
    if not command:
        raise SkillError("invalid_input", "command is required")
    for token in command:
        if token in SHELL_OPERATOR_TOKENS:
            raise SkillError("shell_operator_rejected", f"shell operator is not allowed: {token}")
    return command


def validate_command_policy(command: list[str]) -> str:
    exe = normalize_executable(command[0])
    if exe in BANNED_EXECUTABLES:
        raise SkillError("command_not_allowed", f"executable is not allowed: {exe}")
    if exe not in ALLOWED_EXECUTABLES:
        raise SkillError("command_not_allowed", f"executable is outside validation allowlist: {exe}")
    if exe in PACKAGE_MANAGERS:
        validate_package_manager_command(exe, command)
    if exe == "make":
        validate_make_command(command)
    if exe in {"python", "python3", "py"}:
        validate_python_command(command)
    if exe == "node":
        validate_node_command(command)
    if exe == "go":
        validate_go_command(command)
    if exe == "cargo":
        validate_cargo_command(command)
    if exe == "uv":
        validate_uv_command(command)
    return exe


def validate_package_manager_command(exe: str, command: list[str]) -> None:
    action = first_non_option(command[1:])
    if action is None:
        raise SkillError("command_not_allowed", f"{exe} requires an explicit validation action")
    action = action.lower()
    if action in NETWORKISH_PACKAGE_ACTIONS:
        raise SkillError("command_not_allowed", f"{exe} {action} is not a validation command")
    if action == "run":
        script = first_non_option(command[2:])
        if script is None:
            raise SkillError("command_not_allowed", f"{exe} run requires a script name")
        if not is_validation_target(script):
            raise SkillError("command_not_allowed", f"{exe} run {script} is not an allowed validation target")
        return
    if action not in VALIDATION_TARGET_NAMES:
        raise SkillError("command_not_allowed", f"{exe} {action} is not an allowed validation target")


def validate_make_command(command: list[str]) -> None:
    targets = [part for part in command[1:] if not part.startswith("-")]
    if not targets:
        raise SkillError("command_not_allowed", "make requires an explicit validation target")
    for target in targets:
        if "=" in target:
            continue
        if not is_validation_target(target):
            raise SkillError("command_not_allowed", f"make target is not an allowed validation target: {target}")


def validate_python_command(command: list[str]) -> None:
    lowered = [part.lower() for part in command[1:]]
    if "-c" in lowered:
        raise SkillError("command_not_allowed", "python -c is not allowed by the validation runner")
    if "-m" in lowered:
        idx = lowered.index("-m")
        module = lowered[idx + 1] if idx + 1 < len(lowered) else ""
        if module in {"pip", "venv", "ensurepip"}:
            raise SkillError("command_not_allowed", f"python -m {module} is not a validation command")
        if module not in ALLOWED_PYTHON_MODULES:
            raise SkillError("command_not_allowed", f"python -m {module} is not an allowed validation module")
        return
    raise SkillError("command_not_allowed", "python requires -m with an allowed validation module")


def validate_node_command(command: list[str]) -> None:
    lowered = [part.lower() for part in command[1:]]
    if any(part in {"-e", "--eval", "-p", "--print"} for part in lowered):
        raise SkillError("command_not_allowed", "node inline evaluation is not allowed by the validation runner")
    if "--test" not in lowered:
        raise SkillError("command_not_allowed", "node is limited to the built-in --test validation path")


def validate_go_command(command: list[str]) -> None:
    action = first_non_option(command[1:])
    if action is None or action.lower() not in ALLOWED_GO_ACTIONS:
        raise SkillError("command_not_allowed", f"go {action or '<missing>'} is not an allowed validation command")


def validate_cargo_command(command: list[str]) -> None:
    action = first_non_option(command[1:])
    if action is None or action.lower() not in ALLOWED_CARGO_ACTIONS:
        raise SkillError("command_not_allowed", f"cargo {action or '<missing>'} is not an allowed validation command")


def validate_uv_command(command: list[str]) -> None:
    action = first_non_option(command[1:])
    if action and action.lower() in {"add", "build", "lock", "pip", "publish", "remove", "sync"}:
        raise SkillError("command_not_allowed", f"uv {action} is not allowed by the validation runner")
    if action and action.lower() == "run":
        target = first_non_option(command[2:])
        if target is None or not any(is_validation_target(part) for part in command[2:]):
            raise SkillError("command_not_allowed", "uv run requires a validation-shaped target")


def resolve_executable(executable: str, env: dict[str, str]) -> str:
    if any(sep in executable for sep in ("/", "\\")):
        raise SkillError("command_not_allowed", "path-style executables are not allowed; use a named validation tool")
    found = shutil.which(executable, path=env.get("PATH"))
    if found is None:
        raise SkillError("executable_not_found", f"executable not found on PATH: {executable}")
    return found


def classify_verdict(
    exit_code: int,
    expected_exit_codes: list[int],
    *,
    stdout_truncated: bool,
    stderr_truncated: bool,
) -> str:
    if exit_code not in expected_exit_codes:
        return "failed"
    if stdout_truncated or stderr_truncated:
        return "ambiguous"
    return "passed"


def build_env(args: dict[str, Any]) -> dict[str, str]:
    env: dict[str, str] = {}
    for name in base_env_names():
        value = os.environ.get(name)
        if value is not None:
            env[name] = value
    for name in parse_env_allowlist(args.get("env_allowlist")):
        if is_secret_name(name):
            raise SkillError("env_not_allowed", f"secret-like env name is not allowed: {name}")
        if name in os.environ:
            env[name] = os.environ[name]
    explicit = args.get("env") or {}
    if not isinstance(explicit, dict):
        raise SkillError("invalid_input", "env must be an object")
    for name, value in explicit.items():
        name = str(name)
        if not ENV_NAME_RE.match(name):
            raise SkillError("invalid_input", f"invalid env name: {name}")
        if is_secret_name(name):
            raise SkillError("env_not_allowed", f"secret-like env name is not allowed: {name}")
        env[name] = str(value)
    env["NAVI_VALIDATION"] = "1"
    env.setdefault("NO_COLOR", "1")
    return env


def parse_env_allowlist(value: Any) -> set[str]:
    if value is None:
        return set()
    if not isinstance(value, list):
        raise SkillError("invalid_input", "env_allowlist must be an array")
    names: set[str] = set()
    for item in value:
        name = str(item)
        if not ENV_NAME_RE.match(name):
            raise SkillError("invalid_input", f"invalid env_allowlist name: {name}")
        names.add(name)
    return names


def base_env_names() -> list[str]:
    if os.name == "nt":
        return ["PATH", "PATHEXT", "SystemRoot", "WINDIR", "TEMP", "TMP", "USERPROFILE"]
    return ["PATH", "HOME", "LANG", "LC_ALL", "TMPDIR"]


def parse_expected_exit_codes(value: Any) -> list[int]:
    if value is None:
        return [0]
    if not isinstance(value, list):
        raise SkillError("invalid_input", "expected_exit_codes must be an array")
    try:
        codes = [int(item) for item in value]
    except (TypeError, ValueError):
        raise SkillError("invalid_input", "expected_exit_codes must contain integers")
    if not codes:
        raise SkillError("invalid_input", "expected_exit_codes must not be empty")
    return codes


def first_non_option(values: list[str]) -> str | None:
    for value in values:
        if not value.startswith("-"):
            return value
    return None


def is_validation_target(value: str) -> bool:
    normalized = value.lower().replace("_", "-")
    return any(part in VALIDATION_TARGET_NAMES for part in re.split(r"[-:]", normalized))


def normalize_executable(value: str) -> str:
    name = Path(value).name.lower()
    for suffix in (".exe", ".cmd", ".bat", ".ps1"):
        if name.endswith(suffix):
            return name[: -len(suffix)]
    return name


def is_secret_name(name: str) -> bool:
    return SECRET_NAME_RE.search(name) is not None


def command_display(command: list[str]) -> str:
    return " ".join(shlex.quote(part) for part in command)


def truncate_text(value: str, max_bytes: int) -> tuple[str, bool]:
    data = value.encode("utf-8", errors="replace")
    if len(data) <= max_bytes:
        return value, False
    truncated = data[:max_bytes].decode("utf-8", errors="replace")
    return truncated, True


def to_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return str(value)


def clamp_int(value: Any, default: int, minimum: int, maximum: int) -> int:
    if value is None:
        parsed = default
    else:
        try:
            parsed = int(value)
        except (TypeError, ValueError):
            raise SkillError("invalid_input", f"expected integer, got {value!r}")
    return max(minimum, min(maximum, parsed))


def format_time(value: _dt.datetime) -> str:
    return value.replace(microsecond=0).isoformat().replace("+00:00", "Z")


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
