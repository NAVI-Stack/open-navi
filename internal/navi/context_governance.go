package navi

import (
	"fmt"
	"strings"

	naviruntime "github.com/ceoai/navi/internal/runtime"
)

type contextGovernance struct {
	recentMessages int
	pinnedMessages int
	scratchpad     bool
}

func (l *AgentLoop) contextGovernance() contextGovernance {
	cfg := contextGovernance{
		recentMessages: l.cfg.ContextRecentMessages,
		pinnedMessages: l.cfg.ContextPinnedMessages,
		scratchpad:     l.cfg.EnableRunScratchpad,
	}
	if cfg.recentMessages <= 0 {
		cfg.recentMessages = 12
	}
	if cfg.pinnedMessages < 0 {
		cfg.pinnedMessages = 0
	}
	return cfg
}

func (l *AgentLoop) selectContextMessages(messages []ChatRuntimeMessage) []ChatRuntimeMessage {
	gov := l.contextGovernance()
	if len(messages) <= gov.recentMessages+gov.pinnedMessages {
		return messages
	}
	cutoff := len(messages) - gov.recentMessages
	if cutoff < gov.pinnedMessages {
		cutoff = gov.pinnedMessages
	}
	selected := make([]ChatRuntimeMessage, 0, gov.pinnedMessages+gov.recentMessages)
	selected = append(selected, messages[:gov.pinnedMessages]...)
	selected = append(selected, messages[cutoff:]...)
	return selected
}

func (l *AgentLoop) contextGovernanceNote(messages []ChatRuntimeMessage) (string, bool) {
	selected := l.selectContextMessages(messages)
	omitted := len(messages) - len(selected)
	if omitted <= 0 {
		return "", false
	}
	return fmt.Sprintf(
		"Conversation history policy: %d older messages were aged out of direct context. Use the summary and facts blocks for older context, preserve the opening task framing, and prioritize the recent exchange.",
		omitted,
	), true
}

func (l *AgentLoop) prepareRunScratchpad(run *naviruntime.RunState, item *naviruntime.InboxItem, lastUser string) {
	if run == nil {
		return
	}
	run.SetScratchpadValue("run_id", run.RunID)
	run.SetScratchpadValue("chat_id", run.ChatID)
	run.SetScratchpadValue("mode", string(run.Mode))
	run.SetScratchpadValue("experience_mode", run.ExperienceMode)
	run.SetScratchpadValue("phase", string(run.CurrentPhase))
	run.SetScratchpadValue("inbox_item_id", run.InitiatedByInboxItemID)
	if item != nil && strings.TrimSpace(item.ID) != "" {
		run.SetScratchpadValue("inbox_item_id", item.ID)
	}
	run.SetScratchpadValue("source_channel", runtimeRecoverySurfaceFromInbox(item))
	if item != nil {
		run.SetScratchpadValue("source_message_ref", strings.TrimSpace(item.SourceMessageRef))
	}
	run.SetScratchpadValue("current_user_message", strings.TrimSpace(lastUser))
}
