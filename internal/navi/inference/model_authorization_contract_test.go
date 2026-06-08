package inference

import (
	"context"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
)

func TestDefaultDecisionEnvelope_DoesNotForceToolChoiceForAuthorizedCapability(t *testing.T) {
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))

	if strings.TrimSpace(prior.ExecutionBoundary.ToolChoice) != "" {
		t.Fatalf("authorized capability should not force tool choice, got %q", prior.ExecutionBoundary.ToolChoice)
	}
}

func TestAuthorizeModelResponse_AcceptsNaturalReplyWhenToolsAreAuthorized(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{{Name: "navi.runtime.echo"}},
		},
		Response: orchestration.NormalizedModelResponse{
			Content: "normal assistant reply",
		},
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition == RuntimeDispositionBlockWithReply {
		t.Fatalf("natural reply should not be blocked when tools are only authorized, got %q", decision.ReplyMessage)
	}
	if strings.TrimSpace(decision.ReplyMessage) != "" {
		t.Fatalf("expected no internal reply message, got %q", decision.ReplyMessage)
	}
	if strings.TrimSpace(decision.ExecutionBoundary.FailClosedReason) != "" {
		t.Fatalf("expected no fail-closed reason for natural reply, got %q", decision.ExecutionBoundary.FailClosedReason)
	}
	if len(decision.AuthorizedToolCalls) != 0 {
		t.Fatalf("expected no authorized tool calls for natural reply, got %#v", decision.AuthorizedToolCalls)
	}
}
