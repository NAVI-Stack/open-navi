# NAVI - Agent Directives

You are operating within the NAVI workspace - a self-improving, autonomous 24/7 personal AI agent.

## Orientation

1. **[Conceptual Design Overview](canonical/conceptual-design-overview.md)** - The canonical source of truth for NAVI's architecture: four-layer model, entity classes, governance, conscious/subconscious processes, capability layer, and failure model.
2. **[VISION.md](VISION.md)** - Core mission (One Human. One AI. Sovereign.) and roadmap.
3. **[Go To Root](../README.md)** - Architecture and Manifesto.
4. **Internal Structure:**
   - `internal/navi/`: Agent runtime - conversational/runtime execution loop, ICS control kernel, skills, Persona System, heartbeat, experience layer.
   - `internal/navi/experience/`: Experience Layer (Persona System) - trait-based behavior modulation, reply shaping, and presentation.
   - `internal/orchestrator/`: Cognitive Layer support - directive decomposition and orchestration workflows outside the ICS runtime control kernel.
   - `internal/store/`: World Model - SQLite-backed entity state, relationships, event log.
   - `internal/coder/`, `internal/critic/`, `internal/strategist/`, etc.: Capability Layer task-specific agents.
   - `internal/bus/`: NATS JetStream pub/sub streams.
   - `internal/governor/`: Governance - hard limits, validation pipeline, autonomy enforcement.

## Four-Layer Architecture

All code maps to one of four layers defined in the [Conceptual Design Overview](canonical/conceptual-design-overview.md):

| Layer | What it owns | What it does NOT own |
|-------|-------------|---------------------|
| **Experience** | Persona System (roles, traits, tone, presentation, reply shaping) | Reasoning, World Model writes, command execution |
| **Cognitive** | ICS control kernel, Conscious Process (Perceive -> Decide -> Execute -> Reflect), Subconscious (reflection tiers), Governance validation | Presentation, persona modulation, reply shaping |
| **World Model** | Entity state, relationships, provenance, structured knowledge | Decision-making, execution |
| **Capability** | Commands, Connectors, Plugins, skill execution | Reasoning, direct World Model writes |

When adding new code, identify which layer it belongs to and respect the boundary contracts.

## Design Principles

### Explicit Cognitive Control
> The platform provides transport. The cognitive kernel provides authoritative control.

Plumbing (NATS, SQLite, JWT) lives in Go. Open-ended generation, narration, and presentation live in prompts. But authoritative runtime control belongs in code: focus arbitration, mode routing, governance handoff, execution supervision, recovery, and plan progression live in the ICS path under `internal/navi/inference/` plus the runtime bridge. Do not push approval-boundary or control-state logic back into prompt-only heuristics.

### Sovereign by Design
NAVI belongs to the user. It is not an ephemeral tool but a persistent presence.

### Documentation-Driven Development
Every change follows: **spec -> plan -> implementation -> validation -> revision**. Documentation is the driver of design.

---

## Rules of Engagement

1. **Schema Truth:** `internal/schema/` (Go) is the master. Generate Python models via `make generate-python`. Never manually edit Python models.
2. **Determinism:** Agent loop logic must be deterministic.
3. **Execution Over Orchestration:** We prefer specialized task-oriented agents over generic "Manager" agents.
4. **Test-Driven:** Write failing tests first. Run `make test` often.
5. **No Mocks:** Use actual implementations for tests. Mock **only** LLM calls via fixtures.
6. **Graceful Halting:** If you encounter missing design decisions or broken inherited state, stop and notify the user - do not approximate.
7. **Layer Boundaries:** Respect the four-layer architecture. Experience does not write to the World Model. Capability does not make decisions. Cognitive does not own presentation.
8. **Build Artifacts in `bin/` Only:** Never run `go build ./cmd/...` without `-o bin/<name>`. Use `make build` / `make build-all`. For compile-only checks use `go build -o /dev/null ./cmd/navid/`. A binary appearing in the repo root is always a mistake — delete it. See "Build Artifact Policy" below.

---

## Directive Modes

| Mode | Behavior |
|------|----------|
| `CHAT` | Conversational only. No actions. |
| `ADVISE` | Analysis and planning. No actions. |
| `ASSIST` | Routine tasks within pre-approved boundaries. |
| `ACT` | Full workflow execution with defined permissions. |
| `WATCH` | Monitor conditions; trigger alerts or actions on threshold. |

---

## Technical Stack

- **Core**: Go 1.24+
- **Bus**: NATS JetStream (Subjects: `navi.cmd.>`, `navi.fact.>`)
- **State**: SQLite (WAL mode)
- **Identity**: JWT / Owner-centric / Ed25519 agent identity
- **LLM**: Anthropic / OpenAI / OpenRouter / Ollama
- **Governance**: Governor (hard limits) + Validation pipeline (Permissions -> Policy -> Configuration -> Priority -> Risk)
- **Autonomy**: Three presets (Conservative, Balanced, High) with per-domain overrides

## Development Workflow

From the NAVI repository root (directory containing `compose.yml`):

```bash
docker compose -f compose.yml up -d --build   # NaviD Docker mode
make build build-cli                          # Build local NaviD + NaviExe
npm install navi && navi daemon start         # User-facing local daemon mode
NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon start  # Developer local daemon shortcut
make test                                     # Run unit tests
make generate-python                          # Synchronize Python models
```

### Build Artifact Policy — STRICTLY ENFORCED

**All compiled Go binaries belong in `bin/`. The repo root must never contain binaries.**

- **Use Make targets exclusively for building:** `make build` -> `bin/navid` (`bin/navid.exe` on Windows), `make build-cli` -> `bin/navi.exe`, `make build-all` -> both. The `bin/` directory is gitignored.
- **Never run bare `go build ./cmd/navid/` or `go build ./cmd/navi/`** without an explicit `-o` flag. Go drops the output binary into the current directory — the repo root — creating untracked files that pollute `git status` and break CI hooks.
- **Compile-check without producing a file:** `go build -o /dev/null ./cmd/navid/`
- **Explicit output path if not using Make:** `go build -o bin/navid ./cmd/navid/`
- If a `navid` or `navi.exe` file appears in the repo root, delete it immediately — it is a misplaced build artifact from a bare `go build` invocation.

```bash
# WRONG — binary lands in repo root as untracked garbage
go build ./cmd/navid/

# RIGHT — compile check, no file produced
go build -o /dev/null ./cmd/navid/

# RIGHT — output goes to bin/ where it belongs
make build
```

### Go Cache Policy

Use the default Go cache locations reported by `go env GOCACHE` and
`go env GOMODCACHE`. Do not set `GOCACHE` or `GOMODCACHE` to repo-local
directories such as `.gocache`, `.gomodcache`, `.codex-gocache`,
`.codex-gomodcache`, `.codex-go-cache`, or `.codex-go-modcache`. These duplicate
caches are command-handling workarounds, not project artifacts.

## Agentic Workflow Imports

NAVI now carries a manual, NAVI-native, repo-local port of selected agentic workflow patterns.

- **Skills-first workflow surface:** Reusable repo-owned workflow behavior should live under the owning `plugins/<name>/skills/` package before it becomes a command shim, prompt fragment, or harness-specific integration. The root `skills/` directory is framework/support only.
- **Workspace execution truth:** Active workspaces should keep a short-lived `WORKING_CONTEXT.md` for current sprint truth, blockers, and open queues.
- **Surface inventory before recommendations:** Audit repo, tools, connectors, env-backed services, and skill coverage before proposing new agentic capabilities.
- **Verification before closure:** Match verification depth to the change and treat build/test/diff/safety review as part of done, not an afterthought.
- **Specialist routing:** Prefer `strategist`, `coder`, `critic`, and `scout` for their natural phases instead of collapsing planning, coding, review, and research into one generic pass.

## Documentation

The documentation corpus in `docs/` is the single authoritative knowledge base. The [Conceptual Design Overview](canonical/conceptual-design-overview.md) is the canonical source of truth for architecture. See [docs/GOVERNANCE.md](docs/GOVERNANCE.md) for structure, lifecycle, and approval rules.

- **Canonical (immutable):** `docs/canonical/` - principles, governance, protocols, foundational specs, conceptual design. Do not edit directly; propose changes in `docs-local/` or via PR for human approval.
- **Mutable:** architecture, adr, concepts, design, plans, runbooks, specs, tasks, prompts - update alongside code; follow [docs/_meta/agent-guidelines.md](docs/_meta/agent-guidelines.md).
- **Drift:** When you change architecture, APIs, schemas, or runbooks, update the corresponding doc in the same work (or produce a Docs Delta Plan per docs-drift-check skill).
- **Roles:** Generator (create docs), Indexer (keep INDEX and sub-indexes current), Guardian (docs-drift-check), Librarian (naming, cross-links, orphans), Archivist (move obsolete docs to _archive/_deprecated). See [agent-guidelines](docs/_meta/agent-guidelines.md#documentation-roles).
- **Imported guidance:** See [agents/everything-claude-code-integration.md](agents/everything-claude-code-integration.md) for the NAVI-native mapping of the imported ECC workflow patterns.

---

## Communications Style

- Use `NAVI` (uppercase) for product headings.
- Use `navi` (lowercase) for package names, CLI commands, and paths.
- Commits should follow: `<package>: <description>`.
