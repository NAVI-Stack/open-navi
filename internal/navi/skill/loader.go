package skill

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxSkillFileSize = 256 * 1024 // 256 KB limit per SKILL.md

// SkillsLoader implements the Zero Framework: a skill is just a directory with
// SKILL.md. No Go registration is needed. Add a skill = drop a folder.
//
// Loading follows a 2-tier priority hierarchy for legacy/user skills:
//
//  1. workspace/skills/  — project-specific, highest priority
//  2. ~/.navi/skills/    — user-global skills (or NAVI_SKILLS_DIR env)
//
// Repo-owned plugin skills are loaded separately through active plugin
// manifests via PluginSkillSource, not through tier scanning.
//
// When the same skill name exists in multiple tiers, the highest-priority tier
// wins and lower tiers are silently skipped.
type SkillsLoader struct {
	workspaceSkills string // workspace/skills/
	globalSkills    string // ~/.navi/skills/
}

func workspaceSkillsDir(workspaceDir string) string {
	cleaned := filepath.Clean(workspaceDir)
	if strings.EqualFold(filepath.Base(cleaned), "skills") {
		return cleaned
	}
	return filepath.Join(workspaceDir, "skills")
}

// NewLoader creates a SkillsLoader. workspaceDir is the base workspace
// (e.g. "./workspace" or "/navi/data/workspace"). For backward compatibility,
// callers may also pass the skills directory itself (e.g. "./workspace/skills").
// The global path is derived automatically.
func NewLoader(workspaceDir string) *SkillsLoader {
	globalDir := globalSkillsDir()
	return &SkillsLoader{
		workspaceSkills: workspaceSkillsDir(workspaceDir),
		globalSkills:    globalDir,
	}
}

// ListSkills returns all available skills across the 2 tiers, deduplicated by
// name (workspace > global). Skills whose binary or environment
// dependencies are not met are still returned but marked as non-activatable;
// higher layers are responsible for hiding or explaining them to the LLM.
func (l *SkillsLoader) ListSkills() ([]SkillEntry, error) {
	seen := make(map[string]bool)
	var result []SkillEntry

	tiers := []struct {
		dir  string
		tier SkillTier
	}{
		{l.workspaceSkills, TierWorkspace},
		{l.globalSkills, TierGlobal},
	}

	for _, t := range tiers {
		entries, err := LoadFromDir(t.dir)
		if err != nil {
			// Missing tier directories are expected — just skip them.
			continue
		}
		for _, e := range entries {
			e.Tier = t.tier
			key := skillIdentity(e)
			if seen[key] {
				continue // lower-priority tier; skip duplicate
			}
			if !e.Activatable {
				log.Printf("skill: loaded %q but not activatable (unmet requirements: bins=%v env=%v os=%v)",
					e.Skill.Name, e.Metadata.RequiresBins, e.Metadata.RequiresEnv, e.Metadata.OS)
			}
			seen[key] = true
			result = append(result, e)
		}
	}

	return result, nil
}

// LoadSkill returns the full SkillEntry for a single skill by name.
// It searches tiers in priority order and returns the first match.
func (l *SkillsLoader) LoadSkill(name string) (*SkillEntry, error) {
	tiers := []struct {
		dir  string
		tier SkillTier
	}{
		{l.workspaceSkills, TierWorkspace},
		{l.globalSkills, TierGlobal},
	}
	for _, t := range tiers {
		skillDir := filepath.Join(t.dir, name)

		yamlPath := filepath.Join(skillDir, "SKILL.yaml")
		mdPath := filepath.Join(skillDir, "SKILL.md")

		var chosenPath string
		if _, err := os.Stat(yamlPath); err == nil {
			chosenPath = yamlPath
		} else if _, err := os.Stat(mdPath); err == nil {
			chosenPath = mdPath
		}

		if chosenPath == "" {
			continue
		}

		entry, err := loadSkillEntry(skillDir, chosenPath)
		if err != nil {
			return nil, err
		}
		entry.Tier = t.tier
		return entry, nil
	}
	return nil, fmt.Errorf("navi: skill loader: skill %q not found", name)
}

// FormatPrompt builds the prompt block that is injected into the NAVI system
// prompt. It lists all skill names and descriptions (LLM View).
func FormatPrompt(entries []SkillEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Available Skills\n\n")
	for _, e := range entries {
		emoji := "🔧"
		if e.Spec != nil && e.Spec.Display.Emoji != "" {
			emoji = e.Spec.Display.Emoji
		} else if e.Metadata.Emoji != "" {
			emoji = e.Metadata.Emoji
		}

		sb.WriteString(fmt.Sprintf("- %s **%s**: %s\n", emoji, e.Skill.Name, e.Skill.Description))

		// If it's an OSS-27 skill, we could also provide a compact schema summary if needed.
		// For now, names and descriptions are enough for "discovery".
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// LoadFromDir scans a directory for skills.
func LoadFromDir(dir string) ([]SkillEntry, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil // directory not found is not an error here
	}
	var entries []SkillEntry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == dir {
			return nil
		}

		// Look for SKILL.yaml (OSS-27) or legacy SKILL.md
		yamlPath := filepath.Join(path, "SKILL.yaml")
		mdPath := filepath.Join(path, "SKILL.md")

		var chosenPath string
		if _, err := os.Stat(yamlPath); err == nil {
			chosenPath = yamlPath
		} else if _, err := os.Stat(mdPath); err == nil {
			chosenPath = mdPath
		}

		if chosenPath == "" {
			return nil
		}

		entry, loadErr := loadSkillEntry(path, chosenPath)
		if loadErr != nil {
			log.Printf("skill: skipping %s: %v", path, loadErr)
			return nil
		}
		entries = append(entries, *entry)
		return fs.SkipDir // don't recurse into skill subdirectories
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// LoadPluginSkills loads only manifest-declared plugin skill paths supplied by
// the plugin registry. It never discovers plugin skills by scanning plugins/*.
func LoadPluginSkills(pathsByPlugin map[string][]string) ([]SkillEntry, error) {
	var entries []SkillEntry
	for pluginID, paths := range pathsByPlugin {
		for _, rawPath := range paths {
			path := filepath.Clean(rawPath)
			info, err := os.Stat(path)
			if err != nil {
				log.Printf("skill: skipping plugin skill %s from %s: %v", path, pluginID, err)
				continue
			}
			var skillDir, filePath string
			if info.IsDir() {
				skillDir = path
				yamlPath := filepath.Join(path, "SKILL.yaml")
				mdPath := filepath.Join(path, "SKILL.md")
				if _, err := os.Stat(yamlPath); err == nil {
					filePath = yamlPath
				} else if _, err := os.Stat(mdPath); err == nil {
					filePath = mdPath
				}
			} else {
				filePath = path
				skillDir = filepath.Dir(path)
			}
			if filePath == "" {
				log.Printf("skill: skipping plugin skill %s from %s: no SKILL.yaml or SKILL.md", path, pluginID)
				continue
			}
			entry, err := loadSkillEntry(skillDir, filePath)
			if err != nil {
				log.Printf("skill: skipping plugin skill %s from %s: %v", path, pluginID, err)
				continue
			}
			entry.Tier = TierBuiltin
			entry.SourcePluginID = pluginID
			entries = append(entries, *entry)
		}
	}
	return entries, nil
}

func loadSkillEntry(skillDir, filePath string) (*SkillEntry, error) {
	absSkillDir, err := filepath.Abs(skillDir)
	if err != nil {
		return nil, fmt.Errorf("resolve skill dir %s: %w", skillDir, err)
	}
	skillDir = absSkillDir

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve skill file %s: %w", filePath, err)
	}
	filePath = absFilePath

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", filePath, err)
	}
	if info.Size() > maxSkillFileSize {
		return nil, fmt.Errorf("skill file in %s exceeds 256KB limit", skillDir)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	var entry SkillEntry
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		var spec OSS27Spec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("yaml unmarshal %s: %w", filePath, err)
		}
		applySpecDefaults(&spec)
		if err := validateSpec(&spec); err != nil {
			return nil, fmt.Errorf("invalid spec %s: %w", filePath, err)
		}
		if spec.PythonRuntime != nil {
			if err := ensurePythonVersion(spec.PythonRuntime.PythonVersion); err != nil {
				return nil, fmt.Errorf("python requirements not met for %s: %w", filePath, err)
			}
		}
		entry = SkillEntry{
			Skill: Skill{
				ID:          spec.SkillID,
				Name:        spec.Display.Name,
				Description: spec.Display.Description,
				FilePath:    filePath,
				BaseDir:     skillDir,
			},
			Spec: &spec,
		}
	} else {
		// Legacy SKILL.md
		content := string(data)
		name, description, meta, _ := ParseFrontmatter(content)
		if name == "" {
			name = filepath.Base(skillDir)
		}
		entry = SkillEntry{
			Skill: Skill{
				Name:        name,
				Description: description,
				FilePath:    filePath,
				BaseDir:     skillDir,
				Body:        StripFrontmatter(content),
			},
			Metadata: meta,
		}
	}

	// Compute activatable flag and reasons_unbound based on environment requirements.
	reasons := unmetRequirements(entry.Metadata)
	entry.Activatable = len(reasons) == 0
	entry.ReasonsUnbound = reasons

	return &entry, nil
}

func skillIdentity(entry SkillEntry) string {
	if entry.Spec != nil && entry.Spec.SkillID != "" {
		return entry.Spec.SkillID
	}
	return entry.Skill.Name
}

// unmetRequirements returns a list of human-readable reasons why a skill
// cannot be activated on the current system. An empty slice means all
// requirements are satisfied.
func unmetRequirements(meta SkillMetadata) []string {
	var reasons []string
	for _, bin := range meta.RequiresBins {
		if !binaryRequirementMet(bin) {
			reasons = append(reasons, "missing binary: "+bin)
		}
	}
	for _, env := range meta.RequiresEnv {
		if os.Getenv(env) == "" {
			reasons = append(reasons, "missing environment variable: "+env)
		}
	}
	if len(meta.OS) > 0 {
		goos := runtime.GOOS
		matched := false
		for _, o := range meta.OS {
			if strings.EqualFold(o, goos) {
				matched = true
				break
			}
		}
		if !matched {
			reasons = append(reasons, "unsupported OS: "+goos)
		}
	}
	return reasons
}

func binaryRequirementMet(bin string) bool {
	switch strings.ToLower(strings.TrimSpace(bin)) {
	case "python", "python3":
		_, err := resolvePythonExecutable(bin)
		return err == nil
	default:
		_, err := exec.LookPath(bin)
		return err == nil
	}
}

// globalSkillsDir returns the user-global skills directory.
// Respects NAVI_SKILLS_DIR env override.
func globalSkillsDir() string {
	if v := os.Getenv("NAVI_SKILLS_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".navi", "skills")
}
