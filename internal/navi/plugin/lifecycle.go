package plugin

import (
	"context"
	"errors"
	"strings"
)

// ErrLifecycleNotImplemented is returned by Install and Update when the implementation is not yet available.
var ErrLifecycleNotImplemented = errors.New("plugin lifecycle: not implemented")

// Discover returns plugins that match a capability gap (e.g. "github_pr_review").
// Matches against manifest Capabilities, Triggers, ID, Name, and Description (contains or equals).
func (r *Registry) Discover(ctx context.Context, capabilityGap string) ([]Manifest, error) {
	if capabilityGap == "" {
		return r.Manifests(), nil
	}
	all := r.Manifests()
	gap := strings.ToLower(strings.TrimSpace(capabilityGap))
	var out []Manifest
	for _, m := range all {
		if manifestMatchesCapabilityGap(m, gap) {
			out = append(out, m)
		}
	}
	return out, nil
}

func manifestMatchesCapabilityGap(m Manifest, gap string) bool {
	if strings.Contains(strings.ToLower(m.ID), gap) || strings.Contains(strings.ToLower(m.Name), gap) || strings.Contains(strings.ToLower(m.Description), gap) {
		return true
	}
	for _, c := range m.Capabilities {
		if strings.Contains(strings.ToLower(c), gap) {
			return true
		}
	}
	for _, t := range m.Triggers {
		if strings.Contains(strings.ToLower(t), gap) {
			return true
		}
	}
	return false
}

// InstallOptions configures how a plugin is installed (deps, permissions, path).
type InstallOptions struct {
	Path        string            // optional path or URL for the plugin bundle
	Permissions []string          // requested permission scopes
	Deps        map[string]string // optional dependency overrides
}

// Install adds a plugin to the system from the catalog: if manifestID is already
// registered, returns nil (idempotent). Otherwise looks up the catalog (SetCatalogEntry);
// if found, registers the plugin and enables it. opts.Permissions and opts.Deps are
// for future use (e.g. permission checks before registering). Returns ErrLifecycleNotImplemented
// if manifestID is not in the catalog.
func (r *Registry) Install(ctx context.Context, manifestID string, opts InstallOptions) error {
	if manifestID == "" {
		return ErrLifecycleNotImplemented
	}
	if r.HasManifestID(manifestID) {
		return nil
	}
	m, tools, ok := r.GetCatalogEntry(manifestID)
	if !ok {
		return ErrLifecycleNotImplemented
	}
	if m.ID != manifestID {
		m.ID = manifestID
	}
	r.RegisterPlugin(m, tools)
	r.SetPluginEnabled(manifestID, true)
	return nil
}

// Update evolves an already-installed plugin from the catalog: replaces the
// registered manifest and tools for manifestID with the catalog entry (e.g. new version).
// The plugin remains enabled unless it was disabled. Returns ErrLifecycleNotImplemented
// if manifestID is not in the catalog.
func (r *Registry) Update(ctx context.Context, manifestID string) error {
	if manifestID == "" {
		return ErrLifecycleNotImplemented
	}
	m, tools, ok := r.GetCatalogEntry(manifestID)
	if !ok {
		return ErrLifecycleNotImplemented
	}
	if m.ID != manifestID {
		m.ID = manifestID
	}
	enabled := r.IsPluginEnabled(manifestID)
	r.RemovePlugin(manifestID)
	r.RegisterPlugin(m, tools)
	r.SetPluginEnabled(manifestID, enabled)
	return nil
}

// RetireMode is how to retire a plugin: disable only, replace with another, or remove.
type RetireMode string

const (
	RetireModeDisable RetireMode = "disable" // SetPluginEnabled(id, false)
	RetireModeReplace RetireMode = "replace" // disable this, activate replacement
	RetireModeRemove  RetireMode = "remove"  // unregister and remove from system
)

// RetireOptions supplies optional args for Retire (e.g. replacement plugin ID for Replace mode).
type RetireOptions struct {
	ReplacementID string // required when mode is RetireModeReplace
}

// Retire disables, replaces, or removes a plugin. Replace requires opts.ReplacementID (pass opts as final arg).
func (r *Registry) Retire(ctx context.Context, manifestID string, mode RetireMode, opts ...*RetireOptions) error {
	var opt *RetireOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	switch mode {
	case RetireModeDisable:
		r.SetPluginEnabled(manifestID, false)
		return nil
	case RetireModeReplace:
		if opt == nil || opt.ReplacementID == "" {
			return ErrLifecycleNotImplemented
		}
		r.SetPluginEnabled(manifestID, false)
		r.SetPluginEnabled(opt.ReplacementID, true)
		return nil
	case RetireModeRemove:
		r.RemovePlugin(manifestID)
		return nil
	default:
		return ErrLifecycleNotImplemented
	}
}
