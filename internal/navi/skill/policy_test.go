package skill

import "testing"

func TestPolicyCheck_DeniesPythonNetworkWithoutEgress(t *testing.T) {
	engine := NewPolicyEngine()
	entry := &SkillEntry{
		Spec: &OSS27Spec{
			Display: DisplayMetadata{Name: "net-skill"},
			Effects: EffectMetadata{RiskTier: "low"},
			Security: SecuritySpec{
				Sandbox: SandboxSpec{},
			},
			PythonRuntime: &PythonRuntimeSpec{
				NetworkAccess: true,
				Maturity:      "stable",
			},
		},
	}

	result, _ := engine.Check(entry)
	if result != PolicyDeny {
		t.Fatalf("expected deny, got %v", result)
	}
}

func TestPolicyCheck_ConfirmsPrototypeHighRiskPython(t *testing.T) {
	engine := NewPolicyEngine()
	entry := &SkillEntry{
		Spec: &OSS27Spec{
			Display: DisplayMetadata{Name: "proto-skill"},
			Effects: EffectMetadata{RiskTier: "high"},
			Security: SecuritySpec{
				Sandbox: SandboxSpec{
					NetworkEgress: []string{"example.com"},
				},
			},
			PythonRuntime: &PythonRuntimeSpec{
				NetworkAccess: true,
				Maturity:      "prototype",
			},
		},
	}

	result, _ := engine.Check(entry)
	if result != PolicyConfirmRequired {
		t.Fatalf("expected confirmation required, got %v", result)
	}
}
