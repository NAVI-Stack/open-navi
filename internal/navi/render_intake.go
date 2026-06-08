package navi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/navi/render"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

// renderToolSink carries the run context the model-callable render tool
// (navi.render.visualize) needs to attach a data-driven render payload to the
// current reply. It is threaded through the execution context by the runtime
// (mirroring send_reply's scheduled-messages sink) because the registry executor
// has no direct handle on the active run.
type renderToolSink struct {
	ChatID  string
	RunID   string
	Payload *json.RawMessage
}

type renderToolSinkContextKey struct{}

func withRenderToolSink(ctx context.Context, sink *renderToolSink) context.Context {
	return context.WithValue(ctx, renderToolSinkContextKey{}, sink)
}

func renderToolSinkFromContext(ctx context.Context) *renderToolSink {
	sink, _ := ctx.Value(renderToolSinkContextKey{}).(*renderToolSink)
	return sink
}

// newRenderToolExecutor builds the executor for the model-callable
// navi.render.visualize tool. It is read-only: it loads the current chat's
// persisted tool-usage (the same source as the deterministic short-circuit),
// builds a NaviDataView render payload, attaches it to the run via the sink, and
// returns a short confirmation for the model. The visualization itself is
// rendered client-side from the attached payload.
func newRenderToolExecutor(cfg LoopConfig) navitool.ToolExecutor {
	return navitool.ExecutorFunc(func(ctx context.Context, args map[string]any) (navitool.ToolResult, error) {
		sink := renderToolSinkFromContext(ctx)
		if sink == nil || sink.Payload == nil {
			return navitool.ToolResult{}, fmt.Errorf("%s is only available in a runtime chat session", renderVisualizeToolName)
		}
		if cfg.Chats == nil {
			return navitool.ToolResult{}, fmt.Errorf("render: chat store not configured")
		}
		view, _ := args["view"].(string)
		thread, err := cfg.Chats.GetChatWithMessages(ctx, sink.ChatID)
		if err != nil {
			return navitool.ToolResult{}, fmt.Errorf("render: load chat: %w", err)
		}
		events := extractToolEventsFromThread(thread)
		payload := render.BuildToolUsagePayload(events, view, render.Trace{RunID: sink.RunID, ChatID: sink.ChatID})
		raw, err := json.Marshal(payload)
		if err != nil {
			return navitool.ToolResult{}, fmt.Errorf("render: marshal payload: %w", err)
		}
		*sink.Payload = raw

		summary := "no tool usage has been recorded in this chat yet"
		if payload.DataView != nil && strings.TrimSpace(payload.DataView.Fallback.Summary) != "" {
			summary = payload.DataView.Fallback.Summary
		}
		return navitool.ToolResult{Content: "Rendered a tool-usage visualization for this chat (" + summary + "). It is shown to the user automatically."}, nil
	})
}

// executeDataDrivenRender handles the deterministic data-driven render
// short-circuit (first proving slice: "show me a graph of tool usage in this
// chat"). It builds a renderer-neutral NaviDataView from REAL chat-scoped tool
// usage, attaches an OpenUI render payload (+ fallback markdown) to the run, and
// replies with the fallback markdown as message content so non-UI clients still
// get the data. The console renders the OpenUI lane from metadata.renderPayload
// and falls back to fallbackMarkdown if OpenUI is unavailable.
//
// classification.Capability selects the data capability; only tool_usage is wired.
func (l *AgentLoop) executeDataDrivenRender(
	ctx context.Context,
	run *naviruntime.RunState,
	chatID string,
	experienceMode ExperienceMode,
	thread *ChatThread,
	classification render.Classification,
) (*naviruntime.ExecuteResult, error) {
	events := extractToolEventsFromThread(thread)
	trace := render.Trace{RunID: run.RunID, ChatID: chatID}
	payload := render.BuildToolUsagePayload(events, classification.PreferredView, trace)

	meta := map[string]string{
		"completion":         "data_driven_render",
		"render_intent":      string(classification.Intent),
		"render_capability":  classification.Capability,
		"render_mode":        string(payload.Mode),
		"render_tool_events": strconv.Itoa(len(events)),
		"render_empty":       strconv.FormatBool(len(events) == 0),
	}
	if raw, err := json.Marshal(payload); err == nil {
		run.RenderPayload = raw
		meta["render_payload_bytes"] = strconv.Itoa(len(raw))
	} else {
		meta["render_payload_error"] = err.Error()
	}

	content := payload.FallbackMarkdown
	final := l.shapeReply(content, experienceMode)
	meta["reply_length"] = strconv.Itoa(len(final))
	l.recordDirectRunCompletion(ctx, run, chatID, orchestration.ExecutionModeRunExecute, meta)

	summary := content
	if payload.DataView != nil && strings.TrimSpace(payload.DataView.Fallback.Summary) != "" {
		summary = payload.DataView.Fallback.Summary
	}
	return &naviruntime.ExecuteResult{
		Run:            run,
		Completed:      true,
		FinalContent:   final,
		ReplyLen:       len(final),
		ExperienceMode: string(experienceMode),
		Outcome:        schema.ExecutionOutcomeSucceeded,
		OutcomeSummary: summary,
	}, nil
}

// executePrototypeRender handles the deterministic OpenUI prototype render
// short-circuit (second render slice: "create a dashboard mockup for connector
// health"). It builds a renderer-neutral NaviUIPrototype with clearly-labeled
// PLACEHOLDER data, derives an OpenUI render payload (+ honest fallback markdown),
// attaches it to the run, and replies with the fallback markdown as message content
// so non-UI clients still get a useful, labeled mockup. The console renders the
// OpenUI lane from metadata.renderPayload and falls back to fallbackMarkdown if
// OpenUI is unavailable.
//
// The prototype is read-only (no privileged actions) and is NOT persisted as an
// artifact — this slice is in-chat prototype rendering only.
func (l *AgentLoop) executePrototypeRender(
	ctx context.Context,
	run *naviruntime.RunState,
	chatID string,
	experienceMode ExperienceMode,
	prompt string,
	classification render.Classification,
) (*naviruntime.ExecuteResult, error) {
	purpose := render.NormalizePrototypePurpose(classification.PreferredView)
	trace := render.PrototypeTrace{RunID: run.RunID}
	payload := render.BuildPrototypePayload(purpose, prompt, trace)

	meta := map[string]string{
		"completion":         "openui_prototype_render",
		"render_intent":      string(classification.Intent),
		"render_purpose":     string(purpose),
		"render_mode":        string(payload.Mode),
		"render_data_source": string(payload.DataSourceKind),
	}
	if raw, err := json.Marshal(payload); err == nil {
		run.RenderPayload = raw
		meta["render_payload_bytes"] = strconv.Itoa(len(raw))
	} else {
		meta["render_payload_error"] = err.Error()
	}

	content := payload.FallbackMarkdown
	final := l.shapeReply(content, experienceMode)
	meta["reply_length"] = strconv.Itoa(len(final))
	l.recordDirectRunCompletion(ctx, run, chatID, orchestration.ExecutionModeRunExecute, meta)

	summary := "Generated a UI prototype (placeholder data)."
	if payload.Prototype != nil && strings.TrimSpace(payload.Prototype.Title) != "" {
		summary = payload.Prototype.Title + " (prototype, placeholder data)"
	}
	return &naviruntime.ExecuteResult{
		Run:            run,
		Completed:      true,
		FinalContent:   final,
		ReplyLen:       len(final),
		ExperienceMode: string(experienceMode),
		Outcome:        schema.ExecutionOutcomeSucceeded,
		OutcomeSummary: summary,
	}, nil
}

// extractToolEventsFromThread reads tool invocations from the chat's persisted
// assistant-message toolParts metadata — the current, chat-scoped data source for
// tool-usage views. It never fabricates data: a chat with no recorded tool calls
// yields an empty slice (and an honest degraded view downstream).
//
// Limitation: toolParts lack per-call durations and precise timestamps; UsedAt is
// the message creation time. A richer future source is the event log
// (tool.call.* events) / runtime traces.
func extractToolEventsFromThread(thread *ChatThread) []render.ToolEvent {
	if thread == nil {
		return nil
	}
	var events []render.ToolEvent
	for _, msg := range thread.Messages {
		if msg.Metadata == nil {
			continue
		}
		raw, ok := msg.Metadata["toolParts"]
		if !ok {
			continue
		}
		parts, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			name, _ := part["toolName"].(string)
			isErr, _ := part["isError"].(bool)
			events = append(events, render.ToolEvent{
				ToolName: name,
				IsError:  isErr,
				UsedAt:   msg.CreatedAt,
			})
		}
	}
	return events
}
