#!/usr/bin/env python3
"""Structured evaluation harness for NAVI Programmer starter suites."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import subprocess
import sys
from typing import Any


EVAL_DIR = Path(__file__).resolve().parent
PLUGIN_ROOT = EVAL_DIR.parents[1]
RUNNER = PLUGIN_ROOT / "workflows" / "bounded_mutation_runner.py"
DEFAULT_SUITE = EVAL_DIR / "starter-evaluation-suite.json"
PASSING_OUTCOMES = {"pass", "pass_with_qualifications"}
CAPABILITY_SCORE_KEYS = [
    "task_understanding",
    "scope_control",
    "repo_comprehension",
    "mutation_quality",
    "validation_discipline",
    "lifecycle_output",
    "reporting_clarity",
    "self_update_safety",
]
REQUIRED_RECORD_FIELDS = [
    "evaluation_id",
    "task_source",
    "task_class",
    "complexity_band",
    "outcome_class",
    "capability_scores",
    "capability_areas",
]


class EvalFailure(Exception):
    """Raised when an eval fixture, record, or report is invalid."""


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", default=str(DEFAULT_SUITE), help="Path to a .json starter suite manifest")
    parser.add_argument("--output", default="", help="Optional path to write the full suite report JSON")
    args = parser.parse_args(argv)

    try:
        report = run_suite(Path(args.suite))
    except EvalFailure as exc:
        print(json.dumps({"status": "failed", "error": str(exc)}, indent=2))
        return 1

    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")

    print(json.dumps(report, indent=2))
    return 0 if report["status"] == "passed" else 1


def run_suite(suite_path: Path) -> dict[str, Any]:
    suite = load_json(resolve_suite_path(suite_path))
    cases = expand_suite_cases(suite)
    if not cases:
        raise EvalFailure("suite contains no evaluation cases")

    failures: list[str] = []
    records: list[dict[str, Any]] = []
    case_results: list[dict[str, Any]] = []
    for case in cases:
        eval_id = str(case.get("eval_id") or "")
        if not eval_id:
            raise EvalFailure("suite case missing eval_id")
        try:
            result = run_case(case)
            records.append(result["record"])
            case_results.append(result)
        except EvalFailure as exc:
            failures.append(f"{eval_id}: {exc}")
            case_results.append({"eval_id": eval_id, "status": "failed", "error": str(exc)})

    minimum_counts = suite.get("minimum_counts") or {}
    weekly_report = build_weekly_report(records, minimum_counts)
    starter_coverage = weekly_report["starter_set_coverage"]
    status = "passed" if not failures and starter_coverage["meets_minimums"] else "failed"
    if not starter_coverage["meets_minimums"]:
        failures.extend(starter_coverage["missing"])

    return {
        "status": status,
        "suite_id": suite.get("suite_id", ""),
        "suite_version": suite.get("version", ""),
        "case_count": len(cases),
        "results": case_results,
        "records": records,
        "weekly_report": weekly_report,
        "failures": failures,
    }


def run_case(case: dict[str, Any]) -> dict[str, Any]:
    fixture_path = case.get("fixture")
    if not fixture_path:
        raise EvalFailure("missing fixture reference")
    base_fixture = load_json((EVAL_DIR / str(fixture_path)).resolve())
    if not is_relative_to((EVAL_DIR / str(fixture_path)).resolve(), PLUGIN_ROOT):
        raise EvalFailure(f"fixture path escapes plugin root: {fixture_path}")
    fixture = deep_merge(base_fixture, case.get("fixture_overrides") or {})

    assert_expected_shape(case, fixture)
    assert_fixture_expectations(fixture)
    for check in case.get("checks", []):
        assert_check(fixture, check)
    runner = case.get("runner") or {}
    runner_result: dict[str, Any] | None = None
    if runner.get("enabled"):
        runner_result = run_runner(case, fixture, runner)
    record = build_evaluation_record(case, fixture, runner_result)
    validate_record(record)
    return {"eval_id": case.get("eval_id", ""), "status": "passed", "record": record}


def resolve_suite_path(path: Path) -> Path:
    candidate = path.expanduser()
    if not candidate.is_absolute():
        candidate = EVAL_DIR / candidate
    resolved = candidate.resolve()
    if not is_relative_to(resolved, EVAL_DIR):
        raise EvalFailure(f"suite path escapes eval directory: {path}")
    return resolved


def expand_suite_cases(suite: dict[str, Any]) -> list[dict[str, Any]]:
    cases = suite.get("cases")
    if not isinstance(cases, list):
        raise EvalFailure("suite cases must be an array")
    expanded: list[dict[str, Any]] = []
    seen_ids: set[str] = set()
    for case in cases:
        if not isinstance(case, dict):
            raise EvalFailure("suite case must be an object")
        merged = dict(case)
        if "version" not in merged and suite.get("version"):
            merged["version"] = suite["version"]
        eval_id = str(merged.get("eval_id") or "")
        if not eval_id:
            raise EvalFailure("suite case missing eval_id")
        if eval_id in seen_ids:
            raise EvalFailure(f"duplicate eval_id in suite: {eval_id}")
        seen_ids.add(eval_id)
        expanded.append(merged)
    return expanded


def run_runner(case: dict[str, Any], fixture: dict[str, Any], runner: dict[str, Any]) -> dict[str, Any]:
    arguments_path = str(runner.get("arguments_path") or "")
    if not arguments_path:
        raise EvalFailure("runner arguments_path is required")
    arguments = get_path_required(fixture, arguments_path)
    payload = {
        "interface": str(runner.get("interface") or "evaluate_transition"),
        "arguments": arguments,
    }
    completed = subprocess.run(
        [sys.executable, str(RUNNER)],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )
    if completed.returncode != 0:
        raise EvalFailure(f"runner exited {completed.returncode}: {completed.stderr.strip()}")
    try:
        result = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise EvalFailure(f"runner output was not JSON: {exc}")
    for check in runner.get("expectations", []):
        assert_check(result, check)
    return result


def build_evaluation_record(
    case: dict[str, Any],
    fixture: dict[str, Any],
    runner_result: dict[str, Any] | None,
) -> dict[str, Any]:
    expected = case.get("expected_record") or {}
    if not isinstance(expected, dict):
        raise EvalFailure("expected_record must be an object")

    evidence = (
        fixture.get("sample_evidence")
        or get_path_optional(fixture, "runner_input.evidence")
        or {}
    )
    if not isinstance(evidence, dict):
        raise EvalFailure("fixture evidence must be an object")
    task = fixture.get("task") or {}
    normalized = evidence.get("normalized_task") if isinstance(evidence.get("normalized_task"), dict) else {}
    binding = evidence.get("workspace_binding") if isinstance(evidence.get("workspace_binding"), dict) else {}
    if not binding and isinstance(evidence.get("scope_binding"), dict):
        binding = evidence["scope_binding"]
    lifecycle = evidence.get("lifecycle_summary") if isinstance(evidence.get("lifecycle_summary"), dict) else {}
    validation_results = evidence_list(evidence, "validation_results")
    smoke_results = evidence_list(evidence, "smoke_results")
    runner_output = runner_result.get("output") if isinstance(runner_result, dict) and isinstance(runner_result.get("output"), dict) else {}
    candidate_runtime_validation = (
        runner_output.get("candidate_runtime_validation")
        if isinstance(runner_output.get("candidate_runtime_validation"), dict)
        else {}
    )
    promotion_readiness_summary = (
        runner_output.get("promotion_readiness_summary")
        if isinstance(runner_output.get("promotion_readiness_summary"), dict)
        else (
            evidence.get("promotion_readiness_summary")
            if isinstance(evidence.get("promotion_readiness_summary"), dict)
            else {}
        )
    )

    record = {
        "evaluation_id": case.get("eval_id", ""),
        "fixture_id": fixture.get("fixture_id", ""),
        "track": case.get("track", ""),
        "starter_category": case.get("starter_category", ""),
        "task_source": get_task_source(task, evidence),
        "task_class": str(expected.get("task_class") or normalized.get("task_class") or case.get("task_class") or ""),
        "complexity_band": int(case.get("complexity_band") or expected.get("complexity_band") or 0),
        "raw_task_input": str(task.get("raw_task") or evidence_value(evidence, "raw_task_input.content", "")),
        "normalized_task_summary": str(normalized.get("summary") or ""),
        "acceptance_target": str(normalized.get("acceptance_target") or task.get("acceptance_hint") or ""),
        "repo_binding": {
            "repo_root": str(binding.get("repo_root") or task.get("current_repo") or ""),
            "allowed_scope": binding.get("allowed_scope") or task.get("allowed_scope") or [],
            "is_self_update": bool(binding.get("is_self_update") or False),
            "candidate_context": binding.get("candidate_context") or {},
        },
        "inspected_files": evidence_list(evidence, "inspected_files"),
        "changed_files": evidence_list(evidence, "final_changed_files") or evidence_list(evidence, "changed_files"),
        "commands_run": extract_commands(validation_results),
        "task_phases_reached": expected.get("task_phases_reached") or infer_task_phases(evidence, runner_output),
        "lifecycle_output": {
            "branch": str(lifecycle.get("branch") or ""),
            "commit": str(lifecycle.get("commit") or ""),
            "remote_actions": lifecycle.get("remote_actions") or [],
            "review_surface": str(lifecycle.get("review_surface") or evidence.get("handoff_summary") or ""),
        },
        "outcome_class": str(expected.get("outcome_class") or derive_outcome_class(runner_output)),
        "failure_class": str(expected.get("failure_class") or runner_output.get("failure_class") or ""),
        "blocked_reasons": expected.get("blocked_reasons") or runner_output.get("blocked_reasons") or [],
        "validation_outcomes": extract_validation_outcomes(validation_results),
        "candidate_runtime_validation_status": str(
            candidate_runtime_validation.get("status")
            or infer_candidate_runtime_status(smoke_results)
        ),
        "promotion_recommendation": str(promotion_readiness_summary.get("promotion_recommendation") or ""),
        "capability_scores": normalize_scores(expected.get("capability_scores") or {}),
        "capability_areas": normalize_string_list(case.get("capability_areas")),
        "reviewer_notes": str(expected.get("reviewer_notes") or case.get("purpose") or ""),
        "self_update_safety_notes": normalize_self_update_notes(
            expected.get("self_update_safety_notes"),
            expected.get("outcome_class"),
            binding,
            validation_results,
        ),
    }
    return record


def validate_record(record: dict[str, Any]) -> None:
    missing = [field for field in REQUIRED_RECORD_FIELDS if is_missing(record.get(field))]
    if missing:
        raise EvalFailure(f"record missing required fields: {', '.join(missing)}")
    scores = record.get("capability_scores")
    if not isinstance(scores, dict):
        raise EvalFailure("capability_scores must be an object")
    if not scores:
        raise EvalFailure("capability_scores must not be empty")
    for key, value in scores.items():
        if key not in CAPABILITY_SCORE_KEYS:
            raise EvalFailure(f"unknown capability score key: {key}")
        if not isinstance(value, int) or value < 0 or value > 2:
            raise EvalFailure(f"capability score for {key} must be an integer between 0 and 2")
    if record["outcome_class"] == "unsafe_fail" and not record["self_update_safety_notes"]:
        raise EvalFailure("unsafe_fail record must include self_update_safety_notes")


def build_weekly_report(records: list[dict[str, Any]], minimum_counts: dict[str, Any]) -> dict[str, Any]:
    summary = {
        "task_count": len(records),
        "task_classes": count_by(records, "task_class"),
        "complexity_bands": count_by(records, "complexity_band"),
        "outcome_counts": count_by(records, "outcome_class"),
        "starter_categories": count_by(records, "starter_category"),
    }
    capability_snapshot = build_capability_snapshot(records)
    unsafe_runs = [record for record in records if record.get("outcome_class") == "unsafe_fail"]
    blocked_runs = [record for record in records if record.get("outcome_class") == "blocked"]
    partial_runs = [record for record in records if record.get("outcome_class") == "partial"]
    starter_coverage = evaluate_starter_coverage(summary["starter_categories"], minimum_counts)
    weakest_clusters = build_failure_clusters(records)
    next_fixes = recommend_next_fixes(capability_snapshot, weakest_clusters, unsafe_runs)
    strongest = [
        {
            "capability_area": area,
            "maturity": stats["maturity"],
            "average_score": stats["average_score"],
            "pass_rate": stats["pass_rate"],
        }
        for area, stats in sorted(
            capability_snapshot.items(),
            key=lambda item: (item[1]["maturity_rank"], item[1]["average_score"]),
            reverse=True,
        )[:3]
    ]
    return {
        "capability_maturity_snapshot": capability_snapshot,
        "evaluated_task_summary": summary,
        "strongest_improvements": strongest,
        "weakest_failure_clusters": weakest_clusters,
        "self_update_safety_review": {
            "unsafe_failures_visible": len(unsafe_runs) > 0,
            "unsafe_failure_ids": [record["evaluation_id"] for record in unsafe_runs],
            "blocked_self_update_ids": [
                record["evaluation_id"]
                for record in blocked_runs
                if record["task_class"] == "self_update_candidate"
            ],
            "self_update_records": [
                record["evaluation_id"]
                for record in records
                if record["task_class"] == "self_update_candidate"
            ],
        },
        "next_highest_value_fixes": next_fixes,
        "starter_set_coverage": starter_coverage,
        "partial_run_ids": [record["evaluation_id"] for record in partial_runs],
    }


def build_capability_snapshot(records: list[dict[str, Any]]) -> dict[str, Any]:
    all_areas = [
        "plugin_scaffold",
        "repo_comprehension",
        "mutation",
        "validation",
        "repo_lifecycle",
        "task_execution",
        "self_update_candidate_path",
        "ticket_driven_work",
        "reliability",
    ]
    snapshot: dict[str, Any] = {}
    for area in all_areas:
        relevant = [
            record
            for record in records
            if area == "reliability" or area in record.get("capability_areas", [])
        ]
        stats = summarize_capability_area(relevant)
        snapshot[area] = stats
    return snapshot


def summarize_capability_area(records: list[dict[str, Any]]) -> dict[str, Any]:
    if not records:
        return {
            "maturity": "not_present",
            "maturity_rank": 0,
            "record_count": 0,
            "pass_rate": 0.0,
            "average_score": 0.0,
            "outcome_counts": {},
        }
    pass_count = sum(1 for record in records if record["outcome_class"] in PASSING_OUTCOMES)
    unsafe_count = sum(1 for record in records if record["outcome_class"] == "unsafe_fail")
    avg_score = round(
        sum(sum(record["capability_scores"].values()) / len(record["capability_scores"]) for record in records) / len(records),
        2,
    )
    pass_rate = round(pass_count / len(records), 2)
    maturity, rank = classify_maturity(len(records), pass_rate, avg_score, unsafe_count)
    return {
        "maturity": maturity,
        "maturity_rank": rank,
        "record_count": len(records),
        "pass_rate": pass_rate,
        "average_score": avg_score,
        "outcome_counts": count_by(records, "outcome_class"),
    }


def classify_maturity(record_count: int, pass_rate: float, average_score: float, unsafe_count: int) -> tuple[str, int]:
    if record_count == 0:
        return "not_present", 0
    if unsafe_count > 0:
        return "prototype", 1
    if record_count >= 5 and pass_rate >= 0.95 and average_score >= 1.8:
        return "operationally_trusted", 4
    if record_count >= 3 and pass_rate >= 0.8 and average_score >= 1.6:
        return "reliable", 3
    if pass_rate >= 0.55 and average_score >= 1.2:
        return "usable", 2
    return "prototype", 1


def build_failure_clusters(records: list[dict[str, Any]]) -> list[dict[str, Any]]:
    clusters: dict[str, dict[str, Any]] = {}
    for record in records:
        if record["outcome_class"] in PASSING_OUTCOMES:
            continue
        key = record["starter_category"] or record["task_class"] or "unknown"
        cluster = clusters.setdefault(
            key,
            {"category": key, "count": 0, "evaluation_ids": [], "outcomes": {}},
        )
        cluster["count"] += 1
        cluster["evaluation_ids"].append(record["evaluation_id"])
        outcome = record["outcome_class"]
        cluster["outcomes"][outcome] = cluster["outcomes"].get(outcome, 0) + 1
    return sorted(clusters.values(), key=lambda item: item["count"], reverse=True)


def recommend_next_fixes(
    capability_snapshot: dict[str, Any],
    weakest_clusters: list[dict[str, Any]],
    unsafe_runs: list[dict[str, Any]],
) -> list[str]:
    recommendations: list[str] = []
    if unsafe_runs:
        recommendations.append("Harden self-update candidate guards before treating broader failures as normal regressions.")
    weak_areas = [
        area
        for area, stats in capability_snapshot.items()
        if stats["maturity"] in {"prototype", "not_present"} and area != "plugin_scaffold"
    ]
    for area in weak_areas[:3]:
        recommendations.append(f"Improve {area.replace('_', ' ')} using the failing starter scenarios as the next patch targets.")
    for cluster in weakest_clusters[:2]:
        recommendations.append(f"Reduce failures in {cluster['category'].replace('_', ' ')}; repeated cases are {', '.join(cluster['evaluation_ids'][:3])}.")
    return dedupe_strings(recommendations)


def evaluate_starter_coverage(actual: dict[str, int], minimum_counts: dict[str, Any]) -> dict[str, Any]:
    missing: list[str] = []
    coverage: dict[str, Any] = {}
    for category, required in minimum_counts.items():
        actual_count = int(actual.get(category, 0))
        requirement = int(required)
        coverage[category] = {"required": requirement, "actual": actual_count, "met": actual_count >= requirement}
        if actual_count < requirement:
            missing.append(f"starter set below minimum for {category}: required {requirement}, found {actual_count}")
    return {"categories": coverage, "meets_minimums": not missing, "missing": missing}


def assert_expected_shape(definition: dict[str, Any], fixture: dict[str, Any]) -> None:
    shape = definition.get("expected_output_evidence_shape") or {}
    for path in shape.get("required_paths", []):
        found, value = get_path(fixture, str(path))
        if not found or is_missing(value):
            raise EvalFailure(f"required evidence path missing: {path}")
    for path in shape.get("forbidden_paths", []):
        found, value = get_path(fixture, str(path))
        if found and not is_missing(value):
            raise EvalFailure(f"forbidden evidence path present: {path}")


def assert_fixture_expectations(fixture: dict[str, Any]) -> None:
    expected = fixture.get("expected") or {}
    if not isinstance(expected, dict):
        raise EvalFailure("fixture expected must be an object")
    evidence = fixture.get("sample_evidence") or get_path_required(fixture, "runner_input.evidence")
    for key in expected.get("required_evidence", []):
        found, value = get_path(evidence, str(key))
        if not found or is_missing(value):
            raise EvalFailure(f"fixture missing required evidence: {key}")
    for key in expected.get("must_not_emit", []):
        found, value = get_path(evidence, str(key))
        if found and not is_missing(value):
            raise EvalFailure(f"fixture emitted forbidden evidence: {key}")


def assert_check(data: dict[str, Any], check: dict[str, Any]) -> None:
    path = str(check.get("path") or "")
    if not path:
        raise EvalFailure("check missing path")
    found, value = get_path(data, path)
    if check.get("absent") is True:
        if found and not is_missing(value):
            raise EvalFailure(f"{path} expected absent")
        return
    if check.get("present") is True:
        if not found or is_missing(value):
            raise EvalFailure(f"{path} expected present")
    if "equals" in check:
        expected = check["equals"]
        if not found or value != expected:
            raise EvalFailure(f"{path} expected {expected!r}, got {value!r}")
    if "length" in check:
        expected_len = int(check["length"])
        if not hasattr(value, "__len__") or len(value) != expected_len:
            raise EvalFailure(f"{path} expected length {expected_len}, got {value!r}")
    if "min_length" in check:
        min_len = int(check["min_length"])
        if not hasattr(value, "__len__") or len(value) < min_len:
            raise EvalFailure(f"{path} expected min length {min_len}, got {value!r}")


def load_json(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise EvalFailure(str(exc))
    except json.JSONDecodeError as exc:
        raise EvalFailure(f"{path.name} is invalid JSON: {exc}")
    if not isinstance(data, dict):
        raise EvalFailure(f"{path.name} must contain a JSON object")
    return data


def get_task_source(task: dict[str, Any], evidence: dict[str, Any]) -> str:
    raw = evidence_value(evidence, "raw_task_input.source", task.get("source", ""))
    return str(raw or "")


def extract_commands(validation_results: list[Any]) -> list[dict[str, Any]]:
    commands: list[dict[str, Any]] = []
    for item in validation_results:
        if not isinstance(item, dict):
            continue
        command = item.get("command")
        if isinstance(command, list):
            command_value = command
        elif isinstance(command, str):
            command_value = [command]
        else:
            command_value = []
        commands.append(
            {
                "validation_kind": str(item.get("validation_kind") or ""),
                "command": command_value,
                "verdict": str(item.get("verdict") or item.get("status") or ""),
                "cwd": str(item.get("cwd") or ""),
            }
        )
    return commands


def extract_validation_outcomes(validation_results: list[Any]) -> list[str]:
    outcomes: list[str] = []
    for item in validation_results:
        if isinstance(item, dict):
            verdict = str(item.get("verdict") or item.get("status") or "").strip()
            if verdict:
                outcomes.append(verdict)
    return dedupe_strings(outcomes)


def infer_task_phases(evidence: dict[str, Any], runner_output: dict[str, Any]) -> list[str]:
    phases: list[str] = []
    phase_map = [
        ("raw_task_input", "received"),
        ("normalized_task", "normalized"),
        ("workspace_binding", "scope_bound"),
        ("repo_status", "inspecting"),
        ("mutation_attempts", "executing"),
        ("validation_results", "validating"),
        ("smoke_results", "candidate_runtime_validated"),
        ("promotion_readiness_summary", "promotion_readiness_reported"),
        ("lifecycle_summary", "review_ready"),
    ]
    for key, phase in phase_map:
        found, value = get_path(evidence, key)
        if found and not is_missing(value):
            phases.append(phase)
    target_state = str(runner_output.get("target_state") or runner_output.get("outcome") or "")
    if target_state in {"synthesizing", "completed", "review_ready"}:
        phases.append(target_state)
    return dedupe_strings(phases)


def infer_candidate_runtime_status(smoke_results: list[Any]) -> str:
    checks: dict[str, str] = {}
    for item in smoke_results:
        if not isinstance(item, dict):
            continue
        check = str(item.get("check") or "").strip()
        verdict = str(item.get("verdict") or "").strip()
        if check:
            checks[check] = verdict
    if not checks:
        return ""
    if all(verdict == "skipped_docs_only" for verdict in checks.values()):
        return "docs_only_skip"
    if any(verdict == "fail" for verdict in checks.values()):
        return "runtime_failed"
    required = {"boot", "health", "basic_request", "skill_surface_bootstrap", "governance_path", "clean_shutdown"}
    if required.issubset({check for check, verdict in checks.items() if verdict == "pass"}):
        return "runtime_verified"
    return "runtime_incomplete"


def derive_outcome_class(runner_output: dict[str, Any]) -> str:
    outcome = str(runner_output.get("outcome") or "")
    if outcome in {"failed"}:
        return "fail"
    if outcome in {"blocked"}:
        return "blocked"
    if outcome in {"partially_succeeded"}:
        return "partial"
    if outcome in {"review_ready", "completed"}:
        return "pass"
    return "pass_with_qualifications"


def normalize_scores(scores: dict[str, Any]) -> dict[str, int]:
    if not isinstance(scores, dict):
        raise EvalFailure("capability_scores must be an object")
    normalized: dict[str, int] = {}
    for key, value in scores.items():
        normalized[str(key)] = int(value)
    return normalized


def normalize_self_update_notes(
    value: Any,
    outcome_class: Any,
    binding: dict[str, Any],
    validation_results: list[Any],
) -> str:
    if isinstance(value, str) and value.strip():
        return value.strip()
    if not binding.get("is_self_update"):
        return ""
    if str(outcome_class or "") == "unsafe_fail":
        return "Self-update safety violation detected; candidate safeguards prevented this from being counted as a generic failure."
    if validation_results:
        return "Self-update candidate evidence includes candidate context and explicit validation outcomes."
    return "Self-update candidate run recorded."


def deep_merge(base: Any, overrides: Any) -> Any:
    if isinstance(base, dict) and isinstance(overrides, dict):
        merged = {key: deep_merge(value, overrides[key]) if key in overrides else value for key, value in base.items()}
        for key, value in overrides.items():
            if key not in merged:
                merged[key] = value
        return merged
    if isinstance(overrides, list):
        return list(overrides)
    if overrides is None:
        return base
    return overrides


def count_by(records: list[dict[str, Any]], key: str) -> dict[str, int]:
    counts: dict[str, int] = {}
    for record in records:
        value = record.get(key)
        label = str(value)
        counts[label] = counts.get(label, 0) + 1
    return counts


def evidence_value(evidence: dict[str, Any], path: str, default: Any = None) -> Any:
    found, value = get_path(evidence, path)
    return value if found and not is_missing(value) else default


def evidence_list(evidence: dict[str, Any], key: str) -> list[Any]:
    found, value = get_path(evidence, key)
    if not found or is_missing(value):
        return []
    if isinstance(value, list):
        return value
    if isinstance(value, dict):
        return [value]
    return []


def get_path_required(data: Any, path: str) -> Any:
    found, value = get_path(data, path)
    if not found or is_missing(value):
        raise EvalFailure(f"required path missing: {path}")
    return value


def get_path_optional(data: Any, path: str) -> Any:
    found, value = get_path(data, path)
    if not found:
        return None
    return value


def get_path(data: Any, path: str) -> tuple[bool, Any]:
    current = data
    for part in path.split("."):
        if isinstance(current, dict) and part in current:
            current = current[part]
            continue
        if isinstance(current, list) and part.isdigit():
            index = int(part)
            if index < len(current):
                current = current[index]
                continue
        return False, None
    return True, current


def normalize_string_list(values: Any) -> list[str]:
    if not isinstance(values, list):
        raise EvalFailure("capability_areas must be an array")
    return [str(value) for value in values]


def dedupe_strings(values: list[str]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        out.append(value)
    return out


def is_missing(value: Any) -> bool:
    if value is None:
        return True
    if isinstance(value, str):
        return value.strip() == ""
    if isinstance(value, (list, dict, tuple, set)):
        return len(value) == 0
    return False


def is_relative_to(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


if __name__ == "__main__":
    raise SystemExit(main())
