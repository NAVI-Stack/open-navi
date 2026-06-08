package worldmodel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const defaultWorkspaceOperatingMode = schema.WorkspaceOperatingModeGlobal

// WorldModel is a thin façade over the store package for world-model entities.
type WorldModel struct {
	db *sql.DB
}

// New creates a new WorldModel façade.
func New(db *sql.DB) *WorldModel {
	return &WorldModel{db: db}
}

// DB exposes the underlying database handle for coordinated world-model writes
// that still need store-level helpers.
func (wm *WorldModel) DB() *sql.DB {
	return wm.db
}

// GetOwnerTimezone returns the IANA timezone string for the owner, or "" if not configured.
// Callers that need a safe default should fall back to "UTC" when the return value is empty.
func (wm *WorldModel) GetOwnerTimezone(ctx context.Context) string {
	o, ok, err := store.GetOwner(ctx, wm.db)
	if err != nil || !ok || o.Timezone == "" {
		return ""
	}
	return o.Timezone
}

// TimezoneLocation returns the configured timezone as a *time.Location.
// When no timezone has been explicitly set it falls back to time.Local
// (the server's OS timezone), which for a personal agent is typically
// the owner's own machine and therefore the correct local time.
func (wm *WorldModel) TimezoneLocation(ctx context.Context) *time.Location {
	o, ok, err := store.GetOwner(ctx, wm.db)
	if err != nil || !ok || o.Timezone == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(o.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// SetOwnerTimezone persists the owner's IANA timezone preference.
func (wm *WorldModel) SetOwnerTimezone(ctx context.Context, tz string) error {
	return store.UpdateOwnerTimezone(ctx, wm.db, tz)
}

// UpsertContact inserts or updates a contact and returns the persisted value.
func (wm *WorldModel) UpsertContact(ctx context.Context, c schema.Contact) (schema.Contact, error) {
	if c.ID == "" {
		return schema.Contact{}, fmt.Errorf("worldmodel: contact id required")
	}
	if err := store.SaveContact(ctx, wm.db, c); err != nil {
		return schema.Contact{}, err
	}
	return store.GetContact(ctx, wm.db, c.ID)
}

// GetContact loads a contact by ID.
func (wm *WorldModel) GetContact(ctx context.Context, id string) (schema.Contact, error) {
	return store.GetContact(ctx, wm.db, id)
}

// ListContactsByKindAndPrefix returns contacts filtered by kind and name prefix.
func (wm *WorldModel) ListContactsByKindAndPrefix(ctx context.Context, kind, namePrefix string, limit int) ([]schema.Contact, error) {
	return store.ListContactsByKindAndPrefix(ctx, wm.db, kind, namePrefix, limit)
}

// UpsertEvent inserts or updates a world-model event and returns the persisted value.
func (wm *WorldModel) UpsertEvent(ctx context.Context, e schema.WorldModelEvent) (schema.WorldModelEvent, error) {
	if err := store.SaveWorldModelEvent(ctx, wm.db, e); err != nil {
		return schema.WorldModelEvent{}, err
	}
	events, err := store.ListWorldModelEvents(ctx, wm.db, e.OwnerID, e.StartTime, e.StartTime.Add(time.Nanosecond), 1)
	if err != nil {
		return schema.WorldModelEvent{}, err
	}
	if len(events) == 0 {
		return e, nil
	}
	return events[0], nil
}

// ListEventsForOwner returns events for an owner in a time range.
func (wm *WorldModel) ListEventsForOwner(ctx context.Context, ownerID string, start, end time.Time, limit int) ([]schema.WorldModelEvent, error) {
	return store.ListWorldModelEvents(ctx, wm.db, ownerID, start, end, limit)
}

// CreateOrUpdateMemory inserts or updates a memory and returns the persisted value.
func (wm *WorldModel) CreateOrUpdateMemory(ctx context.Context, m schema.Memory) (schema.Memory, error) {
	saved, err := store.SaveMemory(ctx, wm.db, m)
	if err != nil {
		return schema.Memory{}, err
	}
	persisted, err := store.GetMemory(ctx, wm.db, saved.ID)
	if err != nil {
		return schema.Memory{}, err
	}
	if err := wm.refreshKnowledgeLinks(ctx, "memory", persisted.ID); err != nil {
		return schema.Memory{}, err
	}
	return store.GetMemory(ctx, wm.db, saved.ID)
}

// ForgetMemoryByID applies the Forget lifecycle to a memory by ID.
// Caller is responsible for ensuring the memory exists; downstream
// re-evaluation flags are handled by the store layer.
func (wm *WorldModel) ForgetMemoryByID(ctx context.Context, id string) error {
	return store.ForgetMemory(ctx, wm.db, id)
}

// ArchiveEntity records an Archive lifecycle action (soft-delete) for the given
// entity type and ID, and auto-declines any pending proposals that reference it.
func (wm *WorldModel) ArchiveEntity(ctx context.Context, entityType, entityID string) error {
	return store.ArchiveEntity(ctx, wm.db, entityType, entityID)
}

// TombstoneEntity records a Tombstone (hard-delete), removes the row from the
// corresponding table, and auto-declines pending proposals for this entity.
// Pass proposalID when executing from an approved proposal (e.g. hard_delete_requested).
func (wm *WorldModel) TombstoneEntity(ctx context.Context, entityType, entityID, proposalID string) error {
	return store.TombstoneEntity(ctx, wm.db, entityType, entityID, proposalID)
}

// ListMemories returns memories for a given scope and scopeID.
func (wm *WorldModel) ListMemories(ctx context.Context, scope, scopeID string, limit int) ([]schema.Memory, error) {
	return store.ListMemories(ctx, wm.db, scope, scopeID, limit)
}

// GetMemory returns a single memory by ID.
func (wm *WorldModel) GetMemory(ctx context.Context, id string) (schema.Memory, error) {
	return store.GetMemory(ctx, wm.db, id)
}

// UpsertArtifact inserts or updates an artifact and returns the persisted value.
func (wm *WorldModel) UpsertArtifact(ctx context.Context, a schema.Artifact) (schema.Artifact, error) {
	if err := store.SaveArtifact(ctx, wm.db, a); err != nil {
		return schema.Artifact{}, err
	}
	artifacts, err := store.ListArtifactsForOwner(ctx, wm.db, a.OwnerID, 1)
	if err != nil {
		return schema.Artifact{}, err
	}
	if len(artifacts) == 0 {
		return a, nil
	}
	return artifacts[0], nil
}

// ListArtifactsForOwner returns artifacts owned by the given owner.
func (wm *WorldModel) ListArtifactsForOwner(ctx context.Context, ownerID string, limit int) ([]schema.Artifact, error) {
	return store.ListArtifactsForOwner(ctx, wm.db, ownerID, limit)
}

// UpsertWorkspace inserts or updates a workspace and returns the persisted value.
func (wm *WorldModel) UpsertWorkspace(ctx context.Context, w schema.Workspace) (schema.Workspace, error) {
	if w.ID == "" {
		return schema.Workspace{}, fmt.Errorf("worldmodel: workspace id required")
	}
	if err := store.SaveWorkspace(ctx, wm.db, w); err != nil {
		return schema.Workspace{}, err
	}
	persisted, err := store.GetWorkspace(ctx, wm.db, w.ID)
	if err != nil {
		return schema.Workspace{}, err
	}
	if persisted.Status != schema.WorkspaceStatusActive && wm.GetExplicitActiveWorkspaceID(ctx) == persisted.ID {
		if err := wm.SetActiveWorkspaceID(ctx, ""); err != nil {
			return schema.Workspace{}, err
		}
	}
	return persisted, nil
}

// GetWorkspace returns a single workspace by ID.
func (wm *WorldModel) GetWorkspace(ctx context.Context, id string) (schema.Workspace, error) {
	return store.GetWorkspace(ctx, wm.db, id)
}

// GetWorkspaceByProjectID retrieves a workspace bound to the given project ID.
func (wm *WorldModel) GetWorkspaceByProjectID(ctx context.Context, projectID string) (schema.Workspace, error) {
	return store.GetWorkspaceByProjectID(ctx, wm.db, projectID)
}

// ListWorkspaces returns workspaces, optionally filtered by status.
func (wm *WorldModel) ListWorkspaces(ctx context.Context, status string) ([]schema.Workspace, error) {
	return store.ListWorkspaces(ctx, wm.db, status)
}

// ArchiveWorkspace marks a workspace as archived and returns the persisted value.
func (wm *WorldModel) ArchiveWorkspace(ctx context.Context, id string) (schema.Workspace, error) {
	if strings.TrimSpace(id) == "" {
		return schema.Workspace{}, fmt.Errorf("worldmodel: workspace id required")
	}
	if err := store.ArchiveWorkspace(ctx, wm.db, id, time.Now().UTC()); err != nil {
		return schema.Workspace{}, err
	}
	if wm.GetExplicitActiveWorkspaceID(ctx) == strings.TrimSpace(id) {
		if err := wm.SetActiveWorkspaceID(ctx, ""); err != nil {
			return schema.Workspace{}, err
		}
	}
	return store.GetWorkspace(ctx, wm.db, id)
}

// DeleteWorkspace permanently deletes an unreferenced workspace.
func (wm *WorldModel) DeleteWorkspace(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("worldmodel: workspace id required")
	}
	return store.DeleteWorkspace(ctx, wm.db, id)
}

// UpsertWhitelistRule inserts or updates a whitelist rule for a workspace.
func (wm *WorldModel) UpsertWhitelistRule(ctx context.Context, r schema.WhitelistRule, workspaceID string) error {
	if r.RuleID == "" {
		return fmt.Errorf("worldmodel: rule id required")
	}
	return store.SaveWhitelistRule(ctx, wm.db, r, workspaceID)
}

// ListWhitelistRules returns all rules for a workspace.
func (wm *WorldModel) ListWhitelistRules(ctx context.Context, workspaceID string) ([]schema.WhitelistRule, error) {
	return store.ListWhitelistRules(ctx, wm.db, workspaceID)
}

// ListFacts returns facts for the given scope and scopeID.
func (wm *WorldModel) ListFacts(ctx context.Context, scope, scopeID string, includeGlobal bool, limit int) ([]schema.Fact, error) {
	raw, err := store.ListFacts(ctx, wm.db, scope, scopeID, includeGlobal, limit, false)
	if err != nil {
		return nil, err
	}
	out := make([]schema.Fact, 0, len(raw))
	for _, f := range raw {
		out = append(out, schema.Fact{
			ID:         f.ID,
			Scope:      f.Scope,
			ScopeID:    f.ScopeID,
			Category:   f.Category,
			Key:        f.Key,
			Value:      f.Value,
			Keywords:   f.Keywords,
			Tags:       f.Tags,
			Embedding:  f.Embedding,
			Source:     f.Source,
			Deprecated: f.Deprecated,
			CreatedAt:  f.CreatedAt,
			UpdatedAt:  f.UpdatedAt,
		})
	}
	return out, nil
}

// GetOperatingMode returns the system-level workspace operating mode (global, scoped, hybrid).
func (wm *WorldModel) GetOperatingMode(ctx context.Context) schema.WorkspaceOperatingMode {
	val, ok, err := store.GetConfigurationValue(ctx, wm.db, "global", "system", "workspace_operating_mode")
	if err != nil || !ok {
		return defaultWorkspaceOperatingMode
	}
	mode, err := schema.ParseWorkspaceOperatingMode(val)
	if err != nil {
		return defaultWorkspaceOperatingMode
	}
	return mode
}

// SetOperatingMode sets the system-level workspace operating mode.
func (wm *WorldModel) SetOperatingMode(ctx context.Context, mode string) error {
	parsed, err := schema.ParseWorkspaceOperatingMode(mode)
	if err != nil {
		return fmt.Errorf("worldmodel: %w", err)
	}
	now := time.Now().UTC()
	e := schema.ConfigurationEntry{
		ID:        "workspace_operating_mode",
		Scope:     "global",
		ScopeID:   "system",
		Key:       "workspace_operating_mode",
		Value:     string(parsed),
		Source:    string(schema.StateKindOwnerSet),
		CreatedAt: now,
		UpdatedAt: now,
	}
	return store.SaveConfigurationEntry(ctx, wm.db, e)
}

// GetExplicitActiveWorkspaceID returns the user-selected active workspace ID, if any.
func (wm *WorldModel) GetExplicitActiveWorkspaceID(ctx context.Context) string {
	val, ok, err := store.GetConfigurationValue(ctx, wm.db, "owner", "active", "active_workspace_id")
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// GetActiveWorkspaceID returns the user-selected active workspace ID, if any.
func (wm *WorldModel) GetActiveWorkspaceID(ctx context.Context) string {
	return wm.GetExplicitActiveWorkspaceID(ctx)
}

// SetActiveWorkspaceID sets the user-selected active workspace ID.
func (wm *WorldModel) SetActiveWorkspaceID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id != "" {
		workspace, err := store.GetWorkspace(ctx, wm.db, id)
		if err != nil {
			return err
		}
		if workspace.Status != schema.WorkspaceStatusActive {
			return fmt.Errorf("worldmodel: active workspace must have status %q", schema.WorkspaceStatusActive)
		}
	}
	now := time.Now().UTC()
	e := schema.ConfigurationEntry{
		ID:        "active_workspace_id",
		Scope:     "owner",
		ScopeID:   "active",
		Key:       "active_workspace_id",
		Value:     id,
		Source:    "explicit",
		CreatedAt: now,
		UpdatedAt: now,
	}
	return store.SaveConfigurationEntry(ctx, wm.db, e)
}

// ResolveActiveWorkspace applies the V1 selection rules: explicit selection wins,
// otherwise a project-bound workspace may govern, otherwise none is active.
func (wm *WorldModel) ResolveActiveWorkspace(ctx context.Context, projectID string) (*schema.Workspace, string, error) {
	if explicitID := wm.GetExplicitActiveWorkspaceID(ctx); explicitID != "" {
		workspace, err := store.GetWorkspace(ctx, wm.db, explicitID)
		if err != nil || workspace.Status != schema.WorkspaceStatusActive {
			return nil, "none", nil
		}
		return &workspace, "explicit", nil
	}

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, "none", nil
	}

	workspace, err := wm.GetProjectWorkspace(ctx, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no active workspace bound to project") ||
			strings.Contains(err.Error(), "project not found") ||
			strings.Contains(err.Error(), "workspace not found") {
			return nil, "none", nil
		}
		return nil, "none", err
	}
	if workspace.Status != schema.WorkspaceStatusActive {
		return nil, "none", nil
	}
	return &workspace, "project_bound", nil
}

// FactsBlockForScope returns a markdown block of facts for the given scope suitable for
// injection into a system prompt. Used by orchestrator and other directive-scoped consumers.
func (wm *WorldModel) FactsBlockForScope(ctx context.Context, scope, scopeID string, includeGlobal bool, limit int) (string, error) {
	raw, err := store.ListFacts(ctx, wm.db, scope, scopeID, includeGlobal, limit, false)
	if err != nil {
		return "", err
	}
	return store.FormatFactsForPrompt(raw), nil
}

// PromoteFact inserts or updates a fact in the Knowledge store and records
// its provenance. The fact.ID must be set by the caller so provenance can
// be associated deterministically.
func (wm *WorldModel) PromoteFact(ctx context.Context, f schema.Fact, prov schema.EntityProvenance) (schema.Fact, error) {
	if f.ID == "" {
		return schema.Fact{}, fmt.Errorf("worldmodel: fact id required for PromoteFact")
	}

	sf := store.Fact{
		ID:         f.ID,
		Scope:      f.Scope,
		ScopeID:    f.ScopeID,
		Category:   f.Category,
		Key:        f.Key,
		Value:      f.Value,
		Keywords:   f.Keywords,
		Tags:       f.Tags,
		Embedding:  f.Embedding,
		Source:     f.Source,
		Deprecated: f.Deprecated,
	}
	if err := store.SaveFact(ctx, wm.db, sf); err != nil {
		return schema.Fact{}, err
	}

	if prov.Timestamp.IsZero() {
		prov.Timestamp = time.Now().UTC()
	}
	if prov.Confidence <= 0 {
		prov.Confidence = 1.0
	}
	if err := store.SaveEntityProvenance(ctx, wm.db, "fact", f.ID, prov); err != nil {
		return schema.Fact{}, err
	}
	if err := wm.refreshKnowledgeLinks(ctx, "fact", f.ID); err != nil {
		return schema.Fact{}, err
	}

	saved, err := store.GetFact(ctx, wm.db, f.ID)
	if err != nil {
		return schema.Fact{}, err
	}
	return schema.Fact{
		ID:         saved.ID,
		Scope:      saved.Scope,
		ScopeID:    saved.ScopeID,
		Category:   saved.Category,
		Key:        saved.Key,
		Value:      saved.Value,
		Keywords:   saved.Keywords,
		Tags:       saved.Tags,
		Embedding:  saved.Embedding,
		Source:     saved.Source,
		Deprecated: saved.Deprecated,
		CreatedAt:  saved.CreatedAt,
		UpdatedAt:  saved.UpdatedAt,
	}, nil
}

// SupersedeFactWith records that newFactID supersedes oldFactID and updates
// provenance mutation history for the superseded fact. Callers are responsible
// for creating the new fact before calling this helper.
//
// Owner-set Knowledge: for owner-set (explicit) facts, callers must run
// ValidateAction with governor.ActionDescriptorForLifecycleSupersede(..., true).
// If outcome is RequiresConfirmation, create a proposal and only call SupersedeFactWith
// after the owner approves. Inferred Knowledge may be superseded without a proposal.
func (wm *WorldModel) SupersedeFactWith(ctx context.Context, oldFactID, newFactID string) error {
	if oldFactID == "" || newFactID == "" {
		return fmt.Errorf("worldmodel: both oldFactID and newFactID are required")
	}
	if err := store.SupersedeFact(ctx, wm.db, oldFactID, newFactID); err != nil {
		return err
	}
	prov, err := store.GetEntityProvenance(ctx, wm.db, "fact", oldFactID)
	if err != nil {
		return err
	}
	if prov == nil {
		prov = &schema.EntityProvenance{
			Source:     "consolidation",
			Timestamp:  time.Now().UTC(),
			Confidence: 1.0,
		}
	}
	prov.MutationHistory = append(prov.MutationHistory, "superseded_by:"+newFactID)
	return store.SaveEntityProvenance(ctx, wm.db, "fact", oldFactID, *prov)
}

// DeprecateFact marks a fact as deprecated (soft-delete without replacement).
// For owner-set Knowledge, callers must run ValidateAction with
// governor.ActionDescriptorForLifecycleDeprecate(..., true) and create a proposal
// when outcome is RequiresConfirmation; only call DeprecateFact after approval.
// Inferred Knowledge may be deprecated without a proposal within reflection tier scope.
func (wm *WorldModel) DeprecateFact(ctx context.Context, factID string) error {
	if err := store.DeprecateFact(ctx, wm.db, factID); err != nil {
		return err
	}
	prov, err := store.GetEntityProvenance(ctx, wm.db, "fact", factID)
	if err != nil || prov == nil {
		return nil
	}
	prov.MutationHistory = append(prov.MutationHistory, "deprecated")
	return store.SaveEntityProvenance(ctx, wm.db, "fact", factID, *prov)
}

// RunSummary returns the execution-outcome record for a run, looked up by its
// attempt ID (the identifier the runs API uses as "run id"). It is a read-only
// path over the existing execution ledger; the second return is false when no
// such run exists. Used by the governed query_context read surface.
func (wm *WorldModel) RunSummary(ctx context.Context, runID string) (*schema.ExecutionOutcome, bool, error) {
	eo, err := store.GetExecutionOutcomeByAttemptID(ctx, wm.db, runID)
	if err != nil {
		return nil, false, err
	}
	if eo == nil {
		return nil, false, nil
	}
	return eo, true, nil
}

// AssembleUserModel builds the composite user model for the given owner.
func (wm *WorldModel) AssembleUserModel(ctx context.Context, ownerID string, factsLimit, memoriesLimit, configLimit, prioritiesLimit int) (schema.UserModel, error) {
	return store.AssembleUserModel(ctx, wm.db, ownerID, factsLimit, memoriesLimit, configLimit, prioritiesLimit)
}

// LoadOwnerUserModel loads the owner-scoped UserModel using sensible defaults
// for slice limits. It is a convenience wrapper around AssembleUserModel.
func (wm *WorldModel) LoadOwnerUserModel(ctx context.Context, ownerID string) (schema.UserModel, error) {
	const (
		defaultFactsLimit      = 30
		defaultMemoriesLimit   = 20
		defaultConfigLimit     = 50
		defaultPrioritiesLimit = 20
	)
	return wm.AssembleUserModel(ctx, ownerID, defaultFactsLimit, defaultMemoriesLimit, defaultConfigLimit, defaultPrioritiesLimit)
}

// ListConfigurationByScope returns configuration entries for the given scope and scope ID.
func (wm *WorldModel) ListConfigurationByScope(ctx context.Context, scope, scopeID string, limit int) ([]schema.ConfigurationEntry, error) {
	return store.ListConfigurationByScope(ctx, wm.db, scope, scopeID, limit)
}

// ListPrioritiesByScope returns priorities for the given scope and scope ID.
func (wm *WorldModel) ListPrioritiesByScope(ctx context.Context, scope, scopeID string, limit int) ([]schema.Priority, error) {
	return store.ListPrioritiesByScope(ctx, wm.db, scope, scopeID, limit)
}

// TouchRelationship strengthens or creates a relationship between two entities.
// Confidence is adjusted by delta and clamped to [0,1]; recency is set to now.
func (wm *WorldModel) TouchRelationship(ctx context.Context, fromID, toID, relType, provenance string, delta float64) error {
	now := time.Now().UTC()
	rel, err := store.GetRelationshipByEndpoints(ctx, wm.db, fromID, toID, relType)
	if err != nil {
		return err
	}
	if rel == nil {
		rel = &schema.Relationship{
			FromEntityID:     fromID,
			ToEntityID:       toID,
			RelationshipType: relType,
			Confidence:       clamp01(delta),
			Recency:          now,
			Provenance:       provenance,
		}
	} else {
		rel.Confidence = clamp01(rel.Confidence + delta)
		rel.Recency = now
		if provenance != "" {
			rel.Provenance = provenance
		}
	}
	return store.SaveRelationship(ctx, wm.db, *rel)
}

// DecayRelationships applies multiplicative decay to relationships older than cutoff.
func (wm *WorldModel) DecayRelationships(ctx context.Context, cutoff time.Time, factor float64, limit int) error {
	if factor <= 0 {
		return fmt.Errorf("worldmodel: decay factor must be > 0")
	}
	rels, err := store.ListRelationshipsOlderThan(ctx, wm.db, cutoff, limit)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		rel.Confidence = clamp01(rel.Confidence * factor)
		if err := store.SaveRelationship(ctx, wm.db, rel); err != nil {
			return err
		}
	}
	return nil
}

// PruneWeakRelationships deletes relationships whose confidence is below threshold.
func (wm *WorldModel) PruneWeakRelationships(ctx context.Context, threshold float64, limit int) error {
	rels, err := store.ListRelationshipsBelowConfidence(ctx, wm.db, threshold, limit)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if err := store.DeleteRelationship(ctx, wm.db, rel.ID); err != nil {
			return err
		}
	}
	return nil
}

// ReevaluateFlaggedEntities inspects pending re_evaluation_flags and enqueues
// proposals to re-evaluate affected entities. Callers provide a saveProposal
// callback; when nil, the method is a no-op. This keeps semantics aligned
// with the conceptual design while leaving detailed re-evaluation reasoning
// to higher-level processes.
func (wm *WorldModel) ReevaluateFlaggedEntities(ctx context.Context, limit int, saveProposal func(ctx context.Context, p schema.Proposal) error) error {
	if saveProposal == nil {
		return nil
	}
	flags, err := store.ListReEvaluationFlags(ctx, wm.db, limit)
	if err != nil {
		return err
	}
	for _, f := range flags {
		// Affected entity reference uses the same "type:id" convention as proposals.
		affected := fmt.Sprintf("%s:%s", f.EntityType, f.EntityID)
		affectedJSON, _ := json.Marshal([]string{affected})
		proposedAction := fmt.Sprintf("re-evaluate %s %s", f.EntityType, f.EntityID)
		proposal, err := store.PrepareProposalFromReflection(
			"consolidation",
			f.Reason,
			proposedAction,
			string(affectedJSON),
			fmt.Sprintf("Source: %s:%s", f.SourceEntityType, f.SourceEntityID),
		)
		if err != nil {
			return err
		}
		if err := saveProposal(ctx, proposal); err != nil {
			return err
		}
		if err := store.ClearReEvaluationFlag(ctx, wm.db, f.ID); err != nil {
			return err
		}
	}
	return nil
}

// RecordInteractionEvent writes a minimally interpreted interaction event for later reflection.
func (wm *WorldModel) RecordInteractionEvent(ctx context.Context, ownerID, chatID string, metadata map[string]any) error {
	if ownerID == "" {
		ownerID = "owner:unknown"
	}
	payload := map[string]any{
		"chat_id": chatID,
		"meta":    metadata,
	}
	b, _ := json.Marshal(payload)
	ev := schema.WorldModelEvent{
		OwnerID:   ownerID,
		Title:     "interaction",
		StartTime: time.Now().UTC(),
		Kind:      "interaction",
		Source:    "navi_chat",
		Metadata:  string(b),
	}
	return store.SaveWorldModelEvent(ctx, wm.db, ev)
}

// ContextBlockForChat assembles a compact context block for prompts based
// on the owner's UserModel (facts, memories, configuration, priorities) plus
// any chat-scoped facts and memories for the active chat.
// It is intended for injection into system prompts and should remain concise.
func (wm *WorldModel) ContextBlockForChat(ctx context.Context, ownerID, chatID string, factsLimit, memoriesLimit, configLimit, prioritiesLimit int) (string, error) {
	if ownerID == "" {
		return "", nil
	}
	um, err := wm.AssembleUserModel(ctx, ownerID, factsLimit, memoriesLimit, configLimit, prioritiesLimit)
	if err != nil {
		return "", err
	}
	if um.OwnerContact == nil && len(um.Facts) == 0 && len(um.Memories) == 0 && len(um.Configuration) == 0 && len(um.Priorities) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("\n\n## Owner context\n\n")

	if um.OwnerContact != nil {
		b.WriteString("- Name: " + um.OwnerContact.Name + "\n")
		if um.OwnerContact.Kind != "" {
			b.WriteString("- Kind: " + um.OwnerContact.Kind + "\n")
		}
		b.WriteString("\n")
	}

	if len(um.Facts) > 0 {
		// Reuse the existing fact prompt formatting by mapping back to store.Fact.
		facts := make([]store.Fact, 0, len(um.Facts))
		for _, f := range um.Facts {
			facts = append(facts, store.Fact{
				ID:         f.ID,
				Scope:      f.Scope,
				ScopeID:    f.ScopeID,
				Category:   f.Category,
				Key:        f.Key,
				Value:      f.Value,
				Source:     f.Source,
				Deprecated: f.Deprecated,
			})
		}
		b.WriteString(store.FormatFactsForPrompt(facts))
	}

	if len(um.Memories) > 0 {
		b.WriteString("\n\n## Memories\n\n")
		for _, m := range um.Memories {
			b.WriteString("- " + m.Summary + "\n")
		}
		if unresolved := unresolvedItemsFromMemories(um.Memories); len(unresolved) > 0 {
			b.WriteString("\n## Carryover from previous chats\n\n")
			for _, item := range unresolved {
				b.WriteString("- " + item + "\n")
			}
		}
	}

	if len(um.Configuration) > 0 {
		b.WriteString("\n\n## Configuration\n\n")
		for _, c := range um.Configuration {
			b.WriteString("- " + c.Key + ": " + c.Value + "\n")
		}
	}

	if len(um.Priorities) > 0 {
		b.WriteString("\n\n## Priorities\n\n")
		for _, p := range um.Priorities {
			b.WriteString("- " + p.Name + ": " + p.Description + "\n")
		}
	}

	if chatID != "" {
		chatFacts, err := store.ListFacts(ctx, wm.db, "chat", chatID, false, factsLimit, false)
		if err != nil {
			return "", err
		}
		if len(chatFacts) > 0 {
			b.WriteString("\n\n## Active chat facts\n")
			b.WriteString(store.FormatFactsForPrompt(chatFacts))
		}

		chatMemories, err := store.ListMemories(ctx, wm.db, "chat", chatID, memoriesLimit)
		if err != nil {
			return "", err
		}
		if len(chatMemories) > 0 {
			b.WriteString("\n\n## Active chat memories\n\n")
			for _, m := range chatMemories {
				b.WriteString("- " + m.Summary + "\n")
			}
		}
	}

	if query := recentKnowledgeQuery(ctx, wm.db, chatID); query != "" {
		if hits, err := wm.RecallKnowledge(ctx, query, ownerID, chatID, 4); err == nil && len(hits) > 0 {
			b.WriteString("\n\n## Relevant knowledge\n\n")
			for _, hit := range hits {
				b.WriteString("- " + hit.Summary + "\n")
			}
		}
	}

	return b.String(), nil
}

func unresolvedItemsFromMemories(memories []schema.Memory) []string {
	seen := make(map[string]struct{}, len(memories))
	var out []string
	for _, memory := range memories {
		if memory.Source != "chat_summarizer" || strings.TrimSpace(memory.Details) == "" {
			continue
		}
		var payload struct {
			UnresolvedItems []string `json:"unresolved_items"`
		}
		if err := json.Unmarshal([]byte(memory.Details), &payload); err != nil {
			continue
		}
		for _, item := range payload.UnresolvedItems {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

// RecordArtifactForCommand records an artifact and strengthens relationships around it.
func (wm *WorldModel) RecordArtifactForCommand(ctx context.Context, ownerID string, a schema.Artifact, prov schema.EntityProvenance) error {
	if ownerID == "" {
		return fmt.Errorf("worldmodel: ownerID required for artifact")
	}
	if a.OwnerID == "" {
		a.OwnerID = ownerID
	}
	if prov.Timestamp.IsZero() {
		prov.Timestamp = time.Now().UTC()
	}
	if prov.Confidence <= 0 {
		prov.Confidence = 1.0
	}
	saved, err := wm.UpsertArtifact(ctx, a)
	if err != nil {
		return err
	}
	if err := store.SaveEntityProvenance(ctx, wm.db, "artifact", saved.ID, prov); err != nil {
		return err
	}
	ownerRef := "owner:" + ownerID
	artifactRef := "artifact:" + saved.ID
	if err := wm.TouchRelationship(ctx, ownerRef, artifactRef, "owns", prov.Source, 0.5); err != nil {
		return err
	}
	return nil
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
