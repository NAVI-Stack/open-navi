package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/open-navi/navi/internal/schema"
)

// ConfidenceFromChain returns the minimum confidence among all entities in the
// derivation chain (design: confidence propagation with attenuation). Used to
// cap derived entity confidence so it cannot exceed source confidence.
func ConfidenceFromChain(ctx context.Context, db *sql.DB, derivationChain []string) float64 {
	if len(derivationChain) == 0 {
		return 1.0
	}
	minConf := 1.0
	for _, ref := range derivationChain {
		entityType, entityID := ref, ref
		if idx := indexOf(ref, ':'); idx > 0 {
			entityType = ref[:idx]
			entityID = ref[idx+1:]
		}
		ep, err := GetEntityProvenance(ctx, db, entityType, entityID)
		if err != nil || ep == nil {
			continue
		}
		if ep.Confidence < minConf {
			minConf = ep.Confidence
		}
	}
	return minConf
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// SaveEntityProvenance upserts provenance for an entity (source, derivation_chain, mutation_history, reinforcement_count).
// When derivation_chain is set, confidence is capped by ConfidenceFromChain (design: confidence propagation).
func SaveEntityProvenance(ctx context.Context, db *sql.DB, entityType, entityID string, p schema.EntityProvenance) error {
	if len(p.DerivationChain) > 0 && p.Confidence > 0 {
		capConf := ConfidenceFromChain(ctx, db, p.DerivationChain)
		if p.Confidence > capConf {
			p.Confidence = capConf
		}
	}
	ts := p.Timestamp.UTC().Format(timeFormat)
	var derivationChain, mutationHistory any
	if len(p.DerivationChain) > 0 {
		b, _ := json.Marshal(p.DerivationChain)
		derivationChain = string(b)
	}
	if len(p.MutationHistory) > 0 {
		b, _ := json.Marshal(p.MutationHistory)
		mutationHistory = string(b)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO entity_provenance (entity_type, entity_id, source, timestamp, confidence, derivation_chain, mutation_history, proposal_id, reinforcement_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, entity_id) DO UPDATE SET
			source = excluded.source,
			timestamp = excluded.timestamp,
			confidence = excluded.confidence,
			derivation_chain = excluded.derivation_chain,
			mutation_history = excluded.mutation_history,
			proposal_id = excluded.proposal_id,
			reinforcement_count = excluded.reinforcement_count
	`, entityType, entityID, p.Source, ts, p.Confidence, derivationChain, mutationHistory, nullEmpty(p.ProposalID), p.ReinforcementCount)
	if err != nil {
		return fmt.Errorf("store: save entity provenance: %w", err)
	}
	return nil
}

// GetEntityProvenance returns provenance for an entity, if any.
func GetEntityProvenance(ctx context.Context, db *sql.DB, entityType, entityID string) (*schema.EntityProvenance, error) {
	var (
		source, ts          string
		confidence          float64
		derivationChain     sql.NullString
		mutationHistory     sql.NullString
		proposalID          sql.NullString
		reinforcementCount  int
	)
	err := db.QueryRowContext(ctx, `
		SELECT source, timestamp, confidence, derivation_chain, mutation_history, proposal_id,
		       reinforcement_count
		FROM entity_provenance
		WHERE entity_type = ? AND entity_id = ?
	`, entityType, entityID).Scan(&source, &ts, &confidence, &derivationChain, &mutationHistory, &proposalID, &reinforcementCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get entity provenance: %w", err)
	}
	t, _ := parseTime(ts)
	p := &schema.EntityProvenance{
		Source:             source,
		Timestamp:          t,
		Confidence:         confidence,
		ReinforcementCount: reinforcementCount,
	}
	if proposalID.Valid {
		p.ProposalID = proposalID.String
	}
	if derivationChain.Valid && derivationChain.String != "" {
		_ = json.Unmarshal([]byte(derivationChain.String), &p.DerivationChain)
	}
	if mutationHistory.Valid && mutationHistory.String != "" {
		_ = json.Unmarshal([]byte(mutationHistory.String), &p.MutationHistory)
	}
	return p, nil
}

func nullEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
