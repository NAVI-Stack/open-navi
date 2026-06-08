#!/usr/bin/env python3
"""Conservative task normalization and scope binding for NAVI Programmer."""

from __future__ import annotations

import datetime as _dt
import json
import os
from pathlib import Path
import re
import sys
import time
from typing import Any


SKILL_ID = "navi-programmer.task-normalize"

MUTATION_KEYWORDS = {
    "add",
    "build",
    "change",
    "create",
    "delete",
    "edit",
    "fix",
    "implement",
    "migrate",
    "modify",
    "refactor",
    "remove",
    "rename",
    "replace",
    "scaffold",
    "update",
    "wire",
}
READ_ONLY_KEYWORDS = {
    "analyze",
    "describe",
    "explain",
    "find",
    "identify",
    "inspect",
    "review",
    "search",
    "summarize",
    "understand",
}
VALIDATION_WORDS = {
    "build": "build",
    "compile": "build",
    "lint": "lint",
    "test": "test",
    "typecheck": "typecheck",
    "validate": "test",
    "vet": "static",
}
REMOTE_WORDS = {"deploy", "push", "publish", "release", "pull request", "pr"}
SELF_UPDATE_WORDS = {"navi", "navi programmer", "navi-programmer", "this plugin", "programmer plugin"}
BROAD_WORDS = {"everything", "entire", "complete", "all of", "whole system", "from scratch"}
PATH_RE = re.compile(r"(?P<path>[A-Za-z0-9_.\\/\-]+\.(?:go|py|ts|tsx|js|jsx|md|json|ya?ml|toml|rs|java|cs|css|html|sql))")
QUOTED_PATH_RE = re.compile(r"[`'\"](?P<path>[A-Za-z0-9_.\\/\-]+)[`'\"]")


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
            "normalize_task": normalize_task,
            "bind_scope": bind_scope,
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


def normalize_task(args: dict[str, Any]) -> dict[str, Any]:
    raw_task = str(args.get("raw_task") or "").strip()
    if not raw_task:
        raise SkillError("missing_raw_task", "raw_task is required")
    source = str(args.get("source") or "chat")
    source_id = str(args.get("source_id") or "")
    acceptance_hint = str(args.get("acceptance_hint") or "").strip()
    repo_hints = parse_string_list(args.get("repo_hints"))
    current_repo = str(args.get("current_repo") or "").strip()
    constraints = parse_string_list(args.get("constraints"))
    metadata = args.get("metadata") if isinstance(args.get("metadata"), dict) else {}
    text = raw_task.lower()

    mutation_hits = sorted(word for word in MUTATION_KEYWORDS if has_word(text, word))
    read_hits = sorted(word for word in READ_ONLY_KEYWORDS if has_word(text, word))
    remote_hits = sorted(word for word in REMOTE_WORDS if has_word(text, word))
    broad_hits = sorted(word for word in BROAD_WORDS if has_word(text, word))
    path_hints = extract_paths(raw_task)
    likely_areas = likely_affected_areas(raw_task, path_hints)
    targets_self = targets_self_update(text, repo_hints, current_repo, metadata)
    has_ticket_source = source.lower() in {"linear", "issue", "ticket", "github_issue", "jira"} or bool(source_id)

    mutative = bool(mutation_hits) and not (read_hits and not mutation_hits)
    if targets_self and mutative:
        mutation_intent = "self_update"
        task_class = "self_update_candidate"
    elif mutative:
        mutation_intent = "mutative"
        task_class = "ticket_driven_mutation" if has_ticket_source else "bounded_mutation"
    elif read_hits:
        mutation_intent = "read_only"
        task_class = "read_only_comprehension"
    else:
        mutation_intent = "ambiguous"
        task_class = "unknown"

    validation = validation_hints(raw_task, path_hints, mutation_intent)
    acceptance_target = acceptance_hint or infer_acceptance_target(task_class, mutation_intent, raw_task)
    needs_repo_binding = mutation_intent in {"mutative", "self_update"} or bool(path_hints)
    has_repo_context = bool(repo_hints or current_repo)
    ambiguity_notes: list[str] = []
    blocked_reasons: list[str] = []

    if mutation_intent == "ambiguous":
        ambiguity_notes.append("Could not confidently classify the request as read-only or mutative.")
        blocked_reasons.append("mutation_intent_ambiguous")
    if needs_repo_binding and not has_repo_context:
        ambiguity_notes.append("No repo hint or current_repo was provided for a request that appears to need repository scope.")
        blocked_reasons.append("repo_scope_missing")
    if broad_hits:
        ambiguity_notes.append("Request appears broad; decomposition and explicit scope are required before mutation.")
    if remote_hits:
        ambiguity_notes.append("Request mentions remote or release actions; those actions require confirmation or separate workflow support.")
    if mutation_intent == "self_update" and not has_repo_context:
        blocked_reasons.append("self_update_repo_context_missing")

    decomposition_needed = bool(broad_hits) or len(raw_task) > 500 or len(likely_areas) > 4
    state = "blocked" if blocked_reasons else "normalized"

    return {
        "state": state,
        "blocked": bool(blocked_reasons),
        "blocked_reasons": dedupe(blocked_reasons),
        "raw_task": {
            "source": source,
            "source_id": source_id,
            "content": raw_task,
            "received_at": utc_now(),
            "metadata": metadata,
        },
        "normalized_task": {
            "summary": summarize(raw_task),
            "task_class": task_class,
            "mutation_intent": mutation_intent,
            "self_update": targets_self,
            "acceptance_target": acceptance_target,
            "repo_binding": {
                "required": needs_repo_binding,
                "repo_hints": repo_hints,
                "current_repo": current_repo,
            },
            "likely_affected_areas": likely_areas,
            "validation_hints": validation,
            "decomposition_needed": decomposition_needed,
            "ambiguity_notes": ambiguity_notes,
            "risk_hints": risk_hints(mutation_intent, remote_hits, targets_self, broad_hits, constraints),
            "classification_evidence": {
                "mutation_keywords": mutation_hits,
                "read_only_keywords": read_hits,
                "path_hints": path_hints,
                "remote_keywords": remote_hits,
            },
        },
    }


def bind_scope(args: dict[str, Any]) -> dict[str, Any]:
    normalized = args.get("normalized_task") if isinstance(args.get("normalized_task"), dict) else {}
    raw_task = str(args.get("raw_task") or normalized.get("summary") or "")
    repo_hint = str(args.get("repo_hint") or "").strip()
    explicit_repo = str(args.get("explicit_repo") or "").strip()
    current_repo = str(args.get("current_repo") or "").strip()
    candidates = parse_candidate_repos(args.get("candidate_repos"))
    allowed_scope_input = parse_string_list(args.get("allowed_scope")) or ["."]
    require_existing = bool(args.get("require_existing_root", True))
    allow_single_default = bool(args.get("allow_single_candidate_default", False))
    require_candidate_context = bool(args.get("require_self_update_candidate_context", False))
    candidate_context = args.get("candidate_context") if isinstance(args.get("candidate_context"), dict) else {}

    blocked_reasons: list[str] = []
    ambiguity_notes: list[str] = []
    selected = select_repo(explicit_repo, current_repo, repo_hint, candidates, allow_single_default, blocked_reasons, ambiguity_notes)

    repo_root = ""
    allowed_scope: list[str] = []
    if selected:
        root_path = Path(selected["path"]).expanduser()
        if require_existing:
            if not root_path.exists():
                blocked_reasons.append("repo_not_found")
            elif not root_path.is_dir():
                blocked_reasons.append("repo_not_directory")
        repo_root = str(root_path.resolve()) if root_path.exists() else str(root_path)
        if not blocked_reasons:
            allowed_scope = resolve_allowed_scope(root_path.resolve(), allowed_scope_input, blocked_reasons)

    is_self_update = bool(selected.get("is_self_update")) if selected else infer_self_from_normalized(normalized, raw_task)
    if is_self_update and require_candidate_context and not candidate_context:
        blocked_reasons.append("candidate_context_required")
        ambiguity_notes.append("Self-update binding requires explicit candidate_context.")

    status = "blocked" if blocked_reasons else "bound"
    return {
        "state": "scope_bound" if status == "bound" else "blocked",
        "binding_status": status,
        "scope_status": status,
        "blocked": bool(blocked_reasons),
        "blocked_reasons": dedupe(blocked_reasons),
        "ambiguity_notes": ambiguity_notes,
        "workspace_binding": {
            "binding_status": status,
            "scope_status": status,
            "repo_root": repo_root,
            "allowed_scope": allowed_scope,
            "is_self_update": is_self_update,
            "candidate_context": candidate_context,
            "repo_name": selected.get("name", "") if selected else "",
            "selection_reason": selected.get("selection_reason", "") if selected else "",
        },
    }


def select_repo(
    explicit_repo: str,
    current_repo: str,
    repo_hint: str,
    candidates: list[dict[str, Any]],
    allow_single_default: bool,
    blocked_reasons: list[str],
    ambiguity_notes: list[str],
) -> dict[str, Any]:
    if explicit_repo:
        return {"name": Path(explicit_repo).name, "path": explicit_repo, "aliases": [], "is_self_update": False, "selection_reason": "explicit_repo"}
    if current_repo:
        return {"name": Path(current_repo).name, "path": current_repo, "aliases": [], "is_self_update": False, "selection_reason": "current_repo"}
    if repo_hint and candidates:
        matches = [candidate for candidate in candidates if candidate_matches(candidate, repo_hint)]
        if len(matches) == 1:
            matches[0]["selection_reason"] = "repo_hint"
            return matches[0]
        if len(matches) > 1:
            blocked_reasons.append("multiple_repo_matches")
            ambiguity_notes.append(f"Repo hint matched multiple candidates: {repo_hint}")
            return {}
        blocked_reasons.append("repo_hint_not_found")
        ambiguity_notes.append(f"Repo hint did not match candidates: {repo_hint}")
        return {}
    if len(candidates) == 1 and allow_single_default:
        candidates[0]["selection_reason"] = "single_candidate_default"
        return candidates[0]
    if candidates:
        blocked_reasons.append("repo_scope_ambiguous")
        ambiguity_notes.append("Multiple candidate repos are available, but no repo_hint or explicit_repo selected one.")
        return {}
    blocked_reasons.append("repo_scope_missing")
    ambiguity_notes.append("No repo candidate, current_repo, or explicit_repo was provided.")
    return {}


def parse_candidate_repos(value: Any) -> list[dict[str, Any]]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise SkillError("invalid_input", "candidate_repos must be an array")
    out: list[dict[str, Any]] = []
    for item in value:
        if not isinstance(item, dict):
            raise SkillError("invalid_input", "candidate_repos items must be objects")
        path = str(item.get("path") or "").strip()
        if not path:
            raise SkillError("invalid_input", "candidate repo path is required")
        out.append(
            {
                "name": str(item.get("name") or Path(path).name),
                "path": path,
                "aliases": parse_string_list(item.get("aliases")),
                "is_self_update": bool(item.get("is_self_update", False)),
            }
        )
    return out


def candidate_matches(candidate: dict[str, Any], hint: str) -> bool:
    h = hint.lower()
    names = [candidate.get("name", ""), candidate.get("path", ""), *candidate.get("aliases", [])]
    return any(h == str(name).lower() or h in str(name).lower() for name in names)


def resolve_allowed_scope(repo_root: Path, scopes: list[str], blocked_reasons: list[str]) -> list[str]:
    resolved_scopes: list[str] = []
    for raw in scopes:
        candidate = Path(raw).expanduser()
        if not candidate.is_absolute():
            candidate = repo_root / candidate
        resolved = candidate.resolve()
        if not is_relative_to(resolved, repo_root):
            blocked_reasons.append("scope_out_of_repo")
            continue
        rel = resolved.relative_to(repo_root).as_posix()
        resolved_scopes.append(rel if rel else ".")
    return resolved_scopes


def validation_hints(raw_task: str, path_hints: list[str], mutation_intent: str) -> dict[str, Any]:
    lower = raw_task.lower()
    classes = sorted({kind for word, kind in VALIDATION_WORDS.items() if has_word(lower, word)})
    extensions = {Path(path).suffix.lower() for path in path_hints}
    candidate_commands: list[list[str]] = []
    if mutation_intent == "read_only":
        return {"required": False, "classes": [], "candidate_commands": [], "not_run_reason": "read_only_task"}
    if ".go" in extensions or " go " in f" {lower} ":
        classes.extend(["test", "static"])
        candidate_commands.append(["go", "test", "./..."])
    if extensions.intersection({".ts", ".tsx", ".js", ".jsx"}):
        classes.extend(["test", "lint", "typecheck"])
        candidate_commands.extend([["npm", "test"], ["npm", "run", "lint"], ["npm", "run", "typecheck"]])
    if ".py" in extensions or "python" in lower:
        classes.extend(["test", "lint"])
        candidate_commands.extend([["python", "-m", "pytest"], ["ruff", "check", "."]])
    if extensions and extensions.issubset({".md"}):
        return {
            "required": True,
            "classes": ["other"],
            "candidate_commands": [],
            "not_run_allowed_with_reason": "docs_only_no_project_validation_path",
        }
    if mutation_intent in {"mutative", "self_update"} and not classes:
        classes.append("test")
    return {"required": mutation_intent in {"mutative", "self_update"}, "classes": dedupe(classes), "candidate_commands": candidate_commands}


def likely_affected_areas(raw_task: str, path_hints: list[str]) -> list[dict[str, str]]:
    areas: list[dict[str, str]] = []
    for path in path_hints:
        areas.append({"path": path, "reason": "explicit_path_hint"})
    lower = raw_task.lower()
    keyword_areas = {
        "docs": "documentation",
        "readme": "documentation",
        "test": "tests",
        "api": "api",
        "ui": "frontend",
        "frontend": "frontend",
        "backend": "backend",
        "database": "database",
        "auth": "auth",
        "plugin": "plugins",
        "workflow": "workflows",
        "skill": "skills",
    }
    for word, area in keyword_areas.items():
        if has_word(lower, word):
            areas.append({"path": area, "reason": f"keyword:{word}"})
    unique: list[dict[str, str]] = []
    seen: set[str] = set()
    for area in areas:
        key = area["path"]
        if key not in seen:
            seen.add(key)
            unique.append(area)
    return unique


def extract_paths(raw_task: str) -> list[str]:
    paths = [match.group("path") for match in PATH_RE.finditer(raw_task)]
    for match in QUOTED_PATH_RE.finditer(raw_task):
        value = match.group("path")
        if "/" in value or "\\" in value:
            paths.append(value)
    return dedupe(paths)


def risk_hints(
    mutation_intent: str,
    remote_hits: list[str],
    targets_self: bool,
    broad_hits: list[str],
    constraints: list[str],
) -> list[str]:
    risks: list[str] = []
    if mutation_intent in {"mutative", "self_update"}:
        risks.append("filesystem_mutation")
    if targets_self:
        risks.append("self_update_candidate")
    if remote_hits:
        risks.append("remote_action_confirmation_required")
    if broad_hits:
        risks.append("broad_scope_requires_decomposition")
    for constraint in constraints:
        if "production" in constraint.lower():
            risks.append("production_sensitive")
    return dedupe(risks)


def targets_self_update(text: str, repo_hints: list[str], current_repo: str, metadata: dict[str, Any]) -> bool:
    combined = " ".join([text, *repo_hints, current_repo, str(metadata.get("project", ""))]).lower()
    return any(word in combined for word in SELF_UPDATE_WORDS)


def infer_self_from_normalized(normalized: dict[str, Any], raw_task: str) -> bool:
    if normalized.get("mutation_intent") == "self_update" or normalized.get("task_class") == "self_update_candidate":
        return True
    return targets_self_update(raw_task.lower(), [], "", {})


def infer_acceptance_target(task_class: str, mutation_intent: str, raw_task: str) -> str:
    if task_class == "read_only_comprehension":
        return "Provide the requested repository or code explanation with cited inspected areas."
    if mutation_intent == "self_update":
        return "Produce an isolated self-update candidate with validation evidence and promotion readiness separated from task success."
    if mutation_intent == "mutative":
        return "Implement the requested scoped change, capture validation evidence, and prepare review-ready output."
    return "Clarify task intent, target repo, scope, and acceptance criteria before execution."


def summarize(raw_task: str) -> str:
    compact = " ".join(raw_task.split())
    if len(compact) <= 180:
        return compact
    return compact[:177].rstrip() + "..."


def parse_string_list(value: Any) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise SkillError("invalid_input", "expected an array of strings")
    return [str(item).strip() for item in value if str(item).strip()]


def has_word(text: str, word: str) -> bool:
    if " " in word:
        return word in text
    return re.search(rf"\b{re.escape(word)}\b", text) is not None


def dedupe(values: list[Any]) -> list[Any]:
    out: list[Any] = []
    seen: set[str] = set()
    for value in values:
        key = json.dumps(value, sort_keys=True) if isinstance(value, (dict, list)) else str(value)
        if key in seen:
            continue
        seen.add(key)
        out.append(value)
    return out


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
