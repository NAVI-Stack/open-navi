# Tier-3 Residual Gaps

Tier-3 conceptual gaps that remain partially implemented after the main concept-vs-implementation passes. Tracked for prioritization and cross-linking with the [concept-vs-implementation-gap-analysis plan](../../.cursor/plans/concept-vs-implementation-gap-analysis.plan.md) and [gap report](../concept-vs-implementation-gap-report.md).

---

## 1. Data lifecycle

| Area | Status | Location | Next steps |
|------|--------|----------|------------|
| **Archive** | Partial | `store.ArchiveEntity`, `worldmodel.ArchiveEntity` (façade) | No universal “archive” path for all entity deletes; design default for Delete is Archive. Use Archive for soft-delete when removing entities. |
| **Forget** | Implemented | `store.ForgetMemory`, `worldmodel.ForgetMemoryByID` | Downstream re-evaluation via `FlagDerivativesForReEvaluation`; cascade to proposals. |
| **Supersede** | Implemented | `store.SupersedeFact`, `worldmodel.SupersedeFactWith`, `governor.ActionDescriptorForLifecycleSupersede` | ValidateAction returns RequiresConfirmation for owner-set supersede; callers must create proposal and await approval before calling WorldModel. |
| **Tombstone** | Implemented | `store.TombstoneEntity`, `worldmodel.TombstoneEntity`, `governor.ActionDescriptorForLifecycleTombstone` | ValidateAction returns RequiresConfirmation for tombstone; callers must create proposal and await approval before executing. |
| **Cascade** | Implemented | `AutoDeclineProposalsForEntity` in lifecycle helpers | All lifecycle actions that remove/hide entities auto-decline affected proposals. |

**Summary:** All four lifecycle actions (Archive, Forget, Supersede, Tombstone) are in store and exposed via the World Model façade. Proposal gating: use `ActionDescriptorForLifecycleTombstone` / `ActionDescriptorForLifecycleSupersede` with `ValidateAction`; when outcome is RequiresConfirmation, create a proposal and do not execute until owner approves. **World-model routing:** Orchestrator adapter now uses `WorldModel.FactsBlockForScope` for directive-scoped facts (see plan gap §1). **Configurable retention:** `governor.execution_outcome_retention_days` (and `NAVI_GOVERNOR_EXECUTION_OUTCOME_RETENTION_DAYS`) enable periodic deletion of old execution_outcomes; janitor runs with the proposal janitor ticker. `store.DeleteExecutionOutcomesOlderThan`, `store.GetOwnerID` added.

---

## 2. Failure visibility

| Area | Status | Location | Next steps |
|------|--------|----------|------------|
| **Degradation in loop** | Implemented | `internal/navi/loop.go` | Tool execution errors are classified; `DegradationVisibilityFor` drives message form: Advisory (“Note: …”), Blocking (“Error: …”), Deferred recovery (recovery proposal created, ID in message). |
| **Execution outcomes** | Implemented | `internal/command/executor.go`, `store.SaveExecutionOutcome` | Every command attempt records outcome, failure class, retryable, compensation/recovery status. |
| **Compose / compensation** | Implemented | `internal/command/compose.go`, `store.RunCompensation` | Compose runner and compensation status updates in place. |
| **Gateway/Experience** | Implemented | `internal/gateway/server.go` | `replyErrorStructured` returns optional `degradation_type` and `recovery_proposal_id` in error JSON; used for 403 governance outcomes (blocking, requires_confirmation). |

**Summary:** Failure visibility is applied in the NAVI loop (tool result text and recovery proposals). Gateway uses structured error body for governance rejections; handlers that have a recovery proposal ID can pass it via `replyErrorStructured`.

---

## 3. Plugin / MCP ecosystem

| Area | Status | Location | Next steps |
|------|--------|----------|------------|
| **Plugin registry** | Implemented | `internal/navi/plugin/` (Manifest, Registry, builtin.go), `cmd/navid/main.go` | Registry has Categories, Capabilities, Triggers, TrustTier; RegisterBuiltin adds Domain, Workflow, Integration, Agentic plugins; PluginRegistry wired into NAVI config and loop tool list. |
| **MCP transport** | Implemented | `internal/navi/skill/executor.go` (executeMCPTool), `skills/mcp-echo/SKILL.yaml` | HTTP JSON-RPC bridge for `mcp_tool`; mcp.echo skill validates end-to-end when bridge runs at configured URL. |

**Summary:** Plugin-capabilities and mcp-bridge plan todos are implemented. Discovery and governance can use plugin manifests; MCP skills execute when an HTTP bridge is available.

---

## References

- [Concept vs Implementation Gap Report](../concept-vs-implementation-gap-report.md) — section 6 (Failure, Provenance, Autonomy), 6.3 Data Lifecycle
- [Concept vs Implementation Gap Plan](../../.cursor/plans/concept-vs-implementation-gap-analysis.plan.md) — todos: plugin-capabilities, mcp-bridge, todo-1773416123309-3bl77cjoq (Tier-3 tracking)
