package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/vault/diff"
	"github.com/open-navi/navi/internal/vault/projector"
)

// entityRef identifies a World Model entity the Vault projects.
type entityRef struct {
	Type string
	ID   string
}

// loadAllProjectable enumerates every V1 Vault entity (Contacts, Knowledge,
// Memories, Artifacts) as projector.Entity values. Read-only.
func (w *Worker) loadAllProjectable(ctx context.Context) ([]projector.Entity, error) {
	var out []projector.Entity
	limit := w.cfg.MaxEntities
	if limit <= 0 {
		limit = 1000
	}

	contacts, err := store.ListContacts(ctx, w.db, store.ListContactsFilter{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("vault: list contacts: %w", err)
	}
	for _, c := range contacts {
		out = append(out, w.contactToEntity(ctx, c))
	}

	facts, err := store.ListAllFacts(ctx, w.db, false, limit)
	if err != nil {
		return nil, fmt.Errorf("vault: list facts: %w", err)
	}
	for _, f := range facts {
		out = append(out, w.factToEntity(ctx, f))
	}

	mems, err := store.ListAllMemories(ctx, w.db, limit)
	if err != nil {
		return nil, fmt.Errorf("vault: list memories: %w", err)
	}
	for _, m := range mems {
		out = append(out, w.memoryToEntity(ctx, m))
	}

	arts, err := store.ListAllArtifacts(ctx, w.db, limit)
	if err != nil {
		return nil, fmt.Errorf("vault: list artifacts: %w", err)
	}
	for _, a := range arts {
		out = append(out, w.artifactToEntity(ctx, a))
	}
	return out, nil
}

// loadEntity loads a single entity as a projector.Entity (for reprojection).
func (w *Worker) loadEntity(ctx context.Context, ref entityRef) (projector.Entity, bool, error) {
	switch ref.Type {
	case projector.TypeContact:
		c, err := store.GetContact(ctx, w.db, ref.ID)
		if err != nil {
			return projector.Entity{}, false, nil
		}
		return w.contactToEntity(ctx, c), true, nil
	case projector.TypeKnowledge:
		f, err := store.GetFact(ctx, w.db, ref.ID)
		if err != nil {
			return projector.Entity{}, false, nil
		}
		return w.factToEntity(ctx, f), true, nil
	case projector.TypeMemory:
		m, err := store.GetMemory(ctx, w.db, ref.ID)
		if err != nil {
			return projector.Entity{}, false, nil
		}
		return w.memoryToEntity(ctx, m), true, nil
	case projector.TypeArtifact:
		a, err := store.GetArtifact(ctx, w.db, ref.ID)
		if err != nil || a == nil {
			return projector.Entity{}, false, nil
		}
		return w.artifactToEntity(ctx, *a), true, nil
	}
	return projector.Entity{}, false, nil
}

// buildCurrent loads the current persisted snapshot used by the diff engine.
func (w *Worker) buildCurrent(ctx context.Context, ref entityRef) (diff.Current, bool, error) {
	e, ok, err := w.loadEntity(ctx, ref)
	if err != nil || !ok {
		return diff.Current{}, false, err
	}
	attrs := map[string]string{}
	for _, a := range e.Attrs {
		attrs[a.Key] = a.Value
	}
	return diff.Current{
		EntityType:   e.Type,
		EntityID:     e.ID,
		Title:        e.Title,
		Body:         e.Body,
		Attrs:        attrs,
		Trust:        schema.ContentTrustOwner,
		PrivacyClass: w.entityPrivacy(attrs),
	}, true, nil
}

// entityPrivacy derives the entity's privacy class. Entities do not yet store a
// privacy class natively, so an explicit navi_privacy attr (owner-set) wins;
// otherwise the Vault default is personal (spec §12).
func (w *Worker) entityPrivacy(attrs map[string]string) schema.PrivacyClass {
	if p := strings.TrimSpace(attrs["privacy"]); p != "" {
		return schema.PrivacyClass(strings.ToLower(p))
	}
	return schema.PrivacyClassPersonal
}

// --- concrete-type → projector.Entity converters ---

func (w *Worker) contactToEntity(ctx context.Context, c schema.Contact) projector.Entity {
	notes := ""
	if c.Metadata != "" {
		var m map[string]any
		if json.Unmarshal([]byte(c.Metadata), &m) == nil {
			if n, ok := m["notes"].(string); ok {
				notes = n
			}
		}
	}
	return projector.Entity{
		Type:       projector.TypeContact,
		ID:         c.ID,
		Title:      c.Name,
		Body:       notes,
		Confidence: w.entityConfidence(ctx, projector.TypeContact, c.ID, 0.7),
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		Provenance: w.entityProvenance(ctx, projector.TypeContact, c.ID),
		Attrs:      []projector.Attr{{Key: "kind", Value: c.Kind}, {Key: "trust_level", Value: c.TrustLevel}},
	}
}

func (w *Worker) factToEntity(ctx context.Context, f store.Fact) projector.Entity {
	title := f.Key
	if title == "" {
		title = f.Category
	}
	return projector.Entity{
		Type:       projector.TypeKnowledge,
		ID:         f.ID,
		Title:      title,
		Body:       f.Value,
		Confidence: w.entityConfidence(ctx, projector.TypeKnowledge, f.ID, 0.7),
		CreatedAt:  f.CreatedAt,
		UpdatedAt:  f.UpdatedAt,
		Provenance: w.entityProvenance(ctx, projector.TypeKnowledge, f.ID),
		Attrs:      []projector.Attr{{Key: "category", Value: f.Category}},
	}
}

func (w *Worker) memoryToEntity(ctx context.Context, m schema.Memory) projector.Entity {
	return projector.Entity{
		Type:       projector.TypeMemory,
		ID:         m.ID,
		Title:      m.Summary,
		Body:       m.Details,
		Confidence: w.entityConfidence(ctx, projector.TypeMemory, m.ID, 0.6),
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
		Provenance: w.entityProvenance(ctx, projector.TypeMemory, m.ID),
		Attrs:      []projector.Attr{{Key: "significance", Value: m.Significance}},
	}
}

func (w *Worker) artifactToEntity(ctx context.Context, a schema.Artifact) projector.Entity {
	title := a.DisplayTitle
	if title == "" {
		title = a.CanonicalTitle
	}
	body := strings.TrimSpace(a.Description)
	if a.Location != "" {
		body = strings.TrimSpace(body + "\n\n_Artifact body lives in the Artifact System: `" + a.Location + "`_")
	}
	return projector.Entity{
		Type:       projector.TypeArtifact,
		ID:         a.ID,
		Title:      title,
		Body:       body,
		Confidence: w.entityConfidence(ctx, projector.TypeArtifact, a.ID, 0.9),
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
		Provenance: w.entityProvenance(ctx, projector.TypeArtifact, a.ID),
		Attrs:      []projector.Attr{{Key: "subtype", Value: a.Subtype}, {Key: "lifecycle", Value: string(a.LifecycleState)}},
	}
}

func (w *Worker) entityConfidence(ctx context.Context, entityType, id string, fallback float64) float64 {
	if ep, _ := store.GetEntityProvenance(ctx, w.db, entityType, id); ep != nil && ep.Confidence > 0 {
		return ep.Confidence
	}
	return fallback
}

func (w *Worker) entityProvenance(ctx context.Context, entityType, id string) []projector.Provenance {
	links, _ := store.ListIntakeProvenanceByEntity(ctx, w.db, entityType, id)
	var out []projector.Provenance
	for _, l := range links {
		if l.EntityID == "" {
			continue
		}
		out = append(out, projector.Provenance{
			Source:    l.ConnectorID,
			RecordID:  l.IntakeRecordID,
			FetchedAt: l.FetchedAt,
		})
		if len(out) >= 5 {
			break
		}
	}
	return out
}

// --- approved owner-edit writers (run only after governor approval) ---

// applyApprovedEdit writes the owner's candidate state back to the World Model
// after the governor approved the Reinforce. It also records the owner override
// on the entity provenance (confidence bump + mutation-history note) so a
// concurrent external delta is visibly "overridden by owner" (spec §11).
func (w *Worker) applyApprovedEdit(ctx context.Context, ref entityRef, doc projector.Doc, conf float64) error {
	switch ref.Type {
	case projector.TypeContact:
		c, err := store.GetContact(ctx, w.db, ref.ID)
		if err != nil {
			return err
		}
		if t := strings.TrimSpace(doc.Title); t != "" {
			c.Name = t
		}
		if k := strings.TrimSpace(doc.Attrs["kind"]); k != "" {
			c.Kind = k
		}
		if tl := strings.TrimSpace(doc.Attrs["trust_level"]); tl != "" {
			c.TrustLevel = tl
		}
		c.Metadata = setMetaNotes(c.Metadata, doc.Body)
		if err := store.SaveContact(ctx, w.db, c); err != nil {
			return err
		}
	case projector.TypeKnowledge:
		f, err := store.GetFact(ctx, w.db, ref.ID)
		if err != nil {
			return err
		}
		if t := strings.TrimSpace(doc.Title); t != "" {
			f.Key = t
		}
		if c := strings.TrimSpace(doc.Attrs["category"]); c != "" {
			f.Category = c
		}
		f.Value = strings.TrimSpace(doc.Body)
		f.Source = "vault"
		if err := store.SaveFact(ctx, w.db, f); err != nil {
			return err
		}
	case projector.TypeMemory:
		m, err := store.GetMemory(ctx, w.db, ref.ID)
		if err != nil {
			return err
		}
		if t := strings.TrimSpace(doc.Title); t != "" {
			m.Summary = t
		}
		if s := strings.TrimSpace(doc.Attrs["significance"]); s != "" {
			m.Significance = s
		}
		m.Details = strings.TrimSpace(doc.Body)
		if _, err := store.SaveMemory(ctx, w.db, m); err != nil {
			return err
		}
	case projector.TypeArtifact:
		a, err := store.GetArtifact(ctx, w.db, ref.ID)
		if err != nil || a == nil {
			return fmt.Errorf("vault: artifact not found: %s", ref.ID)
		}
		if t := strings.TrimSpace(doc.Title); t != "" {
			a.DisplayTitle = t
		}
		a.Description = strings.TrimSpace(doc.Body)
		if err := store.SaveArtifact(ctx, w.db, *a); err != nil {
			return err
		}
	default:
		return fmt.Errorf("vault: unsupported entity type %q", ref.Type)
	}
	return w.recordOwnerOverride(ctx, ref, conf)
}

// recordOwnerOverride bumps confidence and appends a mutation-history note so an
// external delta that touched the same entity is recorded as overridden by the
// owner (spec §11 routine-attribute conflict resolution).
func (w *Worker) recordOwnerOverride(ctx context.Context, ref entityRef, conf float64) error {
	ep, _ := store.GetEntityProvenance(ctx, w.db, ref.Type, ref.ID)
	prov := schema.EntityProvenance{
		Source:     "vault",
		Timestamp:  w.now(),
		Confidence: conf,
	}
	if ep != nil {
		prov.DerivationChain = ep.DerivationChain
		prov.ReinforcementCount = ep.ReinforcementCount + 1
		prov.MutationHistory = ep.MutationHistory
		if ep.Confidence > conf {
			// Owner edits raise the bar; never lower confidence below prior.
			prov.Confidence = ep.Confidence
		}
	}
	prov.MutationHistory = append(prov.MutationHistory,
		fmt.Sprintf("owner edit via vault at %s (overrides external deltas)", w.now().Format(time.RFC3339)))
	return store.SaveEntityProvenance(ctx, w.db, ref.Type, ref.ID, prov)
}

func setMetaNotes(metaJSON, notes string) string {
	m := map[string]any{}
	if strings.TrimSpace(metaJSON) != "" {
		_ = json.Unmarshal([]byte(metaJSON), &m)
	}
	notes = strings.TrimSpace(notes)
	if notes == "" {
		delete(m, "notes")
	} else {
		m["notes"] = notes
	}
	b, _ := json.Marshal(m)
	return string(b)
}
