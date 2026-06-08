package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

// Registry stores tools in registration order with collision detection.
type Registry struct {
	mu        sync.RWMutex
	order     []string
	tools     map[string]*Tool
	aliasToID map[string]string
	version   uint64
}

// LookupResult captures a lookup result plus the registry snapshot that produced it.
type LookupResult struct {
	SnapshotID string
	Tool       *Tool
}

// ListResult captures a list result plus the registry snapshot that produced it.
type ListResult struct {
	SnapshotID string
	Tools      []*Tool
}

// Snapshot is an immutable projection of the registry at a specific version.
type Snapshot struct {
	ID    string
	Tools []*Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		order:     make([]string, 0),
		tools:     make(map[string]*Tool),
		aliasToID: make(map[string]string),
		version:   1,
	}
}

// Register adds a tool. Names must be unique across every source.
func (r *Registry) Register(tool *Tool) error {
	if tool == nil {
		return fmt.Errorf("tool registry: tool is nil")
	}

	cloned := *tool
	if err := cloned.NormalizeAndValidate(); err != nil {
		return err
	}
	toolID := cloned.ToolID

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[toolID]; exists {
		return fmt.Errorf("tool registry: duplicate tool %q", toolID)
	}
	for _, alias := range cloned.Aliases {
		if alias == "" || alias == toolID {
			continue
		}
		if existingID, exists := r.aliasToID[alias]; exists {
			return fmt.Errorf("tool registry: duplicate alias %q for tools %q and %q", alias, existingID, toolID)
		}
		if _, exists := r.tools[alias]; exists {
			return fmt.Errorf("tool registry: alias %q conflicts with registered tool_id", alias)
		}
	}
	r.tools[toolID] = &cloned
	for _, alias := range cloned.Aliases {
		if alias == "" || alias == toolID {
			continue
		}
		r.aliasToID[alias] = toolID
	}
	r.order = append(r.order, toolID)
	r.bumpVersionLocked()
	return nil
}

// Unregister removes a tool by tool_id.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; !ok {
		return
	}
	for _, alias := range r.tools[name].Aliases {
		delete(r.aliasToID, alias)
	}
	delete(r.tools, name)
	for i, registered := range r.order {
		if registered == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	r.bumpVersionLocked()
}

// Lookup returns the registered tool by tool_id.
func (r *Registry) Lookup(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		if toolID, aliasOK := r.aliasToID[name]; aliasOK {
			tool, ok = r.tools[toolID]
		}
	}
	if !ok {
		return nil, false
	}
	return tool, true
}

// LookupExact returns a registry-backed lookup using exact tool_id identity only.
func (r *Registry) LookupExact(name string) (LookupResult, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		return LookupResult{}, false
	}
	return LookupResult{
		SnapshotID: r.snapshotIDLocked(),
		Tool:       cloneTool(tool),
	}, true
}

// LookupWithSnapshot returns a lookup result and the registry snapshot id that served it.
func (r *Registry) LookupWithSnapshot(name string) (LookupResult, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		if toolID, aliasOK := r.aliasToID[name]; aliasOK {
			tool, ok = r.tools[toolID]
		}
	}
	if !ok {
		return LookupResult{}, false
	}
	return LookupResult{
		SnapshotID: r.snapshotIDLocked(),
		Tool:       cloneTool(tool),
	}, true
}

// List returns registered tools in registration order, including hidden ones.
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.order))
	for _, name := range r.order {
		if tool, ok := r.tools[name]; ok {
			out = append(out, tool)
		}
	}
	return out
}

// ListWithSnapshot returns registered tools plus the registry snapshot id.
func (r *Registry) ListWithSnapshot() ListResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.order))
	for _, name := range r.order {
		if tool, ok := r.tools[name]; ok {
			out = append(out, cloneTool(tool))
		}
	}
	return ListResult{
		SnapshotID: r.snapshotIDLocked(),
		Tools:      out,
	}
}

// Snapshot returns an immutable registry snapshot for runtime traceability.
func (r *Registry) Snapshot() Snapshot {
	list := r.ListWithSnapshot()
	return Snapshot{
		ID:    list.SnapshotID,
		Tools: list.Tools,
	}
}

// Replace swaps the registry contents in place while preserving the registry pointer.
// This keeps active runtime references stable during hot reload.
func (r *Registry) Replace(other *Registry) {
	if r == nil || other == nil {
		return
	}
	other.mu.RLock()
	order := append([]string(nil), other.order...)
	tools := make(map[string]*Tool, len(other.tools))
	aliases := make(map[string]string, len(other.aliasToID))
	for name, tool := range other.tools {
		if tool == nil {
			continue
		}
		cloned := *tool
		tools[name] = &cloned
	}
	for alias, toolID := range other.aliasToID {
		aliases[alias] = toolID
	}
	other.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = order
	r.tools = tools
	r.aliasToID = aliases
	r.bumpVersionLocked()
}

// Definitions returns the LLM-facing tool definitions in registration order.
func (r *Registry) Definitions() []llm.ToolDefinition {
	return r.DefinitionsFor("")
}

// DefinitionsFor returns the LLM-facing tool definitions visible on the given
// surface. Empty surface means "all visible surfaces".
func (r *Registry) DefinitionsFor(surface string) []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		if tool, ok := r.tools[name]; ok {
			if tool.Hidden {
				continue
			}
			if !VisibleOnSurface(tool, surface) {
				continue
			}
			out = append(out, tool.Definition)
		}
	}
	return out
}

// Execute runs a registered tool through its executor.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolResult{}, ErrorForFailureCode(
			ExecutionFailureCodeSelectedToolUnavailable,
			fmt.Sprintf("tool registry: tool %q not found", name),
		)
	}
	if tool.Executor == nil {
		return ToolResult{}, ErrorForFailureCode(
			ExecutionFailureCodeSelectedActionMissing,
			fmt.Sprintf("tool registry: tool %q has no executor", name),
		)
	}
	if args == nil {
		args = map[string]any{}
	}
	return tool.Executor.Execute(ctx, args)
}

// ListBySource returns tools from a single source in registration order.
func (r *Registry) ListBySource(source ToolSource) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Tool
	for _, name := range r.order {
		tool, ok := r.tools[name]
		if !ok || tool.Source != source {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// ListByDomain returns tools for a governance domain in registration order.
func (r *Registry) ListByDomain(domain string) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Tool
	for _, name := range r.order {
		tool, ok := r.tools[name]
		if !ok || tool.Governance.Domain != domain {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// VisibleOnSurface reports whether a tool should be exposed on the given surface.
func VisibleOnSurface(tool *Tool, surface string) bool {
	if tool == nil {
		return false
	}
	if len(tool.VisibleOn) == 0 || surface == "" {
		return true
	}
	for _, candidate := range tool.VisibleOn {
		if candidate == surface {
			return true
		}
	}
	return false
}

// Count returns the total number of registered tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ReregisterBySource atomically replaces all tools of the given source with new tools,
// preserving tools from other sources. New tools are appended to the registration order.
func (r *Registry) ReregisterBySource(source ToolSource, tools []*Tool) error {
	for _, t := range tools {
		if t == nil {
			return fmt.Errorf("tool registry: ReregisterBySource tool is nil")
		}
		if t.Source != source {
			return fmt.Errorf("tool registry: ReregisterBySource expected source %q, got %q for tool %q", source, t.Source, t.Name)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	newTools := make(map[string]*Tool)
	newAliases := make(map[string]string)
	var newOrder []string

	// Keep existing tools that don't match the source
	for _, name := range r.order {
		if t, ok := r.tools[name]; ok && t.Source != source {
			newTools[name] = t
			newOrder = append(newOrder, name)
			for _, alias := range t.Aliases {
				if alias == "" || alias == name {
					continue
				}
				newAliases[alias] = name
			}
		}
	}

	// Add the new tools at the end
	for _, t := range tools {
		cloned := *t
		if err := cloned.NormalizeAndValidate(); err != nil {
			return err
		}
		toolID := cloned.ToolID
		if _, exists := newTools[toolID]; exists {
			return fmt.Errorf("tool registry: duplicate tool %q", toolID)
		}
		for _, alias := range cloned.Aliases {
			if alias == "" || alias == toolID {
				continue
			}
			if existingID, exists := newAliases[alias]; exists {
				return fmt.Errorf("tool registry: duplicate alias %q for tools %q and %q", alias, existingID, toolID)
			}
			if _, exists := newTools[alias]; exists {
				return fmt.Errorf("tool registry: alias %q conflicts with registered tool_id", alias)
			}
		}
		newTools[toolID] = &cloned
		for _, alias := range cloned.Aliases {
			if alias == "" || alias == toolID {
				continue
			}
			newAliases[alias] = toolID
		}
		newOrder = append(newOrder, toolID)
	}

	r.tools = newTools
	r.order = newOrder
	r.aliasToID = newAliases
	r.bumpVersionLocked()
	return nil
}

// ReregisterSkills atomically replaces all skill tools.
func (r *Registry) ReregisterSkills(skills []*Tool) error {
	return r.ReregisterBySource(ToolSourceSkill, skills)
}

// ReregisterPlugins atomically replaces all plugin tools.
func (r *Registry) ReregisterPlugins(plugins []*Tool) error {
	return r.ReregisterBySource(ToolSourcePlugin, plugins)
}

func (r *Registry) bumpVersionLocked() {
	r.version++
}

func (r *Registry) snapshotIDLocked() string {
	return fmt.Sprintf("tool-registry-v%d", r.version)
}

func cloneTool(tool *Tool) *Tool {
	if tool == nil {
		return nil
	}
	cloned := *tool
	cloned.InputSchema = cloneSchema(tool.InputSchema)
	cloned.OutputSchema = cloneSchema(tool.OutputSchema)
	cloned.EnvironmentVisibility = append([]string(nil), tool.EnvironmentVisibility...)
	cloned.RequiredModes = append([]schema.DirectiveMode(nil), tool.RequiredModes...)
	cloned.FeatureFlags = append([]string(nil), tool.FeatureFlags...)
	cloned.ConnectorDependencies = append([]string(nil), tool.ConnectorDependencies...)
	cloned.Aliases = append([]string(nil), tool.Aliases...)
	cloned.CapabilityTags = append([]string(nil), tool.CapabilityTags...)
	cloned.VisibleOn = append([]string(nil), tool.VisibleOn...)
	cloned.SideEffects = append([]string(nil), tool.SideEffects...)
	cloned.Definition.Parameters = cloneSchema(tool.Definition.Parameters)
	cloned.Metadata.InteractionModes = append([]ToolInteractionMode(nil), tool.Metadata.InteractionModes...)
	cloned.Metadata.Tags = append([]string(nil), tool.Metadata.Tags...)
	if len(tool.Metadata.Notes) > 0 {
		cloned.Metadata.Notes = make(map[string]string, len(tool.Metadata.Notes))
		for key, value := range tool.Metadata.Notes {
			cloned.Metadata.Notes[key] = value
		}
	}
	return &cloned
}
