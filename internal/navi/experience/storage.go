package experience

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

const (
	ConfigScopeGlobal = "global"
	ConfigScopeOwner  = "owner"
	ConfigScopeSystem = "system"

	ConfigKeyCoreIdentity        = "experience.core_identity.v1"
	ConfigKeyOutputPreferences   = "experience.output_preferences.v1"
	ConfigKeyRelationshipProfile = "experience.relationship_profile.v1"
	ConfigKeyModulePrefix        = "experience.module.v1."
)

const snapshotMaterialDeltaThreshold = 0.10

const (
	SnapshotTriggerSessionStart   = "session_start"
	SnapshotTriggerMaterialDelta  = "material_delta"
	SnapshotTriggerProposalWorthy = "proposal_worthy"
	SnapshotTriggerOwnerInspect   = "owner_inspection"
	SnapshotTriggerSessionEnd     = "session_end"
)

type ConfigurationSnapshot struct {
	CoreIdentity        TraitSetConfig      `json:"core_identity"`
	OutputPreferences   OutputPreferences   `json:"output_preferences"`
	RelationshipProfile RelationshipProfile `json:"relationship_profile"`
	PersonaModules      []ModuleConfig      `json:"persona_modules"`
}

type SnapshotRecordOptions struct {
	ChatID         string
	OwnerID        string
	ExperienceMode string
	SessionStart   bool
	ProposalWorthy bool
}

func MaybeRecordExperienceSnapshot(ctx context.Context, db *sql.DB, rendered RenderedControl, opts SnapshotRecordOptions) (bool, error) {
	if db == nil || strings.TrimSpace(opts.ChatID) == "" {
		return false, nil
	}
	latest, err := store.LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, strings.TrimSpace(opts.ChatID))
	if err != nil {
		return false, err
	}
	if opts.SessionStart && latest == nil {
		return true, appendExperienceSnapshotEvent(ctx, db, rendered, schema.NaviExperienceSnapshotPayload{
			ChatID:            strings.TrimSpace(opts.ChatID),
			OwnerID:           strings.TrimSpace(opts.OwnerID),
			ExperienceMode:    strings.TrimSpace(opts.ExperienceMode),
			Trigger:           SnapshotTriggerSessionStart,
			SourceStateID:     rendered.State.StateID,
			CompiledPayloadID: rendered.Payload.PayloadID,
		})
	}

	delta := 0.0
	if latest != nil {
		payload, decodeErr := decodeExperienceSnapshotPayload(latest.Payload)
		if decodeErr != nil {
			return false, decodeErr
		}
		prevState, decodeErr := decodeEffectiveState(payload.EffectiveStateJSON)
		if decodeErr != nil {
			return false, decodeErr
		}
		delta = cumulativeTraitDelta(prevState, rendered.State)
	}

	trigger := ""
	switch {
	case opts.ProposalWorthy:
		trigger = SnapshotTriggerProposalWorthy
	case latest != nil && delta > snapshotMaterialDeltaThreshold:
		trigger = SnapshotTriggerMaterialDelta
	default:
		return false, nil
	}

	return true, appendExperienceSnapshotEvent(ctx, db, rendered, schema.NaviExperienceSnapshotPayload{
		ChatID:               strings.TrimSpace(opts.ChatID),
		OwnerID:              strings.TrimSpace(opts.OwnerID),
		ExperienceMode:       strings.TrimSpace(opts.ExperienceMode),
		Trigger:              trigger,
		CumulativeTraitDelta: delta,
		SourceStateID:        rendered.State.StateID,
		CompiledPayloadID:    rendered.Payload.PayloadID,
	})
}

func RecordSessionEndExperienceSnapshot(ctx context.Context, db *sql.DB, chatID, ownerID, experienceMode string) (bool, error) {
	if db == nil || strings.TrimSpace(chatID) == "" {
		return false, nil
	}
	latest, err := store.LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, strings.TrimSpace(chatID))
	if err != nil {
		return false, err
	}
	if latest == nil {
		return false, nil
	}
	payload, err := decodeExperienceSnapshotPayload(latest.Payload)
	if err != nil {
		return false, err
	}
	if payload.Trigger == SnapshotTriggerSessionEnd {
		return false, nil
	}
	payload.ChatID = strings.TrimSpace(chatID)
	if strings.TrimSpace(ownerID) != "" {
		payload.OwnerID = strings.TrimSpace(ownerID)
	}
	if strings.TrimSpace(experienceMode) != "" {
		payload.ExperienceMode = strings.TrimSpace(experienceMode)
	}
	payload.Trigger = SnapshotTriggerSessionEnd
	payload.CumulativeTraitDelta = 0
	payload.SnapshotID = ""
	return true, appendExperienceSnapshotEventFromPayload(ctx, db, strings.TrimSpace(chatID), payload)
}

// RecordOwnerInspectionSnapshot records an owner_inspection snapshot when the
// owner explicitly inspects experience state through the debugger surface.
// The correlation ID is "owner-inspect" since this is not scoped to a session.
func RecordOwnerInspectionSnapshot(ctx context.Context, db *sql.DB, rendered RenderedControl, ownerID, experienceMode string) error {
	if db == nil {
		return nil
	}
	return appendExperienceSnapshotEvent(ctx, db, rendered, schema.NaviExperienceSnapshotPayload{
		ChatID:            "owner-inspect",
		OwnerID:           strings.TrimSpace(ownerID),
		ExperienceMode:    strings.TrimSpace(experienceMode),
		Trigger:           SnapshotTriggerOwnerInspect,
		SourceStateID:     rendered.State.StateID,
		CompiledPayloadID: rendered.Payload.PayloadID,
	})
}

func ListModuleRegistry(ctx context.Context, db *sql.DB, ownerID string, limit int) ([]ModuleRegistryEntry, error) {
	if db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	entries := make([]ModuleRegistryEntry, 0, limit)
	appendEntries := func(scope, scopeID string) error {
		configEntries, err := store.ListConfigurationByScope(ctx, db, scope, scopeID, limit)
		if err != nil {
			return err
		}
		for _, entry := range configEntries {
			if !strings.HasPrefix(entry.Key, ConfigKeyModulePrefix) {
				continue
			}
			var module ModuleConfig
			if err := json.Unmarshal([]byte(entry.Value), &module); err != nil {
				return fmt.Errorf("experience: decode module registry entry %s: %w", entry.Key, err)
			}
			normalized := normalizeModules([]ModuleConfig{module})
			if len(normalized) == 0 {
				continue
			}
			entries = append(entries, ModuleRegistryEntry{
				ModuleID:           normalized[0].ModuleID,
				Version:            normalized[0].Version,
				Kind:               normalized[0].Kind,
				Scope:              normalized[0].Scope,
				ScopeID:            scopeID,
				Source:             entry.Source,
				Strength:           normalized[0].Strength,
				TraitContributions: normalized[0].TraitContributions,
			})
		}
		return nil
	}

	if err := appendEntries(ConfigScopeGlobal, ConfigScopeSystem); err != nil {
		return nil, err
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID != "" {
		if err := appendEntries(ConfigScopeOwner, ownerID); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if moduleScopeRank(entries[i].Scope) != moduleScopeRank(entries[j].Scope) {
			return moduleScopeRank(entries[i].Scope) > moduleScopeRank(entries[j].Scope)
		}
		if entries[i].ScopeID != entries[j].ScopeID {
			return entries[i].ScopeID < entries[j].ScopeID
		}
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		if entries[i].Version != entries[j].Version {
			return entries[i].Version > entries[j].Version
		}
		return entries[i].ModuleID < entries[j].ModuleID
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func LoadConfigurationSnapshot(ctx context.Context, db *sql.DB, ownerID string) (ConfigurationSnapshot, error) {
	snapshot := ConfigurationSnapshot{
		CoreIdentity:        TraitSetConfig{Traits: map[string]float64{}},
		RelationshipProfile: RelationshipProfile{},
		PersonaModules:      []ModuleConfig{},
	}
	if db == nil {
		return snapshot, nil
	}

	if cfg, ok, err := loadTraitSetConfig(ctx, db, ConfigScopeGlobal, ConfigScopeSystem, ConfigKeyCoreIdentity); err != nil {
		return snapshot, err
	} else if ok {
		snapshot.CoreIdentity = mergeTraitSetConfigs(snapshot.CoreIdentity, cfg)
	}
	if cfg, ok, err := loadOutputPreferencesConfig(ctx, db, ConfigScopeGlobal, ConfigScopeSystem, ConfigKeyOutputPreferences); err != nil {
		return snapshot, err
	} else if ok {
		snapshot.OutputPreferences = mergeOutputPreferences(snapshot.OutputPreferences, cfg)
	}
	if mods, err := listModuleConfigurations(ctx, db, ConfigScopeGlobal, ConfigScopeSystem, 100); err != nil {
		return snapshot, err
	} else {
		snapshot.PersonaModules = append(snapshot.PersonaModules, mods...)
	}

	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		snapshot.RelationshipProfile = normalizeRelationship(snapshot.RelationshipProfile)
		return snapshot, nil
	}

	if cfg, ok, err := loadTraitSetConfig(ctx, db, ConfigScopeOwner, ownerID, ConfigKeyCoreIdentity); err != nil {
		return snapshot, err
	} else if ok {
		snapshot.CoreIdentity = mergeTraitSetConfigs(snapshot.CoreIdentity, cfg)
	}
	if cfg, ok, err := loadOutputPreferencesConfig(ctx, db, ConfigScopeOwner, ownerID, ConfigKeyOutputPreferences); err != nil {
		return snapshot, err
	} else if ok {
		snapshot.OutputPreferences = mergeOutputPreferences(snapshot.OutputPreferences, cfg)
	}
	if rel, ok, err := loadRelationshipProfileConfig(ctx, db, ownerID); err != nil {
		return snapshot, err
	} else if ok {
		snapshot.RelationshipProfile = rel
	}
	if mods, err := listModuleConfigurations(ctx, db, ConfigScopeOwner, ownerID, 100); err != nil {
		return snapshot, err
	} else {
		snapshot.PersonaModules = append(snapshot.PersonaModules, mods...)
	}

	snapshot.CoreIdentity = mergeTraitSetConfigs(TraitSetConfig{Traits: map[string]float64{}}, snapshot.CoreIdentity)
	snapshot.RelationshipProfile = normalizeRelationship(snapshot.RelationshipProfile)
	snapshot.PersonaModules = normalizeModules(snapshot.PersonaModules)
	return snapshot, nil
}

func SaveCoreIdentityConfiguration(ctx context.Context, db *sql.DB, ownerID string, traits map[string]float64, source string) error {
	payload := TraitSetConfig{Traits: sanitizeTraitMap(traits)}
	return saveJSONConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        fmt.Sprintf("experience_core_identity:%s", strings.TrimSpace(ownerID)),
		Scope:     ConfigScopeOwner,
		ScopeID:   strings.TrimSpace(ownerID),
		Key:       ConfigKeyCoreIdentity,
		Source:    normalizeConfigurationSource(source, string(schema.StateKindOwnerSet)),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, payload)
}

func DeleteCoreIdentityConfiguration(ctx context.Context, db *sql.DB, ownerID string) error {
	return store.DeleteConfigurationEntriesByScopeAndKeyPrefix(ctx, db, ConfigScopeOwner, strings.TrimSpace(ownerID), ConfigKeyCoreIdentity)
}

func SaveOutputPreferencesConfiguration(ctx context.Context, db *sql.DB, ownerID string, prefs OutputPreferences, source string) error {
	prefs = sanitizeOutputPreferences(prefs)
	return saveJSONConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        fmt.Sprintf("experience_output_preferences:%s", strings.TrimSpace(ownerID)),
		Scope:     ConfigScopeOwner,
		ScopeID:   strings.TrimSpace(ownerID),
		Key:       ConfigKeyOutputPreferences,
		Source:    normalizeConfigurationSource(source, string(schema.StateKindOwnerSet)),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, prefs)
}

func DeleteOutputPreferencesConfiguration(ctx context.Context, db *sql.DB, ownerID string) error {
	return store.DeleteConfigurationEntriesByScopeAndKeyPrefix(ctx, db, ConfigScopeOwner, strings.TrimSpace(ownerID), ConfigKeyOutputPreferences)
}

func SaveRelationshipProfileConfiguration(ctx context.Context, db *sql.DB, ownerID string, profile RelationshipProfile, source string) error {
	profile = normalizeRelationship(profile)
	return saveJSONConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        fmt.Sprintf("experience_relationship_profile:%s", strings.TrimSpace(ownerID)),
		Scope:     ConfigScopeOwner,
		ScopeID:   strings.TrimSpace(ownerID),
		Key:       ConfigKeyRelationshipProfile,
		Source:    normalizeConfigurationSource(source, string(schema.StateKindInferred)),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, profile)
}

func SaveModuleConfiguration(ctx context.Context, db *sql.DB, scope, scopeID string, module ModuleConfig, source string) error {
	scope = strings.TrimSpace(scope)
	scopeID = strings.TrimSpace(scopeID)
	if scope == "" || scopeID == "" {
		return fmt.Errorf("experience: module configuration requires scope and scope_id")
	}
	modules := normalizeModules([]ModuleConfig{module})
	if len(modules) == 0 {
		return fmt.Errorf("experience: module configuration requires module_id")
	}
	module = modules[0]
	version := strings.TrimSpace(module.Version)
	if version == "" {
		version = "v0.0.0"
		module.Version = version
	}
	keySuffix := fmt.Sprintf("%s@%s", module.ModuleID, version)
	return saveJSONConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        fmt.Sprintf("experience_module:%s:%s:%s", scope, strings.TrimSpace(scopeID), keySuffix),
		Scope:     scope,
		ScopeID:   scopeID,
		Key:       ConfigKeyModulePrefix + keySuffix,
		Source:    normalizeConfigurationSource(source, string(schema.StateKindOwnerSet)),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, module)
}

func ReplaceModuleConfigurations(ctx context.Context, db *sql.DB, scope, scopeID string, modules []ModuleConfig, source string) error {
	scope = strings.TrimSpace(scope)
	scopeID = strings.TrimSpace(scopeID)
	if scope == "" || scopeID == "" {
		return fmt.Errorf("experience: module configuration requires scope and scope_id")
	}
	if err := store.DeleteConfigurationEntriesByScopeAndKeyPrefix(ctx, db, scope, scopeID, ConfigKeyModulePrefix); err != nil {
		return err
	}
	for _, module := range normalizeModules(modules) {
		if err := SaveModuleConfiguration(ctx, db, scope, scopeID, module, source); err != nil {
			return err
		}
	}
	return nil
}

func saveJSONConfigurationEntry(ctx context.Context, db *sql.DB, entry schema.ConfigurationEntry, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("experience: marshal configuration payload: %w", err)
	}
	entry.Value = string(raw)
	return store.SaveConfigurationEntry(ctx, db, entry)
}

func loadTraitSetConfig(ctx context.Context, db *sql.DB, scope, scopeID, key string) (TraitSetConfig, bool, error) {
	var cfg TraitSetConfig
	ok, err := loadJSONConfigurationEntry(ctx, db, scope, scopeID, key, &cfg)
	if cfg.Traits == nil {
		cfg.Traits = map[string]float64{}
	}
	return cfg, ok, err
}

func loadOutputPreferencesConfig(ctx context.Context, db *sql.DB, scope, scopeID, key string) (OutputPreferences, bool, error) {
	var cfg OutputPreferences
	ok, err := loadJSONConfigurationEntry(ctx, db, scope, scopeID, key, &cfg)
	return cfg, ok, err
}

func loadRelationshipProfileConfig(ctx context.Context, db *sql.DB, ownerID string) (RelationshipProfile, bool, error) {
	var profile RelationshipProfile
	ok, err := loadJSONConfigurationEntry(ctx, db, ConfigScopeOwner, ownerID, ConfigKeyRelationshipProfile, &profile)
	return normalizeRelationship(profile), ok, err
}

func loadJSONConfigurationEntry(ctx context.Context, db *sql.DB, scope, scopeID, key string, out any) (bool, error) {
	raw, ok, err := store.GetConfigurationValue(ctx, db, scope, scopeID, key)
	if err != nil || !ok {
		return ok, err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false, fmt.Errorf("experience: decode configuration %s/%s/%s: %w", scope, scopeID, key, err)
	}
	return true, nil
}

func listModuleConfigurations(ctx context.Context, db *sql.DB, scope, scopeID string, limit int) ([]ModuleConfig, error) {
	entries, err := store.ListConfigurationByScope(ctx, db, scope, scopeID, limit)
	if err != nil {
		return nil, err
	}
	modules := make([]ModuleConfig, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Key, ConfigKeyModulePrefix) {
			continue
		}
		var module ModuleConfig
		if err := json.Unmarshal([]byte(entry.Value), &module); err != nil {
			return nil, fmt.Errorf("experience: decode module configuration %s: %w", entry.Key, err)
		}
		module.OriginScope = entry.Scope
		modules = append(modules, module)
	}
	sort.SliceStable(modules, func(i, j int) bool {
		if modules[i].Scope == modules[j].Scope {
			return modules[i].ModuleID < modules[j].ModuleID
		}
		return modules[i].Scope < modules[j].Scope
	})
	return normalizeModules(modules), nil
}

func mergeTraitSetConfigs(base, override TraitSetConfig) TraitSetConfig {
	if base.Traits == nil {
		base.Traits = map[string]float64{}
	}
	for trait, value := range override.Traits {
		if _, ok := traitDefinitions[trait]; !ok {
			continue
		}
		base.Traits[trait] = clamp01(value)
	}
	return base
}

func normalizeConfigurationSource(source, fallback string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return fallback
	}
	return source
}

func sanitizeTraitMap(traits map[string]float64) map[string]float64 {
	if len(traits) == 0 {
		return map[string]float64{}
	}
	clean := map[string]float64{}
	for trait, value := range traits {
		trait = strings.TrimSpace(trait)
		if !IsSupportedTrait(trait) {
			continue
		}
		clean[trait] = clamp01(value)
	}
	return clean
}

func sanitizeOutputPreferences(prefs OutputPreferences) OutputPreferences {
	if prefs.PreferredLength != "" && !IsSupportedPreferredLength(prefs.PreferredLength) {
		prefs.PreferredLength = ""
	}
	if prefs.PreferredFormat != "" && !IsSupportedPreferredFormat(prefs.PreferredFormat) {
		prefs.PreferredFormat = ""
	}
	if prefs.SummaryFirst != nil {
		value := *prefs.SummaryFirst
		prefs.SummaryFirst = &value
	}
	return prefs
}

func appendExperienceSnapshotEvent(ctx context.Context, db *sql.DB, rendered RenderedControl, payload schema.NaviExperienceSnapshotPayload) error {
	stateJSON, err := json.Marshal(rendered.State)
	if err != nil {
		return fmt.Errorf("experience: marshal effective state snapshot: %w", err)
	}
	compiledJSON, err := json.Marshal(rendered.Payload)
	if err != nil {
		return fmt.Errorf("experience: marshal compiled payload snapshot: %w", err)
	}
	payload.SourceStateID = strings.TrimSpace(rendered.State.StateID)
	payload.CompiledPayloadID = strings.TrimSpace(rendered.Payload.PayloadID)
	payload.EffectiveStateJSON = string(stateJSON)
	payload.CompiledPayloadJSON = string(compiledJSON)
	return appendExperienceSnapshotEventFromPayload(ctx, db, payload.ChatID, payload)
}

func appendExperienceSnapshotEventFromPayload(ctx context.Context, db *sql.DB, chatID string, payload schema.NaviExperienceSnapshotPayload) error {
	chatID = strings.TrimSpace(firstNonEmpty(chatID, payload.ChatID))
	if db == nil || chatID == "" {
		return nil
	}
	payload.ChatID = chatID
	payload.OwnerID = strings.TrimSpace(payload.OwnerID)
	payload.ExperienceMode = strings.TrimSpace(payload.ExperienceMode)
	payload.Trigger = strings.TrimSpace(payload.Trigger)
	payload.SourceStateID = strings.TrimSpace(payload.SourceStateID)
	payload.CompiledPayloadID = strings.TrimSpace(payload.CompiledPayloadID)
	if strings.TrimSpace(payload.SnapshotID) == "" {
		payload.SnapshotID = uuid.New().String()
	}
	ev := schema.NewEvent(schema.FactNaviExperienceSnapshot, schema.EventKindFact, chatID, schema.AgentNavi, payload)
	ev.Visibility = schema.VisibilityAudit
	return store.AppendEvent(ctx, db, ev)
}

func decodeExperienceSnapshotPayload(raw any) (schema.NaviExperienceSnapshotPayload, error) {
	switch payload := raw.(type) {
	case schema.NaviExperienceSnapshotPayload:
		return payload, nil
	case *schema.NaviExperienceSnapshotPayload:
		if payload == nil {
			return schema.NaviExperienceSnapshotPayload{}, nil
		}
		return *payload, nil
	case nil:
		return schema.NaviExperienceSnapshotPayload{}, nil
	default:
		buf, err := json.Marshal(raw)
		if err != nil {
			return schema.NaviExperienceSnapshotPayload{}, fmt.Errorf("experience: marshal snapshot payload: %w", err)
		}
		var decoded schema.NaviExperienceSnapshotPayload
		if err := json.Unmarshal(buf, &decoded); err != nil {
			return schema.NaviExperienceSnapshotPayload{}, fmt.Errorf("experience: decode snapshot payload: %w", err)
		}
		return decoded, nil
	}
}

func decodeEffectiveState(raw string) (EffectivePersonaState, error) {
	if strings.TrimSpace(raw) == "" {
		return EffectivePersonaState{ResolvedTraits: map[string]TraitValueState{}}, nil
	}
	var state EffectivePersonaState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return EffectivePersonaState{}, fmt.Errorf("experience: decode effective state: %w", err)
	}
	if state.ResolvedTraits == nil {
		state.ResolvedTraits = map[string]TraitValueState{}
	}
	return state, nil
}

func cumulativeTraitDelta(prev, next EffectivePersonaState) float64 {
	traitNames := make(map[string]struct{}, len(traitDefinitions))
	for trait := range traitDefinitions {
		traitNames[trait] = struct{}{}
	}
	for trait := range prev.ResolvedTraits {
		traitNames[trait] = struct{}{}
	}
	for trait := range next.ResolvedTraits {
		traitNames[trait] = struct{}{}
	}
	total := 0.0
	for trait := range traitNames {
		total += math.Abs(resolvedTraitValue(prev, trait) - resolvedTraitValue(next, trait))
	}
	return total
}

func resolvedTraitValue(state EffectivePersonaState, trait string) float64 {
	if resolved, ok := state.ResolvedTraits[trait]; ok {
		return clamp01(resolved.Value)
	}
	if def, ok := traitDefinitions[trait]; ok {
		return def.DefaultValue
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
