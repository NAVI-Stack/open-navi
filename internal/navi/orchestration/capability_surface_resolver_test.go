package orchestration

import (
	"reflect"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestCapabilitySurfaceResolverIncludesToolsInRegistryOrder(t *testing.T) {
	registry := newResolverTestRegistry(t,
		resolverTestTool("alpha"),
		resolverTestTool("beta", func(tool *navitool.Tool) {
			tool.VisibleOn = []string{CapabilitySurfaceRuntime}
		}),
	)

	result := NewCapabilitySurfaceResolver(registry).Resolve(SurfaceResolutionInput{
		Surface:       CapabilitySurface{Surface: CapabilitySurfaceRuntime, SelectionReason: "runtime policy"},
		ExecutionMode: ExecutionModeRunExecute,
		Model:         ModelProfile{SupportsTools: true},
	})

	if result.IsGuarded() {
		t.Fatalf("expected unguarded result, got %+v", result)
	}
	if result.Resolved.Surface != CapabilitySurfaceRuntime {
		t.Fatalf("resolved surface = %q, want %q", result.Resolved.Surface, CapabilitySurfaceRuntime)
	}
	if result.Resolved.SelectionReason != "runtime policy" {
		t.Fatalf("resolved selection reason = %q", result.Resolved.SelectionReason)
	}
	if len(result.Resolved.ToolNames) != 2 || result.Resolved.ToolNames[0] != resolverToolID("alpha") || result.Resolved.ToolNames[1] != resolverToolID("beta") {
		t.Fatalf("resolved tool names = %#v, want registry order", result.Resolved.ToolNames)
	}
}

func TestCapabilitySurfaceResolverExplainsStructuredExclusions(t *testing.T) {
	registry := newResolverTestRegistry(t,
		resolverTestTool("visible"),
		resolverTestTool("hidden", func(tool *navitool.Tool) {
			tool.Hidden = true
		}),
		resolverTestTool("internal", func(tool *navitool.Tool) {
			tool.Metadata.ExposureClass = navitool.ToolExposureInternal
		}),
		resolverTestTool("dev", func(tool *navitool.Tool) {
			tool.Metadata.ExposureClass = navitool.ToolExposureDevelopment
		}),
		resolverTestTool("test", func(tool *navitool.Tool) {
			tool.Metadata.ExposureClass = navitool.ToolExposureTest
		}),
		resolverTestTool("runtime_only", func(tool *navitool.Tool) {
			tool.VisibleOn = []string{CapabilitySurfaceRuntime}
		}),
		resolverTestTool("action_only", func(tool *navitool.Tool) {
			tool.Metadata.InteractionModes = []navitool.ToolInteractionMode{navitool.ToolInteractionAction}
		}),
		resolverTestTool("broken", func(tool *navitool.Tool) {
			tool.Metadata.ExposureClass = navitool.ToolExposureClass("mystery")
		}),
	)

	result := NewCapabilitySurfaceResolver(registry).Resolve(SurfaceResolutionInput{
		Surface:       CapabilitySurface{Surface: CapabilitySurfaceLoop},
		ExecutionMode: ExecutionModeChatTurn,
		Model:         ModelProfile{SupportsTools: true},
	})

	if result.IsGuarded() {
		t.Fatalf("expected unguarded result, got %+v", result)
	}
	if len(result.Resolved.ToolNames) != 1 || result.Resolved.ToolNames[0] != resolverToolID("visible") {
		t.Fatalf("resolved tool names = %#v, want only visible", result.Resolved.ToolNames)
	}

	assertResolverExclusion(t, result.Exclusions, resolverToolID("hidden"), SurfaceExclusionHidden)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("internal"), SurfaceExclusionInternal)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("dev"), SurfaceExclusionDevelopmentOnly)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("test"), SurfaceExclusionTestOnly)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("runtime_only"), SurfaceExclusionSurfaceMismatch)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("action_only"), SurfaceExclusionActionOnly)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("broken"), SurfaceExclusionInvalidMetadata)
}

func TestCapabilitySurfaceResolverExplainsConversationOnlyExclusionOnActionRuns(t *testing.T) {
	registry := newResolverTestRegistry(t,
		resolverTestTool("conversation_only", func(tool *navitool.Tool) {
			tool.VisibleOn = []string{CapabilitySurfaceRuntime}
			tool.Metadata.InteractionModes = []navitool.ToolInteractionMode{navitool.ToolInteractionConversation}
		}),
	)

	result := NewCapabilitySurfaceResolver(registry).Resolve(SurfaceResolutionInput{
		Surface:       CapabilitySurface{Surface: CapabilitySurfaceRuntime},
		ExecutionMode: ExecutionModeRunExecute,
		Model:         ModelProfile{SupportsTools: true},
	})

	if result.IsGuarded() {
		t.Fatalf("expected unguarded result, got %+v", result)
	}
	if len(result.Resolved.ToolNames) != 0 {
		t.Fatalf("resolved tool names = %#v, want empty set", result.Resolved.ToolNames)
	}
	assertResolverExclusion(t, result.Exclusions, resolverToolID("conversation_only"), SurfaceExclusionConversationOnly)
}

func TestCapabilitySurfaceResolverHandlesExplicitRequestedSubset(t *testing.T) {
	registry := newResolverTestRegistry(t,
		resolverTestTool("alpha"),
		resolverTestTool("beta"),
	)

	result := NewCapabilitySurfaceResolver(registry).Resolve(SurfaceResolutionInput{
		Surface: CapabilitySurface{
			Surface:   CapabilitySurfaceLoop,
			ToolNames: []string{resolverToolID("beta"), resolverToolID("beta"), "missing"},
		},
		ExecutionMode: ExecutionModeChatTurn,
		Model:         ModelProfile{SupportsTools: true},
	})

	if len(result.Resolved.ToolNames) != 1 || result.Resolved.ToolNames[0] != resolverToolID("beta") {
		t.Fatalf("resolved tool names = %#v, want only explicit beta", result.Resolved.ToolNames)
	}
	assertResolverExclusion(t, result.Exclusions, resolverToolID("alpha"), SurfaceExclusionNotRequested)
	assertResolverExclusion(t, result.Exclusions, resolverToolID("beta"), SurfaceExclusionDuplicate)
	assertResolverExclusion(t, result.Exclusions, "missing", SurfaceExclusionUnknownTool)
}

func TestCapabilitySurfaceResolverGuardsInvalidAndEmptySurfaces(t *testing.T) {
	registry := newResolverTestRegistry(t, resolverTestTool("alpha"))
	resolver := NewCapabilitySurfaceResolver(registry)

	invalid := resolver.Resolve(SurfaceResolutionInput{
		Surface: CapabilitySurface{Surface: "mystery"},
		Model:   ModelProfile{SupportsTools: true},
	})
	if invalid.Guard != SurfaceGuardInvalidSurface {
		t.Fatalf("invalid guard = %q, want %q", invalid.Guard, SurfaceGuardInvalidSurface)
	}
	if invalid.GuardReason == "" {
		t.Fatal("expected invalid surface guard reason")
	}

	unavailable := NewCapabilitySurfaceResolver(nil).Resolve(SurfaceResolutionInput{
		Surface: CapabilitySurface{Surface: CapabilitySurfaceLoop},
		Model:   ModelProfile{SupportsTools: true},
	})
	if unavailable.Guard != SurfaceGuardInvalidSurface {
		t.Fatalf("registry-unavailable guard = %q, want %q", unavailable.Guard, SurfaceGuardInvalidSurface)
	}
	if unavailable.GuardReason == "" {
		t.Fatal("expected registry-unavailable guard reason")
	}

	empty := resolver.Resolve(SurfaceResolutionInput{
		Surface: CapabilitySurface{
			Surface:   CapabilitySurfaceLoop,
			ToolNames: []string{"missing"},
		},
		ExecutionMode: ExecutionModeChatTurn,
		Model:         ModelProfile{SupportsTools: true},
	})
	if empty.IsGuarded() {
		t.Fatalf("expected pure resolver result to remain unguarded for empty surface, got %+v", empty)
	}
	if !empty.ExpectedTools {
		t.Fatal("expected empty resolution with explicit requested tools to mark tools expected")
	}
	assertResolverExclusion(t, empty.Exclusions, "missing", SurfaceExclusionUnknownTool)

	invalidRequested := resolver.Resolve(SurfaceResolutionInput{
		Surface: CapabilitySurface{
			Surface:   CapabilitySurfaceLoop,
			ToolNames: []string{" "},
		},
		Model: ModelProfile{SupportsTools: true},
	})
	if invalidRequested.Guard != SurfaceGuardInvalidSurface {
		t.Fatalf("invalid requested-name guard = %q, want %q", invalidRequested.Guard, SurfaceGuardInvalidSurface)
	}
	if invalidRequested.GuardReason == "" {
		t.Fatal("expected invalid requested-name guard reason")
	}
}

func TestCapabilitySurfaceResolverGuardsToolCapableModelRequirement(t *testing.T) {
	registry := newResolverTestRegistry(t, resolverTestTool("alpha"))

	result := NewCapabilitySurfaceResolver(registry).Resolve(SurfaceResolutionInput{
		Surface:       CapabilitySurface{Surface: CapabilitySurfaceRuntime},
		ExecutionMode: ExecutionModeRunExecute,
		Model:         ModelProfile{SupportsTools: false},
		RequiredOutput: RequiredOutput{
			AllowToolCalls: true,
		},
	})

	if result.IsGuarded() {
		t.Fatalf("expected pure resolver result to remain unguarded before guard policy, got %+v", result)
	}
	if !result.RequiresToolCapableModel {
		t.Fatal("expected resolver to record tool-capable-model requirement")
	}
	if len(result.Resolved.ToolNames) != 1 || result.Resolved.ToolNames[0] != resolverToolID("alpha") {
		t.Fatalf("resolver should preserve surfaced tools before guard policy, got %#v", result.Resolved.ToolNames)
	}
}

func TestCapabilitySurfaceResolverIsDeterministic(t *testing.T) {
	registry := newResolverTestRegistry(t,
		resolverTestTool("alpha"),
		resolverTestTool("beta", func(tool *navitool.Tool) {
			tool.VisibleOn = []string{CapabilitySurfaceRuntime}
		}),
		resolverTestTool("hidden", func(tool *navitool.Tool) {
			tool.Hidden = true
			tool.VisibleOn = []string{CapabilitySurfaceRuntime}
		}),
	)
	resolver := NewCapabilitySurfaceResolver(registry)
	input := SurfaceResolutionInput{
		Surface:       CapabilitySurface{Surface: CapabilitySurfaceRuntime, SelectionReason: "deterministic test"},
		ExecutionMode: ExecutionModeRunExecute,
		Model:         ModelProfile{SupportsTools: true},
	}

	first := resolver.Resolve(input)
	second := resolver.Resolve(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("resolver output is not deterministic:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func newResolverTestRegistry(t *testing.T, tools ...*navitool.Tool) *navitool.Registry {
	t.Helper()
	registry := navitool.NewRegistry()
	for _, toolEntry := range tools {
		if err := registry.Register(toolEntry); err != nil {
			t.Fatalf("register %q: %v", toolEntry.Name, err)
		}
	}
	return registry
}

func resolverTestTool(name string, opts ...func(*navitool.Tool)) *navitool.Tool {
	id := resolverToolID(name)
	toolEntry := &navitool.Tool{
		ToolID:                id,
		DisplayName:           name,
		Description:           "Resolver test tool.",
		Source:                navitool.ToolSourceBuiltin,
		SourceID:              "tests.resolver",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{"resolver"},
		Status:                navitool.ToolStatusActive,
		Definition:            llm.ToolDefinition{Name: id, Description: "Resolver test tool.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
	}
	for _, opt := range opts {
		opt(toolEntry)
	}
	return toolEntry
}

func resolverToolID(name string) string {
	return "test.resolver." + name
}

func assertResolverExclusion(t *testing.T, exclusions []SurfaceExclusion, toolName string, reason SurfaceExclusionReason) {
	t.Helper()
	for _, exclusion := range exclusions {
		if exclusion.ToolName == toolName && exclusion.Reason == reason {
			return
		}
	}
	t.Fatalf("expected exclusion %q for tool %q, got %+v", reason, toolName, exclusions)
}
