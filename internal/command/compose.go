package command

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

// Step is a single step in a Compose sequence. Reversibility determines
// how the Compose runner handles failure of this or later steps.
type Step struct {
	CommandType   schema.CommandType
	Reversibility schema.ReversibilityClass
	Run           func(ctx context.Context) (any, error)
}

// ComposeResult holds the composite outcome and the index of the step that failed (if any).
type ComposeResult struct {
	Outcome        schema.ExecutionOutcomeOutcome
	FailureIndex   int
	FailureReason  string
	StepOutcomes   []schema.ExecutionOutcome
	RecoveryStatus schema.RecoveryStatus
}

// RunCompose runs steps in order. On the first step failure it applies mode and returns.
// It persists a composite execution outcome and per-step outcomes via saver.
func RunCompose(ctx context.Context, desc Descriptor, steps []Step, mode schema.ComposeFailureMode, saver Saver) (result any, composeResult *ComposeResult, err error) {
	if len(steps) == 0 {
		return nil, &ComposeResult{Outcome: schema.ExecutionOutcomeSucceeded}, nil
	}

	commandID := desc.CommandID
	if commandID == "" {
		commandID = uuid.New().String()
	}
	compositeID := commandID + ":compose"
	startTime := time.Now().UTC()
	stepOutcomes := make([]schema.ExecutionOutcome, 0, len(steps))
	var lastResult any

	for i, step := range steps {
		stepID := compositeID + ":step:" + fmt.Sprintf("%d", i)
		stepStart := time.Now().UTC()
		res, stepErr := step.Run(ctx)
		stepEnd := time.Now().UTC()
		lastResult = res

		outcome := schema.ExecutionOutcomeSucceeded
		failureClass := schema.FailureClass("")
		failureReason := ""
		compensationRequired := false
		recoveryStatus := schema.RecoveryStatusNotRequired

		if stepErr != nil {
			outcome = schema.ExecutionOutcomeFailed
			failureClass = ClassifyFailure(stepErr)
			failureReason = stepErr.Error()
			if step.Reversibility == schema.ReversibilityCompensable {
				compensationRequired = true
			}
		}

		eo := schema.ExecutionOutcome{
			AttemptID:            stepID + ":1",
			CommandID:            stepID,
			AttemptNumber:        1,
			CommandType:          step.CommandType,
			StartTime:            stepStart,
			EndTime:              &stepEnd,
			Outcome:              outcome,
			FailureClass:         failureClass,
			FailureReason:        failureReason,
			AffectedEntities:     encodeAffectedEntities(desc.RuntimeSessionID, desc.TaskID),
			Retryable:            IsRetryable(step.CommandType, failureClass),
			CompensationRequired: compensationRequired,
			CompensationStatus:   schema.CompensationStatusNotRequired,
			RecoveryStatus:       recoveryStatus,
		}
		if compensationRequired {
			eo.CompensationStatus = schema.CompensationStatusPending
		}
		stepOutcomes = append(stepOutcomes, eo)
		if saver != nil {
			_ = saver(ctx, eo)
		}

		if stepErr != nil {
			endTime := time.Now().UTC()
			compositeOutcome := schema.ExecutionOutcomeFailed
			compositeRecovery := schema.RecoveryStatusNotRequired
			switch mode {
			case schema.ComposeFailureSurfacePartial:
				compositeOutcome = schema.ExecutionOutcomePartiallySucceeded
				compositeRecovery = schema.RecoveryStatusOpen
			case schema.ComposeFailureCompensation:
				anyCompensable := false
				for j := 0; j < i; j++ {
					if steps[j].Reversibility == schema.ReversibilityCompensable {
						anyCompensable = true
						break
					}
				}
				if anyCompensable {
					compositeRecovery = schema.RecoveryStatusOpen
				}
			}
			compositeEO := schema.ExecutionOutcome{
				AttemptID:            compositeID + ":1",
				CommandID:            compositeID,
				AttemptNumber:        1,
				CommandType:          schema.CommandTypeCompose,
				StartTime:            startTime,
				EndTime:              &endTime,
				Outcome:              compositeOutcome,
				FailureClass:         failureClass,
				FailureReason:        failureReason,
				AffectedEntities:     encodeAffectedEntities(desc.RuntimeSessionID, desc.TaskID),
				Retryable:            false,
				CompensationRequired: compensationRequired,
				CompensationStatus:   schema.CompensationStatusNotRequired,
				RecoveryStatus:       compositeRecovery,
			}
			if compensationRequired {
				compositeEO.CompensationStatus = schema.CompensationStatusPending
			}
			if saver != nil {
				_ = saver(ctx, compositeEO)
			}
			return lastResult, &ComposeResult{
				Outcome:        compositeOutcome,
				FailureIndex:   i,
				FailureReason:  failureReason,
				StepOutcomes:   stepOutcomes,
				RecoveryStatus: compositeRecovery,
			}, stepErr
		}
	}

	endTime := time.Now().UTC()
	compositeEO := schema.ExecutionOutcome{
		AttemptID:          compositeID + ":1",
		CommandID:          compositeID,
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeCompose,
		StartTime:          startTime,
		EndTime:            &endTime,
		Outcome:            schema.ExecutionOutcomeSucceeded,
		AffectedEntities:   encodeAffectedEntities(desc.RuntimeSessionID, desc.TaskID),
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		CompensationStatus: schema.CompensationStatusNotRequired,
	}
	if saver != nil {
		_ = saver(ctx, compositeEO)
	}
	return lastResult, &ComposeResult{
		Outcome:      schema.ExecutionOutcomeSucceeded,
		StepOutcomes: stepOutcomes,
	}, nil
}
