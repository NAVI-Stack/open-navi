# Conscious Process — Step Mapping

**Status:** Active  
**Source:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md) (Conscious Process)  
**Implementation:** `internal/navi/loop.go`, `internal/orchestrator/adapter.go`

---

## Design steps

The Conscious Process is defined as:

**Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect**

Each step has a single responsibility. The implementation does not expose named step boundaries; the mapping below shows where each step is realized so that traceability and future step-level instrumentation (e.g., spans) can be added without redesign.

---

## Mapping to current implementation

| Step | Responsibility | Where it happens |
|------|----------------|------------------|
| **Perceive** | Input ingestion: new owner message(s), session identity, channel. | `AgentLoop.processTurn`: session messages loaded; the latest owner message is the perceived input. |
| **Interpret** | Intent and context parsing; what the user is asking or implying. | Fused with Decide: the single LLM call receives the conversation and produces intent implicitly via the response. No separate Interpret phase. |
| **Contextualize** | Pull from World Model (facts, memories, relevant state) to form the context for decision. | `LoopConfig.FactsBlock` (session-scoped) and the context block injected into the system prompt in `processTurn` (around “Build context and system prompt”). Session-summary memories can also surface unresolved carryover items from earlier sessions. |
| **Decide** | What to do: reply in natural language or issue tool/command calls. | Single LLM invocation in `processTurn`; response is either content or a list of tool calls (decided action). |
| **Validate/Govern** | Permissions, policy, configuration, priority alignment, risk. | `governor.ValidateAction` / `ValidateActionWithOwner` in `executeTool` before execution; `SaveProposal` when outcome is RequiresConfirmation. |
| **Execute** | Run the chosen action (tool, skill, or command). | `executeTool` → `CommandExecutor.Execute` (and file/skill/plugin execution); outcomes recorded via `SaveExecutionOutcome`. |
| **Reflect** | Emit a reflection payload for the Subconscious (summary, tier, conversation envelope, facts queued). | `emitReflect` at end of turn (success or LLM error); publishes `ReflectionPayload` to the bus. The current payload can carry a structured JSON envelope with `user_message`, `assistant_reply`, and a `facts` array for shallow fact promotion. Manual memory cues like `remember this` also stamp the payload as owner-scoped memory when possible. The orchestrator path now emits the same `FactReflectionQueued` event type after ACT decomposition, coder task execution, and directive completion. |

---

## Orchestrator (directive) loop

The orchestrator’s per-directive processing in `internal/orchestrator/adapter.go` follows the same logical sequence: load directive and messages (Perceive/Contextualize), call LLM (Interpret/Decide), validate and execute (Validate/Govern, Execute), and persist/publish (Reflect via events). For ACT directives, Reflect is now explicit at the event layer: decomposition, coder execution, and directive completion each queue structured `FactReflectionQueued` payloads for the same reflection worker used by session turns. Step boundaries are not named there either; this doc serves as the single mapping for both the NAVI agent loop and the orchestrator.

---

## Future observability

To add step-level spans or metrics, use the names above and anchor them at:

- **Perceive:** entry to `processTurn` (or first read of session messages).
- **Contextualize:** call to `FactsBlock` and construction of the context block.
- **Decide:** LLM request/response.
- **Validate/Govern:** call to `ValidateAction` / `ValidateActionWithOwner`.
- **Execute:** call to `executor.Execute` or the tool execution path.
- **Reflect:** call to `emitReflect`.

Interpret can be treated as part of the Decide span until a separate intent-parsing step is introduced.

---

## Intentional folding (audit trail)

Perceive, Interpret, and Contextualize are **intentionally not** separate code phases. They are folded into a single LLM turn so that the model receives full context and produces one coherent Decide output. Where each design step is satisfied:

| Step | Satisfied where |
|------|-----------------|
| **Perceive** | At entry to `processTurn`: `Session.GetSession(sessionID)` loads messages; the latest owner message (and session identity/channel if present) is the perceived input. No separate "ingestion" function. |
| **Interpret** | Inside the single LLM call: the model receives the conversation history and optional tools; its response (content or tool calls) is the implicit interpretation of intent. No separate intent API. |
| **Contextualize** | Before the LLM call: `LoopConfig.FactsBlock(ctx, sessionID)` (when set) returns a World Model–derived block; it is concatenated with the system prompt in the "Build context and system prompt" section of `processTurn`. The system prompt also includes persona, skills snapshot, current time, and (when available) workspace context. Owner-scoped session summaries can now also contribute unresolved carryover items, so open threads from prior sessions survive session switches. So "pull relevant entities from the World Model" is realized by FactsBlock + prompt construction, not a separate Contextualize function. |

This folding is a deliberate design choice: explicit Perceive/Interpret/Contextualize phases would require additional LLM or heuristic steps and could duplicate context. The current implementation keeps one LLM round-trip per turn while preserving traceability via this mapping and the Future observability anchors above.
