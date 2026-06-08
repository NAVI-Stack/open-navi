package connectors

import (
	"fmt"
	"strings"
)

// ConnectorCapability describes one action or event surface exposed by a
// runtime connector manifest.
type ConnectorCapability struct {
	Name string `yaml:"name" json:"name"`
}

// ConnectorManifest is the manifest format for workspace/runtime connectors
// discovered from workspace/connectors/<name>/CONNECTOR.yaml.
type ConnectorManifest struct {
	Name         string                `yaml:"name" json:"name"`
	DisplayName  string                `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Description  string                `yaml:"description,omitempty" json:"description,omitempty"`
	Type         string                `yaml:"type" json:"type"`
	Command      string                `yaml:"command,omitempty" json:"command,omitempty"`
	Args         []string              `yaml:"args,omitempty" json:"args,omitempty"`
	Capabilities []ConnectorCapability `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

// Normalize fills derived/default fields using the folder name when needed.
func (m *ConnectorManifest) Normalize(defaultName string) {
	if strings.TrimSpace(m.Name) == "" {
		m.Name = strings.TrimSpace(defaultName)
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		m.DisplayName = m.Name
	}
}

// Validate ensures the manifest contains the minimum information required to
// instantiate a runtime connector.
func (m ConnectorManifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("connector manifest missing name")
	}
	switch strings.TrimSpace(m.Type) {
	case "subprocess":
		if strings.TrimSpace(m.Command) == "" {
			return fmt.Errorf("connector %s: subprocess connector missing command", m.Name)
		}
	default:
		return fmt.Errorf("connector %s: unsupported type %q", m.Name, m.Type)
	}
	return nil
}
