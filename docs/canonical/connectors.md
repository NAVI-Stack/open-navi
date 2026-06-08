**Status:** Canonical (Immutable — changes require human approval)  
**Last Reviewed:** 2026-03-10  

# Connectors — Canonical Specification

Connectors are how NAVI interfaces with the outside world. This document is the **single canonical specification** for connector behavior, boundaries, and contracts in NAVI.

- It **extends** the Connectors section in `docs/canonical/conceptual-design-overview.md`.
- It **normatively constrains** the plugin-owned connector implementations under `plugins/<name>/connectors/`, the shared public connector interfaces in `connectors/`, the connector framework in `internal/connectors/`, and connector-related gateway routes in `internal/gateway/`.
- Architecture overviews, feature catalogues, plans, and reviews **must not contradict** this document. If they do, this document wins.

> **Note**: Repo-owned connector implementations live under their owning plugin package, for example `plugins/telegram/connectors/telegram` and `plugins/slack/connectors/slack`. The root `connectors/` package contains shared interfaces and types only; `internal/connectors/` contains framework/runtime support.

---

## 1. Definition and Role

### 1.1 Canonical definition

A **connector** is an integration adapter that lets NAVI interact with external channels, systems, services, devices, and identity layers. Connectors:

- Translate NAVI-internal effects (commands, outbound messages, events) into **system-specific operations**.
- Handle **authentication, permissions, and protocol details** for the external system.
- Normalize external events into NAVI’s internal message and event formats.

Connectors are part of the **Capability Layer**, sitting:

- **Below** the Cognitive Layer (LLM, skills, agents).
- **Above** concrete transports (HTTP, WebSocket, SDKs, local processes).

They are **distinct from**:

- **Skills** — declarative reasoning/execution recipes that may *use* connectors as execution surfaces.
- **Plugins** — higher-level bundles that can combine skills, commands, and connectors into reusable modules.

### 1.2 Connector categories (recap)

Connector categories from the conceptual overview apply here and are considered exhaustive at the conceptual level:

- **Communication connectors** — messaging, voice, social channels.
- **Information connectors** — files/storage, databases, web/content, streams/feeds.
- **Service connectors** — productivity, business/operations, developer tools, AI/model services.
- **Device and environment connectors** — location, sensors, device controls.

Individual Go implementations today focus on **communication connectors** (chat/messaging), but the contracts below are written to generalize.

---

## 2. Architectural Boundaries and Responsibilities

### 2.1 What connectors are responsible for

Connectors **must**:

- Establish and maintain connections to external systems (webhooks, polling, WebSockets, SDK calls, or HTTP APIs).
- Translate between NAVI’s internal models and external payloads:
  - Map NAVI `OutboundMessage` / `OutboundMediaMessage` into platform-specific send APIs.
  - Map inbound platform events into NAVI session or directive messages.
- Handle **platform-specific auth** within the connector boundary:
  - Token management (where applicable), signature verification, endpoint configuration.
- Implement any supported **optional capabilities** via the capability interfaces defined in `connectors/capabilities.go`.
- Report **health and diagnostics** so the gateway and operators can see which connectors are up, degraded, or down.

### 2.2 What connectors are not responsible for

Connectors **must not**:

- Implement LLM decision-making, task decomposition, or high-level planning.
- Own long-term application state beyond what is required to maintain connectivity or credentials (long-term state belongs in `internal/store/` or purpose-built services).
- Embed arbitrary business logic or skills — they are **thin adapters**, not mini-apps.
- Enforce cost, risk, or autonomy limits (these are enforced by the **Governor** and policy systems).

### 2.3 Relationships to other components

- **Gateway (`internal/gateway/`)**
  - Exposes HTTP routes for connector registration, health, diagnostics, and (for some connectors) webhooks.
  - Uses the connector registry to track live connectors and their status.
- **Connector registry (`internal/connectors/`)**
  - Owns factories and live instances for Go-native connectors and HTTP-registered bridge connectors.
  - Provides a small API surface used by the gateway and daemon startup.
- **Skills and plugins**
  - Skills and plugins may **depend on** connectors to talk to external systems.
  - This dependency must be explicit in their specs; skills/plugins do not reach into connector internals.

---

## 3. Go-Level Connector Interface and Capabilities

The Go interfaces in `connectors/` are the canonical code-level contracts for chat and messaging connectors. They are summarized here; the Go definitions remain the ultimate source for method signatures.

### 3.1 Base connector interface

All concrete connectors **must** implement `connectors.Connector` from `connectors/connector.go`:

- **`Name() string`**  
  Returns the connector identifier. **Invariant:** unique per running process.

- **`Start(ctx context.Context) error`**  
  Starts the connector (sets up webhooks, opens WebSockets, begins polling, etc.).
  - Must be **idempotent** — repeated calls either succeed or return a stable error.
  - Must set the running state so that `IsRunning()` reflects reality.

- **`Stop(ctx context.Context) error`**  
  Stops the connector and releases external resources.
  - Must be safe to call multiple times.
  - Should perform a **best-effort drain** of in-flight work where supported by the platform.

- **`Send(ctx context.Context, msg OutboundMessage) error`**  
  Sends a text message to a user or channel via the external platform.
  - Must classify errors so the Manager can apply appropriate retry behavior (e.g. rate limit vs. permanent failure).
  - Must respect any applicable message length constraints (directly or via `MessageLengthProvider`).

- **`IsRunning() bool`**  
  Indicates whether the connector is currently accepting sends.

The message types used by `Send` are defined in `connectors/connector.go`:

- `OutboundMessage` — channel, chat ID, content, reply/thread IDs, parse mode.
- `OutboundMediaMessage` — channel, chat ID, and one or more `MediaPart` items.

### 3.2 BaseConnector

`connectors.BaseConnector` in `connectors/base.go` provides shared behavior that concrete connectors **should embed**:

- Stores a connector `name`.
- Tracks `running` state (`IsRunning()`, `SetRunning(bool)`).
- Implements `IsAllowed(senderID string)` for allow-list-based sender filtering.

Embedding `BaseConnector` is the recommended pattern to keep implementations consistent and avoid duplicated state management.

### 3.3 Optional capability interfaces

Optional capabilities live in `connectors/capabilities.go` and are discovered by type assertion. A connector implements only the capabilities it actually supports:

- **`WebhookHandler`**
  - `WebhookPath() string` — relative path segment for inbound webhooks (e.g. `"telegram"`).
  - `HandleWebhook(w http.ResponseWriter, r *http.Request)` — processes inbound HTTP webhook requests.
  - The gateway routes `POST /webhooks/{name}` to connectors that implement this capability.

- **`MediaSender`**
  - `SendMedia(ctx context.Context, msg OutboundMediaMessage) error` — sends multi-part media messages.

- **`HealthChecker`**
  - `HealthCheck(ctx context.Context) error` — used by diagnostics endpoints to determine connector health.

- **`TypingCapable`**
  - `StartTyping(ctx context.Context, chatID string) (stop func(), err error)` — starts a typing/thinking indicator.
  - The returned `stop` function **must be idempotent and safe** to call multiple times.

- **`MessageEditor`**
  - `EditMessage(ctx context.Context, chatID, messageID, content string) error` — edits an existing message in place.

- **`ReactionCapable`**
  - `ReactToMessage(ctx context.Context, chatID, messageID string) (undo func(), err error)` — adds a reaction (e.g. 👀).
  - The returned `undo` function **must be idempotent and safe** to call multiple times.

- **`PlaceholderCapable`**
  - `SendPlaceholder(ctx context.Context, chatID string) (messageID string, err error)` — sends a placeholder message to be edited later via `MessageEditor`.
  - Connectors implementing this capability **must also implement** `MessageEditor`.

- **`MessageLengthProvider`**
  - `MaxMessageLength() int` — maximum supported message length; `0` means “no limit”.
  - Used by the Manager to split long outbound messages automatically.

**Guidance:**  
New capabilities should be added as separate interfaces in `connectors/capabilities.go` rather than by extending `Connector` directly.

---

## 4. Connector Registry and Manager

### 4.1 Registry (`internal/connectors/registry.go`)

The registry is the canonical source of truth for known connector factories and live instances:

- **`ConnectorFactory`**
  - Type: `func(cfg *config.Config, b bus.Bus) (connectors.Connector, error)`.
  - Used to construct new connector instances from config and the event bus.

- **`Registry`**
  - Holds:
    - `factories` — registered `ConnectorFactory` values keyed by name.
    - `instances` — live `connectors.Connector` instances keyed by name.
    - `startedAt` — timestamps when connectors were started.
    - `httpRegistered` — names that registered via HTTP (bridge connectors).
  - Core methods:
    - `RegisterFactory(name string, f ConnectorFactory)` — register a factory at startup.
    - `Create(name string, cfg *config.Config, b bus.Bus) error` — create and store an instance (does **not** start it).
    - `RegisterInstance(name string, conn connectors.Connector)` — register a concrete instance (used by bridge connectors).
    - `Register(name string)` / `Deregister(name string)` — track HTTP-registered connectors.
    - `Get(name string) connectors.Connector` — look up an instance.
    - `GetInfo(name string) (ConnectorInfo, bool)` / `List() []ConnectorInfo` — used for health and diagnostics APIs.
    - `FactoryNames() []string` — discover registered connector types.

`internal/gateway/connectors.go` re-exports the registry types and constructor for use by the gateway without leaking internal package details.

### 4.2 Manager responsibilities (conceptual)

The connector manager (lifecycle component using the `Registry`) is responsible for:

- Starting and stopping all configured connectors during daemon startup and shutdown.
- Updating `startedAt` timestamps and health status.
- Surfacing connector health via API endpoints:
  - `GET /api/health/connectors`
  - `GET /api/diagnostics/connectors`

---

## 5. Gateway Contracts and Remote Connectors

### 5.1 Gateway-managed connectors

Go-native connectors configured in `config/runtime.yaml`:

- Have factories registered in `cmd/navid/main.go` (or similar startup code).
- Are created via `Registry.Create` and started by the manager based on config.
- Surface health via the health/diagnostics APIs.

### 5.2 Bridge connectors (remote)

Connectors may also exist **outside** the Go process and integrate via the HTTP gateway:

- **Registration**
  - `POST /api/connectors` — register a connector by name and configuration; returns a token.
  - The gateway uses `Register` / `RegisterInstance` on the registry as needed.

- **Inbound messages**
  - Remote connectors deliver messages into NAVI via:
    - `POST /api/navi/sessions/{id}/message`
    - `POST /api/directives/{id}/message`
  - These endpoints are authenticated using connector-specific tokens or API keys.

- **Outbound messages**
  - NAVI sends replies either via:
    - A Go-native connector implementation; or
    - A **bridge connector** that forwards `OutboundMessage` / `OutboundMediaMessage` payloads to a caller-supplied callback URL.

### 5.3 Webhook routing

For connectors that implement `WebhookHandler`:

- The connector’s `WebhookPath()` determines the path segment used by the gateway.
- The gateway routes:
  - `POST /webhooks/{name}` → `HandleWebhook(...)` for the connector whose name matches `{name}`.
- Connectors must:
  - Validate any required signatures or secrets.
  - Return appropriate HTTP status codes so callers can distinguish success, retryable failure, and permanent failure.

---

## 6. Lifecycle: Creation, Configuration, Execution, Errors

### 6.1 Creation and design

Adding a new Go-native connector follows this pattern:

1. Create `plugins/<name>/connectors/<name>/` and implement `connectors.Connector` (and any relevant capability interfaces).
2. Embed `BaseConnector` for name, running state, and allow-list handling.
3. Add a `ConnectorFactory` in the plugin connector package and register it at startup from the bootstrap boundary:
   - `RegisterFactory("name", func(cfg *config.Config, b bus.Bus) (connectors.Connector, error) { ... })`.

### 6.2 Configuration

Connector configuration lives under a single **`connectors`** key in `internal/config/config.go` and `config/runtime.yaml`:

- **Canonical YAML:** use the `connectors:` section. Each connector type is a sub-key (e.g. `connectors.telegram`, `connectors.slack`). New connector types are added as additional fields under `ConnectorsConfig`.
- **Legacy:** top-level `telegram:` and `slack:` are still supported; on load they are merged into `Config.Connectors` when the corresponding `connectors.*` value was not set. Prefer the `connectors:` section for new configs.
- Each connector type has a dedicated `XxxConfig` struct (e.g. `TelegramConfig`, `SlackConfig`). YAML may include:
  - Enabled flag(s).
  - Endpoint URLs (e.g. `gateway_url`).
  - Tokens and secrets.
  - Allow-lists and other connector-specific options.

Configuration is the **single source of truth** for which connectors are active in a given NAVI instance. All runtime code reads from `cfg.Connectors.Telegram` and `cfg.Connectors.Slack` (not top-level fields).

### 6.3 Startup

During daemon startup:

1. Config is loaded.
2. The connector registry is created via `gateway.NewConnectorRegistry()`.
3. Factories are registered for each known connector type.
4. For each enabled connector in config:
   - `Registry.Create(name, cfg, bus)` constructs the instance.
   - The manager starts the connector and records `startedAt`.

### 6.4 Runtime behavior

At runtime, connectors:

- Receive inbound events from external systems (webhooks, polling, WebSockets) and forward them to NAVI via internal APIs or the gateway.
- Send outbound messages initiated by NAVI tasks via `Send` and, if applicable, `SendMedia`.
- Maintain and periodically report health status via `HealthCheck`.

### 6.5 Error handling and shutdown

Connectors must:

- Distinguish **retryable** errors (e.g. temporary network issues, rate limits) from **non-retryable** errors (e.g. invalid tokens, permission errors) and expose this via error typing where possible.
- Implement graceful shutdown in `Stop`:
  - Stop accepting new work.
  - Attempt to flush or cancel in-flight operations.
  - Clear or update health state as appropriate.

The manager and gateway surface connector failures via:

- Logs.
- `/api/health/connectors` and `/api/diagnostics/connectors`.

---

## 7. Usage Patterns (Narrative Examples)

### 7.1 Telegram connector (Go-native)

1. A user sends a message to the Telegram bot.
2. Telegram delivers the message via webhook to `POST /webhooks/telegram`.
3. The Telegram connector’s `HandleWebhook`:
   - Validates the request.
   - Normalizes the payload into NAVI’s internal message format.
   - Calls NAVI’s session or directive message APIs.
4. NAVI’s agent runtime and CoderAgent process the message, eventually emitting an `OutboundMessage`.
5. The connector manager calls `Send(ctx, OutboundMessage)` on the Telegram connector, which sends a reply via the Telegram Bot API.

### 7.2 Slack connector (Go-native)

Slack follows a similar pattern with Slack-specific nuances (threads, approvals, etc.), but still:

- Uses `WebhookHandler` / `Send` / optional capabilities.
- Surfaces health via the same registry and gateway APIs.

### 7.3 Bridge connector (remote)

1. An external service registers itself via `POST /api/connectors`, obtaining a token.
2. It delivers inbound events by calling the session/directive message APIs with that token.
3. NAVI uses a `BridgeConnector` implementation to forward outbound messages to a configured callback URL.
4. The external service is responsible for translating those callbacks into platform-specific operations.

---

## 8. Constraints and Design Principles

Connectors must adhere to these principles:

- **Out-of-process isolation**  
  Connectors (especially remote/bridge ones) must be designed so that failures do not take down the core NAVI process.

- **Minimal surface area**  
  The base `Connector` interface plus optional capabilities define the full surface. Avoid adding ad-hoc methods on concrete types.

- **No business logic**  
  Business logic, workflows, and reasoning belong in skills, plugins, or agents — not in connectors.

- **Deterministic behavior where possible**  
  Retry and backoff policies should be predictable and, where platform constraints allow, configurable.

- **Security and privacy first**  
  - Use least-privilege credentials and scopes for each connector.
  - Treat user-identifiable information handled by connectors as sensitive.
  - Verify webhooks and signatures rigorously.

- **Testable contracts**  
  - Unit tests should cover connector implementations against the interface and capabilities.
  - Integration tests should exercise real platform interactions where feasible, or stable mocks where not.

---

## 9. Relationship to Other Documents

- **`docs/canonical/conceptual-design-overview.md`**
  - Provides the high-level conceptual view of connectors within the Capability Layer.
  - This document refines and concretizes that view for implementation.

- **`docs/architecture/README.md`**
  - Lists connector-related features and diagrams in the overall architecture.
  - Should treat this document as the source of truth for connector behavior and contracts.

- **`docs/FEATURES.md`**
  - Describes connector features and status by package; links here for semantics and invariants.

- **`docs/plans/connectors/*.plan.md` and `docs/reviews/*.md`**
  - Plans and reviews may **propose** changes to connector behavior.
  - Those changes become canonical only once they are reflected in this document (or its successors) under `docs/canonical/`.

