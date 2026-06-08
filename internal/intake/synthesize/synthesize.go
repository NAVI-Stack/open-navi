// Package synthesize implements CIP stage 8 (Synthesize) — the only stage that
// writes the World Model. It builds a governor.MutationDescriptor from each
// (extraction, resolution, score, trust, privacy, job mode) tuple, runs it through
// the existing governor engine via governor.EvaluateMutation (a new caller of the
// same Pipeline — not a fork), and acts on the ValidationOutcome:
//
//	Approved             -> write the entity (Contact / Memory) with provenance + confidence
//	RequiresConfirmation -> raise a Proposal (delta: per-write; backfill: grouped)
//	Modified             -> write the softened low-confidence Create + possible_merge_with
//	Rejected             -> drop and log
//
// Every write carries provenance back to its source IntakeRecord and a confidence
// in [0,1]. Re-synthesizing the same derivation is a no-op (idempotency via the
// intake_provenance derivation_key). Un-promoted chunks are never deleted —
// synthesis is promotion, not gating (synthesis seam §13–§14).
package synthesize

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/intake/score"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// Input bundles one distilled chunk with the upstream stage outputs synthesis
// consumes. Resolutions are aligned to Extraction.Candidates by index.
type Input struct {
	Chunk       schema.IntakeChunk
	Score       score.Result
	Extraction  schema.ExtractionResult
	Resolutions []schema.ResolutionResult
}

// ItemOutcome records what synthesis decided for one candidate/derivation.
type ItemOutcome struct {
	DerivationKey string
	EntityType    string
	EntityID      string
	Kind          governor.MutationKind
	Outcome       governor.ValidationOutcome
	ProposalID    string
	Confidence    float64
	Skipped       bool // idempotent no-op (already synthesized)
}

// Result summarizes a synthesis pass over a record's chunks.
type Result struct {
	EntitiesWritten   int
	ProposalsRaised   int
	Modified          int
	Rejected          int
	Skipped           int
	GroupedProposalID string
	Outcomes          []ItemOutcome
}

// Synthesizer writes the World Model from intake. It is the single intake path
// that touches entity tables; every write passes through governor.EvaluateMutation.
type Synthesizer struct {
	db   *sql.DB
	opts governor.MutationPipelineOptions
	log  *slog.Logger
	now  func() time.Time
}

// New constructs a Synthesizer. opts carries the autonomy resolver, the
// write-class gate, and any owner veto; the zero value is a valid (autonomy-off)
// configuration.
func New(db *sql.DB, opts governor.MutationPipelineOptions, log *slog.Logger) *Synthesizer {
	if log == nil {
		log = slog.Default()
	}
	return &Synthesizer{db: db, opts: opts, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// entityKindForCandidate maps an extraction candidate kind to the World Model
// entity type it synthesizes into. Only contact-like candidates become entities
// in V1; url/date/identifier remain on the chunk as retrievable provenance.
func entityKindForCandidate(kind string) (entityType, contactKind string, ok bool) {
	switch kind {
	case "person", "proper_noun":
		return "contact", schema.ContactKindPerson, true
	case "organization":
		return "contact", schema.ContactKindOrganization, true
	case "email", "handle":
		return "contact", schema.ContactKindPerson, true
	default:
		return "", "", false
	}
}

// mutationKindForResolution maps a ResolutionResult to a MutationKind and the
// candidate entity ids in scope (synthesis seam §6/§10).
func mutationKindForResolution(res schema.ResolutionResult) (governor.MutationKind, []string) {
	switch {
	case res.Ambiguous:
		// Uncertain match: the resolver proposed a possible merge but is unsure.
		// The Risk rubric collapses this to Modified (low-confidence Create).
		return governor.MutationMergeEntities, res.PossibleMergeIDs
	case res.MatchedID != "":
		return governor.MutationReinforceEntity, nil
	case len(res.PossibleMergeIDs) >= 2:
		// Confident that several existing entities are duplicates of this
		// candidate: a structural merge, hard-floored into a Proposal.
		return governor.MutationMergeEntities, res.PossibleMergeIDs
	default:
		return governor.MutationCreateEntity, nil
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func sourceTrustWeight(trust schema.ContentTrust) float64 {
	switch trust {
	case schema.ContentTrustOwner:
		return 1.0
	case schema.ContentTrustInternalSystem:
		return 0.8
	case schema.ContentTrustTrustedPlugin:
		return 0.6
	default:
		return 0.3
	}
}

// blendConfidence combines source trust, extraction confidence, and resolution
// confidence into a single [0,1] value (synthesis seam §11).
func blendConfidence(trust schema.ContentTrust, extractionConf, resolutionConf float64, matched bool) float64 {
	resComponent := resolutionConf
	if !matched {
		// A clean create has no resolution match; treat the absence of a
		// conflicting match as moderate positive evidence rather than zero.
		resComponent = 0.6
	}
	conf := 0.5*sourceTrustWeight(trust) + 0.3*clamp01(extractionConf) + 0.2*clamp01(resComponent)
	return clamp01(conf)
}

func derivationKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

func recordRef(rec schema.IntakeRecord) schema.IntakeRecordRef {
	return schema.IntakeRecordRef{
		RecordID:    rec.ID,
		ConnectorID: rec.ConnectorID,
		SourceID:    rec.SourceID,
		Cursor:      rec.Cursor,
		FetchedAt:   rec.FetchedAt,
	}
}

// pendingMerge buffers a backfill-mode RequiresConfirmation result for grouping.
type pendingMerge struct {
	derivationKey string
	candidateName string
	candidateRefs []string
	confidence    float64
	chunkID       string
}

// Synthesize runs stage 8 for one record's chunks. jobMode controls Proposal
// granularity: Delta emits one Proposal per write; Backfill batches
// RequiresConfirmation results into a single grouped Proposal (seam §12).
func (s *Synthesizer) Synthesize(ctx context.Context, rec schema.IntakeRecord, inputs []Input, jobMode schema.JobMode) (Result, error) {
	var res Result
	ref := recordRef(rec)
	var pending []pendingMerge

	for _, in := range inputs {
		// Per-chunk Memory append for promotion candidates (append-only history;
		// unconditionally safe → Approved).
		if in.Score.PromoteCandidate {
			if out, err := s.appendChunkMemory(ctx, rec, ref, in); err != nil {
				s.log.Error("intake: synthesize: memory append failed", "err", err, "chunk_id", in.Chunk.ID)
			} else {
				res.apply(out)
			}
		}

		for i, cand := range in.Extraction.Candidates {
			entityType, contactKind, ok := entityKindForCandidate(cand.Kind)
			if !ok {
				continue
			}
			var resn schema.ResolutionResult
			if i < len(in.Resolutions) {
				resn = in.Resolutions[i]
			}
			kind, refs := mutationKindForResolution(resn)

			candKey := strings.ToLower(strings.TrimSpace(cand.Value))
			if candKey == "" {
				candKey = strings.ToLower(strings.TrimSpace(cand.Name))
			}
			dkey := derivationKey("contact", rec.ID, in.Chunk.ID, cand.Kind, candKey)

			exists, err := store.IntakeProvenanceExists(ctx, s.db, dkey)
			if err != nil {
				return res, fmt.Errorf("intake: synthesize: idempotency check: %w", err)
			}
			if exists {
				res.Skipped++
				res.Outcomes = append(res.Outcomes, ItemOutcome{DerivationKey: dkey, Skipped: true, EntityType: entityType, Kind: kind})
				continue
			}

			conf := blendConfidence(in.Chunk.Trust, cand.Confidence, resn.Confidence, resn.MatchedID != "")
			deriv := []string{"record:" + rec.ID, "chunk:" + in.Chunk.ID}

			desc := governor.MutationDescriptor{
				Kind:          kind,
				TargetType:    entityType,
				TargetEntity:  resn.MatchedID,
				CandidateRefs: refs,
				SourceRecords: []schema.IntakeRecordRef{ref},
				Trust:         in.Chunk.Trust,
				PrivacyClass:  in.Chunk.PrivacyClass,
				Resolution:    resn,
				Derivation:    deriv,
				EstConfidence: conf,
				JobMode:       jobMode,
			}

			result := governor.EvaluateMutation(desc, s.opts)

			out, deferGroup, err := s.act(ctx, rec, ref, in, cand, contactKind, kind, refs, conf, deriv, dkey, result, jobMode)
			if err != nil {
				return res, err
			}
			if deferGroup != nil {
				pending = append(pending, *deferGroup)
				// Outcome recorded after the group proposal id is known.
				continue
			}
			res.apply(out)
		}
	}

	// Flush backfill-grouped proposals at the batch boundary.
	if jobMode == schema.JobModeBackfill && len(pending) > 0 {
		groupID, err := s.raiseGroupedProposal(ctx, rec, pending)
		if err != nil {
			return res, err
		}
		res.GroupedProposalID = groupID
		for _, p := range pending {
			if _, err := store.SaveIntakeProvenanceLink(ctx, s.db, store.IntakeProvenanceLink{
				ID:             uuid.NewString(),
				IntakeRecordID: rec.ID,
				ChunkID:        p.chunkID,
				ConnectorID:    rec.ConnectorID,
				SourceID:       rec.SourceID,
				Cursor:         rec.Cursor,
				FetchedAt:      rec.FetchedAt,
				EntityType:     "contact",
				MutationKind:   governor.MutationMergeEntities.String(),
				Outcome:        "proposal",
				DerivationKey:  p.derivationKey,
				Confidence:     p.confidence,
				ProposalID:     groupID,
			}); err != nil {
				return res, fmt.Errorf("intake: synthesize: link grouped proposal: %w", err)
			}
			res.ProposalsRaised++
			res.Outcomes = append(res.Outcomes, ItemOutcome{
				DerivationKey: p.derivationKey, EntityType: "contact",
				Kind: governor.MutationMergeEntities, Outcome: governor.ValidationRequiresConfirmation,
				ProposalID: groupID, Confidence: p.confidence,
			})
		}
	}

	return res, nil
}

func (r *Result) apply(o ItemOutcome) {
	r.Outcomes = append(r.Outcomes, o)
	switch o.Outcome {
	case governor.ValidationApproved:
		if o.EntityID != "" {
			r.EntitiesWritten++
		}
	case governor.ValidationModified:
		r.Modified++
		if o.EntityID != "" {
			r.EntitiesWritten++
		}
	case governor.ValidationRequiresConfirmation:
		r.ProposalsRaised++
	case governor.ValidationRejected:
		r.Rejected++
	}
}

// act performs the entity write / proposal / drop dictated by the governor result
// and records the provenance link. It returns a non-nil *pendingMerge when a
// backfill-mode RequiresConfirmation must be deferred for grouping.
func (s *Synthesizer) act(
	ctx context.Context, rec schema.IntakeRecord, ref schema.IntakeRecordRef, in Input,
	cand schema.ExtractionCandidate, contactKind string, kind governor.MutationKind,
	refs []string, conf float64, deriv []string, dkey string,
	result governor.ValidationResult, jobMode schema.JobMode,
) (ItemOutcome, *pendingMerge, error) {
	out := ItemOutcome{DerivationKey: dkey, EntityType: "contact", Kind: kind, Outcome: result.Outcome, Confidence: conf}

	switch result.Outcome {
	case governor.ValidationApproved:
		entityID, err := s.writeContactApproved(ctx, rec, ref, in, cand, contactKind, kind, conf, deriv)
		if err != nil {
			return out, nil, err
		}
		out.EntityID = entityID
		if err := s.link(ctx, rec, in.Chunk.ID, "contact", entityID, kind, "approved", dkey, conf, ""); err != nil {
			return out, nil, err
		}

	case governor.ValidationModified:
		var soft governor.SoftenedMutation
		_ = json.Unmarshal([]byte(result.ModifiedAction), &soft)
		// Softened: low-confidence Create tagged possible_merge_with (never a merge).
		softConf := clamp01(conf * 0.6)
		entityID, err := s.writeContact(ctx, rec, ref, cand.Name, contactKind, softConf, deriv, soft.PossibleMergeWith)
		if err != nil {
			return out, nil, err
		}
		out.EntityID = entityID
		out.Confidence = softConf
		if err := s.link(ctx, rec, in.Chunk.ID, "contact", entityID, governor.MutationCreateEntity, "modified", dkey, softConf, ""); err != nil {
			return out, nil, err
		}

	case governor.ValidationRequiresConfirmation:
		if jobMode == schema.JobModeBackfill {
			return out, &pendingMerge{
				derivationKey: dkey, candidateName: cand.Name, candidateRefs: refs,
				confidence: conf, chunkID: in.Chunk.ID,
			}, nil
		}
		proposalID, err := s.raiseSingleProposal(ctx, rec, cand.Name, refs, dkey, conf)
		if err != nil {
			return out, nil, err
		}
		out.ProposalID = proposalID
		if err := s.link(ctx, rec, in.Chunk.ID, "contact", "", kind, "proposal", dkey, conf, proposalID); err != nil {
			return out, nil, err
		}

	case governor.ValidationRejected:
		s.log.Info("intake: synthesize: mutation rejected — dropping",
			"chunk_id", in.Chunk.ID, "candidate", cand.Name, "reason", result.Reason)
		if err := s.link(ctx, rec, in.Chunk.ID, "contact", "", kind, "rejected", dkey, conf, ""); err != nil {
			return out, nil, err
		}
	}

	return out, nil, nil
}

// writeContactApproved handles the Approved path for Create and Reinforce.
func (s *Synthesizer) writeContactApproved(
	ctx context.Context, rec schema.IntakeRecord, ref schema.IntakeRecordRef, in Input,
	cand schema.ExtractionCandidate, contactKind string, kind governor.MutationKind,
	conf float64, deriv []string,
) (string, error) {
	if kind == governor.MutationReinforceEntity && in.matchedID(cand) != "" {
		return s.reinforceContact(ctx, in.matchedID(cand), conf, deriv, ref)
	}
	return s.writeContact(ctx, rec, ref, cand.Name, contactKind, conf, deriv, nil)
}

// matchedID returns the resolution match for the candidate at the same index.
func (in Input) matchedID(cand schema.ExtractionCandidate) string {
	for i, c := range in.Extraction.Candidates {
		if c.Name == cand.Name && c.StartOffset == cand.StartOffset {
			if i < len(in.Resolutions) {
				return in.Resolutions[i].MatchedID
			}
		}
	}
	return ""
}

func (s *Synthesizer) writeContact(
	ctx context.Context, rec schema.IntakeRecord, ref schema.IntakeRecordRef,
	name, contactKind string, conf float64, deriv []string, possibleMerge []string,
) (string, error) {
	id := uuid.NewString()
	meta := map[string]any{"source": "intake", "connector_id": rec.ConnectorID}
	if len(possibleMerge) > 0 {
		meta["possible_merge_with"] = possibleMerge
	}
	metaJSON, _ := json.Marshal(meta)
	c := schema.Contact{
		ID:         id,
		Name:       name,
		Kind:       contactKind,
		OwnerType:  schema.ContactOwnerTypeNavi,
		TrustLevel: contactTrustLevel(rec.Trust),
		Metadata:   string(metaJSON),
	}
	if err := store.SaveContact(ctx, s.db, c); err != nil {
		return "", fmt.Errorf("intake: synthesize: save contact: %w", err)
	}
	if err := s.saveProvenance(ctx, "contact", id, ref, conf, deriv, 0); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Synthesizer) reinforceContact(
	ctx context.Context, matchedID string, conf float64, deriv []string, ref schema.IntakeRecordRef,
) (string, error) {
	c, err := store.GetContact(ctx, s.db, matchedID)
	if err != nil {
		// Matched id is stale; fall back to recording provenance only.
		s.log.Warn("intake: synthesize: reinforce target missing", "id", matchedID, "err", err)
		return matchedID, s.saveProvenance(ctx, "contact", matchedID, ref, conf, deriv, 1)
	}
	// Touch the contact so updated_at advances (reinforcement is a real write).
	if err := store.SaveContact(ctx, s.db, c); err != nil {
		return "", fmt.Errorf("intake: synthesize: reinforce contact: %w", err)
	}
	rc := 1
	if ep, _ := store.GetEntityProvenance(ctx, s.db, "contact", matchedID); ep != nil {
		rc = ep.ReinforcementCount + 1
		if ep.Confidence > conf {
			conf = ep.Confidence
		}
	}
	return matchedID, s.saveProvenance(ctx, "contact", matchedID, ref, conf, deriv, rc)
}

// appendChunkMemory writes a Memory for a promotion-candidate chunk (append-only).
func (s *Synthesizer) appendChunkMemory(ctx context.Context, rec schema.IntakeRecord, ref schema.IntakeRecordRef, in Input) (ItemOutcome, error) {
	dkey := derivationKey("memory", rec.ID, in.Chunk.ID)
	out := ItemOutcome{DerivationKey: dkey, EntityType: "memory", Kind: governor.MutationAppendHistory}

	exists, err := store.IntakeProvenanceExists(ctx, s.db, dkey)
	if err != nil {
		return out, err
	}
	if exists {
		out.Skipped = true
		return out, nil
	}

	conf := clamp01(in.Score.Score)
	deriv := []string{"record:" + rec.ID, "chunk:" + in.Chunk.ID}
	desc := governor.MutationDescriptor{
		Kind:          governor.MutationAppendHistory,
		TargetType:    "memory",
		SourceRecords: []schema.IntakeRecordRef{ref},
		Trust:         in.Chunk.Trust,
		PrivacyClass:  in.Chunk.PrivacyClass,
		Derivation:    deriv,
		EstConfidence: conf,
		JobMode:       schema.JobModeDelta,
	}
	result := governor.EvaluateMutation(desc, s.opts)
	out.Outcome = result.Outcome
	out.Confidence = conf
	if result.Outcome != governor.ValidationApproved {
		// Append-only history is unconditionally safe; a non-approval means the
		// write-class is disabled → drop honestly.
		if err := s.link(ctx, rec, in.Chunk.ID, "memory", "", governor.MutationAppendHistory, outcomeLabel(result.Outcome), dkey, conf, ""); err != nil {
			return out, err
		}
		return out, nil
	}

	m := schema.Memory{
		Scope:        "intake",
		ScopeID:      rec.ConnectorID,
		Summary:      summarize(in.Chunk.Content),
		Details:      in.Chunk.Content,
		Significance: string(in.Score.RetentionTier),
		Source:       "intake:" + rec.ConnectorID,
		Keywords:     chunkEntityNames(in.Chunk),
	}
	saved, err := store.SaveMemory(ctx, s.db, m)
	if err != nil {
		return out, fmt.Errorf("intake: synthesize: save memory: %w", err)
	}
	out.EntityID = saved.ID
	if err := s.saveProvenance(ctx, "memory", saved.ID, ref, conf, deriv, 0); err != nil {
		return out, err
	}
	if err := s.link(ctx, rec, in.Chunk.ID, "memory", saved.ID, governor.MutationAppendHistory, "approved", dkey, conf, ""); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Synthesizer) saveProvenance(ctx context.Context, entityType, entityID string, ref schema.IntakeRecordRef, conf float64, deriv []string, reinforcement int) error {
	if err := store.SaveEntityProvenance(ctx, s.db, entityType, entityID, schema.EntityProvenance{
		Source:             "intake:" + ref.ConnectorID,
		Timestamp:          s.now(),
		Confidence:         conf,
		DerivationChain:    deriv,
		ReinforcementCount: reinforcement,
	}); err != nil {
		return fmt.Errorf("intake: synthesize: save entity provenance: %w", err)
	}
	return nil
}

func (s *Synthesizer) link(ctx context.Context, rec schema.IntakeRecord, chunkID, entityType, entityID string, kind governor.MutationKind, outcome, dkey string, conf float64, proposalID string) error {
	_, err := store.SaveIntakeProvenanceLink(ctx, s.db, store.IntakeProvenanceLink{
		ID:             uuid.NewString(),
		IntakeRecordID: rec.ID,
		ChunkID:        chunkID,
		ConnectorID:    rec.ConnectorID,
		SourceID:       rec.SourceID,
		Cursor:         rec.Cursor,
		FetchedAt:      rec.FetchedAt,
		EntityType:     entityType,
		EntityID:       entityID,
		MutationKind:   kind.String(),
		Outcome:        outcome,
		DerivationKey:  dkey,
		Confidence:     conf,
		ProposalID:     proposalID,
	})
	if err != nil {
		return fmt.Errorf("intake: synthesize: save provenance link: %w", err)
	}
	return nil
}

// raiseSingleProposal raises one Proposal for a delta-mode confident merge.
func (s *Synthesizer) raiseSingleProposal(ctx context.Context, rec schema.IntakeRecord, candidateName string, refs []string, dkey string, conf float64) (string, error) {
	boundary := derivationKey("proposal", dkey)
	if existing, _ := store.GetPendingProposalByBoundaryKey(ctx, s.db, boundary); existing.ProposalID != "" {
		return existing.ProposalID, nil
	}
	affected, _ := json.Marshal(refs)
	p := schema.Proposal{
		ProposalID:       uuid.NewString(),
		SourceProcess:    "intake_synthesis",
		SourceTrigger:    rec.ConnectorID,
		BoundaryKey:      boundary,
		ProposedAction:   "merge_contacts:" + boundary,
		AffectedEntities: string(affected),
		Rationale:        fmt.Sprintf("Merge %d contact entities matched to %q from %s (delta sync).", len(refs), candidateName, rec.ConnectorID),
		Priority:         schema.ProposalPriorityQueued,
		Status:           schema.ProposalStatusPending,
	}
	if err := store.SaveProposal(ctx, s.db, p); err != nil {
		return "", fmt.Errorf("intake: synthesize: save proposal: %w", err)
	}
	return p.ProposalID, nil
}

// raiseGroupedProposal raises one grouped Proposal for a backfill batch's merges
// ("N contact merges from <connector> backfill — review as a set"). Grouping is a
// property of the synthesis batch; the governor still produced per-descriptor
// results (seam §12).
func (s *Synthesizer) raiseGroupedProposal(ctx context.Context, rec schema.IntakeRecord, pending []pendingMerge) (string, error) {
	keys := make([]string, len(pending))
	var affected []string
	for i, p := range pending {
		keys[i] = p.derivationKey
		affected = append(affected, p.candidateRefs...)
	}
	sort.Strings(keys)
	boundary := derivationKey(append([]string{"backfill-merge", rec.ConnectorID}, keys...)...)
	if existing, _ := store.GetPendingProposalByBoundaryKey(ctx, s.db, boundary); existing.ProposalID != "" {
		return existing.ProposalID, nil
	}
	affectedJSON, _ := json.Marshal(affected)
	p := schema.Proposal{
		ProposalID:       uuid.NewString(),
		SourceProcess:    "intake_synthesis",
		SourceTrigger:    rec.ConnectorID + ":backfill",
		BoundaryKey:      boundary,
		ProposedAction:   "merge_contacts_group:" + boundary,
		AffectedEntities: string(affectedJSON),
		Rationale:        fmt.Sprintf("%d contact merges from %s backfill — review as a set.", len(pending), rec.ConnectorID),
		Priority:         schema.ProposalPriorityQueued,
		Status:           schema.ProposalStatusPending,
	}
	if err := store.SaveProposal(ctx, s.db, p); err != nil {
		return "", fmt.Errorf("intake: synthesize: save grouped proposal: %w", err)
	}
	return p.ProposalID, nil
}

func contactTrustLevel(trust schema.ContentTrust) string {
	switch trust {
	case schema.ContentTrustOwner:
		return schema.ContactTrustLevelHigh
	case schema.ContentTrustInternalSystem:
		return schema.ContactTrustLevelMedium
	case schema.ContentTrustTrustedPlugin:
		return schema.ContactTrustLevelLow
	default:
		return schema.ContactTrustLevelUnknown
	}
}

func outcomeLabel(o governor.ValidationOutcome) string {
	switch o {
	case governor.ValidationApproved:
		return "approved"
	case governor.ValidationModified:
		return "modified"
	case governor.ValidationRequiresConfirmation:
		return "proposal"
	default:
		return "rejected"
	}
}

func summarize(content string) string {
	content = strings.TrimSpace(content)
	const max = 200
	if len(content) <= max {
		return content
	}
	return content[:max]
}

func chunkEntityNames(chunk schema.IntakeChunk) []string {
	var names []string
	for _, e := range chunk.Provenance.Entities {
		if e.Name != "" {
			names = append(names, e.Name)
		}
	}
	return names
}
