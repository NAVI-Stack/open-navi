package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

// EventPublisher receives normalized artifact observability events.
type EventPublisher func(ctx context.Context, ev schema.Event) error

// ReflectionPublisher queues artifact reflection payloads into the existing
// subconscious reflection pipeline.
type ReflectionPublisher func(ctx context.Context, payload schema.ReflectionPayload) error

// SetEventPublisher installs an optional observability sink for artifact events.
func (s *Service) SetEventPublisher(fn EventPublisher) {
	if s == nil {
		return
	}
	s.eventPublisher = fn
}

// SetReflectionPublisher installs an optional sink for artifact reflection hooks.
func (s *Service) SetReflectionPublisher(fn ReflectionPublisher) {
	if s == nil {
		return
	}
	s.reflectionPublisher = fn
}

func (s *Service) emitArtifactEvent(ctx context.Context, eventType schema.EventType, artifact *schema.Artifact, version *schema.ArtifactVersion, operation, rendererKey, resultStatus string, failureClass schema.FailureClass, failureCode string, startedAt time.Time, actorType schema.ActorType, actorID string) {
	if s == nil || s.eventPublisher == nil || artifact == nil {
		return
	}
	payload := schema.ArtifactObservedPayload{
		WorkspaceID:    artifact.WorkspaceID,
		ProjectID:      artifact.ProjectID,
		ArtifactID:     artifact.ID,
		BranchID:       artifact.CurrentBranchID,
		ActorType:      string(firstActorType(actorType)),
		ActorID:        actorID,
		Operation:      strings.TrimSpace(operation),
		Subtype:        artifact.Subtype,
		RendererKey:    strings.TrimSpace(rendererKey),
		ResultStatus:   strings.TrimSpace(resultStatus),
		FailureClass:   string(failureClass),
		FailureCode:    strings.TrimSpace(failureCode),
		LifecycleState: string(artifact.LifecycleState),
	}
	if !startedAt.IsZero() {
		payload.LatencyMs = time.Since(startedAt).Milliseconds()
	}
	if version != nil {
		payload.VersionID = version.ID
		payload.BranchID = firstNonEmpty(version.BranchID, payload.BranchID)
		payload.HistoryCommandID = version.HistoryCommandID
		payload.HistoryAttemptID = version.HistoryAttemptID
	}
	ev := schema.NewEvent(eventType, schema.EventKindFact, artifact.ID, schema.AgentNavi, payload)
	ev.Visibility = schema.VisibilityOperator
	_ = s.eventPublisher(ctx, ev)
}

func (s *Service) emitArtifactReflection(ctx context.Context, artifact *schema.Artifact, version *schema.ArtifactVersion, operation, summary, runtimeSessionID, rendererKey, resultStatus string, failureClass schema.FailureClass, failureCode string) {
	if s == nil || s.reflectionPublisher == nil || artifact == nil {
		return
	}
	details := map[string]any{
		"category":         "artifact",
		"scope":            "owner",
		"scope_id":         artifact.OwnerID,
		"artifact_id":      artifact.ID,
		"artifact_title":   firstNonEmpty(artifact.DisplayTitle, artifact.CanonicalTitle, artifact.ID),
		"workspace_id":     artifact.WorkspaceID,
		"project_id":       artifact.ProjectID,
		"artifact_type":    string(artifact.Type),
		"artifact_subtype": artifact.Subtype,
		"operation":        strings.TrimSpace(operation),
		"renderer_key":     strings.TrimSpace(rendererKey),
		"result_status":    strings.TrimSpace(resultStatus),
		"failure_class":    strings.TrimSpace(string(failureClass)),
		"failure_code":     strings.TrimSpace(failureCode),
		"lifecycle_state":  string(artifact.LifecycleState),
		"summary":          summary,
	}
	if version != nil {
		details["version_id"] = version.ID
		details["branch_id"] = version.BranchID
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return
	}
	payload := schema.ReflectionPayload{
		ID:               uuid.New().String(),
		RuntimeSessionID: strings.TrimSpace(runtimeSessionID),
		Tier:             schema.ReflectionTierShallow,
		Summary:          firstNonEmpty(summary, fmt.Sprintf("Artifact %s %s", artifact.ID, strings.TrimSpace(operation))),
		Details:          string(raw),
		CreatedAt:        time.Now().UTC(),
	}
	_ = s.reflectionPublisher(ctx, payload)
}
