package intake

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/open-navi/navi/internal/intake/distill"
	"github.com/open-navi/navi/internal/intake/embed"
	"github.com/open-navi/navi/internal/intake/retrieve"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// secretRecord builds a secret-class intake record.
func secretRecord(sourceID, body string) IntakeRecord {
	r := e2eRecord(sourceID, body)
	r.Trust = TrustOwner
	r.PrivacyClass = PrivacySecret
	return r
}

// TestE2E_P4_SecretChunkRetrievalAndRoutingRefusal drives a secret-class chunk
// through the full P1+P2+P3 pipeline, then asserts the P4 invariants:
//
//	(a) the secret chunk is retrievable when the query's privacy ceiling permits it,
//	(b) it is filtered out when the ceiling does not permit it,
//	(c) routing in Local mode refuses cloud providers when the retrieved set
//	    includes secret (no cloud provider is selected),
//	(d) retrieval performs no World Model writes,
//	(e) ingest and retrieval resolve to the SAME distillation function.
func TestE2E_P4_SecretChunkRetrievalAndRoutingRefusal(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	e := embed.NewStubEmbedder()

	driveFullPipeline(t, db, schema.JobModeDelta,
		secretRecord("999:5001", "Secret acquisition terms: target valuation and the merger payout schedule."))

	before := snapshotCounts(t, db)

	// (a) Permitted: ceiling = secret surfaces the secret chunk.
	permitted, err := retrieve.Retrieve(ctx, db, e, "acquisition merger valuation", retrieve.Options{
		Limit: 10, MaxPrivacyClass: schema.PrivacyClassSecret,
	})
	if err != nil {
		t.Fatalf("retrieve (permitted): %v", err)
	}
	if !hasSecret(permitted) {
		t.Fatal("secret chunk should be retrievable when the ceiling permits it")
	}

	// (b) Not permitted: ceiling = personal filters the secret chunk out.
	filtered, err := retrieve.Retrieve(ctx, db, e, "acquisition merger valuation", retrieve.Options{
		Limit: 10, MaxPrivacyClass: schema.PrivacyClassPersonal,
	})
	if err != nil {
		t.Fatalf("retrieve (filtered): %v", err)
	}
	if hasSecret(filtered) {
		t.Error("secret chunk leaked under a personal privacy ceiling")
	}

	// (c) The retrieved set includes secret → effective privacy tier is secret.
	// In Local mode, the router must refuse cloud providers. With only cloud
	// models configured, that is an observable typed refusal; no cloud provider
	// is ever selected.
	effective := retrieve.MaxPrivacyClass(permitted)
	if effective != schema.PrivacyClassSecret {
		t.Fatalf("effective privacy tier = %q, want secret", effective)
	}
	selector := llm.ModelSelector{
		PrivacyMode: llm.PrivacyModeLocal,
		Profiles: []llm.ModelProfile{
			{ProviderKey: "anthropic", ModelID: "claude-opus", SupportsTools: true, ToolCallReliable: true,
				ChatScore: 85, ReasoningScore: 90, Tags: []string{"cloud"}},
			{ProviderKey: "openai", ModelID: "gpt-4o", SupportsTools: true, ToolCallReliable: true,
				ChatScore: 82, ReasoningScore: 84, Tags: []string{"cloud"}},
		},
	}
	route, err := selector.SelectWithPrivacy(llm.TaskClassification{Task: llm.TaskClassChat}, false, effective)
	var refused *llm.RouteRefusedError
	if err == nil {
		t.Fatalf("expected a routing refusal for secret-in-Local, got route to %q/%q", route.Provider, route.Model)
	}
	if !errors.As(err, &refused) {
		t.Fatalf("expected *llm.RouteRefusedError, got %v", err)
	}
	if route.Provider != "" {
		t.Errorf("a cloud provider was selected (%q) despite Local-mode secret content", route.Provider)
	}

	// (d) Retrieval wrote nothing to the World Model.
	after := snapshotCounts(t, db)
	for _, tbl := range worldModelEntityTables {
		if before[tbl] != after[tbl] {
			t.Errorf("retrieval mutated World Model table %q: %d -> %d", tbl, before[tbl], after[tbl])
		}
	}
}

// TestE2E_P4_OneDistillPrimitiveTwoCallSites asserts that ingest-side and
// retrieval-side distillation resolve to the exact same exported function — the
// entire point of P2's dual-use interface (one implementation, two call sites).
func TestE2E_P4_OneDistillPrimitiveTwoCallSites(t *testing.T) {
	primitive := reflect.ValueOf(distill.Distill).Pointer()
	ingest := reflect.ValueOf(IngestDistillFn).Pointer()
	retrieval := reflect.ValueOf(retrieve.DistillFn).Pointer()
	if ingest != primitive {
		t.Error("ingest distillation does not resolve to distill.Distill")
	}
	if retrieval != primitive {
		t.Error("retrieval distillation does not resolve to distill.Distill")
	}
	if ingest != retrieval {
		t.Error("ingest and retrieval use different distillation functions — must be one primitive")
	}
}

// TestE2E_P4_RetrievalSideDistillationPreservesProvenance verifies the
// provenance round-trip: a chunk fetched via retrieval and distilled for context
// still resolves back to its source record/chunk.
func TestE2E_P4_RetrievalSideDistillationPreservesProvenance(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	e := embed.NewStubEmbedder()

	driveFullPipeline(t, db, schema.JobModeDelta,
		e2eRecord("999:6001", "Project Aurora kickoff notes and the milestone calendar for the launch."))

	results, err := retrieve.Retrieve(ctx, db, e, "Aurora milestone launch", retrieve.Options{Limit: 5})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results to distill")
	}
	distilled := retrieve.DistillResults(ctx, results, 256)
	if len(distilled.Chunks) == 0 {
		t.Fatal("distillation dropped everything")
	}
	for _, dc := range distilled.Chunks {
		if dc.RecordID == "" {
			t.Errorf("distilled chunk %s lost its source record link", dc.ChunkID)
		}
		if len(dc.SourceChunkIDs) == 0 {
			t.Errorf("distilled chunk %s lost its source-chunk provenance", dc.ChunkID)
		}
	}
}

func hasSecret(results []retrieve.RetrievalResult) bool {
	for _, r := range results {
		if r.PrivacyClass == schema.PrivacyClassSecret {
			return true
		}
	}
	return false
}
