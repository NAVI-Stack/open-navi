package llm

import (
	"context"

	"github.com/ceoai/navi/internal/llmkb"
	"github.com/ceoai/navi/internal/schema"
)

// LLMKBRepo is the subset of the LLM knowledge-base repository that the
// control plane needs for telemetry and profile enrichment. This interface
// keeps internal/llm independent of internal/store.
type LLMKBRepo interface {
	EnsureRuntimeProfile(ctx context.Context, providerID, modelID string) (*llmkb.LLMProfile, error)
	ResolveProfileForProviderModel(ctx context.Context, provider, model string) (*llmkb.LLMProfile, error)
	AppendRouterDecision(ctx context.Context, d llmkb.RouterDecision) error
	AppendExecutionRecord(ctx context.Context, r llmkb.LLMExecutionRecord) error
}

// ExecutionOutcomeStore is the subset of the store layer that the control plane
// needs for calibration data. Keeps internal/llm independent of internal/store.
type ExecutionOutcomeStore interface {
	ListCalibratedLLMExecutionOutcomes(ctx context.Context, limit int) ([]schema.ExecutionOutcome, error)
}

// ProviderOperationStore persists provider operation records.
type ProviderOperationStore interface {
	SaveProviderOperation(ctx context.Context, op ProviderOperation) error
	UpdateProviderOperation(ctx context.Context, id string, status OperationStatus, progress float64, msg, errMsg string) error
	GetProviderOperation(ctx context.Context, id string) (*ProviderOperation, error)
	ListProviderOperations(ctx context.Context, provider string, limit int) ([]ProviderOperation, error)
}

// HealthSnapshotStore persists provider health snapshots.
type HealthSnapshotStore interface {
	SaveHealthSnapshot(ctx context.Context, h ProviderHealth) error
	LatestHealthSnapshot(ctx context.Context, provider string) (*ProviderHealth, error)
}
