# Discord Connector Development Plan

## 1. MVP Definition
- Connect to Discord Gateway via WebSocket.
- Listen for messages in allowed channels or DMs.
- Route commands (e.g., `/navi`) to the NAVI orchestrator.
- Send text and media responses back to the Discord channel.

## 2. Architecture Integration
- Implement `connectors.Connector` interface.
- Register with `GatewayURL` and use `GatewaySecret` for auth.
- Use `bus.Bus` to dispatch events to the NAVI backend.

## 3. Future Enhancements (v2)
- Support Discord Slash Commands natively.
- Interactive UI components (Buttons, Dropdowns) for HITL approvals.
- Threaded conversation tracking.
