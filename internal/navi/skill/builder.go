package skill

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	pkgconn "github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/store"
	"gopkg.in/yaml.v3"
)

type GovernanceDecision string

const (
	GovernanceDecisionApproved GovernanceDecision = "approved"
	GovernanceDecisionPending  GovernanceDecision = "pending"
	GovernanceDecisionBlocked  GovernanceDecision = "blocked"
)

type GovernanceRequest struct {
	CommandType           string
	Domain                string
	Operation             string
	SourceProcess         string
	SourceTrigger         string
	Payload               map[string]any
	AffectedEntities      []string
	Rationale             string
	RequireConfirmation   bool
	ConfirmationRationale string
}

type GovernanceResponse struct {
	Decision       GovernanceDecision
	Reason         string
	ProposalID     string
	ProposalStatus string
}

type GovernanceFunc func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error)

type SkillBuilder struct {
	registry     *SkillRegistry
	llm          llm.Provider
	model        string
	workspaceDir string
	db           *sql.DB
	govern       GovernanceFunc
	hub          Hub
}

type BuildRequest struct {
	Gap         store.Gap
	UserContext string
	ChatID      string
}

type BuildResult struct {
	Kind                   string
	SkillID                string
	SkillDir               string
	ConnectorName          string
	ConnectorDir           string
	ActivationError        string
	Installed              bool
	HubInstalled           bool
	Error                  error
	GapClosed              bool
	GovernedPause          bool
	GovernedReason         string
	GovernedProposalID     string
	GovernedProposalStatus string
}

func NewSkillBuilder(
	registry *SkillRegistry,
	llmProv llm.Provider,
	model string,
	workspaceDir string,
	db *sql.DB,
	govern GovernanceFunc,
	hub Hub,
) *SkillBuilder {
	return &SkillBuilder{
		registry:     registry,
		llm:          llmProv,
		model:        model,
		workspaceDir: workspaceDir,
		db:           db,
		govern:       govern,
		hub:          hub,
	}
}

func (b *SkillBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	switch req.Gap.ClassificationResult.ExpansionPath {
	case "", "skill":
		return b.buildSkill(ctx, req)
	case "connector":
		return b.buildConnector(ctx, req)
	default:
		return &BuildResult{}, fmt.Errorf("gap %s is classified for %q expansion, not supported by scaffolding", req.Gap.ID, req.Gap.ClassificationResult.ExpansionPath)
	}
}

func (b *SkillBuilder) buildSkill(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	result := &BuildResult{}
	if b.registry == nil {
		return result, fmt.Errorf("skill builder registry not configured")
	}
	if b.llm == nil {
		return result, fmt.Errorf("skill builder llm not configured")
	}
	if req.Gap.ID == "" {
		return result, fmt.Errorf("gap id required")
	}
	prompt := b.buildSkillPrompt(req)

	var spec OSS27Spec
	var mainPy, reqsTxt string
	maxRetries := 2
	specValid := false

	for i := 0; i <= maxRetries; i++ {
		messages := []llm.Message{
			{Role: "system", Content: "You are an AI engineer."},
			{Role: "user", Content: prompt},
		}
		resp, err := b.llm.Chat(ctx, b.model, messages, nil, llm.Options{})
		if err != nil {
			return result, fmt.Errorf("skill synthesis failed: %w", err)
		}

		var payload map[string]string
		// Extract JSON from resp.Content (strip markdown block if present)
		rawOutput := resp.Content
		if strings.HasPrefix(rawOutput, "```json") {
			rawOutput = strings.TrimPrefix(rawOutput, "```json")
			rawOutput = strings.TrimSuffix(strings.TrimSpace(rawOutput), "```")
		} else if strings.HasPrefix(rawOutput, "```") {
			rawOutput = strings.TrimPrefix(rawOutput, "```")
			rawOutput = strings.TrimSuffix(strings.TrimSpace(rawOutput), "```")
		}
		if err := json.Unmarshal([]byte(rawOutput), &payload); err != nil {
			prompt += fmt.Sprintf("\n\nValidation Error: Response is not valid JSON. Ensure you return ONLY JSON mapping 'skill_yaml', 'main_py', and 'requirements_txt'.\n")
			continue
		}

		yamlStr := payload["skill_yaml"]
		mainPy = payload["main_py"]
		reqsTxt = payload["requirements_txt"]

		if yamlStr == "" {
			prompt += fmt.Sprintf("\n\nValidation Error: Missing 'skill_yaml' in JSON response.\n")
			continue
		}

		if err := yaml.Unmarshal([]byte(yamlStr), &spec); err != nil {
			prompt += fmt.Sprintf("\n\nValidation Error: Invalid YAML syntax: %v\n", err)
			continue
		}

		if err := ValidateSpec(&spec); err != nil {
			prompt += fmt.Sprintf("\n\nValidation Error: Invalid SKILL spec: %v\n", err)
			continue
		}
		ApplySpecDefaults(&spec)

		specValid = true
		break
	}

	if !specValid {
		// Fallback to hub search if local scaffolding failed and hub is configured
		if b.hub != nil {
			query := req.Gap.ClassificationResult.Reason
			hubResults, err := b.hub.Search(ctx, query)
			if err == nil && len(hubResults) > 0 {
				// Pick the first result that matches (we could rank them better, but MVP is first match)
				bestMatch := hubResults[0]

				governed, err := b.requestGovernance(ctx, GovernanceRequest{
					CommandType:   "create",
					Domain:        "skills",
					Operation:     "install_skill_from_hub",
					SourceProcess: "gap_skill_scaffolding",
					SourceTrigger: req.Gap.ID,
					Payload: map[string]any{
						"type":     "install_skill_from_hub",
						"skill_id": bestMatch.SkillID,
						"gap_id":   req.Gap.ID,
					},
					AffectedEntities:      buildGapProposalEntities(req.Gap.ID, req.ChatID),
					RequireConfirmation:   bestMatch.TrustTier == "community",
					ConfirmationRationale: "community skill installation requires owner confirmation",
				}, result, "skill installation")
				if err != nil {
					return result, err
				}
				if !governed {
					return result, nil
				}

				if err := b.registry.InstallFromHub(ctx, b.hub, bestMatch, false); err != nil {
					return result, fmt.Errorf("failed to install from hub fallback: %w", err)
				}

				// Verify
				entry, ok := b.lookupInstalledSkill(bestMatch.SkillID)
				if !ok {
					return result, fmt.Errorf("hub skill installed but was not found after reload")
				}
				if !entry.Activatable {
					return result, fmt.Errorf("hub skill installed but not activatable. Reasons: %v", entry.ReasonsUnbound)
				}

				result.SkillID = bestMatch.SkillID
				result.Installed = true
				result.HubInstalled = true
				result.SkillDir = entry.Skill.BaseDir

				if b.db != nil && store.UpdateGapStatus(ctx, b.db, req.Gap.ID, store.GapStatusClosed, nil) == nil {
					result.GapClosed = true
				}
				return result, nil
			}
		}

		return result, fmt.Errorf("failed to synthesize valid skill after retries and no hub fallback available")
	}

	result.SkillID = spec.SkillID
	result.Kind = "skill"

	governed, err := b.requestGovernance(ctx, GovernanceRequest{
		CommandType:   "create",
		Domain:        "skills",
		Operation:     "install_skill",
		SourceProcess: "gap_skill_scaffolding",
		SourceTrigger: req.Gap.ID,
		Payload: map[string]any{
			"type":     "install_skill",
			"skill_id": spec.SkillID,
			"gap_id":   req.Gap.ID,
		},
		AffectedEntities: buildGapProposalEntities(req.Gap.ID, req.ChatID),
	}, result, "skill installation")
	if err != nil {
		return result, err
	}
	if !governed {
		return result, nil
	}

	stagingRoot := b.workspaceDir
	if stagingRoot == "" {
		stagingRoot = os.TempDir()
	}
	stageDir, err := os.MkdirTemp(stagingRoot, "navi-skill-build-*")
	if err != nil {
		return result, fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	skillDirName := sanitizeGeneratedArtifactDir(spec.SkillID)
	skillDir := filepath.Join(stageDir, skillDirName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return result, fmt.Errorf("failed to create skill dir: %w", err)
	}

	yamlData, _ := yaml.Marshal(spec)
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), yamlData, 0o644); err != nil {
		return result, fmt.Errorf("failed to write SKILL.yaml: %w", err)
	}

	hasSubprocessPython := false
	for _, iface := range spec.Interfaces {
		if iface.Transport.Type == "subprocess_python" {
			hasSubprocessPython = true
			break
		}
	}

	if hasSubprocessPython {
		if mainPy != "" {
			if err := os.WriteFile(filepath.Join(skillDir, "main.py"), []byte(mainPy), 0o644); err != nil {
				return result, err
			}
		}
		if reqsTxt != "" {
			if err := os.WriteFile(filepath.Join(skillDir, "requirements.txt"), []byte(reqsTxt), 0o644); err != nil {
				return result, err
			}
		}
	}

	// Install and reload
	if err := b.registry.Install(skillDir, false); err != nil {
		return result, fmt.Errorf("installation failed: %w", err)
	}
	result.Installed = true

	// Verify activation
	entry, ok := b.lookupInstalledSkill(spec.SkillID)
	if !ok {
		return result, fmt.Errorf("skill installed but was not found after reload")
	}
	if !entry.Activatable {
		return result, fmt.Errorf("skill installed but not activatable. Reasons: %v", entry.ReasonsUnbound)
	}
	result.SkillDir = entry.Skill.BaseDir

	// Close gap
	if b.db != nil && store.UpdateGapStatus(ctx, b.db, req.Gap.ID, store.GapStatusClosed, nil) == nil {
		result.GapClosed = true
	}

	return result, nil
}

func (b *SkillBuilder) buildConnector(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	result := &BuildResult{Kind: "connector"}
	if b.llm == nil {
		return result, fmt.Errorf("skill builder llm not configured")
	}
	if req.Gap.ID == "" {
		return result, fmt.Errorf("gap id required")
	}
	if req.Gap.ClassificationResult.ExpansionPath != "" && req.Gap.ClassificationResult.ExpansionPath != "connector" {
		return result, fmt.Errorf("gap %s is classified for %q expansion, not connector scaffolding", req.Gap.ID, req.Gap.ClassificationResult.ExpansionPath)
	}

	prompt := b.buildConnectorPrompt(req)
	var manifest pkgconn.ConnectorManifest
	var mainPy, reqsTxt string
	maxRetries := 2
	valid := false

	for i := 0; i <= maxRetries; i++ {
		resp, err := b.llm.Chat(ctx, b.model, []llm.Message{
			{Role: "system", Content: "You are an AI engineer."},
			{Role: "user", Content: prompt},
		}, nil, llm.Options{})
		if err != nil {
			return result, fmt.Errorf("connector synthesis failed: %w", err)
		}

		var payload map[string]string
		rawOutput := resp.Content
		if strings.HasPrefix(rawOutput, "```json") {
			rawOutput = strings.TrimPrefix(rawOutput, "```json")
			rawOutput = strings.TrimSuffix(strings.TrimSpace(rawOutput), "```")
		} else if strings.HasPrefix(rawOutput, "```") {
			rawOutput = strings.TrimPrefix(rawOutput, "```")
			rawOutput = strings.TrimSuffix(strings.TrimSpace(rawOutput), "```")
		}
		if err := json.Unmarshal([]byte(rawOutput), &payload); err != nil {
			prompt += "\n\nValidation Error: Response is not valid JSON. Return ONLY JSON mapping 'connector_yaml', 'main_py', and 'requirements_txt'.\n"
			continue
		}

		yamlStr := payload["connector_yaml"]
		mainPy = payload["main_py"]
		reqsTxt = payload["requirements_txt"]
		if yamlStr == "" {
			prompt += "\n\nValidation Error: Missing 'connector_yaml' in JSON response.\n"
			continue
		}
		if strings.TrimSpace(mainPy) == "" {
			prompt += "\n\nValidation Error: Missing 'main_py'. Generated subprocess connectors must include a runnable Python entrypoint.\n"
			continue
		}

		if err := yaml.Unmarshal([]byte(yamlStr), &manifest); err != nil {
			prompt += fmt.Sprintf("\n\nValidation Error: Invalid CONNECTOR.yaml syntax: %v\n", err)
			continue
		}
		manifest.Normalize("")
		if manifest.Type == "" {
			manifest.Type = "subprocess"
		}
		if manifest.Command == "" {
			manifest.Command = defaultGeneratedConnectorCommand()
		}
		if len(manifest.Args) == 0 {
			manifest.Args = []string{"main.py"}
		}
		if err := manifest.Validate(); err != nil {
			prompt += fmt.Sprintf("\n\nValidation Error: Invalid connector manifest: %v\n", err)
			continue
		}

		valid = true
		break
	}

	if !valid {
		return result, fmt.Errorf("failed to synthesize valid connector scaffold after retries")
	}

	result.ConnectorName = manifest.Name

	governed, err := b.requestGovernance(ctx, GovernanceRequest{
		CommandType:   "create",
		Domain:        "connectors",
		Operation:     "install_connector",
		SourceProcess: "gap_connector_scaffolding",
		SourceTrigger: req.Gap.ID,
		Payload: map[string]any{
			"type":           "install_connector",
			"connector_name": manifest.Name,
			"gap_id":         req.Gap.ID,
		},
		AffectedEntities: buildGapProposalEntities(req.Gap.ID, req.ChatID),
	}, result, "connector installation")
	if err != nil {
		return result, err
	}
	if !governed {
		return result, nil
	}

	root := b.workspaceDir
	if root == "" {
		root = "."
	}
	connectorsDir := filepath.Join(root, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		return result, fmt.Errorf("failed to create connectors dir: %w", err)
	}
	connectorDir := filepath.Join(connectorsDir, sanitizeGeneratedArtifactDir(manifest.Name))
	if err := os.MkdirAll(connectorDir, 0o755); err != nil {
		return result, fmt.Errorf("failed to create connector dir: %w", err)
	}

	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return result, fmt.Errorf("failed to marshal CONNECTOR.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(connectorDir, "CONNECTOR.yaml"), manifestBytes, 0o644); err != nil {
		return result, fmt.Errorf("failed to write CONNECTOR.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(connectorDir, "main.py"), []byte(mainPy), 0o644); err != nil {
		return result, fmt.Errorf("failed to write main.py: %w", err)
	}
	if strings.TrimSpace(reqsTxt) != "" {
		if err := os.WriteFile(filepath.Join(connectorDir, "requirements.txt"), []byte(reqsTxt), 0o644); err != nil {
			return result, fmt.Errorf("failed to write requirements.txt: %w", err)
		}
	}

	result.ConnectorDir = connectorDir
	result.Installed = true
	if b.db != nil && store.UpdateGapStatus(ctx, b.db, req.Gap.ID, store.GapStatusClosed, nil) == nil {
		result.GapClosed = true
	}
	return result, nil
}

func (b *SkillBuilder) buildSkillPrompt(req BuildRequest) string {
	existing := b.registry.List()
	existingIDs := []string{}
	for _, e := range existing {
		if e.Spec != nil && e.Spec.SkillID != "" {
			existingIDs = append(existingIDs, e.Spec.SkillID)
			continue
		}
		if e.Skill.Name != "" {
			existingIDs = append(existingIDs, e.Skill.Name)
		}
	}

	return fmt.Sprintf(`You are a system capable of self-extension.
A capability gap was detected in the active agent session.
User request: %s
Gap Reason: %s
Gap Error: %s
Gap Context: %s

You need to write a complete OSS-27 SKILL.yaml to fulfill this requirement.
If it involves arbitrary business logic, use transport type 'subprocess_python' and provide 'main.py' and 'requirements.txt'.
Existing skills: %v (do not duplicate these).

Return a JSON object with strictly these keys:
{
  "skill_yaml": "...",
  "main_py": "...",
  "requirements_txt": "..."
}

For skill_yaml, follow the OSS-27 specification strictly (oss27_version: 1.0).
Use full JSON-schema for inputs. Include security blocks, side_effects, etc.
`, req.UserContext, req.Gap.ClassificationResult.Reason, req.Gap.Evidence.ToolError, req.Gap.Evidence.RawContext, existingIDs)
}

func (b *SkillBuilder) buildConnectorPrompt(req BuildRequest) string {
	return fmt.Sprintf(`You are a system capable of self-extension.
A capability gap was detected in the active agent session.
User request: %s
Gap Reason: %s
Gap Error: %s
Gap Context: %s

You need to scaffold a runtime connector for NAVI.
Generate a Python-based subprocess connector that communicates using newline-delimited JSON over stdin/stdout:
- NAVI sends {"type":"start"}, {"type":"send","msg":{...}}, and {"type":"stop"}
- The connector emits inbound user messages as {"type":"message","chat_id":"...","content":"...","source_ref":"...","source_channel":"..."}

Return a JSON object with strictly these keys:
{
  "connector_yaml": "...",
  "main_py": "...",
  "requirements_txt": "..."
}

Requirements:
- connector_yaml must be a valid CONNECTOR.yaml manifest
- Use type: subprocess
- Set command/args so NAVI can run the generated Python entrypoint from the connector directory
- main_py must be a runnable long-lived process
- requirements_txt may be empty if no dependencies are needed
`, req.UserContext, req.Gap.ClassificationResult.Reason, req.Gap.Evidence.ToolError, req.Gap.Evidence.RawContext)
}

func (b *SkillBuilder) lookupInstalledSkill(skillID string) (*SkillEntry, bool) {
	for _, entry := range b.registry.List() {
		if entry.Spec != nil && entry.Spec.SkillID == skillID {
			e := entry
			return &e, true
		}
	}
	return nil, false
}

func sanitizeGeneratedArtifactDir(id string) string {
	if id == "" {
		return "generated-artifact"
	}
	out := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z':
			out = append(out, c)
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
		case c >= '0' && c <= '9':
			out = append(out, c)
		case c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "generated-artifact"
	}
	return string(out)
}

func defaultGeneratedConnectorCommand() string {
	if runtime.GOOS == "windows" {
		return "py"
	}
	return "python3"
}

func buildGapProposalEntities(gapID, chatID string) []string {
	entities := []string{"gap:" + gapID}
	if strings.TrimSpace(chatID) != "" {
		entities = append(entities, "session:"+chatID)
	}
	return entities
}

func (b *SkillBuilder) requestGovernance(ctx context.Context, req GovernanceRequest, result *BuildResult, subject string) (bool, error) {
	// NOTE: proposal creation/persistence stays inside the ICS Validate/Govern seam
	// behind b.govern; builder wiring must only consume the normalized adapter response.
	if b.govern == nil {
		return true, nil
	}
	gov, err := b.govern(ctx, req)
	if err != nil {
		return false, fmt.Errorf("%s governance failed: %w", subject, err)
	}
	switch gov.Decision {
	case "", GovernanceDecisionApproved:
		return true, nil
	case GovernanceDecisionPending:
		result.GovernedPause = true
		result.GovernedReason = strings.TrimSpace(gov.Reason)
		result.GovernedProposalID = strings.TrimSpace(gov.ProposalID)
		result.GovernedProposalStatus = strings.TrimSpace(gov.ProposalStatus)
		if result.GovernedReason == "" {
			result.GovernedReason = "awaiting owner approval"
		}
		return false, nil
	case GovernanceDecisionBlocked:
		reason := strings.TrimSpace(gov.Reason)
		if reason == "" {
			reason = "blocked by policy"
		}
		return false, fmt.Errorf("%s blocked: %s", subject, reason)
	default:
		return false, fmt.Errorf("%s governance returned unknown decision %q", subject, gov.Decision)
	}
}
