import importlib.util
import os
import shutil
import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path


PLUGIN_ROOT = Path(__file__).resolve().parents[1]


def load_skill_module(name: str, relative_path: str):
    module_path = PLUGIN_ROOT / relative_path
    spec = importlib.util.spec_from_file_location(name, module_path)
    module = importlib.util.module_from_spec(spec)
    assert spec is not None and spec.loader is not None
    spec.loader.exec_module(module)
    return module


task_normalize = load_skill_module(
    "task_normalize_main",
    "skills/task-normalize/main.py",
)
repo_inspect = load_skill_module(
    "repo_inspect_main",
    "skills/repo-inspect/main.py",
)
file_mutate = load_skill_module(
    "file_mutate_main",
    "skills/file-mutate/main.py",
)
run_validation = load_skill_module(
    "run_validation_main",
    "skills/run-validation/main.py",
)
git_lifecycle = load_skill_module(
    "git_lifecycle_main",
    "skills/git-lifecycle/main.py",
)
runner = load_skill_module(
    "bounded_mutation_runner",
    "workflows/bounded_mutation_runner.py",
)


class ProgrammerV1EndToEndTests(unittest.TestCase):
    def setUp(self) -> None:
        temp_root = Path(tempfile.gettempdir()) / "programmer-v1-tests"
        temp_root.mkdir(parents=True, exist_ok=True)
        self.root = temp_root / f"programmer-v1-{uuid.uuid4().hex}"
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
        (self.root / "pkg" / "__init__.py").write_text("", encoding="utf-8")
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

    def test_bounded_mutation_runs_through_v1_capability_surface(self) -> None:
        raw_task = "Update pkg/module.py to set VALUE to 2 and validate the Python package."

        normalized = task_normalize.normalize_task(
            {
                "raw_task": raw_task,
                "current_repo": str(self.root),
                "repo_hints": [str(self.root)],
                "acceptance_hint": "VALUE is updated to 2 with validation evidence and review-ready output.",
            }
        )
        self.assertFalse(normalized["blocked"])
        self.assertEqual(normalized["normalized_task"]["task_class"], "bounded_mutation")

        binding = task_normalize.bind_scope(
            {
                "normalized_task": normalized["normalized_task"],
                "current_repo": str(self.root),
                "allowed_scope": ["pkg"],
            }
        )
        self.assertEqual(binding["binding_status"], "bound")
        workspace_binding = binding["workspace_binding"]
        self.assertEqual(workspace_binding["allowed_scope"], ["pkg"])

        workflow = runner.start_run(
            {
                "run_id": "programmer-v1-e2e",
                "raw_task": raw_task,
                "source": "chat",
            }
        )
        evidence = workflow["evidence"]
        evidence["normalized_task"] = normalized["normalized_task"]
        evidence["workspace_binding"] = workspace_binding

        branch = git_lifecycle.create_branch(
            {
                "root": str(self.root),
                "branch_name": "feature/programmer-v1-e2e",
                "checkout": True,
            }
        )
        self.assertEqual(branch["branch"], "feature/programmer-v1-e2e")

        status = repo_inspect.repo_status({"root": str(self.root)})
        read = repo_inspect.read_file({"root": str(self.root), "path": "pkg/module.py"})
        search = repo_inspect.search_text(
            {"root": str(self.root), "path": "pkg", "query": "VALUE"}
        )
        self.assertFalse(status["dirty"])
        self.assertIn("VALUE = 1", read["content"])
        self.assertEqual(search["matches"][0]["path"], "pkg/module.py")
        evidence["repo_status"] = status
        evidence["inspected_files"] = [{"path": read["path"]}]

        mutation = file_mutate.write_file(
            {
                "root": str(self.root),
                "path": "pkg/module.py",
                "content": "VALUE = 2\n",
                "expected_content": read["content"],
            }
        )
        self.assertTrue(mutation["changed"])

        after_mutation = runner.record_step(
            {
                "skill_id": "navi-programmer.file-mutate",
                "interface": "write_file",
                "skill_result": {"status": "success", "output": mutation},
                "evidence": evidence,
            }
        )
        evidence = after_mutation["evidence"]
        self.assertEqual(evidence["changed_files"][0]["path"], "pkg/module.py")

        validation = run_validation.run_command(
            {
                "root": str(self.root),
                "command": ["python", "-m", "compileall", "pkg"],
                "validation_kind": "build",
            }
        )
        self.assertEqual(validation["verdict"], "passed")
        after_validation = runner.record_step(
            {
                "skill_id": "navi-programmer.run-validation",
                "interface": "run_command",
                "skill_result": {"status": "success", "output": validation},
                "evidence": evidence,
            }
        )
        evidence = after_validation["evidence"]

        review = git_lifecycle.prepare_review(
            {
                "root": str(self.root),
                "summary": "Update module value through the V1 programmer loop.",
                "validation_summary": "python -m compileall pkg passed",
                "paths": ["pkg/module.py"],
            }
        )
        commit = git_lifecycle.create_commit(
            {
                "root": str(self.root),
                "message": "module: update value",
                "paths": ["pkg/module.py"],
                "expected_branch": "feature/programmer-v1-e2e",
            }
        )
        self.assertTrue(commit["commit"])
        evidence["lifecycle_summary"] = {
            "branch": commit["branch"],
            "commit": commit["commit"],
            "review_surface": review["handoff_markdown"],
            "changed_files": commit["changed_files"],
        }

        result = runner.synthesize_result(
            {
                "current_state": "review_ready",
                "task_class": "bounded_mutation",
                "evidence": evidence,
            }
        )
        self.assertEqual(result["outcome"], "completed")
        self.assertTrue(result["reviewable"])
        self.assertEqual(result["validation"]["verdicts"], ["passed"])
        self.assertEqual(result["changed_files"][0]["path"], "pkg/module.py")
        self.assertEqual(
            result["lifecycle_summary"]["branch"],
            "feature/programmer-v1-e2e",
        )


if __name__ == "__main__":
    unittest.main()
