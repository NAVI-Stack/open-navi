package model

import (
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
)

type ToolResolver interface {
	ResolveToolDefinitions(names []string) []llm.ToolDefinition
}

type ToolResolverFunc func(names []string) []llm.ToolDefinition

func (f ToolResolverFunc) ResolveToolDefinitions(names []string) []llm.ToolDefinition {
	return f(names)
}

func compileProviderRequest(
	profile orchestration.ModelProfile,
	req orchestration.CanonicalRunRequest,
	pack orchestration.ContextPack,
	stack orchestration.InstructionStack,
	resolver ToolResolver,
) orchestration.CompiledModelRequest {
	profile = ApplyProviderDefaults(profile)
	systemMessages := compileSystemMessages(stack, pack, profile)
	historyMessages := compileConversationMessages(pack)
	messages := append(systemMessages, historyMessages...)
	if len(messages) == 0 && strings.TrimSpace(req.UserMessage) != "" {
		messages = append(messages, llm.Message{Role: "user", Content: strings.TrimSpace(req.UserMessage)})
	}
	if len(messages) > 0 && strings.TrimSpace(req.UserMessage) != "" && !conversationAlreadyIncludesLatestUserMessage(pack, req.UserMessage) {
		messages = append(messages, llm.Message{Role: "user", Content: strings.TrimSpace(req.UserMessage)})
	}

	toolNames := append([]string(nil), req.CapabilitySurface.ToolNames...)
	if len(toolNames) == 0 {
		toolNames = capabilityToolNamesFromStack(stack)
	}

	var tools []llm.ToolDefinition
	if resolver != nil && len(toolNames) > 0 {
		tools = resolver.ResolveToolDefinitions(toolNames)
	}

	return orchestration.CompiledModelRequest{
		Profile:  profile,
		Messages: messages,
		Tools:    tools,
		Options:  DefaultOptionsForRequest(profile, req),
		Metadata: buildRequestMetadata(req, pack, stack, len(tools)),
	}
}

func compileSystemMessages(stack orchestration.InstructionStack, pack orchestration.ContextPack, profile orchestration.ModelProfile) []llm.Message {
	blocks := make([]string, 0, 8)
	for _, layer := range stack.Ordered() {
		text := renderLayer(layer)
		if text != "" {
			blocks = append(blocks, text)
		}
	}
	for _, item := range pack.Items {
		if item.ContextClass == orchestration.ContextClassConversation || item.ContextClass == orchestration.ContextClassCapabilitySurface {
			continue
		}
		text := renderContextItem(item)
		if text != "" {
			blocks = append(blocks, text)
		}
	}
	if len(blocks) == 0 {
		return nil
	}

	role := "system"
	if !ProfileSupportsSystemRole(profile) {
		role = "user"
	}
	if ProfileSupportsSingleSystemMessage(profile) {
		return []llm.Message{{
			Role:    role,
			Content: strings.Join(blocks, "\n\n"),
		}}
	}
	out := make([]llm.Message, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, llm.Message{Role: role, Content: block})
	}
	return out
}

func compileConversationMessages(pack orchestration.ContextPack) []llm.Message {
	out := make([]llm.Message, 0)
	for _, item := range pack.Items {
		if item.ContextClass != orchestration.ContextClassConversation {
			continue
		}
		role := "user"
		if strings.EqualFold(item.Attributes["role"], "assistant") || strings.EqualFold(item.Attributes["role"], "navi") {
			role = "assistant"
		}
		out = append(out, llm.Message{Role: role, Content: item.Content})
	}
	return out
}

func conversationAlreadyIncludesLatestUserMessage(pack orchestration.ContextPack, userMessage string) bool {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return false
	}
	for i := len(pack.Items) - 1; i >= 0; i-- {
		item := pack.Items[i]
		if item.ContextClass != orchestration.ContextClassConversation {
			continue
		}
		return strings.EqualFold(item.Attributes["role"], "user") && strings.TrimSpace(item.Content) == userMessage
	}
	return false
}

func renderLayer(layer orchestration.InstructionLayer) string {
	sections := make([]string, 0, len(layer.Fragments)+1)
	title := formatLayerTitle(layer.Key)
	if title != "" {
		sections = append(sections, "## "+title)
	}
	for _, fragment := range layer.Fragments {
		content := strings.TrimSpace(fragment.Content)
		if content == "" {
			continue
		}
		if key := strings.TrimSpace(fragment.Key); key != "" {
			sections = append(sections, "### "+formatKeyTitle(key)+"\n"+content)
			continue
		}
		sections = append(sections, content)
	}
	return strings.Join(sections, "\n\n")
}

func renderContextItem(item orchestration.ContextItem) string {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return ""
	}
	header := []string{
		fmt.Sprintf("source_type: %s", item.Source),
		fmt.Sprintf("source_ref: %s", item.SourceRef),
		fmt.Sprintf("trust_level: %s", item.Trust),
		fmt.Sprintf("context_class: %s", item.ContextClass),
	}
	if item.Confidence > 0 {
		header = append(header, fmt.Sprintf("confidence: %.2f", item.Confidence))
	}
	if label := strings.TrimSpace(item.Label); label != "" {
		header = append([]string{"## " + label}, header...)
	}
	return strings.Join(append(header, content), "\n")
}

func buildRequestMetadata(req orchestration.CanonicalRunRequest, pack orchestration.ContextPack, stack orchestration.InstructionStack, resolvedToolCount int) map[string]string {
	meta := map[string]string{
		"trace_id":         req.Frame.TraceID,
		"chat_id":          req.Frame.ChatID,
		"run_id":           req.Frame.RunID,
		"layer_count":      fmt.Sprintf("%d", len(stack.Ordered())),
		"context_items":    fmt.Sprintf("%d", len(pack.Items)),
		"tool_count":       fmt.Sprintf("%d", resolvedToolCount),
		"experience_mode":  strings.TrimSpace(req.ExperienceMode),
		"execution_mode":   strings.TrimSpace(string(req.Frame.Mode)),
		"user_message_len": fmt.Sprintf("%d", len(strings.TrimSpace(req.UserMessage))),
	}
	for key, value := range meta {
		if strings.TrimSpace(value) == "" {
			delete(meta, key)
		}
	}
	return meta
}

func capabilityToolNamesFromStack(stack orchestration.InstructionStack) []string {
	for _, fragment := range stack.CapabilitySurface.Fragments {
		raw := strings.TrimSpace(fragment.Content)
		if raw == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "tools:") {
				continue
			}
			return splitAndClean(strings.TrimSpace(strings.TrimPrefix(line, "tools:")))
		}
	}
	return nil
}

func splitAndClean(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatLayerTitle(key orchestration.InstructionLayerKey) string {
	switch key {
	case orchestration.InstructionLayerSystemCore:
		return "System Core"
	case orchestration.InstructionLayerRuntimeConstraints:
		return "Runtime Constraints"
	case orchestration.InstructionLayerExperienceOverlay:
		return "Experience Overlay"
	case orchestration.InstructionLayerTaskFraming:
		return "Task Framing"
	case orchestration.InstructionLayerCapabilitySurface:
		return "Capability Surface"
	default:
		return formatKeyTitle(string(key))
	}
}

func formatKeyTitle(key string) string {
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.ReplaceAll(key, "-", " ")
	fields := strings.Fields(key)
	for i, field := range fields {
		fields[i] = strings.ToUpper(field[:1]) + field[1:]
	}
	return strings.Join(fields, " ")
}
