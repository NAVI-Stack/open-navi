package navi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/selfmod"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

func skillRegistryForToolRegistryTest(t *testing.T) *skill.SkillRegistry {
	t.Helper()
	registry := skill.NewRegistry(filepath.Join("..", ".."))
	if err := registry.Load(); err != nil {
		t.Fatalf("Load skills: %v", err)
	}
	return registry
}

func skillRegistryWithWorkspaceSkillForTest(t *testing.T) *skill.SkillRegistry {
	t.Helper()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(workspaceRoot, "skills", "test-provenance-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	skillYAML := `oss27_version: "1.0"
skill_id: "test-provenance-skill"
semver: "0.1.0"
display:
  name: "Test Provenance Skill"
  description: "Temporary workspace skill for tool source provenance tests."
interfaces:
  - name: "run"
    description: "Run the provenance test tool."
    transport:
      type: "internal"
    input_schema:
      type: "object"
      additionalProperties: false
    output_schema:
      type: "object"
      properties: {}
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
performance:
  expected_p50_ms: 50
  timeout_ms: 1000
observability:
  log_redaction: []
  emit_metrics: []
governance:
  publisher: "navi.test"
  signed: false
  trust_tier: "local"
capability:
  tags: ["test"]
  domains: ["testing"]
  provides: ["testing.provenance"]
  command_type: "invoke"
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.yaml: %v", err)
	}
	registry := skill.NewRegistry(workspaceRoot)
	if err := registry.Load(); err != nil {
		t.Fatalf("Load skills: %v", err)
	}
	return registry
}

type fakeRegistryRouter struct{}

func (f *fakeRegistryRouter) Catalog(ctx context.Context) (llm.LLMCatalog, error) {
	return llm.LLMCatalog{}, nil
}
func (f *fakeRegistryRouter) Profiles(ctx context.Context) ([]llm.ModelProfile, error) {
	return nil, nil
}
func (f *fakeRegistryRouter) GetActive(ctx context.Context) (llm.Active, error) {
	return llm.Active{Provider: "ollama", Model: "llama3"}, nil
}
func (f *fakeRegistryRouter) SetActive(ctx context.Context, p, m string) (llm.Active, error) {
	return llm.Active{Provider: p, Model: m}, nil
}
func (f *fakeRegistryRouter) GetPreferences(ctx context.Context) (llm.ModelPreferences, error) {
	return llm.ModelPreferences{}, nil
}
func (f *fakeRegistryRouter) Route(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error) {
	return llm.RouteDecision{}, nil
}
func (f *fakeRegistryRouter) ExecuteProviderAction(ctx context.Context, req llm.ProviderActionRequest) (llm.ProviderOperation, error) {
	return llm.ProviderOperation{}, nil
}
func (f *fakeRegistryRouter) GetOperation(ctx context.Context, id string) (*llm.ProviderOperation, error) {
	return nil, nil
}
func (f *fakeRegistryRouter) ListOperations(ctx context.Context, provider string, limit int) ([]llm.ProviderOperation, error) {
	return nil, nil
}
func (f *fakeRegistryRouter) ProviderModels(ctx context.Context, providerKey string) ([]llm.ModelDescriptor, error) {
	return nil, nil
}
func (f *fakeRegistryRouter) ProviderRunningModels(ctx context.Context, providerKey string) ([]llm.RunningModel, error) {
	return nil, nil
}
func (f *fakeRegistryRouter) SearchModels(ctx context.Context, query string) ([]llm.ModelDescriptor, error) {
	return nil, nil
}

func hasBrokerSuppression(suppressed []navitool.BrokerSuppressedTool, toolID string, reason navitool.BrokerSuppressionReason) bool {
	for _, s := range suppressed {
		if s.ToolID == toolID && s.Reason == reason {
			return true
		}
	}
	return false
}

func TestBuildRuntimeToolRegistry_RejectsCollisions(t *testing.T) {
	registry := skillRegistryForToolRegistryTest(t)
	pluginRegistry := plugin.NewRegistry(nil)
	pluginRegistry.AddTool(llm.ToolDefinition{
		Name:        sendReplyToolName,
		Description: "conflicting plugin tool",
	})
	_, err := buildRuntimeToolRegistry(LoopConfig{
		Skills:         registry,
		WorkspaceDir:   ".",
		PluginRegistry: pluginRegistry,
	})
	if err == nil {
		t.Fatal("expected duplicate tool registration to fail")
	}
}

func TestBuildRuntimeToolRegistry_RegistersHiddenBuiltinsWithoutExposingThem(t *testing.T) {
	registry := skillRegistryForToolRegistryTest(t)
	toolRegistry, err := buildRuntimeToolRegistry(LoopConfig{
		Skills:       registry,
		WorkspaceDir: ".",
		LLMService:   &fakeRegistryRouter{},
	})
	if err != nil {
		t.Fatalf("buildRuntimeToolRegistry: %v", err)
	}

	for _, name := range []string{routerToolList, routerToolGetActive, routerToolSetActive} {
		toolEntry, ok := toolRegistry.Lookup(name)
		if !ok {
			t.Fatalf("expected hidden builtin %q to be registered", name)
		}
		if !toolEntry.Hidden {
			t.Fatalf("expected hidden builtin %q to be marked hidden", name)
		}
	}

	for _, def := range toolRegistry.Definitions() {
		for _, name := range []string{routerToolList, routerToolGetActive, routerToolSetActive} {
			if def.Name == name {
				t.Fatalf("expected hidden builtin %q to be absent from definitions", name)
			}
		}
	}
}

func TestBuildRuntimeToolRegistry_AssignsWorkspaceActionsToRegisteredTools(t *testing.T) {
	registry := skillRegistryForToolRegistryTest(t)
	pluginRegistry := plugin.NewRegistry(nil)
	pluginRegistry.AddTool(llm.ToolDefinition{Name: "test.plugin.echo", Description: "Echo from plugin"})
	toolRegistry, err := buildRuntimeToolRegistry(LoopConfig{
		Skills:          registry,
		WorkspaceDir:    t.TempDir(),
		PluginRegistry:  pluginRegistry,
		SelfModExecutor: selfmod.NewExecutor(t.TempDir()),
		OnSetTimezone:   func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("buildRuntimeToolRegistry: %v", err)
	}
	for _, toolEntry := range toolRegistry.List() {
		action := toolWorkspaceAction(toolEntry, toolCommandType(toolEntry, schema.CommandTypeInvoke))
		if !action.IsValid() {
			t.Fatalf("tool %q missing workspace action mapping: %+v", toolEntry.Name, toolEntry.Governance)
		}
	}
}

func TestBuildRuntimeToolRegistry_SnapshotCapturesSourceProvenance(t *testing.T) {
	registry := skillRegistryWithWorkspaceSkillForTest(t)
	pluginRegistry := plugin.NewRegistry(nil)
	pluginRegistry.AddToolForPlugin("plugin.test.echo", llm.ToolDefinition{
		Name:        "test.plugin.echo",
		Description: "Echo from plugin",
	})
	toolRegistry, err := buildRuntimeToolRegistry(LoopConfig{
		Skills:          registry,
		WorkspaceDir:    t.TempDir(),
		PluginRegistry:  pluginRegistry,
		SelfModExecutor: selfmod.NewExecutor(t.TempDir()),
		OnSetTimezone:   func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("buildRuntimeToolRegistry: %v", err)
	}

	snapshot := toolRegistry.Snapshot()
	if snapshot.ID == "" {
		t.Fatal("expected registry snapshot id")
	}
	if len(snapshot.Tools) == 0 {
		t.Fatal("expected snapshot to contain registered tools")
	}

	sources := map[navitool.ToolSource]bool{}
	var pluginFound bool
	for _, toolEntry := range snapshot.Tools {
		if toolEntry == nil {
			continue
		}
		sources[toolEntry.Source] = true
		if toolEntry.SourceID == "" {
			t.Fatalf("tool %q missing source id in snapshot", toolEntry.ToolID)
		}
		if toolEntry.ToolID == "test.plugin.echo" {
			pluginFound = true
			if toolEntry.Source != navitool.ToolSourcePlugin {
				t.Fatalf("expected plugin source for %q, got %s", toolEntry.ToolID, toolEntry.Source)
			}
			if toolEntry.SourceID != "plugin.test.echo" {
				t.Fatalf("expected plugin source id to be preserved, got %q", toolEntry.SourceID)
			}
		}
	}
	for _, expected := range []navitool.ToolSource{
		navitool.ToolSourceBuiltin,
		navitool.ToolSourceFileTools,
		navitool.ToolSourcePlugin,
		navitool.ToolSourceSelfMod,
		navitool.ToolSourceSkill,
	} {
		if !sources[expected] {
			t.Fatalf("expected snapshot to include source %s", expected)
		}
	}
	if !pluginFound {
		t.Fatal("expected plugin tool to appear in registry snapshot")
	}
}

func TestPluginToolRegistryAdapterConnectorDependenciesReachBrokerSelection(t *testing.T) {
	pluginRegistry := plugin.NewRegistry(nil)
	pluginRegistry.AddToolForPlugin("linear", llm.ToolDefinition{
		Name:        "navi.linear.create_issue",
		Description: "Create a Linear issue for the user.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})

	toolRegistry, err := buildRuntimeToolRegistry(LoopConfig{PluginRegistry: pluginRegistry})
	if err != nil {
		t.Fatalf("buildRuntimeToolRegistry: %v", err)
	}

	toolEntry, ok := toolRegistry.Lookup("navi.linear.create_issue")
	if !ok {
		t.Fatal("expected plugin tool to be registered")
	}
	if toolEntry.Source != navitool.ToolSourcePlugin {
		t.Fatalf("expected plugin source, got %s", toolEntry.Source)
	}
	if len(toolEntry.ConnectorDependencies) != 1 || toolEntry.ConnectorDependencies[0] != "linear" {
		t.Fatalf("expected connector dependency to be preserved, got %+v", toolEntry.ConnectorDependencies)
	}

	broker := navitool.NewToolBrokerFromRegistry(toolRegistry)
	baseInput := navitool.BrokerInput{
		UserInput:     "navi.linear.create_issue",
		Intent:        navitool.BrokerIntentAssistantTask,
		SessionMode:   navitool.DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: navitool.ToolAuthorityUser,
		ModelProfile: navitool.BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	}

	missing := broker.Resolve(baseInput)
	if len(missing.SelectedToolIDs) != 0 {
		t.Fatalf("expected connector-backed tool to stay suppressed without connector availability, got %+v", missing.SelectedToolIDs)
	}
	if !hasBrokerSuppression(missing.SuppressedTools, "navi.linear.create_issue", navitool.BrokerSuppressionConnectorUnavailable) {
		t.Fatalf("expected connector_unavailable suppression, got %+v", missing.SuppressedTools)
	}

	available := broker.Resolve(navitool.BrokerInput{
		UserInput:           baseInput.UserInput,
		Intent:              baseInput.Intent,
		SessionMode:         baseInput.SessionMode,
		Environment:         baseInput.Environment,
		UserAuthority:       baseInput.UserAuthority,
		ModelProfile:        baseInput.ModelProfile,
		AvailableConnectors: []string{"linear"},
	})
	if len(available.SelectedToolIDs) != 1 || available.SelectedToolIDs[0] != "navi.linear.create_issue" {
		t.Fatalf("expected connector-backed tool to become selectable when connector is available, got %+v", available.SelectedToolIDs)
	}
}

func TestDescriptionForSkillTool_DistinguishesInterfaces(t *testing.T) {
	entry := skill.SkillEntry{
		Skill: skill.Skill{
			Name:        "Core Scheduler",
			Description: "Schedule future tasks and recurring cron jobs for NAVI.",
		},
	}
	scheduleDesc := descriptionForSkillTool(entry, skill.Interface{Name: "schedule_task"})
	listDesc := descriptionForSkillTool(entry, skill.Interface{Name: "list_tasks"})
	deleteDesc := descriptionForSkillTool(entry, skill.Interface{Name: "delete_task"})

	if scheduleDesc == listDesc || scheduleDesc == deleteDesc || listDesc == deleteDesc {
		t.Fatalf("expected per-interface descriptions to differ; got identical:\n  schedule_task=%q\n  list_tasks=%q\n  delete_task=%q",
			scheduleDesc, listDesc, deleteDesc)
	}
	for name, got := range map[string]string{"schedule_task": scheduleDesc, "list_tasks": listDesc, "delete_task": deleteDesc} {
		if !strings.Contains(got, name) {
			t.Errorf("expected %q description to include the interface name %q, got %q", name, name, got)
		}
		if !strings.Contains(got, "Schedule future tasks") {
			t.Errorf("expected %q description to keep the parent skill blurb, got %q", name, got)
		}
	}
}

func TestSendReplyToolDefinition_DescriptionDoesNotAdvertiseHardCap(t *testing.T) {
	def := sendReplyToolDefinition()
	if strings.Contains(def.Description, "max 3600") {
		t.Fatalf("send_reply description still advertises the hard cap %q", def.Description)
	}
	if !strings.Contains(strings.ToLower(def.Description), "durabl") {
		t.Fatalf("send_reply description should mention durable persistence; got %q", def.Description)
	}
	props, _ := def.Parameters["properties"].(map[string]any)
	delay, _ := props["delay_seconds"].(map[string]any)
	delayDesc, _ := delay["description"].(string)
	if strings.Contains(delayDesc, "max 3600") {
		t.Fatalf("delay_seconds parameter description still advertises max 3600: %q", delayDesc)
	}
}

func TestRuntimeSystemCore_ChatBehaviorReferencesExactToolNames(t *testing.T) {
	loop := &AgentLoop{cfg: LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
	}}
	input := loop.runtimeSystemCoreInput(context.Background(), "chat-test", orchestration.ModelProfile{})
	bullets := strings.Join(input.ChatBehavior, "\n")

	for _, phrase := range []string{
		"navi.messaging.send_reply",
		"skill.core-scheduler.schedule_task",
		"durabl",
	} {
		if !strings.Contains(bullets, phrase) {
			t.Errorf("ChatBehavior bullets missing required phrase %q. Bullets:\n%s", phrase, bullets)
		}
	}
}
