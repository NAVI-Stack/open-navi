package navi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/artifact"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	navitool "github.com/open-navi/navi/internal/tool"
)

// ArtifactMaterializer implements the Cognitive Layer decision logic for
// promote skill outputs to durable artifacts.
type ArtifactMaterializer struct {
	svc *artifact.Service
	db  interface {
		GetWorkspaceID(ctx context.Context) (string, error)
		GetOwnerID(ctx context.Context, chatID string) (string, error)
	}
}

type materializerDBShim struct {
	loop *AgentLoop
}

func (s *materializerDBShim) GetWorkspaceID(ctx context.Context) (string, error) {
	// workspaceID is typically chat-scoped or system-scoped.
	// For now, we'll try to get it from the store if possible.
	// Since LoopConfig has the DB, we can use store.GetWorkspaceID.
	return store.GetWorkspaceID(ctx, s.loop.cfg.NAVI.cfg.DB)
}

func (s *materializerDBShim) GetOwnerID(ctx context.Context, chatID string) (string, error) {
	if s.loop.cfg.ResolveOwnerID != nil {
		return s.loop.cfg.ResolveOwnerID(ctx, chatID), nil
	}
	return store.GetOwnerID(ctx, s.loop.cfg.NAVI.cfg.DB)
}

// MaterializeOutcome evaluates a tool execution result and decides whether it
// should be persisted as a durable artifact.
func (l *AgentLoop) MaterializeOutcome(ctx context.Context, chatID, runID string, tool navitool.Tool, args map[string]any, result string) (string, error) {
	if l.cfg.ArtifactService == nil {
		return "", nil
	}

	// 1. Initial judgment: Is this already an artifact operation?
	// If the tool is core-artifact itself, we don't materialize it again.
	if tool.Metadata.SkillName == "core-artifact" {
		return "", nil
	}

	// 2. Materialization Heuristic
	judgment := l.judgeMaterialization(tool, result)
	if !judgment.ShouldMaterialize {
		return "", nil
	}

	// 3. Execution - Convert to Artifact
	ownerID := ""
	if l.cfg.ResolveOwnerID != nil {
		ownerID = l.cfg.ResolveOwnerID(ctx, chatID)
	} else {
		ownerID, _ = store.GetOwnerID(ctx, l.cfg.NAVI.cfg.DB)
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, l.cfg.NAVI.cfg.DB)

	title := judgment.PredictedTitle
	if title == "" {
		title = fmt.Sprintf("%s Output (%s)", tool.Name, time.Now().Format("2006-01-02 15:04"))
	}

	op := schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		WorkspaceID:          workspaceID,
		OwnerID:              ownerID,
		Title:                title,
		ArtifactType:         judgment.PredictedType,
		ArtifactSubtype:      judgment.PredictedSubtype,
		Payload:              result,
		Reason:               fmt.Sprintf("Automatic materialization from tool %s execution", tool.Name),
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: chatID,
		SourceMessageID:      runID,
	}

	a, err := l.cfg.ArtifactService.CreateArtifact(ctx, op)
	if err != nil {
		return "", fmt.Errorf("materialize artifact: %w", err)
	}

	// 4. Emit Events & Trigger Reflection
	l.emitArtifactCreated(ctx, chatID, runID, &a)
	l.emitArtifactMaterialized(ctx, chatID, runID, a.ID, tool.Name)

	// Trigger shallow reflection (OMN-120)
	details := fmt.Sprintf("Materialized artifact '%s' (%s) from tool %s", a.CanonicalTitle, a.ID, tool.Name)
	l.emitReflect(ctx, chatID, "Artifact materialized", details, result)

	return a.ID, nil
}

type materializationJudgment struct {
	ShouldMaterialize bool
	PredictedType     schema.ArtifactType
	PredictedSubtype  string
	PredictedTitle    string
}

func (l *AgentLoop) judgeMaterialization(tool navitool.Tool, result string) materializationJudgment {
	result = strings.TrimSpace(result)
	if result == "" {
		return materializationJudgment{ShouldMaterialize: false}
	}

	// Rule 1: Structural indicators
	if strings.Contains(result, "| --- |") || strings.Contains(result, "|---|") {
		// Found a markdown table
		return materializationJudgment{
			ShouldMaterialize: true,
			PredictedType:     schema.ArtifactTypeDocument,
			PredictedSubtype:  "table",
			PredictedTitle:    "Extracted Table",
		}
	}

	if strings.Contains(result, "graph TD") || strings.Contains(result, "sequenceDiagram") || strings.Contains(result, "mermaid") {
		// Found a diagram
		return materializationJudgment{
			ShouldMaterialize: true,
			PredictedType:     schema.ArtifactTypeDocument,
			PredictedSubtype:  "diagram",
			PredictedTitle:    "Visual Diagram",
		}
	}

	if strings.HasPrefix(result, "```") && strings.HasSuffix(result, "```") {
		// Extracted a code block
		return materializationJudgment{
			ShouldMaterialize: true,
			PredictedType:     schema.ArtifactTypeDocument, // Code is usually in a document container in V1
			PredictedSubtype:  "code",
			PredictedTitle:    "Code Snippet",
		}
	}

	// Rule 2: Substantial content
	if len(result) > 2048 {
		subtype := "text"
		if l.cfg.ArtifactService != nil {
			// Check if registry has a generic 'markdown' or similar
			if d := l.cfg.ArtifactService.Registry().GetRenderer("markdown"); d.ComponentID != "RawRenderer" {
				subtype = "markdown"
			}
		}
		return materializationJudgment{
			ShouldMaterialize: true,
			PredictedType:     schema.ArtifactTypeDocument,
			PredictedSubtype:  subtype,
			PredictedTitle:    "Large Content",
		}
	}

	// Rule 3: Known artifact tools
	switch tool.Name {
	case "scout-search_summarize":
		return materializationJudgment{
			ShouldMaterialize: true,
			PredictedType:     schema.ArtifactTypeDocument,
			PredictedSubtype:  "summary",
		}
	}

	return materializationJudgment{ShouldMaterialize: false}
}

// InitializeWorkflowArtifact creates a draft artifact to track a long-running workflow (OMN-118).
func (l *AgentLoop) InitializeWorkflowArtifact(ctx context.Context, chatID, runID, userGoal string) (string, error) {
	if l.cfg.ArtifactService == nil {
		return "", nil
	}

	ownerID := ""
	if l.cfg.ResolveOwnerID != nil {
		ownerID = l.cfg.ResolveOwnerID(ctx, chatID)
	} else {
		ownerID, _ = store.GetOwnerID(ctx, l.cfg.NAVI.cfg.DB)
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, l.cfg.NAVI.cfg.DB)

	// Heuristic for title
	title := "Workflow Output"
	if userGoal != "" {
		if len(userGoal) > 40 {
			title = userGoal[:37] + "..."
		} else {
			title = userGoal
		}
	}

	op := schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		WorkspaceID:          workspaceID,
		OwnerID:              ownerID,
		Title:                title,
		ArtifactType:         schema.ArtifactTypeDocument,
		ArtifactSubtype:      "workflow_draft",
		Payload:              "Workflow initialized. Generating content...",
		Reason:               "Proactive draft creation for long-running workflow.",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: chatID,
		SourceMessageID:      runID,
	}

	a, err := l.cfg.ArtifactService.CreateArtifact(ctx, op)
	if err != nil {
		return "", fmt.Errorf("initialize workflow artifact: %w", err)
	}

	// Transition to in_progress for workflows (OMN-118)
	opInProgress := schema.ArtifactOperationEnvelope{
		Operation:        schema.ArtifactOpTransitionLifecycle,
		TargetArtifactID: a.ID,
		ToLifecycleState: schema.ArtifactLifecycleInProgress,
		ActorType:        schema.ActorAgent,
		ActorID:          "navi",
	}
	a, err = l.cfg.ArtifactService.TransitionLifecycle(ctx, opInProgress)
	if err != nil {
		return a.ID, fmt.Errorf("transition artifact state: %w", err)
	}

	// Emit event & reflect (OMN-120)
	l.emitArtifactCreated(ctx, chatID, runID, &a)
	l.emitReflect(ctx, chatID, "Workflow artifact initialized", fmt.Sprintf("Started tracking workflow with artifact %s", a.ID), userGoal)

	return a.ID, nil
}

// FinalizeWorkflowArtifact transitions the main artifact to review_ready and updates its content (OMN-118).
func (l *AgentLoop) FinalizeWorkflowArtifact(ctx context.Context, artifactID, finalContent string) error {
	if l.cfg.ArtifactService == nil || artifactID == "" {
		return nil
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, l.cfg.NAVI.cfg.DB)
	ownerID, _ := store.GetOwnerID(ctx, l.cfg.NAVI.cfg.DB)

	// 1. Update content with the final response
	if finalContent != "" {
		op := schema.ArtifactOperationEnvelope{
			Operation:        schema.ArtifactOpReplace,
			TargetArtifactID: artifactID,
			WorkspaceID:      workspaceID,
			OwnerID:          ownerID,
			Payload:          finalContent,
			Reason:           "Final workflow output",
			ActorType:        schema.ActorAgent,
			ActorID:          "navi",
		}

		_, err := l.cfg.ArtifactService.UpdateContent(ctx, op)
		if err != nil {
			slog.Debug("navi: failed to update final artifact content", "artifact_id", artifactID, "error", err)
		}
	}

	// 2. Transition to review_ready
	opFinal := schema.ArtifactOperationEnvelope{
		Operation:        schema.ArtifactOpTransitionLifecycle,
		TargetArtifactID: artifactID,
		ToLifecycleState: schema.ArtifactLifecycleReviewReady,
		ActorType:        schema.ActorAgent,
		ActorID:          "navi",
	}
	a, err := l.cfg.ArtifactService.TransitionLifecycle(ctx, opFinal)
	if err != nil {
		return err
	}

	slog.Info("navi: finalized workflow artifact", "artifact_id", artifactID, "state", a.LifecycleState)

	// Emit event & reflect (OMN-120)
	l.emitArtifactUpdated(ctx, "", "", a.ID, a.HeadVersionNumber, a.LifecycleState)
	l.emitReflect(ctx, "", "Workflow artifact finalized", fmt.Sprintf("Completed workflow for artifact %s, transitioned to %s", a.ID, a.LifecycleState), "")

	return nil
}

// CheckpointWorkflowArtifact commits the current progress as a new version (OMN-118).
func (l *AgentLoop) CheckpointWorkflowArtifact(ctx context.Context, artifactID, content string) error {
	if l.cfg.ArtifactService == nil || artifactID == "" || content == "" {
		return nil
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, l.cfg.NAVI.cfg.DB)
	ownerID, _ := store.GetOwnerID(ctx, l.cfg.NAVI.cfg.DB)

	op := schema.ArtifactOperationEnvelope{
		Operation:        schema.ArtifactOpReplace,
		TargetArtifactID: artifactID,
		WorkspaceID:      workspaceID,
		OwnerID:          ownerID,
		Payload:          content,
		Reason:           "Workflow milestone checkpoint",
		ActorType:        schema.ActorAgent,
		ActorID:          "navi",
	}

	art, err := l.cfg.ArtifactService.UpdateContent(ctx, op)
	if err == nil {
		aHead, gerr := store.GetArtifact(ctx, l.cfg.NAVI.cfg.DB, art.ArtifactID)
		if gerr != nil {
			slog.Debug("navi: checkpoint could not reload artifact head", "artifact_id", art.ArtifactID, "error", gerr)
		}
		if aHead != nil {
			l.emitArtifactUpdated(ctx, "", "", art.ArtifactID, art.VersionNumber, aHead.LifecycleState)
		}
	}
	return err
}

// MaterializeFailure handles tool execution failures by creating a diagnostic artifact (OMN-118).
func (l *AgentLoop) MaterializeFailure(ctx context.Context, chatID, runID string, tool navitool.Tool, args map[string]any, errStr string) (string, error) {
	if l.cfg.ArtifactService == nil {
		return "", nil
	}

	workspaceID, _ := store.GetWorkspaceID(ctx, l.cfg.NAVI.cfg.DB)
	ownerID, _ := store.GetOwnerID(ctx, l.cfg.NAVI.cfg.DB)

	title := fmt.Sprintf("Failure: %s", tool.Name)

	op := schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		WorkspaceID:          workspaceID,
		OwnerID:              ownerID,
		Title:                title,
		ArtifactType:         schema.ArtifactTypeData,
		ArtifactSubtype:      "error_diagnostic",
		Payload:              fmt.Sprintf("Tool: %s\nArguments: %v\nError: %s", tool.Name, args, errStr),
		Reason:               "Preserving context from failed tool execution",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: chatID,
		SourceMessageID:      runID,
	}

	a, err := l.cfg.ArtifactService.CreateArtifact(ctx, op)
	if err != nil {
		return "", err
	}

	// Mark as errored
	opErr := schema.ArtifactOperationEnvelope{
		Operation:        schema.ArtifactOpTransitionLifecycle,
		TargetArtifactID: a.ID,
		ToLifecycleState: schema.ArtifactLifecycleErrored,
		ActorType:        schema.ActorAgent,
		ActorID:          "navi",
	}
	a, _ = l.cfg.ArtifactService.TransitionLifecycle(ctx, opErr)

	// Emit events & reflect (OMN-120)
	l.emitArtifactCreated(ctx, chatID, runID, &a)
	l.emitArtifactMaterialized(ctx, chatID, runID, a.ID, tool.Name)
	l.emitReflect(ctx, chatID, "Tool failure materialized", fmt.Sprintf("Captured diagnostic artifact %s for failed tool %s", a.ID, tool.Name), errStr)

	return a.ID, nil
}
