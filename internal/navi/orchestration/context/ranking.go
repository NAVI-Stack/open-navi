package context

import (
	"sort"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

func rankItems(items []orchestration.ContextItem) []orchestration.ContextItem {
	ranked := append([]orchestration.ContextItem(nil), items...)
	sort.SliceStable(ranked, func(i, j int) bool {
		li := rankingWeight(ranked[i])
		lj := rankingWeight(ranked[j])
		if li != lj {
			return li < lj
		}
		return i < j
	})
	return ranked
}

func rankingWeight(item orchestration.ContextItem) int {
	switch item.ContextClass {
	case orchestration.ContextClassRuntimeState:
		return 10
	case orchestration.ContextClassScratchpad:
		return 20
	case orchestration.ContextClassProposal:
		return 30
	case orchestration.ContextClassCapabilitySurface:
		return 40
	case orchestration.ContextClassChatSummary:
		return 50
	case orchestration.ContextClassFacts:
		return 60
	case orchestration.ContextClassConversation:
		return 70
	case orchestration.ContextClassGovernance:
		return 80
	default:
		return 100
	}
}
