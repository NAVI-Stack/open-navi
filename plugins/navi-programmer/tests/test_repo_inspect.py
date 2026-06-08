import importlib.util
import os
import shutil
import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "repo-inspect" / "main.py"
)
NON_GIT_TMP_ROOT = Path(tempfile.gettempdir()) / "navi-plugin-repo-inspect-nongit"
SPEC = importlib.util.spec_from_file_location("repo_inspect_main", MODULE_PATH)
repo_inspect = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(repo_inspect)


class RepoInspectSkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-plugin-repo-inspect-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"repo-inspect-{uuid.uuid4().hex}"
        self.root.mkdir(parents=True, exist_ok=False)
        self.env_backup = {
            "GIT_CONFIG_GLOBAL": os.environ.get("GIT_CONFIG_GLOBAL"),
            "GIT_CONFIG_NOSYSTEM": os.environ.get("GIT_CONFIG_NOSYSTEM"),
            "HOME": os.environ.get("HOME"),
            "USERPROFILE": os.environ.get("USERPROFILE"),
        }
        blank_gitconfig = self.root / ".gitconfig"
        blank_gitconfig.write_text("", encoding="utf-8")
        os.environ["GIT_CONFIG_GLOBAL"] = str(blank_gitconfig)
        os.environ["GIT_CONFIG_NOSYSTEM"] = "1"
        os.environ["HOME"] = str(self.root)
        os.environ["USERPROFILE"] = str(self.root)

        (self.root / "src").mkdir()
        (self.root / "docs").mkdir()
        (self.root / ".hidden").mkdir()
        (self.root / "src" / "app.py").write_text(
            "line one\nline two\nline three\n", encoding="utf-8"
        )
        (self.root / "docs" / "guide.md").write_text(
            "Hello NAVI Programmer\nValidation gates matter.\n", encoding="utf-8"
        )
        (self.root / ".hidden" / "secret.txt").write_text("secret\n", encoding="utf-8")
        (self.root / "blob.bin").write_bytes(b"\x00\x01\x02")

        self.run_git("init")
        self.run_git("config", "user.name", "Codex")
        self.run_git("config", "user.email", "codex@example.com")
        self.run_git("add", ".")
        self.run_git("commit", "-m", "initial")

    def tearDown(self) -> None:
        for key, value in self.env_backup.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        shutil.rmtree(self.root, ignore_errors=True)

    def run_git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=self.root,
            check=True,
            capture_output=True,
            text=True,
            env=os.environ.copy(),
        )

    def test_list_tree_skips_hidden_paths_by_default(self) -> None:
        result = repo_inspect.list_tree(
            {"root": str(self.root), "path": ".", "recursive": True, "max_depth": 3}
        )
        returned_paths = {entry["path"] for entry in result["entries"]}
        self.assertIn("src", returned_paths)
        self.assertIn("src/app.py", returned_paths)
        self.assertNotIn(".hidden", returned_paths)
        self.assertGreaterEqual(result["excluded_entries"], 1)

    def test_list_tree_rejects_non_directory(self) -> None:
        with self.assertRaises(repo_inspect.SkillError) as err:
            repo_inspect.list_tree({"root": str(self.root), "path": "src/app.py"})
        self.assertEqual(err.exception.code, "not_directory")

    def test_read_file_supports_line_windows(self) -> None:
        result = repo_inspect.read_file(
            {
                "root": str(self.root),
                "path": "src/app.py",
                "start_line": 2,
                "end_line": 3,
            }
        )
        self.assertEqual(
            result["content"].replace("\r\n", "\n"), "line two\nline three\n"
        )
        self.assertEqual(result["line_window"]["start_line"], 2)

    def test_read_file_rejects_binary_content(self) -> None:
        with self.assertRaises(repo_inspect.SkillError) as err:
            repo_inspect.read_file({"root": str(self.root), "path": "blob.bin"})
        self.assertEqual(err.exception.code, "binary_file")

    def test_search_text_supports_regex(self) -> None:
        result = repo_inspect.search_text(
            {
                "root": str(self.root),
                "path": "docs",
                "query": "Validation\\s+gates",
                "regex": True,
            }
        )
        self.assertEqual(result["matches"][0]["path"], "docs/guide.md")
        self.assertFalse(result["truncated"])

    def test_search_text_rejects_invalid_regex(self) -> None:
        with self.assertRaises(repo_inspect.SkillError) as err:
            repo_inspect.search_text(
                {"root": str(self.root), "path": ".", "query": "(", "regex": True}
            )
        self.assertEqual(err.exception.code, "invalid_regex")

    def test_repo_status_reports_dirty_worktree(self) -> None:
        (self.root / "src" / "app.py").write_text(
            "line one\nline two\nline three\nline four\n", encoding="utf-8"
        )
        result = repo_inspect.repo_status({"root": str(self.root)})
        self.assertTrue(result["git_available"])
        self.assertTrue(result["is_git_repo"])
        self.assertTrue(result["dirty"])
        self.assertTrue(any("src/app.py" in line for line in result["status_lines"]))

    def test_repo_status_reports_non_git_directory(self) -> None:
        NON_GIT_TMP_ROOT.mkdir(parents=True, exist_ok=True)
        other_root = NON_GIT_TMP_ROOT / f"repo-inspect-nongit-{uuid.uuid4().hex}"
        other_root.mkdir(parents=True, exist_ok=False)
        self.addCleanup(lambda: shutil.rmtree(other_root, ignore_errors=True))
        (other_root / "notes.txt").write_text("plain\n", encoding="utf-8")
        result = repo_inspect.repo_status({"root": str(other_root)})
        self.assertTrue(result["git_available"])
        self.assertFalse(result["is_git_repo"])

    def test_inspect_diff_reports_patch_and_untracked_files(self) -> None:
        (self.root / "src" / "app.py").write_text(
            "line one\nline two changed\nline three\n", encoding="utf-8"
        )
        (self.root / "docs" / "new.md").write_text("new file\n", encoding="utf-8")

        result = repo_inspect.inspect_diff({"root": str(self.root)})
        changed = {(entry["path"], entry["status"]) for entry in result["files_changed"]}

        self.assertIn(("src/app.py", "M"), changed)
        self.assertIn(("docs/new.md", "untracked"), changed)
        self.assertIn("--- a/src/app.py", result["patch"])
        self.assertIn("line two changed", result["patch"])
        self.assertIn("docs/new.md", result["untracked_files"])

    def test_inspect_diff_rejects_out_of_scope_path(self) -> None:
        with self.assertRaises(repo_inspect.SkillError) as err:
            repo_inspect.inspect_diff(
                {"root": str(self.root), "path": str(self.root.parent / "outside.txt")}
            )
        self.assertEqual(err.exception.code, "path_out_of_scope")


if __name__ == "__main__":
    unittest.main()
