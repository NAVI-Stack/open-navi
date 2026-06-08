package schema

import "time"

// JobMode distinguishes batch backfill ingestion from steady-state delta upkeep
// (CIP §7.1). It is carried on the MutationDescriptor and drives Proposal
// grouping at the synthesis batch boundary (synthesis seam §12).
type JobMode string

const (
	JobModeDelta    JobMode = "delta"
	JobModeBackfill JobMode = "backfill"
)

// IntakeRecordRef is a provenance pointer from a synthesized entity back to the
// source IntakeRecord that produced it (CIP §15 #2): record id, source id, fetch
// time, and the connector cursor position. Synthesis must never produce an
// entity without at least one of these.
type IntakeRecordRef struct {
	RecordID    string    `json:"record_id"`
	ConnectorID string    `json:"connector_id"`
	SourceID    string    `json:"source_id"`
	Cursor      string    `json:"cursor,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// ExtractionCandidate is a single entity candidate extracted from a distilled
// chunk by the Python extraction worker (CIP stage 7). Offsets are chunk-relative
// for span-level provenance. BlockingKeys are the deterministic identifiers the
// resolution matcher uses before falling back to similarity (synthesis seam §10):
// email, handle, normalized name, source-anchored id.
type ExtractionCandidate struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"` // person, organization, email, handle, identifier, date, proper_noun, ...
	Value        string            `json:"value,omitempty"`
	StartOffset  int               `json:"start_offset"`
	EndOffset    int               `json:"end_offset"`
	Confidence   float64           `json:"confidence"` // extraction confidence in [0,1]
	BlockingKeys map[string]string `json:"blocking_keys,omitempty"`
}

// ExtractionResult is the structured envelope the Python extraction worker
// returns for one chunk. Go is the only consumer; Python proposes, Go disposes —
// Python never writes the World Model (synthesis seam §9).
type ExtractionResult struct {
	ChunkID    string                `json:"chunk_id"`
	Candidates []ExtractionCandidate `json:"candidates"`
}

// ResolutionResult is the Python resolution matcher's honest judgement about
// whether an ExtractionCandidate is the same as an existing World Model entity
// (synthesis seam §9–§10). A confident unique match carries MatchedID. Ambiguity
// is reported, never guessed: an ambiguous match becomes a Modified outcome
// (low-confidence Create + possible_merge_with), not a merge.
type ResolutionResult struct {
	CandidateName    string   `json:"candidate_name"`
	MatchedID        string   `json:"matched_id,omitempty"`
	MatchedType      string   `json:"matched_type,omitempty"`
	Confidence       float64  `json:"confidence"` // resolution confidence in [0,1]
	Reason           string   `json:"reason,omitempty"`
	Ambiguous        bool     `json:"ambiguous,omitempty"`
	PossibleMergeIDs []string `json:"possible_merge_ids,omitempty"`
}
