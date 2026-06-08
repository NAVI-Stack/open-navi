# World Model Write Boundary

**Status:** Implemented  
**Concept:** [ARCH-02] in concept-vs-implementation gap plan.

## Rule

Only the **Cognitive Layer** may write to the World Model. Experience and Capability layers do not modify it directly; changes flow through Cognitive processes (orchestrator, NAVI agent loop, reflection).

## Current implementation

`internal/store` is the persistence layer. All directive/message/mode and task/outcome writes from gateway, backlog, and workers go **only** through Cognitive facades:

| Caller | Writes | Classification |
|--------|--------|-----------------|
| `internal/worldmodel` | SaveContact, SaveWorldModelEvent, SaveMemory, SaveArtifact, SaveFact, SaveEntityProvenance, SaveRelationship, DeleteRelationship | **Cognitive** — World Model facade; used only by orchestrator, NAVI, reflection. |
| `internal/orchestrator` | SaveDirective (via adapter) | **Cognitive** — directive lifecycle. |
| `internal/navi` | Uses worldmodel + SaveProposal, ResolveProposal, SaveExecutionOutcome (injected from main) | **Cognitive** — agent loop and proposal/outcome recording. |
| `internal/navi/reflection` | Uses worldmodel for facts/provenance | **Cognitive** — consolidation/deep reflection. |
| `cmd/navid/main.go` | Wires DirectiveWriter and ExecutionRecorder (store-backed); worker failure callbacks use ExecutionRecorder.UpdateTask only | **Cognitive** — no direct store for directive/task/outcome. |
| `internal/gateway/server.go` | No direct store. Create directive, append message, update mode, rollback (DeleteMessage) via DirectiveWriter only; 503 if DirectiveWriter not set. | **Cognitive facade** |
| `internal/backlog/poller.go` | No direct store. SaveDirective and AppendMessage via DirectiveWriter only; no-op if writer nil. | **Cognitive facade** |
| `internal/identity/identity.go` | UpdateAgentIdentity (revoke) | **Exception** — identity/system, not World Model content. |
| `internal/coder/runner.go`, … (workers) | No direct store. UpdateTask, SaveExecutionOutcome, AppendMessage (critic/scout/strategist) only via ExecutionRecorder and DirectiveWriter; Execute requires non-nil facades. | **Cognitive facade** |

## Compliance summary

- **Gateway:** Directive/message/mode writes and rollback go only through DirectiveWriter. No store fallback; 503 if DirectiveWriter not configured.
- **Backlog:** CreateDirectiveFromBacklog writes only through DirectiveWriter; no store fallback.
- **Workers:** All task and execution-outcome writes go through ExecutionRecorder; critic/scout/strategist append via DirectiveWriter. No direct store access; Execute requires non-nil facades.
- **Identity:** Unchanged; system boundary, not World Model content.

## Allow-list and rationale (historical)

Explicit exceptions to “only Cognitive writes”:

1. **Gateway (directive/message/mode)**  
   **Rationale:** Owner-initiated API: create directive, append message, change mode. Conceptually the owner is acting; the gateway is the transport. Moving these behind an “orchestrator or NAVI API” would require the gateway to publish commands and have Cognitive perform the writes—possible future refactor for full parity.

2. **Backlog poller (SaveDirective, AppendMessage)**  
   **Rationale:** Autonomous creation of a directive from backlog when no active directive exists. Behavior is Cognitive-like (creates work for the orchestrator); implementation is a dedicated component. Could be refactored to “publish CreateDirective command, orchestrator writes” for full parity.

3. **Identity (UpdateAgentIdentity)**  
   **Rationale:** Agent identity/revocation is system/security state, not World Model content (directives, tasks, facts, memories). Out of scope for World Model write boundary; acceptable as a separate system write path.

4. **Workers (UpdateTask, SaveExecutionOutcome)**  
   **Rationale:** Workers execute tasks assigned by the orchestrator; they record task status and execution outcomes as part of the Cognitive execution path. They act on behalf of the orchestrator. Full parity would route these through an orchestrator or shared “execution recorder” API that performs the store writes.

## Relevant files

- **Gateway:** Add “directive/message/mode” commands (or equivalent) consumed by orchestrator or NAVI; gateway only publishes; Cognitive performs store writes.
- **Backlog:** Backlog poller publishes “CreateDirectiveFromBacklog”; orchestrator (or dedicated Cognitive handler) performs SaveDirective + AppendMessage.
- **Workers:** Inject a single “ExecutionRecorder” (or use orchestrator) that workers call to UpdateTask and SaveExecutionOutcome; no direct store access from worker packages.
- **Identity:** No change required; keep as system boundary.

## Relevant files

- `internal/cognitive/writer.go` — DirectiveWriter and ExecutionRecorder interfaces and store-backed implementations
- `internal/store/*.go`
- `internal/gateway/server.go`
- `internal/worldmodel/worldmodel.go`
- `cmd/navid/main.go`
- `internal/coder/runner.go`, `internal/critic/runner.go`, `internal/scout/runner.go`, `internal/strategist/runner.go`
- `internal/backlog/poller.go`
- `internal/identity/identity.go`
