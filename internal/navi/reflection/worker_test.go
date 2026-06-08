package reflection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/navi/experience"
	navistore "github.com/open-navi/navi/internal/navi/store"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

func newTestWorker(t *testing.T) (*Worker, *worldmodel.WorldModel, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-1",
		Name:           "Reflection Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/reflection"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(ws-1): %v", err)
	}
	if err := store.SaveProject(ctx, db, schema.Project{
		ID:          "proj-1",
		Title:       "Reflection Project",
		Kind:        schema.ProjectKindGeneral,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: "ws-1",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject(proj-1): %v", err)
	}
	wm := worldmodel.New(db)
	return NewWorker(wm), wm, db
}

func decodeExperienceSnapshotPayloadForTest(t *testing.T, ev *schema.Event) schema.NaviExperienceSnapshotPayload {
	t.Helper()
	if ev == nil {
		t.Fatal("expected snapshot event, got nil")
	}
	raw, err := json.Marshal(ev.Payload)
	if err != nil {
		t.Fatalf("marshal snapshot payload: %v", err)
	}
	var payload schema.NaviExperienceSnapshotPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal snapshot payload: %v", err)
	}
	return payload
}

func TestProcessShallow_CreatesMemoryAndFact(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	runtimeSessionID := "sess-1"

	// Memory from manual escalation.
	memPayload := schema.ReflectionPayload{
		ID:               "mem-1",
		RuntimeSessionID: runtimeSessionID,
		Tier:             schema.ReflectionTierShallow,
		Summary:          "User shared something important",
		Details:          `{"kind":"memory","scope":"owner","scope_id":"owner-1","significance":"high"}`,
		EscalationReason: "manual",
		CreatedAt:        time.Now().UTC(),
	}
	worker.processShallow(ctx, memPayload)

	memories, err := store.ListMemories(ctx, db, "owner", "owner-1", 10)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) == 0 {
		t.Fatalf("expected at least one memory")
	}
	if memories[0].Source != "shallow_reflection" {
		t.Fatalf("expected memory source shallow_reflection, got %s", memories[0].Source)
	}

	// Fact from structured details.
	factPayload := schema.ReflectionPayload{
		ID:               "fact-1",
		RuntimeSessionID: runtimeSessionID,
		Tier:             schema.ReflectionTierShallow,
		Summary:          "User prefers Neovim",
		Details:          `{"kind":"fact","scope":"owner","scope_id":"owner-1","category":"preference","key":"editor","value":"Neovim"}`,
		CreatedAt:        time.Now().UTC(),
	}
	worker.processShallow(ctx, factPayload)

	facts, err := store.ListFacts(ctx, db, "owner", "owner-1", true, 10, false)
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatalf("expected at least one fact")
	}

	// Ensure provenance was written for the promoted fact.
	last := facts[0]
	prov, err := store.GetEntityProvenance(ctx, db, "fact", last.ID)
	if err != nil {
		t.Fatalf("get entity provenance: %v", err)
	}
	if prov == nil {
		t.Fatalf("expected provenance for fact %s", last.ID)
	}
	if prov.Source != "shallow_reflection" {
		t.Fatalf("expected provenance source shallow_reflection, got %s", prov.Source)
	}
}

func TestProcessShallow_PromotesFactsArray(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	payload := schema.ReflectionPayload{
		ID:               "facts-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierShallow,
		Summary:          "Turn completed",
		Details:          `{"summary":"Turn completed","facts":[{"key":"preferred_editor","value":"Neovim","category":"user_preference","scope":"owner","scope_id":"owner-1"},{"key":"current_task","value":"Debugging reflection writes","category":"technical_context","scope":"chat","scope_id":"sess-1"}]}`,
		CreatedAt:        time.Now().UTC(),
	}

	worker.processShallow(ctx, payload)

	ownerFacts, err := store.ListFacts(ctx, db, "owner", "owner-1", true, 10, false)
	if err != nil {
		t.Fatalf("list owner facts: %v", err)
	}
	chatFacts, err := store.ListFacts(ctx, db, "chat", "sess-1", true, 10, false)
	if err != nil {
		t.Fatalf("list chat facts: %v", err)
	}
	if len(ownerFacts) != 1 {
		t.Fatalf("expected 1 owner fact, got %d", len(ownerFacts))
	}
	if len(chatFacts) != 1 {
		t.Fatalf("expected 1 chat fact, got %d", len(chatFacts))
	}
	if ownerFacts[0].Key != "preferred_editor" {
		t.Fatalf("expected owner fact preferred_editor, got %q", ownerFacts[0].Key)
	}
	if chatFacts[0].Key != "current_task" {
		t.Fatalf("expected chat fact current_task, got %q", chatFacts[0].Key)
	}
}

func TestProcessShallow_SignificancePolicy(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	// Manual escalation → significance high when not set in details.
	manualPayload := schema.ReflectionPayload{
		ID:               "manual-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierShallow,
		Summary:          "Owner asked to remember this",
		Details:          `{"kind":"memory","scope":"owner","scope_id":"owner-1"}`,
		EscalationReason: "manual",
		CreatedAt:        time.Now().UTC(),
	}
	worker.processShallow(ctx, manualPayload)
	memories, err := store.ListMemories(ctx, db, "owner", "owner-1", 10)
	if err != nil || len(memories) == 0 {
		t.Fatalf("expected one memory: err=%v len=%d", err, len(memories))
	}
	if memories[0].Significance != "high" {
		t.Errorf("manual escalation: expected significance high, got %q", memories[0].Significance)
	}

	// Automatic with kind=memory, no significance in details → medium.
	autoPayload := schema.ReflectionPayload{
		ID:               "auto-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierShallow,
		Summary:          "Chat summary",
		Details:          `{"kind":"memory","scope":"chat","scope_id":"sess-1"}`,
		CreatedAt:        time.Now().UTC(),
	}
	worker.processShallow(ctx, autoPayload)
	chatMems, err := store.ListMemories(ctx, db, "chat", "sess-1", 10)
	if err != nil || len(chatMems) == 0 {
		t.Fatalf("expected one chat memory: err=%v len=%d", err, len(chatMems))
	}
	if chatMems[0].Significance != "medium" {
		t.Errorf("automatic memory: expected significance medium, got %q", chatMems[0].Significance)
	}
}

func TestProcessShallow_ArtifactReflectionStrengthensRelationships(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	payload := schema.ReflectionPayload{
		ID:               "artifact-reflection-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierShallow,
		Summary:          "Artifact updated",
		Details:          `{"category":"artifact","scope":"owner","scope_id":"owner-1","artifact_id":"art-1","project_id":"proj-1","operation":"replace_content","artifact_type":"document","artifact_subtype":"markdown","lifecycle_state":"draft","version_id":"ver-1","branch_id":"main"}`,
		CreatedAt:        time.Now().UTC(),
	}

	worker.processShallow(ctx, payload)

	if rel, err := store.GetRelationshipByEndpoints(ctx, db, "owner:owner-1", "artifact:art-1", "owns"); err != nil || rel == nil {
		t.Fatalf("expected owner relationship, rel=%+v err=%v", rel, err)
	}
	if rel, err := store.GetRelationshipByEndpoints(ctx, db, "project:proj-1", "artifact:art-1", "contains"); err != nil || rel == nil {
		t.Fatalf("expected project relationship, rel=%+v err=%v", rel, err)
	}
	if rel, err := store.GetRelationshipByEndpoints(ctx, db, "chat:sess-1", "artifact:art-1", "references"); err != nil || rel == nil {
		t.Fatalf("expected chat relationship, rel=%+v err=%v", rel, err)
	}
}

func TestProcessLoop_ManualDeepPersistsOwnerMemoryWithoutProposal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, _, db := newTestWorker(t)
	defer db.Close()

	var proposals []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		proposals = append(proposals, p)
		return nil
	})

	memBus := bus.NewMemBus(db)
	if err := worker.Run(ctx, memBus); err != nil {
		t.Fatalf("Run: %v", err)
	}

	payload := schema.ReflectionPayload{
		ID:               "manual-deep-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierDeep,
		Summary:          "Turn completed",
		Details:          `{"kind":"memory","scope":"owner","scope_id":"owner-1","significance":"high","user_message":"Remember that I prefer Go over Python.","assistant_reply":"I'll remember that.","facts":[{"scope":"owner","scope_id":"owner-1","category":"technical_context","key":"preferred_language","value":"go"}]}`,
		EscalationReason: "manual",
		CreatedAt:        time.Now().UTC(),
	}
	ev := schema.NewEvent(schema.FactReflectionQueued, schema.EventKindFact, payload.ID, schema.AgentNavi, payload)
	if err := memBus.Publish(ctx, ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(3 * time.Second)

	memories, err := store.ListMemories(ctx, db, "owner", "owner-1", 10)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) == 0 {
		t.Fatal("expected manual deep reflection to persist owner memory")
	}

	facts, err := store.ListFacts(ctx, db, "owner", "owner-1", true, 10, false)
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected manual deep reflection to persist owner facts")
	}

	if len(proposals) != 0 {
		t.Fatalf("expected no proposals for manual memory reflection, got %d", len(proposals))
	}
}

func TestProcessConsolidation_ConfigProposalWithAffectedEntities(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "cfg-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Update owner theme preference",
		Details:          `{"kind":"fact","scope":"owner","scope_id":"owner-1","category":"configuration","key":"theme"}`,
		EscalationReason: "automatic",
		CreatedAt:        time.Now().UTC(),
	}

	worker.processConsolidation(ctx, payload)

	if len(got) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(got))
	}
	p := got[0]
	if p.SourceProcess != "consolidation" {
		t.Fatalf("expected SourceProcess consolidation, got %s", p.SourceProcess)
	}
	if p.SourceTrigger != "automatic" {
		t.Fatalf("expected SourceTrigger automatic, got %s", p.SourceTrigger)
	}
	if p.Priority != schema.ProposalPriorityQueued {
		t.Fatalf("expected queued priority for consolidation, got %s", p.Priority)
	}
	if p.Status != schema.ProposalStatusPending {
		t.Fatalf("expected pending status, got %s", p.Status)
	}
	if !strings.Contains(p.AffectedEntities, "owner:owner-1") || !strings.Contains(p.AffectedEntities, "config:theme") {
		t.Fatalf("expected affected_entities to include owner:owner-1 and config:theme, got %s", p.AffectedEntities)
	}
}

func TestProcessConsolidation_SkipsNonConfigPriority(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "rel-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Relationship-only consolidation",
		Details:          `{"kind":"memory","scope":"chat","scope_id":"sess-1","category":"relationship"}`,
		EscalationReason: "automatic",
		CreatedAt:        time.Now().UTC(),
	}

	worker.processConsolidation(ctx, payload)

	if len(got) != 0 {
		t.Fatalf("expected no proposals for non-config/priority consolidation, got %d", len(got))
	}
}

func TestProcessConsolidation_ArtifactClusterQueuesProposalAndStrengthensRelationships(t *testing.T) {
	ctx := context.Background()
	worker, wm, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	for _, id := range []string{"art-1", "art-2", "art-3"} {
		if _, err := wm.UpsertArtifact(ctx, schema.Artifact{
			ID:             id,
			OwnerID:        "owner-1",
			WorkspaceID:    "ws-1",
			ProjectID:      "proj-1",
			DisplayTitle:   id,
			CanonicalTitle: id,
			Type:           schema.ArtifactTypeDocument,
			Subtype:        "markdown",
			LifecycleState: schema.ArtifactLifecycleDraft,
		}); err != nil {
			t.Fatalf("UpsertArtifact(%s): %v", id, err)
		}
	}

	payload := schema.ReflectionPayload{
		ID:               "artifact-consolidation-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Artifact library activity clustered in one project",
		Details:          `{"category":"artifact","scope":"owner","scope_id":"owner-1","artifact_id":"art-1","artifact_title":"Spec Draft","workspace_id":"ws-1","project_id":"proj-1","artifact_subtype":"markdown","operation":"replace_content","result_status":"completed","lifecycle_state":"draft"}`,
		EscalationReason: "automatic",
		CreatedAt:        time.Now().UTC(),
	}

	worker.processConsolidation(ctx, payload)

	if len(got) != 1 {
		t.Fatalf("expected 1 artifact proposal, got %d", len(got))
	}
	if got[0].Priority != schema.ProposalPriorityQueued {
		t.Fatalf("expected queued priority, got %s", got[0].Priority)
	}
	if !strings.Contains(got[0].AffectedEntities, "artifact:art-1") || !strings.Contains(got[0].AffectedEntities, "project:proj-1") || !strings.Contains(got[0].AffectedEntities, "workspace:ws-1") {
		t.Fatalf("expected affected_entities to include artifact/project/workspace, got %s", got[0].AffectedEntities)
	}
	if rel, err := store.GetRelationshipByEndpoints(ctx, db, "artifact:art-1", "artifact:art-2", "related"); err != nil || rel == nil {
		t.Fatalf("expected related artifact relationship, rel=%+v err=%v", rel, err)
	}
}

func TestProcessDeep_ManualEscalationCreatesBlockingProposal(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "deep-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierDeep,
		Summary:          "Re-evaluate owner priorities",
		Details:          `{"kind":"fact","scope":"owner","scope_id":"owner-1","category":"priority","key":"focus"}`,
		EscalationReason: "manual",
		CreatedAt:        time.Now().UTC(),
	}

	worker.processDeep(ctx, payload)

	if len(got) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(got))
	}
	p := got[0]
	if p.SourceProcess != "deep_reflection" {
		t.Fatalf("expected SourceProcess deep_reflection, got %s", p.SourceProcess)
	}
	if p.Priority != schema.ProposalPriorityBlocking {
		t.Fatalf("expected blocking priority for deep reflection, got %s", p.Priority)
	}
	if !strings.Contains(p.AffectedEntities, "owner:owner-1") || !strings.Contains(p.AffectedEntities, "priority:focus") {
		t.Fatalf("expected affected_entities to include owner:owner-1 and priority:focus, got %s", p.AffectedEntities)
	}
}

func TestProcessDeep_ArtifactFailureCreatesBlockingProposal(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "deep-artifact-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierDeep,
		Summary:          "Artifact recovery needs operator input",
		Details:          `{"category":"artifact","scope":"owner","scope_id":"owner-1","artifact_id":"art-1","artifact_title":"Broken Export","workspace_id":"ws-1","project_id":"proj-1","operation":"export_artifact","result_status":"failed","failure_class":"renderer_missing","failure_code":"export_failed","renderer_key":"RawRenderer","lifecycle_state":"errored"}`,
		EscalationReason: "manual",
		CreatedAt:        time.Now().UTC(),
	}

	worker.processDeep(ctx, payload)

	if len(got) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(got))
	}
	p := got[0]
	if p.SourceProcess != "deep_reflection" {
		t.Fatalf("expected deep_reflection source, got %s", p.SourceProcess)
	}
	if p.Priority != schema.ProposalPriorityBlocking {
		t.Fatalf("expected blocking priority, got %s", p.Priority)
	}
	if !strings.Contains(p.AffectedEntities, "artifact:art-1") || !strings.Contains(p.AffectedEntities, "project:proj-1") || !strings.Contains(p.AffectedEntities, "workspace:ws-1") {
		t.Fatalf("expected affected_entities to include artifact/project/workspace, got %s", p.AffectedEntities)
	}
}

func TestProcessDeep_WorkspaceControlStateCreatesBlockingProposalWithoutMutation(t *testing.T) {
	ctx := context.Background()
	worker, wm, db := newTestWorker(t)
	defer db.Close()

	now := time.Now().UTC()
	if _, err := wm.UpsertWorkspace(ctx, schema.Workspace{
		ID:             "ws-1",
		Name:           "Primary Workspace",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/primary"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
	}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, "ws-1"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "deep-workspace-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierDeep,
		Summary:          "Expand workspace roots to include archive directory",
		Details:          `{"category":"workspace","scope":"workspace","scope_id":"ws-1","workspace_id":"ws-1","key":"local_roots","value":"[\"/workspace/primary\",\"/workspace/archive\"]"}`,
		EscalationReason: "manual",
		CreatedAt:        now,
	}

	worker.processDeep(ctx, payload)

	if len(got) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(got))
	}
	if got[0].SourceProcess != "deep_reflection" {
		t.Fatalf("expected deep_reflection source, got %s", got[0].SourceProcess)
	}
	if got[0].Priority != schema.ProposalPriorityBlocking {
		t.Fatalf("expected blocking priority, got %s", got[0].Priority)
	}
	if !strings.Contains(got[0].AffectedEntities, "workspace:ws-1") {
		t.Fatalf("expected affected_entities to include workspace:ws-1, got %s", got[0].AffectedEntities)
	}

	workspace, err := wm.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if len(workspace.LocalRoots) != 1 || workspace.LocalRoots[0] != "/workspace/primary" {
		t.Fatalf("expected workspace local_roots to remain unchanged, got %#v", workspace.LocalRoots)
	}
	if gotActive := wm.GetActiveWorkspaceID(ctx); gotActive != "ws-1" {
		t.Fatalf("expected active workspace selection to remain unchanged, got %q", gotActive)
	}
}

func TestProcessConsolidation_WorkspaceStateDoesNotCreateProposal(t *testing.T) {
	ctx := context.Background()
	worker, wm, db := newTestWorker(t)
	defer db.Close()

	now := time.Now().UTC()
	if _, err := wm.UpsertWorkspace(ctx, schema.Workspace{
		ID:             "ws-1",
		Name:           "Primary Workspace",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/primary"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
	}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	payload := schema.ReflectionPayload{
		ID:               "consolidation-workspace-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Maybe broaden workspace permissions",
		Details:          `{"category":"workspace","scope":"workspace","scope_id":"ws-1","workspace_id":"ws-1","key":"allowed_actions"}`,
		EscalationReason: "automatic",
		CreatedAt:        now,
	}

	worker.processConsolidation(ctx, payload)

	if len(got) != 0 {
		t.Fatalf("expected no proposals for consolidation workspace state, got %d", len(got))
	}
	workspace, err := wm.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if !workspace.AllowedActions.Read || !workspace.AllowedActions.Write {
		t.Fatalf("expected workspace allowed_actions to remain unchanged, got %+v", workspace.AllowedActions)
	}
}

func TestEmitInterruption_GuardAllowsOnlyOnePerCycle(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()
	memBus := bus.NewMemBus(db)
	if err := worker.Run(ctx, memBus); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ok1 := worker.EmitInterruption(ctx, schema.InterruptionAdvisory, "summary", "details", "")
	if !ok1 {
		t.Fatal("first EmitInterruption should succeed")
	}
	ok2 := worker.EmitInterruption(ctx, schema.InterruptionAdvisory, "second", "", "")
	if ok2 {
		t.Fatal("second EmitInterruption should be blocked by guard")
	}
	worker.ClearInterruptionGuard()
	ok3 := worker.EmitInterruption(ctx, schema.InterruptionAdvisory, "third", "", "")
	if !ok3 {
		t.Fatal("third EmitInterruption after clear should succeed")
	}

	events, err := store.EventsSince(ctx, db, 0, 10)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	var interruptCount int
	for _, ev := range events {
		if ev.Type == schema.FactSubconsciousInterruption {
			interruptCount++
		}
	}
	if interruptCount != 2 {
		t.Fatalf("expected 2 interruption events in log, got %d", interruptCount)
	}
}

func TestDetectSignificance(t *testing.T) {
	tests := []struct {
		name     string
		summary  string
		details  string
		wantHigh bool
	}{
		{"empty", "", "", false},
		{"conflict keyword", "Something happened", "there is a conflict with priorities", true},
		{"long summary", strings.Repeat("x", 201), "", true},
		{"long details", "Short", strings.Repeat("y", 501), true},
		{"neutral short", "Turn completed", "replied 50 chars", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := schema.ReflectionPayload{Summary: tt.summary, Details: tt.details}
			got := detectSignificance(p)
			high := got == "high"
			if high != tt.wantHigh {
				t.Errorf("detectSignificance() = %q, wantHigh=%v", got, tt.wantHigh)
			}
		})
	}
}

func TestRunPeriodicConsolidation_PersistsOwnerMemoryAndQueuesProposal(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS owners (
			id TEXT PRIMARY KEY,
			instance_id TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			handle TEXT NOT NULL,
			device_name TEXT NOT NULL DEFAULT '',
			secret_fingerprint TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("create owner table: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO owners (id, instance_id, name, handle, device_name, secret_fingerprint, created_at)
		VALUES ('owner-1', 'inst-1', 'Owner', 'owner', 'device', 'fp', ?)
	`, now); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO navi_chats (chat_id, title, status, visibility, owner_id, created_at, updated_at, last_message_at)
		VALUES ('sess-1', 'Consolidation Chat', 'active', 'private', 'owner-1', ?, ?, ?)
	`, now, now, now); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO navi_chat_messages (message_id, chat_id, role, content, created_at)
		VALUES
			('m1', 'sess-1', 'user', 'I prefer short status updates and this should be my default.', ?),
			('m2', 'sess-1', 'user', 'The most important priority is shipping Memory v2 this week.', ?)
	`, now, now); err != nil {
		t.Fatalf("insert chat messages: %v", err)
	}
	if _, err := store.SaveMemory(ctx, db, schema.Memory{
		Scope:   "chat",
		ScopeID: "sess-1",
		Summary: "Discussed owner preferences for status updates.",
		Source:  "shallow_reflection",
	}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	var proposals []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		proposals = append(proposals, p)
		return nil
	})

	worker.runPeriodicConsolidation(ctx)

	memories, err := store.ListMemories(ctx, db, "owner", "owner-1", 10)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	var consolidated *schema.Memory
	for i := range memories {
		if memories[i].Source == "consolidation" && memories[i].ID == "consolidation:sess-1" {
			consolidated = &memories[i]
			break
		}
	}
	if consolidated == nil {
		t.Fatal("expected periodic consolidation to persist owner-scoped consolidated memory")
	}
	if len(consolidated.Embedding) == 0 {
		t.Fatal("expected consolidated memory to carry knowledge embedding")
	}

	links, err := store.ListKnowledgeLinks(ctx, db, "memory", consolidated.ID, 10)
	if err != nil {
		t.Fatalf("ListKnowledgeLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected consolidated memory to link into the knowledge graph")
	}

	if len(proposals) == 0 {
		t.Fatal("expected periodic consolidation to queue an owner-tier proposal")
	}
	if proposals[0].SourceProcess != "consolidation" {
		t.Fatalf("expected consolidation proposal source, got %s", proposals[0].SourceProcess)
	}

	var affected []string
	if err := json.Unmarshal([]byte(proposals[0].AffectedEntities), &affected); err != nil {
		t.Fatalf("unmarshal affected_entities: %v", err)
	}
	if len(affected) == 0 {
		t.Fatal("expected affected_entities on periodic consolidation proposal")
	}
}

func TestProcessShallow_CapturesPreferenceSignals(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	payload := schema.ReflectionPayload{
		ID:               "signal-1",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierShallow,
		Summary:          "Turn completed",
		Details:          `{"summary":"Turn completed","preference_signals":[{"scope":"owner","scope_id":"owner-1","chat_id":"sess-1","trait":"verbosity","target_value":0.20,"evidence_class":"explicit_correction","signal_strength":1.0,"summary":"User requested concise replies","immediate":true}]}`,
		CreatedAt:        time.Now().UTC(),
	}

	worker.processShallow(ctx, payload)

	signals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, "owner-1", []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusCaptured}, 10)
	if err != nil {
		t.Fatalf("list preference signals: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 preference signal, got %d", len(signals))
	}
	if signals[0].Trait != "verbosity" {
		t.Fatalf("expected verbosity signal, got %+v", signals[0])
	}
}

func TestProcessConsolidation_AppliesRelationshipProfileFromPreferenceSignals(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	for i := 0; i < 3; i++ {
		if err := store.SavePreferenceSignal(ctx, db, schema.PreferenceSignal{
			SignalID:       fmt.Sprintf("signal-%d", i),
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-1",
			Trait:          "verbosity",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested concise replies",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}); err != nil {
			t.Fatalf("save preference signal: %v", err)
		}
	}

	worker.processConsolidation(ctx, schema.ReflectionPayload{
		ID:               "consolidate-signals",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Automatic reflection consolidation",
		EscalationReason: "automatic",
		Details:          `{"scope":"owner","scope_id":"owner-1"}`,
		CreatedAt:        time.Now().UTC(),
	})

	snapshot, err := experience.LoadConfigurationSnapshot(ctx, db, "owner-1")
	if err != nil {
		t.Fatalf("load configuration snapshot: %v", err)
	}
	if got := snapshot.RelationshipProfile.TraitSignalCount["verbosity"]; got != 3 {
		t.Fatalf("expected verbosity signal count 3, got %d", got)
	}
	if got := snapshot.RelationshipProfile.TraitEstimates["verbosity"]; got >= experience.TraitDefinitions()["verbosity"].DefaultValue {
		t.Fatalf("expected verbosity estimate to move toward concise target, got %.2f", got)
	}
	signals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, "owner-1", []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusApplied}, 10)
	if err != nil {
		t.Fatalf("list applied preference signals: %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("expected 3 applied signals, got %d", len(signals))
	}
}

func TestProcessConsolidation_DoesNotDoubleCountAppliedSignals(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := store.SavePreferenceSignal(ctx, db, schema.PreferenceSignal{
			SignalID:       fmt.Sprintf("signal-%d", i),
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-1",
			Trait:          "verbosity",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested concise replies",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			t.Fatalf("save preference signal: %v", err)
		}
	}

	payload := schema.ReflectionPayload{
		ID:               "consolidate-signals",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Automatic reflection consolidation",
		EscalationReason: "automatic",
		Details:          `{"scope":"owner","scope_id":"owner-1"}`,
		CreatedAt:        now,
	}
	worker.processConsolidation(ctx, payload)
	worker.processConsolidation(ctx, payload)

	snapshot, err := experience.LoadConfigurationSnapshot(ctx, db, "owner-1")
	if err != nil {
		t.Fatalf("load configuration snapshot: %v", err)
	}
	if got := snapshot.RelationshipProfile.TraitSignalCount["verbosity"]; got != 3 {
		t.Fatalf("expected verbosity signal count to remain 3 after repeated consolidation, got %d", got)
	}
	signals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, "owner-1", []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusApplied}, 10)
	if err != nil {
		t.Fatalf("list applied preference signals: %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("expected 3 applied signals after repeated consolidation, got %d", len(signals))
	}
}

func TestProcessConsolidation_EscalatesSignificantStableTraitAdaptation(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	var got []schema.Proposal
	worker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		got = append(got, p)
		return nil
	})

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := store.SavePreferenceSignal(ctx, db, schema.PreferenceSignal{
			SignalID:       fmt.Sprintf("stable-signal-%d", i),
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-1",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			t.Fatalf("save preference signal: %v", err)
		}
	}

	worker.processConsolidation(ctx, schema.ReflectionPayload{
		ID:               "consolidate-stable-signals",
		RuntimeSessionID: "sess-1",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Automatic reflection consolidation",
		EscalationReason: "automatic",
		Details:          `{"scope":"owner","scope_id":"owner-1"}`,
		CreatedAt:        now,
	})

	signals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, "owner-1", []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusPersisted}, 10)
	if err != nil {
		t.Fatalf("list persisted preference signals: %v", err)
	}
	if len(signals) != 5 {
		t.Fatalf("expected 5 persisted stable-trait signals, got %d", len(signals))
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 deep-reflection proposal for stable-trait adaptation, got %d", len(got))
	}
	if got[0].SourceProcess != "deep_reflection" {
		t.Fatalf("expected deep_reflection proposal source, got %s", got[0].SourceProcess)
	}
	if got[0].Priority != schema.ProposalPriorityBlocking {
		t.Fatalf("expected blocking proposal priority, got %s", got[0].Priority)
	}
	if !strings.Contains(got[0].Rationale, "experience_adaptation") {
		t.Fatalf("expected proposal rationale to include experience_adaptation details, got %s", got[0].Rationale)
	}

	snapshotEvent, err := store.LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, "sess-1")
	if err != nil {
		t.Fatalf("LatestEventByTypeAndCorrelationID(snapshot): %v", err)
	}
	snapshotPayload := decodeExperienceSnapshotPayloadForTest(t, snapshotEvent)
	if snapshotPayload.Trigger != experience.SnapshotTriggerProposalWorthy {
		t.Fatalf("expected snapshot trigger %q, got %q", experience.SnapshotTriggerProposalWorthy, snapshotPayload.Trigger)
	}
	if snapshotPayload.OwnerID != "owner-1" {
		t.Fatalf("expected snapshot owner_id owner-1, got %q", snapshotPayload.OwnerID)
	}
	if snapshotPayload.ChatID != "sess-1" {
		t.Fatalf("expected snapshot chat_id sess-1, got %q", snapshotPayload.ChatID)
	}
	if snapshotPayload.ExperienceMode != "navi" {
		t.Fatalf("expected snapshot experience_mode navi, got %q", snapshotPayload.ExperienceMode)
	}
	var effective experience.EffectivePersonaState
	if err := json.Unmarshal([]byte(snapshotPayload.EffectiveStateJSON), &effective); err != nil {
		t.Fatalf("unmarshal effective state: %v", err)
	}
	if effective.StateID == "" {
		t.Fatal("expected proposal-worthy snapshot to include an effective state id")
	}
	if len(effective.ResolvedTraits) == 0 {
		t.Fatal("expected proposal-worthy snapshot to include resolved traits")
	}
	if snapshotPayload.SourceStateID == "" || snapshotPayload.CompiledPayloadID == "" {
		t.Fatalf("expected proposal-worthy snapshot identifiers to be populated, got %+v", snapshotPayload)
	}
}

func TestProcessConsolidation_ProposalWorthySnapshotFallsBackToLatestSignalSession(t *testing.T) {
	ctx := context.Background()
	worker, _, db := newTestWorker(t)
	defer db.Close()

	now := time.Now().UTC()
	signals := []schema.PreferenceSignal{
		{
			SignalID:       "stable-fallback-0",
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-older",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now.Add(-5 * time.Minute),
			UpdatedAt:      now.Add(-5 * time.Minute),
		},
		{
			SignalID:       "stable-fallback-1",
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-older",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now.Add(-4 * time.Minute),
			UpdatedAt:      now.Add(-4 * time.Minute),
		},
		{
			SignalID:       "stable-fallback-2",
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-older",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now.Add(-3 * time.Minute),
			UpdatedAt:      now.Add(-3 * time.Minute),
		},
		{
			SignalID:       "stable-fallback-3",
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-newer",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now.Add(-2 * time.Minute),
			UpdatedAt:      now.Add(-2 * time.Minute),
		},
		{
			SignalID:       "stable-fallback-4",
			Scope:          experience.ConfigScopeOwner,
			ScopeID:        "owner-1",
			ChatID:         "sess-newer",
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Summary:        "User requested gentler communication",
			Status:         schema.PreferenceSignalStatusCaptured,
			CreatedAt:      now.Add(-1 * time.Minute),
			UpdatedAt:      now.Add(-1 * time.Minute),
		},
	}
	for _, signal := range signals {
		if err := store.SavePreferenceSignal(ctx, db, signal); err != nil {
			t.Fatalf("save preference signal %s: %v", signal.SignalID, err)
		}
	}

	worker.processConsolidation(ctx, schema.ReflectionPayload{
		ID:               "consolidate-stable-signals-fallback",
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Automatic reflection consolidation",
		EscalationReason: "automatic",
		Details:          `{"scope":"owner","scope_id":"owner-1"}`,
		CreatedAt:        now,
	})

	olderSnapshot, err := store.LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, "sess-older")
	if err != nil {
		t.Fatalf("LatestEventByTypeAndCorrelationID(sess-older): %v", err)
	}
	if olderSnapshot != nil {
		t.Fatalf("expected no snapshot for older chat anchor, got %+v", olderSnapshot)
	}

	newerSnapshot, err := store.LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, "sess-newer")
	if err != nil {
		t.Fatalf("LatestEventByTypeAndCorrelationID(sess-newer): %v", err)
	}
	payload := decodeExperienceSnapshotPayloadForTest(t, newerSnapshot)
	if payload.Trigger != experience.SnapshotTriggerProposalWorthy {
		t.Fatalf("expected snapshot trigger %q, got %q", experience.SnapshotTriggerProposalWorthy, payload.Trigger)
	}
	if payload.ChatID != "sess-newer" {
		t.Fatalf("expected fallback snapshot chat_id sess-newer, got %q", payload.ChatID)
	}
	if payload.OwnerID != "owner-1" {
		t.Fatalf("expected fallback snapshot owner_id owner-1, got %q", payload.OwnerID)
	}
}
