# ADR-003: Stateless Orchestrator at the Model Boundary

**Status:** Accepted  
**Date:** 2026-02-03  
**Deciders:** NAVI AI Core Team  
**See also:** NAVI ADR-003 (shared rationale)

---

## Context

The Orchestrator loop calls an LLM at every tick for pending directives. The question is: does the Orchestrator accumulate conversation state in process memory (stateful model boundary) or reconstruct it from durable storage on every call (stateless model boundary)?

**Stateful approach:** Keep the last N messages in memory. Pass the same message history object to successive LLM calls. Faster context construction; simpler prompt building code.

**Stateless approach:** On every Orchestrator tick, read the conversation history from SQLite. Reconstruct the full context from durable state. Serialize every LLM output back to SQLite before using it. The LLM is treated as a pure function: `f(state from DB) → response`.

---

## Decision

**Stateless at the model boundary.** Every Orchestrator LLM invocation reconstructs full context from SQLite. No conversation state is held in process memory across ticks.

---

## Rationale

**Crash tolerance:**
If the Orchestrator loop process crashes mid-directive (OOM, SIGKILL, deployment restart), a stateful implementation loses all in-flight conversation state. With stateless design, the next process start reads from SQLite and resumes exactly where it left off. The directive continues without the user noticing.

**Deterministic replay:**
Given the same SQLite state, the Orchestrator loop produces the same decisions. This is essential for:
- Debugging: reproduce any Orchestrator decision by seeding SQLite with the same state
- Testing: the `stub_adapter.go` implements the same interface; tests run against deterministic responses without any LLM calls
- Audit: every Orchestrator decision is traceable to a specific SQLite state snapshot

**Model/provider replacement:**
When upgrading from Claude 3 Sonnet to Claude 4 Opus (or switching from Anthropic to Ollama), a stateful implementation requires either migrating in-memory state or restarting all active conversations. Stateless design is provider-agnostic at the model boundary — the new provider simply receives the same SQLite-reconstructed context.

**Clear audit trail:**
Every LLM input (the reconstructed context) and every LLM output (the response, saved before use) is a traceable artifact. This is required for the provenance guarantee: every Orchestrator decision has a recorded input state and a recorded output.

**Against stateful:**
The primary argument for stateful is performance — reconstructing context from SQLite on every tick has a cost. However:
- SQLite WAL reads are fast (<1ms for the directive message history query)
- The Orchestrator tick interval is 2 seconds — there is no latency budget pressure
- The correctness guarantees of stateless design outweigh the marginal reconstruction cost

---

## Implementation

```go
// internal/orchestrator/adapter.go (simplified)
func (a *OrchestratorAdapter) HandleDiscuss(ctx context.Context, directiveID string) error {
    // Reconstruct context from SQLite every time — no in-memory state
    messages, err := a.store.ListDirectiveMessages(ctx, directiveID, last=20)
    if err != nil { return err }

    prompt := a.prompts.BuildDiscussPrompt(messages)
    response, err := a.llm.Chat(ctx, prompt)
    if err != nil { return err }

    // Save to SQLite BEFORE using the response
    err = a.store.AppendDirectiveMessage(ctx, directiveID, role="orchestrator", content=response.Text)
    if err != nil { return err }

    // Publish fact event
    return a.bus.Publish(ctx, FactDirectiveReplied{...})
}
```

The pattern: read from SQLite → call LLM → write to SQLite → publish event. No intermediate state.

---

## Consequences

- SQLite `directive_messages` table is mandatory for every active directive — conversation history is the source of truth
- Prompt construction (`internal/orchestrator/prompts.go`) must be deterministic and testable with fixed inputs
- Context window management is the primary tuning lever — currently loads last 20 messages (~20K tokens budget)
- The `stub_adapter.go` deterministic test double confirms this invariant: same seed state → same output, every time
- Provider switching requires zero migration — restart with new provider config, existing directive history is intact

---

## Status History

| Date | Change |
|---|---|
| 2026-02-03 | Accepted — stateless Orchestrator at model boundary, SQLite as conversation source of truth |
