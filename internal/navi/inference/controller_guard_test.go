package inference

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

// synthesisThatAllowsCapabilityExecution returns a DecisionSynthesis whose
// ExecutionIntent makes AllowsCapabilityExecution() return true, modelling a
// tool-backed request where the routing layer has already committed to tool use.
func synthesisThatAllowsCapabilityExecution(capability string) DecisionSynthesis {
	return DecisionSynthesis{
		Rationale: Rationale{
			ExecutionIntent: ExecutionIntent{
				ActionType:          "act",
				TargetCapability:    capability,
				AllowedCapabilities: []string{capability},
			},
			Governance: GovernanceHandoff{
				TargetCapability:    capability,
				AllowedCapabilities: []string{capability},
			},
		},
	}
}

func TestValidateGovernedDecision_BlocksOnEmptySurfaceGuard(t *testing.T) {
	controller := NewController()
	input := InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-empty-surface",
			UserMessage: "run the tool-backed action",
		},
		Capabilities: []CapabilityAvailability{{Name: "my_tool", Available: true}},
		Extensions: map[string]any{
			"surface_guard_reason": `no tools resolved for surface "runtime"`,
		},
	}

	synthesis := synthesisThatAllowsCapabilityExecution("my_tool")
	decision, err := controller.ValidateGovernedDecision(context.Background(), input, synthesis)
	if err != nil {
		t.Fatalf("ValidateGovernedDecision: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %q", decision.RuntimeDisposition)
	}
	if !strings.Contains(decision.ReplyMessage, "widening tool access") {
		t.Fatalf("expected explicit no-widening reply, got %q", decision.ReplyMessage)
	}
}

func TestValidateGovernedDecision_ChatOnlyRequestPassesThroughGuard(t *testing.T) {
	// Regression: surface guards must not block CHAT-only requests that never
	// needed tool execution. AllowsCapabilityExecution() is false for such
	// requests, so finalizeDecision should skip the guard entirely.
	controller := NewController()
	input := InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-chat-only",
			UserMessage: "show me a python class to poll system status",
		},
		// No Capabilities → no tool surface. Guard is set but should be skipped.
		Extensions: map[string]any{
			"surface_guard_reason": `no tools resolved for surface "runtime"`,
		},
	}

	synthesis := DecisionSynthesis{
		Rationale: Rationale{
			ExecutionIntent: ExecutionIntent{
				// "respond" → AllowsCapabilityExecution() = false
				ActionType: "respond",
			},
		},
	}
	decision, err := controller.ValidateGovernedDecision(context.Background(), input, synthesis)
	if err != nil {
		t.Fatalf("ValidateGovernedDecision: %v", err)
	}
	if decision.RuntimeDisposition == RuntimeDispositionBlockWithReply {
		t.Fatalf("CHAT-only request should not be governance-blocked, got %q (reply: %q)",
			decision.RuntimeDisposition, decision.ReplyMessage)
	}
}

func TestValidateGovernedDecision_BlocksOnToolCapableModelGuard(t *testing.T) {
	controller := NewController()
	input := InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-tool-model-guard",
			UserMessage: "run the tool-backed action",
		},
		Capabilities: []CapabilityAvailability{{Name: "my_tool", Available: true}},
		NCOS: orchestration.CanonicalRunRequest{
			Model: orchestration.ModelProfile{
				Metadata: map[string]string{
					"routing_guard": string(orchestration.SurfaceGuardToolCapableModelRequired),
				},
			},
		},
		Extensions: map[string]any{
			"surface_guard_reason": "resolved surface requires a tool-capable model",
		},
	}

	synthesis := synthesisThatAllowsCapabilityExecution("my_tool")
	decision, err := controller.ValidateGovernedDecision(context.Background(), input, synthesis)
	if err != nil {
		t.Fatalf("ValidateGovernedDecision: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %q", decision.RuntimeDisposition)
	}
	if !strings.Contains(decision.ReplyMessage, "selected model cannot call tools") {
		t.Fatalf("expected explicit tool-capable-model reply, got %q", decision.ReplyMessage)
	}
}
