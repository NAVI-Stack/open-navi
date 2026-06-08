package navi

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
	orchestrationcontext "github.com/open-navi/navi/internal/navi/orchestration/context"
	orchestrationinstructions "github.com/open-navi/navi/internal/navi/orchestration/instructions"
	orchestrationmodel "github.com/open-navi/navi/internal/navi/orchestration/model"
	orchestrationtrace "github.com/open-navi/navi/internal/navi/orchestration/trace"
	"github.com/open-navi/navi/internal/schema"
)

func newRuntimeOrchestrationPipeline(loop *AgentLoop) *orchestration.Pipeline {
	if loop == nil {
		return nil
	}

	contextResolver := orchestrationcontext.ResolverFunc(func(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestrationcontext.AssembleInput, error) {
		return loop.runtimeContextAssembleInput(ctx, req)
	})

	instructionResolver := orchestrationinstructions.ResolverFunc(func(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack) (orchestrationinstructions.CompileRequest, error) {
		return loop.runtimeInstructionCompileRequest(ctx, req, pack)
	})

	toolResolver := orchestrationmodel.ToolResolverFunc(func(names []string) []llm.ToolDefinition {
		return loop.runtimeResolveToolDefinitions(names)
	})

	return orchestration.NewPipeline(
		orchestrationcontext.NewAssembler(contextResolver, orchestrationcontext.Config{}),
		orchestrationinstructions.NewCompiler(instructionResolver),
		orchestrationmodel.NewDefaultAdapter(nil, toolResolver),
		loop.runtimeTraceSink(),
	)
}

func (l *AgentLoop) applyRoutingToCanonicalRequest(ctx context.Context, req orchestration.CanonicalRunRequest) orchestration.CanonicalRunRequest {
	router, ok := l.cfg.LLMService.(routeCapableLLMService)
	if !ok {
		return req
	}

	req.Model.Metadata = cloneStringMap(req.Model.Metadata)
	tools := l.runtimeResolveToolDefinitions(req.CapabilitySurface.ToolNames)
	decision, err := router.Route(ctx, llm.RouteRequest{
		UserMessage:     strings.TrimSpace(req.UserMessage),
		Tools:           tools,
		History:         routeHistoryFromConversation(req.Conversation),
		CurrentProvider: strings.TrimSpace(req.Model.Provider),
		CurrentModel:    strings.TrimSpace(req.Model.Model),
		ChatID:          strings.TrimSpace(req.Frame.ChatID),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "route is not configured") {
			return req
		}
		slog.Warn("navi: NCOS routing decision failed", "chat_id", req.Frame.ChatID, "run_id", req.Frame.RunID, "error", err)
		return req
	}

	if decision.Profile != nil {
		resolved := orchestration.ModelProfile{
			SupportsTools:     decision.Profile.SupportsTools,
			SupportsStreaming: decision.Profile.SupportsStream,
			MaxContextTokens:  decision.Profile.MaxContextTokens,
			Metadata: map[string]string{
				"routing_profile_provider_key": strings.TrimSpace(decision.Profile.ProviderKey),
				"routing_profile_model_id":     strings.TrimSpace(decision.Profile.ModelID),
				"routing_profile_display_name": strings.TrimSpace(decision.Profile.DisplayName),
			},
		}
		req.Model = mergeRuntimeModelProfiles(req.Model, resolved)
	}
	if provider := strings.TrimSpace(decision.Provider); provider != "" {
		req.Model.Provider = provider
	}
	if model := strings.TrimSpace(decision.Model); model != "" {
		req.Model.Model = model
	}
	suppressForChat := orchestrationmodel.ProfileHasQuirk(req.Model, orchestrationmodel.QuirkSuppressToolsForChat) &&
		req.Frame.Mode == orchestration.ExecutionModeChatTurn
	policy := orchestration.ApplyRoutingSurfacePolicy(orchestration.RoutingSurfacePolicyInput{
		Surface:        req.CapabilitySurface,
		RequiredOutput: req.RequiredOutput,
		Decision: orchestration.RoutingSurfaceDecision{
			StripTools: decision.StripTools || suppressForChat,
			Reason:     decision.Reason,
		},
	})
	req.CapabilitySurface = policy.Surface
	req.RequiredOutput = policy.RequiredOutput

	req.Model.Metadata["routing_reason"] = strings.TrimSpace(decision.Reason)
	req.Model.Metadata["routing_switched"] = strconv.FormatBool(decision.Switched)
	req.Model.Metadata["routing_strip_tools"] = strconv.FormatBool(decision.StripTools)
	req.Model.Metadata["routing_surface_policy"] = strings.TrimSpace(policy.Policy)
	req.Model.Metadata["routing_provider"] = strings.TrimSpace(decision.Provider)
	req.Model.Metadata["routing_model"] = strings.TrimSpace(decision.Model)
	req.Model.Metadata["routing_chat_id"] = strings.TrimSpace(decision.ChatID)
	req.Model.Metadata["routing_task_id"] = strings.TrimSpace(decision.TaskID)
	req.Model.Metadata["routing_classification_task"] = strings.TrimSpace(string(decision.Classification.Task))
	req.Model.Metadata["routing_classification_complexity"] = strings.TrimSpace(string(decision.Classification.Complexity))
	if decision.StripTools && runtimeRequestRequiresReliableTools(req.UserMessage) {
		req.Model.Metadata["routing_guard"] = string(orchestration.SurfaceGuardToolCapableModelRequired)
		req.Model.Metadata["routing_guard_reason"] = firstNonEmpty(
			strings.TrimSpace(decision.Reason),
			toolCapableModelRequiredGuardReason,
		)
	}
	if announcement := strings.TrimSpace(decision.Announcement); announcement != "" {
		req.Model.Metadata["routing_announcement"] = announcement
		req.Model.Metadata["routing_announcement_visibility"] = strings.TrimSpace(string(decision.AnnouncementVisibility))
	}
	if policy.Guard != orchestration.SurfaceGuardNone {
		req.Model.Metadata["routing_guard"] = strings.TrimSpace(string(policy.Guard))
		req.Model.Metadata["routing_guard_reason"] = strings.TrimSpace(policy.GuardReason)
	}

	return req
}

func runtimeRequestRequiresReliableTools(raw string) bool {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "<tool_call") || strings.Contains(normalized, "write_file") {
		return true
	}
	if classifyBoundedScheduledTasksQuery(normalized) || strings.Contains(normalized, "recent failures") || strings.Contains(normalized, "recent errors") {
		return true
	}
	if runtimeLooksLikeTimeBasedAction(normalized) {
		return true
	}
	if runtimeLooksLikeRenderIntent(normalized) {
		return true
	}
	if runtimeContainsAny(normalized, "create a skill", "create skill", "build a skill", "build skill", "write a skill", "write skill", "generate a skill", "generate skill", "save a skill", "compile a skill") {
		return true
	}
	if runtimeContainsAny(normalized, "codebase", "repository", "repo") &&
		runtimeContainsAny(normalized, "analyze", "analysis", "inspect", "review", "summarize", "map", "list", "find") {
		return true
	}
	if runtimeContainsAny(normalized, "file", "files", "directory", "directories", "folder", "folders", "workspace") &&
		runtimeContainsAny(normalized, "list", "show", "read", "open", "inspect", "write", "create", "save", "edit") {
		return true
	}
	if runtimeContainsAny(normalized, "run tests", "run the tests", "run build", "run the build", "run command", "run a command", "execute command", "execute a command") {
		return true
	}
	return false
}

func runtimeContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func runtimeLooksLikeActionIntent(raw string) bool {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return false
	}
	return runtimeContainsAny(normalized,
		"remind me", "schedule", "send me", "message me", "notify me",
		"create", "update", "delete", "remove", "run", "execute", "write", "save",
		"open", "read", "list", "find", "inspect", "review", "analyze",
	) || runtimeLooksLikeRenderIntent(normalized)
}

func runtimeLooksLikeRenderIntent(raw string) bool {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return false
	}
	return runtimeContainsAny(normalized,
		"navi.render.visualize",
		"openui",
		"render",
		"visualize",
		"visualise",
		"visualization",
		"graph",
		"chart",
		"plot",
		"dashboard",
		"data view",
		"data-view",
	) || runtimeContainsAny(normalized,
		"show me a table",
		"tabulate",
	)
}

func runtimeLooksLikeTimeBasedAction(raw string) bool {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return false
	}
	actionVerb := runtimeContainsAny(normalized,
		"remind me", "reminder", "schedule", "scheduled", "send me", "tell me",
		"message me", "notify me", "follow up", "check back", "wake me",
	)
	timeSignal := runtimeContainsAny(normalized,
		"second", "seconds", "minute", "minutes", "hour", "hours", "day", "days",
		"tomorrow", "tonight", "later", "apart", "every", "daily", "weekly",
		"morning", "afternoon", "evening",
	) || runtimeContainsClockishPhrase(normalized)
	return actionVerb && timeSignal
}

func runtimeContainsClockishPhrase(normalized string) bool {
	fields := strings.Fields(normalized)
	for i, field := range fields {
		if field == "am" || field == "pm" || strings.HasSuffix(field, "am") || strings.HasSuffix(field, "pm") {
			return true
		}
		if !runtimeLooksNumeric(field) {
			continue
		}
		if i > 0 {
			switch fields[i-1] {
			case "at", "by", "around", "before", "after", "in":
				return true
			}
		}
	}
	return false
}

func runtimeLooksNumeric(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func runtimeHasToolNamed(names []string, candidates ...string) bool {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, candidate := range candidates {
			if name == strings.TrimSpace(candidate) {
				return true
			}
		}
	}
	return false
}

func (l *AgentLoop) runtimeContextAssembleInput(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestrationcontext.AssembleInput, error) {
	var input orchestrationcontext.AssembleInput
	if block := strings.TrimSpace(l.chatSummaryBlock(ctx, req.Frame.ChatID)); block != "" {
		input.Summaries = []orchestrationcontext.SummaryInput{{
			Block:     block,
			SourceRef: req.Frame.ChatID,
		}}
	}
	if l.cfg.FactsBlock != nil {
		if block, err := l.cfg.FactsBlock(ctx, req.Frame.ChatID); err == nil && strings.TrimSpace(block) != "" {
			input.Facts = []orchestrationcontext.FactsInput{{
				Block:      block,
				SourceRef:  req.Frame.ChatID,
				Confidence: 1.0,
			}}
		}
	}
	// CIP P4 Contextualize: pull provenance-bearing retrieved context for the
	// current user message and inject it as a Facts block. A nil retriever or an
	// empty return leaves turn-time behavior unchanged.
	if fact, ok := l.retrievedContextFacts(ctx, req); ok {
		input.Facts = append(input.Facts, fact)
	}
	input.RuntimeStates = append(input.RuntimeStates, l.runtimeStateInputs(ctx, req)...)
	input.Proposals = append(input.Proposals, l.runtimeProposalInputs(ctx, req)...)
	input.Config = orchestrationcontext.Config{
		MaxItems:           24,
		MaxChars:           12000,
		RecentMessageLimit: l.contextGovernance().recentMessages + l.contextGovernance().pinnedMessages,
	}
	return input, nil
}

func (l *AgentLoop) runtimeInstructionCompileRequest(ctx context.Context, req orchestration.CanonicalRunRequest, _ orchestration.ContextPack) (orchestrationinstructions.CompileRequest, error) {
	mode := NormalizeExperienceMode(ExperienceMode(req.ExperienceMode))
	if req.ExperienceMode == "" {
		mode = ExperienceModeStandard
	}
	totalMessages := metadataInt(req.Model.Metadata, "conversation_total_count", len(req.Conversation))
	experienceFragment, err := l.buildRunExperienceOverlay(ctx, req.Frame.ChatID, mode, req.UserMessage, req.Conversation, totalMessages)
	if err != nil {
		return orchestrationinstructions.CompileRequest{}, err
	}
	runtimeConstraints := orchestrationinstructions.RuntimeConstraintsInput{}
	if omitted := totalMessages - len(req.Conversation); omitted > 0 {
		runtimeConstraints.GovernanceNotes = append(runtimeConstraints.GovernanceNotes, fmt.Sprintf(
			"Conversation history policy: %d older messages were aged out of direct context. Use the summary and facts blocks for older context, preserve the opening task framing, and prioritize the recent exchange.",
			omitted,
		))
	} else if warning := stalenessWarning(totalMessages); warning != "" {
		runtimeConstraints.AuthoritativeWarnings = append(runtimeConstraints.AuthoritativeWarnings, warning)
	}
	taskFrame := orchestrationinstructions.TaskFrameInput{
		Objective: strings.TrimSpace(req.UserMessage),
	}
	if taskFrame.Objective == "" && len(req.Conversation) == 0 {
		taskFrame.Instructions = append(taskFrame.Instructions, l.defaultFirstTurnInstruction(mode))
	}
	if runtimeLooksLikeActionIntent(req.UserMessage) {
		taskFrame.Instructions = append(taskFrame.Instructions,
			"The latest user message contains a natural-language command or action request. Treat it as an action intent when the requested action is clear, even if it appears inside casual chat.",
			"If the action intent is plausible but required details are missing, ask one concise clarifying question for the missing detail instead of guessing or discussing the capability abstractly.",
		)
	}
	capabilitySurface := orchestrationinstructions.CapabilitySurfaceInput{
		Surface:         req.CapabilitySurface.Surface,
		ToolNames:       append([]string(nil), req.CapabilitySurface.ToolNames...),
		SelectionReason: req.CapabilitySurface.SelectionReason,
	}
	if runtimeLooksLikeTimeBasedAction(req.UserMessage) {
		taskFrame.Instructions = append(taskFrame.Instructions,
			"The latest user message appears to be a time-based action request. If the what, timing, and recipient are clear, use the surfaced messaging or scheduler tool rather than explaining scheduler design.",
			"For missing scheduling fields, ask exactly one short clarification question.",
		)
		if runtimeHasToolNamed(req.CapabilitySurface.ToolNames, sendReplyToolName, "skill.core-scheduler.schedule_task") {
			capabilitySurface.Additional = append(capabilitySurface.Additional,
				"Time-based command handling: use navi.messaging.send_reply for short delayed replies in this chat, and skill.core-scheduler.schedule_task for durable, recurring, or longer-horizon schedules.",
			)
		}
	}
	if runtimeHasToolNamed(req.CapabilitySurface.ToolNames, "skill.core-artifact.create") {
		capabilitySurface.Additional = append(capabilitySurface.Additional,
			"Artifact handling: when the user asks you to create or save a substantial durable work product such as HTML, code, Markdown, a table, or a diagram, use skill.core-artifact.create for the durable artifact and then reply with a concise artifact reference.",
			"Short conversational answers should remain normal chat replies and should not be saved as artifacts.",
		)
	}
	return orchestrationinstructions.CompileRequest{
		SystemCore:         l.runtimeSystemCoreInput(ctx, req.Frame.ChatID, req.Model),
		RuntimeConstraints: runtimeConstraints,
		ExperienceOverlay: orchestrationinstructions.ExperienceOverlayInput{
			Fragment: experienceFragment,
		},
		TaskFrame: taskFrame,
		OutputContract: orchestrationinstructions.OutputContractInput{
			Present:               true,
			MustReply:             req.RequiredOutput.MustReply,
			AllowToolCalls:        req.RequiredOutput.AllowToolCalls,
			AllowScheduledReplies: req.RequiredOutput.AllowScheduledReplies,
			ExpectStreaming:       req.RequiredOutput.ExpectStreaming,
		},
		CapabilitySurface: capabilitySurface,
		ExecutionFrame: orchestrationinstructions.ExecutionFrameInput{
			Frame: orchestration.ExecutionFrame{
				ChatID:           req.Frame.ChatID,
				RunID:            req.Frame.RunID,
				CheckpointID:     req.Frame.CheckpointID,
				Phase:            req.Frame.Phase,
				Mode:             req.Frame.Mode,
				ResumeProposalID: req.Frame.ResumeProposalID,
				ResumeReason:     req.Frame.ResumeReason,
			},
		},
	}, nil
}

func (l *AgentLoop) runtimeResolveToolDefinitions(names []string) []llm.ToolDefinition {
	reg, err := l.ensureToolRegistry()
	if err != nil {
		slog.Warn("navi: build tool registry failed for NCOS adapter", "error", err)
		return nil
	}
	out := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tool, ok := reg.Lookup(name)
		if !ok || tool.Hidden {
			continue
		}
		out = append(out, tool.Definition)
	}
	return out
}

func (l *AgentLoop) resolveCapabilitySurface(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestration.CanonicalRunRequest, orchestration.SurfaceResolutionResult, error) {
	reg, err := l.ensureToolRegistry()
	if err != nil {
		return req, orchestration.SurfaceResolutionResult{}, fmt.Errorf("navi: ensure tool registry for capability surface resolution: %w", err)
	}

	input := orchestration.SurfaceResolutionInput{
		Surface:        req.CapabilitySurface,
		ExecutionMode:  req.Frame.Mode,
		UserMessage:    req.UserMessage,
		RequiredOutput: req.RequiredOutput,
		Model:          req.Model,
	}
	result := orchestration.NewCapabilitySurfaceResolver(reg).Resolve(input)
	result = orchestration.ApplySurfaceGuardPolicy(input, result)

	if req.Model.Metadata == nil {
		req.Model.Metadata = map[string]string{}
	}
	req.Model.Metadata["capability_surface_requested"] = strings.TrimSpace(result.Requested.Surface)
	req.Model.Metadata["capability_surface_resolved"] = strings.TrimSpace(result.Resolved.Surface)
	req.Model.Metadata["capability_surface_expected_tools"] = strconv.FormatBool(result.ExpectedTools)
	req.Model.Metadata["capability_surface_requires_tool_capable_model"] = strconv.FormatBool(result.RequiresToolCapableModel)
	req.Model.Metadata["capability_surface_guard"] = strings.TrimSpace(string(result.Guard))
	req.Model.Metadata["capability_surface_guard_reason"] = strings.TrimSpace(result.GuardReason)

	if err := orchestrationtrace.RecordSurfaceResolution(ctx, l.runtimeTraceSink(), req.Frame, input, result); err != nil {
		slog.Warn("navi: failed to record capability-surface trace", "chat_id", req.Frame.ChatID, "run_id", req.Frame.RunID, "error", err)
	}

	req.CapabilitySurface.Surface = firstNonEmpty(strings.TrimSpace(result.Resolved.Surface), strings.TrimSpace(req.CapabilitySurface.Surface))
	req.CapabilitySurface.SelectionReason = firstNonEmpty(strings.TrimSpace(result.Resolved.SelectionReason), strings.TrimSpace(req.CapabilitySurface.SelectionReason))
	if result.IsGuarded() {
		req.CapabilitySurface.ToolNames = nil
		req.RequiredOutput.AllowToolCalls = false
		return req, result, nil
	}

	req.CapabilitySurface.ToolNames = append([]string(nil), result.Resolved.ToolNames...)
	return req, result, nil
}

func (l *AgentLoop) runtimeProposalInputs(ctx context.Context, req orchestration.CanonicalRunRequest) []orchestrationcontext.ProposalInput {
	proposalID := strings.TrimSpace(req.Frame.ResumeProposalID)
	if proposalID == "" {
		return nil
	}
	input := orchestrationcontext.ProposalInput{
		ProposalID: proposalID,
		Status:     "pending",
		Summary:    strings.TrimSpace(req.Frame.ResumeReason),
		SourceRef:  "proposal:" + proposalID,
		Confidence: 1.0,
	}
	if l.cfg.GetProposal == nil {
		return []orchestrationcontext.ProposalInput{input}
	}
	proposal, err := l.cfg.GetProposal(ctx, proposalID)
	if err != nil {
		slog.Warn("navi: failed to load proposal for NCOS context", "proposal_id", proposalID, "error", err)
		return []orchestrationcontext.ProposalInput{input}
	}
	input.Status = strings.TrimSpace(string(proposal.Status))
	input.ProposedAction = strings.TrimSpace(proposal.ProposedAction)
	input.Summary = proposalSummaryForContext(proposal, input.Summary)
	input.SourceRef = firstNonEmpty(input.SourceRef, "proposal:"+strings.TrimSpace(proposal.ProposalID))
	return []orchestrationcontext.ProposalInput{input}
}

func (l *AgentLoop) runtimeStateInputs(ctx context.Context, req orchestration.CanonicalRunRequest) []orchestrationcontext.RuntimeStateInput {
	inputs := make([]orchestrationcontext.RuntimeStateInput, 0, 2)
	if block := strings.TrimSpace(buildExecutionRuntimeStateBlock(req)); block != "" {
		inputs = append(inputs, orchestrationcontext.RuntimeStateInput{
			Block:     block,
			SourceRef: firstNonEmpty(req.Frame.RunID, req.Frame.ChatID),
		})
	}
	totalMessages := metadataInt(req.Model.Metadata, "conversation_total_count", len(req.Conversation))
	if block := strings.TrimSpace(l.buildIntrospectionBlock(ctx, req.Frame.ChatID, totalMessages)); block != "" {
		inputs = append(inputs, orchestrationcontext.RuntimeStateInput{
			Block:     block,
			SourceRef: firstNonEmpty(req.Frame.RunID, req.Frame.ChatID),
		})
	}
	return inputs
}

func buildExecutionRuntimeStateBlock(req orchestration.CanonicalRunRequest) string {
	values := map[string]string{
		"active_model":               strings.TrimSpace(req.Model.Model),
		"active_provider":            strings.TrimSpace(req.Model.Provider),
		"checkpoint_id":              strings.TrimSpace(req.Frame.CheckpointID),
		"conversation_total_count":   strings.TrimSpace(req.Model.Metadata["conversation_total_count"]),
		"execution_mode":             strings.TrimSpace(string(req.Frame.Mode)),
		"initiated_by_inbox_item_id": strings.TrimSpace(req.Model.Metadata["initiated_by_inbox_item_id"]),
		"interrupt_class":            strings.TrimSpace(req.Model.Metadata["interrupt_class"]),
		"interrupt_reason":           strings.TrimSpace(req.Model.Metadata["interrupt_reason"]),
		"pause_reason":               strings.TrimSpace(req.Model.Metadata["pause_reason"]),
		"phase":                      strings.TrimSpace(req.Frame.Phase),
		"resume_proposal_id":         strings.TrimSpace(req.Frame.ResumeProposalID),
		"resume_reason":              strings.TrimSpace(req.Frame.ResumeReason),
		"run_id":                     strings.TrimSpace(req.Frame.RunID),
		"run_mode":                   strings.TrimSpace(req.Model.Metadata["run_mode"]),
		"chat_id":                    strings.TrimSpace(req.Frame.ChatID),
		"surface":                    strings.TrimSpace(req.Model.Metadata["surface"]),
	}
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if value == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	lines = append(lines, "runtime execution state:")
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", key, values[key]))
	}
	return strings.Join(lines, "\n")
}

func proposalSummaryForContext(proposal schema.Proposal, fallback string) string {
	return firstNonEmpty(strings.TrimSpace(proposal.Rationale), strings.TrimSpace(fallback))
}

func routeHistoryFromConversation(turns []orchestration.ConversationTurn) []llm.Message {
	out := make([]llm.Message, 0, len(turns))
	for _, turn := range turns {
		role := "user"
		if strings.EqualFold(strings.TrimSpace(turn.Role), "assistant") || strings.EqualFold(strings.TrimSpace(turn.Role), "navi") {
			role = "assistant"
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		out = append(out, llm.Message{Role: role, Content: content})
	}
	return out
}

func mergeRuntimeModelProfiles(base, override orchestration.ModelProfile) orchestration.ModelProfile {
	base.Provider = firstNonEmpty(strings.TrimSpace(override.Provider), strings.TrimSpace(base.Provider))
	base.Model = firstNonEmpty(strings.TrimSpace(override.Model), strings.TrimSpace(base.Model))
	base.SupportsTools = override.SupportsTools
	base.SupportsStreaming = override.SupportsStreaming || base.SupportsStreaming
	base.SupportsSystemRole = override.SupportsSystemRole || base.SupportsSystemRole
	base.SupportsMultiSystemMessages = override.SupportsMultiSystemMessages || base.SupportsMultiSystemMessages
	if override.MaxContextTokens > 0 {
		base.MaxContextTokens = override.MaxContextTokens
	}
	base.Quirks = appendUniqueStrings(base.Quirks, override.Quirks)
	if len(override.Metadata) > 0 {
		base.Metadata = cloneStringMap(base.Metadata)
		for key, value := range override.Metadata {
			if strings.TrimSpace(value) == "" {
				continue
			}
			base.Metadata[key] = value
		}
	}
	return base
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func appendUniqueStrings(base []string, extras []string) []string {
	if len(base) == 0 && len(extras) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, value := range append(append([]string(nil), base...), extras...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
