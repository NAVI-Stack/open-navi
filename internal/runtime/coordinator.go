package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

type Executor interface {
	ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error)
}

type ExecutionOutcomeSaver func(ctx context.Context, eo schema.ExecutionOutcome) error
type ErrorRecorder func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error

type proposalResolutionPayload struct {
	ProposalID string             `json:"proposal_id"`
	Resolution ProposalResolution `json:"resolution"`
	Note       string             `json:"note,omitempty"`
}

type activeRun struct {
	run    *RunState
	cancel context.CancelFunc
}

// DefaultRunTimeout is the maximum duration a single run may execute before
// being cancelled. Prevents indefinite hangs when the LLM or a tool blocks.
const DefaultRunTimeout = 3 * time.Minute

// DefaultInterruptDrainTimeout bounds how long the coordinator waits for the
// executor to finish its post-interrupt cleanup after a forced cancellation.
const DefaultInterruptDrainTimeout = 5 * time.Second

// DefaultSubmitMessageTimeout bounds how long message intake/classification may
// block before the coordinator fails fast. This keeps HTTP-facing ingress from
// hanging indefinitely on DB locks or stalled runtime store operations.
const DefaultSubmitMessageTimeout = 8 * time.Second

type RunCoordinator struct {
	store                 Store
	exec                  Executor
	saveEO                ExecutionOutcomeSaver
	saveErr               ErrorRecorder
	assistantObserver     AssistantMessageObserver
	sched                 *Scheduler
	classifier            *Classifier
	runTimeout            time.Duration
	submitMessageTimeout  time.Duration
	interruptDrainTimeout time.Duration
	wakeCh                chan string
	mu                    sync.Mutex
	running               map[string]*activeRun
	wg                    sync.WaitGroup
	draining              atomic.Bool
}

type AssistantMessageEvent struct {
	Run         *RunState
	MessageID   string
	Content     string
	InboxItemID string
}

type AssistantMessageObserver func(ctx context.Context, event AssistantMessageEvent) error

func NewRunCoordinator(store Store, exec Executor, saveEO ExecutionOutcomeSaver) *RunCoordinator {
	return &RunCoordinator{
		store:                 store,
		exec:                  exec,
		saveEO:                saveEO,
		sched:                 NewScheduler(),
		classifier:            NewClassifier(),
		wakeCh:                make(chan string, 64),
		running:               make(map[string]*activeRun),
		runTimeout:            DefaultRunTimeout,
		submitMessageTimeout:  DefaultSubmitMessageTimeout,
		interruptDrainTimeout: DefaultInterruptDrainTimeout,
	}
}

func (c *RunCoordinator) SetAssistantMessageObserver(fn AssistantMessageObserver) {
	c.assistantObserver = fn
}

func (c *RunCoordinator) SetErrorRecorder(fn ErrorRecorder) {
	c.saveErr = fn
}

// SetRunTimeout overrides the default run timeout. Zero resets to DefaultRunTimeout.
func (c *RunCoordinator) SetRunTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	c.runTimeout = timeout
}

// SetSubmitMessageTimeout overrides the default inbox submit timeout. Zero
// resets to DefaultSubmitMessageTimeout.
func (c *RunCoordinator) SetSubmitMessageTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultSubmitMessageTimeout
	}
	c.submitMessageTimeout = timeout
}

func NewRunCoordinatorWithScheduler(store Store, exec Executor, saveEO ExecutionOutcomeSaver, sched *Scheduler) *RunCoordinator {
	c := NewRunCoordinator(store, exec, saveEO)
	c.sched = sched
	return c
}

func (c *RunCoordinator) Run(ctx context.Context) error {
	slog.Info("runtime: coordinator starting")
	c.recoverOrphanedRuns(ctx)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	slog.Info("runtime: coordinator running")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.dispatchPending(ctx)
		case runtimeSessionID := <-c.wakeCh:
			c.dispatchRuntimeSession(ctx, runtimeSessionID)
		}
	}
}

// Drain prevents new runs from starting and waits for in-flight runs to finish.
// Returns when all runs complete or ctx expires.
func (c *RunCoordinator) Drain(ctx context.Context) error {
	c.draining.Store(true)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *RunCoordinator) recoverOrphanedRuns(ctx context.Context) {
	runs, err := c.store.ListActiveRuns(ctx)
	if err != nil {
		slog.Warn("runtime: failed to list orphaned runs", "error", err)
		return
	}
	for _, run := range runs {
		run.SetStatus(schema.RunStatusFailed)
		run.InterruptClass = InterruptClassSystem
		run.InterruptReason = "orphaned by restart"
		run.UpdatedAt = time.Now().UTC()
		if err := c.store.UpdateRun(ctx, run); err != nil {
			slog.Warn("runtime: failed to recover orphaned run", "run_id", run.RunID, "error", err)
			continue
		}
		slog.Info("runtime: recovered orphaned run", "run_id", run.RunID, "runtime_session_id", run.RuntimeSessionID)
	}
}

func (c *RunCoordinator) Wake(runtimeSessionID string) {
	if runtimeSessionID == "" {
		return
	}
	select {
	case c.wakeCh <- runtimeSessionID:
	default:
	}
}

func (c *RunCoordinator) SubmitMessage(ctx context.Context, runtimeSessionID, experienceMode string, input MessageInput) (*InboxItem, error) {
	runtimeSessionID = firstNonEmpty(input.RuntimeSessionID, runtimeSessionID)
	trace := ProgressTraceFromContext(ctx)
	if strings.TrimSpace(trace.RuntimeSessionID) == "" {
		trace.RuntimeSessionID = runtimeSessionID
	}
	tracer := NewProgressTracer("runtime.coordinator.submit_message", trace)
	submitDone := tracer.StageStart(ctx, "runtime.intake.submit_message")
	defer submitDone(nil)
	source := input.SourceChannel
	if source == "" {
		source = "app"
	}
	item := NewInboxItem(runtimeSessionID, input.Content, source)
	item.RuntimeSessionID = runtimeSessionID
	item.ChatID = strings.TrimSpace(input.ChatID)
	item.MessageID = strings.TrimSpace(input.MessageID)
	item.SourceMessageRef = input.SourceMessageRef
	item.IdempotencyKey = input.IdempotencyKey

	submitCtx, cancel := withOptionalTimeout(ctx, c.submitMessageTimeout)
	defer cancel()

	startedAt := time.Now()
	classifyDone := tracer.StageStart(submitCtx, "runtime.intake.classify")
	c.classifyInboxItem(submitCtx, runtimeSessionID, item)
	classifyDone(nil)
	acceptDone := tracer.StageStart(submitCtx, "runtime.intake.accept_message")
	accepted, err := c.store.AcceptMessage(submitCtx, runtimeSessionID, item, experienceMode)
	acceptDone(err)
	if err != nil {
		if errors.Is(err, ErrDuplicateInboxItem) {
			slog.Debug("runtime: inbox item deduplicated — idempotency key already accepted",
				"runtime_session_id", runtimeSessionID,
				"idempotency_key", item.IdempotencyKey,
			)
			tracer.Mark("runtime.intake.deduplicated", "idempotency_key", item.IdempotencyKey)
			return nil, nil
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(submitCtx.Err(), context.DeadlineExceeded) {
			slog.Warn("runtime: submit message timed out",
				"runtime_session_id", runtimeSessionID,
				"source_channel", source,
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"timeout_ms", c.submitMessageTimeout.Milliseconds(),
			)
			return nil, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) || errors.Is(submitCtx.Err(), context.Canceled) {
			slog.Debug("runtime: submit message cancelled",
				"runtime_session_id", runtimeSessionID,
				"source_channel", source,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			return nil, context.Canceled
		}
		return nil, err
	}
	c.Wake(runtimeSessionID)
	if accepted != nil {
		tracer.Mark("runtime.intake.completed", "queue_action", accepted.QueueAction, "inbox_status", accepted.Status)
	} else {
		tracer.Mark("runtime.intake.completed")
	}
	return accepted, nil
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining <= timeout {
			return context.WithCancel(ctx)
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *RunCoordinator) SubmitSignal(ctx context.Context, item *InboxItem) (*InboxItem, error) {
	if item == nil {
		return nil, fmt.Errorf("runtime: submit signal: nil inbox item")
	}
	if strings.TrimSpace(item.SourceChannel) == "" {
		item.SourceChannel = "system"
	}
	if item.QueueAction == "" {
		item.QueueAction = "append"
	}
	if item.Status == "" {
		item.Status = InboxStatusPending
	}
	if strings.TrimSpace(item.RuntimeSessionID) == "" {
		item.RuntimeSessionID = firstNonEmpty(item.ChatID, item.RuntimeSessionID)
	}
	if item.CorrelationID == "" {
		item.CorrelationID = firstNonEmpty(item.ChatID, item.RuntimeSessionID)
	}
	if item.ActorType == "" {
		item.ActorType = "system"
	}
	accepted, err := c.store.AcceptSignal(ctx, item)
	if err != nil {
		return nil, err
	}
	c.Wake(firstNonEmpty(item.ChatID, item.RuntimeSessionID))
	return accepted, nil
}

func (c *RunCoordinator) EnqueueProposalResolution(ctx context.Context, runtimeSessionID, proposalID string, resolution ProposalResolution, note string) (*InboxItem, error) {
	pausedRun, err := c.store.GetPausedRun(ctx, runtimeSessionID)
	if err != nil {
		return nil, fmt.Errorf("runtime: load paused run for proposal resolution: %w", err)
	}
	if pausedRun == nil {
		return nil, fmt.Errorf("runtime: no paused run for runtime session %s", runtimeSessionID)
	}
	if pausedRun.BlockedOnProposalID != "" && pausedRun.BlockedOnProposalID != proposalID {
		return nil, fmt.Errorf("runtime: paused run %s is blocked on proposal %s, not %s", pausedRun.RunID, pausedRun.BlockedOnProposalID, proposalID)
	}
	payload, _ := json.Marshal(proposalResolutionPayload{
		ProposalID: proposalID,
		Resolution: resolution,
		Note:       note,
	})
	item := &InboxItem{
		ID:               "",
		RuntimeSessionID: runtimeSessionID,
		ChatID:           firstNonEmpty(pausedRun.ChatID, runtimeSessionID),
		SourceChannel:    "proposal",
		ActorType:        "proposal_system",
		PayloadType:      "proposal_resolution",
		QueueAction:      "resume",
		Status:           InboxStatusPending,
		Structured:       payload,
		CorrelationID:    runtimeSessionID,
		ClassifiedReason: "proposal resolution resumes a paused foreground run",
		Confidence:       1,
		ReceivedAt:       time.Now().UTC(),
	}
	accepted, err := c.store.AcceptSignal(ctx, item)
	if err != nil {
		return nil, err
	}
	c.Wake(runtimeSessionID)
	return accepted, nil
}

func (c *RunCoordinator) CancelRun(runtimeSessionID string) error {
	c.mu.Lock()
	active := c.running[runtimeSessionID]
	c.mu.Unlock()
	if active == nil || active.cancel == nil {
		return fmt.Errorf("runtime: no active run for runtime session %s", runtimeSessionID)
	}
	run := active.run
	if run != nil {
		run.InterruptClass = InterruptClassUserCancel
		run.InterruptReason = "cancelled by user"
		_ = c.store.UpdateRun(context.Background(), run)
		c.emitEvent(context.Background(), schema.NewRunEvent(
			schema.FactInterruptRaised,
			schema.EventKindFact,
			runtimeSessionID,
			schema.AgentNavi,
			run.RunID,
			c.runtimeSessionVisibility(context.Background(), runtimeSessionID),
			schema.InterruptRaisedPayload{
				RunID:            run.RunID,
				RuntimeSessionID: runtimeSessionID,
				InterruptClass:   string(InterruptClassUserCancel),
				Reason:           run.InterruptReason,
			},
		))
	}
	active.cancel()
	return nil
}

func (c *RunCoordinator) ResumeRun(runtimeSessionID string) {
	c.Wake(runtimeSessionID)
}

func (c *RunCoordinator) classifyInboxItem(ctx context.Context, runtimeSessionID string, item *InboxItem) {
	if item == nil {
		return
	}
	pausedRun, err := c.store.GetPausedRun(ctx, runtimeSessionID)
	if err != nil {
		slog.Debug("runtime: classify inbox get paused run failed", "runtime_session_id", runtimeSessionID, "error", err)
	}
	pending, err := c.store.ListPendingItems(ctx, runtimeSessionID, 20)
	if err != nil {
		slog.Debug("runtime: classify inbox list pending failed", "runtime_session_id", runtimeSessionID, "error", err)
		return
	}
	if c.classifier == nil {
		c.classifier = NewClassifier()
	}
	c.classifier.classify(item, pausedRun, pending)
}

func (c *RunCoordinator) dispatchPending(ctx context.Context) {
	if c.draining.Load() {
		return
	}
	runtimeSessionIDs, err := c.store.ListPendingRuntimeSessions(ctx)
	if err != nil {
		slog.Warn("runtime: list pending runtime sessions failed", "error", err)
		return
	}
	if len(runtimeSessionIDs) > 0 {
		slog.Debug("runtime: dispatch pending", "session_count", len(runtimeSessionIDs))
	}
	for _, runtimeSessionID := range runtimeSessionIDs {
		c.dispatchRuntimeSession(ctx, runtimeSessionID)
	}
}

func (c *RunCoordinator) dispatchRuntimeSession(ctx context.Context, runtimeSessionID string) {
	if runtimeSessionID == "" || c.draining.Load() {
		return
	}
	c.mu.Lock()
	if c.running[runtimeSessionID] != nil {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	pausedRun, err := c.store.GetPausedRun(ctx, runtimeSessionID)
	if err != nil {
		slog.Debug("runtime: load paused run failed", "runtime_session_id", runtimeSessionID, "error", err)
		return
	}
	items, err := c.store.ListPendingItems(ctx, runtimeSessionID, 20)
	if err != nil {
		slog.Warn("runtime: list pending items failed", "runtime_session_id", runtimeSessionID, "error", err)
		return
	}
	if len(items) == 0 && pausedRun == nil {
		promoted, promoteErr := c.store.PromoteDeferredItems(ctx, runtimeSessionID, 100)
		if promoteErr != nil {
			slog.Debug("runtime: promote deferred failed", "runtime_session_id", runtimeSessionID, "error", promoteErr)
			return
		}
		if promoted > 0 {
			items, err = c.store.ListPendingItems(ctx, runtimeSessionID, 20)
			if err != nil {
				slog.Warn("runtime: list pending items after promote failed", "runtime_session_id", runtimeSessionID, "error", err)
				return
			}
		}
	}
	if len(items) == 0 {
		return
	}
	if pausedRun != nil {
		for _, item := range items {
			if item.PayloadType != "proposal_resolution" && item.QueueAction != "resume" {
				continue
			}
			c.launchRun(ctx, pausedRun, &item)
			return
		}
		return
	}

	item := items[0]
	run := NewRun(runtimeSessionID, "")
	run.ChatID = item.ChatID
	run.OriginEndpointID = strings.TrimSpace(item.OriginEndpointID)
	run.InitiatedByInboxItemID = item.ID
	run.SetStatus(schema.RunStatusActive)
	if err := c.store.CreateRun(ctx, run); err != nil {
		slog.Debug("runtime: create run failed", "runtime_session_id", runtimeSessionID, "error", err)
		return
	}
	if err := c.store.MarkInboxConsumed(ctx, item.ID, run.RunID); err != nil {
		c.failRun(ctx, run, fmt.Sprintf("mark inbox consumed: %v", err))
		return
	}
	DefaultMetrics().RecordRunStarted()
	c.emitEvent(ctx, schema.NewRunEvent(
		schema.FactRunStarted,
		schema.EventKindFact,
		runtimeSessionID,
		schema.AgentNavi,
		run.RunID,
		c.runtimeSessionVisibility(ctx, runtimeSessionID),
		schema.RunStartedPayload{RunID: run.RunID, RuntimeSessionID: runtimeSessionID, ExperienceMode: run.ExperienceMode, Phase: string(run.CurrentPhase)},
	))
	c.emitPhaseChange(ctx, run, "", run.CurrentPhase)
	c.launchRun(ctx, run, &item)
}

func (c *RunCoordinator) launchRun(parentCtx context.Context, run *RunState, item *InboxItem) {
	if surface := inboxSurface(item); surface != "" {
		run.SetScratchpadValue("source_channel", surface)
	}
	if item != nil {
		run.SetScratchpadValue("source_message_ref", strings.TrimSpace(item.SourceMessageRef))
		if strings.HasPrefix(strings.TrimSpace(item.PayloadType), "chat_") && len(item.Structured) > 0 {
			var payload map[string]string
			if err := json.Unmarshal(item.Structured, &payload); err == nil {
				run.SetScratchpadValue("chat_action", strings.TrimSpace(item.PayloadType))
				for key, value := range payload {
					run.SetScratchpadValue("chat_action_"+strings.TrimSpace(key), strings.TrimSpace(value))
				}
			}
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	c.mu.Lock()
	c.running[run.RuntimeSessionID] = &activeRun{run: run, cancel: cancel}
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			cancel()
			c.mu.Lock()
			delete(c.running, run.RuntimeSessionID)
			c.mu.Unlock()
			c.Wake(run.RuntimeSessionID)
		}()

		var checkpoint *Checkpoint
		var resume *ResumeSignal
		if item != nil && item.PayloadType == "proposal_resolution" {
			var err error
			checkpoint, err = c.store.LoadCheckpoint(ctx, run.RunID)
			if err != nil {
				c.failRun(ctx, run, fmt.Sprintf("load checkpoint: %v", err))
				return
			}
			if checkpoint == nil {
				c.failRun(ctx, run, "resume checkpoint missing")
				return
			}
			var payload proposalResolutionPayload
			if len(item.Structured) > 0 {
				_ = json.Unmarshal(item.Structured, &payload)
			}
			if run.BlockedOnProposalID != "" && payload.ProposalID != "" && run.BlockedOnProposalID != payload.ProposalID {
				c.failRun(ctx, run, fmt.Sprintf("resume proposal mismatch: blocked on %s, got %s", run.BlockedOnProposalID, payload.ProposalID))
				return
			}
			resume = &ResumeSignal{ProposalID: payload.ProposalID, Resolution: payload.Resolution, Note: payload.Note}
			if err := c.store.MarkInboxConsumed(ctx, item.ID, run.RunID); err != nil {
				c.failRun(ctx, run, fmt.Sprintf("consume resume signal: %v", err))
				return
			}
			run.SetStatus(schema.RunStatusActive)
			run.PauseReason = ""
			run.BlockedOnProposalID = ""
			if err := c.store.UpdateRun(ctx, run); err != nil {
				c.failRun(ctx, run, fmt.Sprintf("persist resumed run: %v", err))
				return
			}
			c.emitEvent(ctx, schema.NewRunEvent(
				schema.FactRunResumed,
				schema.EventKindFact,
				run.RuntimeSessionID,
				schema.AgentNavi,
				run.RunID,
				c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
				schema.RunResumedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, ProposalID: payload.ProposalID, Phase: string(run.CurrentPhase)},
			))
		}

		execCtx, execCancel := context.WithCancel(ctx)
		defer execCancel()
		trace := ProgressTraceFromContext(execCtx)
		if strings.TrimSpace(trace.RuntimeSessionID) == "" {
			trace.RuntimeSessionID = run.RuntimeSessionID
		}
		trace.RunID = run.RunID
		execCtx = WithProgressTrace(execCtx, trace)
		runTracer := NewProgressTracer("runtime.coordinator.launch_run", trace)
		runDone := runTracer.StageStart(execCtx, "runtime.run.execute")
		defer runDone(nil)

		type executeOutcome struct {
			result *ExecuteResult
			err    error
		}
		resultCh := make(chan executeOutcome, 1)
		go func() {
			execStageDone := runTracer.StageStart(execCtx, "runtime.executor.execute_run")
			result, err := c.exec.ExecuteRun(execCtx, ExecuteInput{
				Run:        run,
				Checkpoint: checkpoint,
				Resume:     resume,
				InboxItem:  item,
				ShouldInterrupt: func() error {
					if execCtx.Err() != nil || run.InterruptClass != "" {
						return ErrRunCancelled
					}
					return nil
				},
			})
			execStageDone(err)
			select {
			case resultCh <- executeOutcome{result: result, err: err}:
			default:
			}
		}()

		var (
			result   *ExecuteResult
			err      error
			timeout  <-chan time.Time
			timedOut bool
		)
		if c.runTimeout > 0 {
			timer := time.NewTimer(c.runTimeout)
			defer timer.Stop()
			timeout = timer.C
		}

		select {
		case outcome := <-resultCh:
			result = outcome.result
			err = outcome.err
		case <-timeout:
			run.InterruptClass = InterruptClassSystem
			run.InterruptReason = fmt.Sprintf("run exceeded timeout after %s", c.runTimeout)
			slog.Warn("runtime: coordinator watchdog timeout",
				"runtime_session_id", run.RuntimeSessionID,
				"run_id", run.RunID,
				"surface", firstNonEmpty(runSurface(run), "unknown"),
				"phase", run.CurrentPhase,
				"timeout", c.runTimeout,
			)
			c.emitEvent(ctx, schema.NewRunEvent(
				schema.FactInterruptRaised,
				schema.EventKindFact,
				run.RuntimeSessionID,
				schema.AgentNavi,
				run.RunID,
				c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
				schema.InterruptRaisedPayload{
					RunID:            run.RunID,
					RuntimeSessionID: run.RuntimeSessionID,
					InterruptClass:   string(run.InterruptClass),
					Reason:           run.InterruptReason,
				},
			))
			execCancel()
			timedOut = true

			drainTimeout := c.interruptDrainTimeout
			if drainTimeout <= 0 {
				drainTimeout = DefaultInterruptDrainTimeout
			}
			drainTimer := time.NewTimer(drainTimeout)
			defer drainTimer.Stop()

			select {
			case outcome := <-resultCh:
				result = outcome.result
				err = outcome.err
			case <-drainTimer.C:
				c.emitInterruptApplied(ctx, run, "timed_out")
				c.failRunWithOutcome(ctx, run, "run_timeout", run.InterruptReason, schema.ExecutionOutcomeTimedOut)
				return
			}
		}
		if err != nil {
			if ctx.Err() == context.Canceled || errors.Is(err, ErrRunCancelled) {
				if run.InterruptClass == InterruptClassSystem {
					c.emitInterruptApplied(ctx, run, "timed_out")
					c.failRunWithOutcome(ctx, run, "run_timeout", run.InterruptReason, schema.ExecutionOutcomeTimedOut)
					return
				}
				run.SetStatus(schema.RunStatusCancelled)
				_ = c.store.UpdateRun(ctx, run)
				c.emitInterruptApplied(ctx, run, "cancelled")
				reason := strings.TrimSpace(run.InterruptReason)
				if reason == "" {
					reason = "cancelled"
				}
				slog.Info("runtime: emitting run.cancelled",
					"runtime_session_id", run.RuntimeSessionID,
					"run_id", run.RunID,
					"surface", firstNonEmpty(runSurface(run), "unknown"),
					"phase", run.CurrentPhase,
					"reason", reason,
				)
				c.emitEvent(ctx, schema.NewRunEvent(
					schema.FactRunCancelled,
					schema.EventKindFact,
					run.RuntimeSessionID,
					schema.AgentNavi,
					run.RunID,
					c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
					schema.RunCancelledPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, Reason: reason},
				))
				DefaultMetrics().RecordRunCancelled()
				c.saveRunOutcome(ctx, run, schema.ExecutionOutcomeCancelled)
				return
			}
			c.failRun(ctx, run, err.Error())
			return
		}
		if timedOut {
			c.emitInterruptApplied(ctx, run, "timed_out")
			c.failRunWithOutcome(ctx, run, "run_timeout", run.InterruptReason, schema.ExecutionOutcomeTimedOut)
			return
		}
		if result == nil {
			return
		}
		run = result.Run
		if result.Paused && result.Checkpoint != nil {
			run.SetStatus(schema.RunStatusWaitingForProposal)
			run.BlockedOnProposalID = result.ProposalID
			run.PauseReason = result.ProposalReason
			run.LatestCheckpointID = result.Checkpoint.CheckpointID
			if err := c.store.SaveCheckpoint(ctx, result.Checkpoint); err != nil {
				c.failRun(ctx, run, fmt.Sprintf("save checkpoint: %v", err))
				return
			}
			if err := c.store.UpdateRun(ctx, run); err != nil {
				c.failRun(ctx, run, fmt.Sprintf("persist paused run: %v", err))
				return
			}
			toolName := ""
			if result.PendingToolCall != nil {
				toolName = result.PendingToolCall.Name
			}
			c.emitEvent(ctx, schema.NewRunEvent(
				schema.FactProposalWaiting,
				schema.EventKindFact,
				run.RuntimeSessionID,
				schema.AgentNavi,
				run.RunID,
				schema.VisibilityUser,
				schema.ProposalWaitingPayload{
					RunID:            run.RunID,
					RuntimeSessionID: run.RuntimeSessionID,
					ProposalID:       result.ProposalID,
					ToolName:         toolName,
					Arguments:        result.ProposalArgs,
					Reason:           result.ProposalReason,
				},
			))
			c.emitEvent(ctx, schema.NewRunEvent(
				schema.FactRunPaused,
				schema.EventKindFact,
				run.RuntimeSessionID,
				schema.AgentNavi,
				run.RunID,
				c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
				schema.RunPausedPayload{
					RunID:            run.RunID,
					RuntimeSessionID: run.RuntimeSessionID,
					Reason:           result.ProposalReason,
					ProposalID:       result.ProposalID,
					Phase:            string(run.CurrentPhase),
				},
			))
			return
		}
		if result.Completed {
			run.SetPhase(RunPhaseStreamFinalize)
			outcome := completedOutcomeForResult(result)
			if len(result.ScheduledMessages) > 0 {
				c.deliverScheduledMessages(ctx, run, result, run.InitiatedByInboxItemID)
			} else {
				msgID, err := c.store.CompleteRun(ctx, run, result.FinalContent, result.ExperienceMode, run.InitiatedByInboxItemID)
				if err != nil {
					c.failRun(ctx, run, fmt.Sprintf("complete run: %v", err))
					return
				}
				c.observeAssistantMessage(ctx, run, msgID, result.FinalContent, run.InitiatedByInboxItemID)
				DefaultMetrics().RecordRunCompleted(time.Since(run.StartedAt))
				c.saveRunOutcome(ctx, run, outcome)
				c.recordCompletedRunDiagnostic(ctx, run, result, outcome)
			}
		}
	}()
}

func (c *RunCoordinator) deliverScheduledMessages(ctx context.Context, run *RunState, result *ExecuteResult, inboxItemID string) {
	msgs := result.ScheduledMessages
	experienceMode := result.ExperienceMode
	outcome := completedOutcomeForResult(result)
	totalLen := 0
	for _, m := range msgs {
		totalLen += len(m.Content)
	}
	var remaining int32
	remaining = int32(len(msgs))
	runStartedAt := run.StartedAt
	var (
		lastMessageID string
		lastMessageMu sync.Mutex
	)

	for _, m := range msgs {
		delay := m.Delay
		if delay < 0 {
			delay = 0
		}
		if delay == 0 || c.sched == nil {
			msgID, err := c.store.AppendAssistantMessage(ctx, run, m.Content, experienceMode, inboxItemID)
			if err != nil {
				c.failRun(ctx, run, fmt.Sprintf("append scheduled message: %v", err))
				return
			}
			c.observeAssistantMessage(ctx, run, msgID, m.Content, inboxItemID)
			lastMessageMu.Lock()
			lastMessageID = msgID
			lastMessageMu.Unlock()
			if atomic.AddInt32(&remaining, -1) == 0 {
				lastMessageMu.Lock()
				finalMessageID := lastMessageID
				lastMessageMu.Unlock()
				if err := c.store.MarkRunCompleted(ctx, run, totalLen, finalMessageID); err != nil {
					slog.Debug("runtime: mark run completed failed", "error", err)
				}
				DefaultMetrics().RecordRunCompleted(time.Since(runStartedAt))
				c.saveRunOutcome(ctx, run, outcome)
				c.recordCompletedRunDiagnostic(ctx, run, result, outcome)
			}
		} else {
			msg := m
			c.sched.RunAfter(delay, func() {
				bg := context.Background()
				msgID, err := c.store.AppendAssistantMessage(bg, run, msg.Content, experienceMode, inboxItemID)
				if err != nil {
					slog.Warn("runtime: scheduled message append failed", "error", err)
				}
				c.observeAssistantMessage(bg, run, msgID, msg.Content, inboxItemID)
				lastMessageMu.Lock()
				lastMessageID = msgID
				lastMessageMu.Unlock()
				if atomic.AddInt32(&remaining, -1) == 0 {
					lastMessageMu.Lock()
					finalMessageID := lastMessageID
					lastMessageMu.Unlock()
					if err := c.store.MarkRunCompleted(bg, run, totalLen, finalMessageID); err != nil {
						slog.Debug("runtime: mark run completed failed", "error", err)
					}
					DefaultMetrics().RecordRunCompleted(time.Since(runStartedAt))
					c.saveRunOutcome(bg, run, outcome)
					c.recordCompletedRunDiagnostic(bg, run, result, outcome)
				}
			})
		}
	}
}

func (c *RunCoordinator) observeAssistantMessage(ctx context.Context, run *RunState, messageID, content, inboxItemID string) {
	if c == nil || c.assistantObserver == nil || strings.TrimSpace(messageID) == "" {
		return
	}
	if err := c.assistantObserver(ctx, AssistantMessageEvent{
		Run:         run,
		MessageID:   messageID,
		Content:     content,
		InboxItemID: inboxItemID,
	}); err != nil {
		runID := ""
		if run != nil {
			runID = run.RunID
		}
		slog.Warn("runtime: assistant message observer failed", "run_id", runID, "message_id", messageID, "error", err)
	}
}

func (c *RunCoordinator) emitPhaseChange(ctx context.Context, run *RunState, previous string, phase RunPhase) {
	c.emitEvent(ctx, schema.NewRunEvent(
		schema.FactRunPhaseChanged,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.RunPhaseChangedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			Phase:            string(phase),
			Previous:         previous,
		},
	))
}

func (c *RunCoordinator) emitEvent(ctx context.Context, ev schema.Event) {
	if err := c.store.AppendRuntimeEvent(ctx, ev); err != nil {
		logFn := slog.Debug
		switch ev.Type {
		case schema.FactRunFailed, schema.FactRunCompleted, schema.FactRunCancelled, schema.FactProposalWaiting, schema.FactAssistantMessageCompleted:
			logFn = slog.Warn
		}
		logFn("runtime: append event failed", "type", ev.Type, "correlation_id", ev.CorrelationID, "run_id", ev.RunID, "error", err)
	}
}

func (c *RunCoordinator) failRun(ctx context.Context, run *RunState, errMsg string) {
	c.failRunWithType(ctx, run, "run_failed", errMsg)
}

func completedOutcomeForResult(result *ExecuteResult) schema.ExecutionOutcomeOutcome {
	if result == nil || result.Outcome == "" {
		return schema.ExecutionOutcomeSucceeded
	}
	return result.Outcome
}

func completedOutcomeErrorType(outcome schema.ExecutionOutcomeOutcome) string {
	switch outcome {
	case schema.ExecutionOutcomeTimedOut:
		return "run_completed_with_timeout"
	case schema.ExecutionOutcomeFailed:
		return "run_completed_with_failure"
	case schema.ExecutionOutcomePartiallySucceeded:
		return "run_completed_with_partial_failure"
	case schema.ExecutionOutcomeCancelled:
		return "run_completed_with_cancellation"
	default:
		return ""
	}
}

func completedOutcomeContextJSON(run *RunState, result *ExecuteResult, outcome schema.ExecutionOutcomeOutcome) string {
	payload := map[string]any{
		"completed": true,
		"outcome":   outcome,
	}
	if run != nil {
		payload["phase"] = string(run.CurrentPhase)
	}
	if result != nil && len(result.ScheduledMessages) > 0 {
		payload["scheduled_messages"] = len(result.ScheduledMessages)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func (c *RunCoordinator) recordCompletedRunDiagnostic(ctx context.Context, run *RunState, result *ExecuteResult, outcome schema.ExecutionOutcomeOutcome) {
	if c.saveErr == nil || run == nil {
		return
	}
	errorType := completedOutcomeErrorType(outcome)
	if errorType == "" {
		return
	}
	message := "run completed with degraded outcome"
	if result != nil && strings.TrimSpace(result.OutcomeSummary) != "" {
		message = strings.TrimSpace(result.OutcomeSummary)
	}
	_ = c.saveErr(
		ctx,
		"runtime",
		run.RuntimeSessionID,
		run.RunID,
		errorType,
		message,
		completedOutcomeContextJSON(run, result, outcome),
	)
}

func (c *RunCoordinator) failRunWithType(ctx context.Context, run *RunState, errorType, errMsg string) {
	c.failRunWithOutcome(ctx, run, errorType, errMsg, schema.ExecutionOutcomeFailed)
}

func (c *RunCoordinator) failRunWithOutcome(ctx context.Context, run *RunState, errorType, errMsg string, outcome schema.ExecutionOutcomeOutcome) {
	if run == nil {
		return
	}
	displayErrMsg := userFacingRunFailureMessage(errMsg, outcome, run)
	surface := firstNonEmpty(runSurface(run), "unknown")

	if c.saveErr != nil {
		// Log the RAW error internally for developers
		_ = c.saveErr(ctx, "runtime", run.RuntimeSessionID, run.RunID, errorType, errMsg, "")
	}
	run.SetStatus(schema.RunStatusFailed)
	if err := c.store.UpdateRun(ctx, run); err != nil {
		slog.Warn("runtime: update failed run state failed", "runtime_session_id", run.RuntimeSessionID, "run_id", run.RunID, "error", err)
	}
	DefaultMetrics().RecordRunFailed()
	slog.Warn("runtime: emitting run.failed",
		"runtime_session_id", run.RuntimeSessionID,
		"run_id", run.RunID,
		"surface", surface,
		"phase", run.CurrentPhase,
		"error_type", errorType,
		"outcome", outcome,
	)
	c.emitEvent(ctx, schema.NewRunEvent(
		schema.FactRunFailed,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
		schema.RunFailedPayload{RunID: run.RunID, RuntimeSessionID: run.RuntimeSessionID, Error: displayErrMsg, DurationMs: run.DurationMs()},
	))
	c.saveRunOutcome(ctx, run, outcome)
}

func (c *RunCoordinator) emitInterruptApplied(ctx context.Context, run *RunState, outcome string) {
	if run == nil {
		return
	}
	c.emitEvent(ctx, schema.NewRunEvent(
		schema.FactInterruptApplied,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		c.runtimeSessionVisibility(ctx, run.RuntimeSessionID),
		schema.InterruptAppliedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			InterruptClass:   string(run.InterruptClass),
			Reason:           run.InterruptReason,
			Outcome:          outcome,
		},
	))
}

// SanitizeErrorMessage provides a user-friendly version of a raw error string.
func SanitizeErrorMessage(msg string) string {
	lowered := strings.ToLower(msg)
	if strings.Contains(lowered, "context deadline exceeded") || strings.Contains(lowered, "timeout") {
		return "The request timed out. Please try again."
	}
	if strings.Contains(lowered, "api key") || strings.Contains(lowered, "unauthorized") || strings.Contains(lowered, "401") {
		return "Authentication failed with the LLM provider. Please check your configuration."
	}
	if strings.Contains(lowered, "rate limit") || strings.Contains(lowered, "429") || strings.Contains(lowered, "too many requests") {
		return "The LLM provider is rate limiting requests. Please wait a moment."
	}
	if strings.Contains(lowered, "connection") || strings.Contains(lowered, "dial") || strings.Contains(lowered, "refused") {
		return "Failed to connect to the LLM provider. Please check your internet connection or provider status."
	}
	if strings.Contains(lowered, "overloaded") || strings.Contains(lowered, "503") {
		return "The LLM provider is currently overloaded. Please try again in a few seconds."
	}
	if strings.Contains(lowered, "no provider configured") {
		return "LLM provider is not configured. Run navi init or open /onboarding to configure an LLM provider."
	}

	// Default fallback for unknown or generic internal errors
	return "An internal error occurred during generation. The technical details have been logged."
}

func (c *RunCoordinator) saveRunOutcome(ctx context.Context, run *RunState, outcome schema.ExecutionOutcomeOutcome) {
	if c.saveEO == nil || run == nil {
		return
	}
	end := time.Now().UTC()
	_ = c.saveEO(ctx, schema.ExecutionOutcome{
		AttemptID:          run.RunID,
		CommandID:          run.RunID,
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeCompose,
		StartTime:          run.StartedAt,
		EndTime:            &end,
		Outcome:            outcome,
		RunID:              run.RunID,
		RuntimeSessionID:   run.RuntimeSessionID,
		CorrelationID:      run.RuntimeSessionID,
		ConnectorIDs:       nil,
		LLMProvider:        run.LLMProvider,
		LLMModel:           run.LLMModel,
		LLMTaskClass:       run.LLMTaskClass,
		LLMComplexity:      run.LLMComplexity,
		AffectedEntities:   fmt.Sprintf(`[{"kind":"runtime_session","id":"%s"}]`, run.RuntimeSessionID),
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
	})
}

func runtimeSessionEventVisibility(kind schema.RuntimeSessionKind, runtimeSessionID string) schema.EventVisibility {
	if schema.IsInternalRuntimeSession(kind, runtimeSessionID) {
		return schema.VisibilityOperator
	}
	return schema.VisibilityUser
}

func (c *RunCoordinator) runtimeSessionVisibility(ctx context.Context, runtimeSessionID string) schema.EventVisibility {
	kind := schema.DefaultRuntimeSessionKindForID(runtimeSessionID)
	if c.store != nil {
		if resolved, err := c.store.LookupRuntimeSessionKind(ctx, runtimeSessionID); err == nil {
			kind = resolved
		}
	}
	return runtimeSessionEventVisibility(kind, runtimeSessionID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func inboxSurface(item *InboxItem) string {
	if item == nil {
		return ""
	}
	switch surface := strings.ToLower(strings.TrimSpace(item.SourceChannel)); surface {
	case "", "proposal", "system":
		return ""
	default:
		return surface
	}
}
