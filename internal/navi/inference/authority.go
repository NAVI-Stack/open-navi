package inference

import (
	"context"
	"strings"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

// RuntimeDisposition is the authoritative instruction ICS gives the runtime.
type RuntimeDisposition string

const (
	RuntimeDispositionCallModel        RuntimeDisposition = "call_model"
	RuntimeDispositionPauseForProposal RuntimeDisposition = "pause_for_proposal"
	RuntimeDispositionBlockWithReply   RuntimeDisposition = "block_with_reply"
	RuntimeDispositionProceedDirect    RuntimeDisposition = "proceed_direct"
)

// ExecutionBoundary is the authoritative executable boundary emitted by ICS.
type ExecutionBoundary struct {
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`
	TargetCapability    string   `json:"target_capability,omitempty"`
	ToolChoice          string   `json:"tool_choice,omitempty"`
	FailClosedReason    string   `json:"fail_closed_reason,omitempty"`
}

// ModelDirective is the exact model-call boundary ICS authorizes for one cycle.
type ModelDirective struct {
	AllowToolCalls bool                    `json:"allow_tool_calls,omitempty"`
	ToolNames      []string                `json:"tool_names,omitempty"`
	ToolChoice     string                  `json:"tool_choice,omitempty"`
	ToolContracts  map[string]ToolContract `json:"tool_contracts,omitempty"`
}

// ToolExecutionKind describes how runtime should dispatch an executable tool.
type ToolExecutionKind string

const (
	ToolExecutionKindRegistry ToolExecutionKind = "registry"
)

// ToolContract is the authoritative executable contract ICS carries into runtime.
type ToolContract struct {
	ID                   string                     `json:"id,omitempty"`
	ToolName             string                     `json:"tool_name,omitempty"`
	SourceType           string                     `json:"source_type,omitempty"`
	ExecutionKind        ToolExecutionKind          `json:"execution_kind,omitempty"`
	CommandType          schema.CommandType         `json:"command_type,omitempty"`
	Domain               string                     `json:"domain,omitempty"`
	ActorKind            string                     `json:"actor_kind,omitempty"`
	WorkspaceAction      schema.WorkspaceActionType `json:"workspace_action,omitempty"`
	TargetPathArg        string                     `json:"target_path_arg,omitempty"`
	WorkspaceScopedPath  bool                       `json:"workspace_scoped_path,omitempty"`
	RequiresConfirmation bool                       `json:"requires_confirmation,omitempty"`
	RiskLevel            schema.RiskLevel           `json:"risk_level,omitempty"`
	Reversibility        schema.ReversibilityClass  `json:"reversibility,omitempty"`
	ExpectedSideEffects  []string                   `json:"expected_side_effects,omitempty"`
	SkillName            string                     `json:"skill_name,omitempty"`
	PluginName           string                     `json:"plugin_name,omitempty"`
	ConnectorID          string                     `json:"connector_id,omitempty"`
}

// ToolPermit is the exact executable permit ICS authorizes for one tool call.
type ToolPermit struct {
	ContractID         string        `json:"contract_id,omitempty"`
	Contract           *ToolContract `json:"contract,omitempty"`
	ToolCallID         string        `json:"tool_call_id,omitempty"`
	ToolName           string        `json:"tool_name,omitempty"`
	ApprovedProposalID string        `json:"approved_proposal_id,omitempty"`
	TargetPath         string        `json:"target_path,omitempty"`
}

// AuthorizedToolInvocation is one fully governed tool call that capability execution may consume.
type AuthorizedToolInvocation struct {
	ToolCall llm.ToolCall `json:"tool_call,omitempty"`
	Permit   ToolPermit   `json:"permit,omitempty"`
}

// DecisionEnvelope is the authoritative control artifact persisted by runtime.
type DecisionEnvelope struct {
	Rationale           Rationale                  `json:"rationale"`
	GovernanceResult    *governor.ValidationResult `json:"governance_result,omitempty"`
	Proposal            *schema.Proposal           `json:"proposal,omitempty"`
	RuntimeDisposition  RuntimeDisposition         `json:"runtime_disposition,omitempty"`
	ExecutionBoundary   ExecutionBoundary          `json:"execution_boundary,omitempty"`
	ModelDirective      *ModelDirective            `json:"model_directive,omitempty"`
	AuthorizedToolCalls []llm.ToolCall             `json:"authorized_tool_calls,omitempty"`
	AuthorizedTools     []AuthorizedToolInvocation `json:"authorized_tools,omitempty"`
	ToolPermit          *ToolPermit                `json:"tool_permit,omitempty"`
	PendingToolCall     *llm.ToolCall              `json:"pending_tool_call,omitempty"`
	PendingToolPermit   *ToolPermit                `json:"pending_tool_permit,omitempty"`
	LastResult          *ExecutionSnapshot         `json:"last_result,omitempty"`
	ReplyMessage        string                     `json:"reply_message,omitempty"`
}

// ModelCallPreparationInput describes the raw model request runtime is asking ICS to shape.
type ModelCallPreparationInput struct {
	Request             orchestration.CanonicalRunRequest  `json:"request,omitempty"`
	Compiled            orchestration.CompiledModelRequest `json:"compiled,omitempty"`
	ExecutableToolNames []string                           `json:"executable_tool_names,omitempty"`
	ToolContracts       map[string]ToolContract            `json:"tool_contracts,omitempty"`
	ActiveToolSet       *navitool.ActiveToolSet            `json:"-"`
	ToolRegistry        *navitool.Registry                 `json:"-"`
}

// ModelResponseAuthorizationInput describes one normalized model response that must be validated by ICS.
type ModelResponseAuthorizationInput struct {
	Compiled                orchestration.CompiledModelRequest    `json:"compiled,omitempty"`
	Response                orchestration.NormalizedModelResponse `json:"response,omitempty"`
	ToolAttempts            []ToolAuthorizationInput              `json:"tool_attempts,omitempty"`
	ToolRegistry            *navitool.Registry                    `json:"-"`
	ActiveToolSet           *navitool.ActiveToolSet               `json:"-"`
	RepairAttemptsRemaining int                                   `json:"repair_attempts_remaining,omitempty"`
}

// ToolAuthorizationInput describes one concrete runtime tool attempt.
type ToolAuthorizationInput struct {
	Chat                 ChatContext    `json:"chat,omitempty"`
	Runtime              RuntimeContext `json:"runtime,omitempty"`
	ToolCallID           string         `json:"tool_call_id,omitempty"`
	ToolName             string         `json:"tool_name,omitempty"`
	Contract             *ToolContract  `json:"contract,omitempty"`
	Arguments            map[string]any `json:"arguments,omitempty"`
	ApprovedProposalID   string         `json:"approved_proposal_id,omitempty"`
	VerifiedProposalID   string         `json:"verified_proposal_id,omitempty"`
	ResolvedTargetPath   string         `json:"resolved_target_path,omitempty"`
	RequiresConfirmation bool           `json:"requires_confirmation,omitempty"`
}

// ControllerDeps supplies external effects the canonical controller may need
// while keeping decision control inside ICS code.
type ControllerDeps struct {
	ValidationDependency ValidationDependency
	GetProposal          func(ctx context.Context, proposalID string) (schema.Proposal, error)
}

func defaultDecisionEnvelope(rationale Rationale) DecisionEnvelope {
	boundary := ExecutionBoundary{
		AllowedCapabilities: compactStrings(append([]string(nil), rationale.ExecutionIntent.AuthorizedCapabilities()...)),
		TargetCapability:    strings.TrimSpace(rationale.ExecutionIntent.PrimaryCapability()),
	}
	if boundary.TargetCapability == "" && strings.TrimSpace(rationale.Governance.TargetCapability) != "" {
		boundary.TargetCapability = strings.TrimSpace(rationale.Governance.TargetCapability)
	}
	if len(boundary.AllowedCapabilities) == 0 {
		boundary.AllowedCapabilities = compactStrings(append([]string(nil), rationale.Governance.AllowedCapabilities...))
	}
	if boundary.TargetCapability == "" && len(boundary.AllowedCapabilities) == 1 {
		boundary.TargetCapability = boundary.AllowedCapabilities[0]
	}
	disposition := RuntimeDispositionCallModel
	replyMessage := ""

	if rationale.ExecutionIntent.AllowsCapabilityExecution() && len(boundary.AllowedCapabilities) == 0 {
		disposition = RuntimeDispositionBlockWithReply
		replyMessage = "ICS did not authorize an executable capability boundary for this run"
		boundary.FailClosedReason = replyMessage
	}

	return DecisionEnvelope{
		Rationale:          rationale,
		RuntimeDisposition: disposition,
		ExecutionBoundary:  boundary,
		ReplyMessage:       replyMessage,
	}
}

func authorizationBlockedEnvelope(prior DecisionEnvelope, validation governor.ValidationResult, reason string) DecisionEnvelope {
	envelope := prior
	copy := validation
	envelope.GovernanceResult = &copy
	envelope.RuntimeDisposition = RuntimeDispositionBlockWithReply
	envelope.ReplyMessage = firstNonEmpty(strings.TrimSpace(reason), strings.TrimSpace(validation.Reason), "ICS blocked this action")
	envelope.ExecutionBoundary.FailClosedReason = envelope.ReplyMessage
	envelope.Rationale.Governance.ValidationResult = &copy
	envelope.Rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(validation.Outcome)
	return envelope
}

func authorizationPauseEnvelope(prior DecisionEnvelope, validation governor.ValidationResult, proposal *schema.Proposal) DecisionEnvelope {
	envelope := prior
	copy := validation
	envelope.GovernanceResult = &copy
	envelope.Proposal = proposal
	envelope.RuntimeDisposition = RuntimeDispositionPauseForProposal
	envelope.ReplyMessage = firstNonEmpty(strings.TrimSpace(validation.Reason), "approval required")
	envelope.ExecutionBoundary.FailClosedReason = envelope.ReplyMessage
	envelope.Rationale.Governance.ValidationResult = &copy
	envelope.Rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(validation.Outcome)
	if proposal != nil {
		envelope.Rationale.ReflectionHooks.ProposalCandidate = proposal.ProposalID
		envelope.Rationale.DecisionTrace.ProposalID = proposal.ProposalID
	}
	return envelope
}

func validationOutcomeName(outcome governor.ValidationOutcome) string {
	switch outcome {
	case governor.ValidationApproved:
		return "approved"
	case governor.ValidationRequiresConfirmation:
		return "requires_confirmation"
	case governor.ValidationModified:
		return "modified"
	case governor.ValidationRejected:
		return "rejected"
	default:
		return ""
	}
}
