package navi

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

// GapDetector analyzes agent turns to identify capability gaps.
type GapDetector struct {
	db *sql.DB
}

// NewGapDetector initializes a new detector.
func NewGapDetector(db *sql.DB) *GapDetector {
	return &GapDetector{db: db}
}

// RecordSignalA identifies gaps from the LLM's own admission of inability.
// It scans the LLM response text for heuristic markers indicating a missing
// capability. If found, a type-A (missing skill) gap is recorded.
func (d *GapDetector) RecordSignalA(ctx context.Context, chatID, content string) (*store.Gap, error) {
	if !isInabilityResponse(content) {
		return nil, nil
	}

	evidence := store.GapEvidence{
		ChatID:     chatID,
		RawContext: truncate(content, 500),
	}

	return d.insertUniqueGap(ctx, store.GapTypeMissingSkill, store.GapClassification{
		GapClass:      store.GapTypeMissingSkill,
		ExpansionPath: "skill",
		Reason:        "LLM self-reported inability in response text",
	}, evidence)
}

// RecordSignalB identifies gaps from tool/skill execution failures.
// It classifies the error into gap types A–F based on the error content
// and the tool name.
func (d *GapDetector) RecordSignalB(ctx context.Context, chatID, toolName, toolError string) (*store.Gap, error) {
	gapType, classification := classifyToolError(toolName, toolError)

	evidence := store.GapEvidence{
		ChatID:    chatID,
		ToolError: truncate(fmt.Sprintf("Tool %s failed: %s", toolName, toolError), 500),
	}

	return d.insertUniqueGap(ctx, gapType, classification, evidence)
}

// classifyToolError maps a tool execution error into a gap type (A–F) and
// a classification with expansion path.
func classifyToolError(toolName, errMsg string) (store.GapType, store.GapClassification) {
	lower := strings.ToLower(errMsg)

	// Type A — Missing skill / tool not found
	if strings.Contains(lower, "tool not found") ||
		strings.Contains(lower, "skill not found") ||
		strings.Contains(lower, "unknown tool") ||
		strings.Contains(lower, "no such skill") {
		return store.GapTypeMissingSkill, store.GapClassification{
			GapClass:      store.GapTypeMissingSkill,
			ExpansionPath: "skill",
			Reason:        fmt.Sprintf("Tool %q not found in registry", toolName),
		}
	}

	// Type B — Missing connector
	if strings.Contains(lower, "connector not running") ||
		strings.Contains(lower, "connector not found") ||
		strings.Contains(lower, "connector not configured") ||
		strings.Contains(lower, "no connector") {
		return store.GapTypeMissingConnector, store.GapClassification{
			GapClass:      store.GapTypeMissingConnector,
			ExpansionPath: "connector",
			Reason:        fmt.Sprintf("Connector required by %q is not available", toolName),
		}
	}

	// Type C — Missing runtime / transport
	if strings.Contains(lower, "transport") ||
		strings.Contains(lower, "runtime not available") ||
		strings.Contains(lower, "subprocess") && strings.Contains(lower, "not found") ||
		strings.Contains(lower, "python") && strings.Contains(lower, "not found") ||
		strings.Contains(lower, "execution environment") {
		return store.GapTypeMissingRuntime, store.GapClassification{
			GapClass:      store.GapTypeMissingRuntime,
			ExpansionPath: "plugin",
			Reason:        fmt.Sprintf("Runtime/transport unavailable for %q", toolName),
		}
	}

	// Type E — Policy / governance rejection (checked before auth because
	// governance messages often contain "forbidden" which overlaps with auth patterns)
	if strings.Contains(lower, "governance policy rejected") ||
		strings.Contains(lower, "operator approval required") ||
		strings.Contains(lower, "governor") && strings.Contains(lower, "rejected") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "policy") && strings.Contains(lower, "blocked") {
		return store.GapTypeMissingPolicyTrust, store.GapClassification{
			GapClass:      store.GapTypeMissingPolicyTrust,
			ExpansionPath: "none",
			Reason:        fmt.Sprintf("Governance policy blocked execution of %q", toolName),
		}
	}

	// Type D — Missing auth / credentials
	if strings.Contains(lower, "auth") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "api key") ||
		strings.Contains(lower, "token") && strings.Contains(lower, "missing") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "401") ||
		strings.Contains(lower, "403") {
		return store.GapTypeMissingAuthConfig, store.GapClassification{
			GapClass:      store.GapTypeMissingAuthConfig,
			ExpansionPath: "none",
			Reason:        fmt.Sprintf("Authentication/credentials missing for %q", toolName),
		}
	}

	// Fallback — default to type A (missing skill) since most tool failures
	// in practice stem from the skill not being fully implemented.
	return store.GapTypeMissingSkill, store.GapClassification{
		GapClass:      store.GapTypeMissingSkill,
		ExpansionPath: "skill",
		Reason:        fmt.Sprintf("Tool %q execution failed (unclassified error)", toolName),
	}
}

// insertUniqueGap records a gap only if no open gap with the same type and
// matching evidence already exists. This prevents duplicate gap records from
// repeated failures of the same tool or capability.
func (d *GapDetector) insertUniqueGap(ctx context.Context, gapType store.GapType, classification store.GapClassification, evidence store.GapEvidence) (*store.Gap, error) {
	if d.db == nil {
		return nil, nil
	}

	openGaps, err := store.ListOpenGaps(ctx, d.db, 100)
	if err == nil {
		for _, g := range openGaps {
			if g.Type != gapType {
				continue
			}
			// Match on session + tool error (Signal B) or session + raw context (Signal A)
			if evidence.ToolError != "" && g.Evidence.ToolError == evidence.ToolError {
				slog.Debug("gap_detector: duplicate gap (tool error match)", "type", gapType, "chat_id", evidence.ChatID)
				return nil, nil
			}
			if evidence.RawContext != "" && g.Evidence.ChatID == evidence.ChatID && g.Evidence.RawContext == evidence.RawContext {
				slog.Debug("gap_detector: duplicate gap (context match)", "type", gapType, "chat_id", evidence.ChatID)
				return nil, nil
			}
		}
	}

	gap := store.Gap{
		ID:                   uuid.New().String(),
		Type:                 gapType,
		Status:               store.GapStatusClassified,
		Evidence:             evidence,
		ClassificationResult: classification,
	}

	slog.Info("gap_detector: recorded capability gap",
		"id", gap.ID,
		"type", gap.Type,
		"class", classification.GapClass,
		"expansion_path", classification.ExpansionPath,
		"reason", classification.Reason,
		"chat_id", evidence.ChatID,
	)
	if err := store.InsertGap(ctx, d.db, gap); err != nil {
		return nil, err
	}
	return &gap, nil
}

// isInabilityResponse checks the LLM's response text for heuristic markers
// indicating it could not fulfill the user's request due to a missing capability.
func isInabilityResponse(content string) bool {
	lower := strings.ToLower(content)
	markers := []string{
		// Skill/capability absence
		"i do not have a skill",
		"i don't have a skill",
		"i don't have a tool",
		"i do not have a tool",
		"no skill available",
		"no tool available",
		"i lack the ability",
		"i lack the capability",
		"i don't have the capability",
		"i do not have the capability",
		"i don't have the ability",
		"i do not have the ability",
		"i don't currently have access to",
		"i do not currently have access to",
		// Action inability
		"i am unable to",
		"i'm unable to",
		"i cannot perform that",
		"i can't perform that",
		"i cannot do that",
		"i can't do that",
		"i don't know how to",
		"i do not know how to",
		"i'm not able to",
		"i am not able to",
		// Missing integration
		"i don't have access to your",
		"i do not have access to your",
		"no integration with",
		"not connected to",
		"i cannot connect to",
		"i can't connect to",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// truncate returns at most maxLen bytes of s, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
