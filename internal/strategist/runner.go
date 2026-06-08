package strategist

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/ceoai/navi/internal/cognitive"
	"github.com/ceoai/navi/internal/command"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/prompts"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

type Config struct {
	LLM               llm.Provider
	Model             string
	DB                *sql.DB
	ExecutionRecorder cognitive.ExecutionRecorder
	DirectiveWriter   cognitive.DirectiveWriter
	CompensatorFor    func(schema.ExecutionOutcome) store.Compensator
	Prompts           *prompts.Manager
}

type Runner struct {
	cfg Config
}

func NewRunner(cfg Config) *Runner {
	if cfg.Model == "" {
		cfg.Model = "chat"
	}
	return &Runner{cfg: cfg}
}

// Execute runs the task. All task/outcome/message writes go through ExecutionRecorder and DirectiveWriter (Cognitive facade).
func (r *Runner) Execute(ctx context.Context, task schema.Task) error {
	if task.AssignedTo != schema.AgentStrategist {
		return fmt.Errorf("strategist: task not assigned to strategist: %s", task.AssignedTo)
	}
	if r.cfg.ExecutionRecorder == nil || r.cfg.DirectiveWriter == nil {
		return fmt.Errorf("strategist: ExecutionRecorder and DirectiveWriter required (World Model write boundary)")
	}
	task.Status = schema.TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	if err := r.cfg.ExecutionRecorder.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("strategist: update task running: %w", err)
	}

	pm := r.cfg.Prompts
	if pm == nil {
		var perr error
		pm, perr = prompts.EmbeddedManager()
		if perr != nil {
			return fmt.Errorf("strategist: prompts: %w", perr)
		}
	}
	systemPrompt, err := pm.Render(prompts.KindStrategistSystem, struct{}{}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("strategist: system prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Task: %s\n\nDescription: %s", task.Title, task.Description)},
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
		resp, chatErr := r.cfg.LLM.Chat(execCtx, r.cfg.Model, messages, nil, llm.Options{MaxTokens: 1024, Temperature: 0.4})
		if chatErr != nil {
			task.Status = schema.TaskStatusFailed
			task.UpdatedAt = time.Now().UTC()
			_ = r.cfg.ExecutionRecorder.UpdateTask(execCtx, task)
			_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusFailed, "", chatErr.Error(), "Re-run the strategist task after checking model availability and prompt scope.", "strategist_agent")
			return nil, fmt.Errorf("strategist: llm chat: %w", chatErr)
		}

		doc := fmt.Sprintf("[Design — %s]\n%s", task.Title, resp.Content)
		msg := schema.DirectiveMessage{
			MessageID:   uuid.New().String(),
			DirectiveID: task.DirectiveID,
			Role:        "navi",
			Content:     doc,
			CreatedAt:   time.Now().UTC(),
			TokensUsed:  resp.InputTokens + resp.OutputTokens,
			Model:       r.cfg.Model,
		}
		if err := r.cfg.DirectiveWriter.AppendMessage(execCtx, msg); err != nil {
			slog.Warn("strategist: append message failed", "error", err)
		}
		task.Status = schema.TaskStatusCompleted
		task.UpdatedAt = time.Now().UTC()
		if err := r.cfg.ExecutionRecorder.UpdateTask(execCtx, task); err != nil {
			return nil, fmt.Errorf("strategist: update task completed: %w", err)
		}
		_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusCompleted, doc, "", "", "strategist_agent")
		slog.Debug("strategist: task completed", "task_id", task.ID)
		return doc, nil
	})

	return err
}
