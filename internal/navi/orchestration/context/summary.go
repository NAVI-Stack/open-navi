package context

import (
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

type SummaryInput struct {
	Block      string
	SourceRef  string
	Confidence float64
}

func summaryContextItem(input SummaryInput) (orchestration.ContextItem, bool) {
	content := normalizeSummaryBlock(input.Block)
	if content == "" {
		return orchestration.ContextItem{}, false
	}
	return newContextItem(
		"summary",
		"chat_summary",
		"chat summary",
		content,
		orchestration.ContextSourceSummary,
		sourceRefWithDefault(input.SourceRef, "chat_summary"),
		orchestration.TrustLabelDerived,
		orchestration.ContextClassChatSummary,
		defaultConfidence(input.Confidence, 0.8),
		nil,
	), true
}

func normalizeSummaryBlock(block string) string {
	block = strings.ReplaceAll(block, "\r\n", "\n")
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
