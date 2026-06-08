package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ceoai/navi/internal/llm"
)

// SkillRegistry manages the loaded skills in memory with hot-reload support.
// It wraps a SkillsLoader to support the 3-tier hierarchy but also keeps
// direct Install() support for skills fetched at runtime (e.g. from a hub).
type SkillRegistry struct {
	mu           sync.RWMutex
	workspaceDir string // base workspace directory
	loader       *SkillsLoader
	pluginSource PluginSkillSource
	entries      []SkillEntry
	prompt       string
}

// PluginSkillSource exposes active, manifest-declared plugin skill paths.
// Implemented by internal/navi/plugin.Registry without making the skill loader
// scan plugin directories directly.
type PluginSkillSource interface {
	ActivePluginSkillPaths() map[string][]string
}

// NewRegistry initializes a registry backed by the 3-tier SkillsLoader.
// workspaceDir is the base workspace (e.g. "./workspace" or "/navi/data/workspace").
// For backward compatibility, if workspaceDir looks like a skills subdirectory
// (ends in "skills"), it is used as the workspace skill dir directly.
func NewRegistry(workspaceDir string) *SkillRegistry {
	return &SkillRegistry{
		workspaceDir: workspaceDir,
		loader:       NewLoader(workspaceDir),
		entries:      make([]SkillEntry, 0),
	}
}

func (r *SkillRegistry) SetPluginSkillSource(source PluginSkillSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pluginSource = source
}

// Load (re)scans all 3 tiers and refreshes the in-memory registry.
// Safe to call multiple times (hot reload).
func (r *SkillRegistry) Load() error {
	r.mu.RLock()
	pluginSource := r.pluginSource
	r.mu.RUnlock()

	entries, err := r.loader.ListSkills()
	if err != nil {
		return fmt.Errorf("navi: skill registry: load: %w", err)
	}
	if pluginSource != nil {
		pluginEntries, err := LoadPluginSkills(pluginSource.ActivePluginSkillPaths())
		if err != nil {
			return fmt.Errorf("navi: skill registry: load plugin skills: %w", err)
		}
		entries = mergeSkillEntries(entries, pluginEntries)
	}

	prompt := FormatPrompt(entries)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = entries
	r.prompt = prompt
	return nil
}

func mergeSkillEntries(base []SkillEntry, additions []SkillEntry) []SkillEntry {
	seen := make(map[string]bool, len(base)+len(additions))
	out := make([]SkillEntry, 0, len(base)+len(additions))
	for _, e := range base {
		key := skillIdentity(e)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	for _, e := range additions {
		key := skillIdentity(e)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

// Reload re-scans the skills directory to pick up any new or modified skills.
func (r *SkillRegistry) Reload() error {
	return r.Load()
}

// Snapshot returns the current prompt block and active skill list.
func (r *SkillRegistry) Snapshot() SkillSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]SkillSummary, 0, len(r.entries))
	for _, e := range r.entries {
		if !e.Activatable {
			continue
		}
		summaries = append(summaries, SkillSummary{
			Name:        e.Skill.Name,
			Description: e.Skill.Description,
		})
	}
	return SkillSnapshot{
		Prompt: r.prompt,
		Skills: summaries,
	}
}

// Tools returns the LLM-compatible tool definitions for all active skills.
// This is the "LLM View" required by OSS-27.
func (r *SkillRegistry) Tools() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []llm.ToolDefinition
	for _, e := range r.entries {
		if !e.Activatable {
			continue
		}
		if e.Spec == nil {
			// Legacy skills don't support function calling yet
			continue
		}

		// Each interface in the spec is exposed as a tool
		for _, idx := range e.Spec.Interfaces {
			tools = append(tools, llm.ToolDefinition{
				Name:        toolNameForInterface(e, idx),
				Description: e.Skill.Description,
				Parameters:  idx.InputSchema,
			})
		}
	}
	return tools
}

// Lookup finds a loaded SkillEntry by its name (exact string match).
func (r *SkillRegistry) Lookup(name string) (*SkillEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.entries {
		if e.Skill.Name == name {
			return &e, true
		}
	}
	return nil, false
}

// FindTool finds the skill and interface corresponding to a generated LLM tool name.
func (r *SkillRegistry) FindTool(toolName string) (*SkillEntry, *Interface, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.entries {
		e := &r.entries[i]
		if e.Spec == nil {
			continue
		}
		for j := range e.Spec.Interfaces {
			idx := &e.Spec.Interfaces[j]
			if toolNameForInterface(*e, *idx) == toolName {
				return e, idx, true
			}
		}
	}
	return nil, nil, false
}

// Install copies a skill directory into the workspace skills tier and reloads.
// source must be a directory containing SKILL.md.
// Returns an error if a skill with the same name already exists unless overwrite=true.
func (r *SkillRegistry) Install(source string, overwrite bool) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("navi: skill registry: install source missing: %w", err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("navi: skill registry: install source must be a directory")
	}
	// Check for SKILL.yaml (OSS-27) or legacy SKILL.md
	yamlPath := filepath.Join(source, "SKILL.yaml")
	mdPath := filepath.Join(source, "SKILL.md")

	if _, err := os.Stat(yamlPath); err != nil {
		if _, err := os.Stat(mdPath); err != nil {
			return fmt.Errorf("navi: skill registry: source directory lacks SKILL.yaml or SKILL.md")
		}
	}

	baseName := filepath.Base(source)
	skillsDir := workspaceSkillsDir(r.workspaceDir)
	targetDir := filepath.Join(skillsDir, baseName)

	if _, err := os.Stat(targetDir); err == nil {
		if !overwrite {
			return fmt.Errorf("navi: skill registry: skill %q already exists", baseName)
		}
		_ = os.RemoveAll(targetDir)
	}

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("navi: skill registry: mkdir skills dir: %w", err)
	}

	if err := copyDir(source, targetDir); err != nil {
		return fmt.Errorf("navi: skill registry: copy failed: %w", err)
	}

	return r.Load()
}

// InstallFromHub downloads a skill from the hub and installs it.
func (r *SkillRegistry) InstallFromHub(ctx context.Context, h Hub, entry HubIndexEntry, overwrite bool) error {
	stagingRoot := r.workspaceDir
	if stagingRoot == "" {
		stagingRoot = os.TempDir()
	}
	stageDir, err := os.MkdirTemp(stagingRoot, "navi-hub-download-*")
	if err != nil {
		return fmt.Errorf("failed to create staging dir for download: %w", err)
	}
	defer os.RemoveAll(stageDir)

	if err := h.Download(ctx, entry, stageDir); err != nil {
		return fmt.Errorf("failed to download skill from hub: %w", err)
	}

	// Install() expects a directory that contains SKILL.yaml
	// Often archives have a top-level directory (e.g., skill-name/SKILL.yaml)
	// We need to find the directory containing SKILL.yaml
	var sourceDir string
	err = filepath.Walk(stageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (info.Name() == "SKILL.yaml" || info.Name() == "SKILL.md") {
			sourceDir = filepath.Dir(path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to search downloaded archive: %w", err)
	}
	if sourceDir == "" {
		return fmt.Errorf("downloaded archive does not contain SKILL.yaml or SKILL.md")
	}

	return r.Install(sourceDir, overwrite)
}

// List returns all currently loaded skill entries.
func (r *SkillRegistry) List() []SkillEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make([]SkillEntry, len(r.entries))
	copy(cp, r.entries)
	return cp
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
}

func toolNameForInterface(entry SkillEntry, iface Interface) string {
	base := entry.Skill.Name
	if entry.Spec != nil && entry.Spec.SkillID != "" {
		base = entry.Spec.SkillID
	}
	return sanitizeToolName(fmt.Sprintf("%s_%s", base, iface.Name))
}

func sanitizeToolName(raw string) string {
	cleanName := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			cleanName = append(cleanName, c)
		} else {
			cleanName = append(cleanName, '_')
		}
	}
	return string(cleanName)
}
