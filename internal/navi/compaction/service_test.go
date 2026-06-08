package compaction

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type memStore struct{ mem ChatMemory }

func (m *memStore) GetChatMemory(context.Context, string) (ChatMemory, error) {
	return m.mem, nil
}
func (m *memStore) PutChatMemory(_ context.Context, memory ChatMemory) error {
	m.mem = memory
	return nil
}

type cpStore struct {
	fail            bool
	checkpointFirst bool
	marked          bool
	last            CompactionCheckpoint
}

func (c *cpStore) AppendCompactionCheckpoint(_ context.Context, checkpoint CompactionCheckpoint) error {
	c.checkpointFirst = true
	if c.fail {
		return errors.New("fail")
	}
	c.last = checkpoint
	return nil
}
func (c *cpStore) MarkMessagesCompacted(context.Context, string, string, string, string, string) error {
	if !c.checkpointFirst {
		return errors.New("ordering")
	}
	c.marked = true
	return nil
}

func TestCheckpoint_WriteOccursBeforeMessageMarkers(t *testing.T) {
	m := &memStore{}
	c := &cpStore{}
	svc := Service{Budget: NewBudgetManager(nil, BudgetConfig{}), Selector: SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}, Rehydrator: Rehydrator{}, Memory: m, Checkpoint: c}
	_, err := svc.Run(context.Background(), RunInput{ChatID: "s1", Messages: []Message{{ID: "1", Content: string(make([]byte, 3000))}, {ID: "2", Content: "a"}, {ID: "3", Content: "b"}, {ID: "4", Content: "c"}}, MaxContextTokens: 1000, ReservedOutputTokens: 100, ReservedToolHeadroom: 100})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !c.marked {
		t.Fatal("expected mark after checkpoint")
	}
}

func TestCheckpoint_Failure_DoesNotMarkMessagesCompacted(t *testing.T) {
	m := &memStore{}
	c := &cpStore{fail: true}
	svc := Service{Budget: NewBudgetManager(nil, BudgetConfig{}), Selector: SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}, Rehydrator: Rehydrator{}, Memory: m, Checkpoint: c}
	_, err := svc.Run(context.Background(), RunInput{ChatID: "s1", Messages: []Message{{ID: "1", Content: string(make([]byte, 3000))}, {ID: "2", Content: "a"}, {ID: "3", Content: "b"}, {ID: "4", Content: "c"}}, MaxContextTokens: 1000, ReservedOutputTokens: 100, ReservedToolHeadroom: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	if c.marked {
		t.Fatal("messages marked despite checkpoint failure")
	}
}

func TestService_TaskContinuity_SurvivesCompactionAndRehydration(t *testing.T) {
	m := &memStore{}
	c := &cpStore{}
	svc := Service{Budget: NewBudgetManager(nil, BudgetConfig{}), Selector: SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}, Rehydrator: Rehydrator{}, Memory: m, Checkpoint: c}
	out, err := svc.Run(context.Background(), RunInput{
		ChatID:   "s1",
		Messages: []Message{{ID: "0", Role: "user", Content: strings.Repeat("old ", 500)}, {ID: "0b", Role: "navi", Content: strings.Repeat("older reply ", 300)}, {ID: "1", RunID: "task-1", Role: "user", Content: "[task:task-1] Build release plan", MessageKind: "task_active"}, {ID: "2", RunID: "task-1", Role: "navi", Content: "BLOCKER: waiting on review\nNext step: get approval", MessageKind: "task_blocked"}, {ID: "3", Role: "user", Content: "tail"}, {ID: "4", Role: "navi", Content: "tail2"}},
		RuntimeSnapshots: []RunSnapshot{{
			RunID:                 "task-1",
			Status:                "running",
			CurrentPhase:          "execute",
			LatestCheckpointID:    "cp-runtime-1",
			BlockedOnProposalID:   "prop-1",
			PendingProposalID:     "prop-1",
			PendingProposalReason: "need approval to publish",
			Scratchpad:            map[string]string{"goal": "Build release plan", "current_step": "wait for approval"},
			MainArtifactID:        "artifact:plan",
		}},
		ProposalRefs:         []string{"prop-1"},
		FailureRefs:          []string{"fail-1"},
		MaxContextTokens:     1000,
		ReservedOutputTokens: 100,
		ReservedToolHeadroom: 100,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(m.mem.TaskFrames) == 0 {
		t.Fatal("expected persisted task frames")
	}
	tf := m.mem.TaskFrames[0]
	if tf.TaskID != "task-1" || tf.Status != "awaiting_user" || tf.NextStep != "wait for approval" {
		t.Fatalf("unexpected task frame continuity: %+v", tf)
	}
	if out.Checkpoint == nil || out.Checkpoint.TaskFrameVersions["task-1"] == 0 {
		t.Fatalf("expected checkpoint task frame provenance, got %+v", out.Checkpoint)
	}
	joined := strings.Join(out.Rehydration.Sections, "\n\n")
	expected := []string{
		"## Session Continuity",
		"## Task Build release plan [awaiting_user]",
		"Blockers: awaiting proposal: need approval to publish",
		"Proposal refs: prop-1",
		"Failure refs: fail-1",
		"## Supporting Context",
		"Provenance:",
		"## Recent Context",
	}
	for _, snippet := range expected {
		if !strings.Contains(joined, snippet) {
			t.Fatalf("expected %q in rehydration output, got %q", snippet, joined)
		}
	}
	if len(out.Rehydration.Dropped) != 0 {
		t.Fatalf("did not expect trimming under normal budget, got %+v", out.Rehydration.Dropped)
	}
	if len(out.Inspection.TaskSurvival) == 0 || out.Inspection.TaskSurvival[0].TaskID != "task-1" {
		t.Fatalf("expected task survival inspection metadata, got %+v", out.Inspection.TaskSurvival)
	}
	if len(m.mem.RetrievalSpans) == 0 || m.mem.RetrievalSpans[0].Provenance.StartMessageID == "" {
		t.Fatalf("expected persisted retrieval spans with provenance, got %+v", m.mem.RetrievalSpans)
	}
}

func TestService_MultipleActiveTasksRemainStableAcrossCompactionRuns(t *testing.T) {
	m := &memStore{}
	c := &cpStore{}
	svc := Service{Budget: NewBudgetManager(nil, BudgetConfig{}), Selector: SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}, Rehydrator: Rehydrator{}, Memory: m, Checkpoint: c}
	input := RunInput{
		ChatID: "s1",
		Messages: []Message{
			{ID: "0", Role: "user", Content: strings.Repeat("older context ", 700)},
			{ID: "1", RunID: "run-a", Role: "user", Content: "[task:run-a] Prepare release notes"},
			{ID: "2", RunID: "run-b", Role: "user", Content: "[task:run-b] Validate Docker health"},
			{ID: "3", Role: "user", Content: "tail"},
			{ID: "4", Role: "navi", Content: "tail2"},
		},
		RuntimeSnapshots: []RunSnapshot{
			{RunID: "run-a", Status: "running", CurrentPhase: "execute", LatestCheckpointID: "cp-a", Scratchpad: map[string]string{"goal": "Prepare release notes", "current_step": "draft intro"}},
			{RunID: "run-b", Status: "waiting_for_proposal", CurrentPhase: "validate_govern", LatestCheckpointID: "cp-b", PendingProposalID: "proposal-b", PendingProposalReason: "needs approval", Scratchpad: map[string]string{"goal": "Validate Docker health"}},
		},
		MaxContextTokens:     1000,
		ReservedOutputTokens: 100,
		ReservedToolHeadroom: 100,
	}
	first, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(m.mem.TaskFrames) < 2 {
		t.Fatalf("expected multiple task frames, got %+v", m.mem.TaskFrames)
	}
	ids := []string{m.mem.TaskFrames[0].TaskID, m.mem.TaskFrames[1].TaskID}
	if !(ids[0] == "run-a" && ids[1] == "run-b") {
		t.Fatalf("unexpected stable task ids: %+v", ids)
	}
	if first.Checkpoint == nil || second.Checkpoint == nil {
		t.Fatalf("expected checkpoints on repeated compaction")
	}
	if first.Checkpoint.TaskFrameVersions["run-a"] == 0 || second.Checkpoint.TaskFrameVersions["run-b"] == 0 {
		t.Fatalf("expected task frame versions for both tasks: first=%+v second=%+v", first.Checkpoint, second.Checkpoint)
	}
}
