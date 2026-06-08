package llmkb

var (
	availabilityStateValues = map[AvailabilityState]struct{}{AvailabilityStateUnknown: {}, AvailabilityStateAvailable: {}, AvailabilityStateUnavailable: {}, AvailabilityStateDegraded: {}}
	runtimeBackendValues    = map[RuntimeBackend]struct{}{RuntimeBackendAPI: {}, RuntimeBackendOllama: {}, RuntimeBackendOpenAI: {}, RuntimeBackendAnthropic: {}, RuntimeBackendOpenRouter: {}, RuntimeBackendCustom: {}}
	connectorStatusValues   = map[ConnectorStatus]struct{}{ConnectorStatusUnknown: {}, ConnectorStatusConfigured: {}, ConnectorStatusConnected: {}, ConnectorStatusFailed: {}}
	authStatusValues        = map[AuthStatus]struct{}{AuthStatusUnknown: {}, AuthStatusUnauthenticated: {}, AuthStatusAuthenticated: {}, AuthStatusExpired: {}}
	healthStatusValues      = map[HealthStatus]struct{}{HealthStatusUnknown: {}, HealthStatusHealthy: {}, HealthStatusDegraded: {}, HealthStatusFailing: {}}
	warmStateValues         = map[WarmState]struct{}{WarmStateCold: {}, WarmStateWarming: {}, WarmStateWarm: {}}
	rateLimitStateValues    = map[RateLimitState]struct{}{RateLimitStateUnknown: {}, RateLimitStateOpen: {}, RateLimitStateNear: {}, RateLimitStateLimited: {}}
	loadedStatusValues      = map[LoadedStatus]struct{}{LoadedStatusUnknown: {}, LoadedStatusNotLoaded: {}, LoadedStatusLoaded: {}, LoadedStatusEvicted: {}}
	releaseChannelValues    = map[ReleaseChannel]struct{}{ReleaseChannelStable: {}, ReleaseChannelPreview: {}, ReleaseChannelExperimental: {}}
	deprecationStatusValues = map[DeprecationStatus]struct{}{DeprecationStatusActive: {}, DeprecationStatusDeprecated: {}, DeprecationStatusSunsetting: {}, DeprecationStatusDiscontinued: {}}
	hostingModeValues       = map[HostingMode]struct{}{HostingModeCloudAPI: {}, HostingModeLocal: {}, HostingModeSelfHosted: {}, HostingModeCloudManaged: {}}
	authMethodValues        = map[AuthMethod]struct{}{AuthMethodAPIKey: {}, AuthMethodOAuth: {}, AuthMethodNone: {}, AuthMethodBearerToken: {}, AuthMethodCustom: {}}
	endpointTypeValues      = map[EndpointType]struct{}{EndpointTypeOpenAICompat: {}, EndpointTypeAnthropic: {}, EndpointTypeGoogle: {}, EndpointTypeCustom: {}, EndpointTypeOllamaLocal: {}}
	pricingModelValues      = map[PricingModel]struct{}{PricingModelPerToken: {}, PricingModelPerRequest: {}, PricingModelSubscription: {}, PricingModelFree: {}, PricingModelSelfHosted: {}}
	reliabilityTrendValues  = map[ReliabilityTrend]struct{}{ReliabilityTrendUnknown: {}, ReliabilityTrendImproving: {}, ReliabilityTrendStable: {}, ReliabilityTrendDeclining: {}}
	surfacingLevelValues    = map[SurfacingLevel]struct{}{SurfacingLevelHidden: {}, SurfacingLevelInternal: {}, SurfacingLevelOwner: {}, SurfacingLevelDefault: {}}
	proposalStatusValues    = map[ProposalStatus]struct{}{ProposalStatusPending: {}, ProposalStatusApproved: {}, ProposalStatusRejected: {}, ProposalStatusApplied: {}}
	costTierValues          = map[CostTier]struct{}{CostTierFree: {}, CostTierCheap: {}, CostTierModerate: {}, CostTierExpensive: {}}
	latencyTierValues       = map[LatencyTier]struct{}{LatencyTierLow: {}, LatencyTierMedium: {}, LatencyTierHigh: {}}
	riskTierValues          = map[RiskTier]struct{}{RiskTierLow: {}, RiskTierMedium: {}, RiskTierHigh: {}, RiskTierCritical: {}}
	trustLevelValues        = map[TrustLevel]struct{}{TrustLevelUnknown: {}, TrustLevelLow: {}, TrustLevelMedium: {}, TrustLevelHigh: {}}
	autonomyLevelValues     = map[AutonomyLevel]struct{}{AutonomyLevelChat: {}, AutonomyLevelAssistive: {}, AutonomyLevelAutonomous: {}}
	fitScoreValues          = map[FitScore]struct{}{FitScorePoor: {}, FitScoreFair: {}, FitScoreGood: {}, FitScoreStrong: {}, FitScoreIdeal: {}}
	rejectionCodeValues     = map[RejectionCode]struct{}{RejectionCodeUnsupportedTask: {}, RejectionCodeToolsUnavailable: {}, RejectionCodeOverBudget: {}, RejectionCodePolicyBlocked: {}, RejectionCodeUnavailable: {}, RejectionCodeUnhealthy: {}, RejectionCodeContextOverflow: {}, RejectionCodeAuthentication: {}, RejectionCodeRateLimited: {}, RejectionCodeDeprecation: {}}
	failurePatternValues    = map[FailurePattern]struct{}{FailurePatternHallucination: {}, FailurePatternToolAvoidance: {}, FailurePatternSchemaViolation: {}, FailurePatternTimeout: {}, FailurePatternRateLimit: {}, FailurePatternAuthFailure: {}, FailurePatternFormattingDrift: {}, FailurePatternPromptInjection: {}, FailurePatternInstructionDrift: {}, FailurePatternLowRecall: {}, FailurePatternLowPrecision: {}}
	provenanceSourceValues  = map[ProvenanceSource]struct{}{ProvenanceSourceVendorDoc: {}, ProvenanceSourceCatalog: {}, ProvenanceSourceCuratedSeed: {}, ProvenanceSourceProbe: {}, ProvenanceSourceExecution: {}, ProvenanceSourceReflection: {}, ProvenanceSourceManual: {}, ProvenanceSourceImported: {}}
)

func (v AvailabilityState) IsValid() bool { return inSet(v, availabilityStateValues) }
func ParseAvailabilityState(raw string) (AvailabilityState, error) {
	return parseEnum(raw, availabilityStateValues, "availability_state")
}
func (v RuntimeBackend) IsValid() bool { return inSet(v, runtimeBackendValues) }
func ParseRuntimeBackend(raw string) (RuntimeBackend, error) {
	return parseEnum(raw, runtimeBackendValues, "runtime_backend")
}
func (v ConnectorStatus) IsValid() bool { return inSet(v, connectorStatusValues) }
func ParseConnectorStatus(raw string) (ConnectorStatus, error) {
	return parseEnum(raw, connectorStatusValues, "connector_status")
}
func (v AuthStatus) IsValid() bool { return inSet(v, authStatusValues) }
func ParseAuthStatus(raw string) (AuthStatus, error) {
	return parseEnum(raw, authStatusValues, "auth_status")
}
func (v HealthStatus) IsValid() bool { return inSet(v, healthStatusValues) }
func ParseHealthStatus(raw string) (HealthStatus, error) {
	return parseEnum(raw, healthStatusValues, "health_status")
}
func (v WarmState) IsValid() bool { return inSet(v, warmStateValues) }
func ParseWarmState(raw string) (WarmState, error) {
	return parseEnum(raw, warmStateValues, "warm_state")
}
func (v RateLimitState) IsValid() bool { return inSet(v, rateLimitStateValues) }
func ParseRateLimitState(raw string) (RateLimitState, error) {
	return parseEnum(raw, rateLimitStateValues, "rate_limit_state")
}
func (v LoadedStatus) IsValid() bool { return inSet(v, loadedStatusValues) }
func ParseLoadedStatus(raw string) (LoadedStatus, error) {
	return parseEnum(raw, loadedStatusValues, "loaded_status")
}
func (v ReleaseChannel) IsValid() bool { return inSet(v, releaseChannelValues) }
func ParseReleaseChannel(raw string) (ReleaseChannel, error) {
	return parseEnum(raw, releaseChannelValues, "release_channel")
}
func (v DeprecationStatus) IsValid() bool { return inSet(v, deprecationStatusValues) }
func ParseDeprecationStatus(raw string) (DeprecationStatus, error) {
	return parseEnum(raw, deprecationStatusValues, "deprecation_status")
}
func (v HostingMode) IsValid() bool { return inSet(v, hostingModeValues) }
func ParseHostingMode(raw string) (HostingMode, error) {
	normalized := normalizeEnumValue(raw)
	if normalized == "cloud" {
		normalized = string(HostingModeCloudAPI)
	}
	raw = normalized
	return parseEnum(raw, hostingModeValues, "hosting_mode")
}
func (v ReliabilityTrend) IsValid() bool { return inSet(v, reliabilityTrendValues) }
func ParseReliabilityTrend(raw string) (ReliabilityTrend, error) {
	return parseEnum(raw, reliabilityTrendValues, "reliability_trend")
}
func (v SurfacingLevel) IsValid() bool { return inSet(v, surfacingLevelValues) }
func ParseSurfacingLevel(raw string) (SurfacingLevel, error) {
	return parseEnum(raw, surfacingLevelValues, "surfacing_level")
}
func (v ProposalStatus) IsValid() bool { return inSet(v, proposalStatusValues) }
func ParseProposalStatus(raw string) (ProposalStatus, error) {
	return parseEnum(raw, proposalStatusValues, "proposal_status")
}
func (v CostTier) IsValid() bool                 { return inSet(v, costTierValues) }
func ParseCostTier(raw string) (CostTier, error) { return parseEnum(raw, costTierValues, "cost_tier") }
func (v LatencyTier) IsValid() bool              { return inSet(v, latencyTierValues) }
func ParseLatencyTier(raw string) (LatencyTier, error) {
	return parseEnum(raw, latencyTierValues, "latency_tier")
}
func (v RiskTier) IsValid() bool                 { return inSet(v, riskTierValues) }
func ParseRiskTier(raw string) (RiskTier, error) { return parseEnum(raw, riskTierValues, "risk_tier") }
func (v TrustLevel) IsValid() bool               { return inSet(v, trustLevelValues) }
func ParseTrustLevel(raw string) (TrustLevel, error) {
	return parseEnum(raw, trustLevelValues, "trust_level")
}
func (v AutonomyLevel) IsValid() bool { return inSet(v, autonomyLevelValues) }
func ParseAutonomyLevel(raw string) (AutonomyLevel, error) {
	return parseEnum(raw, autonomyLevelValues, "autonomy_level")
}
func (v FitScore) IsValid() bool                 { return inSet(v, fitScoreValues) }
func ParseFitScore(raw string) (FitScore, error) { return parseEnum(raw, fitScoreValues, "fit_score") }
func (v RejectionCode) IsValid() bool            { return inSet(v, rejectionCodeValues) }
func ParseRejectionCode(raw string) (RejectionCode, error) {
	return parseEnum(raw, rejectionCodeValues, "rejection_code")
}
func (v FailurePattern) IsValid() bool { return inSet(v, failurePatternValues) }
func ParseFailurePattern(raw string) (FailurePattern, error) {
	return parseEnum(raw, failurePatternValues, "failure_pattern")
}
func (v ProvenanceSource) IsValid() bool { return inSet(v, provenanceSourceValues) }
func ParseProvenanceSource(raw string) (ProvenanceSource, error) {
	return parseEnum(raw, provenanceSourceValues, "provenance_source")
}
func (v AuthMethod) IsValid() bool { return inSet(v, authMethodValues) }
func ParseAuthMethod(raw string) (AuthMethod, error) {
	return parseEnum(raw, authMethodValues, "auth_method")
}
func (v EndpointType) IsValid() bool { return inSet(v, endpointTypeValues) }
func ParseEndpointType(raw string) (EndpointType, error) {
	return parseEnum(raw, endpointTypeValues, "endpoint_type")
}
func (v PricingModel) IsValid() bool { return inSet(v, pricingModelValues) }
func ParsePricingModel(raw string) (PricingModel, error) {
	return parseEnum(raw, pricingModelValues, "pricing_model")
}
