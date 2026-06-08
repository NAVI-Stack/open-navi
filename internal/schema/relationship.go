package schema

import "time"

// Relationship represents a typed edge between two entities in the world model.
// It stores minimal metadata needed for strengthening, decay, and audit.
type Relationship struct {
	ID               string    `json:"id"`
	FromEntityID     string    `json:"from_entity_id"`
	ToEntityID       string    `json:"to_entity_id"`
	RelationshipType string    `json:"relationship_type"`
	Confidence       float64   `json:"confidence"`
	Recency          time.Time `json:"recency"`
	Provenance       string    `json:"provenance"`
}

// Provenance captures lifecycle metadata for entities and relationships.
type Provenance struct {
	Source             string    `json:"source"`
	Timestamp          time.Time `json:"timestamp"`
	Confidence         float64   `json:"confidence"`
	DerivationChain    []string  `json:"derivation_chain,omitempty"`
	ReinforcementCount int       `json:"reinforcement_count"`
	MutationHistory    []string  `json:"mutation_history,omitempty"`
}
