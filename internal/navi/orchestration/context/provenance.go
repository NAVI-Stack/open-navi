package context

import (
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

func newContextItem(
	id string,
	kind string,
	label string,
	content string,
	source orchestration.ContextSource,
	sourceRef string,
	trust orchestration.TrustLabel,
	class orchestration.ContextClass,
	confidence float64,
	attributes map[string]string,
) orchestration.ContextItem {
	return orchestration.ContextItem{
		ID:           strings.TrimSpace(id),
		Kind:         strings.TrimSpace(kind),
		Label:        strings.TrimSpace(label),
		Content:      strings.TrimSpace(content),
		Source:       source,
		SourceRef:    strings.TrimSpace(sourceRef),
		Trust:        trust,
		ContextClass: class,
		Confidence:   clampConfidence(confidence),
		Attributes:   cloneAttributes(attributes),
	}
}

func proposalContextItem(input ProposalInput) (orchestration.ContextItem, bool) {
	summary := strings.TrimSpace(input.Summary)
	proposedAction := strings.TrimSpace(input.ProposedAction)
	if summary == "" && proposedAction == "" && strings.TrimSpace(input.ProposalID) == "" {
		return orchestration.ContextItem{}, false
	}
	content := summary
	if content == "" {
		content = proposedAction
	} else if proposedAction != "" {
		content = content + "\nproposed_action: " + proposedAction
	}
	if status := strings.TrimSpace(input.Status); status != "" {
		content = fmt.Sprintf("%s\nstatus: %s", content, status)
	}
	return newContextItem(
		input.ProposalID,
		"proposal",
		"open proposal",
		content,
		orchestration.ContextSourceGovernance,
		sourceRefWithDefault(input.SourceRef, input.ProposalID),
		orchestration.TrustLabelAuthoritative,
		orchestration.ContextClassProposal,
		defaultConfidence(input.Confidence, 1.0),
		map[string]string{
			"proposal_id":     strings.TrimSpace(input.ProposalID),
			"status":          strings.TrimSpace(input.Status),
			"proposed_action": strings.TrimSpace(input.ProposedAction),
		},
	), true
}

func factsContextItem(input FactsInput) (orchestration.ContextItem, bool) {
	content := strings.TrimSpace(input.Block)
	if content == "" {
		return orchestration.ContextItem{}, false
	}
	return newContextItem(
		"facts",
		"facts_block",
		"facts block",
		content,
		orchestration.ContextSourceFacts,
		sourceRefWithDefault(input.SourceRef, "facts"),
		orchestration.TrustLabelDerived,
		orchestration.ContextClassFacts,
		defaultConfidence(input.Confidence, 0.85),
		nil,
	), true
}

func runtimeStateContextItem(input RuntimeStateInput) (orchestration.ContextItem, bool) {
	content := strings.TrimSpace(input.Block)
	if content == "" {
		return orchestration.ContextItem{}, false
	}
	class := orchestration.ContextClassRuntimeState
	label := "runtime state"
	kind := "runtime_state"
	if strings.Contains(strings.ToLower(content), "scratchpad") {
		class = orchestration.ContextClassScratchpad
		label = "run scratchpad"
		kind = "run_scratchpad"
	}
	return newContextItem(
		kind,
		kind,
		label,
		content,
		orchestration.ContextSourceRuntime,
		sourceRefWithDefault(input.SourceRef, kind),
		orchestration.TrustLabelAuthoritative,
		class,
		1.0,
		nil,
	), true
}

func capabilityContextItem(input CapabilitySnapshotInput) (orchestration.ContextItem, bool) {
	lines := make([]string, 0, len(input.ToolNames)+2)
	if surface := strings.TrimSpace(input.Surface); surface != "" {
		lines = append(lines, "surface: "+surface)
	}
	if len(input.ToolNames) > 0 {
		lines = append(lines, "tools: "+strings.Join(input.ToolNames, ", "))
	}
	if reason := strings.TrimSpace(input.SelectionReason); reason != "" {
		lines = append(lines, "selection_reason: "+reason)
	}
	content := strings.Join(lines, "\n")
	if content == "" {
		return orchestration.ContextItem{}, false
	}
	return newContextItem(
		"capability_surface",
		"capability_surface",
		"capability surface",
		content,
		orchestration.ContextSourceCapability,
		sourceRefWithDefault(input.SourceRef, input.Surface),
		orchestration.TrustLabelAuthoritative,
		orchestration.ContextClassCapabilitySurface,
		1.0,
		map[string]string{
			"surface":          strings.TrimSpace(input.Surface),
			"selection_reason": strings.TrimSpace(input.SelectionReason),
		},
	), true
}

func cloneAttributes(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		dst[key] = value
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func defaultConfidence(value, fallback float64) float64 {
	if value > 0 {
		return clampConfidence(value)
	}
	return clampConfidence(fallback)
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
