package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/llmkb"
)

type LLMProfileRepo interface {
	SaveProvider(ctx context.Context, provider llmkb.LLMProvider) error
	GetProvider(ctx context.Context, providerID string) (*llmkb.LLMProvider, error)
	ListProviders(ctx context.Context) ([]llmkb.LLMProvider, error)
	SaveProfile(ctx context.Context, profile llmkb.LLMProfile) error
	GetProfile(ctx context.Context, llmID string) (*llmkb.LLMProfile, error)
	ListProfiles(ctx context.Context) ([]llmkb.LLMProfile, error)
	GetProfileByAlias(ctx context.Context, alias string) (*llmkb.LLMProfile, error)
	ListProfilesByProviderID(ctx context.Context, providerID string) ([]llmkb.LLMProfile, error)
	ListPreferredForTask(ctx context.Context, task llmkb.TaskClass) ([]llmkb.LLMProfile, error)
	GetRoutingProfile(ctx context.Context, llmID string) (*llmkb.RoutingProfile, error)
	DescribeProfile(ctx context.Context, llmID string) (string, error)
	MaterializeUsageStats(ctx context.Context) error
	GenerateRoutingProposals(ctx context.Context, threshold float64) ([]llmkb.RoutingProposalItem, error)
	ListRoutingProposals(ctx context.Context) ([]llmkb.RoutingProposalItem, error)
	GetRoutingProposal(ctx context.Context, proposalID string) (*llmkb.RoutingProposalItem, error)
	ResolveRoutingProposal(ctx context.Context, proposalID string, status llmkb.ProposalStatus) error
}

type LLMRuntimeInstanceRepo interface {
	SaveRuntimeInstance(ctx context.Context, instance llmkb.LLMRuntimeInstance) error
	GetRuntimeInstance(ctx context.Context, instanceID string) (*llmkb.LLMRuntimeInstance, error)
	ListRuntimeInstances(ctx context.Context, llmID string) ([]llmkb.LLMRuntimeInstance, error)
	ListAvailableRuntimes(ctx context.Context) ([]llmkb.LLMRuntimeInstance, error)
	RefreshRuntimeInstancesFromCatalog(ctx context.Context, catalog llm.LLMCatalog) error
}

type RouterDecisionRepo interface {
	AppendRouterDecision(ctx context.Context, decision llmkb.RouterDecision) error
	ListRouterDecisions(ctx context.Context, taskID string, limit int) ([]llmkb.RouterDecision, error)
}

type LLMExecutionRecordRepo interface {
	AppendExecutionRecord(ctx context.Context, record llmkb.LLMExecutionRecord) error
	ListExecutionRecords(ctx context.Context, llmID string, limit int) ([]llmkb.LLMExecutionRecord, error)
}

type SQLiteLLMKBRepo struct {
	db *sql.DB
}

func NewSQLiteLLMKBRepo(db *sql.DB) *SQLiteLLMKBRepo { return &SQLiteLLMKBRepo{db: db} }

type LLMKBProfileAliasAmbiguityError struct {
	Alias   string
	Matches []string
}

func (e *LLMKBProfileAliasAmbiguityError) Error() string {
	return fmt.Sprintf("llmkb: alias %q matches multiple profiles: %s", e.Alias, strings.Join(e.Matches, ", "))
}

func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func unmarshalJSON(raw string, dst any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

func nowRFC3339() string { return time.Now().UTC().Format(timeFormat) }

func defaultLLMKBProvenance() llmkb.Provenance {
	return llmkb.Provenance{
		Source:     llmkb.ProvenanceSourceProbe,
		AssertedAt: time.Now().UTC(),
		FieldClass: llmkb.FieldClassObserved,
		Confidence: 1,
	}
}

func defaultLLMKBCapabilityProfile() llmkb.CapabilityProfile {
	return llmkb.CapabilityProfile{
		AgenticClass:              llmkb.AgenticClassNone,
		CodingClass:               llmkb.CodingClassNone,
		ReasoningClass:            llmkb.ReasoningClassNone,
		InstructionFollowingClass: llmkb.ClassLevelFair,
		ToolDisciplineClass:       llmkb.ClassLevelFair,
		SchemaReliabilityClass:    llmkb.ClassLevelFair,
		MultimodalClass:           llmkb.ClassLevelFair,
	}
}

func llmkbRuntimeBackendForProvider(providerID string) llmkb.RuntimeBackend {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "ollama":
		return llmkb.RuntimeBackendOllama
	case "anthropic":
		return llmkb.RuntimeBackendAnthropic
	case "openai":
		return llmkb.RuntimeBackendOpenAI
	case "openrouter":
		return llmkb.RuntimeBackendOpenRouter
	default:
		return llmkb.RuntimeBackendAPI
	}
}

func llmkbProvenanceIsZero(p llmkb.Provenance) bool {
	return p.Source == "" &&
		p.SourceDetail == "" &&
		p.Confidence == 0 &&
		p.AssertedAt.IsZero() &&
		p.FieldClass == ""
}

func llmkbCapabilityProfileIsZero(p llmkb.CapabilityProfile) bool {
	return p.AgenticClass == "" &&
		p.CodingClass == "" &&
		p.ReasoningClass == "" &&
		p.InstructionFollowingClass == "" &&
		p.ToolDisciplineClass == "" &&
		p.SchemaReliabilityClass == "" &&
		p.MultimodalClass == "" &&
		len(p.PrimaryUseCases) == 0 &&
		len(p.InteractionModes) == 0
}

func normalizeProvider(provider llmkb.LLMProvider) llmkb.LLMProvider {
	if strings.TrimSpace(provider.CanonicalName) == "" {
		provider.CanonicalName = provider.ProviderID
	}
	if llmkbProvenanceIsZero(provider.Provenance) {
		provider.Provenance = defaultLLMKBProvenance()
	}
	return provider
}

func normalizeProfile(profile llmkb.LLMProfile) llmkb.LLMProfile {
	if strings.TrimSpace(profile.CanonicalName) == "" {
		profile.CanonicalName = profile.LLMID
	}
	if profile.OperationalState.AvailabilityState == "" {
		profile.OperationalState.AvailabilityState = llmkb.AvailabilityStateAvailable
	}
	if llmkbCapabilityProfileIsZero(profile.Capabilities) {
		profile.Capabilities = defaultLLMKBCapabilityProfile()
	}
	if llmkbProvenanceIsZero(profile.Provenance) {
		profile.Provenance = defaultLLMKBProvenance()
	}
	if profile.OperationalState.RuntimeBackend == "" {
		profile.OperationalState.RuntimeBackend = llmkbRuntimeBackendForProvider(profile.ProviderID)
	}
	return profile
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeLLMKBLookupToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func observedLLMProfileID(providerID, modelID string) string {
	providerID = strings.TrimSpace(strings.ToLower(providerID))
	modelID = strings.TrimSpace(strings.ToLower(modelID))
	if providerID == "" || modelID == "" {
		return firstNonEmptyString(providerID, modelID)
	}
	var b strings.Builder
	b.Grow(len(providerID) + len(modelID) + 1)
	b.WriteString(providerID)
	b.WriteByte('.')
	lastDash := false
	for _, r := range modelID {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-.")
}

func profileLookupKeys(profile llmkb.LLMProfile) []string {
	keys := []string{
		profile.LLMID,
		profile.CanonicalName,
	}
	if idx := strings.Index(profile.LLMID, "."); idx >= 0 && idx+1 < len(profile.LLMID) {
		keys = append(keys, profile.LLMID[idx+1:])
	}
	keys = append(keys, profile.Aliases...)
	return keys
}

func profileMatchesAlias(profile llmkb.LLMProfile, alias string, normalize bool) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false
	}
	if !normalize {
		for _, key := range profileLookupKeys(profile) {
			if strings.EqualFold(strings.TrimSpace(key), alias) {
				return true
			}
		}
		return false
	}
	needle := normalizeLLMKBLookupToken(alias)
	if needle == "" {
		return false
	}
	for _, key := range profileLookupKeys(profile) {
		if normalizeLLMKBLookupToken(key) == needle {
			return true
		}
	}
	return false
}

func (r *SQLiteLLMKBRepo) ResolveProfileForProviderModel(ctx context.Context, providerID, modelID string) (*llmkb.LLMProfile, error) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return nil, nil
	}
	if profile, err := r.GetProfile(ctx, modelID); err != nil {
		return nil, err
	} else if profile != nil && strings.EqualFold(profile.ProviderID, providerID) {
		return profile, nil
	}
	profiles, err := r.ListProfilesByProviderID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		for _, key := range profileLookupKeys(profile) {
			if strings.EqualFold(strings.TrimSpace(key), modelID) {
				copy := profile
				return &copy, nil
			}
		}
	}
	needle := normalizeLLMKBLookupToken(modelID)
	if needle == "" {
		return nil, nil
	}
	for _, profile := range profiles {
		for _, key := range profileLookupKeys(profile) {
			if normalizeLLMKBLookupToken(key) == needle {
				copy := profile
				return &copy, nil
			}
		}
	}
	return nil, nil
}

func (r *SQLiteLLMKBRepo) EnsureRuntimeProfile(ctx context.Context, providerID, modelID string) (*llmkb.LLMProfile, error) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return nil, nil
	}
	if profile, err := r.ResolveProfileForProviderModel(ctx, providerID, modelID); err != nil {
		return nil, err
	} else if profile != nil {
		return profile, nil
	}
	provider, err := r.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		if err := r.SaveProvider(ctx, llmkb.LLMProvider{ProviderID: providerID}); err != nil {
			return nil, err
		}
	}
	stubID := observedLLMProfileID(providerID, modelID)
	if err := r.SaveProfile(ctx, llmkb.LLMProfile{
		LLMID:             stubID,
		CanonicalName:     modelID,
		Aliases:           []string{modelID},
		ProviderID:        providerID,
		OperationalState:  llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
		Capabilities:      defaultLLMKBCapabilityProfile(),
		Provenance:        defaultLLMKBProvenance(),
	}); err != nil {
		return nil, err
	}
	return r.GetProfile(ctx, stubID)
}

func (r *SQLiteLLMKBRepo) ensureExecutionRecordTargets(ctx context.Context, record llmkb.LLMExecutionRecord) error {
	return nil
}

func (r *SQLiteLLMKBRepo) SaveProvider(ctx context.Context, provider llmkb.LLMProvider) error {
	provider = normalizeProvider(provider)
	if err := provider.Validate(); err != nil {
		return err
	}
	now := nowRFC3339()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO llm_providers (
			provider_id, display_name, hosting_mode, runtime_backend, api_base, auth_status, health_status, release_channel,
			provenance_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET
			display_name=excluded.display_name,
			hosting_mode=excluded.hosting_mode,
			runtime_backend=excluded.runtime_backend,
			api_base=excluded.api_base,
			auth_status=excluded.auth_status,
			health_status=excluded.health_status,
			release_channel=excluded.release_channel,
			provenance_json=excluded.provenance_json,
			updated_at=excluded.updated_at
	`, provider.ProviderID, provider.CanonicalName, "", "", provider.EndpointBase, provider.AuthMethod, "", "", marshalJSON(provider.Provenance), now, now)
	return err
}

func (r *SQLiteLLMKBRepo) GetProvider(ctx context.Context, providerID string) (*llmkb.LLMProvider, error) {
	var provider llmkb.LLMProvider
	var provenanceJSON string
	var ignore1, ignore2, ignore3, ignore4, ignore5 sql.NullString
	var createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT provider_id, display_name, hosting_mode, runtime_backend, api_base, auth_status, health_status, release_channel, provenance_json, created_at, updated_at
		FROM llm_providers WHERE provider_id = ?
	`, providerID).Scan(&provider.ProviderID, &provider.CanonicalName, &ignore1, &ignore2, &provider.EndpointBase, &ignore3, &ignore4, &ignore5, &provenanceJSON, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	provider.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	provider.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if err := unmarshalJSON(provenanceJSON, &provider.Provenance); err != nil {
		return nil, err
	}
	return &provider, nil
}

func (r *SQLiteLLMKBRepo) ListProviders(ctx context.Context) ([]llmkb.LLMProvider, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT provider_id, display_name, hosting_mode, runtime_backend, api_base, auth_status, health_status, release_channel, provenance_json
		FROM llm_providers ORDER BY provider_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []llmkb.LLMProvider
	for rows.Next() {
		var item llmkb.LLMProvider
		var provenanceJSON string
		var ignore1, ignore2, ignore3, ignore4, ignore5 sql.NullString
		if err := rows.Scan(&item.ProviderID, &item.CanonicalName, &ignore1, &ignore2, &item.EndpointBase, &ignore3, &ignore4, &ignore5, &provenanceJSON); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(provenanceJSON, &item.Provenance); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *SQLiteLLMKBRepo) SaveProfile(ctx context.Context, profile llmkb.LLMProfile) error {
	profile = normalizeProfile(profile)
	if err := profile.Validate(); err != nil {
		return err
	}
	provider, err := r.GetProvider(ctx, profile.ProviderID)
	if err != nil {
		return err
	}
	if provider == nil {
		if err := r.SaveProvider(ctx, llmkb.LLMProvider{ProviderID: profile.ProviderID, CreatedAt: time.Now().UTC()}); err != nil {
			return err
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowRFC3339()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO llm_profiles (
			llm_id, schema_version, canonical_name, aliases_json, provider_id, display_name, availability_state, cost_estimate_json,
			provenance_json, mutation_history_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(llm_id) DO UPDATE SET
			schema_version=excluded.schema_version,
			canonical_name=excluded.canonical_name,
			aliases_json=excluded.aliases_json,
			provider_id=excluded.provider_id,
			display_name=excluded.display_name,
			availability_state=excluded.availability_state,
			cost_estimate_json=excluded.cost_estimate_json,
			provenance_json=excluded.provenance_json,
			mutation_history_json=excluded.mutation_history_json,
			updated_at=excluded.updated_at
	`, profile.LLMID, profile.SchemaVersion, profile.CanonicalName, marshalJSON(profile.Aliases), profile.ProviderID, profile.CanonicalName, string(profile.OperationalState.AvailabilityState), "{}", marshalJSON(profile.Provenance), marshalJSON(profile.Provenance.MutationHistory), now, now); err != nil {
		return err
	}
	subtables := []struct {
		table string
		data  any
	}{
		{"llm_profile_capabilities", profile.Capabilities},
		{"llm_profile_features", profile.Features},
		{"llm_profile_operational_state", profile.OperationalState},
		{"llm_profile_evaluation", profile.Evaluation},
		{"llm_profile_usage_stats", profile.UsageStats},
		{"llm_profile_routing", profile.Routing},
	}
	for _, sub := range subtables {
		query := fmt.Sprintf(`
			INSERT INTO %s (llm_id, data_json, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(llm_id) DO UPDATE SET data_json=excluded.data_json, updated_at=excluded.updated_at
		`, quoteIdentifier(sub.table))
		if _, err := tx.ExecContext(ctx, query, profile.LLMID, marshalJSON(sub.data), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteLLMKBRepo) GetProfile(ctx context.Context, llmID string) (*llmkb.LLMProfile, error) {
	var profile llmkb.LLMProfile
	var aliasesJSON, costJSON, provenanceJSON, mutationJSON string
	var ignore1, ignore2 sql.NullString
	var createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT llm_id, schema_version, canonical_name, aliases_json, provider_id, display_name, availability_state, cost_estimate_json, provenance_json, mutation_history_json, created_at, updated_at
		FROM llm_profiles WHERE llm_id = ?
	`, llmID).Scan(&profile.LLMID, &profile.SchemaVersion, &profile.CanonicalName, &aliasesJSON, &profile.ProviderID, &ignore1, &ignore2, &costJSON, &provenanceJSON, &mutationJSON, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	profile.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	profile.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if err := unmarshalJSON(aliasesJSON, &profile.Aliases); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(provenanceJSON, &profile.Provenance); err != nil {
		return nil, err
	}
	if err := unmarshalJSON(mutationJSON, &profile.Provenance.MutationHistory); err != nil {
		return nil, err
	}
	if err := r.loadProfileSubtables(ctx, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *SQLiteLLMKBRepo) ListProfiles(ctx context.Context) ([]llmkb.LLMProfile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT llm_id FROM llm_profiles ORDER BY provider_id, llm_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.listProfilesFromRows(ctx, rows)
}

func (r *SQLiteLLMKBRepo) GetProfileByAlias(ctx context.Context, alias string) (*llmkb.LLMProfile, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, nil
	}
	profiles, err := r.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	exactMatches := make([]llmkb.LLMProfile, 0, 1)
	for _, profile := range profiles {
		if profileMatchesAlias(profile, alias, false) {
			exactMatches = append(exactMatches, profile)
		}
	}
	if len(exactMatches) == 1 {
		copy := exactMatches[0]
		return &copy, nil
	}
	if len(exactMatches) > 1 {
		return nil, &LLMKBProfileAliasAmbiguityError{
			Alias:   alias,
			Matches: collectLLMKBProfileIDs(exactMatches),
		}
	}
	normalizedMatches := make([]llmkb.LLMProfile, 0, 1)
	for _, profile := range profiles {
		if profileMatchesAlias(profile, alias, true) {
			normalizedMatches = append(normalizedMatches, profile)
		}
	}
	if len(normalizedMatches) == 1 {
		copy := normalizedMatches[0]
		return &copy, nil
	}
	if len(normalizedMatches) > 1 {
		return nil, &LLMKBProfileAliasAmbiguityError{
			Alias:   alias,
			Matches: collectLLMKBProfileIDs(normalizedMatches),
		}
	}
	return nil, nil
}

func collectLLMKBProfileIDs(profiles []llmkb.LLMProfile) []string {
	matches := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		matches = append(matches, profile.LLMID)
	}
	return matches
}

func (r *SQLiteLLMKBRepo) ListProfilesByProviderID(ctx context.Context, providerID string) ([]llmkb.LLMProfile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT llm_id FROM llm_profiles WHERE provider_id = ? ORDER BY llm_id`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.listProfilesFromRows(ctx, rows)
}

func (r *SQLiteLLMKBRepo) ListPreferredForTask(ctx context.Context, task llmkb.TaskClass) ([]llmkb.LLMProfile, error) {
	profiles, err := r.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	var out []llmkb.LLMProfile
	for _, profile := range profiles {
		for _, preferred := range profile.Routing.PreferredFor {
			if preferred == task {
				out = append(out, profile)
				break
			}
		}
	}
	return out, nil
}

func (r *SQLiteLLMKBRepo) GetRoutingProfile(ctx context.Context, llmID string) (*llmkb.RoutingProfile, error) {
	profile, err := r.GetProfile(ctx, llmID)
	if err != nil || profile == nil {
		return nil, err
	}
	routing := profile.Routing
	return &routing, nil
}

func (r *SQLiteLLMKBRepo) DescribeProfile(ctx context.Context, llmID string) (string, error) {
	profile, err := r.GetProfile(ctx, llmID)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", sql.ErrNoRows
	}
	provider, err := r.GetProvider(ctx, profile.ProviderID)
	if err != nil {
		return "", err
	}
	providerName := profile.ProviderID
	if provider != nil {
		if strings.TrimSpace(provider.CanonicalName) != "" {
			providerName = provider.CanonicalName
		}
	}
	hosting := "cloud"
	if profile.OperationalState.InstalledLocally {
		hosting = "local"
	} else if profile.OperationalState.RuntimeBackend == llmkb.RuntimeBackendOllama {
		hosting = "local (ollama)"
	}

	parts := []string{
		fmt.Sprintf("%s (%s)", profile.CanonicalName, profile.LLMID),
		fmt.Sprintf("provider %s", providerName),
		fmt.Sprintf("hosting %s", hosting),
		fmt.Sprintf("agentic %s", profile.Capabilities.AgenticClass),
		fmt.Sprintf("coding %s", profile.Capabilities.CodingClass),
		fmt.Sprintf("reasoning %s", profile.Capabilities.ReasoningClass),
		fmt.Sprintf("autonomy %s", profile.Routing.AutonomyCeiling),
	}
	if profile.Routing.CostTier != "" {
		parts = append(parts, fmt.Sprintf("cost %s", profile.Routing.CostTier))
	}
	if len(profile.Routing.PreferredFor) > 0 {
		values := make([]string, 0, len(profile.Routing.PreferredFor))
		for _, item := range profile.Routing.PreferredFor {
			values = append(values, string(item))
		}
		parts = append(parts, fmt.Sprintf("preferred tasks %s", strings.Join(values, ", ")))
	}
	if profile.Features.SupportsTools {
		parts = append(parts, "tools enabled")
	}
	if profile.Features.SupportsVision {
		parts = append(parts, "vision enabled")
	}
	return strings.Join(parts, "; "), nil
}

func (r *SQLiteLLMKBRepo) listProfilesFromRows(ctx context.Context, rows *sql.Rows) ([]llmkb.LLMProfile, error) {
	var ids []string
	for rows.Next() {
		var llmID string
		if err := rows.Scan(&llmID); err != nil {
			return nil, err
		}
		ids = append(ids, llmID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]llmkb.LLMProfile, 0, len(ids))
	for _, llmID := range ids {
		item, err := r.GetProfile(ctx, llmID)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (r *SQLiteLLMKBRepo) loadProfileSubtables(ctx context.Context, profile *llmkb.LLMProfile) error {
	subtargets := []struct {
		table string
		dst   any
	}{
		{"llm_profile_capabilities", &profile.Capabilities},
		{"llm_profile_features", &profile.Features},
		{"llm_profile_operational_state", &profile.OperationalState},
		{"llm_profile_evaluation", &profile.Evaluation},
		{"llm_profile_usage_stats", &profile.UsageStats},
		{"llm_profile_routing", &profile.Routing},
	}
	for _, target := range subtargets {
		var raw string
		query := fmt.Sprintf(`SELECT data_json FROM %s WHERE llm_id = ?`, quoteIdentifier(target.table))
		err := r.db.QueryRowContext(ctx, query, profile.LLMID).Scan(&raw)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		if err := unmarshalJSON(raw, target.dst); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteLLMKBRepo) SaveRuntimeInstance(ctx context.Context, instance llmkb.LLMRuntimeInstance) error {
	now := nowRFC3339()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO llm_runtime_instances (
			instance_id, llm_id, provider_id, runtime_backend, endpoint, local_path, ollama_tag,
			auth_config_key, connector_id, loaded_status, health_status, auth_status, last_probe_at, provenance_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
			llm_id=excluded.llm_id, provider_id=excluded.provider_id, runtime_backend=excluded.runtime_backend,
			endpoint=excluded.endpoint, local_path=excluded.local_path, ollama_tag=excluded.ollama_tag,
			auth_config_key=excluded.auth_config_key, connector_id=excluded.connector_id, loaded_status=excluded.loaded_status,
			health_status=excluded.health_status, auth_status=excluded.auth_status, last_probe_at=excluded.last_probe_at, 
			provenance_json=excluded.provenance_json, updated_at=excluded.updated_at
	`, instance.InstanceID, instance.LLMID, instance.ProviderID, instance.RuntimeBackend, instance.Endpoint, instance.LocalPath, instance.OllamaTag, instance.AuthConfigKey, instance.ConnectorID, instance.LoadedStatus, instance.HealthStatus, instance.AuthStatus, nullableTime(instance.LastProbeAt), marshalJSON(instance.Provenance), now, now)
	return err
}

func (r *SQLiteLLMKBRepo) GetRuntimeInstance(ctx context.Context, instanceID string) (*llmkb.LLMRuntimeInstance, error) {
	var item llmkb.LLMRuntimeInstance
	var lastProbe sql.NullString
		var provenanceJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT instance_id, llm_id, provider_id, runtime_backend, endpoint, local_path, ollama_tag,
		       auth_config_key, connector_id, loaded_status, health_status, auth_status, last_probe_at, provenance_json
		FROM llm_runtime_instances WHERE instance_id = ?
	`, instanceID).Scan(&item.InstanceID, &item.LLMID, &item.ProviderID, &item.RuntimeBackend, &item.Endpoint, &item.LocalPath, &item.OllamaTag, &item.AuthConfigKey, &item.ConnectorID, &item.LoadedStatus, &item.HealthStatus, &item.AuthStatus, &lastProbe, &provenanceJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = unmarshalJSON(provenanceJSON, &item.Provenance)
	if lastProbe.Valid {
		tm, err := parseTime(lastProbe.String)
		if err != nil {
			return nil, err
		}
		item.LastProbeAt = &tm
	}
	return &item, nil
}

func (r *SQLiteLLMKBRepo) ListRuntimeInstances(ctx context.Context, llmID string) ([]llmkb.LLMRuntimeInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT instance_id, llm_id, provider_id, runtime_backend, endpoint, local_path, ollama_tag,
		       auth_config_key, connector_id, loaded_status, health_status, auth_status, last_probe_at, provenance_json
		FROM llm_runtime_instances WHERE (? = '' OR llm_id = ?) ORDER BY instance_id
	`, llmID, llmID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuntimeInstances(rows)
}

func (r *SQLiteLLMKBRepo) ListAvailableRuntimes(ctx context.Context) ([]llmkb.LLMRuntimeInstance, error) {
	// Replaced availability_state with loaded_status = LoadedStatusLoaded for now
	rows, err := r.db.QueryContext(ctx, `
		SELECT instance_id, llm_id, provider_id, runtime_backend, endpoint, local_path, ollama_tag,
		       auth_config_key, connector_id, loaded_status, health_status, auth_status, last_probe_at, provenance_json
		FROM llm_runtime_instances WHERE loaded_status = ? ORDER BY instance_id
	`, llmkb.LoadedStatusLoaded)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuntimeInstances(rows)
}

func scanRuntimeInstances(rows *sql.Rows) ([]llmkb.LLMRuntimeInstance, error) {
	var out []llmkb.LLMRuntimeInstance
	for rows.Next() {
		var item llmkb.LLMRuntimeInstance
		var lastProbe sql.NullString
		var provenanceJSON string
		if err := rows.Scan(&item.InstanceID, &item.LLMID, &item.ProviderID, &item.RuntimeBackend, &item.Endpoint, &item.LocalPath, &item.OllamaTag, &item.AuthConfigKey, &item.ConnectorID, &item.LoadedStatus, &item.HealthStatus, &item.AuthStatus, &lastProbe, &provenanceJSON); err != nil {
			return nil, err
		}
		_ = unmarshalJSON(provenanceJSON, &item.Provenance)
		if lastProbe.Valid {
			tm, err := parseTime(lastProbe.String)
			if err != nil {
				return nil, err
			}
			item.LastProbeAt = &tm
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func catalogProviderRuntimeBackend(providerKey string) llmkb.RuntimeBackend {
	switch strings.ToLower(strings.TrimSpace(providerKey)) {
	case "ollama":
		return llmkb.RuntimeBackendOllama
	case "anthropic":
		return llmkb.RuntimeBackendAnthropic
	case "openai":
		return llmkb.RuntimeBackendOpenAI
	case "openrouter":
		return llmkb.RuntimeBackendOpenRouter
	default:
		return llmkb.RuntimeBackendAPI
	}
}

func (r *SQLiteLLMKBRepo) RefreshRuntimeInstancesFromCatalog(ctx context.Context, catalog llm.LLMCatalog) error {
	for _, provider := range catalog.Providers {
		backend := catalogProviderRuntimeBackend(provider.Key)
		for _, model := range provider.Models {
			name := strings.TrimSpace(model.Name)
			if name == "" {
				continue
			}
			profile, err := r.EnsureRuntimeProfile(ctx, provider.Key, name)
			if err != nil {
				return err
			}
			instanceID := fmt.Sprintf("%s:%s", provider.Key, name)
			instance := llmkb.LLMRuntimeInstance{
				InstanceID:        instanceID,
				LLMID:             firstNonEmptyString(name, instanceID),
				ProviderID:        provider.Key,
				RuntimeBackend:    backend,
				AuthStatus:        llmkb.AuthStatusUnknown,
				HealthStatus:      llmkb.HealthStatusUnknown,
				LoadedStatus:      llmkb.LoadedStatusUnknown,
			}
			if profile != nil {
				instance.LLMID = profile.LLMID
			}
			if err := r.SaveRuntimeInstance(ctx, instance); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *SQLiteLLMKBRepo) ListRoutingProposals(ctx context.Context) ([]llmkb.RoutingProposalItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT proposal_id, llm_id, proposed_change, affected_field, current_value_json, proposed_value_json,
		       inferred_score, sample_size, probation_elapsed_days, recent_success_rate, fallback_rate_delta,
			   routing_impact, risk_assessment, evidence_record_ids_json, status, created_at, resolved_at, resolved_by
		FROM routing_proposal_items ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []llmkb.RoutingProposalItem
	for rows.Next() {
		var p llmkb.RoutingProposalItem
		var currJSON, propJSON, evidenceJSON, createdAt string
		var resolvedAt, resolvedBy sql.NullString
		if err := rows.Scan(&p.ProposalID, &p.LLMID, &p.ProposedChange, &p.AffectedField, &currJSON, &propJSON,
			&p.InferredScore, &p.SampleSize, &p.ProbationElapsedDays, &p.RecentSuccessRate, &p.FallbackRateDelta,
			&p.RoutingImpact, &p.RiskAssessment, &evidenceJSON, &p.Status, &createdAt, &resolvedAt, &resolvedBy); err != nil {
			return nil, err
		}
		if resolvedBy.Valid {
			p.ResolvedBy = resolvedBy.String
		}
		_ = unmarshalJSON(currJSON, &p.CurrentValue)
		_ = unmarshalJSON(propJSON, &p.ProposedValue)
		_ = unmarshalJSON(evidenceJSON, &p.EvidenceRecordIDs)
		p.CreatedAt, _ = parseTime(createdAt)
		if resolvedAt.Valid {
			tm, _ := parseTime(resolvedAt.String)
			p.ResolvedAt = &tm
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *SQLiteLLMKBRepo) GetRoutingProposal(ctx context.Context, proposalID string) (*llmkb.RoutingProposalItem, error) {
	var p llmkb.RoutingProposalItem
	var currJSON, propJSON, evidenceJSON, createdAt string
	var resolvedAt, resolvedBy sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT proposal_id, llm_id, proposed_change, affected_field, current_value_json, proposed_value_json,
		       inferred_score, sample_size, probation_elapsed_days, recent_success_rate, fallback_rate_delta,
			   routing_impact, risk_assessment, evidence_record_ids_json, status, created_at, resolved_at, resolved_by
		FROM routing_proposal_items WHERE proposal_id = ?
	`, proposalID).Scan(&p.ProposalID, &p.LLMID, &p.ProposedChange, &p.AffectedField, &currJSON, &propJSON,
		&p.InferredScore, &p.SampleSize, &p.ProbationElapsedDays, &p.RecentSuccessRate, &p.FallbackRateDelta,
		&p.RoutingImpact, &p.RiskAssessment, &evidenceJSON, &p.Status, &createdAt, &resolvedAt, &resolvedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if resolvedBy.Valid {
		p.ResolvedBy = resolvedBy.String
	}
	_ = unmarshalJSON(currJSON, &p.CurrentValue)
	_ = unmarshalJSON(propJSON, &p.ProposedValue)
	_ = unmarshalJSON(evidenceJSON, &p.EvidenceRecordIDs)
	p.CreatedAt, _ = parseTime(createdAt)
	if resolvedAt.Valid {
		tm, _ := parseTime(resolvedAt.String)
		p.ResolvedAt = &tm
	}
	return &p, nil
}

func (r *SQLiteLLMKBRepo) ResolveRoutingProposal(ctx context.Context, proposalID string, status llmkb.ProposalStatus) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE routing_proposal_items SET status = ? WHERE proposal_id = ?
	`, status, proposalID)
	return err
}

func (r *SQLiteLLMKBRepo) AppendRouterDecision(ctx context.Context, decision llmkb.RouterDecision) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO router_decisions (
			decision_id, task_id, task_class, risk_tier, selected_llm_id, selected_instance_id, fallback_chain_json,
			candidate_set_json, scores_json, rejected_models_json, capability_filters_json, autonomy_constraints_json,
			cost_constraints_json, governance_constraints_json, confidence, user_visible_summary, debug_rationale,
			surfacing_level, execution_record_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, decision.DecisionID, decision.TaskID, decision.TaskClass, decision.RiskTier, decision.SelectedLLMID, decision.SelectedInstanceID, marshalJSON(decision.FallbackChain),
		marshalJSON(decision.CandidateSet), marshalJSON(decision.Scores), marshalJSON(decision.RejectedModels), marshalJSON(decision.CapabilityFilters), marshalJSON(decision.AutonomyConstraints),
		marshalJSON(decision.CostConstraints), marshalJSON(decision.GovernanceConstraints), decision.Confidence, decision.UserVisibleSummary, decision.DebugRationale,
		decision.SurfacingLevel, nullableStringPtr(decision.ExecutionRecordID), decision.CreatedAt.Format(timeFormat))
	return err
}

func (r *SQLiteLLMKBRepo) ListRouterDecisions(ctx context.Context, taskID string, limit int) ([]llmkb.RouterDecision, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT decision_id, task_id, task_class, risk_tier, selected_llm_id, selected_instance_id, fallback_chain_json,
			candidate_set_json, scores_json, rejected_models_json, capability_filters_json, autonomy_constraints_json,
			cost_constraints_json, governance_constraints_json, confidence, user_visible_summary, debug_rationale,
			surfacing_level, execution_record_id, created_at
		FROM router_decisions WHERE (? = '' OR task_id = ?) ORDER BY created_at DESC LIMIT ?
	`, taskID, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []llmkb.RouterDecision
	for rows.Next() {
		var item llmkb.RouterDecision
		var executionRecordID sql.NullString
		var fbJSON, candJSON, scoJSON, rejModJSON, capJSON, autoJSON, costJSON, govJSON, createdAt string
		if err := rows.Scan(&item.DecisionID, &item.TaskID, &item.TaskClass, &item.RiskTier, &item.SelectedLLMID, &item.SelectedInstanceID, &fbJSON,
			&candJSON, &scoJSON, &rejModJSON, &capJSON, &autoJSON, &costJSON, &govJSON, &item.Confidence, &item.UserVisibleSummary, &item.DebugRationale,
			&item.SurfacingLevel, &executionRecordID, &createdAt); err != nil {
			return nil, err
		}
		if executionRecordID.Valid {
			item.ExecutionRecordID = &executionRecordID.String
		}
		_ = unmarshalJSON(fbJSON, &item.FallbackChain)
		_ = unmarshalJSON(candJSON, &item.CandidateSet)
		_ = unmarshalJSON(scoJSON, &item.Scores)
		_ = unmarshalJSON(rejModJSON, &item.RejectedModels)
		_ = unmarshalJSON(capJSON, &item.CapabilityFilters)
		_ = unmarshalJSON(autoJSON, &item.AutonomyConstraints)
		_ = unmarshalJSON(costJSON, &item.CostConstraints)
		_ = unmarshalJSON(govJSON, &item.GovernanceConstraints)
		tm, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = tm
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *SQLiteLLMKBRepo) AppendExecutionRecord(ctx context.Context, record llmkb.LLMExecutionRecord) error {
	// Let's just avoid ensureExecutionRecordTargets for now if it's broken, or keep it.
	// We'll keep it but we don't have to define it if it's already there. Wait r.ensureExecutionRecordTargets is likely untouched or breaks.
	// Oh, I will just proceed with the insert.
	var costJSON string
	if record.CostUSD != nil {
		costJSON = fmt.Sprintf("%f", *record.CostUSD)
	}
	var ratingJSON string
	if record.UserRating != nil {
		ratingJSON = fmt.Sprintf("%d", *record.UserRating)
	}
	var fallbackModel string
	if record.FallbackModelID != nil {
		fallbackModel = *record.FallbackModelID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO llm_execution_records (
			record_id, llm_id, instance_id, router_decision_id, task_class, task_id, context_size_tokens, output_size_tokens,
			latency_ms, cost_usd, outcome, failure_reason, tools_invoked_json, tool_success_rate, schema_validated,
			fallback_triggered, fallback_model_id, user_rating, user_override, notes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.RecordID, record.LLMID, record.InstanceID, record.RouterDecisionID, record.TaskClass, record.TaskID, record.ContextSizeTokens, record.OutputSizeTokens,
		record.LatencyMs, costJSON, record.Outcome, record.FailureReason, marshalJSON(record.ToolsInvoked), record.ToolSuccessRate, llmkbBoolToInt(record.SchemaValidated),
		llmkbBoolToInt(record.FallbackTriggered), fallbackModel, ratingJSON, llmkbBoolToInt(record.UserOverride), record.Notes, record.CreatedAt.Format(timeFormat))
	return err
}

func (r *SQLiteLLMKBRepo) PruneExecutionRecords(ctx context.Context, olderThan time.Duration) error {
	// ... (unchanged)
	cutoff := time.Now().Add(-olderThan).Format(timeFormat)
	_, err := r.db.ExecContext(ctx, `DELETE FROM llm_execution_records WHERE created_at < ?`, cutoff)
	return err
}

func (r *SQLiteLLMKBRepo) ListExecutionRecords(ctx context.Context, llmID string, limit int) ([]llmkb.LLMExecutionRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT record_id, llm_id, instance_id, router_decision_id, task_class, task_id, context_size_tokens, output_size_tokens,
			latency_ms, cost_usd, outcome, failure_reason, tools_invoked_json, tool_success_rate, schema_validated,
			fallback_triggered, fallback_model_id, user_rating, user_override, notes, created_at
		FROM llm_execution_records WHERE (? = '' OR llm_id = ?) ORDER BY created_at DESC LIMIT ?
	`, llmID, llmID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []llmkb.LLMExecutionRecord
	for rows.Next() {
		var item llmkb.LLMExecutionRecord
		var toolsJSON, createdAt, costStr, fallbackModel, ratingStr string
		var schemaVal, fbTrig, usrOver int
		if err := rows.Scan(&item.RecordID, &item.LLMID, &item.InstanceID, &item.RouterDecisionID, &item.TaskClass, &item.TaskID, &item.ContextSizeTokens, &item.OutputSizeTokens,
			&item.LatencyMs, &costStr, &item.Outcome, &item.FailureReason, &toolsJSON, &item.ToolSuccessRate, &schemaVal,
			&fbTrig, &fallbackModel, &ratingStr, &usrOver, &item.Notes, &createdAt); err != nil {
			return nil, err
		}
		item.SchemaValidated = schemaVal != 0
		item.FallbackTriggered = fbTrig != 0
		item.UserOverride = usrOver != 0
		_ = unmarshalJSON(toolsJSON, &item.ToolsInvoked)
		if fallbackModel != "" {
			item.FallbackModelID = &fallbackModel
		}

		tm, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = tm
		out = append(out, item)
	}
	return out, rows.Err()
}

func nullableStringPtr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Format(timeFormat)
}

func llmkbBoolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
