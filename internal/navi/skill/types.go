package skill

// Skill represents an internal, normalized AI capability.
type Skill struct {
	ID          string // OSS-27 SkillID
	Name        string
	Description string
	FilePath    string // absolute path to SKILL.yaml
	BaseDir     string // parent directory
	Body        string // content for legacy skills
}

// SkillEntry represents a loaded capability with its execution metadata.
type SkillEntry struct {
	Skill          Skill
	Metadata       SkillMetadata
	Spec           *OSS27Spec // The full OSS-27 specification
	Tier           SkillTier
	SourcePluginID string
	Activatable    bool
	ReasonsUnbound []string
}

// SkillMetadata holds environmental and OS constraints for a skill.
type SkillMetadata struct {
	Always       bool     `json:"always" yaml:"always"`
	RequiresBins []string `json:"requires_bins" yaml:"requires_bins"`
	RequiresEnv  []string `json:"requires_env" yaml:"requires_env"`
	OS           []string `json:"os" yaml:"os"`
	Emoji        string   `json:"emoji" yaml:"emoji"`
}

// SkillTier identifies which directory tier a skill was loaded from.
type SkillTier int

const (
	TierWorkspace SkillTier = iota // workspace/skills/ — highest priority
	TierGlobal                     // ~/.navi/skills/
	TierBuiltin                    // skills/ embedded in binary
)

// SkillSnapshot provides the current prompt block and active loaded skills.
type SkillSnapshot struct {
	Prompt string         // formatted block injected into system prompt
	Skills []SkillSummary // metadata list for logging/status
}

// SkillSummary provides a high-level overview of a skill.
type SkillSummary struct {
	Name        string
	Description string
}
