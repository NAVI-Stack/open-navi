package tool

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// DiscoveryQueryMode identifies the search strategy used to produce discovery output.
type DiscoveryQueryMode string

const (
	DiscoveryQueryModeExact    DiscoveryQueryMode = "exact"
	DiscoveryQueryModeLexical  DiscoveryQueryMode = "lexical"
	DiscoveryQueryModeSemantic DiscoveryQueryMode = "semantic"
	DiscoveryQueryModeRelated  DiscoveryQueryMode = "related"
)

// DiscoveryAvailabilityStatus captures whether the query resolved a usable tool,
// a known-but-unavailable tool, related candidates only, or an explicit miss.
type DiscoveryAvailabilityStatus string

const (
	DiscoveryAvailabilityAvailable          DiscoveryAvailabilityStatus = "available"
	DiscoveryAvailabilityVisibleUnavailable DiscoveryAvailabilityStatus = "visible_unavailable"
	DiscoveryAvailabilityHidden             DiscoveryAvailabilityStatus = "hidden"
	DiscoveryAvailabilityRelatedOnly        DiscoveryAvailabilityStatus = "related_only"
	DiscoveryAvailabilityExplicitMiss       DiscoveryAvailabilityStatus = "explicit_miss"
)

// DiscoveryReasonCode explains why a discovered tool or query outcome is unavailable.
type DiscoveryReasonCode string

const (
	DiscoveryReasonNone                DiscoveryReasonCode = ""
	DiscoveryReasonExactMiss           DiscoveryReasonCode = "exact_miss"
	DiscoveryReasonNoMatch             DiscoveryReasonCode = "no_match"
	DiscoveryReasonHidden              DiscoveryReasonCode = "hidden"
	DiscoveryReasonSuspended           DiscoveryReasonCode = "suspended"
	DiscoveryReasonRemoved             DiscoveryReasonCode = "removed"
	DiscoveryReasonInvalid             DiscoveryReasonCode = "invalid"
	DiscoveryReasonUnavailable         DiscoveryReasonCode = "unavailable"
	DiscoveryReasonRequiresEnvironment DiscoveryReasonCode = "requires_environment"
	DiscoveryReasonRequiresMode        DiscoveryReasonCode = "requires_mode"
	DiscoveryReasonRequiresAuthority   DiscoveryReasonCode = "requires_authority"
	DiscoveryReasonRequiresFeatureFlag DiscoveryReasonCode = "requires_feature_flag"
	DiscoveryReasonDisallowedRiskTier  DiscoveryReasonCode = "disallowed_risk_tier"
	DiscoveryReasonDisallowedTrustTier DiscoveryReasonCode = "disallowed_trust_tier"
	DiscoveryReasonHiddenByPolicy      DiscoveryReasonCode = "hidden_by_policy"
)

// DiscoveryNextAction suggests the next safe step without implying authority.
type DiscoveryNextAction string

const (
	DiscoveryNextActionNone              DiscoveryNextAction = "none"
	DiscoveryNextActionUseExactMatch     DiscoveryNextAction = "use_exact_match"
	DiscoveryNextActionInspectRelated    DiscoveryNextAction = "inspect_related_matches"
	DiscoveryNextActionReviewUnavailable DiscoveryNextAction = "review_unavailable_tool"
	DiscoveryNextActionRequestCapability DiscoveryNextAction = "request_missing_capability"
)

// DiscoveryQuery is the caller-supplied request for tool discovery.
type DiscoveryQuery struct {
	ID      string
	Mode    DiscoveryQueryMode
	Text    string
	Limit   int
	Context DiscoveryContext
}

// DiscoveryCandidate is the structured representation of one discovery result.
type DiscoveryCandidate struct {
	SnapshotID            string
	Tool                  *Tool
	Relevance             float64
	MatchKind             string
	AvailabilityStatus    DiscoveryAvailabilityStatus
	UnavailableReasonCode DiscoveryReasonCode
}

// MissingCapabilityEnvelope signals that no exact or related tool currently satisfies the query.
type MissingCapabilityEnvelope struct {
	QueryID        string
	QueryMode      DiscoveryQueryMode
	QueryText      string
	ReasonCode     DiscoveryReasonCode
	RelatedToolIDs []string
}

// DiscoveryResult is the standardized discovery contract shared by index, broker,
// and unknown-tool recovery flows.
type DiscoveryResult struct {
	QueryID               string
	QueryMode             DiscoveryQueryMode
	SnapshotID            string
	ExactMatch            *DiscoveryCandidate
	RelatedMatches        []DiscoveryCandidate
	AvailabilityStatus    DiscoveryAvailabilityStatus
	UnavailableReasonCode DiscoveryReasonCode
	SuggestedNextAction   DiscoveryNextAction
	ExplicitMiss          bool
	MissingCapability     *MissingCapabilityEnvelope
}

var discoveryQuerySeq atomic.Uint64

// Discover resolves a standardized discovery result over the index.
func (idx *Index) Discover(query DiscoveryQuery) DiscoveryResult {
	mode := query.Mode
	if mode == "" {
		mode = DiscoveryQueryModeExact
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	result := DiscoveryResult{
		QueryID:    firstNonEmpty(query.ID, nextDiscoveryQueryID()),
		QueryMode:  mode,
		SnapshotID: idx.SnapshotID(),
	}

	switch mode {
	case DiscoveryQueryModeExact:
		queryText := normalizeSearchText(query.Text)
		if doc, ok := idx.byID[queryText]; ok {
			candidate, visible := idx.discoveryCandidateForDoc(doc, query.Context, 1.0, "exact")
			if visible {
				result.ExactMatch = &candidate
				result.AvailabilityStatus = candidate.AvailabilityStatus
				result.UnavailableReasonCode = candidate.UnavailableReasonCode
				if candidate.AvailabilityStatus == DiscoveryAvailabilityAvailable {
					result.SuggestedNextAction = DiscoveryNextActionUseExactMatch
				} else {
					result.SuggestedNextAction = DiscoveryNextActionReviewUnavailable
				}
				return result
			}
		}
		result.RelatedMatches = idx.discoveryCandidatesFromSearch(idx.relatedCandidates(queryText, limit), query.Context)
		result.ExplicitMiss = true
		result.UnavailableReasonCode = DiscoveryReasonExactMiss
		if len(result.RelatedMatches) > 0 {
			result.AvailabilityStatus = DiscoveryAvailabilityRelatedOnly
			result.SuggestedNextAction = DiscoveryNextActionInspectRelated
			return result
		}
		result.AvailabilityStatus = DiscoveryAvailabilityExplicitMiss
		result.UnavailableReasonCode = DiscoveryReasonNoMatch
		result.SuggestedNextAction = DiscoveryNextActionRequestCapability
		result.MissingCapability = buildMissingCapabilityEnvelope(result, query.Text)
		return result
	case DiscoveryQueryModeLexical:
		result.RelatedMatches = idx.discoveryCandidatesFromSearch(idx.LexicalSearch(query.Text, limit), query.Context)
	case DiscoveryQueryModeSemantic:
		result.RelatedMatches = idx.discoveryCandidatesFromSearch(idx.RelatedTools(query.Text, limit), query.Context)
	case DiscoveryQueryModeRelated:
		result.RelatedMatches = idx.discoveryCandidatesFromSearch(idx.RelatedTools(query.Text, limit), query.Context)
	default:
		result.RelatedMatches = idx.discoveryCandidatesFromSearch(idx.RelatedTools(query.Text, limit), query.Context)
	}

	if len(result.RelatedMatches) > 0 {
		result.AvailabilityStatus = DiscoveryAvailabilityRelatedOnly
		result.SuggestedNextAction = DiscoveryNextActionInspectRelated
		return result
	}
	result.AvailabilityStatus = DiscoveryAvailabilityExplicitMiss
	result.UnavailableReasonCode = DiscoveryReasonNoMatch
	result.SuggestedNextAction = DiscoveryNextActionRequestCapability
	result.ExplicitMiss = true
	result.MissingCapability = buildMissingCapabilityEnvelope(result, query.Text)
	return result
}

func (idx *Index) discoveryCandidateForDoc(doc indexedTool, ctx DiscoveryContext, relevance float64, matchKind string) (DiscoveryCandidate, bool) {
	status, reason := evaluateDiscoveryPolicy(doc.tool, ctx)
	visible := status == DiscoveryAvailabilityAvailable ||
		status == DiscoveryAvailabilityVisibleUnavailable ||
		(status == DiscoveryAvailabilityHidden && ctx.canRevealHiddenExactMatches())
	if !visible {
		return DiscoveryCandidate{}, false
	}
	return DiscoveryCandidate{
		SnapshotID:            idx.snapshotID,
		Tool:                  cloneTool(doc.tool),
		Relevance:             clampScore(relevance),
		MatchKind:             matchKind,
		AvailabilityStatus:    status,
		UnavailableReasonCode: reason,
	}, true
}

func (idx *Index) discoveryCandidatesFromSearch(candidates []SearchCandidate, ctx DiscoveryContext) []DiscoveryCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]DiscoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Tool == nil {
			continue
		}
		doc, ok := idx.byID[normalizeSearchText(candidate.Tool.ToolID)]
		if !ok {
			continue
		}
		discoveryCandidate, visible := idx.discoveryCandidateForDoc(doc, ctx, candidate.Relevance, candidate.MatchKind)
		if !visible || discoveryCandidate.AvailabilityStatus == DiscoveryAvailabilityHidden {
			continue
		}
		out = append(out, discoveryCandidate)
	}
	return out
}

func discoveryAvailabilityForTool(toolEntry *Tool, discoverable bool) (DiscoveryAvailabilityStatus, DiscoveryReasonCode) {
	if toolEntry == nil {
		return DiscoveryAvailabilityExplicitMiss, DiscoveryReasonNoMatch
	}
	if discoverable {
		return DiscoveryAvailabilityAvailable, DiscoveryReasonNone
	}
	if toolEntry.Hidden {
		return DiscoveryAvailabilityHidden, DiscoveryReasonHidden
	}
	switch toolEntry.Status {
	case ToolStatusSuspended:
		return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonSuspended
	case ToolStatusRemoved:
		return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRemoved
	case ToolStatusInvalid:
		return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonInvalid
	default:
		return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonUnavailable
	}
}

func buildMissingCapabilityEnvelope(result DiscoveryResult, queryText string) *MissingCapabilityEnvelope {
	relatedToolIDs := make([]string, 0, len(result.RelatedMatches))
	for _, candidate := range result.RelatedMatches {
		if candidate.Tool == nil {
			continue
		}
		relatedToolIDs = append(relatedToolIDs, candidate.Tool.ToolID)
	}
	return &MissingCapabilityEnvelope{
		QueryID:        result.QueryID,
		QueryMode:      result.QueryMode,
		QueryText:      strings.TrimSpace(queryText),
		ReasonCode:     result.UnavailableReasonCode,
		RelatedToolIDs: relatedToolIDs,
	}
}

func nextDiscoveryQueryID() string {
	return fmt.Sprintf("tool-discovery-%d", discoveryQuerySeq.Add(1))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
