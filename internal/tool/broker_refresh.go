package tool

import "strings"

// ToolAvailabilityState captures the distinct capability states ATS must keep separate.
type ToolAvailabilityState string

const (
	ToolAvailabilityNotRegistered             ToolAvailabilityState = "not_registered"
	ToolAvailabilityRegisteredNotDiscoverable ToolAvailabilityState = "registered_not_discoverable"
	ToolAvailabilityDiscoverableNotLoadable   ToolAvailabilityState = "discoverable_not_loadable"
	ToolAvailabilityLoadableNotLoaded         ToolAvailabilityState = "loadable_not_loaded"
	ToolAvailabilityLoadedNotExposed          ToolAvailabilityState = "loaded_not_exposed"
	ToolAvailabilityExposedGovernanceBlocked  ToolAvailabilityState = "exposed_governance_blocked"
	ToolAvailabilityExposedReady              ToolAvailabilityState = "exposed_ready"
)

// CapabilityRefreshInput is the broker-owned refresh request used when a user asks
// for a capability that is absent from the current active/provider surface.
type CapabilityRefreshInput struct {
	QueryText                string
	BrokerInput              BrokerInput
	ActiveToolSet            *ActiveToolSet
	ExposedToolIDs           []string
	GovernanceBlockedToolIDs []string
	PreviousSnapshotID       string
}

// CapabilityRefreshResult records the late-enabled/discovery refresh outcome.
type CapabilityRefreshResult struct {
	QueryText                 string                `json:"query_text,omitempty"`
	SnapshotID                string                `json:"snapshot_id,omitempty"`
	PreviousSnapshotID        string                `json:"previous_snapshot_id,omitempty"`
	RegistryChanged           bool                  `json:"registry_changed,omitempty"`
	CachedDecisionInvalidated bool                  `json:"cached_decision_invalidated,omitempty"`
	AvailabilityState         ToolAvailabilityState `json:"availability_state,omitempty"`
	ToolID                    string                `json:"tool_id,omitempty"`
	ShouldLoad                bool                  `json:"should_load,omitempty"`
	ShouldExpose              bool                  `json:"should_expose,omitempty"`
	Discovery                 *DiscoveryResult      `json:"discovery,omitempty"`
	Broker                    *BrokerResolution     `json:"broker,omitempty"`
}

// RefreshCapability queries discovery and the registry-backed broker before a hard
// unavailable decision is accepted for a requested capability.
func (b *ToolBroker) RefreshCapability(input CapabilityRefreshInput) CapabilityRefreshResult {
	b.refreshIndexIfStale()
	result := CapabilityRefreshResult{
		QueryText:          strings.TrimSpace(input.QueryText),
		PreviousSnapshotID: strings.TrimSpace(input.PreviousSnapshotID),
		SnapshotID:         snapshotIDFromIndex(b),
	}
	result.RegistryChanged = result.PreviousSnapshotID != "" && result.PreviousSnapshotID != result.SnapshotID

	if b == nil || b.index == nil {
		result.AvailabilityState = ToolAvailabilityNotRegistered
		return result
	}

	brokerInput := input.BrokerInput.normalize()
	if strings.TrimSpace(brokerInput.UserInput) == "" {
		brokerInput.UserInput = result.QueryText
	}
	if input.ActiveToolSet != nil && len(brokerInput.ActiveToolIDs) == 0 {
		brokerInput.ActiveToolIDs = input.ActiveToolSet.ToolIDs()
	}

	exact := b.index.Discover(DiscoveryQuery{
		Mode:    DiscoveryQueryModeExact,
		Text:    result.QueryText,
		Limit:   5,
		Context: brokerInput.discoveryContext(),
	})
	search := b.index.Discover(DiscoveryQuery{
		Mode:    DiscoveryQueryModeLexical,
		Text:    firstNonEmpty(brokerInput.UserInput, result.QueryText),
		Limit:   5,
		Context: brokerInput.discoveryContext(),
	})
	brokerResolution := b.Resolve(brokerInput)

	discovery := chooseRefreshDiscovery(exact, search)
	if discovery != nil {
		discoveryCopy := *discovery
		result.Discovery = &discoveryCopy
	}
	brokerCopy := brokerResolution
	result.Broker = &brokerCopy

	candidate := b.refreshPrimaryCandidate(result.QueryText, brokerInput.discoveryContext(), exact, search)
	if candidate == nil || candidate.Tool == nil {
		result.AvailabilityState = ToolAvailabilityNotRegistered
		result.CachedDecisionInvalidated = result.RegistryChanged &&
			((result.Discovery != nil && len(result.Discovery.RelatedMatches) > 0) || len(brokerResolution.SelectedToolIDs) > 0)
		return result
	}

	result.ToolID = strings.TrimSpace(candidate.Tool.ToolID)
	switch candidate.AvailabilityStatus {
	case DiscoveryAvailabilityHidden:
		result.AvailabilityState = ToolAvailabilityRegisteredNotDiscoverable
		result.CachedDecisionInvalidated = result.RegistryChanged
		return result
	case DiscoveryAvailabilityVisibleUnavailable:
		result.AvailabilityState = ToolAvailabilityDiscoverableNotLoadable
		result.CachedDecisionInvalidated = result.RegistryChanged
		return result
	}

	loaded := activeSetContainsTool(input.ActiveToolSet, candidate.Tool)
	exposed := containsToolID(input.ExposedToolIDs, result.ToolID)
	govBlocked := containsToolID(input.GovernanceBlockedToolIDs, result.ToolID)

	switch {
	case loaded && exposed && govBlocked:
		result.AvailabilityState = ToolAvailabilityExposedGovernanceBlocked
		result.CachedDecisionInvalidated = result.RegistryChanged
		return result
	case loaded && !exposed:
		result.AvailabilityState = ToolAvailabilityLoadedNotExposed
		result.ShouldExpose = true
		result.CachedDecisionInvalidated = true
		return result
	case loaded && exposed:
		result.AvailabilityState = ToolAvailabilityExposedReady
		result.CachedDecisionInvalidated = result.RegistryChanged
		return result
	case containsToolID(brokerResolution.SelectedToolIDs, result.ToolID):
		result.AvailabilityState = ToolAvailabilityLoadableNotLoaded
		result.ShouldLoad = true
		result.CachedDecisionInvalidated = true
		return result
	default:
		result.AvailabilityState = ToolAvailabilityDiscoverableNotLoadable
		result.CachedDecisionInvalidated = result.RegistryChanged
		return result
	}
}

func chooseRefreshDiscovery(exact DiscoveryResult, search DiscoveryResult) *DiscoveryResult {
	if exact.ExactMatch != nil || len(exact.RelatedMatches) > 0 || exact.MissingCapability != nil {
		return &exact
	}
	if len(search.RelatedMatches) > 0 || search.MissingCapability != nil {
		return &search
	}
	return nil
}

func (b *ToolBroker) refreshPrimaryCandidate(query string, ctx DiscoveryContext, exact DiscoveryResult, search DiscoveryResult) *DiscoveryCandidate {
	if b != nil && b.registry != nil {
		if lookup, ok := b.registry.LookupExact(strings.TrimSpace(query)); ok && lookup.Tool != nil {
			status, reason := evaluateDiscoveryPolicy(lookup.Tool, ctx)
			return &DiscoveryCandidate{
				SnapshotID:            firstNonEmpty(lookup.SnapshotID, snapshotIDFromIndex(b)),
				Tool:                  cloneTool(lookup.Tool),
				Relevance:             1.0,
				MatchKind:             "exact",
				AvailabilityStatus:    status,
				UnavailableReasonCode: reason,
			}
		}
	}
	if exact.ExactMatch != nil {
		return exact.ExactMatch
	}
	if len(search.RelatedMatches) > 0 {
		candidate := search.RelatedMatches[0]
		return &candidate
	}
	if len(exact.RelatedMatches) > 0 {
		candidate := exact.RelatedMatches[0]
		return &candidate
	}
	return nil
}

func activeSetContainsTool(set *ActiveToolSet, tool *Tool) bool {
	if set == nil || tool == nil {
		return false
	}
	if set.ContainsToolVersion(tool.ToolID, tool.SchemaVersion) {
		return true
	}
	return set.ContainsTool(tool.ToolID)
}
