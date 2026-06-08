package schema

import "time"

// ArtifactType classifies the high-level family of an artifact.
type ArtifactType string

const (
	ArtifactTypeDocument     ArtifactType = "document"
	ArtifactTypeCode         ArtifactType = "code"
	ArtifactTypeData         ArtifactType = "data"
	ArtifactTypePresentation ArtifactType = "presentation"
)

// ArtifactLifecycleState tracks the workflow state of an artifact.
type ArtifactLifecycleState string

const (
	ArtifactLifecycleDraft       ArtifactLifecycleState = "draft"
	ArtifactLifecycleInProgress  ArtifactLifecycleState = "in_progress"
	ArtifactLifecycleReviewReady ArtifactLifecycleState = "review_ready"
	ArtifactLifecycleApproved    ArtifactLifecycleState = "approved"
	ArtifactLifecyclePublished   ArtifactLifecycleState = "published"
	ArtifactLifecycleArchived    ArtifactLifecycleState = "archived"
	ArtifactLifecycleErrored     ArtifactLifecycleState = "errored"
)

// VersionLifecycleState tracks the commitment state of a version.
type VersionLifecycleState string

const (
	VersionStatePending    VersionLifecycleState = "pending"
	VersionStateCommitted  VersionLifecycleState = "committed"
	VersionStateSuperseded VersionLifecycleState = "superseded"
	VersionStateRestored   VersionLifecycleState = "restored"
	VersionStateFailed     VersionLifecycleState = "failed"
)

// ActorType identifies the category of agent or user who performed an action.
type ActorType string

const (
	ActorUser      ActorType = "user"
	ActorAssistant ActorType = "assistant"
	ActorTool      ActorType = "tool"
	ActorSystem    ActorType = "system"
	ActorMigration ActorType = "migration"
	ActorAgent     ActorType = "agent"
)

// ContentRef is a stable link to blob content in object storage.
type ContentRef struct {
	URI            string `json:"uri"`         // e.g. artifact://artifacts/{aid}/versions/{vid}/content
	StorageKey     string `json:"storage_key"` // object store key
	ChecksumSHA256 string `json:"checksum_sha256"`
}

// Artifact is a durable, user-meaningful work product.
// It is an Attribute-composed World Model entity.
type Artifact struct {
	ID          string `json:"artifact_id"`
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id,omitempty"`
	OwnerID     string `json:"owner_id"`

	CanonicalTitle string       `json:"canonical_title"` // computer-readable name
	DisplayTitle   string       `json:"display_title"`   // user-facing name
	Type           ArtifactType `json:"artifact_type"`
	Subtype        string       `json:"artifact_subtype"` // e.g. "markdown", "python", "csv"
	SchemaVersion  string       `json:"schema_version"`
	ContentFormat  string       `json:"content_format"` // e.g. "text/markdown"

	LifecycleState ArtifactLifecycleState `json:"lifecycle_state"`

	CurrentBranchID   string `json:"current_branch_id"`
	CurrentVersionID  string `json:"current_version_id"`
	HeadVersionNumber int    `json:"head_version_number"`

	CreatedByActorType ActorType `json:"created_by_actor_type"`
	CreatedByActorID   string    `json:"created_by_actor_id,omitempty"`

	ProvenanceRootID string `json:"provenance_root_id"` // initial version or creation event

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`

	// Attributes (JSON-encoded in SQLite, map in Go)
	Attributes map[string]any `json:"attributes,omitempty"`

	// Compatibility fields for the world-model artifact inspection surface.
	Kind              string `json:"kind,omitempty"`
	Location          string `json:"location,omitempty"`
	Description       string `json:"description,omitempty"`
	Status            string `json:"status,omitempty"`
	RenderStatus      string `json:"render_status,omitempty"`
	Version           int    `json:"version,omitempty"`
	SourceTool        string `json:"source_tool,omitempty"`
	SourceExecutionID string `json:"source_execution_id,omitempty"`
	SourceRunID       string `json:"source_run_id,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	RecoverableOutput string `json:"recoverable_output,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
}

// ArtifactVersion is an immutable saved state of an artifact.
type ArtifactVersion struct {
	ID              string `json:"version_id"`
	ArtifactID      string `json:"artifact_id"`
	BranchID        string `json:"branch_id"`
	VersionNumber   int    `json:"version_number"`
	ParentVersionID string `json:"parent_version_id,omitempty"`
	BaseVersionID   string `json:"base_version_id,omitempty"` // for diffs/patches

	AuthorActorType ActorType `json:"author_actor_type"`
	AuthorActorID   string    `json:"author_actor_id,omitempty"`

	SourceMessageID      string `json:"source_message_id,omitempty"`
	SourceConversationID string `json:"source_conversation_id,omitempty"`
	SourceOperationID    string `json:"source_operation_id"`

	ChangeType    string `json:"change_type"` // e.g. "replace", "patch", "append"
	ChangeMode    string `json:"change_mode"` // "explicit", "auto", "revert"
	ChangeSummary string `json:"change_summary"`

	ContentRef     ContentRef  `json:"content_ref"`
	RenderedRef    *ContentRef `json:"rendered_ref,omitempty"`
	DiffRef        *ContentRef `json:"diff_ref,omitempty"`
	ChecksumSHA256 string      `json:"checksum_sha256"`
	SizeInBytes    int64       `json:"size_in_bytes"`

	PatchStrategy string `json:"patch_strategy,omitempty"`
	PatchMetadata string `json:"patch_metadata,omitempty"` // JSON-encoded patch details

	ValidationState string                `json:"validation_state"` // e.g. "passed", "failed", "pending"
	CommitState     VersionLifecycleState `json:"commit_state"`

	// Snapshot references for provenance
	ProjectSnapshotID        string   `json:"project_snapshot_id,omitempty"`
	PolicySnapshotID         string   `json:"policy_snapshot_id"`
	SourceSnapshotIDs        []string `json:"source_snapshot_ids,omitempty"`
	ArtifactInputSnapshotIDs []string `json:"artifact_input_snapshot_ids,omitempty"`

	// History linkage
	HistoryCommandID string `json:"history_command_id,omitempty"`
	HistoryAttemptID string `json:"history_attempt_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	// Attributes delta for this version
	AttributeDelta map[string]any `json:"attribute_delta,omitempty"`

	// Compatibility fields for the world-model artifact inspection surface.
	VersionID          string `json:"compat_version_id,omitempty"`
	Version            int    `json:"compat_version,omitempty"`
	Status             string `json:"status,omitempty"`
	Location           string `json:"location,omitempty"`
	Description        string `json:"description,omitempty"`
	Snapshot           string `json:"snapshot,omitempty"`
	ExecutionAttemptID string `json:"execution_attempt_id,omitempty"`
}

// ArtifactBranch is a named line of evolution for an artifact.
type ArtifactBranch struct {
	ID            string `json:"branch_id"`
	ArtifactID    string `json:"artifact_id"`
	Name          string `json:"name"`
	BaseVersionID string `json:"base_version_id"`
	HeadVersionID string `json:"head_version_id"`
	Status        string `json:"status"` // "active" | "merged" | "abandoned"

	CreatedByActorType ActorType `json:"created_by_actor_type"`
	CreatedByActorID   string    `json:"created_by_actor_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ArtifactReference is a stable link between artifact and other entities.
type ArtifactReference struct {
	ID           string `json:"reference_id"`
	ArtifactID   string `json:"artifact_id"`
	SourceKind   string `json:"source_kind"` // "message", "conversation", "project", etc.
	SourceID     string `json:"source_id"`
	Relationship string `json:"relationship_type"`

	VersionID string `json:"version_id,omitempty"`
	BranchID  string `json:"branch_id,omitempty"`

	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Snapshot is a frozen context record attached to a version.
type Snapshot struct {
	ID           string `json:"snapshot_id"`
	SnapshotType string `json:"snapshot_type"` // e.g. "project_context_snapshot"

	CapturedEntityType string `json:"captured_entity_type"`
	CapturedEntityID   string `json:"captured_entity_id"`
	CapturedVersion    string `json:"captured_version,omitempty"`

	ContentRef     ContentRef `json:"content_ref"`
	ChecksumSHA256 string     `json:"checksum_sha256"`

	CreatedAt time.Time `json:"created_at"`
}

// ExportFormat is the serialized output format for an artifact export.
type ExportFormat string

const (
	ExportFormatMarkdown  ExportFormat = "md"
	ExportFormatPDF       ExportFormat = "pdf"
	ExportFormatJSON      ExportFormat = "json"
	ExportFormatCSV       ExportFormat = "csv"
	ExportFormatText      ExportFormat = "txt"
	ExportFormatFormatted ExportFormat = "formatted"
)

// ArtifactExportTargetKind describes where an export is headed.
type ArtifactExportTargetKind string

const (
	ArtifactExportTargetDownload           ArtifactExportTargetKind = "download"
	ArtifactExportTargetExternalSystem     ArtifactExportTargetKind = "external_system"
	ArtifactExportTargetInternalAttachment ArtifactExportTargetKind = "internal_attachment"
)

// ArtifactExportStatus tracks the lifecycle of an export request.
type ArtifactExportStatus string

const (
	ArtifactExportPending   ArtifactExportStatus = "pending"
	ArtifactExportCompleted ArtifactExportStatus = "completed"
	ArtifactExportFailed    ArtifactExportStatus = "failed"
)

// ArtifactShareScope defines how broadly an artifact is shared.
type ArtifactShareScope string

const (
	ArtifactShareScopeWorkspace ArtifactShareScope = "workspace"
	ArtifactShareScopeProject   ArtifactShareScope = "project"
	ArtifactShareScopeOrg       ArtifactShareScope = "org"
	ArtifactShareScopeLink      ArtifactShareScope = "link"
)

// ArtifactShareAccessLevel defines permissions granted by a share record.
type ArtifactShareAccessLevel string

const (
	ArtifactShareAccessRead    ArtifactShareAccessLevel = "read"
	ArtifactShareAccessComment ArtifactShareAccessLevel = "comment"
	ArtifactShareAccessEdit    ArtifactShareAccessLevel = "edit"
	ArtifactShareAccessCopy    ArtifactShareAccessLevel = "copy"
)

// ArtifactShareStatus tracks the lifecycle of a share record.
type ArtifactShareStatus string

const (
	ArtifactShareStatusActive  ArtifactShareStatus = "active"
	ArtifactShareStatusRevoked ArtifactShareStatus = "revoked"
	ArtifactShareStatusExpired ArtifactShareStatus = "expired"
)

// ArtifactExport records a generated export object and its delivery status.
type ArtifactExport struct {
	ExportID           string                   `json:"export_id"`
	ArtifactID         string                   `json:"artifact_id"`
	VersionID          string                   `json:"version_id"`
	Format             ExportFormat             `json:"format"`
	TargetKind         ArtifactExportTargetKind `json:"target_kind"`
	TargetURI          string                   `json:"target_uri,omitempty"`
	ContentRef         *ContentRef              `json:"content_ref,omitempty"`
	ContentType        string                   `json:"content_type,omitempty"`
	Status             ArtifactExportStatus     `json:"status"`
	FailureClass       FailureClass             `json:"failure_class,omitempty"`
	FailureReason      string                   `json:"failure_reason,omitempty"`
	CreatedByActorType ActorType                `json:"created_by_actor_type"`
	CreatedByActorID   string                   `json:"created_by_actor_id,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	CompletedAt        *time.Time               `json:"completed_at,omitempty"`
}

// ArtifactShare records a durable sharing grant for an artifact.
type ArtifactShare struct {
	ShareID            string                   `json:"share_id"`
	ArtifactID         string                   `json:"artifact_id"`
	VersionID          string                   `json:"version_id,omitempty"`
	Scope              ArtifactShareScope       `json:"scope"`
	AccessLevel        ArtifactShareAccessLevel `json:"access_level"`
	Status             ArtifactShareStatus      `json:"status"`
	ShareURL           string                   `json:"share_url,omitempty"`
	CreatedByActorType ActorType                `json:"created_by_actor_type"`
	CreatedByActorID   string                   `json:"created_by_actor_id,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	ExpiresAt          *time.Time               `json:"expires_at,omitempty"`
	RevokedAt          *time.Time               `json:"revoked_at,omitempty"`
}

// ArtifactOperation qualifies the type of mutation being requested.
type ArtifactOperation string

const (
	ArtifactOpCreate              ArtifactOperation = "create_artifact"
	ArtifactOpReplace             ArtifactOperation = "replace_content"
	ArtifactOpPatch               ArtifactOperation = "patch_content"
	ArtifactOpAppend              ArtifactOperation = "append_content"
	ArtifactOpRename              ArtifactOperation = "rename_artifact"
	ArtifactOpUpdateMetadata      ArtifactOperation = "update_metadata"
	ArtifactOpArchive             ArtifactOperation = "archive_artifact"
	ArtifactOpRestore             ArtifactOperation = "restore_artifact"
	ArtifactOpBranch              ArtifactOperation = "branch_artifact"
	ArtifactOpRestoreVersion      ArtifactOperation = "restore_version"
	ArtifactOpExport              ArtifactOperation = "export_artifact"
	ArtifactOpShare               ArtifactOperation = "share_artifact"
	ArtifactOpSync                ArtifactOperation = "sync_artifact"
	ArtifactOpTransitionLifecycle ArtifactOperation = "transition_lifecycle"
)

// ArtifactOperationEnvelope carries the intent and payload for an artifact operation.
type ArtifactOperationEnvelope struct {
	OperationID string            `json:"operation_id"`
	Operation   ArtifactOperation `json:"operation"`

	TargetArtifactID      string `json:"target_artifact_id,omitempty"`
	TargetBranchID        string `json:"target_branch_id,omitempty"`
	TargetVersionID       string `json:"target_version_id,omitempty"`
	ExpectedBaseVersionID string `json:"expected_base_version_id,omitempty"`

	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id,omitempty"`
	OwnerID     string `json:"owner_id"`

	ArtifactType    ArtifactType `json:"artifact_type,omitempty"`
	ArtifactSubtype string       `json:"artifact_subtype,omitempty"`
	ContentFormat   string       `json:"content_format,omitempty"`
	Title           string       `json:"title,omitempty"`

	Payload       any            `json:"payload,omitempty"`
	MetadataPatch map[string]any `json:"metadata_patch,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`

	Reason string `json:"reason,omitempty"`

	ActorType ActorType `json:"actor_type"`
	ActorID   string    `json:"actor_id,omitempty"`

	SourceMessageID      string `json:"source_message_id,omitempty"`
	SourceConversationID string `json:"source_conversation_id,omitempty"`

	PolicySnapshotID string `json:"policy_snapshot_id,omitempty"`

	ConfirmationMode string `json:"confirmation_mode,omitempty"` // "none" | "required" | "proposal"

	// ToLifecycleState is used by transition_lifecycle operations.
	ToLifecycleState ArtifactLifecycleState `json:"to_lifecycle_state,omitempty"`
}
