package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillSynthesizer handles converting external skill formats into Navi's OSS-27 compliant SKILL.yaml format.
type SkillSynthesizer struct {
	targetDir string
}

func NewSynthesizer(targetDir string) *SkillSynthesizer {
	return &SkillSynthesizer{targetDir: targetDir}
}

// SynthesizeFromOpenClaw converts an OpenClaw-style plugin manifest into a Navi skill.
// It accepts either a plugin directory or a direct path to a JSON manifest.
func (s *SkillSynthesizer) SynthesizeFromOpenClaw(pluginPath string) error {
	manifestPath, err := resolveOpenClawManifestPath(pluginPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("synthesize openclaw: parse manifest: %w", err)
	}

	name := firstNonEmpty(
		stringValue(raw["name"]),
		stringValue(raw["id"]),
		filepath.Base(filepath.Dir(manifestPath)),
	)
	description := firstNonEmpty(
		stringValue(raw["description"]),
		fmt.Sprintf("Synthesized from OpenClaw plugin: %s", name),
	)
	version := firstNonEmpty(stringValue(raw["version"]), "0.1.0")

	tools := extractToolDefinitions(raw)
	if len(tools) == 0 {
		tools = []externalToolDefinition{{
			Name:        "run",
			Description: "Execute the imported OpenClaw capability.",
			InputSchema: defaultPromptInputSchema(),
			Transport: TransportSpec{
				Type: "internal",
			},
		}}
	}

	spec := synthesizedSpec("openclaw", name, description, version, tools)
	spec.Capability.Tags = appendUnique(spec.Capability.Tags, "openclaw", "imported")
	spec.Capability.Domains = appendUnique(spec.Capability.Domains, "migration")
	spec.Capability.Provides = appendUnique(spec.Capability.Provides, spec.SkillID)
	if channels := stringSlice(raw["channels"]); len(channels) > 0 {
		spec.Capability.Tags = appendUnique(spec.Capability.Tags, channels...)
	}

	return s.writeSynthesizedSkill(slugify(name), spec)
}

// SynthesizeFromClaude converts Claude/OpenAI-style tool definitions into a Navi skill.
// The content may be a single JSON tool definition, an array of definitions, or a fenced JSON block.
func (s *SkillSynthesizer) SynthesizeFromClaude(name, content string) error {
	rawJSON, err := extractJSONPayload(content)
	if err != nil {
		return fmt.Errorf("synthesize claude: %w", err)
	}

	tools, err := parseClaudeToolDefinitions(rawJSON)
	if err != nil {
		return fmt.Errorf("synthesize claude: %w", err)
	}
	if len(tools) == 0 {
		tools = []externalToolDefinition{{
			Name:        "execute",
			Description: "Execute the imported Claude capability.",
			InputSchema: defaultPromptInputSchema(),
			Transport: TransportSpec{
				Type: "internal",
			},
		}}
	}

	if strings.TrimSpace(name) == "" {
		name = firstNonEmpty(tools[0].Name, "claude-import")
	}
	description := fmt.Sprintf("Synthesized from Claude tool definition: %s", name)
	if len(tools) > 1 {
		description = fmt.Sprintf("Synthesized from Claude tool set: %s", name)
	}

	spec := synthesizedSpec("claude", name, description, "0.1.0", tools)
	spec.Capability.Tags = appendUnique(spec.Capability.Tags, "claude", "imported")
	spec.Capability.Domains = appendUnique(spec.Capability.Domains, "tool-use")
	spec.Capability.Provides = appendUnique(spec.Capability.Provides, spec.SkillID)

	return s.writeSynthesizedSkill(slugify(name), spec)
}

type externalToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
	Transport   TransportSpec
}

func (s *SkillSynthesizer) writeSynthesizedSkill(dirName string, spec OSS27Spec) error {
	ApplySpecDefaults(&spec)
	if err := ValidateSpec(&spec); err != nil {
		return fmt.Errorf("synthesize skill: invalid generated spec: %w", err)
	}

	destDir := filepath.Join(s.targetDir, dirName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(destDir, "SKILL.yaml"), data, 0o644)
}

func synthesizedSpec(source, name, description, version string, tools []externalToolDefinition) OSS27Spec {
	spec := OSS27Spec{
		OSS27Version: "1.0",
		SkillID:      fmt.Sprintf("synthesized.%s.%s", source, slugify(name)),
		Semver:       normalizeVersion(version),
		Display: DisplayMetadata{
			Name:        sanitizeDisplayName(name),
			Description: description,
		},
		Effects: EffectMetadata{
			SideEffects:          []string{},
			RiskTier:             "low",
			Idempotency:          true,
			IdempotencyLevel:     "idempotent",
			Reversibility:        "reversible_internal",
			RequiresConfirmation: false,
		},
		Security: SecuritySpec{
			Auth: []AuthMethod{},
			DataAccess: DataAccess{
				PII:     "none",
				Secrets: "forbidden",
			},
			Sandbox: SandboxSpec{
				Required:      false,
				NetworkEgress: []string{},
			},
		},
		Observability: ObservabilitySpec{
			LogRedaction: []string{},
			EmitMetrics:  []string{},
		},
		Governance: GovernanceSpec{
			Publisher: "navi.synthesizer",
			Signed:    false,
			TrustTier: "local",
		},
		Capability: CapabilityMetadata{
			CommandType: "invoke",
		},
		Reliability: ReliabilitySpec{
			ExpectedFailureModes: []string{},
			RetryPolicy:          "none",
		},
	}
	spec.Performance.ExpectedP50MS = 250
	spec.Performance.TimeoutMS = 30000
	spec.Performance.RateLimit.QPS = 1
	spec.Performance.RateLimit.Burst = 2

	for _, tool := range tools {
		inputSchema := tool.InputSchema
		if len(inputSchema) == 0 {
			inputSchema = defaultPromptInputSchema()
		}
		transport := normalizeTransport(tool.Transport)
		spec.Interfaces = append(spec.Interfaces, Interface{
			Name:         sanitizeInterfaceName(tool.Name),
			Transport:    transport,
			InputSchema:  inputSchema,
			OutputSchema: defaultOutputSchema(),
		})
		if host := networkHostForTransport(transport); host != "" {
			spec.Security.Sandbox.NetworkEgress = appendUnique(spec.Security.Sandbox.NetworkEgress, host)
		}
	}
	return spec
}

func resolveOpenClawManifestPath(pluginPath string) (string, error) {
	info, err := os.Stat(pluginPath)
	if err != nil {
		return "", fmt.Errorf("synthesize openclaw: stat path: %w", err)
	}
	if !info.IsDir() {
		return pluginPath, nil
	}

	candidates := []string{
		filepath.Join(pluginPath, "openclaw.plugin.json"),
		filepath.Join(pluginPath, "plugin.json"),
		filepath.Join(pluginPath, "manifest.json"),
		filepath.Join(pluginPath, "package.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("synthesize openclaw: no manifest found in %s", pluginPath)
}

func extractToolDefinitions(raw map[string]any) []externalToolDefinition {
	for _, key := range []string{"tools", "functions", "capabilities"} {
		if defs := decodeToolDefinitions(raw[key]); len(defs) > 0 {
			return defs
		}
	}
	if manifest, ok := raw["manifest"].(map[string]any); ok {
		return extractToolDefinitions(manifest)
	}
	return nil
}

func parseClaudeToolDefinitions(rawJSON []byte) ([]externalToolDefinition, error) {
	rawJSON = bytes.TrimSpace(rawJSON)
	if len(rawJSON) == 0 {
		return nil, fmt.Errorf("empty tool definition payload")
	}

	if rawJSON[0] == '[' {
		var items []any
		if err := json.Unmarshal(rawJSON, &items); err != nil {
			return nil, fmt.Errorf("parse tool array: %w", err)
		}
		return decodeToolDefinitions(items), nil
	}

	var single any
	if err := json.Unmarshal(rawJSON, &single); err != nil {
		return nil, fmt.Errorf("parse tool payload: %w", err)
	}
	return decodeToolDefinitions(single), nil
}

func decodeToolDefinitions(raw any) []externalToolDefinition {
	items, ok := raw.([]any)
	if !ok {
		if raw == nil {
			return nil
		}
		items = []any{raw}
	}

	tools := make([]externalToolDefinition, 0, len(items))
	for _, item := range items {
		def, ok := decodeSingleToolDefinition(item)
		if !ok {
			continue
		}
		tools = append(tools, def)
	}
	return tools
}

func decodeSingleToolDefinition(raw any) (externalToolDefinition, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return externalToolDefinition{}, false
	}

	if fnType := stringValue(m["type"]); fnType == "function" {
		if fn, ok := m["function"].(map[string]any); ok {
			m = fn
		}
	}

	name := firstNonEmpty(
		stringValue(m["name"]),
		stringValue(m["id"]),
		stringValue(m["tool"]),
	)
	if strings.TrimSpace(name) == "" {
		return externalToolDefinition{}, false
	}

	description := firstNonEmpty(
		stringValue(m["description"]),
		fmt.Sprintf("Imported tool %s", name),
	)

	inputSchema := schemaMap(
		firstNonNil(
			m["input_schema"],
			m["inputSchema"],
			m["parameters"],
			m["schema"],
			m["args_schema"],
			m["argsSchema"],
		),
	)
	if len(inputSchema) == 0 {
		inputSchema = defaultPromptInputSchema()
	}

	return externalToolDefinition{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Transport:   inferTransport(m),
	}, true
}

func inferTransport(raw map[string]any) TransportSpec {
	if transportRaw, ok := raw["transport"].(map[string]any); ok {
		t := TransportSpec{Type: stringValue(transportRaw["type"])}
		switch t.Type {
		case "rest":
			t.REST = &RESTTransport{
				URL:    firstNonEmpty(stringValue(transportRaw["url"]), stringValue(transportRaw["endpoint"])),
				Method: firstNonEmpty(strings.ToUpper(stringValue(transportRaw["method"])), "POST"),
			}
		case "mcp_tool":
			t.MCP = &MCPTransport{
				Server: stringValue(transportRaw["server"]),
				Tool:   firstNonEmpty(stringValue(transportRaw["tool"]), stringValue(raw["name"])),
			}
		case "subprocess_python":
			t.SubprocessPython = &SubprocessPythonTransport{
				Entrypoint: stringValue(transportRaw["entrypoint"]),
				Function:   stringValue(transportRaw["function"]),
			}
		case "subprocess":
			t.Runtime = stringValue(transportRaw["runtime"])
			t.Command = stringSlice(transportRaw["command"])
			t.Protocol = stringValue(transportRaw["protocol"])
		}
		if t.Type != "" {
			return normalizeTransport(t)
		}
	}

	if url := firstNonEmpty(stringValue(raw["url"]), stringValue(raw["endpoint"]), stringValue(raw["api_url"])); url != "" {
		return TransportSpec{
			Type: "rest",
			REST: &RESTTransport{
				URL:    url,
				Method: firstNonEmpty(strings.ToUpper(stringValue(raw["method"])), "POST"),
			},
		}
	}
	if server := stringValue(raw["server"]); server != "" {
		return TransportSpec{
			Type: "mcp_tool",
			MCP: &MCPTransport{
				Server: server,
				Tool:   firstNonEmpty(stringValue(raw["tool"]), stringValue(raw["name"])),
			},
		}
	}
	return TransportSpec{Type: "internal"}
}

func normalizeTransport(in TransportSpec) TransportSpec {
	switch in.Type {
	case "rest":
		if in.REST == nil {
			in.REST = &RESTTransport{}
		}
		if in.REST.Method == "" {
			in.REST.Method = "POST"
		}
	case "mcp_tool":
		if in.MCP == nil {
			in.Type = "internal"
		}
	case "subprocess_python":
		if in.SubprocessPython == nil || strings.TrimSpace(in.SubprocessPython.Entrypoint) == "" {
			in.Type = "internal"
			in.SubprocessPython = nil
		}
	case "subprocess":
		in.Command = normalizedCommand(in.Command)
		if len(in.Command) == 0 {
			in.Type = "internal"
			in.Runtime = ""
			in.Protocol = ""
			break
		}
		if strings.TrimSpace(in.Runtime) == "" {
			in.Runtime = inferSubprocessRuntime(in.Command[0])
		}
		if strings.TrimSpace(in.Protocol) == "" {
			in.Protocol = defaultSubprocessProto
		}
	default:
		in = TransportSpec{Type: "internal"}
	}
	return in
}

func inferSubprocessRuntime(command string) string {
	base := strings.ToLower(filepath.Base(command))
	base = strings.TrimSuffix(base, ".exe")
	switch {
	case strings.HasPrefix(base, "python"), base == "py":
		return "python"
	case base == "node", strings.HasPrefix(base, "nodejs"):
		return "node"
	case base == "sh", base == "bash", base == "pwsh", base == "powershell":
		return "shell"
	default:
		return "custom"
	}
}

func extractJSONPayload(content string) ([]byte, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty content")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return []byte(trimmed), nil
	}

	re := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	if matches := re.FindStringSubmatch(trimmed); len(matches) == 2 {
		return []byte(strings.TrimSpace(matches[1])), nil
	}

	if start := strings.IndexAny(trimmed, "{["); start >= 0 {
		return []byte(strings.TrimSpace(trimmed[start:])), nil
	}
	return nil, fmt.Errorf("no JSON payload found")
}

func schemaMap(raw any) map[string]any {
	if m, ok := raw.(map[string]any); ok {
		return m
	}
	return nil
}

func defaultPromptInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Natural language instructions for this capability",
			},
		},
		"required": []string{"prompt"},
	}
}

func defaultOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]any{
			"result": map[string]any{
				"type":        "string",
				"description": "Primary result of the capability",
			},
		},
	}
}

func sanitizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "synthesized-skill"
	}
	return name
}

func sanitizeInterfaceName(name string) string {
	name = slugify(name)
	if name == "" {
		return "run"
	}
	return strings.ReplaceAll(name, "-", "_")
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "0.1.0"
	}
	version = strings.TrimPrefix(version, "v")
	if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+$`, version); matched {
		return version
	}
	return "0.1.0"
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := stringValue(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func appendUnique(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := append([]string{}, existing...)
	for _, item := range out {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		out = append(out, value)
		seen[value] = struct{}{}
	}
	return out
}

func networkHostForTransport(transport TransportSpec) string {
	switch transport.Type {
	case "rest":
		if transport.REST == nil || transport.REST.URL == "" {
			return ""
		}
		u, err := url.Parse(transport.REST.URL)
		if err != nil {
			return ""
		}
		return u.Hostname()
	case "mcp_tool":
		if transport.MCP == nil {
			return ""
		}
		return strings.TrimSpace(transport.MCP.Server)
	default:
		return ""
	}
}
