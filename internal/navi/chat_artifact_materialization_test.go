package navi

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/artifact"
	"github.com/ceoai/navi/internal/blob"
	"github.com/ceoai/navi/internal/navi/orchestration"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestArtifactPromotionRequestResolvesPriorAssistantHTML(t *testing.T) {
	thread := &ChatThread{Messages: []ChatMessage{
		{ID: "u1", Role: "user", Content: "make a thing"},
		{ID: "a1", Role: "assistant", Content: "Here it is:\n```html\n<!doctype html><html><head><title>Temporal Star</title></head><body><canvas></canvas><script>draw()</script></body></html>\n```"},
		{ID: "u2", Role: "user", Content: "Save it as an artifact"},
	}}

	if !looksLikeExplicitArtifactPromotion("Save it as an artifact") {
		t.Fatal("expected explicit artifact promotion intent")
	}
	candidate, ok := latestAssistantArtifactCandidate(thread)
	if !ok {
		t.Fatal("expected prior assistant work product candidate")
	}
	if candidate.SourceMessage.ID != "a1" {
		t.Fatalf("source message = %q, want a1", candidate.SourceMessage.ID)
	}
	if candidate.Subtype != "html" || !strings.Contains(candidate.Content, "<canvas>") {
		t.Fatalf("unexpected candidate: subtype=%q content=%q", candidate.Subtype, candidate.Content)
	}
}

func TestExecuteExplicitArtifactPromotionCreatesLinkedArtifact(t *testing.T) {
	ctx := context.Background()
	db, svc, cleanup := newChatArtifactTestDB(t)
	defer cleanup()
	chat, thread := seedChatArtifactThread([]ChatMessage{
		{Role: "user", Content: "Create a glowing star."},
		{Role: "assistant", Content: "```html\n<!doctype html><html><head><title>Temporal Star</title></head><body><canvas id=\"star\"></canvas><script>requestAnimationFrame(()=>{})</script></body></html>\n```"},
		{Role: "user", Content: "Save it as an artifact"},
	})
	run := naviruntime.NewRun("runtime-artifact", string(ExperienceModeStandard))
	run.ChatID = string(chat.ID)
	run.RunID = "run-promote"
	loop := NewAgentLoop(LoopConfig{
		DB:              db,
		ArtifactService: svc,
		ResolveOwnerID:  func(context.Context, string) string { return "owner-1" },
	})

	res, handled, err := loop.executeExplicitArtifactPromotion(ctx, run, thread, "Save it as an artifact", ExperienceModeStandard)
	if err != nil {
		t.Fatalf("executeExplicitArtifactPromotion: %v", err)
	}
	if !handled || res == nil || !res.Completed {
		t.Fatalf("expected handled completed result, handled=%v res=%+v", handled, res)
	}
	if len(run.ArtifactIDs) != 1 || run.MainArtifactID == "" {
		t.Fatalf("expected run artifact refs, main=%q ids=%v", run.MainArtifactID, run.ArtifactIDs)
	}
	if !strings.Contains(res.FinalContent, run.MainArtifactID) {
		t.Fatalf("final content should reference artifact id, got %q", res.FinalContent)
	}
	artifacts, err := store.ListArtifacts(ctx, db, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Subtype != "html" {
		t.Fatalf("artifact subtype = %q, want html", artifacts[0].Subtype)
	}
	var conversationRefs, messageRefs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_references WHERE artifact_id = ? AND source_kind = 'conversation' AND source_id = ?`, run.MainArtifactID, string(chat.ID)).Scan(&conversationRefs); err != nil {
		t.Fatalf("conversation refs query: %v", err)
	}
	sourceMessageID := string(thread.Messages[1].ID)
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_references WHERE artifact_id = ? AND source_kind = 'message' AND source_id = ?`, run.MainArtifactID, sourceMessageID).Scan(&messageRefs); err != nil {
		t.Fatalf("message refs query: %v", err)
	}
	if conversationRefs != 1 || messageRefs != 1 {
		t.Fatalf("expected chat/message references, conversation=%d message=%d", conversationRefs, messageRefs)
	}
}

func TestAutoMaterializeSubstantialHTMLReplyCreatesArtifact(t *testing.T) {
	ctx := context.Background()
	db, svc, cleanup := newChatArtifactTestDB(t)
	defer cleanup()
	chat, thread := seedChatArtifactThread([]ChatMessage{
		{Role: "user", Content: "Generate a self-contained HTML clock."},
	})
	run := naviruntime.NewRun("runtime-auto-artifact", string(ExperienceModeStandard))
	run.ChatID = string(chat.ID)
	run.RunID = "run-auto"
	htmlReply := "Here is the file:\n```html\n<!doctype html><html><head><title>Clock</title></head><body><canvas></canvas><script>" + strings.Repeat("tick();", 80) + "</script></body></html>\n```"
	loop := NewAgentLoop(LoopConfig{
		DB:              db,
		ArtifactService: svc,
		ResolveOwnerID:  func(context.Context, string) string { return "owner-1" },
	})
	note, err := loop.maybeAutoMaterializeChatReply(ctx, run, thread, htmlReply)
	if err != nil {
		t.Fatalf("maybeAutoMaterializeChatReply: %v", err)
	}
	if note == "" || run.MainArtifactID == "" || len(run.ArtifactIDs) != 1 {
		t.Fatalf("expected auto artifact note and run refs, note=%q main=%q ids=%v", note, run.MainArtifactID, run.ArtifactIDs)
	}
	artifacts, err := store.ListArtifacts(ctx, db, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
}

func TestAutoMaterializationSkipsShortChatReply(t *testing.T) {
	ctx := context.Background()
	db, svc, cleanup := newChatArtifactTestDB(t)
	defer cleanup()
	chat, thread := seedChatArtifactThread([]ChatMessage{{Role: "user", Content: "hi"}})
	run := naviruntime.NewRun("runtime-short", string(ExperienceModeStandard))
	run.ChatID = string(chat.ID)
	run.RunID = "run-short"
	loop := NewAgentLoop(LoopConfig{DB: db, ArtifactService: svc, ResolveOwnerID: func(context.Context, string) string { return "owner-1" }})
	note, err := loop.maybeAutoMaterializeChatReply(ctx, run, thread, "Sure, I can help with that.")
	if err != nil {
		t.Fatalf("maybeAutoMaterializeChatReply: %v", err)
	}
	if note != "" || len(run.ArtifactIDs) != 0 {
		t.Fatalf("short reply should not materialize, note=%q ids=%v", note, run.ArtifactIDs)
	}
	artifacts, err := store.ListArtifacts(ctx, db, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(artifacts))
	}
}

func TestExplicitPromotionWithoutPriorWorkProductClarifies(t *testing.T) {
	ctx := context.Background()
	db, svc, cleanup := newChatArtifactTestDB(t)
	defer cleanup()
	chat, thread := seedChatArtifactThread([]ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "Hi."},
		{Role: "user", Content: "Save it as an artifact"},
	})
	run := naviruntime.NewRun("runtime-clarify", string(ExperienceModeStandard))
	run.ChatID = string(chat.ID)
	run.RunID = "run-clarify"
	loop := NewAgentLoop(LoopConfig{DB: db, ArtifactService: svc, ResolveOwnerID: func(context.Context, string) string { return "owner-1" }})
	res, handled, err := loop.executeExplicitArtifactPromotion(ctx, run, thread, "Save it as an artifact", ExperienceModeStandard)
	if err != nil {
		t.Fatalf("executeExplicitArtifactPromotion: %v", err)
	}
	if !handled || res == nil || !strings.Contains(res.FinalContent, "don't see a prior generated work product") {
		t.Fatalf("expected clarification, handled=%v res=%+v", handled, res)
	}
	if len(run.ArtifactIDs) != 0 {
		t.Fatalf("ambiguous promotion should not create run refs, got %v", run.ArtifactIDs)
	}
	artifacts, err := store.ListArtifacts(ctx, db, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(artifacts))
	}
}

func TestArtifactPromptGuidanceSurfacesCoreArtifactCreate(t *testing.T) {
	loop := NewAgentLoop(LoopConfig{})
	req := orchestration.CanonicalRunRequest{
		UserMessage: "Generate a full HTML widget",
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:      true,
			AllowToolCalls: true,
		},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:   orchestration.CapabilitySurfaceRuntime,
			ToolNames: []string{"skill.core-artifact.create"},
		},
	}
	compiled, err := loop.runtimeInstructionCompileRequest(context.Background(), req, orchestration.ContextPack{})
	if err != nil {
		t.Fatalf("runtimeInstructionCompileRequest: %v", err)
	}
	joined := strings.Join(compiled.CapabilitySurface.Additional, "\n")
	if !strings.Contains(joined, "substantial durable work product") {
		t.Fatalf("expected artifact guidance, got %q", joined)
	}
}

func newChatArtifactTestDB(t *testing.T) (*sql.DB, *artifact.Service, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	if err := store.CreateOwner(ctx, db, store.Owner{ID: "owner-1", InstanceID: "inst-1", Name: "Owner", Handle: "owner", CreatedAt: now}); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-1",
		Name:           "Chat Artifact Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/chat-artifact"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := store.SaveConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        "active_workspace_id",
		Scope:     "owner",
		ScopeID:   "active",
		Key:       "active_workspace_id",
		Value:     "ws-1",
		Source:    "test",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConfigurationEntry: %v", err)
	}
	root := t.TempDir()
	blobStore, err := blob.NewFilesystemStore(filepath.Join(root, "blob"))
	if err != nil {
		t.Fatalf("blob.NewFilesystemStore: %v", err)
	}
	return db, artifact.NewService(db, blobStore), func() { _ = db.Close() }
}

func seedChatArtifactThread(messages []ChatMessage) (*Chat, *ChatThread) {
	wsID := ID("ws-1")
	chat := &Chat{
		ID:          "chat-artifact-test",
		OwnerID:     "owner-1",
		WorkspaceID: &wsID,
		Title:       "Temporal Star",
	}
	out := make([]ChatMessage, 0, len(messages))
	for i, msg := range messages {
		if msg.ID == "" {
			msg.ID = ID(fmt.Sprintf("msg-%d", i+1))
		}
		msg.ChatID = chat.ID
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = time.Date(2026, 6, 5, 10, i, 0, 0, time.UTC)
		}
		out = append(out, msg)
	}
	return chat, &ChatThread{Chat: *chat, Messages: out}
}
