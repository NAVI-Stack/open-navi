package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/navi/proposals"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

// ContradictionChecker is an optional hook. When set, the worker calls it before
// applying fact writes; if it returns a non-empty contradiction and materialHarm,
// the worker emits an interruption instead of or in addition to applying the write.
type ContradictionChecker interface {
	Check(ctx context.Context, chatID, directiveID, key, value string) (contradiction string, materialHarm bool)
}

// Worker consumes reflection payloads from the bus (Conscious → Subconscious) and
// processes them at design cadence. Automatic escalation: when significance is high
// (or escalation_reason is set), payloads are escalated to Consolidation or Deep.
// ContradictionChecker, when set, can trigger EmitInterruption before fact writes.
type Worker struct {
	payloadCh              chan schema.ReflectionPayload
	mu                     sync.Mutex
	running                bool
	bus                    bus.Bus
	worldModel             *worldmodel.WorldModel
	saveProposal           func(ctx context.Context, p schema.Proposal) error
	saveExecutionOutcome   func(ctx context.Context, eo schema.ExecutionOutcome) error
	buildExperienceControl func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error)
	contradictionCheck     ContradictionChecker
	consolidationInterval  time.Duration
	interruptGuard         atomic.Bool
}

// NewWorker creates a reflection worker. Call Run to subscribe and start processing.
func NewWorker(wm *worldmodel.WorldModel) *Worker {
	return &Worker{
		payloadCh:             make(chan schema.ReflectionPayload, 256),
		worldModel:            wm,
		consolidationInterval: 30 * time.Minute,
	}
}

// SetSaveProposal wires a proposal persistence callback so reflection tiers
// can enqueue proposals without depending directly on a database handle.
func (w *Worker) SetSaveProposal(fn func(ctx context.Context, p schema.Proposal) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.saveProposal = fn
}

// SetSaveExecutionOutcome wires outcome persistence so reflection can record Create commands for memory/fact writes.
func (w *Worker) SetSaveExecutionOutcome(fn func(ctx context.Context, eo schema.ExecutionOutcome) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.saveExecutionOutcome = fn
}

// SetExperienceControlBuilder wires runtime-aligned experience rendering so
// reflection audit snapshots can reuse the active experience profiles.
func (w *Worker) SetExperienceControlBuilder(fn func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buildExperienceControl = fn
}

// SetContradictionChecker wires an optional checker; when set, the worker calls it
// before applying fact writes and may emit an interruption on contradiction + material harm.
func (w *Worker) SetContradictionChecker(c ContradictionChecker) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.contradictionCheck = c
}

// SetConsolidationInterval overrides the periodic subconscious consolidation cadence.
func (w *Worker) SetConsolidationInterval(interval time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if interval > 0 {
		w.consolidationInterval = interval
	}
}

// Run subscribes to FactReflectionQueued and starts the process loop. Blocks until ctx is done.
func (w *Worker) Run(ctx context.Context, b bus.Bus) error {
	if b == nil {
		return nil
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.bus = b
	w.mu.Unlock()

	if err := b.Subscribe(ctx, schema.FactReflectionQueued, "reflection-worker", func(ctx context.Context, ev schema.Event) {
		if ev.Payload == nil {
			return
		}
		data, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Debug("reflection: marshal payload", "error", err)
			return
		}
		var p schema.ReflectionPayload
		if err := json.Unmarshal(data, &p); err != nil {
			slog.Debug("reflection: unmarshal payload", "error", err)
			return
		}
		select {
		case w.payloadCh <- p:
		default:
			slog.Debug("reflection: queue full, dropping payload", "id", p.ID)
		}
	}); err != nil {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		return err
	}

	go w.processLoop(ctx)
	return nil
}

// processLoop drains the payload channel and processes queued reflection payloads
// frequently while also running periodic subconscious consolidation on its own cadence.
func (w *Worker) processLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	consolidationTicker := time.NewTicker(w.currentConsolidationInterval())
	defer consolidationTicker.Stop()
	var batch []schema.ReflectionPayload
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-w.payloadCh:
			batch = append(batch, p)
		case <-ticker.C:
			if len(batch) == 0 {
				continue
			}
			for _, p := range batch {
				switch p.Tier {
				case schema.ReflectionTierDeep:
					if p.EscalationReason == "manual" {
						w.processShallow(ctx, p)
					}
					w.processDeep(ctx, p)
				case schema.ReflectionTierConsolidation:
					w.processConsolidation(ctx, p)
				default:
					w.processShallow(ctx, p)
				}
			}
			batch = batch[:0]
		case <-consolidationTicker.C:
			w.runPeriodicConsolidation(ctx)
		}
	}
}

func (w *Worker) currentConsolidationInterval() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.consolidationInterval <= 0 {
		return 30 * time.Minute
	}
	return w.consolidationInterval
}

// Promotion policy (Shallow): Observations are promoted to Memory when kind=memory or
// EscalationReason=manual (significance: manual→high, else medium). Observations are
// promoted to Knowledge (facts) only when kind=fact and key+value are non-empty; provenance
// is recorded with source shallow_reflection. Configuration/Priorities are never mutated
// in Shallow; use Consolidation/Deep and the proposal queue for those.
func (w *Worker) processShallow(ctx context.Context, p schema.ReflectionPayload) {
	slog.Debug("reflection: shallow", "id", p.ID, "runtime_session_id", p.RuntimeSessionID, "tier", p.Tier, "summary", p.Summary)

	if w.worldModel == nil {
		return
	}

	details := parseReflectionDetails(p)
	db := w.worldModel.DB()
	if details.Category == "artifact" || strings.TrimSpace(details.ArtifactID) != "" {
		w.processArtifactShallow(ctx, p, details)
	}
	significance := details.Significance
	if significance == "" {
		if p.EscalationReason == "manual" {
			significance = "high"
		} else {
			significance = detectSignificance(p)
			if significance == "" {
				significance = "medium"
			}
		}
	}

	for _, signal := range details.preferenceSignals() {
		if db == nil {
			break
		}
		if signal.CreatedAt.IsZero() {
			signal.CreatedAt = p.CreatedAt
		}
		if signal.CreatedAt.IsZero() {
			signal.CreatedAt = time.Now().UTC()
		}
		if signal.UpdatedAt.IsZero() {
			signal.UpdatedAt = signal.CreatedAt
		}
		if signal.Status == "" {
			signal.Status = schema.PreferenceSignalStatusCaptured
		}
		if err := store.SavePreferenceSignal(ctx, db, signal); err != nil {
			slog.Warn("reflection: preference signal capture failed", "id", p.ID, "trait", signal.Trait, "error", err)
		}
	}

	// Heuristic: manual escalation always produces at least a memory.
	if details.Kind == "memory" || p.EscalationReason == "manual" {
		scope := details.Scope
		if scope == "" {
			scope = "chat"
		}
		scopeID := details.ScopeID
		if scopeID == "" {
			scopeID = p.RuntimeSessionID
		}

		mem := schema.Memory{
			Scope:        scope,
			ScopeID:      scopeID,
			Summary:      p.Summary,
			Details:      p.Details,
			Significance: significance,
			Source:       "shallow_reflection",
		}
		w.runCreateIfRecord(ctx, p.RuntimeSessionID, func(ctx context.Context) (any, error) {
			return w.worldModel.CreateOrUpdateMemory(ctx, mem)
		}, "reflection: shallow memory upsert failed", p.ID)
	}

	// Facts are optional; when present they are additive only and never mutate
	// owner-set configuration or priorities directly.
	for _, fact := range details.extractedFacts() {
		w.mu.Lock()
		checker := w.contradictionCheck
		w.mu.Unlock()
		if checker != nil {
			contradiction, materialHarm := checker.Check(ctx, p.RuntimeSessionID, p.DirectiveID, fact.Key, fact.Value)
			if materialHarm && contradiction != "" {
				w.EmitInterruption(ctx, schema.InterruptionAdvisory, "Possible contradiction with recent context", contradiction, contradiction)
				continue
			}
		}

		scope := strings.TrimSpace(fact.Scope)
		if scope == "" {
			scope = "chat"
		}
		scopeID := strings.TrimSpace(fact.ScopeID)
		if scopeID == "" {
			scopeID = details.ScopeID
		}
		if scopeID == "" {
			scopeID = p.RuntimeSessionID
		}

		factID := uuid.New().String()
		f := schema.Fact{
			ID:       factID,
			Scope:    scope,
			ScopeID:  scopeID,
			Category: fact.Category,
			Key:      fact.Key,
			Value:    fact.Value,
			Source:   "shallow_reflection",
		}
		prov := schema.EntityProvenance{
			Source:             "shallow_reflection",
			Timestamp:          time.Now().UTC(),
			Confidence:         0.7,
			ReinforcementCount: 1,
		}
		w.runCreateIfRecord(ctx, p.RuntimeSessionID, func(ctx context.Context) (any, error) {
			return w.worldModel.PromoteFact(ctx, f, prov)
		}, "reflection: shallow fact promotion failed", p.ID)
	}

	// Automatic escalation: when significance is high and no manual hint, escalate to
	// Consolidation so broader processing (relationships, re-evaluation, proposals) runs.
	if significance == "high" && p.EscalationReason != "manual" {
		p.EscalationReason = "automatic"
		w.processConsolidation(ctx, p)
	}
}

// Promotion policy (Consolidation): Direct mutations are limited to relationship
// decay/prune and re-evaluation of flagged entities. Any change that touches
// Configuration or Priorities is promoted to a Proposal (source_process "consolidation")
// and enqueued via saveProposal; no direct mutation of owner-set config/priorities.
func (w *Worker) processConsolidation(ctx context.Context, p schema.ReflectionPayload) {
	slog.Debug("reflection: consolidation", "id", p.ID, "runtime_session_id", p.RuntimeSessionID, "escalation", p.EscalationReason, "summary", p.Summary)

	details := parseReflectionDetails(p)
	if w.worldModel != nil {
		db := w.worldModel.DB()
		now := time.Now().UTC()
		cutoff := now.Add(-30 * 24 * time.Hour)
		steps := []command.Step{
			{
				CommandType:   schema.CommandTypeUpdate,
				Reversibility: schema.ReversibilityInternal,
				Run: func(ctx context.Context) (any, error) {
					return nil, w.worldModel.DecayRelationships(ctx, cutoff, 0.95, 100)
				},
			},
			{
				CommandType:   schema.CommandTypeUpdate,
				Reversibility: schema.ReversibilityInternal,
				Run: func(ctx context.Context) (any, error) {
					return nil, w.worldModel.PruneWeakRelationships(ctx, 0.05, 100)
				},
			},
			{
				CommandType:   schema.CommandTypeUpdate,
				Reversibility: schema.ReversibilityInternal,
				Run: func(ctx context.Context) (any, error) {
					return nil, w.worldModel.ReevaluateFlaggedEntities(ctx, 50, w.saveProposal)
				},
			},
		}
		_, comp, err := command.RunCompose(ctx, command.Descriptor{Type: schema.CommandTypeCompose, RuntimeSessionID: p.RuntimeSessionID}, steps, schema.ComposeFailureSurfacePartial, nil)
		if err != nil {
			slog.Debug("reflection: consolidation compose failed", "error", err, "outcome", comp)
		}
		if ownerID := resolvePreferenceSignalOwnerID(ctx, db, details); ownerID != "" {
			if err := w.consolidatePreferenceSignals(ctx, ownerID, p.EscalationReason, p.RuntimeSessionID); err != nil {
				slog.Debug("reflection: consolidate preference signals failed", "owner_id", ownerID, "error", err)
			}
		}
	}

	// When reflection suggests changes that may touch explicit/owner-set
	// configuration or priorities, enqueue a proposal instead of mutating
	// directly. We scope this to automatic escalations with structured details
	// that explicitly reference configuration or priorities.
	if w.saveProposal == nil || p.Details == "" || p.EscalationReason != "automatic" {
		return
	}

	if proposal := w.processArtifactConsolidation(ctx, p, details); proposal != nil {
		if err := w.saveProposal(ctx, *proposal); err != nil {
			slog.Warn("reflection: artifact consolidation proposal save failed", "id", p.ID, "error", err)
		} else {
			w.EmitInterruption(ctx, schema.InterruptionAdvisory, proposal.ProposedAction, proposal.Rationale, "")
		}
	}
	if details.Category == "workspace" {
		// Workspace control state is owner-set. Consolidation must not mutate it
		// or generate workspace proposals; only Deep Reflection may escalate it.
		return
	}
	if details.Category != "configuration" && details.Category != "priority" {
		// Relationship-only consolidation and non-owner-set updates do not
		// create proposals; they are handled directly via the world model.
		return
	}

	proposedAction := p.Summary
	if len(proposedAction) > 256 {
		proposedAction = proposedAction[:256]
	}
	affectedEntitiesJSON := buildAffectedEntitiesForReflection(p, details)

	proposal, err := proposals.BuildReflection(
		"consolidation",
		p.EscalationReason,
		proposedAction,
		affectedEntitiesJSON,
		p.Details,
	)
	if err != nil {
		slog.Warn("reflection: consolidation proposal build failed", "id", p.ID, "error", err)
		return
	}
	if err := w.saveProposal(ctx, proposal); err != nil {
		slog.Warn("reflection: consolidation proposal save failed", "id", p.ID, "error", err)
		return
	}
	// Surface consolidation proposal as advisory so Experience layer can show it without blocking.
	w.EmitInterruption(ctx, schema.InterruptionAdvisory, proposedAction, p.Details, "")
}

// Promotion policy (Deep): All high-impact changes are promoted to blocking Proposals
// (source_process "deep_reflection"). Manual "remember this" payloads are first
// persisted in Shallow and only produce deep proposals when they target
// configuration/priority changes.
func (w *Worker) processDeep(ctx context.Context, p schema.ReflectionPayload) {
	slog.Debug("reflection: deep", "id", p.ID, "runtime_session_id", p.RuntimeSessionID, "escalation", p.EscalationReason, "summary", p.Summary)

	// Deep reflection is reserved for high-impact changes. It never mutates
	// owner-set configuration or priorities directly; it only enqueues
	// blocking proposals for human review.
	if w.saveProposal == nil || p.Summary == "" {
		return
	}

	details := parseReflectionDetails(p)
	if proposal := w.processArtifactDeep(p, details); proposal != nil {
		if err := w.saveProposal(ctx, *proposal); err != nil {
			slog.Warn("reflection: deep artifact proposal save failed", "id", p.ID, "error", err)
			return
		}
		w.EmitInterruption(ctx, schema.InterruptionBlocking, proposal.ProposedAction, proposal.Rationale, "")
		return
	}
	if proposal := processWorkspaceDeep(p, details); proposal != nil {
		if err := w.saveProposal(ctx, *proposal); err != nil {
			slog.Warn("reflection: deep workspace proposal save failed", "id", p.ID, "error", err)
			return
		}
		w.EmitInterruption(ctx, schema.InterruptionBlocking, proposal.ProposedAction, proposal.Rationale, "")
		return
	}
	if details.Category != "configuration" && details.Category != "priority" {
		return
	}

	proposedAction := p.Summary
	if len(proposedAction) > 512 {
		proposedAction = proposedAction[:512]
	}
	affectedEntitiesJSON := buildAffectedEntitiesForReflection(p, details)

	proposal, err := proposals.BuildReflection(
		"deep_reflection",
		p.EscalationReason,
		proposedAction,
		affectedEntitiesJSON,
		p.Details,
	)
	if err != nil {
		slog.Warn("reflection: deep proposal build failed", "id", p.ID, "error", err)
		return
	}
	// Deep reflection proposals default to blocking.
	proposal.Priority = schema.ProposalPriorityBlocking
	if err := w.saveProposal(ctx, proposal); err != nil {
		slog.Warn("reflection: deep proposal save failed", "id", p.ID, "error", err)
		return
	}
	// Surface blocking proposal to Conscious process so Experience layer can present it.
	w.EmitInterruption(ctx, schema.InterruptionBlocking, proposedAction, p.Details, "")
}

// EmitInterruption publishes an urgent Subconscious → Conscious interruption when
// criteria are met (contradiction, high confidence, material harm). Recursion guard:
// only one interruption in flight; returns false if guard is set. Experience layer
// should subscribe to FactSubconsciousInterruption and call ClearInterruptionGuard
// when the user acknowledges or the next turn starts.
func (w *Worker) EmitInterruption(ctx context.Context, mode schema.InterruptionMode, summary, details, contradiction string) bool {
	if !w.interruptGuard.CompareAndSwap(false, true) {
		return false
	}
	if w.bus == nil {
		w.interruptGuard.Store(false)
		return false
	}
	payload := schema.SubconsciousInterruption{
		ID:            uuid.New().String(),
		Mode:          mode,
		Summary:       summary,
		Details:       details,
		Contradiction: contradiction,
		CreatedAt:     time.Now().UTC(),
	}
	ev := schema.NewEvent(schema.FactSubconsciousInterruption, schema.EventKindFact, payload.ID, schema.AgentNavi, payload)
	if err := w.bus.Publish(ctx, ev); err != nil {
		slog.Warn("reflection: emit interruption failed", "error", err)
		w.interruptGuard.Store(false)
		return false
	}
	return true
}

// ClearInterruptionGuard clears the recursion guard so a future interruption can be
// emitted. Call when the user acknowledges the interruption or at start of next turn.
func (w *Worker) ClearInterruptionGuard() {
	w.interruptGuard.Store(false)
}

// runCreateIfRecord runs fn; when saveExecutionOutcome is set, records it as a Create command.
func (w *Worker) runCreateIfRecord(ctx context.Context, runtimeSessionID string, fn func(context.Context) (any, error), logMsg, logID string) {
	w.mu.Lock()
	saveOutcome := w.saveExecutionOutcome
	w.mu.Unlock()
	var err error
	if saveOutcome != nil {
		exec := command.NewExecutor(saveOutcome)
		_, err = exec.Execute(ctx, command.Descriptor{Type: schema.CommandTypeCreate, RuntimeSessionID: runtimeSessionID}, fn)
	} else {
		_, err = fn(ctx)
	}
	if err != nil {
		slog.Warn(logMsg, "id", logID, "error", err)
	}
}

// detectSignificance returns "high" when summary or details signal importance
// (conflicts, priorities, configuration, anomalies, emotional weight); otherwise "".
// Used for automatic escalation to Consolidation when no manual hint is set.
func detectSignificance(p schema.ReflectionPayload) string {
	text := strings.ToLower(p.Summary + " " + p.Details)
	keywords := []string{
		"conflict", "contradict", "wrong", "incorrect", "override", "deprecate", "supersede",
		"priority", "priorities", "configuration", "anomaly", "anomalies", "important", "critical",
		"urgent", "harm", "risk", "violat", "reject", "blocking",
	}
	for _, k := range keywords {
		if strings.Contains(text, k) {
			return "high"
		}
	}
	if len(p.Summary) > 200 || len(p.Details) > 500 {
		return "high"
	}
	return ""
}

// reflectionDetails is an internal helper representation for structured
// reflection payload details. Details may be plain text or JSON; parsing
// is best-effort and falls back to zero values when JSON decoding fails.
type reflectionDetails struct {
	Kind              string                    `json:"kind,omitempty"`         // "memory" or "fact"
	Scope             string                    `json:"scope,omitempty"`        // "owner", "chat", "directive", etc.
	ScopeID           string                    `json:"scope_id,omitempty"`     // identifier within scope
	Category          string                    `json:"category,omitempty"`     // fact category
	Key               string                    `json:"key,omitempty"`          // fact key
	Value             string                    `json:"value,omitempty"`        // fact value
	Significance      string                    `json:"significance,omitempty"` // "low", "medium", "high"
	Summary           string                    `json:"summary,omitempty"`
	UserMessage       string                    `json:"user_message,omitempty"`
	Facts             []schema.ExtractedFact    `json:"facts,omitempty"`
	PreferenceSignals []schema.PreferenceSignal `json:"preference_signals,omitempty"`
	ArtifactID        string                    `json:"artifact_id,omitempty"`
	ArtifactTitle     string                    `json:"artifact_title,omitempty"`
	WorkspaceID       string                    `json:"workspace_id,omitempty"`
	ProjectID         string                    `json:"project_id,omitempty"`
	ArtifactType      string                    `json:"artifact_type,omitempty"`
	ArtifactSubtype   string                    `json:"artifact_subtype,omitempty"`
	Operation         string                    `json:"operation,omitempty"`
	RendererKey       string                    `json:"renderer_key,omitempty"`
	ResultStatus      string                    `json:"result_status,omitempty"`
	FailureClass      string                    `json:"failure_class,omitempty"`
	FailureCode       string                    `json:"failure_code,omitempty"`
	LifecycleState    string                    `json:"lifecycle_state,omitempty"`
	VersionID         string                    `json:"version_id,omitempty"`
	BranchID          string                    `json:"branch_id,omitempty"`
}

func parseReflectionDetails(p schema.ReflectionPayload) reflectionDetails {
	if p.Details == "" {
		return reflectionDetails{}
	}
	var out reflectionDetails
	if err := json.Unmarshal([]byte(p.Details), &out); err != nil {
		// Non-JSON details are expected; treat as plain-text details only.
		return reflectionDetails{}
	}
	return out
}

func (d reflectionDetails) extractedFacts() []schema.ExtractedFact {
	out := make([]schema.ExtractedFact, 0, len(d.Facts)+1)
	for _, fact := range d.Facts {
		if strings.TrimSpace(fact.Key) == "" || strings.TrimSpace(fact.Value) == "" || strings.TrimSpace(fact.Category) == "" {
			continue
		}
		if strings.TrimSpace(fact.Scope) == "" {
			fact.Scope = "owner"
		}
		out = append(out, fact)
	}
	if d.Kind == "fact" && d.Key != "" && d.Value != "" {
		scope := d.Scope
		if scope == "" {
			scope = "chat"
		}
		out = append(out, schema.ExtractedFact{
			Key:      d.Key,
			Value:    d.Value,
			Category: d.Category,
			Scope:    scope,
			ScopeID:  d.ScopeID,
		})
	}
	return out
}

func (d reflectionDetails) preferenceSignals() []schema.PreferenceSignal {
	out := make([]schema.PreferenceSignal, 0, len(d.PreferenceSignals))
	for _, signal := range d.PreferenceSignals {
		signal.Trait = strings.TrimSpace(signal.Trait)
		signal.Scope = strings.TrimSpace(signal.Scope)
		signal.ScopeID = strings.TrimSpace(signal.ScopeID)
		signal.ChatID = strings.TrimSpace(signal.ChatID)
		signal.Summary = strings.TrimSpace(signal.Summary)
		if signal.Trait == "" || signal.Scope == "" || signal.ScopeID == "" {
			continue
		}
		out = append(out, signal)
	}
	return out
}

func (w *Worker) processArtifactShallow(ctx context.Context, p schema.ReflectionPayload, d reflectionDetails) {
	if w.worldModel == nil || strings.TrimSpace(d.ArtifactID) == "" {
		return
	}
	artifactRef := "artifact:" + d.ArtifactID
	if d.Scope == "owner" && strings.TrimSpace(d.ScopeID) != "" {
		_ = w.worldModel.TouchRelationship(ctx, "owner:"+d.ScopeID, artifactRef, "owns", "artifact_reflection", 0.2)
	}
	if strings.TrimSpace(d.ProjectID) != "" {
		_ = w.worldModel.TouchRelationship(ctx, "project:"+d.ProjectID, artifactRef, "contains", "artifact_reflection", 0.15)
	}
	runtimeSessionID := strings.TrimSpace(p.RuntimeSessionID)
	if runtimeSessionID == "" && d.Scope == "chat" {
		runtimeSessionID = strings.TrimSpace(d.ScopeID)
	}
	if runtimeSessionID != "" {
		_ = w.worldModel.TouchRelationship(ctx, "chat:"+runtimeSessionID, artifactRef, "references", "artifact_reflection", 0.1)
	}
}

func (w *Worker) processArtifactConsolidation(ctx context.Context, p schema.ReflectionPayload, d reflectionDetails) *schema.Proposal {
	if w.worldModel == nil || strings.TrimSpace(d.ArtifactID) == "" {
		return nil
	}
	ownerID := strings.TrimSpace(d.ScopeID)
	if ownerID == "" && d.Scope == "owner" {
		ownerID = strings.TrimSpace(d.ScopeID)
	}
	if ownerID == "" {
		return nil
	}
	artifacts, err := w.worldModel.ListArtifactsForOwner(ctx, ownerID, 100)
	if err != nil {
		slog.Debug("reflection: artifact consolidation list failed", "artifact_id", d.ArtifactID, "error", err)
		return nil
	}

	currentRef := "artifact:" + d.ArtifactID
	var relatedCount int
	for _, item := range artifacts {
		if item.ID == d.ArtifactID {
			continue
		}
		sameProject := strings.TrimSpace(d.ProjectID) != "" && item.ProjectID == d.ProjectID
		sameSubtype := strings.TrimSpace(d.ArtifactSubtype) != "" && strings.EqualFold(item.Subtype, d.ArtifactSubtype)
		if !sameProject && !sameSubtype {
			continue
		}
		relatedCount++
		_ = w.worldModel.TouchRelationship(ctx, currentRef, "artifact:"+item.ID, "related", "artifact_consolidation", 0.08)
	}

	if w.saveProposal == nil {
		return nil
	}

	if strings.TrimSpace(d.FailureClass) != "" || strings.EqualFold(strings.TrimSpace(d.ResultStatus), "failed") || strings.EqualFold(strings.TrimSpace(d.LifecycleState), string(schema.ArtifactLifecycleErrored)) {
		action := fmt.Sprintf("Review recovery path for artifact %s", artifactDisplayName(d))
		rationale := fmt.Sprintf("Artifact operation %s for %s ended with result=%s failure_class=%s failure_code=%s lifecycle=%s.", firstNonEmptyReflection(d.Operation, "unknown"), artifactDisplayName(d), firstNonEmptyReflection(d.ResultStatus, "unknown"), firstNonEmptyReflection(d.FailureClass, "unknown"), firstNonEmptyReflection(d.FailureCode, "unknown"), firstNonEmptyReflection(d.LifecycleState, "unknown"))
		proposal, err := proposals.BuildReflection("consolidation", p.EscalationReason, action, buildAffectedEntitiesForReflection(p, d), rationale)
		if err != nil {
			slog.Warn("reflection: artifact consolidation proposal build failed", "artifact_id", d.ArtifactID, "error", err)
			return nil
		}
		return &proposal
	}

	if relatedCount >= 2 && strings.TrimSpace(d.ProjectID) != "" {
		action := fmt.Sprintf("Review %s artifact organization in project %s", firstNonEmptyReflection(d.ArtifactSubtype, "related"), d.ProjectID)
		rationale := fmt.Sprintf("Artifact %s is part of a %d-item %s cluster in project %s and may benefit from consolidation, grouping, or clearer lifecycle management.", artifactDisplayName(d), relatedCount+1, firstNonEmptyReflection(d.ArtifactSubtype, "artifact"), d.ProjectID)
		proposal, err := proposals.BuildReflection("consolidation", p.EscalationReason, action, buildAffectedEntitiesForReflection(p, d), rationale)
		if err != nil {
			slog.Warn("reflection: artifact consolidation proposal build failed", "artifact_id", d.ArtifactID, "error", err)
			return nil
		}
		return &proposal
	}

	return nil
}

func (w *Worker) processArtifactDeep(p schema.ReflectionPayload, d reflectionDetails) *schema.Proposal {
	if w.saveProposal == nil || strings.TrimSpace(d.ArtifactID) == "" {
		return nil
	}
	failureClass := strings.TrimSpace(strings.ToLower(d.FailureClass))
	resultStatus := strings.TrimSpace(strings.ToLower(d.ResultStatus))
	lifecycleState := strings.TrimSpace(strings.ToLower(d.LifecycleState))
	if failureClass == "" && resultStatus != "failed" && lifecycleState != string(schema.ArtifactLifecycleErrored) {
		return nil
	}

	action := fmt.Sprintf("Approve recovery plan for artifact %s", artifactDisplayName(d))
	rationale := fmt.Sprintf("Artifact %s needs operator review after %s with result=%s failure_class=%s failure_code=%s lifecycle=%s renderer=%s.", artifactDisplayName(d), firstNonEmptyReflection(d.Operation, "an artifact operation"), firstNonEmptyReflection(d.ResultStatus, "unknown"), firstNonEmptyReflection(d.FailureClass, "unknown"), firstNonEmptyReflection(d.FailureCode, "unknown"), firstNonEmptyReflection(d.LifecycleState, "unknown"), firstNonEmptyReflection(d.RendererKey, "unknown"))
	proposal, err := proposals.BuildReflection("deep_reflection", p.EscalationReason, action, buildAffectedEntitiesForReflection(p, d), rationale)
	if err != nil {
		slog.Warn("reflection: artifact deep proposal build failed", "artifact_id", d.ArtifactID, "error", err)
		return nil
	}
	proposal.Priority = schema.ProposalPriorityBlocking
	return &proposal
}

func artifactDisplayName(d reflectionDetails) string {
	return firstNonEmptyReflection(d.ArtifactTitle, d.ArtifactID, "artifact")
}

func firstNonEmptyReflection(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// buildAffectedEntitiesForReflection constructs the affected_entities list for
// proposals originating from reflection payloads. It uses the same "type:id"
// convention as other world-model helpers (e.g. owner:owner-1, config:theme).
func buildAffectedEntitiesForReflection(p schema.ReflectionPayload, d reflectionDetails) string {
	var ids []string

	// Scope-specific anchors.
	switch d.Scope {
	case "owner":
		if d.ScopeID != "" {
			ids = append(ids, "owner:"+d.ScopeID)
		}
	case "chat":
		if d.ScopeID != "" {
			ids = append(ids, "chat:"+d.ScopeID)
		} else if p.RuntimeSessionID != "" {
			ids = append(ids, "chat:"+p.RuntimeSessionID)
		}
	default:
		if d.ScopeID != "" {
			ids = append(ids, d.Scope+":"+d.ScopeID)
		}
	}

	// Configuration and priorities.
	if d.Category == "configuration" && d.Key != "" {
		ids = append(ids, "config:"+d.Key)
	}
	if d.Category == "priority" && d.Key != "" {
		ids = append(ids, "priority:"+d.Key)
	}
	if d.Category == "artifact" && d.ArtifactID != "" {
		ids = append(ids, "artifact:"+d.ArtifactID)
		if d.WorkspaceID != "" {
			ids = append(ids, "workspace:"+d.WorkspaceID)
		}
		if d.ProjectID != "" {
			ids = append(ids, "project:"+d.ProjectID)
		}
	}
	if d.Category == "workspace" {
		workspaceID := strings.TrimSpace(d.WorkspaceID)
		if workspaceID == "" && d.Scope == "workspace" {
			workspaceID = strings.TrimSpace(d.ScopeID)
		}
		if workspaceID != "" {
			ids = append(ids, "workspace:"+workspaceID)
		}
		switch normalizeWorkspaceControlKey(d.Key) {
		case "active_workspace_id":
			ids = append(ids, "config:active_workspace_id")
		case "protected_paths":
			ids = append(ids, "config:protected_paths")
		}
	}

	if len(ids) == 0 {
		b, _ := json.Marshal([]string{})
		return string(b)
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func processWorkspaceDeep(p schema.ReflectionPayload, d reflectionDetails) *schema.Proposal {
	if d.Category != "workspace" || !isWorkspaceControlStateKey(d.Key) {
		return nil
	}

	proposedAction := strings.TrimSpace(p.Summary)
	if proposedAction == "" {
		proposedAction = fmt.Sprintf("Review workspace control-state change for %s", firstNonEmptyReflection(d.WorkspaceID, d.ScopeID, "workspace"))
	}
	if len(proposedAction) > 512 {
		proposedAction = proposedAction[:512]
	}

	target := firstNonEmptyReflection(d.WorkspaceID, d.ScopeID, "workspace")
	rationale := fmt.Sprintf("Deep Reflection identified a proposed change to owner-controlled workspace state (%s) for %s. Workspace boundaries, active selection, whitelist rules, protected paths, and allowed actions may only change through the Proposal Queue.", firstNonEmptyReflection(normalizeWorkspaceControlKey(d.Key), "workspace_state"), target)
	if strings.TrimSpace(p.Details) != "" {
		rationale = p.Details
	}

	proposal, err := proposals.BuildReflection(
		"deep_reflection",
		p.EscalationReason,
		proposedAction,
		buildAffectedEntitiesForReflection(p, d),
		rationale,
	)
	if err != nil {
		slog.Warn("reflection: workspace deep proposal build failed", "workspace_id", d.WorkspaceID, "error", err)
		return nil
	}
	proposal.Priority = schema.ProposalPriorityBlocking
	return &proposal
}

func isWorkspaceControlStateKey(raw string) bool {
	switch normalizeWorkspaceControlKey(raw) {
	case "local_roots", "repo_roots", "active_workspace_id", "whitelist_rules", "protected_paths", "allowed_actions":
		return true
	default:
		return false
	}
}

func normalizeWorkspaceControlKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	raw = strings.ReplaceAll(raw, " ", "_")
	switch raw {
	case "active_workspace", "workspace_selection":
		return "active_workspace_id"
	case "whitelist_rule", "whitelist_rules", "whitelist":
		return "whitelist_rules"
	default:
		return raw
	}
}
