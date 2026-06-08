package navi

import (
	"context"
	"strings"

	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/orchestration"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
)

// AuthorizeWorkerTask routes worker execution through the same ICS authority seam used by runs.
func (n *NAVI) AuthorizeWorkerTask(ctx context.Context, task schema.Task, actorKind string, cmdType schema.CommandType, domain string) (inference.DecisionEnvelope, error) {
	if n == nil || n.loop == nil {
		return inference.DecisionEnvelope{}, nil
	}
	controller := n.loop.inferenceController()

	goalSummary := firstNonEmpty(strings.TrimSpace(task.Description), strings.TrimSpace(task.Title), "worker task")
	chatID := firstNonEmpty(strings.TrimSpace(task.DirectiveID), strings.TrimSpace(task.ID))
	capabilityName := firstNonEmpty(strings.TrimSpace(actorKind), string(task.AssignedTo), "worker-task")

	input := inference.InferenceInput{
		Version: inference.ContractVersionV1,
		NCOS: orchestration.CanonicalRunRequest{
			UserMessage:    goalSummary,
			ExperienceMode: string(ExperienceModeStandard),
			RequiredOutput: orchestration.RequiredOutput{
				AllowToolCalls: true,
			},
			CapabilitySurface: orchestration.CapabilitySurface{
				ToolNames:       []string{capabilityName},
				SelectionReason: "worker authorization surface",
			},
		},
		GoalStack: inference.GoalStack{
			ActiveGoalID: task.ID,
			Ready: []inference.GoalRef{{
				GoalID:      task.ID,
				Summary:     goalSummary,
				Status:      inference.GoalStatusActive,
				Priority:    1.0,
				Preemptible: false,
			}},
		},
		Governance: inference.GovernanceState{
			RiskHint: task.Risk,
		},
		Chat: inference.ChatContext{
			ChatID:      chatID,
			DirectiveID: task.DirectiveID,
			UserMessage: goalSummary,
			CurrentTask: &task,
		},
		Runtime: inference.RuntimeContext{
			RunID:        firstNonEmpty(strings.TrimSpace(task.ID), strings.TrimSpace(task.DirectiveID)),
			RunStatus:    schema.RunStatusActive,
			CurrentPhase: naviruntime.RunPhaseValidateGovern,
		},
		Capabilities: []inference.CapabilityAvailability{{
			Name:          capabilityName,
			Kind:          strings.TrimSpace(domain),
			Available:     true,
			Governed:      true,
			CommandType:   cmdType,
			RiskHint:      task.Risk,
			Reversibility: schema.ReversibilityInternal,
			Reason:        "worker execution authorization",
		}},
	}

	synthesis, err := controller.Decide(ctx, input)
	if err != nil {
		return inference.DecisionEnvelope{}, err
	}
	return controller.ValidateGovernedDecision(ctx, input, synthesis)
}
