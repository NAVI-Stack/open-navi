package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/navi/inference"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	corestore "github.com/ceoai/navi/internal/store"
)

func TestAssistantEventVisibilityInternalHeartbeatSession(t *testing.T) {
	if got := assistantEventVisibility(schema.RuntimeSessionKindInternal, navi.HeartbeatAutoRuntimeSessionID); got != schema.VisibilityOperator {
		t.Fatalf("assistantEventVisibility(heartbeat) = %q, want %q", got, schema.VisibilityOperator)
	}
}

func TestAssistantEventVisibilityRegularSession(t *testing.T) {
	if got := assistantEventVisibility(schema.RuntimeSessionKindUser, "sess-user"); got != schema.VisibilityUser {
		t.Fatalf("assistantEventVisibility(user) = %q, want %q", got, schema.VisibilityUser)
	}
}

func TestRunEventVisibilityInternalHeartbeatSession(t *testing.T) {
	if got := runEventVisibility(schema.RuntimeSessionKindInternal, navi.HeartbeatAutoRuntimeSessionID); got != schema.VisibilityOperator {
		t.Fatalf("runEventVisibility(heartbeat) = %q, want %q", got, schema.VisibilityOperator)
	}
}

func TestRuntimeSessionEventVisibilityInternalHeartbeatSession(t *testing.T) {
	if got := runtimeSessionEventVisibility(schema.RuntimeSessionKindInternal, navi.HeartbeatAutoRuntimeSessionID); got != schema.VisibilityOperator {
		t.Fatalf("runtimeSessionEventVisibility(heartbeat) = %q, want %q", got, schema.VisibilityOperator)
	}
}

func TestCompleteRunPersistsArtifactRefsAndAssistantMetadata(t *testing.T) {
	db := prepareTestDB(t)
	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{
		ID:             "chat-artifact-metadata",
		OwnerID:        "owner-1",
		Title:          "Artifact Metadata",
		ExperienceMode: string(navi.ExperienceModeStandard),
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	run := naviruntime.NewRun("runtime-artifact-metadata", string(navi.ExperienceModeStandard))
	run.RunID = "run-artifact-metadata"
	run.ChatID = string(chat.ID)
	run.MainArtifactID = "artifact-1"
	run.ArtifactIDs = []string{"artifact-1", "artifact-2"}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.CompleteRun(ctx, run, "Saved as artifact.", string(navi.ExperienceModeStandard), ""); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	loaded, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if loaded.MainArtifactID != "artifact-1" || len(loaded.ArtifactIDs) != 2 {
		t.Fatalf("persisted artifact refs missing, main=%q ids=%v", loaded.MainArtifactID, loaded.ArtifactIDs)
	}
	thread, err := store.GetChatWithMessages(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	last := thread.Messages[len(thread.Messages)-1]
	b, _ := json.Marshal(last.Metadata)
	if !strings.Contains(string(b), "artifact-1") || !strings.Contains(string(b), "artifact-2") {
		t.Fatalf("assistant metadata missing artifact refs: %s", b)
	}
}

func TestInternalSessionMessagesNotPersistedInConversationTable(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	item := &naviruntime.InboxItem{
		Content:       "internal heartbeat prompt",
		SourceChannel: "heartbeat",
		ReceivedAt:    time.Now().UTC(),
	}
	if _, err := store.AcceptMessage(ctx, navi.HeartbeatAutoRuntimeSessionID, item, string(navi.ExperienceModeStandard)); err != nil {
		t.Fatalf("accept internal message: %v", err)
	}

	run := &naviruntime.RunState{
		RunID:            "run-heartbeat-1",
		RuntimeSessionID: navi.HeartbeatAutoRuntimeSessionID,
		StartedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		CurrentPhase:     naviruntime.RunPhaseDecide,
	}
	if _, err := store.AppendAssistantMessage(ctx, run, "HEARTBEAT_OK", string(navi.ExperienceModeStandard), item.ID); err != nil {
		t.Fatalf("append internal assistant message: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ?`, navi.HeartbeatAutoRuntimeSessionID).Scan(&count); err != nil {
		t.Fatalf("count internal chat transcript rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no chat transcript rows for internal runtime, got %d", count)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chats WHERE chat_id = ?`, navi.HeartbeatAutoRuntimeSessionID).Scan(&count); err != nil {
		t.Fatalf("count internal heartbeat chats: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected heartbeat runtime not to create a chat, got %d rows", count)
	}

	var kind string
	if err := db.QueryRowContext(ctx, `SELECT kind FROM runtime_sessions WHERE runtime_session_id = ?`, navi.HeartbeatAutoRuntimeSessionID).Scan(&kind); err != nil {
		t.Fatalf("load internal runtime session kind: %v", err)
	}
	if kind != string(navi.RuntimeSessionKindInternal) {
		t.Fatalf("expected internal runtime session kind, got %q", kind)
	}
}

func TestTypedInternalSessionMessagesNotPersistedInConversationTable(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	runtimeSessionID := "sess-internal-typed"
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_sessions (runtime_session_id, kind, status, experience_mode, source_channel, started_at, last_active_at)
		VALUES (?, ?, 'active', ?, 'system', ?, ?)
	`, runtimeSessionID, string(schema.RuntimeSessionKindInternal), string(navi.ExperienceModeStandard), now, now); err != nil {
		t.Fatalf("seed typed internal runtime session: %v", err)
	}

	item := &naviruntime.InboxItem{
		Content:       "system-only prompt",
		SourceChannel: "system",
		ReceivedAt:    now,
	}
	if _, err := store.AcceptMessage(ctx, runtimeSessionID, item, string(navi.ExperienceModeStandard)); err != nil {
		t.Fatalf("accept typed internal message: %v", err)
	}

	run := &naviruntime.RunState{
		RunID:            "run-typed-internal-1",
		RuntimeSessionID: runtimeSessionID,
		StartedAt:        now,
		UpdatedAt:        now,
		CurrentPhase:     naviruntime.RunPhaseDecide,
	}
	if _, err := store.AppendAssistantMessage(ctx, run, "SYSTEM_OK", string(navi.ExperienceModeStandard), item.ID); err != nil {
		t.Fatalf("append typed internal assistant message: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ?`, runtimeSessionID).Scan(&count); err != nil {
		t.Fatalf("count typed internal chat transcript rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no navi_chat_messages rows for typed internal runtime session, got %d", count)
	}
}

func TestAcceptMessageSuppressesDuplicateIdempotencyKeyAfterConsumption(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	runtimeSessionID := "sess-duplicate-idempotency"
	key := "telegram:100:44"

	first := naviruntime.NewInboxItem(runtimeSessionID, "hello", "telegram")
	first.IdempotencyKey = key
	accepted, err := store.AcceptMessage(ctx, runtimeSessionID, first, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage first: %v", err)
	}
	if err := store.MarkInboxConsumed(ctx, accepted.ID, "run-1"); err != nil {
		t.Fatalf("MarkInboxConsumed: %v", err)
	}

	duplicate := naviruntime.NewInboxItem(runtimeSessionID, "hello", "telegram")
	duplicate.IdempotencyKey = key
	acceptedDuplicate, err := store.AcceptMessage(ctx, runtimeSessionID, duplicate, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage duplicate: %v", err)
	}
	if acceptedDuplicate.Status != naviruntime.InboxStatusSuperseded {
		t.Fatalf("duplicate status = %q, want %q", acceptedDuplicate.Status, naviruntime.InboxStatusSuperseded)
	}
	if acceptedDuplicate.QueueAction != "supersede" {
		t.Fatalf("duplicate queue action = %q, want supersede", acceptedDuplicate.QueueAction)
	}
	if acceptedDuplicate.MergedIntoID != accepted.ID {
		t.Fatalf("duplicate target = %q, want %q", acceptedDuplicate.MergedIntoID, accepted.ID)
	}

	var inboxRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_inbox WHERE idempotency_key = ?`, key).Scan(&inboxRows); err != nil {
		t.Fatalf("count inbox rows: %v", err)
	}
	if inboxRows != 1 {
		t.Fatalf("expected one persisted inbox row for duplicate idempotency key, got %d", inboxRows)
	}
	pending, err := store.ListPendingItems(ctx, runtimeSessionID, 10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected duplicate not to create pending work, got %#v", pending)
	}
}

func TestAcceptMessageWritesChatTranscriptRuntimeInboxAndHistory(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	chatID := "chat-intake"
	runtimeSessionID := "runtime-intake"
	if _, err := store.CreateChat(ctx, navi.CreateChatInput{
		ID:             chatID,
		Title:          "Intake Chat",
		OwnerID:        "owner-intake",
		WorkspaceID:    "workspace-intake",
		ProjectID:      "project-intake",
		ExperienceMode: string(navi.ExperienceModeStandard),
	}); err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := store.CreateRuntimeSession(ctx, navi.CreateRuntimeSessionInput{
		ID:             runtimeSessionID,
		OwnerID:        "owner-intake",
		WorkspaceID:    "workspace-intake",
		ProjectID:      "project-intake",
		Kind:           navi.RuntimeSessionKindUser,
		ExperienceMode: string(navi.ExperienceModeStandard),
		SourceChannel:  "app",
	}); err != nil {
		t.Fatalf("CreateRuntimeSession: %v", err)
	}
	if err := store.AttachChatToRuntimeSession(ctx, runtimeSessionID, chatID, navi.RuntimeSessionChatPrimary); err != nil {
		t.Fatalf("AttachChatToRuntimeSession: %v", err)
	}

	item := naviruntime.NewInboxItem(runtimeSessionID, "hello from chat", "app")
	item.ChatID = chatID
	item.RuntimeSessionID = runtimeSessionID
	accepted, err := store.AcceptMessage(ctx, runtimeSessionID, item, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage: %v", err)
	}
	if accepted.ChatID != chatID {
		t.Fatalf("accepted chat id = %q, want %q", accepted.ChatID, chatID)
	}
	if accepted.RuntimeSessionID != runtimeSessionID {
		t.Fatalf("accepted runtime session id = %q, want %q", accepted.RuntimeSessionID, runtimeSessionID)
	}

	var inboxChatID, inboxRuntimeSessionID string
	if err := db.QueryRowContext(ctx, `
		SELECT chat_id, runtime_session_id
		FROM navi_inbox
		WHERE inbox_item_id = ?
	`, accepted.ID).Scan(&inboxChatID, &inboxRuntimeSessionID); err != nil {
		t.Fatalf("load inbox identities: %v", err)
	}
	if inboxChatID != chatID || inboxRuntimeSessionID != runtimeSessionID {
		t.Fatalf("inbox identities chat=%q runtime=%q", inboxChatID, inboxRuntimeSessionID)
	}

	var chatMessageRuntimeID string
	if err := db.QueryRowContext(ctx, `
		SELECT runtime_session_id
		FROM navi_chat_messages
		WHERE chat_id = ? AND inbox_item_id = ?
	`, chatID, accepted.ID).Scan(&chatMessageRuntimeID); err != nil {
		t.Fatalf("load chat transcript message: %v", err)
	}
	if chatMessageRuntimeID != runtimeSessionID {
		t.Fatalf("chat message runtime_session_id = %q", chatMessageRuntimeID)
	}

	// navi_messages was dropped in Session B; intake writes only to navi_chat_messages.
	events, err := corestore.EventsByCorrelationID(ctx, db, chatID)
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected message intake to write history/audit events")
	}
	if events[0].CorrelationID != chatID {
		t.Fatalf("event correlation = %q, want chat id %q", events[0].CorrelationID, chatID)
	}
	rawPayload, err := json.Marshal(events[0].Payload)
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	var payload schema.MessageReceivedPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal message.received payload: %v", err)
	}
	if payload.ChatID != chatID || payload.RuntimeSessionID != runtimeSessionID {
		t.Fatalf("payload identities chat=%q runtime=%q", payload.ChatID, payload.RuntimeSessionID)
	}
}

func TestAcceptMessagePreservesDistinctMergedUserMessagesInSessionHistory(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	runtimeSessionID := "sess-merged-history"
	if _, err := store.CreateChat(ctx, navi.CreateChatInput{ID: runtimeSessionID, Title: "Merged History"}); err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	now := time.Now().UTC()

	first := naviruntime.NewInboxItem(runtimeSessionID, "Quick test message 1", "telegram")
	first.ReceivedAt = now
	acceptedFirst, err := store.AcceptMessage(ctx, runtimeSessionID, first, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage first: %v", err)
	}

	second := naviruntime.NewInboxItem(runtimeSessionID, "Quick test message 2", "telegram")
	second.ReceivedAt = now.Add(500 * time.Millisecond)
	second.QueueAction = naviruntime.QueueActionMerge
	second.Status = naviruntime.InboxStatusMerged
	second.MergedIntoID = acceptedFirst.ID
	second.ClassifiedReason = "bursty same-source message merged into the already pending runtime session turn"
	second.Confidence = 0.8
	acceptedSecond, err := store.AcceptMessage(ctx, runtimeSessionID, second, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage second: %v", err)
	}
	if acceptedSecond.Status != naviruntime.InboxStatusMerged {
		t.Fatalf("second status = %q, want %q", acceptedSecond.Status, naviruntime.InboxStatusMerged)
	}

	if err := store.MarkInboxConsumed(ctx, acceptedFirst.ID, "run-merged-history"); err != nil {
		t.Fatalf("MarkInboxConsumed first: %v", err)
	}

	thread, err := store.GetChatWithMessages(ctx, runtimeSessionID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if thread == nil {
		t.Fatal("expected chat thread")
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("expected 2 visible user messages, got %d: %#v", len(thread.Messages), thread.Messages)
	}
	if thread.Messages[0].Role != "user" || thread.Messages[0].Content != "Quick test message 1" {
		t.Fatalf("first visible message = %#v", thread.Messages[0])
	}
	if thread.Messages[1].Role != "user" || thread.Messages[1].Content != "Quick test message 2" {
		t.Fatalf("second visible message = %#v", thread.Messages[1])
	}

	pending, err := store.ListPendingItems(ctx, runtimeSessionID, 10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending items after consumption, got %#v", pending)
	}

	var mergedInboxContent string
	if err := db.QueryRowContext(ctx, `
		SELECT content
		FROM navi_inbox
		WHERE inbox_item_id = ?
	`, acceptedFirst.ID).Scan(&mergedInboxContent); err != nil {
		t.Fatalf("load merged inbox content: %v", err)
	}
	if mergedInboxContent != "Quick test message 1\nQuick test message 2" {
		t.Fatalf("merged inbox content = %q", mergedInboxContent)
	}
}

func TestRunScratchpadPersistsDuringRunAndClearsOnCompletion(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Run Scratchpad"})
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
	runtimeSessionID := string(chat.ID)

	run := naviruntime.NewRun(runtimeSessionID, string(navi.ExperienceModeStandard))
	run.SetStatus(schema.RunStatusActive)
	run.SetScratchpadValue("goal", "draft summary")
	run.SetScratchpadValue("inbox_item_id", "inbox-1")

	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	loadedRun, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if loadedRun == nil || loadedRun.Scratchpad["goal"] != "draft summary" {
		t.Fatalf("expected persisted run scratchpad, got %+v", loadedRun)
	}

	cp := naviruntime.NewCheckpoint(run)
	if err := store.SaveCheckpoint(ctx, cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	loadedCheckpoint, err := store.LoadCheckpoint(ctx, run.RunID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if loadedCheckpoint == nil || loadedCheckpoint.Scratchpad["goal"] != "draft summary" {
		t.Fatalf("expected persisted checkpoint scratchpad, got %+v", loadedCheckpoint)
	}

	if err := store.MarkRunCompleted(ctx, run, 12, ""); err != nil {
		t.Fatalf("MarkRunCompleted: %v", err)
	}

	completedRun, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun after completion: %v", err)
	}
	if completedRun == nil {
		t.Fatal("expected completed run row")
	}
	if len(completedRun.Scratchpad) != 0 {
		t.Fatalf("expected cleared run scratchpad, got %+v", completedRun.Scratchpad)
	}

	completedCheckpoint, err := store.LoadCheckpoint(ctx, run.RunID)
	if err != nil {
		t.Fatalf("LoadCheckpoint after completion: %v", err)
	}
	if completedCheckpoint == nil {
		t.Fatal("expected checkpoint row")
	}
	if len(completedCheckpoint.Scratchpad) != 0 {
		t.Fatalf("expected cleared checkpoint scratchpad, got %+v", completedCheckpoint.Scratchpad)
	}
}

func TestListActiveRunsDoesNotBlockWithSingleSQLiteConnection(t *testing.T) {
	db := prepareTestDB(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Run Scratchpad"})
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
	runtimeSessionID := string(chat.ID)

	run := naviruntime.NewRun(runtimeSessionID, string(navi.ExperienceModeStandard))
	run.SetStatus(schema.RunStatusActive)
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.SaveICSState(ctx, run.RunID, inference.DecisionEnvelope{
		Rationale: inference.Rationale{Version: inference.ContractVersionV1},
	}); err != nil {
		t.Fatalf("SaveICSState: %v", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	activeRuns, err := store.ListActiveRuns(listCtx)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	if len(activeRuns) != 1 {
		t.Fatalf("expected 1 active run, got %d", len(activeRuns))
	}
	if activeRuns[0].RunID != run.RunID {
		t.Fatalf("active run id = %q, want %q", activeRuns[0].RunID, run.RunID)
	}
	if activeRuns[0].ICSStateVersion != inference.ContractVersionV1 {
		t.Fatalf("ICS state version = %q, want %q", activeRuns[0].ICSStateVersion, inference.ContractVersionV1)
	}
}

func TestCompleteRunEmitsAssistantMessageCompletedBeforeRunCompleted(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Complete Run"})
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
	runtimeSessionID := string(chat.ID)

	item := &naviruntime.InboxItem{
		Content:       "hello",
		SourceChannel: "telegram",
		ReceivedAt:    time.Now().UTC(),
	}
	accepted, err := store.AcceptMessage(ctx, runtimeSessionID, item, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("accept message: %v", err)
	}

	run := naviruntime.NewRun(runtimeSessionID, string(navi.ExperienceModeStandard))
	run.SetStatus(schema.RunStatusActive)
	run.InitiatedByInboxItemID = accepted.ID
	run.SetScratchpadValue("source_channel", "telegram")
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := store.MarkInboxConsumed(ctx, accepted.ID, run.RunID); err != nil {
		t.Fatalf("MarkInboxConsumed: %v", err)
	}

	finalContent := naviruntime.TimeoutFallbackContent("telegram")
	msgID, err := store.CompleteRun(ctx, run, finalContent, string(navi.ExperienceModeStandard), accepted.ID)
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	if msgID == "" {
		t.Fatal("expected assistant message id")
	}

	events, err := corestore.EventsByCorrelationID(ctx, db, runtimeSessionID)
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected assistant and run completion events, got %#v", events)
	}
	if events[len(events)-2].Type != schema.FactAssistantMessageCompleted {
		t.Fatalf("expected assistant.message.completed before terminal completion, got %#v", events)
	}
	if events[len(events)-1].Type != schema.FactRunCompleted {
		t.Fatalf("expected run.completed as final event, got %#v", events)
	}

	rawPayload, err := json.Marshal(events[len(events)-2].Payload)
	if err != nil {
		t.Fatalf("marshal assistant.message.completed payload: %v", err)
	}
	var payload schema.AssistantMessageCompletedPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal assistant.message.completed payload: %v", err)
	}
	if payload.RunID != run.RunID {
		t.Fatalf("assistant.message.completed run_id = %q, want %q", payload.RunID, run.RunID)
	}
	if payload.Content != finalContent {
		t.Fatalf("assistant.message.completed content = %q, want %q", payload.Content, finalContent)
	}
	if payload.MessageKind != string(schema.AssistantMessageKindReply) {
		t.Fatalf("assistant.message.completed message_kind = %q, want %q", payload.MessageKind, schema.AssistantMessageKindReply)
	}

	rawRunCompletedPayload, err := json.Marshal(events[len(events)-1].Payload)
	if err != nil {
		t.Fatalf("marshal run.completed payload: %v", err)
	}
	var runCompleted schema.RunCompletedPayload
	if err := json.Unmarshal(rawRunCompletedPayload, &runCompleted); err != nil {
		t.Fatalf("unmarshal run.completed payload: %v", err)
	}
	if runCompleted.FinalMessageID != msgID {
		t.Fatalf("run.completed final_message_id = %q, want %q", runCompleted.FinalMessageID, msgID)
	}
}

// TestMergeInboxPreservesChronologicalOrderOnRaceCondition verifies that
// merged inbox content stays in chronological order even when the later
// message was inserted into the DB first (a race between two concurrent
// SubmitMessage goroutines).
func TestMergeInboxPreservesChronologicalOrderOnRaceCondition(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	runtimeSessionID := "sess-merge-race"
	t0 := time.Now().UTC()

	// msg2 has a LATER ReceivedAt but wins the DB race and is inserted first.
	msg2 := naviruntime.NewInboxItem(runtimeSessionID, "message 2", "telegram")
	msg2.ReceivedAt = t0.Add(200 * time.Millisecond)
	acceptedMsg2, err := store.AcceptMessage(ctx, runtimeSessionID, msg2, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage msg2: %v", err)
	}

	// msg1 has an EARLIER ReceivedAt but arrives second; the classifier sees
	// msg2 as the pending item and sets msg1 to merge into it.
	msg1 := naviruntime.NewInboxItem(runtimeSessionID, "message 1", "telegram")
	msg1.ReceivedAt = t0 // earlier timestamp
	msg1.QueueAction = naviruntime.QueueActionMerge
	msg1.Status = naviruntime.InboxStatusMerged
	msg1.MergedIntoID = acceptedMsg2.ID
	msg1.ClassifiedReason = "simulated race: earlier message merged into later pending target"
	msg1.Confidence = 0.8
	if _, err := store.AcceptMessage(ctx, runtimeSessionID, msg1, string(navi.ExperienceModeStandard)); err != nil {
		t.Fatalf("AcceptMessage msg1: %v", err)
	}

	var merged string
	if err := db.QueryRowContext(ctx, `SELECT content FROM navi_inbox WHERE inbox_item_id = ?`, acceptedMsg2.ID).Scan(&merged); err != nil {
		t.Fatalf("load merged content: %v", err)
	}
	// msg1 was sent first (earlier ReceivedAt) so it must appear before msg2.
	const want = "message 1\nmessage 2"
	if merged != want {
		t.Fatalf("merged content = %q, want %q", merged, want)
	}
}
