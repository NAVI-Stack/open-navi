package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// RegisterAgent records an agent as live (retired_at IS NULL).
func RegisterAgent(ctx context.Context, db *sql.DB, agentID string, agentType schema.AgentType) error {
	if agentID == "" {
		return fmt.Errorf("store: agent_id is required")
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `INSERT INTO agents (agent_id, agent_type, registered_at, retired_at) VALUES (?, ?, ?, NULL)
		ON CONFLICT(agent_id) DO UPDATE SET agent_type = excluded.agent_type, retired_at = NULL`,
		agentID, string(agentType), now)
	if err != nil {
		return fmt.Errorf("store: register agent: %w", err)
	}
	return nil
}

// RetireAgent sets retired_at for the agent.
func RetireAgent(ctx context.Context, db *sql.DB, agentID string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := db.ExecContext(ctx, `UPDATE agents SET retired_at = ? WHERE agent_id = ? AND retired_at IS NULL`, now, agentID)
	if err != nil {
		return fmt.Errorf("store: retire agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: agent not found or already retired: %s", agentID)
	}
	return nil
}

// GetAgent returns the agent type and whether the agent is live (not retired).
func GetAgent(ctx context.Context, db *sql.DB, agentID string) (schema.AgentType, bool, error) {
	var agentType string
	var retiredAt sql.NullString
	err := db.QueryRowContext(ctx, `SELECT agent_type, retired_at FROM agents WHERE agent_id = ?`, agentID).Scan(&agentType, &retiredAt)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get agent: %w", err)
	}
	return schema.AgentType(agentType), !retiredAt.Valid, nil
}

// LiveAgent is a live agent instance.
type LiveAgent struct {
	AgentID   string
	AgentType schema.AgentType
}

// ListLiveAgents returns all agents with retired_at IS NULL.
func ListLiveAgents(ctx context.Context, db *sql.DB) ([]LiveAgent, error) {
	rows, err := db.QueryContext(ctx, `SELECT agent_id, agent_type FROM agents WHERE retired_at IS NULL ORDER BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list live agents: %w", err)
	}
	defer rows.Close()
	var out []LiveAgent
	for rows.Next() {
		var a LiveAgent
		var typ string
		if err := rows.Scan(&a.AgentID, &typ); err != nil {
			return nil, fmt.Errorf("store: scan agent: %w", err)
		}
		a.AgentType = schema.AgentType(typ)
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateHeartbeat sets last_heartbeat = now for the agent.
func UpdateHeartbeat(ctx context.Context, db *sql.DB, agentID string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := db.ExecContext(ctx, `UPDATE agents SET last_heartbeat = ? WHERE agent_id = ?`, now, agentID)
	if err != nil {
		return fmt.Errorf("store: update heartbeat: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: agent not found: %s", agentID)
	}
	return nil
}

// ListStaleAgents returns live agents (retired_at IS NULL) whose last_heartbeat is older than olderThan OR is NULL.
func ListStaleAgents(ctx context.Context, db *sql.DB, olderThan time.Time) ([]LiveAgent, error) {
	threshold := olderThan.UTC().Format(timeFormat)
	rows, err := db.QueryContext(ctx, `
		SELECT agent_id, agent_type
		FROM agents
		WHERE retired_at IS NULL
		  AND (last_heartbeat IS NULL OR last_heartbeat < ?)
		ORDER BY agent_id
	`, threshold)
	if err != nil {
		return nil, fmt.Errorf("store: list stale agents: %w", err)
	}
	defer rows.Close()
	var out []LiveAgent
	for rows.Next() {
		var a LiveAgent
		var typ string
		if err := rows.Scan(&a.AgentID, &typ); err != nil {
			return nil, fmt.Errorf("store: scan stale agent: %w", err)
		}
		a.AgentType = schema.AgentType(typ)
		out = append(out, a)
	}
	return out, rows.Err()
}
