package compaction

type TokenEstimator interface {
	Estimate(messages []Message) int
}

type ConservativeEstimator struct{}

func (ConservativeEstimator) Estimate(messages []Message) int {
	tokens := 0
	for _, msg := range messages {
		tokens += len(msg.Content)/4 + 8
	}
	return tokens
}

type BudgetConfig struct {
	SoftThresholdPct      float64
	HardThresholdPct      float64
	EmergencyThresholdPct float64
	MinimumRecentPairs    int
	DefaultRecentPairs    int
}

type BudgetManager struct {
	estimator TokenEstimator
	cfg       BudgetConfig
}

func NewBudgetManager(estimator TokenEstimator, cfg BudgetConfig) BudgetManager {
	if estimator == nil {
		estimator = ConservativeEstimator{}
	}
	if cfg.SoftThresholdPct == 0 {
		cfg.SoftThresholdPct = 0.60
	}
	if cfg.HardThresholdPct == 0 {
		cfg.HardThresholdPct = 0.75
	}
	if cfg.EmergencyThresholdPct == 0 {
		cfg.EmergencyThresholdPct = 0.90
	}
	if cfg.MinimumRecentPairs == 0 {
		cfg.MinimumRecentPairs = 4
	}
	if cfg.DefaultRecentPairs == 0 {
		cfg.DefaultRecentPairs = 10
	}
	return BudgetManager{estimator: estimator, cfg: cfg}
}

func (b BudgetManager) Snapshot(maxContext, reservedOutput, reservedTools int, messages []Message) BudgetSnapshot {
	usable := maxContext - reservedOutput - reservedTools
	if usable < 0 {
		usable = 0
	}
	est := b.estimator.Estimate(messages)
	return BudgetSnapshot{
		MaxContextTokens:      maxContext,
		ReservedOutputTokens:  reservedOutput,
		ReservedToolHeadroom:  reservedTools,
		UsableTokens:          usable,
		EstimatedPromptTokens: est,
		SoftThreshold:         int(float64(usable) * b.cfg.SoftThresholdPct),
		HardThreshold:         int(float64(usable) * b.cfg.HardThresholdPct),
		EmergencyThreshold:    int(float64(usable) * b.cfg.EmergencyThresholdPct),
	}
}

func (b BudgetManager) TriggerClass(s BudgetSnapshot) TriggerClass {
	if s.EstimatedPromptTokens >= s.EmergencyThreshold {
		return TriggerEmergency
	}
	if s.EstimatedPromptTokens >= s.HardThreshold {
		return TriggerHard
	}
	if s.EstimatedPromptTokens >= s.SoftThreshold {
		return TriggerSoft
	}
	return ""
}
