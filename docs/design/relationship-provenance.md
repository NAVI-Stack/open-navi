# Relationship Provenance (Full Chain)

**Status:** Accepted  
**Concept:** [REL-01] in concept-vs-implementation gap plan.

## Rule

Every relationship carries provenance: which event, source, or reflection tier produced it. Full provenance metadata (derivation chain, mutation history) is defined in the Provenance Model so Subconscious can strengthen, decay, and audit relationships.

## Current implementation

- **`schema.Relationship.Provenance`** — A string used as a short source/summary (e.g. `"reflection"`, `"user_input"`). Stored in `entity_relationships.provenance`.
- **`schema.EntityProvenance`** — Full metadata: source, timestamp, confidence, derivation_chain, mutation_history, proposal_id, reinforcement_count. Stored in `entity_provenance` keyed by `(entity_type, entity_id)`.

Relationships are first-class entities for provenance: the same `entity_provenance` table supports `entity_type = "relationship"` and `entity_id = rel.ID`.

- **Store:** `SaveRelationship` delegates to `SaveRelationshipWithProvenance(ctx, db, rel, nil)`. Every relationship write upserts a row in `entity_provenance` for `("relationship", rel.ID)`:
  - If the caller passes non-nil `*EntityProvenance`, that is written (full derivation_chain and mutation_history when provided).
  - If the caller passes nil, a minimal provenance row is written (source=rel.Provenance, timestamp=now, confidence=rel.Confidence, empty derivation_chain and mutation_history).
- **Queries:** Full derivation chain and mutation history for a relationship: `store.GetEntityProvenance(ctx, db, "relationship", rel.ID)`. Reflection and audit use this to traverse derivation.

## Contract

1. **Short label:** `Relationship.Provenance` remains a string for backward compatibility and quick display.
2. **Full chain:** Every relationship write records provenance in `entity_provenance`. For full derivation chain and mutation history when reading, use `store.GetEntityProvenance(ctx, db, "relationship", rel.ID)`.
3. **Callers with full provenance:** When creating or updating a relationship with derivation_chain/mutation_history (e.g. from reflection), call `SaveRelationshipWithProvenance(ctx, db, rel, &prov)` with a populated `EntityProvenance`; otherwise `SaveRelationship` (or `SaveRelationshipWithProvenance(rel, nil)`) is sufficient and records minimal provenance.

## Relevant files

- `internal/schema/relationship.go` (Relationship.Provenance, Provenance struct)
- `internal/schema/worldmodel.go` (EntityProvenance)
- `internal/store/relationship.go`
- `internal/store/entity_provenance.go`
- `internal/worldmodel/worldmodel.go`
