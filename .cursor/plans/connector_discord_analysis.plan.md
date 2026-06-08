---
name: Discord Connector Analysis
overview: Research plan to compare the Discord connector implementations in OpenClaw and PicoClaw, and extract design and security principles for NAVI's internal Discord connector.
todos: []
isProject: false
---

# Discord Connector – Analysis Plan

---

## 1. Objectives

- **Primary goal**: Design a high-confidence blueprint for an internal Discord connector that is efficient, reliable, and secure, using OpenClaw and PicoClaw as reference implementations.
- **Outcomes**:
  - Clear understanding of how Discord is integrated in both ecosystems.
  - List of patterns we will deliberately adopt or avoid.
  - Constraints and risks that must shape our implementation plan.

---

## 2. Reference Implementations

### 2.1 OpenClaw Discord Extension

- **Location**: `.example-code/openclaw/extensions/discord/`
- **Key artifacts to study**:
  - `index.ts` – plugin entrypoint and `register(api)` wiring.
  - `openclaw.plugin.json` – manifest, declared `channels`, config schema, security metadata.
  - `src/channel.ts` – `ChannelPlugin` definition (capabilities, messaging/outbound config, status, directory).
  - `src/runtime.ts` – runtime host (Discord SDK usage, gateway lifecycle).
  - `src/subagent-hooks.ts` – any message-level or subagent hooks.
- **Focus questions**:
  - How does the extension configure and start the Discord Gateway WebSocket?
  - How are inbound events normalized into OpenClaw’s internal message model?
  - How are outbound messages, edits, reactions, and threads implemented?
  - Where do they handle rate limits, retries, and backoff?
  - How do they express capabilities (DMs, channels, threads, media, reactions)?

### 2.2 PicoClaw Discord Channel

- **Location**: upstream PicoClaw repo, `pkg/channels/discord/` (Go).
- **Key artifacts to study**:
  - `discord.go` – `DiscordChannel` struct and `Start/Stop/Send/IsRunning` methods.
  - `init.go` – factory registration with `channels.RegisterFactory("discord", ...)`.
  - Any helper files (rate limiting, reconnect handling, message normalization).
- **Focus questions**:
  - How does the channel implement the `Channel` interface (especially `IsAllowed`, `ReasoningChannelID`)?
  - How is the Gateway WebSocket configured and reconnected on failure?
  - How do they map Discord messages (channels, threads, mentions) into internal types?
  - How do they handle media, attachments, and embeds?
  - How are rate limits and message length limits enforced?

---

## 3. Architecture & Pattern Breakdown

### 3.1 Implementation Shape (OpenClaw)

Research tasks:

1. **Connector surface**:
  - Document the `ChannelPlugin` fields relevant to Discord (capabilities, messaging, outbound, directory, status).
  - Capture how Discord-specific options (guilds, channels, threads) are modeled.
2. **Runtime & lifecycle**:
  - Diagram how `register(api)` wires into the plugin registry and how runtime startup occurs.
  - Identify how the Discord runtime is started, stopped, and reloaded.
3. **Event & message flow**:
  - Trace the path from a Discord `MESSAGE_CREATE`/`MESSAGE_UPDATE`/`INTERACTION_CREATE` event into OpenClaw’s internal structures.
  - Identify how threading, replies, and mentions are represented.
4. **Outbound flow**:
  - Document how text, embeds, and attachments are sent (Discord REST API usage).
  - Note any message chunking or coalescing logic.

### 3.2 Implementation Shape (PicoClaw)

Research tasks:

1. **Channel struct & interfaces**:
  - List all interfaces implemented by `DiscordChannel` (e.g. `TypingCapable`, `MediaSender`, `PlaceholderCapable`, `ReactionCapable`, `MessageLengthProvider`).
  - Capture how allowlists and group triggers are wired.
2. **Lifecycle & workers**:
  - Understand how `Start` spawns goroutines, subscribes to the Gateway, and registers handlers.
  - Note how `Stop` and reconnection are implemented.
3. **Integration with Manager**:
  - See how Discord messages enter the bus and how outbound workers call into `Send`.
  - Record how errors are classified (`ErrRateLimit`, `ErrTemporary`, `ErrSendFailed`).
4. **Message normalization**:
  - Map Discord concepts (guild, channel, thread, user, DM) to the channel’s internal `SenderInfo` and message types.

---

## 4. Strengths, Weaknesses, and Design Principles

During research, collect concrete examples to answer:

- **What OpenClaw does well for Discord**:
  - Extensibility via `ChannelPlugin` fields (directory, status, actions, threading).
  - High-level configuration knobs that are valuable for us (allowlists, group mention rules, default channels).
- **What PicoClaw does well for Discord**:
  - Clean Go implementation that composes with `BaseChannel` and the shared Manager.
  - Practical rate limiting, reconnection, and error handling patterns.
- **Weaknesses / pain points**:
  - Overly complex configuration, tight coupling to the plugin runtime, or brittle error handling.
  - Patterns that are hard to test or that assume monolithic in-process operation.

From this, derive a **short list of design principles** to guide NAVI’s Discord connector (e.g. “single source of truth for channel mapping”, “graceful reconnection with jittered backoff”, “explicit capability surface for threads and edits”).

---

## 5. Platform API Assessment (Discord)

Research questions to answer using the latest Discord API docs:

1. **Auth & scopes**:
  - Which OAuth scopes and bot permissions are required for MVP (DMs, guild text channels, threads)?
  - What is the recommended way to store and rotate bot tokens?
2. **Gateway & events**:
  - Required intents for our use-cases (message content, guild members, reactions).
  - Limits and best practices for sharding (record as v2 concern).
3. **REST & rate limits**:
  - Global vs per-route rate limits.
  - Best practices for retries and backoff when 429s occur.
4. **Message model**:
  - Maximum content length, attachment rules, threading rules.
  - How replies and message references are encoded.
5. **Security & compliance**:
  - Discord’s policies on storing message content and user identifiers.

---

## 6. NAVI-Facing Requirements

Translate research findings into concrete requirements for NAVI’s connector:

- **Functional**:
  - Support DMs and guild text channels as MVP.
  - Map Discord threads to NAVI’s session/thread concepts.
  - Allow per-guild or per-channel allowlists for who can invoke NAVI.
- **Non-functional**:
  - High availability under Gateway reconnects.
  - Respect rate limits with backoff and observability.
  - Observable health (per-connector health checks, metrics).

---

## 7. Deliverables

This analysis phase is complete when we have:

1. A concise written comparison of OpenClaw vs PicoClaw Discord implementations.
2. A documented list of **patterns to adopt**, **patterns to avoid**, and **questions left open**.
3. A set of concrete, NAVI-specific requirements that directly feed into `connector_discord_development.plan.md`.
4. Any discovered blockers or prerequisites (e.g. shared Discord config helper, common rate limiter) clearly captured.

