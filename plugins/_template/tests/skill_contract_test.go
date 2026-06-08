package tests

import "testing"

func TestSkillContractsAreGoverned(t *testing.T) {
	// TODO: Replace with NAVI's skill spec validator once integrated.
	// Assert every SKILL.yaml:
	// - has skill_id and semver
	// - declares at least one interface
	// - declares input/output schemas
	// - declares effects.side_effects, risk_tier, confirmation, idempotency, reversibility
	// - declares security metadata for external transports
	// - uses canonical tool naming: <skill_id>.<interface_name>
}
