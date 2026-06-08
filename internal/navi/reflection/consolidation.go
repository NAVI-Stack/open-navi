package reflection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/navi/proposals"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

type consolidationCandidate struct {
	ChatID  string
	OwnerID string
	Summary string
	Details string
}

func (w *Worker) runPeriodicConsolidation(ctx context.Context) {
	if w.worldModel == nil {
		return
	}
	db := w.worldModel.DB()
	if db == nil {
		return
	}
	ownerID, _ := store.GetOwnerID(ctx, db)

	candidates, err := collectConsolidationCandidates(ctx, db, 20)
	if err != nil {
		slog.Debug("reflection: collect consolidation candidates failed", "error", err)
		return
	}
	for _, candidate := range candidates {
		if candidate.OwnerID == "" {
			candidate.OwnerID = ownerID
		}
		if err := w.persistConsolidatedMemory(ctx, candidate); err != nil {
			slog.Debug("reflection: persist consolidated memory failed", "chat_id", candidate.ChatID, "error", err)
		}
		if err := w.proposeOwnerChangesFromChat(ctx, candidate); err != nil {
			slog.Debug("reflection: synthesize proposal from chat failed", "chat_id", candidate.ChatID, "error", err)
		}
	}

	payload := schema.ReflectionPayload{
		ID:               fmt.Sprintf("consolidation-tick:%d", time.Now().UTC().UnixNano()),
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          "Periodic reflection consolidation",
		EscalationReason: "periodic",
		CreatedAt:        time.Now().UTC(),
	}
	w.processConsolidation(ctx, payload)
}

func collectConsolidationCandidates(ctx context.Context, db *sql.DB, limit int) ([]consolidationCandidate, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `
		SELECT c.chat_id
		FROM navi_chats c
		WHERE c.archived_at IS NULL AND c.deleted_at IS NULL
		ORDER BY COALESCE(c.last_message_at, c.updated_at) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chatIDs := make([]string, 0, limit)
	for rows.Next() {
		var chatID string
		if err := rows.Scan(&chatID); err != nil {
			return nil, err
		}
		chatIDs = append(chatIDs, chatID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []consolidationCandidate
	for _, chatID := range chatIDs {
		summary, details, ok, err := buildChatConsolidationSummary(ctx, db, chatID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, consolidationCandidate{
			ChatID:  chatID,
			Summary: summary,
			Details: details,
		})
	}
	return out, nil
}

func buildChatConsolidationSummary(ctx context.Context, db *sql.DB, chatID string) (summary, details string, ok bool, err error) {
	chatMemories, err := store.ListMemories(ctx, db, "chat", chatID, 8)
	if err != nil {
		return "", "", false, err
	}
	chatFacts, err := store.ListFacts(ctx, db, "chat", chatID, false, 8, false)
	if err != nil {
		return "", "", false, err
	}
	if len(chatMemories) == 0 && len(chatFacts) == 0 {
		return "", "", false, nil
	}

	summaryParts := make([]string, 0, 3)
	if len(chatMemories) > 0 {
		summaryParts = append(summaryParts, chatMemories[0].Summary)
	}
	if len(chatFacts) > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%s: %s", chatFacts[0].Key, chatFacts[0].Value))
	}
	summary = "Consolidated chat memory"
	if len(summaryParts) > 0 {
		summary = "Consolidated chat memory: " + strings.Join(summaryParts, " | ")
	}

	payload := map[string]any{
		"chat_id":  chatID,
		"memories": summarizeMemoryItems(chatMemories),
		"facts":    summarizeFactItems(chatFacts),
	}
	raw, _ := json.Marshal(payload)
	return summary, string(raw), true, nil
}

func summarizeMemoryItems(memories []schema.Memory) []map[string]any {
	out := make([]map[string]any, 0, len(memories))
	for _, memory := range memories {
		out = append(out, map[string]any{
			"id":           memory.ID,
			"summary":      memory.Summary,
			"keywords":     memory.Keywords,
			"tags":         memory.Tags,
			"significance": memory.Significance,
		})
	}
	return out
}

func summarizeFactItems(facts []store.Fact) []map[string]any {
	out := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		out = append(out, map[string]any{
			"id":       fact.ID,
			"category": fact.Category,
			"key":      fact.Key,
			"value":    fact.Value,
			"keywords": fact.Keywords,
			"tags":     fact.Tags,
		})
	}
	return out
}

func (w *Worker) persistConsolidatedMemory(ctx context.Context, candidate consolidationCandidate) error {
	if strings.TrimSpace(candidate.OwnerID) == "" || strings.TrimSpace(candidate.ChatID) == "" {
		return nil
	}
	mem := schema.Memory{
		ID:           "consolidation:" + candidate.ChatID,
		Scope:        "owner",
		ScopeID:      candidate.OwnerID,
		Summary:      candidate.Summary,
		Details:      candidate.Details,
		Significance: "high",
		Source:       "consolidation",
		Tags:         []string{"consolidation", "chat_rollup"},
	}
	w.runCreateIfRecord(ctx, candidate.ChatID, func(ctx context.Context) (any, error) {
		return w.worldModel.CreateOrUpdateMemory(ctx, mem)
	}, "reflection: consolidated memory upsert failed", candidate.ChatID)
	return nil
}

func (w *Worker) proposeOwnerChangesFromChat(ctx context.Context, candidate consolidationCandidate) error {
	if w.saveProposal == nil || strings.TrimSpace(candidate.OwnerID) == "" {
		return nil
	}
	db := w.worldModel.DB()
	if db == nil {
		return nil
	}
	messages, err := loadChatMessages(ctx, db, candidate.ChatID, 12)
	if err != nil {
		return err
	}
	text := strings.ToLower(strings.Join(messages, "\n"))

	if strings.Contains(text, "i prefer ") || strings.Contains(text, "my preference") {
		details := schema.ReflectionDetails{
			Kind:     "fact",
			Scope:    "owner",
			ScopeID:  candidate.OwnerID,
			Category: "configuration",
			Key:      "preference_review",
			Summary:  candidate.Summary,
		}
		return w.saveReflectionProposal(ctx, "consolidation", "periodic", "Review inferred owner preference change", details)
	}
	if strings.Contains(text, "priority") || strings.Contains(text, "focus on") || strings.Contains(text, "most important") {
		details := schema.ReflectionDetails{
			Kind:     "fact",
			Scope:    "owner",
			ScopeID:  candidate.OwnerID,
			Category: "priority",
			Key:      "focus_review",
			Summary:  candidate.Summary,
		}
		return w.saveReflectionProposal(ctx, "consolidation", "periodic", "Review inferred owner priority change", details)
	}
	return nil
}

func loadChatMessages(ctx context.Context, db *sql.DB, chatID string, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT content
		FROM navi_chat_messages
		WHERE chat_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		out = append(out, content)
	}
	return out, rows.Err()
}

func (w *Worker) saveReflectionProposal(ctx context.Context, sourceProcess, trigger, action string, details schema.ReflectionDetails) error {
	payload, _ := json.Marshal(details)
	refPayload := schema.ReflectionPayload{
		ID:               "periodic-proposal:" + details.Key,
		RuntimeSessionID: details.ScopeID,
		Tier:             schema.ReflectionTierConsolidation,
		Summary:          action,
		Details:          string(payload),
		EscalationReason: trigger,
		CreatedAt:        time.Now().UTC(),
	}
	proposal, err := proposals.BuildReflection(
		sourceProcess,
		trigger,
		action,
		buildAffectedEntitiesForReflection(refPayload, reflectionDetails{
			Kind:     details.Kind,
			Scope:    details.Scope,
			ScopeID:  details.ScopeID,
			Category: details.Category,
			Key:      details.Key,
		}),
		string(payload),
	)
	if err != nil {
		return err
	}
	return w.saveProposal(ctx, proposal)
}

func resolvePreferenceSignalOwnerID(ctx context.Context, db *sql.DB, details reflectionDetails) string {
	for _, signal := range details.preferenceSignals() {
		if signal.Scope == experience.ConfigScopeOwner && strings.TrimSpace(signal.ScopeID) != "" {
			return strings.TrimSpace(signal.ScopeID)
		}
	}
	if details.Scope == experience.ConfigScopeOwner && strings.TrimSpace(details.ScopeID) != "" {
		return strings.TrimSpace(details.ScopeID)
	}
	ownerID, _ := store.GetOwnerID(ctx, db)
	return strings.TrimSpace(ownerID)
}

func (w *Worker) consolidatePreferenceSignals(ctx context.Context, ownerID, trigger, preferredChatID string) error {
	if w.worldModel == nil || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	db := w.worldModel.DB()
	if db == nil {
		return nil
	}
	capturedSignals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, ownerID, []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusCaptured}, 200)
	if err != nil {
		return err
	}
	appliedSignals, err := store.ListPreferenceSignalsByScope(ctx, db, experience.ConfigScopeOwner, ownerID, []schema.PreferenceSignalStatus{schema.PreferenceSignalStatusApplied}, 200)
	if err != nil {
		return err
	}
	snapshot, err := experience.LoadConfigurationSnapshot(ctx, db, ownerID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	profile, deltas := experience.UpdateRelationshipProfile(snapshot.RelationshipProfile, capturedSignals, now)
	if !reflect.DeepEqual(profile, snapshot.RelationshipProfile) {
		if err := experience.SaveRelationshipProfileConfiguration(ctx, db, ownerID, profile, string(schema.StateKindInferred)); err != nil {
			return err
		}
	}
	snapshot.RelationshipProfile = profile

	capturedByTrait := preferenceSignalIDsByTrait(capturedSignals)
	appliedByTrait := preferenceSignalIDsByTrait(appliedSignals)
	toApplied := make([]string, 0)
	toPersisted := make([]string, 0)
	proposalDeltas := make([]experience.AdaptationDelta, 0, len(deltas))
	for _, delta := range deltas {
		if delta.Persisted {
			toPersisted = append(toPersisted, capturedByTrait[delta.Trait]...)
			toPersisted = append(toPersisted, appliedByTrait[delta.Trait]...)
		} else if delta.Applied {
			toApplied = append(toApplied, capturedByTrait[delta.Trait]...)
		}
		if delta.ProposalWorthy {
			proposalDeltas = append(proposalDeltas, delta)
		}
	}
	if err := store.UpdatePreferenceSignalStatus(ctx, db, dedupeSignalIDs(toApplied), schema.PreferenceSignalStatusApplied); err != nil {
		return err
	}
	if err := store.UpdatePreferenceSignalStatus(ctx, db, dedupeSignalIDs(toPersisted), schema.PreferenceSignalStatusPersisted); err != nil {
		return err
	}
	if len(proposalDeltas) > 0 {
		if err := w.recordProposalWorthyExperienceSnapshot(ctx, db, ownerID, preferredChatID, snapshot, capturedSignals, appliedSignals, proposalDeltas); err != nil {
			slog.Debug("reflection: proposal-worthy experience snapshot skipped", "owner_id", ownerID, "chat_id", preferredChatID, "error", err)
		}
	}
	if len(proposalDeltas) == 0 || w.saveProposal == nil {
		return nil
	}
	if strings.TrimSpace(trigger) == "" {
		trigger = "automatic"
	}
	summaries := make([]string, 0, len(proposalDeltas))
	for _, delta := range proposalDeltas {
		summaries = append(summaries, fmt.Sprintf("%s %.2f→%.2f (%d signals)", delta.Trait, delta.PreviousValue, delta.NewValue, delta.SignalCount))
	}
	action := "Review significant experience adaptation"
	detailsPayload, _ := json.Marshal(schema.ReflectionDetails{
		Kind:     "fact",
		Scope:    experience.ConfigScopeOwner,
		ScopeID:  ownerID,
		Category: "configuration",
		Key:      "experience_adaptation",
		Summary:  "Consolidation detected meaningful preference adaptation signals: " + strings.Join(summaries, "; "),
	})
	w.processDeep(ctx, schema.ReflectionPayload{
		ID:               fmt.Sprintf("deep-experience-adaptation:%s:%d", ownerID, now.UnixNano()),
		Tier:             schema.ReflectionTierDeep,
		Summary:          action,
		Details:          string(detailsPayload),
		EscalationReason: trigger,
		CreatedAt:        now,
	})
	return nil
}

func (w *Worker) recordProposalWorthyExperienceSnapshot(
	ctx context.Context,
	db *sql.DB,
	ownerID, preferredChatID string,
	snapshot experience.ConfigurationSnapshot,
	capturedSignals, appliedSignals []schema.PreferenceSignal,
	proposalDeltas []experience.AdaptationDelta,
) error {
	chatID := proposalSnapshotChatID(preferredChatID, capturedSignals, appliedSignals, proposalDeltas)
	if chatID == "" {
		return nil
	}

	mode, lastUser, err := loadProposalSnapshotChatContext(ctx, db, chatID)
	if err != nil {
		return err
	}

	sessionOverrides := experience.ExplicitTurnInput{}
	sessionTrace := experience.ExplicitTurnTrace{}
	sessionPrefs := experience.OutputPreferences{}
	immediateSignals, err := store.ListImmediatePreferenceSignalsBySession(ctx, db, chatID, 100)
	if err != nil {
		return err
	}
	if len(immediateSignals) > 0 {
		sessionOverrides, sessionTrace, sessionPrefs = experience.BuildSessionPreferenceOverrides(immediateSignals)
	}

	req := experience.BuildRequest{
		ChatID:                   chatID,
		Mode:                     mode,
		LastUserMessage:          lastUser,
		CoreIdentity:             snapshot.CoreIdentity,
		PersonaModules:           snapshot.PersonaModules,
		RelationshipProfile:      snapshot.RelationshipProfile,
		OutputPreferences:        snapshot.OutputPreferences,
		ExplicitSessionOverrides: sessionOverrides,
		SessionOverrideTrace:     sessionTrace,
		SessionOutputPreferences: sessionPrefs,
	}
	rendered, err := w.buildExperienceSnapshot(ctx, mode, req)
	if err != nil {
		return err
	}
	_, err = experience.MaybeRecordExperienceSnapshot(ctx, db, rendered, experience.SnapshotRecordOptions{
		ChatID:         chatID,
		OwnerID:        ownerID,
		ExperienceMode: mode,
		ProposalWorthy: true,
	})
	return err
}

func (w *Worker) buildExperienceSnapshot(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error) {
	w.mu.Lock()
	builder := w.buildExperienceControl
	w.mu.Unlock()
	if builder != nil {
		return builder(ctx, mode, req)
	}
	engine := experience.NewEngine(nil)
	return engine.Build(ctx, experience.DefaultStandardProfile(), req)
}

func proposalSnapshotChatID(preferredChatID string, capturedSignals, appliedSignals []schema.PreferenceSignal, proposalDeltas []experience.AdaptationDelta) string {
	if preferred := strings.TrimSpace(preferredChatID); preferred != "" {
		return preferred
	}
	proposalTraits := make(map[string]struct{}, len(proposalDeltas))
	for _, delta := range proposalDeltas {
		trait := strings.TrimSpace(delta.Trait)
		if trait == "" || !delta.ProposalWorthy {
			continue
		}
		proposalTraits[trait] = struct{}{}
	}
	if len(proposalTraits) == 0 {
		return ""
	}

	chosenChatID := ""
	var chosenAt time.Time
	consider := func(signal schema.PreferenceSignal) {
		if _, ok := proposalTraits[strings.TrimSpace(signal.Trait)]; !ok {
			return
		}
		chatID := strings.TrimSpace(signal.ChatID)
		if chatID == "" {
			return
		}
		signalAt := signal.UpdatedAt
		if signalAt.IsZero() {
			signalAt = signal.CreatedAt
		}
		if chosenChatID == "" || (!signalAt.IsZero() && (chosenAt.IsZero() || signalAt.After(chosenAt))) {
			chosenChatID = chatID
			chosenAt = signalAt
		}
	}

	for _, signal := range capturedSignals {
		consider(signal)
	}
	for _, signal := range appliedSignals {
		consider(signal)
	}
	return chosenChatID
}

func loadProposalSnapshotChatContext(ctx context.Context, db *sql.DB, chatID string) (mode, lastUser string, err error) {
	mode = "navi"
	chatID = strings.TrimSpace(chatID)
	if db == nil || chatID == "" {
		return mode, "", nil
	}

	// Experience mode is no longer stored as a column on navi_chats; the
	// proposal snapshot defaults to "navi" unless overridden elsewhere.
	mode = normalizeProposalSnapshotMode("")

	switch scanErr := db.QueryRowContext(ctx, `
		SELECT content
		FROM navi_chat_messages
		WHERE chat_id = ? AND LOWER(TRIM(role)) = 'user'
		ORDER BY created_at DESC
		LIMIT 1
	`, chatID).Scan(&lastUser); {
	case scanErr == nil:
	case scanErr == sql.ErrNoRows || isMissingTableQueryErr(scanErr):
	default:
		return "", "", scanErr
	}
	return mode, lastUser, nil
}

func normalizeProposalSnapshotMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "wizard":
		return "wizard"
	default:
		return "navi"
	}
}

func isMissingTableQueryErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func preferenceSignalIDsByTrait(signals []schema.PreferenceSignal) map[string][]string {
	idsByTrait := make(map[string][]string, len(signals))
	for _, signal := range signals {
		trait := strings.TrimSpace(signal.Trait)
		id := strings.TrimSpace(signal.SignalID)
		if trait == "" || id == "" {
			continue
		}
		idsByTrait[trait] = append(idsByTrait[trait], id)
	}
	return idsByTrait
}

func dedupeSignalIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
