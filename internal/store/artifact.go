package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

const (
	artifactAttrKind              = "_wm_kind"
	artifactAttrLocation          = "_wm_location"
	artifactAttrDescription       = "_wm_description"
	artifactAttrStatus            = "_wm_status"
	artifactAttrRenderStatus      = "_wm_render_status"
	artifactAttrVersion           = "_wm_version"
	artifactAttrSourceTool        = "_wm_source_tool"
	artifactAttrSourceExecutionID = "_wm_source_execution_id"
	artifactAttrSourceRunID       = "_wm_source_run_id"
	artifactAttrLastError         = "_wm_last_error"
	artifactAttrRecoverableOutput = "_wm_recoverable_output"
	artifactAttrMetadata          = "_wm_metadata"

	artifactVersionAttrStatus             = "_wm_status"
	artifactVersionAttrLocation           = "_wm_location"
	artifactVersionAttrDescription        = "_wm_description"
	artifactVersionAttrSnapshot           = "_wm_snapshot"
	artifactVersionAttrExecutionAttemptID = "_wm_execution_attempt_id"
)

// SaveArtifact inserts or updates a first-class artifact entity.
func SaveArtifact(ctx context.Context, db *sql.DB, a schema.Artifact) error {
	if err := validateArtifactProjectRelationship(ctx, db, a.WorkspaceID, a.ProjectID); err != nil {
		return err
	}
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	applyArtifactCompatToAttributes(&a)

	attrJSON, _ := json.Marshal(a.Attributes)
	if attrJSON == nil {
		attrJSON = []byte("{}")
	}

	var archivedStr *string
	if a.ArchivedAt != nil {
		s := a.ArchivedAt.Format(timeFormat)
		archivedStr = &s
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifacts (
			id, workspace_id, project_id, owner_id, 
			canonical_title, display_title, type, subtype, 
			schema_version, content_format, lifecycle_state, 
			current_branch_id, current_version_id, head_version_number, 
			created_by_actor_type, created_by_actor_id, provenance_root_id, 
			attributes, created_at, updated_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			workspace_id=excluded.workspace_id,
			project_id=excluded.project_id,
			owner_id=excluded.owner_id,
			canonical_title=excluded.canonical_title,
			display_title=excluded.display_title,
			type=excluded.type,
			subtype=excluded.subtype,
			schema_version=excluded.schema_version,
			content_format=excluded.content_format,
			lifecycle_state=excluded.lifecycle_state,
			current_branch_id=excluded.current_branch_id,
			current_version_id=excluded.current_version_id,
			head_version_number=excluded.head_version_number,
			attributes=excluded.attributes,
			updated_at=excluded.updated_at,
			archived_at=excluded.archived_at
	`, a.ID, a.WorkspaceID, a.ProjectID, a.OwnerID,
		a.CanonicalTitle, a.DisplayTitle, string(a.Type), a.Subtype,
		a.SchemaVersion, a.ContentFormat, string(a.LifecycleState),
		a.CurrentBranchID, a.CurrentVersionID, a.HeadVersionNumber,
		string(a.CreatedByActorType), a.CreatedByActorID, a.ProvenanceRootID,
		string(attrJSON), a.CreatedAt.Format(timeFormat), a.UpdatedAt.Format(timeFormat), archivedStr)
	if err != nil {
		return fmt.Errorf("store: save artifact: %w", err)
	}
	return nil
}

// SaveArtifactHeadIfCurrent advances an artifact head only when the caller's
// expected current version and head number still match the stored row.
func SaveArtifactHeadIfCurrent(ctx context.Context, db *sql.DB, a schema.Artifact, expectedCurrentVersionID string, expectedHeadVersionNumber int) (bool, error) {
	if strings.TrimSpace(a.ID) == "" {
		return false, fmt.Errorf("store: artifact id required")
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	applyArtifactCompatToAttributes(&a)

	attrJSON, _ := json.Marshal(a.Attributes)
	if attrJSON == nil {
		attrJSON = []byte("{}")
	}

	res, err := db.ExecContext(ctx, `
		UPDATE artifacts
		SET current_branch_id = ?, current_version_id = ?, head_version_number = ?, attributes = ?, updated_at = ?
		WHERE id = ? AND current_version_id = ? AND head_version_number = ?
	`, a.CurrentBranchID, a.CurrentVersionID, a.HeadVersionNumber, string(attrJSON), a.UpdatedAt.Format(timeFormat), a.ID, expectedCurrentVersionID, expectedHeadVersionNumber)
	if err != nil {
		return false, fmt.Errorf("store: save artifact head if current: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: artifact head rows affected: %w", err)
	}
	return rows > 0, nil
}

// GetArtifact loads an artifact by ID.
func GetArtifact(ctx context.Context, db *sql.DB, id string) (*schema.Artifact, error) {
	row := db.QueryRowContext(ctx, `
		SELECT 
			id, workspace_id, project_id, owner_id, 
			canonical_title, display_title, type, subtype, 
			schema_version, content_format, lifecycle_state, 
			current_branch_id, current_version_id, head_version_number, 
			created_by_actor_type, created_by_actor_id, provenance_root_id, 
			attributes, created_at, updated_at, archived_at
		FROM artifacts WHERE id = ?
	`, id)
	return scanArtifactScanner(row.Scan)
}

// SaveArtifactVersion inserts a committed artifact version and auto-increments the
// version number when omitted.
func SaveArtifactVersion(ctx context.Context, db *sql.DB, v schema.ArtifactVersion) (schema.ArtifactVersion, error) {
	if v.ArtifactID == "" {
		return schema.ArtifactVersion{}, fmt.Errorf("store: artifact version requires artifact_id")
	}
	if v.ID == "" {
		if v.VersionID != "" {
			v.ID = v.VersionID
		} else {
			v.ID = uuid.New().String()
		}
	}
	v.VersionID = v.ID
	if v.VersionNumber <= 0 {
		if v.Version > 0 {
			v.VersionNumber = v.Version
		} else {
			if err := db.QueryRowContext(ctx, `
				SELECT COALESCE(MAX(version_number), 0) + 1
				FROM artifact_versions
				WHERE artifact_id = ?
			`, v.ArtifactID).Scan(&v.VersionNumber); err != nil {
				return schema.ArtifactVersion{}, fmt.Errorf("store: next artifact version: %w", err)
			}
		}
	}
	v.Version = v.VersionNumber
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.BranchID == "" {
		v.BranchID = "main"
	}
	if v.AuthorActorType == "" {
		v.AuthorActorType = schema.ActorAgent
	}
	if v.SourceOperationID == "" {
		v.SourceOperationID = firstCompatValue(v.ExecutionAttemptID, v.ID)
	}
	if v.ChangeType == "" {
		v.ChangeType = "materialize"
	}
	if v.ChangeMode == "" {
		v.ChangeMode = "auto"
	}
	if v.ChangeSummary == "" {
		v.ChangeSummary = firstCompatValue(v.Description, "artifact materialization")
	}
	if v.ContentRef.URI == "" {
		v.ContentRef.URI = firstCompatValue(v.Location, fmt.Sprintf("artifact://artifacts/%s/versions/%s/content", v.ArtifactID, v.ID))
	}
	if v.ContentRef.StorageKey == "" {
		v.ContentRef.StorageKey = fmt.Sprintf("artifact_versions/%s/%s", v.ArtifactID, v.ID)
	}
	if v.ValidationState == "" {
		v.ValidationState = "passed"
	}
	if v.CommitState == "" {
		v.CommitState = schema.VersionStateCommitted
	}
	if v.PolicySnapshotID == "" {
		v.PolicySnapshotID = "worldmodel.materialize"
	}
	if v.SizeInBytes == 0 && v.Snapshot != "" {
		v.SizeInBytes = int64(len(v.Snapshot))
	}

	if v.AttributeDelta == nil {
		v.AttributeDelta = map[string]any{}
	}
	if v.Status != "" {
		v.AttributeDelta[artifactVersionAttrStatus] = v.Status
	}
	if v.Location != "" {
		v.AttributeDelta[artifactVersionAttrLocation] = v.Location
	}
	if v.Description != "" {
		v.AttributeDelta[artifactVersionAttrDescription] = v.Description
	}
	if v.Snapshot != "" {
		v.AttributeDelta[artifactVersionAttrSnapshot] = v.Snapshot
	}
	if v.ExecutionAttemptID != "" {
		v.AttributeDelta[artifactVersionAttrExecutionAttemptID] = v.ExecutionAttemptID
	}

	sourceSnapshots, _ := json.Marshal(v.SourceSnapshotIDs)
	inputSnapshots, _ := json.Marshal(v.ArtifactInputSnapshotIDs)
	attrDelta, _ := json.Marshal(v.AttributeDelta)

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_versions (
			id, artifact_id, branch_id, version_number, parent_version_id, base_version_id,
			author_actor_type, author_actor_id, source_message_id, source_conversation_id, source_operation_id,
			change_type, change_mode, change_summary, 
			content_uri, storage_key, checksum_sha256, 
			rendered_uri, rendered_storage_key, rendered_checksum,
			diff_uri, diff_storage_key, diff_checksum,
			size_in_bytes, patch_strategy, patch_metadata,
			validation_state, commit_state,
			project_snapshot_id, policy_snapshot_id, source_snapshot_ids, artifact_input_snapshot_ids,
			history_command_id, history_attempt_id, attribute_delta, created_at
		) VALUES (
			?, ?, ?, ?, ?, ?, 
			?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?
		)
	`, v.ID, v.ArtifactID, v.BranchID, v.VersionNumber, v.ParentVersionID, v.BaseVersionID,
		string(v.AuthorActorType), v.AuthorActorID, v.SourceMessageID, v.SourceConversationID, v.SourceOperationID,
		v.ChangeType, v.ChangeMode, v.ChangeSummary,
		v.ContentRef.URI, v.ContentRef.StorageKey, v.ContentRef.ChecksumSHA256,
		optionalURI(v.RenderedRef), optionalKey(v.RenderedRef), optionalChecksum(v.RenderedRef),
		optionalURI(v.DiffRef), optionalKey(v.DiffRef), optionalChecksum(v.DiffRef),
		v.SizeInBytes, v.PatchStrategy, v.PatchMetadata,
		v.ValidationState, string(v.CommitState),
		v.ProjectSnapshotID, v.PolicySnapshotID, string(sourceSnapshots), string(inputSnapshots),
		v.HistoryCommandID, v.HistoryAttemptID, string(attrDelta), v.CreatedAt.Format(timeFormat))

	if err != nil {
		return schema.ArtifactVersion{}, fmt.Errorf("store: save artifact version: %w", err)
	}
	return hydrateArtifactVersionCompat(v), nil
}

func GetArtifactVersion(ctx context.Context, db *sql.DB, versionID string) (schema.ArtifactVersion, error) {
	var v schema.ArtifactVersion
	var actorType, commitState, createdStr string
	var sourceSnaps, inputSnaps, attrDelta string
	var rURI, rKey, rCheck, dURI, dKey, dCheck *string

	err := db.QueryRowContext(ctx, `
		SELECT 
			id, artifact_id, branch_id, version_number, parent_version_id, base_version_id,
			author_actor_type, author_actor_id, source_message_id, source_conversation_id, source_operation_id,
			change_type, change_mode, change_summary, 
			content_uri, storage_key, checksum_sha256, 
			rendered_uri, rendered_storage_key, rendered_checksum,
			diff_uri, diff_storage_key, diff_checksum,
			size_in_bytes, patch_strategy, patch_metadata,
			validation_state, commit_state,
			project_snapshot_id, policy_snapshot_id, source_snapshot_ids, artifact_input_snapshot_ids,
			history_command_id, history_attempt_id, attribute_delta, created_at
		FROM artifact_versions WHERE id = ?
	`, versionID).Scan(
		&v.ID, &v.ArtifactID, &v.BranchID, &v.VersionNumber, &v.ParentVersionID, &v.BaseVersionID,
		&actorType, &v.AuthorActorID, &v.SourceMessageID, &v.SourceConversationID, &v.SourceOperationID,
		&v.ChangeType, &v.ChangeMode, &v.ChangeSummary,
		&v.ContentRef.URI, &v.ContentRef.StorageKey, &v.ContentRef.ChecksumSHA256,
		&rURI, &rKey, &rCheck, &dURI, &dKey, &dCheck,
		&v.SizeInBytes, &v.PatchStrategy, &v.PatchMetadata,
		&v.ValidationState, &commitState,
		&v.ProjectSnapshotID, &v.PolicySnapshotID, &sourceSnaps, &inputSnaps,
		&v.HistoryCommandID, &v.HistoryAttemptID, &attrDelta, &createdStr)

	if err == sql.ErrNoRows {
		return v, fmt.Errorf("store: version not found: %s", versionID)
	}
	if err != nil {
		return v, err
	}

	v.AuthorActorType = schema.ActorType(actorType)
	v.CommitState = schema.VersionLifecycleState(commitState)
	v.CreatedAt, _ = parseTime(createdStr)

	if rURI != nil {
		v.RenderedRef = &schema.ContentRef{URI: *rURI, StorageKey: *rKey, ChecksumSHA256: *rCheck}
	}
	if dURI != nil {
		v.DiffRef = &schema.ContentRef{URI: *dURI, StorageKey: *dKey, ChecksumSHA256: *dCheck}
	}

	_ = json.Unmarshal([]byte(sourceSnaps), &v.SourceSnapshotIDs)
	_ = json.Unmarshal([]byte(inputSnaps), &v.ArtifactInputSnapshotIDs)
	_ = json.Unmarshal([]byte(attrDelta), &v.AttributeDelta)

	return hydrateArtifactVersionCompat(v), nil
}

// ListArtifactVersions returns versions for an artifact, ordered by number DESC.
func ListArtifactVersions(ctx context.Context, db *sql.DB, artifactID string, limit int) ([]schema.ArtifactVersion, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
		SELECT 
			id, artifact_id, branch_id, version_number, parent_version_id, base_version_id,
			author_actor_type, author_actor_id, source_message_id, source_conversation_id, source_operation_id,
			change_type, change_mode, change_summary, 
			content_uri, storage_key, checksum_sha256, 
			rendered_uri, rendered_storage_key, rendered_checksum,
			diff_uri, diff_storage_key, diff_checksum,
			size_in_bytes, patch_strategy, patch_metadata,
			validation_state, commit_state,
			project_snapshot_id, policy_snapshot_id, source_snapshot_ids, artifact_input_snapshot_ids,
			history_command_id, history_attempt_id, attribute_delta, created_at
		FROM artifact_versions 
		WHERE artifact_id = ? 
		ORDER BY version_number DESC 
		LIMIT ?
	`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list artifact versions: %w", err)
	}
	defer rows.Close()

	var out []schema.ArtifactVersion
	for rows.Next() {
		var v schema.ArtifactVersion
		var actorType, commitState, createdStr string
		var sourceSnaps, inputSnaps, attrDelta string
		var rURI, rKey, rCheck, dURI, dKey, dCheck *string

		err := rows.Scan(
			&v.ID, &v.ArtifactID, &v.BranchID, &v.VersionNumber, &v.ParentVersionID, &v.BaseVersionID,
			&actorType, &v.AuthorActorID, &v.SourceMessageID, &v.SourceConversationID, &v.SourceOperationID,
			&v.ChangeType, &v.ChangeMode, &v.ChangeSummary,
			&v.ContentRef.URI, &v.ContentRef.StorageKey, &v.ContentRef.ChecksumSHA256,
			&rURI, &rKey, &rCheck, &dURI, &dKey, &dCheck,
			&v.SizeInBytes, &v.PatchStrategy, &v.PatchMetadata,
			&v.ValidationState, &commitState,
			&v.ProjectSnapshotID, &v.PolicySnapshotID, &sourceSnaps, &inputSnaps,
			&v.HistoryCommandID, &v.HistoryAttemptID, &attrDelta, &createdStr)

		if err != nil {
			return nil, err
		}

		v.AuthorActorType = schema.ActorType(actorType)
		v.CommitState = schema.VersionLifecycleState(commitState)
		v.CreatedAt, _ = parseTime(createdStr)

		if rURI != nil {
			v.RenderedRef = &schema.ContentRef{URI: *rURI, StorageKey: *rKey, ChecksumSHA256: *rCheck}
		}
		if dURI != nil {
			v.DiffRef = &schema.ContentRef{URI: *dURI, StorageKey: *dKey, ChecksumSHA256: *dCheck}
		}

		_ = json.Unmarshal([]byte(sourceSnaps), &v.SourceSnapshotIDs)
		_ = json.Unmarshal([]byte(inputSnaps), &v.ArtifactInputSnapshotIDs)
		_ = json.Unmarshal([]byte(attrDelta), &v.AttributeDelta)

		out = append(out, hydrateArtifactVersionCompat(v))
	}
	return out, rows.Err()
}

// Helpers for optional refs
func optionalURI(r *schema.ContentRef) *string {
	if r == nil {
		return nil
	}
	return &r.URI
}
func optionalKey(r *schema.ContentRef) *string {
	if r == nil {
		return nil
	}
	return &r.StorageKey
}
func optionalChecksum(r *schema.ContentRef) *string {
	if r == nil {
		return nil
	}
	return &r.ChecksumSHA256
}

// SaveArtifactBranch persists a branch.
func SaveArtifactBranch(ctx context.Context, db *sql.DB, b schema.ArtifactBranch) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_branches (
			id, artifact_id, name, base_version_id, head_version_id, status,
			created_by_actor_type, created_by_actor_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			head_version_id=excluded.head_version_id,
			status=excluded.status,
			updated_at=excluded.updated_at
	`, b.ID, b.ArtifactID, b.Name, b.BaseVersionID, b.HeadVersionID, b.Status,
		string(b.CreatedByActorType), b.CreatedByActorID, b.CreatedAt.Format(timeFormat), b.UpdatedAt.Format(timeFormat))

	return err
}

// ListArtifactBranches returns branches for an artifact ordered by update time.
func ListArtifactBranches(ctx context.Context, db *sql.DB, artifactID string, limit int) ([]schema.ArtifactBranch, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, artifact_id, name, base_version_id, head_version_id, status,
		       created_by_actor_type, created_by_actor_id, created_at, updated_at
		FROM artifact_branches
		WHERE artifact_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list artifact branches: %w", err)
	}
	defer rows.Close()

	var out []schema.ArtifactBranch
	for rows.Next() {
		var item schema.ArtifactBranch
		var actorType, createdAt, updatedAt string
		if err := rows.Scan(
			&item.ID, &item.ArtifactID, &item.Name, &item.BaseVersionID, &item.HeadVersionID, &item.Status,
			&actorType, &item.CreatedByActorID, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan artifact branch: %w", err)
		}
		item.CreatedByActorType = schema.ActorType(actorType)
		item.CreatedAt, _ = parseTime(createdAt)
		item.UpdatedAt, _ = parseTime(updatedAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

// SaveArtifactReference persists a reference.
func SaveArtifactReference(ctx context.Context, db *sql.DB, r schema.ArtifactReference) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}

	meta, _ := json.Marshal(r.Metadata)

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_references (
			id, artifact_id, source_kind, source_id, relationship_type, version_id, branch_id, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.ArtifactID, r.SourceKind, r.SourceID, r.Relationship, r.VersionID, r.BranchID, string(meta), r.CreatedAt.Format(timeFormat))

	return err
}

// ListArtifactIDsBySource returns distinct artifact ids linked to a source entity.
func ListArtifactIDsBySource(ctx context.Context, db *sql.DB, sourceKind, sourceID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT artifact_id
		FROM artifact_references
		WHERE source_kind = ? AND source_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sourceKind, sourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list artifact ids by source: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var artifactID string
		if err := rows.Scan(&artifactID); err != nil {
			return nil, fmt.Errorf("store: scan artifact reference id: %w", err)
		}
		out = append(out, artifactID)
	}
	return out, rows.Err()
}

// SaveSnapshot persists a context snapshot.
func SaveSnapshot(ctx context.Context, db *sql.DB, s schema.Snapshot) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_snapshots (
			id, snapshot_type, captured_entity_type, captured_entity_id, captured_version,
			content_uri, storage_key, checksum_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.SnapshotType, s.CapturedEntityType, s.CapturedEntityID, s.CapturedVersion,
		s.ContentRef.URI, s.ContentRef.StorageKey, s.ContentRef.ChecksumSHA256, s.CreatedAt.Format(timeFormat))

	return err
}

// SaveArtifactExport inserts or updates an export record.
func SaveArtifactExport(ctx context.Context, db *sql.DB, e schema.ArtifactExport) error {
	if e.ExportID == "" {
		e.ExportID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	var (
		contentURI     any
		storageKey     any
		checksumSHA256 any
		completedAt    any
	)
	if e.ContentRef != nil {
		contentURI = e.ContentRef.URI
		storageKey = e.ContentRef.StorageKey
		checksumSHA256 = e.ContentRef.ChecksumSHA256
	}
	if e.CompletedAt != nil && !e.CompletedAt.IsZero() {
		completedAt = e.CompletedAt.UTC().Format(timeFormat)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_exports (
			export_id, artifact_id, version_id, format, target_kind, target_uri,
			content_uri, storage_key, checksum_sha256, content_type, status,
			failure_class, failure_reason, created_by_actor_type, created_by_actor_id,
			created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(export_id) DO UPDATE SET
			version_id=excluded.version_id,
			format=excluded.format,
			target_kind=excluded.target_kind,
			target_uri=excluded.target_uri,
			content_uri=excluded.content_uri,
			storage_key=excluded.storage_key,
			checksum_sha256=excluded.checksum_sha256,
			content_type=excluded.content_type,
			status=excluded.status,
			failure_class=excluded.failure_class,
			failure_reason=excluded.failure_reason,
			created_by_actor_type=excluded.created_by_actor_type,
			created_by_actor_id=excluded.created_by_actor_id,
			completed_at=excluded.completed_at
	`, e.ExportID, e.ArtifactID, e.VersionID, string(e.Format), string(e.TargetKind), e.TargetURI,
		contentURI, storageKey, checksumSHA256, e.ContentType, string(e.Status),
		string(e.FailureClass), e.FailureReason, string(e.CreatedByActorType), e.CreatedByActorID,
		e.CreatedAt.UTC().Format(timeFormat), completedAt)
	if err != nil {
		return fmt.Errorf("store: save artifact export: %w", err)
	}
	return nil
}

// GetArtifactExport loads an export by id.
func GetArtifactExport(ctx context.Context, db *sql.DB, exportID string) (*schema.ArtifactExport, error) {
	row := db.QueryRowContext(ctx, `
		SELECT export_id, artifact_id, version_id, format, target_kind, target_uri,
		       content_uri, storage_key, checksum_sha256, content_type, status,
		       failure_class, failure_reason, created_by_actor_type, created_by_actor_id,
		       created_at, completed_at
		FROM artifact_exports
		WHERE export_id = ?
	`, exportID)
	return scanArtifactExportScanner(row.Scan)
}

// ListArtifactExports returns recent exports for an artifact.
func ListArtifactExports(ctx context.Context, db *sql.DB, artifactID string, limit int) ([]schema.ArtifactExport, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
		SELECT export_id, artifact_id, version_id, format, target_kind, target_uri,
		       content_uri, storage_key, checksum_sha256, content_type, status,
		       failure_class, failure_reason, created_by_actor_type, created_by_actor_id,
		       created_at, completed_at
		FROM artifact_exports
		WHERE artifact_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list artifact exports: %w", err)
	}
	defer rows.Close()

	var out []schema.ArtifactExport
	for rows.Next() {
		item, err := scanArtifactExportScanner(rows.Scan)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, rows.Err()
}

// SaveArtifactShare inserts or updates a share record.
func SaveArtifactShare(ctx context.Context, db *sql.DB, s schema.ArtifactShare) error {
	if s.ShareID == "" {
		s.ShareID = uuid.New().String()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	var expiresAt any
	if s.ExpiresAt != nil && !s.ExpiresAt.IsZero() {
		expiresAt = s.ExpiresAt.UTC().Format(timeFormat)
	}
	var revokedAt any
	if s.RevokedAt != nil && !s.RevokedAt.IsZero() {
		revokedAt = s.RevokedAt.UTC().Format(timeFormat)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO artifact_shares (
			share_id, artifact_id, version_id, scope, access_level, status,
			share_url, created_by_actor_type, created_by_actor_id, created_at,
			expires_at, revoked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(share_id) DO UPDATE SET
			version_id=excluded.version_id,
			scope=excluded.scope,
			access_level=excluded.access_level,
			status=excluded.status,
			share_url=excluded.share_url,
			created_by_actor_type=excluded.created_by_actor_type,
			created_by_actor_id=excluded.created_by_actor_id,
			expires_at=excluded.expires_at,
			revoked_at=excluded.revoked_at
	`, s.ShareID, s.ArtifactID, optionalTextValue(s.VersionID), string(s.Scope), string(s.AccessLevel), string(s.Status),
		optionalTextValue(s.ShareURL), string(s.CreatedByActorType), s.CreatedByActorID, s.CreatedAt.UTC().Format(timeFormat),
		expiresAt, revokedAt)
	if err != nil {
		return fmt.Errorf("store: save artifact share: %w", err)
	}
	return nil
}

// ListArtifactShares returns recent shares for an artifact.
func ListArtifactShares(ctx context.Context, db *sql.DB, artifactID string, limit int) ([]schema.ArtifactShare, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
		SELECT share_id, artifact_id, version_id, scope, access_level, status,
		       share_url, created_by_actor_type, created_by_actor_id, created_at,
		       expires_at, revoked_at
		FROM artifact_shares
		WHERE artifact_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list artifact shares: %w", err)
	}
	defer rows.Close()

	var out []schema.ArtifactShare
	for rows.Next() {
		item, err := scanArtifactShareScanner(rows.Scan)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, rows.Err()
}

// ListArtifacts returns a list of artifacts for a workspace.
func ListArtifacts(ctx context.Context, db *sql.DB, workspaceID string, limit int) ([]schema.Artifact, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, workspace_id, project_id, owner_id, canonical_title, display_title, type, subtype,
		       schema_version, content_format, lifecycle_state, current_branch_id, current_version_id, 
		       head_version_number, created_by_actor_type, created_by_actor_id, provenance_root_id,
		       attributes, created_at, updated_at, archived_at
		FROM artifacts
		WHERE workspace_id = ?
		ORDER BY updated_at DESC
		LIMIT ?`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.Artifact
	for rows.Next() {
		a, err := scanArtifactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ListArtifactsForOwner returns artifacts for the given owner_id.
func ListArtifactsForOwner(ctx context.Context, db *sql.DB, ownerID string, limit int) ([]schema.Artifact, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, workspace_id, project_id, owner_id, canonical_title, display_title, type, subtype,
		       schema_version, content_format, lifecycle_state, current_branch_id, current_version_id, 
		       head_version_number, created_by_actor_type, created_by_actor_id, provenance_root_id,
		       attributes, created_at, updated_at, archived_at
		FROM artifacts
		WHERE owner_id = ?
		ORDER BY updated_at DESC
		LIMIT ?`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.Artifact
	for rows.Next() {
		a, err := scanArtifactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// SearchArtifacts searches for artifacts by title or display title.
func SearchArtifacts(ctx context.Context, db *sql.DB, workspaceID, query string, limit int) ([]schema.Artifact, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, workspace_id, project_id, owner_id, canonical_title, display_title, type, subtype,
		       schema_version, content_format, lifecycle_state, current_branch_id, current_version_id, 
		       head_version_number, created_by_actor_type, created_by_actor_id, provenance_root_id,
		       attributes, created_at, updated_at, archived_at
		FROM artifacts
		WHERE workspace_id = ? AND (canonical_title LIKE ? OR display_title LIKE ?)
		ORDER BY updated_at DESC
		LIMIT ?`, workspaceID, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.Artifact
	for rows.Next() {
		a, err := scanArtifactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanArtifactRow(rows *sql.Rows) (*schema.Artifact, error) {
	return scanArtifactScanner(rows.Scan)
}

func scanArtifactScanner(scan func(dest ...any) error) (*schema.Artifact, error) {
	var a schema.Artifact
	var createdStr, updatedStr string
	var archivedStr *string
	var attrJSON string
	var tp, state, actorType string

	if err := scan(
		&a.ID, &a.WorkspaceID, &a.ProjectID, &a.OwnerID,
		&a.CanonicalTitle, &a.DisplayTitle, &tp, &a.Subtype,
		&a.SchemaVersion, &a.ContentFormat, &state,
		&a.CurrentBranchID, &a.CurrentVersionID, &a.HeadVersionNumber,
		&actorType, &a.CreatedByActorID, &a.ProvenanceRootID,
		&attrJSON, &createdStr, &updatedStr, &archivedStr,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan artifact: %w", err)
	}

	a.Type = schema.ArtifactType(tp)
	a.LifecycleState = schema.ArtifactLifecycleState(state)
	a.CreatedByActorType = schema.ActorType(actorType)
	a.CreatedAt, _ = parseTime(createdStr)
	a.UpdatedAt, _ = parseTime(updatedStr)
	if archivedStr != nil {
		t, _ := parseTime(*archivedStr)
		a.ArchivedAt = &t
	}
	_ = json.Unmarshal([]byte(attrJSON), &a.Attributes)
	applyArtifactCompatFromAttributes(&a)
	return &a, nil
}

func hydrateArtifactVersionCompat(v schema.ArtifactVersion) schema.ArtifactVersion {
	v.VersionID = v.ID
	v.Version = v.VersionNumber
	if raw, ok := attrString(v.AttributeDelta, artifactVersionAttrStatus); ok {
		v.Status = raw
	}
	if raw, ok := attrString(v.AttributeDelta, artifactVersionAttrLocation); ok {
		v.Location = raw
	}
	if raw, ok := attrString(v.AttributeDelta, artifactVersionAttrDescription); ok {
		v.Description = raw
	}
	if raw, ok := attrString(v.AttributeDelta, artifactVersionAttrSnapshot); ok {
		v.Snapshot = raw
	}
	if raw, ok := attrString(v.AttributeDelta, artifactVersionAttrExecutionAttemptID); ok {
		v.ExecutionAttemptID = raw
	}
	return v
}

func scanArtifactExportScanner(scan func(dest ...any) error) (*schema.ArtifactExport, error) {
	var item schema.ArtifactExport
	var (
		format, targetKind, status, failureClass, actorType                              string
		targetURI, contentURI, storageKey, checksum, contentType, failureReason, actorID sql.NullString
		createdAt, completedAt                                                           sql.NullString
	)
	if err := scan(
		&item.ExportID, &item.ArtifactID, &item.VersionID, &format, &targetKind, &targetURI,
		&contentURI, &storageKey, &checksum, &contentType, &status,
		&failureClass, &failureReason, &actorType, &actorID,
		&createdAt, &completedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan artifact export: %w", err)
	}
	item.Format = schema.ExportFormat(format)
	item.TargetKind = schema.ArtifactExportTargetKind(targetKind)
	item.Status = schema.ArtifactExportStatus(status)
	item.FailureClass = schema.FailureClass(failureClass)
	item.CreatedByActorType = schema.ActorType(actorType)
	if targetURI.Valid {
		item.TargetURI = targetURI.String
	}
	if contentURI.Valid || storageKey.Valid || checksum.Valid {
		item.ContentRef = &schema.ContentRef{
			URI:            targetOrEmpty(contentURI),
			StorageKey:     targetOrEmpty(storageKey),
			ChecksumSHA256: targetOrEmpty(checksum),
		}
	}
	if contentType.Valid {
		item.ContentType = contentType.String
	}
	if failureReason.Valid {
		item.FailureReason = failureReason.String
	}
	if actorID.Valid {
		item.CreatedByActorID = actorID.String
	}
	if createdAt.Valid {
		item.CreatedAt, _ = parseTime(createdAt.String)
	}
	if completedAt.Valid && completedAt.String != "" {
		t, _ := parseTime(completedAt.String)
		item.CompletedAt = &t
	}
	return &item, nil
}

func scanArtifactShareScanner(scan func(dest ...any) error) (*schema.ArtifactShare, error) {
	var item schema.ArtifactShare
	var (
		scope, accessLevel, status, actorType                         string
		versionID, shareURL, actorID, createdAt, expiresAt, revokedAt sql.NullString
	)
	if err := scan(
		&item.ShareID, &item.ArtifactID, &versionID, &scope, &accessLevel, &status,
		&shareURL, &actorType, &actorID, &createdAt, &expiresAt, &revokedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan artifact share: %w", err)
	}
	if versionID.Valid {
		item.VersionID = versionID.String
	}
	item.Scope = schema.ArtifactShareScope(scope)
	item.AccessLevel = schema.ArtifactShareAccessLevel(accessLevel)
	item.Status = schema.ArtifactShareStatus(status)
	item.CreatedByActorType = schema.ActorType(actorType)
	if shareURL.Valid {
		item.ShareURL = shareURL.String
	}
	if actorID.Valid {
		item.CreatedByActorID = actorID.String
	}
	if createdAt.Valid {
		item.CreatedAt, _ = parseTime(createdAt.String)
	}
	if expiresAt.Valid && expiresAt.String != "" {
		t, _ := parseTime(expiresAt.String)
		item.ExpiresAt = &t
	}
	if revokedAt.Valid && revokedAt.String != "" {
		t, _ := parseTime(revokedAt.String)
		item.RevokedAt = &t
	}
	return &item, nil
}

func optionalTextValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func targetOrEmpty(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func applyArtifactCompatToAttributes(a *schema.Artifact) {
	if a.Attributes == nil {
		a.Attributes = map[string]any{}
	}
	setAttrString(a.Attributes, artifactAttrKind, a.Kind)
	setAttrString(a.Attributes, artifactAttrLocation, a.Location)
	setAttrString(a.Attributes, artifactAttrDescription, a.Description)
	setAttrString(a.Attributes, artifactAttrStatus, a.Status)
	setAttrString(a.Attributes, artifactAttrRenderStatus, a.RenderStatus)
	if a.Version > 0 {
		a.Attributes[artifactAttrVersion] = a.Version
	}
	setAttrString(a.Attributes, artifactAttrSourceTool, a.SourceTool)
	setAttrString(a.Attributes, artifactAttrSourceExecutionID, a.SourceExecutionID)
	setAttrString(a.Attributes, artifactAttrSourceRunID, a.SourceRunID)
	setAttrString(a.Attributes, artifactAttrLastError, a.LastError)
	setAttrString(a.Attributes, artifactAttrRecoverableOutput, a.RecoverableOutput)
	setAttrString(a.Attributes, artifactAttrMetadata, a.Metadata)
	if a.Version <= 0 {
		a.Version = a.HeadVersionNumber
	}
}

func applyArtifactCompatFromAttributes(a *schema.Artifact) {
	if a == nil {
		return
	}
	if a.Kind == "" {
		if raw, ok := attrString(a.Attributes, artifactAttrKind); ok {
			a.Kind = raw
		} else {
			a.Kind = string(a.Type)
		}
	}
	if raw, ok := attrString(a.Attributes, artifactAttrLocation); ok {
		a.Location = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrDescription); ok {
		a.Description = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrStatus); ok {
		a.Status = raw
	} else {
		a.Status = string(a.LifecycleState)
	}
	if raw, ok := attrString(a.Attributes, artifactAttrRenderStatus); ok {
		a.RenderStatus = raw
	}
	if raw, ok := attrInt(a.Attributes, artifactAttrVersion); ok {
		a.Version = raw
	} else {
		a.Version = a.HeadVersionNumber
	}
	if raw, ok := attrString(a.Attributes, artifactAttrSourceTool); ok {
		a.SourceTool = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrSourceExecutionID); ok {
		a.SourceExecutionID = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrSourceRunID); ok {
		a.SourceRunID = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrLastError); ok {
		a.LastError = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrRecoverableOutput); ok {
		a.RecoverableOutput = raw
	}
	if raw, ok := attrString(a.Attributes, artifactAttrMetadata); ok {
		a.Metadata = raw
	}
}

func setAttrString(attrs map[string]any, key, value string) {
	if value == "" {
		return
	}
	attrs[key] = value
}

func attrString(attrs map[string]any, key string) (string, bool) {
	if attrs == nil {
		return "", false
	}
	raw, ok := attrs[key]
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, true
	default:
		return fmt.Sprint(v), true
	}
}

func attrInt(attrs map[string]any, key string) (int, bool) {
	if attrs == nil {
		return 0, false
	}
	raw, ok := attrs[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	default:
		return 0, false
	}
}

func firstCompatValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
