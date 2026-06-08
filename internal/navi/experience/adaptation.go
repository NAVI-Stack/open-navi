package experience

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

type AdaptationDelta struct {
	Trait          string
	PreviousValue  float64
	NewValue       float64
	SignalCount    int
	Threshold      float64
	Applied        bool
	Persisted      bool
	ProposalWorthy bool
}

type traitSignalAggregate struct {
	Count             int
	StrengthSum       float64
	TargetWeightedSum float64
	ExplicitCount     int
	LatestSignalAt    time.Time
}

func DetectPreferenceSignals(content, ownerID, chatID string) []schema.PreferenceSignal {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return nil
	}
	positiveQualifier := containsAny(lower, "exactly", "that's better", "this works", "that works", "works", "worked", "perfect", "nailed it", "keep it like this")
	scope := ConfigScopeOwner
	scopeID := strings.TrimSpace(ownerID)
	if scopeID == "" {
		scope = "session"
		scopeID = strings.TrimSpace(chatID)
	}
	dedupe := map[string]schema.PreferenceSignal{}
	add := func(trait string, target float64, evidence schema.PreferenceEvidenceClass, strength float64, summary string, immediate bool) {
		if _, ok := traitDefinitions[trait]; !ok {
			return
		}
		key := strings.Join([]string{trait, string(evidence), summary}, ":")
		dedupe[key] = schema.PreferenceSignal{
			Trait:          trait,
			TargetValue:    clamp01(target),
			EvidenceClass:  evidence,
			SignalStrength: clamp01(strength),
			Scope:          scope,
			ScopeID:        scopeID,
			ChatID:         strings.TrimSpace(chatID),
			Summary:        summary,
			Immediate:      immediate,
		}
	}

	if !positiveQualifier && containsAny(lower, "be brief", "be concise", "short answer", "quick answer", "keep it short", "keep it concise") {
		add("verbosity", 0.22, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested more concise replies", true)
	}
	if !positiveQualifier && containsAny(lower, "step by step", "step-by-step", "walk me through", "break it down") {
		add("structure_level", 0.80, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested step-by-step structure", true)
	}
	if !positiveQualifier && containsAny(lower, "more structured", "be structured", "use bullets", "outline") {
		add("structure_level", 0.68, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested a more structured response", true)
	}
	if !positiveQualifier && containsAny(lower, "be direct", "be blunt", "just tell me", "tell me clearly") {
		add("directness", 0.74, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested more direct communication", true)
		add("recommendation_directiveness", 0.66, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested clearer recommendations", true)
	}
	if !positiveQualifier && containsAny(lower, "gentle", "gently", "softer", "soften") {
		add("directness", 0.56, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested gentler communication", true)
		add("warmth", 0.66, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested a warmer tone", true)
	}
	if !positiveQualifier && containsAny(lower, "formal", "professional tone") {
		add("formality", 0.72, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested a more formal tone", true)
	}
	if !positiveQualifier && containsAny(lower, "casual", "informal") {
		add("formality", 0.36, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested a less formal tone", true)
		add("conversationality", 0.62, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested a more conversational tone", true)
	}
	if !positiveQualifier && (containsAny(lower, "serious", "keep it serious") || containsAny(lower, "stop using humor", "don't joke", "do not joke")) {
		add("humor_playfulness", 0.0, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested less humor", true)
	}
	if !positiveQualifier && containsAny(lower, "lighter", "more playful", "more levity") {
		add("humor_playfulness", 0.30, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested more levity", true)
	}
	if !positiveQualifier && containsAny(lower, "ask clarifying", "ask questions first") {
		add("clarification_threshold", 0.25, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested more clarifying questions", true)
	}
	if !positiveQualifier && containsAny(lower, "don't ask questions", "do not ask questions", "infer when safe") {
		add("clarification_threshold", 0.70, schema.PreferenceEvidenceExplicitCorrection, 1.0, "User requested fewer clarifying questions", true)
	}

	repeatedQualifier := containsAny(lower, "again", "still", "same as before", "like last time", "every time", "keep doing")
	if repeatedQualifier && containsAny(lower, "brief", "concise", "short") {
		add("verbosity", 0.22, schema.PreferenceEvidenceRepeatedBehavior, 0.55, "User repeated a request for concise replies", false)
	}
	if repeatedQualifier && containsAny(lower, "structured", "bullets", "step by step", "step-by-step") {
		add("structure_level", 0.72, schema.PreferenceEvidenceRepeatedBehavior, 0.55, "User repeated a request for structured output", false)
	}
	if repeatedQualifier && containsAny(lower, "direct", "clear recommendation", "tell me clearly") {
		add("recommendation_directiveness", 0.64, schema.PreferenceEvidenceRepeatedBehavior, 0.55, "User repeated a request for direct recommendations", false)
	}

	if containsAny(lower, "too long", "too verbose", "rambling", "wall of text") {
		add("verbosity", 0.22, schema.PreferenceEvidenceAnomaly, 0.35, "User reacted negatively to overly long replies", false)
	}
	if containsAny(lower, "too blunt", "too harsh", "came off harsh", "that was harsh") {
		add("directness", 0.56, schema.PreferenceEvidenceAnomaly, 0.35, "User reacted negatively to blunt phrasing", false)
		add("warmth", 0.66, schema.PreferenceEvidenceAnomaly, 0.35, "User reacted negatively to low-warmth phrasing", false)
	}
	if containsAny(lower, "too formal", "too stiff") {
		add("formality", 0.36, schema.PreferenceEvidenceAnomaly, 0.30, "User reacted negatively to overly formal phrasing", false)
	}
	if containsAny(lower, "too casual", "too informal") {
		add("formality", 0.72, schema.PreferenceEvidenceAnomaly, 0.30, "User reacted negatively to overly casual phrasing", false)
	}
	if containsAny(lower, "too many questions", "stop asking so many questions") {
		add("clarification_threshold", 0.70, schema.PreferenceEvidenceAnomaly, 0.35, "User reacted negatively to too many clarifying questions", false)
	}
	if containsAny(lower, "too many bullets", "too structured", "less structure") {
		add("structure_level", 0.44, schema.PreferenceEvidenceAnomaly, 0.30, "User reacted negatively to overly structured output", false)
	}

	if positiveQualifier && containsAny(lower, "brief", "concise", "short") {
		add("verbosity", 0.22, schema.PreferenceEvidenceSilentWin, 0.20, "User positively reinforced concise output", false)
	}
	if positiveQualifier && containsAny(lower, "structured", "bullets", "step by step", "format") {
		add("structure_level", 0.72, schema.PreferenceEvidenceSilentWin, 0.20, "User positively reinforced structured output", false)
	}
	if positiveQualifier && containsAny(lower, "direct", "clear", "straightforward") {
		add("recommendation_directiveness", 0.62, schema.PreferenceEvidenceSilentWin, 0.20, "User positively reinforced direct guidance", false)
	}
	if positiveQualifier && containsAny(lower, "warmer", "gentle", "tone") {
		add("warmth", 0.66, schema.PreferenceEvidenceSilentWin, 0.20, "User positively reinforced warmer tone", false)
	}

	if len(dedupe) == 0 {
		return nil
	}
	keys := make([]string, 0, len(dedupe))
	for key := range dedupe {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]schema.PreferenceSignal, 0, len(keys))
	for _, key := range keys {
		out = append(out, dedupe[key])
	}
	return out
}

func BuildSessionPreferenceOverrides(signals []schema.PreferenceSignal) (ExplicitTurnInput, ExplicitTurnTrace, OutputPreferences) {
	filtered := make([]schema.PreferenceSignal, 0, len(signals))
	for _, signal := range signals {
		trait := strings.TrimSpace(signal.Trait)
		if !signal.Immediate || signal.EvidenceClass != schema.PreferenceEvidenceExplicitCorrection {
			continue
		}
		if _, ok := traitDefinitions[trait]; !ok {
			continue
		}
		filtered = append(filtered, signal)
	}
	if len(filtered) == 0 {
		return ExplicitTurnInput{}, ExplicitTurnTrace{}, OutputPreferences{}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := filtered[i].CreatedAt
		right := filtered[j].CreatedAt
		if left.Equal(right) {
			return filtered[i].SignalID < filtered[j].SignalID
		}
		if left.IsZero() {
			return true
		}
		if right.IsZero() {
			return false
		}
		return left.Before(right)
	})

	input := ExplicitTurnInput{TraitOverrides: map[string]float64{}}
	latestByTrait := make(map[string]schema.PreferenceSignal, len(filtered))
	for _, signal := range filtered {
		trait := strings.TrimSpace(signal.Trait)
		target := clamp01(signal.TargetValue)
		input.TraitOverrides[trait] = target
		latestByTrait[trait] = signal
	}
	if len(input.TraitOverrides) == 0 {
		input.TraitOverrides = nil
	}
	trace := ExplicitTurnTrace{}
	prefs := OutputPreferences{}
	latestSignals := make([]schema.PreferenceSignal, 0, len(latestByTrait))
	for _, signal := range latestByTrait {
		latestSignals = append(latestSignals, signal)
	}
	sort.SliceStable(latestSignals, func(i, j int) bool {
		left := latestSignals[i].CreatedAt
		right := latestSignals[j].CreatedAt
		if left.Equal(right) {
			return latestSignals[i].SignalID < latestSignals[j].SignalID
		}
		if left.IsZero() {
			return true
		}
		if right.IsZero() {
			return false
		}
		return left.Before(right)
	})
	for _, signal := range latestSignals {
		applyImmediateSignalHints(&trace, &prefs, signal.Trait, input.TraitOverrides[signal.Trait])
	}
	return input, trace, prefs
}

func applyImmediateSignalHints(trace *ExplicitTurnTrace, prefs *OutputPreferences, trait string, target float64) {
	target = clamp01(target)
	switch trait {
	case "verbosity":
		if target <= 0.35 {
			trace.MustBeBrief = true
			prefs.PreferredLength = LengthShort
		} else if target >= 0.65 {
			prefs.PreferredLength = LengthLong
		}
	case "structure_level":
		if target >= 0.65 {
			trace.MustBeHighlyStructured = true
			prefs.PreferredFormat = FormatStructured
		}
		if target >= 0.80 {
			prefs.PreferredFormat = FormatStepwise
		}
	case "humor_playfulness":
		if target <= 0.05 {
			trace.MustNotUseHumor = true
		}
	case "clarification_threshold":
		if target <= 0.35 {
			trace.MustAskClarifyingQuestion = true
		}
	case "directness":
		if target >= traitDefinitions[trait].DefaultValue {
			trace.UserRequestedToneShift = ToneMoreDirect
		} else {
			trace.UserRequestedToneShift = ToneGentler
		}
	case "warmth":
		if target >= traitDefinitions[trait].DefaultValue {
			trace.UserRequestedToneShift = ToneWarmer
		} else {
			trace.UserRequestedToneShift = ToneCooler
		}
	case "formality":
		if target >= traitDefinitions[trait].DefaultValue {
			trace.UserRequestedToneShift = ToneMoreFormal
		} else {
			trace.UserRequestedToneShift = ToneMoreCasual
		}
	case "seriousness":
		if target >= traitDefinitions[trait].DefaultValue {
			trace.UserRequestedToneShift = ToneMoreSerious
		} else {
			trace.UserRequestedToneShift = ToneLighter
		}
	}
}

func UpdateRelationshipProfile(existing RelationshipProfile, signals []schema.PreferenceSignal, now time.Time) (RelationshipProfile, []AdaptationDelta) {
	profile := normalizeRelationship(existing)
	if profile.TraitSignalCount == nil {
		profile.TraitSignalCount = map[string]int{}
	}
	if profile.TraitAverageStrength == nil {
		profile.TraitAverageStrength = map[string]float64{}
	}
	if profile.TraitLastSignalAt == nil {
		profile.TraitLastSignalAt = map[string]string{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	removeStableTraitEstimates(&profile)
	decayRelationshipProfile(&profile, now)

	aggregates := map[string]*traitSignalAggregate{}
	for _, signal := range signals {
		trait := strings.TrimSpace(signal.Trait)
		if _, ok := traitDefinitions[trait]; !ok {
			continue
		}
		agg := aggregates[trait]
		if agg == nil {
			agg = &traitSignalAggregate{}
			aggregates[trait] = agg
		}
		agg.Count++
		strength := clamp01(signal.SignalStrength)
		agg.StrengthSum += strength
		agg.TargetWeightedSum += clamp01(signal.TargetValue) * strength
		if signal.EvidenceClass == schema.PreferenceEvidenceExplicitCorrection {
			agg.ExplicitCount++
		}
		createdAt := signal.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		if createdAt.After(agg.LatestSignalAt) {
			agg.LatestSignalAt = createdAt
		}
	}

	coldStartMultiplier := 1.0
	if profile.Confidence < ColdStartConfidenceFloor {
		coldStartMultiplier = ColdStartLearningMultiplier
	}

	deltas := make([]AdaptationDelta, 0, len(aggregates))
	for trait, agg := range aggregates {
		definition := traitDefinitions[trait]
		previousValue := relationshipTraitValue(profile, trait)
		previousCount := profile.TraitSignalCount[trait]
		previousAvgStrength := profile.TraitAverageStrength[trait]
		newCount := previousCount + agg.Count
		avgStrength := agg.StrengthSum / math.Max(1, float64(agg.Count))
		avgTarget := agg.TargetWeightedSum / math.Max(agg.StrengthSum, 1e-9)
		learningRate := averageLearningRateForSignals(trait, signals)
		applied := newCount >= SignalApplyThreshold
		persisted := newCount >= SignalPersistThreshold
		newValue := previousValue
		proposalValue := previousValue

		if definition.AdaptivityClass == AdaptivityStable {
			proposalValue = avgTarget
			delete(profile.TraitEstimates, trait)
		} else if applied {
			newValue = clamp01(previousValue + ((avgTarget - previousValue) * learningRate * float64(agg.Count) * coldStartMultiplier))
			proposalValue = newValue
			if math.Abs(newValue-definition.DefaultValue) < 1e-6 {
				delete(profile.TraitEstimates, trait)
			} else {
				profile.TraitEstimates[trait] = newValue
			}
		}

		profile.TraitSignalCount[trait] = newCount
		profile.TraitAverageStrength[trait] = weightedAverage(previousAvgStrength, float64(previousCount), avgStrength, float64(agg.Count))
		latest := agg.LatestSignalAt
		if latest.IsZero() {
			latest = now
		}
		profile.TraitLastSignalAt[trait] = latest.UTC().Format(time.RFC3339)
		profile.TotalSignalCount += agg.Count
		profile.ExplicitSignalCount += agg.ExplicitCount

		threshold, proposalWorthy := proposalThresholdForTrait(trait, math.Abs(proposalValue-previousValue), newCount)
		deltas = append(deltas, AdaptationDelta{
			Trait:          trait,
			PreviousValue:  previousValue,
			NewValue:       proposalValue,
			SignalCount:    newCount,
			Threshold:      threshold,
			Applied:        applied,
			Persisted:      persisted,
			ProposalWorthy: proposalWorthy,
		})
	}

	recomputeRelationshipConfidence(&profile, now)
	return profile, deltas
}

func removeStableTraitEstimates(profile *RelationshipProfile) {
	for trait, definition := range traitDefinitions {
		if definition.AdaptivityClass != AdaptivityStable {
			continue
		}
		delete(profile.TraitEstimates, trait)
	}
}

func decayRelationshipProfile(profile *RelationshipProfile, now time.Time) {
	for trait, value := range profile.TraitEstimates {
		definition, ok := traitDefinitions[trait]
		if !ok {
			continue
		}
		if definition.AdaptivityClass == AdaptivityStable {
			delete(profile.TraitEstimates, trait)
			continue
		}
		raw := strings.TrimSpace(profile.TraitLastSignalAt[trait])
		if raw == "" {
			continue
		}
		lastSignalAt, err := time.Parse(time.RFC3339, raw)
		if err != nil || now.Before(lastSignalAt) {
			continue
		}
		cycles := now.Sub(lastSignalAt).Hours() / 24.0
		if cycles <= 0 {
			continue
		}
		decayed := decayTowardDefault(value, definition.DefaultValue, cycles)
		if math.Abs(decayed-definition.DefaultValue) < 1e-6 {
			delete(profile.TraitEstimates, trait)
			continue
		}
		profile.TraitEstimates[trait] = decayed
	}
}

func decayTowardDefault(value, defaultValue, cycles float64) float64 {
	if cycles <= 0 {
		return clamp01(value)
	}
	factor := math.Min(1.0, DecayRate*cycles)
	return clamp01(value + ((defaultValue - value) * factor))
}

func recomputeRelationshipConfidence(profile *RelationshipProfile, now time.Time) {
	traitsWithSignals := 0
	for _, count := range profile.TraitSignalCount {
		if count > 0 {
			traitsWithSignals++
		}
	}
	explicitRatio := 0.0
	if profile.TotalSignalCount > 0 {
		explicitRatio = float64(profile.ExplicitSignalCount) / float64(profile.TotalSignalCount)
	}
	signalDiversity := 0.0
	if len(traitDefinitions) > 0 {
		signalDiversity = float64(traitsWithSignals) / float64(len(traitDefinitions))
	}
	updateDensity := math.Min(1.0, float64(profile.TotalSignalCount)/30.0)
	profile.Confidence = math.Min(1.0, (signalDiversity*0.40)+(updateDensity*0.35)+(explicitRatio*0.25))

	if profile.PerTraitConfidence == nil {
		profile.PerTraitConfidence = map[string]float64{}
	}
	for trait, count := range profile.TraitSignalCount {
		recencyWeight := 1.0
		if raw := strings.TrimSpace(profile.TraitLastSignalAt[trait]); raw != "" {
			if ts, err := time.Parse(time.RFC3339, raw); err == nil {
				cycles := math.Max(0, now.Sub(ts).Hours()/24.0)
				recencyWeight = math.Max(0.10, 1.0-(DecayRate*cycles))
			}
		}
		avgStrength := profile.TraitAverageStrength[trait]
		profile.PerTraitConfidence[trait] = math.Min(1.0, float64(count)*avgStrength*recencyWeight)
	}
}

func relationshipTraitValue(profile RelationshipProfile, trait string) float64 {
	if value, ok := profile.TraitEstimates[trait]; ok {
		return clamp01(value)
	}
	if definition, ok := traitDefinitions[trait]; ok {
		return definition.DefaultValue
	}
	return 0
}

func averageLearningRateForSignals(trait string, signals []schema.PreferenceSignal) float64 {
	total := 0.0
	count := 0.0
	for _, signal := range signals {
		if signal.Trait != trait {
			continue
		}
		total += learningRateForEvidenceClass(signal.EvidenceClass)
		count++
	}
	if count == 0 {
		return LearningRateAnomaly
	}
	return total / count
}

func learningRateForEvidenceClass(class schema.PreferenceEvidenceClass) float64 {
	switch class {
	case schema.PreferenceEvidenceExplicitCorrection:
		return LearningRateExplicit
	case schema.PreferenceEvidenceRepeatedBehavior:
		return LearningRatePattern
	case schema.PreferenceEvidenceSilentWin:
		return LearningRateSilentWin
	default:
		return LearningRateAnomaly
	}
}

func proposalThresholdForTrait(trait string, delta float64, signalCount int) (float64, bool) {
	definition, ok := traitDefinitions[trait]
	if !ok || signalCount < SignalPersistThreshold {
		return 0, false
	}
	switch definition.AdaptivityClass {
	case AdaptivityStable:
		return ProposalSignificanceStable, delta >= ProposalSignificanceStable
	case AdaptivitySemiAdaptive:
		return ProposalSignificanceSemi, delta >= ProposalSignificanceSemi
	default:
		return 0, false
	}
}

func weightedAverage(existing float64, existingCount float64, next float64, nextCount float64) float64 {
	total := existingCount + nextCount
	if total <= 0 {
		return next
	}
	return ((existing * existingCount) + (next * nextCount)) / total
}
