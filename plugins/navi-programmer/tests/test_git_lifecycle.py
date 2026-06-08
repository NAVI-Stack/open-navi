import importlib.util
import os
import shutil
import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "git-lifecycle" / "main.py"
)
SPEC = importlib.util.spec_from_file_location("git_lifecycle_main", MODULE_PATH)
git_lifecycle = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(git_lifecycle)


class GitLifecycleSkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-programmer-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"git-lifecycle-{uuid.uuid4().hex}"
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

        (self.root / "pkg").mkdir()
        (self.root / "pkg" / "module.py").write_text("VALUE = 1\n", encoding="utf-8")

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

    def test_create_branch_blocks_dirty_worktree_without_override(self) -> None:
        (self.root / "pkg" / "module.py").write_text("VALUE = 2\n", encoding="utf-8")
        with self.assertRaises(git_lifecycle.SkillError) as ctx:
            git_lifecycle.create_branch(
                {
                    "root": str(self.root),
                    "branch_name": "feature/dirty-branch",
                }
            )
        self.assertEqual(ctx.exception.code, "dirty_worktree_blocked")

    def test_lifecycle_surfaces_branch_commit_diff_and_review_handoff(self) -> None:
        branch = git_lifecycle.create_branch(
            {
                "root": str(self.root),
                "branch_name": "feature/reviewable-output",
                "checkout": True,
            }
        )
        self.assertEqual(branch["branch"], "feature/reviewable-output")
        self.assertTrue(branch["checked_out"])

        (self.root / "pkg" / "module.py").write_text("VALUE = 2\n", encoding="utf-8")
        status = git_lifecycle.inspect_status(
            {
                "root": str(self.root),
                "include_diff": True,
                "paths": ["pkg/module.py"],
            }
        )
        self.assertTrue(status["dirty"])
        self.assertEqual(status["changed_files"][0]["path"], "pkg/module.py")
        self.assertTrue(status["diff"]["included"])
        self.assertIn("VALUE = 2", status["diff"]["diff"])

        review = git_lifecycle.prepare_review(
            {
                "root": str(self.root),
                "summary": "Prepare local review handoff.",
                "validation_summary": "python -m unittest passed",
                "paths": ["pkg/module.py"],
            }
        )
        self.assertIn("python -m unittest passed", review["handoff_markdown"])
        self.assertIn("pkg/module.py", review["handoff_markdown"])
        self.assertTrue(review["diff"]["included"])

        commit = git_lifecycle.create_commit(
            {
                "root": str(self.root),
                "message": "module: update value",
                "paths": ["pkg/module.py"],
                "expected_branch": "feature/reviewable-output",
            }
        )
        self.assertEqual(commit["branch"], "feature/reviewable-output")
        self.assertTrue(commit["commit"])
        self.assertEqual(commit["changed_files"][0]["path"], "pkg/module.py")

    def test_create_commit_rejects_branch_mismatch(self) -> None:
        git_lifecycle.create_branch(
            {
                "root": str(self.root),
                "branch_name": "feature/actual-branch",
                "checkout": True,
            }
        )
        (self.root / "pkg" / "module.py").write_text("VALUE = 3\n", encoding="utf-8")
        with self.assertRaises(git_lifecycle.SkillError) as ctx:
            git_lifecycle.create_commit(
                {
                    "root": str(self.root),
                    "message": "module: update value again",
                    "paths": ["pkg/module.py"],
                    "expected_branch": "feature/different-branch",
                }
            )
        self.assertEqual(ctx.exception.code, "branch_mismatch")


if __name__ == "__main__":
    unittest.main()
