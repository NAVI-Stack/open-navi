package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/onboarding"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

const (
	ceremonySource              = "navi_ceremony"
	ceremonyPresenceKey         = "ceremony.presence_preference.v1"
	ceremonyTrustBoundariesKey  = "ceremony.trust_boundaries.v1"
	ceremonyPersonalizationKey  = "ceremony.personalization_seed.v1"
	ceremonyDefaultPresenceMode = "balanced"
	ceremonyPersonalSeedSource  = "owner_input"
	ceremonyStreamChunkRunes    = 18
	ceremonyStreamChunkDelay    = 28 * time.Millisecond
)

type ceremonyResponse struct {
	JourneyState          onboarding.CeremonyJourneyState `json:"journeyState"`
	OwnerProfileSeed      ceremonyOwnerProfileSeed        `json:"ownerProfileSeed"`
	PresencePreference    ceremonyPresencePreference      `json:"presencePreference"`
	TrustBoundaryDefaults ceremonyTrustBoundaryDefaults   `json:"trustBoundaryDefaults"`
	PersonalizationSeed   ceremonyPersonalizationSeed     `json:"personalizationSeed"`
	PactSummary           []string                        `json:"pactSummary"`
	ChatID                string                          `json:"chatId,omitempty"`
	Redirect              string                          `json:"redirect,omitempty"`
}

// Pre-scripted ceremony messages for Phase A.
// Ceremony-V2 note: replace these with LLM-generated messages guided by ceremony system context,
// allowing NAVI to ask naturally and follow up conversationally.
const (
	ceremonyOpeningMessage  = "Good to meet you.\n\nBefore we begin, I’d like to learn a few simple few things about you so I may assist you properly and with due care and discretion. Nothing here is permanent — you may change it at any time.\n\nFirst, what should I call you?"
	ceremonyPresenceMessage = "How would you prefer I conduct myself while assisting you?\n\nFor example: quiet and concise, warm and conversational, direct and practical, careful and deliberate, or in some other manner?."
	ceremonyTrustMessage    = "Are there any matters where I should always ask for your permission before proceeding?\n\nor example: sending messages, changing files, making purchases, deleting anything, accessing sensitive information, or making decisions with real or lasting consequences."
	ceremonyRememberMessage = "Is there anything you would like me to remember starting now?\n\nIt may be a preference, a goal, a boundary, or simply something that helps me serve you better."
)

var (
	ceremonyPresenceControls = []map[string]any{
		{"key": "calm_quiet", "label": "Calm & quiet", "description": "Low-key, minimal interruptions"},
		{"key": "warm_conversational", "label": "Warm & conversational", "description": "Friendly, personable tone"},
		{"key": "direct_strategic", "label": "Direct & strategic", "description": "Efficient, focused on outcomes"},
		{"key": "fast_focused", "label": "Fast & focused", "description": "Brief, prioritizes speed"},
		{"key": "balanced", "label": "Balanced", "description": "Adapts to context"},
	}
	ceremonyTrustControls = []map[string]any{
		{"key": "confirm_before_sending_messages", "label": "Sending messages"},
		{"key": "confirm_before_changing_files", "label": "Changing files"},
		{"key": "confirm_before_purchases", "label": "Making purchases or subscriptions"},
		{"key": "confirm_before_remembering_sensitive_details", "label": "Remembering sensitive personal details"},
		{"key": "confirm_before_acting_on_inferred_preferences", "label": "Acting on inferred preferences"},
		{"key": "confirm_before_interrupting_proactively", "label": "Interrupting proactively"},
		{"key": "confirm_before_external_changes", "label": "Making external changes"},
	}
	ceremonyTrustDefaults = map[string]bool{
		"confirm_before_sending_messages":               false,
		"confirm_before_changing_files":                 true,
		"confirm_before_purchases":                      true,
		"confirm_before_remembering_sensitive_details":  true,
		"confirm_before_acting_on_inferred_preferences": false,
		"confirm_before_interrupting_proactively":       false,
		"confirm_before_external_changes":               true,
	}
	ceremonyPactActions = []map[string]any{
		{"key": "confirm", "label": "Looks right", "variant": "primary"},
		{"key": "adjust", "label": "Adjust", "variant": "secondary"},
		{"key": "skip", "label": "Skip for now", "variant": "ghost"},
	}
)

type ceremonyOwnerProfileSeed struct {
	DisplayName string `json:"displayName"`
	CreatedFrom string `json:"createdFrom,omitempty"`
	OwnerSet    bool   `json:"ownerSet,omitempty"`
}

type ceremonyPresencePreference struct {
	Mode        string `json:"mode"`
	CreatedFrom string `json:"createdFrom,omitempty"`
	OwnerSet    bool   `json:"ownerSet,omitempty"`
}

type ceremonyTrustBoundaryDefaults struct {
	ConfirmBeforeSendingMessages             bool   `json:"confirm_before_sending_messages"`
	ConfirmBeforeChangingFiles               bool   `json:"confirm_before_changing_files"`
	ConfirmBeforePurchases                   bool   `json:"confirm_before_purchases"`
	ConfirmBeforeRememberingSensitiveDetails bool   `json:"confirm_before_remembering_sensitive_details"`
	ConfirmBeforeActingOnInferredPreferences bool   `json:"confirm_before_acting_on_inferred_preferences"`
	ConfirmBeforeInterruptingProactively     bool   `json:"confirm_before_interrupting_proactively"`
	ConfirmBeforeExternalChanges             bool   `json:"confirm_before_external_changes"`
	CreatedFrom                              string `json:"created_from,omitempty"`
	OwnerSet                                 bool   `json:"owner_set,omitempty"`
}

type ceremonyPersonalizationSeed struct {
	Text        string `json:"text"`
	Source      string `json:"source,omitempty"`
	CreatedFrom string `json:"createdFrom,omitempty"`
	OwnerSet    bool   `json:"ownerSet,omitempty"`
	RoutedTo    string `json:"routedTo,omitempty"`
	Reviewable  bool   `json:"reviewable"`
}

type ceremonyPersonalizationSeedConfig struct {
	Text        string `json:"text"`
	Source      string `json:"source,omitempty"`
	CreatedFrom string `json:"created_from,omitempty"`
	OwnerSet    bool   `json:"owner_set,omitempty"`
	RoutedTo    string `json:"routed_to,omitempty"`
	Reviewable  bool   `json:"reviewable"`
}

type ceremonyCompleteRequest struct {
	DisplayName         string                        `json:"display_name"`
	PresenceMode        string                        `json:"presence_mode"`
	TrustBoundaries     ceremonyTrustBoundaryDefaults `json:"trust_boundaries"`
	PersonalizationSeed string                        `json:"personalization_seed"`
}

type ceremonyStartRequest struct {
	CurrentStep string `json:"current_step"`
}

func (s *Server) handleGetCeremony(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerReadAccess(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	resp, err := s.buildCeremonyResponse(r.Context(), ownerID, "")
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStartCeremony(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	var req ceremonyStartRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if _, err := onboarding.StartCeremonyJourney(r.Context(), s.cfg.DB, req.CurrentStep); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	resp, err := s.buildCeremonyResponse(r.Context(), ownerID, "")
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSkipCeremony(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	if _, err := onboarding.SkipCeremonyJourney(r.Context(), s.cfg.DB); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	resp, err := s.buildCeremonyResponse(r.Context(), ownerID, "/")
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCompleteCeremony(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	status, err := onboarding.DeriveFirstRunStatus(r.Context(), s.cfg.DB, s.dataDir())
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "status check failed", nil)
		return
	}
	if !status.Complete {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "complete required onboarding first", nil)
		return
	}

	var req ceremonyCompleteRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "display_name required", nil)
		return
	}
	presenceMode := normalizeCeremonyPresenceMode(req.PresenceMode)
	if presenceMode == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported presence_mode", nil)
		return
	}

	owner, exists, err := store.GetOwner(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "owner lookup failed", nil)
		return
	}
	if !exists {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "owner unavailable", nil)
		return
	}
	if err := s.saveCeremonyOwnerProfileSeed(r.Context(), owner, displayName); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if err := s.saveCeremonyPresencePreference(r.Context(), ownerID, presenceMode); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	boundaries := normalizeCeremonyTrustBoundaries(req.TrustBoundaries)
	if err := saveCeremonyConfiguration(r.Context(), s.cfg.DB, ownerID, ceremonyTrustBoundariesKey, boundaries); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	seed := s.routeCeremonyPersonalizationSeed(r.Context(), ownerID, req.PersonalizationSeed)
	if err := saveCeremonyConfiguration(r.Context(), s.cfg.DB, ownerID, ceremonyPersonalizationKey, personalizationConfigFromResponse(seed)); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if err := s.savePreferenceSignalsForCeremonySeed(r.Context(), ownerID, seed); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if _, err := onboarding.CompleteCeremonyJourney(r.Context(), s.cfg.DB); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}

	resp, err := s.buildCeremonyResponse(r.Context(), ownerID, "/")
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) buildCeremonyResponse(ctx context.Context, ownerID, redirect string) (ceremonyResponse, error) {
	state, err := onboarding.LoadCeremonyJourneyState(ctx, s.cfg.DB)
	if err != nil {
		return ceremonyResponse{}, err
	}
	resolvedRedirect := redirect
	if resolvedRedirect == "" && state.ChatID != "" {
		resolvedRedirect = "/chats/" + state.ChatID
	}
	ownerSeed := s.loadCeremonyOwnerProfileSeed(ctx, ownerID)
	presence := loadCeremonyPresencePreference(ctx, s.cfg.DB, ownerID)
	boundaries := loadCeremonyTrustBoundaries(ctx, s.cfg.DB, ownerID)
	seed := loadCeremonyPersonalizationSeed(ctx, s.cfg.DB, ownerID)
	return ceremonyResponse{
		JourneyState:          state,
		OwnerProfileSeed:      ownerSeed,
		PresencePreference:    presence,
		TrustBoundaryDefaults: boundaries,
		PersonalizationSeed:   seed,
		PactSummary:           buildCeremonyPactSummary(ownerSeed, presence, boundaries, seed),
		ChatID:                state.ChatID,
		Redirect:              resolvedRedirect,
	}, nil
}

func (s *Server) postOnboardingRedirect(ctx context.Context) string {
	state, err := onboarding.LoadCeremonyJourneyState(ctx, s.cfg.DB)
	if err != nil || !onboarding.CeremonyRequiresRouting(state) {
		return "/"
	}
	if state.ChatID != "" {
		return "/chats/" + state.ChatID
	}
	// No chat yet — frontend will call init-chat to create one.
	return "/"
}

func (s *Server) saveCeremonyOwnerProfileSeed(ctx context.Context, owner store.Owner, displayName string) error {
	meta := map[string]any{
		"created_from": ceremonySource,
		"owner_set":    true,
		"source":       ceremonyPersonalSeedSource,
		"display_name": displayName,
	}
	raw, _ := json.Marshal(meta)
	contact := schema.Contact{
		ID:         owner.ID,
		Name:       displayName,
		Kind:       schema.ContactKindPerson,
		OwnerType:  schema.ContactOwnerTypeOwner,
		TrustLevel: schema.ContactTrustLevelVerified,
		Metadata:   string(raw),
		CreatedAt:  owner.CreatedAt,
	}
	if s.cfg.WorldModel != nil {
		_, err := s.cfg.WorldModel.UpsertContact(ctx, contact)
		return err
	}
	return store.SaveContact(ctx, s.cfg.DB, contact)
}

func (s *Server) loadCeremonyOwnerProfileSeed(ctx context.Context, ownerID string) ceremonyOwnerProfileSeed {
	contact, err := store.GetContact(ctx, s.cfg.DB, ownerID)
	if err != nil {
		return ceremonyOwnerProfileSeed{}
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(contact.Metadata), &meta); err != nil {
		return ceremonyOwnerProfileSeed{}
	}
	if meta["created_from"] != ceremonySource {
		return ceremonyOwnerProfileSeed{}
	}
	return ceremonyOwnerProfileSeed{
		DisplayName: contact.Name,
		CreatedFrom: ceremonySource,
		OwnerSet:    true,
	}
}

func (s *Server) saveCeremonyPresencePreference(ctx context.Context, ownerID, mode string) error {
	preference := ceremonyPresencePreference{
		Mode:        mode,
		CreatedFrom: ceremonySource,
		OwnerSet:    true,
	}
	if err := saveCeremonyConfiguration(ctx, s.cfg.DB, ownerID, ceremonyPresenceKey, preferenceStorage(preference)); err != nil {
		return err
	}
	traits, prefs := ceremonyPresenceMapping(mode)
	currentTraits := loadOwnerCoreIdentityTraits(ctx, s.cfg.DB, ownerID)
	for trait, value := range traits {
		currentTraits[trait] = value
	}
	if err := experience.SaveCoreIdentityConfiguration(ctx, s.cfg.DB, ownerID, currentTraits, string(schema.StateKindOwnerSet)); err != nil {
		return err
	}
	if prefs.PreferredLength != "" || prefs.PreferredFormat != "" || prefs.SummaryFirst != nil {
		currentPrefs := loadOwnerOutputPreferences(ctx, s.cfg.DB, ownerID)
		currentPrefs = mergeCeremonyOutputPreferences(currentPrefs, prefs)
		if err := experience.SaveOutputPreferencesConfiguration(ctx, s.cfg.DB, ownerID, currentPrefs, string(schema.StateKindOwnerSet)); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCeremonyPresenceMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = ceremonyDefaultPresenceMode
	}
	switch mode {
	case "calm_quiet", "warm_conversational", "direct_strategic", "fast_focused", "balanced":
		return mode
	default:
		return ""
	}
}

func ceremonyPresenceMapping(mode string) (map[string]float64, experience.OutputPreferences) {
	summaryFirst := true
	switch mode {
	case "calm_quiet":
		return map[string]float64{
			"verbosity":        0.32,
			"initiative_style": 0.28,
			"warmth":           0.58,
			"supportiveness":   0.62,
		}, experience.OutputPreferences{PreferredLength: experience.LengthShort}
	case "warm_conversational":
		return map[string]float64{
			"warmth":            0.78,
			"conversationality": 0.72,
			"supportiveness":    0.74,
			"verbosity":         0.58,
		}, experience.OutputPreferences{PreferredLength: experience.LengthMedium, PreferredFormat: experience.FormatFreeform}
	case "direct_strategic":
		return map[string]float64{
			"directness":                   0.80,
			"challenge_intensity":          0.68,
			"structure_level":              0.72,
			"recommendation_directiveness": 0.72,
			"verbosity":                    0.44,
		}, experience.OutputPreferences{PreferredLength: experience.LengthMedium, PreferredFormat: experience.FormatExecutive}
	case "fast_focused":
		return map[string]float64{
			"directness":       0.74,
			"verbosity":        0.26,
			"initiative_style": 0.62,
			"structure_level":  0.64,
		}, experience.OutputPreferences{PreferredLength: experience.LengthShort, SummaryFirst: &summaryFirst}
	default:
		return map[string]float64{}, experience.OutputPreferences{}
	}
}

func normalizeCeremonyTrustBoundaries(boundaries ceremonyTrustBoundaryDefaults) ceremonyTrustBoundaryDefaults {
	boundaries.CreatedFrom = ceremonySource
	boundaries.OwnerSet = true
	return boundaries
}

func (s *Server) routeCeremonyPersonalizationSeed(ctx context.Context, ownerID, text string) ceremonyPersonalizationSeed {
	_ = ctx
	_ = ownerID
	seed := ceremonyPersonalizationSeed{
		Text:        strings.TrimSpace(text),
		Source:      ceremonyPersonalSeedSource,
		CreatedFrom: ceremonySource,
		OwnerSet:    true,
		Reviewable:  false,
	}
	if seed.Text == "" {
		return seed
	}
	seed.Reviewable = true
	lower := strings.ToLower(seed.Text)
	switch {
	case looksSensitiveOrAmbiguous(lower):
		seed.RoutedTo = "review_candidate"
	case looksLikeTrustBoundary(lower):
		seed.RoutedTo = "trust_boundary"
	case looksLikeCommunicationPreference(lower):
		seed.RoutedTo = "experience_preference"
	default:
		seed.RoutedTo = "review_candidate"
	}
	return seed
}

func (s *Server) savePreferenceSignalsForCeremonySeed(ctx context.Context, ownerID string, seed ceremonyPersonalizationSeed) error {
	if seed.RoutedTo != "experience_preference" || strings.TrimSpace(seed.Text) == "" {
		return nil
	}
	lower := strings.ToLower(seed.Text)
	signals := []schema.PreferenceSignal{}
	add := func(trait string, target float64, summary string) {
		signals = append(signals, schema.PreferenceSignal{
			Scope:          "owner",
			ScopeID:        ownerID,
			Trait:          trait,
			TargetValue:    target,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Immediate:      true,
			Summary:        "navi_ceremony: " + summary,
			Status:         schema.PreferenceSignalStatusCaptured,
		})
	}
	if strings.Contains(lower, "direct") {
		add("directness", 0.74, "owner requested direct communication")
	}
	if strings.Contains(lower, "short") || strings.Contains(lower, "brief") || strings.Contains(lower, "concise") {
		add("verbosity", 0.22, "owner requested concise replies")
	}
	if strings.Contains(lower, "warm") {
		add("warmth", 0.70, "owner requested warmer communication")
	}
	if strings.Contains(lower, "pushback") || strings.Contains(lower, "challenge") {
		add("challenge_intensity", 0.62, "owner requested thoughtful pushback")
	}
	for _, signal := range signals {
		if err := store.SavePreferenceSignal(ctx, s.cfg.DB, signal); err != nil {
			return err
		}
	}
	return nil
}

func buildCeremonyConfirmClosing(owner ceremonyOwnerProfileSeed) string {
	name := strings.TrimSpace(owner.DisplayName)
	if name == "" {
		name = "there"
	}
	return "Good, " + name + ". I'm ready.\n\nWhenever you like, just say what's on your mind — we can plan, explore, or start on something together."
}

func buildCeremonySkipClosing() string {
	return "No problem — we can always revisit these choices later.\n\nWhenever you're ready, just say what's on your mind."
}

func buildCeremonyPactSummary(owner ceremonyOwnerProfileSeed, presence ceremonyPresencePreference, boundaries ceremonyTrustBoundaryDefaults, seed ceremonyPersonalizationSeed) []string {
	name := strings.TrimSpace(owner.DisplayName)
	if name == "" {
		name = "you"
	}
	presenceLabel := ceremonyPresenceLabel(presence.Mode)
	if presenceLabel == "" {
		presenceLabel = "balanced"
	}
	askBefore := ceremonyBoundarySummary(boundaries)
	if askBefore == "" {
		askBefore = "the choices you mark as important"
	}
	remember := strings.TrimSpace(seed.Text)
	if remember == "" {
		remember = "nothing extra yet"
	}
	return []string{
		"Here's what I understand so far:",
		ceremonySentence("I'll call you ", name),
		ceremonySentence("I'll usually show up ", presenceLabel),
		ceremonySentence("I'll ask before ", askBefore),
		ceremonySentence("I'll remember ", remember),
		"I'll learn carefully, and you can correct what I know anytime.",
	}
}

func ceremonySentence(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(prefix) + "."
	}
	if strings.HasSuffix(value, ".") || strings.HasSuffix(value, "!") || strings.HasSuffix(value, "?") {
		return prefix + value
	}
	return prefix + value + "."
}

func ceremonyPresenceLabel(mode string) string {
	switch mode {
	case "calm_quiet":
		return "calm and quiet"
	case "warm_conversational":
		return "warm and conversational"
	case "direct_strategic":
		return "direct and strategic"
	case "fast_focused":
		return "fast and focused"
	case "balanced":
		return "balanced"
	default:
		return ""
	}
}

func ceremonyBoundarySummary(boundaries ceremonyTrustBoundaryDefaults) string {
	items := []string{}
	if boundaries.ConfirmBeforeSendingMessages {
		items = append(items, "sending messages")
	}
	if boundaries.ConfirmBeforeChangingFiles {
		items = append(items, "changing files")
	}
	if boundaries.ConfirmBeforePurchases {
		items = append(items, "making purchases or subscriptions")
	}
	if boundaries.ConfirmBeforeRememberingSensitiveDetails {
		items = append(items, "remembering sensitive personal details")
	}
	if boundaries.ConfirmBeforeActingOnInferredPreferences {
		items = append(items, "acting on inferred preferences")
	}
	if boundaries.ConfirmBeforeInterruptingProactively {
		items = append(items, "interrupting proactively")
	}
	if boundaries.ConfirmBeforeExternalChanges {
		items = append(items, "making external changes")
	}
	return strings.Join(items, ", ")
}

func looksLikeCommunicationPreference(text string) bool {
	markers := []string{"direct", "short", "brief", "concise", "explanation", "explanations", "answers", "answer", "warm", "tone", "pushback", "challenge", "focused"}
	return containsAny(text, markers)
}

func looksLikeTrustBoundary(text string) bool {
	markers := []string{"ask before", "confirm before", "permission", "do not", "don't", "never send", "before sending", "before changing"}
	return containsAny(text, markers)
}

func looksSensitiveOrAmbiguous(text string) bool {
	markers := []string{"burnout", "health", "medical", "therapy", "trauma", "wife", "husband", "partner", "child", "secret", "password", "private", "sensitive", "diagnosis", "depression", "anxiety"}
	return containsAny(text, markers)
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func saveCeremonyConfiguration(ctx context.Context, db *sql.DB, ownerID, key string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ceremony: marshal %s: %w", key, err)
	}
	now := time.Now().UTC()
	return store.SaveConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        fmt.Sprintf("%s:%s", key, strings.TrimSpace(ownerID)),
		Scope:     "owner",
		ScopeID:   strings.TrimSpace(ownerID),
		Key:       key,
		Value:     string(raw),
		Source:    string(schema.StateKindOwnerSet),
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func loadCeremonyPresencePreference(ctx context.Context, db *sql.DB, ownerID string) ceremonyPresencePreference {
	var stored struct {
		Mode        string `json:"mode"`
		CreatedFrom string `json:"created_from"`
		OwnerSet    bool   `json:"owner_set"`
	}
	if loadCeremonyConfiguration(ctx, db, ownerID, ceremonyPresenceKey, &stored) {
		return ceremonyPresencePreference{Mode: stored.Mode, CreatedFrom: stored.CreatedFrom, OwnerSet: stored.OwnerSet}
	}
	return ceremonyPresencePreference{Mode: ceremonyDefaultPresenceMode}
}

func loadCeremonyTrustBoundaries(ctx context.Context, db *sql.DB, ownerID string) ceremonyTrustBoundaryDefaults {
	var boundaries ceremonyTrustBoundaryDefaults
	if loadCeremonyConfiguration(ctx, db, ownerID, ceremonyTrustBoundariesKey, &boundaries) {
		return boundaries
	}
	return ceremonyTrustBoundaryDefaults{}
}

func loadCeremonyPersonalizationSeed(ctx context.Context, db *sql.DB, ownerID string) ceremonyPersonalizationSeed {
	var stored ceremonyPersonalizationSeedConfig
	if loadCeremonyConfiguration(ctx, db, ownerID, ceremonyPersonalizationKey, &stored) {
		return personalizationResponseFromConfig(stored)
	}
	return ceremonyPersonalizationSeed{Source: ceremonyPersonalSeedSource, CreatedFrom: ceremonySource, OwnerSet: true}
}

func loadCeremonyConfiguration(ctx context.Context, db *sql.DB, ownerID, key string, out any) bool {
	raw, ok, err := store.GetConfigurationValue(ctx, db, "owner", strings.TrimSpace(ownerID), key)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return false
	}
	return json.Unmarshal([]byte(raw), out) == nil
}

func preferenceStorage(preference ceremonyPresencePreference) any {
	return struct {
		Mode        string `json:"mode"`
		CreatedFrom string `json:"created_from"`
		OwnerSet    bool   `json:"owner_set"`
	}{
		Mode:        preference.Mode,
		CreatedFrom: preference.CreatedFrom,
		OwnerSet:    preference.OwnerSet,
	}
}

func personalizationConfigFromResponse(seed ceremonyPersonalizationSeed) ceremonyPersonalizationSeedConfig {
	return ceremonyPersonalizationSeedConfig{
		Text:        seed.Text,
		Source:      seed.Source,
		CreatedFrom: seed.CreatedFrom,
		OwnerSet:    seed.OwnerSet,
		RoutedTo:    seed.RoutedTo,
		Reviewable:  seed.Reviewable,
	}
}

func personalizationResponseFromConfig(seed ceremonyPersonalizationSeedConfig) ceremonyPersonalizationSeed {
	return ceremonyPersonalizationSeed{
		Text:        seed.Text,
		Source:      seed.Source,
		CreatedFrom: seed.CreatedFrom,
		OwnerSet:    seed.OwnerSet,
		RoutedTo:    seed.RoutedTo,
		Reviewable:  seed.Reviewable,
	}
}

func loadOwnerCoreIdentityTraits(ctx context.Context, db *sql.DB, ownerID string) map[string]float64 {
	traits := map[string]float64{}
	raw, ok, err := store.GetConfigurationValue(ctx, db, experience.ConfigScopeOwner, strings.TrimSpace(ownerID), experience.ConfigKeyCoreIdentity)
	if err != nil || !ok {
		return traits
	}
	var cfg experience.TraitSetConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return traits
	}
	for trait, value := range cfg.Traits {
		traits[trait] = value
	}
	return traits
}

func loadOwnerOutputPreferences(ctx context.Context, db *sql.DB, ownerID string) experience.OutputPreferences {
	raw, ok, err := store.GetConfigurationValue(ctx, db, experience.ConfigScopeOwner, strings.TrimSpace(ownerID), experience.ConfigKeyOutputPreferences)
	if err != nil || !ok {
		return experience.OutputPreferences{}
	}
	var prefs experience.OutputPreferences
	_ = json.Unmarshal([]byte(raw), &prefs)
	return prefs
}

func mergeCeremonyOutputPreferences(base, override experience.OutputPreferences) experience.OutputPreferences {
	if override.PreferredLength != "" {
		base.PreferredLength = override.PreferredLength
	}
	if override.PreferredFormat != "" {
		base.PreferredFormat = override.PreferredFormat
	}
	if override.SummaryFirst != nil {
		base.SummaryFirst = override.SummaryFirst
	}
	return base
}

// initChatResponse is returned by POST /api/ceremony/init-chat.
type initChatResponse struct {
	ChatID   string `json:"chatId"`
	Redirect string `json:"redirect"`
}

// handleInitCeremonyChat creates or returns the "Meet NAVI" ceremony chat thread.
// Idempotent: repeated calls return the same chatId once created.
func (s *Server) handleInitCeremonyChat(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	if s.cfg.Navi == nil || s.cfg.DB == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "UNAVAILABLE", "ceremony chat not available", nil)
		return
	}

	status, err := onboarding.DeriveFirstRunStatus(r.Context(), s.cfg.DB, s.dataDir())
	if err != nil || !status.Complete {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "complete required onboarding first", nil)
		return
	}

	state, err := onboarding.LoadCeremonyJourneyState(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}

	// If ceremony is done, return existing chatId or root.
	if !onboarding.CeremonyRequiresRouting(state) {
		if state.ChatID != "" {
			replyJSON(w, http.StatusOK, initChatResponse{ChatID: state.ChatID, Redirect: "/chats/" + state.ChatID})
		} else {
			replyJSON(w, http.StatusOK, initChatResponse{Redirect: "/"})
		}
		return
	}

	// Idempotent: chat already created.
	if state.ChatID != "" {
		replyJSON(w, http.StatusOK, initChatResponse{ChatID: state.ChatID, Redirect: "/chats/" + state.ChatID})
		return
	}

	chatID, err := s.cfg.Navi.CreateCeremonyChat(r.Context())
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}

	if _, err := onboarding.UpdateCeremonyJourneyChatID(r.Context(), s.cfg.DB, chatID); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if _, err := onboarding.StartCeremonyJourney(r.Context(), s.cfg.DB, "owner_recognition"); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}

	openingMeta := map[string]any{"ceremonyStep": "owner_recognition"}
	if err := s.injectCeremonyAssistantMessage(r.Context(), chatID, ceremonyOpeningMessage, openingMeta); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}

	replyJSON(w, http.StatusCreated, initChatResponse{ChatID: chatID, Redirect: "/chats/" + chatID})
}

// ceremonyStepRequest is the body for POST /api/ceremony/step.
type ceremonyStepRequest struct {
	ChatID string          `json:"chatId"`
	Step   string          `json:"step"`
	Value  json.RawMessage `json:"value,omitempty"`
	Action string          `json:"action,omitempty"`
}

// handleCeremonyStep processes a structured choice in the ceremony flow.
func (s *Server) handleCeremonyStep(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	if s.cfg.Navi == nil || s.cfg.DB == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "UNAVAILABLE", "ceremony not available", nil)
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}

	var req ceremonyStepRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.ChatID) == "" || strings.TrimSpace(req.Step) == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chatId and step required", nil)
		return
	}

	state, err := onboarding.LoadCeremonyJourneyState(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if state.ChatID != req.ChatID {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chatId does not match ceremony chat", nil)
		return
	}
	if state.Status != onboarding.CeremonyStatusInProgress {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "ceremony is not in progress", nil)
		return
	}

	switch req.Step {
	case "navi_presence":
		var mode string
		if err := json.Unmarshal(req.Value, &mode); err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "value must be a presence mode string", nil)
			return
		}
		mode = normalizeCeremonyPresenceMode(mode)
		if mode == "" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported presence mode", nil)
			return
		}
		if err := s.saveCeremonyPresencePreference(r.Context(), ownerID, mode); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		if _, err := onboarding.StartCeremonyJourney(r.Context(), s.cfg.DB, "trust_boundaries"); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		trustMeta := map[string]any{
			"ceremonyStep":     "trust_boundaries",
			"ceremonyType":     "multi_select",
			"ceremonyControls": ceremonyTrustControls,
			"ceremonyDefaults": ceremonyTrustDefaults,
		}
		if err := s.injectCeremonyAssistantMessage(r.Context(), req.ChatID, ceremonyTrustMessage, trustMeta); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}

	case "trust_boundaries":
		var boundaries map[string]bool
		if err := json.Unmarshal(req.Value, &boundaries); err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "value must be a trust boundaries map", nil)
			return
		}
		tb := normalizeCeremonyTrustBoundaries(boundariesFromMap(boundaries))
		if err := saveCeremonyConfiguration(r.Context(), s.cfg.DB, ownerID, ceremonyTrustBoundariesKey, tb); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		if _, err := onboarding.StartCeremonyJourney(r.Context(), s.cfg.DB, "personalization_seed"); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		rememberMeta := map[string]any{"ceremonyStep": "personalization_seed"}
		if err := s.injectCeremonyAssistantMessage(r.Context(), req.ChatID, ceremonyRememberMessage, rememberMeta); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}

	case "pact_summary":
		switch req.Action {
		case "confirm":
			if _, err := s.cfg.Navi.RecordUserMessage(r.Context(), req.ChatID, "Looks right"); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}
			if _, err := onboarding.CompleteCeremonyJourney(r.Context(), s.cfg.DB); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}
			ownerSeed := s.loadCeremonyOwnerProfileSeed(r.Context(), ownerID)
			if err := s.injectCeremonyAssistantMessage(r.Context(), req.ChatID, buildCeremonyConfirmClosing(ownerSeed), nil); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}

		case "skip":
			if _, err := s.cfg.Navi.RecordUserMessage(r.Context(), req.ChatID, "Skip for now"); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}
			if _, err := onboarding.SkipCeremonyJourney(r.Context(), s.cfg.DB); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}
			if err := s.injectCeremonyAssistantMessage(r.Context(), req.ChatID, buildCeremonySkipClosing(), nil); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}

		case "adjust":
			if _, err := onboarding.StartCeremonyJourney(r.Context(), s.cfg.DB, "owner_recognition"); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}
			adjustMeta := map[string]any{"ceremonyStep": "owner_recognition"}
			if err := s.injectCeremonyAssistantMessage(r.Context(), req.ChatID, "Let's revisit. What should I call you?", adjustMeta); err != nil {
				replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
				return
			}

		default:
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "action must be confirm, skip, or adjust", nil)
			return
		}

	default:
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported step: "+req.Step, nil)
		return
	}

	replyJSON(w, http.StatusOK, map[string]any{"step": req.Step, "ok": true})
}

// tryCeremonyMessageIntercept checks if the incoming message is for a ceremony chat and handles it
// directly without going through the normal LLM agent loop. Returns true if handled.
//
// Free-text steps (owner_recognition, personalization_seed): extract value, save, inject next message.
// Structured-choice steps (navi_presence, trust_boundaries, pact_summary): inject a steering message.
//
// Ceremony-V2 note: for structured-choice derailments, allow 2-3 conversational volleys (LLM-handled)
// before steering, so NAVI can engage naturally with tangents before returning to the step.
func (s *Server) tryCeremonyMessageIntercept(ctx context.Context, w http.ResponseWriter, chatID, content string) bool {
	if s.cfg.DB == nil || s.cfg.Navi == nil {
		return false
	}

	state, err := onboarding.LoadCeremonyJourneyState(ctx, s.cfg.DB)
	if err != nil || state.Status != onboarding.CeremonyStatusInProgress {
		return false
	}
	if state.ChatID != chatID {
		return false
	}

	ownerID, err := store.GetOwnerID(ctx, s.cfg.DB)
	if err != nil || ownerID == "" {
		return false
	}

	switch state.CurrentStep {
	case "owner_recognition":
		if _, err := s.cfg.Navi.RecordUserMessage(ctx, chatID, content); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to record message")
			return true
		}
		owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
		if err != nil || !exists {
			replyError(w, http.StatusInternalServerError, "owner lookup failed")
			return true
		}
		displayName := strings.TrimSpace(content)
		if displayName == "" {
			displayName = "there"
		}
		if err := s.saveCeremonyOwnerProfileSeed(ctx, owner, displayName); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to save name")
			return true
		}
		if _, err := onboarding.StartCeremonyJourney(ctx, s.cfg.DB, "navi_presence"); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to advance ceremony")
			return true
		}
		presenceMeta := map[string]any{
			"ceremonyStep":     "navi_presence",
			"ceremonyType":     "single_select",
			"ceremonyControls": ceremonyPresenceControls,
		}
		if err := s.injectCeremonyAssistantMessage(ctx, chatID, ceremonyPresenceMessage, presenceMeta); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to inject ceremony message")
			return true
		}
		replyJSON(w, http.StatusCreated, map[string]any{"status": "queued"})
		return true

	case "personalization_seed":
		if _, err := s.cfg.Navi.RecordUserMessage(ctx, chatID, content); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to record message")
			return true
		}
		seed := s.routeCeremonyPersonalizationSeed(ctx, ownerID, content)
		if err := saveCeremonyConfiguration(ctx, s.cfg.DB, ownerID, ceremonyPersonalizationKey, personalizationConfigFromResponse(seed)); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to save personalization")
			return true
		}
		if err := s.savePreferenceSignalsForCeremonySeed(ctx, ownerID, seed); err != nil {
			slog.Warn("ceremony: failed to save preference signals", "error", err)
		}
		if _, err := onboarding.StartCeremonyJourney(ctx, s.cfg.DB, "pact_summary"); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to advance ceremony")
			return true
		}
		ownerSeed := s.loadCeremonyOwnerProfileSeed(ctx, ownerID)
		presence := loadCeremonyPresencePreference(ctx, s.cfg.DB, ownerID)
		boundaries := loadCeremonyTrustBoundaries(ctx, s.cfg.DB, ownerID)
		pactLines := buildCeremonyPactSummary(ownerSeed, presence, boundaries, seed)
		pactMeta := map[string]any{
			"ceremonyStep":    "pact_summary",
			"ceremonyType":    "actions",
			"ceremonyActions": ceremonyPactActions,
		}
		if err := s.injectCeremonyAssistantMessage(ctx, chatID, strings.Join(pactLines, "\n\n"), pactMeta); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to inject ceremony message")
			return true
		}
		replyJSON(w, http.StatusCreated, map[string]any{"status": "queued"})
		return true

	case "navi_presence", "trust_boundaries", "pact_summary":
		// Structured-choice step: record the stray message and inject a steering response.
		if _, err := s.cfg.Navi.RecordUserMessage(ctx, chatID, content); err != nil {
			slog.Warn("ceremony: failed to record message during structured step", "error", err)
		}
		if err := s.injectCeremonyAssistantMessage(ctx, chatID, ceremonySteeringMessageForStep(state.CurrentStep), nil); err != nil {
			replyError(w, http.StatusInternalServerError, "failed to inject steering message")
			return true
		}
		replyJSON(w, http.StatusCreated, map[string]any{"status": "queued"})
		return true
	}

	return false
}

func ceremonySteeringMessageForStep(step string) string {
	switch step {
	case "navi_presence":
		return "I just need one thing — please choose how I should show up for you using the options above."
	case "trust_boundaries":
		return "I just need one thing — please let me know which actions need your approval using the options above."
	case "pact_summary":
		return "Almost there — please confirm, adjust, or skip the summary above to continue."
	default:
		return "I just need one thing from you — please answer using the options above."
	}
}

func emitCeremonyAssistantMessagePartial(ctx context.Context, db *sql.DB, chatID, runID, content string) error {
	if db == nil {
		return nil
	}
	ev := schema.NewRunEvent(
		schema.FactAssistantMessagePartial,
		schema.EventKindFact,
		chatID,
		schema.AgentNavi,
		runID,
		schema.VisibilityUser,
		schema.AssistantMessagePartialPayload{
			RunID:            runID,
			RuntimeSessionID: chatID,
			Content:          content,
			State:            "streaming",
		},
	)
	return store.AppendEvent(ctx, db, ev)
}

func emitCeremonyAssistantMessageCompleted(ctx context.Context, db *sql.DB, chatID, messageID, content string) error {
	if db == nil {
		return nil
	}
	ev := schema.NewRunEvent(
		schema.FactAssistantMessageCompleted,
		schema.EventKindFact,
		chatID,
		schema.AgentNavi,
		"",
		schema.VisibilityUser,
		schema.AssistantMessageCompletedPayload{
			ChatID:    chatID,
			MessageID: messageID,
			Content:   content,
		},
	)
	return store.AppendEvent(ctx, db, ev)
}

func ceremonyCumulativeChunks(content string, runesPerChunk int) []string {
	if content == "" {
		return nil
	}
	if runesPerChunk <= 0 {
		runesPerChunk = ceremonyStreamChunkRunes
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return nil
	}
	out := make([]string, 0, (len(runes)/runesPerChunk)+1)
	for end := runesPerChunk; end < len(runes); end += runesPerChunk {
		out = append(out, string(runes[:end]))
	}
	out = append(out, content)
	return out
}

func (s *Server) deliverCeremonyAssistantStream(chatID, messageID, content string) {
	if s == nil {
		return
	}
	ctx := context.Background()
	db := s.cfg.DB
	runID := "ceremony-" + messageID
	for _, partial := range ceremonyCumulativeChunks(content, ceremonyStreamChunkRunes) {
		if err := emitCeremonyAssistantMessagePartial(ctx, db, chatID, runID, partial); err != nil {
			slog.Warn("ceremony: failed to emit assistant.message.partial", "chat_id", chatID, "error", err)
			return
		}
		time.Sleep(ceremonyStreamChunkDelay)
	}
	if err := emitCeremonyAssistantMessageCompleted(ctx, db, chatID, messageID, content); err != nil {
		slog.Warn("ceremony: failed to emit assistant.message.completed", "chat_id", chatID, "error", err)
		return
	}
	if err := emitCeremonyRunCompleted(ctx, db, chatID); err != nil {
		slog.Warn("ceremony: failed to emit run.completed", "chat_id", chatID, "error", err)
	}
}

func emitCeremonyRunCompleted(ctx context.Context, db *sql.DB, chatID string) error {
	if db == nil {
		return nil
	}
	ev := schema.NewRunEvent(
		schema.FactRunCompleted,
		schema.EventKindFact,
		chatID,
		schema.AgentNavi,
		"",
		schema.VisibilityUser,
		schema.RunCompletedPayload{ChatID: chatID},
	)
	return store.AppendEvent(ctx, db, ev)
}

// injectCeremonyAssistantMessage persists a ceremony assistant turn and streams it over the live feed.
func (s *Server) injectCeremonyAssistantMessage(ctx context.Context, chatID, content string, metadata map[string]any) error {
	if s.cfg.Navi == nil {
		return fmt.Errorf("navi not configured")
	}
	msgID, err := s.cfg.Navi.InjectCeremonyMessage(ctx, chatID, content, metadata)
	if err != nil {
		return err
	}
	go s.deliverCeremonyAssistantStream(chatID, msgID, content)
	return nil
}

func boundariesFromMap(m map[string]bool) ceremonyTrustBoundaryDefaults {
	return ceremonyTrustBoundaryDefaults{
		ConfirmBeforeSendingMessages:             m["confirm_before_sending_messages"],
		ConfirmBeforeChangingFiles:               m["confirm_before_changing_files"],
		ConfirmBeforePurchases:                   m["confirm_before_purchases"],
		ConfirmBeforeRememberingSensitiveDetails: m["confirm_before_remembering_sensitive_details"],
		ConfirmBeforeActingOnInferredPreferences: m["confirm_before_acting_on_inferred_preferences"],
		ConfirmBeforeInterruptingProactively:     m["confirm_before_interrupting_proactively"],
		ConfirmBeforeExternalChanges:             m["confirm_before_external_changes"],
	}
}
