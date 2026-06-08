package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ceoai/navi/internal/navi/skill"
)

// TestLookupSkillEntryResolvesCoderAlias proves the gateway skill-by-id lookup
// resolves a Coder-facing alias (navi-coder.*) to the loaded legacy skill that
// holds the implementation (migration ticket NEW-A), while leaving legacy and
// unrelated ids unchanged.
func TestLookupSkillEntryResolvesCoderAlias(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "run-validation")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillYAML := `oss27_version: "1.0"
skill_id: "navi-programmer.run-validation"
semver: "0.1.0"
display:
  name: "Run Validation"
  description: "Legacy programmer skill for alias resolution test."
interfaces:
  - name: "run_command"
    transport:
      type: "internal"
    input_schema:
      type: "object"
      additionalProperties: true
    output_schema:
      type: "object"
      additionalProperties: true
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
performance:
  expected_p50_ms: 50
  timeout_ms: 1000
observability:
  log_redaction: []
  emit_metrics: []
governance:
  publisher: "navi.programmer"
  signed: false
  trust_tier: "builtin"
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("write SKILL.yaml: %v", err)
	}
	reg := skill.NewRegistry(workspace)
	if err := reg.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	srv := NewServer(Config{SkillRegistry: reg, WorkspaceDir: workspace, Addr: ":0"})

	// Legacy id resolves (unchanged behavior).
	if entry, ok := srv.lookupSkillEntry("navi-programmer.run-validation"); !ok || entry == nil {
		t.Fatal("legacy skill id must resolve")
	}
	// Coder alias resolves to the same legacy skill.
	entry, ok := srv.lookupSkillEntry("navi-coder.run-validation")
	if !ok || entry == nil {
		t.Fatal("coder alias skill id must resolve to the legacy skill")
	}
	if entry.Spec == nil || entry.Spec.SkillID != "navi-programmer.run-validation" {
		t.Errorf("resolved entry SkillID = %v, want navi-programmer.run-validation", entry.Spec)
	}
	// An unrelated coder-namespaced id still does not resolve.
	if _, ok := srv.lookupSkillEntry("navi-coder.does-not-exist"); ok {
		t.Error("unrelated coder id must not resolve")
	}
}
