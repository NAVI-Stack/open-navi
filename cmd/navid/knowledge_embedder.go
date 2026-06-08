package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/store"
)

const (
	knowledgeEmbedSkillID       = "navi.llm.embed"
	knowledgeEmbedInterfaceName = "embed"
)

func configureKnowledgeEmbedder(registry *skill.SkillRegistry, openAIKey string) {
	syncKnowledgeEmbedderEnv(openAIKey)
	store.SetKnowledgeEmbedder(buildSkillKnowledgeEmbedder(registry))
}

func syncKnowledgeEmbedderEnv(openAIKey string) {
	openAIKey = strings.TrimSpace(openAIKey)
	if openAIKey == "" {
		return
	}
	if err := os.Setenv("OPENAI_API_KEY", openAIKey); err != nil {
		slog.Warn("knowledge embedder: could not set OPENAI_API_KEY for llm-embed skill", "error", err)
	}
}

func buildSkillKnowledgeEmbedder(registry *skill.SkillRegistry) store.KnowledgeEmbedder {
	if registry == nil {
		return nil
	}
	return func(ctx context.Context, text string) ([]float64, error) {
		entry, iface, ok := findKnowledgeEmbedSkill(registry)
		if !ok {
			return nil, fmt.Errorf("knowledge embed skill not loaded")
		}
		raw, err := skill.Execute(ctx, entry, iface, map[string]any{"text": text})
		if err != nil {
			return nil, err
		}
		return parseKnowledgeEmbedVector(raw)
	}
}

func findKnowledgeEmbedSkill(registry *skill.SkillRegistry) (*skill.SkillEntry, *skill.Interface, bool) {
	for _, entry := range registry.List() {
		if entry.Spec == nil || entry.Spec.SkillID != knowledgeEmbedSkillID {
			continue
		}
		for i := range entry.Spec.Interfaces {
			if entry.Spec.Interfaces[i].Name != knowledgeEmbedInterfaceName {
				continue
			}
			entryCopy := entry
			ifaceCopy := entry.Spec.Interfaces[i]
			return &entryCopy, &ifaceCopy, true
		}
	}
	return nil, nil, false
}

func parseKnowledgeEmbedVector(raw string) ([]float64, error) {
	var result skill.SkillExecutionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("decode embed result: %w", err)
	}
	if result.Status != "success" {
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			return nil, fmt.Errorf("%s: %s", result.Error.Type, result.Error.Message)
		}
		return nil, fmt.Errorf("embed skill returned status %s", result.Status)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("embed skill payload missing vector")
	}
	return decodeKnowledgeVector(payload["vector"])
}

func decodeKnowledgeVector(raw any) ([]float64, error) {
	switch values := raw.(type) {
	case []float64:
		out := make([]float64, len(values))
		copy(out, values)
		return out, nil
	case []any:
		out := make([]float64, 0, len(values))
		for _, value := range values {
			number, ok := value.(float64)
			if !ok {
				return nil, fmt.Errorf("embed vector contained non-numeric value")
			}
			out = append(out, number)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("embed vector was empty")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("embed vector missing or invalid")
	}
}
