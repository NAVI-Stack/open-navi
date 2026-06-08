package experience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// GovernanceBoundsProvider exposes persona-relevant governance bounds.
type GovernanceBoundsProvider interface {
	Bounds(ctx context.Context, req BoundsRequest) GovernanceBounds
}

// BoundsRequest is the stub governance adapter input.
type BoundsRequest struct {
	Mode            string
	ActiveRole      ActiveRole
	TaskArchetype   string
	LastUserMessage string
	LiveContext     LiveContext
}

// DefaultGovernanceBoundsProvider provides the Stream-0 stub bounds adapter.
type DefaultGovernanceBoundsProvider struct{}

// Bounds derives safe default governance bounds from live context.
func (DefaultGovernanceBoundsProvider) Bounds(_ context.Context, req BoundsRequest) GovernanceBounds {
	live := req.LiveContext
	lower := strings.ToLower(req.LastUserMessage)
	highStakes := live.StakesLevel >= 0.72
	emotionallySensitive := live.EmotionalIntensity >= 0.68
	requiresNeutrality := strings.Contains(lower, "neutral") || strings.Contains(lower, "objective") || strings.Contains(lower, "impartial") || (highStakes && strings.Contains(lower, "review"))

	initiativeCap := 0.80
	if highStakes || requiresNeutrality {
		initiativeCap = 0.60
	}
	if req.Mode == "wizard" {
		initiativeCap = maxFloat(initiativeCap, 0.72)
	}

	recommendationCap := 0.78
	if live.AmbiguityLevel >= 0.70 {
		recommendationCap = 0.50
	} else if highStakes {
		recommendationCap = 0.60
	}

	directnessCap := 0.88
	if emotionallySensitive {
		directnessCap = 0.58
	} else if requiresNeutrality {
		directnessCap = 0.62
	}

	challengeCap := 0.82
	if emotionallySensitive {
		challengeCap = 0.40
	} else if highStakes {
		challengeCap = 0.55
	}

	caps := []GovernanceCap{{Trait: "initiative_style", MaxValue: initiativeCap, Reason: "governance_cap"}, {Trait: "recommendation_directiveness", MaxValue: recommendationCap, Reason: "governance_cap"}, {Trait: "directness", MaxValue: directnessCap, Reason: "governance_cap"}, {Trait: "challenge_intensity", MaxValue: challengeCap, Reason: "governance_cap"}}
	gates := []GovernanceGate{}
	if highStakes || emotionallySensitive {
		gates = append(gates, GovernanceGate{Trait: "humor_playfulness", GateActive: true, Reason: "high_stakes_context"})
		caps = append(caps, GovernanceCap{Trait: "humor_playfulness", MaxValue: 0.0, Reason: "high_stakes_context"})
	}

	return GovernanceBounds{
		TraitCaps:   caps,
		TraitFloors: nil,
		TraitGates:  gates,
		ContextFlags: GovernanceContextFlags{
			HighStakes:           highStakes,
			EmotionallySensitive: emotionallySensitive,
			RequiresNeutrality:   requiresNeutrality,
		},
	}
}

// Engine builds the effective experience-layer state for a turn.
type Engine struct {
	boundsProvider GovernanceBoundsProvider
}

// NewEngine constructs a new experience engine.
func NewEngine(bounds GovernanceBoundsProvider) *Engine {
	if bounds == nil {
		bounds = DefaultGovernanceBoundsProvider{}
	}
	return &Engine{boundsProvider: bounds}
}

// Build computes the merge input, effective state, compiled payload, and serialized fragment.
func (e *Engine) Build(ctx context.Context, profile ProfileConfig, req BuildRequest) (RenderedControl, error) {
	profile = normalizeProfile(profile, req.Mode)
	role := resolveRoleContext(profile, req)
	live := deriveLiveContext(req.LastUserMessage)
	relationship := normalizeRelationship(req.RelationshipProfile)
	turnInput, turnTrace, turnPrefs := deriveTurnOverrides(req.LastUserMessage)
	explicitInput := mergeExplicitTurnInput(req.ExplicitSessionOverrides, turnInput)
	explicitTrace := mergeExplicitTurnTrace(req.SessionOverrideTrace, turnTrace)
	outputPrefs := mergeOutputPreferences(profile.OutputPreferences, req.OutputPreferences)
	outputPrefs = mergeOutputPreferences(outputPrefs, req.SessionOutputPreferences)
	outputPrefs = mergeOutputPreferences(outputPrefs, turnPrefs)
	coreIdentity := mergeTraitSetConfigs(profile.CoreIdentity, req.CoreIdentity)
	var modules []ModuleConfig
	for _, m := range profile.PersonaModules {
		m.Source = "stored"
		modules = append(modules, m)
	}
	for _, m := range req.PersonaModules {
		m.Source = "request"
		modules = append(modules, m)
	}

	input := MergeEngineInput{
		CoreIdentity:          TraitSetConfig{Traits: mergedCoreIdentityTraits(coreIdentity.Traits)},
		GovernanceBounds:      e.boundsProvider.Bounds(ctx, BoundsRequest{Mode: profile.ID, ActiveRole: role.ActiveRole, TaskArchetype: role.TaskArchetype, LastUserMessage: req.LastUserMessage, LiveContext: live}),
		RoleContext:           role,
		PersonaModules:        normalizeModules(modules),
		RelationshipProfile:   relationship,
		LiveContext:           live,
		ExplicitTurnOverrides: explicitInput,
		OutputPreferences:     outputPrefs,
	}

	state := computeEffectivePersonaState(input, explicitTrace)
	payload := compilePayload(state)
	fragment, budget, audit := serializePayload(payload)
	payload.Budget = budget
	payload.Audit = audit

	return RenderedControl{
		Input:    input,
		State:    state,
		Payload:  payload,
		Fragment: fragment,
	}, nil
}

func normalizeProfile(profile ProfileConfig, mode string) ProfileConfig {
	mode = strings.ToLower(strings.TrimSpace(mode))
	var base ProfileConfig
	if mode == "wizard" || strings.EqualFold(profile.ID, "wizard") {
		base = DefaultWizardProfile()
	} else {
		base = DefaultStandardProfile()
	}
	if profile.ID != "" {
		base.ID = strings.ToLower(strings.TrimSpace(profile.ID))
	}
	if profile.DisplayName != "" {
		base.DisplayName = profile.DisplayName
	}
	if len(profile.CoreIdentity.Traits) > 0 {
		if base.CoreIdentity.Traits == nil {
			base.CoreIdentity.Traits = map[string]float64{}
		}
		for trait, value := range profile.CoreIdentity.Traits {
			base.CoreIdentity.Traits[strings.TrimSpace(trait)] = clamp01(value)
		}
	}
	if len(profile.PersonaModules) > 0 {
		base.PersonaModules = normalizeModules(profile.PersonaModules)
	}
	base.OutputPreferences = mergeOutputPreferences(base.OutputPreferences, profile.OutputPreferences)
	if profile.RoleContext.ActiveRole != "" {
		base.RoleContext.ActiveRole = profile.RoleContext.ActiveRole
	}
	if profile.RoleContext.TaskArchetype != "" {
		base.RoleContext.TaskArchetype = profile.RoleContext.TaskArchetype
	}
	if profile.RoleContext.DelegationMode != "" {
		base.RoleContext.DelegationMode = profile.RoleContext.DelegationMode
	}
	if base.RoleContext.ActiveRole == "" {
		base.RoleContext.ActiveRole = RoleAssistant
	}
	if base.RoleContext.DelegationMode == "" {
		base.RoleContext.DelegationMode = DelegationNone
	}
	return base
}

func resolveRoleContext(profile ProfileConfig, req BuildRequest) RoleContext {
	role := profile.RoleContext
	if req.ActiveRole != "" {
		role.ActiveRole = req.ActiveRole
	}
	if req.TaskArchetype != "" {
		role.TaskArchetype = req.TaskArchetype
	}
	if req.DelegationMode != "" {
		role.DelegationMode = req.DelegationMode
	}
	if role.ActiveRole == "" {
		role.ActiveRole = RoleAssistant
	}
	if role.DelegationMode == "" {
		role.DelegationMode = DelegationNone
	}
	return role
}

func mergedCoreIdentityTraits(overrides map[string]float64) map[string]float64 {
	traits := DefaultCoreIdentityTraits()
	for trait, value := range overrides {
		if _, ok := traitDefinitions[trait]; !ok {
			continue
		}
		traits[trait] = clamp01(value)
	}
	return traits
}

func normalizeModules(modules []ModuleConfig) []ModuleConfig {
	if len(modules) == 0 {
		return nil
	}

	type mappedMod struct {
		idx int
		mod ModuleConfig
	}
	// Dedupe key is ModuleID only. Module IDs are unique logical identifiers;
	// if two modules share an ID they represent the same logical entity regardless
	// of kind. Use distinct IDs for distinct logical modules (e.g. "tone_overlay"
	// vs "tone_base").
	best := make(map[string]mappedMod)

	for i, module := range modules {
		if strings.TrimSpace(module.ModuleID) == "" {
			continue
		}
		clean := ModuleConfig{
			ModuleID:           strings.TrimSpace(module.ModuleID),
			Version:            strings.TrimSpace(module.Version),
			Kind:               module.Kind,
			Scope:              module.Scope,
			OriginScope:        strings.TrimSpace(module.OriginScope),
			Source:             strings.TrimSpace(module.Source),
			IsLocked:           module.IsLocked,
			Strength:           module.Strength,
			TraitContributions: map[string]float64{},
		}
		if clean.Version == "" {
			clean.Version = "v1"
		}
		if clean.Kind == "" {
			clean.Kind = ModuleKindBundle
		}
		if clean.Scope == "" {
			clean.Scope = ModuleScopeGlobal
		}
		if clean.Strength <= 0 {
			clean.Strength = 1
		}
		// Enforce explicit origin tagging. Every module entering the
		// normalization pipeline must have a known origin. Untagged modules
		// are treated as system-level to prevent silent mis-ranking.
		if clean.Source == "" && clean.OriginScope == "" {
			clean.OriginScope = "system"
		}
		for trait, value := range module.TraitContributions {
			if _, ok := traitDefinitions[trait]; !ok {
				continue
			}
			clean.TraitContributions[trait] = clamp01(value)
		}

		existing, ok := best[clean.ModuleID]
		if !ok {
			best[clean.ModuleID] = mappedMod{idx: i, mod: clean}
			continue
		}

		// Primary: origin = request > owner > system
		r1 := originRank(clean.Source, clean.OriginScope)
		r2 := originRank(existing.mod.Source, existing.mod.OriginScope)

		// Support "locked" / non-overridable owner modules.
		// When an owner module is marked as locked, request-origin modules
		// with the same ID should NOT override it. This prevents one-off
		// requests from overriding persistent owner constraints that represent
		// hard preferences or governance-like behavior.
		if existing.mod.IsLocked && r1 > r2 {
			continue
		}

		if r1 > r2 {
			best[clean.ModuleID] = mappedMod{idx: i, mod: clean}
			continue
		} else if r1 < r2 {
			continue
		}

		// Secondary: same logical module ID -> highest valid semver
		v1 := clean.Version
		v2 := existing.mod.Version
		if !strings.HasPrefix(v1, "v") {
			v1 = "v" + v1
		}
		if !strings.HasPrefix(v2, "v") {
			v2 = "v" + v2
		}
		if !semver.IsValid(v1) {
			v1 = "v0.0.0"
		}
		if !semver.IsValid(v2) {
			v2 = "v0.0.0"
		}
		cmp := semver.Compare(v1, v2)
		if cmp > 0 {
			best[clean.ModuleID] = mappedMod{idx: i, mod: clean}
			continue
		} else if cmp < 0 {
			continue
		}

		// Tertiary: later ordinal wins
		if i > existing.idx {
			best[clean.ModuleID] = mappedMod{idx: i, mod: clean}
		}
	}

	out := make([]ModuleConfig, 0, len(best))
	for _, m := range best {
		out = append(out, m.mod)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if moduleScopeRank(out[i].Scope) != moduleScopeRank(out[j].Scope) {
			return moduleScopeRank(out[i].Scope) > moduleScopeRank(out[j].Scope)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Strength != out[j].Strength {
			return out[i].Strength > out[j].Strength
		}
		if out[i].Version != out[j].Version {
			return out[i].Version > out[j].Version
		}
		return out[i].ModuleID < out[j].ModuleID
	})
	return out
}

func normalizeRelationship(profile RelationshipProfile) RelationshipProfile {
	if profile.TraitEstimates == nil {
		profile.TraitEstimates = map[string]float64{}
	}
	if profile.PerTraitConfidence == nil {
		profile.PerTraitConfidence = map[string]float64{}
	}
	if profile.TraitSignalCount == nil {
		profile.TraitSignalCount = map[string]int{}
	}
	if profile.TraitAverageStrength == nil {
		profile.TraitAverageStrength = map[string]float64{}
	}
	if profile.TraitLastSignalAt == nil {
		profile.TraitLastSignalAt = map[string]string{}
	}
	profile.Confidence = clamp01(profile.Confidence)
	if profile.TotalSignalCount < 0 {
		profile.TotalSignalCount = 0
	}
	if profile.ExplicitSignalCount < 0 {
		profile.ExplicitSignalCount = 0
	}
	if profile.ExplicitSignalCount > profile.TotalSignalCount {
		profile.ExplicitSignalCount = profile.TotalSignalCount
	}
	for trait, value := range profile.TraitEstimates {
		if _, ok := traitDefinitions[trait]; !ok {
			delete(profile.TraitEstimates, trait)
			continue
		}
		profile.TraitEstimates[trait] = clamp01(value)
	}
	for trait, value := range profile.PerTraitConfidence {
		if _, ok := traitDefinitions[trait]; !ok {
			delete(profile.PerTraitConfidence, trait)
			continue
		}
		profile.PerTraitConfidence[trait] = clamp01(value)
	}
	for trait, count := range profile.TraitSignalCount {
		if _, ok := traitDefinitions[trait]; !ok {
			delete(profile.TraitSignalCount, trait)
			continue
		}
		if count < 0 {
			profile.TraitSignalCount[trait] = 0
		}
	}
	for trait, value := range profile.TraitAverageStrength {
		if _, ok := traitDefinitions[trait]; !ok {
			delete(profile.TraitAverageStrength, trait)
			continue
		}
		profile.TraitAverageStrength[trait] = clamp01(value)
	}
	for trait, value := range profile.TraitLastSignalAt {
		if _, ok := traitDefinitions[trait]; !ok {
			delete(profile.TraitLastSignalAt, trait)
			continue
		}
		profile.TraitLastSignalAt[trait] = strings.TrimSpace(value)
	}
	return profile
}

func mergeOutputPreferences(base, override OutputPreferences) OutputPreferences {
	if override.PreferredLength != "" {
		base.PreferredLength = override.PreferredLength
	}
	if override.PreferredFormat != "" {
		base.PreferredFormat = override.PreferredFormat
	}
	if override.SummaryFirst != nil {
		value := *override.SummaryFirst
		base.SummaryFirst = &value
	}
	return base
}

func mergeExplicitTurnInput(base, override ExplicitTurnInput) ExplicitTurnInput {
	merged := ExplicitTurnInput{}
	if len(base.TraitOverrides) > 0 {
		merged.TraitOverrides = map[string]float64{}
		for trait, value := range base.TraitOverrides {
			merged.TraitOverrides[trait] = clamp01(value)
		}
	}
	if len(base.TraitCaps) > 0 {
		merged.TraitCaps = map[string]float64{}
		for trait, value := range base.TraitCaps {
			merged.TraitCaps[trait] = clamp01(value)
		}
	}
	if len(base.TraitFloors) > 0 {
		merged.TraitFloors = map[string]float64{}
		for trait, value := range base.TraitFloors {
			merged.TraitFloors[trait] = clamp01(value)
		}
	}
	if len(override.TraitOverrides) > 0 {
		if merged.TraitOverrides == nil {
			merged.TraitOverrides = map[string]float64{}
		}
		for trait, value := range override.TraitOverrides {
			merged.TraitOverrides[trait] = clamp01(value)
		}
	}
	if len(override.TraitCaps) > 0 {
		if merged.TraitCaps == nil {
			merged.TraitCaps = map[string]float64{}
		}
		for trait, value := range override.TraitCaps {
			merged.TraitCaps[trait] = clamp01(value)
		}
	}
	if len(override.TraitFloors) > 0 {
		if merged.TraitFloors == nil {
			merged.TraitFloors = map[string]float64{}
		}
		for trait, value := range override.TraitFloors {
			merged.TraitFloors[trait] = clamp01(value)
		}
	}
	if len(merged.TraitOverrides) == 0 {
		merged.TraitOverrides = nil
	}
	if len(merged.TraitCaps) == 0 {
		merged.TraitCaps = nil
	}
	if len(merged.TraitFloors) == 0 {
		merged.TraitFloors = nil
	}
	return merged
}

func mergeExplicitTurnTrace(base, override ExplicitTurnTrace) ExplicitTurnTrace {
	merged := base
	merged.MustBeBrief = merged.MustBeBrief || override.MustBeBrief
	merged.MustAskClarifyingQuestion = merged.MustAskClarifyingQuestion || override.MustAskClarifyingQuestion
	merged.MustNotUseHumor = merged.MustNotUseHumor || override.MustNotUseHumor
	merged.MustBeHighlyStructured = merged.MustBeHighlyStructured || override.MustBeHighlyStructured
	if override.UserRequestedToneShift != "" {
		merged.UserRequestedToneShift = override.UserRequestedToneShift
	}
	return merged
}

func deriveLiveContext(content string) LiveContext {
	lower := strings.ToLower(strings.TrimSpace(content))
	wordCount := len(strings.Fields(lower))
	ambiguity := keywordScore(lower, []string{"maybe", "not sure", "unsure", "what do you think", "should i", "could you", "help me think", "perhaps"}, 0.22)
	if wordCount > 0 && wordCount <= 4 {
		ambiguity += 0.20
	}
	urgency := keywordScore(lower, []string{"urgent", "asap", "immediately", "right now", "today", "blocked", "deadline", "outage", "incident"}, 0.22)
	emotion := keywordScore(lower, []string{"upset", "anxious", "worried", "frustrated", "sad", "angry", "panic", "overwhelmed", "stressed"}, 0.18)
	stakes := keywordScore(lower, []string{"medical", "legal", "lawsuit", "tax", "financial", "security", "production", "prod", "compliance", "contract", "patient", "safety", "incident"}, 0.24)
	complexity := keywordScore(lower, []string{"compare", "tradeoff", "architecture", "migration", "refactor", "multi-step", "debug", "design", "implement", "plan", "analyze"}, 0.16)
	if wordCount > 80 {
		complexity += 0.18
	}
	if wordCount > 160 {
		complexity += 0.12
	}
	if strings.Count(lower, "?") > 1 {
		ambiguity += 0.08
	}
	return LiveContext{
		AmbiguityLevel:     clamp01(0.15 + ambiguity),
		UrgencyLevel:       clamp01(0.10 + urgency),
		EmotionalIntensity: clamp01(0.10 + emotion),
		StakesLevel:        clamp01(0.10 + stakes),
		ComplexityLevel:    clamp01(0.20 + complexity),
	}
}

func deriveTurnOverrides(content string) (ExplicitTurnInput, ExplicitTurnTrace, OutputPreferences) {
	lower := strings.ToLower(strings.TrimSpace(content))
	input := ExplicitTurnInput{
		TraitOverrides: map[string]float64{},
		TraitCaps:      map[string]float64{},
		TraitFloors:    map[string]float64{},
	}
	trace := ExplicitTurnTrace{}
	prefs := OutputPreferences{}

	setFloor := func(trait string, value float64) {
		if current, ok := input.TraitFloors[trait]; !ok || value > current {
			input.TraitFloors[trait] = clamp01(value)
		}
	}
	setCap := func(trait string, value float64) {
		if current, ok := input.TraitCaps[trait]; !ok || value < current {
			input.TraitCaps[trait] = clamp01(value)
		}
	}

	if containsAny(lower, "brief", "concise", "short answer", "quick answer", "keep it short") {
		trace.MustBeBrief = true
		input.TraitOverrides["verbosity"] = 0.22
		prefs.PreferredLength = LengthShort
	}
	if containsAny(lower, "step by step", "step-by-step", "walk me through", "break it down") {
		trace.MustBeHighlyStructured = true
		setFloor("structure_level", 0.80)
		prefs.PreferredFormat = FormatStepwise
	}
	if containsAny(lower, "structured", "use bullets", "outline") {
		trace.MustBeHighlyStructured = true
		setFloor("structure_level", 0.68)
		prefs.PreferredFormat = FormatStructured
	}
	if containsAny(lower, "executive summary", "summary first") {
		summary := true
		prefs.SummaryFirst = &summary
		prefs.PreferredFormat = FormatExecutive
	}
	if containsAny(lower, "be direct", "be blunt", "just tell me", "tell me clearly") {
		trace.UserRequestedToneShift = ToneMoreDirect
		setFloor("directness", 0.74)
		setFloor("recommendation_directiveness", 0.66)
	}
	if containsAny(lower, "gentle", "gently", "softer", "soften") {
		trace.UserRequestedToneShift = ToneGentler
		setCap("directness", 0.56)
		setFloor("warmth", 0.66)
	}
	if containsAny(lower, "formal", "professional tone") {
		trace.UserRequestedToneShift = ToneMoreFormal
		setFloor("formality", 0.72)
	}
	if containsAny(lower, "casual", "informal") {
		trace.UserRequestedToneShift = ToneMoreCasual
		setCap("formality", 0.36)
		setFloor("conversationality", 0.62)
	}
	if containsAny(lower, "serious", "keep it serious") {
		trace.UserRequestedToneShift = ToneMoreSerious
		trace.MustNotUseHumor = true
		setFloor("seriousness", 0.78)
		setCap("humor_playfulness", 0.0)
	}
	if containsAny(lower, "lighter", "more playful", "more levity") {
		trace.UserRequestedToneShift = ToneLighter
		setFloor("humor_playfulness", 0.30)
	}
	if containsAny(lower, "ask clarifying", "ask questions first") {
		trace.MustAskClarifyingQuestion = true
		setCap("clarification_threshold", 0.25)
	}
	if containsAny(lower, "don't ask questions", "do not ask questions", "infer when safe") {
		setFloor("clarification_threshold", 0.70)
	}

	if len(input.TraitOverrides) == 0 {
		input.TraitOverrides = nil
	}
	if len(input.TraitCaps) == 0 {
		input.TraitCaps = nil
	}
	if len(input.TraitFloors) == 0 {
		input.TraitFloors = nil
	}
	return input, trace, prefs
}

func computeEffectivePersonaState(input MergeEngineInput, trace ExplicitTurnTrace) EffectivePersonaState {
	liveAdjustments := deriveLiveContextAdjustments(input.LiveContext)
	resolved := make(map[string]TraitValueState, len(traitOrder))
	gatedTraits := make([]string, 0)
	warnings := make([]string, 0)
	governanceApplied := false
	coldStart := input.RelationshipProfile.Confidence < ColdStartConfidenceFloor
	if coldStart {
		warnings = appendUniqueStrings(warnings, "cold_start_active")
	}

	for _, trait := range traitOrder {
		def := traitDefinitions[trait]
		state := TraitValueState{Domain: def.Domain, MergeClass: def.MergeClass, AdaptivityClass: def.AdaptivityClass}
		value, usedGovernance, gated := resolveTraitValue(trait, input, liveAdjustments, coldStart)
		if gated {
			state.GatedOff = true
			gatedTraits = appendUniqueStrings(gatedTraits, trait)
		}
		if usedGovernance {
			governanceApplied = true
		}
		state.Value = clamp01(value)
		resolved[trait] = state
	}

	applyDependencyCorrections(resolved, input, trace)
	if hasDependencyCorrections(resolved) {
		warnings = appendUniqueStrings(warnings, "dependency_correction_applied")
	}
	if governanceApplied || len(gatedTraits) > 0 {
		warnings = appendUniqueStrings(warnings, "governance_override_applied")
	}

	state := EffectivePersonaState{
		SchemaVersion:         SchemaVersion,
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		MergeEngineVersion:    MergeEngineVersion,
		RoleContext:           input.RoleContext,
		ResolvedTraits:        resolved,
		GovernanceTrace:       buildGovernanceTrace(input.GovernanceBounds, input.LiveContext, coldStart),
		LiveContextTrace:      buildLiveContextTrace(input.LiveContext),
		ExplicitTurnOverrides: trace,
		OutputPreferences:     input.OutputPreferences,
		Audit: EffectiveStateAudit{
			SourcePrecedenceOrder: defaultSourcePrecedenceOrder(),
			GatedSources:          gatedTraits,
			WarningCodes:          warnings,
		},
	}
	state.StateID = "eps-" + hashCanonical(canonicalStateShape(state))[:12]
	return state
}

func resolveTraitValue(trait string, input MergeEngineInput, liveAdjustments map[string]float64, coldStart bool) (float64, bool, bool) {
	def := traitDefinitions[trait]
	if input.ExplicitTurnOverrides.TraitOverrides != nil {
		if value, ok := input.ExplicitTurnOverrides.TraitOverrides[trait]; ok {
			value, governanceApplied, gated := applyTraitBounds(trait, value, input, coldStart)
			return value, governanceApplied, gated
		}
	}

	gated := isTraitGated(trait, input.GovernanceBounds)
	var value float64
	if def.MergeClass == MergePriorityOverride {
		value = resolvePriorityTrait(trait, input, liveAdjustments)
	} else {
		value = resolveBlendedTrait(trait, input, liveAdjustments)
	}
	if def.MergeClass == MergeGatedActivation && gated {
		value = 0
	}
	value, governanceApplied, gatedByClamp := applyTraitBounds(trait, value, input, coldStart)
	return value, governanceApplied, gated || gatedByClamp
}

func resolveBlendedTrait(trait string, input MergeEngineInput, liveAdjustments map[string]float64) float64 {
	if input.CoreIdentity.Traits == nil {
		input.CoreIdentity.Traits = map[string]float64{}
	}
	totalWeight := 0.0
	weighted := 0.0
	if value, ok := input.CoreIdentity.Traits[trait]; ok {
		totalWeight += 0.40
		weighted += value * 0.40
	}
	if traitDefinitions[trait].AdaptivityClass != AdaptivityStable {
		if value, ok := input.RelationshipProfile.TraitEstimates[trait]; ok {
			weight := 0.20 * clamp01(input.RelationshipProfile.Confidence)
			if perTrait, ok := input.RelationshipProfile.PerTraitConfidence[trait]; ok {
				weight *= clamp01(perTrait)
			}
			if weight > 0 {
				totalWeight += weight
				weighted += value * weight
			}
		}
	}
	modules := modulesForTrait(input.PersonaModules, trait)
	if len(modules) > 0 {
		moduleStrength := 0.0
		for _, module := range modules {
			moduleStrength += module.Strength
		}
		if moduleStrength <= 0 {
			moduleStrength = float64(len(modules))
		}
		for _, module := range modules {
			weight := 0.15 * (module.Strength / moduleStrength)
			totalWeight += weight
			weighted += module.TraitContributions[trait] * weight
		}
	}
	if value, ok := liveAdjustments[trait]; ok {
		totalWeight += 0.10
		weighted += value * 0.10
	}
	if totalWeight == 0 {
		return traitDefinitions[trait].DefaultValue
	}
	return clamp01(weighted / totalWeight)
}

func resolvePriorityTrait(trait string, input MergeEngineInput, liveAdjustments map[string]float64) float64 {
	if value, ok := liveAdjustments[trait]; ok {
		return clamp01(value)
	}
	if value, ok := input.RelationshipProfile.TraitEstimates[trait]; ok && input.RelationshipProfile.Confidence > 0 {
		return clamp01(value)
	}
	for _, module := range input.PersonaModules {
		if value, ok := module.TraitContributions[trait]; ok {
			return clamp01(value)
		}
	}
	if value, ok := input.CoreIdentity.Traits[trait]; ok {
		return clamp01(value)
	}
	return traitDefinitions[trait].DefaultValue
}

func modulesForTrait(modules []ModuleConfig, trait string) []ModuleConfig {
	out := make([]ModuleConfig, 0)
	for _, module := range modules {
		if _, ok := module.TraitContributions[trait]; ok {
			out = append(out, module)
		}
	}
	return out
}

func applyTraitBounds(trait string, value float64, input MergeEngineInput, coldStart bool) (float64, bool, bool) {
	minValue, maxValue, governanceApplied := effectiveBoundsForTrait(trait, input, coldStart)
	before := value
	value = clamp(value, minValue, maxValue)
	gated := (trait == "humor_playfulness" && maxValue == 0)
	if math.Abs(before-value) > 0.0001 {
		governanceApplied = true
	}
	return value, governanceApplied, gated
}

func effectiveBoundsForTrait(trait string, input MergeEngineInput, coldStart bool) (float64, float64, bool) {
	minValue, maxValue := 0.0, 1.0
	governanceApplied := false
	for _, floor := range input.GovernanceBounds.TraitFloors {
		if floor.Trait != trait {
			continue
		}
		minValue = maxFloat(minValue, clamp01(floor.MinValue))
		governanceApplied = true
	}
	for _, cap := range input.GovernanceBounds.TraitCaps {
		if cap.Trait != trait {
			continue
		}
		maxValue = minFloat(maxValue, clamp01(cap.MaxValue))
		governanceApplied = true
	}
	if coldStart {
		switch trait {
		case "humor_playfulness":
			maxValue = minFloat(maxValue, 0.15)
		case "familiarity":
			maxValue = minFloat(maxValue, 0.20)
		case "clarification_threshold":
			maxValue = minFloat(maxValue, 0.45)
		case "challenge_intensity":
			maxValue = minFloat(maxValue, 0.60)
		case "recommendation_directiveness":
			maxValue = minFloat(maxValue, 0.60)
		case "structure_level":
			minValue = maxFloat(minValue, 0.60)
		}
	}
	if input.ExplicitTurnOverrides.TraitFloors != nil {
		if floor, ok := input.ExplicitTurnOverrides.TraitFloors[trait]; ok {
			minValue = maxFloat(minValue, clamp01(floor))
		}
	}
	if input.ExplicitTurnOverrides.TraitCaps != nil {
		if cap, ok := input.ExplicitTurnOverrides.TraitCaps[trait]; ok {
			maxValue = minFloat(maxValue, clamp01(cap))
		}
	}
	if minValue > maxValue {
		minValue = maxValue
	}
	return minValue, maxValue, governanceApplied
}

func isTraitGated(trait string, bounds GovernanceBounds) bool {
	for _, gate := range bounds.TraitGates {
		if gate.Trait == trait && gate.GateActive {
			return true
		}
	}
	return false
}

func deriveLiveContextAdjustments(live LiveContext) map[string]float64 {
	adjustments := map[string]float64{}
	if live.ComplexityLevel >= 0.55 {
		adjustments["analytic_depth"] = clamp01(0.55 + live.ComplexityLevel*0.35)
		adjustments["structure_level"] = clamp01(0.48 + live.ComplexityLevel*0.38)
		adjustments["verbosity"] = clamp01(0.32 + live.ComplexityLevel*0.30)
	}
	if live.UrgencyLevel >= 0.50 {
		adjustments["directness"] = clamp01(0.50 + live.UrgencyLevel*0.28)
		adjustments["decisiveness"] = clamp01(0.46 + live.UrgencyLevel*0.26)
		adjustments["recommendation_directiveness"] = clamp01(0.44 + live.UrgencyLevel*0.24)
	}
	if live.EmotionalIntensity >= 0.45 {
		adjustments["warmth"] = clamp01(0.50 + live.EmotionalIntensity*0.30)
		adjustments["supportiveness"] = clamp01(0.48 + live.EmotionalIntensity*0.34)
		adjustments["emotional_attunement"] = clamp01(0.52 + live.EmotionalIntensity*0.32)
		adjustments["challenge_intensity"] = clamp01(0.62 - live.EmotionalIntensity*0.26)
	}
	if live.StakesLevel >= 0.55 {
		adjustments["seriousness"] = clamp01(0.60 + live.StakesLevel*0.30)
		adjustments["structure_level"] = maxFloat(adjustments["structure_level"], clamp01(0.52+live.StakesLevel*0.26))
		adjustments["humor_playfulness"] = 0.0
	}
	if live.AmbiguityLevel >= 0.50 {
		adjustments["clarification_threshold"] = clamp01(0.70 - live.AmbiguityLevel*0.62)
		adjustments["recommendation_directiveness"] = minFloat(existingOr(adjustments, "recommendation_directiveness", 0.56), clamp01(0.72-live.AmbiguityLevel*0.34))
	}
	return adjustments
}

func applyDependencyCorrections(resolved map[string]TraitValueState, input MergeEngineInput, trace ExplicitTurnTrace) {
	applyCorrection := func(code string, traits ...string) {
		for _, trait := range traits {
			state := resolved[trait]
			state.DependencyCorrectionsApplied = appendUniqueStrings(state.DependencyCorrectionsApplied, code)
			resolved[trait] = state
		}
	}
	if resolved["directness"].Value > 0.75 && resolved["warmth"].Value < 0.30 && !(hasExplicitTrait(input, "directness") && hasExplicitTrait(input, "warmth")) && trace.UserRequestedToneShift != ToneMoreDirect {
		warmth := resolved["warmth"]
		if warmth.ClampRange != nil && warmth.ClampRange.Min >= 0.30 && warmth.Value <= warmth.ClampRange.Min+0.0001 {
			directness := resolved["directness"]
			directness.Value = clamp01(directness.Value - 0.10)
			resolved["directness"] = directness
			applyCorrection("directness_warmth_softened", "directness")
		} else {
			warmth.Value = maxFloat(warmth.Value, 0.30)
			resolved["warmth"] = warmth
			applyCorrection("directness_warmth_softened", "warmth")
		}
	}
	if resolved["challenge_intensity"].Value > 0.70 && resolved["supportiveness"].Value < 0.35 && trace.UserRequestedToneShift != ToneMoreDirect {
		support := resolved["supportiveness"]
		support.Value = maxFloat(support.Value, 0.35)
		resolved["supportiveness"] = support
		applyCorrection("challenge_supportiveness_raised", "supportiveness")
	}
	if resolved["humor_playfulness"].Value > 0 && (input.GovernanceBounds.ContextFlags.HighStakes || input.GovernanceBounds.ContextFlags.EmotionallySensitive) {
		humor := resolved["humor_playfulness"]
		if trace.UserRequestedToneShift == ToneLighter {
			cap := 0.25
			if input.GovernanceBounds.ContextFlags.EmotionallySensitive {
				cap = 0.15
			}
			humor.Value = minFloat(humor.Value, cap)
		} else {
			humor.Value = 0
			humor.GatedOff = true
		}
		resolved["humor_playfulness"] = humor
		applyCorrection("humor_gated_off", "humor_playfulness")
	}
	if resolved["initiative_style"].Value > maxInitiativeCap(input.GovernanceBounds) {
		initiative := resolved["initiative_style"]
		initiative.Value = maxInitiativeCap(input.GovernanceBounds)
		resolved["initiative_style"] = initiative
		applyCorrection("initiative_governance_capped", "initiative_style")
	}
	if resolved["recommendation_directiveness"].Value > 0.60 && input.LiveContext.AmbiguityLevel > 0.70 && trace.UserRequestedToneShift != ToneMoreDirect {
		recommendation := resolved["recommendation_directiveness"]
		recommendation.Value = minFloat(recommendation.Value, 0.50)
		resolved["recommendation_directiveness"] = recommendation
		applyCorrection("recommendation_ambiguity_capped", "recommendation_directiveness")
	}
	if input.LiveContext.ComplexityLevel > 0.70 && resolved["structure_level"].Value < 0.55 && input.OutputPreferences.PreferredFormat != FormatFreeform {
		structure := resolved["structure_level"]
		structure.Value = maxFloat(structure.Value, 0.60)
		resolved["structure_level"] = structure
		applyCorrection("structure_complexity_raised", "structure_level")
	}
	if resolved["clarification_threshold"].Value < 0.30 && input.LiveContext.AmbiguityLevel < 0.30 && !trace.MustAskClarifyingQuestion {
		clarification := resolved["clarification_threshold"]
		clarification.Value = maxFloat(clarification.Value, 0.35)
		resolved["clarification_threshold"] = clarification
		applyCorrection("clarification_suppressed", "clarification_threshold")
	}
}

func hasExplicitTrait(input MergeEngineInput, trait string) bool {
	if input.ExplicitTurnOverrides.TraitOverrides != nil {
		if _, ok := input.ExplicitTurnOverrides.TraitOverrides[trait]; ok {
			return true
		}
	}
	if input.ExplicitTurnOverrides.TraitFloors != nil {
		if _, ok := input.ExplicitTurnOverrides.TraitFloors[trait]; ok {
			return true
		}
	}
	if input.ExplicitTurnOverrides.TraitCaps != nil {
		if _, ok := input.ExplicitTurnOverrides.TraitCaps[trait]; ok {
			return true
		}
	}
	return false
}

func hasDependencyCorrections(resolved map[string]TraitValueState) bool {
	for _, state := range resolved {
		if len(state.DependencyCorrectionsApplied) > 0 {
			return true
		}
	}
	return false
}

func buildGovernanceTrace(bounds GovernanceBounds, live LiveContext, coldStart bool) GovernanceTrace {
	reasons := make([]string, 0)
	if bounds.ContextFlags.HighStakes {
		reasons = append(reasons, "high_stakes_context")
	}
	if bounds.ContextFlags.EmotionallySensitive {
		reasons = append(reasons, "emotionally_sensitive_context")
	}
	if bounds.ContextFlags.RequiresNeutrality {
		reasons = append(reasons, "requires_neutrality")
	}
	if live.AmbiguityLevel >= 0.70 {
		reasons = appendUniqueStrings(reasons, "high_ambiguity")
	}
	if coldStart {
		reasons = appendUniqueStrings(reasons, "cold_start_mode")
	}
	constraints := make([]GovernanceConstraint, 0, len(bounds.TraitCaps)+len(bounds.TraitFloors)+len(bounds.TraitGates))
	for _, cap := range bounds.TraitCaps {
		constraints = append(constraints, GovernanceConstraint{Trait: cap.Trait, ConstraintType: "cap", Value: cap.MaxValue, Reason: cap.Reason})
	}
	for _, floor := range bounds.TraitFloors {
		constraints = append(constraints, GovernanceConstraint{Trait: floor.Trait, ConstraintType: "floor", Value: floor.MinValue, Reason: floor.Reason})
	}
	for _, gate := range bounds.TraitGates {
		if !gate.GateActive {
			continue
		}
		constraints = append(constraints, GovernanceConstraint{Trait: gate.Trait, ConstraintType: "gate", Value: 1, Reason: gate.Reason})
	}
	return GovernanceTrace{
		AllowHumor:                     !isTraitGated("humor_playfulness", bounds),
		AllowHighChallenge:             challengeCap(bounds) >= 0.65,
		AllowHighDirectness:            directnessCap(bounds) >= 0.70,
		MaxInitiativeStyle:             maxInitiativeCap(bounds),
		MaxRecommendationDirectiveness: maxRecommendationCap(bounds),
		HighStakesContext:              bounds.ContextFlags.HighStakes,
		EmotionallySensitiveContext:    bounds.ContextFlags.EmotionallySensitive,
		RequiresNeutrality:             bounds.ContextFlags.RequiresNeutrality,
		Reasons:                        reasons,
		AdditionalConstraints:          constraints,
	}
}

func buildLiveContextTrace(live LiveContext) LiveContextTrace {
	confidence := clamp01(0.92 - (live.AmbiguityLevel * 0.55) - (live.StakesLevel * 0.18))
	return LiveContextTrace{
		AmbiguityLevel:     live.AmbiguityLevel,
		UrgencyLevel:       live.UrgencyLevel,
		EmotionalIntensity: live.EmotionalIntensity,
		StakesLevel:        live.StakesLevel,
		ComplexityLevel:    live.ComplexityLevel,
		ConfidenceLevel:    confidence,
	}
}

func compilePayload(state EffectivePersonaState) CompiledPersonaPayload {
	sourceHash := hashCanonical(canonicalStateShape(state))
	compiledAt := time.Now().UTC().Format(time.RFC3339)
	styleFlags := deriveStyleFlags(state)
	preferredLength := derivePreferredLength(state)
	responseShape := deriveResponseShape(state)
	summaryFirst := state.OutputPreferences.SummaryFirst != nil && *state.OutputPreferences.SummaryFirst
	formattingRules := deriveFormattingRules(responseShape, summaryFirst)
	prohibitedPatterns := deriveProhibitedPatterns(state)
	preserveVerbatim := activeTurnConstraints(state.ExplicitTurnOverrides)

	payload := CompiledPersonaPayload{
		SchemaVersion:   SchemaVersion,
		PayloadID:       "cpp-" + sourceHash[:12],
		CompiledAt:      compiledAt,
		SourceStateID:   state.StateID,
		CompilerVersion: CompilerVersion,
		SourceHash:      sourceHash,
		RoleAdapter:     state.RoleContext,
		CognitiveModulation: CognitiveModulation{
			AnalyticDepth:      state.ResolvedTraits["analytic_depth"].Value,
			Skepticism:         state.ResolvedTraits["skepticism"].Value,
			Decisiveness:       state.ResolvedTraits["decisiveness"].Value,
			ReasoningPosture:   deriveReasoningPosture(state.ResolvedTraits["analytic_depth"].Value),
			UncertaintyPosture: deriveUncertaintyPosture(state.ResolvedTraits["decisiveness"].Value),
			ChallengePosture:   deriveChallengePosture(state.ResolvedTraits["challenge_intensity"].Value),
		},
		ExpressionPolicy: ExpressionPolicy{
			Directness:        state.ResolvedTraits["directness"].Value,
			Warmth:            state.ResolvedTraits["warmth"].Value,
			Formality:         state.ResolvedTraits["formality"].Value,
			Seriousness:       state.ResolvedTraits["seriousness"].Value,
			Verbosity:         state.ResolvedTraits["verbosity"].Value,
			Conversationality: state.ResolvedTraits["conversationality"].Value,
			HumorPlayfulness:  state.ResolvedTraits["humor_playfulness"].Value,
			StyleFlags:        styleFlags,
		},
		BehaviorPolicy: BehaviorPolicy{
			InitiativeStyle:             state.ResolvedTraits["initiative_style"].Value,
			ClarificationThreshold:      state.ResolvedTraits["clarification_threshold"].Value,
			ChallengeIntensity:          state.ResolvedTraits["challenge_intensity"].Value,
			EmotionalAttunement:         state.ResolvedTraits["emotional_attunement"].Value,
			Supportiveness:              state.ResolvedTraits["supportiveness"].Value,
			Familiarity:                 state.ResolvedTraits["familiarity"].Value,
			RecommendationDirectiveness: state.ResolvedTraits["recommendation_directiveness"].Value,
			AskVsInferMode:              deriveAskVsInferMode(state.ResolvedTraits["clarification_threshold"].Value),
			RecommendationMode:          deriveRecommendationMode(state.ResolvedTraits["recommendation_directiveness"].Value),
			InterpersonalMode:           deriveInterpersonalMode(state.ResolvedTraits["emotional_attunement"].Value),
		},
		OutputContract: OutputContract{
			StructureLevel:     state.ResolvedTraits["structure_level"].Value,
			ResponseShape:      responseShape,
			SummaryFirst:       summaryFirst,
			PreferredLength:    preferredLength,
			FormattingRules:    formattingRules,
			ProhibitedPatterns: prohibitedPatterns,
		},
		GatesAndClamps: GatesAndClamps{
			HumorAllowed:                   state.GovernanceTrace.AllowHumor,
			ChallengeCap:                   challengeCapFromTrace(state.GovernanceTrace),
			DirectnessCap:                  directnessCapFromTrace(state.GovernanceTrace),
			InitiativeCap:                  state.GovernanceTrace.MaxInitiativeStyle,
			RecommendationDirectivenessCap: state.GovernanceTrace.MaxRecommendationDirectiveness,
			ActiveReasons:                  append([]string(nil), state.GovernanceTrace.Reasons...),
		},
		SerializationHints: SerializationHints{
			PriorityOrder:    []string{"cognitive_modulation", "behavior_policy", "expression_policy", "output_contract"},
			SafeToTrimFirst:  []string{"style_flags", "formatting_rules", "prohibited_patterns"},
			PreserveVerbatim: preserveVerbatim,
		},
		Audit: CompiledAudit{
			DependencyCorrectionsApplied: collectDependencyCorrections(state),
			GatedTraits:                  append([]string(nil), state.Audit.GatedSources...),
			WarningCodes:                 append([]string(nil), state.Audit.WarningCodes...),
		},
	}
	return payload
}

func serializePayload(payload CompiledPersonaPayload) (string, CompiledBudget, CompiledAudit) {
	audit := payload.Audit
	trimLevel := 0
	fragment := renderFragment(payload, trimLevel)
	estimated := estimateTokens(fragment)
	if estimated > 220 {
		trimLevel = 1
		audit.TrimmedFields = appendUniqueStrings(audit.TrimmedFields, "style_flags")
		audit.WarningCodes = appendUniqueStrings(audit.WarningCodes, "budget_trim_required")
		fragment = renderFragment(payload, trimLevel)
		estimated = estimateTokens(fragment)
	}
	if estimated > 320 {
		trimLevel = 2
		audit.TrimmedFields = appendUniqueStrings(audit.TrimmedFields, "formatting_rules", "prohibited_patterns")
		audit.WarningCodes = appendUniqueStrings(audit.WarningCodes, "budget_trim_required")
		fragment = renderFragment(payload, trimLevel)
		estimated = estimateTokens(fragment)
	}
	if estimated > 480 {
		trimLevel = 3
		audit.WarningCodes = appendUniqueStrings(audit.WarningCodes, "fallback_payload_emitted")
		audit.TrimmedFields = appendUniqueStrings(audit.TrimmedFields, "expression_policy", "output_contract")
		fragment = renderFallbackFragment(payload)
		estimated = estimateTokens(fragment)
	}
	budget := CompiledBudget{TargetTokens: 220, HardMaxTokens: 480, EstimatedTokens: estimated, TrimLevel: trimLevel}
	return fragment, budget, audit
}

func renderFragment(payload CompiledPersonaPayload, trimLevel int) string {
	styleFlags := append([]string(nil), payload.ExpressionPolicy.StyleFlags...)
	formattingRules := append([]string(nil), payload.OutputContract.FormattingRules...)
	prohibitedPatterns := append([]string(nil), payload.OutputContract.ProhibitedPatterns...)
	if trimLevel >= 1 {
		styleFlags = nil
	}
	if trimLevel >= 2 {
		formattingRules = nil
		prohibitedPatterns = nil
	}
	var b strings.Builder
	b.WriteString("## Experience Control\n")
	b.WriteString(fmt.Sprintf("role: %s", payload.RoleAdapter.ActiveRole))
	if payload.RoleAdapter.TaskArchetype != "" {
		b.WriteString(fmt.Sprintf(" | task: %s", payload.RoleAdapter.TaskArchetype))
	}
	b.WriteString(fmt.Sprintf(" | delegation: %s", payload.RoleAdapter.DelegationMode))
	b.WriteString(fmt.Sprintf("\nreasoning: %s; uncertainty: %s; challenge: %s.", payload.CognitiveModulation.ReasoningPosture, payload.CognitiveModulation.UncertaintyPosture, payload.CognitiveModulation.ChallengePosture))
	b.WriteString(fmt.Sprintf("\nexpression: directness %.2f, warmth %.2f, formality %.2f, seriousness %.2f, verbosity %.2f, conversationality %.2f, humor %.2f.", payload.ExpressionPolicy.Directness, payload.ExpressionPolicy.Warmth, payload.ExpressionPolicy.Formality, payload.ExpressionPolicy.Seriousness, payload.ExpressionPolicy.Verbosity, payload.ExpressionPolicy.Conversationality, payload.ExpressionPolicy.HumorPlayfulness))
	if len(styleFlags) > 0 {
		b.WriteString(fmt.Sprintf("\nstyle_flags: %s.", strings.Join(styleFlags, ", ")))
	}
	b.WriteString(fmt.Sprintf("\nbehavior: initiative %.2f, ask_vs_infer %s, recommendation %s, interpersonal %s.", payload.BehaviorPolicy.InitiativeStyle, payload.BehaviorPolicy.AskVsInferMode, payload.BehaviorPolicy.RecommendationMode, payload.BehaviorPolicy.InterpersonalMode))
	b.WriteString(fmt.Sprintf("\noutput: shape %s, preferred_length %s, summary_first %t.", payload.OutputContract.ResponseShape, payload.OutputContract.PreferredLength, payload.OutputContract.SummaryFirst))
	if len(formattingRules) > 0 {
		b.WriteString(fmt.Sprintf("\nformatting_rules: %s.", strings.Join(formattingRules, ", ")))
	}
	if len(prohibitedPatterns) > 0 {
		b.WriteString(fmt.Sprintf("\nprohibited_patterns: %s.", strings.Join(prohibitedPatterns, ", ")))
	}
	if len(payload.SerializationHints.PreserveVerbatim) > 0 {
		b.WriteString(fmt.Sprintf("\nactive_turn_constraints: %s.", strings.Join(payload.SerializationHints.PreserveVerbatim, ", ")))
	}
	b.WriteString(fmt.Sprintf("\nconstraints: humor_allowed %t; directness_cap %.2f; initiative_cap %.2f; recommendation_cap %.2f.", payload.GatesAndClamps.HumorAllowed, payload.GatesAndClamps.DirectnessCap, payload.GatesAndClamps.InitiativeCap, payload.GatesAndClamps.RecommendationDirectivenessCap))
	return b.String()
}

func renderFallbackFragment(payload CompiledPersonaPayload) string {
	return fmt.Sprintf("## Experience Control\nrole: %s | delegation: %s\nreasoning: balanced; uncertainty: balanced; challenge: gentle.\noutput: shape structured, preferred_length medium, summary_first false.\nconstraints: humor_allowed %t; directness_cap %.2f; initiative_cap %.2f; recommendation_cap %.2f.", payload.RoleAdapter.ActiveRole, payload.RoleAdapter.DelegationMode, payload.GatesAndClamps.HumorAllowed, payload.GatesAndClamps.DirectnessCap, payload.GatesAndClamps.InitiativeCap, payload.GatesAndClamps.RecommendationDirectivenessCap)
}

func deriveReasoningPosture(value float64) ReasoningPosture {
	if value < 0.35 {
		return ReasoningLightweight
	}
	if value < 0.65 {
		return ReasoningBalanced
	}
	return ReasoningDeep
}

func deriveUncertaintyPosture(value float64) UncertaintyPosture {
	if value < 0.35 {
		return UncertaintyPreserve
	}
	if value < 0.65 {
		return UncertaintyBalanced
	}
	return UncertaintyConverge
}

func deriveChallengePosture(value float64) ChallengePosture {
	if value < 0.35 {
		return ChallengeGentle
	}
	if value < 0.65 {
		return ChallengeBalanced
	}
	return ChallengeForceful
}

func deriveAskVsInferMode(value float64) AskVsInferMode {
	if value < 0.35 {
		return AskEarly
	}
	if value < 0.65 {
		return AskBalanced
	}
	return AskInferSafe
}

func deriveRecommendationMode(value float64) RecommendationMode {
	if value < 0.35 {
		return RecommendOptions
	}
	if value < 0.65 {
		return RecommendWithOptions
	}
	return RecommendClearly
}

func deriveInterpersonalMode(value float64) InterpersonalMode {
	if value < 0.40 {
		return InterpersonalNeutral
	}
	if value < 0.70 {
		return InterpersonalSupportive
	}
	return InterpersonalAttuned
}

func deriveResponseShape(state EffectivePersonaState) ResponseShape {
	if state.OutputPreferences.PreferredFormat != "" {
		switch state.OutputPreferences.PreferredFormat {
		case FormatStepwise:
			return ResponseStepwise
		case FormatStructured:
			return ResponseStructured
		case FormatExecutive:
			return ResponseExecutive
		default:
			return ResponseFreeform
		}
	}
	if state.OutputPreferences.SummaryFirst != nil && *state.OutputPreferences.SummaryFirst {
		return ResponseExecutive
	}
	structure := state.ResolvedTraits["structure_level"].Value
	if structure < 0.30 {
		return ResponseFreeform
	}
	if structure < 0.55 {
		return ResponseAnalysisAnswer
	}
	if structure < 0.75 {
		return ResponseStructured
	}
	return ResponseStepwise
}

func derivePreferredLength(state EffectivePersonaState) PreferredLength {
	if state.OutputPreferences.PreferredLength != "" {
		return state.OutputPreferences.PreferredLength
	}
	verbosity := state.ResolvedTraits["verbosity"].Value
	if verbosity < 0.35 {
		return LengthShort
	}
	if verbosity > 0.70 {
		return LengthLong
	}
	return LengthMedium
}

func deriveStyleFlags(state EffectivePersonaState) []string {
	flags := make([]string, 0, 8)
	traits := state.ResolvedTraits
	if traits["verbosity"].Value < 0.35 {
		flags = append(flags, "concise")
	}
	if traits["verbosity"].Value > 0.70 {
		flags = append(flags, "detailed")
	}
	if traits["formality"].Value < 0.35 {
		flags = append(flags, "plainspoken")
	}
	if traits["formality"].Value > 0.70 {
		flags = append(flags, "polished")
	}
	if traits["warmth"].Value > 0.65 {
		flags = append(flags, "friendly")
	}
	if traits["warmth"].Value < 0.35 {
		flags = append(flags, "reserved")
	}
	if traits["seriousness"].Value > 0.70 {
		flags = append(flags, "sober")
	}
	if traits["humor_playfulness"].Value > 0.40 && !traits["humor_playfulness"].GatedOff {
		flags = append(flags, "playful")
	}
	return flags
}

func deriveFormattingRules(shape ResponseShape, summaryFirst bool) []string {
	rules := []string{"use_code_blocks_for_code"}
	switch shape {
	case ResponseStructured, ResponseAnalysisAnswer, ResponseExecutive, ResponseStepwise:
		rules = append(rules, "use_headers_for_complexity", "use_bullets_for_lists")
	}
	if shape == ResponseStepwise {
		rules = append(rules, "use_stepwise_for_instructions")
	}
	if summaryFirst {
		rules = append(rules, "lead_with_summary")
	}
	return rules
}

func deriveProhibitedPatterns(state EffectivePersonaState) []string {
	patterns := make([]string, 0, 4)
	if !state.GovernanceTrace.AllowHumor {
		patterns = append(patterns, "no_humor_in_high_stakes")
	}
	if state.LiveContextTrace.StakesLevel > 0.65 {
		patterns = append(patterns, "no_unsolicited_opinions")
	}
	if state.ResolvedTraits["decisiveness"].Value >= 0.55 {
		patterns = append(patterns, "no_excessive_hedging")
	}
	if state.ResolvedTraits["supportiveness"].Value < 0.45 {
		patterns = append(patterns, "no_filler_reassurance")
	}
	return patterns
}

func activeTurnConstraints(trace ExplicitTurnTrace) []string {
	out := make([]string, 0, 4)
	if trace.MustNotUseHumor {
		out = append(out, "must_not_use_humor")
	}
	if trace.MustBeBrief {
		out = append(out, "must_be_brief")
	}
	if trace.MustAskClarifyingQuestion {
		out = append(out, "must_ask_clarifying_question")
	}
	if trace.MustBeHighlyStructured {
		out = append(out, "must_be_highly_structured")
	}
	return out
}

func collectDependencyCorrections(state EffectivePersonaState) []string {
	codes := make([]string, 0)
	for _, trait := range traitOrder {
		codes = appendUniqueStrings(codes, state.ResolvedTraits[trait].DependencyCorrectionsApplied...)
	}
	return codes
}

func canonicalStateShape(state EffectivePersonaState) any {
	return struct {
		RoleContext           RoleContext                `json:"role_context"`
		ResolvedTraits        map[string]TraitValueState `json:"resolved_traits"`
		GovernanceTrace       GovernanceTrace            `json:"governance_trace"`
		LiveContextTrace      LiveContextTrace           `json:"live_context_trace"`
		ExplicitTurnOverrides ExplicitTurnTrace          `json:"explicit_turn_overrides"`
		OutputPreferences     OutputPreferences          `json:"output_preferences"`
	}{
		RoleContext:           state.RoleContext,
		ResolvedTraits:        state.ResolvedTraits,
		GovernanceTrace:       state.GovernanceTrace,
		LiveContextTrace:      state.LiveContextTrace,
		ExplicitTurnOverrides: state.ExplicitTurnOverrides,
		OutputPreferences:     state.OutputPreferences,
	}
}

func hashCanonical(v any) string {
	data, _ := json.Marshal(v)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func estimateTokens(text string) int {
	wordEstimate := int(math.Ceil(float64(len(strings.Fields(text))) * 1.3))
	charEstimate := int(math.Ceil(float64(len(text)) / 4.0))
	if charEstimate > wordEstimate {
		return charEstimate
	}
	return wordEstimate
}

func moduleScopeRank(scope ModuleScope) int {
	switch scope {
	case ModuleScopeTurn:
		return 4
	case ModuleScopeTask:
		return 3
	case ModuleScopeConversation:
		return 2
	default:
		return 1
	}
}

func originRank(source, scope string) int {
	if source == "request" {
		return 3
	}
	if scope == "owner" {
		return 2
	}
	return 1 // system or default
}

func keywordScore(content string, terms []string, weight float64) float64 {
	score := 0.0
	for _, term := range terms {
		if strings.Contains(content, term) {
			score += weight
		}
	}
	return score
}

func containsAny(content string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(content, term) {
			return true
		}
	}
	return false
}

func challengeCap(bounds GovernanceBounds) float64 {
	value := 1.0
	for _, cap := range bounds.TraitCaps {
		if cap.Trait == "challenge_intensity" {
			value = minFloat(value, cap.MaxValue)
		}
	}
	return value
}

func directnessCap(bounds GovernanceBounds) float64 {
	value := 1.0
	for _, cap := range bounds.TraitCaps {
		if cap.Trait == "directness" {
			value = minFloat(value, cap.MaxValue)
		}
	}
	return value
}

func maxInitiativeCap(bounds GovernanceBounds) float64 {
	value := 1.0
	for _, cap := range bounds.TraitCaps {
		if cap.Trait == "initiative_style" {
			value = minFloat(value, cap.MaxValue)
		}
	}
	return value
}

func maxRecommendationCap(bounds GovernanceBounds) float64 {
	value := 1.0
	for _, cap := range bounds.TraitCaps {
		if cap.Trait == "recommendation_directiveness" {
			value = minFloat(value, cap.MaxValue)
		}
	}
	return value
}

func challengeCapFromTrace(trace GovernanceTrace) float64 {
	for _, constraint := range trace.AdditionalConstraints {
		if constraint.Trait == "challenge_intensity" && constraint.ConstraintType == "cap" {
			return constraint.Value
		}
	}
	return 1.0
}

func directnessCapFromTrace(trace GovernanceTrace) float64 {
	for _, constraint := range trace.AdditionalConstraints {
		if constraint.Trait == "directness" && constraint.ConstraintType == "cap" {
			return constraint.Value
		}
	}
	return 1.0
}

func existingOr(values map[string]float64, key string, fallback float64) float64 {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func clamp01(value float64) float64 {
	return clamp(value, 0, 1)
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base))
	for _, value := range base {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		base = append(base, value)
		seen[value] = struct{}{}
	}
	return base
}
