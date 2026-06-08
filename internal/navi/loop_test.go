package navi

import (
	"strings"
	"testing"
)

func TestIsRouterStateTool(t *testing.T) {
	for _, name := range []string{"llm-router_list", "llm-router_get_active", "llm-router_set_active"} {
		if !isRouterStateTool(name) {
			t.Errorf("expected isRouterStateTool(%q) true", name)
		}
	}
	for _, name := range []string{"llm-router_other", "file_read"} {
		if isRouterStateTool(name) {
			t.Errorf("expected isRouterStateTool(%q) false", name)
		}
	}
}

func TestFormatRouterStructuredReply(t *testing.T) {
	t.Run("get_active_success", func(t *testing.T) {
		results := []struct{ Name, Content string }{
			{routerToolGetActive, `{"provider":"anthropic","model":"claude-3.5-sonnet"}`},
		}
		out := formatRouterStructuredReply(results)
		if !strings.Contains(out, "Active provider: anthropic") || !strings.Contains(out, "Active model: claude-3.5-sonnet") {
			t.Errorf("expected active provider/model in output: %s", out)
		}
	})
	t.Run("set_active_success", func(t *testing.T) {
		results := []struct{ Name, Content string }{
			{routerToolSetActive, `{"provider":"ollama","model":"llama3"}`},
		}
		out := formatRouterStructuredReply(results)
		if !strings.Contains(strings.ToLower(out), "ollama") || !strings.Contains(strings.ToLower(out), "llama3") {
			t.Errorf("expected switch confirmation referencing provider/model in output: %s", out)
		}
	})
	t.Run("failure", func(t *testing.T) {
		results := []struct{ Name, Content string }{
			{routerToolList, "Error: no provider configured"},
		}
		out := formatRouterStructuredReply(results)
		if !strings.Contains(out, "Could not retrieve provider/model state") || !strings.Contains(out, "no provider configured") {
			t.Errorf("expected error message in output: %s", out)
		}
	})
}
