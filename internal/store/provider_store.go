package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/llm"
)

// --- Health Snapshots ---

// SaveHealthSnapshot persists a provider health check result.
func SaveHealthSnapshot(ctx context.Context, db *sql.DB, h llm.ProviderHealth) error {
	healthy := 0
	if h.Healthy {
		healthy = 1
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO provider_health_snapshots (provider, healthy, latency_ms, message, model_count, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		h.Provider, healthy, h.Latency, h.Message, h.ModelCount, h.CheckedAt.Format(timeFormat),
	)
	return err
}

// LatestHealthSnapshot returns the most recent health snapshot for a provider.
func LatestHealthSnapshot(ctx context.Context, db *sql.DB, provider string) (*llm.ProviderHealth, error) {
	row := db.QueryRowContext(ctx,
		`SELECT provider, healthy, latency_ms, message, model_count, checked_at
		 FROM provider_health_snapshots
		 WHERE provider = ?
		 ORDER BY checked_at DESC LIMIT 1`,
		provider,
	)

	var h llm.ProviderHealth
	var healthy int
	var checkedAtStr string
	if err := row.Scan(&h.Provider, &healthy, &h.Latency, &h.Message, &h.ModelCount, &checkedAtStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	h.Healthy = healthy == 1
	if t, err := parseTime(checkedAtStr); err == nil {
		h.CheckedAt = t
	}
	return &h, nil
}

// ListHealthSnapshots returns recent health snapshots for a provider.
func ListHealthSnapshots(ctx context.Context, db *sql.DB, provider string, limit int) ([]llm.ProviderHealth, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		`SELECT provider, healthy, latency_ms, message, model_count, checked_at
		 FROM provider_health_snapshots
		 WHERE provider = ?
		 ORDER BY checked_at DESC LIMIT ?`,
		provider, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []llm.ProviderHealth
	for rows.Next() {
		var h llm.ProviderHealth
		var healthy int
		var checkedAtStr string
		if err := rows.Scan(&h.Provider, &healthy, &h.Latency, &h.Message, &h.ModelCount, &checkedAtStr); err != nil {
			return nil, err
		}
		h.Healthy = healthy == 1
		if t, err := parseTime(checkedAtStr); err == nil {
			h.CheckedAt = t
		}
		results = append(results, h)
	}
	return results, rows.Err()
}

// --- Provider Operations ---

// SaveProviderOperation persists a provider operation record.
func SaveProviderOperation(ctx context.Context, db *sql.DB, op llm.ProviderOperation) error {
	var completedAt *string
	if op.CompletedAt != nil {
		s := op.CompletedAt.Format(timeFormat)
		completedAt = &s
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO provider_operations (id, provider, action, model, status, progress, message, error, started_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   status=excluded.status, progress=excluded.progress, message=excluded.message,
		   error=excluded.error, completed_at=excluded.completed_at`,
		op.ID, op.Provider, op.Action, op.Model, string(op.Status), op.Progress, op.Message, op.Error,
		op.StartedAt.Format(timeFormat), completedAt,
	)
	return err
}

// UpdateProviderOperation updates the status/progress of an existing operation.
func UpdateProviderOperation(ctx context.Context, db *sql.DB, id string, status llm.OperationStatus, progress float64, msg, errMsg string) error {
	var completedAt *string
	if status == llm.OperationStatusCompleted || status == llm.OperationStatusFailed {
		s := time.Now().UTC().Format(timeFormat)
		completedAt = &s
	}
	_, err := db.ExecContext(ctx,
		`UPDATE provider_operations SET status=?, progress=?, message=?, error=?, completed_at=? WHERE id=?`,
		string(status), progress, msg, errMsg, completedAt, id,
	)
	return err
}

// GetProviderOperation retrieves a single operation by ID.
func GetProviderOperation(ctx context.Context, db *sql.DB, id string) (*llm.ProviderOperation, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, provider, action, model, status, progress, message, error, started_at, completed_at
		 FROM provider_operations WHERE id = ?`, id,
	)
	return scanProviderOperation(row)
}

// ListProviderOperations returns recent operations for a provider.
func ListProviderOperations(ctx context.Context, db *sql.DB, provider string, limit int) ([]llm.ProviderOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, provider, action, model, status, progress, message, error, started_at, completed_at
		 FROM provider_operations
		 WHERE provider = ?
		 ORDER BY started_at DESC LIMIT ?`,
		provider, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []llm.ProviderOperation
	for rows.Next() {
		var op llm.ProviderOperation
		var statusStr, startedAtStr string
		var completedAtStr sql.NullString
		if err := rows.Scan(&op.ID, &op.Provider, &op.Action, &op.Model, &statusStr, &op.Progress, &op.Message, &op.Error, &startedAtStr, &completedAtStr); err != nil {
			return nil, err
		}
		op.Status = llm.OperationStatus(statusStr)
		if t, err := parseTime(startedAtStr); err == nil {
			op.StartedAt = t
		}
		if completedAtStr.Valid {
			if t, err := parseTime(completedAtStr.String); err == nil {
				op.CompletedAt = &t
			}
		}
		results = append(results, op)
	}
	return results, rows.Err()
}

func scanProviderOperation(row *sql.Row) (*llm.ProviderOperation, error) {
	var op llm.ProviderOperation
	var statusStr, startedAtStr string
	var completedAtStr sql.NullString
	if err := row.Scan(&op.ID, &op.Provider, &op.Action, &op.Model, &statusStr, &op.Progress, &op.Message, &op.Error, &startedAtStr, &completedAtStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	op.Status = llm.OperationStatus(statusStr)
	if t, err := parseTime(startedAtStr); err == nil {
		op.StartedAt = t
	}
	if completedAtStr.Valid {
		if t, err := parseTime(completedAtStr.String); err == nil {
			op.CompletedAt = &t
		}
	}
	return &op, nil
}

// --- Discovered Models ---

// UpsertDiscoveredModels bulk-upserts discovered models for a provider.
func UpsertDiscoveredModels(ctx context.Context, db *sql.DB, provider string, models []llm.ModelDescriptor) error {
	if len(models) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(timeFormat)
	const batchSize = 100
	const paramsPerModel = 9

	for i := 0; i < len(models); i += batchSize {
		end := i + batchSize
		if end > len(models) {
			end = len(models)
		}
		batch := models[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO discovered_models (provider, name, size_bytes, family, parameter_size, quantization, digest, modified_at, discovered_at) VALUES ")
		args := make([]any, 0, len(batch)*paramsPerModel)

		for j, m := range batch {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?)")

			modifiedAt := ""
			if !m.ModifiedAt.IsZero() {
				modifiedAt = m.ModifiedAt.Format(timeFormat)
			}
			args = append(args, provider, m.Name, m.Size, m.Family, m.ParameterSize, m.QuantLevel, m.Digest, modifiedAt, now)
		}

		sb.WriteString(` ON CONFLICT(provider, name) DO UPDATE SET
		   size_bytes=excluded.size_bytes, family=excluded.family, parameter_size=excluded.parameter_size,
		   quantization=excluded.quantization, digest=excluded.digest, modified_at=excluded.modified_at,
		   discovered_at=excluded.discovered_at`)

		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListDiscoveredModels returns cached model inventory for a provider.
func ListDiscoveredModels(ctx context.Context, db *sql.DB, provider string) ([]llm.ModelDescriptor, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, size_bytes, family, parameter_size, quantization, digest, modified_at
		 FROM discovered_models WHERE provider = ? ORDER BY name`, provider,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []llm.ModelDescriptor
	for rows.Next() {
		var m llm.ModelDescriptor
		var modifiedAtStr string
		if err := rows.Scan(&m.Name, &m.Size, &m.Family, &m.ParameterSize, &m.QuantLevel, &m.Digest, &modifiedAtStr); err != nil {
			return nil, err
		}
		if modifiedAtStr != "" {
			if t, err := parseTime(modifiedAtStr); err == nil {
				m.ModifiedAt = t
			}
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// --- Store Adapters (implement llm.ProviderOperationStore and llm.HealthSnapshotStore) ---

// ProviderStoreAdapter wraps a *sql.DB to implement llm store interfaces.
type ProviderStoreAdapter struct {
	DB *sql.DB
}

func (a *ProviderStoreAdapter) SaveProviderOperation(ctx context.Context, op llm.ProviderOperation) error {
	return SaveProviderOperation(ctx, a.DB, op)
}

func (a *ProviderStoreAdapter) UpdateProviderOperation(ctx context.Context, id string, status llm.OperationStatus, progress float64, msg, errMsg string) error {
	return UpdateProviderOperation(ctx, a.DB, id, status, progress, msg, errMsg)
}

func (a *ProviderStoreAdapter) GetProviderOperation(ctx context.Context, id string) (*llm.ProviderOperation, error) {
	return GetProviderOperation(ctx, a.DB, id)
}

func (a *ProviderStoreAdapter) ListProviderOperations(ctx context.Context, provider string, limit int) ([]llm.ProviderOperation, error) {
	return ListProviderOperations(ctx, a.DB, provider, limit)
}

func (a *ProviderStoreAdapter) SaveHealthSnapshot(ctx context.Context, h llm.ProviderHealth) error {
	return SaveHealthSnapshot(ctx, a.DB, h)
}

func (a *ProviderStoreAdapter) LatestHealthSnapshot(ctx context.Context, provider string) (*llm.ProviderHealth, error) {
	return LatestHealthSnapshot(ctx, a.DB, provider)
}
