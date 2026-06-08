import importlib.util
import json
import unittest
from pathlib import Path


PLUGIN_ROOT = Path(__file__).resolve().parents[1]
RUNNER_PATH = PLUGIN_ROOT / "workflows" / "bounded_mutation_runner.py"
SPEC = importlib.util.spec_from_file_location("bounded_mutation_runner", RUNNER_PATH)
runner = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(runner)


def read_declared_skill_components() -> dict[str, str]:
    refs: dict[str, str] = {}
    current_path = ""
    for raw_line in (PLUGIN_ROOT / "plugin.yaml").read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line.startswith("- path: "):
            current_path = line.split(":", 1)[1].strip()
            continue
        if current_path and line.startswith("skill_id: "):
            refs[current_path] = line.split(":", 1)[1].strip().strip('"')
            current_path = ""
    return refs


def read_skill_yaml_id(skill_dir: Path) -> str:
    for raw_line in (skill_dir / "SKILL.yaml").read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line.startswith("skill_id:"):
            return line.split(":", 1)[1].strip().strip('"')
    raise AssertionError(f"missing skill_id in {skill_dir / 'SKILL.yaml'}")


class ManifestAndRunnerContractTests(unittest.TestCase):
    def test_manifest_skill_ids_match_skill_yaml(self) -> None:
        refs = read_declared_skill_components()
        self.assertGreaterEqual(len(refs), 7)
        for component_path, declared_id in refs.items():
            actual_id = read_skill_yaml_id(PLUGIN_ROOT / component_path)
            self.assertEqual(
                declared_id,
                actual_id,
                f"{component_path} manifest skill_id must match SKILL.yaml",
            )
            self.assertTrue(
                actual_id.startswith("navi-programmer."),
                f"{actual_id} should use the plugin skill-id namespace",
            )

    def test_compiled_contract_keeps_repo_inspect_interface_set_current(self) -> None:
        contract = json.loads(
            (PLUGIN_ROOT / "workflows" / "bounded-mutation.compiled.json").read_text(
                encoding="utf-8"
            )
        )
        repo_contract = next(
            item
            for item in contract["skill_contracts"]["executable_now"]
            if item["skill_id"] == "navi-programmer.repo-inspect"
        )
        self.assertIn("inspect_diff", repo_contract["interfaces"])
        self.assertIn(
            "navi-programmer.repo-inspect.inspect_diff",
            contract["states"]["inspecting"]["skills"],
        )

    def test_ticket_driven_contract_is_compiled_for_ticket_only_execution(self) -> None:
        contract = json.loads(
            (PLUGIN_ROOT / "workflows" / "ticket-driven-coding.compiled.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(contract["workflow_id"], "navi.programmer.ticket_driven_coding")
        self.assertEqual(contract["source_yaml"], "ticket-driven-coding.yaml")
        self.assertEqual(contract["task_class"]["includes"], ["ticket_driven_mutation"])

    def test_self_update_contract_is_compiled_for_candidate_execution(self) -> None:
        contract = json.loads(
            (PLUGIN_ROOT / "workflows" / "self-update-candidate.compiled.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertEqual(contract["workflow_id"], "navi.programmer.self_update_candidate")
        self.assertEqual(contract["source_yaml"], "self-update-candidate.yaml")
        self.assertEqual(contract["task_class"]["includes"], ["self_update_candidate"])

    def test_runner_records_skill_step_into_evidence_ledger(self) -> None:
        result = runner.record_step(
            {
                "skill_id": "navi-programmer.file-mutate",
                "interface": "write_file",
                "skill_result": {
                    "status": "success",
                    "output": {
                        "outcome": "clean",
                        "changed_files": [
                            {
                                "path": "docs/example.md",
                                "action": "replace",
                                "summary": "Updated docs example.",
                            }
                        ],
                        "diff_summary": {"additions": 1, "deletions": 0},
                    },
                },
                "evidence": {},
            }
        )
        self.assertEqual(result["step_evidence"]["state"], "executing")
        self.assertEqual(result["evidence"]["changed_files"][0]["path"], "docs/example.md")
        self.assertEqual(result["evidence"]["diff_summary"]["additions"], 1)
        self.assertIn("mutation_attempts", result["evidence"])

    def test_runner_synthesizes_failed_result_when_validation_missing(self) -> None:
        result = runner.synthesize_result(
            {
                "current_state": "validating",
                "task_class": "bounded_mutation",
                "evidence": {
                    "normalized_task": {
                        "summary": "Update docs example.",
                        "task_class": "bounded_mutation",
                    },
                    "workspace_binding": {
                        "repo_root": str(PLUGIN_ROOT),
                        "allowed_scope": ["docs"],
                    },
                    "changed_files": [{"path": "docs/example.md"}],
                },
            }
        )
        self.assertEqual(result["outcome"], "failed")
        self.assertEqual(result["failure_class"], "validation_missing")
        self.assertIn("run validation", " ".join(result["next_actions"]))

    def test_runner_records_smoke_results_for_self_update_validation(self) -> None:
        result = runner.record_step(
            {
                "skill_id": "navi-programmer.run-validation",
                "interface": "run_command",
                "skill_result": {
                    "status": "success",
                    "output": {
                        "validation_kind": "smoke",
                        "verdict": "passed",
                        "command": ["go", "test", "./cmd/navid"],
                        "cwd": ".candidate/navi-programmer",
                        "duration_ms": 500,
                    },
                },
                "candidate_runtime": {
                    "port": 6384,
                    "config_dir": ".candidate/navi-programmer/.navi-config",
                    "state_dir": ".candidate/navi-programmer/.navi-state",
                    "connectors_mode": "disabled",
                },
                "smoke_check": "boot",
                "evidence": {},
            }
        )
        smoke_result = result["evidence"]["smoke_results"][0]
        self.assertEqual(smoke_result["check"], "boot")
        self.assertEqual(smoke_result["verdict"], "pass")
        self.assertEqual(smoke_result["candidate_runtime"]["port"], 6384)

    def test_runner_blocks_self_update_completion_without_runtime_smoke(self) -> None:
        result = runner.evaluate_transition(
            {
                "current_state": "review_ready",
                "target_state": "completed",
                "task_class": "self_update_candidate",
                "evidence": {
                    "normalized_task": {
                        "summary": "Improve self-update reporting.",
                        "task_class": "self_update_candidate",
                    },
                    "workspace_binding": {
                        "repo_root": str(PLUGIN_ROOT),
                        "allowed_scope": ["plugins/navi-programmer"],
                        "is_self_update": True,
                        "candidate_context": {"candidate_repo_root": ".candidate/navi-programmer"},
                    },
                    "changed_files": [{"path": "plugins/navi-programmer/tests/evals/README.md"}],
                    "validation_results": [
                        {
                            "validation_kind": "test",
                            "verdict": "passed",
                            "command": ["python", "-m", "unittest"],
                        }
                    ],
                    "lifecycle_summary": {
                        "branch": "candidate/programmer-reporting",
                        "commit": "9876abc",
                        "review_surface": "Local validation only.",
                    },
                },
            }
        )
        self.assertTrue(result["blocked"])
        self.assertIn("self_update_smoke_results_missing", result["blocked_reasons"])

    def test_runner_synthesizes_runtime_verified_self_update_result(self) -> None:
        result = runner.synthesize_result(
            {
                "contract_path": "self-update-candidate.compiled.json",
                "current_state": "review_ready",
                "task_class": "self_update_candidate",
                "evidence": json.loads(
                    (PLUGIN_ROOT / "tests" / "fixtures" / "self-update-candidate-pass.json").read_text(encoding="utf-8")
                )["runner_input"]["evidence"],
            }
        )
        self.assertEqual(result["verification_tier"], "runtime_verified")
        self.assertEqual(result["candidate_runtime_validation"]["status"], "runtime_verified")
        self.assertEqual(result["promotion_readiness_summary"]["promotion_recommendation"], "recommended")
        self.assertEqual(
            result["candidate_context"]["candidate_repo_root"],
            ".candidate/navi-programmer",
        )

    def test_runner_start_run_uses_ticket_driven_contract_for_linear_source(self) -> None:
        result = runner.start_run(
            {
                "contract_path": "ticket-driven-coding.compiled.json",
                "run_id": "np-run-ticket-1",
                "raw_task": "Implement the ticket-driven intake path.",
                "source": "linear",
                "source_id": "OMN-247",
            }
        )
        self.assertEqual(
            result["programmer_run"]["workflow_id"],
            "navi.programmer.ticket_driven_coding",
        )
        self.assertEqual(result["programmer_run"]["task_source"], "linear")
        self.assertEqual(result["programmer_run"]["task_source_id"], "OMN-247")

    def test_runner_synthesizes_source_reference_for_ticket_driven_result(self) -> None:
        fixture = json.loads(
            (PLUGIN_ROOT / "tests" / "fixtures" / "ticket-driven-mutation.json").read_text(
                encoding="utf-8"
            )
        )
        result = runner.synthesize_result(
            {
                "contract_path": "ticket-driven-coding.compiled.json",
                "current_state": fixture["runner_input"]["current_state"],
                "task_class": fixture["runner_input"]["task_class"],
                "evidence": fixture["runner_input"]["evidence"],
            }
        )
        self.assertEqual(result["task_source"], "linear")
        self.assertEqual(result["source_reference"]["source"], "linear")
        self.assertEqual(result["source_reference"]["source_id"], "OMN-277")

    def test_runner_keeps_repo_lifecycle_branch_and_commit_in_result_summary(self) -> None:
        fixture = json.loads(
            (PLUGIN_ROOT / "tests" / "fixtures" / "repo-lifecycle-success.json").read_text(encoding="utf-8")
        )
        runner_input = fixture["runner_input"]
        result = runner.synthesize_result(
            {
                "current_state": runner_input["current_state"],
                "task_class": runner_input["task_class"],
                "evidence": runner_input["evidence"],
            }
        )
        self.assertEqual(result["outcome"], "completed")
        self.assertTrue(result["reviewable"])
        self.assertEqual(result["lifecycle_summary"]["branch"], "feat/programmer-eval-regression")
        self.assertEqual(result["lifecycle_summary"]["commit"], "abc1234")
        self.assertIn("human review", result["lifecycle_summary"]["review_surface"])


if __name__ == "__main__":
    unittest.main()
