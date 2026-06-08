package presence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// Service manages the authoritative presence state of NAVI and observes PET user presence.
type Service interface {
	// Snapshot returns the current combined presence snapshot.
	Snapshot(ctx context.Context) PresenceSnapshot

	// Revision returns the combined snapshot revision.
	// Kept for backward compatibility; prefer SnapshotRevision for new callers.
	Revision() int64

	// SnapshotRevision returns the combined snapshot revision.
	SnapshotRevision() int64

	// NaviRevision returns the authoritative NAVI presence revision.
	NaviRevision() int64

	// NaviPresence returns the current authoritative NAVI presence envelope.
	NaviPresence(ctx context.Context) PresenceEnvelope

	// UpdateUserPresence updates the locally cached user presence (from PET).
	UpdateUserPresence(ctx context.Context, envelope PresenceEnvelope) error

	// Heartbeat refreshes the transport observation time for the user.
	Heartbeat(ctx context.Context)

	// Close shuts down the presence service.
	Close() error
}

// StatusSource provides raw runtime status. Usually implemented by *navi.StatusTracker.
type StatusSource interface {
	Snapshot() schema.AgentStatusSnapshot
}

// AttentionSource provides information about pending proposals or required actions.
type AttentionSource interface {
	Attention(ctx context.Context, chatID string) PresenceAttention
}

// WorkClassificationSource determines if a task is active, working, or busy.
type WorkClassificationSource interface {
	Classify(ctx context.Context, snapshot schema.AgentStatusSnapshot) (publicStatus string, statusText string, subtext string)
}

// HealthSource provides detailed health metrics.
type HealthSource interface {
	Health(ctx context.Context) PresenceHealth
}

// DefaultService is the standard implementation of Service.
type DefaultService struct {
	statusSource         StatusSource
	attentionSource      AttentionSource
	classificationSource WorkClassificationSource
	healthSource         HealthSource
	dreamingSource       DreamingSource
	db                   *sql.DB
	mu                   sync.RWMutex
	naviEnvelope         PresenceEnvelope
	userEnvelope         *PresenceEnvelope
	naviRevision         int64
	snapshotRevision     int64
	lastRefreshedAt      time.Time
}

// NewDefaultService creates a new presence service.
// dreaming is optional and defaults to NoOpDreamingSource when omitted.
func NewDefaultService(db *sql.DB, status StatusSource, attention AttentionSource, classification WorkClassificationSource, health HealthSource, dreaming ...DreamingSource) *DefaultService {
	if attention == nil {
		attention = &NoOpAttentionSource{}
	}
	if classification == nil {
		classification = &DefaultClassificationSource{}
	}
	if health == nil {
		health = NewDefaultHealthSource(status)
	}
	dreamingSource := DreamingSource(&NoOpDreamingSource{})
	if len(dreaming) > 0 && dreaming[0] != nil {
		dreamingSource = dreaming[0]
	}

	s := &DefaultService{
		db:                   db,
		statusSource:         status,
		attentionSource:      attention,
		classificationSource: classification,
		healthSource:         health,
		dreamingSource:       dreamingSource,
	}
	s.restoreUserPresence()
	s.refreshNaviPresence(context.Background())
	return s
}

func (s *DefaultService) NaviPresence(ctx context.Context) PresenceEnvelope {
	if s == nil {
		return offlinePresenceEnvelope()
	}
	s.refreshNaviPresence(ctx)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.naviEnvelope
}

func (s *DefaultService) Snapshot(ctx context.Context) PresenceSnapshot {
	if s == nil {
		navi := offlinePresenceEnvelope()
		return PresenceSnapshot{
			Version:          Version,
			SnapshotRevision: navi.StateRevision,
			GeneratedAt:      time.Now().UTC(),
			Navi:             &navi,
			Transport: TransportMetadata{
				State:        "connected",
				StaleAfterMS: 45000,
			},
		}
	}

	navi := s.NaviPresence(ctx)

	s.mu.RLock()
	var user *PresenceEnvelope
	if s.userEnvelope != nil {
		copy := *s.userEnvelope
		user = &copy
	}
	revision := s.snapshotRevision
	s.mu.RUnlock()

	return PresenceSnapshot{
		Version:          Version,
		SnapshotRevision: revision,
		GeneratedAt:      time.Now().UTC(),
		Navi:             &navi,
		User:             user,
		Transport: TransportMetadata{
			State:        "connected", // If the service is running, the gateway considers itself connected.
			StaleAfterMS: 45000,
		},
	}
}

func (s *DefaultService) UpdateUserPresence(ctx context.Context, envelope PresenceEnvelope) error {
	if s == nil {
		return nil
	}
	if err := validateUserPresenceEnvelope(envelope); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userEnvelope != nil && !userEnvelopeIsNewer(*s.userEnvelope, envelope) {
		return nil
	}

	advanceSnapshotRevision := s.userEnvelope == nil
	if s.userEnvelope != nil {
		advanceSnapshotRevision = userEnvelopeMateriallyChanged(*s.userEnvelope, envelope)
	}
	s.userEnvelope = &envelope
	if advanceSnapshotRevision {
		s.bumpSnapshotRevisionLocked()
	}

	// Persist to DB if available
	if s.db != nil {
		data, _ := json.Marshal(envelope)
		_ = s.setSetting(ctx, "presence_user_envelope", string(data))
	}

	return nil
}

func (s *DefaultService) restoreUserPresence() {
	if s.db == nil {
		return
	}
	ctx := context.Background()
	val, found, err := s.getSetting(ctx, "presence_user_envelope")
	if err != nil || !found {
		return
	}

	var envelope PresenceEnvelope
	if err := json.Unmarshal([]byte(val), &envelope); err != nil {
		return
	}
	if err := validateUserPresenceEnvelope(envelope); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.userEnvelope = &envelope
}

func (s *DefaultService) getSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *DefaultService) setSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

func (s *DefaultService) Heartbeat(ctx context.Context) {
	_ = ctx
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userEnvelope != nil {
		s.userEnvelope.TransportObservedAt = time.Now().UTC()
	}
}

func (s *DefaultService) Revision() int64 {
	return s.SnapshotRevision()
}

func (s *DefaultService) SnapshotRevision() int64 {
	if s == nil {
		return 0
	}
	s.refreshNaviPresence(context.Background())
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotRevision
}

func (s *DefaultService) NaviRevision() int64 {
	if s == nil {
		return 0
	}
	s.refreshNaviPresence(context.Background())
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.naviRevision
}

func (s *DefaultService) Close() error {
	return nil
}

func (s *DefaultService) refreshNaviPresence(ctx context.Context) {
	snap := s.currentSnapshot()

	s.mu.Lock()
	if !s.shouldRefreshNaviPresenceLocked(snap) {
		s.mu.Unlock()
		return
	}
	s.lastRefreshedAt = time.Now()
	s.mu.Unlock()

	publicStatus, statusText, subtext := s.classificationSource.Classify(ctx, snap)
	dreaming := s.dreamingSource.Dreaming(ctx)
	attention := s.attentionSource.Attention(ctx, snap.ActiveRuntimeSessionID)
	health := s.healthSource.Health(ctx)

	if dreaming.Active {
		publicStatus = NaviStatusDreaming
		if strings.TrimSpace(dreaming.StatusText) != "" {
			statusText = strings.TrimSpace(dreaming.StatusText)
		} else if strings.TrimSpace(statusText) == "" {
			statusText = "Dreaming"
		}
		if strings.TrimSpace(dreaming.Subtext) != "" {
			subtext = strings.TrimSpace(dreaming.Subtext)
		}
	}

	// Override public status if attention is required.
	if attention.Level == "needs_attention" {
		publicStatus = NaviStatusNeedsAttention
		if attention.ProposalID != nil {
			subtext = "Action required: Proposal pending"
		} else {
			subtext = "Awaiting your input"
		}
	} else if attention.Level == "wants_attention" && publicStatus == NaviStatusIdle {
		publicStatus = NaviStatusWantsAttention
		subtext = "New proposals available"
	}

	payload := NaviPresencePayload{
		PublicStatus:           publicStatus,
		InternalStatus:         mapInternalStatus(snap.State),
		StatusText:             statusText,
		Subtext:                subtext,
		ActiveRuntimeSessionID: snap.ActiveRuntimeSessionID,
		CurrentDetail:   snap.CurrentDetail,
		Attention:       attention,
		Health:          health,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	stateUpdatedAt := snap.UpdatedAt
	if stateUpdatedAt.IsZero() {
		stateUpdatedAt = now
	}

	if s.naviRevision == 0 {
		s.naviRevision = 1
		s.ensureSnapshotRevisionLocked()
	} else if !reflect.DeepEqual(s.naviPayload(), payload) {
		s.naviRevision++
		s.bumpSnapshotRevisionLocked()
	} else {
		stateUpdatedAt = s.naviEnvelope.StateUpdatedAt
	}

	s.naviEnvelope = PresenceEnvelope{
		Version:             Version,
		Source:              AuthorityNavi,
		SubjectType:         SubjectNavi,
		SubjectID:           "navi",
		Authority:           AuthorityNavi,
		TransportObservedAt: now,
		StateUpdatedAt:      stateUpdatedAt,
		StateRevision:       s.naviRevision,
		Visibility:          VisibilityMixed,
		Payload:             payload,
	}
}

func (s *DefaultService) shouldRefreshNaviPresenceLocked(snap schema.AgentStatusSnapshot) bool {
	if s.naviRevision == 0 || s.lastRefreshedAt.IsZero() {
		return true
	}
	if time.Since(s.lastRefreshedAt) >= 100*time.Millisecond {
		return true
	}
	if snap.UpdatedAt.IsZero() {
		return false
	}
	currentUpdatedAt := s.naviEnvelope.StateUpdatedAt
	return currentUpdatedAt.IsZero() || snap.UpdatedAt.After(currentUpdatedAt)
}

func (s *DefaultService) currentSnapshot() schema.AgentStatusSnapshot {
	if s == nil || s.statusSource == nil {
		now := time.Now().UTC()
		return schema.AgentStatusSnapshot{
			State:          schema.AgentStateOffline,
			Since:          now,
			LastActivityAt: now,
			UpdatedAt:      now,
		}
	}
	return s.statusSource.Snapshot()
}

func (s *DefaultService) naviPayload() NaviPresencePayload {
	payload, _ := s.naviEnvelope.Payload.(NaviPresencePayload)
	return payload
}

func (s *DefaultService) ensureSnapshotRevisionLocked() {
	if s.snapshotRevision == 0 {
		s.snapshotRevision = 1
	}
}

func (s *DefaultService) bumpSnapshotRevisionLocked() {
	if s.snapshotRevision == 0 {
		s.snapshotRevision = 1
		return
	}
	s.snapshotRevision++
}

func offlinePresenceEnvelope() PresenceEnvelope {
	now := time.Now().UTC()
	return PresenceEnvelope{
		Version:             Version,
		Source:              AuthorityNavi,
		SubjectType:         SubjectNavi,
		SubjectID:           "navi",
		Authority:           AuthorityNavi,
		TransportObservedAt: now,
		StateUpdatedAt:      now,
		StateRevision:       1,
		Visibility:          VisibilityMixed,
		Payload: NaviPresencePayload{
			PublicStatus:   NaviStatusOffline,
			InternalStatus: NaviInternalOffline,
			StatusText:     "Offline",
			Attention: PresenceAttention{
				Level:    "none",
				Blocking: false,
			},
			Health: PresenceHealth{
				State:          "offline",
				LastActivityAt: now,
				StaleAfterMS:   int(DefaultStaleAfterMs),
			},
		},
	}
}

func mapInternalStatus(state schema.AgentActivityState) string {
	switch state {
	case schema.AgentStateIdle:
		return NaviInternalIdle
	case schema.AgentStateProcessing:
		return NaviInternalProcessing
	case schema.AgentStateToolExecuting:
		return NaviInternalToolExecuting
	case schema.AgentStateWaitingForInput:
		return NaviInternalWaitingForInput
	case schema.AgentStateHeartbeat:
		return NaviInternalHeartbeat
	case schema.AgentStateDegraded:
		return NaviInternalDegraded
	case schema.AgentStateOffline:
		return NaviInternalOffline
	case schema.AgentStateUnresponsive:
		return NaviInternalUnresponsive
	default:
		return NaviInternalIdle
	}
}

func validateUserPresenceEnvelope(envelope PresenceEnvelope) error {
	if strings.TrimSpace(envelope.SubjectType) != SubjectUser {
		return fmt.Errorf("presence subject_type must be %q", SubjectUser)
	}
	if strings.TrimSpace(envelope.Authority) != AuthorityPet {
		return fmt.Errorf("presence authority must be %q for user subject", AuthorityPet)
	}
	if strings.TrimSpace(envelope.SubjectID) == "" {
		return fmt.Errorf("presence subject_id is required")
	}
	return nil
}

func userEnvelopeIsNewer(current PresenceEnvelope, incoming PresenceEnvelope) bool {
	switch {
	case incoming.StateRevision > current.StateRevision:
		return true
	case incoming.StateRevision < current.StateRevision:
		return false
	}

	switch {
	case incoming.StateUpdatedAt.After(current.StateUpdatedAt):
		return true
	case current.StateUpdatedAt.After(incoming.StateUpdatedAt):
		return false
	}

	return incoming.TransportObservedAt.After(current.TransportObservedAt)
}

func userEnvelopeMateriallyChanged(current PresenceEnvelope, incoming PresenceEnvelope) bool {
	current.TransportObservedAt = time.Time{}
	incoming.TransportObservedAt = time.Time{}
	return !reflect.DeepEqual(current, incoming)
}
