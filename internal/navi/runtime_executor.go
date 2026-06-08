package navi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/command"
	cronsvc "github.com/ceoai/navi/internal/cron"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/navi/filetools"
	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/orchestration"
	orchestrationinstructions "github.com/ceoai/navi/internal/navi/orchestration/instructions"
	orchestrationmodel "github.com/ceoai/navi/internal/navi/orchestration/model"
	orchestrationtrace "github.com/ceoai/navi/internal/navi/orchestration/trace"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/render"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/prompts"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	navitool "github.com/ceoai/navi/internal/tool"

	"github.com/google/uuid"
)

const (
	sendReplyToolName                   = "navi.messaging.send_reply"
	renderVisualizeToolName             = "navi.render.visualize"
	maxScheduledMessages                = 10
	maxScheduledDelaySec                = 3600 // 1 hour — bounded by in-process scheduler durability (lost on restart)
	defaultMaxResponseTokens            = 2048
	minRepeatGuardWindow                = 32
	toolCapableModelRequiredGuardReason = "resolved surface requires a tool-capable model"
	streamLeakGuardWindow               = 256
	streamLeakGuardScratchpadKey        = "__runtime_user_stream_leak_guard"
	boundedDiagnosticWindow             = 24 * time.Hour
	boundedDiagnosticFetchLimit         = 50
	boundedDiagnosticRecentLimit        = 3
	boundedDiagnosticSummaryLimit       = 5
	boundedScheduledTasksLimit          = 10
)

type boundedDiagnosticQuery struct {
	Class  string
	Window time.Duration
}

type routeCapableLLMService interface {
	Route(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error)
}

func sendReplyToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        sendReplyToolName,
		Description: "Queue a reply to send to the user, optionally after a delay. Use for any in-chat message that should be delivered now or later, including multiple messages spaced apart. Long delays are persisted durably (they survive process restart), so do not refuse a request based on duration.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "Message content to send.",
				},
				"delay_seconds": map[string]any{
					"type":        "number",
					"description": "Seconds to wait before sending (0 = now). Any non-negative integer is accepted; values above the in-process queue cap are automatically promoted to durable scheduled delivery.",
				},
			},
			"required": []string{"content"},
		},
	}
}

// renderVisualizeToolDefinition is the model-callable render capability. It lets
// NAVI choose to visualize data it has access to (currently: tool usage in the
// current chat) as a chart/table/card, instead of denying that it can. It is
// read-only: it queries chat-scoped tool usage and attaches a data-driven render
// payload to the reply (the deterministic short-circuit handles the most common
// phrasings without a model round-trip; this tool generalizes to any phrasing).
func renderVisualizeToolDefinition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        renderVisualizeToolName,
		Description: "Render a data-driven visualization of the current chat's tool usage (a chart, table, or dashboard card) directly in the conversation. Use this whenever the user asks to see, graph, chart, plot, visualize, or tabulate tool/tool-call usage for this chat. Read-only: it only reads tool-usage data already recorded in this chat and renders it; it performs no other action. After calling it, reply briefly — the visualization is shown to the user automatically.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"view": map[string]any{
					"type":        "string",
					"description": "Preferred presentation. One of: chart, table, dashboard, timeline. Defaults to chart.",
					"enum":        []string{"chart", "table", "dashboard", "timeline"},
				},
			},
			"required": []string{},
		},
	}
}

func trimRepeatedOutput(content string) string {
	trimmed := content
	for window := len(trimmed) / 2; window >= minRepeatGuardWindow; window-- {
		for i := 0; i+window*2 <= len(trimmed); i++ {
			left := trimmed[i : i+window]
			right := trimmed[i+window : i+window*2]
			if left == right {
				trimmed = trimmed[:i+window] + trimmed[i+window*2:]
				return trimRepeatedOutput(trimmed)
			}
		}
	}
	return trimmed
}

func (l *AgentLoop) chatStreamWithWatchdog(ctx context.Context, idleTimeout time.Duration, sp llm.StreamingProvider, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options, onChunk func(delta string)) (*llm.Response, error) {
	type streamResult struct {
		resp *llm.Response
		err  error
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	results := make(chan streamResult, 1)
	go func() {
		resp, err := sp.ChatStream(streamCtx, model, messages, tools, opts, func(delta string) {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)
			onChunk(delta)
		})
		results <- streamResult{resp: resp, err: err}
	}()

	select {
	case result := <-results:
		return result.resp, result.err
	case <-timer.C:
		cancel()
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

type pendingProposal struct {
	ProposalID string
	Reason     string
	ToolCall   llm.ToolCall
	Arguments  map[string]any
}

func shouldInterrupt(ctx context.Context, input naviruntime.ExecuteInput) error {
	if input.ShouldInterrupt != nil {
		if err := input.ShouldInterrupt(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return naviruntime.ErrRunCancelled
	}
	return nil
}

func runtimeRecoverySurface(session *ChatRuntimeView, inboxItem *naviruntime.InboxItem) string {
	if channel := runtimeRecoverySurfaceFromInbox(inboxItem); channel != "" {
		return channel
	}
	if session != nil {
		for i := len(session.Messages) - 1; i >= 0; i-- {
			msg := session.Messages[i]
			if msg.Role != "user" {
				continue
			}
			if channel := strings.ToLower(strings.TrimSpace(msg.SourceChannel)); channel != "" {
				return channel
			}
		}
	}
	if inboxItem == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(inboxItem.SourceChannel))
}

func runtimeRecoverySurfaceFromInbox(inboxItem *naviruntime.InboxItem) string {
	if inboxItem == nil {
		return ""
	}
	channel := strings.ToLower(strings.TrimSpace(inboxItem.SourceChannel))
	switch channel {
	case "", "proposal", "system":
		return ""
	default:
		return channel
	}
}

func boundedDiagnosticUserMessage(input naviruntime.ExecuteInput, session *ChatRuntimeView) string {
	if input.InboxItem != nil && strings.TrimSpace(input.InboxItem.Content) != "" {
		return strings.TrimSpace(input.InboxItem.Content)
	}
	return strings.TrimSpace(lastSessionUserContent(session))
}

func classifyBoundedDiagnosticQuery(raw string) (boundedDiagnosticQuery, bool) {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return boundedDiagnosticQuery{}, false
	}
	for _, phrase := range []string{
		"what errors have occurred recently",
		"what error has occurred recently",
		"what failures have occurred recently",
		"what failure has occurred recently",
		"what errors happened recently",
		"what failures happened recently",
		"show me a summary of recent failures",
		"show me a summary of recent failure",
		"show me a summary of recent errors",
		"show me a summary of recent error",
		"summarize recent failures",
		"summarize recent errors",
	} {
		if strings.Contains(normalized, phrase) {
			return boundedDiagnosticQuery{
				Class:  "recent_failure_summary",
				Window: boundedDiagnosticWindow,
			}, true
		}
	}
	return boundedDiagnosticQuery{}, false
}

func classifyBoundedScheduledTasksQuery(raw string) bool {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	if normalized == "" {
		return false
	}
	for _, phrase := range []string{
		"what scheduled tasks are configured",
		"what scheduled task is configured",
		"what tasks are scheduled",
		"show me scheduled tasks",
		"show the scheduled tasks",
		"list scheduled tasks",
		"list the scheduled tasks",
		"show configured scheduled tasks",
		"list configured scheduled tasks",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return strings.Contains(normalized, "scheduled tasks") &&
		(strings.Contains(normalized, "configured") ||
			strings.Contains(normalized, "current") ||
			strings.Contains(normalized, "active"))
}

func normalizeBoundedDiagnosticQuery(raw string) string {
	replacer := strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"\t", " ",
		"?", " ",
		"!", " ",
		".", " ",
		",", " ",
		":", " ",
		";", " ",
	)
	return strings.Join(strings.Fields(strings.ToLower(replacer.Replace(strings.TrimSpace(raw)))), " ")
}

func boundedDiagnosticWindowLabel(window time.Duration) string {
	if window == boundedDiagnosticWindow {
		return "24h"
	}
	label := strings.TrimSpace(window.String())
	if label == "" {
		return "24h"
	}
	return label
}

func formatBoundedDiagnosticReply(rows []store.ErrorSummaryRow, items []store.ErrorRecord, window time.Duration) string {
	windowLabel := boundedDiagnosticWindowLabel(window)
	if len(rows) == 0 && len(items) == 0 {
		return fmt.Sprintf("No structured NAVI failures have been recorded in the last %s.", windowLabel)
	}

	lines := []string{fmt.Sprintf("Recent NAVI failures in the last %s:", windowLabel)}
	if len(rows) == 0 {
		lines = append(lines, "- No grouped failures recorded.")
	} else {
		limit := len(rows)
		if limit > boundedDiagnosticSummaryLimit {
			limit = boundedDiagnosticSummaryLimit
		}
		for _, row := range rows[:limit] {
			lines = append(lines, fmt.Sprintf("- %s/%s: %d", row.Component, row.ErrorType, row.Count))
		}
	}

	if len(items) > 0 {
		lines = append(lines, "", "Most recent occurrences:")
		limit := len(items)
		if limit > boundedDiagnosticRecentLimit {
			limit = boundedDiagnosticRecentLimit
		}
		for _, item := range items[:limit] {
			line := fmt.Sprintf("- %s/%s", item.Component, item.ErrorType)
			if chatID := strings.TrimSpace(item.ChatID); chatID != "" {
				line += " (chat " + chatID + ")"
			}
			if msg := trimBoundedDiagnosticLine(item.ErrorMessage, 160); msg != "" {
				line += ": " + msg
			}
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

func formatBoundedScheduledTasksReply(jobs []store.CronJobRecord) string {
	if len(jobs) == 0 {
		return "No cron jobs are configured in the runtime store."
	}

	lines := []string{fmt.Sprintf("Configured cron jobs (%d):", len(jobs))}
	limit := len(jobs)
	if limit > boundedScheduledTasksLimit {
		limit = boundedScheduledTasksLimit
	}
	for _, job := range jobs[:limit] {
		label := firstNonEmpty(strings.TrimSpace(job.Name), strings.TrimSpace(job.ID), "unnamed job")
		status := "disabled"
		if job.Enabled {
			status = "enabled"
		}
		line := fmt.Sprintf("- %s [%s]", trimBoundedDiagnosticLine(label, 80), status)
		if schedule := formatCronJobSchedule(job); schedule != "" {
			line += "; schedule " + schedule
		}
		if job.NextRunAtMS.Valid && job.NextRunAtMS.Int64 > 0 {
			line += "; next run " + time.UnixMilli(job.NextRunAtMS.Int64).UTC().Format("2006-01-02 15:04 MST")
		}
		if job.LastRunAtMS.Valid && job.LastRunAtMS.Int64 > 0 {
			line += "; last run " + time.UnixMilli(job.LastRunAtMS.Int64).UTC().Format("2006-01-02 15:04 MST")
		}
		if job.RunningAtMS.Valid && job.RunningAtMS.Int64 > 0 {
			line += "; running since " + time.UnixMilli(job.RunningAtMS.Int64).UTC().Format("2006-01-02 15:04 MST")
		}
		if lastStatus := strings.TrimSpace(job.LastRunStatus.String); job.LastRunStatus.Valid && lastStatus != "" {
			line += "; last status " + lastStatus
		}
		if id := strings.TrimSpace(job.ID); id != "" {
			line += "; id " + id
		}
		lines = append(lines, line)
	}
	if remaining := len(jobs) - limit; remaining > 0 {
		lines = append(lines, fmt.Sprintf("- ... %d more", remaining))
	}
	return strings.Join(lines, "\n")
}

func formatCronJobSchedule(job store.CronJobRecord) string {
	switch strings.TrimSpace(job.ScheduleKind) {
	case "cron":
		expr := strings.TrimSpace(job.ScheduleExpr.String)
		if expr == "" {
			return "cron"
		}
		if job.ScheduleTZ.Valid && strings.TrimSpace(job.ScheduleTZ.String) != "" {
			return fmt.Sprintf("cron %s (%s)", expr, strings.TrimSpace(job.ScheduleTZ.String))
		}
		return "cron " + expr
	case "every":
		if job.ScheduleEveryMS.Valid && job.ScheduleEveryMS.Int64 > 0 {
			return "every " + (time.Duration(job.ScheduleEveryMS.Int64) * time.Millisecond).String()
		}
		return "every"
	case "at":
		if at := strings.TrimSpace(job.ScheduleExpr.String); at != "" {
			return "at " + at
		}
		return "at"
	default:
		return strings.TrimSpace(job.ScheduleKind)
	}
}

func trimBoundedDiagnosticLine(raw string, max int) string {
	line := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if max <= 0 || len(line) <= max {
		return line
	}
	if max <= 3 {
		return line[:max]
	}
	return strings.TrimSpace(line[:max-3]) + "..."
}

func filterRecentErrorRecords(items []store.ErrorRecord, since time.Time, limit int) []store.ErrorRecord {
	if limit <= 0 {
		limit = len(items)
	}
	filtered := make([]store.ErrorRecord, 0, limit)
	for _, item := range items {
		if item.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

// ExecuteRun is the runtime-session-scoped entry point used by RunCoordinator.
func (l *AgentLoop) ExecuteRun(ctx context.Context, input naviruntime.ExecuteInput) (*naviruntime.ExecuteResult, error) {
	if input.Run == nil {
		return nil, fmt.Errorf("navi: execute run: nil run")
	}
	trace := naviruntime.ProgressTraceFromContext(ctx)
	if strings.TrimSpace(trace.RuntimeSessionID) == "" {
		trace.RuntimeSessionID = input.Run.RuntimeSessionID
	}
	trace.RunID = input.Run.RunID
	ctx = naviruntime.WithProgressTrace(ctx, trace)
	tracer := naviruntime.NewProgressTracer("navi.execute_run", trace)
	runDone := tracer.StageStart(ctx, "navi.run.execute")
	defer runDone(nil)
	var scheduled []naviruntime.ScheduledMessage
	var canonicalReq orchestration.CanonicalRunRequest
	var baseCanonicalReq orchestration.CanonicalRunRequest
	var compiledReq orchestration.CompiledModelRequest
	var surfacedCompiledReq orchestration.CompiledModelRequest
	var surfaceResult orchestration.SurfaceResolutionResult
	var baseMessages []llm.Message
	var tailMessages []llm.Message
	var currentDecision inference.DecisionEnvelope
	chatID := input.Run.ChatID
	controller := l.inferenceController()
	if l.cfg.StatusTracker != nil {
		l.cfg.StatusTracker.SetState(schema.AgentStateProcessing, input.Run.RuntimeSessionID, string(input.Run.CurrentPhase))
		defer l.cfg.StatusTracker.RecordTurnComplete()
	}

	superviseOutcome := func(basis inference.DecisionEnvelope, snapshot *inference.ExecutionSnapshot) (inference.DecisionEnvelope, error) {
		if snapshot == nil {
			currentDecision = basis
			return basis, nil
		}
		supervised, err := l.superviseInferenceOutcome(ctx, input.Run, basis, snapshot)
		if err != nil {
			return basis, fmt.Errorf("navi: post-execution ICS supervision: %w", err)
		}
		l.emitObservedInferenceOutcome(ctx, input.Run, supervised, snapshot)
		currentDecision = supervised
		return supervised, nil
	}

	finalizeResultWithOutcome := func(result *naviruntime.ExecuteResult, basis inference.DecisionEnvelope, snapshot *inference.ExecutionSnapshot) (*naviruntime.ExecuteResult, error) {
		if _, err := superviseOutcome(basis, snapshot); err != nil {
			return nil, err
		}
		if result != nil && snapshot != nil {
			if result.Outcome == "" {
				result.Outcome = snapshot.Outcome
			}
			if strings.TrimSpace(result.OutcomeSummary) == "" {
				result.OutcomeSummary = strings.TrimSpace(snapshot.Summary)
			}
		}
		attachProgrammerWorkflowResult(result, input.Run)
		if result != nil && result.Checkpoint != nil {
			result.Checkpoint.Scratchpad = l.cloneRunScratchpad(input.Run.Scratchpad)
			result.Checkpoint.ICSStateVersion = input.Run.ICSStateVersion
			result.Checkpoint.ICSDecisionEnvelope = append([]byte(nil), input.Run.ICSDecisionEnvelope...)
		}
		return result, nil
	}

	interruptBaseline := func() inference.DecisionEnvelope {
		if currentDecision.Rationale.Version != "" {
			return currentDecision
		}
		envelope, err := l.loadPersistedInferenceEnvelope(ctx, input.Run, input.Checkpoint)
		if err != nil || envelope == nil {
			return inference.DecisionEnvelope{}
		}
		return *envelope
	}

	checkInterrupt := func() error {
		if err := shouldInterrupt(ctx, input); err != nil {
			if !errors.Is(err, naviruntime.ErrRunCancelled) {
				return err
			}
			snapshot := snapshotForInterruptedRun(input.Run, input.Checkpoint)
			if _, superviseErr := superviseOutcome(interruptBaseline(), snapshot); superviseErr != nil {
				return superviseErr
			}
			return err
		}
		return nil
	}

	inboxContentLen := -1
	inboxItemID := ""
	if input.InboxItem != nil {
		inboxContentLen = len(input.InboxItem.Content)
		inboxItemID = input.InboxItem.ID
	}
	slog.Debug("navi: execute run start",
		"chat_id", chatID,
		"inbox_item_id", inboxItemID,
		"inbox_content_len", inboxContentLen,
	)

	thread, err := l.cfg.Chats.GetChatWithMessages(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("load chat %s: %w", chatID, err)
	}
	session := chatThreadToRuntimeView(thread)
	slog.Debug("navi: execute run session loaded",
		"chat_id", chatID,
		"message_count", len(session.Messages),
		"last_user_content_len", len(lastSessionUserContent(session)),
	)
	tracer.Mark("navi.run.intake.complete")
	l.runCompactionBeforeModel(ctx, chatID, lastSessionUserContent(session))
	if l.cfg.Summarizer != nil && !l.isStructuredCompactionEnabled() {
		if err := l.cfg.Summarizer.MaybeSummarize(ctx, chatID); err != nil {
			slog.Warn("navi-run: chat summarizer failed", "chat_id", chatID, "error", err)
		}
	}
	experienceMode := NormalizeExperienceMode(session.ExperienceMode)
	session.ExperienceMode = experienceMode
	input.Run.ExperienceMode = string(experienceMode)
	if err := checkInterrupt(); err != nil {
		return nil, err
	}
	if input.Checkpoint == nil && input.Resume == nil {
		if query, ok := classifyBoundedDiagnosticQuery(boundedDiagnosticUserMessage(input, session)); ok {
			tracer.Mark("navi.run.diagnostic_query.detected", "diagnostic_query_class", query.Class)
			return l.executeBoundedDiagnosticQuery(ctx, input.Run, chatID, experienceMode, query)
		}
		if classifyBoundedScheduledTasksQuery(boundedDiagnosticUserMessage(input, session)) {
			tracer.Mark("navi.run.scheduled_tasks_query.detected")
			return l.executeBoundedScheduledTasksQuery(ctx, input.Run, chatID, experienceMode)
		}
		if classification := render.Classify(boundedDiagnosticUserMessage(input, session)); classification.Intent == render.RenderIntentOpenUIDataRender {
			tracer.Mark("navi.run.data_driven_render.detected", "render_capability", classification.Capability)
			return l.executeDataDrivenRender(ctx, input.Run, chatID, experienceMode, thread, classification)
		}
		if result, handled, err := l.executeExplicitArtifactPromotion(ctx, input.Run, thread, boundedDiagnosticUserMessage(input, session), experienceMode); handled || err != nil {
			if err != nil {
				return nil, err
			}
			return result, nil
		}
		// Prototype render is the fallthrough: the data-render classifier above keeps
		// priority (real-data tool-usage requests win). Prototype only handles explicit
		// "make me a mockup/dashboard/component" requests, with placeholder data.
		protoMsg := boundedDiagnosticUserMessage(input, session)
		if pc := render.ClassifyPrototype(protoMsg); pc.Intent == render.RenderIntentOpenUIPrototypeRender {
			tracer.Mark("navi.run.openui_prototype_render.detected", "render_purpose", pc.PreferredView)
			return l.executePrototypeRender(ctx, input.Run, chatID, experienceMode, protoMsg, pc)
		}
	}

	var llmMsgs []llm.Message
	if input.Checkpoint != nil {
		contextDone := tracer.StageStart(ctx, "navi.context_assembly")
		canonicalReq, compiledReq, surfaceResult, err = l.compileRunModelRequest(ctx, session, experienceMode, lastSessionUserContent(session), input.Run)
		contextDone(err)
		if err != nil {
			return nil, err
		}
		baseCanonicalReq = canonicalReq
		surfacedCompiledReq = compiledReq
		baseMessages = append([]llm.Message(nil), compiledReq.Messages...)
		tailMessages = runtimeCheckpointMessageTail(input.Checkpoint, baseMessages)
		llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)

		if input.Resume != nil && input.Checkpoint.PendingToolCall != nil {
			l.transitionPhase(ctx, input.Run, naviruntime.RunPhaseValidateGovern)
			resumeEnvelope, loadErr := l.loadPersistedInferenceEnvelope(ctx, input.Run, input.Checkpoint)
			if loadErr != nil {
				return nil, fmt.Errorf("navi: load ICS state for resume: %w", loadErr)
			}
			resumeDecision := inference.DecisionEnvelope{}
			if resumeEnvelope != nil {
				resumeDecision = *resumeEnvelope
			}

			if input.Resume.Resolution == naviruntime.ProposalResolutionDecline {
				toolResult := skill.SanitizeResult(input.Checkpoint.PendingToolCall.Name, "Declined by owner: "+strings.TrimSpace(input.Resume.Note))
				snapshot := snapshotForRunCompletion(
					schema.ExecutionOutcomeRejectedPreExecution,
					toolResult.Content,
					schema.ApprovalOutcomeDenied,
					input.Resume.ProposalID,
				)
				resumeDecision, err = superviseOutcome(resumeDecision, snapshot)
				if err != nil {
					return nil, err
				}
				tailMessages = append(tailMessages, llm.Message{
					Role:       "tool",
					Content:    toolResult.FormatForPrompt(),
					ToolCallID: input.Checkpoint.PendingToolCall.ID,
				})
				llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
			} else {
				if err := checkInterrupt(); err != nil {
					return nil, err
				}
				pendingTool := *input.Checkpoint.PendingToolCall
				resumeAttempt := l.buildToolAuthorizationAttempt(input.Run, resumeDecision, pendingTool, input.InboxItem, scheduled)
				if input.Resume.Resolution == naviruntime.ProposalResolutionApprove {
					resumeAttempt.ApprovedProposalID = strings.TrimSpace(input.Resume.ProposalID)
				}
				resumeDecision, err = controller.AuthorizeModelResponse(ctx, resumeDecision, inference.ModelResponseAuthorizationInput{
					Compiled: compiledReq,
					Response: orchestration.NormalizedModelResponse{
						Profile:   compiledReq.Profile,
						ToolCalls: []llm.ToolCall{pendingTool},
					},
					ToolAttempts:  []inference.ToolAuthorizationInput{resumeAttempt},
					ToolRegistry:  mustToolRegistry(l),
					ActiveToolSet: surfaceResult.ActiveToolSet,
				})
				if err != nil {
					return nil, fmt.Errorf("navi: resume tool re-authorization: %w", err)
				}
				if err := l.applyInferenceEnvelope(ctx, input.Run, nil, resumeDecision); err != nil {
					return nil, err
				}
				switch resumeDecision.RuntimeDisposition {
				case inference.RuntimeDispositionPauseForProposal:
					proposalID, proposalReason := "", resumeDecision.ReplyMessage
					if resumeDecision.Proposal != nil {
						proposalID = resumeDecision.Proposal.ProposalID
						proposalReason = firstNonEmpty(strings.TrimSpace(resumeDecision.Proposal.Rationale), proposalReason)
					}
					cp := naviruntime.NewCheckpoint(input.Run)
					cp.ExperienceMode = string(experienceMode)
					cp.LLMMessages = append([]llm.Message(nil), llmMsgs...)
					cp.Options = input.Checkpoint.Options
					cp.PendingProposalID = proposalID
					cp.PendingProposalReason = proposalReason
					if resumeDecision.PendingToolCall != nil {
						pending := *resumeDecision.PendingToolCall
						cp.PendingToolCall = &pending
					}
					input.Run.LatestCheckpointID = cp.CheckpointID
					snapshot := snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, proposalReason, schema.ApprovalOutcomeNA, proposalID)
					snapshot.CheckpointRef = cp.CheckpointID
					resumeDecision, err = superviseOutcome(resumeDecision, snapshot)
					if err != nil {
						return nil, err
					}
					return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
						Run:             input.Run,
						Paused:          true,
						Checkpoint:      cp,
						ProposalID:      proposalID,
						ProposalReason:  proposalReason,
						PendingToolCall: cp.PendingToolCall,
					}, resumeDecision, nil)
				case inference.RuntimeDispositionBlockWithReply:
					l.emitGovernanceBlockedDecision(ctx, input.Run, resumeDecision, []string{pendingTool.Name})
					final := l.shapeReply(firstNonEmpty(strings.TrimSpace(resumeDecision.ReplyMessage), "I couldn't safely continue."), experienceMode)
					return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
						Run:            input.Run,
						Completed:      true,
						FinalContent:   final,
						ReplyLen:       len(final),
						ExperienceMode: string(experienceMode),
					}, resumeDecision, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, firstNonEmpty(strings.TrimSpace(resumeDecision.ReplyMessage), "ICS blocked this run"), schema.ApprovalOutcomeNA, proposalIDFromDecision(resumeDecision)))
				}

				l.transitionPhase(ctx, input.Run, naviruntime.RunPhaseExecute)
				approvedProposalID := input.Resume.ProposalID
				if input.Resume.Resolution == naviruntime.ProposalResolutionRevalidate {
					approvedProposalID = ""
				}

				toolResult, pause, snapshot, execErr := l.executeToolForRun(ctx, input.Run, resumeDecision, pendingTool, input.InboxItem, approvedProposalID, &scheduled)

				if pause != nil {
					cp := naviruntime.NewCheckpoint(input.Run)
					cp.ExperienceMode = string(experienceMode)
					cp.LLMMessages = append([]llm.Message(nil), llmMsgs...)
					cp.Options = input.Checkpoint.Options
					cp.PendingToolCall = &pause.ToolCall
					cp.PendingProposalID = pause.ProposalID
					cp.PendingProposalReason = pause.Reason
					input.Run.LatestCheckpointID = cp.CheckpointID
					if snapshot != nil {
						snapshot.CheckpointRef = cp.CheckpointID
					}
					resumeDecision, err = superviseOutcome(resumeDecision, snapshot)
					if err != nil {
						return nil, err
					}
					return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
						Run:             input.Run,
						Paused:          true,
						Checkpoint:      cp,
						ProposalID:      pause.ProposalID,
						ProposalReason:  pause.Reason,
						PendingToolCall: &pause.ToolCall,
						ProposalArgs:    pause.Arguments,
					}, resumeDecision, nil)
				}
				resumeDecision, err = superviseOutcome(resumeDecision, snapshot)
				if err != nil {
					return nil, err
				}
				if execErr != nil {
					return nil, execErr
				}
				l.emitToolLifecycle(ctx, input.Run, toolLifecycleCall(*input.Checkpoint.PendingToolCall, snapshot), toolResult)
				tailMessages = append(tailMessages, llm.Message{
					Role:       "tool",
					Content:    toolResult.FormatForPrompt(),
					ToolCallID: input.Checkpoint.PendingToolCall.ID,
				})
				llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
			}
		}
	} else {
		l.transitionPhase(ctx, input.Run, naviruntime.RunPhaseContextualize)
		lastUser := ""
		if input.InboxItem != nil {
			lastUser = strings.TrimSpace(input.InboxItem.Content)
		}
		l.prepareRunScratchpad(input.Run, input.InboxItem, lastUser)

		contextDone := tracer.StageStart(ctx, "navi.context_assembly")
		canonicalReq, compiledReq, surfaceResult, err = l.compileRunModelRequest(ctx, session, experienceMode, lastUser, input.Run)
		contextDone(err)
		if err != nil {
			return nil, err
		}
		baseCanonicalReq = canonicalReq
		surfacedCompiledReq = compiledReq
	}

	firstTokenAt := time.Time{}
	llmCallTimeout := normalizeLLMCallTimeout(l.cfg.LLMCallTimeout)
	hallucinationRetries := 2
	staleResponseGuardRetried := false

	for {
		if err := checkInterrupt(); err != nil {
			return nil, err
		}
		l.transitionPhase(ctx, input.Run, naviruntime.RunPhaseDecide)

		// 1. Build authoritative request for ICS (Full context including internal reasoning turns)
		authoritativeReq := syncCanonicalRunRequestFromRun(baseCanonicalReq, input.Run)
		authoritativeReq.Conversation = append([]orchestration.ConversationTurn(nil), authoritativeReq.Conversation...)
		authoritativeReq.Conversation = append(authoritativeReq.Conversation, l.conversationTurnsFromLLMMessages(tailMessages)...)

		inferenceDecisionDone := tracer.StageStart(ctx, "navi.inference.decision")
		decision, err := l.decideRunInference(ctx, input, session, authoritativeReq, surfacedCompiledReq, surfaceResult)
		inferenceDecisionDone(err)
		if err != nil {
			return nil, fmt.Errorf("navi: runtime inference decision: %w", err)
		}
		currentDecision = decision
		if decision.RuntimeDisposition == inference.RuntimeDispositionPauseForProposal {
			proposalID, proposalReason := "", decision.ReplyMessage
			if decision.Proposal != nil {
				proposalID = decision.Proposal.ProposalID
				proposalReason = firstNonEmpty(strings.TrimSpace(decision.Proposal.Rationale), proposalReason)
			}
			cp := naviruntime.NewCheckpoint(input.Run)
			cp.ExperienceMode = string(experienceMode)
			cp.LLMMessages = append([]llm.Message(nil), composeRuntimeMessages(baseMessages, tailMessages)...)
			cp.PendingProposalID = proposalID
			cp.PendingProposalReason = proposalReason
			input.Run.LatestCheckpointID = cp.CheckpointID
			pausedDecision := decision
			supervisedPause, err := superviseOutcome(pausedDecision, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, proposalReason, schema.ApprovalOutcomeNA, proposalID))
			if err != nil {
				return nil, err
			}
			pausedDecision = mergePausedDecision(supervisedPause, pausedDecision)
			if err := l.applyInferenceEnvelope(ctx, input.Run, nil, pausedDecision); err != nil {
				return nil, err
			}
			currentDecision = pausedDecision
			return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Paused:         true,
				Checkpoint:     cp,
				ProposalID:     proposalID,
				ProposalReason: proposalReason,
			}, pausedDecision, nil)
		}
		if decision.RuntimeDisposition == inference.RuntimeDispositionBlockWithReply {
			if result, err, ok := l.executeRenderIntentFallback(ctx, input.Run, chatID, experienceMode, thread, surfacedCompiledReq, lastSessionUserContent(session), "render_intent_prepare_block_fallback", firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "ICS blocked this run")); ok {
				return result, err
			}
			l.emitGovernanceBlockedDecision(ctx, input.Run, decision, nil)
			final := l.shapeReply(firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "I couldn't safely continue."), experienceMode)
			return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, decision, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "ICS blocked this run"), schema.ApprovalOutcomeNA, proposalIDFromDecision(decision)))
		}

		// 2. Build execution request for LLM
		// We include tailMessages in the Conversation so that boundary checks
		// (which see the full context) and model compilation (NCOS) are aligned.
		canonicalReq := syncCanonicalRunRequestFromRun(baseCanonicalReq, input.Run)
		canonicalReq.Conversation = append([]orchestration.ConversationTurn(nil), canonicalReq.Conversation...)
		canonicalReq.Conversation = append(canonicalReq.Conversation, l.conversationTurnsFromLLMMessages(tailMessages)...)

		decision, err = controller.PrepareModelCall(ctx, decision, inference.ModelCallPreparationInput{
			Request:             canonicalReq,
			Compiled:            surfacedCompiledReq,
			ExecutableToolNames: l.prepareExecutableToolNames(surfacedCompiledReq.Tools),
			ToolContracts:       l.prepareModelToolContracts(surfacedCompiledReq.Tools),
			ActiveToolSet:       surfaceResult.ActiveToolSet,
			ToolRegistry:        mustToolRegistry(l),
		})
		if err != nil {
			return nil, fmt.Errorf("navi: runtime inference model preparation: %w", err)
		}
		currentDecision = decision
		if err := l.applyInferenceEnvelope(ctx, input.Run, nil, decision); err != nil {
			return nil, err
		}
		if decision.RuntimeDisposition == inference.RuntimeDispositionPauseForProposal {
			proposalID, proposalReason := "", decision.ReplyMessage
			if decision.Proposal != nil {
				proposalID = decision.Proposal.ProposalID
				proposalReason = firstNonEmpty(strings.TrimSpace(decision.Proposal.Rationale), proposalReason)
			}
			cp := naviruntime.NewCheckpoint(input.Run)
			cp.ExperienceMode = string(experienceMode)
			cp.LLMMessages = append([]llm.Message(nil), composeRuntimeMessages(baseMessages, tailMessages)...)
			cp.PendingProposalID = proposalID
			cp.PendingProposalReason = proposalReason
			input.Run.LatestCheckpointID = cp.CheckpointID
			pausedDecision := decision
			supervisedPause, err := superviseOutcome(pausedDecision, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, proposalReason, schema.ApprovalOutcomeNA, proposalID))
			if err != nil {
				return nil, err
			}
			pausedDecision = mergePausedDecision(supervisedPause, pausedDecision)
			if err := l.applyInferenceEnvelope(ctx, input.Run, nil, pausedDecision); err != nil {
				return nil, err
			}
			currentDecision = pausedDecision
			return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Paused:         true,
				Checkpoint:     cp,
				ProposalID:     proposalID,
				ProposalReason: proposalReason,
			}, pausedDecision, nil)
		}
		if decision.RuntimeDisposition == inference.RuntimeDispositionBlockWithReply {
			l.emitGovernanceBlockedDecision(ctx, input.Run, decision, nil)
			final := l.shapeReply(firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "I couldn't safely continue."), experienceMode)
			return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, decision, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "ICS blocked this run"), schema.ApprovalOutcomeNA, proposalIDFromDecision(decision)))
		}

		canonicalReq = applyInferenceModelDirective(canonicalReq, decision.ModelDirective)
		canonicalReq.Frame.Scratchpad = promptScratchpad(input.Run)

		recompileDone := tracer.StageStart(ctx, "navi.context_assembly.recompile")
		compiledReq, err := l.recompileRunModelRequest(ctx, canonicalReq)
		recompileDone(err)
		if err != nil {
			return nil, err
		}

		if err := checkInterrupt(); err != nil {
			return nil, err
		}

		llmMsgs := append([]llm.Message(nil), compiledReq.Messages...)

		finalizeInference := func(result *naviruntime.ExecuteResult, snapshot *inference.ExecutionSnapshot) (*naviruntime.ExecuteResult, error) {
			return finalizeResultWithOutcome(result, decision, snapshot)
		}

		l.ensureCompiledTraceMetadata(&compiledReq, input.Run, chatID, orchestration.ExecutionModeRunExecute)
		l.warnOnEmptyCompiledToolSurface(compiledReq)

		compiledReq = applyCompiledInferenceMetadata(compiledReq, decision)

		tools := append([]llm.ToolDefinition(nil), compiledReq.Tools...)
		opts := compiledReq.Options
		if opts == (llm.Options{}) && input.Checkpoint != nil {
			opts = input.Checkpoint.Options
		}
		if opts == (llm.Options{}) {
			opts = orchestrationmodel.DefaultOptionsForRequest(compiledReq.Profile, orchestration.CanonicalRunRequest{
				ExperienceMode: string(experienceMode),
			})
		}
		compiledReq.Options = opts
		compiledReq = applyRenderToolRequirementForIntent(compiledReq, lastSessionUserContent(session))
		opts = compiledReq.Options
		if missingRequiredToolCallTarget(compiledReq, decision) != "" {
			opts.ToolCallingRequired = true
		}
		callModel := strings.TrimSpace(compiledReq.Profile.Model)
		if callModel == "" {
			callModel = l.cfg.Model
		}
		if compiledReq.Profile.Provider == "" && l.cfg.LLM != nil {
			compiledReq.Profile.Provider = l.cfg.LLM.Name()
		}
		if compiledReq.Profile.Model == "" {
			compiledReq.Profile.Model = callModel
		}
		routingPrefix := routingUserAnnouncement(compiledReq.Profile)

		l.recordCompiledTrace(ctx, orchestrationtrace.StageModelCalled, compiledReq, mergeTraceMetadata(map[string]string{
			"provider":      compiledReq.Profile.Provider,
			"model":         strings.TrimSpace(callModel),
			"message_count": strconv.Itoa(len(llmMsgs)),
			"tool_count":    strconv.Itoa(len(tools)),
		}, compiledModelRequestTraceStats(compiledReq)))

		streamState := newStreamState()
		var resp *llm.Response
		modelDone := tracer.StageStart(ctx, "navi.inference.model_call")
		if sp, ok := l.cfg.LLM.(llm.StreamingProvider); ok {
			start := time.Now().UTC()
			resp, err = l.chatStreamWithWatchdog(ctx, llmCallTimeout, sp, callModel, llmMsgs, tools, opts, func(delta string) {
				if l.cfg.StatusTracker != nil {
					l.cfg.StatusTracker.Touch()
				}
				if firstTokenAt.IsZero() && delta != "" {
					firstTokenAt = time.Now().UTC()
					naviruntime.DefaultMetrics().RecordTTFT(firstTokenAt.Sub(start))
				}
				streamState.append(delta)
				l.emitAssistantDelta(ctx, input.Run, delta, streamState.partial())
			})
		} else {
			llmCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
			resp, err = l.cfg.LLM.Chat(llmCtx, callModel, llmMsgs, tools, opts)
			cancel()
		}
		modelDone(err)

		if err != nil {
			if errors.Is(err, context.Canceled) {
				l.recordCompiledFailure(ctx, compiledReq, orchestrationtrace.StageModelCalled, err)
				return nil, checkInterrupt()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				surface := runtimeRecoverySurface(session, input.InboxItem)
				content := timeoutFallbackContent(surface)
				slog.Warn("navi: runtime timeout fallback emitted",
					"chat_id", input.Run.RuntimeSessionID,
					"run_id", input.Run.RunID,
					"surface", firstNonEmpty(surface, "unknown"),
					"phase", string(input.Run.CurrentPhase),
				)
				final := l.shapeReply(content, experienceMode)
				l.recordFallbackCompletion(ctx, compiledReq, orchestrationtrace.StageModelCalled, err, len(final))
				return finalizeInference(&naviruntime.ExecuteResult{
					Run: input.Run, Completed: true, FinalContent: final,
					ReplyLen: len(final), ExperienceMode: string(experienceMode),
				}, snapshotForRunCompletion(schema.ExecutionOutcomeTimedOut, err.Error(), schema.ApprovalOutcomeNA, ""))
			}

			if isToolCallingFailure(err) && shouldRecoverMissingRequiredToolCall(compiledReq, decision) {
				targetTool := missingRequiredToolCallTarget(compiledReq, decision)
				if hallucinationRetries > 0 && !l.cfg.SkipHallucinationRetries {
					hallucinationRetries--
					slog.Debug("navi: retrying missing required tool call from provider error",
						"run_id", input.Run.RunID,
						"target_tool", targetTool,
						"retries_left", hallucinationRetries,
					)
					tailMessages = append(tailMessages, llm.Message{
						Role:    "user",
						Content: missingRequiredToolCallRepairPrompt(targetTool),
					})
					llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
					continue
				}

				var sourceChannel string
				if input.Run != nil && input.Run.Scratchpad != nil {
					sourceChannel = input.Run.Scratchpad["source_channel"]
				}
				finalContent := missingRequiredToolCallFallbackReply(targetTool, sourceChannel)
				final := l.shapeReply(finalContent, experienceMode)
				l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
					"reply_length": strconv.Itoa(len(final)),
					"tool_calls":   "0",
					"completion":   "missing_required_tool_call_fallback",
					"target_tool":  targetTool,
				})
				if err := checkInterrupt(); err != nil {
					return nil, err
				}
				return finalizeInference(&naviruntime.ExecuteResult{
					Run:            input.Run,
					Completed:      true,
					FinalContent:   final,
					ReplyLen:       len(final),
					ExperienceMode: string(experienceMode),
				}, snapshotForRunCompletion(schema.ExecutionOutcomeFailed, "missing required tool call: "+targetTool, schema.ApprovalOutcomeNA, ""))
			}

			content := "I ran into an internal error while generating a reply. Please check the NAVI server logs."
			msg := err.Error()
			if strings.Contains(msg, "no provider configured") {
				content = "LLM provider is not configured. Run navi init or open /onboarding to configure an LLM provider."
			}
			if l.cfg.Debug && msg != "" {
				content = content + "\n\n(Error: " + msg + ")"
			}
			final := l.shapeReply(content, experienceMode)
			l.recordFallbackCompletion(ctx, compiledReq, orchestrationtrace.StageModelCalled, err, len(final))
			return finalizeInference(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, snapshotForRunCompletion(schema.ExecutionOutcomeFailed, msg, schema.ApprovalOutcomeNA, ""))
		}

		normalized, normErr := l.normalizeRunModelResponse(ctx, compiledReq, resp)
		if normErr != nil {
			return nil, normErr
		}
		normalized = canonicalizeRenderToolCalls(normalized, compiledReq, lastSessionUserContent(session))

		toolAttempts := make([]inference.ToolAuthorizationInput, 0, len(normalized.ToolCalls))
		for _, tc := range normalized.ToolCalls {
			toolAttempts = append(toolAttempts, l.buildToolAuthorizationAttempt(input.Run, decision, tc, input.InboxItem, scheduled))
		}

		decision, err = controller.AuthorizeModelResponse(ctx, decision, inference.ModelResponseAuthorizationInput{
			Compiled:                compiledReq,
			Response:                normalized,
			ToolAttempts:            toolAttempts,
			ToolRegistry:            mustToolRegistry(l),
			ActiveToolSet:           surfaceResult.ActiveToolSet,
			RepairAttemptsRemaining: hallucinationRetries,
		})
		if err != nil {
			return nil, fmt.Errorf("navi: runtime inference model response authorization: %w", err)
		}
		currentDecision = decision
		if err := l.applyInferenceEnvelope(ctx, input.Run, nil, decision); err != nil {
			return nil, err
		}
		if decision.RuntimeDisposition == inference.RuntimeDispositionPauseForProposal {
			proposalID, proposalReason := "", decision.ReplyMessage
			if decision.Proposal != nil {
				proposalID = decision.Proposal.ProposalID
				proposalReason = firstNonEmpty(strings.TrimSpace(decision.Proposal.Rationale), proposalReason)
			}
			cp := naviruntime.NewCheckpoint(input.Run)
			cp.ExperienceMode = string(experienceMode)
			cp.LLMMessages = append([]llm.Message(nil), composeRuntimeMessages(baseMessages, tailMessages)...)
			cp.Options = opts
			cp.PendingProposalID = proposalID
			cp.PendingProposalReason = proposalReason
			if decision.PendingToolCall != nil {
				pending := *decision.PendingToolCall
				cp.PendingToolCall = &pending
			}
			input.Run.LatestCheckpointID = cp.CheckpointID
			snapshot := snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, proposalReason, schema.ApprovalOutcomeNA, proposalID)
			snapshot.CheckpointRef = cp.CheckpointID
			pausedDecision := decision
			supervisedPause, err := superviseOutcome(pausedDecision, snapshot)
			if err != nil {
				return nil, err
			}
			pausedDecision = mergePausedDecision(supervisedPause, pausedDecision)
			if err := l.applyInferenceEnvelope(ctx, input.Run, nil, pausedDecision); err != nil {
				return nil, err
			}
			currentDecision = pausedDecision
			return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
				Run:             input.Run,
				Paused:          true,
				Checkpoint:      cp,
				ProposalID:      proposalID,
				ProposalReason:  proposalReason,
				PendingToolCall: cp.PendingToolCall,
				ProposalArgs:    pendingToolArguments(pausedDecision),
			}, pausedDecision, nil)
		}
		if decision.RuntimeDisposition == inference.RuntimeDispositionBlockWithReply {
			reason := firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), "ICS blocked this run")
			if result, err, ok := l.executeRenderIntentFallback(ctx, input.Run, chatID, experienceMode, thread, surfacedCompiledReq, lastSessionUserContent(session), "render_intent_model_block_fallback", reason); ok {
				return result, err
			}
			l.emitGovernanceBlockedDecision(ctx, input.Run, decision, toolCallNames(normalized.ToolCalls))

			if len(normalized.ToolCalls) > 0 && hallucinationRetries > 0 && !l.cfg.SkipHallucinationRetries {
				hallucinationRetries--
				slog.Debug("navi: reflecting governance rejection back to model", "run_id", input.Run.RunID, "retries_left", hallucinationRetries, "reason", reason)
				assistantMsg := llm.Message{
					Role:      "assistant",
					Content:   normalized.Content,
					ToolCalls: normalized.ToolCalls,
				}
				tailMessages = append(tailMessages, assistantMsg)
				recoveryPrompts := toolRecoveryPrompts(decision)
				for _, tc := range normalized.ToolCalls {
					repairPrompt := firstNonEmpty(recoveryPrompts[strings.TrimSpace(tc.ID)], recoveryPrompts[strings.TrimSpace(tc.Name)], "Action Rejected: "+reason+" - Please reply directly to the user without this action or pick a valid authorized tool.")
					tailMessages = append(tailMessages, llm.Message{
						Role:       "tool",
						Content:    repairPrompt,
						ToolCallID: tc.ID,
					})
				}
				llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
				continue
			}

			final := l.shapeReply("I couldn't continue because "+reason+".", experienceMode)
			l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
				"reply_length": strconv.Itoa(len(final)),
				"tool_calls":   strconv.Itoa(len(normalized.ToolCalls)),
				"completion":   "ics_model_response_blocked",
			})
			if err := checkInterrupt(); err != nil {
				return nil, err
			}
			return finalizeInference(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, snapshotForRunCompletion(schema.ExecutionOutcomeRejectedPreExecution, reason, schema.ApprovalOutcomeNA, proposalIDFromDecision(decision)))
		}

		if len(normalized.ToolCalls) == 0 {
			if shouldAttachRenderPayloadForMissingToolCall(surfacedCompiledReq, lastSessionUserContent(session)) {
				classification := render.Classification{
					Intent:        render.RenderIntentOpenUIDataRender,
					Capability:    "tool_usage",
					PreferredView: runtimeRenderPreferredView(lastSessionUserContent(session)),
				}
				l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
					"tool_calls":  "0",
					"completion":  "render_intent_missing_tool_call_fallback",
					"target_tool": renderVisualizeToolName,
				})
				if err := checkInterrupt(); err != nil {
					return nil, err
				}
				return l.executeDataDrivenRender(ctx, input.Run, chatID, experienceMode, thread, classification)
			}
			if shouldRecoverMissingRequiredToolCall(compiledReq, decision) {
				targetTool := missingRequiredToolCallTarget(compiledReq, decision)
				if hallucinationRetries > 0 && !l.cfg.SkipHallucinationRetries {
					hallucinationRetries--
					slog.Debug("navi: retrying missing required tool call",
						"run_id", input.Run.RunID,
						"target_tool", targetTool,
						"retries_left", hallucinationRetries,
					)
					if content := strings.TrimSpace(normalized.Content); content != "" {
						tailMessages = append(tailMessages, llm.Message{Role: "assistant", Content: content})
					}
					tailMessages = append(tailMessages, llm.Message{
						Role:    "user",
						Content: missingRequiredToolCallRepairPrompt(targetTool),
					})
					llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
					continue
				}

				var sourceChannel string
				if input.Run != nil && input.Run.Scratchpad != nil {
					sourceChannel = input.Run.Scratchpad["source_channel"]
				}
				finalContent := missingRequiredToolCallFallbackReply(targetTool, sourceChannel)
				final := l.shapeReply(finalContent, experienceMode)
				l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
					"reply_length": strconv.Itoa(len(final)),
					"tool_calls":   "0",
					"completion":   "missing_required_tool_call_fallback",
					"target_tool":  targetTool,
				})
				if err := checkInterrupt(); err != nil {
					return nil, err
				}
				return finalizeInference(&naviruntime.ExecuteResult{
					Run:            input.Run,
					Completed:      true,
					FinalContent:   final,
					ReplyLen:       len(final),
					ExperienceMode: string(experienceMode),
				}, snapshotForRunCompletion(schema.ExecutionOutcomeFailed, "missing required tool call: "+targetTool, schema.ApprovalOutcomeNA, ""))
			}

			if len(scheduled) > 0 {
				l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
					"scheduled_messages": strconv.Itoa(len(scheduled)),
				})
				return finalizeInference(&naviruntime.ExecuteResult{
					Run:               input.Run,
					Completed:         true,
					ScheduledMessages: scheduled,
					ExperienceMode:    string(experienceMode),
				}, snapshotForRunCompletion(schema.ExecutionOutcomeSucceeded, "scheduled messages queued", schema.ApprovalOutcomeNA, ""))
			}

			finalContent := trimRepeatedOutput(normalized.Content)
			if routingPrefix != "" {
				finalContent = routingPrefix + "\n\n" + finalContent
			}
			final := l.shapeReply(finalContent, experienceMode)
			if isStaleResponseRepeat(final, session) {
				if staleResponseGuardRetried {
					finalContent = "I couldn't generate a fresh reply without repeating the previous response. Please send a little more detail and I can try again."
					final = l.shapeReply(finalContent, experienceMode)
					l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
						"reply_length": strconv.Itoa(len(final)),
						"tool_calls":   strconv.Itoa(len(normalized.ToolCalls)),
						"completion":   "stale_response_guard_exhausted",
					})
					if err := checkInterrupt(); err != nil {
						return nil, err
					}
					return finalizeInference(&naviruntime.ExecuteResult{
						Run:            input.Run,
						Completed:      true,
						FinalContent:   final,
						ReplyLen:       len(final),
						ExperienceMode: string(experienceMode),
					}, snapshotForRunCompletion(schema.ExecutionOutcomeFailed, "stale response guard exhausted", schema.ApprovalOutcomeNA, ""))
				}
				slog.Warn("navi: stale response guard triggered — reply matches last assistant message; retrying",
					"chat_id", chatID,
					"content_prefix", final[:min(80, len(final))],
				)
				// Inject an explicit freshness directive and retry exactly once.
				staleResponseGuardRetried = true
				tailMessages = append(tailMessages, llm.Message{Role: "assistant", Content: normalized.Content})
				tailMessages = append(tailMessages, llm.Message{Role: "user", Content: "Your previous message was already delivered to the user. Please generate a fresh, distinct reply to the most recent message in the conversation."})
				llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
				continue
			}
			if artifactNote, err := l.maybeAutoMaterializeChatReply(ctx, input.Run, thread, finalContent); err != nil {
				logAutoArtifactFailure(err)
			} else if artifactNote != "" {
				finalContent = strings.TrimSpace(finalContent + "\n\n" + artifactNote)
				final = l.shapeReply(finalContent, experienceMode)
			}
			l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
				"reply_length": strconv.Itoa(len(final)),
				"tool_calls":   strconv.Itoa(len(normalized.ToolCalls)),
			})
			if err := checkInterrupt(); err != nil {
				return nil, err
			}
			return finalizeInference(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, snapshotForRunCompletion(schema.ExecutionOutcomeSucceeded, finalContent, schema.ApprovalOutcomeNA, ""))
		}

		assistantMsg := llm.Message{
			Role:      "assistant",
			Content:   normalized.Content,
			ToolCalls: normalized.ToolCalls,
		}
		tailMessages = append(tailMessages, assistantMsg)
		llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)

		var routerResults []struct{ Name, Content string }
		authorizedTools := append([]inference.AuthorizedToolInvocation(nil), decision.AuthorizedTools...)
		for _, authorized := range authorizedTools {
			if err := checkInterrupt(); err != nil {
				return nil, err
			}
			l.transitionPhase(ctx, input.Run, naviruntime.RunPhaseValidateGovern)
			tc := authorized.ToolCall
			executionDecision := decision
			executionDecision.AuthorizedToolCalls = []llm.ToolCall{tc}
			executionDecision.AuthorizedTools = []inference.AuthorizedToolInvocation{authorized}
			permit := authorized.Permit
			executionDecision.ToolPermit = &permit
			toolResult, pause, snapshot, execErr := l.executeToolForRun(ctx, input.Run, executionDecision, tc, input.InboxItem, "", &scheduled)

			if pause != nil {
				cp := naviruntime.NewCheckpoint(input.Run)
				cp.ExperienceMode = string(experienceMode)
				cp.LLMMessages = append([]llm.Message(nil), llmMsgs...)
				cp.Options = opts
				cp.PendingToolCall = &pause.ToolCall
				cp.PendingProposalID = pause.ProposalID
				cp.PendingProposalReason = pause.Reason
				input.Run.LatestCheckpointID = cp.CheckpointID
				if snapshot != nil {
					snapshot.CheckpointRef = cp.CheckpointID
				}
				decision, err = superviseOutcome(decision, snapshot)
				if err != nil {
					return nil, err
				}
				return finalizeResultWithOutcome(&naviruntime.ExecuteResult{
					Run:             input.Run,
					Paused:          true,
					Checkpoint:      cp,
					ProposalID:      pause.ProposalID,
					ProposalReason:  pause.Reason,
					PendingToolCall: &pause.ToolCall,
					ProposalArgs:    pause.Arguments,
				}, decision, nil)
			}
			decision, err = superviseOutcome(decision, snapshot)
			if err != nil {
				return nil, err
			}
			if execErr != nil {
				return nil, execErr
			}

			l.emitToolLifecycle(ctx, input.Run, toolLifecycleCall(tc, snapshot), toolResult)

			if isRouterStateTool(tc.Name) {
				routerResults = append(routerResults, struct{ Name, Content string }{tc.Name, toolResult.Content})
			}
			tailMessages = append(tailMessages, llm.Message{
				Role:       "tool",
				Content:    toolResult.FormatForPrompt(),
				ToolCallID: tc.ID,
			})
			llmMsgs = composeRuntimeMessages(baseMessages, tailMessages)
		}

		if len(scheduled) > 0 {
			l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
				"scheduled_messages": strconv.Itoa(len(scheduled)),
			})
			return finalizeInference(&naviruntime.ExecuteResult{
				Run:               input.Run,
				Completed:         true,
				ScheduledMessages: scheduled,
				ExperienceMode:    string(experienceMode),
			}, snapshotForRunCompletion(schema.ExecutionOutcomeSucceeded, "scheduled messages queued", schema.ApprovalOutcomeNA, ""))
		}

		if len(routerResults) == len(decision.AuthorizedToolCalls) && len(routerResults) > 0 {
			content := formatRouterStructuredReply(routerResults)
			final := l.shapeReply(content, experienceMode)
			l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiledReq, map[string]string{
				"reply_length": strconv.Itoa(len(final)),
				"tool_calls":   strconv.Itoa(len(decision.AuthorizedToolCalls)),
			})
			if err := checkInterrupt(); err != nil {
				return nil, err
			}
			return finalizeInference(&naviruntime.ExecuteResult{
				Run:            input.Run,
				Completed:      true,
				FinalContent:   final,
				ReplyLen:       len(final),
				ExperienceMode: string(experienceMode),
			}, snapshotForRunCompletion(schema.ExecutionOutcomeSucceeded, content, schema.ApprovalOutcomeNA, ""))
		}
	}
}

func runtimeCheckpointMessageTail(checkpoint *naviruntime.Checkpoint, baseMessages []llm.Message) []llm.Message {
	if checkpoint == nil || len(checkpoint.LLMMessages) == 0 {
		return nil
	}
	if len(checkpoint.LLMMessages) <= len(baseMessages) {
		return nil
	}
	return append([]llm.Message(nil), checkpoint.LLMMessages[len(baseMessages):]...)
}

func composeRuntimeMessages(baseMessages, tailMessages []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(baseMessages)+len(tailMessages))
	out = append(out, baseMessages...)
	out = append(out, tailMessages...)
	return out
}

func syncCanonicalRunRequestFromRun(req orchestration.CanonicalRunRequest, run *naviruntime.RunState) orchestration.CanonicalRunRequest {
	req.Frame.Scratchpad = promptScratchpad(run)
	if req.Frame.TraceID == "" {
		if envelope := runtimeInferenceEnvelope(run, nil); envelope != nil {
			req.Frame.TraceID = strings.TrimSpace(envelope.Rationale.DecisionTrace.TraceID)
		}
	}
	return req
}

func cloneRunScratchpadMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func (l *AgentLoop) compileRunModelRequest(ctx context.Context, session *ChatRuntimeView, experienceMode ExperienceMode, lastUser string, run *naviruntime.RunState) (orchestration.CanonicalRunRequest, orchestration.CompiledModelRequest, orchestration.SurfaceResolutionResult, error) {
	pipeline := l.runtimeOrchestrationPipeline()
	if pipeline == nil {
		return orchestration.CanonicalRunRequest{}, orchestration.CompiledModelRequest{}, orchestration.SurfaceResolutionResult{}, fmt.Errorf("navi: runtime orchestration pipeline not configured")
	}
	reg, err := l.ensureToolRegistry()
	if err != nil {
		return orchestration.CanonicalRunRequest{}, orchestration.CompiledModelRequest{}, orchestration.SurfaceResolutionResult{}, fmt.Errorf("navi: ensure tool registry for active tool set: %w", err)
	}
	req, err := l.buildCanonicalRequestBaseForSurface(ctx, session, experienceMode, lastUser, run, runtimeToolSurface, orchestration.ExecutionModeRunExecute)
	if err != nil {
		return orchestration.CanonicalRunRequest{}, orchestration.CompiledModelRequest{}, orchestration.SurfaceResolutionResult{}, err
	}
	req, surfaceResult, err := l.resolveCapabilitySurface(ctx, req)
	if err != nil {
		return orchestration.CanonicalRunRequest{}, orchestration.CompiledModelRequest{}, orchestration.SurfaceResolutionResult{}, err
	}
	if surfaceResult.IsGuarded() {
		return req, orchestration.CompiledModelRequest{}, surfaceResult, nil
	}
	req = l.applyRoutingToCanonicalRequest(ctx, req)
	if run != nil {
		run.SetScratchpadValue("environment", runtimeEnvironmentForRequest(run, req))
		run.SetScratchpadValue("session_mode", string(runtimeDiscoverySessionMode(experienceMode)))
		run.SetScratchpadValue("tool_authority", string(l.runtimeToolAuthority(ctx, firstNonEmpty(sessionIDFromSession(session), strings.TrimSpace(req.Frame.ChatID)))))
	}
	if strings.EqualFold(strings.TrimSpace(req.Model.Metadata["routing_guard"]), string(orchestration.SurfaceGuardToolCapableModelRequired)) {
		surfaceResult.Guard = orchestration.SurfaceGuardToolCapableModelRequired
		surfaceResult.GuardReason = firstNonEmpty(
			strings.TrimSpace(req.Model.Metadata["routing_guard_reason"]),
			surfaceResult.GuardReason,
			toolCapableModelRequiredGuardReason,
		)
		return req, orchestration.CompiledModelRequest{}, surfaceResult, nil
	}
	if activeToolSet := l.buildProviderCallActiveToolSet(ctx, reg, session, experienceMode, lastUser, run, req); activeToolSet != nil {
		surfaceResult.ActiveToolSet = activeToolSet
		if req.Model.Metadata == nil {
			req.Model.Metadata = map[string]string{}
		}
		req.Model.Metadata["active_tool_set_id"] = strings.TrimSpace(activeToolSet.ActiveToolSetID)
		req.Model.Metadata["active_tool_set_scope"] = strings.TrimSpace(string(activeToolSet.Scope))
		req.Model.Metadata["active_tool_set_snapshot_id"] = strings.TrimSpace(activeToolSet.Provenance.RegistrySnapshotID)
	}
	_, _, compiled, err := pipeline.Compile(ctx, req)
	if err != nil {
		return orchestration.CanonicalRunRequest{}, orchestration.CompiledModelRequest{}, orchestration.SurfaceResolutionResult{}, fmt.Errorf("navi: compile NCOS run request: %w", err)
	}
	return req, compiled, surfaceResult, nil
}

func (l *AgentLoop) recompileRunModelRequest(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestration.CompiledModelRequest, error) {
	pipeline := l.runtimeOrchestrationPipeline()
	if pipeline == nil {
		return orchestration.CompiledModelRequest{}, fmt.Errorf("navi: runtime orchestration pipeline not configured")
	}
	_, _, compiled, err := pipeline.Compile(ctx, req)
	if err != nil {
		return orchestration.CompiledModelRequest{}, fmt.Errorf("navi: compile NCOS run request: %w", err)
	}
	return compiled, nil
}

func (l *AgentLoop) buildProviderCallActiveToolSet(ctx context.Context, reg *navitool.Registry, session *ChatRuntimeView, experienceMode ExperienceMode, lastUser string, run *naviruntime.RunState, req orchestration.CanonicalRunRequest) *navitool.ActiveToolSet {
	if reg == nil {
		return nil
	}
	discoveryCtx := l.runtimeDiscoveryContext(ctx, firstNonEmpty(sessionIDFromSession(session), strings.TrimSpace(req.Frame.ChatID)), experienceMode, run, req)
	broker := navitool.NewToolBrokerFromRegistry(reg)
	activeSet := broker.BuildActiveToolSet(
		navitool.BrokerInput{
			UserInput:     strings.TrimSpace(lastUser),
			SessionMode:   discoveryCtx.SessionMode,
			Environment:   discoveryCtx.Environment,
			UserAuthority: discoveryCtx.Authority,
			ModelProfile: navitool.BrokerModelProfile{
				Name:             firstNonEmpty(strings.TrimSpace(req.Model.Model), strings.TrimSpace(req.Model.Provider)),
				SupportsTools:    req.Model.SupportsTools,
				ToolCallReliable: req.Model.SupportsTools,
			},
		},
		navitool.BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: append([]string(nil), req.CapabilitySurface.ToolNames...),
			BrokerReason:    strings.TrimSpace(req.CapabilitySurface.SelectionReason),
		},
		navitool.ActiveToolSetSpec{
			Scope:        navitool.ActiveToolSetScopeProviderCall,
			ChatID:       firstNonEmpty(sessionIDFromSession(session), strings.TrimSpace(req.Frame.ChatID)),
			WorkflowID:   strings.TrimSpace(req.Frame.RunID),
			Mode:         string(NormalizeExperienceMode(experienceMode)),
			Environment:  discoveryCtx.Environment,
			ModelProfile: firstNonEmpty(strings.TrimSpace(req.Model.Model), strings.TrimSpace(req.Model.Provider)),
			ToolIDs:      append([]string(nil), req.CapabilitySurface.ToolNames...),
			LoadedAt:     time.Now().UTC(),
			LoadReason: firstNonEmpty(
				strings.TrimSpace(req.CapabilitySurface.SelectionReason),
				"provider_call_surface",
			),
			Constraints: navitool.ActiveToolSetConstraints{
				MaxToolsExposed:  len(compactRuntimeStrings(append([]string(nil), req.CapabilitySurface.ToolNames...))),
				MaxParallelCalls: 1,
				RiskCeiling:      "high",
				ConfirmationRules: []string{
					"respect_tool_governance_requires_confirm",
				},
			},
		},
	)
	return &activeSet
}

func sessionIDFromSession(session *ChatRuntimeView) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.ChatID)
}

func (l *AgentLoop) normalizeRunModelResponse(ctx context.Context, compiled orchestration.CompiledModelRequest, raw *llm.Response) (orchestration.NormalizedModelResponse, error) {
	pipeline := l.runtimeOrchestrationPipeline()
	if pipeline == nil {
		return orchestration.NormalizedModelResponse{}, fmt.Errorf("navi: runtime orchestration pipeline not configured")
	}
	resp, err := pipeline.Normalize(ctx, compiled, raw)
	if err != nil {
		return orchestration.NormalizedModelResponse{}, fmt.Errorf("navi: normalize NCOS run response: %w", err)
	}
	return resp, nil
}

func (l *AgentLoop) runtimeOrchestrationPipeline() *orchestration.Pipeline {
	if l == nil {
		return nil
	}
	if l.cfg.Orchestration != nil {
		return l.cfg.Orchestration
	}
	if l.cfg.NAVI != nil && l.cfg.NAVI.orchestration != nil {
		return l.cfg.NAVI.orchestration
	}
	return nil
}

func (l *AgentLoop) warnOnEmptyCompiledToolSurface(compiled orchestration.CompiledModelRequest) {
	if len(compiled.Tools) > 0 {
		return
	}
	requested := metadataInt(compiled.Metadata, "tool_count", 0)
	if requested <= 0 {
		return
	}
	slog.Warn(
		"navi: NCOS capability surface resolved to zero tools",
		"surface", strings.TrimSpace(compiled.Profile.Metadata["surface"]),
		"provider", strings.TrimSpace(compiled.Profile.Provider),
		"model", strings.TrimSpace(compiled.Profile.Model),
		"requested_tool_count", requested,
		"run_id", strings.TrimSpace(compiled.Metadata["run_id"]),
		"chat_id", strings.TrimSpace(compiled.Metadata["chat_id"]),
	)
}

func (l *AgentLoop) runtimeTraceSink() orchestration.TraceSink {
	sinks := make([]orchestration.TraceSink, 0, 2)
	if l == nil || l.cfg.Debug || l.cfg.WorldModel == nil {
		sinks = append(sinks, orchestrationtrace.NewLoggerSink(slog.Default().With("subsystem", "ncos")))
	}
	if l != nil && l.cfg.WorldModel != nil {
		sinks = append(sinks, orchestrationtrace.SinkFunc(func(ctx context.Context, event orchestration.TraceEvent) error {
			if strings.TrimSpace(event.ChatID) == "" {
				return nil
			}
			ownerID := ""
			if l.cfg.ResolveOwnerID != nil {
				ownerID = strings.TrimSpace(l.cfg.ResolveOwnerID(ctx, event.ChatID))
			}
			return l.cfg.WorldModel.RecordInteractionEvent(ctx, ownerID, event.ChatID, orchestrationtrace.InteractionMetadata(event))
		}))
	}
	return orchestrationtrace.NewMultiSink(sinks...)
}

func (l *AgentLoop) ensureCompiledTraceMetadata(compiled *orchestration.CompiledModelRequest, run *naviruntime.RunState, chatID string, mode orchestration.ExecutionMode) {
	if compiled == nil {
		return
	}
	if compiled.Metadata == nil {
		compiled.Metadata = map[string]string{}
	}
	if strings.TrimSpace(chatID) == "" && run != nil {
		chatID = run.RuntimeSessionID
	}
	runID := ""
	if run != nil {
		runID = run.RunID
	}
	if compiled.Metadata["chat_id"] == "" {
		compiled.Metadata["chat_id"] = strings.TrimSpace(chatID)
	}
	if compiled.Metadata["run_id"] == "" && strings.TrimSpace(runID) != "" {
		compiled.Metadata["run_id"] = strings.TrimSpace(runID)
	}
	compiled.Metadata["trace_id"] = orchestrationtrace.EnsureTraceID(
		compiled.Metadata["trace_id"],
		compiled.Metadata["run_id"],
		compiled.Metadata["chat_id"],
		string(mode),
	)
}

func (l *AgentLoop) recordCompiledTrace(ctx context.Context, stage string, compiled orchestration.CompiledModelRequest, meta map[string]string) {
	pipeline := l.runtimeOrchestrationPipeline()
	if pipeline == nil || pipeline.TraceSink == nil {
		return
	}
	_ = pipeline.TraceSink.Record(ctx, orchestration.TraceEvent{
		Stage:      stage,
		TraceID:    strings.TrimSpace(compiled.Metadata["trace_id"]),
		RunID:      strings.TrimSpace(compiled.Metadata["run_id"]),
		ChatID:     strings.TrimSpace(compiled.Metadata["chat_id"]),
		OccurredAt: time.Now().UTC(),
		Metadata:   meta,
	})
}

func mergeTraceMetadata(base map[string]string, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	for key, value := range extra {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func compiledModelRequestTraceStats(compiled orchestration.CompiledModelRequest) map[string]string {
	var messageChars int
	var systemChars int
	var userChars int
	var assistantChars int
	for _, msg := range compiled.Messages {
		chars := len(msg.Content)
		messageChars += chars
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			systemChars += chars
		case "assistant":
			assistantChars += chars
		default:
			userChars += chars
		}
	}

	stats := map[string]string{
		"message_count":     strconv.Itoa(len(compiled.Messages)),
		"message_chars":     strconv.Itoa(messageChars),
		"system_chars":      strconv.Itoa(systemChars),
		"user_chars":        strconv.Itoa(userChars),
		"tool_schema_chars": strconv.Itoa(toolDefinitionSchemaChars(compiled.Tools)),
	}
	if assistantChars > 0 {
		stats["assistant_chars"] = strconv.Itoa(assistantChars)
	}
	if compiled.Options.MaxTokens > 0 {
		stats["max_tokens"] = strconv.Itoa(compiled.Options.MaxTokens)
	}
	if compiled.Options.Temperature != 0 {
		stats["temperature"] = strconv.FormatFloat(compiled.Options.Temperature, 'f', -1, 64)
	}
	if compiled.Options.RepeatPenalty != 0 {
		stats["repeat_penalty"] = strconv.FormatFloat(compiled.Options.RepeatPenalty, 'f', -1, 64)
	}
	if compiled.Options.RepeatLastN > 0 {
		stats["repeat_last_n"] = strconv.Itoa(compiled.Options.RepeatLastN)
	}
	if compiled.Options.Think != nil {
		stats["think"] = strconv.FormatBool(*compiled.Options.Think)
	}
	return stats
}

func toolDefinitionSchemaChars(tools []llm.ToolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	payload, err := json.Marshal(tools)
	if err == nil {
		return len(payload)
	}
	var chars int
	for _, tool := range tools {
		chars += len(tool.Name) + len(tool.Description)
	}
	return chars
}

func (l *AgentLoop) recordCompiledFailure(ctx context.Context, compiled orchestration.CompiledModelRequest, stage string, err error) {
	if err == nil {
		return
	}
	l.recordCompiledTrace(ctx, orchestrationtrace.StageRunFailed, compiled, map[string]string{
		"failed_stage": stage,
		"error":        err.Error(),
	})
}

func (l *AgentLoop) recordFallbackCompletion(ctx context.Context, compiled orchestration.CompiledModelRequest, failedStage string, err error, replyLength int) {
	meta := map[string]string{
		"completion":   "fallback_reply",
		"reply_length": strconv.Itoa(replyLength),
	}
	if strings.TrimSpace(failedStage) != "" {
		meta["failed_stage"] = failedStage
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, compiled, meta)
}

func (l *AgentLoop) recordSurfaceGuardCompletion(ctx context.Context, run *naviruntime.RunState, chatID string, mode orchestration.ExecutionMode, result orchestration.SurfaceResolutionResult, replyLength int) {
	sink := l.runtimeTraceSink()
	if sink == nil {
		return
	}
	runID := ""
	if run != nil {
		runID = strings.TrimSpace(run.RunID)
		chatID = firstNonEmpty(strings.TrimSpace(run.RuntimeSessionID), strings.TrimSpace(chatID))
	}
	_ = sink.Record(ctx, orchestration.TraceEvent{
		Stage:      orchestrationtrace.StageRunCompleted,
		TraceID:    orchestrationtrace.EnsureTraceID("", runID, chatID, string(mode)),
		RunID:      runID,
		ChatID:     chatID,
		OccurredAt: time.Now().UTC(),
		Metadata: map[string]string{
			"completion":           "capability_surface_guard",
			"reply_length":         strconv.Itoa(replyLength),
			"surface_guard":        strings.TrimSpace(string(result.Guard)),
			"surface_guard_reason": strings.TrimSpace(result.GuardReason),
		},
	})
}

func (l *AgentLoop) recordDirectRunCompletion(ctx context.Context, run *naviruntime.RunState, chatID string, mode orchestration.ExecutionMode, meta map[string]string) {
	sink := l.runtimeTraceSink()
	if sink == nil {
		return
	}
	runID := ""
	if run != nil {
		runID = strings.TrimSpace(run.RunID)
		chatID = firstNonEmpty(strings.TrimSpace(run.RuntimeSessionID), strings.TrimSpace(chatID))
	}
	_ = sink.Record(ctx, orchestration.TraceEvent{
		Stage:      orchestrationtrace.StageRunCompleted,
		TraceID:    orchestrationtrace.EnsureTraceID("", runID, chatID, string(mode)),
		RunID:      runID,
		ChatID:     chatID,
		OccurredAt: time.Now().UTC(),
		Metadata:   cloneStringMap(meta),
	})
}

func (l *AgentLoop) executeBoundedDiagnosticQuery(ctx context.Context, run *naviruntime.RunState, chatID string, experienceMode ExperienceMode, query boundedDiagnosticQuery) (*naviruntime.ExecuteResult, error) {
	meta := map[string]string{
		"completion":             "bounded_diagnostic_query",
		"diagnostic_query_class": strings.TrimSpace(query.Class),
		"diagnostic_window":      boundedDiagnosticWindowLabel(query.Window),
	}

	content := ""
	if l.cfg.DB == nil {
		content = "Structured NAVI failure diagnostics are unavailable because the runtime error log is not configured."
		meta["diagnostic_source"] = "unavailable"
	} else {
		since := time.Now().UTC().Add(-query.Window)
		rows, err := store.ErrorSummary(ctx, l.cfg.DB, since)
		if err != nil {
			return nil, fmt.Errorf("navi: bounded diagnostics summary: %w", err)
		}
		items, err := store.ListErrorRecords(ctx, l.cfg.DB, store.ListErrorsFilter{Limit: boundedDiagnosticFetchLimit})
		if err != nil {
			return nil, fmt.Errorf("navi: bounded diagnostics recent errors: %w", err)
		}
		recent := filterRecentErrorRecords(items, since, boundedDiagnosticRecentLimit)
		meta["diagnostic_source"] = "error_log"
		meta["diagnostic_summary_rows"] = strconv.Itoa(len(rows))
		meta["diagnostic_recent_records"] = strconv.Itoa(len(recent))
		content = formatBoundedDiagnosticReply(rows, recent, query.Window)
	}

	final := l.shapeReply(content, experienceMode)
	meta["reply_length"] = strconv.Itoa(len(final))
	l.recordDirectRunCompletion(ctx, run, chatID, orchestration.ExecutionModeRunExecute, meta)
	return &naviruntime.ExecuteResult{
		Run:            run,
		Completed:      true,
		FinalContent:   final,
		ReplyLen:       len(final),
		ExperienceMode: string(experienceMode),
		Outcome:        schema.ExecutionOutcomeSucceeded,
		OutcomeSummary: content,
	}, nil
}

func (l *AgentLoop) executeBoundedScheduledTasksQuery(ctx context.Context, run *naviruntime.RunState, chatID string, experienceMode ExperienceMode) (*naviruntime.ExecuteResult, error) {
	meta := map[string]string{
		"completion": "bounded_scheduled_tasks_query",
	}

	content := ""
	if l.cfg.DB == nil {
		content = "Scheduled task inspection is unavailable because the runtime store is not configured."
		meta["scheduled_tasks_source"] = "unavailable"
	} else {
		jobs, err := store.ListCronJobs(ctx, l.cfg.DB)
		if err != nil {
			return nil, fmt.Errorf("navi: bounded scheduled tasks: %w", err)
		}
		meta["scheduled_tasks_source"] = "cron_jobs"
		meta["scheduled_tasks_count"] = strconv.Itoa(len(jobs))
		content = formatBoundedScheduledTasksReply(jobs)
	}

	final := l.shapeReply(content, experienceMode)
	meta["reply_length"] = strconv.Itoa(len(final))
	l.recordDirectRunCompletion(ctx, run, chatID, orchestration.ExecutionModeRunExecute, meta)
	return &naviruntime.ExecuteResult{
		Run:            run,
		Completed:      true,
		FinalContent:   final,
		ReplyLen:       len(final),
		ExperienceMode: string(experienceMode),
		Outcome:        schema.ExecutionOutcomeSucceeded,
		OutcomeSummary: content,
	}, nil
}

func (l *AgentLoop) buildCanonicalRequestBaseForSurface(ctx context.Context, session *ChatRuntimeView, experienceMode ExperienceMode, lastUser string, run *naviruntime.RunState, surface string, mode orchestration.ExecutionMode) (orchestration.CanonicalRunRequest, error) {
	experienceMode = NormalizeExperienceMode(experienceMode)
	userMessage := strings.TrimSpace(lastUser)
	if userMessage == "" && session != nil {
		userMessage = lastSessionUserContent(session)
	}
	if userMessage == "" && session != nil && len(session.Messages) == 0 {
		userMessage = l.defaultFirstTurnInstruction(experienceMode)
	}
	if action := strings.TrimSpace(runScratchpadValue(run, "chat_action")); action != "" {
		switch action {
		case "chat_continuation":
			prefix := strings.TrimSpace(runScratchpadValue(run, "chat_action_assistant_prefix"))
			userMessage = "Continue the previous assistant reply from exactly where it stopped. Do not restart, summarize, or repeat the existing text."
			if prefix != "" {
				userMessage += "\n\nExisting assistant text to continue:\n" + prefix
			}
		case "chat_regenerate_variant":
			if requested := strings.TrimSpace(runScratchpadValue(run, "chat_action_user_content")); requested != "" {
				userMessage = requested
			}
			userMessage = strings.TrimSpace(userMessage + "\n\nRegenerate a different assistant response to this request. Do not continue the previous assistant answer or mention that this is a variant.")
		}
	}
	chatID := ""
	runID := ""
	phase := ""
	scratchpad := map[string]string(nil)
	if run != nil {
		chatID = run.RuntimeSessionID
		runID = run.RunID
		phase = string(run.CurrentPhase)
		scratchpad = l.cloneRunScratchpad(run.Scratchpad)
	}
	if session != nil && strings.TrimSpace(session.ChatID) != "" {
		chatID = session.ChatID
	}
	selectedMessages := l.selectContextMessages(nil)
	totalMessages := 0
	if session != nil {
		selectedMessages = l.selectContextMessages(session.Messages)
		totalMessages = len(session.Messages)
	}
	allowToolCalls := runtimeShouldExposeToolsForTurn(userMessage, run, mode)
	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			TraceID:          orchestrationtrace.EnsureTraceID("", runID, chatID, string(mode)),
			ChatID:           chatID,
			RunID:            runID,
			CheckpointID:     runtimeCheckpointID(run),
			Phase:            phase,
			Mode:             mode,
			Scratchpad:       scratchpad,
			ResumeProposalID: runtimeResumeProposalID(run),
			ResumeReason:     runtimeResumeReason(run),
		},
		ExperienceMode: string(experienceMode),
		UserMessage:    userMessage,
		Conversation:   l.conversationTurnsFromMessages(selectedMessages),
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:             true,
			AllowToolCalls:        allowToolCalls,
			AllowScheduledReplies: allowToolCalls,
			ExpectStreaming:       true,
		},
		CapabilitySurface: orchestration.SkillSurfaceRef{
			Surface:         surface,
			SelectionReason: fmt.Sprintf("%s execution surface", surface),
		},
		Model: l.runtimeModelProfile(ctx, run, totalMessages, surface),
	}
	return req, nil
}

func runtimeShouldExposeToolsForTurn(userMessage string, run *naviruntime.RunState, mode orchestration.ExecutionMode) bool {
	if mode == orchestration.ExecutionModeResume {
		return true
	}
	if run != nil {
		if strings.TrimSpace(run.LatestCheckpointID) != "" ||
			strings.TrimSpace(run.BlockedOnProposalID) != "" ||
			strings.TrimSpace(run.PauseReason) != "" ||
			strings.TrimSpace(run.InterruptReason) != "" ||
			len(run.ICSDecisionEnvelope) > 0 {
			return true
		}
	}
	return runtimeLooksLikeActionIntent(userMessage)
}

func runScratchpadValue(run *naviruntime.RunState, key string) string {
	if run == nil || len(run.Scratchpad) == 0 {
		return ""
	}
	return strings.TrimSpace(run.Scratchpad[key])
}

func (l *AgentLoop) runtimeModelProfile(ctx context.Context, run *naviruntime.RunState, conversationTotal int, surface string) orchestration.ModelProfile {
	provider, modelName := l.activeLLMForIntrospection(ctx)
	if strings.TrimSpace(modelName) == "" {
		modelName = l.cfg.Model
	}
	profile := orchestration.ModelProfile{
		Provider:          strings.TrimSpace(provider),
		Model:             strings.TrimSpace(modelName),
		SupportsTools:     true,
		SupportsStreaming: false,
		Metadata: map[string]string{
			"surface": strings.TrimSpace(surface),
		},
	}
	if _, ok := l.cfg.LLM.(llm.StreamingProvider); ok {
		profile.SupportsStreaming = true
	}
	if run != nil {
		if profile.Metadata == nil {
			profile.Metadata = map[string]string{}
		}
		profile.Metadata["initiated_by_inbox_item_id"] = strings.TrimSpace(run.InitiatedByInboxItemID)
		profile.Metadata["interrupt_class"] = strings.TrimSpace(string(run.InterruptClass))
		profile.Metadata["interrupt_reason"] = strings.TrimSpace(run.InterruptReason)
		profile.Metadata["pause_reason"] = strings.TrimSpace(run.PauseReason)
		profile.Metadata["run_mode"] = strings.TrimSpace(string(run.Mode))
	}
	if conversationTotal > 0 {
		profile.Metadata["conversation_total_count"] = fmt.Sprintf("%d", conversationTotal)
	}
	return orchestrationmodel.ApplyProviderDefaults(profile)
}

func runtimeCheckpointID(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.LatestCheckpointID)
}

func runtimeResumeProposalID(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.BlockedOnProposalID)
}

func runtimeResumeReason(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(run.PauseReason), strings.TrimSpace(run.InterruptReason))
}

func (l *AgentLoop) runtimeSystemCoreInput(ctx context.Context, chatID string, profile orchestration.ModelProfile) orchestrationinstructions.SystemCoreInput {
	now, tzName := l.runtimeClockContext(ctx)
	pm := l.cfg.Prompts

	rulesData := prompts.NCOSRulesData{
		CurrentTime: now.Format("Monday, January 2, 2006 3:04 PM MST"),
	}
	opts := prompts.RenderOptions{}

	input := orchestrationinstructions.SystemCoreInput{
		IdentityRules:      loadRulesOrDefault(pm, prompts.KindNCOSIdentityRules, rulesData, opts),
		SessionContext:     "Active chat: " + strings.TrimSpace(chatID),
		ChatBehavior:       loadRulesOrDefault(pm, prompts.KindNCOSChatBehavior, rulesData, opts),
		DiagnosticsRules:   loadRulesOrDefault(pm, prompts.KindNCOSDiagnosticsRules, rulesData, opts),
		SecurityRules:      loadRulesOrDefault(pm, prompts.KindNCOSSecurityRules, rulesData, opts),
		ProviderStateRules: loadRulesOrDefault(pm, prompts.KindNCOSProviderStateRules, rulesData, opts),
		// Always-on guidance steering code-change requests through the
		// navi-programmer bounded-mutation workflow (normalize -> bind -> inspect
		// -> mutate -> validate -> commit). Guidance lives in the prompt, not Go.
		ToolUseRules: loadRulesOrDefault(pm, prompts.KindNCOSProgrammerWorkflow, rulesData, opts),
	}
	if tzName != "" {
		input.TimeContext = "Owner timezone: " + tzName
	}
	if orchestrationmodel.ProfileHasQuirk(profile, orchestrationmodel.QuirkPlainFunctionalStyle) {
		input.ModelStyleRules = loadRulesOrDefault(pm, prompts.KindNCOSModelStyleRules, rulesData, opts)
	}
	return input
}

func loadRulesOrDefault(pm *prompts.Manager, kind prompts.Kind, data any, opts prompts.RenderOptions) []string {
	if pm == nil {
		em, err := prompts.EmbeddedManager()
		if err != nil {
			return nil
		}
		pm = em
	}
	lines, err := pm.RenderLines(kind, data, opts)
	if err != nil || len(lines) == 0 {
		return nil
	}
	return lines
}

func (l *AgentLoop) runtimeClockContext(ctx context.Context) (time.Time, string) {
	loc := time.Local
	ownerTZ := ""
	if l.cfg.WorldModel != nil {
		ownerTZ = l.cfg.WorldModel.GetOwnerTimezone(ctx) // "" when not configured
		if ownerTZ != "" {
			loc = l.cfg.WorldModel.TimezoneLocation(ctx)
		}
	}
	now := time.Now().In(loc)
	// ownerTZ is only non-empty when the user explicitly configured a timezone.
	// When empty the time still reflects time.Local (host TZ via compose), but we
	// don't emit "Owner timezone: Local" which would be misleading.
	return now, ownerTZ
}

func (l *AgentLoop) buildRunExperienceOverlay(ctx context.Context, chatID string, mode ExperienceMode, lastUser string, conversation []orchestration.ConversationTurn, conversationTotal int) (string, error) {
	if l.cfg.ExperienceManager == nil {
		return "", nil
	}
	if strings.TrimSpace(lastUser) == "" {
		lastUser = lastConversationUserContent(conversation)
	}
	ownerID := ""
	if l.cfg.ResolveOwnerID != nil {
		ownerID = strings.TrimSpace(l.cfg.ResolveOwnerID(ctx, chatID))
	}
	snapshot := experience.ConfigurationSnapshot{}
	sessionOverrides, sessionTrace, sessionPrefs := experience.ExplicitTurnInput{}, experience.ExplicitTurnTrace{}, experience.OutputPreferences{}
	var sqlDB *sql.DB
	if l.cfg.WorldModel != nil {
		sqlDB = l.cfg.WorldModel.DB()
	}
	if sqlDB != nil {
		loaded, loadErr := experience.LoadConfigurationSnapshot(ctx, sqlDB, ownerID)
		if loadErr != nil {
			slog.Debug("navi: load experience configuration skipped", "chat_id", chatID, "owner_id", ownerID, "error", loadErr)
		} else {
			snapshot = loaded
		}
	}
	if sqlDB != nil {
		signals, signalErr := store.ListImmediatePreferenceSignalsBySession(ctx, sqlDB, chatID, 100)
		if signalErr != nil {
			slog.Debug("navi: load session preference signals skipped", "chat_id", chatID, "error", signalErr)
		} else {
			sessionOverrides, sessionTrace, sessionPrefs = experience.BuildSessionPreferenceOverrides(signals)
		}
	}
	rendered, err := l.cfg.ExperienceManager.Build(ctx, mode, experience.BuildRequest{
		ChatID:                   chatID,
		Mode:                     string(mode),
		LastUserMessage:          lastUser,
		CoreIdentity:             snapshot.CoreIdentity,
		PersonaModules:           snapshot.PersonaModules,
		RelationshipProfile:      snapshot.RelationshipProfile,
		OutputPreferences:        snapshot.OutputPreferences,
		ExplicitSessionOverrides: sessionOverrides,
		SessionOverrideTrace:     sessionTrace,
		SessionOutputPreferences: sessionPrefs,
	})
	if err != nil {
		return "", fmt.Errorf("navi: build experience control: %w", err)
	}
	if sqlDB != nil {
		sessionStart := conversationTotal <= 1
		if _, snapshotErr := experience.MaybeRecordExperienceSnapshot(ctx, sqlDB, rendered, experience.SnapshotRecordOptions{
			ChatID:         chatID,
			OwnerID:        ownerID,
			ExperienceMode: string(mode),
			SessionStart:   sessionStart,
		}); snapshotErr != nil {
			slog.Debug("navi: experience snapshot skipped", "chat_id", chatID, "owner_id", ownerID, "error", snapshotErr)
		}
	}
	return rendered.Fragment, nil
}

func (l *AgentLoop) conversationTurnsFromMessages(messages []ChatRuntimeMessage) []orchestration.ConversationTurn {
	out := make([]orchestration.ConversationTurn, 0, len(messages))
	for _, msg := range messages {
		out = append(out, orchestration.ConversationTurn{
			ID:          msg.ID,
			Role:        msg.Role,
			Content:     msg.Content,
			MessageKind: msg.MessageKind,
			CreatedAt:   msg.CreatedAt,
			Metadata: map[string]string{
				"inbox_item_id":      msg.InboxItemID,
				"run_id":             msg.RunID,
				"source_channel":     msg.SourceChannel,
				"source_message_ref": msg.SourceMessageRef,
			},
		})
	}
	return out
}

func (l *AgentLoop) conversationTurnsFromLLMMessages(messages []llm.Message) []orchestration.ConversationTurn {
	out := make([]orchestration.ConversationTurn, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		if role == "navi" {
			role = "assistant"
		}
		turn := orchestration.ConversationTurn{
			Role:    role,
			Content: m.Content,
		}
		if len(m.ToolCalls) > 0 {
			turn.MessageKind = "tool_call"
			if turn.Metadata == nil {
				turn.Metadata = map[string]string{}
			}
			var names []string
			for _, tc := range m.ToolCalls {
				names = append(names, tc.Name)
			}
			turn.Metadata["tool_calls"] = strings.Join(names, ",")
		}
		if m.ToolCallID != "" {
			turn.MessageKind = "tool_result"
			if turn.Metadata == nil {
				turn.Metadata = map[string]string{}
			}
			turn.Metadata["tool_call_id"] = m.ToolCallID
		}
		out = append(out, turn)
	}
	return out
}

func (l *AgentLoop) defaultFirstTurnInstruction(mode ExperienceMode) string {
	if NormalizeExperienceMode(mode) == ExperienceModeWizard {
		return "Begin Phase 1. Welcome me and ask if I am ready to start setup."
	}
	return "Please greet the user and briefly introduce yourself. Invite them to chat naturally; you are a chat assistant."
}

func (l *AgentLoop) cloneRunScratchpad(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func lastConversationUserContent(turns []orchestration.ConversationTurn) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(turns[i].Role), "user") && strings.TrimSpace(turns[i].Content) != "" {
			return turns[i].Content
		}
	}
	return ""
}

func lastSessionUserContent(session *ChatRuntimeView) string {
	if session == nil {
		return ""
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role == "user" && strings.TrimSpace(session.Messages[i].Content) != "" {
			return session.Messages[i].Content
		}
	}
	return ""
}

func metadataInt(meta map[string]string, key string, fallback int) int {
	if len(meta) == 0 {
		return fallback
	}
	raw := strings.TrimSpace(meta[key])
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func routingUserAnnouncement(profile orchestration.ModelProfile) string {
	if len(profile.Metadata) == 0 {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(profile.Metadata["routing_announcement_visibility"]), string(llm.RoutingVisibilityUser)) {
		return ""
	}
	return strings.TrimSpace(profile.Metadata["routing_announcement"])
}

func routingGuardReply(profile orchestration.ModelProfile) string {
	if len(profile.Metadata) == 0 {
		return ""
	}
	switch strings.TrimSpace(profile.Metadata["routing_guard"]) {
	case string(orchestration.SurfaceGuardToolCapableModelRequired), "no_tool_capable_model":
		return "This task needs reliable tool use, but I do not have a tool-capable model available right now. Switch to a tool-capable model and try again."
	default:
		return ""
	}
}

func capabilitySurfaceGuardReply(result orchestration.SurfaceResolutionResult) string {
	switch result.Guard {
	case orchestration.SurfaceGuardInvalidSurface:
		return "I couldn't safely resolve a valid tool surface for this run, so I stopped instead of widening tool access."
	case orchestration.SurfaceGuardEmptySurface:
		return "I couldn't safely expose any tools for this request, so I stopped instead of widening tool access."
	case orchestration.SurfaceGuardToolCapableModelRequired:
		return "This task needs reliable tool use, but I do not have a tool-capable model available right now. Switch to a tool-capable model and try again."
	default:
		return ""
	}
}

func (l *AgentLoop) transitionPhase(ctx context.Context, run *naviruntime.RunState, next naviruntime.RunPhase) {
	if run == nil || run.CurrentPhase == next {
		return
	}
	prev := run.CurrentPhase
	run.SetPhase(next)
	if l.cfg.StatusTracker != nil {
		state := schema.AgentStateProcessing
		if next == naviruntime.RunPhaseExecute {
			state = schema.AgentStateToolExecuting
		}
		l.cfg.StatusTracker.SetState(state, run.RuntimeSessionID, string(next))
	}
	ev := schema.NewRunEvent(
		schema.FactRunPhaseChanged,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.RunPhaseChangedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			Phase:            string(next),
			Previous:         string(prev),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

type streamState struct {
	builder     strings.Builder
	lastPartial time.Time
}

func newStreamState() *streamState {
	return &streamState{lastPartial: time.Now().UTC()}
}

func (s *streamState) append(delta string) {
	s.builder.WriteString(delta)
}

func (s *streamState) partial() string {
	if time.Since(s.lastPartial) < 200*time.Millisecond {
		return ""
	}
	s.lastPartial = time.Now().UTC()
	return s.builder.String()
}

func (l *AgentLoop) emitAssistantDelta(ctx context.Context, run *naviruntime.RunState, delta, partial string) {
	if delta == "" {
		return
	}
	visibility := l.eventVisibility(ctx, run.ChatID, run.RuntimeSessionID)
	if visibility == schema.VisibilityUser && l.userStreamChunkLooksInternal(run, delta) {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactAssistantTokenDelta,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		visibility,
		schema.AssistantTokenDeltaPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			Delta:            delta,
		},
	)
	l.appendRuntimeEvent(ctx, ev)
	if partial != "" {
		if visibility == schema.VisibilityUser && l.userStreamChunkLooksInternal(run, partial) {
			return
		}
		partialEv := schema.NewRunEvent(
			schema.FactAssistantMessagePartial,
			schema.EventKindFact,
			run.RuntimeSessionID,
			schema.AgentNavi,
			run.RunID,
			visibility,
			schema.AssistantMessagePartialPayload{
				RunID:            run.RunID,
				RuntimeSessionID: run.RuntimeSessionID,
				Content:          partial,
				State:            "streaming",
			},
		)
		l.appendRuntimeEvent(ctx, partialEv)
	}
}

func (l *AgentLoop) userStreamChunkLooksInternal(run *naviruntime.RunState, chunk string) bool {
	combined := strings.TrimSpace(chunk)
	if run != nil {
		prev := strings.TrimSpace(run.Scratchpad[streamLeakGuardScratchpadKey])
		combined = strings.TrimSpace(prev + " " + chunk)
		if len(combined) > streamLeakGuardWindow {
			combined = combined[len(combined)-streamLeakGuardWindow:]
		}
		run.SetScratchpadValue(streamLeakGuardScratchpadKey, combined)
	}
	return runtimeReasonLooksInternal(chunk) || runtimeReasonLooksInternal(combined)
}

func (l *AgentLoop) emitToolLifecycle(ctx context.Context, run *naviruntime.RunState, tc llm.ToolCall, result skill.DataEnvelope) {
	visibility := l.eventVisibility(ctx, run.ChatID, run.RuntimeSessionID)
	recordPart := visibility == schema.VisibilityUser && tc.Name != sendReplyToolName
	if strings.HasPrefix(result.Content, "Error:") || strings.HasPrefix(result.Content, "CRITICAL:") || strings.TrimSpace(result.Content) == runtimeGovernanceBlockedUserReply {
		errMsg := result.Content
		if visibility == schema.VisibilityUser {
			errMsg = sanitizeRuntimeUserFacingReply(errMsg)
		}
		if recordPart {
			run.RecordToolCallResult(tc.ID, tc.Name, toolPartPreview(errMsg), true)
		}
		failed := schema.NewRunEvent(
			schema.FactToolCallFailed,
			schema.EventKindFact,
			run.RuntimeSessionID,
			schema.AgentNavi,
			run.RunID,
			visibility,
			schema.ToolCallFailedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, CallID: tc.ID, ToolName: tc.Name, Error: errMsg},
		)
		l.appendRuntimeEvent(ctx, failed)
		return
	}
	if recordPart {
		run.RecordToolCallResult(tc.ID, tc.Name, toolPartPreview(result.Content), false)
	}
	completed := schema.NewRunEvent(
		schema.FactToolCallCompleted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		visibility,
		schema.ToolCallCompletedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, CallID: tc.ID, ToolName: tc.Name, Result: toolPartPreview(result.Content)},
	)
	l.appendRuntimeEvent(ctx, completed)
}

// toolPartPreview trims a tool result/error to a short single-line preview for
// the chat-UI tool chip.
func toolPartPreview(raw string) string {
	return trimBoundedDiagnosticLine(raw, 200)
}

func toolLifecycleCall(tc llm.ToolCall, snapshot *inference.ExecutionSnapshot) llm.ToolCall {
	if snapshot == nil {
		return tc
	}
	executed := strings.TrimSpace(snapshot.ExecutedCapability)
	if executed == "" || executed == strings.TrimSpace(tc.Name) {
		return tc
	}
	tc.Name = executed
	return tc
}

func (l *AgentLoop) emitToolProgress(ctx context.Context, run *naviruntime.RunState, tc llm.ToolCall, stage, message string, startedAt time.Time) {
	if run == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactToolCallProgress,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.ToolCallProgressPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			CallID:           tc.ID,
			ToolName:         tc.Name,
			Stage:            stage,
			Message:          message,
			DurationMs:       time.Since(startedAt).Milliseconds(),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

func (l *AgentLoop) emitGovernanceBlockedDecision(ctx context.Context, run *naviruntime.RunState, decision inference.DecisionEnvelope, toolNames []string) {
	if run == nil {
		return
	}
	userReason := firstNonEmpty(strings.TrimSpace(decision.ReplyMessage), strings.TrimSpace(decision.ExecutionBoundary.FailClosedReason))
	operatorReason := firstNonEmpty(strings.TrimSpace(decision.ExecutionBoundary.FailClosedReason), strings.TrimSpace(decision.ReplyMessage))
	if userReason == "" && operatorReason == "" {
		return
	}
	outcome := governor.ValidationRejected
	if decision.GovernanceResult != nil {
		outcome = decision.GovernanceResult.Outcome
	}
	names := compactStrings(append([]string(nil), toolNames...))
	if len(names) == 0 {
		names = compactStrings([]string{
			strings.TrimSpace(decision.ExecutionBoundary.TargetCapability),
			rationaleTargetCapability(decision.Rationale),
			strings.TrimSpace(decision.Rationale.ExecutionIntent.TargetCapability),
		})
	}
	if len(names) == 0 {
		names = []string{""}
	}
	for _, toolName := range names {
		l.emitGovernanceBlockedEvents(ctx, run, toolName, userReason, operatorReason, outcome)
	}
}

func (l *AgentLoop) emitGovernanceBlockedEvents(ctx context.Context, run *naviruntime.RunState, toolName, userReason, operatorReason string, outcome governor.ValidationOutcome) {
	if run == nil {
		return
	}
	l.appendRuntimeEvent(ctx, schema.NewRunEvent(
		schema.FactGovernanceBlocked,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityUser,
		schema.GovernanceBlockedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			ToolName:         toolName,
			Reason:           sanitizeRuntimeUserFacingReply(userReason),
			Outcome:          validationOutcomeName(outcome),
		},
	))
	l.appendRuntimeEvent(ctx, schema.NewRunEvent(
		schema.FactGovernanceBlocked,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.GovernanceBlockedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			ToolName:         toolName,
			Reason:           strings.TrimSpace(operatorReason),
			Outcome:          validationOutcomeName(outcome),
		},
	))
}

func (l *AgentLoop) appendRuntimeEvent(ctx context.Context, ev schema.Event) {
	if l.cfg.RuntimeStore != nil {
		if err := l.cfg.RuntimeStore.AppendRuntimeEvent(ctx, ev); err != nil {
			slog.Debug("navi: append runtime event failed", "type", ev.Type, "error", err)
		}
		return
	}
	if l.cfg.Bus != nil {
		_ = l.cfg.Bus.Publish(ctx, ev)
	}
}

// tryPromoteSendReplyToCron persists a delayed send_reply message to the
// core scheduler when the requested delay exceeds the in-process queue's
// cap. Returns (snapshot, true, nil) when the cron job was created and the
// caller should treat the send_reply as accepted; returns (nil, false, err)
// when promotion is not possible (cron not wired, no runtime session id, or the
// create failed) and the caller should fall back to its existing error
// path.
func (l *AgentLoop) tryPromoteSendReplyToCron(ctx context.Context, run *naviruntime.RunState, tc llm.ToolCall, content string, delaySec int, startedAt time.Time) (*inference.ExecutionSnapshot, bool, error) {
	if l.cfg.Cron == nil {
		return nil, false, nil
	}
	if run == nil || strings.TrimSpace(run.RuntimeSessionID) == "" {
		return nil, false, nil
	}
	if strings.TrimSpace(content) == "" {
		return nil, false, nil
	}
	fireAt := time.Now().UTC().Add(time.Duration(delaySec) * time.Second)
	job := cronsvc.Job{
		ID:          uuid.New().String(),
		Name:        fmt.Sprintf("send_reply runtime_session=%s +%ds", run.RuntimeSessionID, delaySec),
		Description: "Auto-promoted send_reply; durable delivery via cron.",
		Enabled:     true,
		Schedule: cronsvc.Schedule{
			Kind:  cronsvc.ScheduleAt,
			AtRFC: fireAt.Format(time.RFC3339),
		},
		SessionTarget:  cronsvc.SessionTarget(cronsvc.SessionTargetPrefix + run.RuntimeSessionID),
		WakeMode:       cronsvc.WakeNow,
		PayloadKind:    cronsvc.PayloadAssistantMessage,
		PayloadText:    content,
		DeleteAfterRun: true,
	}
	created, err := l.cfg.Cron.Create(ctx, job)
	if err != nil {
		return nil, false, fmt.Errorf("cron.Create: %w", err)
	}
	run.IncrementToolCalls()
	naviruntime.DefaultMetrics().RecordToolCall()
	summary := fmt.Sprintf("send_reply: scheduled for %s via core scheduler (job_id=%s)", fireAt.Format(time.RFC3339), created.ID)
	l.appendRuntimeEvent(ctx, schema.NewRunEvent(
		schema.FactToolCallStarted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		map[string]any{
			"tool":          sendReplyToolName,
			"call_id":       tc.ID,
			"promoted":      true,
			"cron_job_id":   created.ID,
			"delay_seconds": delaySec,
			"fire_at":       fireAt.Format(time.RFC3339),
			"chat_id":       run.RuntimeSessionID,
			"started_at_ms": startedAt.UnixMilli(),
		},
	))
	return &inference.ExecutionSnapshot{
		Outcome:            schema.ExecutionOutcomeSucceeded,
		ExecutedCapability: sendReplyToolName,
		Summary:            summary,
	}, true, nil
}

func (l *AgentLoop) executeToolForRun(ctx context.Context, run *naviruntime.RunState, decision inference.DecisionEnvelope, tc llm.ToolCall, inboxItem *naviruntime.InboxItem, approvedProposalID string, scheduled *[]naviruntime.ScheduledMessage) (skill.DataEnvelope, *pendingProposal, *inference.ExecutionSnapshot, error) {
	return l.executeToolForRunInternal(ctx, run, decision, tc, inboxItem, approvedProposalID, scheduled, true)
}

func (l *AgentLoop) executeToolForRunInternal(ctx context.Context, run *naviruntime.RunState, decision inference.DecisionEnvelope, tc llm.ToolCall, inboxItem *naviruntime.InboxItem, approvedProposalID string, scheduled *[]naviruntime.ScheduledMessage, allowFallback bool) (skill.DataEnvelope, *pendingProposal, *inference.ExecutionSnapshot, error) {
	trace := naviruntime.ProgressTraceFromContext(ctx)
	if run != nil {
		if strings.TrimSpace(trace.RuntimeSessionID) == "" {
			trace.RuntimeSessionID = run.RuntimeSessionID
		}
		trace.RunID = run.RunID
	}
	toolTracer := naviruntime.NewProgressTracer("navi.execute_tool", trace)
	toolDone := toolTracer.StageStart(ctx, "navi.tool.execution", "tool_name", strings.TrimSpace(tc.Name), "tool_call_id", strings.TrimSpace(tc.ID))
	defer toolDone(nil)
	startedAt := time.Now().UTC()
	visibility := l.eventVisibility(ctx, run.ChatID, run.RuntimeSessionID)
	type toolKind int

	const (
		toolKindRouter toolKind = iota
		toolKindFile
		toolKindSendReply
		toolKindSkill
		toolKindPlugin
	)

	var (
		kind          toolKind
		cmdType       = schema.CommandTypeInvoke
		skillEntry    *skill.SkillEntry
		skillSpec     *skill.Interface
		registryEntry *navitool.Tool
	)

	if reg, err := l.ensureToolRegistry(); err == nil {
		if toolEntry, ok := reg.Lookup(tc.Name); ok {
			registryEntry = toolEntry
			if tc.Name == sendReplyToolName && scheduled != nil {
				kind = toolKindSendReply
			}
			if toolEntry.Source == navitool.ToolSourceSkill && l.cfg.Skills != nil {
				if entry, spec, found := l.cfg.Skills.FindTool(tc.Name); found {
					skillEntry = entry
					skillSpec = spec
					kind = toolKindSkill
				}
			}
		}
	}

	if tc.Name == sendReplyToolName && scheduled != nil && registryEntry == nil {
		kind = toolKindSendReply
	} else if strings.HasPrefix(tc.Name, "llm-router_") {
		kind = toolKindRouter
		cmdType = schema.CommandTypeQuery
	} else if filetools.IsFileTool(tc.Name) {
		kind = toolKindFile
		if tc.Name == filetools.WriteFileToolName {
			cmdType = schema.CommandTypeUpdate
		} else {
			cmdType = schema.CommandTypeQuery
		}
	} else if registryEntry == nil {
		if l.cfg.PluginRegistry != nil {
			for _, t := range l.cfg.PluginRegistry.ToolEntries() {
				if t.Definition.Name == tc.Name {
					kind = toolKindPlugin
					break
				}
			}
		}
		if kind != toolKindPlugin {
			kind = toolKindSkill
			var ok bool
			skillEntry, skillSpec, ok = l.cfg.Skills.FindTool(tc.Name)
			if !ok {
				return skill.SanitizeResult(tc.Name, fmt.Sprintf("Error: Tool not found: %s", tc.Name)), nil, &inference.ExecutionSnapshot{
					Outcome:            schema.ExecutionOutcomeFailed,
					FailureClass:       schema.FailureClassExecutionFailure,
					FailureCode:        string(navitool.ExecutionFailureCodeSelectedToolUnavailable),
					ExecutedCapability: strings.TrimSpace(tc.Name),
					Summary:            fmt.Sprintf("tool not found: %s", tc.Name),
				}, nil
			}
		}
	}

	permit := decisionToolPermit(decision, tc)
	userGovernanceBlock := runtimeGovernanceBlockedUserReply
	if permit == nil {
		reason := fmt.Sprintf("ICS permit for %q did not match the attempted runtime tool call", tc.Name)
		l.emitGovernanceBlockedEvents(ctx, run, tc.Name, userGovernanceBlock, reason, governor.ValidationRejected)
		return skill.SanitizeResult(tc.Name, userGovernanceBlock), nil, &inference.ExecutionSnapshot{
			Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
			FailureClass:       schema.FailureClassPolicyBlocked,
			FailureCode:        string(navitool.ExecutionFailureCodeGovernanceBlocked),
			ExecutedCapability: strings.TrimSpace(tc.Name),
			Summary:            reason,
		}, nil
	}
	if !toolPermitAllowsProposalLinkage(permit, approvedProposalID) {
		reason := fmt.Sprintf("ICS permit for %q did not match the verified proposal linkage required for resumed execution", tc.Name)
		l.emitGovernanceBlockedEvents(ctx, run, tc.Name, userGovernanceBlock, reason, governor.ValidationRejected)
		return skill.SanitizeResult(tc.Name, userGovernanceBlock), nil, &inference.ExecutionSnapshot{
			Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
			FailureClass:       schema.FailureClassPolicyBlocked,
			FailureCode:        string(navitool.ExecutionFailureCodeGovernanceBlocked),
			ExecutedCapability: strings.TrimSpace(tc.Name),
			Summary:            reason,
		}, nil
	}
	if permit.Contract == nil || strings.TrimSpace(permit.ContractID) == "" || strings.TrimSpace(permit.Contract.ToolName) != strings.TrimSpace(tc.Name) {
		reason := fmt.Sprintf("ICS did not provide an executable contract for runtime tool %q", tc.Name)
		l.emitGovernanceBlockedEvents(ctx, run, tc.Name, userGovernanceBlock, reason, governor.ValidationRejected)
		return skill.SanitizeResult(tc.Name, userGovernanceBlock), nil, &inference.ExecutionSnapshot{
			Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
			FailureClass:       schema.FailureClassPolicyBlocked,
			FailureCode:        string(navitool.ExecutionFailureCodeGovernanceBlocked),
			ExecutedCapability: strings.TrimSpace(tc.Name),
			Summary:            reason,
		}, nil
	}
	// Contract compilation may inspect registry metadata, but once a permit has
	// been issued runtime must consume execution semantics from permit.Contract.
	contract := permit.Contract
	cmdType = contract.CommandType
	l.emitToolProgress(ctx, run, tc, "governance_check", "ics permit matched for execution", startedAt)

	if kind == toolKindSendReply {
		content, _ := tc.Arguments["content"].(string)
		delaySec := 0
		if n, ok := tc.Arguments["delay_seconds"]; ok {
			switch v := n.(type) {
			case float64:
				delaySec = int(v)
			case int:
				delaySec = v
			}
		}
		if delaySec < 0 {
			delaySec = 0
		}
		if delaySec > maxScheduledDelaySec {
			// Attempt to durably promote the over-cap delay to the core
			// scheduler. The in-process queue cannot survive restart and is
			// capped at maxScheduledDelaySec; the cron service can.
			if snapshot, ok, promoteErr := l.tryPromoteSendReplyToCron(ctx, run, tc, content, delaySec, startedAt); ok {
				return skill.SanitizeResult(sendReplyToolName, snapshot.Summary), nil, snapshot, nil
			} else if promoteErr != nil {
				slog.WarnContext(ctx, "send_reply: cron promotion failed; falling back to redirect",
					"chat_id", run.RuntimeSessionID, "delay_seconds", delaySec, "error", promoteErr)
			}
			msg := fmt.Sprintf("delay_seconds cannot exceed %d (use core-scheduler schedule_task with kind=at or kind=every for longer horizons)", maxScheduledDelaySec)
			return skill.SanitizeResult(sendReplyToolName, "Error: "+msg), nil, &inference.ExecutionSnapshot{
				Outcome:            schema.ExecutionOutcomeFailed,
				FailureClass:       schema.FailureClassExecutionFailure,
				FailureCode:        string(navitool.ExecutionFailureCodePolicyRejected),
				ExecutedCapability: sendReplyToolName,
				Summary:            msg,
			}, nil
		}
		if len(*scheduled) >= maxScheduledMessages {
			msg := fmt.Sprintf("maximum %d scheduled messages per run", maxScheduledMessages)
			return skill.SanitizeResult(sendReplyToolName, "Error: "+msg), nil, &inference.ExecutionSnapshot{
				Outcome:            schema.ExecutionOutcomeFailed,
				FailureClass:       schema.FailureClassExecutionFailure,
				FailureCode:        string(navitool.ExecutionFailureCodePolicyRejected),
				ExecutedCapability: sendReplyToolName,
				Summary:            msg,
			}, nil
		}
		*scheduled = append(*scheduled, naviruntime.ScheduledMessage{
			Content: content,
			Delay:   time.Duration(delaySec) * time.Second,
		})
		run.IncrementToolCalls()
		naviruntime.DefaultMetrics().RecordToolCall()
		l.appendRuntimeEvent(ctx, schema.NewRunEvent(
			schema.FactToolCallStarted,
			schema.EventKindFact,
			run.RuntimeSessionID,
			schema.AgentNavi,
			run.RunID,
			visibility,
			schema.ToolCallStartedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, CallID: tc.ID, ToolName: tc.Name},
		))
		return skill.DataEnvelope{Content: "Queued."}, nil, &inference.ExecutionSnapshot{
			Outcome:            schema.ExecutionOutcomeSucceeded,
			ExecutedCapability: sendReplyToolName,
			CommandType:        cmdType,
			Summary:            "reply queued",
		}, nil
	}
	preparedArgs, err := prepareProgrammerToolArguments(l.cfg.WorkspaceDir, run, tc, registryEntry, skillEntry, skillSpec)
	if err != nil {
		return skill.SanitizeResult(tc.Name, "Error: "+err.Error()), nil, &inference.ExecutionSnapshot{
			Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
			FailureClass:       schema.FailureClassPolicyBlocked,
			FailureCode:        string(navitool.ExecutionFailureCodeGovernanceBlocked),
			ExecutedCapability: strings.TrimSpace(tc.Name),
			CommandType:        cmdType,
			Summary:            err.Error(),
		}, nil
	}
	tc.Arguments = preparedArgs

	var skillIDs []string
	if skillEntry != nil {
		skillIDs = []string{skillEntry.Skill.Name}
	}
	var connectorIDs []string
	if inboxItem != nil && inboxItem.SourceChannel != "" {
		connectorIDs = []string{inboxItem.SourceChannel}
	}
	run.IncrementToolCalls()
	naviruntime.DefaultMetrics().RecordToolCall()
	started := schema.NewRunEvent(
		schema.FactToolCallStarted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		visibility,
		schema.ToolCallStartedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, CallID: tc.ID, ToolName: tc.Name},
	)
	l.appendRuntimeEvent(ctx, started)
	if visibility == schema.VisibilityUser && tc.Name != sendReplyToolName {
		run.RecordToolCallStarted(tc.ID, tc.Name)
	}
	l.emitToolProgress(ctx, run, tc, "executing", "tool execution started", startedAt)

	executor := command.NewExecutor(l.cfg.SaveExecutionOutcome)
	execCtx := ctx
	execCtx = skill.WithExecutionContext(execCtx, skill.ExecutionContext{
		ChatID:       run.RuntimeSessionID,
		RunID:        run.RunID,
		WorkspaceDir: l.cfg.WorkspaceDir,
		SandboxProfileResolver: func(ctx context.Context, profileID string) (schema.SandboxProfile, error) {
			if l.cfg.DB == nil {
				return schema.SandboxProfile{}, errors.New("database not configured")
			}
			return store.GetSandboxProfile(ctx, l.cfg.DB, profileID)
		},
	})
	if registryEntry != nil && tc.Name == sendReplyToolName {
		execCtx = withScheduledMessages(execCtx, scheduled)
	}
	if registryEntry != nil && tc.Name == renderVisualizeToolName && run != nil {
		// Give the render executor the run context it needs to attach the
		// data-driven render payload to this reply (mirrors send_reply's sink).
		execCtx = withRenderToolSink(execCtx, &renderToolSink{
			ChatID:  run.ChatID,
			RunID:   run.RunID,
			Payload: &run.RenderPayload,
		})
	}
	res, execErr := executor.Execute(execCtx, command.Descriptor{
		Type:             cmdType,
		UserFacing:       true,
		RuntimeSessionID: run.RuntimeSessionID,
		RunID:            run.RunID,
		CorrelationID:    run.RuntimeSessionID,
		ParentRunID:      run.RunID,
		SkillIDs:         skillIDs,
		ConnectorIDs:     connectorIDs,
		ProposalID:       strings.TrimSpace(permit.ApprovedProposalID),
	}, func(execCtx context.Context) (any, error) {
		if registryEntry != nil && registryEntry.Executor != nil {
			result, err := registryEntry.Executor.Execute(execCtx, tc.Arguments)
			if err != nil {
				return nil, err
			}
			return result.Content, nil
		}
		switch kind {
		case toolKindRouter:
			switch {
			case strings.HasSuffix(tc.Name, "_list") && l.cfg.ListLLMs != nil:
				return l.cfg.ListLLMs(execCtx)
			case strings.HasSuffix(tc.Name, "_get_active") && l.cfg.GetActiveLLM != nil:
				provider, model, err := l.cfg.GetActiveLLM(execCtx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"provider": provider, "model": model}, nil
			case strings.HasSuffix(tc.Name, "_set_active") && l.cfg.SetActiveLLM != nil:
				provider, _ := tc.Arguments["provider"].(string)
				model, _ := tc.Arguments["model"].(string)
				providerOut, modelOut, err := l.cfg.SetActiveLLM(execCtx, provider, model)
				if err != nil {
					return nil, err
				}
				return map[string]any{"provider": providerOut, "model": modelOut}, nil
			default:
				return nil, fmt.Errorf("unknown router tool: %s", tc.Name)
			}
		case toolKindFile:
			var checker filetools.PathChecker
			if l.cfg.Governor != nil {
				checker = l.cfg.Governor
			}
			return filetools.Execute(execCtx, tc.Name, tc.Arguments, l.cfg.WorkspaceDir, checker)
		case toolKindSkill:
			return skill.Execute(execCtx, skillEntry, skillSpec, tc.Arguments)
		case toolKindPlugin:
			if !plugin.IsBuiltinExecutable(tc.Name) {
				return nil, fmt.Errorf("plugin tool %q has no executor", tc.Name)
			}
			switch tc.Name {
			case plugin.CalendarTimeWindowToolName:
				now := time.Now().UTC()
				return map[string]any{"start_utc": now.Format(time.RFC3339), "end_utc": now.Add(time.Hour).Format(time.RFC3339)}, nil
			default:
				return nil, fmt.Errorf("plugin tool %q has no executor", tc.Name)
			}
		default:
			return nil, fmt.Errorf("unknown tool kind for %s", tc.Name)
		}
	})
	if execErr != nil {
		failure := navitool.ClassifyExecutionFailure(execErr)
		if skillEntry != nil && skillEntry.Spec != nil && skillSpec != nil {
			slog.Warn("navi: tool execution failed",
				"tool", strings.TrimSpace(tc.Name),
				"skill_id", strings.TrimSpace(skillEntry.Spec.SkillID),
				"interface", strings.TrimSpace(skillSpec.Name),
				"failure_code", string(failure.Code),
				"failure_class", string(failure.FailureClass),
				"error", execErr,
			)
		}
		l.emitToolProgress(ctx, run, tc, "failed", execErr.Error(), startedAt)
		outcomeSummary := strings.TrimSpace(execErr.Error())
		snapshot := snapshotForToolExecutionFailure(tc.Name, cmdType, res, execErr, outcomeSummary)
		if allowFallback {
			if fallbackResult, fallbackPause, fallbackSnapshot, handled, fallbackErr := l.routeGovernedFallbackForRun(ctx, run, decision, tc, inboxItem, scheduled, snapshot, failure); fallbackErr != nil {
				return skill.DataEnvelope{}, nil, nil, fallbackErr
			} else if handled {
				return fallbackResult, fallbackPause, fallbackSnapshot, nil
			}
		}
		failureClass := snapshot.FailureClass
		irreversible := skillEntry != nil && skillEntry.Spec != nil && skillEntry.Spec.Effects.Reversibility == "irreversible"
		vis := schema.DegradationVisibilityFor(failureClass, true, irreversible)
		msg := userFacingToolFailureSummary(failure, outcomeSummary, l.cfg.Debug)
		switch vis {
		case schema.DegradationAdvisory:
			msg = "Note: " + msg
		case schema.DegradationDeferredRecovery:
			msg = "Recovery needed: " + msg
		default:
			msg = "Error: " + msg
		}
		degrade := schema.NewRunEvent(
			schema.FactDegradationNoted,
			schema.EventKindFact,
			run.RuntimeSessionID,
			schema.AgentNavi,
			run.RunID,
			schema.VisibilityUser,
			schema.DegradationNotedPayload{
				RunID:            run.RunID,
				RuntimeSessionID: run.RuntimeSessionID,
				DegradationType:  string(vis),
				Message:          sanitizeRuntimeUserFacingReply(msg),
				ToolName:         strings.TrimSpace(tc.Name),
				FailureCode:      string(failure.Code),
			},
		)
		l.appendRuntimeEvent(ctx, degrade)
		return skill.SanitizeResult(tc.Name, msg), nil, snapshot, nil
	}
	if err := l.captureProgrammerWorkflowEvidence(ctx, run, inboxItem, tc, registryEntry, skillEntry, skillSpec, res); err != nil {
		wrapped := fmt.Errorf("programmer workflow bridge: %w", err)
		l.emitToolProgress(ctx, run, tc, "failed", wrapped.Error(), startedAt)
		return skill.SanitizeResult(tc.Name, "Error: "+wrapped.Error()), nil, snapshotForToolExecutionFailure(tc.Name, cmdType, res, wrapped, wrapped.Error()), nil
	}
	l.emitToolProgress(ctx, run, tc, "completed", "tool execution completed", startedAt)
	return skill.SanitizeResult(tc.Name, toolResultString(res)), nil, &inference.ExecutionSnapshot{
		Outcome:            schema.ExecutionOutcomeSucceeded,
		ExecutedCapability: strings.TrimSpace(tc.Name),
		CommandType:        cmdType,
		Summary:            toolResultString(res),
	}, nil
}

func snapshotForToolExecutionFailure(toolName string, cmdType schema.CommandType, result any, execErr error, summary string) *inference.ExecutionSnapshot {
	failure := navitool.ClassifyExecutionFailure(execErr)
	outcome := failure.Outcome
	failureClass := failure.FailureClass
	failureCode := string(failure.Code)
	if outcome == "" {
		outcome = schema.ExecutionOutcomeFailed
	}
	switch {
	case errors.Is(execErr, context.DeadlineExceeded):
		outcome = schema.ExecutionOutcomeTimedOut
		failureClass = schema.FailureClassTimeout
		failureCode = string(navitool.ExecutionFailureCodeTransientTimeout)
	case errors.Is(execErr, context.Canceled):
		outcome = schema.ExecutionOutcomeCancelled
		if failureClass == "" {
			failureClass = schema.FailureClassPermissionDenial
		}
	}
	if composed := composeFailureResult(result); composed != nil {
		switch composed.Outcome {
		case schema.ExecutionOutcomePartiallySucceeded, schema.ExecutionOutcomeTimedOut, schema.ExecutionOutcomeCancelled:
			outcome = composed.Outcome
		}
		if strings.TrimSpace(composed.FailureReason) != "" {
			summary = strings.TrimSpace(composed.FailureReason)
		}
		if composed.Outcome == schema.ExecutionOutcomePartiallySucceeded {
			failureClass = schema.FailureClassPartialExecution
			failureCode = string(navitool.ExecutionFailureCodePartialFailure)
		}
		if composed.Outcome == schema.ExecutionOutcomeTimedOut {
			failureClass = schema.FailureClassTimeout
			failureCode = string(navitool.ExecutionFailureCodeTransientTimeout)
		}
	}
	if failureClass == "" {
		failureClass = schema.FailureClassExecutionFailure
	}
	if failureCode == "" {
		failureCode = string(navitool.ExecutionFailureCodeExecutionFailed)
	}
	return &inference.ExecutionSnapshot{
		Outcome:            outcome,
		FailureClass:       failureClass,
		FailureCode:        failureCode,
		ExecutedCapability: strings.TrimSpace(toolName),
		CommandType:        cmdType,
		Summary:            strings.TrimSpace(summary),
	}
}

func userFacingToolFailureSummary(failure navitool.ExecutionFailure, raw string, debug bool) string {
	raw = strings.TrimSpace(raw)
	summary := ""
	switch failure.Code {
	case navitool.ExecutionFailureCodeSelectedToolUnavailable:
		summary = "The selected tool is currently unavailable."
	case navitool.ExecutionFailureCodeSelectedActionMissing:
		summary = "The selected action is currently unavailable."
	case navitool.ExecutionFailureCodeConnectorUnavailable:
		summary = "The connector or upstream service is currently unavailable."
	case navitool.ExecutionFailureCodeAuthFailure:
		summary = "Authentication failed for the selected tool."
	case navitool.ExecutionFailureCodeSchemaMismatch:
		summary = "The tool request or response did not match the expected schema."
	case navitool.ExecutionFailureCodeAPIContractDrift:
		summary = "The integration returned an unexpected response format."
	case navitool.ExecutionFailureCodeTransientTimeout:
		summary = "The selected tool timed out."
	case navitool.ExecutionFailureCodeExecutionFailed:
		summary = "The selected tool failed during execution."
	case navitool.ExecutionFailureCodePartialFailure:
		summary = "The selected tool completed with partial failures."
	case navitool.ExecutionFailureCodeGovernanceBlocked, navitool.ExecutionFailureCodePolicyRejected:
		summary = raw
	}
	if strings.TrimSpace(summary) == "" {
		summary = "The selected tool failed."
	}
	if debug && raw != "" && raw != summary {
		return summary + " (" + raw + ")"
	}
	return summary
}

func (l *AgentLoop) routeGovernedFallbackForRun(ctx context.Context, run *naviruntime.RunState, decision inference.DecisionEnvelope, tc llm.ToolCall, inboxItem *naviruntime.InboxItem, scheduled *[]naviruntime.ScheduledMessage, originalSnapshot *inference.ExecutionSnapshot, failure navitool.ExecutionFailure) (skill.DataEnvelope, *pendingProposal, *inference.ExecutionSnapshot, bool, error) {
	if l == nil || run == nil || !failure.FallbackAllowed {
		return skill.DataEnvelope{}, nil, nil, false, nil
	}
	reg, err := l.ensureToolRegistry()
	if err != nil || reg == nil {
		return skill.DataEnvelope{}, nil, nil, false, nil
	}

	discovery := navitool.DiscoverFallbackCandidates(navitool.FallbackDiscoveryInput{
		Registry:          reg,
		FailedToolID:      strings.TrimSpace(tc.Name),
		FailedAction:      strings.TrimSpace(string(originalSnapshot.CommandType)),
		Failure:           failure,
		Intent:            fallbackIntentSummary(decision, tc),
		Arguments:         tc.Arguments,
		Context:           l.runtimeFallbackDiscoveryContext(ctx, run),
		MaximumRiskTier:   strings.TrimSpace(string(decision.Rationale.ExecutionIntent.RiskLevel)),
		SideEffectCeiling: append([]string(nil), decision.Rationale.ExecutionIntent.ExpectedSideEffects...),
	})

	plan, ok := governedFallbackPlanForRun(reg, discovery)
	if !ok {
		return skill.DataEnvelope{}, nil, nil, false, nil
	}

	trace := newFallbackExecutionTrace(tc.Name, originalSnapshot.CommandType, failure, plan.Candidate, "")
	return l.executeGovernedFallbackPlanForRun(ctx, run, decision, tc, inboxItem, scheduled, trace, plan)
}

type governedFallbackPlan struct {
	Candidate navitool.FallbackCandidate
	Tools     []*navitool.Tool
	Contracts []inference.ToolContract
}

func governedFallbackPlanForRun(reg *navitool.Registry, discovery navitool.FallbackDiscoveryResult) (governedFallbackPlan, bool) {
	if reg == nil {
		return governedFallbackPlan{}, false
	}
	candidates := append([]navitool.FallbackCandidate(nil), discovery.Candidates...)
	for _, preferMulti := range []bool{true, false} {
		for _, candidate := range candidates {
			if (len(candidate.ToolIDs) > 1) != preferMulti {
				continue
			}
			if len(candidate.ToolIDs) == 0 {
				continue
			}
			tools := make([]*navitool.Tool, 0, len(candidate.ToolIDs))
			contracts := make([]inference.ToolContract, 0, len(candidate.ToolIDs))
			valid := true
			for _, toolID := range candidate.ToolIDs {
				lookup, ok := reg.LookupExact(toolID)
				if !ok || lookup.Tool == nil {
					valid = false
					break
				}
				contract, ok := buildRuntimeToolContract(lookup.Tool)
				if !ok {
					valid = false
					break
				}
				tools = append(tools, lookup.Tool)
				contracts = append(contracts, contract)
			}
			if !valid || len(tools) == 0 {
				continue
			}
			return governedFallbackPlan{
				Candidate: candidate,
				Tools:     tools,
				Contracts: contracts,
			}, true
		}
	}
	return governedFallbackPlan{}, false
}

func fallbackDecisionEnvelope(prior inference.DecisionEnvelope, toolEntry *navitool.Tool, contract inference.ToolContract, candidate navitool.FallbackCandidate) (inference.DecisionEnvelope, orchestration.CompiledModelRequest) {
	toolID := strings.TrimSpace(toolEntry.ToolID)
	next := prior
	next.GovernanceResult = nil
	next.Proposal = nil
	next.RuntimeDisposition = inference.RuntimeDispositionCallModel
	next.ModelDirective = nil
	next.AuthorizedToolCalls = nil
	next.AuthorizedTools = nil
	next.ToolPermit = nil
	next.PendingToolCall = nil
	next.PendingToolPermit = nil
	next.ReplyMessage = ""
	next.ExecutionBoundary.AllowedCapabilities = []string{toolID}
	next.ExecutionBoundary.TargetCapability = toolID
	next.ExecutionBoundary.ToolChoice = toolID
	next.ExecutionBoundary.FailClosedReason = ""

	detail := inference.CapabilityAvailability{
		Name:                 toolID,
		Kind:                 firstNonEmpty(strings.TrimSpace(toolEntry.Governance.Domain), contract.Domain),
		SourceType:           contract.SourceType,
		Domain:               contract.Domain,
		Available:            true,
		Governed:             true,
		CommandType:          contract.CommandType,
		WorkspaceAction:      contract.WorkspaceAction,
		TargetPathArg:        contract.TargetPathArg,
		WorkspaceScopedPath:  contract.WorkspaceScopedPath,
		RequiresConfirmation: candidate.RequiresConfirmation || contract.RequiresConfirmation,
	}
	approvalRequirement := next.Rationale.Governance.ApprovalRequirement
	if candidate.RequiresConfirmation && approvalRequirement == inference.ApprovalRequirementNone {
		approvalRequirement = inference.ApprovalRequirementBlockingProposal
	}
	next.Rationale.Governance.TargetCapability = toolID
	next.Rationale.Governance.AllowedCapabilities = []string{toolID}
	next.Rationale.Governance.AllowedCapabilityDetails = []inference.CapabilityAvailability{detail}
	next.Rationale.Governance.CommandType = contract.CommandType
	next.Rationale.Governance.Domain = firstNonEmpty(contract.Domain, next.Rationale.Governance.Domain)
	next.Rationale.Governance.ApprovalRequirement = approvalRequirement
	next.Rationale.Governance.ConfirmationRequired = candidate.RequiresConfirmation
	next.Rationale.ExecutionIntent.TargetCapability = toolID
	next.Rationale.ExecutionIntent.AllowedCapabilities = []string{toolID}
	next.Rationale.ExecutionIntent.ActionType = string(contract.CommandType)
	next.Rationale.ChosenAction.TargetCapability = toolID
	next.Rationale.ChosenAction.AllowedCapabilities = []string{toolID}
	next.Rationale.DecisionTrace.TargetCapability = toolID
	next.Rationale.DecisionTrace.AllowedCapabilities = []string{toolID}

	return next, orchestration.CompiledModelRequest{
		Tools: []llm.ToolDefinition{toolEntry.Definition},
	}
}

func blockedFallbackSnapshot(toolName string, cmdType schema.CommandType, reason, proposalID string, trace *inference.FallbackExecutionTrace) *inference.ExecutionSnapshot {
	return &inference.ExecutionSnapshot{
		Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
		FailureClass:       schema.FailureClassPolicyBlocked,
		FailureCode:        string(navitool.ExecutionFailureCodeGovernanceBlocked),
		ApprovalOutcome:    schema.ApprovalOutcomeNA,
		ProposalID:         strings.TrimSpace(proposalID),
		ExecutedCapability: strings.TrimSpace(toolName),
		CommandType:        cmdType,
		Summary:            strings.TrimSpace(reason),
		Fallback:           trace,
	}
}

func (l *AgentLoop) executeGovernedFallbackPlanForRun(ctx context.Context, run *naviruntime.RunState, decision inference.DecisionEnvelope, originalCall llm.ToolCall, inboxItem *naviruntime.InboxItem, scheduled *[]naviruntime.ScheduledMessage, trace *inference.FallbackExecutionTrace, plan governedFallbackPlan) (skill.DataEnvelope, *pendingProposal, *inference.ExecutionSnapshot, bool, error) {
	controller := l.inferenceController()
	var finalResult skill.DataEnvelope
	var finalSnapshot *inference.ExecutionSnapshot

	for idx, fallbackTool := range plan.Tools {
		contract := plan.Contracts[idx]
		fallbackDecision, compiled := fallbackDecisionEnvelope(decision, fallbackTool, contract, plan.Candidate)
		prepared, err := controller.PrepareModelCall(ctx, fallbackDecision, inference.ModelCallPreparationInput{
			Compiled:            compiled,
			ExecutableToolNames: []string{fallbackTool.ToolID},
			ToolContracts: map[string]inference.ToolContract{
				fallbackTool.ToolID: contract,
			},
			ToolRegistry: mustToolRegistry(l),
		})
		if err != nil {
			return skill.DataEnvelope{}, nil, nil, false, err
		}
		if prepared.RuntimeDisposition == inference.RuntimeDispositionBlockWithReply {
			trace.Disposition = "blocked"
			reason := firstNonEmpty(strings.TrimSpace(prepared.ReplyMessage), "fallback preparation was blocked")
			snapshot := blockedFallbackSnapshot(fallbackTool.ToolID, contract.CommandType, reason, "", trace)
			l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, reason)
			return skill.SanitizeResult(originalCall.Name, "Error: "+reason), nil, snapshot, true, nil
		}

		fallbackCall := llm.ToolCall{
			ID:        fallbackToolCallID(originalCall.ID, fallbackTool.ToolID),
			Name:      fallbackTool.ToolID,
			Arguments: cloneToolArguments(originalCall.Arguments),
		}
		var scheduledCopy []naviruntime.ScheduledMessage
		if scheduled != nil {
			scheduledCopy = append([]naviruntime.ScheduledMessage(nil), (*scheduled)...)
		}
		attempt := l.buildToolAuthorizationAttempt(run, prepared, fallbackCall, inboxItem, scheduledCopy)
		if plan.Candidate.RequiresConfirmation {
			attempt.RequiresConfirmation = true
		}

		authorized, err := controller.AuthorizeModelResponse(ctx, prepared, inference.ModelResponseAuthorizationInput{
			Compiled: compiled,
			Response: orchestration.NormalizedModelResponse{
				Profile:   compiled.Profile,
				ToolCalls: []llm.ToolCall{fallbackCall},
			},
			ToolAttempts:  []inference.ToolAuthorizationInput{attempt},
			ToolRegistry:  mustToolRegistry(l),
			ActiveToolSet: nil,
		})
		if err != nil {
			return skill.DataEnvelope{}, nil, nil, false, err
		}

		switch authorized.RuntimeDisposition {
		case inference.RuntimeDispositionPauseForProposal:
			proposalID, proposalReason := "", authorized.ReplyMessage
			if authorized.Proposal != nil {
				proposalID = authorized.Proposal.ProposalID
				proposalReason = firstNonEmpty(strings.TrimSpace(authorized.Proposal.Rationale), proposalReason)
			}
			trace.Disposition = "proposal_required"
			pendingCall := fallbackCall
			if authorized.PendingToolCall != nil {
				pendingCall = *authorized.PendingToolCall
			}
			snapshot := blockedFallbackSnapshot(pendingCall.Name, contract.CommandType, proposalReason, proposalID, trace)
			l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, proposalReason)
			return skill.DataEnvelope{}, &pendingProposal{
				ProposalID: proposalID,
				Reason:     proposalReason,
				ToolCall:   pendingCall,
				Arguments:  pendingCall.Arguments,
			}, snapshot, true, nil
		case inference.RuntimeDispositionBlockWithReply:
			trace.Disposition = "blocked"
			reason := firstNonEmpty(strings.TrimSpace(authorized.ReplyMessage), "fallback authorization was blocked")
			snapshot := blockedFallbackSnapshot(fallbackTool.ToolID, contract.CommandType, reason, proposalIDFromDecision(authorized), trace)
			l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, reason)
			return skill.SanitizeResult(originalCall.Name, "Error: "+reason), nil, snapshot, true, nil
		case inference.RuntimeDispositionProceedDirect:
			result, pause, snapshot, execErr := l.executeToolForRunInternal(ctx, run, authorized, fallbackCall, inboxItem, "", scheduled, false)
			if snapshot == nil {
				snapshot = &inference.ExecutionSnapshot{
					Outcome:            schema.ExecutionOutcomeSucceeded,
					ExecutedCapability: fallbackTool.ToolID,
					CommandType:        contract.CommandType,
				}
			}
			snapshot.Fallback = trace
			if execErr != nil {
				trace.Disposition = "failed"
				l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, firstNonEmpty(strings.TrimSpace(snapshot.Summary), "fallback execution failed"))
				return result, pause, snapshot, true, execErr
			}
			if pause != nil {
				trace.Disposition = "proposal_required"
				l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, firstNonEmpty(strings.TrimSpace(snapshot.Summary), pause.Reason))
				return result, pause, snapshot, true, nil
			}
			if snapshot.Outcome != schema.ExecutionOutcomeSucceeded && snapshot.Outcome != schema.ExecutionOutcomePartiallySucceeded {
				trace.Disposition = "failed"
				l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, snapshot, firstNonEmpty(strings.TrimSpace(snapshot.Summary), "fallback execution failed"))
				return result, nil, snapshot, true, nil
			}
			finalResult = result
			finalSnapshot = snapshot
		default:
			return skill.DataEnvelope{}, nil, nil, false, nil
		}
	}

	if finalSnapshot == nil {
		return skill.DataEnvelope{}, nil, nil, false, nil
	}
	trace.Disposition = "executed"
	finalSnapshot.Fallback = trace
	l.emitFallbackDegradationEvent(ctx, run, originalCall.Name, trace, finalSnapshot, firstNonEmpty(strings.TrimSpace(finalSnapshot.Summary), "fallback execution completed"))
	return finalResult, nil, finalSnapshot, true, nil
}

func newFallbackExecutionTrace(toolName string, cmdType schema.CommandType, failure navitool.ExecutionFailure, candidate navitool.FallbackCandidate, disposition string) *inference.FallbackExecutionTrace {
	return &inference.FallbackExecutionTrace{
		OriginalToolName:    strings.TrimSpace(toolName),
		OriginalAction:      strings.TrimSpace(string(cmdType)),
		OriginalFailureCode: strings.TrimSpace(string(failure.Code)),
		CandidateID:         strings.TrimSpace(candidate.CandidateID),
		SelectedToolIDs:     compactStrings(append([]string(nil), candidate.ToolIDs...)),
		Disposition:         strings.TrimSpace(disposition),
	}
}

func (l *AgentLoop) emitFallbackDegradationEvent(ctx context.Context, run *naviruntime.RunState, originalToolName string, trace *inference.FallbackExecutionTrace, snapshot *inference.ExecutionSnapshot, message string) {
	if l == nil || run == nil || trace == nil {
		return
	}
	degradationType := string(schema.DegradationAdvisory)
	switch strings.TrimSpace(trace.Disposition) {
	case "proposal_required":
		degradationType = string(schema.DegradationDeferredRecovery)
	case "blocked", "failed":
		degradationType = string(schema.DegradationBlocking)
	}
	ev := schema.NewRunEvent(
		schema.FactDegradationNoted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.DegradationNotedPayload{
			RunID:               run.RunID,
			RuntimeSessionID:    run.RuntimeSessionID,
			DegradationType:     degradationType,
			Message:             strings.TrimSpace(message),
			RecoveryProposalID:  proposalIDFromSnapshot(snapshot),
			ToolName:            strings.TrimSpace(originalToolName),
			FailureCode:         strings.TrimSpace(trace.OriginalFailureCode),
			FallbackPath:        append([]string(nil), trace.SelectedToolIDs...),
			FallbackDisposition: strings.TrimSpace(trace.Disposition),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

func (l *AgentLoop) runtimeFallbackDiscoveryContext(ctx context.Context, run *naviruntime.RunState) navitool.DiscoveryContext {
	chatID := ""
	mode := ExperienceModeStandard
	if run != nil {
		chatID = strings.TrimSpace(run.RuntimeSessionID)
		mode = NormalizeExperienceMode(ExperienceMode(run.ExperienceMode))
	}
	return l.runtimeDiscoveryContext(ctx, chatID, mode, run, orchestration.CanonicalRunRequest{})
}

func (l *AgentLoop) runtimeDiscoveryContext(ctx context.Context, chatID string, experienceMode ExperienceMode, run *naviruntime.RunState, req orchestration.CanonicalRunRequest) navitool.DiscoveryContext {
	return navitool.DiscoveryContext{
		Environment: runtimeEnvironmentForRequest(run, req),
		SessionMode: runtimeDiscoverySessionMode(experienceMode),
		Authority:   l.runtimeToolAuthority(ctx, chatID),
	}
}

func runtimeEnvironmentForRequest(run *naviruntime.RunState, req orchestration.CanonicalRunRequest) string {
	if env := strings.TrimSpace(req.Model.Metadata["environment"]); env != "" {
		return strings.ToLower(env)
	}
	if run != nil {
		if env := strings.TrimSpace(run.Scratchpad["environment"]); env != "" {
			return strings.ToLower(env)
		}
	}
	return "production"
}

func runtimeDiscoverySessionMode(experienceMode ExperienceMode) navitool.DiscoverySessionMode {
	switch NormalizeExperienceMode(experienceMode) {
	case ExperienceModeWizard:
		return navitool.DiscoverySessionModeCompanion
	default:
		return navitool.DiscoverySessionModeAssistant
	}
}

func (l *AgentLoop) runtimeToolAuthority(ctx context.Context, runtimeSessionID string) navitool.ToolAuthority {
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if l != nil && l.cfg.ResolveToolAuthority != nil {
		if authority := l.cfg.ResolveToolAuthority(ctx, runtimeSessionID); authority != "" {
			return authority
		}
	}
	if l != nil && l.cfg.NAVI != nil && l.cfg.NAVI.isOwnerChatOrRuntimeSession(ctx, runtimeSessionID) {
		return navitool.ToolAuthorityOwner
	}
	type runtimeSessionKindLookup interface {
		LookupRuntimeSessionKind(ctx context.Context, runtimeSessionID string) (schema.RuntimeSessionKind, error)
	}
	if lookup, ok := l.cfg.RuntimeStore.(runtimeSessionKindLookup); ok {
		if kind, err := lookup.LookupRuntimeSessionKind(ctx, runtimeSessionID); err == nil && (kind == schema.RuntimeSessionKindInternal || runtimeSessionID == schema.HeartbeatAutoRuntimeSessionID) {
			return navitool.ToolAuthorityOwner
		}
	}
	if kind := schema.DefaultRuntimeSessionKindForID(runtimeSessionID); kind == schema.RuntimeSessionKindInternal || runtimeSessionID == schema.HeartbeatAutoRuntimeSessionID {
		return navitool.ToolAuthorityOwner
	}
	return navitool.ToolAuthorityUser
}

func fallbackIntentSummary(decision inference.DecisionEnvelope, tc llm.ToolCall) string {
	return firstNonEmpty(
		strings.TrimSpace(decision.Rationale.ExecutionIntent.TargetCapability),
		strings.TrimSpace(decision.Rationale.ChosenAction.TargetCapability),
		strings.TrimSpace(tc.Name),
	)
}

func fallbackToolCallID(toolCallID, toolName string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	toolName = strings.TrimSpace(toolName)
	if toolCallID == "" {
		return "fallback:" + toolName
	}
	return toolCallID + "/fallback/" + toolName
}

func cloneToolArguments(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func composeFailureResult(result any) *command.ComposeResult {
	switch typed := result.(type) {
	case command.ComposeResult:
		copy := typed
		return &copy
	case *command.ComposeResult:
		return typed
	default:
		return nil
	}
}

func validationOutcomeName(outcome governor.ValidationOutcome) string {
	switch outcome {
	case governor.ValidationApproved:
		return "approved"
	case governor.ValidationRequiresConfirmation:
		return "requires_confirmation"
	case governor.ValidationModified:
		return "modified"
	case governor.ValidationRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

func approvalOutcomeForApprovedProposal(proposalID string) schema.ApprovalOutcome {
	if strings.TrimSpace(proposalID) == "" {
		return schema.ApprovalOutcomeNA
	}
	return schema.ApprovalOutcomeAllowOnce
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toolCallNames(calls []llm.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return compactStrings(names)
}

func mustToolRegistry(loop *AgentLoop) *navitool.Registry {
	if loop == nil {
		return nil
	}
	reg, err := loop.ensureToolRegistry()
	if err != nil {
		return nil
	}
	return reg
}

func toolRecoveryPrompts(decision inference.DecisionEnvelope) map[string]string {
	raw := decision.Rationale.Extensions["tool_call_recovery"]
	switch typed := raw.(type) {
	case []navitool.ToolCallRecoveryResult:
		return toolRecoveryPromptMap(typed)
	case []any:
		out := make(map[string]string, len(typed))
		for _, item := range typed {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			prompt, _ := record["repair_prompt"].(string)
			if strings.TrimSpace(prompt) == "" {
				continue
			}
			if toolCallID, _ := record["tool_call_id"].(string); strings.TrimSpace(toolCallID) != "" {
				out[strings.TrimSpace(toolCallID)] = strings.TrimSpace(prompt)
			}
			if toolName, _ := record["tool_name"].(string); strings.TrimSpace(toolName) != "" {
				out[strings.TrimSpace(toolName)] = strings.TrimSpace(prompt)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func toolRecoveryPromptMap(results []navitool.ToolCallRecoveryResult) map[string]string {
	if len(results) == 0 {
		return nil
	}
	out := make(map[string]string, len(results)*2)
	for _, result := range results {
		prompt := strings.TrimSpace(result.RepairPrompt)
		if prompt == "" {
			continue
		}
		if toolCallID := strings.TrimSpace(result.ToolCallID); toolCallID != "" {
			out[toolCallID] = prompt
		}
		if toolName := strings.TrimSpace(result.ToolName); toolName != "" {
			out[toolName] = prompt
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldRecoverMissingRequiredToolCall(compiled orchestration.CompiledModelRequest, decision inference.DecisionEnvelope) bool {
	targetTool := missingRequiredToolCallTarget(compiled, decision)
	if targetTool == "" || !compiledToolSurfaceContains(compiled.Tools, targetTool) {
		return false
	}
	needs := profileNeedsMissingToolCallRecovery(compiled.Profile)
	if !needs {
		return false
	}
	return true
}

func missingRequiredToolCallTarget(compiled orchestration.CompiledModelRequest, decision inference.DecisionEnvelope) string {
	if decision.ModelDirective != nil && decision.ModelDirective.AllowToolCalls {
		if target := strings.TrimSpace(decision.ModelDirective.ToolChoice); target != "" {
			return target
		}
	}
	if choice := strings.TrimSpace(compiled.Options.ToolChoice); choice != "" {
		return choice
	}
	if decision.Rationale.DominantMode == inference.DecisionModeExecute || decision.Rationale.ExecutionIntent.AllowsCapabilityExecution() {
		targetCapability := strings.TrimSpace(decision.ExecutionBoundary.TargetCapability)
		if targetCapability != "" && compiledToolSurfaceContains(compiled.Tools, targetCapability) {
			return targetCapability
		}
	}
	return ""
}

func applyRenderToolRequirementForIntent(compiled orchestration.CompiledModelRequest, userMessage string) orchestration.CompiledModelRequest {
	if !runtimeLooksLikeRenderIntent(userMessage) {
		return compiled
	}
	if !compiledToolSurfaceContains(compiled.Tools, renderVisualizeToolName) {
		return compiled
	}
	if strings.TrimSpace(compiled.Options.ToolChoice) != "" {
		return compiled
	}
	compiled.Options.ToolChoice = renderVisualizeToolName
	if compiled.Metadata == nil {
		compiled.Metadata = map[string]string{}
	}
	compiled.Metadata["runtime_required_tool"] = renderVisualizeToolName
	compiled.Metadata["runtime_required_tool_reason"] = "render_intent"
	return compiled
}

func shouldAttachRenderPayloadForMissingToolCall(compiled orchestration.CompiledModelRequest, userMessage string) bool {
	return runtimeLooksLikeRenderIntent(userMessage) && compiledToolSurfaceContains(compiled.Tools, renderVisualizeToolName)
}

func (l *AgentLoop) executeRenderIntentFallback(ctx context.Context, run *naviruntime.RunState, chatID string, experienceMode ExperienceMode, thread *ChatThread, surfaced orchestration.CompiledModelRequest, userMessage, completion, reason string) (*naviruntime.ExecuteResult, error, bool) {
	if !shouldAttachRenderPayloadForMissingToolCall(surfaced, userMessage) {
		return nil, nil, false
	}
	classification := render.Classification{
		Intent:        render.RenderIntentOpenUIDataRender,
		Capability:    "tool_usage",
		PreferredView: runtimeRenderPreferredView(userMessage),
	}
	l.recordCompiledTrace(ctx, orchestrationtrace.StageRunCompleted, surfaced, map[string]string{
		"completion":     strings.TrimSpace(completion),
		"target_tool":    renderVisualizeToolName,
		"blocked_reason": strings.TrimSpace(reason),
	})
	result, err := l.executeDataDrivenRender(ctx, run, chatID, experienceMode, thread, classification)
	return result, err, true
}

func canonicalizeRenderToolCalls(normalized orchestration.NormalizedModelResponse, compiled orchestration.CompiledModelRequest, userMessage string) orchestration.NormalizedModelResponse {
	if !runtimeLooksLikeRenderIntent(userMessage) || !compiledToolSurfaceContains(compiled.Tools, renderVisualizeToolName) {
		return normalized
	}
	for i := range normalized.ToolCalls {
		if isRenderVisualizeAlias(normalized.ToolCalls[i].Name) {
			normalized.ToolCalls[i].Name = renderVisualizeToolName
		}
	}
	return normalized
}

func isRenderVisualizeAlias(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case renderVisualizeToolName, "visualize", "visualise", "render.visualize", "render.visualise", "render_visualize", "render_visualise", "navi.visualize", "navi.visualise", "navi_render_visualize", "navi_render_visualise":
		return true
	default:
		return false
	}
}

func runtimeRenderPreferredView(raw string) string {
	normalized := normalizeBoundedDiagnosticQuery(raw)
	switch {
	case strings.Contains(normalized, "timeline"):
		return "timeline"
	case strings.Contains(normalized, "dashboard"):
		return "dashboard"
	case strings.Contains(normalized, "table") || strings.Contains(normalized, "tabulate"):
		return "table"
	default:
		return "chart"
	}
}

func compiledToolSurfaceContains(tools []llm.ToolDefinition, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == target {
			return true
		}
	}
	return false
}

func profileNeedsMissingToolCallRecovery(profile orchestration.ModelProfile) bool {
	provider := strings.ToLower(firstNonEmpty(
		strings.TrimSpace(profile.Provider),
		strings.TrimSpace(profile.Metadata["routing_profile_provider_key"]),
		strings.TrimSpace(profile.Metadata["routing_provider"]),
	))
	if provider == "ollama" {
		return true
	}
	return orchestrationmodel.ProfileHasQuirk(profile, orchestrationmodel.QuirkPlainFunctionalStyle)
}

func missingRequiredToolCallRepairPrompt(targetTool string) string {
	targetTool = strings.TrimSpace(targetTool)
	if targetTool == "" {
		targetTool = "the required tool"
	}
	return fmt.Sprintf("Runtime repair: the previous assistant response did not call %q. Do not answer in plain text. Call exactly that tool now with valid arguments from the user's request. If required arguments are unknown, ask one concise clarification instead of inventing values.", targetTool)
}

func missingRequiredToolCallFallbackReply(targetTool string, sourceChannel string) string {
	targetTool = strings.TrimSpace(targetTool)
	isTelegram := isTelegramSourceChannel(sourceChannel)
	if targetTool == "" {
		if isTelegram {
			return "I couldn't complete that with the current model because it did not emit the required tool call. Please try again."
		}
		return "I couldn't complete that with the current model because it did not emit the required tool call. Please switch to a tool-capable model or try again after changing models."
	}
	if isTelegram {
		return fmt.Sprintf("I couldn't complete that with the current model because it did not emit the required tool call for %s. Please try again.", targetTool)
	}
	return fmt.Sprintf("I couldn't complete that with the current model because it did not emit the required tool call for %s. Please switch to a tool-capable model or try again after changing models.", targetTool)
}

func applyCompiledInferenceMetadata(compiled orchestration.CompiledModelRequest, decision inference.DecisionEnvelope) orchestration.CompiledModelRequest {
	if compiled.Metadata == nil {
		compiled.Metadata = map[string]string{}
	}
	compiled.Metadata["ics_mode"] = string(decision.Rationale.DominantMode)
	compiled.Metadata["ics_candidate_id"] = decision.Rationale.ChosenAction.CandidateID
	if decision.ModelDirective != nil {
		compiled.Metadata["ics_allowed_tools"] = strings.Join(compactStrings(append([]string(nil), decision.ModelDirective.ToolNames...)), ",")
		if target := strings.TrimSpace(decision.ModelDirective.ToolChoice); target != "" && compiledToolSurfaceContains(compiled.Tools, target) {
			compiled.Options.ToolChoice = target
			compiled.Metadata["ics_target_capability"] = target
		}
	}
	if compiled.Metadata["ics_target_capability"] == "" {
		compiled.Metadata["ics_target_capability"] = firstNonEmpty(
			decision.ExecutionBoundary.TargetCapability,
			decision.Rationale.ExecutionIntent.TargetCapability,
			decision.Rationale.Governance.TargetCapability,
			decision.Rationale.ChosenAction.TargetCapability,
		)
	}
	return compiled
}

func mergePausedDecision(supervised, paused inference.DecisionEnvelope) inference.DecisionEnvelope {
	merged := supervised
	merged.RuntimeDisposition = paused.RuntimeDisposition
	merged.Proposal = paused.Proposal
	merged.ExecutionBoundary = paused.ExecutionBoundary
	merged.ModelDirective = paused.ModelDirective
	merged.AuthorizedToolCalls = append([]llm.ToolCall(nil), paused.AuthorizedToolCalls...)
	merged.AuthorizedTools = append([]inference.AuthorizedToolInvocation(nil), paused.AuthorizedTools...)
	merged.ToolPermit = paused.ToolPermit
	merged.PendingToolCall = paused.PendingToolCall
	merged.PendingToolPermit = paused.PendingToolPermit
	merged.ReplyMessage = paused.ReplyMessage
	if paused.GovernanceResult != nil {
		validationCopy := *paused.GovernanceResult
		merged.GovernanceResult = &validationCopy
		merged.Rationale.Governance.ValidationResult = &validationCopy
	}
	if paused.Proposal != nil {
		merged.Rationale.DecisionTrace.ProposalID = paused.Proposal.ProposalID
		merged.Rationale.ReflectionHooks.ProposalCandidate = paused.Proposal.ProposalID
	}
	return merged
}

func (l *AgentLoop) buildToolAuthorizationAttempt(run *naviruntime.RunState, decision inference.DecisionEnvelope, tc llm.ToolCall, inboxItem *naviruntime.InboxItem, scheduled []naviruntime.ScheduledMessage) inference.ToolAuthorizationInput {
	attempt := inference.ToolAuthorizationInput{
		Chat: inference.ChatContext{
			ChatID: run.RuntimeSessionID,
		},
		Runtime: inference.RuntimeContext{
			Run:   run,
			RunID: run.RunID,
		},
		ToolCallID: string(strings.TrimSpace(tc.ID)),
		ToolName:   tc.Name,
		Arguments:  tc.Arguments,
	}
	contract := inference.ModelDirectiveToolContract(decision.ModelDirective, tc.Name)
	if contract == nil {
		return attempt
	}
	contractCopy := *contract
	attempt.Contract = &contractCopy
	attempt.RequiresConfirmation = contractCopy.RequiresConfirmation

	if strings.TrimSpace(contractCopy.TargetPathArg) != "" {
		rawTargetPath := stringArgFromArgs(tc.Arguments, contractCopy.TargetPathArg)
		if rawTargetPath != "" {
			if contractCopy.WorkspaceScopedPath {
				attempt.ResolvedTargetPath = l.resolveWorkspaceScopedPath(rawTargetPath)
			} else {
				attempt.ResolvedTargetPath = rawTargetPath
			}
		}
	}
	return attempt
}

func (l *AgentLoop) prepareModelToolContracts(tools []llm.ToolDefinition) map[string]inference.ToolContract {
	if len(tools) == 0 {
		return nil
	}
	reg, err := l.ensureToolRegistry()
	if err != nil || reg == nil {
		return nil
	}
	out := make(map[string]inference.ToolContract, len(tools))
	for _, def := range tools {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}
		toolEntry, ok := reg.Lookup(name)
		if !ok || toolEntry == nil || toolEntry.Executor == nil {
			continue
		}
		contract, ok := buildRuntimeToolContract(toolEntry)
		if !ok {
			continue
		}
		out[name] = contract
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (l *AgentLoop) prepareExecutableToolNames(tools []llm.ToolDefinition) []string {
	if len(tools) == 0 {
		return nil
	}
	reg, err := l.ensureToolRegistry()
	if err != nil || reg == nil {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, def := range tools {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}
		toolEntry, ok := reg.Lookup(name)
		if !ok || toolEntry == nil || toolEntry.Executor == nil {
			continue
		}
		out = append(out, name)
	}
	return compactRuntimeStrings(out)
}

func buildRuntimeToolContract(toolEntry *navitool.Tool) (inference.ToolContract, bool) {
	if toolEntry == nil {
		return inference.ToolContract{}, false
	}
	name := strings.TrimSpace(toolEntry.Name)
	if name == "" {
		name = strings.TrimSpace(toolEntry.Definition.Name)
	}
	if name == "" {
		return inference.ToolContract{}, false
	}
	// Contract compilation is the only point where runtime may translate tool
	// registry metadata into authoritative execution semantics.
	commandType := toolCommandType(toolEntry, schema.CommandTypeInvoke)
	domain := firstNonEmpty(strings.TrimSpace(toolEntry.Governance.Domain), strings.TrimSpace(toolEntry.Metadata.Component), string(toolEntry.Source))
	targetPathArg := strings.TrimSpace(toolEntry.Governance.PathRestriction)
	workspaceScopedPath := false
	switch toolEntry.Source {
	case navitool.ToolSourceFileTools, navitool.ToolSourceSelfMod:
		targetPathArg = "path"
		workspaceScopedPath = true
	case navitool.ToolSourceSkill, navitool.ToolSourcePlugin, navitool.ToolSourceBuiltin:
		workspaceScopedPath = false
	}
	contract := inference.ToolContract{
		ID:                   "contract:" + name,
		ToolName:             name,
		SourceType:           runtimeToolSourceType(toolEntry),
		ExecutionKind:        inference.ToolExecutionKindRegistry,
		CommandType:          commandType,
		Domain:               domain,
		ActorKind:            "navi",
		WorkspaceAction:      toolWorkspaceAction(toolEntry, commandType),
		TargetPathArg:        targetPathArg,
		WorkspaceScopedPath:  workspaceScopedPath,
		RequiresConfirmation: toolEntry.Governance.RequiresConfirm,
		RiskLevel:            riskLevelFromToolGovernance(toolEntry.Governance.RiskTier),
		Reversibility:        reversibilityFromToolGovernance(toolEntry.Governance.Reversibility),
		ExpectedSideEffects:  expectedSideEffectsForTool(toolEntry),
		SkillName:            strings.TrimSpace(toolEntry.Metadata.SkillName),
		PluginName:           strings.TrimSpace(toolEntry.Metadata.PluginID),
		ConnectorID:          strings.TrimSpace(toolEntry.Metadata.Notes["connector_id"]),
	}
	if !contract.WorkspaceAction.IsValid() {
		return inference.ToolContract{}, false
	}
	return contract, true
}

func runtimeToolSourceType(toolEntry *navitool.Tool) string {
	if toolEntry == nil {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(toolEntry.Name), "llm-router_") {
		return "router"
	}
	switch toolEntry.Source {
	case navitool.ToolSourceSkill:
		return "skill"
	case navitool.ToolSourcePlugin:
		return "plugin"
	case navitool.ToolSourceFileTools:
		return "file_tool"
	case navitool.ToolSourceSelfMod:
		return "selfmod"
	case navitool.ToolSourceBuiltin:
		return "builtin"
	default:
		return string(toolEntry.Source)
	}
}

func expectedSideEffectsForTool(toolEntry *navitool.Tool) []string {
	if toolEntry == nil {
		return nil
	}
	if len(toolEntry.Metadata.Tags) > 0 {
		return append([]string(nil), toolEntry.Metadata.Tags...)
	}
	switch toolEntry.Governance.CommandType {
	case schema.CommandTypeQuery:
		return []string{"read_only"}
	case schema.CommandTypeUpdate, schema.CommandTypeCreate:
		return []string{"state_mutation"}
	case schema.CommandTypeDelete:
		return []string{"destructive_change"}
	case schema.CommandTypeSend:
		return []string{"external_message"}
	default:
		return nil
	}
}

func pendingToolArguments(decision inference.DecisionEnvelope) map[string]any {
	if decision.PendingToolCall == nil || len(decision.PendingToolCall.Arguments) == 0 {
		return nil
	}
	return decision.PendingToolCall.Arguments
}

func decisionToolPermit(decision inference.DecisionEnvelope, tc llm.ToolCall) *inference.ToolPermit {
	for _, authorized := range decision.AuthorizedTools {
		permit := authorized.Permit
		if toolPermitMatchesCall(&permit, tc) {
			copy := permit
			return &copy
		}
	}
	if decision.ToolPermit != nil && toolPermitMatchesCall(decision.ToolPermit, tc) {
		copy := *decision.ToolPermit
		return &copy
	}
	return nil
}

func toolPermitMatchesCall(permit *inference.ToolPermit, tc llm.ToolCall) bool {
	if permit == nil {
		return false
	}
	if expectedToolCallID := strings.TrimSpace(permit.ToolCallID); expectedToolCallID != "" && expectedToolCallID != strings.TrimSpace(tc.ID) {
		return false
	}
	return strings.TrimSpace(permit.ToolName) == strings.TrimSpace(tc.Name)
}

func toolPermitAllowsProposalLinkage(permit *inference.ToolPermit, approvedProposalID string) bool {
	if permit == nil {
		return false
	}
	approvedProposalID = strings.TrimSpace(approvedProposalID)
	if approvedProposalID == "" {
		return true
	}
	return strings.TrimSpace(permit.ApprovedProposalID) == approvedProposalID
}

// isStaleResponseRepeat returns true when content is identical to the most
// recent assistant message already stored in the chat. A match means the
// pipeline is about to deliver a duplicate of a prior turn — triggering this
// guard causes the caller to retry with an explicit freshness directive.
func isStaleResponseRepeat(content string, session *ChatRuntimeView) bool {
	content = strings.TrimSpace(content)
	if content == "" || session == nil {
		return false
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		msg := session.Messages[i]
		if msg.Role == "navi" || msg.Role == "assistant" {
			return strings.TrimSpace(msg.Content) == content
		}
	}
	return false
}

func isToolCallingFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to call tools") ||
		(strings.Contains(msg, "tool") && (strings.Contains(msg, "not support") || strings.Contains(msg, "invalid") || strings.Contains(msg, "unsupported") || strings.Contains(msg, "no tool")))
}
