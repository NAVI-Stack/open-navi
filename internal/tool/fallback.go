package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// FallbackDiscoveryOutcome captures the discovery-only result of degradation
// fallback lookup. It never implies execution authority.
type FallbackDiscoveryOutcome string

const (
	FallbackDiscoveryOutcomeNoFallbackAvailable  FallbackDiscoveryOutcome = "no_fallback_available"
	FallbackDiscoveryOutcomeFallbackCandidates   FallbackDiscoveryOutcome = "fallback_candidates"
	FallbackDiscoveryOutcomeRequiresConfirmation FallbackDiscoveryOutcome = "fallback_requires_confirmation"
	FallbackDiscoveryOutcomeBlockedByGovernance  FallbackDiscoveryOutcome = "fallback_blocked_by_governance"
)

// FallbackDiscoveryInput is the normalized contract for recoverable tool-action
// degradation lookup.
type FallbackDiscoveryInput struct {
	Registry          *Registry
	FailedToolID      string
	FailedAction      string
	Failure           ExecutionFailure
	FailureClass      schema.FailureClass
	Intent            string
	Arguments         map[string]any
	ActiveToolSet     *ActiveToolSet
	Context           DiscoveryContext
	MaximumRiskTier   string
	SideEffectCeiling []string
	Limit             int
}

// FallbackCandidate is a discovery-only candidate path that preserves intent
// without implying authority to execute it.
type FallbackCandidate struct {
	CandidateID          string              `json:"candidate_id,omitempty"`
	ToolIDs              []string            `json:"tool_ids,omitempty"`
	MatchKind            string              `json:"match_kind,omitempty"`
	Relevance            float64             `json:"relevance,omitempty"`
	Reason               string              `json:"reason,omitempty"`
	Rationale            []string            `json:"rationale,omitempty"`
	RiskComparison       string              `json:"risk_comparison,omitempty"`
	SideEffectComparison string              `json:"side_effect_comparison,omitempty"`
	RequiresConfirmation bool                `json:"requires_confirmation,omitempty"`
	GovernanceBlocked    bool                `json:"governance_blocked,omitempty"`
	BlockedReasonCode    DiscoveryReasonCode `json:"blocked_reason_code,omitempty"`
}

// FallbackDiscoveryResult is the structured result of degraded-action fallback
// discovery. The caller must still route any selected path through governance.
type FallbackDiscoveryResult struct {
	SnapshotID    string                   `json:"snapshot_id,omitempty"`
	FailedToolID  string                   `json:"failed_tool_id,omitempty"`
	FailedAction  string                   `json:"failed_action,omitempty"`
	FailureCode   ExecutionFailureCode     `json:"failure_code,omitempty"`
	FailureClass  schema.FailureClass      `json:"failure_class,omitempty"`
	Outcome       FallbackDiscoveryOutcome `json:"outcome,omitempty"`
	Candidates    []FallbackCandidate      `json:"candidates,omitempty"`
	SafeReason    string                   `json:"safe_reason,omitempty"`
	BlockedReason string                   `json:"blocked_reason,omitempty"`
}

// DiscoverFallbackCandidates resolves safe fallback candidates for recoverable
// degradation without ever granting execution authority.
func DiscoverFallbackCandidates(input FallbackDiscoveryInput) FallbackDiscoveryResult {
	failure := normalizeFallbackFailure(input)
	result := FallbackDiscoveryResult{
		FailedToolID: strings.TrimSpace(input.FailedToolID),
		FailedAction: strings.TrimSpace(input.FailedAction),
		FailureCode:  failure.Code,
		FailureClass: failure.FailureClass,
		Outcome:      FallbackDiscoveryOutcomeNoFallbackAvailable,
	}
	if input.Registry == nil {
		result.SafeReason = "tool fallback discovery has no registry snapshot"
		return result
	}

	snapshot := input.Registry.Snapshot()
	result.SnapshotID = snapshot.ID

	if failure.Code == ExecutionFailureCodeGovernanceBlocked || failure.Code == ExecutionFailureCodePolicyRejected || !failure.FallbackAllowed {
		result.Outcome = FallbackDiscoveryOutcomeBlockedByGovernance
		result.SafeReason = "fallback discovery is disabled for governance or policy rejections"
		result.BlockedReason = "original action was blocked by governance/policy"
		return result
	}

	idx := NewIndexFromSnapshot(snapshot)
	original := lookupFallbackOriginalTool(snapshot.Tools, result.FailedToolID)
	query := fallbackQueryText(input, original)
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	available := make([]FallbackCandidate, 0, limit)
	blocked := make([]FallbackCandidate, 0, limit)

	singleAvailable, singleBlocked := discoverSingleFallbackCandidates(idx, original, failure, input, query, limit)
	available = append(available, singleAvailable...)
	blocked = append(blocked, singleBlocked...)

	if gitCandidate, ok := discoverGitWriteFallbackPath(idx, original, failure, input, query); ok {
		if gitCandidate.GovernanceBlocked {
			blocked = append(blocked, gitCandidate)
		} else {
			available = append(available, gitCandidate)
		}
	}

	available = dedupeFallbackCandidates(available)
	blocked = dedupeFallbackCandidates(blocked)

	sort.SliceStable(available, func(i, j int) bool {
		if available[i].Relevance == available[j].Relevance {
			return available[i].CandidateID < available[j].CandidateID
		}
		return available[i].Relevance > available[j].Relevance
	})
	if len(available) > limit {
		available = available[:limit]
	}

	if len(available) == 0 {
		if len(blocked) > 0 {
			result.Outcome = FallbackDiscoveryOutcomeBlockedByGovernance
			result.SafeReason = "fallback candidates exist but are not available in the current governed context"
			result.BlockedReason = blocked[0].Reason
			return result
		}
		result.SafeReason = "no equivalent governed fallback candidates were found"
		return result
	}

	result.Candidates = available
	result.SafeReason = "fallback candidates are discovery-only and must still pass normal runtime governance"
	result.Outcome = FallbackDiscoveryOutcomeFallbackCandidates
	for _, candidate := range available {
		if candidate.RequiresConfirmation {
			result.Outcome = FallbackDiscoveryOutcomeRequiresConfirmation
			break
		}
	}
	return result
}

func normalizeFallbackFailure(input FallbackDiscoveryInput) ExecutionFailure {
	if !input.Failure.IsZero() {
		return input.Failure
	}
	switch input.FailureClass {
	case schema.FailureClassTimeout:
		return FailureForCode(ExecutionFailureCodeTransientTimeout, "")
	case schema.FailureClassConnectorUnavailable:
		return FailureForCode(ExecutionFailureCodeConnectorUnavailable, "")
	case schema.FailureClassPartialExecution:
		return FailureForCode(ExecutionFailureCodePartialFailure, "")
	case schema.FailureClassPolicyBlocked:
		return FailureForCode(ExecutionFailureCodeGovernanceBlocked, "")
	case schema.FailureClassSchemaInvalid:
		return FailureForCode(ExecutionFailureCodeSchemaMismatch, "")
	default:
		return FailureForCode(ExecutionFailureCodeExecutionFailed, "")
	}
}

func lookupFallbackOriginalTool(tools []*Tool, failedToolID string) *Tool {
	failedToolID = normalizeSearchText(failedToolID)
	for _, toolEntry := range tools {
		if toolEntry == nil {
			continue
		}
		if normalizeSearchText(toolEntry.ToolID) == failedToolID {
			return cloneTool(toolEntry)
		}
	}
	return nil
}

func fallbackQueryText(input FallbackDiscoveryInput, original *Tool) string {
	parts := []string{
		input.Intent,
		input.FailedAction,
		input.FailedToolID,
	}
	if original != nil {
		parts = append(parts,
			original.DisplayName,
			original.Description,
			original.Governance.Domain,
			strings.Join(original.CapabilityTags, " "),
			strings.Join(original.SideEffects, " "),
		)
	}
	return strings.Join(parts, " ")
}

func discoverSingleFallbackCandidates(idx *Index, original *Tool, failure ExecutionFailure, input FallbackDiscoveryInput, query string, limit int) ([]FallbackCandidate, []FallbackCandidate) {
	if idx == nil {
		return nil, nil
	}
	queryTokens := tokenSet(query)
	originalTags := tokenSet(strings.Join(originalCapabilityTerms(original), " "))
	originalToolID := strings.TrimSpace(input.FailedToolID)
	search := append(idx.LexicalSearch(query, limit*2), idx.TagSearch(query, limit*2)...)
	candidatesByToolID := make(map[string]SearchCandidate, len(search))
	for _, candidate := range search {
		if candidate.Tool == nil {
			continue
		}
		toolID := strings.TrimSpace(candidate.Tool.ToolID)
		if toolID == "" || toolID == originalToolID {
			continue
		}
		if existing, ok := candidatesByToolID[toolID]; ok && existing.Relevance >= candidate.Relevance {
			continue
		}
		candidatesByToolID[toolID] = candidate
	}

	available := make([]FallbackCandidate, 0, len(candidatesByToolID))
	blocked := make([]FallbackCandidate, 0, len(candidatesByToolID))
	for _, ranked := range candidatesByToolID {
		if ranked.Tool == nil {
			continue
		}
		toolEntry := ranked.Tool
		explicit := fallbackNoteContains(toolEntry, "fallback_for", originalToolID) || fallbackNoteContains(toolEntry, "fallback_for_action", input.FailedAction)
		semanticScore, semanticReasons := fallbackSemanticScore(original, toolEntry, queryTokens, originalTags, explicit)
		if semanticScore < 0.28 {
			continue
		}
		status, reason := evaluateDiscoveryPolicy(toolEntry, input.Context)
		candidate := buildSingleFallbackCandidate(original, toolEntry, input, ranked, semanticScore, semanticReasons, failure, status, reason)
		switch status {
		case DiscoveryAvailabilityAvailable:
			available = append(available, candidate)
		case DiscoveryAvailabilityVisibleUnavailable, DiscoveryAvailabilityHidden:
			if isGovernanceUnavailableReason(reason) {
				candidate.GovernanceBlocked = true
				blocked = append(blocked, candidate)
			}
		}
	}
	return available, blocked
}

func buildSingleFallbackCandidate(original *Tool, toolEntry *Tool, input FallbackDiscoveryInput, ranked SearchCandidate, semanticScore float64, semanticReasons []string, failure ExecutionFailure, status DiscoveryAvailabilityStatus, reason DiscoveryReasonCode) FallbackCandidate {
	relevance := clampScore(maxFloat(ranked.Relevance, semanticScore))
	if input.ActiveToolSet != nil && input.ActiveToolSet.ContainsTool(toolEntry.ToolID) {
		relevance = clampScore(relevance + 0.05)
		semanticReasons = append(semanticReasons, "already_loaded_in_active_tool_set")
	}
	riskComparison := compareRiskTier(originalRiskTier(original), toolEntry.RiskTier)
	sideEffectComparison := compareSideEffects(originalSideEffects(original, input.SideEffectCeiling), toolEntry.SideEffects)
	requiresConfirmation := toolEntry.Governance.RequiresConfirm || riskComparison == "higher" || sideEffectComparison == "broader"
	candidate := FallbackCandidate{
		CandidateID:          "fallback:" + toolEntry.ToolID,
		ToolIDs:              []string{toolEntry.ToolID},
		MatchKind:            firstNonEmpty(ranked.MatchKind, "semantic"),
		Relevance:            relevance,
		Reason:               fallbackReasonForSingleCandidate(toolEntry, riskComparison, sideEffectComparison),
		Rationale:            dedupeStrings(semanticReasons),
		RiskComparison:       riskComparison,
		SideEffectComparison: sideEffectComparison,
		RequiresConfirmation: requiresConfirmation,
		GovernanceBlocked:    status != DiscoveryAvailabilityAvailable,
		BlockedReasonCode:    reason,
	}
	if failure.Code == ExecutionFailureCodeAPIContractDrift {
		candidate.Rationale = append(candidate.Rationale, "api_contract_drift_recovery")
	}
	if failure.Code == ExecutionFailureCodeSelectedActionMissing {
		candidate.Rationale = append(candidate.Rationale, "selected_action_missing_recovery")
	}
	candidate.Rationale = dedupeStrings(candidate.Rationale)
	return candidate
}

func discoverGitWriteFallbackPath(idx *Index, original *Tool, failure ExecutionFailure, input FallbackDiscoveryInput, query string) (FallbackCandidate, bool) {
	if idx == nil || original == nil || !originalRepresentsFileWrite(original, input) {
		return FallbackCandidate{}, false
	}

	type gitStageMatch struct {
		stage  string
		order  int
		tool   *Tool
		status DiscoveryAvailabilityStatus
		reason DiscoveryReasonCode
		score  float64
	}

	matches := make([]gitStageMatch, 0, 4)
	queryTokens := tokenSet(query)
	for _, doc := range idx.docs {
		if doc.tool == nil || strings.TrimSpace(doc.tool.ToolID) == strings.TrimSpace(input.FailedToolID) {
			continue
		}
		stage, order, ok := gitWriteFallbackStage(doc.tool)
		if !ok {
			continue
		}
		if !toolLooksLikeGitWriteFallback(doc.tool, queryTokens) {
			continue
		}
		status, reason := evaluateDiscoveryPolicy(doc.tool, input.Context)
		score, _ := fallbackSemanticScore(original, doc.tool, queryTokens, tokenSet(strings.Join(originalCapabilityTerms(original), " ")), false)
		if input.ActiveToolSet != nil && input.ActiveToolSet.ContainsTool(doc.tool.ToolID) {
			score = clampScore(score + 0.05)
		}
		matches = append(matches, gitStageMatch{
			stage:  stage,
			order:  order,
			tool:   cloneTool(doc.tool),
			status: status,
			reason: reason,
			score:  score,
		})
	}
	if len(matches) < 2 {
		return FallbackCandidate{}, false
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].order == matches[j].order {
			return matches[i].tool.ToolID < matches[j].tool.ToolID
		}
		return matches[i].order < matches[j].order
	})

	availableTools := make([]*Tool, 0, len(matches))
	blockedReason := DiscoveryReasonNone
	rationale := []string{"decomposed_git_write_path", "preserves_file_update_intent"}
	relevance := 0.0
	for _, match := range matches {
		if match.status == DiscoveryAvailabilityAvailable {
			availableTools = append(availableTools, match.tool)
			relevance += match.score
			rationale = append(rationale, "git_stage_"+match.stage)
			continue
		}
		if blockedReason == DiscoveryReasonNone && isGovernanceUnavailableReason(match.reason) {
			blockedReason = match.reason
		}
	}

	if len(availableTools) < 2 {
		if blockedReason == DiscoveryReasonNone {
			return FallbackCandidate{}, false
		}
		return FallbackCandidate{
			CandidateID:       "fallback:git_write_path",
			MatchKind:         "decomposed_path",
			Reason:            "decomposed Git write fallback exists but is blocked in the current governed context",
			Rationale:         dedupeStrings(rationale),
			GovernanceBlocked: true,
			BlockedReasonCode: blockedReason,
		}, true
	}

	relevance = clampScore(relevance / float64(len(availableTools)))
	toolIDs := make([]string, 0, len(availableTools))
	requiresConfirmation := false
	riskTier := ""
	sideEffects := make([]string, 0, len(availableTools)*2)
	for _, toolEntry := range availableTools {
		toolIDs = append(toolIDs, toolEntry.ToolID)
		if riskTierRank(toolEntry.RiskTier) > riskTierRank(riskTier) {
			riskTier = toolEntry.RiskTier
		}
		sideEffects = append(sideEffects, toolEntry.SideEffects...)
		if toolEntry.Governance.RequiresConfirm {
			requiresConfirmation = true
		}
	}
	sideEffects = dedupeStrings(sideEffects)
	riskComparison := compareRiskTier(original.RiskTier, riskTier)
	sideEffectComparison := compareSideEffects(originalSideEffects(original, input.SideEffectCeiling), sideEffects)
	if riskComparison == "higher" || sideEffectComparison == "broader" {
		requiresConfirmation = true
	}
	return FallbackCandidate{
		CandidateID:          "fallback:git_write_path",
		ToolIDs:              toolIDs,
		MatchKind:            "decomposed_path",
		Relevance:            relevance,
		Reason:               "decomposed Git write path can preserve the file-update intent",
		Rationale:            dedupeStrings(rationale),
		RiskComparison:       riskComparison,
		SideEffectComparison: sideEffectComparison,
		RequiresConfirmation: requiresConfirmation,
	}, true
}

func fallbackSemanticScore(original *Tool, candidate *Tool, queryTokens map[string]struct{}, originalTags map[string]struct{}, explicit bool) (float64, []string) {
	if candidate == nil {
		return 0, nil
	}
	score := 0.0
	rationale := []string{}
	if explicit {
		score += 0.55
		rationale = append(rationale, "explicit_fallback_mapping")
	}
	if original != nil && normalizeSearchText(original.Governance.Domain) != "" && normalizeSearchText(original.Governance.Domain) == normalizeSearchText(candidate.Governance.Domain) {
		score += 0.18
		rationale = append(rationale, "same_governance_domain")
	}
	if original != nil && original.Governance.WorkspaceAction != "" && original.Governance.WorkspaceAction == candidate.Governance.WorkspaceAction {
		score += 0.14
		rationale = append(rationale, "same_workspace_action")
	}
	if original != nil && original.Governance.CommandType != "" && original.Governance.CommandType == candidate.Governance.CommandType {
		score += 0.08
		rationale = append(rationale, "same_command_type")
	}
	overlap := 0.0
	candidateTags := tokenSet(strings.Join(originalCapabilityTerms(candidate), " "))
	for token := range originalTags {
		if _, ok := candidateTags[token]; ok {
			overlap += 0.08
		}
	}
	for token := range queryTokens {
		if _, ok := candidateTags[token]; ok {
			overlap += 0.06
		}
	}
	if overlap > 0 {
		score += overlap
		rationale = append(rationale, "intent_tag_overlap")
	}
	if candidate.Category == ToolCategoryWorkflowAction || candidate.Category == ToolCategoryDevTest || candidate.Category == ToolCategoryAdminCritical {
		score += 0.04
	}
	return clampScore(score), rationale
}

func originalCapabilityTerms(toolEntry *Tool) []string {
	if toolEntry == nil {
		return nil
	}
	values := append([]string(nil), toolEntry.CapabilityTags...)
	values = append(values, toolEntry.Governance.Domain)
	values = append(values, toolEntry.DisplayName, toolEntry.Description)
	values = append(values, toolEntry.SideEffects...)
	values = append(values, toolEntry.Metadata.Tags...)
	return values
}

func originalRepresentsFileWrite(original *Tool, input FallbackDiscoveryInput) bool {
	if original == nil {
		return containsAny(normalizeSearchText(input.Intent+" "+input.FailedAction), "file", "files", "write", "update", "patch")
	}
	if original.Governance.WorkspaceAction == schema.WorkspaceActionWrite || original.Governance.CommandType == schema.CommandTypeUpdate || original.Governance.CommandType == schema.CommandTypeCreate {
		return true
	}
	tokens := tokenSet(strings.Join(originalCapabilityTerms(original), " "))
	for _, token := range []string{"file", "files", "workspace", "write", "update", "patch"} {
		if _, ok := tokens[token]; ok {
			return true
		}
	}
	return false
}

func toolLooksLikeGitWriteFallback(toolEntry *Tool, queryTokens map[string]struct{}) bool {
	if toolEntry == nil {
		return false
	}
	tokens := tokenSet(strings.Join(originalCapabilityTerms(toolEntry), " "))
	if _, ok := tokens["git"]; !ok {
		return false
	}
	if _, ok := tokens["write"]; ok {
		return true
	}
	for token := range queryTokens {
		if _, ok := tokens[token]; ok {
			return true
		}
	}
	return false
}

func gitWriteFallbackStage(toolEntry *Tool) (string, int, bool) {
	if toolEntry == nil {
		return "", 0, false
	}
	if stage := strings.TrimSpace(toolEntry.Metadata.Notes["fallback_stage"]); stage != "" {
		switch normalizeSearchText(stage) {
		case "blob":
			return "blob", 10, true
		case "tree":
			return "tree", 20, true
		case "commit":
			return "commit", 30, true
		case "ref":
			return "ref", 40, true
		case "patch":
			return "patch", 15, true
		}
	}
	text := normalizeSearchText(strings.Join(originalCapabilityTerms(toolEntry), " ") + " " + toolEntry.ToolID)
	switch {
	case containsAny(text, "blob"):
		return "blob", 10, true
	case containsAny(text, "patch"):
		return "patch", 15, true
	case containsAny(text, "tree"):
		return "tree", 20, true
	case containsAny(text, "commit"):
		return "commit", 30, true
	case containsAny(text, "ref", "reference"):
		return "ref", 40, true
	default:
		return "", 0, false
	}
}

func compareRiskTier(originalRiskTier string, candidateRiskTier string) string {
	originalRank := riskTierRank(originalRiskTier)
	candidateRank := riskTierRank(candidateRiskTier)
	switch {
	case candidateRank == 0 || originalRank == 0:
		return "equivalent"
	case candidateRank < originalRank:
		return "safer"
	case candidateRank > originalRank:
		return "higher"
	default:
		return "equivalent"
	}
}

func compareSideEffects(original []string, candidate []string) string {
	originalSet := stringSet(original)
	candidateSet := stringSet(candidate)
	switch {
	case len(candidateSet) == 0 && len(originalSet) == 0:
		return "equivalent"
	case isSubsetSet(candidateSet, originalSet) && isSubsetSet(originalSet, candidateSet):
		return "equivalent"
	case isSubsetSet(candidateSet, originalSet):
		return "narrower"
	case isSubsetSet(originalSet, candidateSet):
		return "broader"
	default:
		return "broader"
	}
}

func originalRiskTier(original *Tool) string {
	if original == nil {
		return ""
	}
	return original.RiskTier
}

func originalSideEffects(original *Tool, ceiling []string) []string {
	if len(ceiling) > 0 {
		return dedupeStrings(ceiling)
	}
	if original == nil {
		return nil
	}
	return dedupeStrings(original.SideEffects)
}

func isSubsetSet(left map[string]struct{}, right map[string]struct{}) bool {
	if len(left) == 0 {
		return true
	}
	if len(right) == 0 {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func dedupeFallbackCandidates(candidates []FallbackCandidate) []FallbackCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]FallbackCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate.CandidateID)
		if key == "" {
			key = strings.Join(candidate.ToolIDs, "|")
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func fallbackReasonForSingleCandidate(toolEntry *Tool, riskComparison string, sideEffectComparison string) string {
	if toolEntry == nil {
		return "fallback candidate preserves the original intent"
	}
	base := fmt.Sprintf("%s can preserve the original intent", firstNonEmpty(toolEntry.DisplayName, toolEntry.ToolID))
	switch {
	case riskComparison == "higher" || sideEffectComparison == "broader":
		return base + " but changes risk or side effects"
	case riskComparison == "safer" || sideEffectComparison == "narrower":
		return base + " with an equivalent or safer execution profile"
	default:
		return base + " with an equivalent execution profile"
	}
}

func fallbackNoteContains(toolEntry *Tool, key string, want string) bool {
	if toolEntry == nil || len(toolEntry.Metadata.Notes) == 0 {
		return false
	}
	value := strings.TrimSpace(toolEntry.Metadata.Notes[key])
	want = strings.TrimSpace(want)
	if value == "" || want == "" {
		return false
	}
	for _, part := range strings.Split(value, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

func isGovernanceUnavailableReason(reason DiscoveryReasonCode) bool {
	switch reason {
	case DiscoveryReasonRequiresEnvironment,
		DiscoveryReasonRequiresMode,
		DiscoveryReasonRequiresAuthority,
		DiscoveryReasonRequiresFeatureFlag,
		DiscoveryReasonDisallowedRiskTier,
		DiscoveryReasonDisallowedTrustTier,
		DiscoveryReasonHiddenByPolicy,
		DiscoveryReasonHidden:
		return true
	default:
		return false
	}
}
