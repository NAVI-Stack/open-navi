package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	"github.com/open-navi/navi/internal/navi/plugin"
	"github.com/open-navi/navi/internal/navi/selfmod"
	"github.com/open-navi/navi/internal/navi/skill"
)

// ExecutorFunc adapts a function into a ToolExecutor.
type ExecutorFunc func(ctx context.Context, args map[string]any) (ToolResult, error)

// Execute implements ToolExecutor.
func (f ExecutorFunc) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return f(ctx, args)
}

// NewFileToolExecutor wraps the existing file tool execution path.
func NewFileToolExecutor(name, workspaceDir string, checker filetools.PathChecker) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		out, err := filetools.Execute(ctx, name, args, workspaceDir, checker)
		return ToolResult{Content: out}, err
	})
}

// NewSkillToolExecutor wraps the existing skill execution path.
func NewSkillToolExecutor(entry *skill.SkillEntry, iface *skill.Interface) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		out, err := skill.Execute(ctx, entry, iface, args)
		return ToolResult{Content: out}, err
	})
}

// NewSelfModToolExecutor wraps the existing selfmod execution path.
func NewSelfModToolExecutor(name string, exec *selfmod.Executor) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		out, err := selfmod.Execute(ctx, name, args, exec)
		return ToolResult{Content: out}, err
	})
}

// RouterLLMService provides access to provider state for tools.
type RouterLLMService interface {
	Catalog(ctx context.Context) (llm.LLMCatalog, error)
	GetActive(ctx context.Context) (llm.Active, error)
	SetActive(ctx context.Context, provider, model string) (llm.Active, error)
}

// NewRouterToolExecutor wraps the current llm-router builtin behavior.
func NewRouterToolExecutor(name string, svc RouterLLMService) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		switch name {
		case "llm-router_list", "navi.llm.router.list":
			if svc == nil {
				return ToolResult{}, fmt.Errorf("llm router list is not configured")
			}
			out, err := svc.Catalog(ctx)
			return ToolResult{Content: out}, err
		case "llm-router_get_active", "navi.llm.router.get_active":
			if svc == nil {
				return ToolResult{}, fmt.Errorf("llm router get_active is not configured")
			}
			active, err := svc.GetActive(ctx)
			if err != nil {
				return ToolResult{}, err
			}
			return ToolResult{Content: map[string]any{"provider": active.Provider, "model": active.Model}}, nil
		case "llm-router_set_active", "navi.llm.router.set_active":
			if svc == nil {
				return ToolResult{}, fmt.Errorf("llm router set_active is not configured")
			}
			provider, _ := args["provider"].(string)
			model, _ := args["model"].(string)
			active, err := svc.SetActive(ctx, provider, model)
			if err != nil {
				return ToolResult{}, err
			}
			return ToolResult{Content: map[string]any{"provider": active.Provider, "model": active.Model}}, nil
		default:
			return ToolResult{}, fmt.Errorf("unknown router tool %q", name)
		}
	})
}

// NewSetTimezoneExecutor returns an executor for setting the owner timezone.
func NewSetTimezoneExecutor(fn func(ctx context.Context, tz string) error) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		tz, _ := args["timezone"].(string)
		if tz == "" {
			return ToolResult{}, fmt.Errorf("timezone argument is required")
		}
		if fn != nil {
			if err := fn(ctx, tz); err != nil {
				return ToolResult{}, err
			}
		}
		return ToolResult{Content: fmt.Sprintf("Timezone set to %s.", tz)}, nil
	})
}

// NewBuiltinPluginExecutor wraps the currently executable builtin plugin subset.
func NewBuiltinPluginExecutor(name string, tzProvider func(ctx context.Context) string) ToolExecutor {
	return ExecutorFunc(func(ctx context.Context, args map[string]any) (ToolResult, error) {
		if !plugin.IsBuiltinExecutable(name) {
			return ToolResult{}, fmt.Errorf("plugin tool %q has no executor", name)
		}
		switch name {
		case plugin.CalendarTimeWindowToolName:
			tzName := "UTC"
			if tzProvider != nil {
				tzName = tzProvider(ctx)
			}
			loc, err := time.LoadLocation(tzName)
			if err != nil {
				loc = time.UTC
			}
			now := time.Now().In(loc)
			return ToolResult{Content: map[string]any{
				"start_utc":  now.UTC().Format(time.RFC3339),
				"end_utc":    now.UTC().Add(time.Hour).Format(time.RFC3339),
				"local_time": now.Format(time.RFC3339),
				"timezone":   tzName,
			}}, nil
		default:
			return ToolResult{}, fmt.Errorf("plugin tool %q has no executor", name)
		}
	})
}

// CloneDefinitions copies a tool definition list.
func CloneDefinitions(defs []llm.ToolDefinition) []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, len(defs))
	copy(out, defs)
	return out
}
