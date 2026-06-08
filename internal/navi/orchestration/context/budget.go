package context

import (
	"fmt"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

func applyBudget(items []orchestration.ContextItem, cfg Config) ([]orchestration.ContextItem, orchestration.ContextBudget, []string) {
	cfg = effectiveConfig(cfg)
	included := make([]orchestration.ContextItem, 0, min(len(items), cfg.MaxItems))
	usedChars := 0
	usedTokens := 0
	warnings := make([]string, 0, 2)
	excluded := 0

	for _, item := range items {
		contentChars := len(item.Content)
		contentTokens := estimateTokens(contentChars)
		if len(included) >= cfg.MaxItems || usedChars+contentChars > cfg.MaxChars || (cfg.MaxTokens > 0 && usedTokens+contentTokens > cfg.MaxTokens) {
			excluded++
			continue
		}
		included = append(included, item)
		usedChars += contentChars
		usedTokens += contentTokens
	}

	truncated := excluded > 0
	if truncated {
		warnings = append(warnings, fmt.Sprintf("Context budget excluded %d items from direct context.", excluded))
	}

	budget := orchestration.ContextBudget{
		MaxItems:        cfg.MaxItems,
		MaxChars:        cfg.MaxChars,
		MaxTokens:       cfg.MaxTokens,
		UsedItems:       len(included),
		UsedChars:       usedChars,
		UsedTokens:      usedTokens,
		ConsideredItems: len(items),
		IncludedItems:   len(included),
		ExcludedItems:   excluded,
		Truncated:       truncated,
	}
	return included, budget, warnings
}

func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	// Simple first-pass estimate suitable for reporting only.
	return (chars + 3) / 4
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
