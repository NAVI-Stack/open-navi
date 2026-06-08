package coderalias

import "testing"

func TestCanonicalSkillID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Coder alias -> legacy canonical.
		{"navi-coder.file-mutate", "navi-programmer.file-mutate"},
		{"navi-coder.git-lifecycle", "navi-programmer.git-lifecycle"},
		{"navi-coder.patch-apply", "navi-programmer.patch-apply"},
		{"navi-coder.repo-inspect", "navi-programmer.repo-inspect"},
		{"navi-coder.remote-review", "navi-programmer.remote-review"},
		{"navi-coder.run-validation", "navi-programmer.run-validation"},
		{"navi-coder.task-normalize", "navi-programmer.task-normalize"},
		// Legacy ids are already canonical (pass through).
		{"navi-programmer.run-validation", "navi-programmer.run-validation"},
		// Collision-safety: the unrelated real skill must pass through untouched.
		{"navi.coder.repo", "navi.coder.repo"},
		// Unknown / unrelated ids pass through untouched.
		{"navi-coder.unknown-skill", "navi-coder.unknown-skill"},
		{"navi-contacts", "navi-contacts"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CanonicalSkillID(c.in); got != c.want {
			t.Errorf("CanonicalSkillID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCoderSkillIDInvertsCanonical(t *testing.T) {
	for _, s := range skillSuffixes {
		legacy := legacySkillPrefix + s
		coder := coderSkillPrefix + s
		if got := CoderSkillID(legacy); got != coder {
			t.Errorf("CoderSkillID(%q) = %q, want %q", legacy, got, coder)
		}
		// Round-trip both directions.
		if got := CanonicalSkillID(CoderSkillID(legacy)); got != legacy {
			t.Errorf("round-trip CanonicalSkillID(CoderSkillID(%q)) = %q, want %q", legacy, got, legacy)
		}
	}
	// Non-legacy ids pass through.
	if got := CoderSkillID("navi.coder.repo"); got != "navi.coder.repo" {
		t.Errorf("CoderSkillID(navi.coder.repo) = %q, want unchanged", got)
	}
}

func TestIsProgrammerScopedSkillID(t *testing.T) {
	truthy := []string{
		"navi-programmer.run-validation",
		"navi-programmer.task-normalize",
		"navi-coder.run-validation",
		"navi-coder.task-normalize",
	}
	for _, id := range truthy {
		if !IsProgrammerScopedSkillID(id) {
			t.Errorf("IsProgrammerScopedSkillID(%q) = false, want true", id)
		}
	}
	falsy := []string{
		"navi.coder.repo", // dotted, unrelated real skill
		"navi-coder",      // bare plugin id, no trailing dot
		"navi-contacts.lookup",
		"navi.programmer", // plugin id, not a skill id
		"",
	}
	for _, id := range falsy {
		if IsProgrammerScopedSkillID(id) {
			t.Errorf("IsProgrammerScopedSkillID(%q) = true, want false", id)
		}
	}
}

func TestCanonicalWorkflowID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"navi.coder.bounded_mutation", "navi.programmer.bounded_mutation"},
		{"navi.coder.self_update_candidate", "navi.programmer.self_update_candidate"},
		{"navi.coder.ticket_driven_coding", "navi.programmer.ticket_driven_coding"},
		// Legacy ids pass through.
		{"navi.programmer.bounded_mutation", "navi.programmer.bounded_mutation"},
		// Collision-safety: the unrelated dotted skill is not a workflow and must
		// never be rewritten by a prefix swap.
		{"navi.coder.repo", "navi.coder.repo"},
		{"navi.coder.unknown_workflow", "navi.coder.unknown_workflow"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CanonicalWorkflowID(c.in); got != c.want {
			t.Errorf("CanonicalWorkflowID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCoderWorkflowIDInvertsCanonical(t *testing.T) {
	for _, w := range workflowSuffixes {
		legacy := legacyWorkflowPrefix + w
		coder := coderWorkflowPrefix + w
		if got := CoderWorkflowID(legacy); got != coder {
			t.Errorf("CoderWorkflowID(%q) = %q, want %q", legacy, got, coder)
		}
		if got := CanonicalWorkflowID(CoderWorkflowID(legacy)); got != legacy {
			t.Errorf("round-trip CanonicalWorkflowID(CoderWorkflowID(%q)) = %q, want %q", legacy, got, legacy)
		}
	}
}

func TestPluginIDAlias(t *testing.T) {
	if got := CanonicalPluginID(CoderPluginID); got != LegacyPluginID {
		t.Errorf("CanonicalPluginID(%q) = %q, want %q", CoderPluginID, got, LegacyPluginID)
	}
	if got := CanonicalPluginID(LegacyPluginID); got != LegacyPluginID {
		t.Errorf("CanonicalPluginID(%q) = %q, want unchanged", LegacyPluginID, got)
	}
	// The unrelated real plugin id must pass through and must NOT be equivalent.
	if got := CanonicalPluginID("navi-coder"); got != "navi-coder" {
		t.Errorf("CanonicalPluginID(navi-coder) = %q, want unchanged", got)
	}

	if !PluginIDsEquivalent(CoderPluginID, LegacyPluginID) {
		t.Errorf("PluginIDsEquivalent(%q, %q) = false, want true", CoderPluginID, LegacyPluginID)
	}
	if !PluginIDsEquivalent(LegacyPluginID, LegacyPluginID) {
		t.Errorf("PluginIDsEquivalent(self) = false, want true")
	}
	if PluginIDsEquivalent("navi-coder", LegacyPluginID) {
		t.Errorf("PluginIDsEquivalent(navi-coder, navi.programmer) = true, want false")
	}
	if PluginIDsEquivalent("navi-coder", CoderPluginID) {
		t.Errorf("PluginIDsEquivalent(navi-coder, navi.coder) = true, want false")
	}
}
