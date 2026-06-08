# NAVI Agent Task Primer

**Read this entire document before writing any code.**

You are an implementation agent working on the NAVI codebase. You have been assigned a Linear ticket. Your job is to deliver **complete, production-quality implementation** — not scaffolding, not stubs, not "good enough for now."

---

## What Went Wrong Before (Why This Document Exists)

In March 2026, a codebase audit found a systemic problem: agents were marking tickets Done while shipping incomplete work. Specific patterns:

1. **Struct-only delivery.** Agents defined Go types matching the spec, then marked the ticket Done. No storage layer, no migrations, no query API, no tests, no integration with the runtime. Types without persistence are inert code.

2. **Generic infrastructure instead of spec-specific implementation.** The LLM Knowledge Base spec defines six first-class entities (LLMProvider, LLMProfile, LLMRuntimeInstance, RouterDecision, LLMExecutionRecord, LLMEvaluation). Agents built a generic fact/memory system and marked the tickets Done. The generic system is useful, but it is not what the spec describes.

3. **Stored-but-not-enforced policy.** The Workspace system persists boundary_policy, allowed_actions, whitelist rules, and protected paths. But nothing in the governor, tool planner, or execution paths actually checks these values. Persistence without enforcement is worse than no policy — it creates false confidence.

4. **Phantom foreign keys.** `project_id` is threaded through sessions, artifacts, and workspaces, but no Project entity exists. These IDs point at nothing. Agents treated nullable string fields as "project support."

**You must not repeat these patterns.**

---

## Rules of Engagement

### 1. Read Before You Write

Before writing any code:

- Read the **full ticket description** including acceptance criteria.
- Read the **referenced spec sections** (file paths are in the ticket).
- Read the **existing code** in the packages you will touch. Use file listing and reading tools to understand the current state. Do not assume you know what exists.
- Check for **existing partial implementations** that may need to be extended rather than replaced.

### 2. No Stubs, No Placeholders, No TODOs

Every function you write must be a complete implementation.

**Prohibited patterns:**
```go
// ❌ DO NOT DO THIS
func (s *Store) SaveProject(p *Project) error {
    // TODO: implement
    return nil
}

// ❌ DO NOT DO THIS
func (s *Store) GetProject(id string) (*Project, error) {
    panic("not implemented")
}

// ❌ DO NOT DO THIS
func validateBoundary(ctx context.Context, path string) error {
    // placeholder — enforcement coming in next ticket
    return nil
}
```

**Required pattern:**
```go
// ✅ DO THIS — complete implementation
func (s *Store) SaveProject(ctx context.Context, p *Project) error {
    query := `INSERT INTO projects (project_id, workspace_id, title, slug, ...) 
              VALUES (?, ?, ?, ?, ...) 
              ON CONFLICT(project_id) DO UPDATE SET ...`
    _, err := s.db.ExecContext(ctx, query, p.ProjectID, p.WorkspaceID, p.Title, p.Slug, ...)
    return err
}
```

If a function is too complex to implement fully within scope, **that is a signal the ticket needs to be split**, not a reason to stub it. Raise this with the owner — do not ship stubs.

### 3. Every Ticket Must Compile and Pass Tests

Before you consider any ticket complete:

1. **`go build ./...`** — the entire project must compile with your changes.
2. **`go test ./...`** — all existing tests must pass. Do not break existing functionality.
3. **Your new code must have tests.** Not test stubs. Real tests that:
   - Verify the happy path works end-to-end
   - Verify at least one error/edge case
   - For store operations: round-trip persistence (write → read → verify equality)
   - For enforcement: both the "allowed" and "denied" paths
   - For enum validation: valid values accepted, invalid values rejected

### 4. Migrations Are Real SQL

When creating database tables:

- Write real `CREATE TABLE` statements with proper column types, NOT NULL constraints, defaults, and indexes.
- Include `ON CONFLICT` or `INSERT OR REPLACE` semantics where appropriate.
- Test that migrations run on a fresh database AND are idempotent (running twice produces the same result).
- Add the migration to the existing migration runner pattern in the codebase.

### 5. Wire Into the Runtime

Code that exists in isolation is dead code. Every implementation must be wired into the running system:

- **Store layers** must be instantiated in the startup path (look at how existing stores are created in `cmd/navid/main.go` or `internal/navi/navi.go`).
- **Services** must be registered in the NAVI struct or gateway as appropriate.
- **Enforcement checks** must be called from actual execution paths, not just defined as standalone functions.
- **API handlers** must be registered on the router with proper auth middleware.

If your code is not reachable from `main()`, it is not done.

### 6. Follow Existing Patterns

Before inventing new patterns, look at how the codebase already does things:

- **Store pattern:** Look at `internal/store/` — raw SQL, no ORM, `ExecContext`/`QueryRowContext`, manual scanning.
- **Service pattern:** Look at `internal/artifact/service.go` — service struct wrapping store + dependencies, methods taking context.
- **Gateway pattern:** Look at `internal/gateway/` — HTTP handlers with JWT auth middleware.
- **World Model pattern:** Look at `internal/worldmodel/worldmodel.go` — methods delegating to store with additional business logic.
- **Enum pattern:** Look at `internal/llmkb/enums.go` and `enum_methods.go` — typed string constants with `IsValid()` helpers.

Match the existing style. Do not introduce new frameworks, ORMs, code generators, or structural patterns without explicit approval.

### 7. Acceptance Criteria Are Literal

The ticket has acceptance criteria with checkboxes. Every checkbox must be satisfiable by your implementation — not "could be satisfied later" or "is satisfied in spirit." If the acceptance criterion says "Round-trip persistence tests pass for all six entities," there must be a test file where six entities are written to the database and read back with field-level equality assertions.

---

## Spec Reading Protocol

Tickets reference spec files at `docs/specs/*.md`. When a ticket says "Spec Reference: §5, §8", you must:

1. Open and read those sections of the spec file.
2. Cross-reference every field, enum value, and behavior described in those sections against your implementation.
3. If the spec says a field exists, your struct must have that field.
4. If the spec says a field is Governed, your code must not allow autonomous mutation of that field.
5. If the spec describes a behavior ("boundary crossing must prompt the user"), your code must implement that behavior — not just store the configuration that would enable it.

**The spec is the contract. The ticket is the scope. Both must be satisfied.**

---

## Verification Checklist (Run Before Declaring Done)

```
□ I read the full ticket description and all referenced spec sections.
□ I read the existing code in every package I modified.
□ Every function I wrote has a complete implementation (no stubs, no TODOs, no panics).
□ I wrote real tests (not test stubs) covering happy path and at least one error case.
□ `go build ./...` passes with my changes.
□ `go test ./...` passes with my changes (including pre-existing tests).
□ My code is wired into the runtime (reachable from main).
□ Database migrations are real SQL that runs on fresh DB and is idempotent.
□ Every acceptance criterion checkbox in the ticket is satisfied by my implementation.
□ I did not introduce any new frameworks, ORMs, or structural patterns.
□ I matched existing codebase patterns for store/service/handler/enum code.
□ I did not break any existing functionality.
```

If any checkbox is not checked, the ticket is not done.

---

## Codebase Reference

**Monorepo root:** `C:\Users\evirg\codespace\NAVI-Ecosystem\`
**NAVI backend:** `projects/navi/`
**Specs:** `projects/navi/docs/specs/`
**Ticket manifest:** `projects/navi/docs/plans/march-2026-audit-ticket-manifest.md`

**Key packages you will likely touch:**

| Package | Purpose |
|---------|---------|
| `internal/store/` | SQLite persistence — raw SQL, no ORM |
| `internal/worldmodel/` | World Model facade over stores |
| `internal/artifact/` | Artifact service layer |
| `internal/navi/` | Agent runtime, sessions, personas, skills |
| `internal/llmkb/` | LLM Knowledge Base entities (partially implemented) |
| `internal/gateway/` | HTTP/WS API handlers |
| `internal/governor/` | Hard constraint enforcement |
| `internal/connectors/` | Connector manager and workspace connector loader |
| `internal/config/` | Configuration loading |
| `cmd/navid/` | Daemon entrypoint — startup wiring |

**Build and test:**
```bash
cd projects/navi
go build ./...
go test ./... -count=1
```

**Database:** SQLite with WAL mode. DB file at `navi.db`. Migrations run on startup.

---

## What "Done" Means

A ticket is Done when:

1. The implementation is complete (not stubbed).
2. Tests pass (not skipped).
3. The code is wired into the runtime (not orphaned).
4. Acceptance criteria are met (not approximated).
5. The build is clean (no compilation errors, no test failures).

If you cannot complete a ticket fully, **say so explicitly** rather than shipping partial work marked as Done. Partial work marked as Done is worse than work marked as In Progress — it causes other agents to build on false assumptions.
