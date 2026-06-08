package llmkb

import (
	"fmt"
	"strings"
)

func (p Provenance) Validate() error {
	if !p.Source.IsValid() {
		return fmt.Errorf("llmkb: invalid provenance source %q", p.Source)
	}
	if p.FieldClass != "" && !p.FieldClass.IsValid() {
		return fmt.Errorf("llmkb: invalid field class %q", p.FieldClass)
	}
	if p.AssertedAt.IsZero() {
		return fmt.Errorf("llmkb: provenance asserted_at is required")
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return fmt.Errorf("llmkb: provenance confidence must be between 0 and 1")
	}
	if p.Source == ProvenanceSourceManual && strings.TrimSpace(p.SourceDetail) == "" {
		return fmt.Errorf("llmkb: manual provenance requires source_detail")
	}
	return nil
}

func (c CapabilityProfile) Validate() error {
	if !c.AgenticClass.IsValid() || !c.CodingClass.IsValid() || !c.ReasoningClass.IsValid() {
		return fmt.Errorf("llmkb: capability classes must be valid")
	}
	if !c.InstructionFollowingClass.IsValid() || !c.ToolDisciplineClass.IsValid() || !c.SchemaReliabilityClass.IsValid() || !c.MultimodalClass.IsValid() {
		return fmt.Errorf("llmkb: capability class levels must be valid")
	}
	for _, v := range c.PrimaryUseCases {
		if !v.IsValid() {
			return fmt.Errorf("llmkb: invalid supported use case %q", v)
		}
	}
	for _, v := range c.InteractionModes {
		if !v.IsValid() {
			return fmt.Errorf("llmkb: invalid interaction mode %q", v)
		}
	}
	return nil
}

func (o OperationalState) Validate() error {
	if o.AvailabilityState == "" && o.RuntimeBackend == "" {
		return nil
	}
	if !o.AvailabilityState.IsValid() || !o.RuntimeBackend.IsValid() {
		return fmt.Errorf("llmkb: operational state requires valid availability_state and runtime_backend")
	}
	if o.ConnectorStatus != "" && !o.ConnectorStatus.IsValid() {
		return fmt.Errorf("llmkb: invalid connector_status %q", o.ConnectorStatus)
	}
	if o.AuthStatus != "" && !o.AuthStatus.IsValid() {
		return fmt.Errorf("llmkb: invalid auth_status %q", o.AuthStatus)
	}
	if o.HealthStatus != "" && !o.HealthStatus.IsValid() {
		return fmt.Errorf("llmkb: invalid health_status %q", o.HealthStatus)
	}
	if o.WarmState != "" && !o.WarmState.IsValid() {
		return fmt.Errorf("llmkb: invalid warm_state %q", o.WarmState)
	}
	return nil
}

func (r RoutingProfile) Validate() error {
	for _, v := range r.PreferredFor {
		if !v.IsValid() {
			return fmt.Errorf("llmkb: invalid preferred task class %q", v)
		}
	}
	if r.CostTier != "" && !r.CostTier.IsValid() {
		return fmt.Errorf("llmkb: invalid cost_tier %q", r.CostTier)
	}
	if r.LatencyTier != "" && !r.LatencyTier.IsValid() {
		return fmt.Errorf("llmkb: invalid latency_tier %q", r.LatencyTier)
	}
	if r.MaxRiskTierAllowed != "" && !r.MaxRiskTierAllowed.IsValid() {
		return fmt.Errorf("llmkb: invalid risk_tier %q", r.MaxRiskTierAllowed)
	}
	if r.AutonomyCeiling != "" && !r.AutonomyCeiling.IsValid() {
		return fmt.Errorf("llmkb: invalid autonomy_level %q", r.AutonomyCeiling)
	}
	if r.TrustLevel != "" && !r.TrustLevel.IsValid() {
		return fmt.Errorf("llmkb: invalid trust_level %q", r.TrustLevel)
	}
	return nil
}

func (p LLMProvider) Validate() error {
	if strings.TrimSpace(p.ProviderID) == "" {
		return fmt.Errorf("llmkb: provider_id is required")
	}
	if strings.TrimSpace(p.CanonicalName) == "" {
		return fmt.Errorf("llmkb: provider canonical_name is required")
	}
	if p.AuthMethod != "" && !p.AuthMethod.IsValid() {
		return fmt.Errorf("llmkb: invalid auth_method %q", p.AuthMethod)
	}
	if p.EndpointType != "" && !p.EndpointType.IsValid() {
		return fmt.Errorf("llmkb: invalid endpoint_type %q", p.EndpointType)
	}
	if p.PricingModel != "" && !p.PricingModel.IsValid() {
		return fmt.Errorf("llmkb: invalid pricing_model %q", p.PricingModel)
	}
	if p.TrustLevel != "" && !p.TrustLevel.IsValid() {
		return fmt.Errorf("llmkb: invalid trust_level %q", p.TrustLevel)
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("llmkb: provider created_at is required")
	}
	return p.Provenance.Validate()
}

func (p LLMProfile) Validate() error {
	if strings.TrimSpace(p.LLMID) == "" || strings.TrimSpace(p.CanonicalName) == "" || strings.TrimSpace(p.ProviderID) == "" {
		return fmt.Errorf("llmkb: llm_id, canonical_name, and provider_id are required")
	}
	if p.SchemaVersion < 0 {
		return fmt.Errorf("llmkb: schema_version must be non-negative")
	}
	if err := p.Capabilities.Validate(); err != nil {
		return err
	}
	if err := p.OperationalState.Validate(); err != nil {
		return err
	}
	if err := p.Routing.Validate(); err != nil {
		return err
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("llmkb: profile created_at is required")
	}
	return p.Provenance.Validate()
}

func (p LLMRuntimeInstance) Validate() error {
	if strings.TrimSpace(p.InstanceID) == "" || strings.TrimSpace(p.LLMID) == "" || strings.TrimSpace(p.ProviderID) == "" {
		return fmt.Errorf("llmkb: instance_id, llm_id, and provider_id are required")
	}
	if !p.RuntimeBackend.IsValid() {
		return fmt.Errorf("llmkb: runtime_backend is required and must be valid")
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("llmkb: instance created_at is required")
	}
	return p.Provenance.Validate()
}

func (d RouterDecision) Validate() error {
	if strings.TrimSpace(d.DecisionID) == "" {
		return fmt.Errorf("llmkb: decision_id is required")
	}
	if !d.TaskClass.IsValid() {
		return fmt.Errorf("llmkb: invalid task_class %q", d.TaskClass)
	}
	if d.RiskTier != "" && !d.RiskTier.IsValid() {
		return fmt.Errorf("llmkb: invalid risk_tier %q", d.RiskTier)
	}
	if d.SurfacingLevel != "" && !d.SurfacingLevel.IsValid() {
		return fmt.Errorf("llmkb: invalid surfacing_level %q", d.SurfacingLevel)
	}
	if d.CreatedAt.IsZero() {
		return fmt.Errorf("llmkb: created_at is required")
	}
	return d.Provenance.Validate()
}

func (v ExecutionOutcome) IsValid() bool {
	switch v {
	case ExecutionOutcomeSuccess, ExecutionOutcomeFailure, ExecutionOutcomeTimeout, ExecutionOutcomeUserAborted, ExecutionOutcomeFallback:
		return true
	}
	return false
}

func (e LLMExecutionRecord) Validate() error {
	if strings.TrimSpace(e.RecordID) == "" {
		return fmt.Errorf("llmkb: record_id is required")
	}
	if strings.TrimSpace(e.LLMID) == "" || strings.TrimSpace(e.InstanceID) == "" || strings.TrimSpace(e.RouterDecisionID) == "" {
		return fmt.Errorf("llmkb: llm_id, instance_id, and router_decision_id are required")
	}
	if !e.Outcome.IsValid() {
		return fmt.Errorf("llmkb: outcome is required and must be valid")
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("llmkb: created_at is required")
	}
	return e.Provenance.Validate()
}
