package navi

import (
	"context"
	"log/slog"

	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
)

const (
	EvtNaviMessageReceived       = "navi.cmd.message"
	EvtNaviReplied               = "navi.fact.replied"
	EvtNaviReplyChunk            = "navi.fact.reply.chunk"
	EvtNaviExperienceModeChanged = "navi.fact.experience_mode_changed"
	EvtNaviSkillLoaded           = "navi.fact.skill_loaded"
	EvtNaviSkillError            = "navi.fact.skill_error"
)

// NaviReplyChunkPayload is the payload for navi.fact.reply.chunk (streaming token delta).
type NaviReplyChunkPayload struct {
	ChatID string `json:"chat_id"`
	Delta  string `json:"delta"`
}

// NaviMessagePayload represents an incoming message meant for NAVI.
type NaviMessagePayload struct {
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
	Source  string `json:"source"` // "telegram" | "web" | "ide"
}

// NaviRepliedPayload represents a completed response from NAVI.
type NaviRepliedPayload struct {
	ChatID         string         `json:"chat_id"`
	MessageID      string         `json:"message_id"`
	ExperienceMode ExperienceMode `json:"experience_mode"`
}

// ---------------------------------------------------------------------------
// Run lifecycle event emitters
// ---------------------------------------------------------------------------

// emitRunStarted publishes a run.started event through the bus.
func (l *AgentLoop) emitRunStarted(ctx context.Context, run *naviruntime.RunState) {
	if l.cfg.Bus == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactRunStarted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityUser,
		schema.RunStartedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			ExperienceMode:   run.ExperienceMode,
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish run.started", "error", err)
	}
}

// emitRunCompleted publishes a run.completed event through the bus.
func (l *AgentLoop) emitRunCompleted(ctx context.Context, run *naviruntime.RunState, replyLen int) {
	if l.cfg.Bus == nil {
		return
	}
	run.SetStatus("completed")
	ev := schema.NewRunEvent(
		schema.FactRunCompleted,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityUser,
		schema.RunCompletedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			ReplyLen:         replyLen,
			ToolCalls:        run.ToolCalls,
			DurationMs:       run.DurationMs(),
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish run.completed", "error", err)
	}
}

// emitRunFailed publishes a run.failed event through the bus.
func (l *AgentLoop) emitRunFailed(ctx context.Context, run *naviruntime.RunState, errMsg string) {
	if l.cfg.Bus == nil {
		return
	}
	run.SetStatus("failed")
	ev := schema.NewRunEvent(
		schema.FactRunFailed,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityUser,
		schema.RunFailedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			Error:            errMsg,
			DurationMs:       run.DurationMs(),
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish run.failed", "error", err)
	}
}

// emitToolCallStarted publishes a tool.call.started event through the bus.
func (l *AgentLoop) emitArtifactCreated(ctx context.Context, runtimeSessionID, runID string, art *schema.Artifact) {
	if l.cfg.Bus == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactArtifactCreated,
		schema.EventKindFact,
		runtimeSessionID,
		schema.AgentNavi,
		runID,
		schema.VisibilityUser,
		schema.ArtifactCreatedPayload{
			ArtifactID:     art.ID,
			ChatID:         runtimeSessionID,
			RunID:          runID,
			Type:           string(art.Type),
			Subtype:        art.Subtype,
			Title:          firstNonEmpty(art.DisplayTitle, art.CanonicalTitle),
			LifecycleState: string(art.LifecycleState),
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish artifact.created", "error", err)
	}
}

// emitArtifactUpdated publishes an artifact.updated event through the bus.
func (l *AgentLoop) emitArtifactUpdated(ctx context.Context, runtimeSessionID, runID string, artID string, version int, state schema.ArtifactLifecycleState) {
	if l.cfg.Bus == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactArtifactUpdated,
		schema.EventKindFact,
		runtimeSessionID,
		schema.AgentNavi,
		runID,
		schema.VisibilityUser,
		schema.ArtifactUpdatedPayload{
			ArtifactID:     artID,
			ChatID:         runtimeSessionID,
			RunID:          runID,
			Version:        version,
			LifecycleState: string(state),
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish artifact.updated", "error", err)
	}
}

// emitArtifactMaterialized publishes an artifact.materialized event through the bus.
func (l *AgentLoop) emitArtifactMaterialized(ctx context.Context, chatID, runID, artID, toolName string) {
	if l.cfg.Bus == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactArtifactMaterialized,
		schema.EventKindFact,
		chatID,
		schema.AgentNavi,
		runID,
		schema.VisibilityOperator,
		schema.ArtifactMaterializedPayload{
			ArtifactID: artID,
			ToolName:   toolName,
			RunID:      runID,
		},
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish artifact.materialized", "error", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, s := range values {
		if s != "" {
			return s
		}
	}
	return ""
}
