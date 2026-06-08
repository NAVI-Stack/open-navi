//go:build e2e

package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/test/e2e/harness"
	"github.com/stretchr/testify/require"
)

func waitForTelegramOutboundText(t *testing.T, env *scenarioEnv, needle string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()

	require.Eventually(t, func() bool {
		outbound, err := env.stack.TelegramOutbound(ctx)
		require.NoError(t, err)
		for _, item := range outbound {
			text, _ := item["text"].(string)
			if strings.Contains(text, needle) {
				return true
			}
		}
		return false
	}, timeout, 500*time.Millisecond)
}

func waitForNavidLog(t *testing.T, env *scenarioEnv, needle string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := scenarioContext(t)
	defer cancel()

	require.Eventually(t, func() bool {
		logs, err := env.stack.Logs(ctx, "navid")
		require.NoError(t, err)
		return strings.Contains(logs, needle)
	}, timeout, 500*time.Millisecond)
}

func TestE2E_ConnectorStatus_Degraded(t *testing.T) {
	env := newScenarioEnv(t)
	connectTelegram(t, env, 44001)

	ctx, cancel := scenarioContext(t)
	defer cancel()
	require.NoError(t, env.stack.SetTelegramMode(ctx, harness.FakeTelegramMode{
		HealthStatus: 503,
	}))

	require.Eventually(t, func() bool {
		health := fetchConnectorHealth(t, env)
		for _, item := range health {
			if item.Name == "telegram" && item.Status == "down" && item.State == "error" {
				return true
			}
		}
		return false
	}, 20*time.Second, 500*time.Millisecond)
}

func TestE2E_Telegram_SendReceive(t *testing.T) {
	env := newScenarioEnv(t)
	connectTelegram(t, env, 44002)

	ctx, cancel := scenarioContext(t)
	defer cancel()

	require.NoError(t, env.stack.TelegramInjectMessage(ctx, harness.FakeTelegramInbound{
		ChatID: 44002,
		Text:   "/navi",
	}))
	waitForTelegramOutboundText(t, env, "NAVI Session Started", 20*time.Second)
	waitForNavidLog(t, env, "telegram: connected to /ws/live for NAVI stream", 20*time.Second)

	require.NoError(t, env.stack.TelegramInjectMessage(ctx, harness.FakeTelegramInbound{
		ChatID: 44002,
		Text:   "hello from fake telegram",
	}))

	waitForTelegramOutboundText(t, env, "hello from fake telegram", 30*time.Second)

	require.Eventually(t, func() bool {
		return ollamaSawUserText(t, env, "hello from fake telegram")
	}, 20*time.Second, 500*time.Millisecond)
}

func TestE2E_ConnectorFailureIsolation(t *testing.T) {
	env := newScenarioEnv(t)
	connectTelegram(t, env, 44003)

	ctx, cancel := scenarioContext(t)
	defer cancel()

	require.NoError(t, env.stack.TelegramInjectMessage(ctx, harness.FakeTelegramInbound{
		ChatID: 44003,
		Text:   "/navi",
	}))
	waitForTelegramOutboundText(t, env, "NAVI Session Started", 20*time.Second)
	waitForNavidLog(t, env, "telegram: connected to /ws/live for NAVI stream", 20*time.Second)

	require.NoError(t, env.stack.SetTelegramMode(ctx, harness.FakeTelegramMode{
		SendStatus: 500,
	}))
	require.NoError(t, env.stack.TelegramInjectMessage(ctx, harness.FakeTelegramInbound{
		ChatID: 44003,
		Text:   "this should trigger a connector send failure",
	}))

	require.Eventually(t, func() bool {
		requests, err := env.stack.TelegramRequests(ctx)
		require.NoError(t, err)
		for _, item := range requests {
			action, _ := item["action"].(string)
			if action != "sendMessage" && action != "editMessageText" {
				continue
			}
			body, _ := item["body"].(map[string]any)
			text, _ := body["text"].(string)
			if strings.Contains(text, "this should trigger a connector send failure") {
				return true
			}
		}
		return false
	}, 30*time.Second, 500*time.Millisecond)

	askRes, err := env.cli.Ask(ctx, "cli still works after telegram failures")
	require.NoError(t, err)
	require.Equal(t, 0, askRes.ExitCode)
	require.Contains(t, askRes.Stdout, "cli still works after telegram failures")
}
