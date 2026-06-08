package store

import (
	"context"

	"github.com/ceoai/navi/internal/llmkb"
)

// MaterializeUsageStats aggregates raw execution records into profile usage stats.
func (r *SQLiteLLMKBRepo) MaterializeUsageStats(ctx context.Context) error {
	profiles, err := r.ListProfiles(ctx)
	if err != nil {
		return err
	}

	for _, p := range profiles {
		stats, err := r.computeStatsForProfile(ctx, &p)
		if err != nil {
			continue
		}

		p.UsageStats = *stats

		if err := r.SaveProfile(ctx, p); err != nil {
			return err
		}
	}

	return nil
}

func (r *SQLiteLLMKBRepo) computeStatsForProfile(ctx context.Context, profile *llmkb.LLMProfile) (*llmkb.UsageStats, error) {
	query := `
		SELECT 
			COUNT(*),
			AVG(CASE WHEN outcome = 'success' THEN 1.0 ELSE 0.0 END),
			AVG(latency_ms),
			AVG(context_size_tokens),
			AVG(output_size_tokens),
			SUM(context_size_tokens),
			SUM(output_size_tokens)
		FROM llm_execution_records
		WHERE llm_id = ?
	`
	
	var total int
	var successRate, avgDuration, avgInput, avgOutput float64
	var sumInput, sumOutput int
	
	err := r.db.QueryRowContext(ctx, query, profile.LLMID).Scan(&total, &successRate, &avgDuration, &avgInput, &avgOutput, &sumInput, &sumOutput)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		return &llmkb.UsageStats{}, nil
	}

	var accCost, avgCost float64
	if profile.OperationalState.CurrentCostEstimate != nil {
		inputPrice := profile.OperationalState.CurrentCostEstimate.InputPer1M / 1000.0
		outputPrice := profile.OperationalState.CurrentCostEstimate.OutputPer1M / 1000.0
		accCost = (float64(sumInput) / 1000.0 * inputPrice) + (float64(sumOutput) / 1000.0 * outputPrice)
		avgCost = accCost / float64(total)
	}

	return &llmkb.UsageStats{
		TotalExecutions:     total,
		SuccessRate:         successRate,
		AverageLatencyMS:    avgDuration,
		AverageInputTokens:  avgInput,
		AverageOutputTokens: avgOutput,
		AverageCost:         avgCost,
		AccumulatedCost:     accCost,
	}, nil
}
