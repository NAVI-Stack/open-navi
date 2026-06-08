package heartbeat

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/prompts"
	"github.com/open-navi/navi/internal/schema"
)

const (
	minInterval       = 5 * time.Minute
	defaultInterval   = 30 * time.Minute
	defaultResultWait = 60 * time.Second
	heartbeatFile     = "HEARTBEAT.md"
	logFile           = "heartbeat.log"
)

// defaultHeartbeatMD is written to the workspace on first run.
const defaultHeartbeatMD = `# Heartbeat Check List

Tasks below are executed periodically by NAVI. Add or remove items as needed.

- [ ] Check for any pending messages or unread notifications
- [ ] Review active task status and report any blockers
- [ ] Confirm system health (NATS, database connectivity)

Instructions:
- Execute ALL tasks listed above
- Use available tools for complex or async tasks
- Respond with HEARTBEAT_OK when done
`

// HeartbeatHandler processes a heartbeat prompt. Implementations deliver the
// prompt into the NAVI chat pipeline; results flow back via the event bus.
type HeartbeatHandler func(ctx context.Context, runtimeSessionID, prompt string) error
type ResultLoader func(ctx context.Context, runtimeSessionID string, since time.Time) (string, error)
type ProactiveSender func(ctx context.Context, content string) error

// HeartbeatService drives periodic background tasks defined in HEARTBEAT.md.
// It is the Go equivalent of PicoClaw's heartbeat package, adapted for NAVI's
// NATS-based architecture.
type HeartbeatService struct {
	workspaceDir    string
	bus             bus.Bus
	handler         HeartbeatHandler
	resultLoader    ResultLoader
	proactiveSender ProactiveSender
	interval        time.Duration
	resultWait      time.Duration
	enabled         bool
	quietStart      string
	quietEnd        string
	dndEnabled      bool
	timezone        string
	now             func() time.Time
	prompts         *prompts.Manager

	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}

	wakeMu       sync.Mutex
	wakeTimer    *time.Timer
	pendingWake  map[string]*pendingWake // sessionKey → merged pending wake request
	wakeCoalesce time.Duration           // optional; zero uses defaultWakeCoalesce
}

// New creates a HeartbeatService targeting the given workspace directory.
// interval is clamped to minInterval (5m). Passes zero to use defaultInterval.
func New(workspaceDir string, interval time.Duration) *HeartbeatService {
	if interval < minInterval {
		interval = defaultInterval
	}
	return &HeartbeatService{
		workspaceDir: workspaceDir,
		interval:     interval,
		resultWait:   defaultResultWait,
		enabled:      true,
		now:          time.Now,
	}
}

// SetBus injects the event bus used to publish FactNaviHeartbeatDone events.
func (h *HeartbeatService) SetBus(b bus.Bus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bus = b
}

// SetHandler injects the callback that processes heartbeat prompts.
func (h *HeartbeatService) SetHandler(fn HeartbeatHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handler = fn
}

// SetResultLoader injects a callback that waits for the heartbeat-auto result.
func (h *HeartbeatService) SetResultLoader(fn ResultLoader) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resultLoader = fn
}

// SetProactiveSender injects the callback used to surface useful heartbeat findings.
func (h *HeartbeatService) SetProactiveSender(fn ProactiveSender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.proactiveSender = fn
}

// SetQuietHours configures local quiet hours for proactive heartbeat surfacing.
func (h *HeartbeatService) SetQuietHours(start, end string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.quietStart = strings.TrimSpace(start)
	h.quietEnd = strings.TrimSpace(end)
}

// SetTimezone configures the timezone used for prompt framing and quiet hours.
func (h *HeartbeatService) SetTimezone(tz string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timezone = tz
}

// SetDNDEnabled suppresses proactive heartbeat surfacing when enabled.
func (h *HeartbeatService) SetDNDEnabled(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dndEnabled = enabled
}

// SetPromptManager wires external prompt templates for heartbeat cycles.
func (h *HeartbeatService) SetPromptManager(m *prompts.Manager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompts = m
}

// Start launches the background ticker. It is safe to call multiple times;
// subsequent calls are no-ops if already running. The service stops when ctx
// is cancelled or Stop is called.
func (h *HeartbeatService) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running || !h.enabled {
		h.mu.Unlock()
		return
	}
	h.stopChan = make(chan struct{})
	h.running = true
	h.mu.Unlock()

	// Ensure the workspace directory and HEARTBEAT.md exist.
	h.ensureWorkspace()

	go h.runLoop(ctx)
	log.Printf("heartbeat: service started (interval=%s)", h.interval)
}

// Stop signals the background goroutine to exit cleanly.
func (h *HeartbeatService) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	close(h.stopChan)
	h.running = false
	h.wakeMu.Lock()
	if h.wakeTimer != nil {
		h.wakeTimer.Stop()
		h.wakeTimer = nil
	}
	h.pendingWake = nil
	h.wakeMu.Unlock()
	log.Println("heartbeat: service stopped")
}

// IsRunning returns true if the background goroutine is active.
func (h *HeartbeatService) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// runLoop blocks until the context is cancelled or Stop is called.
func (h *HeartbeatService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			h.running = false
			h.mu.Unlock()
			return
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.executeHeartbeat(ctx, "")
		}
	}
}

// executeHeartbeat reads HEARTBEAT.md, builds a prompt, and calls the handler.
func (h *HeartbeatService) executeHeartbeat(ctx context.Context, extraWakeNotes string) {
	start := time.Now()

	mdPath := filepath.Join(h.workspaceDir, heartbeatFile)
	tasks, err := Parser(mdPath)
	if err != nil {
		log.Printf("heartbeat: parse error: %v", err)
		return
	}
	if len(tasks) == 0 {
		log.Println("heartbeat: no tasks found in HEARTBEAT.md — skipping cycle")
		return
	}

	prompt := h.buildPrompt(tasks)
	if w := strings.TrimSpace(extraWakeNotes); w != "" {
		prompt += "\n\n## Out-of-cycle wake notes\n" + w + "\n"
	}
	runtimeSessionID := navi.HeartbeatAutoRuntimeSessionID

	h.mu.RLock()
	handler := h.handler
	b := h.bus
	loader := h.resultLoader
	sender := h.proactiveSender
	h.mu.RUnlock()

	if handler == nil {
		log.Println("heartbeat: no handler registered — skipping cycle")
		return
	}

	if err := handler(ctx, runtimeSessionID, prompt); err != nil {
		log.Printf("heartbeat: handler error: %v", err)
		h.appendLog(fmt.Sprintf("[ERROR] %v", err))
		return
	}

	if loader != nil && sender != nil {
		waitCtx, cancel := context.WithTimeout(ctx, h.resultWait)
		reply, err := loader(waitCtx, runtimeSessionID, start)
		cancel()
		if err != nil {
			log.Printf("heartbeat: result loader error: %v", err)
			h.appendLog(fmt.Sprintf("[WARN] heartbeat result unavailable: %v", err))
		} else if proactive, ok := extractProactiveMessage(reply); ok {
			// Skip replies that are just noise or errors disguised as messages
			if strings.HasPrefix(proactive, "Error:") || strings.HasPrefix(proactive, "CRITICAL:") {
				h.appendLog(fmt.Sprintf("[SKIP] suppressed raw error reply: %s", proactive))
			} else if h.shouldSuppressProactive(h.now()) {
				h.appendLog("[QUIET] proactive heartbeat message suppressed by DND/quiet hours")
			} else if err := sender(ctx, proactive); err != nil {
				log.Printf("heartbeat: proactive send error: %v", err)
				h.appendLog(fmt.Sprintf("[WARN] proactive heartbeat send failed: %v", err))
			} else {
				h.appendLog("[INFO] proactive heartbeat message surfaced to active chat")
			}
		}
	}

	elapsed := time.Since(start)
	h.appendLog(fmt.Sprintf("[OK] %d tasks dispatched in %s", len(tasks), elapsed.Round(time.Millisecond)))

	// Publish a fact so Orchestrator and subscribers know a heartbeat cycle ran.
	if b != nil {
		ev := schema.NewEvent(
			schema.FactNaviHeartbeatDone,
			schema.EventKindFact,
			runtimeSessionID,
			schema.AgentType("navi"),
			schema.NaviHeartbeatDonePayload{
				RuntimeSessionID: runtimeSessionID,
				TaskCount:        len(tasks),
				DurationMs:       elapsed.Milliseconds(),
			},
		)
		if err := b.Publish(ctx, ev); err != nil {
			log.Printf("heartbeat: publish event error: %v", err)
		}
	}
}

// buildPrompt formats a HEARTBEAT.md task list into an LLM-ready prompt.
func (h *HeartbeatService) buildPrompt(tasks []Task) string {
	h.mu.RLock()
	pm := h.prompts
	h.mu.RUnlock()
	if pm != nil {
		lines := make([]prompts.HeartbeatTaskLine, 0, len(tasks))
		for i, t := range tasks {
			lines = append(lines, prompts.HeartbeatTaskLine{N: i + 1, Description: t.Description})
		}
		ts, tzLabel, isLocal := h.heartbeatPromptMeta()
		if s, err := pm.Render(prompts.KindHeartbeatCycle, prompts.HeartbeatCycleData{
			Timestamp: ts,
			TZLabel:   tzLabel,
			IsLocal:   isLocal,
			Tasks:     lines,
		}, prompts.RenderOptions{}); err == nil {
			return s
		} else {
			log.Printf("heartbeat: template render failed, using built-in prompt: %v", err)
		}
	}
	h.mu.RLock()
	tzName := h.timezone
	h.mu.RUnlock()
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}
	now := h.now().In(loc)
	ts := now.Format(time.RFC3339)

	var sb strings.Builder
	if tzName != "UTC" {
		fmt.Fprintf(&sb, "## Heartbeat Cycle — %s (Local: %s)\n\nPlease execute the following scheduled tasks:\n\n", ts, tzName)
	} else {
		fmt.Fprintf(&sb, "## Heartbeat Cycle — %s\n\nPlease execute the following scheduled tasks:\n\n", ts)
	}
	for i, t := range tasks {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, t.Description)
	}
	sb.WriteString("\nIf there is nothing useful to surface to the owner, reply with exactly `HEARTBEAT_OK`.")
	sb.WriteString("\nIf there is something worth surfacing proactively, reply with:\nHEARTBEAT_NOTIFY:\n<concise owner-facing update>")
	return sb.String()
}

func (h *HeartbeatService) heartbeatPromptMeta() (timestamp, tzLabel string, isLocal bool) {
	h.mu.RLock()
	tzName := h.timezone
	h.mu.RUnlock()
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
		tzName = "UTC"
	}
	now := h.now().In(loc)
	return now.Format(time.RFC3339), tzName, tzName != "UTC"
}

// ensureWorkspace creates the workspace directory and seeds HEARTBEAT.md if absent.
func (h *HeartbeatService) ensureWorkspace() {
	if err := os.MkdirAll(h.workspaceDir, 0o755); err != nil {
		log.Printf("heartbeat: mkdir workspace: %v", err)
		return
	}

	mdPath := filepath.Join(h.workspaceDir, heartbeatFile)
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		if err := os.WriteFile(mdPath, []byte(defaultHeartbeatMD), 0o644); err != nil {
			log.Printf("heartbeat: seed %s: %v", heartbeatFile, err)
		} else {
			log.Printf("heartbeat: created default %s in %s", heartbeatFile, h.workspaceDir)
		}
	}
}

// appendLog writes a timestamped entry to workspace/heartbeat.log.
func (h *HeartbeatService) appendLog(msg string) {
	logPath := filepath.Join(h.workspaceDir, logFile)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("heartbeat: open log: %v", err)
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s %s\n", ts, msg)
}

// uiFallbackPhrases are shapeReply/DefaultLayer artifacts that must never
// surface as proactive owner notifications.
var uiFallbackPhrases = []string{
	"i didn't generate a reply",
	"please try again or rephrase",
	"hi! how can i help you today",
}

func extractProactiveMessage(reply string) (string, bool) {
	text := strings.TrimSpace(reply)
	if text == "" {
		return "", false
	}

	// Reject UI shaping fallback messages that leaked from the agent loop.
	lower := strings.ToLower(text)
	for _, phrase := range uiFallbackPhrases {
		if strings.Contains(lower, phrase) {
			return "", false
		}
	}

	// Remove common noise markers
	lines := strings.Split(text, "\n")
	var filtered []string
	explicitNotify := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "HEARTBEAT_OK" {
			continue
		}
		if strings.HasPrefix(trimmed, "HEARTBEAT_NOTIFY:") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "HEARTBEAT_NOTIFY:"))
			explicitNotify = true
		}
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}

	if len(filtered) == 0 {
		return "", false
	}

	final := strings.Join(filtered, "\n")

	// If it's an error message picked up from the session, skip it (OMN-122).
	if strings.HasPrefix(final, "Error:") || strings.HasPrefix(final, "CRITICAL:") {
		return "", false
	}

	// Noise suppression (OMN-121):
	// If the LLM didn't use HEARTBEAT_NOTIFY:, we are very strict about what counts as useful.
	if !explicitNotify {
		// Generic confirmations that all is well should NOT notify the user.
		lower := strings.ToLower(final)
		skipKeywords := []string{
			"everything is fine", "all tasks complete", "no unread", "no pending",
			"no alerts", "system health is good", "i have checked", "dispatched successfully",
			"completed successfully", "performing tasks", "routine check",
		}
		for _, k := range skipKeywords {
			if strings.Contains(lower, k) {
				return "", false
			}
		}

		// Minimum length check for non-explicit reports.
		if len(final) < 20 {
			return "", false
		}
	}

	// Final quality check.
	if strings.EqualFold(final, "OK") || strings.EqualFold(final, "OK.") {
		return "", false
	}

	return final, true
}

func (h *HeartbeatService) shouldSuppressProactive(now time.Time) bool {
	h.mu.RLock()
	dnd := h.dndEnabled
	start := h.quietStart
	end := h.quietEnd
	tzName := h.timezone
	h.mu.RUnlock()

	if dnd {
		return true
	}

	if tzName != "" {
		loc, err := time.LoadLocation(tzName)
		if err == nil {
			now = now.In(loc)
		}
	} else {
		now = now.Local()
	}

	return inQuietHours(now, start, end)
}

func inQuietHours(now time.Time, start, end string) bool {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" || end == "" {
		return false
	}
	startMinutes, ok := parseClockMinutes(start)
	if !ok {
		return false
	}
	endMinutes, ok := parseClockMinutes(end)
	if !ok {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if startMinutes == endMinutes {
		return false
	}
	if startMinutes < endMinutes {
		return current >= startMinutes && current < endMinutes
	}
	return current >= startMinutes || current < endMinutes
}

func parseClockMinutes(value string) (int, bool) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
