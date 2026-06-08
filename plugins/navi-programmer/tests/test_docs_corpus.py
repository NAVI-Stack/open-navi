import unittest
from pathlib import Path


PLUGIN_ROOT = Path(__file__).resolve().parents[1]
DOCS_ROOT = PLUGIN_ROOT / "docs"

REQUIRED_DOCS = {
    "concepts/navi-programmer.md": "concept",
    "design/navi-programmer-plugin-architecture.md": "design",
    "design/navi-programmer-self-update-safety.md": "design",
    "specs/navi-programmer-plugin-v1.md": "spec",
    "specs/navi-programmer-task-execution.md": "spec",
    "plans/navi-programmer-implementation.plan.md": "plan",
    "plans/navi-programmer-evaluation.plan.md": "plan",
}


def read_doc(relative_path: str) -> str:
    return (DOCS_ROOT / relative_path).read_text(encoding="utf-8")


class DocumentationCorpusTests(unittest.TestCase):
    def test_required_docs_exist_in_expected_sections(self) -> None:
        for relative_path, section in REQUIRED_DOCS.items():
            path = DOCS_ROOT / relative_path
            self.assertTrue(path.exists(), f"{relative_path} should exist in the docs corpus")
            self.assertTrue(path.is_file(), f"{relative_path} should be a file")
            self.assertEqual(
                path.parent.name,
                section + "s" if section != "design" else "design",
                f"{relative_path} should live under the {section} corpus section",
            )

    def test_docs_entrypoints_link_to_the_required_corpus(self) -> None:
        readme = read_doc("README.md")
        index_doc = read_doc("INDEX.md")

        for relative_path in REQUIRED_DOCS:
            self.assertIn(
                relative_path,
                readme,
                f"docs/README.md should link to {relative_path}",
            )
            self.assertIn(
                relative_path,
                index_doc,
                f"docs/INDEX.md should link to {relative_path}",
            )

    def test_concept_and_architecture_frame_programmer_as_plugin_not_role(self) -> None:
        concept = read_doc("concepts/navi-programmer.md")
        architecture = read_doc("design/navi-programmer-plugin-architecture.md")

        self.assertIn("first-class programming capability plugin", concept)
        self.assertIn("not a role", concept)
        self.assertIn("first-class capability plugin", architecture)
        self.assertIn("The role is not the plugin. The plugin is not the role.", architecture)

    def test_self_update_and_plans_keep_the_corpus_contract_explicit(self) -> None:
        self_update = read_doc("design/navi-programmer-self-update-safety.md")
        implementation_plan = read_doc("plans/navi-programmer-implementation.plan.md")
        evaluation_plan = read_doc("plans/navi-programmer-evaluation.plan.md")

        self.assertIn("candidate workspace", self_update)
        self.assertIn("candidate runtime", self_update)
        self.assertIn("promotion is explicit", self_update.lower())
        self.assertIn("docs/concepts/navi-programmer.md", implementation_plan)
        self.assertIn("docs/plans/navi-programmer-evaluation.plan.md", implementation_plan)
        self.assertIn("NAVI Programmer plugin as an execution capability", evaluation_plan)
        self.assertIn("self-update scenarios", evaluation_plan)


if __name__ == "__main__":
    unittest.main()
