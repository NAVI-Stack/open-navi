package schema

import "time"

// ContentTrust classifies who authored a piece of text and how much authority
// it carries. See docs/concepts/content-trust.md.
type ContentTrust string

const (
	ContentTrustOwner             ContentTrust = "owner"
	ContentTrustInternalSystem    ContentTrust = "internal_system"
	ContentTrustTrustedPlugin     ContentTrust = "trusted_plugin"
	ContentTrustExternalUntrusted ContentTrust = "external_untrusted"
)

// PrivacyClass gates which models an IntakeRecord may touch downstream (CIP §9).
type PrivacyClass string

const (
	PrivacyClassPublic    PrivacyClass = "public"
	PrivacyClassPersonal  PrivacyClass = "personal"
	PrivacyClassSensitive PrivacyClass = "sensitive"
	PrivacyClassSecret    PrivacyClass = "secret"
)

// IntakeProvenance carries connector-source metadata for audit and retrieval.
type IntakeProvenance struct {
	ConnectorID string `json:"connector_id"`
	AccountID   string `json:"account_id,omitempty"`
	LinkBack    string `json:"link_back,omitempty"`
}

// IntakeRecord is the canonical CIP intake envelope (CIP §5). Connectors emit
// one record per source item onto navi.refinery.queue. The Admit stage stamps
// and validates it before persistence into intake_records.
type IntakeRecord struct {
	ID           string
	ConnectorID  string // e.g. "telegram:acct-1"
	SourceKind   string // "message" | "email" | "issue" | ...
	SourceID     string // stable external id — used for dedupe
	Cursor       string // opaque per-connector resume position
	FetchedAt    time.Time
	Trust        ContentTrust
	PrivacyClass PrivacyClass
	Author       string
	Raw          []byte
	RawMIME      string
	Provenance   IntakeProvenance
	CreatedAt    time.Time
}

// IntakeChunk is a distilled, provenance-bearing slice of a canonicalized
// IntakeRecord (CIP §6 stages 3–5, §13). It is the durable output of the P2
// pipeline (Canonicalize → Chunk → Distill) and the unit retrieval consumes in
// P4. Every chunk carries a verifiable link back to its source record and the
// span offsets it occupies in the canonical Markdown rendering.
type IntakeChunk struct {
	ID             string // stable id — deterministic for identical input bytes
	IntakeRecordID string // FK to intake_records.id
	ConnectorID    string // denormalized for retrieval-time filtering
	SourceID       string
	ChunkIndex     int    // ordinal within the source record
	ParentChunkID  string // non-empty when further-derived from another chunk
	Content        string // distilled Markdown
	ContentMIME    string // typically "text/markdown"
	StartOffset    int    // start char offset into the canonical Markdown
	EndOffset      int    // end char offset into the canonical Markdown
	TokenEstimate  int
	Trust          ContentTrust
	PrivacyClass   PrivacyClass
	Provenance     ChunkProvenance
	CreatedAt      time.Time
}

// ChunkProvenance is the retrieval-time-filterable provenance summary persisted
// alongside each IntakeChunk. Distillation must never discard provenance: every
// extracted quote and entity span links back to its offsets, and any chunk
// collapsed by near-duplicate dedupe is recorded in CollapsedChunkIDs so the
// derivation chain remains intact.
type ChunkProvenance struct {
	ConnectorID       string        `json:"connector_id"`
	AccountID         string        `json:"account_id,omitempty"`
	LinkBack          string        `json:"link_back,omitempty"`
	Author            string        `json:"author,omitempty"`
	SourceKind        string        `json:"source_kind,omitempty"`
	FetchedAt         time.Time     `json:"fetched_at,omitempty"`
	SourceChunkIDs    []string      `json:"source_chunk_ids,omitempty"`    // self + any collapsed siblings
	CollapsedChunkIDs []string      `json:"collapsed_chunk_ids,omitempty"` // near-duplicates folded into this chunk
	Quotes            []ChunkQuote  `json:"quotes,omitempty"`
	Entities          []ChunkEntity `json:"entities,omitempty"`
}

// ChunkQuote is an extracted quote with span-level provenance (offsets are
// relative to the chunk Content).
type ChunkQuote struct {
	Text        string `json:"text"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
}

// ChunkEntity is a deterministically extracted entity-name span (offsets are
// relative to the chunk Content). Kind is one of: proper_noun, url, email,
// handle, hashtag, identifier.
type ChunkEntity struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
}
