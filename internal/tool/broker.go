package tool

import (
	"fmt"
	"sort"
	"strings"
)

// ToolChoiceMode is the provider-neutral broker output for tool exposure.
type ToolChoiceMode string

const (
	ToolChoiceModeNone          ToolChoiceMode = "none"
	ToolChoiceModeAuto          ToolChoiceMode = "auto"
	ToolChoiceModeRequired      ToolChoiceMode = "required"
	ToolChoiceModeForcedSingle  ToolChoiceMode = "forced_single"
	ToolChoiceModeAllowedSubset ToolChoiceMode = "allowed_subset"
)

// BrokerIntent captures the broker's best understanding of the current turn.
type BrokerIntent string

const (
	BrokerIntentSmalltalk         BrokerIntent = "smalltalk"
	BrokerIntentAmbiguous         BrokerIntent = "ambiguous"
	BrokerIntentCodingQuery       BrokerIntent = "coding_query"
	BrokerIntentDiagnosticRequest BrokerIntent = "diagnostic_request"
	BrokerIntentAssistantTask     BrokerIntent = "assistant_task"
)

// BrokerSuppressionReason is the machine-readable explanation for why a tool
// was not exposed for the current turn.
type BrokerSuppressionReason string

const (
	BrokerSuppressionSmalltalkNoTools     BrokerSuppressionReason = "smalltalk_no_tools"
	BrokerSuppressionAmbiguousRequest     BrokerSuppressionReason = "ambiguous_request"
	BrokerSuppressionTooManySimilarTools  BrokerSuppressionReason = "too_many_similar_tools"
	BrokerSuppressionLocalModelBudget     BrokerSuppressionReason = "local_model_budget_limit"
	BrokerSuppressionConnectorUnavailable BrokerSuppressionReason = "connector_unavailable"
	BrokerSuppressionRequiresDebugMode    BrokerSuppressionReason = "requires_debug_mode"
	BrokerSuppressionRequiresCoderMode    BrokerSuppressionReason = "requires_coder_mode"
	BrokerSuppressionEnvironmentBlocked   BrokerSuppressionReason = "environment_blocked"
	BrokerSuppressionRequiresAuthority    BrokerSuppressionReason = "requires_authority"
	BrokerSuppressionFeatureFlagBlocked   BrokerSuppressionReason = "feature_flag_blocked"
	BrokerSuppressionRiskTierTooHigh      BrokerSuppressionReason = "risk_tier_too_high"
	BrokerSuppressionTrustTierTooLow      BrokerSuppressionReason = "trust_tier_too_low"
	BrokerSuppressionToolUnavailable      BrokerSuppressionReason = "tool_unavailable"
	BrokerSuppressionModelNotToolCapable  BrokerSuppressionReason = "model_not_tool_capable"
)

// BrokerModelProfile is the broker's minimal model capability contract.
type BrokerModelProfile struct {
	Name             string
	SupportsTools    bool
	ToolCallReliable bool
}

func (p BrokerModelProfile) normalize() BrokerModelProfile {
	normalized := p
	if !normalized.SupportsTools && !normalized.ToolCallReliable && strings.TrimSpace(normalized.Name) == "" {
		normalized.SupportsTools = true
		normalized.ToolCallReliable = true
		return normalized
	}
	if normalized.SupportsTools && !normalized.ToolCallReliable {
		normalized.ToolCallReliable = true
	}
	if !normalized.SupportsTools {
		normalized.ToolCallReliable = false
	}
	return normalized
}

func (p BrokerModelProfile) canUseTools() bool {
	normalized := p.normalize()
	return normalized.SupportsTools && normalized.ToolCallReliable
}

func (p BrokerModelProfile) maxToolCount() int {
	normalized := p.normalize()
	if !normalized.canUseTools() {
		return 0
	}
	lower := strings.ToLower(strings.TrimSpace(normalized.Name))
	switch {
	case strings.Contains(lower, "local_weak"),
		strings.Contains(lower, "weak"),
		strings.Contains(lower, "mini"),
		strings.Contains(lower, "haiku"):
		return 2
	case strings.Contains(lower, "local_medium"),
		strings.Contains(lower, "medium"),
		strings.Contains(lower, "local"):
		return 3
	case strings.Contains(lower, "frontier"),
		strings.Contains(lower, "strong"),
		strings.Contains(lower, "opus"),
		strings.Contains(lower, "sonnet"),
		strings.Contains(lower, "gpt"):
		return 5
	default:
		return 3
	}
}

// BrokerInput is the structured tool-broker turn contract.
type BrokerInput struct {
	UserInput                 string
	Intent                    BrokerIntent
	SessionMode               DiscoverySessionMode
	Environment               string
	UserAuthority             ToolAuthority
	ModelProfile              BrokerModelProfile
	WorkflowState             string
	ActiveToolSetID           string
	ActiveToolIDs             []string
	RecentToolIDs             []string
	AllowCachedTools          bool
	AvailableConnectors       []string
	KnownFalsePositiveToolIDs []string
	FeatureFlags              []string
	MaximumRiskTier           string
	MinimumTrustTier          string
}

func (in BrokerInput) normalize() BrokerInput {
	normalized := in
	normalized.UserInput = strings.TrimSpace(normalized.UserInput)
	normalized.WorkflowState = strings.TrimSpace(normalized.WorkflowState)
	if normalized.SessionMode == "" {
		normalized.SessionMode = DiscoverySessionModeAssistant
	}
	if normalized.Environment == "" {
		normalized.Environment = "production"
	}
	if normalized.UserAuthority == "" {
		normalized.UserAuthority = ToolAuthorityUser
	}
	normalized.ModelProfile = normalized.ModelProfile.normalize()
	normalized.ActiveToolIDs = dedupeStrings(normalized.ActiveToolIDs)
	normalized.RecentToolIDs = dedupeStrings(normalized.RecentToolIDs)
	normalized.AvailableConnectors = dedupeStrings(normalized.AvailableConnectors)
	normalized.KnownFalsePositiveToolIDs = dedupeStrings(normalized.KnownFalsePositiveToolIDs)
	normalized.FeatureFlags = dedupeStrings(normalized.FeatureFlags)
	return normalized
}

func (in BrokerInput) discoveryContext() DiscoveryContext {
	return DiscoveryContext{
		Environment:      in.Environment,
		SessionMode:      in.SessionMode,
		Authority:        in.UserAuthority,
		FeatureFlags:     append([]string(nil), in.FeatureFlags...),
		MaximumRiskTier:  in.MaximumRiskTier,
		MinimumTrustTier: in.MinimumTrustTier,
	}
}

// BrokerSuppressedTool records one tool the broker explicitly withheld.
type BrokerSuppressedTool struct {
	ToolID    string
	Reason    BrokerSuppressionReason
	Score     float64
	MatchKind string
	Rationale []string
}

// BrokerSelectedTool records one tool the broker intentionally exposed.
type BrokerSelectedTool struct {
	ToolID    string
	Score     float64
	MatchKind string
	Rationale []string
}

// BrokerTraceCandidate captures the broker's scored candidate state so later
// runtime traces can compare exposure against actual tool usage.
type BrokerTraceCandidate struct {
	ToolID            string
	Score             float64
	MatchKind         string
	OverlapKey        string
	Selected          bool
	SuppressionReason BrokerSuppressionReason
	Rationale         []string
}

// BrokerTrace captures trace-friendly ranking and selection metadata.
type BrokerTrace struct {
	SelectionLimit   int
	CandidateCount   int
	RankedCandidates []BrokerTraceCandidate
}

// BrokerResolution is the broker's explicit turn-level tool exposure plan.
type BrokerResolution struct {
	SnapshotID      string
	Intent          BrokerIntent
	SelectedToolIDs []string
	SelectedTools   []BrokerSelectedTool
	SuppressedTools []BrokerSuppressedTool
	ToolChoiceMode  ToolChoiceMode
	BrokerReason    string
	Trace           BrokerTrace
}

// ToolBroker resolves narrow turn-level tool exposure without executing tools.
type ToolBroker struct {
	index    *Index
	registry *Registry
}

// NewToolBroker creates a broker over an immutable index snapshot.
func NewToolBroker(index *Index) *ToolBroker {
	return &ToolBroker{index: index}
}

// NewToolBrokerFromRegistry creates a broker over the latest registry snapshot.
func NewToolBrokerFromRegistry(reg *Registry) *ToolBroker {
	return &ToolBroker{
		index:    NewIndex(reg),
		registry: reg,
	}
}

type brokerCandidate struct {
	tool              *Tool
	relevance         float64
	matchKind         string
	availability      DiscoveryAvailabilityStatus
	discoveryReason   DiscoveryReasonCode
	suppressionReason BrokerSuppressionReason
	rationale         []string
	overlapKey        string
	selectable        bool
}

// Resolve decides which tools, if any, should be exposed for the current turn.
func (b *ToolBroker) Resolve(input BrokerInput) BrokerResolution {
	b.refreshIndexIfStale()
	input = input.normalize()
	selectionLimit := input.ModelProfile.maxToolCount()
	result := BrokerResolution{
		SnapshotID:      "",
		SelectedToolIDs: []string{},
		SelectedTools:   []BrokerSelectedTool{},
		SuppressedTools: []BrokerSuppressedTool{},
		ToolChoiceMode:  ToolChoiceModeNone,
		Trace: BrokerTrace{
			SelectionLimit: selectionLimit,
		},
	}
	if b == nil || b.index == nil {
		result.BrokerReason = "tool broker has no registry snapshot"
		return result
	}
	result.SnapshotID = b.index.SnapshotID()

	intent := input.Intent
	if intent == "" {
		intent = inferBrokerIntent(input.UserInput)
	}
	result.Intent = intent

	switch intent {
	case BrokerIntentSmalltalk:
		result.BrokerReason = "smalltalk turn defaults to no-tool chat"
		result.SuppressedTools = suppressExplicitTools(input.ActiveToolIDs, BrokerSuppressionSmalltalkNoTools)
		return result
	case BrokerIntentAmbiguous:
		result.BrokerReason = "ambiguous turn prefers no tools until the task is clearer"
		result.SuppressedTools = suppressExplicitTools(input.ActiveToolIDs, BrokerSuppressionAmbiguousRequest)
		return result
	}

	candidates := b.rankCandidates(input, intent)
	result.Trace.CandidateCount = len(candidates)
	if len(candidates) == 0 {
		result.BrokerReason = "no relevant tools discovered for this turn"
		return result
	}

	if !input.ModelProfile.canUseTools() {
		result.BrokerReason = "current model profile is not tool-capable"
		result.SuppressedTools = topSuppressedCandidates(candidates, BrokerSuppressionModelNotToolCapable, 4)
		result.Trace.RankedCandidates = brokerTraceCandidates(candidates, nil, result.SuppressedTools)
		return result
	}

	available := make([]brokerCandidate, 0, len(candidates))
	suppressed := make([]BrokerSuppressedTool, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.tool == nil {
			continue
		}
		if candidate.availability == DiscoveryAvailabilityAvailable && candidate.selectable {
			available = append(available, candidate)
			continue
		}
		if candidate.suppressionReason == "" {
			continue
		}
		suppressed = append(suppressed, suppressedToolFromCandidate(candidate))
	}

	if len(available) == 0 {
		result.SuppressedTools = dedupeSuppressedTools(suppressed)
		result.BrokerReason = blockedBrokerReason(intent, input, result.SuppressedTools)
		result.Trace.RankedCandidates = brokerTraceCandidates(candidates, nil, result.SuppressedTools)
		return result
	}

	selected, overlapSuppressed := selectBrokerCandidates(available, selectionLimit)
	result.SelectedToolIDs = make([]string, 0, len(selected))
	result.SelectedTools = make([]BrokerSelectedTool, 0, len(selected))
	selectedSet := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		if candidate.tool == nil {
			continue
		}
		selectedSet[candidate.tool.ToolID] = struct{}{}
		result.SelectedToolIDs = append(result.SelectedToolIDs, candidate.tool.ToolID)
		result.SelectedTools = append(result.SelectedTools, BrokerSelectedTool{
			ToolID:    candidate.tool.ToolID,
			Score:     candidate.relevance,
			MatchKind: candidate.matchKind,
			Rationale: append([]string(nil), candidate.rationale...),
		})
	}
	suppressed = append(suppressed, overlapSuppressed...)

	trimReason := BrokerSuppressionTooManySimilarTools
	if selectionLimit <= 2 && len(available) > len(selected) {
		trimReason = BrokerSuppressionLocalModelBudget
	}
	alreadySuppressed := suppressedToolIDSet(suppressed)
	for _, candidate := range available {
		if candidate.tool == nil {
			continue
		}
		if _, ok := selectedSet[candidate.tool.ToolID]; ok {
			continue
		}
		if _, ok := alreadySuppressed[candidate.tool.ToolID]; ok {
			continue
		}
		suppressed = append(suppressed, BrokerSuppressedTool{
			ToolID:    candidate.tool.ToolID,
			Reason:    trimReason,
			Score:     candidate.relevance,
			MatchKind: candidate.matchKind,
			Rationale: append([]string(nil), candidate.rationale...),
		})
	}
	result.SuppressedTools = dedupeSuppressedTools(suppressed)

	result.ToolChoiceMode = brokerChoiceMode(selected, available)
	result.BrokerReason = selectedBrokerReason(intent, input, selected, len(available))
	result.Trace.RankedCandidates = brokerTraceCandidates(candidates, result.SelectedToolIDs, result.SuppressedTools)
	return result
}

// CachedDecisionInvalid returns whether a cached broker outcome should be discarded
// because the underlying registry snapshot has changed since it was produced.
func (b *ToolBroker) CachedDecisionInvalid(snapshotID string) bool {
	b.refreshIndexIfStale()
	current := snapshotIDFromIndex(b)
	return strings.TrimSpace(snapshotID) != "" && current != "" && strings.TrimSpace(snapshotID) != current
}

func (b *ToolBroker) refreshIndexIfStale() {
	if b == nil || b.registry == nil {
		return
	}
	current := b.registry.Snapshot()
	if b.index != nil && strings.TrimSpace(b.index.SnapshotID()) == strings.TrimSpace(current.ID) {
		return
	}
	b.index = NewIndexFromSnapshot(current)
}

func (b *ToolBroker) rankCandidates(input BrokerInput, intent BrokerIntent) []brokerCandidate {
	if b == nil || b.index == nil || len(b.index.docs) == 0 {
		return nil
	}
	query := input.UserInput
	if strings.TrimSpace(query) == "" {
		return nil
	}
	normalizedQuery := normalizeSearchText(query)
	queryTokens := tokenSet(query)
	workflowTokens := tokenSet(input.WorkflowState)
	activeToolIDs := stringSet(input.ActiveToolIDs)
	recentToolIDs := stringSet(input.RecentToolIDs)
	availableConnectors := stringSet(input.AvailableConnectors)
	falsePositiveToolIDs := stringSet(input.KnownFalsePositiveToolIDs)
	ctx := input.discoveryContext()
	out := make([]brokerCandidate, 0, len(b.index.docs))
	for _, doc := range b.index.docs {
		if doc.tool == nil {
			continue
		}
		score, matchKind := lexicalScore(doc, normalizedQuery, queryTokens)
		rationale := []string{}
		if score > 0 {
			rationale = append(rationale, "lexical_match")
		}
		candidateTagScore := tagScore(doc, normalizedQuery, queryTokens)
		if candidateTagScore > score {
			score = candidateTagScore
			matchKind = "tag"
			rationale = append(rationale, "capability_tag_match")
		} else if candidateTagScore > 0 {
			score += candidateTagScore * 0.35
			rationale = append(rationale, "domain_tag_support")
		}
		intentBoost, intentReasons := brokerIntentBoost(doc, intent, workflowTokens)
		score += intentBoost
		rationale = append(rationale, intentReasons...)
		if input.AllowCachedTools {
			if _, ok := activeToolIDs[doc.tool.ToolID]; ok {
				score += 0.04
				rationale = append(rationale, "active_tool_set_reuse")
			}
		}
		if _, ok := recentToolIDs[doc.tool.ToolID]; ok {
			score += 0.06
			rationale = append(rationale, "session_recency")
		}
		if _, ok := falsePositiveToolIDs[doc.tool.ToolID]; ok {
			score -= 0.18
			rationale = append(rationale, "false_positive_history_penalty")
		}
		if connectorReady, connectorReason := connectorsSatisfied(doc.tool, availableConnectors); !connectorReady {
			rationale = append(rationale, connectorReason...)
		} else if len(doc.tool.ConnectorDependencies) > 0 && len(availableConnectors) > 0 {
			score += 0.05
			rationale = append(rationale, "connector_available")
		}
		if riskBoost, riskReasons := brokerRiskFitBoost(doc.tool, input.MaximumRiskTier); riskBoost != 0 {
			score += riskBoost
			rationale = append(rationale, riskReasons...)
		}
		if penalty, penaltyReasons := brokerModelPenalty(input.ModelProfile, doc.tool); penalty != 0 {
			score += penalty
			rationale = append(rationale, penaltyReasons...)
		}
		score = clampScore(score)
		if score < brokerCandidateThreshold(intent, doc.tool) {
			continue
		}

		status, reason := evaluateDiscoveryPolicy(doc.tool, ctx)
		selectable := status == DiscoveryAvailabilityAvailable
		suppressionReason := brokerSuppressionReasonFor(doc.tool, status, reason, input, intent)
		if connectorReady, _ := connectorsSatisfied(doc.tool, availableConnectors); !connectorReady {
			selectable = false
			if suppressionReason == "" {
				suppressionReason = BrokerSuppressionConnectorUnavailable
			}
		}
		out = append(out, brokerCandidate{
			tool:              cloneTool(doc.tool),
			relevance:         score,
			matchKind:         matchKind,
			availability:      status,
			discoveryReason:   reason,
			suppressionReason: suppressionReason,
			rationale:         dedupeStrings(rationale),
			overlapKey:        brokerOverlapKey(doc.tool),
			selectable:        selectable,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].relevance == out[j].relevance {
			leftAvailable := out[i].availability == DiscoveryAvailabilityAvailable
			rightAvailable := out[j].availability == DiscoveryAvailabilityAvailable
			if leftAvailable != rightAvailable {
				return leftAvailable
			}
			return out[i].tool.ToolID < out[j].tool.ToolID
		}
		return out[i].relevance > out[j].relevance
	})

	limit := input.ModelProfile.maxToolCount()*3 + 2
	if limit < 4 {
		limit = 4
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func inferBrokerIntent(userInput string) BrokerIntent {
	normalized := normalizeSearchText(userInput)
	if normalized == "" {
		return BrokerIntentAmbiguous
	}
	words := strings.Fields(normalized)
	if len(words) <= 4 && hasAnyPhrase(normalized, "yo", "hi", "hello", "hey", "thanks", "thank you", "cool", "nice") {
		return BrokerIntentSmalltalk
	}
	if hasAnyPhrase(normalized, "code", "repo", "repository", "file", "files", "source", "golang", "go test", "git", "diff", "function", "implementation") {
		return BrokerIntentCodingQuery
	}
	if hasAnyPhrase(normalized, "diagnose", "diagnostic", "debug", "trace", "logs", "log", "error", "failure", "why", "duplicate", "duplicated", "broken") {
		return BrokerIntentDiagnosticRequest
	}
	if len(words) <= 5 && !hasAnyPhrase(normalized, "read", "search", "find", "show", "list", "check", "summarize", "open") {
		return BrokerIntentAmbiguous
	}
	return BrokerIntentAssistantTask
}

func brokerCandidateThreshold(intent BrokerIntent, toolEntry *Tool) float64 {
	switch intent {
	case BrokerIntentCodingQuery:
		if toolEntry != nil && toolEntry.Category == ToolCategoryWorkflowAction {
			return 0.28
		}
		return 0.16
	case BrokerIntentDiagnosticRequest:
		return 0.15
	default:
		return 0.18
	}
}

func brokerIntentBoost(doc indexedTool, intent BrokerIntent, workflowTokens map[string]struct{}) (float64, []string) {
	score := 0.0
	rationale := []string{}
	switch intent {
	case BrokerIntentCodingQuery:
		if doc.tool.Category == ToolCategoryReadOnly || doc.tool.Category == ToolCategoryDevTest {
			score += 0.08
			rationale = append(rationale, "coding_safe_read_bias")
		}
		if doc.tool.NormalizedExposureClass() == ToolExposureDevelopment {
			score += 0.08
			rationale = append(rationale, "development_tool_fit")
		}
		if docHasAnyToken(doc, "repo", "search", "file", "files", "code", "workspace", "git", "diff", "test") {
			score += 0.14
			rationale = append(rationale, "coding_domain_match")
		}
		if doc.tool.Category == ToolCategoryWorkflowAction {
			score -= 0.08
			rationale = append(rationale, "coding_action_penalty")
		}
	case BrokerIntentDiagnosticRequest:
		if doc.tool.Category == ToolCategoryInternalDiagnostic {
			score += 0.18
			rationale = append(rationale, "diagnostic_category_match")
		}
		if doc.tool.Category == ToolCategoryReadOnly {
			score += 0.04
			rationale = append(rationale, "safe_inspection_fit")
		}
		if docHasAnyToken(doc, "diagnostic", "debug", "trace", "log", "logs", "error", "session", "duplicate") {
			score += 0.14
			rationale = append(rationale, "diagnostic_keyword_match")
		}
		if doc.tool.Category == ToolCategoryWorkflowAction {
			score -= 0.10
			rationale = append(rationale, "diagnostic_action_penalty")
		}
	case BrokerIntentAssistantTask:
		if doc.tool.Category == ToolCategoryReadOnly || doc.tool.Category == ToolCategoryChatSafe {
			score += 0.08
			rationale = append(rationale, "assistant_safe_tool_fit")
		}
		if doc.tool.Category == ToolCategoryWorkflowAction {
			score += 0.03
			rationale = append(rationale, "assistant_action_possible")
		}
	}
	if len(workflowTokens) > 0 {
		for token := range workflowTokens {
			if docHasAnyToken(doc, token) {
				score += 0.10
				rationale = append(rationale, "workflow_state_relevant")
				break
			}
		}
	}
	return score, rationale
}

func docHasAnyToken(doc indexedTool, tokens ...string) bool {
	for _, token := range tokens {
		token = normalizeSearchText(token)
		if token == "" {
			continue
		}
		if _, ok := doc.idTokens[token]; ok {
			return true
		}
		if _, ok := doc.nameTokens[token]; ok {
			return true
		}
		if _, ok := doc.aliasTokens[token]; ok {
			return true
		}
		if _, ok := doc.descTokens[token]; ok {
			return true
		}
		if _, ok := doc.tagTokens[token]; ok {
			return true
		}
	}
	return false
}

func brokerSuppressionReasonFor(toolEntry *Tool, status DiscoveryAvailabilityStatus, reason DiscoveryReasonCode, input BrokerInput, intent BrokerIntent) BrokerSuppressionReason {
	if toolEntry == nil {
		return ""
	}
	switch status {
	case DiscoveryAvailabilityAvailable:
		return ""
	case DiscoveryAvailabilityHidden, DiscoveryAvailabilityVisibleUnavailable:
		switch reason {
		case DiscoveryReasonRequiresEnvironment:
			return BrokerSuppressionEnvironmentBlocked
		case DiscoveryReasonRequiresAuthority:
			return BrokerSuppressionRequiresAuthority
		case DiscoveryReasonRequiresFeatureFlag:
			return BrokerSuppressionFeatureFlagBlocked
		case DiscoveryReasonDisallowedRiskTier:
			return BrokerSuppressionRiskTierTooHigh
		case DiscoveryReasonDisallowedTrustTier:
			return BrokerSuppressionTrustTierTooLow
		case DiscoveryReasonSuspended, DiscoveryReasonRemoved, DiscoveryReasonInvalid, DiscoveryReasonUnavailable:
			return BrokerSuppressionToolUnavailable
		case DiscoveryReasonRequiresMode:
			if intent == BrokerIntentDiagnosticRequest || toolEntry.Category == ToolCategoryInternalDiagnostic || toolEntry.NormalizedExposureClass() == ToolExposureInternal {
				return BrokerSuppressionRequiresDebugMode
			}
			return BrokerSuppressionRequiresCoderMode
		case DiscoveryReasonHidden, DiscoveryReasonHiddenByPolicy:
			if intent == BrokerIntentDiagnosticRequest || toolEntry.Category == ToolCategoryInternalDiagnostic || toolEntry.NormalizedExposureClass() == ToolExposureInternal {
				return BrokerSuppressionRequiresDebugMode
			}
			if toolEntry.Category == ToolCategoryDevTest || toolEntry.NormalizedExposureClass() == ToolExposureDevelopment || toolEntry.NormalizedExposureClass() == ToolExposureTest {
				return BrokerSuppressionRequiresCoderMode
			}
			if !toolVisibleInEnvironment(toolEntry, input.Environment) {
				return BrokerSuppressionEnvironmentBlocked
			}
			if !authoritySatisfies(input.UserAuthority, toolEntry.RequiredAuthority) {
				return BrokerSuppressionRequiresAuthority
			}
			return BrokerSuppressionToolUnavailable
		default:
			return BrokerSuppressionToolUnavailable
		}
	default:
		return ""
	}
}

func selectBrokerCandidates(available []brokerCandidate, max int) ([]brokerCandidate, []BrokerSuppressedTool) {
	if len(available) == 0 || max <= 0 {
		return nil, nil
	}
	if available[0].matchKind == "exact" || available[0].relevance >= 0.96 {
		return []brokerCandidate{available[0]}, nil
	}
	if len(available) > 1 && available[0].relevance-available[1].relevance >= 0.35 {
		return []brokerCandidate{available[0]}, nil
	}
	selected := make([]brokerCandidate, 0, len(available))
	suppressed := make([]BrokerSuppressedTool, 0)
	selectedOverlap := make(map[string]struct{}, max)
	for _, candidate := range available {
		if candidate.tool == nil {
			continue
		}
		if candidate.overlapKey != "" {
			if _, exists := selectedOverlap[candidate.overlapKey]; exists {
				suppressed = append(suppressed, BrokerSuppressedTool{
					ToolID:    candidate.tool.ToolID,
					Reason:    BrokerSuppressionTooManySimilarTools,
					Score:     candidate.relevance,
					MatchKind: candidate.matchKind,
					Rationale: append([]string(nil), candidate.rationale...),
				})
				continue
			}
		}
		if len(selected) >= max {
			break
		}
		selected = append(selected, candidate)
		if candidate.overlapKey != "" {
			selectedOverlap[candidate.overlapKey] = struct{}{}
		}
	}
	return selected, suppressed
}

func brokerChoiceMode(selected []brokerCandidate, available []brokerCandidate) ToolChoiceMode {
	switch len(selected) {
	case 0:
		return ToolChoiceModeNone
	case 1:
		if len(available) == 1 || selected[0].matchKind == "exact" || selected[0].relevance >= 0.85 {
			return ToolChoiceModeForcedSingle
		}
		return ToolChoiceModeAuto
	default:
		if len(available) > len(selected) {
			return ToolChoiceModeAllowedSubset
		}
		return ToolChoiceModeAuto
	}
}

func blockedBrokerReason(intent BrokerIntent, input BrokerInput, suppressed []BrokerSuppressedTool) string {
	if hasSuppressionReason(suppressed, BrokerSuppressionRequiresDebugMode) {
		return fmt.Sprintf("%s tools unavailable in current %s/%s context", brokerIntentLabel(intent), input.SessionMode, strings.ToLower(strings.TrimSpace(input.Environment)))
	}
	if hasSuppressionReason(suppressed, BrokerSuppressionRequiresCoderMode) {
		return "relevant development tools require a coder or debug context"
	}
	if hasSuppressionReason(suppressed, BrokerSuppressionEnvironmentBlocked) {
		return "relevant tools are blocked in the current environment"
	}
	if hasSuppressionReason(suppressed, BrokerSuppressionRequiresAuthority) {
		return "relevant tools exceed the current authority tier"
	}
	return "relevant tools were filtered by policy for this turn"
}

func selectedBrokerReason(intent BrokerIntent, input BrokerInput, selected []brokerCandidate, availableCount int) string {
	switch intent {
	case BrokerIntentCodingQuery:
		if len(selected) == 1 {
			return "coder turn resolved to one high-confidence inspection tool"
		}
		if availableCount > len(selected) && input.ModelProfile.maxToolCount() <= 2 {
			return "coder turn narrowed to a small local-model inspection subset"
		}
		return "coder turn narrowed to the most relevant inspection tools"
	case BrokerIntentDiagnosticRequest:
		if len(selected) == 1 {
			return "diagnostic turn resolved to a single safe inspection tool"
		}
		return "diagnostic turn narrowed to safe inspection tools"
	default:
		if len(selected) == 1 {
			return "targeted assistant request resolved to a single tool"
		}
		return "assistant turn narrowed to the most relevant tools"
	}
}

func brokerIntentLabel(intent BrokerIntent) string {
	switch intent {
	case BrokerIntentDiagnosticRequest:
		return "diagnostic"
	case BrokerIntentCodingQuery:
		return "coding"
	default:
		return "requested"
	}
}

func suppressExplicitTools(toolIDs []string, reason BrokerSuppressionReason) []BrokerSuppressedTool {
	if len(toolIDs) == 0 {
		return nil
	}
	out := make([]BrokerSuppressedTool, 0, len(toolIDs))
	for _, toolID := range dedupeStrings(toolIDs) {
		out = append(out, BrokerSuppressedTool{
			ToolID: toolID,
			Reason: reason,
		})
	}
	return out
}

func suppressedToolFromCandidate(candidate brokerCandidate) BrokerSuppressedTool {
	toolID := ""
	if candidate.tool != nil {
		toolID = candidate.tool.ToolID
	}
	return BrokerSuppressedTool{
		ToolID:    toolID,
		Reason:    candidate.suppressionReason,
		Score:     candidate.relevance,
		MatchKind: candidate.matchKind,
		Rationale: append([]string(nil), candidate.rationale...),
	}
}

func topSuppressedCandidates(candidates []brokerCandidate, reason BrokerSuppressionReason, limit int) []BrokerSuppressedTool {
	out := make([]BrokerSuppressedTool, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.tool == nil {
			continue
		}
		if _, ok := seen[candidate.tool.ToolID]; ok {
			continue
		}
		seen[candidate.tool.ToolID] = struct{}{}
		out = append(out, BrokerSuppressedTool{
			ToolID:    candidate.tool.ToolID,
			Reason:    reason,
			Score:     candidate.relevance,
			MatchKind: candidate.matchKind,
			Rationale: append([]string(nil), candidate.rationale...),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func dedupeSuppressedTools(items []BrokerSuppressedTool) []BrokerSuppressedTool {
	if len(items) == 0 {
		return nil
	}
	out := make([]BrokerSuppressedTool, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ToolID) == "" || item.Reason == "" {
			continue
		}
		key := item.ToolID + "|" + string(item.Reason)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func suppressedToolIDSet(items []BrokerSuppressedTool) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ToolID) == "" {
			continue
		}
		out[item.ToolID] = struct{}{}
	}
	return out
}

func brokerTraceCandidates(candidates []brokerCandidate, selectedToolIDs []string, suppressed []BrokerSuppressedTool) []BrokerTraceCandidate {
	if len(candidates) == 0 {
		return nil
	}
	selectedSet := stringSet(selectedToolIDs)
	suppressedByID := make(map[string]BrokerSuppressionReason, len(suppressed))
	for _, item := range suppressed {
		if strings.TrimSpace(item.ToolID) == "" || item.Reason == "" {
			continue
		}
		if _, exists := suppressedByID[item.ToolID]; !exists {
			suppressedByID[item.ToolID] = item.Reason
		}
	}
	out := make([]BrokerTraceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.tool == nil {
			continue
		}
		_, selected := selectedSet[candidate.tool.ToolID]
		out = append(out, BrokerTraceCandidate{
			ToolID:            candidate.tool.ToolID,
			Score:             candidate.relevance,
			MatchKind:         candidate.matchKind,
			OverlapKey:        candidate.overlapKey,
			Selected:          selected,
			SuppressionReason: suppressedByID[candidate.tool.ToolID],
			Rationale:         append([]string(nil), candidate.rationale...),
		})
	}
	return out
}

func connectorsSatisfied(toolEntry *Tool, availableConnectors map[string]struct{}) (bool, []string) {
	if toolEntry == nil || len(toolEntry.ConnectorDependencies) == 0 {
		return true, nil
	}
	if len(availableConnectors) == 0 {
		return false, []string{"connector_dependency_missing"}
	}
	for _, dependency := range toolEntry.ConnectorDependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		if _, ok := availableConnectors[dependency]; !ok {
			return false, []string{"connector_dependency_missing"}
		}
	}
	return true, nil
}

func brokerRiskFitBoost(toolEntry *Tool, maximumRiskTier string) (float64, []string) {
	if toolEntry == nil {
		return 0, nil
	}
	toolRank := riskTierRank(toolEntry.RiskTier)
	maxRank := riskTierRank(maximumRiskTier)
	switch {
	case toolRank <= 1:
		return 0.04, []string{"low_risk_fit"}
	case maxRank > 0 && toolRank > 0 && toolRank < maxRank:
		return 0.03, []string{"under_risk_ceiling"}
	default:
		return 0, nil
	}
}

func brokerModelPenalty(profile BrokerModelProfile, toolEntry *Tool) (float64, []string) {
	if toolEntry == nil {
		return 0, nil
	}
	complexity := toolComplexityScore(toolEntry)
	lower := strings.ToLower(strings.TrimSpace(profile.Name))
	switch {
	case strings.Contains(lower, "local_weak"), strings.Contains(lower, "weak"), strings.Contains(lower, "mini"), strings.Contains(lower, "haiku"):
		if complexity >= 3 {
			return -0.16, []string{"local_model_complexity_penalty"}
		}
	case strings.Contains(lower, "local_medium"), strings.Contains(lower, "medium"), strings.Contains(lower, "local"):
		if complexity >= 4 {
			return -0.08, []string{"local_model_complexity_penalty"}
		}
	}
	return 0, nil
}

func toolComplexityScore(toolEntry *Tool) int {
	if toolEntry == nil {
		return 0
	}
	score := 0
	if toolEntry.Category == ToolCategoryWorkflowAction || toolEntry.Category == ToolCategoryAdminCritical {
		score += 2
	}
	if len(toolEntry.InputSchema) > 0 {
		if properties, ok := toolEntry.InputSchema["properties"].(map[string]any); ok {
			score += len(properties)
		}
	}
	if len(toolEntry.ConnectorDependencies) > 0 {
		score++
	}
	if len(toolEntry.FeatureFlags) > 0 {
		score++
	}
	return score
}

func brokerOverlapKey(toolEntry *Tool) string {
	if toolEntry == nil {
		return ""
	}
	if domain := normalizeSearchText(toolEntry.Governance.Domain); domain != "" {
		return domain
	}
	if len(toolEntry.CapabilityTags) > 0 {
		normalized := make([]string, 0, len(toolEntry.CapabilityTags))
		for _, tag := range toolEntry.CapabilityTags {
			tag = normalizeSearchText(tag)
			if tag == "" {
				continue
			}
			normalized = append(normalized, tag)
		}
		if len(normalized) > 0 {
			sort.Strings(normalized)
			if len(normalized) > 2 {
				normalized = normalized[:2]
			}
			return strings.Join(normalized, "|")
		}
	}
	return string(toolEntry.Category)
}

func hasSuppressionReason(items []BrokerSuppressedTool, reason BrokerSuppressionReason) bool {
	for _, item := range items {
		if item.Reason == reason {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func hasAnyPhrase(haystack string, needles ...string) bool {
	haystack = " " + normalizeSearchText(haystack) + " "
	for _, needle := range needles {
		needle = normalizeSearchText(needle)
		if needle == "" {
			continue
		}
		if strings.Contains(haystack, " "+needle+" ") {
			return true
		}
	}
	return false
}
