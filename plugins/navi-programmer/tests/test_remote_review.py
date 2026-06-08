import importlib.util
import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest
import uuid
from pathlib import Path


MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "skills" / "remote-review" / "main.py"
)
SPEC = importlib.util.spec_from_file_location("remote_review_main", MODULE_PATH)
remote_review = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
SPEC.loader.exec_module(remote_review)


class RemoteReviewSkillTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "navi-programmer-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"remote-review-{uuid.uuid4().hex}"
        self.root.mkdir(parents=True, exist_ok=False)
        self.env_backup = {
            "GIT_CONFIG_GLOBAL": os.environ.get("GIT_CONFIG_GLOBAL"),
            "GIT_CONFIG_NOSYSTEM": os.environ.get("GIT_CONFIG_NOSYSTEM"),
            "HOME": os.environ.get("HOME"),
            "USERPROFILE": os.environ.get("USERPROFILE"),
            "PATH": os.environ.get("PATH"),
            "GH_LOG": os.environ.get("GH_LOG"),
        }
        blank_gitconfig = self.root / ".gitconfig"
        blank_gitconfig.write_text("", encoding="utf-8")
        os.environ["GIT_CONFIG_GLOBAL"] = str(blank_gitconfig)
        os.environ["GIT_CONFIG_NOSYSTEM"] = "1"
        os.environ["HOME"] = str(self.root)
        os.environ["USERPROFILE"] = str(self.root)

        (self.root / "pkg").mkdir()
        (self.root / "pkg" / "module.py").write_text("VALUE = 1\n", encoding="utf-8")

        self.run_git(self.root, "init")
        self.run_git(self.root, "config", "user.name", "Codex")
        self.run_git(self.root, "config", "user.email", "codex@example.com")
        self.run_git(self.root, "add", ".")
        self.run_git(self.root, "commit", "-m", "initial")
        self.run_git(self.root, "checkout", "-b", "feature/test-remote-review")
        (self.root / "pkg" / "module.py").write_text("VALUE = 2\n", encoding="utf-8")
        self.run_git(self.root, "commit", "-am", "update module")

        self.remote_root = self.root / "remote.git"
        self.run_git(self.root.parent, "init", "--bare", str(self.remote_root))
        self.run_git(self.root, "remote", "add", "origin", str(self.remote_root))

        self.fake_bin = self.root / "fake-bin"
        self.fake_bin.mkdir()
        self.gh_log = self.root / "gh.log"
        gh_cmd = self.fake_bin / "gh.cmd"
        gh_cmd.write_text(
            textwrap.dedent(
                """
                @echo off
                echo %* > "%GH_LOG%"
                echo https://example.test/omniv/pull/123
                """
            ).strip()
            + "\n",
            encoding="utf-8",
        )
        os.environ["GH_LOG"] = str(self.gh_log)
        os.environ["PATH"] = str(self.fake_bin) + os.pathsep + (os.environ.get("PATH") or "")

    def tearDown(self) -> None:
        for key, value in self.env_backup.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        shutil.rmtree(self.root, ignore_errors=True)

    def run_git(self, cwd: Path, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=cwd,
            check=True,
            capture_output=True,
            text=True,
            env=os.environ.copy(),
        )

    def test_push_branch_blocks_without_confirmation(self) -> None:
        result = remote_review.push_branch({"root": str(self.root), "remote": "origin"})
        self.assertEqual(result["outcome"], "blocked_requires_confirmation")
        self.assertTrue(result["remote_actions"]["confirmation_required"])
        self.assertIn("confirmation", result["next_step"])

    def test_push_branch_pushes_with_confirmation(self) -> None:
        result = remote_review.push_branch(
            {
                "root": str(self.root),
                "remote": "origin",
                "changed_files": ["pkg/module.py"],
                "validation_summary": "python -m unittest passed",
                "confirmation": {
                    "approved": True,
                    "actor": "owner-1",
                    "mode": "explicit_confirmation",
                    "rationale": "Ready for review",
                },
            }
        )
        self.assertEqual(result["outcome"], "clean")
        self.assertTrue(result["remote_actions"]["push"])
        self.assertEqual(result["remote_actions"]["confirmed_by"], "owner-1")
        pushed = subprocess.run(
            ["git", "--git-dir", str(self.remote_root), "rev-parse", "--verify", "refs/heads/feature/test-remote-review"],
            check=False,
            capture_output=True,
            text=True,
            env=os.environ.copy(),
        )
        self.assertEqual(pushed.returncode, 0, pushed.stderr)

    def test_create_pull_request_blocks_without_confirmation(self) -> None:
        result = remote_review.create_pull_request(
            {
                "root": str(self.root),
                "title": "Review module update",
                "summary": "Prepare review surface",
            }
        )
        self.assertEqual(result["outcome"], "blocked_requires_confirmation")
        self.assertFalse(result["remote_actions"]["pull_request"])

    def test_create_pull_request_uses_gh_and_includes_validation_summary(self) -> None:
        result = remote_review.create_pull_request(
            {
                "root": str(self.root),
                "gh_executable": str(self.fake_bin / "gh.cmd"),
                "title": "Review module update",
                "summary": "Prepare review surface",
                "validation_summary": "python -m unittest passed",
                "changed_files": [{"path": "pkg/module.py"}],
                "draft": True,
                "confirmation": {
                    "approved": True,
                    "actor": "owner-1",
                    "mode": "explicit_confirmation",
                    "rationale": "Open draft PR",
                },
            }
        )
        self.assertEqual(result["outcome"], "clean")
        self.assertTrue(result["remote_actions"]["pull_request"])
        self.assertIn("python -m unittest passed", result["body"])
        self.assertEqual(result["pull_request_url"], "https://example.test/omniv/pull/123")
        log = self.gh_log.read_text(encoding="utf-8")
        self.assertIn("pr create", log)
        self.assertIn("--draft", log)


if __name__ == "__main__":
    unittest.main()
