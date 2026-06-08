package tool

import (
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// ToolCallRecoveryDisposition classifies the bounded repair path for a tool call
// that falls outside the currently active tool surface.
type ToolCallRecoveryDisposition string

const (
	ToolCallRecoveryDispositionNone         ToolCallRecoveryDisposition = ""
	ToolCallRecoveryDispositionUnknownTool  ToolCallRecoveryDisposition = "unknown_tool"
	ToolCallRecoveryDispositionUnloadedTool ToolCallRecoveryDisposition = "unloaded_tool"
)

// ToolCallRecoveryInput is the normalized runtime/inference handoff for
// controlled unknown-tool recovery.
type ToolCallRecoveryInput struct {
	Registry                *Registry
	ToolCall                llm.ToolCall
	ActiveToolIDs           []string
	BrokerInput             BrokerInput
	RepairAttemptsRemaining int
}

// ToolCallRecoveryResult captures the structured result of one recovery
// decision without implying authority to execute or expose a tool directly.
type ToolCallRecoveryResult struct {
	SnapshotID                string                      `json:"snapshot_id,omitempty"`
	ToolCallID                string                      `json:"tool_call_id,omitempty"`
	ToolName                  string                      `json:"tool_name,omitempty"`
	Disposition               ToolCallRecoveryDisposition `json:"disposition,omitempty"`
	AvailabilityState         ToolAvailabilityState       `json:"availability_state,omitempty"`
	Hallucinated              bool                        `json:"hallucinated,omitempty"`
	ExistingToolID            string                      `json:"existing_tool_id,omitempty"`
	RelatedToolIDs            []string                    `json:"related_tool_ids,omitempty"`
	LoadAllowed               bool                        `json:"load_allowed,omitempty"`
	CachedDecisionInvalidated bool                        `json:"cached_decision_invalidated,omitempty"`
	CanRetry                  bool                        `json:"can_retry,omitempty"`
	SafeReason                string                      `json:"safe_reason,omitempty"`
	RepairPrompt              string                      `json:"repair_prompt,omitempty"`
	Discovery                 *DiscoveryResult            `json:"discovery,omitempty"`
	Broker                    *BrokerResolution           `json:"broker,omitempty"`
}

// RecoverToolCall classifies an out-of-surface tool call and produces a
// bounded repair contract without executing or directly exposing the tool.
func RecoverToolCall(input ToolCallRecoveryInput) ToolCallRecoveryResult {
	result := ToolCallRecoveryResult{
		ToolCallID: strings.TrimSpace(input.ToolCall.ID),
		ToolName:   strings.TrimSpace(input.ToolCall.Name),
		CanRetry:   input.RepairAttemptsRemaining > 0,
	}
	if result.ToolName == "" {
		result.Disposition = ToolCallRecoveryDispositionUnknownTool
		result.Hallucinated = true
		result.SafeReason = "the requested action referenced a tool that is not available in this runtime"
		result.RepairPrompt = "Tool unavailable: use only the currently surfaced tools for this turn, or reply directly to the user."
		return result
	}

	brokerInput := input.BrokerInput.normalize()
	if brokerInput.UserInput == "" {
		brokerInput.UserInput = result.ToolName
	}
	if len(brokerInput.ActiveToolIDs) == 0 {
		brokerInput.ActiveToolIDs = dedupeStrings(input.ActiveToolIDs)
	}

	if input.Registry == nil {
		result.Disposition = ToolCallRecoveryDispositionUnknownTool
		result.Hallucinated = true
		result.SafeReason = "the requested action referenced a tool that is not available in this runtime"
		result.RepairPrompt = "Tool unavailable: use only the currently surfaced tools for this turn, or reply directly to the user."
		return result
	}
	exact, exactOK := input.Registry.LookupExact(result.ToolName)

	refresh := NewToolBrokerFromRegistry(input.Registry).RefreshCapability(CapabilityRefreshInput{
		QueryText:   result.ToolName,
		BrokerInput: brokerInput,
	})
	result.SnapshotID = refresh.SnapshotID
	result.AvailabilityState = refresh.AvailabilityState
	result.CachedDecisionInvalidated = refresh.CachedDecisionInvalidated
	if refresh.Discovery != nil {
		result.RelatedToolIDs = relatedToolIDsFromDiscovery(*refresh.Discovery)
		discoveryCopy := *refresh.Discovery
		result.Discovery = &discoveryCopy
	}
	if refresh.Broker != nil {
		brokerCopy := *refresh.Broker
		result.Broker = &brokerCopy
	}
	if !exactOK || exact.Tool == nil {
		result.Disposition = ToolCallRecoveryDispositionUnknownTool
		result.Hallucinated = true
		result.SafeReason = "the requested action referenced a tool that is not available in this runtime"
		result.RepairPrompt = "Tool unavailable: use only the currently surfaced tools for this turn, or reply directly to the user."
		if len(result.RelatedToolIDs) > 0 {
			result.RepairPrompt += " Related registered tools: " + strings.Join(result.RelatedToolIDs, ", ") + "."
		}
		return result
	}
	result.ExistingToolID = exact.Tool.ToolID

	switch refresh.AvailabilityState {
	case ToolAvailabilityNotRegistered:
		result.Disposition = ToolCallRecoveryDispositionUnknownTool
		result.Hallucinated = true
		result.SafeReason = "the requested action referenced a tool that is not available in this runtime"
		result.RepairPrompt = "Tool unavailable: use only the currently surfaced tools for this turn, or reply directly to the user."
		if len(result.RelatedToolIDs) > 0 {
			result.RepairPrompt += " Related registered tools: " + strings.Join(result.RelatedToolIDs, ", ") + "."
		}
		return result
	}

	result.Disposition = ToolCallRecoveryDispositionUnloadedTool
	if containsToolID(brokerInput.ActiveToolIDs, result.ExistingToolID) {
		return result
	}
	result.LoadAllowed = refresh.ShouldLoad
	result.SafeReason = "the requested action referenced a tool that is outside the current active tool surface"
	result.RepairPrompt = "Tool unavailable: do not call tools outside the current active tool surface. Use only the surfaced tools for this turn, or reply directly to the user."
	if result.LoadAllowed {
		result.SafeReason = "the requested action referenced a real tool that is not loaded in the current active tool surface"
		result.RepairPrompt = "Tool not loaded: this tool exists but is not loaded in the current active tool surface. Do not call it directly; use the surfaced tools for this turn, or reply directly to the user."
	}
	if refresh.AvailabilityState == ToolAvailabilityLoadedNotExposed {
		result.SafeReason = "the requested action referenced a real tool that is loaded but not currently exposed"
		result.RepairPrompt = "Tool not exposed: this tool is already loaded but not exposed for this provider call. Do not call it directly; use the surfaced tools for this turn, or reply directly to the user."
	}
	if refresh.AvailabilityState == ToolAvailabilityExposedGovernanceBlocked {
		result.SafeReason = "the requested action referenced a tool that is exposed but blocked by governance"
		result.RepairPrompt = "Tool blocked: this tool is present but governance does not currently permit execution. Reply directly to the user or choose a different surfaced tool."
	}
	return result
}

func relatedToolIDsFromDiscovery(result DiscoveryResult) []string {
	if len(result.RelatedMatches) == 0 {
		return nil
	}
	out := make([]string, 0, len(result.RelatedMatches))
	for _, candidate := range result.RelatedMatches {
		if candidate.Tool == nil || strings.TrimSpace(candidate.Tool.ToolID) == "" {
			continue
		}
		out = append(out, candidate.Tool.ToolID)
	}
	return dedupeStrings(out)
}

func containsToolID(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
