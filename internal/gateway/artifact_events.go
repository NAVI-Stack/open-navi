package gateway

import (
	"context"
	"time"

	artifactsvc "github.com/ceoai/navi/internal/artifact"
	"github.com/ceoai/navi/internal/schema"
)

func (s *Server) emitArtifactWorkspaceEvent(ctx context.Context, eventType schema.EventType, artifact *schema.Artifact, version *schema.ArtifactVersion, resultStatus string, failureClass schema.FailureClass, failureCode string, startedAt time.Time) {
	if s == nil || s.cfg.Bus == nil || artifact == nil {
		return
	}
	renderer := artifactsvc.NewRegistry().GetRenderer(artifact.Subtype)
	payload := schema.ArtifactObservedPayload{
		WorkspaceID:    artifact.WorkspaceID,
		ProjectID:      artifact.ProjectID,
		ArtifactID:     artifact.ID,
		BranchID:       artifact.CurrentBranchID,
		ActorType:      string(schema.ActorUser),
		Operation:      "workspace_load",
		Subtype:        artifact.Subtype,
		RendererKey:    renderer.ComponentID,
		ResultStatus:   resultStatus,
		FailureClass:   string(failureClass),
		FailureCode:    failureCode,
		LifecycleState: string(artifact.LifecycleState),
	}
	if !startedAt.IsZero() {
		payload.LatencyMs = time.Since(startedAt).Milliseconds()
	}
	if version != nil {
		payload.VersionID = version.ID
		payload.BranchID = version.BranchID
		payload.HistoryCommandID = version.HistoryCommandID
		payload.HistoryAttemptID = version.HistoryAttemptID
	}
	ev := schema.NewEvent(eventType, schema.EventKindFact, artifact.ID, schema.AgentNavi, payload)
	ev.Visibility = schema.VisibilityOperator
	_ = s.cfg.Bus.Publish(ctx, ev)
}
