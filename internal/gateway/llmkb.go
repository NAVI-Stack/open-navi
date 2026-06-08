package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
	"github.com/open-navi/navi/internal/store"
)

type llmkbGroupProvenance struct {
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence,omitempty"`
	AssertedAt string  `json:"asserted_at,omitempty"`
}

type llmkbFieldGroup struct {
	Name           string               `json:"name"`
	Classification string               `json:"classification"`
	Provenance     llmkbGroupProvenance `json:"provenance"`
	Fields         any                  `json:"fields"`
}

func (s *Server) handleDebugLLMKBProfiles(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	repo := store.NewSQLiteLLMKBRepo(s.cfg.DB)
	profiles, err := repo.ListProfiles(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		LLMID             string `json:"llm_id"`
		CanonicalName     string `json:"canonical_name"`
		Provider          string `json:"provider"`
		AgenticClass      string `json:"agentic_class"`
		CodingClass       string `json:"coding_class"`
		CostTier          string `json:"cost_tier"`
		AvailabilityState string `json:"availability_state"`
		Summary           string `json:"summary"`
	}
	out := make([]item, 0, len(profiles))
	for _, profile := range profiles {
		summary, err := repo.DescribeProfile(r.Context(), profile.LLMID)
		if err != nil {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, item{
			LLMID:             profile.LLMID,
			CanonicalName:     profile.CanonicalName,
			Provider:          profile.ProviderID,
			AgenticClass:      string(profile.Capabilities.AgenticClass),
			CodingClass:       string(profile.Capabilities.CodingClass),
			CostTier:          string(profile.Routing.CostTier),
			AvailabilityState: string(profile.OperationalState.AvailabilityState),
			Summary:           summary,
		})
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"description": "Human-readable LLM KB profile summaries for debug inspection.",
		"profiles":    out,
	})
}

func (s *Server) handleDebugLLMKBProfile(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	repo := store.NewSQLiteLLMKBRepo(s.cfg.DB)
	id := strings.TrimSpace(r.PathValue("id"))
	profile, err := repo.GetProfile(r.Context(), id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if profile == nil {
		profile, err = repo.GetProfileByAlias(r.Context(), id)
		if err != nil {
			var ambiguity *store.LLMKBProfileAliasAmbiguityError
			if errors.As(err, &ambiguity) {
				replyJSON(w, http.StatusConflict, map[string]any{
					"error":   "profile alias is ambiguous",
					"alias":   ambiguity.Alias,
					"matches": ambiguity.Matches,
				})
				return
			}
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if profile == nil {
		replyError(w, http.StatusNotFound, "profile not found")
		return
	}
	summary, err := repo.DescribeProfile(r.Context(), profile.LLMID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider, err := repo.GetProvider(r.Context(), profile.ProviderID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runtimes, err := repo.ListRuntimeInstances(r.Context(), profile.LLMID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	assertedAt := ""
	if !profile.Provenance.AssertedAt.IsZero() {
		assertedAt = profile.Provenance.AssertedAt.Format(time.RFC3339)
	}
	curatedProv := llmkbGroupProvenance{
		Source:     string(profile.Provenance.Source),
		Confidence: profile.Provenance.Confidence,
		AssertedAt: assertedAt,
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"description": "Human-readable first; grouped LLM KB profile inspection with explicit field classifications and provenance.",
		"summary":     summary,
		"profile_id":  profile.LLMID,
		"groups": []llmkbFieldGroup{
			{
				Name:           "identity",
				Classification: "Observed",
				Provenance:     curatedProv,
				Fields: map[string]any{
					"llm_id":             profile.LLMID,
					"schema_version":     profile.SchemaVersion,
					"canonical_name":     profile.CanonicalName,
					"aliases":            profile.Aliases,
					"provider_id":        profile.ProviderID,
					"availability_state": profile.OperationalState.AvailabilityState,
					"provider":           provider,
				},
			},
			{
				Name:           "capability_classes",
				Classification: "Inferred",
				Provenance:     curatedProv,
				Fields: map[string]any{
					"agentic_class":         profile.Capabilities.AgenticClass,
					"coding_class":          profile.Capabilities.CodingClass,
					"reasoning_class":       profile.Capabilities.ReasoningClass,
					"instruction_following": profile.Capabilities.InstructionFollowingClass,
					"tool_discipline":       profile.Capabilities.ToolDisciplineClass,
					"schema_reliability":    profile.Capabilities.SchemaReliabilityClass,
					"multimodal_class":      profile.Capabilities.MultimodalClass,
				},
			},
			{
				Name:           "capability_scope",
				Classification: "Governed",
				Provenance:     curatedProv,
				Fields: map[string]any{
					"supported_use_cases": profile.Capabilities.PrimaryUseCases,
					"interaction_modes":   profile.Capabilities.InteractionModes,
				},
			},
			{
				Name:           "technical_features",
				Classification: "Observed",
				Provenance:     curatedProv,
				Fields:         profile.Features,
			},
			{
				Name:           "routing_profile",
				Classification: "Governed",
				Provenance:     curatedProv,
				Fields: map[string]any{
					"preferred_task_classes": profile.Routing.PreferredFor,
					"cost_tier":              profile.Routing.CostTier,
					"latency_tier":           profile.Routing.LatencyTier,
					"risk_tier":              profile.Routing.MaxRiskTierAllowed,
					"autonomy_level":         profile.Routing.AutonomyCeiling,
				},
			},
			{
				Name:           "operational_state",
				Classification: "Observed",
				Provenance: llmkbGroupProvenance{
					Source:     "runtime_state",
					Confidence: 1.0,
					AssertedAt: assertedAt,
				},
				Fields: profile.OperationalState,
			},
			{
				Name:           "evaluation_profile",
				Classification: "Inferred",
				Provenance: llmkbGroupProvenance{
					Source: "reflection_or_eval",
				},
				Fields: profile.Evaluation,
			},
			{
				Name:           "usage_stats",
				Classification: "Observed",
				Provenance: llmkbGroupProvenance{
					Source:     "execution_telemetry",
					Confidence: 1.0,
				},
				Fields: profile.UsageStats,
			},
		},
		"runtime_instances": runtimes,
		"mutation_history":  profile.Provenance.MutationHistory,
	})
}

func (s *Server) handleListLLMKBRoutingProposals(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	repo := store.NewSQLiteLLMKBRepo(s.cfg.DB)
	list, err := repo.ListRoutingProposals(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []llmkb.RoutingProposalItem{}
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) handleResolveLLMKBRoutingProposal(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "write") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "write scope required", nil)
		return
	}
	id := r.PathValue("id")
	var req struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	status := llmkb.ProposalStatusRejected
	if req.Accepted {
		status = llmkb.ProposalStatusApproved
	}

	repo := store.NewSQLiteLLMKBRepo(s.cfg.DB)
	if err := repo.ResolveRoutingProposal(r.Context(), id, status); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}

	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
