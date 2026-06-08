package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

const (
	defaultSandboxProfileTimeoutMS = 30000
	defaultSandboxProfileMaxMS     = 600000
	defaultWorkspaceMountTarget    = "/workspace"
)

// SaveSandboxProfile inserts or updates a sandbox profile.
func SaveSandboxProfile(ctx context.Context, db *sql.DB, profile schema.SandboxProfile) error {
	normalized, err := normalizeAndValidateSandboxProfile(profile)
	if err != nil {
		return err
	}
	commands, err := json.Marshal(normalized.CommandAllowlist)
	if err != nil {
		return fmt.Errorf("store: marshal sandbox command_allowlist: %w", err)
	}
	env, err := json.Marshal(normalized.EnvAllowlist)
	if err != nil {
		return fmt.Errorf("store: marshal sandbox env_allowlist: %w", err)
	}
	metadata := strings.TrimSpace(normalized.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return &ValidationError{Entity: "sandbox_profile", Field: "metadata", Message: "must be valid JSON"}
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO sandbox_profiles (
			id, name, description, runtime, status, image, network_mode,
			workspace_mount_target, command_allowlist, env_allowlist,
			default_timeout_ms, max_timeout_ms, cpu_limit, memory_limit,
			created_at, updated_at, created_by, metadata
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			runtime = excluded.runtime,
			status = excluded.status,
			image = excluded.image,
			network_mode = excluded.network_mode,
			workspace_mount_target = excluded.workspace_mount_target,
			command_allowlist = excluded.command_allowlist,
			env_allowlist = excluded.env_allowlist,
			default_timeout_ms = excluded.default_timeout_ms,
			max_timeout_ms = excluded.max_timeout_ms,
			cpu_limit = excluded.cpu_limit,
			memory_limit = excluded.memory_limit,
			updated_at = excluded.updated_at,
			created_by = excluded.created_by,
			metadata = excluded.metadata
	`, normalized.ID, normalized.Name, normalized.Description, normalized.Runtime, normalized.Status, normalized.Image,
		normalized.NetworkMode, normalized.WorkspaceMountTarget, string(commands), string(env),
		normalized.DefaultTimeoutMS, normalized.MaxTimeoutMS, normalized.CPULimit, normalized.MemoryLimit,
		normalized.CreatedAt.UTC().Format(timeFormat), normalized.UpdatedAt.UTC().Format(timeFormat),
		normalized.CreatedBy, metadata)
	if err != nil {
		return fmt.Errorf("store: save sandbox profile: %w", err)
	}
	return nil
}

// GetSandboxProfile retrieves one sandbox profile.
func GetSandboxProfile(ctx context.Context, db *sql.DB, id string) (schema.SandboxProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return schema.SandboxProfile{}, &ValidationError{Entity: "sandbox_profile", Field: "sandbox_profile_id", Message: "is required"}
	}
	row := db.QueryRowContext(ctx, `
		SELECT id, name, description, runtime, status, image, network_mode,
			workspace_mount_target, command_allowlist, env_allowlist,
			default_timeout_ms, max_timeout_ms, cpu_limit, memory_limit,
			created_at, updated_at, created_by, metadata
		FROM sandbox_profiles
		WHERE id = ?
	`, id)
	profile, err := scanSandboxProfile(row)
	if err == sql.ErrNoRows {
		return schema.SandboxProfile{}, fmt.Errorf("store: sandbox profile not found: %s", id)
	}
	if err != nil {
		return schema.SandboxProfile{}, err
	}
	return profile, nil
}

// ListSandboxProfiles returns sandbox profiles, optionally filtered by status.
func ListSandboxProfiles(ctx context.Context, db *sql.DB, status schema.SandboxProfileStatus) ([]schema.SandboxProfile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if status != "" {
		if !status.IsValid() {
			return nil, &ValidationError{Entity: "sandbox_profile", Field: "status", Message: "invalid sandbox profile status"}
		}
		rows, err = db.QueryContext(ctx, `
			SELECT id, name, description, runtime, status, image, network_mode,
				workspace_mount_target, command_allowlist, env_allowlist,
				default_timeout_ms, max_timeout_ms, cpu_limit, memory_limit,
				created_at, updated_at, created_by, metadata
			FROM sandbox_profiles
			WHERE status = ?
			ORDER BY updated_at DESC, id ASC
		`, status)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT id, name, description, runtime, status, image, network_mode,
				workspace_mount_target, command_allowlist, env_allowlist,
				default_timeout_ms, max_timeout_ms, cpu_limit, memory_limit,
				created_at, updated_at, created_by, metadata
			FROM sandbox_profiles
			ORDER BY updated_at DESC, id ASC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list sandbox profiles: %w", err)
	}
	defer rows.Close()

	var profiles []schema.SandboxProfile
	for rows.Next() {
		profile, err := scanSandboxProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list sandbox profile rows: %w", err)
	}
	if profiles == nil {
		profiles = []schema.SandboxProfile{}
	}
	return profiles, nil
}

type sandboxProfileScanner interface {
	Scan(dest ...any) error
}

func scanSandboxProfile(scanner sandboxProfileScanner) (schema.SandboxProfile, error) {
	var (
		profile                  schema.SandboxProfile
		description              sql.NullString
		runtime, status, network string
		commandJSON, envJSON     string
		createdAt, updatedAt     string
	)
	if err := scanner.Scan(&profile.ID, &profile.Name, &description, &runtime, &status, &profile.Image, &network,
		&profile.WorkspaceMountTarget, &commandJSON, &envJSON, &profile.DefaultTimeoutMS, &profile.MaxTimeoutMS,
		&profile.CPULimit, &profile.MemoryLimit, &createdAt, &updatedAt, &profile.CreatedBy, &profile.Metadata); err != nil {
		return schema.SandboxProfile{}, err
	}
	profile.Description = description.String
	profile.Runtime = schema.SandboxRuntime(runtime)
	profile.Status = schema.SandboxProfileStatus(status)
	profile.NetworkMode = schema.SandboxNetworkMode(network)
	if commandJSON == "" {
		commandJSON = "[]"
	}
	if envJSON == "" {
		envJSON = "[]"
	}
	if err := json.Unmarshal([]byte(commandJSON), &profile.CommandAllowlist); err != nil {
		return schema.SandboxProfile{}, fmt.Errorf("store: decode sandbox command_allowlist: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &profile.EnvAllowlist); err != nil {
		return schema.SandboxProfile{}, fmt.Errorf("store: decode sandbox env_allowlist: %w", err)
	}
	if createdAt != "" {
		if ts, err := parseTime(createdAt); err == nil {
			profile.CreatedAt = ts
		}
	}
	if updatedAt != "" {
		if ts, err := parseTime(updatedAt); err == nil {
			profile.UpdatedAt = ts
		}
	}
	if strings.TrimSpace(profile.Metadata) == "" {
		profile.Metadata = "{}"
	}
	return profile, nil
}

func normalizeAndValidateSandboxProfile(profile schema.SandboxProfile) (schema.SandboxProfile, error) {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Image = strings.TrimSpace(profile.Image)
	profile.WorkspaceMountTarget = strings.TrimSpace(profile.WorkspaceMountTarget)
	profile.CPULimit = strings.TrimSpace(profile.CPULimit)
	profile.MemoryLimit = strings.TrimSpace(profile.MemoryLimit)
	profile.CreatedBy = strings.TrimSpace(profile.CreatedBy)
	if profile.ID == "" {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "sandbox_profile_id", Message: "is required"}
	}
	if profile.Name == "" {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "name", Message: "is required"}
	}
	if profile.Runtime == "" {
		profile.Runtime = schema.SandboxRuntimeDocker
	}
	if parsed, err := schema.ParseSandboxRuntime(string(profile.Runtime)); err == nil {
		profile.Runtime = parsed
	} else {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "runtime", Message: err.Error()}
	}
	if profile.Status == "" {
		profile.Status = schema.SandboxProfileStatusActive
	}
	if parsed, err := schema.ParseSandboxProfileStatus(string(profile.Status)); err == nil {
		profile.Status = parsed
	} else {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "status", Message: err.Error()}
	}
	if profile.NetworkMode == "" {
		profile.NetworkMode = schema.SandboxNetworkNone
	}
	if parsed, err := schema.ParseSandboxNetworkMode(string(profile.NetworkMode)); err == nil {
		profile.NetworkMode = parsed
	} else {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "network_mode", Message: err.Error()}
	}
	if profile.Image == "" {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "image", Message: "is required"}
	}
	if profile.WorkspaceMountTarget == "" {
		profile.WorkspaceMountTarget = defaultWorkspaceMountTarget
	}
	if !strings.HasPrefix(profile.WorkspaceMountTarget, "/") {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "workspace_mount_target", Message: "must be an absolute container path"}
	}
	profile.CommandAllowlist = cleanStringList(profile.CommandAllowlist)
	profile.EnvAllowlist = cleanStringList(profile.EnvAllowlist)
	if profile.DefaultTimeoutMS <= 0 {
		profile.DefaultTimeoutMS = defaultSandboxProfileTimeoutMS
	}
	if profile.MaxTimeoutMS <= 0 {
		profile.MaxTimeoutMS = defaultSandboxProfileMaxMS
	}
	if profile.MaxTimeoutMS < profile.DefaultTimeoutMS {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "max_timeout_ms", Message: "must be >= default_timeout_ms"}
	}
	now := time.Now().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = profile.CreatedAt
	}
	if profile.CreatedBy == "" {
		return profile, &ValidationError{Entity: "sandbox_profile", Field: "created_by", Message: "is required"}
	}
	if strings.TrimSpace(profile.Metadata) == "" {
		profile.Metadata = "{}"
	}
	return profile, nil
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
