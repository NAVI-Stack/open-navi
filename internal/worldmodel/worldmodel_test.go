package worldmodel

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func newTestWorldModel(t *testing.T) (*WorldModel, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-default",
		Name:           "Default Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/default"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(ws-default): %v", err)
	}
	return New(db), db
}

func TestWorldModel_ContactAndArtifactCRUD(t *testing.T) {
	ctx := context.Background()
	wm, _ := newTestWorldModel(t)

	contact := schema.Contact{
		ID:   "owner-1",
		Name: "Owner",
		Kind: "person",
	}
	if _, err := wm.UpsertContact(ctx, contact); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}

	mem, err := wm.CreateOrUpdateMemory(ctx, schema.Memory{
		Scope:   "owner",
		ScopeID: "owner-1",
		Summary: "test memory",
		Source:  "test",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateMemory: %v", err)
	}
	if mem.ID == "" {
		t.Fatalf("expected memory ID to be set")
	}

	art := schema.Artifact{
		ID:                 "art-test-1",
		WorkspaceID:        "ws-default",
		OwnerID:            "owner-1",
		CanonicalTitle:     "note://1",
		DisplayTitle:       "test note",
		Type:               schema.ArtifactTypeDocument,
		Subtype:            "note",
		SchemaVersion:      "1.0",
		ContentFormat:      "text/plain",
		LifecycleState:     schema.ArtifactLifecycleDraft,
		CurrentBranchID:    "br-1",
		CurrentVersionID:   "v-1",
		HeadVersionNumber:  1,
		CreatedByActorType: schema.ActorUser,
		ProvenanceRootID:   "v-1",
	}
	saved, err := wm.UpsertArtifact(ctx, art)
	if err != nil {
		t.Fatalf("UpsertArtifact: %v", err)
	}
	if saved.ID == "" {
		t.Fatalf("expected artifact ID to be set")
	}

	list, err := wm.ListArtifactsForOwner(ctx, "owner-1", 10)
	if err != nil {
		t.Fatalf("ListArtifactsForOwner: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least one artifact")
	}
}

func TestWorldModel_RelationshipsStrengthenDecayPrune(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)

	from := "owner:1"
	to := "artifact:1"

	// Strengthen relationship via TouchRelationship.
	if err := wm.TouchRelationship(ctx, from, to, "owns", "test", 0.5); err != nil {
		t.Fatalf("TouchRelationship: %v", err)
	}
	if err := wm.TouchRelationship(ctx, from, to, "owns", "test", 0.3); err != nil {
		t.Fatalf("TouchRelationship strengthen: %v", err)
	}

	// Verify confidence is clamped and equals 0.8.
	rel, err := store.GetRelationshipByEndpoints(ctx, db, from, to, "owns")
	if err != nil {
		t.Fatalf("GetRelationshipByEndpoints: %v", err)
	}
	if rel == nil {
		t.Fatalf("expected relationship to exist")
	}
	if rel.Confidence < 0.79 || rel.Confidence > 0.81 {
		t.Fatalf("expected confidence ~0.8, got %f", rel.Confidence)
	}

	// Insert an old, high-confidence relationship to test decay and prune.
	old := schema.Relationship{
		FromEntityID:     from,
		ToEntityID:       "artifact:old",
		RelationshipType: "owns",
		Confidence:       1.0,
		Recency:          time.Now().Add(-2 * time.Hour),
		Provenance:       "test",
	}
	if err := store.SaveRelationship(ctx, db, old); err != nil {
		t.Fatalf("SaveRelationship (old): %v", err)
	}

	// Decay relationships older than 1h ago by factor 0.5.
	cutoff := time.Now().Add(-1 * time.Hour)
	if err := wm.DecayRelationships(ctx, cutoff, 0.5, 10); err != nil {
		t.Fatalf("DecayRelationships: %v", err)
	}
	decayed, err := store.GetRelationshipByEndpoints(ctx, db, from, "artifact:old", "owns")
	if err != nil {
		t.Fatalf("GetRelationshipByEndpoints (decayed): %v", err)
	}
	if decayed == nil {
		t.Fatalf("expected old relationship to exist after decay")
	}
	if decayed.Confidence < 0.49 || decayed.Confidence > 0.51 {
		t.Fatalf("expected decayed confidence ~0.5, got %f", decayed.Confidence)
	}

	// Prune relationships weaker than 0.6 — should remove the decayed edge.
	if err := wm.PruneWeakRelationships(ctx, 0.6, 10); err != nil {
		t.Fatalf("PruneWeakRelationships: %v", err)
	}
	pruned, err := store.GetRelationshipByEndpoints(ctx, db, from, "artifact:old", "owns")
	if err != nil {
		t.Fatalf("GetRelationshipByEndpoints (after prune): %v", err)
	}
	if pruned != nil {
		t.Fatalf("expected old relationship to be pruned")
	}
}

func TestWorldModel_RecordInteractionEventPersistsEvent(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)

	ownerID := "owner-1"
	chatID := "chat-1"
	start := time.Now().Add(-1 * time.Minute)
	end := time.Now().Add(1 * time.Minute)

	if err := wm.RecordInteractionEvent(ctx, ownerID, chatID, map[string]any{
		"foo": "bar",
	}); err != nil {
		t.Fatalf("RecordInteractionEvent: %v", err)
	}

	events, err := store.ListWorldModelEvents(ctx, db, ownerID, start, end, 10)
	if err != nil {
		t.Fatalf("ListWorldModelEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected at least one world model event")
	}
	if events[0].Metadata == "" {
		t.Fatalf("expected metadata to be populated")
	}
}

func TestWorldModel_OperatingModeAndActiveWorkspaceSelection(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	if got := wm.GetOperatingMode(ctx); got != schema.WorkspaceOperatingModeGlobal {
		t.Fatalf("expected default operating mode %q, got %q", schema.WorkspaceOperatingModeGlobal, got)
	}
	if err := wm.SetOperatingMode(ctx, "wide_open"); err == nil {
		t.Fatal("expected invalid workspace operating mode to be rejected")
	}
	if err := wm.SetOperatingMode(ctx, string(schema.WorkspaceOperatingModeScoped)); err != nil {
		t.Fatalf("SetOperatingMode: %v", err)
	}
	if got := wm.GetOperatingMode(ctx); got != schema.WorkspaceOperatingModeScoped {
		t.Fatalf("expected scoped operating mode, got %q", got)
	}
	configs, err := wm.ListConfigurationByScope(ctx, "global", "system", 10)
	if err != nil {
		t.Fatalf("ListConfigurationByScope: %v", err)
	}
	if len(configs) == 0 || configs[0].Source != string(schema.StateKindOwnerSet) {
		t.Fatalf("expected owner_set configuration entry for workspace mode, got %#v", configs)
	}

	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	if err := store.SaveProject(ctx, db, schema.Project{
		ID:        "proj-1",
		Title:     "Project One",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject(proj-1): %v", err)
	}
	projectWorkspace := schema.Workspace{
		ID:               "ws-project",
		Name:             "Project Workspace",
		Kind:             schema.WorkspaceKindProject,
		Status:           schema.WorkspaceStatusActive,
		LocalRoots:       []string{"/repo/project"},
		AllowedActions:   schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:        now,
		UpdatedAt:        now,
		CreatedBy:        "owner-1",
		RelatedProjectID: "proj-1",
	}
	explicitWorkspace := schema.Workspace{
		ID:             "ws-explicit",
		Name:           "Explicit Workspace",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/repo/other"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
	}
	if _, err := wm.UpsertWorkspace(ctx, projectWorkspace); err != nil {
		t.Fatalf("UpsertWorkspace(project): %v", err)
	}
	if _, err := wm.UpsertWorkspace(ctx, explicitWorkspace); err != nil {
		t.Fatalf("UpsertWorkspace(explicit): %v", err)
	}

	resolved, source, err := wm.ResolveActiveWorkspace(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ResolveActiveWorkspace(project): %v", err)
	}
	if resolved == nil || resolved.ID != projectWorkspace.ID || source != "project_bound" {
		t.Fatalf("expected project-bound workspace to govern, got workspace=%#v source=%q", resolved, source)
	}

	if err := wm.SetActiveWorkspaceID(ctx, explicitWorkspace.ID); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}
	resolved, source, err = wm.ResolveActiveWorkspace(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ResolveActiveWorkspace(explicit): %v", err)
	}
	if resolved == nil || resolved.ID != explicitWorkspace.ID || source != "explicit" {
		t.Fatalf("expected explicit workspace to win, got workspace=%#v source=%q", resolved, source)
	}

	explicitWorkspace.Status = schema.WorkspaceStatusInactive
	explicitWorkspace.UpdatedAt = explicitWorkspace.UpdatedAt.Add(time.Minute)
	if _, err := wm.UpsertWorkspace(ctx, explicitWorkspace); err != nil {
		t.Fatalf("UpsertWorkspace(inactive explicit): %v", err)
	}
	if got := wm.GetExplicitActiveWorkspaceID(ctx); got != "" {
		t.Fatalf("expected inactive explicit workspace selection to be cleared, got %q", got)
	}
	resolved, source, err = wm.ResolveActiveWorkspace(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ResolveActiveWorkspace(after inactive update): %v", err)
	}
	if resolved == nil || resolved.ID != projectWorkspace.ID || source != "project_bound" {
		t.Fatalf("expected project-bound fallback after inactive update, got workspace=%#v source=%q", resolved, source)
	}

	explicitWorkspace.Status = schema.WorkspaceStatusActive
	explicitWorkspace.UpdatedAt = explicitWorkspace.UpdatedAt.Add(time.Minute)
	if _, err := wm.UpsertWorkspace(ctx, explicitWorkspace); err != nil {
		t.Fatalf("UpsertWorkspace(reactivate explicit): %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, explicitWorkspace.ID); err != nil {
		t.Fatalf("SetActiveWorkspaceID(reactivated explicit): %v", err)
	}

	if _, err := wm.ArchiveWorkspace(ctx, explicitWorkspace.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}
	if got := wm.GetExplicitActiveWorkspaceID(ctx); got != "" {
		t.Fatalf("expected archived explicit selection to be cleared, got %q", got)
	}
	resolved, source, err = wm.ResolveActiveWorkspace(ctx, "proj-1")
	if err != nil {
		t.Fatalf("ResolveActiveWorkspace(after archive): %v", err)
	}
	if resolved == nil || resolved.ID != projectWorkspace.ID || source != "project_bound" {
		t.Fatalf("expected project-bound workspace after clearing explicit selection, got workspace=%#v source=%q", resolved, source)
	}

	if err := wm.SetActiveWorkspaceID(ctx, explicitWorkspace.ID); err == nil {
		t.Fatal("expected archived workspace to be rejected as an active selection")
	}
}

func TestWorldModel_ContextBlockForChatIncludesUserModel(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	ownerID := "owner-1"

	// Seed owner contact.
	if _, err := wm.UpsertContact(ctx, schema.Contact{
		ID:   ownerID,
		Name: "Owner Name",
		Kind: "person",
	}); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}

	// Seed a fact (category must be one of user_preference, project_decision, technical_context, task_outcome for FormatFactsForPrompt).
	if err := store.SaveFact(ctx, db, store.Fact{
		Scope:    "owner",
		ScopeID:  ownerID,
		Category: "user_preference",
		Key:      "editor",
		Value:    "Neovim",
		Source:   "explicit",
	}); err != nil {
		t.Fatalf("SaveFact: %v", err)
	}

	// Seed a memory.
	if _, err := store.SaveMemory(ctx, db, schema.Memory{
		Scope:   "owner",
		ScopeID: ownerID,
		Summary: "Important past event",
		Source:  "test",
	}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Seed configuration and priority directly via SQL to keep the helper focused.
	const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"
	now := time.Now().UTC().Format(timeFormat)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO configuration (id, scope, scope_id, key, value, source, created_at, updated_at)
		VALUES ('cfg-1', 'owner', ?, 'theme', 'dark', 'explicit', ?, ?)
	`, ownerID, now, now); err != nil {
		t.Fatalf("insert configuration: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO priorities (id, scope, scope_id, name, description, source, created_at, updated_at)
		VALUES ('pri-1', 'owner', ?, 'focus', 'ship NAVI', 'owner_set', ?, ?)
	`, ownerID, now, now); err != nil {
		t.Fatalf("insert priority: %v", err)
	}

	block, err := wm.ContextBlockForChat(ctx, ownerID, "", 10, 10, 10, 10)
	if err != nil {
		t.Fatalf("ContextBlockForChat: %v", err)
	}
	if block == "" {
		t.Fatalf("expected non-empty context block")
	}
	if !strings.Contains(block, "Owner context") {
		t.Fatalf("expected Owner context header in block, got %q", block)
	}
	if !strings.Contains(block, "editor: Neovim") {
		t.Fatalf("expected fact in block, got %q", block)
	}
	if !strings.Contains(block, "Memories") {
		t.Fatalf("expected Memories section in block, got %q", block)
	}
	if !strings.Contains(block, "Configuration") || !strings.Contains(block, "theme: dark") {
		t.Fatalf("expected Configuration section with theme in block, got %q", block)
	}
	if !strings.Contains(block, "Priorities") || !strings.Contains(block, "ship NAVI") {
		t.Fatalf("expected Priorities section in block, got %q", block)
	}
}

func TestWorldModel_ContextBlockForChatIncludesActiveChatScope(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	ownerID := "owner-1"
	chatID := "chat-123"

	if _, err := wm.UpsertContact(ctx, schema.Contact{
		ID:   ownerID,
		Name: "Owner Name",
		Kind: "person",
	}); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}

	if err := store.SaveFact(ctx, db, store.Fact{
		Scope:    "owner",
		ScopeID:  ownerID,
		Category: "user_preference",
		Key:      "editor",
		Value:    "Neovim",
		Source:   "explicit",
	}); err != nil {
		t.Fatalf("SaveFact owner: %v", err)
	}

	if err := store.SaveFact(ctx, db, store.Fact{
		Scope:    "chat",
		ScopeID:  chatID,
		Category: "technical_context",
		Key:      "active_file",
		Value:    "internal/worldmodel/worldmodel.go",
		Source:   "shallow_reflection",
	}); err != nil {
		t.Fatalf("SaveFact chat: %v", err)
	}

	if _, err := store.SaveMemory(ctx, db, schema.Memory{
		Scope:   "chat",
		ScopeID: chatID,
		Summary: "Discussing active chat memory visibility",
		Source:  "test",
	}); err != nil {
		t.Fatalf("SaveMemory chat: %v", err)
	}

	block, err := wm.ContextBlockForChat(ctx, ownerID, chatID, 10, 10, 10, 10)
	if err != nil {
		t.Fatalf("ContextBlockForChat: %v", err)
	}
	if !strings.Contains(block, "editor: Neovim") {
		t.Fatalf("expected owner fact in block, got %q", block)
	}
	if !strings.Contains(block, "Active chat facts") || !strings.Contains(block, "active_file: internal/worldmodel/worldmodel.go") {
		t.Fatalf("expected chat facts in block, got %q", block)
	}
	if !strings.Contains(block, "Active chat memories") || !strings.Contains(block, "Discussing active chat memory visibility") {
		t.Fatalf("expected chat memories in block, got %q", block)
	}
}

func TestWorldModel_ContextBlockForChatIncludesUnresolvedCarryover(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	ownerID := "owner-1"
	if _, err := wm.UpsertContact(ctx, schema.Contact{
		ID:   ownerID,
		Name: "Owner Name",
		Kind: "person",
	}); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}

	if _, err := store.SaveMemory(ctx, db, schema.Memory{
		ID:      "owner-chat-summary:test:1",
		Scope:   "owner",
		ScopeID: ownerID,
		Summary: "We paused with one remaining infrastructure task.",
		Details: `{"unresolved_items":["Finish the SMTP migration.","Audit retry behavior."]}`,
		Source:  "chat_summarizer",
	}); err != nil {
		t.Fatalf("SaveMemory owner chat summary: %v", err)
	}

	block, err := wm.ContextBlockForChat(ctx, ownerID, "", 10, 10, 10, 10)
	if err != nil {
		t.Fatalf("ContextBlockForChat: %v", err)
	}
	if !strings.Contains(block, "Carryover from previous chats") {
		t.Fatalf("expected unresolved carryover section, got %q", block)
	}
	if !strings.Contains(block, "Finish the SMTP migration.") || !strings.Contains(block, "Audit retry behavior.") {
		t.Fatalf("expected unresolved items in carryover section, got %q", block)
	}
}

func TestWorldModel_ReevaluateFlaggedEntitiesCreatesProposalsAndClearsFlags(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	// Seed a re_evaluation_flag row.
	now := time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := db.ExecContext(ctx, `
		INSERT INTO re_evaluation_flags (id, entity_type, entity_id, reason, source_entity_type, source_entity_id, created_at)
		VALUES ('flag-1', 'memory', 'mem-1', 'forgotten_memory', 'memory', 'mem-0', ?)
	`, now); err != nil {
		t.Fatalf("insert re_evaluation_flag: %v", err)
	}

	var got []schema.Proposal
	saveProposal := func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	}

	if err := wm.ReevaluateFlaggedEntities(ctx, 10, saveProposal); err != nil {
		t.Fatalf("ReevaluateFlaggedEntities: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 proposal from re_evaluation_flag, got %d", len(got))
	}
	p := got[0]
	if p.SourceProcess != "consolidation" {
		t.Fatalf("expected SourceProcess consolidation, got %s", p.SourceProcess)
	}
	if !strings.Contains(p.AffectedEntities, "memory:mem-1") {
		t.Fatalf("expected affected_entities to reference memory:mem-1, got %s", p.AffectedEntities)
	}

	// Flag should be cleared after processing.
	flags, err := store.ListReEvaluationFlags(ctx, db, 10)
	if err != nil {
		t.Fatalf("ListReEvaluationFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Fatalf("expected flags to be cleared after processing, got %d", len(flags))
	}
}

func TestWorldModel_SupersedeFactWithUpdatesProvenance(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	ownerID := "owner-1"
	prov := schema.EntityProvenance{Source: "test", Timestamp: time.Now().UTC(), Confidence: 0.9}

	oldFact := schema.Fact{ID: "fact-old", Scope: "owner", ScopeID: ownerID, Category: "preference", Key: "tool", Value: "old", Source: "test"}
	if _, err := wm.PromoteFact(ctx, oldFact, prov); err != nil {
		t.Fatalf("PromoteFact old: %v", err)
	}
	newFact := schema.Fact{ID: "fact-new", Scope: "owner", ScopeID: ownerID, Category: "preference", Key: "tool", Value: "new", Source: "test"}
	if _, err := wm.PromoteFact(ctx, newFact, prov); err != nil {
		t.Fatalf("PromoteFact new: %v", err)
	}

	if err := wm.SupersedeFactWith(ctx, "fact-old", "fact-new"); err != nil {
		t.Fatalf("SupersedeFactWith: %v", err)
	}

	p, err := store.GetEntityProvenance(ctx, db, "fact", "fact-old")
	if err != nil {
		t.Fatalf("GetEntityProvenance: %v", err)
	}
	if p == nil {
		t.Fatal("expected provenance for superseded fact")
	}
	var found bool
	for _, m := range p.MutationHistory {
		if m == "superseded_by:fact-new" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MutationHistory to contain superseded_by:fact-new, got %v", p.MutationHistory)
	}
}

func TestWorldModel_RecallKnowledgeRanksSemanticMatchesAndCreatesLinks(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()

	memory, err := wm.CreateOrUpdateMemory(ctx, schema.Memory{
		Scope:   "owner",
		ScopeID: "owner-1",
		Summary: "Use Go for backend services and command-line tooling.",
		Details: "The project standard is Go rather than Python for daemon and CLI code.",
		Source:  "explicit",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateMemory: %v", err)
	}
	fact, err := wm.PromoteFact(ctx, schema.Fact{
		ID:       "fact-go",
		Scope:    "owner",
		ScopeID:  "owner-1",
		Category: "technical_context",
		Key:      "preferred_language",
		Value:    "go",
		Source:   "explicit",
	}, schema.EntityProvenance{Source: "test", Timestamp: time.Now().UTC(), Confidence: 0.9})
	if err != nil {
		t.Fatalf("PromoteFact: %v", err)
	}

	hits, err := wm.RecallKnowledge(ctx, "Which programming language should we use for the service?", "owner-1", "", 5)
	if err != nil {
		t.Fatalf("RecallKnowledge: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one semantic knowledge hit")
	}
	if hits[0].EntityID != fact.ID && hits[0].EntityID != memory.ID {
		t.Fatalf("expected Go-related fact or memory ranked first, got %+v", hits[0])
	}

	links, err := store.ListKnowledgeLinks(ctx, db, "fact", fact.ID, 10)
	if err != nil {
		t.Fatalf("ListKnowledgeLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected automatic knowledge links after related fact/memory writes")
	}
}

func TestWorldModel_ContextBlockForChatIncludesRelevantKnowledge(t *testing.T) {
	ctx := context.Background()
	wm, db := newTestWorldModel(t)
	defer db.Close()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS navi_chats (
			chat_id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL DEFAULT '',
			project_id TEXT,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			visibility TEXT NOT NULL DEFAULT 'private',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_message_at DATETIME,
			archived_at DATETIME,
			deleted_at DATETIME,
			message_count INTEGER NOT NULL DEFAULT 0,
			user_message_count INTEGER NOT NULL DEFAULT 0,
			assistant_message_count INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE TABLE IF NOT EXISTS navi_chat_messages (
			message_id TEXT PRIMARY KEY,
			chat_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		t.Fatalf("create navi chat tables: %v", err)
	}

	ownerID := "owner-1"
	chatID := "chat-semantic"

	if _, err := wm.UpsertContact(ctx, schema.Contact{ID: ownerID, Name: "Owner Name", Kind: "person"}); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}
	if err := store.SaveFact(ctx, db, store.Fact{
		ID:       "fact-db",
		Scope:    "global",
		ScopeID:  "",
		Category: "technical_context",
		Key:      "database",
		Value:    "sqlite",
		Source:   "explicit",
	}); err != nil {
		t.Fatalf("SaveFact: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO navi_chats (chat_id, title, status, visibility, owner_id, created_at, updated_at, last_message_at)
		VALUES (?, 'Semantic Recall', 'active', 'private', ?, ?, ?, ?)
	`, chatID, ownerID, now, now, now); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO navi_chat_messages (message_id, chat_id, role, content, created_at)
		VALUES
			('m1', ?, 'user', 'Should we keep SQLite as the database for this daemon?', ?),
			('m2', ?, 'assistant', 'We are reviewing the database choice.', ?)
	`, chatID, now, chatID, now); err != nil {
		t.Fatalf("insert chat messages: %v", err)
	}

	block, err := wm.ContextBlockForChat(ctx, ownerID, chatID, 10, 10, 10, 10)
	if err != nil {
		t.Fatalf("ContextBlockForChat: %v", err)
	}
	if !strings.Contains(block, "Relevant knowledge") {
		t.Fatalf("expected relevant knowledge section, got %q", block)
	}
	if !strings.Contains(strings.ToLower(block), "database: sqlite") {
		t.Fatalf("expected semantically recalled database fact, got %q", block)
	}
}
