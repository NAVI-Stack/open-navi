package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/prompts"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/ceoai/navi/internal/worldmodel"
	"github.com/google/uuid"
)

// LLMDirectiveAdapter implements LLMAdapter using a real llm.Provider and the
// directive/message tables in SQLite. It is responsible for turning active
// directives into NAVI replies on each orchestration tick.
type LLMDirectiveAdapter struct {
	provider             llm.Provider
	chatModel            string
	db                   *sql.DB
	wm                   *worldmodel.WorldModel
	workspaceDir         string
	prompts              *prompts.Manager
	saveExecutionOutcome func(ctx context.Context, eo schema.ExecutionOutcome) error
}

// NewLLMDirectiveAdapter creates a new directive-centric adapter backed by a real LLM provider.
// World-model fact reads for directive context are routed through wm when non-nil.
// workspaceDir is the agent workspace root; when set and an existing directory, the ACT
// decompose step uses KindOrchestratorDecompose with a directory listing.
// pm may be nil; embedded defaults are used via prompts.EmbeddedManager.
func NewLLMDirectiveAdapter(provider llm.Provider, chatModel string, db *sql.DB, wm *worldmodel.WorldModel, workspaceDir string, pm *prompts.Manager) (*LLMDirectiveAdapter, error) {
	if pm == nil {
		var err error
		pm, err = prompts.EmbeddedManager()
		if err != nil {
			return nil, fmt.Errorf("orchestrator: embedded prompts: %w", err)
		}
		if pm == nil {
			return nil, fmt.Errorf("orchestrator: embedded prompts manager is nil")
		}
	}
	ws := strings.TrimSpace(workspaceDir)
	if ws != "" {
		ws = filepath.Clean(ws)
	}
	return &LLMDirectiveAdapter{
		provider:     provider,
		chatModel:    chatModel,
		db:           db,
		wm:           wm,
		workspaceDir: ws,
		prompts:      pm,
	}, nil
}

// SetSaveExecutionOutcome wires outcome persistence so the adapter can record Delegate commands when assigning tasks.
func (a *LLMDirectiveAdapter) SetSaveExecutionOutcome(fn func(ctx context.Context, eo schema.ExecutionOutcome) error) {
	a.saveExecutionOutcome = fn
}

// HasWork returns whether the orchestrator currently has directive work that
// merits consuming governor action budget on this tick.
func (a *LLMDirectiveAdapter) HasWork(ctx context.Context) (bool, error) {
	directives, err := store.GetActiveDirectives(ctx, a.db)
	if err != nil {
		return false, fmt.Errorf("orchestrator adapter: get directives: %w", err)
	}
	if len(directives) == 0 {
		return false, nil
	}

	d := directives[0]
	msgs, err := store.GetMessages(ctx, a.db, d.DirectiveID, 1)
	if err == nil && len(msgs) > 0 && msgs[0].Role == "navi" && d.Mode != schema.DirectiveModeAct {
		return false, nil
	}
	if d.Mode != schema.DirectiveModeAct {
		return true, nil
	}

	tasks, err := store.GetTasksByDirective(ctx, a.db, d.DirectiveID)
	if err != nil {
		return false, fmt.Errorf("orchestrator adapter: get tasks: %w", err)
	}
	if len(tasks) == 0 {
		return true, nil
	}
	for _, t := range tasks {
		switch t.Status {
		case schema.TaskStatusPending, schema.TaskStatusRunning, schema.TaskStatusBlocked:
			return true, nil
		}
	}
	return true, nil
}

// Decide implements LLMAdapter. It scans for active directives in any mode
// (CHAT/ADVISE/ASSIST/ACT/WATCH), reconstructs the most recent directive's
// conversation history, calls the LLM once, and emits a single
// FactDirectiveReplied event carrying the reply.
func (a *LLMDirectiveAdapter) Decide(ctx context.Context) (Decision, error) {
	directives, err := store.GetActiveDirectives(ctx, a.db)
	if err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: get directives: %w", err)
	}
	if len(directives) == 0 {
		return Decision{}, nil
	}

	d := directives[0]

	// OMN-121: Prevent spamming by checking the last message role.
	// If NAVI already replied, we wait for a new user message or a task state change.
	msgs, err := store.GetMessages(ctx, a.db, d.DirectiveID, 1)
	if err == nil && len(msgs) > 0 && msgs[0].Role == "navi" {
		// Only skip if it's NOT in ACT mode with pending work,
		// because ACT mode might need to transition to completion summary.
		if d.Mode != schema.DirectiveModeAct {
			return Decision{}, nil
		}
	}
	if d.Mode == schema.DirectiveModeAct {
		tasks, err := store.GetTasksByDirective(ctx, a.db, d.DirectiveID)
		if err != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: get tasks: %w", err)
		}
		if len(tasks) == 0 {
			return a.handleImplement(ctx, d)
		}
		if len(tasks) > 0 {
			allTerminal := true
			for _, t := range tasks {
				switch t.Status {
				case schema.TaskStatusPending, schema.TaskStatusRunning, schema.TaskStatusBlocked:
					allTerminal = false
				}
			}
			if allTerminal {
				return a.finalizeDirective(ctx, d, tasks)
			}
		}
		hasPending := false
		for _, t := range tasks {
			switch t.Status {
			case schema.TaskStatusPending, schema.TaskStatusRunning, schema.TaskStatusBlocked:
				hasPending = true
				break
			}
		}
		if !hasPending {
			return a.finalizeDirective(ctx, d, tasks)
		}
	}
	return a.handleDirective(ctx, d)
}

func (a *LLMDirectiveAdapter) handleDirective(ctx context.Context, d schema.Directive) (Decision, error) {
	// Load conversation history (D-009: stateless reconstruction)
	msgs, err := store.GetMessages(ctx, a.db, d.DirectiveID, 50)
	if err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: get messages: %w", err)
	}
	if len(msgs) == 0 {
		return Decision{}, nil
	}

	// Build LLM messages from history — reverse to chronological order
	systemPrompt, err := a.prompts.Render(prompts.KindOrchestratorDirective, prompts.OrchestratorDirectiveData{
		Title:           d.Title,
		ModeDescription: a.directiveModeDescription(d.Mode),
	}, prompts.RenderOptions{})
	if err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: directive prompt: %w", err)
	}
	if a.wm != nil {
		if block, _ := a.wm.FactsBlockForScope(ctx, "directive", d.DirectiveID, true, 30); block != "" {
			systemPrompt += block
		}
		ownerID, _ := store.GetOwnerID(ctx, a.db)
		if ownerID != "" {
			if block, _ := a.wm.ContextBlockForChat(ctx, ownerID, "", 30, 20, 50, 20); block != "" {
				systemPrompt += "\n" + block
			}
		}
	} else {
		facts, _ := store.ListFacts(ctx, a.db, "directive", d.DirectiveID, true, 30, false)
		if block := store.FormatFactsForPrompt(facts); block != "" {
			systemPrompt += block
		}
	}
	llmMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		role := "user"
		if m.Role == "navi" {
			role = "assistant"
		}
		llmMessages = append(llmMessages, llm.Message{
			Role:    role,
			Content: m.Content,
		})
	}

	resp, err := a.provider.Chat(ctx, a.chatModel, llmMessages, nil, llm.Options{
		MaxTokens:   2048,
		Temperature: 0.7,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: llm chat: %w", err)
	}

	// Persist NAVI's reply in the directive log.
	replyMsg := schema.DirectiveMessage{
		MessageID:   uuid.New().String(),
		DirectiveID: d.DirectiveID,
		Role:        "navi",
		Content:     resp.Content,
		CreatedAt:   time.Now().UTC(),
		TokensUsed:  resp.InputTokens + resp.OutputTokens,
		Model:       a.chatModel,
	}
	if err := store.AppendMessage(ctx, a.db, replyMsg); err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: save reply: %w", err)
	}

	// Emit FactDirectiveReplied event so connectors and UIs can react.
	ev := schema.NewEvent(
		schema.FactDirectiveReplied,
		schema.EventKindFact,
		d.DirectiveID,
		schema.AgentNavi,
		schema.DirectiveRepliedPayload{
			DirectiveID: d.DirectiveID,
			MessageID:   replyMsg.MessageID,
		},
	)

	// Estimate cost from token counts (rough: $0.001 per 1K tokens)
	cost := float64(resp.InputTokens+resp.OutputTokens) * 0.001 / 1000.0

	return Decision{
		Events:  []schema.Event{ev},
		Cost:    cost,
		Content: resp.Content,
	}, nil
}

func (a *LLMDirectiveAdapter) directiveModeDescription(mode schema.DirectiveMode) string {
	if a.prompts != nil {
		modes, err := a.prompts.RenderMap(prompts.KindDirectiveModes, nil, prompts.RenderOptions{})
		if err == nil && len(modes) > 0 {
			key := strings.ToLower(string(mode))
			if desc, ok := modes[key]; ok {
				return desc
			}
			if desc, ok := modes["default"]; ok {
				return desc
			}
		}
	}
	switch mode {
	case schema.DirectiveModeChat:
		return "You are in CHAT mode: respond conversationally and helpfully. Do not take external actions."
	case schema.DirectiveModeAdvise:
		return "You are in ADVISE mode: focus on analysis, planning, and clear recommendations. Do not take external actions."
	case schema.DirectiveModeAssist:
		return "You are in ASSIST mode: you may perform routine support tasks within safe, pre-approved boundaries."
	case schema.DirectiveModeAct:
		return "You are in ACT mode: you may execute workflows and automations according to the user's configured skills and policies."
	case schema.DirectiveModeWatch:
		return "You are in WATCH mode: monitor conditions, summarize observations, and suggest or trigger alerts when thresholds are met."
	default:
		return "Respond as NAVI, a helpful software engineering assistant."
	}
}

const maxDecomposeTasks = 8
const maxWorkspaceListEntries = 120

func workspaceDirListForPrompt(root string) (string, bool) {
	if root == "" {
		return "", false
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	sort.Slice(ents, func(i, j int) bool { return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name()) })
	var b strings.Builder
	for i, e := range ents {
		if i >= maxWorkspaceListEntries {
			fmt.Fprintf(&b, "\n... (%d more entries omitted)", len(ents)-i)
			break
		}
		name := e.Name()
		if e.IsDir() {
			b.WriteString(name + "/\n")
		} else {
			b.WriteString(name + "\n")
		}
	}
	if b.Len() == 0 {
		return "(empty workspace)", true
	}
	return strings.TrimSpace(b.String()), true
}

func (a *LLMDirectiveAdapter) handleImplement(ctx context.Context, d schema.Directive) (Decision, error) {
	var systemPrompt string
	var err error
	if dirList, ok := workspaceDirListForPrompt(a.workspaceDir); ok {
		systemPrompt, err = a.prompts.Render(prompts.KindOrchestratorDecompose, prompts.OrchestratorDecomposeData{
			Title:   d.Title,
			DirList: dirList,
		}, prompts.RenderOptions{})
		if err != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: decompose prompt: %w", err)
		}
	} else {
		systemPrompt, err = a.prompts.Render(prompts.KindOrchestratorImplement, prompts.OrchestratorImplementData{
			Title:    d.Title,
			MaxTasks: maxDecomposeTasks,
		}, prompts.RenderOptions{})
		if err != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: implement prompt: %w", err)
		}
	}
	if a.wm != nil {
		if block, _ := a.wm.FactsBlockForScope(ctx, "directive", d.DirectiveID, true, 30); block != "" {
			systemPrompt += block
		}
		ownerID, _ := store.GetOwnerID(ctx, a.db)
		if ownerID != "" {
			if block, _ := a.wm.ContextBlockForChat(ctx, ownerID, "", 30, 20, 50, 20); block != "" {
				systemPrompt += "\n" + block
			}
		}
	} else {
		facts, _ := store.ListFacts(ctx, a.db, "directive", d.DirectiveID, true, 30, false)
		if block := store.FormatFactsForPrompt(facts); block != "" {
			systemPrompt += block
		}
	}

	msgs, err := store.GetMessages(ctx, a.db, d.DirectiveID, 10)
	if err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: get messages: %w", err)
	}
	llmMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		role := "user"
		if m.Role == "navi" {
			role = "assistant"
		}
		llmMessages = append(llmMessages, llm.Message{Role: role, Content: m.Content})
	}
	if len(llmMessages) == 1 {
		llmMessages = append(llmMessages, llm.Message{Role: "user", Content: "Please decompose this directive into implementation tasks."})
	}

	var resp *llm.Response
	retriesLeft := 2
	for {
		var chatErr error
		resp, chatErr = a.provider.Chat(ctx, a.chatModel, llmMessages, []llm.ToolDefinition{DecomposeTasksTool}, llm.Options{
			MaxTokens:           2048,
			Temperature:         0.3,
			ToolChoice:          DecomposeTasksTool.Name,
			ToolCallingRequired: true,
		})
		if chatErr != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: implement chat: %w", chatErr)
		}

		hasDecompose := false
		for _, tc := range resp.ToolCalls {
			if tc.Name == "decompose_tasks" {
				hasDecompose = true
				break
			}
		}

		if !hasDecompose && retriesLeft > 0 {
			retriesLeft--
			slog.Debug("orchestrator: retrying task decomposition", "directive_id", d.DirectiveID, "retries_left", retriesLeft)
			if content := strings.TrimSpace(resp.Content); content != "" {
				llmMessages = append(llmMessages, llm.Message{Role: "assistant", Content: content})
			}
			llmMessages = append(llmMessages, llm.Message{
				Role:    "user",
				Content: "Your previous response did not call the 'decompose_tasks' tool. You must call the 'decompose_tasks' tool to specify the list of tasks needed to implement this directive. Do not answer in plain text without calling this tool.",
			})
			continue
		}
		break
	}

	cost := float64(resp.InputTokens+resp.OutputTokens) * 0.001 / 1000.0
	var events []schema.Event

	for _, tc := range resp.ToolCalls {
		if tc.Name != "decompose_tasks" {
			continue
		}
		tasksRaw, ok := tc.Arguments["tasks"]
		if !ok {
			continue
		}
		tasksSlice, ok := tasksRaw.([]any)
		if !ok {
			continue
		}
		if len(tasksSlice) > maxDecomposeTasks {
			tasksSlice = tasksSlice[:maxDecomposeTasks]
		}
		for _, tAny := range tasksSlice {
			tMap, ok := tAny.(map[string]any)
			if !ok {
				continue
			}
			title, _ := tMap["title"].(string)
			description, _ := tMap["description"].(string)
			surfacePath, _ := tMap["surface_path"].(string)
			if title == "" {
				continue
			}
			if description == "" {
				description = title
			}
			if surfacePath == "" {
				surfacePath = "."
			}
			assignedTo := schema.AgentCoder
			if at, _ := tMap["agent_type"].(string); at != "" {
				switch at {
				case "critic":
					assignedTo = schema.AgentCritic
				case "strategist":
					assignedTo = schema.AgentStrategist
				case "scout":
					assignedTo = schema.AgentScout
				default:
					assignedTo = schema.AgentCoder
				}
			}
			accessMode := "write"
			if assignedTo == schema.AgentCritic {
				accessMode = "read"
			}
			task := schema.Task{
				ID:          uuid.New().String(),
				Title:       title,
				Description: description,
				Status:      schema.TaskStatusPending,
				Risk:        schema.RiskLow,
				AssignedTo:  assignedTo,
				DirectiveID: d.DirectiveID,
				Surfaces:    []schema.SurfaceDeclaration{{Path: surfacePath, AccessMode: accessMode}},
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}
			if err := task.Validate(); err != nil {
				continue
			}
			if err := store.PersistTask(ctx, a.db, task); err != nil {
				return Decision{}, fmt.Errorf("orchestrator adapter: persist task: %w", err)
			}
			events = append(events, schema.NewEvent(
				schema.CmdTaskAssign,
				schema.EventKindCommand,
				task.ID,
				schema.AgentType("orchestrator"),
				schema.TaskAssignedPayload{Task: task},
			))
			if a.saveExecutionOutcome != nil {
				affected, _ := json.Marshal([]struct{ Kind, ID string }{{"task", task.ID}})
				now := time.Now().UTC()
				if err := a.saveExecutionOutcome(ctx, schema.ExecutionOutcome{
					AttemptID:            task.ID + ":1",
					CommandID:            task.ID,
					AttemptNumber:        1,
					CommandType:          schema.CommandTypeDelegate,
					StartTime:            now,
					EndTime:              &now,
					Outcome:              schema.ExecutionOutcomeSucceeded,
					AffectedEntities:     string(affected),
					CompensationRequired: false,
					CompensationStatus:   schema.CompensationStatusNotRequired,
					RecoveryStatus:       schema.RecoveryStatusNotRequired,
				}); err != nil {
					slog.Warn("orchestrator: failed to save execution outcome", "task_id", task.ID, "error", err)
				}
			}
		}
		break
	}

	if len(events) == 0 {
		replyMsg := schema.DirectiveMessage{
			MessageID:   uuid.New().String(),
			DirectiveID: d.DirectiveID,
			Role:        "navi",
			Content:     resp.Content,
			CreatedAt:   time.Now().UTC(),
			TokensUsed:  resp.InputTokens + resp.OutputTokens,
			Model:       a.chatModel,
		}
		if err := store.AppendMessage(ctx, a.db, replyMsg); err != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: save reply: %w", err)
		}
		events = append(events, schema.NewEvent(
			schema.FactDirectiveReplied,
			schema.EventKindFact,
			d.DirectiveID,
			schema.AgentNavi,
			schema.DirectiveRepliedPayload{DirectiveID: d.DirectiveID, MessageID: replyMsg.MessageID},
		))
	} else {
		summary := fmt.Sprintf("Decomposed into %d task(s). Tasks have been assigned to the coder worker.", len(events))
		replyMsg := schema.DirectiveMessage{
			MessageID:   uuid.New().String(),
			DirectiveID: d.DirectiveID,
			Role:        "navi",
			Content:     summary,
			CreatedAt:   time.Now().UTC(),
			TokensUsed:  resp.InputTokens + resp.OutputTokens,
			Model:       a.chatModel,
		}
		if err := store.AppendMessage(ctx, a.db, replyMsg); err != nil {
			return Decision{}, fmt.Errorf("orchestrator adapter: save decomposition reply: %w", err)
		}
		events = append(events, schema.NewEvent(
			schema.FactDirectiveReplied,
			schema.EventKindFact,
			d.DirectiveID,
			schema.AgentNavi,
			schema.DirectiveRepliedPayload{DirectiveID: d.DirectiveID, MessageID: replyMsg.MessageID},
		))
		events = append(events, orchestratorReflectionEvent(d.DirectiveID, "Task decomposition complete", map[string]any{
			"summary": summary,
			"details": resp.Content,
			"facts": []map[string]string{
				{
					"scope":    "directive",
					"scope_id": d.DirectiveID,
					"category": "project_decision",
					"key":      "task_decomposition",
					"value":    summary,
				},
			},
		}))
	}

	return Decision{
		Events:  events,
		Cost:    cost,
		Content: resp.Content,
	}, nil
}

func (a *LLMDirectiveAdapter) finalizeDirective(ctx context.Context, d schema.Directive, tasks []schema.Task) (Decision, error) {
	var completed, failed int
	for _, t := range tasks {
		switch t.Status {
		case schema.TaskStatusCompleted:
			completed++
		case schema.TaskStatusFailed, schema.TaskStatusCancelled:
			failed++
		}
	}

	status := schema.DirectiveStatusComplete
	if failed > 0 {
		status = schema.DirectiveStatusStalled
	}
	if err := store.UpdateDirectiveStatus(ctx, a.db, d.DirectiveID, status); err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: update directive status: %w", err)
	}

	summary := fmt.Sprintf("Directive task execution summary: %d completed, %d failed.", completed, failed)
	if facts, err := store.ListFacts(ctx, a.db, "directive", d.DirectiveID, false, 5, false); err == nil {
		var recent []string
		for _, fact := range facts {
			if fact.Category == "task_outcome" {
				recent = append(recent, fact.Value)
			}
		}
		if len(recent) > 0 {
			summary += " Recent outcomes: " + strings.Join(recent, " | ")
		}
	}

	learningCaptured := false
	if ownerID, err := store.GetOwnerID(ctx, a.db); err == nil && ownerID != "" {
		key := truncateDirectiveKey(d.DirectiveID)
		if taskFacts, err := store.ListFacts(ctx, a.db, "directive", d.DirectiveID, true, 50, false); err == nil {
			var highlights []string
			for _, fact := range taskFacts {
				if fact.Category == "task_outcome" {
					highlights = append(highlights, fact.Value)
					if len(highlights) == 3 {
						break
					}
				}
			}
			if len(highlights) > 0 {
				learning := fmt.Sprintf("Directive '%s' outcomes: %s", d.Title, strings.Join(highlights, " | "))
				if err := store.SaveFact(ctx, a.db, store.Fact{
					Scope:    "owner",
					ScopeID:  ownerID,
					Category: "technical_context",
					Key:      "learned_from_directive_" + key,
					Value:    learning,
					Source:   "post_directive_reflection",
				}); err != nil {
					return Decision{}, fmt.Errorf("orchestrator adapter: save owner learning fact: %w", err)
				}
				learningCaptured = true
			}
		}
		if !learningCaptured {
			if err := store.SaveFact(ctx, a.db, store.Fact{
				Scope:    "owner",
				ScopeID:  ownerID,
				Category: "technical_context",
				Key:      "learned_from_directive_" + key,
				Value:    summary,
				Source:   "post_directive_reflection",
			}); err != nil {
				slog.Warn("orchestrator: failed to save fallback learning fact", "key", key, "error", err)
			}
		}
	}
	if learningCaptured {
		summary += " Promoted key task outcomes into owner-scoped context."
	}
	replyMsg := schema.DirectiveMessage{
		MessageID:   uuid.New().String(),
		DirectiveID: d.DirectiveID,
		Role:        "navi",
		Content:     summary,
		CreatedAt:   time.Now().UTC(),
		Model:       a.chatModel,
	}
	if err := store.AppendMessage(ctx, a.db, replyMsg); err != nil {
		return Decision{}, fmt.Errorf("orchestrator adapter: append directive summary: %w", err)
	}

	ev := schema.NewEvent(
		schema.FactDirectiveReplied,
		schema.EventKindFact,
		d.DirectiveID,
		schema.AgentNavi,
		schema.DirectiveRepliedPayload{DirectiveID: d.DirectiveID, MessageID: replyMsg.MessageID},
	)
	events := []schema.Event{ev}
	if ownerID, err := store.GetOwnerID(ctx, a.db); err == nil && ownerID != "" {
		events = append(events, orchestratorReflectionEvent(d.DirectiveID, "Directive completed", map[string]any{
			"summary":         summary,
			"tasks_completed": completed,
			"tasks_failed":    failed,
			"facts": []map[string]string{
				{
					"scope":    "owner",
					"scope_id": ownerID,
					"category": "technical_context",
					"key":      "learned_from_directive_" + truncateDirectiveKey(d.DirectiveID),
					"value":    summary,
				},
			},
		}))
	}
	return Decision{Events: events}, nil
}

func orchestratorReflectionEvent(directiveID, summary string, details map[string]any) schema.Event {
	data, _ := json.Marshal(details)
	return schema.NewEvent(
		schema.FactReflectionQueued,
		schema.EventKindFact,
		directiveID,
		schema.AgentType("orchestrator"),
		schema.ReflectionPayload{
			ID:          uuid.New().String(),
			DirectiveID: directiveID,
			Tier:        schema.ReflectionTierShallow,
			Summary:     summary,
			Details:     string(data),
			CreatedAt:   time.Now().UTC(),
		},
	)
}

func truncateDirectiveKey(directiveID string) string {
	if len(directiveID) > 8 {
		return directiveID[:8]
	}
	return directiveID
}
