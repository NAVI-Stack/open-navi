//go:build e2e

package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_ContainerStartupHealth(t *testing.T) {
	env := newScenarioEnv(t)

	health := fetchPublicJSON[map[string]any](t, env.stack.GatewayURL()+"/health")
	require.Equal(t, "ok", health["status"])

	onboarding := fetchPublicJSON[map[string]any](t, env.stack.GatewayURL()+"/api/onboarding/status")
	require.Equal(t, true, onboarding["claimed"])

	status := fetchStatus(t, env)
	require.True(t, status.Gateway.Reachable)
	require.True(t, status.LLM.Configured)
	require.Equal(t, "ollama/llama3:latest", status.LLM.Status)
	require.False(t, status.Degraded)

	ctx, cancel := scenarioContext(t)
	defer cancel()
	logs, err := env.stack.Logs(ctx, "navid")
	require.NoError(t, err)
	require.Contains(t, logs, "Starting NAVI Agent")
}

func TestE2E_CLIChat_SimpleMessage(t *testing.T) {
	env := newScenarioEnv(t)
	ctx, cancel := scenarioContext(t)
	defer cancel()

	res, err := env.cli.Ask(ctx, "hello from the e2e cli")
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "NAVI: ")
	require.Contains(t, res.Stdout, "hello from the e2e cli")
	require.Empty(t, strings.TrimSpace(res.Stderr))
}

func TestE2E_CLIChat_MultiTurnAndCommands(t *testing.T) {
	env := newScenarioEnv(t)
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
	require.NoError(t, session.SendLine("first multi-turn message"))
	require.NoError(t, session.WaitFor("fake-ollama(llama3:latest): first multi-turn message", 30*time.Second))
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, session.SendLine("second multi-turn message"))
	require.NoError(t, session.WaitFor("fake-ollama(llama3:latest): second multi-turn message", 30*time.Second))
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, session.SendLine("/status"))
	require.NoError(t, session.WaitFor("Gateway:", 15*time.Second))
	require.NoError(t, session.SendLine("/sessions"))
	require.NoError(t, session.WaitFor("Sessions:", 15*time.Second))
	require.NoError(t, session.SendLine("/debug"))
	require.NoError(t, session.WaitFor("Debug mode enabled", 15*time.Second))

	res, err := session.CloseGracefully(15 * time.Second)
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "Bye!")
}

func TestE2E_Models_ListAndChange(t *testing.T) {
	env := newScenarioEnv(t)
	ctx, cancel := scenarioContext(t)
	defer cancel()

	listRes, err := env.cli.Run(ctx, "models", "-json", "list")
	require.NoError(t, err)
	require.Equal(t, 0, listRes.ExitCode)
	require.Contains(t, listRes.Stdout, `"key":"ollama"`)
	require.Contains(t, listRes.Stdout, `"name":"llama3:latest"`)
	require.Contains(t, listRes.Stdout, `"name":"mistral:latest"`)

	activeRes, err := env.cli.Run(ctx, "models", "active")
	require.NoError(t, err)
	require.Equal(t, 0, activeRes.ExitCode)
	require.Contains(t, activeRes.Stdout, "ollama/llama3:latest")

	setRes, err := env.cli.Run(ctx, "models", "set", "ollama", "mistral")
	require.NoError(t, err)
	require.Equal(t, 0, setRes.ExitCode)
	require.Contains(t, setRes.Stdout, "Active model set to ollama/mistral:latest")

	status := fetchStatus(t, env)
	require.Equal(t, "ollama/mistral:latest", status.LLM.Status)

	askRes, err := env.cli.Ask(ctx, "confirm the active model")
	require.NoError(t, err)
	require.Equal(t, 0, askRes.ExitCode)
	require.Contains(t, askRes.Stdout, "confirm the active model")
	require.Contains(t, ollamaRequestModels(t, env), "mistral:latest")

	readOnlyKey, err := env.stack.NewAPIKey(ctx, "readonly-models", []string{"read"})
	require.NoError(t, err)
	blockedRes, err := env.cli.RunWithAPIKey(ctx, readOnlyKey, "models", "set", "ollama", "llama3:latest")
	require.NoError(t, err)
	require.Equal(t, 3, blockedRes.ExitCode)
	require.Equal(t, "ollama/mistral:latest", fetchStatus(t, env).LLM.Status)
}

func TestE2E_Streaming_IncrementalOutput(t *testing.T) {
	env := newScenarioEnv(t)
	ctx, cancel := scenarioContext(t)
	defer cancel()

	require.NoError(t, env.stack.SetOllamaScenario(ctx, harness.FakeOllamaScenario{
		Mode:    "happy",
		Message: "chunk-one chunk-two chunk-three",
		Chunks:  []string{"chunk-one ", "chunk-two ", "chunk-three"},
		DelayMS: 250,
	}))

	res, err := env.cli.Ask(ctx, "stream this response")
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "chunk-one chunk-two chunk-three")
	require.GreaterOrEqual(t, len(res.Transcript), 2)

	var chunkTimes []time.Duration
	for _, chunk := range res.Transcript {
		if chunk.Stream != "stdout" {
			continue
		}
		if strings.Contains(chunk.Text, "chunk-one") || strings.Contains(chunk.Text, "chunk-two") || strings.Contains(chunk.Text, "chunk-three") {
			chunkTimes = append(chunkTimes, chunk.At)
		}
	}
	require.GreaterOrEqual(t, len(chunkTimes), 2, "expected multiple timed stdout chunks for stream")
	assert.GreaterOrEqual(t, chunkTimes[len(chunkTimes)-1]-chunkTimes[0], 200*time.Millisecond)
	assert.GreaterOrEqual(t, res.Duration, 400*time.Millisecond)
}
