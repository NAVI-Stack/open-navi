//go:build e2e

package scenarios

import (
	"net/http"
	"testing"
	"time"

	"github.com/ceoai/navi/test/e2e/harness"
	"github.com/stretchr/testify/require"
)

// TestE2E_DataDrivenRender_ToolUsage exercises the real chat path end to end:
// the deterministic render short-circuit must classify a tool-usage visualization
// request and attach an OpenUI render payload to the assistant message metadata,
// while an ordinary "which tools did you use" question must NOT.
func TestE2E_DataDrivenRender_ToolUsage(t *testing.T) {
	env := newScenarioEnv(t)

	t.Run("visualize request attaches an openui render payload", func(t *testing.T) {
		chatID := createChat(t, env, "render slice")
		sendChatMessage(t, env, chatID, "show me a graph of tool usage in this chat")

		payload := eventuallyRenderPayload(t, env, chatID)
		require.Equal(t, "openui", payload["mode"], "render payload mode should be openui")
		require.NotEmpty(t, payload["openuiLang"], "openui lang program should be present")
		require.NotEmpty(t, payload["fallbackMarkdown"], "fallback markdown must always be present")
		require.NotNil(t, payload["dataView"], "canonical data view must be carried")
	})

	t.Run("plain tool question does not render", func(t *testing.T) {
		chatID := createChat(t, env, "plain question")
		sendChatMessage(t, env, chatID, "which tools did you use in this chat?")

		// Give the run time to persist an assistant reply, then assert no payload.
		require.Eventually(t, func() bool {
			return assistantMessageCount(t, env, chatID) > 0
		}, 60*time.Second, 500*time.Millisecond, "expected an assistant reply")
		require.Nil(t, latestRenderPayload(t, env, chatID),
			"a plain tool question must not produce a render payload")
	})

	t.Run("visualize request renders real persisted tool usage rows", func(t *testing.T) {
		ctx, cancel := scenarioContext(t)
		defer cancel()
		require.NoError(t, env.stack.SetOllamaScenario(ctx, harness.FakeOllamaScenario{
			Mode:     "tool-call",
			ToolName: "navi.files.list",
		}))

		chatID := createChat(t, env, "render with tool usage")
		sendChatMessage(t, env, chatID, "list files in the workspace")
		require.Eventually(t, func() bool {
			return latestToolCount(t, env, chatID, "navi.files.list") > 0
		}, 60*time.Second, 500*time.Millisecond, "expected a real persisted file-list tool invocation")

		require.NoError(t, env.stack.SetOllamaScenario(ctx, harness.FakeOllamaScenario{Mode: "happy"}))
		sendChatMessage(t, env, chatID, "show me a table of tool usage in this chat")

		payload := eventuallyRenderPayload(t, env, chatID)
		require.Equal(t, "openui", payload["mode"], "render payload mode should be openui")
		require.NotEmpty(t, payload["openuiLang"], "openui lang program should be present")
		require.NotEmpty(t, payload["fallbackMarkdown"], "fallback markdown must always be present")
		dataView, ok := payload["dataView"].(map[string]any)
		require.True(t, ok, "canonical data view must be carried")
		dataset, ok := dataView["dataset"].(map[string]any)
		require.True(t, ok, "data view dataset must be present")
		rows, ok := dataset["rows"].([]any)
		require.True(t, ok, "data view rows must be an array")
		require.NotEmpty(t, rows, "tool usage render must include rows from prior tool usage")

		row, ok := rows[0].(map[string]any)
		require.True(t, ok, "row must be an object")
		require.Equal(t, "navi.files.list", row["tool"])
		require.GreaterOrEqual(t, rowNumber(t, row, "count"), 1)
	})
}

// TestE2E_PrototypeRender exercises the second render lane end to end: an explicit
// "create a mockup" request must classify as a prototype render and attach an OpenUI
// render payload carrying clearly-labeled PLACEHOLDER data, while an ordinary
// real-data weather question must NOT produce a prototype.
func TestE2E_PrototypeRender(t *testing.T) {
	env := newScenarioEnv(t)

	t.Run("dashboard mockup request attaches a placeholder prototype payload", func(t *testing.T) {
		chatID := createChat(t, env, "prototype slice")
		sendChatMessage(t, env, chatID, "Create a dashboard mockup for connector health.")

		payload := eventuallyRenderPayload(t, env, chatID)
		require.Equal(t, "openui", payload["mode"], "prototype render payload mode should be openui")
		require.NotEmpty(t, payload["openuiLang"], "openui lang program should be present")
		require.NotEmpty(t, payload["fallbackMarkdown"], "fallback markdown must always be present")
		require.Equal(t, "placeholder", payload["dataSourceKind"], "prototype data must be labeled placeholder")
		require.Nil(t, payload["dataView"], "a prototype carries no canonical telemetry data view")
		proto, ok := payload["prototype"].(map[string]any)
		require.True(t, ok, "canonical prototype model must be carried")
		require.Equal(t, "Connector Health Dashboard", proto["title"])
		require.Equal(t, "placeholder", proto["dataSourceKind"])
	})

	t.Run("ordinary weather question does not render a prototype", func(t *testing.T) {
		chatID := createChat(t, env, "real weather question")
		sendChatMessage(t, env, chatID, "Show me the weather for this week.")

		require.Eventually(t, func() bool {
			return assistantMessageCount(t, env, chatID) > 0
		}, 60*time.Second, 500*time.Millisecond, "expected an assistant reply")
		require.Nil(t, latestRenderPayload(t, env, chatID),
			"a real-data weather request must not produce a placeholder prototype")
	})
}

func createChat(t *testing.T, env *scenarioEnv, title string) string {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, env.stack.APIRequest(ctx, http.MethodPost, "/api/navi/chats",
		map[string]any{"content": title}, &out))
	require.NotEmpty(t, out.ID, "chat id should be returned")
	return out.ID
}

func sendChatMessage(t *testing.T, env *scenarioEnv, chatID, content string) {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	// The reply may stream asynchronously; ignore the immediate body and poll.
	_ = env.stack.APIRequest(ctx, http.MethodPost, "/api/navi/chats/"+chatID+"/message",
		map[string]any{"content": content}, nil)
}

type chatThreadResponse struct {
	Messages []struct {
		Role     string         `json:"role"`
		Content  string         `json:"content"`
		Metadata map[string]any `json:"metadata"`
	} `json:"messages"`
}

func fetchThread(t *testing.T, env *scenarioEnv, chatID string) chatThreadResponse {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	var out chatThreadResponse
	require.NoError(t, env.stack.APIRequest(ctx, http.MethodGet, "/api/navi/chats/"+chatID, nil, &out))
	return out
}

func assistantMessageCount(t *testing.T, env *scenarioEnv, chatID string) int {
	t.Helper()
	n := 0
	for _, m := range fetchThread(t, env, chatID).Messages {
		if m.Role == "assistant" || m.Role == "navi" {
			n++
		}
	}
	return n
}

func latestRenderPayload(t *testing.T, env *scenarioEnv, chatID string) map[string]any {
	t.Helper()
	thread := fetchThread(t, env, chatID)
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		m := thread.Messages[i]
		if m.Metadata == nil {
			continue
		}
		if rp, ok := m.Metadata["renderPayload"].(map[string]any); ok {
			return rp
		}
	}
	return nil
}

func eventuallyRenderPayload(t *testing.T, env *scenarioEnv, chatID string) map[string]any {
	t.Helper()
	var payload map[string]any
	require.Eventually(t, func() bool {
		payload = latestRenderPayload(t, env, chatID)
		return payload != nil
	}, 60*time.Second, 500*time.Millisecond, "expected an assistant message carrying a render payload")
	return payload
}

func latestToolCount(t *testing.T, env *scenarioEnv, chatID, toolName string) int {
	t.Helper()
	thread := fetchThread(t, env, chatID)
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		rawParts, ok := thread.Messages[i].Metadata["toolParts"].([]any)
		if !ok {
			continue
		}
		count := 0
		for _, raw := range rawParts {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if part["toolName"] == toolName {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	return 0
}

func rowNumber(t *testing.T, row map[string]any, key string) float64 {
	t.Helper()
	n, ok := row[key].(float64)
	require.True(t, ok, "row[%q] must be numeric, got %#v", key, row[key])
	return n
}
