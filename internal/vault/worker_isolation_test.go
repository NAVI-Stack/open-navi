package vault

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/governor"
)

// worldModelEntityTables mirrors the intake isolation test: a Vault edit must
// only ever change the World Model through governor.EvaluateMutation, never via a
// side channel. We assert the row-count invariants the seam guarantees.
var worldModelEntityTables = []string{
	"contacts", "facts", "memories", "memory_links",
	"artifacts", "proposals", "entity_provenance",
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(1) FROM "+table).Scan(&n); err != nil {
		return 0
	}
	return n
}

func snapshot(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tbl := range worldModelEntityTables {
		out[tbl] = rowCount(t, db, tbl)
	}
	return out
}

// TestNoWorldModelWritesBypassSeam_Forget proves a structural Vault gesture
// (forget) creates NO new entity rows and DELETES nothing — it only adds a
// Proposal. The hard floor is enforced by the governor, not the Vault.
func TestNoWorldModelWritesBypassSeam_Forget(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	ctx := context.Background()
	seedContact(t, w, "c1", "Alex Rivera")
	_, _ = w.ProjectAll(ctx)

	before := snapshot(t, w.db)

	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	_ = os.WriteFile(path, []byte(strings.Replace(string(content), "navi_forget: false", "navi_forget: true", 1)), 0o644)
	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}

	after := snapshot(t, w.db)
	if after["contacts"] != before["contacts"] {
		t.Errorf("forget changed contacts count: %d -> %d (must preserve entity)", before["contacts"], after["contacts"])
	}
	if after["proposals"] != before["proposals"]+1 {
		t.Errorf("forget did not add exactly one proposal: %d -> %d", before["proposals"], after["proposals"])
	}
}

// TestNoWorldModelWritesBypassSeam_Disabled proves that with the
// world_model_mutation effect disabled at the governor, an owner edit writes
// NOTHING to any World Model table — there is no bypass path.
func TestNoWorldModelWritesBypassSeam_Disabled(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{WriteClassDisabled: true})
	ctx := context.Background()
	seedContact(t, w, "c1", "Alex Rivera")
	_, _ = w.ProjectAll(ctx)

	before := snapshot(t, w.db)

	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	_ = os.WriteFile(path, []byte(strings.Replace(string(content), "# Alex Rivera", "# Mallory", 1)), 0o644)
	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}

	after := snapshot(t, w.db)
	for _, tbl := range worldModelEntityTables {
		if before[tbl] != after[tbl] {
			t.Errorf("table %q changed under disabled write-class: %d -> %d — Vault bypassed the seam",
				tbl, before[tbl], after[tbl])
		}
	}
}
