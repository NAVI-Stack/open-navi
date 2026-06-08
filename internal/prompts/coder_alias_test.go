package prompts

import "testing"

// TestCoderWorkflowKindAliasesProgrammer proves the Coder-facing prompt kind
// resolves to the existing Programmer template on the read path (migration
// ticket NEW-A), without an on-disk ncos/coder_workflow.md.
func TestCoderWorkflowKindAliasesProgrammer(t *testing.T) {
	m, err := EmbeddedManager()
	if err != nil {
		t.Fatalf("EmbeddedManager: %v", err)
	}
	legacy, err := m.Render(KindNCOSProgrammerWorkflow, struct{}{}, RenderOptions{})
	if err != nil {
		t.Fatalf("Render(programmer): %v", err)
	}
	coder, err := m.Render(KindNCOSCoderWorkflow, struct{}{}, RenderOptions{})
	if err != nil {
		t.Fatalf("Render(coder alias): %v", err)
	}
	if coder != legacy {
		t.Errorf("Render(coder) != Render(programmer):\ncoder=%q\nlegacy=%q", coder, legacy)
	}

	if !IsKnownKind(KindNCOSCoderWorkflow) {
		t.Error("coder kind alias should be reported as known")
	}

	snap, ok := m.Snapshot(KindNCOSCoderWorkflow)
	if !ok {
		t.Fatal("Snapshot(coder alias) should resolve")
	}
	if snap.RelPath != "ncos/programmer_workflow.md" {
		t.Errorf("Snapshot RelPath = %q, want ncos/programmer_workflow.md", snap.RelPath)
	}
}

// TestCoderWorkflowKindNotManaged guards the invariant that the alias kind is
// NOT part of AllKinds() — otherwise EnsureDefaults/Reload would try to read a
// nonexistent ncos/coder_workflow.md and fail.
func TestCoderWorkflowKindNotManaged(t *testing.T) {
	for _, k := range AllKinds() {
		if k == KindNCOSCoderWorkflow {
			t.Fatal("KindNCOSCoderWorkflow must NOT be a member of AllKinds()")
		}
	}
	m := New("")
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload (embed-only) failed: %v", err)
	}
}
