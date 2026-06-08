package schema

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventKind distinguishes commands (intent) from facts (what happened).
// Mixing them breaks event-log replay.
type EventKind string

const (
	EventKindCommand EventKind = "command"
	EventKindFact    EventKind = "fact"
)

// EventType is the fully-qualified name of an event on the bus.
type EventType string

const (
	// --- Agent commands ---
	CmdAgentRegister EventType = "cmd.agent.register"
	CmdAgentRetire   EventType = "cmd.agent.retire"

	// --- Directive facts & commands ---
	FactDirectiveReplied EventType = "fact.directive.replied"
	CmdDirectiveMessage  EventType = "cmd.directive.message"

	// --- Task commands (IMPLEMENT mode) ---
	CmdTaskAssign EventType = "cmd.task.assign"

	// --- Cost facts ---
	FactCostRecorded EventType = "fact.cost.recorded"

	// --- Governor facts ---
	FactGovernorTripped     EventType = "fact.governor.tripped"
	FactAgentBudgetExceeded EventType = "fact.agent.budget.exceeded"

	// --- NAVI facts ---
	FactNaviReplied               EventType = "navi.fact.replied"
	FactNaviReplyChunk            EventType = "navi.fact.reply.chunk"
	FactNaviExperienceModeChanged EventType = "navi.fact.experience_mode_changed"
	FactNaviExperienceSnapshot    EventType = "navi.fact.experience_snapshot"
	FactNaviSkillLoaded           EventType = "navi.fact.skill_loaded"
	FactNaviSkillError            EventType = "navi.fact.skill_error"
	FactNaviHeartbeatDone         EventType = "navi.fact.heartbeat_done"
	FactNaviActionRequested       EventType = "navi.fact.action_requested"
	FactReflectionQueued          EventType = "navi.fact.reflection_queued"
	FactSubconsciousInterruption  EventType = "navi.fact.subconscious_interruption"

	// --- Run lifecycle facts ---
	FactRunStarted             EventType = "run.started"
	FactRunPhaseChanged        EventType = "run.phase.changed"
	FactInferenceDecisionTrace EventType = "inference.decision.trace"
	FactRunPaused              EventType = "run.paused"
	FactRunResumed             EventType = "run.resumed"
	FactRunCompleted           EventType = "run.completed"
	FactRunFailed              EventType = "run.failed"
	FactRunCancelled           EventType = "run.cancelled"
	FactInterruptRaised        EventType = "interrupt.raised"
	FactInterruptApplied       EventType = "interrupt.applied"

	// --- Tool call lifecycle facts ---
	FactToolCallStarted   EventType = "tool.call.started"
	FactToolCallProgress  EventType = "tool.call.progress"
	FactToolCallCompleted EventType = "tool.call.completed"
	FactToolCallFailed    EventType = "tool.call.failed"

	// --- Message intake facts ---
	FactMessageReceived   EventType = "message.received"
	FactMessageClassified EventType = "message.classified"
	FactMessageMerged     EventType = "message.merged"
	FactMessageDeferred   EventType = "message.deferred"
	FactMessageSuperseded EventType = "message.superseded"

	// --- Assistant streaming facts ---
	FactAssistantTokenDelta       EventType = "assistant.token.delta"
	FactAssistantMessagePartial   EventType = "assistant.message.partial"
	FactAssistantMessageCompleted EventType = "assistant.message.completed"

	// --- Proposal/runtime integration facts ---
	// FactContextRead records a governed read through the query_context surface
	// (Language-Layer Contract §4: every read is purpose-scoped and audit-attributed).
	FactContextRead EventType = "context.read"

	FactGovernanceBlocked EventType = "governance.blocked"
	FactProposalWaiting   EventType = "proposal.waiting"
	FactProposalResolved  EventType = "proposal.resolved"
	FactDegradationNoted  EventType = "degradation.noted"
	FactRecoveryRequired  EventType = "recovery.required"

	// --- Artifact lifecycle facts ---
	FactArtifactCreated                 EventType = "artifact.created"
	FactArtifactUpdated                 EventType = "artifact.updated"
	FactArtifactArchived                EventType = "artifact.archived"
	FactArtifactRestored                EventType = "artifact.restored"
	FactArtifactBranched                EventType = "artifact.branched"
	FactArtifactDeleted                 EventType = "artifact.deleted"
	FactArtifactVersionCommitted        EventType = "artifact.version.committed"
	FactArtifactVersionRestoreRequested EventType = "artifact.version.restore_requested"
	FactArtifactLifecycleChanged        EventType = "artifact.lifecycle.changed"
	FactArtifactExecutionStarted        EventType = "artifact.execution.started"
	FactArtifactExecutionCompleted      EventType = "artifact.execution.completed"
	FactArtifactExecutionFailed         EventType = "artifact.execution.failed"
	FactArtifactPatchFailed             EventType = "artifact.patch.failed"
	FactArtifactRendererResolved        EventType = "artifact.renderer.resolved"
	FactArtifactRendererMissing         EventType = "artifact.renderer.missing"
	FactArtifactWorkspaceLoaded         EventType = "artifact.workspace.loaded"
	FactArtifactWorkspaceLoadFailed     EventType = "artifact.workspace.load_failed"
	FactArtifactExportCompleted         EventType = "artifact.export.completed"
	FactArtifactSyncCompleted           EventType = "artifact.sync.completed"
	FactArtifactSyncFailed              EventType = "artifact.sync.failed"
	FactArtifactConflictDetected        EventType = "artifact.conflict.detected"
	FactArtifactMaterialized            EventType = "artifact.materialized"
)

// SchemaVersion must be incremented when breaking changes are made to any
// payload struct. Bus consumers reject mismatched versions.
//
// Version history:
//   - 1.0.0: Original event envelope and payloads.
//   - 2.0.0: Introduced RunID / Visibility on the envelope and the full
//     streaming runtime event vocabulary for runs, tools, intake, and
//     assistant streaming.
const SchemaVersion = "2.0.0"

func AcceptableSchemaVersions() map[string]bool {
	// During the 1.x → 2.x migration window, accept both versions so that
	// older consumers can continue to read events written by newer binaries.
	//
	// Once all deployed components have been upgraded to understand 2.0.0
	// payloads, 1.0.0 can be removed from this map in a follow-up change.
	return map[string]bool{
		"1.0.0":       true,
		SchemaVersion: true,
	}
}

// EventVisibility controls which stream class an event appears in.
type EventVisibility string

const (
	VisibilityUser     EventVisibility = "user_visible"
	VisibilityOperator EventVisibility = "operator_visible"
	VisibilityAudit    EventVisibility = "audit_only"
)

// Event is the envelope for every message on the bus.
type Event struct {
	ID            string          `json:"id"`
	Type          EventType       `json:"type"`
	Kind          EventKind       `json:"kind"`
	CorrelationID string          `json:"correlation_id"`
	CausalParent  string          `json:"causal_parent,omitempty"`
	SourceAgent   AgentType       `json:"source_agent"`
	TargetAgent   AgentType       `json:"target_agent,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       any             `json:"payload"`
	SchemaVersion string          `json:"schema_version"`
	Seq           int64           `json:"seq,omitempty"`
	RunID         string          `json:"run_id,omitempty"`
	Visibility    EventVisibility `json:"visibility,omitempty"`
}

// NewEvent constructs an Event with a generated ID and current timestamp.
func NewEvent(eventType EventType, kind EventKind, correlationID string, sourceAgent AgentType, payload any) Event {
	return Event{
		ID:            uuid.New().String(),
		Type:          eventType,
		Kind:          kind,
		CorrelationID: correlationID,
		SourceAgent:   sourceAgent,
		Timestamp:     time.Now().UTC(),
		Payload:       payload,
		SchemaVersion: SchemaVersion,
	}
}

// NewRunEvent constructs an Event scoped to a specific run with visibility.
func NewRunEvent(eventType EventType, kind EventKind, correlationID string, sourceAgent AgentType, runID string, visibility EventVisibility, payload any) Event {
	ev := NewEvent(eventType, kind, correlationID, sourceAgent, payload)
	ev.RunID = runID
	ev.Visibility = visibility
	return ev
}

// Validate checks the event envelope for required fields.
func (e *Event) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event: id is required")
	}
	if e.Type == "" {
		return fmt.Errorf("event: type is required")
	}
	if e.Kind != EventKindCommand && e.Kind != EventKindFact {
		return fmt.Errorf("event: kind must be %q or %q, got %q", EventKindCommand, EventKindFact, e.Kind)
	}
	if e.CorrelationID == "" {
		return fmt.Errorf("event: correlation_id is required")
	}
	if e.SourceAgent == "" {
		return fmt.Errorf("event: source_agent is required")
	}
	if e.Payload == nil {
		return fmt.Errorf("event: payload is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Typed payload structs — one per EventType
// ---------------------------------------------------------------------------

type AgentRegisterPayload struct {
	AgentID   string    `json:"agent_id"`
	AgentType AgentType `json:"agent_type"`
}

type AgentRetirePayload struct {
	AgentID string `json:"agent_id"`
}

type CostRecordedPayload struct {
	Cost CostAttribution `json:"cost"`
}

// GovernorTrippedPayload carries the reason the global governor stopped orchestration.
type GovernorTrippedPayload struct {
	GovernorType string `json:"governor_type"` // e.g. "actions", "cost", "duration"
	Limit        string `json:"limit"`
	Actual       string `json:"actual"`
}

// AgentBudgetExceededPayload is emitted when an individual agent exhausts its action budget.
type AgentBudgetExceededPayload struct {
	RuntimeSessionID string    `json:"runtime_session_id"`
	AgentType        AgentType `json:"agent_type"`
	ActionCount      int       `json:"action_count"`
	Budget           int       `json:"budget"`
}

type DirectiveRepliedPayload struct {
	DirectiveID string `json:"directive_id"`
	MessageID   string `json:"message_id"`
}

type DirectiveMessagePayload struct {
	DirectiveID string `json:"directive_id"`
	MessageID   string `json:"message_id"`
	OwnerID     string `json:"owner_id"`
}

// NaviHeartbeatDonePayload is emitted after NAVI completes a heartbeat cycle.
type NaviHeartbeatDonePayload struct {
	RuntimeSessionID string `json:"runtime_session_id"`
	TaskCount        int    `json:"task_count"`
	DurationMs       int64  `json:"duration_ms"`
}

// NaviExperienceSnapshotPayload is an audit/history record of the derived
// experience-layer state for a turn or session boundary.
type NaviExperienceSnapshotPayload struct {
	SnapshotID           string  `json:"snapshot_id"`
	ChatID               string  `json:"chat_id,omitempty"`
	OwnerID              string  `json:"owner_id,omitempty"`
	ExperienceMode       string  `json:"experience_mode,omitempty"`
	Trigger              string  `json:"trigger"`
	CumulativeTraitDelta float64 `json:"cumulative_trait_delta,omitempty"`
	SourceStateID        string  `json:"source_state_id"`
	CompiledPayloadID    string  `json:"compiled_payload_id,omitempty"`
	EffectiveStateJSON   string  `json:"effective_state_json"`
	CompiledPayloadJSON  string  `json:"compiled_payload_json,omitempty"`
}

// NaviActionRequestedPayload is emitted when NAVI wants to execute a high-risk
// action that requires operator approval. ProposalID is set when a proposal was
// created and can be used to resolve (approve/decline) via the API.
type NaviActionRequestedPayload struct {
	ChatID     string         `json:"chat_id"`
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments"`
	Reason     string         `json:"reason"`
	ProposalID string         `json:"proposal_id,omitempty"`
}

// TaskAssignedPayload is the payload for CmdTaskAssign; carries the task for the worker.
type TaskAssignedPayload struct {
	Task Task `json:"task"`
}

// ---------------------------------------------------------------------------
// Run lifecycle payloads
// ---------------------------------------------------------------------------

// RunStatus is the lifecycle state of a run.
type RunStatus string

const (
	RunStatusStarting           RunStatus = "starting"
	RunStatusActive             RunStatus = "active"
	RunStatusPaused             RunStatus = "paused"
	RunStatusWaitingForTool     RunStatus = "waiting_for_tool"
	RunStatusWaitingForProposal RunStatus = "waiting_for_proposal"
	RunStatusWaitingForRecovery RunStatus = "waiting_for_recovery"
	RunStatusCompleted          RunStatus = "completed"
	RunStatusFailed             RunStatus = "failed"
	RunStatusCancelled          RunStatus = "cancelled"
)

// RunStartedPayload is emitted when processTurn begins a new run.
type RunStartedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	ExperienceMode   string `json:"experience_mode,omitempty"`
	Phase            string `json:"phase,omitempty"`
}

// RunPhaseChangedPayload tracks the current runtime phase within a run.
type RunPhaseChangedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Phase            string `json:"phase"`
	Previous         string `json:"previous,omitempty"`
	Checkpoint       string `json:"checkpoint,omitempty"`
}

// InferenceDecisionTracePayload exposes the committed ICS decision artifact for a run cycle.
type InferenceDecisionTracePayload struct {
	RunID                  string            `json:"run_id"`
	RuntimeSessionID       string            `json:"runtime_session_id"`
	TraceID                string            `json:"trace_id,omitempty"`
	ContractVersion        string            `json:"contract_version,omitempty"`
	FocusID                string            `json:"focus_id,omitempty"`
	SelectedGoalID         string            `json:"selected_goal_id,omitempty"`
	FocusReason            string            `json:"focus_reason,omitempty"`
	DominantMode           string            `json:"dominant_mode,omitempty"`
	SubmodeChain           []string          `json:"submode_chain,omitempty"`
	InvokedSubreasoners    []string          `json:"invoked_subreasoners,omitempty"`
	PinnedSubreasoners     []string          `json:"pinned_subreasoners,omitempty"`
	SuppressedSubreasoners []string          `json:"suppressed_subreasoners,omitempty"`
	ArbitrationNotes       []string          `json:"arbitration_notes,omitempty"`
	CandidateIDs           []string          `json:"candidate_ids,omitempty"`
	CandidateID            string            `json:"candidate_id,omitempty"`
	CandidateType          string            `json:"candidate_type,omitempty"`
	CandidateScore         float64           `json:"candidate_score,omitempty"`
	ThresholdBand          string            `json:"threshold_band,omitempty"`
	GovernanceOutcome      string            `json:"governance_outcome,omitempty"`
	ExecutionActionType    string            `json:"execution_action_type,omitempty"`
	ExecutionTarget        string            `json:"execution_target,omitempty"`
	ApprovalRequirement    string            `json:"approval_requirement,omitempty"`
	TraceStage             string            `json:"trace_stage,omitempty"`
	TargetCapability       string            `json:"target_capability,omitempty"`
	AllowedCapabilities    []string          `json:"allowed_capabilities,omitempty"`
	PlanID                 string            `json:"plan_id,omitempty"`
	CurrentNodeID          string            `json:"current_node_id,omitempty"`
	PlanStatus             string            `json:"plan_status,omitempty"`
	CheckpointRefs         []string          `json:"checkpoint_refs,omitempty"`
	RecoveryRoute          string            `json:"recovery_route,omitempty"`
	RecoveryCheckpointRef  string            `json:"recovery_checkpoint_ref,omitempty"`
	ExecutionOutcomeRef    string            `json:"execution_outcome_ref,omitempty"`
	ExecutedCapability     string            `json:"executed_capability,omitempty"`
	ExecutedCommandType    string            `json:"executed_command_type,omitempty"`
	ProposalID             string            `json:"proposal_id,omitempty"`
	RecoveryRef            string            `json:"recovery_ref,omitempty"`
	ResumeFocusID          string            `json:"resume_focus_id,omitempty"`
	ReflectionTrigger      string            `json:"reflection_trigger,omitempty"`
	ReflectionRef          string            `json:"reflection_ref,omitempty"`
	Tags                   map[string]string `json:"tags,omitempty"`
}

// RunPausedPayload is emitted when a run is paused for external input.
type RunPausedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Reason           string `json:"reason"`
	ProposalID       string `json:"proposal_id,omitempty"`
	Phase            string `json:"phase,omitempty"`
}

// RunResumedPayload is emitted when a paused run resumes execution.
type RunResumedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	ProposalID       string `json:"proposal_id,omitempty"`
	Phase            string `json:"phase,omitempty"`
}

// RunCompletedPayload is emitted when a run finishes successfully.
type RunCompletedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	ChatID           string `json:"chat_id,omitempty"`
	ReplyLen         int    `json:"reply_len"`
	ToolCalls        int    `json:"tool_calls"`
	DurationMs       int64  `json:"duration_ms"`
	FinalMessageID   string `json:"final_message_id,omitempty"`
}

// RunFailedPayload is emitted when a run fails.
type RunFailedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Error            string `json:"error"`
	DurationMs       int64  `json:"duration_ms"`
}

// RunCancelledPayload is emitted when a run is cancelled (e.g. by interrupt).
type RunCancelledPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Reason           string `json:"reason"`
}

type InterruptRaisedPayload struct {
	RunID            string `json:"run_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id"`
	InterruptClass   string `json:"interrupt_class"`
	Reason           string `json:"reason"`
	SourceChannel    string `json:"source_channel,omitempty"`
}

type InterruptAppliedPayload struct {
	RunID            string `json:"run_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id"`
	InterruptClass   string `json:"interrupt_class"`
	Reason           string `json:"reason"`
	Outcome          string `json:"outcome,omitempty"`
}

// ---------------------------------------------------------------------------
// Tool call lifecycle payloads
// ---------------------------------------------------------------------------

// ToolCallStartedPayload is emitted before tool execution begins.
type ToolCallStartedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	CallID           string `json:"call_id"`
	ToolName         string `json:"tool_name"`
}

type ToolCallProgressPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	CallID           string `json:"call_id"`
	ToolName         string `json:"tool_name"`
	Stage            string `json:"stage"`
	Message          string `json:"message,omitempty"`
	DurationMs       int64  `json:"duration_ms,omitempty"`
}

// ToolCallCompletedPayload is emitted after a tool returns successfully.
type ToolCallCompletedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	CallID           string `json:"call_id"`
	ToolName         string `json:"tool_name"`
	Result           string `json:"result,omitempty"`
	DurationMs       int64  `json:"duration_ms"`
}

// ToolCallFailedPayload is emitted when a tool call fails.
type ToolCallFailedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	CallID           string `json:"call_id"`
	ToolName         string `json:"tool_name"`
	Error            string `json:"error"`
	DurationMs       int64  `json:"duration_ms"`
}

// ---------------------------------------------------------------------------
// Message intake payload
// ---------------------------------------------------------------------------

// MessageReceivedPayload is emitted when an inbound message is accepted.
type MessageReceivedPayload struct {
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	ChatID           string `json:"chat_id,omitempty"`
	InboxItemID      string `json:"inbox_item_id,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	SourceChannel    string `json:"source_channel"` // "cli" | "app" | "web" | "telegram" | "system"
	SourceMessageRef string `json:"source_message_ref,omitempty"`
	ContentLen       int    `json:"content_len"`
	QueueAction      string `json:"queue_action,omitempty"`
}

// MessageClassifiedPayload records runtime queue handling for an inbox item.
type MessageClassifiedPayload struct {
	RuntimeSessionID string  `json:"runtime_session_id,omitempty"`
	ChatID           string  `json:"chat_id,omitempty"`
	InboxItemID      string  `json:"inbox_item_id"`
	QueueAction      string  `json:"queue_action"`
	Reason           string  `json:"reason,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`
}

// MessageQueueActionPayload records a concrete queue-state transition.
type MessageQueueActionPayload struct {
	RuntimeSessionID  string  `json:"runtime_session_id,omitempty"`
	ChatID            string  `json:"chat_id,omitempty"`
	InboxItemID       string  `json:"inbox_item_id"`
	QueueAction       string  `json:"queue_action"`
	Reason            string  `json:"reason,omitempty"`
	Confidence        float64 `json:"confidence,omitempty"`
	TargetInboxItemID string  `json:"target_inbox_item_id,omitempty"`
}

// AssistantTokenDeltaPayload streams raw model text deltas.
type AssistantTokenDeltaPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Delta            string `json:"delta"`
}

// AssistantMessagePartialPayload streams throttled partial message snapshots.
type AssistantMessagePartialPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	Content          string `json:"content"`
	State            string `json:"state,omitempty"`
}

type AssistantMessageKind string

const (
	AssistantMessageKindReply     AssistantMessageKind = "reply"
	AssistantMessageKindProactive AssistantMessageKind = "proactive"
)

// AssistantMessageCompletedPayload marks the committed assistant message.
type AssistantMessageCompletedPayload struct {
	RunID            string `json:"run_id"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	ChatID           string `json:"chat_id,omitempty"`
	MessageID        string `json:"message_id"`
	Content          string `json:"content"`
	ExperienceMode   string `json:"experience_mode,omitempty"`
	InboxItemID      string `json:"inbox_item_id,omitempty"`
	MessageKind      string `json:"message_kind,omitempty"`
}

type GovernanceBlockedPayload struct {
	RunID            string `json:"run_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	ToolName         string `json:"tool_name"`
	Reason           string `json:"reason"`
	Outcome          string `json:"outcome"`
	ProposalID       string `json:"proposal_id,omitempty"`
}

// ProposalWaitingPayload is emitted when a run pauses on approval.
type ProposalWaitingPayload struct {
	RunID            string         `json:"run_id"`
	RuntimeSessionID string         `json:"runtime_session_id"`
	ProposalID       string         `json:"proposal_id"`
	ToolName         string         `json:"tool_name"`
	Arguments        map[string]any `json:"arguments,omitempty"`
	Reason           string         `json:"reason"`
}

// ProposalResolvedPayload records proposal resolution against a paused run.
type ProposalResolvedPayload struct {
	RunID            string `json:"run_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	ProposalID       string `json:"proposal_id"`
	Status           string `json:"status"`
	ResolvedBy       string `json:"resolved_by,omitempty"`
	Note             string `json:"note,omitempty"`
}

// DegradationNotedPayload exposes user-visible degradation without pausing the run.
type DegradationNotedPayload struct {
	RunID               string   `json:"run_id,omitempty"`
	RuntimeSessionID    string   `json:"runtime_session_id,omitempty"`
	DegradationType     string   `json:"degradation_type"`
	Message             string   `json:"message"`
	RecoveryProposalID  string   `json:"recovery_proposal_id,omitempty"`
	ToolName            string   `json:"tool_name,omitempty"`
	FailureCode         string   `json:"failure_code,omitempty"`
	FallbackPath        []string `json:"fallback_path,omitempty"`
	FallbackDisposition string   `json:"fallback_disposition,omitempty"`
}

// RecoveryRequiredPayload records deferred recovery work.
type RecoveryRequiredPayload struct {
	RunID            string   `json:"run_id,omitempty"`
	RuntimeSessionID string   `json:"runtime_session_id,omitempty"`
	ProposalID       string   `json:"proposal_id,omitempty"`
	RecoveryRoute    string   `json:"recovery_route,omitempty"`
	CheckpointRef    string   `json:"checkpoint_ref,omitempty"`
	CurrentTask      string   `json:"current_task,omitempty"`
	CurrentStep      string   `json:"current_step,omitempty"`
	PendingBlockers  []string `json:"pending_blockers,omitempty"`
	ResumeConditions []string `json:"resume_conditions,omitempty"`
	RollbackPoint    string   `json:"rollback_point,omitempty"`
	FailureReason    string   `json:"failure_reason"`
}

// ---------------------------------------------------------------------------
// Artifact lifecycle payloads
// ---------------------------------------------------------------------------

type ArtifactCreatedPayload struct {
	ArtifactID     string `json:"artifact_id"`
	ChatID         string `json:"chat_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Type           string `json:"type"`
	Subtype        string `json:"subtype,omitempty"`
	Title          string `json:"title"`
	LifecycleState string `json:"lifecycle_state"`
}

type ArtifactUpdatedPayload struct {
	ArtifactID     string `json:"artifact_id"`
	ChatID         string `json:"chat_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Version        int    `json:"version"`
	LifecycleState string `json:"lifecycle_state,omitempty"`
}

type ArtifactVersionCommittedPayload struct {
	ArtifactID string `json:"artifact_id"`
	Version    int    `json:"version"`
	RunID      string `json:"run_id,omitempty"`
	Checkpoint string `json:"checkpoint,omitempty"`
}

type ArtifactLifecycleChangedPayload struct {
	ArtifactID string `json:"artifact_id"`
	Previous   string `json:"previous"`
	Current    string `json:"current"`
	Reason     string `json:"reason,omitempty"`
}

type ArtifactMaterializedPayload struct {
	ArtifactID string `json:"artifact_id"`
	ToolName   string `json:"tool_name"`
	RunID      string `json:"run_id,omitempty"`
}

// ArtifactObservedPayload is the normalized artifact observability payload used
// by the V1 artifact event surface.
type ArtifactObservedPayload struct {
	WorkspaceID      string `json:"workspace_id,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	ArtifactID       string `json:"artifact_id"`
	VersionID        string `json:"version_id,omitempty"`
	BranchID         string `json:"branch_id,omitempty"`
	HistoryCommandID string `json:"history_command_id,omitempty"`
	HistoryAttemptID string `json:"history_attempt_id,omitempty"`
	ActorType        string `json:"actor_type,omitempty"`
	ActorID          string `json:"actor_id,omitempty"`
	Operation        string `json:"operation,omitempty"`
	Subtype          string `json:"subtype,omitempty"`
	RendererKey      string `json:"renderer_key,omitempty"`
	LatencyMs        int64  `json:"latency_ms,omitempty"`
	ResultStatus     string `json:"result_status,omitempty"`
	FailureClass     string `json:"failure_class,omitempty"`
	FailureCode      string `json:"failure_code,omitempty"`
	LifecycleState   string `json:"lifecycle_state,omitempty"`
	ExportID         string `json:"export_id,omitempty"`
	ShareID          string `json:"share_id,omitempty"`
}

// ContextReadAuditPayload is the audit attribution recorded for every governed
// read through the query_context surface (Language-Layer Contract §4). It records
// who read, for what purpose, at what scope, and which redactions were applied —
// never the read content itself.
type ContextReadAuditPayload struct {
	Caller     string   `json:"caller"`
	Purpose    string   `json:"purpose"`
	Scope      string   `json:"scope"`
	RunID      string   `json:"run_id,omitempty"`
	Redactions []string `json:"redactions,omitempty"`
	Found      bool     `json:"found"`
}
