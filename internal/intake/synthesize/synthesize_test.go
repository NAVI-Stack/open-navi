package synthesize

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/intake/score"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func testRecord() schema.IntakeRecord {
	return schema.IntakeRecord{
		ID:           "rec-1",
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     "999:1001",
		Cursor:       "999:1001",
		FetchedAt:    time.Now().UTC(),
		Trust:        schema.ContentTrustExternalUntrusted,
		PrivacyClass: schema.PrivacyClassPersonal,
	}
}

func testChunk() schema.IntakeChunk {
	return schema.IntakeChunk{
		ID:             "chunk-1",
		IntakeRecordID: "rec-1",
		ConnectorID:    "telegram:acct-1",
		SourceID:       "999:1001",
		Content:        "Met Alex Rivera at the conference.",
		Trust:          schema.ContentTrustExternalUntrusted,
		PrivacyClass:   schema.PrivacyClassPersonal,
	}
}

func candidate(name, kind string) schema.ExtractionCandidate {
	return schema.ExtractionCandidate{Name: name, Kind: kind, Value: name, Confidence: 0.6}
}

func newInput(res schema.ResolutionResult, promote bool) Input {
	return Input{
		Chunk:       testChunk(),
		Score:       score.Result{Score: 0.7, PromoteCandidate: promote, RetentionTier: score.RetentionDurable},
		Extraction:  schema.ExtractionResult{ChunkID: "chunk-1", Candidates: []schema.ExtractionCandidate{candidate("Alex Rivera", "proper_noun")}},
		Resolutions: []schema.ResolutionResult{res},
	}
}

func TestSynthesize_CreateApproved(t *testing.T) {
	db := store.InitTestDB(t)
	s := New(db, governor.MutationPipelineOptions{}, nil)

	res, err := s.Synthesize(context.Background(), testRecord(), []Input{newInput(schema.ResolutionResult{}, false)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.EntitiesWritten != 1 {
		t.Fatalf("want 1 entity written, got %d (%+v)", res.EntitiesWritten, res.Outcomes)
	}
	contacts, _ := store.ListContacts(context.Background(), db, store.ListContactsFilter{Limit: 10})
	if len(contacts) != 1 || contacts[0].Name != "Alex Rivera" {
		t.Fatalf("contact not written: %+v", contacts)
	}
	// Provenance + confidence recorded on the entity.
	ep, err := store.GetEntityProvenance(context.Background(), db, "contact", contacts[0].ID)
	if err != nil || ep == nil {
		t.Fatalf("entity provenance missing: %v", err)
	}
	if ep.Confidence <= 0 || ep.Confidence > 1 {
		t.Errorf("confidence out of range: %v", ep.Confidence)
	}
	// Provenance link back to the source record.
	links, _ := store.ListIntakeProvenanceByRecord(context.Background(), db, "rec-1")
	var found bool
	for _, l := range links {
		if l.EntityType == "contact" && l.EntityID == contacts[0].ID && l.Outcome == "approved" {
			found = true
			if l.SourceID != "999:1001" || l.Cursor != "999:1001" {
				t.Errorf("provenance link missing source/cursor: %+v", l)
			}
		}
	}
	if !found {
		t.Errorf("no approved provenance link for the contact: %+v", links)
	}
}

func TestSynthesize_ReinforceApproved(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	// Seed an existing contact the resolution matches.
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "contact-existing", Name: "Alex Rivera", Kind: "person"})

	s := New(db, governor.MutationPipelineOptions{}, nil)
	res, err := s.Synthesize(ctx, testRecord(), []Input{newInput(schema.ResolutionResult{
		MatchedID: "contact-existing", Confidence: 0.95,
	}, false)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.EntitiesWritten != 1 {
		t.Errorf("reinforce should count as a write, got %d", res.EntitiesWritten)
	}
	// No new contact should be created — still exactly one.
	contacts, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 10})
	if len(contacts) != 1 {
		t.Errorf("reinforce must not create a new contact, got %d", len(contacts))
	}
	ep, _ := store.GetEntityProvenance(ctx, db, "contact", "contact-existing")
	if ep == nil || ep.ReinforcementCount < 1 {
		t.Errorf("reinforcement_count not bumped: %+v", ep)
	}
}

func TestSynthesize_ModifiedOnAmbiguous(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	s := New(db, governor.MutationPipelineOptions{}, nil)

	res, err := s.Synthesize(ctx, testRecord(), []Input{newInput(schema.ResolutionResult{
		Ambiguous: true, PossibleMergeIDs: []string{"c1", "c2"},
	}, false)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.Modified != 1 {
		t.Fatalf("want 1 modified, got %d (%+v)", res.Modified, res.Outcomes)
	}
	// A low-confidence Create was written, tagged possible_merge_with — NOT a merge.
	contacts, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 10})
	if len(contacts) != 1 {
		t.Fatalf("ambiguous should write one low-confidence create, got %d", len(contacts))
	}
	if !contains(contacts[0].Metadata, "possible_merge_with") {
		t.Errorf("modified contact missing possible_merge_with tag: %s", contacts[0].Metadata)
	}
}

func TestSynthesize_ConfidentMergeRaisesProposal_Delta(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	s := New(db, governor.MutationPipelineOptions{}, nil)

	res, err := s.Synthesize(ctx, testRecord(), []Input{newInput(schema.ResolutionResult{
		PossibleMergeIDs: []string{"c1", "c2"}, Confidence: 0.95,
	}, false)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.ProposalsRaised != 1 {
		t.Fatalf("want 1 proposal, got %d (%+v)", res.ProposalsRaised, res.Outcomes)
	}
	// A merge must NOT write an entity.
	contacts, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 10})
	if len(contacts) != 0 {
		t.Errorf("confident merge must not write an entity until approved, got %d", len(contacts))
	}
	pending, _ := store.ListPendingProposals(ctx, db, 10)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending proposal, got %d", len(pending))
	}
}

func TestSynthesize_BackfillGroupsProposals(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	s := New(db, governor.MutationPipelineOptions{}, nil)

	// Two chunks each producing a confident-merge candidate.
	mkInput := func(chunkID, name string, refs []string) Input {
		ch := testChunk()
		ch.ID = chunkID
		return Input{
			Chunk:       ch,
			Score:       score.Result{Score: 0.7},
			Extraction:  schema.ExtractionResult{ChunkID: chunkID, Candidates: []schema.ExtractionCandidate{candidate(name, "proper_noun")}},
			Resolutions: []schema.ResolutionResult{{PossibleMergeIDs: refs, Confidence: 0.95}},
		}
	}
	res, err := s.Synthesize(ctx, testRecord(), []Input{
		mkInput("chunk-a", "Alex Rivera", []string{"c1", "c2"}),
		mkInput("chunk-b", "Sam Doe", []string{"c3", "c4"}),
	}, schema.JobModeBackfill)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.GroupedProposalID == "" {
		t.Fatal("backfill should produce a grouped proposal id")
	}
	// Exactly ONE proposal for the whole batch (grouped), not one per merge.
	pending, _ := store.ListPendingProposals(ctx, db, 10)
	if len(pending) != 1 {
		t.Fatalf("backfill should group into 1 proposal, got %d", len(pending))
	}
	if res.ProposalsRaised != 2 {
		t.Errorf("two merges should be accounted, got %d", res.ProposalsRaised)
	}
	if !contains(pending[0].Rationale, "review as a set") {
		t.Errorf("grouped proposal rationale unexpected: %q", pending[0].Rationale)
	}
}

func TestSynthesize_GovernorDisabledDrops(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	// Disabling the write-class effect path must make synthesis drop, not write.
	s := New(db, governor.MutationPipelineOptions{WriteClassDisabled: true}, nil)

	res, err := s.Synthesize(ctx, testRecord(), []Input{newInput(schema.ResolutionResult{}, true)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.EntitiesWritten != 0 {
		t.Fatalf("write-class disabled must write zero entities, got %d", res.EntitiesWritten)
	}
	contacts, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 10})
	if len(contacts) != 0 {
		t.Errorf("no contacts should be written when governor write-class disabled, got %d", len(contacts))
	}
	mems, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 10)
	if len(mems) != 0 {
		t.Errorf("no memories should be written when governor write-class disabled, got %d", len(mems))
	}
}

func TestSynthesize_MemoryAppendForPromotedChunk(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	s := New(db, governor.MutationPipelineOptions{}, nil)

	_, err := s.Synthesize(ctx, testRecord(), []Input{newInput(schema.ResolutionResult{}, true)}, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	mems, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 10)
	if len(mems) != 1 {
		t.Fatalf("promoted chunk should append one memory, got %d", len(mems))
	}
}

// TestSynthesize_ReplayIdempotent verifies a re-run produces zero duplicate
// entities and zero duplicate proposals (acceptance: idempotency replay).
func TestSynthesize_ReplayIdempotent(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	s := New(db, governor.MutationPipelineOptions{}, nil)
	rec := testRecord()

	inputs := []Input{newInput(schema.ResolutionResult{}, true)}

	first, err := s.Synthesize(ctx, rec, inputs, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.EntitiesWritten == 0 {
		t.Fatal("first pass should write entities")
	}

	contactsAfter1, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 50})
	memsAfter1, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 50)

	// Replay the identical pass.
	second, err := s.Synthesize(ctx, rec, inputs, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("replay pass: %v", err)
	}
	if second.EntitiesWritten != 0 {
		t.Errorf("replay must write zero new entities, got %d", second.EntitiesWritten)
	}
	if second.Skipped == 0 {
		t.Errorf("replay should skip already-synthesized derivations, got skipped=%d", second.Skipped)
	}

	contactsAfter2, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 50})
	memsAfter2, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 50)
	if len(contactsAfter2) != len(contactsAfter1) {
		t.Errorf("replay created duplicate contacts: %d -> %d", len(contactsAfter1), len(contactsAfter2))
	}
	if len(memsAfter2) != len(memsAfter1) {
		t.Errorf("replay created duplicate memories: %d -> %d", len(memsAfter1), len(memsAfter2))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOfSubstr(haystack, needle) >= 0)
}

func indexOfSubstr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
