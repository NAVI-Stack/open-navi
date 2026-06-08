---
name: Connector Extension Architecture Review
overview: A comparative technical review of OpenClaw, PicoClaw, and NAVI's connector/extension architectures, culminating in a recommended refined architecture that combines the strongest patterns from each system.
todos:
  - id: define-connector-interface
    content: Define Connector interface, ConnectorFactory, and optional capability interfaces in connectors/connector.go and connectors/capabilities.go
    status: completed
  - id: upgrade-registry
    content: Upgrade ConnectorRegistry to manage factories, live instances, and health checks. Wire it into main.go
    status: completed
  - id: refactor-connectors
    content: Refactor telegram and slack connectors to implement the new Connector interface with init()-based factory registration
    status: completed
  - id: add-hook-system
    content: Implement typed hook system in internal/navi/hooks/ with priority-ordered runner
    status: completed
  - id: add-plugin-api
    content: Create PluginAPI in internal/navi/plugin/ for unified code-based extension registration (tools, hooks, services, HTTP routes)
    status: completed
  - id: registry-driven-startup
    content: Replace switch-case connector startup in main.go with registry-driven initialization from config
    status: completed
isProject: false
---

# Connector and Extension Architecture Review

---

## Feature / Architecture Matrix


| Capability                   | OpenClaw (TypeScript)                                                           | PicoClaw (Go)                                                          | NAVI (Go)                                                                         |
| ---------------------------- | ------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| **Connector interface**      | `ChannelPlugin` type with ~25 adapter slots                                     | `Channel` interface (Name/Start/Stop/Send) + optional interfaces       | No shared interface; ad-hoc per connector                                         |
| **Registration**             | `api.registerChannel()` during plugin `register()`                              | `init()` + `RegisterFactory(name, fn)`                                 | `switch` statement in `main.go`, HTTP self-registration                           |
| **Discovery**                | Manifest scan: config paths, workspace, global, bundled                         | Blank imports trigger `init()` factories                               | Hardcoded in `main.go`; no discovery                                              |
| **Extension model**          | Unified plugin system (`OpenClawPluginApi`) with 10+ registration methods       | Skills = Markdown docs; Tools = code-registered structs                | OSS-27 YAML specs with interfaces; 3-tier file loading                            |
| **Tool registration**        | `api.registerTool()` per plugin                                                 | Explicit code in `ToolRegistry`                                        | `SkillRegistry.Tools()` derived from OSS-27 interfaces                            |
| **Hook/event system**        | Typed hooks (`before_model_resolve`, `message_received`, etc.) + internal hooks | Message bus (channels: inbound/outbound/media)                         | NATS JetStream streams + HTTP callbacks                                           |
| **Connector-to-core comm**   | In-process (same runtime)                                                       | In-process message bus                                                 | HTTP + WebSocket (out-of-process)                                                 |
| **Config**                   | YAML/JSON + env + per-plugin entries + allow/deny lists                         | JSON + env tags (`caarlos0/env`)                                       | YAML + env + SQLite persisted settings                                            |
| **Security/isolation**       | Allowlists + deny lists + skill scanner (regex); no sandbox                     | Channel allow lists + exec deny patterns + workspace `os.Root` sandbox | PolicyEngine (risk_tier, confirmation); OSS-27 security fields (declarative only) |
| **Lifecycle**                | `register()` -> gateway start -> service `start()`/`stop()`                     | `init()` -> factory -> `Start(ctx)`/`Stop(ctx)` per channel            | Linear startup in main; no connector lifecycle interface                          |
| **Multi-capability plugins** | Yes: channels, tools, hooks, HTTP, services, providers, CLI in one plugin       | No: channels and tools are separate subsystems                         | Partially: OSS-27 interfaces on skills, but connectors are entirely separate      |
| **Hot reload**               | Plugin reload via `configPrefixes`                                              | No                                                                     | Skills can be re-loaded; connectors cannot                                        |


---

## Pros / Cons Breakdown

### OpenClaw

**Pros:**

- **Richest extension API**: Single `register(api)` entry point exposes tools, channels, hooks, HTTP handlers, services, providers, CLI commands -- everything a plugin could need
- **Manifest-driven discovery**: `openclaw.plugin.json` / `package.json` metadata enables tooling, UI, and automated installation
- **Typed hook system**: Named hooks with priority ordering allow precise interception of agent lifecycle events
- **Dock/Plugin separation**: Lightweight `ChannelDock` for metadata vs full `ChannelPlugin` for implementation -- good for UI and routing without loading heavy dependencies
- **Workspace packages**: `pnpm-workspace.yaml` makes extensions first-class packages with dependency management

**Cons:**

- **No process isolation**: All plugins run in-process; a malicious or buggy plugin can crash the entire system
- **Complex type surface**: `ChannelPlugin` has ~25 optional adapter slots -- high cognitive load for connector authors
- **Security is reactive**: Regex-based skill scanner catches known patterns but cannot prevent novel attacks
- **Monolithic registry**: Single `PluginRegistry` holds everything; no namespace separation between plugin categories

### PicoClaw

**Pros:**

- **Clean `Channel` interface**: 8 methods, easy to implement and test
- **Optional interfaces pattern**: `WebhookHandler`, `MediaSender`, `TypingCapable` etc. allow incremental capability adoption
- **Real sandbox**: `os.Root`-backed filesystem sandbox with path-escape prevention
- **Message bus decoupling**: Channels don't call the agent directly; the bus provides clean separation
- **Per-channel workers**: Rate limiting and queuing per channel prevents one slow channel from blocking others

**Cons:**

- `**init()` registration is implicit**: No manifest or declaration -- discovery depends on blank imports, making it hard to add channels without modifying gateway code
- **Skills are documentation-only**: No programmatic interface, no tool registration, no lifecycle -- skills are just Markdown injected into prompts
- **No plugin system**: Channels and tools are separate compile-time subsystems; no unified extension model
- **Hardcoded channel activation**: `initChannels()` has an explicit `if cfg.Channels.Telegram.Enabled` block per channel

### NAVI

**Pros:**

- **OSS-27 spec**: Structured skill metadata (interfaces, effects, security, governance) is the most forward-looking design of the three
- **PolicyEngine**: Pre-execution policy checks with risk tiers and confirmation requirements
- **Out-of-process connectors**: HTTP-based connector communication provides natural isolation -- connectors can crash without taking down the core
- **NATS JetStream**: Durable, replayable event streams with well-defined subjects -- superior to in-memory buses for reliability
- **Three-tier skill loading**: workspace > global > builtin with override semantics
- **Provenance tracking**: `DataEnvelope` with trust metadata on skill data

**Cons:**

- **No connector interface**: `telegram.Bot` and `slack.Bot` share no common type -- adding a new connector requires modifying `main.go`
- **Switch-case startup**: Connector initialization uses a `switch connType` block rather than a registry or factory
- **ConnectorRegistry is unused**: The registry exists in `internal/gateway/connectors.go` but is never wired in `main.go`
- **Only `internal` transport**: OSS-27 supports `mcp_tool`, `rest`, `internal` transports but only `internal` is implemented
- **No hook system**: No way for skills or connectors to intercept agent lifecycle events
- **No connector hot-reload**: Once started, connectors cannot be reconfigured without restarting the process

---

## Key Architectural Differences

### 1. Connector abstraction level

- **OpenClaw**: Connectors are adapter-slot plugins with 25+ optional capabilities. Maximum flexibility, high complexity.
- **PicoClaw**: Connectors implement a minimal interface + optional capability interfaces. Clean separation of concerns.
- **NAVI**: Connectors have no shared abstraction. Each is a standalone program communicating via HTTP.

### 2. Extension model

- **OpenClaw**: Unified plugin system -- everything (channels, tools, hooks, services) registers through one API.
- **PicoClaw**: Split model -- channels are Go packages with `init()`, tools are code-registered, skills are Markdown files.
- **NAVI**: Split model -- connectors are external programs, skills are YAML/MD files with OSS-27 spec, tool exposure is derived from specs.

### 3. Communication pattern

- **OpenClaw**: In-process function calls. Fast but tightly coupled.
- **PicoClaw**: In-process message bus with channels. Decoupled but still in-process.
- **NAVI**: Out-of-process HTTP + NATS JetStream. Maximum isolation, higher latency.

### 4. Security model

- **OpenClaw**: Allowlists + regex scanning. No runtime sandbox.
- **PicoClaw**: Filesystem sandbox (`os.Root`) + exec deny patterns + channel allow lists. Practical runtime protection.
- **NAVI**: Policy engine + OSS-27 security metadata. Richest declarative model but minimal runtime enforcement.

### 5. Discovery mechanism

- **OpenClaw**: Manifest files scanned from multiple paths with precedence.
- **PicoClaw**: Compile-time blank imports; no runtime discovery.
- **NAVI**: Hardcoded in `main.go`; skills discovered from filesystem.

---

## Internal Comparison: Where NAVI Stands

### Stronger

1. **OSS-27 skill specification** -- Neither OpenClaw nor PicoClaw has a structured, versioned skill spec with interfaces, effects, security metadata, and governance fields. This is a genuine innovation.
2. **NATS JetStream** -- Durable event streaming with replay and persistence beats in-memory buses for reliability, audit, and multi-process architectures.
3. **Out-of-process connectors** -- Natural fault isolation. A crashing Telegram bot does not take down the agent.
4. **PolicyEngine** -- Pre-execution policy checks with risk tiers is more principled than regex scanning or deny patterns.
5. **Provenance / trust metadata** -- `DataEnvelope` and governance fields position NAVI well for supply-chain security of skills.

### Weaker

1. **No connector interface** -- The lack of a `Connector` interface makes adding new connectors require surgical changes to `main.go`. Both OpenClaw and PicoClaw have clear contracts.
2. **No hook/event interception** -- OpenClaw's typed hooks let plugins intercept `before_model_resolve`, `message_received`, etc. NAVI has no equivalent.
3. **No unified registration** -- Adding a connector or skill requires different patterns and different parts of the codebase. OpenClaw's single `register(api)` is much cleaner.
4. **No channel capability interfaces** -- PicoClaw's optional interfaces (`MediaSender`, `TypingCapable`) allow connectors to declare capabilities incrementally. NAVI has no equivalent.

### Over-engineered

1. **OSS-27 security fields with no runtime enforcement** -- Fields like `sandbox.required`, `network_egress`, `data_access.pii` are declared but ignored at runtime. This creates a false sense of security. Either enforce them or defer their definition.
2. **NATS for single-process deployment** -- For the current architecture (single `navid` binary), embedded NATS adds operational complexity. The JetStream benefits (durability, replay) are valuable but the embedded server is heavy for the use case.

### Missing capabilities

1. **Connector lifecycle management** -- No `Start(ctx)/Stop(ctx)` contract, no health checks, no graceful restart.
2. **Connector factory/registry** -- `ConnectorRegistry` exists but is not wired. No `ConnectorFactory` pattern.
3. **Plugin hook system** -- No way to intercept or modify agent behavior at well-defined lifecycle points.
4. **Connector capability declaration** -- No way for a connector to say "I support media", "I support typing indicators", "I support message editing".
5. **Runtime skill transport** -- Only `internal` transport is implemented; `mcp_tool` and `rest` transports in OSS-27 are stubs.

---

## Recommended Connector / Extension Architecture

The recommendation combines:

- **PicoClaw's** clean `Channel` interface + optional capability interfaces + factory registry
- **OpenClaw's** unified registration API + typed hooks + manifest-driven discovery
- **NAVI's** OSS-27 spec + PolicyEngine + NATS JetStream + out-of-process communication model

### Module Layout

```
connectors/
  connector.go          # Connector interface + ConnectorFactory type
  capabilities.go       # Optional capability interfaces
  registry.go           # ConnectorRegistry (factory map + live instances)
  lifecycle.go          # Lifecycle manager (start/stop/health/restart)
  telegram/
    connector.go        # Implements Connector + relevant capabilities
    init.go             # RegisterFactory("telegram", ...)
  slack/
    connector.go
    init.go
  ...

internal/
  bus/                  # Keep NATS JetStream (already strong)
  navi/
    skill/              # Keep OSS-27 + PolicyEngine (already strong)
    hooks/
      hooks.go          # Typed hook system (new)
      runner.go         # Hook runner with priority
    plugin/
      api.go            # PluginAPI -- unified registration surface
      registry.go       # PluginRegistry (tools, hooks, services)
      loader.go         # Discovery from workspace/global/builtin paths
      manifest.go       # Plugin manifest format
```

### Connector Interface Model

Adopt PicoClaw's clean base interface with NAVI's out-of-process capability:

```go
type Connector interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Send(ctx context.Context, msg schema.OutboundMessage) error
    IsRunning() bool
    Capabilities() []string
}

type ConnectorFactory func(cfg *config.Config, bus bus.Bus) (Connector, error)
```

Optional capability interfaces (adopted from PicoClaw):

```go
type WebhookHandler interface {
    WebhookPath() string
    HandleWebhook(w http.ResponseWriter, r *http.Request)
}

type MediaSender interface {
    SendMedia(ctx context.Context, msg schema.OutboundMediaMessage) error
}

type HealthChecker interface {
    HealthCheck(ctx context.Context) error
}

type TypingCapable interface {
    SendTyping(ctx context.Context, chatID string) error
}
```

### Connector Lifecycle

```
1. RegisterFactory("telegram", factory)     -- init() time
2. registry.Create("telegram", cfg, bus)    -- config-driven instantiation
3. connector.Start(ctx)                     -- begin polling/webhooks
4. connector.Send(ctx, msg)                 -- outbound messages
5. connector.Stop(ctx)                      -- graceful shutdown
```

Health monitoring via `HealthChecker` interface, with the lifecycle manager periodically calling `HealthCheck()` and tracking status in `ConnectorRegistry`.

### Extension Interface Model

Adopt OpenClaw's unified registration pattern, scoped to NAVI's needs:

```go
type PluginAPI struct {
    ID      string
    Config  *config.Config
    Bus     bus.Bus
    Logger  *slog.Logger

    RegisterTool(tool ToolDefinition)
    RegisterHook(name HookName, handler HookHandler, priority int)
    RegisterConnector(factory ConnectorFactory)
    RegisterService(svc Service)
    RegisterHTTPRoute(path string, handler http.Handler)
}
```

Keep OSS-27 for file-based skills. Add `PluginAPI` for code-based extensions that need deeper integration (hooks, services, custom tools).

### Hook System

Adopt OpenClaw's typed hooks, adapted to NAVI's event model:

```go
type HookName string

const (
    HookBeforeToolCall   HookName = "before_tool_call"
    HookAfterToolCall    HookName = "after_tool_call"
    HookMessageReceived  HookName = "message_received"
    HookMessageSending   HookName = "message_sending"
    HookSessionStart     HookName = "session_start"
    HookSessionEnd       HookName = "session_end"
    HookBeforePromptBuild HookName = "before_prompt_build"
)
```

Hooks run in priority order and can modify payloads or short-circuit execution. This enables middleware-style extensibility without modifying core code.

### Integration Pattern with Agent Runtime

```
                     +-----------------+
                     |   PluginLoader  |
                     |  (discovery +   |
                     |   manifests)    |
                     +--------+--------+
                              |
                              v
                     +--------+--------+
                     | PluginRegistry  |
                     |  tools, hooks,  |
                     |  connectors,    |
                     |  services       |
                     +--------+--------+
                              |
            +-----------------+-----------------+
            |                 |                 |
            v                 v                 v
    +-------+------+  +------+-------+  +------+-------+
    | ConnectorMgr |  |  HookRunner  |  | SkillRegistry|
    | (lifecycle,  |  | (typed hooks |  | (OSS-27 +    |
    |  factories)  |  |  w/ priority)|  |  PolicyEngine)|
    +-------+------+  +------+-------+  +------+-------+
            |                 |                 |
            v                 v                 v
    +-------+------+  +------+-------+  +------+-------+
    |  Connectors  |  |  AgentLoop   |  |   Tools      |
    | (telegram,   |  | (LLM + tool  |  | (from skills |
    |  slack, ...) |  |  execution)  |  |  + plugins)  |
    +-------+------+  +--------------+  +--------------+
            |
            v
      NATS JetStream
      (events, audit)
```

### What Changes vs. Current NAVI


| Area                             | Change                                                                        | Rationale                                                                     |
| -------------------------------- | ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `connectors/`                    | Add `Connector` interface, `ConnectorFactory`, optional capability interfaces | Eliminates switch-case startup, enables new connectors without modifying core |
| `connectors/`                    | Add `init.go` per connector with `RegisterFactory()`                          | PicoClaw pattern; clean compile-time registration                             |
| `internal/gateway/connectors.go` | Upgrade `ConnectorRegistry` to manage factories + live instances + health     | Activate the existing but unused registry                                     |
| `internal/navi/hooks/`           | New: typed hook system with priority runner                                   | Enables middleware-style extensibility (from OpenClaw)                        |
| `internal/navi/plugin/`          | New: `PluginAPI` for code-based extensions                                    | Unified registration for tools, hooks, services (from OpenClaw)               |
| `cmd/navid/main.go`              | Replace switch-case with registry-driven connector startup                    | Cleaner initialization                                                        |
| `internal/navi/skill/`           | Keep as-is (OSS-27 + PolicyEngine)                                            | Already the strongest component                                               |
| `internal/bus/`                  | Keep as-is (NATS JetStream)                                                   | Already strong; provides durability PicoClaw and OpenClaw lack                |


### What NOT to Change

- **OSS-27 skill spec** -- already superior to both alternatives. Continue building on it.
- **NATS JetStream** -- keep the durable event bus. Do not regress to in-memory channels.
- **PolicyEngine** -- keep and extend. Add runtime enforcement for OSS-27 security fields incrementally.
- **Three-tier skill loading** -- workspace > global > builtin is a proven pattern used by both PicoClaw and OpenClaw.
- **Out-of-process connector model** -- preserve the HTTP/WS communication for production connectors; the new `Connector` interface is for in-process connectors that also support out-of-process deployment via the same bus.

