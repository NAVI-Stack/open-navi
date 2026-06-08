import importlib.util
import shutil
import tempfile
import textwrap
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "run-validation" / "main.py"
)
SPEC = importlib.util.spec_from_file_location("run_validation_main", MODULE_PATH)
run_validation = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(run_validation)


class RunValidationSkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-programmer-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"run-validation-{uuid.uuid4().hex}"
        self.root.mkdir(parents=True, exist_ok=False)
        (self.root / "pkg").mkdir()
        (self.root / "pkg" / "__init__.py").write_text("", encoding="utf-8")
        (self.root / "pkg" / "module.py").write_text("VALUE = 1\n", encoding="utf-8")

    def tearDown(self) -> None:
        shutil.rmtree(self.root, ignore_errors=True)

    def test_run_command_passes_in_repo_context(self) -> None:
        result = run_validation.run_command(
            {
                "root": str(self.root),
                "cwd": ".",
                "command": ["python", "-m", "compileall", "pkg"],
                "validation_kind": "build",
            }
        )
        self.assertEqual(result["verdict"], "passed")
        self.assertEqual(result["cwd"], ".")
        self.assertEqual(result["exit_code"], 0)
        self.assertFalse(result["stdout_truncated"])
        self.assertFalse(result["stderr_truncated"])

    def test_run_command_reports_failed_exit_code(self) -> None:
        (self.root / "test_fail.py").write_text(
            textwrap.dedent(
                """
                import unittest

                class FailureCase(unittest.TestCase):
                    def test_failure(self):
                        self.assertEqual(1, 2)
                """
            ).strip()
            + "\n",
            encoding="utf-8",
        )
        result = run_validation.run_command(
            {
                "root": str(self.root),
                "command": [
                    "python",
                    "-m",
                    "unittest",
                    "discover",
                    "-s",
                    ".",
                    "-p",
                    "test_fail.py",
                ],
                "validation_kind": "test",
            }
        )
        self.assertEqual(result["verdict"], "failed")
        self.assertNotEqual(result["exit_code"], 0)
        self.assertIn("FAILED", result["stderr"] or result["stdout"])

    def test_run_command_reports_ambiguous_when_output_is_truncated(self) -> None:
        for index in range(120):
            (self.root / "pkg" / f"module_{index}.py").write_text(
                f"VALUE_{index} = {index}\n", encoding="utf-8"
            )
        result = run_validation.run_command(
            {
                "root": str(self.root),
                "command": ["python", "-m", "compileall", "pkg"],
                "validation_kind": "build",
                "max_output_bytes": 1024,
            }
        )
        self.assertEqual(result["verdict"], "ambiguous")
        self.assertEqual(result["exit_code"], 0)
        self.assertTrue(result["stdout_truncated"] or result["stderr_truncated"])

    def test_run_command_reports_timeout(self) -> None:
        (self.root / "test_slow.py").write_text(
            textwrap.dedent(
                """
                import time
                import unittest

                class SlowCase(unittest.TestCase):
                    def test_slow(self):
                        time.sleep(2)
                """
            ).strip()
            + "\n",
            encoding="utf-8",
        )
        result = run_validation.run_command(
            {
                "root": str(self.root),
                "command": [
                    "python",
                    "-m",
                    "unittest",
                    "discover",
                    "-s",
                    ".",
                    "-p",
                    "test_slow.py",
                ],
                "validation_kind": "test",
                "timeout_ms": 1000,
            }
        )
        self.assertEqual(result["verdict"], "timed_out")
        self.assertIsNone(result["exit_code"])
        self.assertTrue(result["timed_out"])

    def test_run_command_rejects_non_validation_python_script(self) -> None:
        (self.root / "mutate.py").write_text("print('mutate')\n", encoding="utf-8")
        with self.assertRaises(run_validation.SkillError) as err:
            run_validation.run_command(
                {
                    "root": str(self.root),
                    "command": ["python", "mutate.py"],
                    "validation_kind": "other",
                }
            )
        self.assertEqual(err.exception.code, "command_not_allowed")

    def test_record_not_run_returns_structured_result(self) -> None:
        result = run_validation.record_not_run(
            {
                "root": str(self.root),
                "reason": "lint tool unavailable in fixture environment",
                "blocked_by": "missing_ruff",
                "recommended_command": ["python", "-m", "ruff", "check", "."],
                "validation_kind": "lint",
            }
        )
        self.assertEqual(result["verdict"], "not_run")
        self.assertEqual(result["blocked_by"], "missing_ruff")
        self.assertEqual(
            result["recommended_command"],
            ["python", "-m", "ruff", "check", "."],
        )


if __name__ == "__main__":
    unittest.main()
