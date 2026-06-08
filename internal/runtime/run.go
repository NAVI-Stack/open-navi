package runtime

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// ScheduledMessage is one message to send, with an optional delay before delivery.
// Delay is relative to when the executor returns (run completion).
type ScheduledMessage struct {
	Content string
	Delay   time.Duration
}

// ToolInvocationPart is a structured record of a tool call made during a run,
// surfaced as a chip in the chat UI. State is "call" while the tool is running
// and "result" once it has returned (Result holds a short preview).
type ToolInvocationPart struct {
	ToolInvocationID string `json:"toolInvocationId"`
	ToolName         string `json:"toolName"`
	State            string `json:"state"`
	Result           string `json:"result,omitempty"`
	IsError          bool   `json:"isError,omitempty"`
}

type RunMode string

const (
	RunModeForeground RunMode = "foreground"
	RunModeBackground RunMode = "background"
)

type RunPhase string

const (
	RunPhaseIntake         RunPhase = "intake"
	RunPhaseContextualize  RunPhase = "contextualize"
	RunPhaseDecide         RunPhase = "decide"
	RunPhaseValidateGovern RunPhase = "validate_govern"
	RunPhaseExecute        RunPhase = "execute"
	RunPhaseStreamFinalize RunPhase = "stream_finalize"
)

type InboxStatus string

const (
	InboxStatusPending    InboxStatus = "pending"
	InboxStatusConsumed   InboxStatus = "consumed"
	InboxStatusDeferred   InboxStatus = "deferred"
	InboxStatusMerged     InboxStatus = "merged"
	InboxStatusSuperseded InboxStatus = "superseded"
)

type InterruptClass string

const (
	InterruptClassUserCancel InterruptClass = "user_cancel"
	InterruptClassGovernance InterruptClass = "governance"
	InterruptClassSystem     InterruptClass = "system"
)

// RunState tracks the lifecycle of a single run within a runtime session.
// A run is the unit of work triggered by an inbound message — it encompasses
// the LLM call loop, tool executions, and reply emission.
type RunState struct {
	RunID                  string            `json:"run_id"`
	RuntimeSessionID       string            `json:"runtime_session_id"`
	ChatID                 string            `json:"chat_id,omitempty"`
	ProjectID              string            `json:"project_id,omitempty"`
	WorkspaceID            string            `json:"workspace_id,omitempty"`
	ExperienceMode         string            `json:"experience_mode,omitempty"`
	OriginEndpointID       string            `json:"origin_endpoint_id,omitempty"`
	Mode                   RunMode           `json:"mode"`
	Status                 schema.RunStatus  `json:"status"`
	CurrentPhase           RunPhase          `json:"current_phase"`
	InitiatedByInboxItemID string            `json:"initiated_by_inbox_item_id,omitempty"`
	PauseReason            string            `json:"pause_reason,omitempty"`
	BlockedOnProposalID    string            `json:"blocked_on_proposal_id,omitempty"`
	InterruptClass         InterruptClass    `json:"interrupt_class,omitempty"`
	InterruptReason        string            `json:"interrupt_reason,omitempty"`
	LatestCheckpointID     string            `json:"latest_checkpoint_id,omitempty"`
	StartedAt              time.Time         `json:"started_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	ToolCalls              int               `json:"tool_calls"`
	LLMProvider            string            `json:"llm_provider,omitempty"`
	LLMModel               string            `json:"llm_model,omitempty"`
	LLMTaskClass           string            `json:"llm_task_class,omitempty"`
	LLMComplexity          string            `json:"llm_complexity,omitempty"`
	Scratchpad             map[string]string `json:"scratchpad,omitempty"`
	ICSStateVersion        string            `json:"ics_state_version,omitempty"`
	ICSDecisionEnvelope    []byte            `json:"ics_decision_envelope,omitempty"`

	// Artifacts (OMN-118)
	MainArtifactID string   `json:"main_artifact_id,omitempty"` // The primary work product of this run
	ArtifactIDs    []string `json:"artifact_ids,omitempty"`     // All artifacts created/updated during this run

	// ToolParts records tool invocations surfaced to the chat UI for this run.
	// Not persisted with the run; consumed when the assistant message is written.
	ToolParts []ToolInvocationPart `json:"tool_parts,omitempty"`

	// RenderPayload carries an opaque, pre-marshaled data-driven render payload
	// (a render.RenderPayload as JSON) produced by the render layer. Kept as raw
	// JSON so this package stays free of an import on internal/navi/render. Like
	// ToolParts it is not persisted with the run — it is merged into the
	// assistant message's metadata_json when the reply is written.
	RenderPayload json.RawMessage `json:"render_payload,omitempty"`
}

// RecordToolCallStarted appends a running ("call" state) tool part for callID,
// unless one already exists for that id.
func (r *RunState) RecordToolCallStarted(callID, toolName string) {
	if r == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	for i := range r.ToolParts {
		if r.ToolParts[i].ToolInvocationID == callID {
			return
		}
	}
	r.ToolParts = append(r.ToolParts, ToolInvocationPart{
		ToolInvocationID: callID,
		ToolName:         strings.TrimSpace(toolName),
		State:            "call",
	})
}

// RecordToolCallResult resolves the tool part for callID to the "result" state.
// If no matching "call" part exists, a resolved part is appended.
func (r *RunState) RecordToolCallResult(callID, toolName, result string, isError bool) {
	if r == nil {
		return
	}
	callID = strings.TrimSpace(callID)
	for i := range r.ToolParts {
		if r.ToolParts[i].ToolInvocationID == callID {
			r.ToolParts[i].State = "result"
			r.ToolParts[i].Result = result
			r.ToolParts[i].IsError = isError
			if name := strings.TrimSpace(toolName); name != "" {
				r.ToolParts[i].ToolName = name
			}
			return
		}
	}
	r.ToolParts = append(r.ToolParts, ToolInvocationPart{
		ToolInvocationID: callID,
		ToolName:         strings.TrimSpace(toolName),
		State:            "result",
		Result:           result,
		IsError:          isError,
	})
}

// NewRun creates a RunState in "starting" status.
func NewRun(runtimeSessionID, experienceMode string) *RunState {
	now := time.Now().UTC()
	return &RunState{
		RunID:            uuid.New().String(),
		RuntimeSessionID: runtimeSessionID,
		ExperienceMode:   experienceMode,
		Mode:             RunModeForeground,
		Status:           schema.RunStatusStarting,
		CurrentPhase:     RunPhaseIntake,
		StartedAt:        now,
		UpdatedAt:        now,
	}
}

// SetStatus transitions the run to a new status.
func (r *RunState) SetStatus(status schema.RunStatus) {
	r.Status = status
	r.UpdatedAt = time.Now().UTC()
}

// SetPhase updates the run's current phase.
func (r *RunState) SetPhase(phase RunPhase) {
	r.CurrentPhase = phase
	r.UpdatedAt = time.Now().UTC()
}

// IncrementToolCalls increments the tool call counter.
func (r *RunState) IncrementToolCalls() {
	r.ToolCalls++
}

// DurationMs returns the elapsed time since the run started.
func (r *RunState) DurationMs() int64 {
	return time.Since(r.StartedAt).Milliseconds()
}

type ProposalResolution string

const (
	ProposalResolutionApprove    ProposalResolution = "approve"
	ProposalResolutionDecline    ProposalResolution = "decline"
	ProposalResolutionRevalidate ProposalResolution = "revalidate"
)

type ResumeSignal struct {
	ProposalID string             `json:"proposal_id,omitempty"`
	Resolution ProposalResolution `json:"resolution,omitempty"`
	Note       string             `json:"note,omitempty"`
}

type Checkpoint struct {
	CheckpointID          string            `json:"checkpoint_id"`
	RunID                 string            `json:"run_id"`
	RuntimeSessionID      string            `json:"runtime_session_id"`
	ChatID                string            `json:"chat_id,omitempty"`
	ProjectID             string            `json:"project_id,omitempty"`
	WorkspaceID           string            `json:"workspace_id,omitempty"`
	Phase                 RunPhase          `json:"phase"`
	ExperienceMode        string            `json:"experience_mode,omitempty"`
	LLMMessages           []llm.Message     `json:"llm_messages,omitempty"`
	Options               llm.Options       `json:"options,omitempty"`
	PendingToolCall       *llm.ToolCall     `json:"pending_tool_call,omitempty"`
	PendingProposalID     string            `json:"pending_proposal_id,omitempty"`
	PendingProposalReason string            `json:"pending_proposal_reason,omitempty"`
	Scratchpad            map[string]string `json:"scratchpad,omitempty"`
	ICSStateVersion       string            `json:"ics_state_version,omitempty"`
	ICSDecisionEnvelope   []byte            `json:"ics_decision_envelope,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`

	// Artifacts (OMN-118)
	MainArtifactID string   `json:"main_artifact_id,omitempty"`
	ArtifactIDs    []string `json:"artifact_ids,omitempty"`
}

func NewCheckpoint(run *RunState) *Checkpoint {
	return &Checkpoint{
		CheckpointID:        uuid.New().String(),
		RunID:               run.RunID,
		RuntimeSessionID:    run.RuntimeSessionID,
		ChatID:              run.ChatID,
		ProjectID:           run.ProjectID,
		WorkspaceID:         run.WorkspaceID,
		Phase:               run.CurrentPhase,
		ExperienceMode:      run.ExperienceMode,
		Scratchpad:          cloneScratchpad(run.Scratchpad),
		ICSStateVersion:     run.ICSStateVersion,
		ICSDecisionEnvelope: append([]byte(nil), run.ICSDecisionEnvelope...),
		MainArtifactID:      run.MainArtifactID,
		ArtifactIDs:         run.ArtifactIDs,
		CreatedAt:           time.Now().UTC(),
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneScratchpad(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func (r *RunState) SetScratchpadValue(key, value string) {
	if r == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if strings.TrimSpace(value) == "" {
		if r.Scratchpad != nil {
			delete(r.Scratchpad, key)
		}
		return
	}
	if r.Scratchpad == nil {
		r.Scratchpad = make(map[string]string)
	}
	r.Scratchpad[key] = value
	r.UpdatedAt = time.Now().UTC()
}

func (r *RunState) ClearScratchpad() {
	if r == nil {
		return
	}
	r.Scratchpad = nil
	r.UpdatedAt = time.Now().UTC()
}

type ExecuteInput struct {
	Run             *RunState
	Checkpoint      *Checkpoint
	Resume          *ResumeSignal
	InboxItem       *InboxItem
	ShouldInterrupt func() error
}

type ExecuteResult struct {
	Run               *RunState
	FinalContent      string
	ScheduledMessages []ScheduledMessage
	ExperienceMode    string
	Outcome           schema.ExecutionOutcomeOutcome
	OutcomeSummary    string
	ProgrammerResult  map[string]any
	Completed         bool
	Paused            bool
	Cancelled         bool
	ReplyLen          int
	Checkpoint        *Checkpoint
	ProposalID        string
	ProposalReason    string
	PendingToolCall   *llm.ToolCall
	ProposalArgs      map[string]any
	DeclinedContinue  bool
}
