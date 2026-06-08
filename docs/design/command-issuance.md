# Command Issuance Breadth

**Status:** Accepted (roadmap)  
**Concept:** [CMD-02] in concept-vs-implementation gap plan.

## Rule

Execute step issues Commands. State commands (Query, Create, Update, Delete) and Effect commands (Invoke, Send, Acquire, Schedule) and Coordination (Delegate, Compose) are used systematically. Complex behavior via composition. Design expects the 10 primitives as the stable execution surface; “every execution is a command.”

## Current implementation

- **Schema:** All 10 `CommandType` values exist in `internal/schema/command.go`: Query, Create, Update, Delete, Invoke, Send, Acquire, Schedule, Delegate, Compose. Idempotency and failure semantics are defined per type.
- **Executor:** `internal/command/executor.go` records execution outcomes; loop and workers use it. Compose is used in the reflection worker.
- **In practice:** Skills and tool calls map primarily to **Invoke**. File tools (ReadFile, ListDir, WriteFile) and connector sends are not uniformly modeled as Create/Update/Delete/Send through the same Command descriptor and Executor. Most execution paths are “Invoke (skill/tool).”

## Implemented

- **Send:** Connector worker records each delivery as a Send command. `ManagerConfig.SaveExecutionOutcome` is wired in main; when set, the worker builds an `ExecutionOutcome` with `CommandType: Send` and persists it (success or failure with reason). So connector sends are systematically issued as Send and recorded.
- **Create (interaction events):** Loop records World Model interaction events (e.g. turn completed, reply length) as Create via `recordInteractionEventAsCreate` when `SaveExecutionOutcome` is set.
- **Create (reflection):** Reflection worker records memory and fact creation as Create when `SetSaveExecutionOutcome` is wired; main wires it with `store.SaveExecutionOutcome`.
- **Delete (tombstone):** When a proposal is resolved Approved with `proposed_action == "tombstone"`, each `TombstoneEntity` call in the host `ResolveProposal` callback is executed through the Executor as a Delete command and the outcome is recorded.
- **Delegate (task assignment):** When the orchestrator assigns tasks in ACT mode (`handleImplement`), each task assignment is recorded as a Delegate execution outcome via `SetSaveExecutionOutcome` on the adapter; main wires it with `store.SaveExecutionOutcome`.
- **Update (task status):** When workers update task status via `ExecutionRecorder.UpdateTask`, the Cognitive-layer `StoreExecutionRecorder` records an Update execution outcome after persisting the task; all task runners (coder, critic, scout, strategist) use this path when `ExecutionRecorder` is set.

## Remaining roadmap

1. ~~**Create (loop/workers):**~~ Loop interaction events and reflection memory/fact writes now issue Create and record.
2. ~~**Update (task status):**~~ Implemented via ExecutionRecorder.UpdateTask; StoreExecutionRecorder records Update after each task status write. Other entity updates (e.g. artifacts) can follow the same pattern when those paths are added.
3. ~~**Delete (tombstone):**~~ Tombstone in ResolveProposal now issues Delete per entity and records.
4. **Acquire/Schedule:** To be used when resource-lock or deferred-work flows exist; Delegate and Update are implemented.
5. **Loop and connectors:** Continue expanding so every execution path that creates, updates, deletes, or sends goes through the same Command/Executor surface.

## Relevant files

- `internal/schema/command.go` (CommandType, idempotency, failure classes)
- `internal/command/executor.go`
- `internal/navi/loop.go`
- `internal/connectors/` (bridge, send path)
- `internal/gateway/server.go` (connector send)
