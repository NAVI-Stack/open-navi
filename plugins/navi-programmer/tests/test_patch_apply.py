import importlib.util
import shutil
import tempfile
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "patch-apply" / "main.py"
)
SPEC = importlib.util.spec_from_file_location("patch_apply_main", MODULE_PATH)
patch_apply = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(patch_apply)


class PatchApplySkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-programmer-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"patch-apply-{uuid.uuid4().hex}"
        self.root.mkdir(parents=True, exist_ok=False)
        (self.root / "pkg").mkdir()
        (self.root / "pkg" / "module.py").write_text(
            "alpha\nbeta\ngamma\n", encoding="utf-8"
        )

    def tearDown(self) -> None:
        shutil.rmtree(self.root, ignore_errors=True)

    def test_preview_patch_reports_diff_without_writing_file(self) -> None:
        result = patch_apply.preview_patch(
            {
                "root": str(self.root),
                "operations": [
                    {
                        "op": "replace_text",
                        "path": "pkg/module.py",
                        "old_text": "beta",
                        "new_text": "beta changed",
                    }
                ],
            }
        )
        current = (self.root / "pkg" / "module.py").read_text(encoding="utf-8")
        self.assertEqual(current, "alpha\nbeta\ngamma\n")
        self.assertTrue(result["changed"])
        self.assertTrue(result["dry_run"])
        self.assertEqual(result["changed_files"][0]["path"], "pkg/module.py")
        self.assertEqual(result["changed_files"][0]["action"], "patch")
        self.assertIn("beta changed", result["diff_summary"]["unified_diff"])

    def test_apply_patch_writes_changes_and_reports_changed_files(self) -> None:
        result = patch_apply.apply_patch(
            {
                "root": str(self.root),
                "operations": [
                    {
                        "op": "insert_after",
                        "path": "pkg/module.py",
                        "anchor": "beta",
                        "text": "\ninserted",
                    }
                ],
            }
        )
        current = (self.root / "pkg" / "module.py").read_text(encoding="utf-8")
        self.assertEqual(current, "alpha\nbeta\ninserted\ngamma\n")
        self.assertTrue(result["changed"])
        self.assertFalse(result["dry_run"])
        self.assertEqual(result["changed_files"][0]["path"], "pkg/module.py")
        self.assertEqual(result["operation_results"][0]["op"], "insert_after")
        self.assertIn("+inserted", result["diff_summary"]["unified_diff"])

    def test_apply_patch_rejects_missing_anchor(self) -> None:
        with self.assertRaises(patch_apply.SkillError) as err:
            patch_apply.apply_patch(
                {
                    "root": str(self.root),
                    "operations": [
                        {
                            "op": "insert_after",
                            "path": "pkg/module.py",
                            "anchor": "missing",
                            "text": "\ninserted",
                        }
                    ],
                }
            )
        self.assertEqual(err.exception.code, "anchor_not_found")


if __name__ == "__main__":
    unittest.main()
