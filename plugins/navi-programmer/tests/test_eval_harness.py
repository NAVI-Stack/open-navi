import unittest
from pathlib import Path

from evals import eval_harness


PLUGIN_ROOT = Path(__file__).resolve().parents[1]
STARTER_SUITE = PLUGIN_ROOT / "tests" / "evals" / "starter-evaluation-suite.json"


class EvalHarnessTests(unittest.TestCase):
    def test_starter_suite_runs_and_meets_minimum_coverage(self) -> None:
        report = eval_harness.run_suite(STARTER_SUITE)
        self.assertEqual(report["status"], "passed")
        self.assertEqual(report["case_count"], 25)
        self.assertTrue(report["weekly_report"]["starter_set_coverage"]["meets_minimums"])
        self.assertGreaterEqual(len(report["records"]), 25)

    def test_weekly_report_surfaces_unsafe_failures_separately(self) -> None:
        report = eval_harness.run_suite(STARTER_SUITE)
        safety = report["weekly_report"]["self_update_safety_review"]
        self.assertTrue(safety["unsafe_failures_visible"])
        self.assertIn("np-selfupdate-003", safety["unsafe_failure_ids"])
        self.assertNotIn("np-selfupdate-003", report["weekly_report"]["partial_run_ids"])

    def test_self_update_records_include_runtime_verification_and_promotion_signal(self) -> None:
        report = eval_harness.run_suite(STARTER_SUITE)
        record = next(item for item in report["records"] if item["evaluation_id"] == "np-selfupdate-001")
        self.assertEqual(record["candidate_runtime_validation_status"], "runtime_verified")
        self.assertEqual(record["promotion_recommendation"], "recommended")
        self.assertIn("candidate_runtime_validated", record["task_phases_reached"])
        self.assertIn("promotion_readiness_reported", record["task_phases_reached"])

    def test_validate_record_requires_unsafe_fail_notes(self) -> None:
        with self.assertRaises(eval_harness.EvalFailure):
            eval_harness.validate_record(
                {
                    "evaluation_id": "unsafe",
                    "task_source": "chat",
                    "task_class": "self_update_candidate",
                    "complexity_band": 3,
                    "outcome_class": "unsafe_fail",
                    "capability_scores": {"self_update_safety": 0},
                    "capability_areas": ["self_update_candidate_path"],
                    "self_update_safety_notes": "",
                }
            )


if __name__ == "__main__":
    unittest.main()
