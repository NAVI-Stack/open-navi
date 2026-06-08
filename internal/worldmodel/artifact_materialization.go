package worldmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const (
	ArtifactStatusDraft       = "draft"
	ArtifactStatusInProgress  = "in_progress"
	ArtifactStatusReviewReady = "review_ready"
	ArtifactStatusApproved    = "approved"
	ArtifactStatusFailed      = "failed"

	ArtifactRenderStatusUnknown    = "unknown"
	ArtifactRenderStatusRenderable = "renderable"
	ArtifactRenderStatusInvalid    = "invalid"
)

type ArtifactMaterializationRequest struct {
	ArtifactID        string
	OwnerID           string
	Kind              string
	Subtype           string
	Location          string
	Description       string
	Status            string
	SourceTool        string
	SourceExecutionID string
	SourceRunID       string
	Snapshot          any
	RecoverableOutput any
	Metadata          map[string]any
}

type MaterializedArtifact struct {
	Artifact schema.Artifact
	Version  schema.ArtifactVersion
}

func (wm *WorldModel) GetArtifact(ctx context.Context, id string) (*schema.Artifact, error) {
	return store.GetArtifact(ctx, wm.db, id)
}

func (wm *WorldModel) ListArtifactVersions(ctx context.Context, artifactID string, limit int) ([]schema.ArtifactVersion, error) {
	return store.ListArtifactVersions(ctx, wm.db, artifactID, limit)
}

func (wm *WorldModel) MaterializeArtifact(ctx context.Context, req ArtifactMaterializationRequest) (*MaterializedArtifact, error) {
	if strings.TrimSpace(req.OwnerID) == "" {
		return nil, fmt.Errorf("worldmodel: artifact owner_id required")
	}
	if req.ArtifactID == "" {
		req.ArtifactID = fmt.Sprintf("artifact:%s:%s", req.OwnerID, uuidLikeTime())
	}

	rawSnapshot := marshalArtifactValue(req.Snapshot)
	rawRecoverable := marshalArtifactValue(req.RecoverableOutput)
	metadata := marshalArtifactValue(req.Metadata)
	status := normalizeArtifactStatus(req.Status)
	renderStatus := deriveRenderStatus(req.Kind, req.Subtype, req.Location, rawSnapshot)

	existing, err := store.GetArtifact(ctx, wm.db, req.ArtifactID)
	if err != nil {
		return nil, err
	}

	if err := validateArtifactMaterialization(req.Kind, req.Location, rawSnapshot, renderStatus); err != nil {
		workspaceID, _ := store.GetWorkspaceID(ctx, wm.db)
		artifact := buildCompatArtifact(existing, req, ArtifactStatusFailed, ArtifactRenderStatusInvalid, firstNonEmptyString(workspaceID, "ws-default"))
		artifact.LastError = err.Error()
		artifact.RecoverableOutput = firstNonEmptyString(rawRecoverable, rawSnapshot)
		artifact.Metadata = metadata
		if saveErr := store.SaveArtifact(ctx, wm.db, artifact); saveErr != nil {
			return nil, saveErr
		}
		saved, saveErr := store.GetArtifact(ctx, wm.db, artifact.ID)
		if saveErr != nil {
			return nil, saveErr
		}
		if saved == nil {
			return nil, err
		}
		return &MaterializedArtifact{Artifact: *saved}, err
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, wm.db)
	artifact := buildCompatArtifact(existing, req, status, renderStatus, firstNonEmptyString(workspaceID, "ws-default"))
	artifact.RecoverableOutput = rawRecoverable
	artifact.Metadata = metadata
	if err := store.SaveArtifact(ctx, wm.db, artifact); err != nil {
		return nil, err
	}

	versionID := fmt.Sprintf("%s:v%d", artifact.ID, artifact.Version)
	version, err := store.SaveArtifactVersion(ctx, wm.db, schema.ArtifactVersion{
		ID:                 versionID,
		VersionID:          versionID,
		ArtifactID:         artifact.ID,
		BranchID:           firstNonEmptyString(artifact.CurrentBranchID, "main"),
		VersionNumber:      artifact.Version,
		Version:            artifact.Version,
		ChangeType:         "materialize",
		ChangeMode:         "auto",
		ChangeSummary:      firstNonEmptyString(artifact.Description, "artifact materialization"),
		ValidationState:    "passed",
		CommitState:        schema.VersionStateCommitted,
		PolicySnapshotID:   "worldmodel.materialize",
		Status:             status,
		Location:           artifact.Location,
		Description:        artifact.Description,
		Snapshot:           rawSnapshot,
		ExecutionAttemptID: artifact.SourceExecutionID,
		HistoryAttemptID:   artifact.SourceExecutionID,
		ContentRef: schema.ContentRef{
			URI:        firstNonEmptyString(artifact.Location, fmt.Sprintf("artifact://artifacts/%s/versions/%s/content", artifact.ID, versionID)),
			StorageKey: fmt.Sprintf("artifact_versions/%s/%s", artifact.ID, versionID),
		},
		SizeInBytes: int64(len(rawSnapshot)),
	})
	if err != nil {
		return nil, err
	}

	saved, err := store.GetArtifact(ctx, wm.db, artifact.ID)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, fmt.Errorf("worldmodel: materialized artifact missing after save")
	}
	return &MaterializedArtifact{Artifact: *saved, Version: version}, nil
}

func buildCompatArtifact(existing *schema.Artifact, req ArtifactMaterializationRequest, status, renderStatus, workspaceID string) schema.Artifact {
	now := time.Now().UTC()
	artifact := schema.Artifact{
		ID:                 req.ArtifactID,
		WorkspaceID:        workspaceID,
		OwnerID:            req.OwnerID,
		CanonicalTitle:     firstNonEmptyString(strings.TrimSpace(req.Location), req.ArtifactID),
		DisplayTitle:       firstNonEmptyString(strings.TrimSpace(req.Description), strings.TrimSpace(req.Location), req.ArtifactID),
		Type:               mapArtifactKind(req.Kind),
		Subtype:            strings.TrimSpace(req.Subtype),
		SchemaVersion:      "1.0",
		ContentFormat:      artifactContentFormat(req.Subtype),
		LifecycleState:     mapArtifactStatus(status),
		CurrentBranchID:    "main",
		CurrentVersionID:   "",
		HeadVersionNumber:  1,
		CreatedByActorType: schema.ActorAgent,
		CreatedByActorID:   "navi",
		ProvenanceRootID:   req.ArtifactID,
		Kind:               strings.TrimSpace(req.Kind),
		Location:           strings.TrimSpace(req.Location),
		Description:        strings.TrimSpace(req.Description),
		Status:             status,
		RenderStatus:       renderStatus,
		Version:            1,
		SourceTool:         req.SourceTool,
		SourceExecutionID:  req.SourceExecutionID,
		SourceRunID:        req.SourceRunID,
		CreatedAt:          now,
	}
	if artifact.Kind == "" {
		artifact.Kind = string(artifact.Type)
	}
	if existing != nil {
		artifact.WorkspaceID = firstNonEmptyString(existing.WorkspaceID, artifact.WorkspaceID)
		artifact.ProjectID = existing.ProjectID
		artifact.CurrentBranchID = firstNonEmptyString(existing.CurrentBranchID, artifact.CurrentBranchID)
		artifact.CreatedAt = existing.CreatedAt
		artifact.ProvenanceRootID = firstNonEmptyString(existing.ProvenanceRootID, artifact.ProvenanceRootID)
		artifact.HeadVersionNumber = existing.HeadVersionNumber + 1
		artifact.Version = existing.Version + 1
		if artifact.Version <= 1 {
			artifact.Version = existing.HeadVersionNumber + 1
		}
		artifact.CurrentVersionID = existing.CurrentVersionID
		if existing.Attributes != nil {
			artifact.Attributes = existing.Attributes
		}
	}
	return artifact
}

func mapArtifactKind(kind string) schema.ArtifactType {
	switch strings.TrimSpace(kind) {
	case "code":
		return schema.ArtifactTypeCode
	case "data":
		return schema.ArtifactTypeData
	case "presentation":
		return schema.ArtifactTypePresentation
	default:
		return schema.ArtifactTypeDocument
	}
}

func mapArtifactStatus(status string) schema.ArtifactLifecycleState {
	switch strings.TrimSpace(status) {
	case ArtifactStatusDraft:
		return schema.ArtifactLifecycleDraft
	case ArtifactStatusInProgress:
		return schema.ArtifactLifecycleInProgress
	case ArtifactStatusApproved:
		return schema.ArtifactLifecycleApproved
	case ArtifactStatusFailed:
		return schema.ArtifactLifecycleErrored
	default:
		return schema.ArtifactLifecycleReviewReady
	}
}

func artifactContentFormat(subtype string) string {
	switch strings.TrimSpace(strings.ToLower(subtype)) {
	case "md", "markdown":
		return "text/markdown"
	case "json":
		return "application/json"
	case "html":
		return "text/html"
	default:
		return "text/plain"
	}
}

func normalizeArtifactStatus(status string) string {
	switch strings.TrimSpace(status) {
	case ArtifactStatusDraft:
		return ArtifactStatusDraft
	case ArtifactStatusInProgress:
		return ArtifactStatusInProgress
	case ArtifactStatusApproved:
		return ArtifactStatusApproved
	case ArtifactStatusFailed:
		return ArtifactStatusFailed
	default:
		return ArtifactStatusReviewReady
	}
}

func validateArtifactMaterialization(kind, location, snapshot, renderStatus string) error {
	switch strings.TrimSpace(kind) {
	case "file", "note", "link", "document", "code", "data", "presentation", "":
	default:
		return fmt.Errorf("artifact materialization: unsupported kind %q", kind)
	}
	if strings.TrimSpace(location) == "" && strings.TrimSpace(snapshot) == "" {
		return fmt.Errorf("artifact materialization: location or snapshot required")
	}
	if renderStatus == ArtifactRenderStatusInvalid {
		return fmt.Errorf("artifact materialization: invalid renderability for kind %q", kind)
	}
	return nil
}

func deriveRenderStatus(kind, subtype, location, snapshot string) string {
	if strings.TrimSpace(kind) == "" && strings.TrimSpace(location) == "" && strings.TrimSpace(snapshot) == "" {
		return ArtifactRenderStatusInvalid
	}
	if strings.TrimSpace(subtype) != "" {
		return ArtifactRenderStatusRenderable
	}
	ext := strings.ToLower(filepath.Ext(location))
	switch ext {
	case ".md", ".txt", ".json", ".html", ".yaml", ".yml":
		return ArtifactRenderStatusRenderable
	case "":
		if strings.TrimSpace(snapshot) != "" {
			return ArtifactRenderStatusRenderable
		}
		return ArtifactRenderStatusUnknown
	default:
		return ArtifactRenderStatusRenderable
	}
}

func marshalArtifactValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func uuidLikeTime() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}
