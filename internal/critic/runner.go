package critic

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	"github.com/open-navi/navi/internal/prompts"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type PathChecker interface {
	CheckPath(targetPath string) error
}

type Config struct {
	WorkspaceDir      string
	Governor          PathChecker
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

// Execute runs the critic task. All task/outcome/message writes go through ExecutionRecorder and DirectiveWriter (Cognitive facade).
func (r *Runner) Execute(ctx context.Context, task schema.Task) error {
	if task.AssignedTo != schema.AgentCritic {
		return fmt.Errorf("critic: task not assigned to critic: %s", task.AssignedTo)
	}
	if r.cfg.ExecutionRecorder == nil || r.cfg.DirectiveWriter == nil {
		return fmt.Errorf("critic: ExecutionRecorder and DirectiveWriter required (World Model write boundary)")
	}
	task.Status = schema.TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	if err := r.cfg.ExecutionRecorder.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("critic: update task running: %w", err)
	}

	var checker filetools.PathChecker
	if r.cfg.Governor != nil {
		checker = r.cfg.Governor
	}
	var contents string
	for _, surf := range task.Surfaces {
		if surf.Path == "" {
			continue
		}
		result, err := filetools.Execute(ctx, filetools.ReadFileToolName, map[string]any{"path": surf.Path}, r.cfg.WorkspaceDir, checker)
		if err != nil {
			contents += fmt.Sprintf("\n[%s: %v]\n", surf.Path, err)
			continue
		}
		contents += fmt.Sprintf("\n--- %s ---\n%s\n", surf.Path, result)
	}
	if contents == "" {
		contents = "(No file content available for review.)"
	}

	pm := r.cfg.Prompts
	if pm == nil {
		var err error
		pm, err = prompts.EmbeddedManager()
		if err != nil {
			return fmt.Errorf("critic: prompts: %w", err)
		}
	}
	systemPrompt, err := pm.Render(prompts.KindCriticSystem, prompts.CriticSystemData{
		Title:       task.Title,
		Description: task.Description,
	}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("critic: system prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Review this code:\n" + contents},
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
		resp, chatErr := r.cfg.LLM.Chat(execCtx, r.cfg.Model, messages, nil, llm.Options{MaxTokens: 1024, Temperature: 0.3})
		if chatErr != nil {
			task.Status = schema.TaskStatusFailed
			task.UpdatedAt = time.Now().UTC()
			_ = r.cfg.ExecutionRecorder.UpdateTask(execCtx, task)
			_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusFailed, "", chatErr.Error(), "Retry after narrowing the review surfaces or fixing the upstream model/tool issue.", "critic_agent")
			return nil, fmt.Errorf("critic: llm chat: %w", chatErr)
		}

		report := fmt.Sprintf("[Critic report — task %s]\n%s", task.Title, resp.Content)
		msg := schema.DirectiveMessage{
			MessageID:   uuid.New().String(),
			DirectiveID: task.DirectiveID,
			Role:        "navi",
			Content:     report,
			CreatedAt:   time.Now().UTC(),
			TokensUsed:  resp.InputTokens + resp.OutputTokens,
			Model:       r.cfg.Model,
		}
		if err := r.cfg.DirectiveWriter.AppendMessage(execCtx, msg); err != nil {
			slog.Warn("critic: append message failed", "error", err)
		}
		task.Status = schema.TaskStatusCompleted
		task.UpdatedAt = time.Now().UTC()
		if err := r.cfg.ExecutionRecorder.UpdateTask(execCtx, task); err != nil {
			return nil, fmt.Errorf("critic: update task completed: %w", err)
		}
		_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusCompleted, report, "", "", "critic_agent")
		slog.Debug("critic: task completed", "task_id", task.ID)
		return report, nil
	})

	return err
}
