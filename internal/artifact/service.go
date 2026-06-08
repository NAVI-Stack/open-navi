package artifact

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/blob"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// Service provides artifact management operations.
type Service struct {
	db                  *sql.DB
	blob                blob.Store
	registry            *Registry
	eventPublisher      EventPublisher
	reflectionPublisher ReflectionPublisher
}

// NewService creates a new Service.
func NewService(db *sql.DB, b blob.Store) *Service {
	return &Service{
		db:       db,
		blob:     b,
		registry: NewRegistry(),
	}
}

// Registry returns the renderer/editor registry for this service.
func (s *Service) Registry() *Registry {
	return s.registry
}

// CreateArtifact creates a new artifact, an initial branch, and a first version.
func (s *Service) CreateArtifact(ctx context.Context, op schema.ArtifactOperationEnvelope) (schema.Artifact, error) {
	if op.Operation != "create_artifact" {
		return schema.Artifact{}, fmt.Errorf("artifact: invalid operation for CreateArtifact: %s", op.Operation)
	}
	startedAt := time.Now().UTC()

	aID := uuid.New().String()
	bID := uuid.New().String()
	vID := uuid.New().String()

	a := schema.Artifact{
		ID:                 aID,
		WorkspaceID:        op.WorkspaceID,
		ProjectID:          op.ProjectID,
		OwnerID:            op.OwnerID,
		CanonicalTitle:     op.Title,
		DisplayTitle:       op.Title,
		Type:               op.ArtifactType,
		Subtype:            op.ArtifactSubtype,
		SchemaVersion:      "1.0",
		ContentFormat:      op.ContentFormat,
		LifecycleState:     schema.ArtifactLifecycleDraft,
		CurrentBranchID:    bID,
		CurrentVersionID:   "",
		HeadVersionNumber:  0,
		CreatedByActorType: op.ActorType,
		CreatedByActorID:   op.ActorID,
		ProvenanceRootID:   vID,
		Attributes:         op.Attributes,
	}

	b := schema.ArtifactBranch{
		ID:                 bID,
		ArtifactID:         aID,
		Name:               "main",
		BaseVersionID:      vID,
		HeadVersionID:      vID,
		Status:             "active",
		CreatedByActorType: op.ActorType,
		CreatedByActorID:   op.ActorID,
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionStarted, &a, nil, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "started", "", "", startedAt, op.ActorType, op.ActorID)

	payload, err := serializePayload(op.ArtifactSubtype, op.Payload)
	if err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, nil, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassValidationRejection, "serialize_payload", startedAt, op.ActorType, op.ActorID)
		return schema.Artifact{}, err
	}

	v := schema.ArtifactVersion{
		ID:                   vID,
		ArtifactID:           aID,
		BranchID:             bID,
		VersionNumber:        1,
		AuthorActorType:      op.ActorType,
		AuthorActorID:        op.ActorID,
		SourceMessageID:      op.SourceMessageID,
		SourceConversationID: op.SourceConversationID,
		SourceOperationID:    op.OperationID,
		ChangeType:           "create",
		ChangeMode:           "explicit",
		ChangeSummary:        op.Reason,
		ContentRef: schema.ContentRef{
			URI: fmt.Sprintf("artifact://artifacts/%s/versions/%s/content", aID, vID),
			// For now, in V1 MVP, we just store the payload URI.
			// Full blob store integration in OMN-113.
			StorageKey: fmt.Sprintf("artifacts/%s/%s.blob", aID, vID),
		},
		SizeInBytes:      int64(len(payload)),
		ValidationState:  "passed",
		CommitState:      schema.VersionStateCommitted,
		PolicySnapshotID: firstNonEmpty(op.PolicySnapshotID, "artifact.create"),
	}

	// Transactional write
	if err := s.blob.Put(ctx, v.ContentRef.StorageKey, strings.NewReader(payload)); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "blob_put", startedAt, op.ActorType, op.ActorID)
		return a, fmt.Errorf("artifact: store content: %w", err)
	}

	if err := store.SaveArtifact(ctx, s.db, a); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return a, err
	}
	if err := store.SaveArtifactBranch(ctx, s.db, b); err != nil {
		_ = s.markArtifactErrored(ctx, a, v.ContentRef, err)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_branch", startedAt, op.ActorType, op.ActorID)
		return a, err
	}
	if _, err := store.SaveArtifactVersion(ctx, s.db, v); err != nil {
		_ = s.markArtifactErrored(ctx, a, v.ContentRef, err)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_version", startedAt, op.ActorType, op.ActorID)
		return a, err
	}
	a.CurrentVersionID = vID
	a.HeadVersionNumber = 1
	if err := store.SaveArtifact(ctx, s.db, a); err != nil {
		_ = s.markArtifactErrored(ctx, a, v.ContentRef, err)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact_head", startedAt, op.ActorType, op.ActorID)
		return a, err
	}
	if err := s.persistReferences(ctx, a.ID, b.ID, v.ID, op, "created"); err != nil {
		_ = s.markArtifactErrored(ctx, a, v.ContentRef, err)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, &a, &v, string(op.Operation), s.registry.GetRenderer(op.ArtifactSubtype).ComponentID, "failed", schema.FailureClassStorageFailure, "persist_references", startedAt, op.ActorType, op.ActorID)
		return a, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactRendererResolved, &a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "resolved", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactCreated, &a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactVersionCommitted, &a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "committed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, &a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, &a, &v, string(op.Operation), fmt.Sprintf("Artifact %s created", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "")

	return a, nil
}

// UpdateContent applies a replacement, patch, or append to an artifact.
func (s *Service) UpdateContent(ctx context.Context, op schema.ArtifactOperationEnvelope) (schema.ArtifactVersion, error) {
	startedAt := time.Now().UTC()
	// Conflict Detection
	a, err := store.GetArtifact(ctx, s.db, op.TargetArtifactID)
	if err != nil {
		return schema.ArtifactVersion{}, err
	}
	if a == nil {
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: artifact not found: %s", op.TargetArtifactID)
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionStarted, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "started", "", "", startedAt, op.ActorType, op.ActorID)

	if op.ExpectedBaseVersionID != "" && a.CurrentVersionID != op.ExpectedBaseVersionID {
		if a.Attributes == nil {
			a.Attributes = map[string]any{}
		}
		a.Attributes["conflict_state"] = "conflicted"
		a.Attributes["conflict_expected_base_version_id"] = op.ExpectedBaseVersionID
		a.Attributes["conflict_head_version_id"] = a.CurrentVersionID
		_ = store.SaveArtifact(ctx, s.db, *a)
		s.emitArtifactEvent(ctx, schema.FactArtifactConflictDetected, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "conflicted", schema.FailureClassVersionConflict, "version_conflict", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, nil, string(op.Operation), fmt.Sprintf("Artifact %s update conflicted", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict")
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: version_conflict: current head %s differs from expected base %s", a.CurrentVersionID, op.ExpectedBaseVersionID)
	}

	vID := uuid.New().String()
	v := schema.ArtifactVersion{
		ID:                   vID,
		ArtifactID:           a.ID,
		BranchID:             a.CurrentBranchID,
		VersionNumber:        a.HeadVersionNumber + 1,
		ParentVersionID:      a.CurrentVersionID,
		BaseVersionID:        a.CurrentVersionID,
		AuthorActorType:      op.ActorType,
		AuthorActorID:        op.ActorID,
		SourceMessageID:      op.SourceMessageID,
		SourceConversationID: op.SourceConversationID,
		SourceOperationID:    op.OperationID,
		ChangeType:           string(op.Operation),
		ChangeMode:           "explicit",
		ChangeSummary:        op.Reason,
		ContentRef: schema.ContentRef{
			URI:        fmt.Sprintf("artifact://artifacts/%s/versions/%s/content", a.ID, vID),
			StorageKey: fmt.Sprintf("artifacts/%s/%s.blob", a.ID, vID),
		},
		PolicySnapshotID: op.PolicySnapshotID,
		CommitState:      schema.VersionStateCommitted,
		ValidationState:  "passed",
	}

	baseContent, err := s.loadVersionContent(ctx, v.BaseVersionID)
	if err != nil {
		baseContent = ""
	}
	payload, err := prepareContentForOperation(a.Subtype, op.Operation, baseContent, op.Payload)
	if err != nil {
		failureClass := classifyArtifactUpdateFailure(op.Operation, err)
		s.recordArtifactFailure(ctx, a, failureClass, err)
		if op.Operation == schema.ArtifactOpPatch {
			s.emitArtifactEvent(ctx, schema.FactArtifactPatchFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", failureClass, "prepare_content", startedAt, op.ActorType, op.ActorID)
		}
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", failureClass, "prepare_content", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &v, string(op.Operation), fmt.Sprintf("Artifact %s update failed", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "failed", failureClass, "prepare_content")
		return schema.ArtifactVersion{}, err
	}
	v.SizeInBytes = int64(len(payload))
	v.PatchStrategy = choosePatchStrategy(op.Operation, s.registry.GetEditor(a.Subtype))
	v.PolicySnapshotID = firstNonEmpty(op.PolicySnapshotID, "artifact.update")

	// Persist to blob store
	if err := s.blob.Put(ctx, v.ContentRef.StorageKey, strings.NewReader(payload)); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "blob_put", startedAt, op.ActorType, op.ActorID)
		return v, fmt.Errorf("artifact: store update content: %w", err)
	}
	if diff, diffErr := buildDiffPayload(a.Subtype, baseContent, payload); diffErr == nil {
		diffKey := fmt.Sprintf("artifacts/%s/%s.diff.json", a.ID, vID)
		if putErr := s.blob.Put(ctx, diffKey, strings.NewReader(diff)); putErr == nil {
			v.DiffRef = &schema.ContentRef{
				URI:        fmt.Sprintf("artifact://artifacts/%s/versions/%s/diff", a.ID, vID),
				StorageKey: diffKey,
			}
		}
	}

	// Apply metadata patch if present
	if len(op.MetadataPatch) > 0 {
		if a.Attributes == nil {
			a.Attributes = make(map[string]any)
		}
		for k, v := range op.MetadataPatch {
			a.Attributes[k] = v
		}
	}
	clearArtifactConflictState(a.Attributes)

	// Update artifact head only after the new content is ready to commit.
	previousHeadVersionID := a.CurrentVersionID
	previousHeadVersionNumber := a.HeadVersionNumber
	a.CurrentVersionID = vID
	a.HeadVersionNumber = v.VersionNumber

	if _, err := store.SaveArtifactVersion(ctx, s.db, v); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_version", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactVersion{}, err
	}
	if err := s.persistReferences(ctx, a.ID, a.CurrentBranchID, v.ID, op, "updated"); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "persist_references", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactVersion{}, err
	}
	advanced, err := store.SaveArtifactHeadIfCurrent(ctx, s.db, *a, previousHeadVersionID, previousHeadVersionNumber)
	if err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return v, err
	}
	if !advanced {
		rollbackPreparedArtifactVersion(ctx, s.db, v.ID)
		latest, _ := store.GetArtifact(ctx, s.db, a.ID)
		if latest != nil {
			a = latest
			if a.Attributes == nil {
				a.Attributes = map[string]any{}
			}
			a.Attributes["conflict_state"] = "conflicted"
			a.Attributes["conflict_expected_base_version_id"] = previousHeadVersionID
			a.Attributes["conflict_head_version_id"] = a.CurrentVersionID
			_ = store.SaveArtifact(ctx, s.db, *a)
		}
		s.emitArtifactEvent(ctx, schema.FactArtifactConflictDetected, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "conflicted", schema.FailureClassVersionConflict, "version_conflict_commit", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict_commit", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &v, string(op.Operation), fmt.Sprintf("Artifact %s update conflicted during commit", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict_commit")
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: version_conflict: current head changed while committing update")
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactRendererResolved, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "resolved", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactUpdated, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactVersionCommitted, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "committed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, a, &v, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, a, &v, string(op.Operation), fmt.Sprintf("Artifact %s updated", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "")

	return v, nil
}

func (s *Service) loadVersionContent(ctx context.Context, versionID string) (string, error) {
	version, err := store.GetArtifactVersion(ctx, s.db, versionID)
	if err != nil {
		return "", err
	}
	reader, err := s.blob.Get(ctx, version.ContentRef.StorageKey)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("artifact: read version content: %w", err)
	}
	return string(buf), nil
}

// ReadVersionContent loads the stored content payload for a committed version.
func (s *Service) ReadVersionContent(ctx context.Context, versionID string) (string, error) {
	return s.loadVersionContent(ctx, versionID)
}

// ReadVersionDiff loads the stored diff payload for a committed version.
func (s *Service) ReadVersionDiff(ctx context.Context, versionID string) (string, error) {
	version, err := store.GetArtifactVersion(ctx, s.db, versionID)
	if err != nil {
		return "", err
	}
	if version.DiffRef == nil || strings.TrimSpace(version.DiffRef.StorageKey) == "" {
		return "", fmt.Errorf("artifact: diff not available for version %s", versionID)
	}
	reader, err := s.blob.Get(ctx, version.DiffRef.StorageKey)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("artifact: read version diff: %w", err)
	}
	return string(buf), nil
}

func serializePayload(subtype string, payload any) (string, error) {
	switch normalizeSubtype(subtype) {
	case "":
		if b, ok := payload.(string); ok {
			return b, nil
		}
		bb, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("artifact: encode payload: %w", err)
		}
		return string(bb), nil
	default:
		return serializeForSubtype(subtype, payload)
	}
}

func choosePatchStrategy(op schema.ArtifactOperation, editor EditorDescriptor) string {
	if len(editor.PatchStrategies) == 0 {
		if op == schema.ArtifactOpReplace {
			return PatchStrategyFull
		}
		return ""
	}
	if op == schema.ArtifactOpReplace {
		return PatchStrategyFull
	}
	return editor.PatchStrategies[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func clearArtifactConflictState(attrs map[string]any) {
	if attrs == nil {
		return
	}
	delete(attrs, "conflict_state")
	delete(attrs, "conflict_expected_base_version_id")
	delete(attrs, "conflict_head_version_id")
}

func rollbackPreparedArtifactVersion(ctx context.Context, db *sql.DB, versionID string) {
	if db == nil || strings.TrimSpace(versionID) == "" {
		return
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM artifact_references WHERE version_id = ?`, versionID)
	_, _ = db.ExecContext(ctx, `DELETE FROM artifact_versions WHERE id = ?`, versionID)
}

func (s *Service) markArtifactErrored(ctx context.Context, artifact schema.Artifact, contentRef schema.ContentRef, cause error) error {
	artifact.LifecycleState = schema.ArtifactLifecycleErrored
	artifact.LastError = cause.Error()
	recovery, _ := json.Marshal(map[string]any{
		"recovery_status": "open",
		"draft_content_ref": map[string]any{
			"uri":             contentRef.URI,
			"storage_key":     contentRef.StorageKey,
			"checksum_sha256": contentRef.ChecksumSHA256,
		},
	})
	artifact.RecoverableOutput = string(recovery)
	return store.SaveArtifact(ctx, s.db, artifact)
}

func (s *Service) recordArtifactFailure(ctx context.Context, artifact *schema.Artifact, failureClass schema.FailureClass, cause error) {
	if artifact == nil {
		return
	}
	if artifact.Attributes == nil {
		artifact.Attributes = map[string]any{}
	}
	artifact.LastError = cause.Error()
	artifact.Attributes["failure_class"] = string(failureClass)
	artifact.Attributes["last_error"] = cause.Error()
	if _, ok := artifact.Attributes["recoverable_output"]; !ok {
		recovery, _ := json.Marshal(map[string]any{
			"recovery_status": "open",
			"retryable":       true,
		})
		artifact.RecoverableOutput = string(recovery)
	}
	_ = store.SaveArtifact(ctx, s.db, *artifact)
}

func classifyArtifactUpdateFailure(op schema.ArtifactOperation, err error) schema.FailureClass {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "version_conflict"):
		return schema.FailureClassVersionConflict
	case strings.Contains(msg, "invalid json"), strings.Contains(msg, "schema"), strings.Contains(msg, "structured json patch"):
		return schema.FailureClassSchemaInvalid
	case op == schema.ArtifactOpPatch:
		return schema.FailureClassPatchApplyFailed
	default:
		return schema.FailureClassValidationRejection
	}
}

func (s *Service) persistReferences(ctx context.Context, artifactID, branchID, versionID string, op schema.ArtifactOperationEnvelope, relationship string) error {
	var refs []schema.ArtifactReference
	if strings.TrimSpace(op.SourceConversationID) != "" {
		refs = append(refs, schema.ArtifactReference{
			ArtifactID:   artifactID,
			SourceKind:   "conversation",
			SourceID:     op.SourceConversationID,
			Relationship: relationship,
			VersionID:    versionID,
			BranchID:     branchID,
		})
	}
	if strings.TrimSpace(op.SourceMessageID) != "" {
		refs = append(refs, schema.ArtifactReference{
			ArtifactID:   artifactID,
			SourceKind:   "message",
			SourceID:     op.SourceMessageID,
			Relationship: relationship,
			VersionID:    versionID,
			BranchID:     branchID,
		})
	}
	if strings.TrimSpace(op.ProjectID) != "" {
		refs = append(refs, schema.ArtifactReference{
			ArtifactID:   artifactID,
			SourceKind:   "project",
			SourceID:     op.ProjectID,
			Relationship: relationship,
			VersionID:    versionID,
			BranchID:     branchID,
		})
	}
	for _, ref := range refs {
		if err := store.SaveArtifactReference(ctx, s.db, ref); err != nil {
			return fmt.Errorf("artifact: save reference: %w", err)
		}
	}
	return nil
}

// ArchiveArtifact transitions an artifact to archived state.
func (s *Service) ArchiveArtifact(ctx context.Context, op schema.ArtifactOperationEnvelope) error {
	startedAt := time.Now().UTC()
	a, err := store.GetArtifact(ctx, s.db, op.TargetArtifactID)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("artifact: artifact not found: %s", op.TargetArtifactID)
	}

	now := time.Now().UTC()
	a.LifecycleState = schema.ArtifactLifecycleArchived
	a.ArchivedAt = &now

	if err := store.SaveArtifact(ctx, s.db, *a); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactArchived, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, a, nil, string(op.Operation), fmt.Sprintf("Artifact %s archived", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "")
	return nil
}

// TransitionLifecycle updates the state of an existing artifact (e.g., from in_progress to review_ready).
func (s *Service) TransitionLifecycle(ctx context.Context, op schema.ArtifactOperationEnvelope) (schema.Artifact, error) {
	startedAt := time.Now().UTC()
	a, err := store.GetArtifact(ctx, s.db, op.TargetArtifactID)
	if err != nil {
		return schema.Artifact{}, err
	}
	if a == nil {
		return schema.Artifact{}, fmt.Errorf("artifact: artifact not found: %s", op.TargetArtifactID)
	}

	previous := a.LifecycleState
	a.LifecycleState = op.ToLifecycleState
	if err := store.SaveArtifact(ctx, s.db, *a); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return *a, err
	}
	if previous == schema.ArtifactLifecycleArchived && a.LifecycleState != schema.ArtifactLifecycleArchived {
		s.emitArtifactEvent(ctx, schema.FactArtifactRestored, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactLifecycleChanged, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, a, nil, string(op.Operation), fmt.Sprintf("Artifact %s moved to %s", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID), a.LifecycleState), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "")

	return *a, nil
}

// BranchArtifact creates a new active branch from a selected version and switches the artifact to it.
func (s *Service) BranchArtifact(ctx context.Context, op schema.ArtifactOperationEnvelope) (schema.ArtifactBranch, error) {
	startedAt := time.Now().UTC()
	a, err := store.GetArtifact(ctx, s.db, op.TargetArtifactID)
	if err != nil {
		return schema.ArtifactBranch{}, err
	}
	if a == nil {
		return schema.ArtifactBranch{}, fmt.Errorf("artifact: artifact not found: %s", op.TargetArtifactID)
	}
	baseVersionID := firstNonEmpty(op.ExpectedBaseVersionID, a.CurrentVersionID)
	if baseVersionID == "" {
		return schema.ArtifactBranch{}, fmt.Errorf("artifact: branch base version required")
	}
	baseVersion, err := store.GetArtifactVersion(ctx, s.db, baseVersionID)
	if err != nil {
		return schema.ArtifactBranch{}, err
	}
	if baseVersion.ArtifactID != a.ID {
		return schema.ArtifactBranch{}, fmt.Errorf("artifact: version %s does not belong to artifact %s", baseVersionID, a.ID)
	}

	branch := schema.ArtifactBranch{
		ID:                 firstNonEmpty(op.TargetBranchID, uuid.New().String()),
		ArtifactID:         a.ID,
		Name:               firstNonEmpty(op.Reason, fmt.Sprintf("branch-%d", time.Now().UTC().Unix())),
		BaseVersionID:      baseVersionID,
		HeadVersionID:      baseVersionID,
		Status:             "active",
		CreatedByActorType: firstActorType(op.ActorType),
		CreatedByActorID:   op.ActorID,
	}
	if err := store.SaveArtifactBranch(ctx, s.db, branch); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_branch", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactBranch{}, err
	}
	a.CurrentBranchID = branch.ID
	a.CurrentVersionID = baseVersionID
	if a.Attributes == nil {
		a.Attributes = map[string]any{}
	}
	a.Attributes["conflict_state"] = "branched"
	a.Attributes["last_branch_id"] = branch.ID
	a.Attributes["last_branch_base_version_id"] = baseVersionID
	if err := store.SaveArtifact(ctx, s.db, *a); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactBranch{}, err
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactBranched, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, a, nil, string(op.Operation), s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, a, nil, string(op.Operation), fmt.Sprintf("Artifact %s branched", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, s.registry.GetRenderer(a.Subtype).ComponentID, "completed", "", "")
	return branch, nil
}

// RestoreVersion creates a new head version whose content matches a prior committed version.
func (s *Service) RestoreVersion(ctx context.Context, op schema.ArtifactOperationEnvelope) (schema.ArtifactVersion, error) {
	startedAt := time.Now().UTC()
	a, err := store.GetArtifact(ctx, s.db, op.TargetArtifactID)
	if err != nil {
		return schema.ArtifactVersion{}, err
	}
	if a == nil {
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: artifact not found: %s", op.TargetArtifactID)
	}
	targetVersionID := strings.TrimSpace(op.TargetVersionID)
	if targetVersionID == "" {
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: restore target version required")
	}
	targetVersion, err := store.GetArtifactVersion(ctx, s.db, targetVersionID)
	if err != nil {
		return schema.ArtifactVersion{}, err
	}
	if targetVersion.ArtifactID != a.ID {
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: version %s does not belong to artifact %s", targetVersionID, a.ID)
	}

	renderer := s.registry.GetRenderer(a.Subtype)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionStarted, a, &targetVersion, string(op.Operation), renderer.ComponentID, "started", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactVersionRestoreRequested, a, &targetVersion, string(op.Operation), renderer.ComponentID, "requested", "", "", startedAt, op.ActorType, op.ActorID)

	if op.ExpectedBaseVersionID != "" && a.CurrentVersionID != op.ExpectedBaseVersionID {
		if a.Attributes == nil {
			a.Attributes = map[string]any{}
		}
		a.Attributes["conflict_state"] = "conflicted"
		a.Attributes["conflict_expected_base_version_id"] = op.ExpectedBaseVersionID
		a.Attributes["conflict_head_version_id"] = a.CurrentVersionID
		_ = store.SaveArtifact(ctx, s.db, *a)
		s.emitArtifactEvent(ctx, schema.FactArtifactConflictDetected, a, &targetVersion, string(op.Operation), renderer.ComponentID, "conflicted", schema.FailureClassVersionConflict, "version_conflict", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &targetVersion, string(op.Operation), fmt.Sprintf("Artifact %s restore conflicted", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, renderer.ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict")
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: version_conflict: current head %s differs from expected base %s", a.CurrentVersionID, op.ExpectedBaseVersionID)
	}

	restoredContent, err := s.loadVersionContent(ctx, targetVersion.ID)
	if err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "read_restore_source", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &targetVersion, string(op.Operation), fmt.Sprintf("Artifact %s restore failed", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "read_restore_source")
		return schema.ArtifactVersion{}, err
	}

	baseContent := ""
	if strings.TrimSpace(a.CurrentVersionID) != "" {
		if currentContent, readErr := s.loadVersionContent(ctx, a.CurrentVersionID); readErr == nil {
			baseContent = currentContent
		}
	}

	vID := uuid.New().String()
	v := schema.ArtifactVersion{
		ID:                   vID,
		ArtifactID:           a.ID,
		BranchID:             a.CurrentBranchID,
		VersionNumber:        a.HeadVersionNumber + 1,
		ParentVersionID:      a.CurrentVersionID,
		BaseVersionID:        targetVersion.ID,
		AuthorActorType:      op.ActorType,
		AuthorActorID:        op.ActorID,
		SourceMessageID:      op.SourceMessageID,
		SourceConversationID: op.SourceConversationID,
		SourceOperationID:    op.OperationID,
		ChangeType:           string(op.Operation),
		ChangeMode:           "explicit",
		ChangeSummary:        firstNonEmpty(op.Reason, fmt.Sprintf("Restored from version %d", targetVersion.VersionNumber)),
		ContentRef: schema.ContentRef{
			URI:        fmt.Sprintf("artifact://artifacts/%s/versions/%s/content", a.ID, vID),
			StorageKey: fmt.Sprintf("artifacts/%s/%s.blob", a.ID, vID),
		},
		PolicySnapshotID: firstNonEmpty(op.PolicySnapshotID, "artifact.restore_version"),
		CommitState:      schema.VersionStateCommitted,
		ValidationState:  "passed",
		PatchStrategy:    PatchStrategyFull,
		SizeInBytes:      int64(len(restoredContent)),
	}

	if err := s.blob.Put(ctx, v.ContentRef.StorageKey, strings.NewReader(restoredContent)); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "blob_put", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &targetVersion, string(op.Operation), fmt.Sprintf("Artifact %s restore failed", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "blob_put")
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: store restore content: %w", err)
	}
	if diff, diffErr := buildDiffPayload(a.Subtype, baseContent, restoredContent); diffErr == nil {
		diffKey := fmt.Sprintf("artifacts/%s/%s.diff.json", a.ID, vID)
		if putErr := s.blob.Put(ctx, diffKey, strings.NewReader(diff)); putErr == nil {
			v.DiffRef = &schema.ContentRef{
				URI:        fmt.Sprintf("artifact://artifacts/%s/versions/%s/diff", a.ID, vID),
				StorageKey: diffKey,
			}
		}
	}

	clearArtifactConflictState(a.Attributes)
	previousHeadVersionID := a.CurrentVersionID
	previousHeadVersionNumber := a.HeadVersionNumber
	a.CurrentVersionID = vID
	a.HeadVersionNumber = v.VersionNumber
	if _, err := store.SaveArtifactVersion(ctx, s.db, v); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "save_version", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactVersion{}, err
	}
	if err := s.persistReferences(ctx, a.ID, a.CurrentBranchID, v.ID, op, "restored"); err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "persist_references", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactVersion{}, err
	}
	advanced, err := store.SaveArtifactHeadIfCurrent(ctx, s.db, *a, previousHeadVersionID, previousHeadVersionNumber)
	if err != nil {
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassStorageFailure, "save_artifact", startedAt, op.ActorType, op.ActorID)
		return schema.ArtifactVersion{}, err
	}
	if !advanced {
		rollbackPreparedArtifactVersion(ctx, s.db, v.ID)
		latest, _ := store.GetArtifact(ctx, s.db, a.ID)
		if latest != nil {
			a = latest
			if a.Attributes == nil {
				a.Attributes = map[string]any{}
			}
			a.Attributes["conflict_state"] = "conflicted"
			a.Attributes["conflict_expected_base_version_id"] = previousHeadVersionID
			a.Attributes["conflict_head_version_id"] = a.CurrentVersionID
			_ = store.SaveArtifact(ctx, s.db, *a)
		}
		s.emitArtifactEvent(ctx, schema.FactArtifactConflictDetected, a, &targetVersion, string(op.Operation), renderer.ComponentID, "conflicted", schema.FailureClassVersionConflict, "version_conflict_commit", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactEvent(ctx, schema.FactArtifactExecutionFailed, a, &targetVersion, string(op.Operation), renderer.ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict_commit", startedAt, op.ActorType, op.ActorID)
		s.emitArtifactReflection(ctx, a, &targetVersion, string(op.Operation), fmt.Sprintf("Artifact %s restore conflicted during commit", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID)), op.SourceConversationID, renderer.ComponentID, "failed", schema.FailureClassVersionConflict, "version_conflict_commit")
		return schema.ArtifactVersion{}, fmt.Errorf("artifact: version_conflict: current head changed while committing restore")
	}
	s.emitArtifactEvent(ctx, schema.FactArtifactRendererResolved, a, &v, string(op.Operation), renderer.ComponentID, "resolved", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactUpdated, a, &v, string(op.Operation), renderer.ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactVersionCommitted, a, &v, string(op.Operation), renderer.ComponentID, "committed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactEvent(ctx, schema.FactArtifactExecutionCompleted, a, &v, string(op.Operation), renderer.ComponentID, "completed", "", "", startedAt, op.ActorType, op.ActorID)
	s.emitArtifactReflection(ctx, a, &v, string(op.Operation), fmt.Sprintf("Artifact %s restored from version %d", firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, a.ID), targetVersion.VersionNumber), op.SourceConversationID, renderer.ComponentID, "completed", "", "")
	return v, nil
}
