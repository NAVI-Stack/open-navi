package skill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecuteMCPTool(t *testing.T) {
	// Mock MCP Bridge server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req["method"] != "tools/call" {
			t.Errorf("expected method tools/call, got %v", req["method"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"result": "echo: " + req["params"].(map[string]any)["arguments"].(map[string]any)["message"].(string),
		})
	}))
	defer server.Close()

	entry := &SkillEntry{
		Spec: &OSS27Spec{
			SkillID: "test.mcp",
			Performance: PerformanceSpec{
				TimeoutMS: 1000,
			},
		},
	}
	iface := &Interface{
		Name: "echo",
		Transport: TransportSpec{
			Type: "mcp_tool",
			MCP: &MCPTransport{
				Server: server.URL,
				Tool:   "echo",
			},
		},
	}

	raw, err := Execute(context.Background(), entry, iface, map[string]any{"message": "hello mcp"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s (%+v)", result.Status, result.Error)
	}
	payload := result.Payload.(map[string]any)
	if payload["result"] != "echo: hello mcp" {
		t.Fatalf("unexpected payload: %v", result.Payload)
	}
}

func TestExecuteMCPTool_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("bridge error"))
	}))
	defer server.Close()

	entry := &SkillEntry{
		Spec: &OSS27Spec{
			SkillID: "test.mcp",
		},
	}
	iface := &Interface{
		Name: "echo",
		Transport: TransportSpec{
			Type: "mcp_tool",
			MCP: &MCPTransport{
				Server: server.URL,
				Tool:   "echo",
			},
		},
	}

	raw, err := Execute(context.Background(), entry, iface, map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := decodeExecutionResult(t, raw)
	if result.Status != "error" {
		t.Fatalf("expected error, got %s", result.Status)
	}
	if result.Error == nil || result.Error.Type != "HTTPError" {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}
