package navi

import (
	"testing"

	"github.com/ceoai/navi/internal/navi/skill"
	navitool "github.com/ceoai/navi/internal/tool"
)

// These tests prove the workflow bridge accepts Coder-facing skill aliases
// (navi-coder.*) and canonicalizes them to the legacy id, while leaving the
// unrelated navi.coder.repo skill untouched (migration ticket NEW-A).

func TestResolveProgrammerWorkflowSkillAcceptsCoderAliasFromEntry(t *testing.T) {
	skillID, ifaceName, runnerPath, ok := resolveProgrammerWorkflowSkill(nil, &skill.SkillEntry{
		Skill: skill.Skill{BaseDir: "/repo/plugins/navi-programmer/skills/task-normalize"},
		Spec:  &skill.OSS27Spec{SkillID: "navi-coder.task-normalize"},
	}, &skill.Interface{Name: "bind_scope"})
	if !ok {
		t.Fatal("expected coder-aliased skill entry to resolve")
	}
	if skillID != "navi-programmer.task-normalize" {
		t.Errorf("skillID = %q, want canonical navi-programmer.task-normalize", skillID)
	}
	if ifaceName != "bind_scope" {
		t.Errorf("ifaceName = %q, want bind_scope", ifaceName)
	}
	if runnerPath == "" {
		t.Error("expected a non-empty runner path")
	}
}

func TestResolveProgrammerWorkflowSkillAcceptsCoderAliasFromTool(t *testing.T) {
	tool := &navitool.Tool{
		Name:     "navi-coder.run-validation.run_command",
		Source:   navitool.ToolSourceSkill,
		SourceID: "navi-coder.run-validation",
		Metadata: navitool.ToolMetadata{SkillInterface: "run_command"},
	}
	skillID, ifaceName, _, ok := resolveProgrammerWorkflowSkill(tool, nil, nil)
	if !ok {
		t.Fatal("expected coder-aliased registered tool to resolve")
	}
	if skillID != "navi-programmer.run-validation" {
		t.Errorf("skillID = %q, want canonical navi-programmer.run-validation", skillID)
	}
	if ifaceName != "run_command" {
		t.Errorf("ifaceName = %q, want run_command", ifaceName)
	}
}

func TestResolveProgrammerWorkflowSkillLegacyAndUnrelated(t *testing.T) {
	// Legacy id still resolves unchanged.
	skillID, _, _, ok := resolveProgrammerWorkflowSkill(nil, &skill.SkillEntry{
		Skill: skill.Skill{BaseDir: "/repo/plugins/navi-programmer/skills/file-mutate"},
		Spec:  &skill.OSS27Spec{SkillID: "navi-programmer.file-mutate"},
	}, &skill.Interface{Name: "write_file"})
	if !ok || skillID != "navi-programmer.file-mutate" {
		t.Errorf("legacy resolution: ok=%v skillID=%q", ok, skillID)
	}

	// The unrelated, real navi.coder.repo skill must NOT be captured as a
	// programmer workflow skill.
	if _, _, _, ok := resolveProgrammerWorkflowSkill(nil, &skill.SkillEntry{
		Skill: skill.Skill{BaseDir: "/repo/plugins/navi-coder/skills/repo"},
		Spec:  &skill.OSS27Spec{SkillID: "navi.coder.repo"},
	}, &skill.Interface{Name: "run_tests"}); ok {
		t.Error("navi.coder.repo must not resolve as a programmer workflow skill")
	}
}
