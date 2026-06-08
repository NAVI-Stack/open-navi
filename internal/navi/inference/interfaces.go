package inference

import (
	"context"
	"time"
)

// DecisionController is the canonical ICS entrypoint for one governed decision cycle.
type DecisionController interface {
	Decide(ctx context.Context, input InferenceInput) (DecisionSynthesis, error)
	ValidateGovernedDecision(ctx context.Context, input InferenceInput, synthesis DecisionSynthesis) (DecisionEnvelope, error)
	ObserveOutcome(ctx context.Context, prior DecisionEnvelope, snapshot ExecutionSnapshot) (DecisionEnvelope, error)
	PrepareModelCall(ctx context.Context, prior DecisionEnvelope, input ModelCallPreparationInput) (DecisionEnvelope, error)
	AuthorizeModelResponse(ctx context.Context, prior DecisionEnvelope, input ModelResponseAuthorizationInput) (DecisionEnvelope, error)
	AuthorizeToolCall(ctx context.Context, prior DecisionEnvelope, input ToolAuthorizationInput) (DecisionEnvelope, error)
}

// FocusArbiter selects the foreground focus for one inference cycle.
type FocusArbiter interface {
	Select(input InferenceInput, now time.Time) FocusSelection
}

// ModeRouter selects one dominant mode plus a bounded submode chain.
type ModeRouter interface {
	Route(input InferenceInput, focus FocusFrame) ModeSelection
}

// CandidateEvaluator generates, scores, and selects the candidate set for one cycle.
type CandidateEvaluator interface {
	Evaluate(input InferenceInput, focus FocusFrame, mode ModeSelection) EvaluationResult
}

// RecoveryManager builds structured recovery checkpoints and routes.
type RecoveryManager interface {
	Build(input InferenceInput, focus FocusFrame, candidate CandidateSummary, approvals []ApprovalRef) RecoveryCheckpoint
}

// PlanGraphManager builds or updates the explicit plan graph for one cycle.
type PlanGraphManager interface {
	Build(input InferenceInput, focus FocusFrame, mode ModeSelection, candidate CandidateSummary) *PlanGraph
}
