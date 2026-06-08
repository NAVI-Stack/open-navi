package navi

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
	"github.com/open-navi/navi/internal/worldmodel"
)

type artifactReference struct {
	ID      string
	Status  string
	Version int
}

func (l *AgentLoop) maybeMaterializeArtifactOutputs(ctx context.Context, chatID string, run *naviruntime.RunState, tc llm.ToolCall, registeredTool *navitool.Tool, result any) ([]artifactReference, error) {
	if l.cfg.WorldModel == nil || registeredTool == nil {
		return nil, nil
	}
	ownerID := ""
	if l.cfg.ResolveOwnerID != nil {
		ownerID = l.cfg.ResolveOwnerID(ctx, chatID)
	}
	if strings.TrimSpace(ownerID) == "" {
		return nil, nil
	}

	requests, err := artifactRequestsForResult(ownerID, tc, registeredTool, result)
	if err != nil || len(requests) == 0 {
		return nil, err
	}

	refs := make([]artifactReference, 0, len(requests))
	for _, req := range requests {
		commandType := schema.CommandTypeCreate
		if req.Status == worldmodel.ArtifactStatusInProgress || req.Status == worldmodel.ArtifactStatusReviewReady || req.Status == worldmodel.ArtifactStatusApproved {
			if strings.TrimSpace(req.ArtifactID) != "" {
				commandType = schema.CommandTypeUpdate
			}
		}

		commandID := uuid.New().String()
		req.SourceExecutionID = commandID + ":1"
		if run != nil {
			req.SourceRunID = run.RunID
		}

		var materialized *worldmodel.MaterializedArtifact
		exec := command.NewExecutor(l.cfg.SaveExecutionOutcome)
		_, execErr := exec.Execute(ctx, command.Descriptor{
			Type:             commandType,
			CommandID:        commandID,
			RuntimeSessionID: chatID,
			RunID:            req.SourceRunID,
			CorrelationID:    chatID,
			ParentRunID:      req.SourceRunID,
			SkillIDs:         artifactSkillIDs(registeredTool),
			LLMProvider:      llmProviderFromRun(run),
			LLMModel:         llmModelFromRun(run),
			LLMTaskClass:     llmTaskClassFromRun(run),
			LLMComplexity:    llmComplexityFromRun(run),
		}, func(execCtx context.Context) (any, error) {
			var err error
			materialized, err = l.cfg.WorldModel.MaterializeArtifact(execCtx, req)
			return materialized, err
		})
		if execErr != nil {
			return refs, execErr
		}
		if materialized != nil {
			refs = append(refs, artifactReference{
				ID:      materialized.Artifact.ID,
				Status:  materialized.Artifact.Status,
				Version: materialized.Artifact.Version,
			})
		}
	}
	return refs, nil
}

func artifactRequestsForResult(ownerID string, tc llm.ToolCall, registeredTool *navitool.Tool, result any) ([]worldmodel.ArtifactMaterializationRequest, error) {
	if registeredTool.Source == navitool.ToolSourceFileTools && tc.Name == filetools.WriteFileToolName {
		path, _ := tc.Arguments["path"].(string)
		content, _ := tc.Arguments["content"].(string)
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			return nil, fmt.Errorf("artifact materialization: write-file path missing")
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		return []worldmodel.ArtifactMaterializationRequest{{
			ArtifactID:  fmt.Sprintf("file:%s:%s", ownerID, path),
			OwnerID:     ownerID,
			Kind:        "file",
			Subtype:     ext,
			Location:    path,
			Description: "Written by WriteFileTool",
			Status:      worldmodel.ArtifactStatusReviewReady,
			SourceTool:  tc.Name,
			Snapshot:    content,
			Metadata: map[string]any{
				"tool_result": result,
			},
		}}, nil
	}
	if registeredTool.Source != navitool.ToolSourceSkill {
		return nil, nil
	}
	payload, ok := result.(map[string]any)
	if !ok {
		return nil, nil
	}
	if single, ok := payload["artifact"]; ok {
		req, err := parseArtifactRequest(ownerID, tc.Name, single, payload)
		if err != nil {
			return nil, err
		}
		return []worldmodel.ArtifactMaterializationRequest{req}, nil
	}
	if list, ok := payload["artifacts"].([]any); ok {
		out := make([]worldmodel.ArtifactMaterializationRequest, 0, len(list))
		for _, item := range list {
			req, err := parseArtifactRequest(ownerID, tc.Name, item, payload)
			if err != nil {
				return nil, err
			}
			out = append(out, req)
		}
		return out, nil
	}
	return nil, nil
}

func parseArtifactRequest(ownerID, toolName string, raw any, fullPayload map[string]any) (worldmodel.ArtifactMaterializationRequest, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return worldmodel.ArtifactMaterializationRequest{
			OwnerID:           ownerID,
			Kind:              "document",
			Status:            worldmodel.ArtifactStatusFailed,
			SourceTool:        toolName,
			RecoverableOutput: fullPayload,
		}, fmt.Errorf("artifact materialization: artifact payload must be an object")
	}
	req := worldmodel.ArtifactMaterializationRequest{
		ArtifactID:        stringMapValue(m, "artifact_id"),
		OwnerID:           ownerID,
		Kind:              stringMapValue(m, "kind"),
		Subtype:           stringMapValue(m, "subtype"),
		Location:          stringMapValue(m, "location"),
		Description:       stringMapValue(m, "description"),
		Status:            firstNonEmptyMapValue(m, "status", worldmodel.ArtifactStatusReviewReady),
		SourceTool:        toolName,
		Snapshot:          m["snapshot"],
		RecoverableOutput: firstNonNil(m["recoverable_output"], fullPayload),
		Metadata:          mapValueMap(m, "metadata"),
	}
	if req.Description == "" {
		req.Description = "Materialized from skill output"
	}
	if req.Kind == "" {
		req.Kind = "document"
	}
	return req, nil
}

func appendArtifactReferences(result string, refs []artifactReference) string {
	if len(refs) == 0 {
		return result
	}
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		lines = append(lines, fmt.Sprintf("Artifact tracked: %s (status=%s version=%d)", ref.ID, ref.Status, ref.Version))
	}
	if strings.TrimSpace(result) == "" {
		return strings.Join(lines, "\n")
	}
	return result + "\n" + strings.Join(lines, "\n")
}

func artifactSkillIDs(registeredTool *navitool.Tool) []string {
	if registeredTool == nil || registeredTool.Metadata.SkillName == "" {
		return nil
	}
	return []string{registeredTool.Metadata.SkillName}
}

func llmProviderFromRun(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return run.LLMProvider
}

func llmModelFromRun(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return run.LLMModel
}

func llmTaskClassFromRun(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return run.LLMTaskClass
}

func llmComplexityFromRun(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return run.LLMComplexity
}

func stringMapValue(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func firstNonEmptyMapValue(m map[string]any, key, fallback string) string {
	if v := stringMapValue(m, key); v != "" {
		return v
	}
	return fallback
}

func mapValueMap(m map[string]any, key string) map[string]any {
	if nested, ok := m[key].(map[string]any); ok {
		return nested
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
