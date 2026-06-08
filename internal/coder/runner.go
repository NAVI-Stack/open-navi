package coder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/cognitive"
	"github.com/ceoai/navi/internal/command"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/filetools"
	"github.com/ceoai/navi/internal/prompts"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

// PathChecker is implemented by governor.Governor; used to validate paths before file I/O.
type PathChecker interface {
	CheckPath(targetPath string) error
}

// Config holds dependencies for the coder worker.
type Config struct {
	WorkspaceDir      string
	Governor          PathChecker
	LLM               llm.Provider
	Model             string
	DB                *sql.DB
	ExecutionRecorder cognitive.ExecutionRecorder // when set, task/outcome writes go through Cognitive facade
	DirectiveWriter   cognitive.DirectiveWriter
	Bus               bus.Bus
	CompensatorFor    func(schema.ExecutionOutcome) store.Compensator
	Prompts           *prompts.Manager
}

// Runner executes tasks assigned to the coder agent using file tools and LLM.
type Runner struct {
	cfg Config
}

// NewRunner creates a coder runner.
func NewRunner(cfg Config) *Runner {
	if cfg.Model == "" {
		cfg.Model = "chat"
	}
	return &Runner{cfg: cfg}
}

// Execute runs the task: updates status to running, runs LLM with file tools until done, then updates status.
// All task and execution-outcome writes go through ExecutionRecorder (Cognitive facade); direct store access is not used.
func (r *Runner) Execute(ctx context.Context, task schema.Task) error {
	if task.AssignedTo != schema.AgentCoder {
		return fmt.Errorf("coder: task not assigned to coder: %s", task.AssignedTo)
	}
	if r.cfg.ExecutionRecorder == nil || r.cfg.DirectiveWriter == nil {
		return fmt.Errorf("coder: ExecutionRecorder and DirectiveWriter required (World Model write boundary)")
	}
	execRoot, err := r.resolveExecutionRoot(ctx, task)
	if err != nil {
		reason := err.Error()
		task.Status = schema.TaskStatusBlocked
		task.LifecyclePhase = "execution_blocked"
		task.BlockReason = reason
		task.UpdatedAt = time.Now().UTC()
		if updateErr := r.cfg.ExecutionRecorder.UpdateTask(ctx, task); updateErr != nil {
			return fmt.Errorf("coder: block task after execution root failure: %w", updateErr)
		}
		recordTaskOutcome(ctx, r.cfg.ExecutionRecorder, r.cfg.DirectiveWriter, r.cfg.Bus, task, r.cfg.Model, "coder_agent", "", reason)
		return fmt.Errorf("coder: execution root unavailable: %w", err)
	}
	if task.WorkspaceID == "" {
		task.WorkspaceID = execRoot.WorkspaceID
	}
	task.Status = schema.TaskStatusRunning
	task.LifecyclePhase = "execution_running"
	task.BlockReason = ""
	task.UpdatedAt = time.Now().UTC()
	if err := r.cfg.ExecutionRecorder.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("coder: update task running: %w", err)
	}

	pm := r.cfg.Prompts
	if pm == nil {
		var err error
		pm, err = prompts.EmbeddedManager()
		if err != nil {
			return fmt.Errorf("coder: prompts: %w", err)
		}
	}
	systemPrompt, err := pm.Render(prompts.KindCoderSystem, prompts.CoderSystemData{
		Title:       task.Title,
		Description: task.Description,
		Surfaces:    fmt.Sprintf("%v", task.Surfaces),
	}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("coder: system prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Please implement the task. Start by exploring the relevant files if needed."},
	}
	tools := filetools.Tools()
	var checker filetools.PathChecker
	if r.cfg.Governor != nil {
		checker = r.cfg.Governor
	}

	saveOutcome := func(execCtx context.Context, eo schema.ExecutionOutcome) error {
		return r.cfg.ExecutionRecorder.SaveExecutionOutcome(execCtx, eo)
	}
	executor := command.NewExecutor(saveOutcome)

	_, err = executor.Execute(ctx, command.Descriptor{
		Type:       schema.CommandTypeDelegate,
		UserFacing: true,
		TaskID:     task.ID,
	}, func(execCtx context.Context) (any, error) {
		retriesLeft := 2
		for {
			resp, chatErr := r.cfg.LLM.Chat(execCtx, r.cfg.Model, messages, tools, llm.Options{
				MaxTokens:           4096,
				Temperature:         0.2,
				ToolCallingRequired: len(messages) <= 2,
			})
			if chatErr != nil {
				task.Status = schema.TaskStatusFailed
				task.LifecyclePhase = "execution_failed"
				task.BlockReason = chatErr.Error()
				task.UpdatedAt = time.Now().UTC()
				_ = r.cfg.ExecutionRecorder.UpdateTask(execCtx, task)
				recordTaskOutcome(execCtx, r.cfg.ExecutionRecorder, r.cfg.DirectiveWriter, r.cfg.Bus, task, r.cfg.Model, "coder_agent", "", chatErr.Error())
				return nil, fmt.Errorf("coder: llm chat: %w", chatErr)
			}

			if len(resp.ToolCalls) == 0 {
				if len(messages) <= 2 && retriesLeft > 0 {
					retriesLeft--
					slog.Debug("coder: retrying missing required tool call", "task_id", task.ID, "retries_left", retriesLeft)
					if content := strings.TrimSpace(resp.Content); content != "" {
						messages = append(messages, llm.Message{Role: "assistant", Content: content})
					}
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: "Your previous response did not call any tools. You must call a tool (e.g. read_file, list_dir, write_file) to perform this task. Do not answer in plain text without calling a tool.",
					})
					continue
				}

				if len(messages) <= 2 {
					chatErr = fmt.Errorf("coder: model failed to emit required tool calls after retries")
					task.Status = schema.TaskStatusFailed
					task.LifecyclePhase = "execution_failed"
					task.BlockReason = chatErr.Error()
					task.UpdatedAt = time.Now().UTC()
					_ = r.cfg.ExecutionRecorder.UpdateTask(execCtx, task)
					recordTaskOutcome(execCtx, r.cfg.ExecutionRecorder, r.cfg.DirectiveWriter, r.cfg.Bus, task, r.cfg.Model, "coder_agent", "", chatErr.Error())
					return nil, fmt.Errorf("coder: llm chat: %w", chatErr)
				}

				slog.Debug("coder: task completed", "task_id", task.ID, "summary_len", len(resp.Content))
				summary := fmt.Sprintf("[Coder result — task %s]\n%s", task.Title, resp.Content)
				msg := schema.DirectiveMessage{
					MessageID:   uuid.New().String(),
					DirectiveID: task.DirectiveID,
					Role:        "navi",
					Content:     summary,
					CreatedAt:   time.Now().UTC(),
					TokensUsed:  resp.InputTokens + resp.OutputTokens,
					Model:       r.cfg.Model,
				}
				if err := r.cfg.DirectiveWriter.AppendMessage(execCtx, msg); err != nil {
					slog.Warn("coder: append message failed", "error", err)
				}
				task.Status = schema.TaskStatusCompleted
				task.LifecyclePhase = "execution_completed"
				task.BlockReason = ""
				task.UpdatedAt = time.Now().UTC()
				if err := r.cfg.ExecutionRecorder.UpdateTask(execCtx, task); err != nil {
					return nil, fmt.Errorf("coder: update task completed: %w", err)
				}
				recordTaskOutcome(execCtx, r.cfg.ExecutionRecorder, r.cfg.DirectiveWriter, r.cfg.Bus, task, r.cfg.Model, "coder_agent", resp.Content, "")
				return resp.Content, nil
			}

			messages = append(messages, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
			for _, tc := range resp.ToolCalls {
				args := tc.Arguments
				if args == nil {
					args = make(map[string]any)
				}
				result, toolErr := filetools.Execute(execCtx, tc.Name, args, execRoot.Path, checker)
				if toolErr != nil {
					result = "Error: " + toolErr.Error()
				}
				messages = append(messages, llm.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
			}
		}
	})

	return err
}

type executionRoot struct {
	Path        string
	WorkspaceID string
}

func (r *Runner) resolveExecutionRoot(ctx context.Context, task schema.Task) (executionRoot, error) {
	task.WorkspaceID = strings.TrimSpace(task.WorkspaceID)
	task.ProjectID = strings.TrimSpace(task.ProjectID)

	if task.WorkspaceID != "" {
		return r.resolveWorkspaceExecutionRoot(ctx, task.WorkspaceID)
	}
	if task.ProjectID != "" {
		return r.resolveProjectExecutionRoot(ctx, task.ProjectID)
	}

	workspaceDir := strings.TrimSpace(r.cfg.WorkspaceDir)
	if workspaceDir == "" {
		return executionRoot{}, fmt.Errorf("workspace directory is not configured")
	}
	return executionRoot{Path: filepath.Clean(workspaceDir)}, nil
}

func (r *Runner) resolveProjectExecutionRoot(ctx context.Context, projectID string) (executionRoot, error) {
	if r.cfg.DB == nil {
		return executionRoot{}, fmt.Errorf("database is required to resolve project workspace")
	}
	project, err := store.GetProject(ctx, r.cfg.DB, projectID)
	if err != nil {
		return executionRoot{}, err
	}
	if strings.TrimSpace(project.WorkspaceID) != "" {
		return r.resolveWorkspaceExecutionRoot(ctx, project.WorkspaceID)
	}
	workspace, err := store.GetWorkspaceByProjectID(ctx, r.cfg.DB, project.ID)
	if err != nil {
		return executionRoot{}, fmt.Errorf("project %s has no active workspace binding", project.ID)
	}
	return executionRootForWorkspace(workspace)
}

func (r *Runner) resolveWorkspaceExecutionRoot(ctx context.Context, workspaceID string) (executionRoot, error) {
	if r.cfg.DB == nil {
		return executionRoot{}, fmt.Errorf("database is required to resolve workspace %s", workspaceID)
	}
	workspace, err := store.GetWorkspace(ctx, r.cfg.DB, workspaceID)
	if err != nil {
		return executionRoot{}, err
	}
	return executionRootForWorkspace(workspace)
}

func executionRootForWorkspace(workspace schema.Workspace) (executionRoot, error) {
	if workspace.Status != schema.WorkspaceStatusActive {
		return executionRoot{}, fmt.Errorf("workspace %s is not active", workspace.ID)
	}
	if root := firstRoot(workspace.RepoRoots); root != "" {
		return executionRoot{Path: filepath.Clean(root), WorkspaceID: workspace.ID}, nil
	}
	if root := firstRoot(workspace.LocalRoots); root != "" {
		return executionRoot{Path: filepath.Clean(root), WorkspaceID: workspace.ID}, nil
	}
	return executionRoot{}, fmt.Errorf("workspace %s has no local or repo roots", workspace.ID)
}

func firstRoot(roots []string) string {
	for _, root := range roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func recordTaskOutcome(ctx context.Context, recorder cognitive.ExecutionRecorder, directiveWriter cognitive.DirectiveWriter, eventBus bus.Bus, task schema.Task, model, source, summary, failureReason string) {
	if recorder == nil {
		return
	}
	value := fmt.Sprintf("Task '%s' completed.", task.Title)
	if summary != "" {
		value = fmt.Sprintf("Task '%s' completed: %s", task.Title, summary)
	}
	if failureReason != "" {
		value = fmt.Sprintf("Task '%s' FAILED: %s", task.Title, failureReason)
		if directiveWriter != nil && task.DirectiveID != "" {
			msg := schema.DirectiveMessage{
				MessageID:   uuid.New().String(),
				DirectiveID: task.DirectiveID,
				Role:        "navi",
				Content:     "[Task failure — " + task.Title + "]\n" + failureReason,
				CreatedAt:   time.Now().UTC(),
				Model:       model,
			}
			if err := directiveWriter.AppendMessage(ctx, msg); err != nil {
				slog.Warn("coder: append failure message failed", "error", err)
			}
		}
	}
	fact := schema.Fact{
		ID:       uuid.New().String(),
		Scope:    "directive",
		ScopeID:  task.DirectiveID,
		Category: "task_outcome",
		Key:      "task_" + task.ID + "_outcome",
		Value:    value,
		Source:   source,
	}
	prov := &schema.EntityProvenance{
		Source:             source,
		Timestamp:          time.Now().UTC(),
		Confidence:         1.0,
		ReinforcementCount: 1,
	}
	if err := recorder.SaveFact(ctx, fact, prov); err != nil {
		slog.Warn("coder: save task outcome fact failed", "task_id", task.ID, "error", err)
	}
	if eventBus != nil {
		details, err := json.Marshal(map[string]any{
			"summary": summary,
			"details": value,
			"facts": []map[string]string{
				{
					"scope":    "directive",
					"scope_id": task.DirectiveID,
					"category": "task_outcome",
					"key":      fact.Key,
					"value":    value,
				},
			},
			"task_id":      task.ID,
			"project_id":   task.ProjectID,
			"workspace_id": task.WorkspaceID,
			"task_class":   string(task.TaskClass),
			"outcome":      map[bool]string{true: "failed", false: "completed"}[failureReason != ""],
			"error":        failureReason,
		})
		if err != nil {
			slog.Warn("coder: marshal task outcome details failed", "task_id", task.ID, "error", err)
		} else {
			ev := schema.NewEvent(
				schema.FactReflectionQueued,
				schema.EventKindFact,
				task.ID,
				schema.AgentCoder,
				schema.ReflectionPayload{
					ID:          uuid.New().String(),
					DirectiveID: task.DirectiveID,
					Tier:        schema.ReflectionTierShallow,
					Summary:     "Task executed",
					Details:     string(details),
					CreatedAt:   time.Now().UTC(),
				},
			)
			if err := eventBus.Publish(ctx, ev); err != nil {
				slog.Warn("coder: publish reflection failed", "task_id", task.ID, "error", err)
			}
		}
	}
}
