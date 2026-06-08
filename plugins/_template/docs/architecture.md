# Example Messaging Plugin Architecture

This plugin demonstrates the desired NAVI capability package shape.

## Boundary

The plugin is the installable and enable/disable boundary.

The connector is the external API adapter.

The skills are governed callable interfaces exposed to NAVI's cognitive layer.

The policies constrain use inside the plugin's scope, but they do not override owner or system governance.

## Flow

1. NAVI decides to call `navi.example.messaging.send_message.send`.
2. Skill registry validates availability and policy.
3. Skill executor dispatches to `navi.connector.example_messaging`.
4. Connector calls the external provider.
5. Result is normalized into a structured skill execution result.
6. Cognitive layer decides whether/how to reflect outcome into History or World Model.

## Non-goals

- The connector does not reason.
- The plugin does not bypass governance.
- The skill does not write directly to the World Model.
- The runtime does not own durable truth.
