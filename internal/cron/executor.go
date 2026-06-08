package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// DirectiveWriter is the subset of directive writes required for cron.
type DirectiveWriter interface {
	SaveDirective(ctx context.Context, d schema.Directive) error
	AppendMessage(ctx context.Context, msg schema.DirectiveMessage) error
}

// ChatAppender lets cron jobs deliver a pre-composed assistant message
// directly into a chat, without spinning up a new agent turn. This backs the
// auto-promoted send_reply path: when an in-chat delayed send would exceed
// the in-process scheduler cap, we persist it as a one-shot cron job that,
// on fire, calls AppendAssistantMessage on the configured chat id.
type ChatAppender interface {
	AppendAssistantMessage(ctx context.Context, chatID, content string) error
}

// ErrNoChatTarget signals that a scheduled job has no resolvable chat target.
// The executor surfaces this as a skipped outcome rather than fabricating a
// chat — callers must record the outcome in runtime/history only.
var ErrNoChatTarget = errors.New("cron: no chat target")

// Heartbeat wake priority values (mirror internal/navi/heartbeat.WakePriority iota ordering).
const (
	HeartbeatWakePriAction = iota
	HeartbeatWakePriDefault
	HeartbeatWakePriInterval
	HeartbeatWakePriRetry
)

// HeartbeatWaker optionally triggers immediate heartbeat cycles.
type HeartbeatWaker interface {
	RequestWake(reason string, priority int, sessionKey string)
}

func executeDispatch(ctx context.Context, w DirectiveWriter, sa ChatAppender, hb HeartbeatWaker, j Job) RunResult {
	if chatID, ok := chatIDFromTarget(j.SessionTarget); ok {
		return executeChatMessageDispatch(ctx, sa, chatID, j)
	}
	switch j.SessionTarget {
	case SessionMain:
		return executeMainDispatch(ctx, w, hb, j)
	case SessionIsolated:
		return RunResult{
			Status: RunSkipped,
			Error:  "isolated-session cron targets are not implemented yet",
		}
	default:
		return RunResult{
			Status: RunError,
			Error:  fmt.Sprintf("unknown session target %q", j.SessionTarget),
		}
	}
}

// chatIDFromTarget extracts the chat id from a legacy "session:<id>" cron target.
func chatIDFromTarget(t SessionTarget) (string, bool) {
	s := string(t)
	if !strings.HasPrefix(s, SessionTargetPrefix) {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(s, SessionTargetPrefix))
	if id == "" {
		return "", false
	}
	return id, true
}

// executeChatMessageDispatch delivers a single assistant message into the
// given chat. Requires ChatAppender to be configured at Service-init
// time; jobs created without it will be retried until the deployment wires
// one in (or removed by the operator).
func executeChatMessageDispatch(ctx context.Context, sa ChatAppender, chatID string, j Job) RunResult {
	if j.PayloadKind != PayloadAssistantMessage {
		return RunResult{
			Status: RunError,
			Error:  fmt.Sprintf("session-targeted job requires payload_kind=%q, got %q", PayloadAssistantMessage, j.PayloadKind),
		}
	}
	if strings.TrimSpace(j.PayloadText) == "" {
		return RunResult{
			Status: RunError,
			Error:  "session-targeted job requires non-empty payload_text",
		}
	}
	if sa == nil {
		// Transient: the deployment can be reconfigured to provide an
		// appender. Surfaced as RunError so the standard retry policy applies.
		return RunResult{
			Status: RunError,
			Error:  "cron: ChatAppender not configured; cannot deliver assistant message",
		}
	}
	if err := sa.AppendAssistantMessage(ctx, chatID, j.PayloadText); err != nil {
		if errors.Is(err, ErrNoChatTarget) {
			// No transcript fabrication: record runtime/history-only outcome.
			return RunResult{Status: RunSkipped, Error: err.Error()}
		}
		return RunResult{Status: RunError, Error: err.Error()}
	}
	return RunResult{Status: RunOK}
}

func executeMainDispatch(ctx context.Context, w DirectiveWriter, hb HeartbeatWaker, j Job) RunResult {
	if j.PayloadKind == PayloadAgentTurn {
		return RunResult{
			Status: RunSkipped,
			Error:  "main job payload kind agentTurn is not implemented yet; use systemEvent",
		}
	}
	if j.PayloadKind != PayloadSystemEvent || strings.TrimSpace(j.PayloadText) == "" {
		return RunResult{
			Status: RunSkipped,
			Error:  "main job requires payload kind systemEvent and non-empty payload text",
		}
	}
	d := schema.NewDirective(fmt.Sprintf("Scheduled: %s", j.Name), schema.DirectiveModeAct, "scheduler")
	if err := w.SaveDirective(ctx, d); err != nil {
		return RunResult{Status: RunError, Error: err.Error()}
	}
	msg := schema.NewDirectiveMessage(d.DirectiveID, "owner", j.PayloadText)
	if err := w.AppendMessage(ctx, msg); err != nil {
		return RunResult{Status: RunError, Error: err.Error()}
	}
	if j.WakeMode == WakeNow && hb != nil {
		hb.RequestWake(fmt.Sprintf("cron:%s (%s)", j.ID, j.Name), HeartbeatWakePriInterval, "")
	}
	return RunResult{Status: RunOK}
}
