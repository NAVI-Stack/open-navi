package connectors

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgconn "github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/config"
	"gopkg.in/yaml.v3"
)

// WorkspaceConnectorSpec describes one runtime connector discovered from the
// workspace connector directory.
type WorkspaceConnectorSpec struct {
	Dir      string
	Manifest pkgconn.ConnectorManifest
}

// LoadWorkspaceConnectorSpecs discovers CONNECTOR.yaml manifests from
// workspace/connectors/<name>/ and returns normalized specs.
func LoadWorkspaceConnectorSpecs(root string) ([]WorkspaceConnectorSpec, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspace connectors: %w", err)
	}

	specs := make([]WorkspaceConnectorSpec, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		manifestPath := filepath.Join(dir, "CONNECTOR.yaml")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", manifestPath, err)
		}

		var manifest pkgconn.ConnectorManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
		}
		manifest.Normalize(entry.Name())
		if err := manifest.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}

		specs = append(specs, WorkspaceConnectorSpec{
			Dir:      dir,
			Manifest: manifest,
		})
	}

	return specs, nil
}

// RegisterWorkspaceConnectorDrivers loads workspace connector specs and
// registers a driver for each one.
func RegisterWorkspaceConnectorDrivers(root string, reg *Registry) ([]WorkspaceConnectorSpec, error) {
	specs, err := LoadWorkspaceConnectorSpecs(root)
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return specs, nil
	}
	for _, spec := range specs {
		reg.RegisterDriver(workspaceConnectorDriver{spec: spec})
	}
	return specs, nil
}

type workspaceConnectorDriver struct {
	spec WorkspaceConnectorSpec
}

func (d workspaceConnectorDriver) DriverID() string {
	return d.spec.Manifest.Name
}

func (d workspaceConnectorDriver) Kind() string {
	return d.spec.Manifest.Type
}

func (d workspaceConnectorDriver) DisplayName() string {
	return d.spec.Manifest.DisplayName
}

func (d workspaceConnectorDriver) SetupDescriptor() SetupDescriptor {
	return SetupDescriptor{
		Type:        d.DriverID(),
		DisplayName: d.DisplayName(),
	}
}

func (d workspaceConnectorDriver) CreateInstance(_ *config.Config, _ bus.Bus, _ map[string]string) (pkgconn.Connector, error) {
	switch d.spec.Manifest.Type {
	case "subprocess":
		conn := NewSubprocessConnector(d.spec.Manifest.Name, d.spec.Manifest.Command, d.spec.Manifest.Args, nil)
		conn.SetWorkDir(d.spec.Dir)
		return conn, nil
	default:
		return nil, fmt.Errorf("connector %s: unsupported type %q", d.spec.Manifest.Name, d.spec.Manifest.Type)
	}
}

func (d workspaceConnectorDriver) Capabilities() []CapabilityDescriptor {
	if len(d.spec.Manifest.Capabilities) == 0 {
		return []CapabilityDescriptor{{Name: "messaging.send"}}
	}
	out := make([]CapabilityDescriptor, 0, len(d.spec.Manifest.Capabilities))
	for _, capability := range d.spec.Manifest.Capabilities {
		out = append(out, CapabilityDescriptor{Name: capability.Name})
	}
	return out
}
