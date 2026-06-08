# Connector taxonomy in code (CONN-01)

**Status:** Design (implemented)  
**Spec:** [connectors.md](../canonical/specs/connectors.md) §1.3

## Summary

The canonical connector categories (Communication, Information, Service, Device) are represented in code. Identity and Access is cross-cutting per spec and is not a peer category.

## Code mapping

| Spec category        | Code constant                 | Implemented by              |
|----------------------|-------------------------------|-----------------------------|
| Communication        | `connectors.CategoryCommunication` | Slack, Telegram             |
| Information          | `connectors.CategoryInformation`   | (future)                    |
| Service              | `connectors.CategoryService`       | (future)                    |
| Device and environment | `connectors.CategoryDevice`     | (future)                    |

- **Interface:** `connectors.Categorizable` in `connectors/capabilities.go`. Optional capability; implement to declare category.
- **Discovery:** `internal/connectors.Registry.List()` and `GetInfo(name)` populate `ConnectorInfo.Category` when the connector implements `Categorizable`.
- **Requirement:** New connectors **must** implement `Categorizable` and return one of the four constants so capability resolution and observability can use the taxonomy.

## References

- `connectors/capabilities.go`: `Categorizable`, `Category*` constants
- `plugins/slack/connectors/slack/bot.go`, `plugins/telegram/connectors/telegram/bot.go`: `Category() string { return connectors.CategoryCommunication }`
- `internal/connectors/registry.go`: `ConnectorInfo.Category`, `List()`, `GetInfo()`
