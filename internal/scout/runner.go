package scout

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/prompts"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// Config holds dependencies for the scout worker.
type Config struct {
	LLM               llm.Provider
	Model             string
	DB                *sql.DB
	Skills            *skill.SkillRegistry
	ExecutionRecorder cognitive.ExecutionRecorder
	DirectiveWriter   cognitive.DirectiveWriter
	CompensatorFor    func(schema.ExecutionOutcome) store.Compensator
	Prompts           *prompts.Manager
}

// Runner executes tasks assigned to the scout agent (research / external knowledge).
type Runner struct {
	cfg Config
}

// NewRunner creates a scout runner.
func NewRunner(cfg Config) *Runner {
	if cfg.Model == "" {
		cfg.Model = "chat"
	}
	return &Runner{cfg: cfg}
}

// Execute runs the task: produces a short research note and appends it to the directive.
// All task/outcome/message writes go through ExecutionRecorder and DirectiveWriter (Cognitive facade).
func (r *Runner) Execute(ctx context.Context, task schema.Task) error {
	if task.AssignedTo != schema.AgentScout {
		return fmt.Errorf("scout: task not assigned to scout: %s", task.AssignedTo)
	}
	if r.cfg.ExecutionRecorder == nil || r.cfg.DirectiveWriter == nil {
		return fmt.Errorf("scout: ExecutionRecorder and DirectiveWriter required (World Model write boundary)")
	}
	task.Status = schema.TaskStatusRunning
	task.UpdatedAt = time.Now().UTC()
	if err := r.cfg.ExecutionRecorder.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("scout: update task running: %w", err)
	}

	pm := r.cfg.Prompts
	if pm == nil {
		var err error
		pm, err = prompts.EmbeddedManager()
		if err != nil {
			return fmt.Errorf("scout: prompts: %w", err)
		}
	}
	systemPrompt, err := pm.Render(prompts.KindScoutSystem, struct{}{}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("scout: system prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Research topic: %s\n\nDescription: %s", task.Title, task.Description)},
	}

	saveOutcome := func(execCtx context.Context, eo schema.ExecutionOutcome) error {
		return r.cfg.ExecutionRecorder.SaveExecutionOutcome(execCtx, eo)
	}
	executor := command.NewExecutor(saveOutcome)

	var tools []llm.ToolDefinition
	if r.cfg.Skills != nil {
		skillsSnapshot := r.cfg.Skills.Snapshot()
		for _, s := range skillsSnapshot.Skills {
			// Scout only uses a subset of skills: search, summarize, etc.
			if s.Name == "scout-search" || s.Name == "summarize" || s.Name == "github" {
				if entry, ok := r.cfg.Skills.Lookup(s.Name); ok && entry.Spec != nil {
					for _, idx := range entry.Spec.Interfaces {
						tools = append(tools, llm.ToolDefinition{
							Name:        fmt.Sprintf("%s_%s", entry.Spec.SkillID, idx.Name),
							Description: entry.Skill.Description,
							Parameters:  idx.InputSchema,
						})
					}
				}
			}
		}
	}

	var finalContent string
	var totalTokens int

	_, err = executor.Execute(ctx, command.Descriptor{
		Type:       schema.CommandTypeDelegate,
		UserFacing: true,
		TaskID:     task.ID,
	}, func(execCtx context.Context) (any, error) {
		maxTurns := 5
		retriesLeft := 2
		for turn := 0; turn < maxTurns; turn++ {
			resp, chatErr := r.cfg.LLM.Chat(execCtx, r.cfg.Model, messages, tools, llm.Options{
				MaxTokens:           1024,
				Temperature:         0.4,
				ToolCallingRequired: turn == 0 && len(tools) > 0,
			})
			if chatErr != nil {
				_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusFailed, "", chatErr.Error(), "Retry the research task after checking tool availability or reducing scope.", "scout_agent")
				return nil, fmt.Errorf("scout: llm chat: %w", chatErr)
			}
			totalTokens += resp.InputTokens + resp.OutputTokens

			if len(resp.ToolCalls) == 0 {
				if turn == 0 && len(tools) > 0 && retriesLeft > 0 {
					retriesLeft--
					slog.Debug("scout: retrying missing required tool call", "task_id", task.ID, "retries_left", retriesLeft)
					if content := strings.TrimSpace(resp.Content); content != "" {
						messages = append(messages, llm.Message{Role: "assistant", Content: content})
					}
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: "Your previous response did not call any tools. You must call a tool to research this topic (e.g. search, lookup). Do not answer in plain text without calling a tool.",
					})
					turn-- // Repeat turn 0
					continue
				}

				if turn == 0 && len(tools) > 0 {
					chatErr = fmt.Errorf("scout: model failed to emit required tool calls after retries")
					_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusFailed, "", chatErr.Error(), "Retry the research task after checking tool availability or reducing scope.", "scout_agent")
					return nil, fmt.Errorf("scout: llm chat: %w", chatErr)
				}

				finalContent = resp.Content
				break
			}

			if resp.Content != "" {
				messages = append(messages, llm.Message{Role: "assistant", Content: resp.Content})
			} else if len(resp.ToolCalls) > 0 {
				messages = append(messages, llm.Message{Role: "assistant", ToolCalls: resp.ToolCalls})
			}

			// Handle tool calls
			for _, tc := range resp.ToolCalls {
				slog.Debug("scout: tool call", "name", tc.Name, "args", tc.Arguments)
				var toolResult string
				if entry, iface, ok := r.cfg.Skills.FindTool(tc.Name); ok {
					res, err := skill.Execute(execCtx, entry, iface, tc.Arguments)
					if err != nil {
						toolResult = fmt.Sprintf("Error: %v", err)
					} else {
						toolResult = res
					}
				} else {
					toolResult = fmt.Sprintf("Error: tool %q not found", tc.Name)
				}
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}
		}

		if finalContent == "" {
			_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusFailed, "", "Scout reached max turns without producing a research note.", "Reduce the scope or inspect failing tool calls before retrying.", "scout_agent")
			return nil, fmt.Errorf("scout: reached max turns without research result")
		}

		note := fmt.Sprintf("[Scout research — %s]\n%s", task.Title, finalContent)
		msg := schema.DirectiveMessage{
			MessageID:   uuid.New().String(),
			DirectiveID: task.DirectiveID,
			Role:        "navi",
			Content:     note,
			CreatedAt:   time.Now().UTC(),
			TokensUsed:  totalTokens,
			Model:       r.cfg.Model,
		}
		if err := r.cfg.DirectiveWriter.AppendMessage(execCtx, msg); err != nil {
			slog.Warn("scout: append message failed", "error", err)
		}
		task.Status = schema.TaskStatusCompleted
		task.UpdatedAt = time.Now().UTC()
		if err := r.cfg.ExecutionRecorder.UpdateTask(execCtx, task); err != nil {
			return nil, fmt.Errorf("scout: update task completed: %w", err)
		}
		_ = store.SaveTaskOutcomeFact(execCtx, r.cfg.DB, task, schema.TaskStatusCompleted, note, "", "", "scout_agent")
		slog.Debug("scout: task completed", "task_id", task.ID)
		return note, nil
	})

	if err != nil {
		task.Status = schema.TaskStatusFailed
		task.UpdatedAt = time.Now().UTC()
		_ = r.cfg.ExecutionRecorder.UpdateTask(ctx, task)
	}

	return err
}
