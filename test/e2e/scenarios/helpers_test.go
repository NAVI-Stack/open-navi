//go:build e2e

package scenarios

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/test/e2e/harness"
	"github.com/stretchr/testify/require"
)

type scenarioEnv struct {
	stack *harness.Stack
	cli   *harness.CLIRunner
}

type statusSnapshot struct {
	Gateway struct {
		Reachable bool   `json:"reachable"`
		Version   string `json:"version"`
	} `json:"gateway"`
	Connectors struct {
		Total    int `json:"total"`
		Running  int `json:"running"`
		Degraded int `json:"degraded"`
		Error    int `json:"error"`
		Items    []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"items"`
	} `json:"connectors"`
	LLM struct {
		Configured bool   `json:"configured"`
		Status     string `json:"status"`
	} `json:"llm"`
	Degraded bool `json:"degraded"`
}

type connectorHealth struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Status     string `json:"status"`
	ErrorCount int64  `json:"error_count"`
}

func newScenarioEnv(t *testing.T) *scenarioEnv {
	t.Helper()
	require.NotNil(t, suiteStack, "suite stack must be initialized")
	t.Cleanup(suiteStack.AttachArtifacts(t.Name(), t.Failed))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	require.NoError(t, suiteStack.ResetAndClaim(ctx))
	cli, err := harness.NewCLIRunner(ctx, suiteStack)
	require.NoError(t, err)

	env := &scenarioEnv{
		stack: suiteStack,
		cli:   cli,
	}
	t.Cleanup(func() {
		_ = cli.Cleanup()
	})
	return env
}

func scenarioContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func fetchPublicJSON[T any](t *testing.T, url string) T {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func fetchStatus(t *testing.T, env *scenarioEnv) statusSnapshot {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	var out statusSnapshot
	require.NoError(t, env.stack.APIRequest(ctx, http.MethodGet, "/api/status", nil, &out))
	return out
}

func fetchConnectorHealth(t *testing.T, env *scenarioEnv) []connectorHealth {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	var out []connectorHealth
	require.NoError(t, env.stack.APIRequest(ctx, http.MethodGet, "/api/health/connectors", nil, &out))
	return out
}

func connectTelegram(t *testing.T, env *scenarioEnv, chatID int64) {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()

	session, err := env.cli.StartChat(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		select {
		case <-session.Done():
			return
		default:
			_, _ = session.Kill()
		}
	})

	require.NoError(t, session.WaitFor("You:", 20*time.Second))
	require.NoError(t, session.SendLine("/connect telegram"))
	require.NoError(t, session.WaitFor("Bot Token:", 10*time.Second))
	require.NoError(t, session.SendLine("fake-telegram-token"))
	require.NoError(t, session.WaitFor("Your Chat ID:", 10*time.Second))
	require.NoError(t, session.SendLine(strconv.FormatInt(chatID, 10)))
	require.NoError(t, session.WaitFor(`Connector "telegram" activated.`, 20*time.Second))

	res, err := session.CloseGracefully(15 * time.Second)
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)

	require.Eventually(t, func() bool {
		health := fetchConnectorHealth(t, env)
		for _, item := range health {
			if item.Name == "telegram" {
				return true
			}
		}
		return false
	}, 20*time.Second, 250*time.Millisecond)
}

func ollamaRequestModels(t *testing.T, env *scenarioEnv) []string {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	requests, err := env.stack.OllamaRequests(ctx)
	require.NoError(t, err)

	models := make([]string, 0, len(requests))
	for _, req := range requests {
		nested, ok := req["request"].(map[string]any)
		if !ok {
			continue
		}
		model, _ := nested["model"].(string)
		if model != "" {
			models = append(models, model)
		}
	}
	return models
}

func ollamaSawUserText(t *testing.T, env *scenarioEnv, needle string) bool {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()
	requests, err := env.stack.OllamaRequests(ctx)
	require.NoError(t, err)

	for _, req := range requests {
		nested, ok := req["request"].(map[string]any)
		if !ok {
			continue
		}
		messages, ok := nested["messages"].([]any)
		if !ok {
			continue
		}
		for _, raw := range messages {
			msg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			content, _ := msg["content"].(string)
			if strings.Contains(content, needle) {
				return true
			}
		}
	}
	return false
}
