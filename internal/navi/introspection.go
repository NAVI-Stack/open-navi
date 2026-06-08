package navi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/prompts"
)

// ConnectorHealthInfo holds the name and status of a connector for introspection.
type ConnectorHealthInfo struct {
	Name   string
	Status string // "running", "degraded", "stopped", "error"
}

// buildIntrospectionBlock gathers live runtime state and returns a formatted
// block for inclusion in the NCOS context pack. Every field is queried fresh
// (no caching) so the model always sees current state.
func (l *AgentLoop) buildIntrospectionBlock(ctx context.Context, chatID string, messageCount int) string {
	var b strings.Builder

	b.WriteString("\n\n## Runtime State (authoritative — overrides conversation history)\n")

	// Identity
	b.WriteString("- Agent: NAVI\n")

	// Active LLM — prefer live callback, fall back to static config
	provider, model := l.activeLLMForIntrospection(ctx)
	if provider != "" && model != "" {
		fmt.Fprintf(&b, "- Active LLM: %s/%s\n", provider, model)
	} else if provider != "" {
		fmt.Fprintf(&b, "- Active LLM: %s\n", provider)
	} else {
		b.WriteString("- Active LLM: unknown (no provider configured)\n")
	}

	// Governor state
	l.writeGovernorState(&b)

	// Connector health
	if l.cfg.ConnectorHealth != nil {
		connectors := l.cfg.ConnectorHealth()
		if len(connectors) > 0 {
			parts := make([]string, 0, len(connectors))
			for _, c := range connectors {
				parts = append(parts, fmt.Sprintf("%s (%s)", c.Name, c.Status))
			}
			fmt.Fprintf(&b, "- Connectors: %s\n", strings.Join(parts, ", "))
		} else {
			b.WriteString("- Connectors: none\n")
		}
	}

	// Chat
	fmt.Fprintf(&b, "- Chat: %s | messages: %d\n", chatID, messageCount)

	// Timestamp
	fmt.Fprintf(&b, "- Snapshot: %s\n", time.Now().UTC().Format(time.RFC3339))

	// Grounding rules
	b.WriteString("\n### Grounding Rules (non-negotiable)\n")
	b.WriteString(l.introspectionGroundingRules())

	return b.String()
}

// activeLLMForIntrospection returns the current provider and model, preferring
// the live GetActiveLLM callback over static config values.
func (l *AgentLoop) activeLLMForIntrospection(ctx context.Context) (provider, model string) {
	if l.cfg.GetActiveLLM != nil {
		if p, m, err := l.cfg.GetActiveLLM(ctx); err == nil && p != "" {
			return p, m
		}
	}
	// Fallback to static config
	provider = "unknown"
	if l.cfg.LLM != nil {
		provider = l.cfg.LLM.Name()
	}
	return provider, l.cfg.Model
}

// writeGovernorState type-asserts the Governor interface to access Stats/Limits
// and writes the governor state line to the builder.
func (l *AgentLoop) writeGovernorState(b *strings.Builder) {
	gov, ok := l.cfg.Governor.(*governor.Governor)
	if !ok || gov == nil {
		b.WriteString("- Governor: not configured\n")
		return
	}

	stats := gov.Stats()
	limits := gov.Limits()

	tripped := stats.Actions >= limits.MaxActionBudget ||
		stats.TotalUSD >= limits.CostCeiling ||
		stats.Elapsed >= limits.AutonomousDuration ||
		stats.Retries >= limits.MaxRetries

	status := "active"
	if tripped {
		status = "TRIPPED"
	}

	remaining := limits.AutonomousDuration - stats.Elapsed
	if remaining < 0 {
		remaining = 0
	}

	fmt.Fprintf(b, "- Governor: %s | budget %d/%d | cost $%.2f/$%.2f | duration remaining %s\n",
		status, stats.Actions, limits.MaxActionBudget,
		stats.TotalUSD, limits.CostCeiling,
		remaining.Truncate(time.Second))
}

func (l *AgentLoop) introspectionGroundingRules() string {
	pm := l.cfg.Prompts
	if pm == nil {
		em, err := prompts.EmbeddedManager()
		if err == nil {
			pm = em
		}
	}
	if pm != nil {
		raw, err := pm.Render(prompts.KindIntrospectionGrounding, nil, prompts.RenderOptions{})
		if err == nil && strings.TrimSpace(raw) != "" {
			if !strings.HasSuffix(raw, "\n") {
				raw += "\n"
			}
			return raw
		}
	}
	return "1. This Runtime State block is generated fresh by the Go runtime at the start of THIS turn. It is always more current than any information from earlier conversation messages.\n" +
		"2. When a user asks about your current model, governor, provider, status, or identity, answer from THIS block first. Supplement with tool calls for richer detail, but never contradict this block.\n" +
		"3. You ARE NAVI, a software agent. You HAVE a governor. You are software running on a server. Do not deny your own architecture or claim to be something else.\n" +
		"4. If you say you will perform an action (switch model, run a search, create a file), you MUST actually call the corresponding tool. Stating intent without a tool call is fabrication.\n" +
		"5. Earlier messages in this conversation may contain stale model lists, outdated governor states, or incorrect claims (including from your own prior replies). This Runtime State block supersedes all of them.\n" +
		"6. If a tool result contradicts something you said earlier, trust the tool result and correct yourself.\n"
}

// stalenessWarning returns a context note for conversations with many messages,
// reminding the LLM that earlier state claims may be outdated.
func stalenessWarning(messageCount int) string {
	if messageCount <= 10 {
		return ""
	}
	return "Note: This conversation has " + fmt.Sprintf("%d", messageCount) +
		" messages. Earlier messages may contain stale model lists, provider states, " +
		"or status claims. Always prefer the Runtime State block and fresh tool results " +
		"over conversation history for state queries."
}
