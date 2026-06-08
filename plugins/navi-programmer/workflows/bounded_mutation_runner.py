#!/usr/bin/env python3
"""Local evidence-gate runner for the NAVI Programmer bounded mutation workflow."""

from __future__ import annotations

import datetime as _dt
import json
import os
from pathlib import Path
import sys
import time
from typing import Any
import uuid


RUNNER_ID = "navi-programmer.bounded-mutation-runner"
DEFAULT_CONTRACT = Path(__file__).with_name("bounded-mutation.compiled.json")
VALIDATION_REQUIRED_CONDITIONS = {
    "changed_files present",
    "code_files_changed",
    "config_or_build_files_changed",
    "tests_changed",
    "acceptance_target_requires_execution",
}
COMPLETION_STATES = {"synthesizing", "review_ready", "completed"}
NON_COMPLETED_OUTCOMES = {"blocked", "failed", "partially_succeeded"}
EVIDENCE_ALIASES = {
    "raw_task_input": ["raw_task"],
    "task_source": ["raw_task_input.source", "raw_task.source"],
    "mutation_intent": ["normalized_task.mutation_intent"],
    "acceptance_target": ["normalized_task.acceptance_target"],
    "validation_requirement": [
        "normalized_task.validation_hints",
        "normalized_task.validation_requirement",
        "validation_plan",
    ],
    "workspace_binding": ["scope_binding"],
    "allowed_scope": ["workspace_binding.allowed_scope", "scope_binding.allowed_scope"],
    "self_update_classification": [
        "workspace_binding.is_self_update",
        "scope_binding.is_self_update",
        "normalized_task.self_update",
    ],
    "candidate_context": [
        "workspace_binding.candidate_context",
        "scope_binding.candidate_context",
        "candidate_context",
    ],
    "relevant_patterns": ["relevant_patterns", "search_results"],
    "intended_file_touch_set": [
        "intended_file_touch_set",
        "execution_plan.intended_file_touch_set",
    ],
    "validation_plan": ["validation_plan", "execution_plan.validation_plan"],
    "rollback_or_recovery_notes": [
        "rollback_or_recovery_notes",
        "execution_plan.rollback_or_recovery_notes",
    ],
    "mutation_attempts": ["mutation_attempts", "mutation_result"],
    "diff_summary": ["diff_summary", "mutation_result.diff_summary"],
    "outcome_classification": ["outcome_classification", "outcome_summary.outcome"],
    "result_summary": ["result_summary", "outcome_summary.summary"],
    "residual_risks": ["residual_risks", "outcome_summary.residual_risks"],
    "final_changed_files": ["final_changed_files", "changed_files"],
    "final_diff_summary": ["final_diff_summary", "diff_summary", "lifecycle_summary.diff"],
    "validation_summary": ["validation_summary", "validation_results"],
    "handoff_summary": ["handoff_summary", "lifecycle_summary.review_surface"],
    "smoke_results": ["smoke_results", "candidate_runtime_validation.results"],
    "promotion_readiness_summary": ["promotion_readiness_summary", "self_update_promotion_summary"],
}
SMOKE_REQUIRED_CHECKS = {
    "boot",
    "health",
    "basic_request",
    "skill_surface_bootstrap",
    "governance_path",
    "clean_shutdown",
}


class RunnerError(Exception):
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
            raise RunnerError("invalid_input", "expected a JSON object payload")
        interface = str(payload.get("interface") or "")
        args = payload.get("arguments") or {}
        if not interface:
            raise RunnerError("invalid_input", "missing interface")
        if not isinstance(args, dict):
            raise RunnerError("invalid_input", "arguments must be an object")

        handlers = {
            "evaluate_transition": evaluate_transition,
            "inspect_contract": inspect_contract,
            "start_run": start_run,
            "record_step": record_step,
            "synthesize_result": synthesize_result,
        }
        handler = handlers.get(interface)
        if handler is None:
            raise RunnerError("unknown_interface", f"unsupported interface: {interface}")

        output = handler(args)
        emit("success", started, interface, output=output)
        return 0
    except RunnerError as exc:
        emit("error", started, interface, error={"type": exc.code, "message": exc.message})
        return 0
    except json.JSONDecodeError as exc:
        emit("error", started, interface, error={"type": "invalid_json", "message": str(exc)})
        return 0
    except Exception as exc:  # Defensive envelope; workflow evidence should stay structured.
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
            "runner_id": RUNNER_ID,
            "interface": interface,
            "invocation_id": os.environ.get("NAVI_WORKFLOW_INVOCATION_ID", ""),
            "python_version": ".".join(str(part) for part in sys.version_info[:3]),
        },
    }
    if output is not None:
        result["output"] = output
    if error is not None:
        result["error"] = error
    print(json.dumps(result, ensure_ascii=True, separators=(",", ":")))


def inspect_contract(args: dict[str, Any]) -> dict[str, Any]:
    contract = load_contract(args)
    states = contract.get("states", {})
    return {
        "workflow_id": contract.get("workflow_id", ""),
        "version": contract.get("version", ""),
        "source_yaml": contract.get("source_yaml", ""),
        "state_order": contract.get("state_order", []),
        "terminal_outcomes": sorted((contract.get("terminal_outcomes") or {}).keys()),
        "executable_skills": executable_skill_ids(contract),
        "state_requirements": {
            state_id: {
                "required_evidence": state.get("required_evidence", []),
                "allowed_next": state.get("allowed_next", []),
                "skills": state.get("skills", []),
            }
            for state_id, state in states.items()
        },
        "validation_gate": contract.get("validation_gate", {}),
        "governance": contract.get("governance", {}),
        "runner_interfaces": runner_interfaces(),
        "result_contract": result_contract(),
    }


def start_run(args: dict[str, Any]) -> dict[str, Any]:
    contract = load_contract(args)
    raw_task = str(args.get("raw_task") or "").strip()
    if not raw_task:
        raise RunnerError("invalid_input", "raw_task is required")
    source = str(args.get("source") or "chat")
    source_id = str(args.get("source_id") or "")
    run_id = str(args.get("run_id") or f"np-{uuid.uuid4().hex[:12]}")
    evidence = {
        "raw_task_input": {
            "source": source,
            "source_id": source_id,
            "content": raw_task,
            "received_at": utc_now(),
        }
    }
    if args.get("metadata") and isinstance(args["metadata"], dict):
        evidence["raw_task_input"]["metadata"] = args["metadata"]
    next_state = default_target_state(contract, "received")
    return {
        "programmer_run": {
            "run_id": run_id,
            "workflow_id": contract.get("workflow_id", ""),
            "workflow_version": contract.get("version", ""),
            "state": "received",
            "status": "running",
            "task_source": source,
            "task_source_id": source_id,
            "created_at": evidence["raw_task_input"]["received_at"],
        },
        "contract_path": str(resolve_contract_path(args.get("contract_path")).name),
        "evidence": evidence,
        "next_state": next_state,
        "required_next_evidence": contract.get("states", {}).get(next_state, {}).get("required_evidence", []),
        "step_order": contract.get("state_order", []),
    }


def record_step(args: dict[str, Any]) -> dict[str, Any]:
    contract = load_contract(args)
    evidence = clone_json_object(args.get("evidence") or {})
    if not isinstance(evidence, dict):
        raise RunnerError("invalid_input", "evidence must be an object")
    skill_result = args.get("skill_result") or args.get("result") or {}
    if not isinstance(skill_result, dict):
        raise RunnerError("invalid_input", "skill_result must be an object")

    metadata = skill_result.get("metadata") if isinstance(skill_result.get("metadata"), dict) else {}
    skill_id = str(args.get("skill_id") or metadata.get("skill_id") or "").strip()
    interface = str(args.get("interface") or metadata.get("interface") or "").strip()
    if not skill_id:
        raise RunnerError("invalid_input", "skill_id is required")
    if not interface:
        raise RunnerError("invalid_input", "interface is required")

    status = str(skill_result.get("status") or "success").strip()
    output = skill_result.get("output") if "output" in skill_result else skill_result
    if output is None:
        output = {}
    if not isinstance(output, dict):
        raise RunnerError("invalid_input", "skill_result.output must be an object when present")

    before_keys = set(evidence.keys())
    step_state = state_for_skill_interface(skill_id, interface)
    step_evidence = {
        "skill_id": skill_id,
        "interface": interface,
        "state": step_state,
        "status": status,
        "recorded_at": utc_now(),
    }
    if status != "success":
        error = skill_result.get("error") if isinstance(skill_result.get("error"), dict) else {}
        step_evidence.update(
            {
                "blocked": True,
                "failure_class": str(error.get("type") or "skill_error"),
                "failure_reason": str(error.get("message") or "skill returned non-success status"),
            }
        )
        append_evidence_list(evidence, "step_results", step_evidence)
        return {
            "workflow_id": contract.get("workflow_id", ""),
            "evidence": evidence,
            "step_evidence": step_evidence,
            "evidence_keys_added": [],
            "next_state_hint": next_state_after(contract, step_state),
        }

    merge_step_output(evidence, skill_id, interface, output, args)
    after_keys = set(evidence.keys())
    step_evidence["blocked"] = bool(output.get("blocked", False))
    if output.get("blocked_reasons"):
        step_evidence["blocked_reasons"] = output.get("blocked_reasons")
    append_evidence_list(evidence, "step_results", step_evidence)
    return {
        "workflow_id": contract.get("workflow_id", ""),
        "evidence": evidence,
        "step_evidence": step_evidence,
        "evidence_keys_added": sorted(after_keys - before_keys),
        "evidence_summary": summarize_evidence(evidence),
        "next_state_hint": next_state_after(contract, step_state),
    }


def synthesize_result(args: dict[str, Any]) -> dict[str, Any]:
    contract = load_contract(args)
    evidence = args.get("evidence") or {}
    if not isinstance(evidence, dict):
        raise RunnerError("invalid_input", "evidence must be an object")
    current_state = str(args.get("current_state") or args.get("state") or "")
    task_class = infer_task_class(args, evidence)
    blocked_reasons = parse_string_list(args.get("blocked_reasons"))
    validation_present, validation_path = evidence_present(evidence, "validation_results")
    verdicts = validation_verdicts(evidence)
    changed_files = evidence_list(evidence, "final_changed_files") or evidence_list(evidence, "changed_files")
    lifecycle_summary = evidence_dict(evidence, "lifecycle_summary")
    failure_class = str(args.get("failure_class") or "")

    if not failure_class and changed_files and task_class != "read_only_comprehension" and not validation_present:
        failure_class = "validation_missing"
    outcome = str(args.get("outcome") or "")
    if not outcome:
        outcome = classify_programmer_result(
            current_state=current_state,
            task_class=task_class,
            changed_files=changed_files,
            validation_present=validation_present,
            verdicts=verdicts,
            lifecycle_summary=lifecycle_summary,
            blocked_reasons=blocked_reasons,
            failure_class=failure_class,
            candidate_runtime_status=self_update_runtime_status(evidence_list(evidence, "smoke_results")),
            promotion_recommendation=str(
                synthesize_promotion_readiness_summary(
                    evidence,
                    evidence_list(evidence, "validation_results"),
                    evidence_list(evidence, "smoke_results"),
                ).get("promotion_recommendation")
                or ""
            ),
        )

    normalized = evidence_dict(evidence, "normalized_task")
    binding = evidence_dict(evidence, "workspace_binding") or evidence_dict(evidence, "scope_binding")
    raw_task_input = evidence_dict(evidence, "raw_task_input") or evidence_dict(evidence, "raw_task")
    validation_results = evidence_list(evidence, "validation_results")
    smoke_results = evidence_list(evidence, "smoke_results")
    candidate_runtime_validation = candidate_runtime_validation_summary(smoke_results)
    promotion_readiness_summary = synthesize_promotion_readiness_summary(
        evidence,
        validation_results,
        smoke_results,
    )
    runtime_status = str(candidate_runtime_validation.get("status") or "")
    verification_tier = "standard"
    if task_class == "self_update_candidate":
        if runtime_status == "runtime_verified":
            verification_tier = "runtime_verified"
        elif runtime_status == "docs_only_skip":
            verification_tier = "docs_only_skip"
        else:
            verification_tier = "locally_validated_only"
    promotion_recommendation = str(promotion_readiness_summary.get("promotion_recommendation") or "")
    return {
        "workflow_id": contract.get("workflow_id", ""),
        "workflow_version": contract.get("version", ""),
        "outcome": outcome,
        "current_state": current_state,
        "task_class": task_class,
        "task_summary": str(normalized.get("summary") or ""),
        "task_source": str(raw_task_input.get("source") or ""),
        "source_reference": {
            "source": str(raw_task_input.get("source") or ""),
            "source_id": str(raw_task_input.get("source_id") or ""),
            "metadata": raw_task_input.get("metadata") if isinstance(raw_task_input.get("metadata"), dict) else {},
        },
        "repo_root": str(binding.get("repo_root") or ""),
        "allowed_scope": binding.get("allowed_scope") or [],
        "candidate_context": binding.get("candidate_context") if isinstance(binding.get("candidate_context"), dict) else {},
        "changed_files": changed_files,
        "validation": {
            "present": validation_present,
            "path": validation_path,
            "verdicts": verdicts,
            "results": validation_results,
            "summary": validation_summary(validation_results),
        },
        "candidate_runtime_validation": candidate_runtime_validation,
        "promotion_readiness_summary": promotion_readiness_summary,
        "verification_tier": verification_tier,
        "lifecycle_summary": lifecycle_summary,
        "blocked_reasons": blocked_reasons,
        "failure_class": failure_class,
        "residual_risks": evidence_value(evidence, "residual_risks", []),
        "reviewable": bool(lifecycle_summary or changed_files),
        "next_actions": next_actions_for_result(
            outcome,
            task_class,
            failure_class,
            verdicts,
            lifecycle_summary,
            runtime_status,
            promotion_recommendation,
        ),
        "evidence_summary": summarize_evidence(evidence),
        "synthesized_at": utc_now(),
    }


def evaluate_transition(args: dict[str, Any]) -> dict[str, Any]:
    contract = load_contract(args)
    evidence = args.get("evidence") or {}
    if not isinstance(evidence, dict):
        raise RunnerError("invalid_input", "evidence must be an object")

    current_state = str(args.get("current_state") or args.get("state") or "")
    if not current_state:
        raise RunnerError("invalid_input", "current_state is required")
    target_state = str(args.get("target_state") or "")
    if not target_state:
        target_state = default_target_state(contract, current_state)

    states = contract.get("states", {})
    terminal_outcomes = contract.get("terminal_outcomes", {})
    if current_state not in states:
        raise RunnerError("unknown_state", f"unknown current_state: {current_state}")
    if target_state not in states and target_state not in terminal_outcomes:
        raise RunnerError("unknown_state", f"unknown target_state: {target_state}")

    task_class = infer_task_class(args, evidence)
    available_skills_arg = args.get("available_skills")
    available_skills = parse_available_skills(available_skills_arg)
    checks = build_transition_checks(contract, current_state, target_state, task_class, evidence)
    missing_evidence = [item for item in checks if not item["present"]]
    skill_requirements = required_skills_for_path(contract, current_state, target_state)
    missing_skills = required_missing_skills(skill_requirements, available_skills_arg, available_skills)
    gate_results = evaluate_validation_gate(contract, current_state, target_state, task_class, evidence, args)
    governance_results = evaluate_governance(contract, task_class, evidence)
    self_update_results = evaluate_self_update_gate(current_state, target_state, task_class, evidence)
    transition_allowed = target_state in states[current_state].get("allowed_next", [])

    blocked_reasons: list[str] = []
    failure_class = ""
    if not task_class_allowed(contract, task_class):
        blocked_reasons.append("task_class_not_supported")
    if not transition_allowed:
        blocked_reasons.append("invalid_transition")
    if missing_evidence:
        blocked_reasons.append("required_evidence_missing")
    if missing_skills:
        blocked_reasons.append("required_skill_missing")
    for gate in gate_results:
        if gate.get("blocked"):
            blocked_reasons.append(str(gate.get("reason") or gate.get("id") or "validation_gate_blocked"))
        if gate.get("failure_class") and not failure_class:
            failure_class = str(gate["failure_class"])
    for result in governance_results:
        if result.get("blocked"):
            blocked_reasons.append(str(result.get("reason") or result.get("id") or "governance_blocked"))
    for result in self_update_results:
        if result.get("blocked"):
            blocked_reasons.append(str(result.get("reason") or result.get("id") or "self_update_gate_blocked"))

    blocked_reasons = dedupe(blocked_reasons)
    blocked = bool(blocked_reasons)
    outcome = classify_outcome(target_state, blocked, failure_class)
    next_allowed_states = states[current_state].get("allowed_next", [])
    transition_evidence = {
        "workflow_id": contract.get("workflow_id", ""),
        "workflow_version": contract.get("version", ""),
        "from_state": current_state,
        "to_state": target_state,
        "task_class": task_class,
        "checked_at": utc_now(),
        "required_evidence_checked": checks,
        "required_skills_checked": skill_requirements,
        "validation_gate": gate_results,
        "governance": governance_results,
        "self_update_gate": self_update_results,
        "outcome": outcome,
    }

    return {
        "workflow_id": contract.get("workflow_id", ""),
        "workflow_version": contract.get("version", ""),
        "current_state": current_state,
        "target_state": target_state,
        "task_class": task_class,
        "transition_allowed_by_contract": transition_allowed,
        "blocked": blocked,
        "blocked_reasons": blocked_reasons,
        "failure_class": failure_class,
        "outcome": outcome,
        "missing_evidence": missing_evidence,
        "missing_skills": missing_skills,
        "gate_results": gate_results,
        "governance_results": governance_results,
        "self_update_results": self_update_results,
        "evidence_summary": summarize_evidence(evidence),
        "transition_evidence": transition_evidence,
        "next_allowed_states": next_allowed_states,
    }


def runner_interfaces() -> list[dict[str, Any]]:
    return [
        {
            "name": "start_run",
            "purpose": "Create the initial run envelope and raw-task evidence for a bounded programming task.",
        },
        {
            "name": "record_step",
            "purpose": "Merge one skill result into the workflow evidence ledger without invoking the skill itself.",
        },
        {
            "name": "evaluate_transition",
            "purpose": "Check whether current evidence may move from one workflow state to another.",
        },
        {
            "name": "synthesize_result",
            "purpose": "Build a normalized programming result envelope from accumulated evidence.",
        },
        {
            "name": "inspect_contract",
            "purpose": "Return workflow, evidence, skill, governance, validation, and result-contract metadata.",
        },
    ]


def result_contract() -> dict[str, Any]:
    return {
        "required_fields": [
            "workflow_id",
            "outcome",
            "current_state",
            "task_class",
            "task_summary",
            "repo_root",
            "changed_files",
            "validation",
            "lifecycle_summary",
            "blocked_reasons",
            "failure_class",
            "residual_risks",
            "reviewable",
            "next_actions",
            "candidate_runtime_validation",
            "promotion_readiness_summary",
            "verification_tier",
        ],
        "outcomes": ["completed", "review_ready", "partially_succeeded", "blocked", "failed"],
        "validation_verdicts": ["passed", "failed", "not_run", "ambiguous", "timed_out"],
    }


def clone_json_object(value: Any) -> Any:
    try:
        return json.loads(json.dumps(value))
    except (TypeError, ValueError):
        raise RunnerError("invalid_input", "value must be JSON-serializable")


def state_for_skill_interface(skill_id: str, interface: str) -> str:
    if skill_id == "navi-programmer.task-normalize":
        return "scope_bound" if interface == "bind_scope" else "normalized"
    if skill_id == "navi-programmer.repo-inspect":
        return "inspecting"
    if skill_id in {"navi-programmer.file-mutate", "navi-programmer.patch-apply"}:
        return "executing"
    if skill_id == "navi-programmer.run-validation":
        return "validating"
    if skill_id in {"navi-programmer.git-lifecycle", "navi-programmer.remote-review"}:
        return "review_ready"
    return ""


def next_state_after(contract: dict[str, Any], state: str) -> str:
    states = contract.get("states", {})
    for candidate in states.get(state, {}).get("allowed_next", []):
        if candidate in states:
            return str(candidate)
    return ""


def merge_step_output(
    evidence: dict[str, Any],
    skill_id: str,
    interface: str,
    output: dict[str, Any],
    args: dict[str, Any],
) -> None:
    if skill_id == "navi-programmer.task-normalize" and interface == "normalize_task":
        if isinstance(output.get("raw_task"), dict):
            evidence["raw_task_input"] = output["raw_task"]
        if isinstance(output.get("normalized_task"), dict):
            evidence["normalized_task"] = output["normalized_task"]
        return

    if skill_id == "navi-programmer.task-normalize" and interface == "bind_scope":
        if isinstance(output.get("workspace_binding"), dict):
            evidence["workspace_binding"] = output["workspace_binding"]
        return

    if skill_id == "navi-programmer.repo-inspect":
        merge_repo_inspection(evidence, interface, output, args)
        return

    if skill_id in {"navi-programmer.file-mutate", "navi-programmer.patch-apply"}:
        record_mutation_attempt(evidence, skill_id, interface, output)
        return

    if skill_id == "navi-programmer.run-validation":
        result = {**output, "skill_id": skill_id, "interface": interface}
        append_evidence_list(evidence, "validation_results", result)
        smoke_result = normalize_smoke_result(result, args)
        if smoke_result:
            append_evidence_list(evidence, "smoke_results", smoke_result)
        return

    if skill_id in {"navi-programmer.git-lifecycle", "navi-programmer.remote-review"}:
        merge_lifecycle_output(evidence, skill_id, interface, output)


def merge_repo_inspection(
    evidence: dict[str, Any],
    interface: str,
    output: dict[str, Any],
    args: dict[str, Any],
) -> None:
    if interface == "repo_status":
        evidence["repo_status"] = output
        return
    if interface == "inspect_diff":
        evidence["repo_diff"] = output
        if isinstance(output.get("files_changed"), list):
            append_evidence_list(
                evidence,
                "relevant_patterns",
                {
                    "path": output.get("path_filter") or ".",
                    "reason": "inspect_diff",
                    "files_changed": output.get("files_changed"),
                },
            )
        return
    if interface == "search_text":
        matches = output.get("matches") if isinstance(output.get("matches"), list) else []
        for match in matches:
            append_evidence_list(evidence, "relevant_patterns", match)
        return
    if interface == "read_file":
        line_window = output.get("line_window") if isinstance(output.get("line_window"), dict) else {}
        append_evidence_list(
            evidence,
            "inspected_files",
            {
                "path": output.get("path", ""),
                "reason": str(args.get("reason") or "read_file"),
                "content_hash_or_read_window": f"read_file:{line_window or output.get('bytes_read', '')}",
            },
        )
        return
    if interface == "list_tree":
        append_evidence_list(
            evidence,
            "inspected_files",
            {
                "path": output.get("base_path", "."),
                "reason": str(args.get("reason") or "list_tree"),
                "content_hash_or_read_window": f"entries:{len(output.get('entries', []))}",
            },
        )


def record_mutation_attempt(
    evidence: dict[str, Any],
    skill_id: str,
    interface: str,
    output: dict[str, Any],
) -> None:
    changed_files = output.get("changed_files") if isinstance(output.get("changed_files"), list) else []
    append_evidence_list(
        evidence,
        "mutation_attempts",
        {
            "skill": f"{skill_id}.{interface}",
            "status": output.get("outcome") or "success",
            "targets": [item.get("path", "") for item in changed_files if isinstance(item, dict)],
            "dry_run": bool(output.get("dry_run", False)),
        },
    )
    extend_changed_files(evidence, changed_files)
    if isinstance(output.get("diff_summary"), dict):
        evidence["diff_summary"] = merge_diff_summary(
            evidence.get("diff_summary"),
            output["diff_summary"],
        )


def merge_lifecycle_output(
    evidence: dict[str, Any],
    skill_id: str,
    interface: str,
    output: dict[str, Any],
) -> None:
    lifecycle = evidence.get("lifecycle_summary")
    if not isinstance(lifecycle, dict):
        lifecycle = {}
    lifecycle[interface] = output
    lifecycle["last_skill"] = f"{skill_id}.{interface}"
    if output.get("branch"):
        lifecycle["branch"] = output.get("branch")
    if output.get("commit"):
        lifecycle["commit"] = output.get("commit")
    if output.get("remote_actions"):
        lifecycle["remote_actions"] = output.get("remote_actions")
    if output.get("handoff_markdown"):
        lifecycle["review_surface"] = output.get("handoff_markdown")
        evidence["handoff_summary"] = output.get("handoff_markdown")
    if output.get("validation_summary"):
        evidence["validation_summary"] = output.get("validation_summary")
    status = output.get("status") if isinstance(output.get("status"), dict) else {}
    if isinstance(status.get("changed_files"), list):
        evidence["final_changed_files"] = status["changed_files"]
    if isinstance(output.get("diff"), dict):
        evidence["final_diff_summary"] = output["diff"]
    evidence["lifecycle_summary"] = lifecycle


def append_evidence_list(evidence: dict[str, Any], key: str, value: Any) -> None:
    if is_missing(value):
        return
    current = evidence.get(key)
    if not isinstance(current, list):
        current = []
    current.append(value)
    evidence[key] = dedupe(current)


def extend_changed_files(evidence: dict[str, Any], changed_files: list[Any]) -> None:
    current = evidence.get("changed_files")
    if not isinstance(current, list):
        current = []
    for item in changed_files:
        if isinstance(item, dict):
            current.append(item)
    evidence["changed_files"] = dedupe(current)


def merge_diff_summary(existing: Any, new_value: dict[str, Any]) -> dict[str, Any]:
    if not isinstance(existing, dict):
        return dict(new_value)
    merged = dict(existing)
    for key in ("additions", "insertions", "deletions", "files_changed"):
        if isinstance(new_value.get(key), int):
            merged[key] = int(merged.get(key) or 0) + int(new_value[key])
    if new_value.get("unified_diff"):
        merged["unified_diff"] = "\n".join(
            item for item in [str(merged.get("unified_diff") or ""), str(new_value["unified_diff"])] if item
        )
    if new_value.get("truncated"):
        merged["truncated"] = True
    return merged


def evidence_value(evidence: dict[str, Any], key: str, default: Any = None) -> Any:
    paths = [key, *EVIDENCE_ALIASES.get(key, [])]
    for path in paths:
        found, value = get_path(evidence, path)
        if found and not is_missing(value):
            return value
    return default


def evidence_dict(evidence: dict[str, Any], key: str) -> dict[str, Any]:
    value = evidence_value(evidence, key, {})
    return value if isinstance(value, dict) else {}


def evidence_list(evidence: dict[str, Any], key: str) -> list[Any]:
    value = evidence_value(evidence, key, [])
    if isinstance(value, list):
        return value
    if isinstance(value, dict):
        return [value]
    return []


def validation_summary(results: list[Any]) -> str:
    if not results:
        return ""
    parts: list[str] = []
    for item in results:
        if not isinstance(item, dict):
            continue
        verdict = str(item.get("verdict") or item.get("status") or "unknown")
        command = item.get("command")
        if isinstance(command, list) and command:
            command_text = " ".join(str(part) for part in command)
        else:
            command_text = str(item.get("reason") or item.get("validation_kind") or "validation")
        parts.append(f"{verdict}: {command_text}")
    return "; ".join(parts)


def normalize_smoke_result(output: dict[str, Any], args: dict[str, Any]) -> dict[str, Any] | None:
    validation_kind = str(output.get("validation_kind") or args.get("validation_kind") or "").strip()
    if validation_kind != "smoke":
        return None
    check_name = str(args.get("smoke_check") or output.get("smoke_check") or "smoke").strip() or "smoke"
    verdict = str(output.get("verdict") or output.get("status") or "").strip()
    if verdict == "passed":
        smoke_verdict = "pass"
    elif verdict in {"failed", "timed_out", "ambiguous"}:
        smoke_verdict = "fail"
    elif verdict == "not_run" and str(output.get("blocked_by") or "").strip() == "docs_only":
        smoke_verdict = "skipped_docs_only"
    else:
        smoke_verdict = verdict or "unknown"
    smoke_result: dict[str, Any] = {
        "check": check_name,
        "verdict": smoke_verdict,
        "validation_verdict": verdict,
        "command": output.get("command") or [],
        "cwd": output.get("cwd") or "",
        "duration_ms": output.get("duration_ms") or 0,
        "reason": str(output.get("reason") or output.get("stderr") or "").strip(),
    }
    candidate_runtime = args.get("candidate_runtime") or output.get("candidate_runtime")
    if isinstance(candidate_runtime, dict) and candidate_runtime:
        smoke_result["candidate_runtime"] = candidate_runtime
    return smoke_result


def self_update_runtime_status(smoke_results: list[Any]) -> str:
    checks: dict[str, str] = {}
    saw_non_docs_skip = False
    for item in smoke_results:
        if not isinstance(item, dict):
            continue
        check = str(item.get("check") or "").strip()
        verdict = str(item.get("verdict") or "").strip()
        if check:
            checks[check] = verdict
        if verdict not in {"", "skipped_docs_only"}:
            saw_non_docs_skip = True
    if not checks:
        return "runtime_missing"
    if all(verdict == "skipped_docs_only" for verdict in checks.values()):
        return "docs_only_skip"
    if any(verdict == "fail" for verdict in checks.values()):
        return "runtime_failed"
    if SMOKE_REQUIRED_CHECKS.issubset({check for check, verdict in checks.items() if verdict == "pass"}):
        return "runtime_verified"
    if saw_non_docs_skip:
        return "runtime_incomplete"
    return "runtime_missing"


def synthesize_promotion_readiness_summary(
    evidence: dict[str, Any],
    validation_results: list[Any],
    smoke_results: list[Any],
) -> dict[str, Any]:
    existing = evidence_dict(evidence, "promotion_readiness_summary")
    if existing:
        return existing
    validation_verdict_list = validation_verdicts(evidence)
    runtime_status = self_update_runtime_status(smoke_results)
    residual_risks = evidence_value(evidence, "residual_risks", [])
    unresolved_risks = list(residual_risks) if isinstance(residual_risks, list) else []
    skipped_layers_have_reasons = True
    if "not_run" in validation_verdict_list:
        skipped_layers_have_reasons = validation_not_run_has_reason(evidence)
    if runtime_status == "runtime_missing":
        unresolved_risks.append("candidate runtime smoke not recorded")
    elif runtime_status == "runtime_incomplete":
        unresolved_risks.append("candidate runtime smoke is incomplete")
    elif runtime_status == "runtime_failed":
        unresolved_risks.append("candidate runtime smoke did not pass cleanly")

    any_validation_failed = any(verdict in {"failed", "timed_out", "ambiguous"} for verdict in validation_verdict_list)
    if runtime_status == "runtime_failed":
        any_validation_failed = True

    candidate_runtime_passed: bool | str
    if runtime_status == "runtime_verified":
        candidate_runtime_passed = True
    elif runtime_status == "docs_only_skip":
        candidate_runtime_passed = "not_applicable"
    else:
        candidate_runtime_passed = False

    if any_validation_failed or runtime_status == "runtime_failed":
        recommendation = "not_recommended"
    elif runtime_status == "runtime_verified" and not unresolved_risks:
        recommendation = "recommended"
    else:
        recommendation = "needs_review"

    return {
        "all_validation_layers_ran": bool(validation_results),
        "skipped_layers_have_reasons": skipped_layers_have_reasons,
        "any_validation_failed": any_validation_failed,
        "candidate_runtime_passed": candidate_runtime_passed,
        "unresolved_risks": dedupe(unresolved_risks),
        "promotion_recommendation": recommendation,
    }


def candidate_runtime_validation_summary(smoke_results: list[Any]) -> dict[str, Any]:
    status = self_update_runtime_status(smoke_results)
    return {
        "status": status,
        "results": smoke_results,
        "required_checks": sorted(SMOKE_REQUIRED_CHECKS),
        "runtime_verified": status == "runtime_verified",
        "docs_only_skip": status == "docs_only_skip",
    }


def classify_programmer_result(
    *,
    current_state: str,
    task_class: str,
    changed_files: list[Any],
    validation_present: bool,
    verdicts: list[str],
    lifecycle_summary: dict[str, Any],
    blocked_reasons: list[str],
    failure_class: str,
    candidate_runtime_status: str = "",
    promotion_recommendation: str = "",
) -> str:
    if failure_class:
        return "failed"
    if blocked_reasons:
        return "blocked"
    if task_class == "read_only_comprehension":
        return "completed" if current_state in {"synthesizing", "completed", "review_ready"} else current_state or "synthesizing"
    if changed_files and not validation_present:
        return "failed"
    if task_class == "self_update_candidate":
        if any(verdict in verdicts for verdict in ("failed", "timed_out", "ambiguous")) or candidate_runtime_status == "runtime_failed":
            return "partially_succeeded"
        if current_state == "completed" and promotion_recommendation in {"recommended", "needs_review"} and candidate_runtime_status in {"runtime_verified", "docs_only_skip"}:
            return "completed"
        if lifecycle_summary:
            return "review_ready"
        return current_state or "synthesizing"
    if any(verdict in verdicts for verdict in ("failed", "timed_out", "ambiguous")):
        return "partially_succeeded"
    if current_state == "completed":
        return "completed"
    if current_state == "review_ready" and lifecycle_summary:
        return "completed"
    if lifecycle_summary:
        return "review_ready"
    return current_state or "synthesizing"


def next_actions_for_result(
    outcome: str,
    task_class: str,
    failure_class: str,
    verdicts: list[str],
    lifecycle_summary: dict[str, Any],
    candidate_runtime_status: str,
    promotion_recommendation: str,
) -> list[str]:
    if task_class == "self_update_candidate":
        if candidate_runtime_status == "runtime_missing":
            return ["run isolated candidate runtime smoke", "do not treat local validation as promotion readiness"]
        if candidate_runtime_status == "runtime_incomplete":
            return ["finish the remaining candidate runtime smoke checks", "update the promotion readiness summary"]
        if candidate_runtime_status == "runtime_failed":
            return ["inspect candidate runtime smoke failures", "do not promote this candidate"]
        if promotion_recommendation == "not_recommended":
            return ["preserve the candidate evidence", "do not promote this candidate"]
        if promotion_recommendation == "needs_review":
            return ["review promotion readiness caveats", "confirm before any remote action"]
    if outcome == "completed":
        return ["review final handoff evidence"]
    if outcome == "review_ready":
        return ["review local branch and commit", "confirm before any remote action"]
    if outcome == "blocked":
        return ["resolve the blocked reason, then rerun transition evaluation"]
    if failure_class == "validation_missing":
        return ["run validation or record an explicit not_run reason"]
    if any(verdict in verdicts for verdict in ("failed", "timed_out", "ambiguous")):
        return ["inspect validation output", "fix or preserve partial work with clear risk notes"]
    if not lifecycle_summary:
        return ["prepare review handoff when evidence is complete"]
    return ["inspect residual risks"]


def load_contract(args: dict[str, Any]) -> dict[str, Any]:
    path = resolve_contract_path(args.get("contract_path"))
    try:
        contract = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise RunnerError("contract_unreadable", str(exc))
    except json.JSONDecodeError as exc:
        raise RunnerError("contract_invalid_json", str(exc))
    if not isinstance(contract, dict):
        raise RunnerError("contract_invalid", "compiled contract must be an object")
    required = ["workflow_id", "version", "states", "state_order", "terminal_outcomes"]
    missing = [key for key in required if key not in contract]
    if missing:
        raise RunnerError("contract_invalid", f"compiled contract missing keys: {', '.join(missing)}")
    return contract


def resolve_contract_path(value: Any) -> Path:
    if value in (None, ""):
        return DEFAULT_CONTRACT
    raw = Path(str(value)).expanduser()
    if not raw.is_absolute():
        raw = Path(__file__).parent / raw
    resolved = raw.resolve()
    workflow_dir = Path(__file__).parent.resolve()
    if not is_relative_to(resolved, workflow_dir):
        raise RunnerError("contract_out_of_scope", f"contract path escapes workflow directory: {value}")
    return resolved


def default_target_state(contract: dict[str, Any], current_state: str) -> str:
    states = contract.get("states", {})
    allowed = states.get(current_state, {}).get("allowed_next", [])
    if not allowed:
        raise RunnerError("invalid_input", "target_state is required because current_state has no allowed_next")
    return str(allowed[0])


def infer_task_class(args: dict[str, Any], evidence: dict[str, Any]) -> str:
    explicit = str(args.get("task_class") or "").strip()
    if explicit:
        return explicit
    found, value = get_path(evidence, "normalized_task.task_class")
    if found and not is_missing(value):
        return str(value)
    return "bounded_mutation"


def build_transition_checks(
    contract: dict[str, Any],
    current_state: str,
    target_state: str,
    task_class: str,
    evidence: dict[str, Any],
) -> list[dict[str, Any]]:
    required = cumulative_required_evidence(contract, current_state)
    if target_state in contract.get("terminal_outcomes", {}) and current_state == "review_ready":
        required.extend(contract.get("states", {}).get("review_ready", {}).get("required_evidence", []))
    if task_class == "self_update_candidate" and target_state == "completed":
        required.extend(["smoke_results", "promotion_readiness_summary"])
    required = dedupe(required)
    checks = []
    for key in required:
        present, path = evidence_present(evidence, key)
        checks.append({"name": key, "present": present, "path": path})
    return checks


def cumulative_required_evidence(contract: dict[str, Any], current_state: str) -> list[str]:
    states = contract.get("states", {})
    ordered = contract.get("state_order", [])
    if current_state not in ordered:
        return states.get(current_state, {}).get("required_evidence", [])
    current_index = ordered.index(current_state)
    required: list[str] = []
    for state_id in ordered[: current_index + 1]:
        required.extend(states.get(state_id, {}).get("required_evidence", []))
    return required


def required_skills_for_path(
    contract: dict[str, Any],
    current_state: str,
    target_state: str,
) -> list[str]:
    states = contract.get("states", {})
    ordered = contract.get("state_order", [])
    if current_state not in ordered:
        state_ids = [current_state]
    else:
        current_index = ordered.index(current_state)
        state_ids = ordered[: current_index + 1]
    if target_state in ordered:
        target_index = ordered.index(target_state)
        state_ids = ordered[: max(len(state_ids), target_index + 1)]
    skills: list[str] = []
    for state_id in state_ids:
        for skill in states.get(state_id, {}).get("skills", []):
            skills.append(skill_id_from_ref(str(skill)))
    return dedupe(skills)


def skill_id_from_ref(value: str) -> str:
    parts = value.split(".")
    if len(parts) <= 2:
        return value
    return ".".join(parts[:-1])


def required_missing_skills(
    required_skills: list[str],
    available_skills_arg: Any,
    available_skills: set[str],
) -> list[str]:
    if not required_skills:
        return []
    if available_skills_arg is None:
        return required_skills
    return [skill for skill in required_skills if skill not in available_skills]


def parse_available_skills(value: Any) -> set[str]:
    if value is None:
        return set()
    if not isinstance(value, list):
        raise RunnerError("invalid_input", "available_skills must be an array")
    skills: set[str] = set()
    for item in value:
        ref = str(item)
        skills.add(ref)
        skills.add(skill_id_from_ref(ref))
    return skills


def evaluate_validation_gate(
    contract: dict[str, Any],
    current_state: str,
    target_state: str,
    task_class: str,
    evidence: dict[str, Any],
    args: dict[str, Any],
) -> list[dict[str, Any]]:
    gate = contract.get("validation_gate", {})
    gate_id = "validation_evidence_required"
    required = validation_required(task_class, evidence, args)
    validation_present, validation_path = evidence_present(evidence, "validation_results")
    verdicts = validation_verdicts(evidence)
    leaving_validation = current_state == "validating" and target_state not in {"blocked", "failed"}
    entering_completion = target_state in COMPLETION_STATES
    results: list[dict[str, Any]] = [
        {
            "id": gate_id,
            "required": required,
            "present": validation_present,
            "path": validation_path,
            "verdicts": verdicts,
            "blocked": False,
            "effect": "not_required" if not required else "pending",
        }
    ]
    result = results[0]
    if not required:
        return results
    if (leaving_validation or entering_completion) and not validation_present:
        violation = gate.get("violation", {})
        result.update(
            {
                "blocked": True,
                "reason": "validation_missing",
                "effect": "failed",
                "failure_class": violation.get("failure_class", "validation_missing"),
            }
        )
        return results

    if not validation_present:
        result["effect"] = "required_before_completion"
        return results

    if "not_run" in verdicts and not validation_not_run_has_reason(evidence):
        result.update(
            {
                "blocked": True,
                "reason": "validation_not_run_reason_missing",
                "effect": "blocked",
            }
        )
        return results
    if target_state == "completed" and any(verdict in verdicts for verdict in ("failed", "timed_out")):
        result.update(
            {
                "blocked": True,
                "reason": "validation_not_clean_for_completion",
                "effect": "route_to_partially_succeeded_or_failed",
            }
        )
        return results
    if "passed" in verdicts:
        result["effect"] = "eligible_for_completed"
    elif "not_run" in verdicts:
        result["effect"] = "eligible_only_with_explicit_reason"
    elif "failed" in verdicts:
        result["effect"] = "route_to_partially_succeeded_or_failed"
    elif "timed_out" in verdicts:
        result["effect"] = "route_to_partially_succeeded_or_blocked"
    else:
        result["effect"] = "validation_verdict_unknown"
    return results


def validation_required(task_class: str, evidence: dict[str, Any], args: dict[str, Any]) -> bool:
    if task_class == "read_only_comprehension":
        return False
    if has_changed_files(evidence):
        return True
    conditions = parse_string_list(args.get("conditions"))
    if any(condition in VALIDATION_REQUIRED_CONDITIONS for condition in conditions):
        return True
    found, hints = get_path(evidence, "normalized_task.validation_hints.required")
    if found and hints is True:
        return True
    return False


def has_changed_files(evidence: dict[str, Any]) -> bool:
    for key in ("changed_files", "final_changed_files"):
        found, value = get_path(evidence, key)
        if found and not is_missing(value):
            return True
    return False


def validation_verdicts(evidence: dict[str, Any]) -> list[str]:
    found, value = get_path(evidence, "validation_results")
    if not found or is_missing(value):
        found, value = get_path(evidence, "validation_summary")
    if not found or is_missing(value):
        return []
    if isinstance(value, dict):
        value = [value]
    if not isinstance(value, list):
        return []
    verdicts: list[str] = []
    for item in value:
        if isinstance(item, dict):
            verdict = str(item.get("verdict") or item.get("status") or "").strip()
            if verdict:
                verdicts.append(verdict)
    return dedupe(verdicts)


def validation_not_run_has_reason(evidence: dict[str, Any]) -> bool:
    found, value = get_path(evidence, "validation_results")
    if not found or is_missing(value):
        found, value = get_path(evidence, "validation_summary")
    if isinstance(value, dict):
        value = [value]
    if not isinstance(value, list):
        return False
    for item in value:
        if not isinstance(item, dict):
            continue
        verdict = str(item.get("verdict") or item.get("status") or "")
        if verdict == "not_run" and str(item.get("reason") or "").strip():
            return True
    return False


def evaluate_governance(contract: dict[str, Any], task_class: str, evidence: dict[str, Any]) -> list[dict[str, Any]]:
    governance = contract.get("governance", {})
    results: list[dict[str, Any]] = []
    if governance.get("mutation_requires_workspace_binding") and task_class != "read_only_comprehension":
        binding_present, binding_path = evidence_present(evidence, "workspace_binding")
        scope_present, scope_path = evidence_present(evidence, "allowed_scope")
        results.append(
            {
                "id": "mutation_requires_workspace_binding",
                "blocked": not (binding_present and scope_present),
                "reason": "" if binding_present and scope_present else "workspace_binding_missing",
                "paths": [binding_path, scope_path],
            }
        )
    if governance.get("self_update_requires_candidate_context") and task_class == "self_update_candidate":
        found, is_self_update = get_path(evidence, "workspace_binding.is_self_update")
        context_present, context_path = evidence_present(evidence, "workspace_binding.candidate_context")
        required = bool(found and is_self_update)
        results.append(
            {
                "id": "self_update_requires_candidate_context",
                "required": required,
                "blocked": required and not context_present,
                "reason": "" if not required or context_present else "self_update_candidate_context_missing",
                "paths": [context_path],
            }
        )
    return results


def evaluate_self_update_gate(
    current_state: str,
    target_state: str,
    task_class: str,
    evidence: dict[str, Any],
) -> list[dict[str, Any]]:
    if task_class != "self_update_candidate" or target_state != "completed":
        return []
    smoke_results = evidence_list(evidence, "smoke_results")
    promotion_summary = synthesize_promotion_readiness_summary(
        evidence,
        evidence_list(evidence, "validation_results"),
        smoke_results,
    )
    runtime_status = self_update_runtime_status(smoke_results)
    recommendation = str(promotion_summary.get("promotion_recommendation") or "")
    return [
        {
            "id": "self_update_smoke_results_required",
            "blocked": not bool(smoke_results),
            "reason": "" if smoke_results else "self_update_smoke_results_missing",
            "paths": ["smoke_results"],
        },
        {
            "id": "self_update_runtime_must_be_verified_or_docs_only",
            "blocked": runtime_status not in {"runtime_verified", "docs_only_skip"},
            "reason": "" if runtime_status in {"runtime_verified", "docs_only_skip"} else "candidate_runtime_not_verified",
            "status": runtime_status,
        },
        {
            "id": "self_update_promotion_readiness_required",
            "blocked": recommendation not in {"recommended", "needs_review"},
            "reason": "" if recommendation in {"recommended", "needs_review"} else "promotion_not_recommended",
            "promotion_recommendation": recommendation,
        },
    ]


def task_class_allowed(contract: dict[str, Any], task_class: str) -> bool:
    task_config = contract.get("task_class", {})
    includes = set(task_config.get("includes", []))
    excludes = set(task_config.get("excludes", []))
    if task_class in excludes:
        return False
    return not includes or task_class in includes


def classify_outcome(target_state: str, blocked: bool, failure_class: str) -> str:
    if not blocked:
        return target_state
    if failure_class:
        return "failed"
    if target_state in NON_COMPLETED_OUTCOMES:
        return target_state
    return "blocked"


def evidence_present(evidence: dict[str, Any], key: str) -> tuple[bool, str]:
    paths = [key, *EVIDENCE_ALIASES.get(key, [])]
    for path in paths:
        found, value = get_path(evidence, path)
        if found and not is_missing(value):
            return True, path
    return False, ""


def get_path(data: Any, path: str) -> tuple[bool, Any]:
    current = data
    for part in path.split("."):
        if isinstance(current, dict) and part in current:
            current = current[part]
            continue
        return False, None
    return True, current


def is_missing(value: Any) -> bool:
    if value is None:
        return True
    if isinstance(value, str):
        return value.strip() == ""
    if isinstance(value, (list, tuple, set, dict)):
        return len(value) == 0
    return False


def summarize_evidence(evidence: dict[str, Any]) -> dict[str, Any]:
    keys = sorted(evidence.keys())
    changed = has_changed_files(evidence)
    validation_present, _ = evidence_present(evidence, "validation_results")
    binding_present, _ = evidence_present(evidence, "workspace_binding")
    smoke_results = evidence_list(evidence, "smoke_results")
    return {
        "top_level_keys": keys,
        "changed_files_present": changed,
        "validation_results_present": validation_present,
        "workspace_binding_present": binding_present,
        "validation_verdicts": validation_verdicts(evidence),
        "smoke_results_present": bool(smoke_results),
        "candidate_runtime_status": self_update_runtime_status(smoke_results),
    }


def executable_skill_ids(contract: dict[str, Any]) -> list[str]:
    skills = contract.get("skill_contracts", {}).get("executable_now", [])
    return [str(item.get("skill_id")) for item in skills if isinstance(item, dict) and item.get("skill_id")]


def parse_string_list(value: Any) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise RunnerError("invalid_input", "conditions must be an array")
    return [str(item) for item in value]


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


def utc_now() -> str:
    return _dt.datetime.now(tz=_dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def is_relative_to(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


if __name__ == "__main__":
    raise SystemExit(main())
