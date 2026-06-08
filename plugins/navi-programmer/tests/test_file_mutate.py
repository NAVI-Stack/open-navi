import importlib.util
import shutil
import tempfile
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "file-mutate" / "main.py"
)
SPEC = importlib.util.spec_from_file_location("file_mutate_main", MODULE_PATH)
file_mutate = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(file_mutate)


class FileMutateSkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-programmer-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"file-mutate-{uuid.uuid4().hex}"
        self.root.mkdir(parents=True, exist_ok=False)
        (self.root / "docs").mkdir()
        (self.root / "docs" / "guide.md").write_bytes(b"before\n")

    def tearDown(self) -> None:
        shutil.rmtree(self.root, ignore_errors=True)

    def test_create_file_writes_new_file_and_reports_changed_file(self) -> None:
        result = file_mutate.create_file(
            {
                "root": str(self.root),
                "path": "docs/new.md",
                "content": "hello\nworld\n",
                "create_parent_dirs": True,
            }
        )
        created = self.root / "docs" / "new.md"
        self.assertTrue(created.exists())
        self.assertEqual(created.read_text(encoding="utf-8"), "hello\nworld\n")
        self.assertTrue(result["changed"])
        self.assertEqual(result["changed_files"][0]["path"], "docs/new.md")
        self.assertEqual(result["changed_files"][0]["action"], "create")
        self.assertIn("+hello", result["diff_summary"]["unified_diff"])

    def test_write_file_updates_existing_file_with_expected_content(self) -> None:
        result = file_mutate.write_file(
            {
                "root": str(self.root),
                "path": "docs/guide.md",
                "content": "after\n",
                "expected_content": "before\n",
            }
        )
        updated = self.root / "docs" / "guide.md"
        self.assertEqual(updated.read_text(encoding="utf-8"), "after\n")
        self.assertTrue(result["changed"])
        self.assertEqual(result["changed_files"][0]["path"], "docs/guide.md")
        self.assertEqual(result["changed_files"][0]["action"], "replace")
        self.assertIn("-before", result["diff_summary"]["unified_diff"])
        self.assertIn("+after", result["diff_summary"]["unified_diff"])

    def test_write_file_requires_precondition_for_existing_target(self) -> None:
        with self.assertRaises(file_mutate.SkillError) as err:
            file_mutate.write_file(
                {
                    "root": str(self.root),
                    "path": "docs/guide.md",
                    "content": "after\n",
                }
            )
        self.assertEqual(err.exception.code, "precondition_required")


if __name__ == "__main__":
    unittest.main()
