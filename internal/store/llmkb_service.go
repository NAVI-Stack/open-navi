package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/ceoai/navi/internal/llm"
)

// LLMKBMaintenanceService manages background maintenance tasks for the LLM Knowledge Base.
type LLMKBMaintenanceService struct {
	Repo            *SQLiteLLMKBRepo
	CatalogBuilder  func(ctx context.Context) llm.LLMCatalog
	Interval        time.Duration
	RetentionPeriod time.Duration
	Logger          *slog.Logger
}

// NewLLMKBMaintenanceService creates a new maintenance service.
func NewLLMKBMaintenanceService(repo *SQLiteLLMKBRepo, builder func(ctx context.Context) llm.LLMCatalog, interval, retention time.Duration, logger *slog.Logger) *LLMKBMaintenanceService {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if retention <= 0 {
		retention = 90 * 24 * time.Hour // Default 90 days.
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LLMKBMaintenanceService{
		Repo:            repo,
		CatalogBuilder:  builder,
		Interval:        interval,
		RetentionPeriod: retention,
		Logger:          logger,
	}
}

// Start begins the background maintenance loop.
func (s *LLMKBMaintenanceService) Start(ctx context.Context) {
	s.Logger.Info("llmkb: starting maintenance service", "interval", s.Interval)
	
	go func() {
		ticker := time.NewTicker(s.Interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				s.Logger.Info("llmkb: stopping maintenance service")
				return
			case <-ticker.C:
				s.runOnce(ctx)
			}
		}
	}()
}

func (s *LLMKBMaintenanceService) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("llmkb: maintenance loop panic", "recover", r)
		}
	}()

	catalog := s.CatalogBuilder(ctx)
	
	// Phase 3: Health Refresh
	if err := s.Repo.RefreshRuntimeInstancesFromCatalog(ctx, catalog); err != nil {
		s.Logger.Debug("llmkb: health refresh failed", "error", err)
	}
	
	// Phase 4: Stats Materialization
	if err := s.Repo.MaterializeUsageStats(ctx); err != nil {
		s.Logger.Debug("llmkb: stats materialization failed", "error", err)
	}
	
	// Phase 5: Proposal Generation
	if _, err := s.Repo.GenerateRoutingProposals(ctx, 0.8); err != nil {
		s.Logger.Debug("llmkb: proposal generation failed", "error", err)
	}

	// Phase 6: Data Retention
	if err := s.Repo.PruneExecutionRecords(ctx, s.RetentionPeriod); err != nil {
		s.Logger.Debug("llmkb: telemetry pruning failed", "error", err)
	}
}
