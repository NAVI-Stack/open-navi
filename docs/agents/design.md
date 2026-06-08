# Design - ADR and Phase Route Map

**Status:** Active  
**Purpose:** Identify relevant ADRs and phase specs for contract review before execution.

## Architecture Decision Records

| ADR | Topic |
|-----|--------|
| [ADR-001](../adr/ADR-001-go-orchestration-python-ai.md) | Go orchestration, Python AI |
| [ADR-002](../adr/ADR-002-nats-jetstream-bus.md) | NATS JetStream bus |
| [ADR-003](../adr/ADR-003-stateless-orchestrator.md) | Stateless orchestrator |
| [ADR-008](../adr/ADR-008-ncos-capability-surface-policy.md) | NCOS Phase 2 capability-surface policy |

## Phase and Architecture

- [Architecture README](../architecture/README.md) - System design, CoderAgent, file tools
- [Orchestration loop](../concepts/orchestration-loop.md) - Orchestrator tick, DecomposeTasksTool, IMPLEMENT mode
- [Blockers](../tasks/blockers.md) - Launch blockers
- [Outstanding work](../tasks/outstanding-work.md) - Non-blocker issues

## Frozen Design Contract

When a task spec in `docs/agents/tasks/NAVI-XXX-YYY.md` defines "What you must NOT change," agents must not modify those files or boundaries unless the spec is updated by a human.
