package compaction

import "time"

type TriggerClass string

const (
	TriggerSoft      TriggerClass = "soft"
	TriggerHard      TriggerClass = "hard"
	TriggerEmergency TriggerClass = "emergency"
	TriggerManual    TriggerClass = "manual"
)

type ChatFrame struct {
	SchemaVersion      int                    `json:"schema_version"`
	ChatID             string                 `json:"chat_id"`
	FrameVersion       int                    `json:"frame_version"`
	CurrentEpochID     string                 `json:"current_epoch_id"`
	PrimaryObjective   string                 `json:"primary_objective"`
	ActiveTopics       []string               `json:"active_topics"`
	OpenQuestions      []string               `json:"open_questions"`
	ActiveConstraints  []string               `json:"active_constraints"`
	ActiveCommitments  []string               `json:"active_commitments"`
	SocialContinuity   string                 `json:"social_continuity"`
	ActiveArtifactRefs []string               `json:"active_artifact_refs"`
	ActiveProposalRefs []string               `json:"active_proposal_refs"`
	ActiveFailureRefs  []string               `json:"active_failure_refs"`
	SourceSpanRefs     []string               `json:"source_span_refs"`
	Provenance         []SourceSpanProvenance `json:"provenance,omitempty"`
}

type TaskFrame struct {
	TaskID         string                 `json:"task_id"`
	ParentTaskID   string                 `json:"parent_task_id,omitempty"`
	RunID          string                 `json:"run_id,omitempty"`
	CheckpointID   string                 `json:"checkpoint_id,omitempty"`
	FrameVersion   int                    `json:"frame_version,omitempty"`
	Title          string                 `json:"title"`
	Objective      string                 `json:"objective"`
	Status         string                 `json:"status"`
	Phase          string                 `json:"phase"`
	Blockers       []string               `json:"blockers"`
	Dependencies   []string               `json:"dependencies"`
	CurrentPlan    []string               `json:"current_plan"`
	CompletedSteps []string               `json:"completed_steps"`
	NextStep       string                 `json:"next_step"`
	LinkedEntities []string               `json:"linked_entities"`
	ArtifactRefs   []string               `json:"artifact_refs"`
	ProposalRefs   []string               `json:"proposal_refs"`
	FailureRefs    []string               `json:"failure_refs"`
	SourceSpanRefs []string               `json:"source_span_refs"`
	IdentitySource string                 `json:"identity_source,omitempty"`
	StatusSource   string                 `json:"status_source,omitempty"`
	LastUpdatedAt  time.Time              `json:"last_updated_at,omitempty"`
	Provenance     []SourceSpanProvenance `json:"provenance,omitempty"`
}

type RetrievalSpan struct {
	SpanID           string               `json:"span_id"`
	ChatID           string               `json:"chat_id"`
	CheckpointID     string               `json:"checkpoint_id"`
	Kind             string               `json:"kind"`
	SupportClass     string               `json:"support_class,omitempty"`
	SourceMessageIDs []string             `json:"source_message_ids"`
	Excerpt          string               `json:"excerpt"`
	ArtifactRefs     []string             `json:"artifact_refs"`
	RelatedTaskIDs   []string             `json:"related_task_ids,omitempty"`
	Tags             []string             `json:"tags"`
	Priority         int                  `json:"priority,omitempty"`
	Provenance       SourceSpanProvenance `json:"provenance,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

type CompactionCheckpoint struct {
	CheckpointID            string               `json:"checkpoint_id"`
	ChatID                  string               `json:"chat_id"`
	EpochID                 string               `json:"epoch_id"`
	TriggerClass            TriggerClass         `json:"trigger_class"`
	TriggerReason           string               `json:"trigger_reason,omitempty"`
	CompactedMessageStartID string               `json:"compacted_message_start_id"`
	CompactedMessageEndID   string               `json:"compacted_message_end_id"`
	ChatFrameVersion        int                  `json:"chat_frame_version"`
	TaskFrameVersions       map[string]int       `json:"task_frame_versions"`
	SourceMessageIDs        []string             `json:"source_message_ids"`
	SelectedSpan            SourceSpanProvenance `json:"selected_span,omitempty"`
	SelectionTrace          CompactionSelection  `json:"selection_trace,omitempty"`
	TaskSurvival            []TaskSurvivalRecord `json:"task_survival,omitempty"`
	RetrievalSpanIDs        []string             `json:"retrieval_span_ids,omitempty"`
	EstimatorSnapshot       BudgetSnapshot       `json:"estimator_snapshot"`
	CreatedAt               time.Time            `json:"created_at"`
}

type ChatMemory struct {
	ChatID         string
	SchemaVersion  int
	MemoryVersion  int
	CurrentEpochID string
	ChatFrame      ChatFrame
	TaskFrames     []TaskFrame
	RetrievalSpans []RetrievalSpan
	UpdatedAt      time.Time
}

type Message struct {
	ID               string
	Role             string
	RunID            string
	Content          string
	MessageKind      string
	SourceMessageRef string
	CreatedAt        time.Time
}

type BudgetSnapshot struct {
	MaxContextTokens      int
	ReservedOutputTokens  int
	ReservedToolHeadroom  int
	UsableTokens          int
	EstimatedPromptTokens int
	SoftThreshold         int
	HardThreshold         int
	EmergencyThreshold    int
}

type SelectionResult struct {
	StartIndex int
	EndIndex   int
	Messages   []Message
	Trace      CompactionSelection
}

type RehydrationInput struct {
	PermanentInstructions []string
	ChatFrame             ChatFrame
	TaskFrames            []TaskFrame
	ProposalRefs          []string
	FailureRefs           []string
	RetrievalSpans        []RetrievalSpan
	LiveTail              []Message
	NewInput              string
	Budget                BudgetSnapshot
	ProtectedRecentWindow int
	MaxTaskFrames         int
	MaxSupportSpans       int
}

type RehydrationOutput struct {
	Sections []string
	Dropped  []string
	Metadata RehydrationMetadata
}

type SourceSpanProvenance struct {
	Ref              string   `json:"ref,omitempty"`
	StartMessageID   string   `json:"start_message_id,omitempty"`
	EndMessageID     string   `json:"end_message_id,omitempty"`
	SourceMessageIDs []string `json:"source_message_ids,omitempty"`
	RunIDs           []string `json:"run_ids,omitempty"`
	ProposalRefs     []string `json:"proposal_refs,omitempty"`
	FailureRefs      []string `json:"failure_refs,omitempty"`
	ArtifactRefs     []string `json:"artifact_refs,omitempty"`
	CheckpointRefs   []string `json:"checkpoint_refs,omitempty"`
}

type RunSnapshot struct {
	RunID                  string            `json:"run_id"`
	Status                 string            `json:"status,omitempty"`
	CurrentPhase           string            `json:"current_phase,omitempty"`
	BlockedOnProposalID    string            `json:"blocked_on_proposal_id,omitempty"`
	LatestCheckpointID     string            `json:"latest_checkpoint_id,omitempty"`
	MainArtifactID         string            `json:"main_artifact_id,omitempty"`
	ArtifactIDs            []string          `json:"artifact_ids,omitempty"`
	Scratchpad             map[string]string `json:"scratchpad,omitempty"`
	PendingProposalID      string            `json:"pending_proposal_id,omitempty"`
	PendingProposalReason  string            `json:"pending_proposal_reason,omitempty"`
	PendingToolCall        bool              `json:"pending_tool_call,omitempty"`
	CheckpointArtifactIDs  []string          `json:"checkpoint_artifact_ids,omitempty"`
	CheckpointMainArtifact string            `json:"checkpoint_main_artifact_id,omitempty"`
	CheckpointCreatedAt    time.Time         `json:"checkpoint_created_at,omitempty"`
	UpdatedAt              time.Time         `json:"updated_at,omitempty"`
}

type SelectorInput struct {
	Messages         []Message
	Trigger          TriggerClass
	RuntimeSnapshots []RunSnapshot
}

type BoundaryReason struct {
	Kind      string `json:"kind,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	RefID     string `json:"ref_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type CompactionSelection struct {
	ProtectedRecentWindow int             `json:"protected_recent_window,omitempty"`
	CandidateStartID      string          `json:"candidate_start_id,omitempty"`
	CandidateEndID        string          `json:"candidate_end_id,omitempty"`
	SelectedCount         int             `json:"selected_count,omitempty"`
	StopReason            *BoundaryReason `json:"stop_reason,omitempty"`
}

type TaskSurvivalRecord struct {
	TaskID               string   `json:"task_id"`
	RunID                string   `json:"run_id,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	SourceMessageStartID string   `json:"source_message_start_id,omitempty"`
	SourceMessageEndID   string   `json:"source_message_end_id,omitempty"`
	ProposalRefs         []string `json:"proposal_refs,omitempty"`
	FailureRefs          []string `json:"failure_refs,omitempty"`
}

type DroppedSection struct {
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type RehydrationMetadata struct {
	IncludedTaskIDs        []string         `json:"included_task_ids,omitempty"`
	IncludedSupportSpanIDs []string         `json:"included_support_span_ids,omitempty"`
	ProtectedLabels        []string         `json:"protected_labels,omitempty"`
	DroppedSections        []DroppedSection `json:"dropped_sections,omitempty"`
}
