package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// Descriptor captures metadata about a command execution.
type Descriptor struct {
	// Type is the primitive command type (query, create, update, delete, invoke, send, acquire, schedule, delegate, compose).
	Type schema.CommandType

	// CommandID is an optional stable identifier for the logical command.
	// When empty, a new UUID is generated.
	CommandID string

	// UserFacing indicates whether failures are directly visible to the end user.
	UserFacing bool

	// RuntimeSessionID, TaskID and Domain identify affected high-level entities.
	RuntimeSessionID     string
	TaskID        string
	Domain        string
	RunID         string
	CorrelationID string
	ParentRunID   string
	SkillIDs      []string
	ConnectorIDs  []string
	ProposalID    string
	LLMProvider   string
	LLMModel      string
	LLMTaskClass  string
	LLMComplexity string

	// ComposeMode is reserved for future multi-step compose semantics.
	ComposeMode schema.ComposeFailureMode

	// Reversibility hints whether failure may require compensation (used for outcome fields).
	Reversibility schema.ReversibilityClass

	// Workspace fields (OMN-108)
	WorkspaceID      string
	BoundaryCrossing bool
	ApprovalRequired bool
	ApprovalOutcome  schema.ApprovalOutcome
	ArtifactID       string
}

// Saver persists a single ExecutionOutcome.
type Saver func(ctx context.Context, eo schema.ExecutionOutcome) error

// Executor records execution outcomes for commands using a provided Saver.
type Executor struct {
	Save Saver
}

// NewExecutor creates a new Executor that uses the given Saver.
func NewExecutor(save Saver) *Executor {
	return &Executor{Save: save}
}

// Execute runs fn as a command described by desc, records an ExecutionOutcome, and returns fn's result.
func (e *Executor) Execute(ctx context.Context, desc Descriptor, fn func(ctx context.Context) (any, error)) (any, error) {
	if e == nil || e.Save == nil {
		// No-op executor when no saver is configured.
		return fn(ctx)
	}

	commandID := desc.CommandID
	if commandID == "" {
		commandID = uuid.New().String()
	}
	attemptID := commandID + ":1"
	startTime := time.Now().UTC()

	result, err := fn(WithDescriptor(ctx, desc))

	endTime := time.Now().UTC()
	outcome := schema.ExecutionOutcomeSucceeded
	failureClass := schema.FailureClass("")
	failureReason := ""
	retryable := false

	if err != nil {
		outcome = schema.ExecutionOutcomeFailed
		failureReason = err.Error()
		failureClass = ClassifyFailure(err)
		retryable = IsRetryable(desc.Type, failureClass)
		_ = schema.DegradationVisibilityFor(failureClass, desc.UserFacing, false)
	}

	affectedEntities := encodeAffectedEntities(desc.RuntimeSessionID, desc.TaskID)

	compensationRequired := false
	recoveryStatus := schema.RecoveryStatusNotRequired
	if err != nil && desc.Reversibility == schema.ReversibilityCompensable {
		compensationRequired = true
	}
	if outcome == schema.ExecutionOutcomePartiallySucceeded {
		recoveryStatus = schema.RecoveryStatusOpen
	}
	compensationStatus := schema.CompensationStatusNotRequired
	if compensationRequired {
		compensationStatus = schema.CompensationStatusPending
	}

	executionOutcome := schema.ExecutionOutcome{
		AttemptID:            attemptID,
		CommandID:            commandID,
		AttemptNumber:        1,
		CommandType:          desc.Type,
		StartTime:            startTime,
		EndTime:              &endTime,
		Outcome:              outcome,
		FailureClass:         failureClass,
		FailureReason:        failureReason,
		AffectedEntities:     affectedEntities,
		Retryable:            retryable,
		CompensationRequired: compensationRequired,
		CompensationStatus:   compensationStatus,
		RecoveryStatus:       recoveryStatus,
		ProposalID:           desc.ProposalID,
		RunID:                desc.RunID,
		RuntimeSessionID:     desc.RuntimeSessionID,
		CorrelationID:        desc.CorrelationID,
		ParentRunID:          desc.ParentRunID,
		SkillIDs:             desc.SkillIDs,
		ConnectorIDs:         desc.ConnectorIDs,
		LLMProvider:          desc.LLMProvider,
		LLMModel:             desc.LLMModel,
		LLMTaskClass:         desc.LLMTaskClass,
		LLMComplexity:        desc.LLMComplexity,

		// Workspace fields (OMN-108)
		WorkspaceID:      desc.WorkspaceID,
		BoundaryCrossing: desc.BoundaryCrossing,
		ApprovalRequired: desc.ApprovalRequired,
		ApprovalOutcome:  desc.ApprovalOutcome,
		ArtifactID:       desc.ArtifactID,
	}

	_ = e.Save(ctx, executionOutcome)

	return result, err
}

func encodeAffectedEntities(runtimeSessionID, taskID string) string {
	type entity struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}

	entities := make([]entity, 0, 2)
	if runtimeSessionID != "" {
		entities = append(entities, entity{Kind: "runtime_session", ID: runtimeSessionID})
	}
	if taskID != "" {
		entities = append(entities, entity{Kind: "task", ID: taskID})
	}
	// We don't always append workspace here automatically to keep JSON stable,
	// but the workspaceID has its own column in the execution_outcomes table.
	if len(entities) == 0 {
		return "[]"
	}

	b, err := json.Marshal(entities)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// ClassifyFailure maps an error to a FailureClass. Initial implementation treats all execution errors uniformly.
func ClassifyFailure(err error) schema.FailureClass {
	if err == nil {
		return ""
	}

	var classified interface{ FailureClass() schema.FailureClass }
	if errors.As(err, &classified) {
		if failureClass := classified.FailureClass(); failureClass != "" {
			return failureClass
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return schema.FailureClassTimeout
	case errors.Is(err, context.Canceled):
		return schema.FailureClassPermissionDenial
	}

	if providerErr := llm.ClassifyError(err, "", "", 0); providerErr != nil {
		switch providerErr.Reason {
		case llm.ReasonAuth, llm.ReasonBilling:
			return schema.FailureClassPermissionDenial
		case llm.ReasonTimeout, llm.ReasonRateLimit, llm.ReasonOverloaded:
			return schema.FailureClassTimeout
		case llm.ReasonFormat:
			return schema.FailureClassSchemaInvalid
		}
	}

	lowered := strings.ToLower(err.Error())
	switch {
	case containsFailurePhrase(lowered, "connector unavailable", "service unavailable", "connection refused", "dial tcp", "no such host", "temporarily unavailable"):
		return schema.FailureClassConnectorUnavailable
	case containsFailurePhrase(lowered, "policy rejected", "governance blocked", "control boundary blocked", "outside workspace scope", "protected path", "explicitly denied"):
		return schema.FailureClassPolicyBlocked
	case containsFailurePhrase(lowered, "invalid schema", "schema mismatch", "schema invalid", "invalid request format", "cannot unmarshal", "cannot decode", "validation failed"):
		return schema.FailureClassSchemaInvalid
	case containsFailurePhrase(lowered, "timeout", "timed out", "deadline exceeded"):
		return schema.FailureClassTimeout
	}
	return schema.FailureClassExecutionFailure
}

func containsFailurePhrase(value string, phrases ...string) bool {
	for _, phrase := range phrases {
		phrase = strings.TrimSpace(strings.ToLower(phrase))
		if phrase == "" {
			continue
		}
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

// IsRetryable decides whether a failure is retryable based on command type and failure class.
func IsRetryable(cmdType schema.CommandType, failureClass schema.FailureClass) bool {
	if failureClass == "" {
		return false
	}
	switch failureClass {
	case schema.FailureClassConnectorUnavailable, schema.FailureClassTimeout:
		return true
	}
	switch cmdType.Idempotency() {
	case schema.IdempotencyIdempotent, schema.IdempotencyVersionAware:
		return true
	default:
		return false
	}
}
