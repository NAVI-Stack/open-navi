package presence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

type stubStatusSource struct {
	snapshot schema.AgentStatusSnapshot
}

func (s stubStatusSource) Snapshot() schema.AgentStatusSnapshot {
	return s.snapshot
}

type mutableStatusSource struct {
	snapshot schema.AgentStatusSnapshot
}

func (s *mutableStatusSource) Snapshot() schema.AgentStatusSnapshot {
	return s.snapshot
}

type stubDreamingSource struct {
	summary DreamingSummary
}

func (s stubDreamingSource) Dreaming(ctx context.Context) DreamingSummary {
	return s.summary
}

func TestDefaultService_UpdateUserPresenceRejectsAuthorityMismatch(t *testing.T) {
	t.Parallel()

	svc := NewDefaultService(nil, stubStatusSource{snapshot: testPresenceStatusSnapshot()}, nil, nil, nil)
	err := svc.UpdateUserPresence(context.Background(), PresenceEnvelope{
		Version:             Version,
		Source:              AuthorityPet,
		SubjectType:         SubjectUser,
		SubjectID:           "owner",
		Authority:           AuthorityNavi,
		TransportObservedAt: time.Now().UTC(),
		StateUpdatedAt:      time.Now().UTC(),
		StateRevision:       1,
		Visibility:          VisibilityMixed,
		Payload: UserPresencePayload{
			PublicStatus: UserStatusAvailable,
		},
	})
	if err == nil {
		t.Fatal("expected authority mismatch to be rejected")
	}
	if !strings.Contains(err.Error(), AuthorityPet) {
		t.Fatalf("expected pet authority error, got %v", err)
	}
}

func TestDefaultService_UpdateUserPresenceAcceptsEqualRevisionWithNewerTimestamp(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewDefaultService(nil, stubStatusSource{snapshot: testPresenceStatusSnapshot()}, nil, nil, nil)

	firstObserved := time.Date(2026, 4, 17, 15, 0, 0, 0, time.UTC)
	firstUpdated := firstObserved.Add(-time.Minute)
	first := PresenceEnvelope{
		Version:             Version,
		Source:              AuthorityPet,
		SubjectType:         SubjectUser,
		SubjectID:           "owner",
		Authority:           AuthorityPet,
		TransportObservedAt: firstObserved,
		StateUpdatedAt:      firstUpdated,
		StateRevision:       5,
		Visibility:          VisibilityMixed,
		Payload: UserPresencePayload{
			PublicStatus: UserStatusAvailable,
			StatusText:   "Heads down",
		},
	}
	if err := svc.UpdateUserPresence(ctx, first); err != nil {
		t.Fatalf("UpdateUserPresence(first): %v", err)
	}
	afterFirst := svc.Revision()

	secondObserved := firstObserved.Add(2 * time.Minute)
	secondUpdated := firstUpdated.Add(1 * time.Minute)
	second := first
	second.TransportObservedAt = secondObserved
	second.StateUpdatedAt = secondUpdated
	second.Payload = UserPresencePayload{
		PublicStatus: UserStatusBusy,
		StatusText:   "In review",
	}
	if err := svc.UpdateUserPresence(ctx, second); err != nil {
		t.Fatalf("UpdateUserPresence(second): %v", err)
	}
	afterSecond := svc.Revision()
	if afterSecond <= afterFirst {
		t.Fatalf("expected material equal-revision update to advance revision, got first=%d second=%d", afterFirst, afterSecond)
	}

	snapshot := svc.Snapshot(ctx)
	if snapshot.User == nil {
		t.Fatal("expected user envelope in snapshot")
	}
	payload, ok := snapshot.User.Payload.(UserPresencePayload)
	if !ok {
		t.Fatalf("expected user payload type, got %#v", snapshot.User.Payload)
	}
	if payload.PublicStatus != UserStatusBusy || payload.StatusText != "In review" {
		t.Fatalf("expected newer equal-revision envelope to win, got %#v", payload)
	}

	transportOnly := second
	transportOnly.TransportObservedAt = secondObserved.Add(time.Minute)
	if err := svc.UpdateUserPresence(ctx, transportOnly); err != nil {
		t.Fatalf("UpdateUserPresence(transportOnly): %v", err)
	}
	if got := svc.Revision(); got != afterSecond {
		t.Fatalf("expected transport-only refresh not to advance revision, got %d want %d", got, afterSecond)
	}
}

func TestDefaultService_DreamingSourceOverridesPublicStatusWithoutChangingInternalStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := NewDefaultService(
		nil,
		stubStatusSource{snapshot: schema.AgentStatusSnapshot{
			State:          schema.AgentStateHeartbeat,
			Since:          time.Date(2026, 4, 18, 18, 0, 0, 0, time.UTC),
			LastActivityAt: time.Date(2026, 4, 18, 18, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 4, 18, 18, 0, 0, 0, time.UTC),
		}},
		nil,
		nil,
		nil,
		stubDreamingSource{summary: DreamingSummary{
			Active:     true,
			StatusText: "Dreaming",
			Subtext:    "Consolidating recent context",
		}},
	)

	envelope := svc.NaviPresence(ctx)
	payload, ok := envelope.Payload.(NaviPresencePayload)
	if !ok {
		t.Fatalf("expected navi payload type, got %#v", envelope.Payload)
	}
	if payload.PublicStatus != NaviStatusDreaming {
		t.Fatalf("expected public dreaming status, got %q", payload.PublicStatus)
	}
	if payload.InternalStatus != NaviInternalHeartbeat {
		t.Fatalf("expected internal heartbeat status to remain runtime-truth, got %q", payload.InternalStatus)
	}
	if payload.StatusText != "Dreaming" {
		t.Fatalf("expected dreaming status text, got %q", payload.StatusText)
	}
	if payload.Subtext != "Consolidating recent context" {
		t.Fatalf("expected dreaming subtext, got %q", payload.Subtext)
	}
}

func TestDefaultService_RevisionRefreshBypassesThrottleWhenStatusSnapshotAdvances(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 28, 22, 0, 0, 0, time.UTC)
	source := &mutableStatusSource{snapshot: schema.AgentStatusSnapshot{
		State:          schema.AgentStateIdle,
		Since:          now,
		LastActivityAt: now,
		UpdatedAt:      now,
	}}
	svc := NewDefaultService(nil, source, nil, nil, nil)
	before := svc.NaviRevision()

	source.snapshot = schema.AgentStatusSnapshot{
		State:          schema.AgentStateProcessing,
		Since:          now.Add(time.Millisecond),
		LastActivityAt: now.Add(time.Millisecond),
		UpdatedAt:      now.Add(time.Millisecond),
	}

	after := svc.NaviRevision()
	if after <= before {
		t.Fatalf("expected advanced status snapshot to refresh despite throttle, before=%d after=%d", before, after)
	}
	envelope := svc.NaviPresence(context.Background())
	payload, ok := envelope.Payload.(NaviPresencePayload)
	if !ok {
		t.Fatalf("expected navi payload type, got %#v", envelope.Payload)
	}
	if payload.InternalStatus != NaviInternalProcessing {
		t.Fatalf("expected processing status after refresh, got %q", payload.InternalStatus)
	}
}

func testPresenceStatusSnapshot() schema.AgentStatusSnapshot {
	now := time.Date(2026, 4, 17, 15, 0, 0, 0, time.UTC)
	return schema.AgentStatusSnapshot{
		State:          schema.AgentStateIdle,
		Since:          now,
		LastActivityAt: now,
		UpdatedAt:      now,
	}
}
