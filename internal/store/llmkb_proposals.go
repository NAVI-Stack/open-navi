package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/llmkb"
	"github.com/google/uuid"
)

// SaveRoutingProposal persists a single routing proposal row.
func (r *SQLiteLLMKBRepo) SaveRoutingProposal(ctx context.Context, p llmkb.RoutingProposalItem) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO routing_proposal_items (
			proposal_id, llm_id, proposed_change, affected_field, current_value_json, proposed_value_json,
			inferred_score, sample_size, probation_elapsed_days,
			recent_success_rate, fallback_rate_delta,
			routing_impact, risk_assessment, evidence_record_ids_json,
			status, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ProposalID, p.LLMID, p.ProposedChange, p.AffectedField, marshalJSON(p.CurrentValue), marshalJSON(p.ProposedValue),
		p.InferredScore, p.SampleSize, p.ProbationElapsedDays,
		p.RecentSuccessRate, p.FallbackRateDelta,
		p.RoutingImpact, p.RiskAssessment, marshalJSON(p.EvidenceRecordIDs),
		p.Status, p.CreatedAt.UTC().Format(timeFormat))
	return err
}

// GenerateRoutingProposals identifies high-performing models and suggests them as preferred for specific task classes.
func (r *SQLiteLLMKBRepo) GenerateRoutingProposals(ctx context.Context, threshold float64) ([]llmkb.RoutingProposalItem, error) {
	pendingKeys, err := r.pendingRoutingProposalKeys(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT 
			task_class,
			llm_id,
			AVG(CASE WHEN outcome = 'success' THEN 1.0 ELSE 0.0 END) as success_rate,
			COUNT(*) as total_executions
		FROM llm_execution_records
		GROUP BY task_class, llm_id
		HAVING success_rate >= ? AND total_executions >= 10
	`

	rows, err := r.db.QueryContext(ctx, query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		taskClass   llmkb.TaskClass
		llmID       string
		successRate float64
		sampleSize  int
	}
	var candidates []candidate
	var proposals []llmkb.RoutingProposalItem
	for rows.Next() {
		var tc string
		var lid string
		var sr float64
		var total int
		if err := rows.Scan(&tc, &lid, &sr, &total); err != nil {
			continue
		}

		taskClass, err := llmkb.ParseTaskClass(tc)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			taskClass:   taskClass,
			llmID:       lid,
			successRate: sr,
			sampleSize:  total,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	for _, item := range candidates {
		p, err := r.GetProfile(ctx, item.llmID)
		if err != nil || p == nil {
			continue
		}

		isPreferred := false
		for _, pref := range p.Routing.PreferredFor {
			if pref == item.taskClass {
				isPreferred = true
				break
			}
		}

		if isPreferred {
			continue
		}

		// Promotion constraints (v3 migration)
		thresholdToUse := threshold
		if p.Routing.PromotionThreshold > 0 {
			thresholdToUse = p.Routing.PromotionThreshold
		}
		minSamples := 10
		if p.Routing.PromotionMinSamples > 0 {
			minSamples = p.Routing.PromotionMinSamples
		}

		if item.successRate < thresholdToUse || item.sampleSize < minSamples {
			continue
		}

		// Probation check
		probationDays := 0
		if p.UsageStats.FirstSeenAt != nil {
			probationDays = int(time.Since(*p.UsageStats.FirstSeenAt).Hours() / 24)
		}
		if p.Routing.PromotionProbationDays > 0 && probationDays < p.Routing.PromotionProbationDays {
			continue
		}

		key := routingProposalKey(item.llmID, item.taskClass)
		if _, exists := pendingKeys[key]; exists {
			continue
		}

		// Calculate fallback impact (simple heuristic for now)
		fallbackDelta := 0.0
		if p.UsageStats.FallbackRate > 0 {
			fallbackDelta = -0.05 // Hypothesize a 5% reduction in fallbacks if promoted
		}

		proposal := llmkb.RoutingProposalItem{
			ProposalID:           uuid.New().String(),
			LLMID:                item.llmID,
			AffectedField:        "routing.preferred_for",
			CurrentValue:         p.Routing.PreferredFor,
			ProposedValue:        item.taskClass,
			ProposedChange:       fmt.Sprintf("Add %s to preferred tasks for %s", item.taskClass, item.llmID),
			Status:               llmkb.ProposalStatusPending,
			InferredScore:        item.successRate,
			SampleSize:           item.sampleSize,
			ProbationElapsedDays: probationDays,
			RecentSuccessRate:    item.successRate,
			FallbackRateDelta:    fallbackDelta,
			CreatedAt:            time.Now().UTC(),
		}

		if err := r.SaveRoutingProposal(ctx, proposal); err != nil {
			continue
		}
		pendingKeys[key] = struct{}{}
		proposals = append(proposals, proposal)
	}

	return proposals, nil
}

func (r *SQLiteLLMKBRepo) pendingRoutingProposalKeys(ctx context.Context) (map[string]struct{}, error) {
	existing, err := r.ListRoutingProposals(ctx)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{}, len(existing))
	for _, proposal := range existing {
		if proposal.Status != llmkb.ProposalStatusPending {
			continue
		}
		if proposal.AffectedField == "routing.preferred_for" {
			val, ok := proposal.ProposedValue.(string)
			if ok {
				keys[routingProposalKey(proposal.LLMID, llmkb.TaskClass(val))] = struct{}{}
			}
		}
	}
	return keys, nil
}

func routingProposalKey(llmID string, taskClass llmkb.TaskClass) string {
	return fmt.Sprintf("%s|%s", llmID, taskClass)
}
