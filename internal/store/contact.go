package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// GetContact returns a contact by ID.
func GetContact(ctx context.Context, db *sql.DB, id string) (schema.Contact, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, kind, owner_type, trust_level, metadata, created_at, updated_at
		FROM contacts WHERE id = ?
	`, id)
	var c schema.Contact
	var createdStr, updatedStr string
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.OwnerType, &c.TrustLevel, &c.Metadata, &createdStr, &updatedStr); err != nil {
		if err == sql.ErrNoRows {
			return schema.Contact{}, fmt.Errorf("store: contact not found: %w", err)
		}
		return schema.Contact{}, err
	}
	c.CreatedAt, _ = parseTime(createdStr)
	c.UpdatedAt, _ = parseTime(updatedStr)
	return c, nil
}

// SaveContact inserts or updates a contact.
func SaveContact(ctx context.Context, db *sql.DB, c schema.Contact) error {
	if c.ID == "" {
		return fmt.Errorf("store: contact id required")
	}
	if c.OwnerType == "" {
		c.OwnerType = schema.ContactOwnerTypeNavi
	}
	if c.TrustLevel == "" {
		c.TrustLevel = schema.ContactTrustLevelUnknown
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := db.ExecContext(ctx, `
		INSERT INTO contacts (id, name, kind, owner_type, trust_level, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			kind=excluded.kind,
			owner_type=excluded.owner_type,
			trust_level=excluded.trust_level,
			metadata=excluded.metadata,
			updated_at=excluded.updated_at
	`, c.ID, c.Name, c.Kind, c.OwnerType, c.TrustLevel, c.Metadata, c.CreatedAt.Format(timeFormat), c.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: save contact: %w", err)
	}
	return nil
}

// DeleteContact permanently removes a contact by ID.
func DeleteContact(ctx context.Context, db *sql.DB, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM contacts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete contact: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete contact rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: contact not found: %w", sql.ErrNoRows)
	}
	return nil
}

// ListContactsFilter controls which contacts ListContacts returns.
type ListContactsFilter struct {
	OwnerType  string // "" = all; "navi" or "owner"
	Kind       string // "" = all kinds
	TrustLevel string // "" = all trust levels; "unknown", "low", "medium", "high", "verified"
	NamePrefix string // "" = no prefix filter
	Limit      int    // 0 defaults to 50
}

// ListContacts returns contacts matching the filter, ordered by name ascending.
func ListContacts(ctx context.Context, db *sql.DB, f ListContactsFilter) ([]schema.Contact, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	query := `SELECT id, name, kind, owner_type, trust_level, metadata, created_at, updated_at FROM contacts WHERE 1=1`
	args := []any{}
	if f.OwnerType != "" {
		query += ` AND owner_type = ?`
		args = append(args, f.OwnerType)
	}
	if f.Kind != "" {
		query += ` AND kind = ?`
		args = append(args, f.Kind)
	}
	if f.TrustLevel != "" {
		query += ` AND trust_level = ?`
		args = append(args, f.TrustLevel)
	}
	if f.NamePrefix != "" {
		query += ` AND name LIKE ?`
		args = append(args, f.NamePrefix+"%")
	}
	query += ` ORDER BY name ASC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list contacts: %w", err)
	}
	defer rows.Close()
	return scanContacts(rows)
}

// SearchContactsFilter controls which contacts SearchContacts returns.
type SearchContactsFilter struct {
	Query     string // required; matched against name and metadata blob
	OwnerType string // "" = all
	Limit     int    // 0 defaults to 20
}

// SearchContacts finds contacts whose name or metadata blob matches the query string.
// The metadata match is a simple LIKE scan on the JSON blob — adequate for personal-assistant scale.
func SearchContacts(ctx context.Context, db *sql.DB, f SearchContactsFilter) ([]schema.Contact, error) {
	if strings.TrimSpace(f.Query) == "" {
		return nil, fmt.Errorf("store: search query required")
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	pattern := "%" + f.Query + "%"
	query := `
		SELECT id, name, kind, owner_type, trust_level, metadata, created_at, updated_at
		FROM contacts
		WHERE (name LIKE ? OR metadata LIKE ?)
	`
	args := []any{pattern, pattern}
	if f.OwnerType != "" {
		query += ` AND owner_type = ?`
		args = append(args, f.OwnerType)
	}
	query += ` ORDER BY name ASC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: search contacts: %w", err)
	}
	defer rows.Close()
	return scanContacts(rows)
}

// ListContactsByKindAndPrefix returns contacts of a given kind whose name starts with the given prefix.
func ListContactsByKindAndPrefix(ctx context.Context, db *sql.DB, kind, namePrefix string, limit int) ([]schema.Contact, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, kind, owner_type, trust_level, metadata, created_at, updated_at
		FROM contacts
		WHERE kind = ? AND name LIKE ?
		ORDER BY name ASC
		LIMIT ?
	`, kind, namePrefix+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("store: list contacts: %w", err)
	}
	defer rows.Close()
	return scanContacts(rows)
}

func scanContacts(rows *sql.Rows) ([]schema.Contact, error) {
	var out []schema.Contact
	for rows.Next() {
		var (
			c          schema.Contact
			createdStr string
			updatedStr string
		)
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.OwnerType, &c.TrustLevel, &c.Metadata, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan contact: %w", err)
		}
		c.CreatedAt, _ = parseTime(createdStr)
		c.UpdatedAt, _ = parseTime(updatedStr)
		out = append(out, c)
	}
	return out, rows.Err()
}
