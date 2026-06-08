package skill

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSynthesizeFromOpenClawManifest(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	pluginDir := filepath.Join(t.TempDir(), "search-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := `{
  "id": "web-search",
  "name": "Web Search",
  "description": "Search the web through an imported plugin",
  "version": "1.2.3",
  "channels": ["web", "research"],
  "tools": [
    {
      "name": "search_web",
      "description": "Search the web for a query",
      "input_schema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" }
        },
        "required": ["query"]
      },
      "transport": {
        "type": "rest",
        "url": "https://example.test/search",
        "method": "POST"
      }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "openclaw.plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewSynthesizer(targetDir)
	if err := s.SynthesizeFromOpenClaw(pluginDir); err != nil {
		t.Fatalf("SynthesizeFromOpenClaw: %v", err)
	}

	spec := readSynthesizedSpec(t, filepath.Join(targetDir, "web-search", "SKILL.yaml"))
	if spec.SkillID != "synthesized.openclaw.web-search" {
		t.Fatalf("unexpected skill id: %s", spec.SkillID)
	}
	if spec.Semver != "1.2.3" {
		t.Fatalf("unexpected semver: %s", spec.Semver)
	}
	if len(spec.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(spec.Interfaces))
	}
	if spec.Interfaces[0].Name != "search_web" {
		t.Fatalf("unexpected interface name: %s", spec.Interfaces[0].Name)
	}
	if spec.Interfaces[0].Transport.Type != "rest" || spec.Interfaces[0].Transport.REST == nil {
		t.Fatalf("expected rest transport, got %+v", spec.Interfaces[0].Transport)
	}
	if spec.Interfaces[0].Transport.REST.URL != "https://example.test/search" {
		t.Fatalf("unexpected rest url: %s", spec.Interfaces[0].Transport.REST.URL)
	}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatalf("generated spec should validate: %v", err)
	}
}

func TestSynthesizeFromClaudeAnthropicTools(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	content := `[
  {
    "name": "search_docs",
    "description": "Search documentation",
    "input_schema": {
      "type": "object",
      "properties": {
        "query": { "type": "string" }
      },
      "required": ["query"]
    }
  },
  {
    "name": "open_page",
    "description": "Open a page",
    "input_schema": {
      "type": "object",
      "properties": {
        "url": { "type": "string" }
      },
      "required": ["url"]
    }
  }
]`

	s := NewSynthesizer(targetDir)
	if err := s.SynthesizeFromClaude("Docs Toolkit", content); err != nil {
		t.Fatalf("SynthesizeFromClaude: %v", err)
	}

	spec := readSynthesizedSpec(t, filepath.Join(targetDir, "docs-toolkit", "SKILL.yaml"))
	if spec.SkillID != "synthesized.claude.docs-toolkit" {
		t.Fatalf("unexpected skill id: %s", spec.SkillID)
	}
	if len(spec.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(spec.Interfaces))
	}
	if spec.Interfaces[0].Transport.Type != "internal" {
		t.Fatalf("expected internal transport, got %s", spec.Interfaces[0].Transport.Type)
	}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatalf("generated spec should validate: %v", err)
	}
}

func TestSynthesizeFromClaudeOpenAIFunctionWrapper(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	content := "```json\n{\n  \"type\": \"function\",\n  \"function\": {\n    \"name\": \"lookup_contact\",\n    \"description\": \"Look up a contact by email\",\n    \"parameters\": {\n      \"type\": \"object\",\n      \"properties\": {\n        \"email\": { \"type\": \"string\" }\n      },\n      \"required\": [\"email\"]\n    }\n  }\n}\n```"

	s := NewSynthesizer(targetDir)
	if err := s.SynthesizeFromClaude("", content); err != nil {
		t.Fatalf("SynthesizeFromClaude: %v", err)
	}

	spec := readSynthesizedSpec(t, filepath.Join(targetDir, "lookup-contact", "SKILL.yaml"))
	if len(spec.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(spec.Interfaces))
	}
	if spec.Interfaces[0].Name != "lookup_contact" {
		t.Fatalf("unexpected interface name: %s", spec.Interfaces[0].Name)
	}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatalf("generated spec should validate: %v", err)
	}
}

func TestSynthesizeFromOpenClawGenericSubprocessTransport(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	pluginDir := filepath.Join(t.TempDir(), "worker-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := `{
  "id": "worker-plugin",
  "name": "Worker Plugin",
  "description": "Runs a subprocess worker",
  "tools": [
    {
      "name": "run_worker",
      "transport": {
        "type": "subprocess",
        "command": ["node", "./dist/worker.js"],
        "protocol": "jsonrpc_stdio"
      }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := NewSynthesizer(targetDir)
	if err := s.SynthesizeFromOpenClaw(pluginDir); err != nil {
		t.Fatalf("SynthesizeFromOpenClaw: %v", err)
	}

	spec := readSynthesizedSpec(t, filepath.Join(targetDir, "worker-plugin", "SKILL.yaml"))
	if len(spec.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(spec.Interfaces))
	}
	transport := spec.Interfaces[0].Transport
	if transport.Type != "subprocess" || transport.Runtime != "node" || transport.Protocol != "jsonrpc_stdio" {
		t.Fatalf("unexpected subprocess transport: %+v", transport)
	}
	if len(transport.Command) != 2 || transport.Command[0] != "node" || transport.Command[1] != "./dist/worker.js" {
		t.Fatalf("unexpected command: %#v", transport.Command)
	}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatalf("generated spec should validate: %v", err)
	}
}

func readSynthesizedSpec(t *testing.T, path string) OSS27Spec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var spec OSS27Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	ApplySpecDefaults(&spec)
	return spec
}
