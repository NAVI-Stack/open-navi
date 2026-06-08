---
name: Connector Expansion Roadmap
overview: Comprehensive roadmap to build 17 internal Go connectors (from OpenClaw/PicoClaw reference), each following a Research-Develop-Harden-Document lifecycle with dedicated plan files per phase.
todos:
  - id: discord-analysis
    content: "Discord: Create connector_discord_analysis.plan.md -- compare OpenClaw and PicoClaw implementations, assess Discord Gateway WebSocket API, identify adoptable patterns"
    status: completed
  - id: discord-development
    content: "Discord: Create connector_discord_development.plan.md -- MVP (Gateway WS, REST send, basic auth, media), config schema, capabilities matrix, v2 features (sharding, slash commands, embeds)"
    status: completed
  - id: discord-validation
    content: "Discord: Create connector_discord_validation.plan.md -- unit/integration tests, webhook security, token management, threat model, user and technical docs"
    status: completed
  - id: whatsapp-analysis
    content: "WhatsApp: Create connector_whatsapp_analysis.plan.md -- compare OpenClaw Web bridge and PicoClaw native/bridge, assess Meta Cloud API vs Web API tradeoffs"
    status: completed
  - id: whatsapp-development
    content: "WhatsApp: Create connector_whatsapp_development.plan.md -- MVP (Meta Cloud API webhook, send/receive text+media), config schema, session management, v2 features (templates, interactive messages)"
    status: completed
  - id: whatsapp-validation
    content: "WhatsApp: Create connector_whatsapp_validation.plan.md -- webhook signature verification, token rotation, phone number privacy, end-to-end testing strategy"
    status: completed
  - id: signal-analysis
    content: "Signal: Create connector_signal_analysis.plan.md -- review OpenClaw signal-cli/HTTP bridge approach, assess signald vs signal-cli REST vs libsignal options"
    status: completed
  - id: signal-development
    content: "Signal: Create connector_signal_development.plan.md -- MVP (signal-cli REST bridge, DMs, group messages), config schema, bridge dependency management, v2 features (attachments, reactions)"
    status: completed
  - id: signal-validation
    content: "Signal: Create connector_signal_validation.plan.md -- E2EE considerations, bridge process security, phone number handling, integration test harness"
    status: completed
  - id: msteams-analysis
    content: "MS Teams: Create connector_msteams_analysis.plan.md -- review OpenClaw Bot Framework implementation (60+ files), assess Azure registration, Graph API, Adaptive Cards patterns"
    status: in_progress
  - id: msteams-development
    content: "MS Teams: Create connector_msteams_development.plan.md -- MVP (Bot Framework webhook, DMs, channel messages), OAuth flow, config schema, v2 features (cards, file consent, tabs)"
    status: pending
  - id: msteams-validation
    content: "MS Teams: Create connector_msteams_validation.plan.md -- OAuth token security, Azure app hardening, webhook verification, conversation store testing"
    status: pending
  - id: matrix-analysis
    content: "Matrix: Create connector_matrix_analysis.plan.md -- review OpenClaw matrix-js-sdk implementation (70+ files), assess Client-Server API sync protocol, room/DM models"
    status: pending
  - id: matrix-development
    content: "Matrix: Create connector_matrix_development.plan.md -- MVP (HTTP sync, DMs, room messages, basic media), config schema, v2 features (E2EE, threads, reactions, auto-join)"
    status: pending
  - id: matrix-validation
    content: "Matrix: Create connector_matrix_validation.plan.md -- access token management, homeserver trust, room permission model, E2EE threat analysis"
    status: pending
  - id: mattermost-analysis
    content: "Mattermost: Create connector_mattermost_analysis.plan.md -- review OpenClaw REST+WS implementation, assess self-hosted deployment patterns, reconnect handling"
    status: pending
  - id: mattermost-development
    content: "Mattermost: Create connector_mattermost_development.plan.md -- MVP (WebSocket events, REST send, DMs, channels), config schema, v2 features (threads, reactions, file upload)"
    status: pending
  - id: mattermost-validation
    content: "Mattermost: Create connector_mattermost_validation.plan.md -- self-hosted security, token management, WebSocket reconnect resilience, integration testing"
    status: pending
  - id: feishu-analysis
    content: "Feishu: Create connector_feishu_analysis.plan.md -- compare OpenClaw (45+ files) and PicoClaw implementations, assess Lark SDK, dual webhook/WebSocket modes"
    status: pending
  - id: feishu-development
    content: "Feishu: Create connector_feishu_development.plan.md -- MVP (webhook mode, DMs, group messages), config schema, v2 features (interactive cards, wiki/drive, streaming cards)"
    status: pending
  - id: feishu-validation
    content: "Feishu: Create connector_feishu_validation.plan.md -- webhook encryption/verification, app credential management, rate limiting, Lark platform compliance"
    status: pending
  - id: line-analysis
    content: "LINE: Create connector_line_analysis.plan.md -- compare OpenClaw and PicoClaw webhook implementations, assess LINE Messaging API, signature verification patterns"
    status: pending
  - id: line-development
    content: "LINE: Create connector_line_development.plan.md -- MVP (webhook handler, text messages, basic media), config schema, LINE ID format handling, v2 features (rich menus, flex messages)"
    status: pending
  - id: line-validation
    content: "LINE: Create connector_line_validation.plan.md -- webhook signature verification, channel secret management, LINE platform security requirements"
    status: pending
  - id: googlechat-analysis
    content: "Google Chat: Create connector_googlechat_analysis.plan.md -- review OpenClaw implementation (15+ files), assess Google Chat API, OAuth service account flow, space model"
    status: pending
  - id: googlechat-development
    content: "Google Chat: Create connector_googlechat_development.plan.md -- MVP (webhook handler, DMs, space messages), OAuth config, v2 features (cards, threads, dialogs)"
    status: pending
  - id: googlechat-validation
    content: "Google Chat: Create connector_googlechat_validation.plan.md -- OAuth credential security, service account key management, Google Workspace compliance"
    status: pending
  - id: nextcloud-analysis
    content: "Nextcloud Talk: Create connector_nextcloud-talk_analysis.plan.md -- review OpenClaw implementation, assess Nextcloud Talk API, self-hosted patterns, signature verification"
    status: pending
  - id: nextcloud-development
    content: "Nextcloud Talk: Create connector_nextcloud-talk_development.plan.md -- MVP (webhook handler, room messages, basic media), config schema, v2 features (polls, reactions, file sharing)"
    status: pending
  - id: nextcloud-validation
    content: "Nextcloud Talk: Create connector_nextcloud-talk_validation.plan.md -- webhook signature verification, self-hosted network security, room policy testing"
    status: pending
  - id: irc-analysis
    content: "IRC: Create connector_irc_analysis.plan.md -- review OpenClaw raw TCP implementation, assess IRC protocol (RFC 2812), TLS, SASL auth, modern IRCv3 capabilities"
    status: pending
  - id: irc-development
    content: "IRC: Create connector_irc_development.plan.md -- MVP (TCP/TLS connection, channel join, DMs, nick registration), config schema, v2 features (SASL, multi-server, channel modes)"
    status: pending
  - id: irc-validation
    content: "IRC: Create connector_irc_validation.plan.md -- TLS certificate verification, nick/password security, flood protection, connection resilience testing"
    status: pending
  - id: nostr-analysis
    content: "Nostr: Create connector_nostr_analysis.plan.md -- review OpenClaw relay WebSocket implementation, assess NIP-04 DMs, relay discovery, event signing, state persistence"
    status: pending
  - id: nostr-development
    content: "Nostr: Create connector_nostr_development.plan.md -- MVP (single relay, NIP-04 DMs, event signing), config schema (nsec key), v2 features (multi-relay, NIP-44, circuit breaker)"
    status: pending
  - id: nostr-validation
    content: "Nostr: Create connector_nostr_validation.plan.md -- private key management, relay trust model, seen-event deduplication, NIP compliance testing"
    status: pending
  - id: synology-analysis
    content: "Synology Chat: Create connector_synology-chat_analysis.plan.md -- review OpenClaw implementation, assess Synology Chat API, incoming/outgoing webhook model"
    status: pending
  - id: synology-development
    content: "Synology Chat: Create connector_synology-chat_development.plan.md -- MVP (webhook handler, send via incoming URL), config schema, v2 features (file sharing, user directory)"
    status: pending
  - id: synology-validation
    content: "Synology Chat: Create connector_synology-chat_validation.plan.md -- NAS network security, webhook token management, self-hosted deployment testing"
    status: pending
  - id: twitch-analysis
    content: "Twitch: Create connector_twitch_analysis.plan.md -- review OpenClaw Twurple implementation (30+ files), assess Twitch Chat (IRC-like WS), OAuth, EventSub API"
    status: pending
  - id: twitch-development
    content: "Twitch: Create connector_twitch_development.plan.md -- MVP (Chat WebSocket, channel messages, OAuth), config schema, v2 features (EventSub, channel points, whispers)"
    status: pending
  - id: twitch-validation
    content: "Twitch: Create connector_twitch_validation.plan.md -- OAuth token refresh, channel access control, rate limit compliance, Twitch ToS alignment"
    status: pending
  - id: zalo-analysis
    content: "Zalo: Create connector_zalo_analysis.plan.md -- review OpenClaw dual-mode implementation, assess Zalo Official Account API, webhook vs polling tradeoffs"
    status: pending
  - id: zalo-development
    content: "Zalo: Create connector_zalo_development.plan.md -- MVP (webhook mode, DMs, group messages), config schema, proxy support, v2 features (rich messages, group policy)"
    status: pending
  - id: zalo-validation
    content: "Zalo: Create connector_zalo_validation.plan.md -- webhook verification, API credential management, Vietnamese data residency considerations"
    status: pending
  - id: bluebubbles-analysis
    content: "BlueBubbles: Create connector_bluebubbles_analysis.plan.md -- review OpenClaw implementation (25+ files), assess BlueBubbles server API, debounce/reply-cache patterns"
    status: pending
  - id: bluebubbles-development
    content: "BlueBubbles: Create connector_bluebubbles_development.plan.md -- MVP (webhook handler, send messages, basic media), config schema, v2 features (reactions, attachments, reply threading)"
    status: pending
  - id: bluebubbles-validation
    content: "BlueBubbles: Create connector_bluebubbles_validation.plan.md -- BlueBubbles server trust, macOS dependency, webhook security, message deduplication testing"
    status: pending
  - id: imessage-analysis
    content: "iMessage: Create connector_imessage_analysis.plan.md -- review OpenClaw core bridge approach, assess macOS-only constraints, BlueBubbles vs AppleScript vs pypush options"
    status: pending
  - id: imessage-development
    content: "iMessage: Create connector_imessage_development.plan.md -- MVP (BlueBubbles bridge, DMs, basic media), config schema, platform constraints, v2 features (group chats, tapbacks)"
    status: pending
  - id: imessage-validation
    content: "iMessage: Create connector_imessage_validation.plan.md -- macOS security model, bridge process isolation, Apple account credential handling, platform limitation documentation"
    status: pending
isProject: false
---

# Connector Expansion Roadmap

This plan turns every missing connector into an independent workstream following the **Research -> Develop -> Harden -> Document** lifecycle. Each connector produces 3 plan files in `.cursor/plans/`.

---

## Reference Architecture

All new connectors implement the established NAVI pattern from [connectors/connector.go](connectors/connector.go):

```go
type Connector interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Send(ctx context.Context, msg OutboundMessage) error
    IsRunning() bool
}
```

With optional capabilities from [connectors/capabilities.go](connectors/capabilities.go): `WebhookHandler`, `MediaSender`, `HealthChecker`, `TypingCapable`, `MessageEditor`, `ReactionCapable`, `PlaceholderCapable`, `MessageLengthProvider`.

Base pattern from [connectors/base.go](connectors/base.go): `BaseConnector` for allow-list and running state.

Registry: [internal/connectors/registry.go](internal/connectors/registry.go) -- `RegisterFactory(name, factory)`, `Create(name, cfg, bus)`.

Config: [internal/config/config.go](internal/config/config.go) -- add `XxxConfig` struct + field on `Config`.

Wiring: [cmd/navid/main.go](cmd/navid/main.go) -- register factory, extend `startConnector`.

---

## Reference Sources Per Connector

Each analysis plan compares implementations from two reference codebases:

- **OpenClaw** (TypeScript): `.example-code/openclaw/extensions/<name>/`
- **PicoClaw** (Go): upstream `pkg/channels/<name>/` (where available)

Cross-reference availability:


| Connector      | OpenClaw                       | PicoClaw              | Transport               |
| -------------- | ------------------------------ | --------------------- | ----------------------- |
| Discord        | Yes (thin + SDK)               | Yes (~500 LOC)        | Gateway WebSocket       |
| WhatsApp       | Yes (Web bridge)               | Yes (bridge + native) | WebSocket / HTTP bridge |
| Signal         | Yes (HTTP/CLI)                 | No                    | HTTP / signal-cli       |
| MS Teams       | Yes (60+ files, Bot Framework) | No                    | HTTP webhook            |
| Matrix         | Yes (70+ files, matrix-js-sdk) | No                    | WebSocket sync          |
| Mattermost     | Yes (25+ files, REST+WS)       | No                    | REST + WebSocket        |
| Feishu         | Yes (45+ files, SDK)           | Yes                   | Webhook / WebSocket     |
| LINE           | Yes (webhook)                  | Yes                   | Webhook                 |
| Google Chat    | Yes (15+ files, OAuth)         | No                    | Webhook                 |
| Nextcloud Talk | Yes (15+ files)                | No                    | Webhook                 |
| IRC            | Yes (20+ files, raw TCP)       | No                    | Raw TCP/TLS             |
| Nostr          | Yes (20+ files, relay WS)      | No                    | WebSocket (relays)      |
| Synology Chat  | Yes (12+ files)                | No                    | Webhook                 |
| Twitch         | Yes (30+ files, Twurple)       | No                    | WebSocket (Chat)        |
| Zalo           | Yes (18+ files)                | No                    | Webhook + polling       |
| BlueBubbles    | Yes (25+ files)                | No                    | Webhook                 |
| iMessage       | Yes (thin + core)              | No                    | macOS bridge            |


---

## Plan File Convention

Each connector produces 3 plan files:

```
.cursor/plans/connector_<name>_analysis.plan.md      -- Phase 1: Research
.cursor/plans/connector_<name>_development.plan.md    -- Phase 2: Develop
.cursor/plans/connector_<name>_validation.plan.md     -- Phase 3: Harden + Document
```

---

## Plan File Templates

### Analysis Plan Template (Phase 1: Research)

Each `connector_<name>_analysis.plan.md` must contain:

1. **Platform Overview** -- What the platform is, user base, API maturity, documentation quality
2. **OpenClaw Implementation Review** -- Architecture, file structure, patterns, transport mechanism, capabilities used, strengths, weaknesses
3. **PicoClaw Implementation Review** (if available) -- Same breakdown; note "N/A" if PicoClaw has no implementation
4. **Comparative Analysis** -- What each does well, where each falls short, design patterns worth adopting
5. **Platform API Assessment** -- Auth model, rate limits, message formats, media support, webhook vs polling vs WebSocket, threading model
6. **Adoptable Patterns** -- Concrete list of patterns/approaches to carry into our implementation
7. **Risk and Complexity Assessment** -- API stability, dependency weight, platform restrictions, regional considerations

### Development Plan Template (Phase 2: Develop)

Each `connector_<name>_development.plan.md` must contain:

1. **MVP Scope** -- Minimum viable connector: connect, receive messages, send messages, basic auth
2. **File Structure** -- Exact files to create under `plugins/<name>/connectors/<name>/` plus `plugins/<name>/plugin.yaml`
3. **Config Schema** -- `XxxConfig` struct fields, YAML keys, environment variable overrides
4. **Connector Implementation** -- Core struct, Start/Stop lifecycle, message handling flow
5. **Capabilities Matrix** -- Which optional interfaces to implement in MVP vs v2
6. **Integration Points** -- Registry factory, `main.go` wiring, gateway registration
7. **Message Format Mapping** -- Platform message types to NAVI `OutboundMessage` / `InboundMessage`
8. **Dependencies** -- Go libraries needed (prefer stdlib + minimal SDKs)
9. **v2 / Future Enhancements** -- Features deferred from MVP (threading, reactions, media, rich cards, etc.)

### Validation Plan Template (Phase 3: Harden + Document)

Each `connector_<name>_validation.plan.md` must contain:

1. **Unit Testing Strategy** -- Mock boundaries, table-driven tests, message parsing tests
2. **Integration Testing Strategy** -- Test harness for platform API, webhook simulation, end-to-end flow
3. **Security Analysis** -- Token/secret handling, webhook signature verification, input validation, rate limiting
4. **Secret Management** -- How credentials are stored (`store.SetSetting`), rotated, and protected
5. **Data Protection** -- PII handling, message content logging policy, data retention
6. **Threat Model** -- Spoofed webhooks, token leakage, privilege escalation, replay attacks
7. **User Documentation** -- Configuration guide, supported commands, setup instructions
8. **Technical Documentation** -- Architecture diagram, component descriptions, API surface
9. **Operational Notes** -- Deployment, monitoring, troubleshooting, health check behavior

---

## Implementation Order

### Tier 1 -- High Reach, Strong References (Start Here)

These have the most users, strongest reference implementations, and closest similarity to existing Telegram/Slack connectors.

**1. Discord**

- References: OpenClaw (thin + SDK), PicoClaw (~500 LOC Go)
- Transport: Gateway WebSocket + REST
- Complexity: Medium -- WebSocket lifecycle, gateway intents, sharding (v2)
- PicoClaw Go implementation is directly referenceable

**2. WhatsApp**

- References: OpenClaw (Web bridge), PicoClaw (bridge + native)
- Transport: WebSocket (WhatsApp Web) or Meta Cloud API (webhook + REST)
- Complexity: High -- multiple API options, session management, phone number verification
- MVP: Meta Cloud API (webhook) for simplicity

**3. Signal**

- References: OpenClaw only (HTTP/signal-cli)
- Transport: HTTP bridge to signal-cli or signald
- Complexity: Medium -- depends on external bridge process
- MVP: signal-cli REST API integration

### Tier 2 -- Enterprise / Collaboration

**4. MS Teams**

- References: OpenClaw (60+ files, Bot Framework)
- Transport: HTTP webhook (Bot Framework / Graph API)
- Complexity: High -- Azure app registration, OAuth, Adaptive Cards, conversation store
- MVP: Bot Framework webhook, direct messages, basic channel messages

**5. Matrix**

- References: OpenClaw (70+ files, matrix-js-sdk)
- Transport: Client-Server API (HTTP sync or WebSocket)
- Complexity: Medium-High -- sync protocol, room management, E2EE (v2)
- MVP: HTTP sync, DMs, room messages

**6. Mattermost**

- References: OpenClaw (25+ files)
- Transport: REST + WebSocket
- Complexity: Medium -- similar to Slack, self-hosted
- MVP: WebSocket events, REST send, basic threading

### Tier 3 -- Regional / Niche

**7. Feishu (Lark)**

- References: OpenClaw (45+ files), PicoClaw (available)
- Transport: Webhook or WebSocket (dual mode)
- Complexity: Medium-High -- Lark SDK, interactive cards, wiki/drive integration (v2)
- MVP: Webhook mode, DMs, group messages

**8. LINE**

- References: OpenClaw (webhook), PicoClaw (available)
- Transport: Webhook (Messaging API)
- Complexity: Low-Medium -- straightforward webhook, signature verification
- MVP: Webhook handler, text messages, basic media

**9. Google Chat**

- References: OpenClaw (15+ files, OAuth)
- Transport: Webhook (HTTP)
- Complexity: Medium -- OAuth service account, space/user targeting
- MVP: Webhook handler, DMs, space messages

**10. Nextcloud Talk**

- References: OpenClaw (15+ files)
- Transport: Webhook (HTTP)
- Complexity: Low-Medium -- self-hosted, signature verification
- MVP: Webhook handler, room messages

**11. IRC**

- References: OpenClaw (20+ files, raw TCP)
- Transport: Raw TCP/TLS
- Complexity: Medium -- connection management, nick registration, channel join, no native media
- MVP: TCP connection, channel messages, DMs

**12. Nostr**

- References: OpenClaw (20+ files, relay WebSocket)
- Transport: WebSocket (relay network)
- Complexity: Medium-High -- relay discovery, NIP-04 encryption, event signing, circuit breaker
- MVP: Single relay, DMs (NIP-04), basic event handling

**13. Synology Chat**

- References: OpenClaw (12+ files)
- Transport: Webhook (HTTP)
- Complexity: Low -- incoming URL, simple webhook, self-hosted NAS
- MVP: Webhook handler, send via incoming URL

**14. Twitch**

- References: OpenClaw (30+ files, Twurple)
- Transport: WebSocket (Twitch Chat / IRC-like)
- Complexity: Medium -- OAuth, channel-based, no DMs, access control
- MVP: Chat WebSocket, channel messages, basic auth

**15. Zalo**

- References: OpenClaw (18+ files)
- Transport: Webhook + polling (dual mode)
- Complexity: Medium -- Vietnamese market, group policy, proxy support
- MVP: Webhook mode, DMs, group messages

**16. BlueBubbles**

- References: OpenClaw (25+ files)
- Transport: Webhook (HTTP to BlueBubbles server)
- Complexity: Medium -- macOS dependency (BlueBubbles server), debounce, reply cache
- MVP: Webhook handler, send messages, basic media

**17. iMessage**

- References: OpenClaw (thin + core)
- Transport: macOS bridge (AppleScript / BlueBubbles)
- Complexity: High (platform constraint) -- macOS only, bridge dependency
- MVP: BlueBubbles bridge integration, DMs, basic media

---

## PicoClaw-Only Connectors (Bonus Tier)

These exist in PicoClaw but not OpenClaw. Consider for future expansion:

- **DingTalk** -- Chinese enterprise messaging (webhook/API)
- **OneBot** -- QQ bot protocol (WebSocket)
- **WeCom / WeCom App** -- WeChat Work (webhook/API)
- **QQ** -- Chinese IM

These are **not** included as active todos but should be tracked for future consideration.

---

## Workflow Per Connector

```mermaid
flowchart TD
    Start["Select Connector"] --> Analysis["Phase 1: Analysis Plan"]
    Analysis --> AnalysisDoc["connector_{name}_analysis.plan.md"]
    AnalysisDoc --> Development["Phase 2: Development Plan"]
    Development --> DevDoc["connector_{name}_development.plan.md"]
    DevDoc --> Implementation["Build MVP"]
    Implementation --> Validation["Phase 3: Validation Plan"]
    Validation --> ValDoc["connector_{name}_validation.plan.md"]
    ValDoc --> Testing["Execute Tests + Security Review"]
    Testing --> Documentation["Update docs/"]
    Documentation --> Done["Connector Complete"]
```



Each phase gate requires the previous plan to be complete before proceeding. Connectors within the same tier can be worked in parallel.

---

## Execution Strategy

- Work **one tier at a time**, starting with Tier 1
- Within a tier, connectors can be parallelized across workstreams
- Each connector's Phase 1 (Analysis) can begin immediately -- it is read-only research
- Phase 2 (Development) begins only after Phase 1 is reviewed
- Phase 3 (Validation) begins alongside or immediately after MVP implementation
- All plans live in `.cursor/plans/` and follow the templates above

