# NAVI-KB-001: LLM-Knowledge Base (LLM-KB) v3.0 Architecture

This document specifies the v3.0 knowledge representation and governance model for NAVI's LLM ecosystem. This architecture enables autonomous model selection and promotion while maintaining strict administrative guardrails.

## Data Model Separation

The v3 model enforces a strict boundary between three data classes:

1. **Governed (Immutable without Proposal)**:
   - Tiered capabilities (Agentic, Coding, Reasoning).
   - Core identifiers (LLMID, Aliases, Provider).
   - Administrative bounds (Autonomy Ceiling, Max Risk Tier, Probation period).
2. **Observed (Empirical)**:
   - Performance metrics (Latency, Context Utility).
   - Reliability data (Historical Success Rates, Failure counts).
   - Execution lineages (Linkage between Prompt, Model, and Outcome).
3. **Inferred (Calculated)**:
   - Task fit scores (Normalized preference for Chat vs. Coding vs. Automation).
   - Effective routing priority.

## Persistence Schema (SQLite)

The v3 architecture utilizes a multi-table structure to support high-granularity updates without full record rewrites:

- `llm_profiles`: Central registry and identity.
- `llm_profile_capabilities`: Tiered capability definitions.
- `llm_profile_features`: Technical tool/vision/context support features.
- `llm_profile_operational_state`: Real-time availability and hosting mode (local vs. cloud).
- `llm_profile_routing`: Administrative guardrails and preferred task classifications.
- `llm_execution_records`: Detailed historical trace for every model interaction.
- `routing_proposal_items`: Staging area for autonomous promotion candidates.

## Governance & Promotion Lifecycle

Models move through a structured promotion pipeline based on empirical success:

1. **Probation**: New models enter the KB with a "probation" timer (default: 7 days). They are eligible for trial runs but not "Preferred" tasks.
2. **Observation**: Execution records accumulate success rates and sample sizes.
3. **Proposal**: Once `PromotionThreshold` and `PromotionMinSamples` are met, the orchestrator generates a `RoutingProposal`.
4. **Promotion**: Upon approval (manual or autonomous based on `AutonomyCeiling`), the model is added to the `PreferredFor` task set.

## Known Limitations & Future Considerations

### Performance: N+1 Discovery Pattern
The current repository implementation utilizes an $N+1$ query pattern in discovery methods (`ListProfiles`, `ListPreferredForTask`).
- **Current State**: Loads ID list first, then performs 7 separate sub-table reads for each profile object.
- **Future Optimization**: For catalogs exceeding 100+ models, these should be refactored into joined SQL views or utilizing SQLite's JSON capabilities to aggregate sub-record data in a single pass.
