package skill

import (
	"fmt"
)

// PolicyResult defines the outcome of a policy check.
type PolicyResult int

const (
	PolicyAllow PolicyResult = iota
	PolicyDeny
	PolicyConfirmRequired
)

// PolicyEngine enforces runtime safety invariants for skills.
type PolicyEngine struct {
	// Future: tenant policies, user-specific overrides
}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// Check evaluates whether a skill execution is permitted.
func (pe *PolicyEngine) Check(entry *SkillEntry) (PolicyResult, string) {
	if entry.Spec == nil {
		// Legacy skill: default to medium risk if unsure
		return PolicyAllow, ""
	}

	spec := entry.Spec

	if spec.PythonRuntime != nil {
		if spec.PythonRuntime.NetworkAccess && len(spec.Security.Sandbox.NetworkEgress) == 0 {
			return PolicyDeny, fmt.Sprintf("Python skill %s requests network access without declaring sandbox network egress", spec.Display.Name)
		}
		if spec.PythonRuntime.Maturity == "prototype" && spec.Effects.RiskTier == "high" {
			return PolicyConfirmRequired, fmt.Sprintf("Prototype Python skill %s is high risk and requires confirmation", spec.Display.Name)
		}
	}

	// Check risk tier
	if spec.Effects.RiskTier == "high" {
		return PolicyConfirmRequired, fmt.Sprintf("High-risk action: %s. This skill has side effects: %v", spec.Display.Name, spec.Effects.SideEffects)
	}

	if spec.Effects.RequiresConfirmation {
		return PolicyConfirmRequired, fmt.Sprintf("Confirmation required for %s", spec.Display.Name)
	}

	// Future: check data access scopes (e.g., if secrets == "read", ensure user is admin)

	return PolicyAllow, ""
}
