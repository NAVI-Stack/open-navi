// Package diff turns an owner-edited Vault file into structured World Model
// mutations. It compares a parsed candidate (projector.Doc, read from disk) with
// the current persisted entity snapshot and emits governor.MutationDescriptors
// tagged ContentTrust: owner / Source: "vault" (memory-projection-v1.md §5).
//
// This is the structured-diff safeguard, not lossless Markdown storage: an owner
// edit becomes a *proposed mutation against the entity*, subject to the same
// governance as an external delta. Owner trust raises the auto-approval bar; it
// never eliminates the gate. Structural intent the owner expresses in Markdown
// (changing navi_entity_id, deleting a file, navi_forget: true) is interpreted as
// a *request* for that mutation — it raises a Proposal, it never executes
// silently (spec §10).
package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/vault/projector"
)

// Current is the persisted snapshot of the entity a Vault file projects. The
// worker assembles it from the concrete store types before calling Plan.
type Current struct {
	EntityType   string
	EntityID     string
	Title        string
	Body         string
	Attrs        map[string]string
	Trust        schema.ContentTrust
	PrivacyClass schema.PrivacyClass
}

// Change is one human-readable field delta for the sync log.
type Change struct {
	Field string
	From  string
	To    string
}

// Result is the outcome of diffing one file.
type Result struct {
	Mutations  []governor.MutationDescriptor
	Changes    []Change
	Summary    string
	Structural bool // a merge/forget request was detected (hard-floored → Proposal)
	NoOp       bool // nothing changed
}

// PlanOptions carry the per-file context the descriptors need.
type PlanOptions struct {
	RelPath string // vault-relative file path, for provenance
}

// Plan compares a candidate Doc against the current entity and produces the
// mutation plan. It is pure (no I/O) so it is fully unit-testable.
func Plan(doc projector.Doc, cur Current, opts PlanOptions) Result {
	var res Result
	trust := schema.ContentTrustOwner
	privacy := cur.PrivacyClass
	if privacy == "" {
		privacy = schema.PrivacyClassPersonal
	}
	base := func(kind governor.MutationKind) governor.MutationDescriptor {
		return governor.MutationDescriptor{
			Kind:          kind,
			TargetType:    cur.EntityType,
			TargetEntity:  cur.EntityID,
			Trust:         trust,
			PrivacyClass:  privacy,
			Derivation:    []string{"vault:" + opts.RelPath, "entity:" + cur.EntityType + ":" + cur.EntityID},
			EstConfidence: 1.0, // owner-authored
			JobMode:       schema.JobModeDelta,
			Source:        governor.MutationSourceVault,
			VaultPath:     opts.RelPath,
		}
	}

	// 1) Forget gesture (navi_forget: true) — hard-floored request, raises a
	//    Forget Proposal. Takes precedence: the owner asked to forget the entity.
	if doc.Forget {
		m := base(governor.MutationForgetMemory)
		res.Mutations = append(res.Mutations, m)
		res.Changes = append(res.Changes, Change{Field: "navi_forget", From: "false", To: "true"})
		res.Structural = true
		res.Summary = "owner requested forget (navi_forget: true)"
		return res
	}

	// 2) entity_id swap — a re-link / merge request, hard-floored. Never silently
	//    re-links the file (spec §10; acceptance: frontmatter entity_id change).
	if doc.EntityID != "" && doc.EntityID != cur.EntityID {
		m := base(governor.MutationMergeEntities)
		m.CandidateRefs = []string{cur.EntityID, doc.EntityID}
		res.Mutations = append(res.Mutations, m)
		res.Changes = append(res.Changes, Change{Field: "navi_entity_id", From: cur.EntityID, To: doc.EntityID})
		res.Structural = true
		res.Summary = fmt.Sprintf("owner requested re-link/merge %s → %s", cur.EntityID, doc.EntityID)
		return res
	}

	// 3) Routine attribute edits — title (name/key/summary), body, and owner
	//    scalar attrs. These collapse to a single Reinforce (Approved under owner
	//    trust); the worker applies the candidate state on approval.
	if t := strings.TrimSpace(doc.Title); t != "" && t != strings.TrimSpace(cur.Title) {
		res.Changes = append(res.Changes, Change{Field: "title", From: cur.Title, To: t})
	}
	if normalizeBody(doc.Body) != normalizeBody(cur.Body) {
		res.Changes = append(res.Changes, Change{Field: "body", From: snippet(cur.Body), To: snippet(doc.Body)})
	}
	for _, k := range attrKeys(doc.Attrs, cur.Attrs) {
		dv := strings.TrimSpace(doc.Attrs[k])
		cv := strings.TrimSpace(cur.Attrs[k])
		if dv != cv {
			res.Changes = append(res.Changes, Change{Field: k, From: cv, To: dv})
		}
	}

	if len(res.Changes) == 0 {
		res.NoOp = true
		res.Summary = "no changes"
		return res
	}

	m := base(governor.MutationReinforceEntity)
	res.Mutations = append(res.Mutations, m)
	res.Summary = summarize(res.Changes)
	return res
}

// ForgetOnDelete builds the mutation plan for a deleted file: a Forget request
// (hard-floored → Proposal). The file deletion never deletes the entity itself
// (spec §10; acceptance: file deletion raises a Forget Proposal).
func ForgetOnDelete(cur Current, relPath string) Result {
	privacy := cur.PrivacyClass
	if privacy == "" {
		privacy = schema.PrivacyClassPersonal
	}
	m := governor.MutationDescriptor{
		Kind:          governor.MutationForgetMemory,
		TargetType:    cur.EntityType,
		TargetEntity:  cur.EntityID,
		Trust:         schema.ContentTrustOwner,
		PrivacyClass:  privacy,
		Derivation:    []string{"vault-delete:" + relPath, "entity:" + cur.EntityType + ":" + cur.EntityID},
		EstConfidence: 1.0,
		JobMode:       schema.JobModeDelta,
		Source:        governor.MutationSourceVault,
		VaultPath:     relPath,
	}
	return Result{
		Mutations:  []governor.MutationDescriptor{m},
		Changes:    []Change{{Field: "file", From: relPath, To: "(deleted)"}},
		Structural: true,
		Summary:    "owner deleted file — forget requested",
	}
}

func attrKeys(a, b map[string]string) []string {
	seen := map[string]struct{}{}
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func normalizeBody(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func snippet(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	const max = 80
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func summarize(changes []Change) string {
	parts := make([]string, 0, len(changes))
	for _, c := range changes {
		switch c.Field {
		case "body":
			parts = append(parts, "body changed")
		default:
			parts = append(parts, fmt.Sprintf("%s: %q → %q", c.Field, c.From, c.To))
		}
	}
	return strings.Join(parts, "; ")
}
