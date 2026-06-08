import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
INVENTORY = REPO_ROOT / "docs" / "plans" / "navi-coder-migration-inventory.md"
PLANS_INDEX = REPO_ROOT / "docs" / "plans" / "INDEX.md"


class CoderMigrationInventoryTests(unittest.TestCase):
    def test_inventory_exists_and_is_indexed(self) -> None:
        self.assertTrue(INVENTORY.exists(), "NAVI Coder migration inventory should exist")
        index = PLANS_INDEX.read_text(encoding="utf-8")
        self.assertIn("navi-coder-migration-inventory.md", index)

    def test_user_facing_stale_naming_is_prioritized_first(self) -> None:
        content = INVENTORY.read_text(encoding="utf-8")

        user_facing = content.index("## User-Facing References To Rename First")
        compatibility = content.index("## Compatibility References Requiring Staged Rename")
        history = content.index("## Historical Or Migration References To Preserve")

        self.assertLess(user_facing, compatibility)
        self.assertLess(user_facing, history)
        self.assertIn("plugins/navi-programmer/plugin.yaml", content)
        self.assertIn("internal/prompts/defaults/ncos/programmer_workflow.md", content)
        self.assertIn("No `web-src/` console labels matched", content)

    def test_inventory_does_not_present_programmer_as_current_product(self) -> None:
        content = INVENTORY.read_text(encoding="utf-8")
        normalized = " ".join(content.split())

        self.assertIn("NAVI Coder is the canonical user-facing capability name", content)
        self.assertIn("legacy, transitional, compatibility-preserving, or historical", normalized)
        self.assertNotIn("NAVI Programmer is NAVI's first-class", content)
        self.assertNotIn("NAVI Programmer is the canonical", content)

    def test_missing_ticket_source_docs_are_recorded(self) -> None:
        content = INVENTORY.read_text(encoding="utf-8")
        normalized = " ".join(content.split())

        self.assertIn("docs/canonical/navi-coder.md", content)
        self.assertIn("docs/plans/navi-coder-migration.plan.md", content)
        self.assertIn("not present in this checkout", normalized)


if __name__ == "__main__":
    unittest.main()
