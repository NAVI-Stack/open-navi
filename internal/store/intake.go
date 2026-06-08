package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

// ErrIntakeDuplicate is returned by SaveIntakeRecord when the
// (connector_id, source_id) pair already exists in intake_records.
var ErrIntakeDuplicate = errors.New("store: intake record already exists")

// SaveIntakeRecord inserts r into intake_records.
// Returns ErrIntakeDuplicate if the (connector_id, source_id) pair already exists.
func SaveIntakeRecord(ctx context.Context, db *sql.DB, r schema.IntakeRecord) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	provJSON, err := json.Marshal(r.Provenance)
	if err != nil {
		return fmt.Errorf("store: save intake record: marshal provenance: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO intake_records
			(id, connector_id, source_kind, source_id, cursor, fetched_at,
			 trust, privacy_class, author, raw, raw_mime, provenance, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ConnectorID, r.SourceKind, r.SourceID, r.Cursor,
		r.FetchedAt.Format(timeFormat), string(r.Trust), string(r.PrivacyClass),
		r.Author, r.Raw, r.RawMIME, string(provJSON),
		r.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		if isIntakeUniqueViolation(err) {
			return ErrIntakeDuplicate
		}
		return fmt.Errorf("store: save intake record: %w", err)
	}
	return nil
}

// GetIntakeRecordCursor returns just the opaque cursor for a record id, the
// lightweight lookup P4 retrieval uses to attach cursor provenance to a result
// without loading the full raw payload. Returns ("", nil) when the record is
// missing.
func GetIntakeRecordCursor(ctx context.Context, db *sql.DB, recordID string) (string, error) {
	var cursor string
	err := db.QueryRowContext(ctx, `SELECT cursor FROM intake_records WHERE id = ?`, recordID).Scan(&cursor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get intake record cursor: %w", err)
	}
	return cursor, nil
}

// IntakeRecordSummary is a slim, provenance-bearing view of an intake record
// (no Raw blob) for the Console "recent intake" surface (CIP §11). Each row
// links back to the source via LinkBack.
type IntakeRecordSummary struct {
	RecordID     string              `json:"record_id"`
	ConnectorID  string              `json:"connector_id"`
	SourceKind   string              `json:"source_kind"`
	SourceID     string              `json:"source_id"`
	Author       string              `json:"author,omitempty"`
	LinkBack     string              `json:"link_back,omitempty"`
	Trust        schema.ContentTrust `json:"trust"`
	PrivacyClass schema.PrivacyClass `json:"privacy_class"`
	FetchedAt    time.Time           `json:"fetched_at"`
	ChunkCount   int                 `json:"chunk_count"`
}

// ListRecentIntakeRecords returns the most recently created intake records
// (metadata only) with their distilled-chunk counts. connectorID == "" returns
// all connectors. Powers the right-inspector recent-intake panel.
func ListRecentIntakeRecords(ctx context.Context, db *sql.DB, connectorID string, limit int) ([]IntakeRecordSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	const cols = `r.id, r.connector_id, r.source_kind, r.source_id, r.author,
		r.provenance, r.trust, r.privacy_class, r.fetched_at,
		(SELECT COUNT(1) FROM intake_chunks c WHERE c.intake_record_id = r.id)`
	var (
		rows *sql.Rows
		err  error
	)
	if connectorID == "" {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM intake_records r ORDER BY r.created_at DESC LIMIT ?`, limit)
	} else {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM intake_records r WHERE r.connector_id = ?
			ORDER BY r.created_at DESC LIMIT ?`, connectorID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list recent intake records: %w", err)
	}
	defer rows.Close()

	var out []IntakeRecordSummary
	for rows.Next() {
		var (
			s         IntakeRecordSummary
			provJSON  string
			trust     string
			privacy   string
			fetchedAt string
		)
		if err := rows.Scan(&s.RecordID, &s.ConnectorID, &s.SourceKind, &s.SourceID, &s.Author,
			&provJSON, &trust, &privacy, &fetchedAt, &s.ChunkCount); err != nil {
			return nil, fmt.Errorf("store: scan recent intake record: %w", err)
		}
		s.Trust = schema.ContentTrust(trust)
		s.PrivacyClass = schema.PrivacyClass(privacy)
		s.FetchedAt, _ = parseTime(fetchedAt)
		var prov schema.IntakeProvenance
		if json.Unmarshal([]byte(provJSON), &prov) == nil {
			s.LinkBack = prov.LinkBack
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountIntakeRecordsByConnector returns how many records a connector has landed.
// Used by backfill consent (CIP §7.1) to decide between a full historical sweep
// and a resume-from-cursor Proposal: a non-zero count means historical state
// already exists.
func CountIntakeRecordsByConnector(ctx context.Context, db *sql.DB, connectorID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM intake_records WHERE connector_id = ?`, connectorID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count intake records by connector: %w", err)
	}
	return count, nil
}

// LatestCursorByConnector returns the cursor of the most recently created record
// for a connector, for the resume-from-cursor backfill Proposal (CIP §7.1).
func LatestCursorByConnector(ctx context.Context, db *sql.DB, connectorID string) (string, error) {
	var cursor string
	err := db.QueryRowContext(ctx,
		`SELECT cursor FROM intake_records WHERE connector_id = ?
		 ORDER BY created_at DESC LIMIT 1`, connectorID).Scan(&cursor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: latest cursor by connector: %w", err)
	}
	return cursor, nil
}

// IntakeRecordExists reports whether a record with the given (connectorID, sourceID) already exists.
func IntakeRecordExists(ctx context.Context, db *sql.DB, connectorID, sourceID string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM intake_records WHERE connector_id = ? AND source_id = ?`,
		connectorID, sourceID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("store: intake record exists: %w", err)
	}
	return count > 0, nil
}

// GetIntakeRecord fetches a single record by primary key. Returns an error if not found.
func GetIntakeRecord(ctx context.Context, db *sql.DB, id string) (schema.IntakeRecord, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, connector_id, source_kind, source_id, cursor, fetched_at,
		       trust, privacy_class, author, raw, raw_mime, provenance, created_at
		FROM intake_records
		WHERE id = ?`, id)
	return scanIntakeRecord(row)
}

// GetIntakeRecordBySource fetches a record by its (connectorID, sourceID)
// dedupe key. Used by the P2 worker to recover a stored record's id when the
// record was already admitted (resume-after-crash path). Returns an error if
// not found.
func GetIntakeRecordBySource(ctx context.Context, db *sql.DB, connectorID, sourceID string) (schema.IntakeRecord, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, connector_id, source_kind, source_id, cursor, fetched_at,
		       trust, privacy_class, author, raw, raw_mime, provenance, created_at
		FROM intake_records
		WHERE connector_id = ? AND source_id = ?`, connectorID, sourceID)
	return scanIntakeRecord(row)
}

func scanIntakeRecord(row *sql.Row) (schema.IntakeRecord, error) {
	var r schema.IntakeRecord
	var fetchedAt, createdAt, provJSON, trust, privClass string
	if err := row.Scan(
		&r.ID, &r.ConnectorID, &r.SourceKind, &r.SourceID, &r.Cursor,
		&fetchedAt, &trust, &privClass, &r.Author, &r.Raw, &r.RawMIME,
		&provJSON, &createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schema.IntakeRecord{}, fmt.Errorf("store: intake record not found")
		}
		return schema.IntakeRecord{}, fmt.Errorf("store: get intake record: %w", err)
	}
	r.Trust = schema.ContentTrust(trust)
	r.PrivacyClass = schema.PrivacyClass(privClass)
	r.FetchedAt, _ = parseTime(fetchedAt)
	r.CreatedAt, _ = parseTime(createdAt)
	_ = json.Unmarshal([]byte(provJSON), &r.Provenance)
	return r, nil
}

func isIntakeUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "PRIMARY KEY constraint")
}
