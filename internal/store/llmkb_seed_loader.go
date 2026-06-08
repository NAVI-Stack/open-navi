package store

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
	"gopkg.in/yaml.v3"
)

const supportedLLMSeedSchemaVersion = 1

type seedValidationError struct {
	File  string
	Field string
	Value any
	Msg   string
}

func (e *seedValidationError) Error() string {
	return fmt.Sprintf("llmkb seed %s field %s value %v: %s", e.File, e.Field, e.Value, e.Msg)
}

type llmSeedDocument struct {
	Kind          string   `yaml:"kind"`
	SchemaVersion int      `yaml:"schema_version"`
	LLMID         string   `yaml:"llm_id"`
	CanonicalName string   `yaml:"canonical_name"`
	Aliases       []string `yaml:"aliases"`
	Identity      struct {
		Provider              string   `yaml:"provider"`
		Publisher             string   `yaml:"publisher"`
		ReleaseChannel        string   `yaml:"release_channel"`
		HostingModesSupported []string `yaml:"hosting_modes_supported"`
	} `yaml:"identity"`
	Capabilities struct {
		PrimaryUseCases           []string `yaml:"primary_use_cases"`
		InteractionModes          []string `yaml:"interaction_modes"`
		AgenticClass              string   `yaml:"agentic_class"`
		CodingClass               string   `yaml:"coding_class"`
		ReasoningClass            string   `yaml:"reasoning_class"`
		InstructionFollowingClass string   `yaml:"instruction_following_class"`
		ToolDisciplineClass       string   `yaml:"tool_discipline_class"`
		SchemaReliabilityClass    string   `yaml:"schema_reliability_class"`
		MultimodalClass           string   `yaml:"multimodal_class"`
	} `yaml:"capabilities"`
	TechnicalProfile struct {
		SupportsTools         bool `yaml:"supports_tools"`
		SupportsParallelTools bool `yaml:"supports_parallel_tools"`
		SupportsJSONSchema    bool `yaml:"supports_json_schema"`
		SupportsStreaming     bool `yaml:"supports_streaming"`
		SupportsVision        bool `yaml:"supports_vision"`
		SupportsAudioIn       bool `yaml:"supports_audio_in"`
		SupportsAudioOut      bool `yaml:"supports_audio_out"`
		MaxOutputTokens       int     `yaml:"max_output_tokens"`
		InputPricePer1k       float64 `yaml:"input_price_per_1k"`
		OutputPricePer1k      float64 `yaml:"output_price_per_1k"`
		ContextWindow         struct {
			InputTokens int `yaml:"input_tokens"`
		} `yaml:"context_window"`
	} `yaml:"technical_profile"`
	RoutingProfile struct {
		PreferredFor         []string `yaml:"preferred_for"`
		PreferredUseCases    []string `yaml:"preferred_use_cases"`
		CostTier             string   `yaml:"cost_tier"`
		LatencyTier          string   `yaml:"latency_tier"`
		MaxRiskTierAllowed   string   `yaml:"max_risk_tier_allowed"`
		AutonomyCeiling      string   `yaml:"autonomy_ceiling"`
		SurfacingLevel       string   `yaml:"surfacing_level"`
	} `yaml:"routing_profile"`
}

func LoadLLMSeeds(ctx context.Context, repo *SQLiteLLMKBRepo, dir string, logger *slog.Logger) error {
	return repo.LoadLLMSeeds(ctx, dir, logger)
}

func (r *SQLiteLLMKBRepo) LoadLLMSeeds(ctx context.Context, dir string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	for _, name := range names {
		if err := r.loadLLMSeedFile(ctx, filepath.Join(dir, name), logger); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteLLMKBRepo) loadLLMSeedFile(ctx context.Context, path string, logger *slog.Logger) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var seed llmSeedDocument
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&seed); err != nil {
		return &seedValidationError{File: filepath.Base(path), Field: "$", Value: path, Msg: err.Error()}
	}
	if err := validateLLMSeed(path, seed); err != nil {
		return err
	}
	provider, profile, err := buildProfileFromSeed(path, seed)
	if err != nil {
		return err
	}
	if err := r.SaveProvider(ctx, provider); err != nil {
		return err
	}
	existing, err := r.GetProfile(ctx, profile.LLMID)
	if err != nil {
		return err
	}
	return r.SaveProfile(ctx, mergeSeedProfile(existing, profile, logger))
}

func validateLLMSeed(path string, seed llmSeedDocument) error {
	file := filepath.Base(path)
	if seed.Kind != "" && seed.Kind != "LLMProfile" {
		return &seedValidationError{File: file, Field: "kind", Value: seed.Kind, Msg: "must be LLMProfile"}
	}
	if seed.SchemaVersion != supportedLLMSeedSchemaVersion {
		return &seedValidationError{File: file, Field: "schema_version", Value: seed.SchemaVersion, Msg: "unknown schema_version"}
	}
	if strings.TrimSpace(seed.LLMID) == "" {
		return &seedValidationError{File: file, Field: "llm_id", Value: seed.LLMID, Msg: "is required"}
	}
	if strings.TrimSpace(seed.CanonicalName) == "" {
		return &seedValidationError{File: file, Field: "canonical_name", Value: seed.CanonicalName, Msg: "is required"}
	}
	if strings.TrimSpace(seed.Identity.Provider) == "" {
		return &seedValidationError{File: file, Field: "identity.provider", Value: seed.Identity.Provider, Msg: "is required"}
	}
	requiredEnums := []struct {
		field string
		value string
		parse func(string) error
	}{
		{"capabilities.agentic_class", seed.Capabilities.AgenticClass, func(v string) error { _, err := llmkb.ParseAgenticClass(v); return err }},
		{"capabilities.coding_class", seed.Capabilities.CodingClass, func(v string) error { _, err := llmkb.ParseCodingClass(v); return err }},
		{"capabilities.reasoning_class", seed.Capabilities.ReasoningClass, func(v string) error { _, err := llmkb.ParseReasoningClass(v); return err }},
		{"capabilities.instruction_following_class", seed.Capabilities.InstructionFollowingClass, func(v string) error { _, err := llmkb.ParseClassLevel(v); return err }},
		{"capabilities.tool_discipline_class", seed.Capabilities.ToolDisciplineClass, func(v string) error { _, err := llmkb.ParseClassLevel(v); return err }},
		{"capabilities.schema_reliability_class", seed.Capabilities.SchemaReliabilityClass, func(v string) error { _, err := llmkb.ParseClassLevel(v); return err }},
		{"capabilities.multimodal_class", seed.Capabilities.MultimodalClass, func(v string) error { _, err := llmkb.ParseClassLevel(v); return err }},
	}
	for _, item := range requiredEnums {
		if strings.TrimSpace(item.value) == "" {
			return &seedValidationError{File: file, Field: item.field, Value: item.value, Msg: "is required"}
		}
		if err := item.parse(item.value); err != nil {
			return &seedValidationError{File: file, Field: item.field, Value: item.value, Msg: err.Error()}
		}
	}
	for _, value := range seed.Capabilities.PrimaryUseCases {
		if _, err := mapSeedUseCase(value); err != nil {
			return &seedValidationError{File: file, Field: "capabilities.primary_use_cases", Value: value, Msg: err.Error()}
		}
	}
	for _, value := range seed.Capabilities.InteractionModes {
		if _, err := llmkb.ParseInteractionMode(value); err != nil {
			return &seedValidationError{File: file, Field: "capabilities.interaction_modes", Value: value, Msg: err.Error()}
		}
	}
	for _, value := range seed.RoutingProfile.PreferredFor {
		if _, err := llmkb.ParseTaskClass(value); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.preferred_for", Value: value, Msg: err.Error()}
		}
	}
	if seed.RoutingProfile.CostTier != "" {
		if _, err := llmkb.ParseCostTier(seed.RoutingProfile.CostTier); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.cost_tier", Value: seed.RoutingProfile.CostTier, Msg: err.Error()}
		}
	}
	if seed.RoutingProfile.LatencyTier != "" {
		if _, err := llmkb.ParseLatencyTier(seed.RoutingProfile.LatencyTier); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.latency_tier", Value: seed.RoutingProfile.LatencyTier, Msg: err.Error()}
		}
	}
	if seed.RoutingProfile.MaxRiskTierAllowed != "" {
		if _, err := llmkb.ParseRiskTier(seed.RoutingProfile.MaxRiskTierAllowed); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.max_risk_tier_allowed", Value: seed.RoutingProfile.MaxRiskTierAllowed, Msg: err.Error()}
		}
	}
	if seed.RoutingProfile.AutonomyCeiling != "" {
		if _, err := llmkb.ParseAutonomyLevel(seed.RoutingProfile.AutonomyCeiling); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.autonomy_ceiling", Value: seed.RoutingProfile.AutonomyCeiling, Msg: err.Error()}
		}
	}
	if seed.RoutingProfile.SurfacingLevel != "" {
		if _, err := llmkb.ParseSurfacingLevel(seed.RoutingProfile.SurfacingLevel); err != nil {
			return &seedValidationError{File: file, Field: "routing_profile.surfacing_level", Value: seed.RoutingProfile.SurfacingLevel, Msg: err.Error()}
		}
	}
	return nil
}

func buildProfileFromSeed(path string, seed llmSeedDocument) (llmkb.LLMProvider, llmkb.LLMProfile, error) {
	now := time.Now().UTC()
	providerID := strings.ToLower(strings.TrimSpace(seed.Identity.Provider))
	// hostingMode, runtimeBackend := deriveProviderRuntime(providerID, seed.Identity.HostingModesSupported) // deprecated, not used directly here anymore.
	
	provenance := llmkb.Provenance{
		Source:       llmkb.ProvenanceSourceCuratedSeed,
		SourceDetail: path,
		Confidence:   0.9,
		AssertedAt:   now,
		FieldClass:   llmkb.FieldClassGoverned,
	}
	releaseChannel := llmkb.ReleaseChannelStable
	if seed.Identity.ReleaseChannel != "" {
		parsed, _ := llmkb.ParseReleaseChannel(seed.Identity.ReleaseChannel)
		releaseChannel = parsed
	}

	useCases := make([]llmkb.UseCase, 0, len(seed.Capabilities.PrimaryUseCases))
	for _, item := range seed.Capabilities.PrimaryUseCases {
		parsed, _ := mapSeedUseCase(item)
		useCases = append(useCases, parsed)
	}
	interactionModes := make([]llmkb.InteractionMode, 0, len(seed.Capabilities.InteractionModes))
	for _, item := range seed.Capabilities.InteractionModes {
		parsed, _ := llmkb.ParseInteractionMode(item)
		interactionModes = append(interactionModes, parsed)
	}
	preferredFor := make([]llmkb.TaskClass, 0, len(seed.RoutingProfile.PreferredFor))
	for _, item := range seed.RoutingProfile.PreferredFor {
		parsed, _ := llmkb.ParseTaskClass(item)
		preferredFor = append(preferredFor, parsed)
	}
	agenticClass, _ := llmkb.ParseAgenticClass(seed.Capabilities.AgenticClass)
	codingClass, _ := llmkb.ParseCodingClass(seed.Capabilities.CodingClass)
	reasoningClass, _ := llmkb.ParseReasoningClass(seed.Capabilities.ReasoningClass)
	instructionFollowing, _ := llmkb.ParseClassLevel(seed.Capabilities.InstructionFollowingClass)
	toolDiscipline, _ := llmkb.ParseClassLevel(seed.Capabilities.ToolDisciplineClass)
	schemaReliability, _ := llmkb.ParseClassLevel(seed.Capabilities.SchemaReliabilityClass)
	multimodalClass, _ := llmkb.ParseClassLevel(seed.Capabilities.MultimodalClass)

	provider := llmkb.LLMProvider{
		ProviderID:     providerID,
		CanonicalName:  firstNonEmpty(strings.TrimSpace(seed.Identity.Publisher), providerDisplayName(providerID)),
		Provenance:     provenance,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	profile := llmkb.LLMProfile{
		SchemaVersion:     seed.SchemaVersion,
		LLMID:             strings.TrimSpace(seed.LLMID),
		ProviderModelID:   strings.TrimSpace(seed.LLMID), // assuming llm_id is provider model ID in YAML
		CanonicalName:     strings.TrimSpace(seed.CanonicalName),
		Aliases:           seed.Aliases,
		ProviderID:        providerID,
		ReleaseChannel:    releaseChannel,
		Capabilities: llmkb.CapabilityProfile{
			AgenticClass:              agenticClass,
			CodingClass:               codingClass,
			ReasoningClass:            reasoningClass,
			InstructionFollowingClass: instructionFollowing,
			ToolDisciplineClass:       toolDiscipline,
			SchemaReliabilityClass:    schemaReliability,
			MultimodalClass:           multimodalClass,
			PrimaryUseCases:           useCases,
			InteractionModes:          interactionModes,
		},
		Features: llmkb.TechnicalFeatures{
			SupportsTools:             seed.TechnicalProfile.SupportsTools,
			SupportsStreaming:         seed.TechnicalProfile.SupportsStreaming,
			SupportsVision:            seed.TechnicalProfile.SupportsVision,
			SupportsAudioIn:           seed.TechnicalProfile.SupportsAudioIn,
			SupportsAudioOut:          seed.TechnicalProfile.SupportsAudioOut,
			SupportsJSONSchema:        seed.TechnicalProfile.SupportsJSONSchema,
			SupportsParallelTools:     seed.TechnicalProfile.SupportsParallelTools,
			ContextWindowTokens:       seed.TechnicalProfile.ContextWindow.InputTokens,
			MaxOutputTokens:           seed.TechnicalProfile.MaxOutputTokens,
		},
		OperationalState: llmkb.OperationalState{
			AvailabilityState: llmkb.AvailabilityStateAvailable,
			InstalledLocally:  contains(seed.Identity.HostingModesSupported, "local"),
			RuntimeBackend:   llmkb.RuntimeBackend(providerID), // simplified for now
		},
		Routing: llmkb.RoutingProfile{
			PreferredFor:         preferredFor,
		},
		Provenance:     provenance,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastVerifiedAt: now,
	}
	if seed.RoutingProfile.CostTier != "" {
		profile.Routing.CostTier, _ = llmkb.ParseCostTier(seed.RoutingProfile.CostTier)
	}
	if seed.RoutingProfile.LatencyTier != "" {
		profile.Routing.LatencyTier, _ = llmkb.ParseLatencyTier(seed.RoutingProfile.LatencyTier)
	}
	if seed.RoutingProfile.MaxRiskTierAllowed != "" {
		profile.Routing.MaxRiskTierAllowed, _ = llmkb.ParseRiskTier(seed.RoutingProfile.MaxRiskTierAllowed)
	}
	if seed.RoutingProfile.AutonomyCeiling != "" {
		profile.Routing.AutonomyCeiling, _ = llmkb.ParseAutonomyLevel(seed.RoutingProfile.AutonomyCeiling)
	}
	return provider, profile, nil
}

func mergeSeedProfile(existing *llmkb.LLMProfile, seeded llmkb.LLMProfile, logger *slog.Logger) llmkb.LLMProfile {
	if existing == nil {
		seeded.Provenance.MutationHistory = append(seeded.Provenance.MutationHistory, llmkb.MutationRecord{
			FieldName: "seed_load",
			NewValue:  fmt.Sprintf("schema_version=%d", seeded.SchemaVersion),
			ChangedAt: time.Now().UTC(),
			Source:    llmkb.ProvenanceSourceCuratedSeed,
		})
		return seeded
	}
	merged := *existing
	
	// Always update basic metadata from seed
	merged.SchemaVersion = max(existing.SchemaVersion, seeded.SchemaVersion)
	merged.CanonicalName = seeded.CanonicalName
	merged.Aliases = seeded.Aliases
	merged.ProviderID = seeded.ProviderID
	
	// Update Governed Capabilities
	merged.Capabilities.AgenticClass = seeded.Capabilities.AgenticClass
	merged.Capabilities.CodingClass = seeded.Capabilities.CodingClass
	merged.Capabilities.ReasoningClass = seeded.Capabilities.ReasoningClass
	merged.Capabilities.InstructionFollowingClass = seeded.Capabilities.InstructionFollowingClass
	merged.Capabilities.ToolDisciplineClass = seeded.Capabilities.ToolDisciplineClass
	merged.Capabilities.SchemaReliabilityClass = seeded.Capabilities.SchemaReliabilityClass
	merged.Capabilities.MultimodalClass = seeded.Capabilities.MultimodalClass
	
	// Update Features (Governed)
	merged.Features = seeded.Features
	
	// Provenance of the *current* version
	merged.Provenance.Source = seeded.Provenance.Source
	merged.Provenance.SourceDetail = seeded.Provenance.SourceDetail
	merged.Provenance.Confidence = seeded.Provenance.Confidence
	merged.Provenance.AssertedAt = seeded.Provenance.AssertedAt
	merged.Provenance.FieldClass = seeded.Provenance.FieldClass

	if existing.SchemaVersion < seeded.SchemaVersion {
		// On version bump, curated seed takes precedence for these governed fields
		merged.Capabilities.PrimaryUseCases = seeded.Capabilities.PrimaryUseCases
		merged.Capabilities.InteractionModes = seeded.Capabilities.InteractionModes
		merged.Routing = seeded.Routing
		
		merged.Provenance.MutationHistory = append(merged.Provenance.MutationHistory, llmkb.MutationRecord{
			FieldName: "seed_reload",
			OldValue:  fmt.Sprintf("schema_version=%d", existing.SchemaVersion),
			NewValue:  fmt.Sprintf("schema_version=%d", seeded.SchemaVersion),
			ChangedAt: time.Now().UTC(),
			Source:    llmkb.ProvenanceSourceCuratedSeed,
		})
		return merged
	}

	// Schema version is equal - only fill in missing governed data, don't revert user/inferred changes
	if len(existing.Capabilities.PrimaryUseCases) == 0 {
		merged.Capabilities.PrimaryUseCases = seeded.Capabilities.PrimaryUseCases
	}
	if len(existing.Capabilities.InteractionModes) == 0 {
		merged.Capabilities.InteractionModes = seeded.Capabilities.InteractionModes
	}
	if isZeroRoutingProfile(existing.Routing) {
		merged.Routing = seeded.Routing
	}
	
	// No version bump + same data = no history record to keep it clean
	return merged
}

func isZeroRoutingProfile(profile llmkb.RoutingProfile) bool {
	return len(profile.PreferredFor) == 0 &&
		len(profile.AvoidFor) == 0 &&
		len(profile.FallbackModels) == 0 &&
		profile.CostTier == "" &&
		profile.LatencyTier == "" &&
		profile.MaxRiskTierAllowed == "" &&
		profile.AutonomyCeiling == "" &&
		profile.TrustLevel == ""
}

func mapSeedUseCase(raw string) (llmkb.UseCase, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chat", "complex_chat", "general_chat":
		return llmkb.UseCaseGeneralChat, nil
	case "coding", "coding_assistant", "coding_generation":
		return llmkb.UseCaseCodingAssistant, nil
	case "research", "analysis", "reasoning_heavy_analysis":
		return llmkb.UseCaseResearch, nil
	case "planning":
		return llmkb.UseCasePlanning, nil
	case "automation":
		return llmkb.UseCaseAutomation, nil
	case "structured_output", "summarization":
		return llmkb.UseCaseStructuredOutput, nil
	case "long_context":
		return llmkb.UseCaseLongContext, nil
	case "multimodal", "image_understanding":
		return llmkb.UseCaseMultimodal, nil
	case "evaluation":
		return llmkb.UseCaseEvaluation, nil
	default:
		return "", fmt.Errorf("llmkb: invalid seed use case %q", raw)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func providerDisplayName(providerID string) string {
	switch providerID {
	case "openai":
		return "OpenAI"
	case "openrouter":
		return "OpenRouter"
	case "anthropic":
		return "Anthropic"
	case "ollama":
		return "Ollama"
	default:
		return providerID
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}
