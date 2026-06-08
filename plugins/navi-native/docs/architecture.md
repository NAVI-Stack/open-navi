# NAVI Native Plugin Architecture

This plugin provides agentic integration for native filesystem and OS dialog capabilities through the PET desktop bridge.

## Boundary

The plugin is the installable and enable/disable boundary.

The connector is the bridge adapter between NaviD (Docker) and PET (Tauri desktop shell).

The skills are governed callable interfaces exposed to NAVI's cognitive layer.

The policies constrain use inside the plugin's scope, but they do not override owner or system governance.

## Flow

1. NAVI decides to call `navi.native.file_read.read`.
2. Skill registry validates availability and policy.
3. Skill executor dispatches to `navi.connector.pet_bridge`.
4. Connector relays the request over WebSocket/IPC to the PET desktop shell.
5. PET shell performs the native OS operation (Tauri FS/Dialog plugin).
6. Result is relayed back through the bridge to the connector.
7. Result is normalized into a structured skill execution result.
8. Cognitive layer decides whether/how to reflect outcome into History or World Model.

## Bridge Model

```
┌─────────────────────────────────┐     ┌──────────────────────────────┐
│  NaviD (Docker Container)       │     │  PET Desktop (Tauri Shell)   │
│                                 │     │                              │
│  Cognitive Layer                │     │  Tauri Plugin: FS            │
│    ↓                            │     │  Tauri Plugin: Dialog        │
│  Skill Executor                 │     │  Tauri Plugin: Notification  │
│    ↓                            │     │                              │
│  navi.connector.pet_bridge ─────┼─WS──┼─► Bridge Handler             │
│                                 │     │    ↓                         │
│                                 │     │  Host OS Filesystem          │
└─────────────────────────────────┘     └──────────────────────────────┘
```

## Non-goals

- The connector does not reason.
- The plugin does not bypass governance.
- The skill does not write directly to the World Model.
- The bridge does not own durable truth.
- The plugin does not replace workspace boundary enforcement.
