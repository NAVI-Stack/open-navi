package experience

// TraitDomain is the canonical v1 experience-trait domain.
type TraitDomain string

const (
	DomainIdentity   TraitDomain = "identity"
	DomainCognitive  TraitDomain = "cognitive"
	DomainExpression TraitDomain = "expression"
	DomainBehavioral TraitDomain = "behavioral"
	DomainRelational TraitDomain = "relational"
	DomainOutput     TraitDomain = "output"
)

// MergeClass is the canonical merge behavior for a trait.
type MergeClass string

const (
	MergeWeightedBlend    MergeClass = "weighted_blend"
	MergePriorityOverride MergeClass = "priority_override"
	MergeRangeClamp       MergeClass = "range_clamp"
	MergeGatedActivation  MergeClass = "gated_activation"
)

// AdaptivityClass indicates how much a trait can adapt over time.
type AdaptivityClass string

const (
	AdaptivityStable         AdaptivityClass = "stable"
	AdaptivitySemiAdaptive   AdaptivityClass = "semi_adaptive"
	AdaptivityHighlyAdaptive AdaptivityClass = "highly_adaptive"
)

// ActiveRole is the role adapter's active role.
type ActiveRole string

const (
	RoleCharacter ActiveRole = "Character"
	RoleAssistant ActiveRole = "Assistant"
	RoleCoder     ActiveRole = "Coder"
)

// DelegationMode indicates whether the current turn is delegated.
type DelegationMode string

const (
	DelegationNone     DelegationMode = "none"
	DelegationPrimary  DelegationMode = "primary"
	DelegationSubagent DelegationMode = "subagent"
)

// ModuleScope is the scope of a persona module.
type ModuleScope string

const (
	ModuleScopeGlobal       ModuleScope = "global"
	ModuleScopeConversation ModuleScope = "conversation"
	ModuleScopeTask         ModuleScope = "task"
	ModuleScopeTurn         ModuleScope = "turn"
)

// ModuleKind describes the shape of a stored experience module.
type ModuleKind string

const (
	ModuleKindBundle  ModuleKind = "bundle"
	ModuleKindMicro   ModuleKind = "micro_module"
	ModuleKindOverlay ModuleKind = "overlay"
)

// PreferredLength is the response length preference.
type PreferredLength string

const (
	LengthShort  PreferredLength = "short"
	LengthMedium PreferredLength = "medium"
	LengthLong   PreferredLength = "long"
)

// PreferredFormat is the response format preference.
type PreferredFormat string

const (
	FormatFreeform   PreferredFormat = "freeform"
	FormatStructured PreferredFormat = "structured"
	FormatStepwise   PreferredFormat = "stepwise"
	FormatExecutive  PreferredFormat = "executive"
)

func IsSupportedTrait(name string) bool {
	_, ok := traitDefinitions[name]
	return ok
}

func IsSupportedModuleScope(scope ModuleScope) bool {
	switch scope {
	case ModuleScopeGlobal, ModuleScopeConversation, ModuleScopeTask, ModuleScopeTurn:
		return true
	default:
		return false
	}
}

func IsSupportedModuleKind(kind ModuleKind) bool {
	switch kind {
	case ModuleKindBundle, ModuleKindMicro, ModuleKindOverlay:
		return true
	default:
		return false
	}
}

func IsSupportedPreferredLength(length PreferredLength) bool {
	switch length {
	case LengthShort, LengthMedium, LengthLong:
		return true
	default:
		return false
	}
}

func IsSupportedPreferredFormat(format PreferredFormat) bool {
	switch format {
	case FormatFreeform, FormatStructured, FormatStepwise, FormatExecutive:
		return true
	default:
		return false
	}
}

// ResponseShape is the compiled output response shape.
type ResponseShape string

const (
	ResponseFreeform       ResponseShape = "freeform"
	ResponseStructured     ResponseShape = "structured"
	ResponseStepwise       ResponseShape = "stepwise"
	ResponseExecutive      ResponseShape = "executive_summary"
	ResponseAnalysisAnswer ResponseShape = "analysis_then_answer"
)

// ReasoningPosture is the compiled reasoning posture.
type ReasoningPosture string

const (
	ReasoningLightweight ReasoningPosture = "lightweight"
	ReasoningBalanced    ReasoningPosture = "balanced"
	ReasoningDeep        ReasoningPosture = "deep"
)

// UncertaintyPosture is the compiled uncertainty posture.
type UncertaintyPosture string

const (
	UncertaintyPreserve UncertaintyPosture = "preserve_ambiguity"
	UncertaintyBalanced UncertaintyPosture = "balanced"
	UncertaintyConverge UncertaintyPosture = "converge_when_ready"
)

// ChallengePosture is the compiled challenge posture.
type ChallengePosture string

const (
	ChallengeGentle   ChallengePosture = "gentle"
	ChallengeBalanced ChallengePosture = "balanced"
	ChallengeForceful ChallengePosture = "forceful"
)

// AskVsInferMode is the compiled clarification mode.
type AskVsInferMode string

const (
	AskEarly     AskVsInferMode = "ask_early"
	AskBalanced  AskVsInferMode = "balanced"
	AskInferSafe AskVsInferMode = "infer_when_safe"
)

// RecommendationMode is the compiled recommendation mode.
type RecommendationMode string

const (
	RecommendOptions     RecommendationMode = "present_options"
	RecommendWithOptions RecommendationMode = "recommend_with_options"
	RecommendClearly     RecommendationMode = "recommend_clearly"
)

// InterpersonalMode is the compiled interpersonal mode.
type InterpersonalMode string

const (
	InterpersonalNeutral    InterpersonalMode = "neutral"
	InterpersonalSupportive InterpersonalMode = "supportive"
	InterpersonalAttuned    InterpersonalMode = "high_attunement"
)

// ToneShift is an explicit user-requested tone shift.
type ToneShift string

const (
	ToneWarmer      ToneShift = "warmer"
	ToneCooler      ToneShift = "cooler"
	ToneMoreDirect  ToneShift = "more_direct"
	ToneGentler     ToneShift = "gentler"
	ToneMoreFormal  ToneShift = "more_formal"
	ToneMoreCasual  ToneShift = "more_casual"
	ToneMoreSerious ToneShift = "more_serious"
	ToneLighter     ToneShift = "lighter"
)

// TraitDefinition is the canonical trait definition metadata.
type TraitDefinition struct {
	Name               string
	Domain             TraitDomain
	DefaultValue       float64
	AdaptivityClass    AdaptivityClass
	MergeClass         MergeClass
	DependencyPartners []string
	BehaviorLow        string
	BehaviorHigh       string
}

// TraitValueState is the resolved state for a single trait.
type TraitValueState struct {
	Value                        float64         `json:"value"`
	Domain                       TraitDomain     `json:"domain"`
	MergeClass                   MergeClass      `json:"merge_class"`
	AdaptivityClass              AdaptivityClass `json:"adaptivity_class"`
	GatedOff                     bool            `json:"gated_off"`
	ClampApplied                 bool            `json:"clamp_applied"`
	ClampRange                   *ClampRange     `json:"clamp_range,omitempty"`
	DependencyCorrectionsApplied []string        `json:"dependency_corrections_applied"`
}

// ClampRange captures the final active clamp range.
type ClampRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// GovernanceCap is a trait cap from governance.
type GovernanceCap struct {
	Trait    string  `json:"trait" yaml:"trait"`
	MaxValue float64 `json:"max_value" yaml:"max_value"`
	Reason   string  `json:"reason" yaml:"reason"`
}

// GovernanceFloor is a trait floor from governance.
type GovernanceFloor struct {
	Trait    string  `json:"trait" yaml:"trait"`
	MinValue float64 `json:"min_value" yaml:"min_value"`
	Reason   string  `json:"reason" yaml:"reason"`
}

// GovernanceGate is a trait gate from governance.
type GovernanceGate struct {
	Trait      string `json:"trait" yaml:"trait"`
	GateActive bool   `json:"gate_active" yaml:"gate_active"`
	Reason     string `json:"reason" yaml:"reason"`
}

// GovernanceContextFlags describe governance-relevant context.
type GovernanceContextFlags struct {
	HighStakes           bool `json:"high_stakes" yaml:"high_stakes"`
	EmotionallySensitive bool `json:"emotionally_sensitive" yaml:"emotionally_sensitive"`
	RequiresNeutrality   bool `json:"requires_neutrality" yaml:"requires_neutrality"`
}

// GovernanceBounds is the merge-engine governance input.
type GovernanceBounds struct {
	TraitCaps    []GovernanceCap        `json:"trait_caps"`
	TraitFloors  []GovernanceFloor      `json:"trait_floors"`
	TraitGates   []GovernanceGate       `json:"trait_gates"`
	ContextFlags GovernanceContextFlags `json:"context_flags"`
}

// RoleContext describes the active role adapter state.
type RoleContext struct {
	ActiveRole     ActiveRole     `json:"active_role" yaml:"active_role"`
	TaskArchetype  string         `json:"task_archetype" yaml:"task_archetype"`
	DelegationMode DelegationMode `json:"delegation_mode" yaml:"delegation_mode"`
}

// ModuleConfig is a reusable persona module definition.
type ModuleConfig struct {
	ModuleID           string             `json:"module_id" yaml:"module_id"`
	Version            string             `json:"version,omitempty" yaml:"version,omitempty"`
	Kind               ModuleKind         `json:"kind,omitempty" yaml:"kind,omitempty"`
	Scope              ModuleScope        `json:"scope" yaml:"scope"`
	OriginScope        string             `json:"origin_scope,omitempty" yaml:"origin_scope,omitempty"`
	Source             string             `json:"source,omitempty" yaml:"source,omitempty"`
	IsLocked           bool               `json:"is_locked,omitempty" yaml:"is_locked,omitempty"`
	TraitContributions map[string]float64 `json:"trait_contributions" yaml:"trait_contributions"`
	Strength           float64            `json:"strength" yaml:"strength"`
}

// ModuleRegistryEntry is the queryable registry shape for stored experience modules.
type ModuleRegistryEntry struct {
	ModuleID           string             `json:"module_id"`
	Version            string             `json:"version"`
	Kind               ModuleKind         `json:"kind"`
	Scope              ModuleScope        `json:"scope"`
	ScopeID            string             `json:"scope_id"`
	Source             string             `json:"source"`
	Strength           float64            `json:"strength"`
	TraitContributions map[string]float64 `json:"trait_contributions"`
}

// RelationshipProfile is the current derived relationship profile.
type RelationshipProfile struct {
	TraitEstimates       map[string]float64 `json:"trait_estimates" yaml:"trait_estimates"`
	Confidence           float64            `json:"confidence" yaml:"confidence"`
	PerTraitConfidence   map[string]float64 `json:"per_trait_confidence" yaml:"per_trait_confidence"`
	TotalSignalCount     int                `json:"total_signal_count,omitempty" yaml:"total_signal_count,omitempty"`
	ExplicitSignalCount  int                `json:"explicit_signal_count,omitempty" yaml:"explicit_signal_count,omitempty"`
	TraitSignalCount     map[string]int     `json:"trait_signal_count,omitempty" yaml:"trait_signal_count,omitempty"`
	TraitAverageStrength map[string]float64 `json:"trait_average_strength,omitempty" yaml:"trait_average_strength,omitempty"`
	TraitLastSignalAt    map[string]string  `json:"trait_last_signal_at,omitempty" yaml:"trait_last_signal_at,omitempty"`
}

// LiveContext is the current live-context modulator state.
type LiveContext struct {
	AmbiguityLevel     float64 `json:"ambiguity_level"`
	UrgencyLevel       float64 `json:"urgency_level"`
	EmotionalIntensity float64 `json:"emotional_intensity"`
	StakesLevel        float64 `json:"stakes_level"`
	ComplexityLevel    float64 `json:"complexity_level"`
}

// ExplicitTurnInput is the merge-engine turn override input.
type ExplicitTurnInput struct {
	TraitOverrides map[string]float64 `json:"trait_overrides"`
	TraitCaps      map[string]float64 `json:"trait_caps"`
	TraitFloors    map[string]float64 `json:"trait_floors"`
}

// OutputPreferences are owner- or turn-level output preferences.
type OutputPreferences struct {
	PreferredLength PreferredLength `json:"preferred_length,omitempty" yaml:"preferred_length,omitempty"`
	PreferredFormat PreferredFormat `json:"preferred_format,omitempty" yaml:"preferred_format,omitempty"`
	SummaryFirst    *bool           `json:"summary_first,omitempty" yaml:"summary_first,omitempty"`
}

// TraitSetConfig is the YAML/config representation of a trait map.
type TraitSetConfig struct {
	Traits map[string]float64 `json:"traits" yaml:"traits"`
}

// ProfileConfig is the experience-layer source config for a mode.
type ProfileConfig struct {
	ID                string            `json:"id" yaml:"id"`
	DisplayName       string            `json:"display_name" yaml:"display_name"`
	CoreIdentity      TraitSetConfig    `json:"core_identity" yaml:"core_identity"`
	PersonaModules    []ModuleConfig    `json:"persona_modules" yaml:"persona_modules"`
	OutputPreferences OutputPreferences `json:"output_preferences" yaml:"output_preferences"`
	RoleContext       RoleContext       `json:"role_context" yaml:"role_context"`
}

// MergeEngineInput is the canonical merge-engine input contract.
type MergeEngineInput struct {
	CoreIdentity          TraitSetConfig      `json:"core_identity"`
	GovernanceBounds      GovernanceBounds    `json:"governance_bounds"`
	RoleContext           RoleContext         `json:"role_context"`
	PersonaModules        []ModuleConfig      `json:"persona_modules"`
	RelationshipProfile   RelationshipProfile `json:"relationship_profile"`
	LiveContext           LiveContext         `json:"live_context"`
	ExplicitTurnOverrides ExplicitTurnInput   `json:"explicit_turn_overrides"`
	OutputPreferences     OutputPreferences   `json:"output_preferences"`
}

// GovernanceConstraint is an audit-visible governance constraint.
type GovernanceConstraint struct {
	Trait          string  `json:"trait"`
	ConstraintType string  `json:"constraint_type"`
	Value          float64 `json:"value"`
	Reason         string  `json:"reason"`
}

// GovernanceTrace is the resolved governance trace in the effective state.
type GovernanceTrace struct {
	AllowHumor                     bool                   `json:"allow_humor"`
	AllowHighChallenge             bool                   `json:"allow_high_challenge"`
	AllowHighDirectness            bool                   `json:"allow_high_directness"`
	MaxInitiativeStyle             float64                `json:"max_initiative_style"`
	MaxRecommendationDirectiveness float64                `json:"max_recommendation_directiveness"`
	HighStakesContext              bool                   `json:"high_stakes_context"`
	EmotionallySensitiveContext    bool                   `json:"emotionally_sensitive_context"`
	RequiresNeutrality             bool                   `json:"requires_neutrality"`
	Reasons                        []string               `json:"reasons"`
	AdditionalConstraints          []GovernanceConstraint `json:"additional_constraints"`
}

// LiveContextTrace is the live-context trace in the effective state.
type LiveContextTrace struct {
	AmbiguityLevel     float64 `json:"ambiguity_level"`
	UrgencyLevel       float64 `json:"urgency_level"`
	EmotionalIntensity float64 `json:"emotional_intensity"`
	StakesLevel        float64 `json:"stakes_level"`
	ComplexityLevel    float64 `json:"complexity_level"`
	ConfidenceLevel    float64 `json:"confidence_level"`
}

// ExplicitTurnTrace is the output-side explicit-turn trace.
type ExplicitTurnTrace struct {
	MustBeBrief               bool      `json:"must_be_brief"`
	MustAskClarifyingQuestion bool      `json:"must_ask_clarifying_question"`
	MustNotUseHumor           bool      `json:"must_not_use_humor"`
	MustBeHighlyStructured    bool      `json:"must_be_highly_structured"`
	UserRequestedToneShift    ToneShift `json:"user_requested_tone_shift,omitempty"`
}

// EffectiveStateAudit is the audit section of EffectivePersonaState.
type EffectiveStateAudit struct {
	SourcePrecedenceOrder []string `json:"source_precedence_order"`
	GatedSources          []string `json:"gated_sources"`
	WarningCodes          []string `json:"warning_codes"`
}

// EffectivePersonaState is the canonical merge-engine output.
type EffectivePersonaState struct {
	SchemaVersion         string                     `json:"schema_version"`
	StateID               string                     `json:"state_id"`
	GeneratedAt           string                     `json:"generated_at"`
	MergeEngineVersion    string                     `json:"merge_engine_version"`
	RoleContext           RoleContext                `json:"role_context"`
	ResolvedTraits        map[string]TraitValueState `json:"resolved_traits"`
	GovernanceTrace       GovernanceTrace            `json:"governance_trace"`
	LiveContextTrace      LiveContextTrace           `json:"live_context_trace"`
	ExplicitTurnOverrides ExplicitTurnTrace          `json:"explicit_turn_overrides"`
	OutputPreferences     OutputPreferences          `json:"output_preferences"`
	Audit                 EffectiveStateAudit        `json:"audit"`
}

// CompiledBudget is the compiled budget summary.
type CompiledBudget struct {
	TargetTokens    int `json:"target_tokens"`
	HardMaxTokens   int `json:"hard_max_tokens"`
	EstimatedTokens int `json:"estimated_tokens"`
	TrimLevel       int `json:"trim_level"`
}

// CognitiveModulation is the compiled cognitive surface.
type CognitiveModulation struct {
	AnalyticDepth      float64            `json:"analytic_depth"`
	Skepticism         float64            `json:"skepticism"`
	Decisiveness       float64            `json:"decisiveness"`
	ReasoningPosture   ReasoningPosture   `json:"reasoning_posture"`
	UncertaintyPosture UncertaintyPosture `json:"uncertainty_posture"`
	ChallengePosture   ChallengePosture   `json:"challenge_posture"`
}

// ExpressionPolicy is the compiled expression surface.
type ExpressionPolicy struct {
	Directness        float64  `json:"directness"`
	Warmth            float64  `json:"warmth"`
	Formality         float64  `json:"formality"`
	Seriousness       float64  `json:"seriousness"`
	Verbosity         float64  `json:"verbosity"`
	Conversationality float64  `json:"conversationality"`
	HumorPlayfulness  float64  `json:"humor_playfulness"`
	StyleFlags        []string `json:"style_flags"`
}

// BehaviorPolicy is the compiled behavioral surface.
type BehaviorPolicy struct {
	InitiativeStyle             float64            `json:"initiative_style"`
	ClarificationThreshold      float64            `json:"clarification_threshold"`
	ChallengeIntensity          float64            `json:"challenge_intensity"`
	EmotionalAttunement         float64            `json:"emotional_attunement"`
	Supportiveness              float64            `json:"supportiveness"`
	Familiarity                 float64            `json:"familiarity"`
	RecommendationDirectiveness float64            `json:"recommendation_directiveness"`
	AskVsInferMode              AskVsInferMode     `json:"ask_vs_infer_mode"`
	RecommendationMode          RecommendationMode `json:"recommendation_mode"`
	InterpersonalMode           InterpersonalMode  `json:"interpersonal_mode"`
}

// OutputContract is the compiled output surface.
type OutputContract struct {
	StructureLevel     float64         `json:"structure_level"`
	ResponseShape      ResponseShape   `json:"response_shape"`
	SummaryFirst       bool            `json:"summary_first"`
	PreferredLength    PreferredLength `json:"preferred_length"`
	FormattingRules    []string        `json:"formatting_rules"`
	ProhibitedPatterns []string        `json:"prohibited_patterns"`
}

// GatesAndClamps is the compiled gates and clamps summary.
type GatesAndClamps struct {
	HumorAllowed                   bool     `json:"humor_allowed"`
	ChallengeCap                   float64  `json:"challenge_cap"`
	DirectnessCap                  float64  `json:"directness_cap"`
	InitiativeCap                  float64  `json:"initiative_cap"`
	RecommendationDirectivenessCap float64  `json:"recommendation_directiveness_cap"`
	ActiveReasons                  []string `json:"active_reasons"`
}

// SerializationHints guides serializer trimming.
type SerializationHints struct {
	PriorityOrder    []string `json:"priority_order"`
	SafeToTrimFirst  []string `json:"safe_to_trim_first"`
	PreserveVerbatim []string `json:"preserve_verbatim"`
}

// CompiledAudit is the audit section of the compiled payload.
type CompiledAudit struct {
	DependencyCorrectionsApplied []string `json:"dependency_corrections_applied"`
	GatedTraits                  []string `json:"gated_traits"`
	TrimmedFields                []string `json:"trimmed_fields"`
	WarningCodes                 []string `json:"warning_codes"`
}

// CompiledPersonaPayload is the canonical compiler output.
type CompiledPersonaPayload struct {
	SchemaVersion       string              `json:"schema_version"`
	PayloadID           string              `json:"payload_id"`
	CompiledAt          string              `json:"compiled_at"`
	SourceStateID       string              `json:"source_state_id"`
	CompilerVersion     string              `json:"compiler_version"`
	SourceHash          string              `json:"source_hash"`
	Budget              CompiledBudget      `json:"budget"`
	RoleAdapter         RoleContext         `json:"role_adapter"`
	CognitiveModulation CognitiveModulation `json:"cognitive_modulation"`
	ExpressionPolicy    ExpressionPolicy    `json:"expression_policy"`
	BehaviorPolicy      BehaviorPolicy      `json:"behavior_policy"`
	OutputContract      OutputContract      `json:"output_contract"`
	GatesAndClamps      GatesAndClamps      `json:"gates_and_clamps"`
	SerializationHints  SerializationHints  `json:"serialization_hints"`
	Audit               CompiledAudit       `json:"audit"`
}

// BuildRequest is the runtime request used to derive the experience payload.
type BuildRequest struct {
	ChatID                   string
	Mode                     string
	LastUserMessage          string
	ActiveRole               ActiveRole
	TaskArchetype            string
	DelegationMode           DelegationMode
	CoreIdentity             TraitSetConfig
	PersonaModules           []ModuleConfig
	RelationshipProfile      RelationshipProfile
	OutputPreferences        OutputPreferences
	ExplicitSessionOverrides ExplicitTurnInput
	SessionOverrideTrace     ExplicitTurnTrace
	SessionOutputPreferences OutputPreferences
}

// RenderedControl is the complete derived experience-layer output for a turn.
type RenderedControl struct {
	Input    MergeEngineInput
	State    EffectivePersonaState
	Payload  CompiledPersonaPayload
	Fragment string
}
