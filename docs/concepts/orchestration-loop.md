# NAVI AI — Orchestrator Loop

> **The orchestration heartbeat: how directives become plans, plans become tasks, and tasks get executed.**

**Status:** Active  
**Last Updated:** 2026-03-20  
**Package:** `internal/orchestrator/`  
**See also:** [Skills System](./skills.md) · [NAVI AI Architecture](../architecture/README.md)

---

## What Is the Orchestrator Loop?

The Orchestrator Loop is the central orchestration engine of NAVI AI. It runs on a **configurable tick** (e.g. 2 seconds in navid) — a continuous background loop that reads active directives from the database, calls the LLM to plan or respond, and dispatches tasks to specialized workers.

The mental model: you are the board of directors. The Orchestrator Loop is your Orchestrator. You give high-level intent; the Orchestrator breaks it down, delegates execution, monitors progress, and closes the loop.

The Orchestrator Loop is **stateless at the model boundary**: every tick, it reconstructs its full context from SQLite. There is no in-memory state that can get out of sync. Crash and restart — it picks up exactly where it left off.

**Source of truth for modes and statuses:** `internal/schema/` (directive.go, task.go, agent.go). JSON Schema in `schema/jsonschema/` is generated from Go.

---

## Directive Modes

Every directive has a **mode** from the schema: `CHAT`, `ADVISE`, `ASSIST`, `ACT`, or `WATCH`.

| Mode | Behavior |
|------|----------|
| **CHAT** | Conversational only. No actions. |
| **ADVISE** | Analysis and planning. No actions. |
| **ASSIST** | Routine tasks within pre-approved boundaries. |
| **ACT** | Full workflow execution with defined permissions. When there are no existing tasks, the Orchestrator may decompose the directive into tasks and publish `CmdTaskAssign`. Once all tasks reach terminal states, it finalizes the directive instead of decomposing again. |
| **WATCH** | Monitor conditions; trigger alerts or actions on threshold. |

The Orchestrator uses a **single conversational path** for all modes: it reconstructs the directive conversation, calls the LLM for a reply, and persists the response. For **ACT** mode only, when there are no existing tasks it invokes task decomposition (see below). When tasks already exist and all are terminal, it writes a completion summary and owner-scoped learning instead.

Directive **status** values (schema): `ACTIVE`, `PAUSED`, `COMPLETE`, `STALLED`, `CLOSED`. Task **status** values: `pending`, `running`, `blocked`, `completed`, `failed`, `cancelled`.

---

## The Tick Loop

The loop is implemented as `RunLoop(ctx, cfg LoopConfig)` in `internal/orchestrator/loop.go`. There is no `Loop` struct; the tick interval is set by `cfg.TickInterval` (e.g. 2s in `cmd/navid/main.go`).

Each tick:

1. Governor checks (duration, action budget, cost ceiling).
2. Adapter’s `Decide(ctx)` runs once per tick: it obtains active directives (e.g. via `GetActiveDirectives`) and processes one (e.g. the first).
3. For that directive, the adapter builds conversation context, calls the LLM, and persists the reply (`handleDirective`). In **ACT** mode, if there are no tasks yet, it calls the LLM with `DecomposeTasksTool`, persists tasks, publishes `CmdTaskAssign` per task, and queues a structured reflection event summarizing the decomposition. If tasks exist and all are terminal, it finalizes the directive, writes a summary fact for later owner context, and queues a structured completion reflection.

Worker agents are concurrent — tasks run in parallel via NATS. Only the Orchestrator’s tick is serialized.

---

## Conversational Flow (All Modes)

```
1. Read directive + last N DirectiveMessages from DB
2. Build system prompt (orchestrator persona, mode constraints, conversation history)
3. Call LLM: Chat(prompt, messages)
4. Persist DirectiveMessage (role: "navi" or as defined in schema)
5. Publish `FactDirectiveReplied`
6. Gateway pushes to WebSocket clients
```

CHAT, ADVISE, and ASSIST never create tasks. ACT may create tasks when there are none, and may finalize the directive when all tasks are terminal.

---

## Source & Intent Attribution

Before the Orchestrator decides what to do with an apparent instruction, it first determines **who it is really coming from**.

Conceptually, the flows include an internal **Source & Intent Attribution** sub-step:

1. **Classify content origin** — For each user-visible instruction, the adapter traces back to underlying history and `ContentTrust` (see [Content Trust Model](./content-trust.md)). Owner-authenticated input is tagged as `owner`; external text may be treated as `external_untrusted`.

2. **Interpret intent with trust awareness** — Instructions from `owner` or `internal_system` may be candidates for execution (subject to Governance). Instructions only in `external_untrusted` content are treated as **data, not commands**.

3. **Prompt construction** — Untrusted content in the LLM context is delimited and accompanied by system-prompt guidance so it is never treated as authoritative directives.

---

## ACT Mode: Task Decomposition

When the directive mode is **ACT** and there are **no** tasks yet for that directive:

```
1. Build system prompt with workspace context and decomposition constraints
2. Call LLM with DecomposeTasksTool
3. Receive task list; validate (count ≤ 8, surfaces non-empty, agent type and risk level valid)
4. INSERT tasks into DB (status: pending)
5. Publish `CmdTaskAssign` × N → `NAVI_AGENTS`
6. Publish `FactReflectionQueued` with a structured JSON envelope describing the task plan
```

**Schema values:** Use `assigned_to` values from schema `AgentType`: e.g. `"coder"`, `"critic"`, `"strategist"`, `"scout"`, `"heartbeat"`, `"connector"`, `"navi"`. Use `risk_level`: `"low"`, `"medium"`, `"high"`, `"critical"`. Task status in DB is lowercase: `pending`, `running`, `completed`, etc.

---

## DecomposeTasksTool

Defined in `internal/orchestrator/tools.go`. The LLM returns tasks with `title`, `surfaces`, `assigned_to` (schema agent type), `risk_level` (schema value), `rationale`. Go validates count ≤ 8, non-empty surfaces, and valid enum values before persisting.

---

## Orchestrator Invariants

| Invariant | Enforcement |
|-----------|-------------|
| Stateless at model boundary | Context fully reconstructed from DB each tick |
| No duplicate task creation | Decomposition runs only when no pending/running/blocked tasks |
| Closed feedback loop | Worker results become `task_outcome` facts, structured reflection events, and final directive learnings |
| Task count bounded | Go validates `len(tasks) ≤ 8` before INSERT |
| Valid agent type and risk | Go validates against `internal/schema` enums |
| All costs attributed | `RecordCost()` after every LLM call — governor can trip |
| Schema version on events | Consumers reject mismatches |

---

## Error Handling

| Scenario | What happens |
|----------|--------------|
| LLM call fails | Logged; directive unchanged; retried next tick |
| Invalid task spec from LLM | Go validation fails; error logged |
| Governor trips during LLM call | `ErrCostCeiling` (or similar); directive marked failed / not retried |
| NATS publish fails | Logged; tasks already in DB; workers requeued on restart |
| DB write fails | Fatal — loop halts; ops must investigate |

---

## Reflection and Learning

ACT directives now feed the same reflection pipeline as session chat turns:

- After task decomposition, the orchestrator emits `FactReflectionQueued` with a structured JSON payload that includes the decomposition summary and a directive-scoped `project_decision` fact candidate.
- After coder task execution, the worker writes a directive-scoped `task_outcome` fact, appends a directive message summarizing the result, and emits `FactReflectionQueued` with the structured outcome payload.
- After directive completion, the orchestrator writes the owner-scoped `post_directive_reflection` summary as before and also emits `FactReflectionQueued` so the shallow reflection worker can promote durable learnings through the same event path used by conversation turns.

This keeps multi-step ACT work visible to both the directive thread and the reflection worker without adding a separate learning subsystem for orchestrated tasks.

---

## File Reference

| File | Contents |
|------|----------|
| `internal/orchestrator/loop.go` | `RunLoop`, `RunOnce`, `tick` — tick loop and governor checks |
| `internal/orchestrator/adapter.go` | `Decide`, `handleDirective`, `handleImplement` — conversation and ACT decomposition |
| `internal/orchestrator/prompts.go` | System prompt construction |
| `internal/orchestrator/tools.go` | `DecomposeTasksTool` schema and handler |
| `internal/orchestrator/planner.go` | Planning and DAG helpers |
| `internal/orchestrator/stub_adapter.go` | Test stub — fixed task sets |
| `internal/schema/directive.go` | DirectiveMode, DirectiveStatus |
| `internal/schema/task.go` | TaskStatus, RiskLevel |
| `internal/schema/agent.go` | AgentType |

---

*NAVI AI Orchestrator Loop — maintained by the NAVI Documentation Research Assistant.*  
*Last updated: 2026-03-20*
