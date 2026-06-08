// Package fold implements CIP stage 9 (Fold): updating a shallow summary index
// by entity type after synthesis writes. The fold is DERIVED state and is never
// authoritative (synthesis seam §14; CIP §6 stage 9) — it can be rebuilt from
// entity_provenance / intake_provenance at any time. V1 is intentionally flat;
// hierarchical depth can iterate later.
package fold

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/open-navi/navi/internal/store"
)

// Entry is a single synthesized entity to fold into the summary index.
type Entry struct {
	EntityType string
	EntityID   string
	Summary    string
}

// Folder maintains the by-entity-type summary index.
type Folder struct {
	db  *sql.DB
	log *slog.Logger
}

// New constructs a Folder.
func New(db *sql.DB, log *slog.Logger) *Folder {
	if log == nil {
		log = slog.Default()
	}
	return &Folder{db: db, log: log}
}

// Fold updates the summary index for the given written entities and returns the
// per-entity-type increment counts applied. Entries with an empty EntityID
// (proposals, drops) are ignored — only realized entity writes fold.
func (f *Folder) Fold(ctx context.Context, entries []Entry) (map[string]int, error) {
	counts := map[string]int{}
	for _, e := range entries {
		if e.EntityID == "" || e.EntityType == "" {
			continue
		}
		counts[e.EntityType]++
	}
	for entityType, delta := range counts {
		last := lastEntity(entries, entityType)
		if err := store.BumpIntakeSummary(ctx, f.db, entityType, last.EntityID, last.Summary, delta); err != nil {
			return counts, err
		}
	}
	if len(counts) > 0 {
		f.log.Debug("intake: fold: summary index updated", "counts", counts)
	}
	return counts, nil
}

func lastEntity(entries []Entry, entityType string) Entry {
	var last Entry
	for _, e := range entries {
		if e.EntityType == entityType && e.EntityID != "" {
			last = e
		}
	}
	return last
}
