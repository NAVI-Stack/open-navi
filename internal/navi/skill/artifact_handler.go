package skill

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/open-navi/navi/internal/artifact"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// RegisterArtifactHandler registers internal handlers for the core-artifact skill.
func RegisterArtifactHandler(db *sql.DB, svc *artifact.Service) {
	if svc == nil {
		return
	}

	RegisterInternalHandler("core-artifact", "create", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		title, _ := args["title"].(string)
		artType, _ := args["type"].(string)
		subtype, _ := args["subtype"].(string)
		contentFormat, _ := args["content_format"].(string)
		payload := args["content"]
		reason, _ := args["reason"].(string)
		sourceConversationID, _ := args["source_conversation_id"].(string)
		sourceMessageID, _ := args["source_message_id"].(string)
		projectID, _ := args["project_id"].(string)

		if title == "" || artType == "" {
			return nil, fmt.Errorf("missing required arguments for artifact.create: title, type")
		}

		workspaceID, _ := store.GetWorkspaceID(ctx, db)
		ownerID, _ := store.GetOwnerID(ctx, db)
		if override, _ := args["workspace_id"].(string); override != "" {
			workspaceID = override
		}
		if override, _ := args["owner_id"].(string); override != "" {
			ownerID = override
		}

		op := schema.ArtifactOperationEnvelope{
			Operation:            schema.ArtifactOpCreate,
			WorkspaceID:          workspaceID,
			ProjectID:            projectID,
			OwnerID:              ownerID,
			Title:                title,
			ArtifactType:         schema.ArtifactType(artType),
			ArtifactSubtype:      subtype,
			ContentFormat:        contentFormat,
			Payload:              payload,
			Reason:               reason,
			ActorType:            schema.ActorAgent,
			ActorID:              "navi",
			SourceConversationID: sourceConversationID,
			SourceMessageID:      sourceMessageID,
		}

		a, err := svc.CreateArtifact(ctx, op)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"artifact_id": a.ID,
			"version_id":  a.CurrentVersionID,
			"title":       a.CanonicalTitle,
			"message":     "Artifact created successfully.",
		}, nil
	})

	RegisterInternalHandler("core-artifact", "update", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		id, _ := args["artifact_id"].(string)
		payload := args["content"]
		reason, _ := args["reason"].(string)
		expectedBase, _ := args["expected_base_version_id"].(string)
		sourceConversationID, _ := args["source_conversation_id"].(string)
		sourceMessageID, _ := args["source_message_id"].(string)

		if id == "" {
			return nil, fmt.Errorf("missing required argument for artifact.update: artifact_id")
		}

		workspaceID, _ := store.GetWorkspaceID(ctx, db)
		ownerID, _ := store.GetOwnerID(ctx, db)

		op := schema.ArtifactOperationEnvelope{
			Operation:             schema.ArtifactOpReplace, // Default to replace for now
			TargetArtifactID:      id,
			ExpectedBaseVersionID: expectedBase,
			WorkspaceID:           workspaceID,
			OwnerID:               ownerID,
			Payload:               payload,
			Reason:                reason,
			ActorType:             schema.ActorAgent,
			ActorID:               "navi",
			SourceConversationID:  sourceConversationID,
			SourceMessageID:       sourceMessageID,
		}

		v, err := svc.UpdateContent(ctx, op)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"artifact_id":    v.ArtifactID,
			"version_id":     v.ID,
			"version_number": v.VersionNumber,
			"message":        "Artifact updated successfully.",
		}, nil
	})

	RegisterInternalHandler("core-artifact", "list", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		workspaceID, _ := store.GetWorkspaceID(ctx, db)
		limit, _ := args["limit"].(float64)
		if limit == 0 {
			limit = 50
		}

		artifacts, err := store.ListArtifacts(ctx, db, workspaceID, int(limit))
		if err != nil {
			return nil, err
		}

		var list []map[string]any
		for _, a := range artifacts {
			list = append(list, map[string]any{
				"id":              a.ID,
				"title":           a.CanonicalTitle,
				"display_title":   a.DisplayTitle,
				"type":            a.Type,
				"subtype":         a.Subtype,
				"lifecycle_state": a.LifecycleState,
				"current_version": a.CurrentVersionID,
				"version_number":  a.HeadVersionNumber,
				"updated_at":      a.UpdatedAt.Format(time.RFC3339),
			})
		}

		return map[string]any{"artifacts": list}, nil
	})

	RegisterInternalHandler("core-artifact", "search", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		workspaceID, _ := store.GetWorkspaceID(ctx, db)
		query, _ := args["query"].(string)

		if query == "" {
			return nil, fmt.Errorf("missing query for artifact.search")
		}

		// Basic search implementation in store
		artifacts, err := store.SearchArtifacts(ctx, db, workspaceID, query, 20)
		if err != nil {
			return nil, err
		}

		var list []map[string]any
		for _, a := range artifacts {
			list = append(list, map[string]any{
				"id":              a.ID,
				"title":           a.CanonicalTitle,
				"display_title":   a.DisplayTitle,
				"type":            a.Type,
				"subtype":         a.Subtype,
				"lifecycle_state": a.LifecycleState,
				"version_id":      a.CurrentVersionID,
				"updated_at":      a.UpdatedAt.Format(time.RFC3339),
			})
		}

		return map[string]any{"artifacts": list}, nil
	})

	RegisterInternalHandler("core-artifact", "update_lifecycle", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		id, _ := args["artifact_id"].(string)
		state, _ := args["state"].(string)

		if id == "" || state == "" {
			return nil, fmt.Errorf("missing required arguments for artifact.update_lifecycle: artifact_id, state")
		}

		a, err := store.GetArtifact(ctx, db, id)
		if err != nil {
			return nil, err
		}
		if a == nil {
			return nil, fmt.Errorf("artifact %s not found", id)
		}

		a.LifecycleState = schema.ArtifactLifecycleState(state)
		if err := store.SaveArtifact(ctx, db, *a); err != nil {
			return nil, err
		}

		return map[string]any{
			"artifact_id":     a.ID,
			"lifecycle_state": a.LifecycleState,
			"message":         "Artifact lifecycle state updated.",
		}, nil
	})
}
