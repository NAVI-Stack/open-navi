package gateway

import (
	"github.com/open-navi/navi/internal/connectors"
)

// ConnectorInfo describes a registered connector instance (re-exported for API compatibility).
type ConnectorInfo = connectors.ConnectorInfo

// ConnectorRegistry is the connector registry (factories, instances, lifecycle). Re-exported for API compatibility.
type ConnectorRegistry = connectors.Registry

// NewConnectorRegistry creates a new connector registry.
func NewConnectorRegistry() *ConnectorRegistry {
	return connectors.NewRegistry()
}
