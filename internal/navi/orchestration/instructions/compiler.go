package instructions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

func (c *Compiler) Compile(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack) (orchestration.InstructionStack, error) {
	input := defaultCompileRequest(req)
	if c != nil && c.Resolver != nil {
		resolved, err := c.Resolver.Resolve(ctx, req, pack)
		if err != nil {
			return orchestration.InstructionStack{}, err
		}
		input = mergeCompileRequest(input, resolved)
	}
	return CompileLayers(input), nil
}

func CompileLayers(input CompileRequest) orchestration.InstructionStack {
	return orchestration.InstructionStack{
		SystemCore:         buildSystemCoreLayer(input.SystemCore),
		RuntimeConstraints: buildRuntimeConstraintsLayer(input.RuntimeConstraints, input.ExecutionFrame),
		ExperienceOverlay:  buildExperienceOverlayLayer(input.ExperienceOverlay),
		TaskFraming:        buildTaskFramingLayer(input.TaskFrame, input.OutputContract),
		CapabilitySurface:  buildCapabilitySurfaceLayer(input.CapabilitySurface),
	}
}

func defaultCompileRequest(req orchestration.CanonicalRunRequest) CompileRequest {
	return CompileRequest{
		TaskFrame: TaskFrameInput{
			Objective: req.UserMessage,
		},
		OutputContract: OutputContractInput{
			Present:               req.RequiredOutput.MustReply || req.RequiredOutput.AllowToolCalls || req.RequiredOutput.AllowScheduledReplies || req.RequiredOutput.ExpectStreaming,
			MustReply:             req.RequiredOutput.MustReply,
			AllowToolCalls:        req.RequiredOutput.AllowToolCalls,
			AllowScheduledReplies: req.RequiredOutput.AllowScheduledReplies,
			ExpectStreaming:       req.RequiredOutput.ExpectStreaming,
		},
		CapabilitySurface: CapabilitySurfaceInput{
			Surface:         req.CapabilitySurface.Surface,
			ToolNames:       append([]string(nil), req.CapabilitySurface.ToolNames...),
			SelectionReason: req.CapabilitySurface.SelectionReason,
		},
		ExecutionFrame: ExecutionFrameInput{
			Frame: cloneFrame(req.Frame),
		},
	}
}

func mergeCompileRequest(base, override CompileRequest) CompileRequest {
	base.SystemCore = mergeSystemCore(base.SystemCore, override.SystemCore)
	base.RuntimeConstraints = mergeRuntimeConstraints(base.RuntimeConstraints, override.RuntimeConstraints)
	base.ExperienceOverlay = mergeExperienceOverlay(base.ExperienceOverlay, override.ExperienceOverlay)
	base.TaskFrame = mergeTaskFrame(base.TaskFrame, override.TaskFrame)
	base.OutputContract = mergeOutputContract(base.OutputContract, override.OutputContract)
	base.CapabilitySurface = mergeCapabilitySurface(base.CapabilitySurface, override.CapabilitySurface)
	base.ExecutionFrame = mergeExecutionFrame(base.ExecutionFrame, override.ExecutionFrame)
	return base
}

func mergeSystemCore(base, override SystemCoreInput) SystemCoreInput {
	base.IdentityRules = mergeStringLists(base.IdentityRules, override.IdentityRules)
	base.ToolUseRules = mergeStringLists(base.ToolUseRules, override.ToolUseRules)
	base.ChatBehavior = mergeStringLists(base.ChatBehavior, override.ChatBehavior)
	base.DiagnosticsRules = mergeStringLists(base.DiagnosticsRules, override.DiagnosticsRules)
	base.SecurityRules = mergeStringLists(base.SecurityRules, override.SecurityRules)
	base.ProviderStateRules = mergeStringLists(base.ProviderStateRules, override.ProviderStateRules)
	base.ModelStyleRules = mergeStringLists(base.ModelStyleRules, override.ModelStyleRules)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	base.TimeContext = firstNonEmpty(override.TimeContext, base.TimeContext)
	base.SessionContext = firstNonEmpty(override.SessionContext, base.SessionContext)
	return base
}

func mergeRuntimeConstraints(base, override RuntimeConstraintsInput) RuntimeConstraintsInput {
	base.GovernanceNotes = mergeStringLists(base.GovernanceNotes, override.GovernanceNotes)
	base.AuthoritativeWarnings = mergeStringLists(base.AuthoritativeWarnings, override.AuthoritativeWarnings)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func mergeExperienceOverlay(base, override ExperienceOverlayInput) ExperienceOverlayInput {
	base.Fragment = firstNonEmpty(override.Fragment, base.Fragment)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func mergeTaskFrame(base, override TaskFrameInput) TaskFrameInput {
	base.Objective = firstNonEmpty(override.Objective, base.Objective)
	base.Instructions = mergeStringLists(base.Instructions, override.Instructions)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func mergeOutputContract(base, override OutputContractInput) OutputContractInput {
	if override.Present {
		base.Present = true
		base.MustReply = override.MustReply
		base.AllowToolCalls = override.AllowToolCalls
		base.AllowScheduledReplies = override.AllowScheduledReplies
		base.ExpectStreaming = override.ExpectStreaming
	} else {
		base.MustReply = base.MustReply || override.MustReply
		base.AllowToolCalls = base.AllowToolCalls || override.AllowToolCalls
		base.AllowScheduledReplies = base.AllowScheduledReplies || override.AllowScheduledReplies
		base.ExpectStreaming = base.ExpectStreaming || override.ExpectStreaming
	}
	base.Present = base.Present || override.Present
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func mergeCapabilitySurface(base, override CapabilitySurfaceInput) CapabilitySurfaceInput {
	base.Surface = firstNonEmpty(override.Surface, base.Surface)
	if len(override.ToolNames) > 0 {
		base.ToolNames = append([]string(nil), override.ToolNames...)
	}
	base.SelectionReason = firstNonEmpty(override.SelectionReason, base.SelectionReason)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func mergeExecutionFrame(base, override ExecutionFrameInput) ExecutionFrameInput {
	base.Frame = mergeFrame(base.Frame, override.Frame)
	base.Additional = mergeStringLists(base.Additional, override.Additional)
	return base
}

func buildSystemCoreLayer(input SystemCoreInput) orchestration.InstructionLayer {
	fragments := make([]orchestration.InstructionFragment, 0, 8)
	if fragment := newFragment("identity_and_tool_use", flattenSections(
		input.IdentityRules,
		input.ToolUseRules,
		optionalLine(input.TimeContext),
		optionalLine(input.SessionContext),
	)); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("chat_behavior", input.ChatBehavior); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("runtime_diagnostics", input.DiagnosticsRules); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("security_rules", input.SecurityRules); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("provider_state_grounding", input.ProviderStateRules); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("model_style_rules", input.ModelStyleRules); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := newFragment("system_core_additional", input.Additional); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	return orchestration.InstructionLayer{
		Key:       orchestration.InstructionLayerSystemCore,
		Fragments: fragments,
	}
}

func buildRuntimeConstraintsLayer(input RuntimeConstraintsInput, frame ExecutionFrameInput) orchestration.InstructionLayer {
	fragments := make([]orchestration.InstructionFragment, 0, 4)
	if fragment := newFragment("runtime_constraints", flattenSections(
		input.GovernanceNotes,
		input.AuthoritativeWarnings,
		input.Additional,
	)); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := buildExecutionFrameFragment(frame); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	return orchestration.InstructionLayer{
		Key:       orchestration.InstructionLayerRuntimeConstraints,
		Fragments: fragments,
	}
}

func buildExperienceOverlayLayer(input ExperienceOverlayInput) orchestration.InstructionLayer {
	fragments := make([]orchestration.InstructionFragment, 0, 2)
	if fragment := newFragment("experience_overlay", flattenSections(
		optionalLine(input.Fragment),
		input.Additional,
	)); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	return orchestration.InstructionLayer{
		Key:       orchestration.InstructionLayerExperienceOverlay,
		Fragments: fragments,
	}
}

func buildTaskFramingLayer(task TaskFrameInput, contract OutputContractInput) orchestration.InstructionLayer {
	fragments := make([]orchestration.InstructionFragment, 0, 3)
	if fragment := buildTaskFrameFragment(task); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	if fragment := buildOutputContractFragment(contract); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	return orchestration.InstructionLayer{
		Key:       orchestration.InstructionLayerTaskFraming,
		Fragments: fragments,
	}
}

func buildCapabilitySurfaceLayer(input CapabilitySurfaceInput) orchestration.InstructionLayer {
	fragments := make([]orchestration.InstructionFragment, 0, 3)
	if fragment := newFragment("capability_surface", flattenSections(
		optionalLine(formatCapabilityMetadata(input)),
		input.Additional,
	)); fragment.Content != "" {
		fragments = append(fragments, fragment)
	}
	return orchestration.InstructionLayer{
		Key:       orchestration.InstructionLayerCapabilitySurface,
		Fragments: fragments,
	}
}

func buildTaskFrameFragment(input TaskFrameInput) orchestration.InstructionFragment {
	lines := make([]string, 0, len(input.Instructions)+len(input.Additional)+1)
	if objective := strings.TrimSpace(input.Objective); objective != "" {
		lines = append(lines, "objective: "+objective)
	}
	for _, instruction := range input.Instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		lines = append(lines, instruction)
	}
	lines = append(lines, cleanedLines(input.Additional)...)
	return newFragment("task_frame", lines)
}

func buildOutputContractFragment(input OutputContractInput) orchestration.InstructionFragment {
	lines := cleanedLines(input.Additional)
	if !input.Present && !input.MustReply && !input.AllowToolCalls && !input.AllowScheduledReplies && !input.ExpectStreaming && len(lines) == 0 {
		return orchestration.InstructionFragment{}
	}
	contract := []string{
		fmt.Sprintf("must_reply: %t", input.MustReply),
		fmt.Sprintf("allow_tool_calls: %t", input.AllowToolCalls),
		fmt.Sprintf("allow_scheduled_replies: %t", input.AllowScheduledReplies),
		fmt.Sprintf("expect_streaming: %t", input.ExpectStreaming),
	}
	lines = append(contract, lines...)
	return newFragment("output_contract", lines)
}

func buildExecutionFrameFragment(input ExecutionFrameInput) orchestration.InstructionFragment {
	frame := input.Frame
	scratchpad := sanitizeExecutionFrameScratchpad(frame.Scratchpad)
	lines := make([]string, 0, len(scratchpad)+len(input.Additional)+8)
	if value := strings.TrimSpace(frame.ChatID); value != "" {
		lines = append(lines, "chat_id: "+value)
	}
	if value := strings.TrimSpace(frame.RunID); value != "" {
		lines = append(lines, "run_id: "+value)
	}
	if value := strings.TrimSpace(frame.CheckpointID); value != "" {
		lines = append(lines, "checkpoint_id: "+value)
	}
	if value := strings.TrimSpace(frame.Phase); value != "" {
		lines = append(lines, "phase: "+value)
	}
	if value := strings.TrimSpace(string(frame.Mode)); value != "" {
		lines = append(lines, "mode: "+value)
	}
	if value := strings.TrimSpace(frame.ResumeProposalID); value != "" {
		lines = append(lines, "resume_proposal_id: "+value)
	}
	if value := strings.TrimSpace(frame.ResumeReason); value != "" {
		lines = append(lines, "resume_reason: "+value)
	}
	if len(scratchpad) > 0 {
		keys := make([]string, 0, len(scratchpad))
		for key, value := range scratchpad {
			if strings.TrimSpace(value) == "" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("scratchpad.%s: %s", key, scratchpad[key]))
		}
	}
	lines = append(lines, cleanedLines(input.Additional)...)
	return newFragment("execution_frame", lines)
}

func sanitizeExecutionFrameScratchpad(scratchpad map[string]string) map[string]string {
	if len(scratchpad) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(scratchpad))
	for key, value := range scratchpad {
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(key, "ics.") {
			continue
		}
		sanitized[key] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func formatCapabilityMetadata(input CapabilitySurfaceInput) string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(input.Surface); value != "" {
		parts = append(parts, "surface: "+value)
	}
	if len(input.ToolNames) > 0 {
		parts = append(parts, "tools: "+strings.Join(input.ToolNames, ", "))
	}
	if value := strings.TrimSpace(input.SelectionReason); value != "" {
		parts = append(parts, "selection_reason: "+value)
	}
	return strings.Join(parts, "\n")
}

func newFragment(key string, lines []string) orchestration.InstructionFragment {
	content := strings.Join(cleanedLines(lines), "\n")
	if content == "" {
		return orchestration.InstructionFragment{}
	}
	return orchestration.InstructionFragment{
		Key:      key,
		Content:  content,
		Required: true,
	}
}

func flattenSections(sections ...[]string) []string {
	var out []string
	for _, section := range sections {
		out = append(out, cleanedLines(section)...)
	}
	return out
}

func optionalLine(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	return []string{line}
}

func cleanedLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func mergeStringLists(base, override []string) []string {
	out := append([]string(nil), cleanedLines(base)...)
	out = append(out, cleanedLines(override)...)
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneFrame(frame orchestration.ExecutionFrame) orchestration.ExecutionFrame {
	frame.Scratchpad = cloneStringMap(frame.Scratchpad)
	return frame
}

func mergeFrame(base, override orchestration.ExecutionFrame) orchestration.ExecutionFrame {
	base.ChatID = firstNonEmpty(override.ChatID, base.ChatID)
	base.RunID = firstNonEmpty(override.RunID, base.RunID)
	base.CheckpointID = firstNonEmpty(override.CheckpointID, base.CheckpointID)
	base.Phase = firstNonEmpty(override.Phase, base.Phase)
	if strings.TrimSpace(string(override.Mode)) != "" {
		base.Mode = override.Mode
	}
	base.ResumeProposalID = firstNonEmpty(override.ResumeProposalID, base.ResumeProposalID)
	base.ResumeReason = firstNonEmpty(override.ResumeReason, base.ResumeReason)
	if len(override.Scratchpad) > 0 {
		base.Scratchpad = cloneStringMap(override.Scratchpad)
	} else {
		base.Scratchpad = cloneStringMap(base.Scratchpad)
	}
	return base
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
