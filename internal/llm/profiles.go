package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

type TaskClass string

const (
	TaskClassChat        TaskClass = "chat"
	TaskClassAgentic     TaskClass = "agentic"
	TaskClassCoding      TaskClass = "coding"
	TaskClassReasoning   TaskClass = "reasoning"
	TaskClassLightweight TaskClass = "lightweight"
)

// RoutingVisibility controls whether a routing announcement reaches the user.
type RoutingVisibility string

const (
	// RoutingVisibilitySilent logs the routing decision but never shows it to the user. Default.
	RoutingVisibilitySilent RoutingVisibility = "silent"
	// RoutingVisibilityThinking injects the announcement into the LLM context so it can
	// reason about the routing decision, but does not render it in the user-visible reply.
	RoutingVisibilityThinking RoutingVisibility = "thinking"
	// RoutingVisibilityDebug shows the announcement only when the user has opted into
	// routing traceability (routing_visibility = "debug").
	RoutingVisibilityDebug RoutingVisibility = "debug"
	// RoutingVisibilityUser always shows the announcement (explicit user-initiated switch).
	RoutingVisibilityUser RoutingVisibility = "user"
)

// ShouldShowAnnouncement returns true when the announcement should be rendered
// in the user-visible reply text.
//   - "user" tier: always shown regardless of preference
//   - "debug" tier: shown only when user preference is "debug"
//   - "thinking" tier: never shown (injected as LLM context instead)
//   - "silent" tier: never shown
func ShouldShowAnnouncement(announcementVis, userPref RoutingVisibility) bool {
	if announcementVis == RoutingVisibilityUser {
		return true
	}
	if announcementVis == RoutingVisibilityDebug && userPref == RoutingVisibilityDebug {
		return true
	}
	return false
}

// ShouldInjectAsContext returns true when the announcement should be injected
// into the LLM message context so the model can reason about the routing
// decision without exposing it in the user-visible reply. Only the "thinking"
// tier triggers injection.
func ShouldInjectAsContext(announcementVis RoutingVisibility) bool {
	return announcementVis == RoutingVisibilityThinking
}

type ComplexityLevel string

const (
	ComplexityLow    ComplexityLevel = "low"
	ComplexityMedium ComplexityLevel = "medium"
	ComplexityHigh   ComplexityLevel = "high"
)

const modelPreferencesSettingKey = "llm_routing_preferences"

type TaskClassification struct {
	Task              TaskClass        `json:"task"`
	Complexity        ComplexityLevel  `json:"complexity"`
	Score             int              `json:"score"`
	Signals           []string         `json:"signals,omitempty"`
	PreferredProvider string           `json:"preferred_provider,omitempty"`
	PreferredModel    string           `json:"preferred_model,omitempty"`
	PreferencePatch   *PreferencePatch `json:"preference_patch,omitempty"`
}

type ModelProfile struct {
	ProviderKey           string   `json:"provider_key"`
	ModelID               string   `json:"model_id"`
	DisplayName           string   `json:"display_name"`
	SupportsTools         bool     `json:"supports_tools"`
	SupportsStream        bool     `json:"supports_stream"`
	AgenticScore          int      `json:"agentic_score"`
	CodingScore           int      `json:"coding_score"`
	ChatScore             int      `json:"chat_score"`
	ReasoningScore        int      `json:"reasoning_score"`
	SpeedScore            int      `json:"speed_score"`
	CostScore             int      `json:"cost_score"`
	MaxContextTokens      int      `json:"max_context_tokens"`
	ToolCallReliable      bool     `json:"tool_call_reliable"`
	Tags                  []string `json:"tags,omitempty"`
	LearnedAgenticDelta   int      `json:"learned_agentic_delta,omitempty"`
	LearnedCodingDelta    int      `json:"learned_coding_delta,omitempty"`
	LearnedChatDelta      int      `json:"learned_chat_delta,omitempty"`
	LearnedReasoningDelta int      `json:"learned_reasoning_delta,omitempty"`
	LearnedEvidenceCount  int      `json:"learned_evidence_count,omitempty"`
}

type ModelPreferences struct {
	DefaultAgentic     string            `json:"default_agentic,omitempty"`
	DefaultCoding      string            `json:"default_coding,omitempty"`
	DefaultChat        string            `json:"default_chat,omitempty"`
	DefaultReasoning   string            `json:"default_reasoning,omitempty"`
	DefaultLightweight string            `json:"default_lightweight,omitempty"`
	PreferLocal        bool              `json:"prefer_local,omitempty"`
	CostSensitive      bool              `json:"cost_sensitive,omitempty"`
	RoutingVisibility  RoutingVisibility `json:"routing_visibility,omitempty"` // default "silent"
	// FavoriteModelIDs holds user-pinned model IDs (format: "providerKey/modelId").
	// Persisted server-side so favorites survive across clients/sessions.
	FavoriteModelIDs []string `json:"favorite_model_ids,omitempty"`
	// AutoRoutingEnabled signals that the user selected NAVI Auto — the per-turn
	// routing engine picks the best model for each message instead of using a pinned selection.
	AutoRoutingEnabled bool `json:"auto_routing_enabled,omitempty"`
	// PrivacyMode is the CIP §9 locality posture (Local / Hybrid / Cloud) that
	// gates which privacy classes may route to a cloud model (P4). Empty = Cloud
	// (permissive) for backward compatibility.
	PrivacyMode PrivacyMode `json:"privacy_mode,omitempty"`
}

type PreferencePatch struct {
	DefaultAgentic     *string            `json:"default_agentic,omitempty"`
	DefaultCoding      *string            `json:"default_coding,omitempty"`
	DefaultChat        *string            `json:"default_chat,omitempty"`
	DefaultReasoning   *string            `json:"default_reasoning,omitempty"`
	DefaultLightweight *string            `json:"default_lightweight,omitempty"`
	PreferLocal        *bool              `json:"prefer_local,omitempty"`
	CostSensitive      *bool              `json:"cost_sensitive,omitempty"`
	RoutingVisibility  *RoutingVisibility `json:"routing_visibility,omitempty"`
	// Pointer-to-slice so callers can explicitly set to empty ("clear all favorites").
	FavoriteModelIDs   *[]string    `json:"favorite_model_ids,omitempty"`
	AutoRoutingEnabled *bool        `json:"auto_routing_enabled,omitempty"`
	PrivacyMode        *PrivacyMode `json:"privacy_mode,omitempty"`
}

func (p ModelPreferences) Apply(patch PreferencePatch) ModelPreferences {
	out := p
	if patch.DefaultAgentic != nil {
		out.DefaultAgentic = strings.TrimSpace(*patch.DefaultAgentic)
	}
	if patch.DefaultCoding != nil {
		out.DefaultCoding = strings.TrimSpace(*patch.DefaultCoding)
	}
	if patch.DefaultChat != nil {
		out.DefaultChat = strings.TrimSpace(*patch.DefaultChat)
	}
	if patch.DefaultReasoning != nil {
		out.DefaultReasoning = strings.TrimSpace(*patch.DefaultReasoning)
	}
	if patch.DefaultLightweight != nil {
		out.DefaultLightweight = strings.TrimSpace(*patch.DefaultLightweight)
	}
	if patch.PreferLocal != nil {
		out.PreferLocal = *patch.PreferLocal
	}
	if patch.CostSensitive != nil {
		out.CostSensitive = *patch.CostSensitive
	}
	if patch.RoutingVisibility != nil {
		out.RoutingVisibility = *patch.RoutingVisibility
	}
	if patch.FavoriteModelIDs != nil {
		out.FavoriteModelIDs = *patch.FavoriteModelIDs
	}
	if patch.AutoRoutingEnabled != nil {
		out.AutoRoutingEnabled = *patch.AutoRoutingEnabled
	}
	if patch.PrivacyMode != nil {
		out.PrivacyMode = *patch.PrivacyMode
	}
	return out
}

func LoadModelPreferences(ctx context.Context, store SettingStore) (ModelPreferences, error) {
	if store == nil {
		return ModelPreferences{}, nil
	}
	raw, found, err := store.GetSetting(ctx, modelPreferencesSettingKey)
	if err != nil || !found || strings.TrimSpace(raw) == "" {
		return ModelPreferences{}, err
	}
	var prefs ModelPreferences
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return ModelPreferences{}, fmt.Errorf("llm: decode model preferences: %w", err)
	}
	return prefs, nil
}

func SaveModelPreferences(ctx context.Context, store SettingStore, prefs ModelPreferences) error {
	if store == nil {
		return nil
	}
	payload, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("llm: encode model preferences: %w", err)
	}
	return store.SetSetting(ctx, modelPreferencesSettingKey, string(payload))
}

func ClassifyTask(userMessage string, tools []ToolDefinition, history []Message) TaskClassification {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	words := strings.Fields(lower)
	signals := make([]string, 0, 6)
	result := TaskClassification{
		Task:       TaskClassChat,
		Complexity: ComplexityMedium,
		Score:      50,
	}
	if lower == "" {
		result.Task = TaskClassLightweight
		result.Complexity = ComplexityLow
		result.Score = 10
		return result
	}

	if patch := detectPreferencePatch(lower); patch != nil {
		result.PreferencePatch = patch
		signals = append(signals, "persistent_preference")
	}
	if provider, model := detectPerTurnOverride(lower); provider != "" || model != "" {
		result.PreferredProvider = provider
		result.PreferredModel = model
		signals = append(signals, "per_turn_override")
	}

	toolsNeeded := len(tools) > 0
	if toolsNeeded {
		signals = append(signals, "tools_available")
	}

	if isLightweight(lower, len(words)) {
		result.Task = TaskClassLightweight
		result.Complexity = ComplexityLow
		result.Score = 20
		signals = append(signals, "short_status_query")
	} else if hasAny(lower, "code", "coding", "function", "repo", "repository", "file", "build", "test", "bug", "refactor", "review", "pull request", "git", "compile", "debug") {
		result.Task = TaskClassCoding
		result.Score = 78
		signals = append(signals, "coding_keywords")
	} else if hasAny(lower, "analyze", "analysis", "compare", "why", "reason", "tradeoff", "trade-off", "design", "architect", "plan", "strategy", "math", "proof") {
		result.Task = TaskClassReasoning
		result.Score = 74
		signals = append(signals, "reasoning_keywords")
	} else if hasAny(lower, "use the tool", "step by step", "run this", "create a task", "send a message", "schedule", "connect", "configure", "install", "fix this end-to-end") {
		result.Task = TaskClassAgentic
		result.Score = 80
		signals = append(signals, "agentic_keywords")
	}

	if result.Task == TaskClassCoding && hasAny(lower, "review", "architecture", "architect", "large", "entire", "codebase", "refactor", "design") {
		result.Complexity = ComplexityHigh
		result.Score = 92
		signals = append(signals, "high_complexity_coding")
	} else if (result.Task == TaskClassReasoning || result.Task == TaskClassAgentic) && (len(words) > 40 || hasAny(lower, "architect", "multi-step", "multiple steps", "complex", "deep", "compare", "plan")) {
		result.Complexity = ComplexityHigh
		result.Score = 90
		signals = append(signals, "high_complexity_reasoning")
	} else if len(words) <= 10 && result.Task == TaskClassChat {
		result.Complexity = ComplexityLow
		result.Score = 30
		signals = append(signals, "short_chat")
	}

	// NOTE: tools_bias_agentic removed (OMN-86). Tool *availability* is an
	// infrastructure fact, not an intent signal. Tools are always registered;
	// reclassifying chat → agentic based on tool presence caused every casual
	// message to trigger model switching. Classification now relies solely on
	// message content keywords and explicit per-turn overrides.
	if len(history) > 12 && result.Complexity == ComplexityMedium && result.Task != TaskClassLightweight {
		result.Complexity = ComplexityHigh
		result.Score += 8
		signals = append(signals, "deep_session_history")
	}
	result.Signals = signals
	return result
}

func isLightweight(lower string, wordCount int) bool {
	if wordCount <= 4 && hasAny(lower, "hi", "hello", "hey", "thanks", "thank you", "status", "ping", "who are you", "/model") {
		return true
	}
	return wordCount <= 8 && hasAny(lower, "what model", "which model", "list models", "show models", "what llms", "current model", "/model list")
}

func detectPerTurnOverride(lower string) (provider string, model string) {
	if !hasAny(lower, "for this", "this time", "right now", "for this task") {
		return "", ""
	}
	switch {
	case strings.Contains(lower, "use local"):
		return "ollama", ""
	case strings.Contains(lower, "use anthropic"):
		return "anthropic", ""
	case strings.Contains(lower, "use openai"):
		return "openai", ""
	case strings.Contains(lower, "use openrouter"):
		return "openrouter", ""
	}
	switch {
	case strings.Contains(lower, "opus"):
		return "", "opus"
	case strings.Contains(lower, "sonnet"):
		return "", "sonnet"
	case strings.Contains(lower, "haiku"):
		return "", "haiku"
	case strings.Contains(lower, "llama"):
		return "", "llama"
	case strings.Contains(lower, "qwen"):
		return "", "qwen"
	case strings.Contains(lower, "deepseek"):
		return "", "deepseek"
	}
	return "", ""
}

func detectPreferencePatch(lower string) *PreferencePatch {
	switch {
	case hasAny(lower, "always use local", "prefer local", "stick to local"):
		value := true
		return &PreferencePatch{PreferLocal: &value}
	case hasAny(lower, "always use cloud", "prefer cloud"):
		value := false
		return &PreferencePatch{PreferLocal: &value}
	case hasAny(lower, "be cost sensitive", "prefer cheaper", "prefer cheap", "save cost"):
		value := true
		return &PreferencePatch{CostSensitive: &value}
	case hasAny(lower, "show routing", "routing visibility debug", "debug routing"):
		v := RoutingVisibilityDebug
		return &PreferencePatch{RoutingVisibility: &v}
	case hasAny(lower, "hide routing", "routing visibility silent", "silent routing"):
		v := RoutingVisibilitySilent
		return &PreferencePatch{RoutingVisibility: &v}
	case hasAny(lower, "explain routing", "routing visibility thinking", "thinking routing"):
		v := RoutingVisibilityThinking
		return &PreferencePatch{RoutingVisibility: &v}
	case hasAny(lower, "always show routing", "routing visibility user", "user-visible routing"):
		v := RoutingVisibilityUser
		return &PreferencePatch{RoutingVisibility: &v}
	}
	return nil
}

func SeedProfiles(catalog LLMCatalog) []ModelProfile {
	out := make([]ModelProfile, 0)
	for _, provider := range catalog.Providers {
		for _, model := range provider.Models {
			out = append(out, seedProfile(provider.Key, provider.DisplayName, model.Name))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderKey == out[j].ProviderKey {
			return out[i].ModelID < out[j].ModelID
		}
		return out[i].ProviderKey < out[j].ProviderKey
	})
	return out
}

func ApplyCalibration(profiles []ModelProfile, outcomes []schema.ExecutionOutcome) []ModelProfile {
	if len(profiles) == 0 || len(outcomes) == 0 {
		return profiles
	}
	type delta struct {
		agentic  int
		coding   int
		chat     int
		reason   int
		evidence int
	}
	byKey := make(map[string]*delta)
	for i := range outcomes {
		eo := outcomes[i]
		if strings.TrimSpace(eo.LLMProvider) == "" || strings.TrimSpace(eo.LLMModel) == "" {
			continue
		}
		key := eo.LLMProvider + "/" + eo.LLMModel
		d := byKey[key]
		if d == nil {
			d = &delta{}
			byKey[key] = d
		}
		d.evidence++
		success := eo.Outcome == schema.ExecutionOutcomeSucceeded || eo.Outcome == schema.ExecutionOutcomePartiallySucceeded
		switch eo.LLMTaskClass {
		case string(TaskClassAgentic):
			if success {
				d.agentic += 2
			} else {
				d.agentic -= 6
			}
		case string(TaskClassCoding):
			if success {
				d.coding += 2
			} else {
				d.coding -= 4
				d.agentic -= 1
			}
		case string(TaskClassReasoning):
			if success {
				d.reason += 2
			} else {
				d.reason -= 4
			}
		case string(TaskClassLightweight):
			if success {
				d.chat += 2
			} else {
				d.chat -= 2
			}
		default:
			if success {
				d.chat++
			} else {
				d.chat -= 2
			}
		}
	}

	out := append([]ModelProfile(nil), profiles...)
	for i := range out {
		key := out[i].ProviderKey + "/" + out[i].ModelID
		d := byKey[key]
		if d == nil {
			continue
		}
		d.agentic = clampDelta(d.agentic)
		d.coding = clampDelta(d.coding)
		d.chat = clampDelta(d.chat)
		d.reason = clampDelta(d.reason)
		out[i].LearnedAgenticDelta = d.agentic
		out[i].LearnedCodingDelta = d.coding
		out[i].LearnedChatDelta = d.chat
		out[i].LearnedReasoningDelta = d.reason
		out[i].LearnedEvidenceCount = d.evidence
		out[i].AgenticScore = clampScore(out[i].AgenticScore + d.agentic)
		out[i].CodingScore = clampScore(out[i].CodingScore + d.coding)
		out[i].ChatScore = clampScore(out[i].ChatScore + d.chat)
		out[i].ReasoningScore = clampScore(out[i].ReasoningScore + d.reason)
	}
	return out
}

func seedProfile(providerKey, displayName, model string) ModelProfile {
	lower := strings.ToLower(model)
	tags := []string{}
	supportsTools := providerKey != "ollama"
	supportsStream := true
	toolReliable := providerKey != "ollama"
	agentic, coding, chat, reasoning, speed, cost := 55, 55, 60, 60, 60, 60
	maxContext := 128000

	switch providerKey {
	case "ollama":
		tags = append(tags, "local", "free")
		supportsTools = false
		toolReliable = false
		cost = 100
		speed = 78
		// Temporary stopgap: once LLM-KB is fully implemented, replace this
		// with a query to the LLM-KB repository for tool-capability detection.
		if ollamaModelSupportsTools(lower) {
			supportsTools = true
			toolReliable = true
		}
	case "anthropic", "openai", "openrouter":
		tags = append(tags, "cloud")
	}

	switch {
	case strings.Contains(lower, "opus") || strings.Contains(lower, "o1"):
		agentic, coding, chat, reasoning, speed, cost = 95, 90, 72, 98, 35, 10
		tags = append(tags, "architect", "premium")
	case strings.Contains(lower, "sonnet") || strings.Contains(lower, "gpt-4.1") || strings.Contains(lower, "gpt-4o"):
		agentic, coding, chat, reasoning, speed, cost = 88, 91, 78, 86, 68, 42
		tags = append(tags, "generalist")
	case strings.Contains(lower, "haiku") || strings.Contains(lower, "mini"):
		agentic, coding, chat, reasoning, speed, cost = 72, 70, 84, 68, 94, 90
		tags = append(tags, "workhorse", "fast")
	case strings.Contains(lower, "coder") || strings.Contains(lower, "deepseek") || strings.Contains(lower, "qwen"):
		agentic, coding, chat, reasoning, speed, cost = 48, 93, 58, 74, 70, 88
		tags = append(tags, "coding")
	case strings.Contains(lower, "llama") || strings.Contains(lower, "mistral"):
		agentic, coding, chat, reasoning, speed, cost = 35, 50, 74, 55, 82, 98
		tags = append(tags, "chat")
	}

	if strings.Contains(lower, "70b") || strings.Contains(lower, "405b") {
		reasoning += 8
		coding += 5
		speed -= 10
	}

	return ModelProfile{
		ProviderKey:      providerKey,
		ModelID:          model,
		DisplayName:      displayName + " " + model,
		SupportsTools:    supportsTools,
		SupportsStream:   supportsStream,
		AgenticScore:     clampScore(agentic),
		CodingScore:      clampScore(coding),
		ChatScore:        clampScore(chat),
		ReasoningScore:   clampScore(reasoning),
		SpeedScore:       clampScore(speed),
		CostScore:        clampScore(cost),
		MaxContextTokens: maxContext,
		ToolCallReliable: toolReliable,
		Tags:             uniqueSorted(tags),
	}
}

// ollamaModelSupportsTools returns true for Ollama model families known to
// handle tool/function calling reliably. Temporary stopgap — will be replaced
// by LLM-KB capability queries once that subsystem is fully implemented.
func ollamaModelSupportsTools(modelLower string) bool {
	for _, p := range []string{
		"llama3.1", "llama3.2", "llama3.3", "llama-3.1", "llama-3.2", "llama-3.3",
		"qwen2.5", "qwen3",
		"gemma3", "gemma4",
		"mistral-nemo", "mistral-large",
		"command-r", "hermes", "functionary", "firefunction",
	} {
		if strings.Contains(modelLower, p) {
			return true
		}
	}
	return false
}

type ModelSelector struct {
	Profiles    []ModelProfile
	Preferences ModelPreferences
	// PrivacyMode is the CIP §9 locality posture applied as an additional filter
	// before the final route choice (P4). Empty preserves pre-P4 behavior
	// (permissive — equivalent to Cloud). See SelectWithPrivacy and privacy.go.
	PrivacyMode PrivacyMode
}

type RouteSelection struct {
	Provider       string             `json:"provider"`
	Model          string             `json:"model"`
	Classification TaskClassification `json:"classification"`
	Reason         string             `json:"reason,omitempty"`
	Profile        *ModelProfile      `json:"profile,omitempty"`
}

type RouteRequest struct {
	UserMessage     string           `json:"user_message"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	History         []Message        `json:"history,omitempty"`
	CurrentProvider string           `json:"current_provider,omitempty"`
	CurrentModel    string           `json:"current_model,omitempty"`
	ChatID          string           `json:"chat_id,omitempty"`
	TaskID          string           `json:"task_id,omitempty"`
}

type RouteDecision struct {
	Provider               string             `json:"provider"`
	Model                  string             `json:"model"`
	Classification         TaskClassification `json:"classification"`
	ChatID                 string             `json:"chat_id,omitempty"`
	TaskID                 string             `json:"task_id,omitempty"`
	Switched               bool               `json:"switched"`
	Announcement           string             `json:"announcement,omitempty"`
	AnnouncementVisibility RoutingVisibility  `json:"announcement_visibility,omitempty"`
	Reason                 string             `json:"reason,omitempty"`
	Profile                *ModelProfile      `json:"profile,omitempty"`
	StripTools             bool               `json:"strip_tools,omitempty"`
}

func (s ModelSelector) Select(classification TaskClassification, toolsNeeded bool) (RouteSelection, bool) {
	profiles := append([]ModelProfile(nil), s.Profiles...)
	if len(profiles) == 0 {
		return RouteSelection{Classification: classification}, false
	}

	if selected, ok := s.selectPreferred(profiles, classification, toolsNeeded); ok {
		return RouteSelection{
			Provider:       selected.ProviderKey,
			Model:          selected.ModelID,
			Classification: classification,
			Reason:         "preference_override",
			Profile:        &selected,
		}, true
	}

	type scored struct {
		profile ModelProfile
		score   int
	}
	var ranked []scored
	for _, profile := range profiles {
		if toolsNeeded && (!profile.SupportsTools || !profile.ToolCallReliable) {
			continue
		}
		score := scoreProfile(profile, classification, s.Preferences)
		ranked = append(ranked, scored{profile: profile, score: score})
	}
	if len(ranked) == 0 {
		return RouteSelection{Classification: classification}, false
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].profile.ProviderKey == ranked[j].profile.ProviderKey {
				return ranked[i].profile.ModelID < ranked[j].profile.ModelID
			}
			return ranked[i].profile.ProviderKey < ranked[j].profile.ProviderKey
		}
		return ranked[i].score > ranked[j].score
	})
	best := ranked[0].profile
	return RouteSelection{
		Provider:       best.ProviderKey,
		Model:          best.ModelID,
		Classification: classification,
		Reason:         "score",
		Profile:        &best,
	}, true
}

func (s ModelSelector) selectPreferred(profiles []ModelProfile, classification TaskClassification, toolsNeeded bool) (ModelProfile, bool) {
	if classification.PreferredProvider != "" || classification.PreferredModel != "" {
		if p, ok := findMatchingProfile(profiles, classification.PreferredProvider, classification.PreferredModel, toolsNeeded); ok {
			return p, true
		}
	}

	var taskDefault string
	switch classification.Task {
	case TaskClassAgentic:
		taskDefault = s.Preferences.DefaultAgentic
	case TaskClassCoding:
		taskDefault = s.Preferences.DefaultCoding
	case TaskClassReasoning:
		taskDefault = s.Preferences.DefaultReasoning
	case TaskClassLightweight:
		taskDefault = s.Preferences.DefaultLightweight
	default:
		taskDefault = s.Preferences.DefaultChat
	}
	if taskDefault != "" {
		provider, model := splitModelSpec(taskDefault)
		if p, ok := findMatchingProfile(profiles, provider, model, toolsNeeded); ok {
			return p, true
		}
	}
	return ModelProfile{}, false
}

func scoreProfile(profile ModelProfile, classification TaskClassification, prefs ModelPreferences) int {
	score := profile.ChatScore
	switch classification.Task {
	case TaskClassAgentic:
		score = profile.AgenticScore
	case TaskClassCoding:
		score = profile.CodingScore
	case TaskClassReasoning:
		score = profile.ReasoningScore
	case TaskClassLightweight:
		score = (profile.SpeedScore + profile.CostScore + profile.ChatScore) / 3
	}

	switch classification.Complexity {
	case ComplexityLow:
		score += profile.SpeedScore/3 + profile.CostScore/4
	case ComplexityHigh:
		score += profile.ReasoningScore / 3
		if hasTag(profile.Tags, "architect") {
			score += 18
		}
	}

	if prefs.PreferLocal {
		if hasTag(profile.Tags, "local") {
			score += 20
		} else {
			score -= 10
		}
	}
	if prefs.CostSensitive {
		score += profile.CostScore/3 + profile.SpeedScore/5
	}
	return score
}

func findMatchingProfile(profiles []ModelProfile, provider, model string, toolsNeeded bool) (ModelProfile, bool) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	model = strings.TrimSpace(strings.ToLower(model))
	for _, profile := range profiles {
		if toolsNeeded && (!profile.SupportsTools || !profile.ToolCallReliable) {
			continue
		}
		if provider != "" && strings.ToLower(profile.ProviderKey) != provider {
			continue
		}
		if model == "" {
			return profile, true
		}
		if strings.Contains(strings.ToLower(profile.ModelID), model) {
			return profile, true
		}
	}
	return ModelProfile{}, false
}

func splitModelSpec(spec string) (provider, model string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", ""
	}
	parts := strings.SplitN(spec, "/", 2)
	if len(parts) != 2 {
		return "", spec
	}
	return parts[0], parts[1]
}

func hasAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func uniqueSorted(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func clampDelta(v int) int {
	if v < -20 {
		return -20
	}
	if v > 20 {
		return 20
	}
	return v
}
