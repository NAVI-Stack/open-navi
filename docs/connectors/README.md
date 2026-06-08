# NAVI AI — Connectors

> How NAVI interfaces with external channels and services.

**Status:** Active
**Last Updated:** 2026-06-01
**Audience:** Connector authors, integration engineers, plugin developers
**See also:** [Canonical Connector Spec](../canonical/specs/connectors.md) · [Architecture](../architecture/README.md) · [ADR-007 (Dual-Plane LLM)](../adr/ADR-007-dual-plane-llm.md) · [Docs Index](../README.md)

---

## What Is a Connector?

A **connector** is an integration adapter that lets NAVI interact with external channels, services, and systems. Connectors translate NAVI-internal effect requests into provider-specific operations, and normalize inbound events from those providers into NAVI's internal message surface.

Connectors sit in the **Capability Layer** — below the Cognitive Layer (CEO loop, workers, skills) and above concrete transport protocols.

For the full normative specification, see **[canonical/specs/connectors.md](../canonical/specs/connectors.md)**.

---

## Connector Types

NAVI supports two connector deployment models:

| Model | Description | Config location |
|---|---|---|
| **Builtin (plugin)** | Compiled into `navid` via Go plugin packages under `plugins/`. Registered via `plugin_bootstrap.go`. | `config/runtime.yaml` |
| **Subprocess (runtime)** | Out-of-process executable declared in a `CONNECTOR.yaml` manifest. Loaded at runtime from `workspace/connectors/<name>/`. | `workspace/connectors/<name>/CONNECTOR.yaml` |

---

## Built-in Connectors

| Connector | Status | Config key | Auth requirements |
|---|---|---|---|
| [Telegram](telegram.md) | ✅ Active | `connectors.telegram` | Bot token from @BotFather; owner chat ID from @userinfobot |
| [Slack](slack.md) | ✅ Active | `connectors.slack` | Slack Bot Token; optional App Token for socket mode |
| [Webhook](webhook-connector.md) | ✅ Active | `connectors.webhook` | Optional HMAC-SHA256 shared secret per endpoint |
| [Gateway Bridge](gateway-bridge.md) | ✅ Active | `connectors.helm_bridge` | Helm API key; Helm instance URL |

---

## Subprocess / Runtime Connectors

Any connector following the `CONNECTOR.yaml` manifest format can be installed at runtime without recompiling `navid`. The connector manager discovers manifests under `workspace/connectors/` at startup.

**Minimal `CONNECTOR.yaml`:**

```yaml
name: my-connector
display_name: My Connector
description: Short description of what this connector does.
type: subprocess
command: ./my-connector-binary
args: []
capabilities:
  - name: messaging.send
```

The runtime connector manager wires the subprocess's `stdin`/`stdout` as the inbound/outbound message channels. See the [canonical spec](../canonical/specs/connectors.md) for the full manifest schema and capability taxonomy.

---

## Gateway API — Connector Endpoints

The gateway exposes connector management and health endpoints authenticated by API key:

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health/connectors` | Health status for all active connectors |
| GET | `/api/diagnostics/connectors` | Detailed diagnostics per connector |
| GET | `/api/connectors/setup-schema` | Setup descriptor for each registered connector type |
| POST | `/api/setup/connector` | Start or configure a connector |
| GET | `/api/webhooks` | List registered webhook sources |
| PUT | `/api/webhooks/{source}` | Register or update a webhook |
| DELETE | `/api/webhooks/{source}` | Remove a webhook registration |
| POST | `/api/webhooks/{source}` | Inbound webhook payload ingress (auth varies per connector) |
| POST | `/webhooks/{name}` | Connector-native webhook ingress |

For the full gateway route surface, see [specs/gateway-api.md](../specs/gateway-api.md).

---

## Connector Lifecycle

```
navid startup
    │
    ├── plugin_bootstrap.go registers builtin factories
    │   (telegram, slack)
    │
    ├── ConnectorRegistry scans workspace/connectors/ for CONNECTOR.yaml manifests
    │
    ├── For each registered factory: connector.Start(ctx)
    │   └── Connector enters running state, begins polling/listening
    │
    ├── Runtime loop: inbound messages → NAVI session inbox
    │                 outbound requests → connector.Send(ctx, msg)
    │
    └── Graceful shutdown: connector.Stop(ctx)
```

Connectors that implement `InboundHandlerAware` surface messages directly into the NAVI session via an injected callback. The callback routes the `InboundMessage` to the appropriate session's inbox.

---

*See also: [Canonical Connector Spec](../canonical/specs/connectors.md) · [Gateway API](../specs/gateway-api.md) · [Skills](../concepts/skills.md)*
