package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Owner represents the single owner of a NAVI instance.
type Owner struct {
	ID                string // uuid-style hex
	InstanceID        string // stable per-install identifier
	Name              string // display name ("Eric")
	Handle            string // short handle ("evirg")
	DeviceName        string // optional ("Eric's Mac")
	SecretFingerprint string // first 8 chars of owner secret hash — safe to display
	CreatedAt         time.Time
	Timezone          string // IANA timezone name (e.g. "America/New_York")
}

// CreateOwner inserts the owner record. Fails if an owner already exists.
func CreateOwner(ctx context.Context, db *sql.DB, o Owner) error {
	if o.ID == "" {
		o.ID = newKeyID()
	}
	if o.InstanceID == "" {
		o.InstanceID = newInstanceID()
	}
	if o.Timezone == "" {
		o.Timezone = "UTC"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO owners (id, instance_id, name, handle, device_name, secret_fingerprint, created_at, timezone)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.InstanceID, o.Name, o.Handle, o.DeviceName, o.SecretFingerprint,
		o.CreatedAt.UTC().Format(timeFormat), o.Timezone,
	)
	if err != nil {
		return fmt.Errorf("store: create owner: %w", err)
	}
	return nil
}

// GetOwner returns the single owner record, or found=false if not yet created.
func GetOwner(ctx context.Context, db *sql.DB) (Owner, bool, error) {
	var o Owner
	var createdRaw string
	err := db.QueryRowContext(ctx, `
		SELECT id, instance_id, name, handle, device_name, secret_fingerprint, created_at, timezone
		FROM owners LIMIT 1`).
		Scan(&o.ID, &o.InstanceID, &o.Name, &o.Handle, &o.DeviceName, &o.SecretFingerprint, &createdRaw, &o.Timezone)
	if err == sql.ErrNoRows {
		return Owner{}, false, nil
	}
	if err != nil {
		return Owner{}, false, fmt.Errorf("store: get owner: %w", err)
	}
	o.CreatedAt, _ = parseTime(createdRaw)
	return o, true, nil
}

// GetOwnerID returns the single owner's ID, or empty string if no owner exists.
func GetOwnerID(ctx context.Context, db *sql.DB) (string, error) {
	o, ok, err := GetOwner(ctx, db)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return o.ID, nil
}

// OwnerExists returns true if the owner has been created.
func OwnerExists(ctx context.Context, db *sql.DB) (bool, error) {
	_, found, err := GetOwner(ctx, db)
	return found, err
}

// UpdateOwnerTimezone updates the single owner's timezone preference.
func UpdateOwnerTimezone(ctx context.Context, db *sql.DB, timezone string) error {
	_, err := db.ExecContext(ctx, `UPDATE owners SET timezone = ?`, timezone)
	if err != nil {
		return fmt.Errorf("store: update owner timezone: %w", err)
	}
	return nil
}

// newInstanceID generates a stable random instance identifier.
func newInstanceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "navi_inst_" + hex.EncodeToString(b)
}
