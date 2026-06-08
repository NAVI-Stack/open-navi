package gateway

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func decorateArtifactMap(out map[string]any, a *schema.Artifact) map[string]any {
	if out == nil || a == nil {
		return out
	}
	out["attributes"] = a.Attributes
	out["last_updated"] = a.UpdatedAt.UTC().Format(time.RFC3339)
	out["shared_exported"] = artifactIsSharedOrExported(*a)
	out["sync_state"] = artifactSyncState(*a)
	out["pending_execution_state"] = artifactPendingExecutionState(*a)
	return out
}

func artifactListItemMap(ctx context.Context, db *sql.DB, a schema.Artifact) map[string]any {
	out := decorateArtifactMap(artifactToMap(&a), &a)
	out["last_editor"] = map[string]any{
		"actor_id":   firstNonEmptyGateway(a.CreatedByActorID, string(a.CreatedByActorType)),
		"actor_type": string(a.CreatedByActorType),
	}
	if strings.TrimSpace(a.CurrentVersionID) == "" {
		return out
	}
	version, err := store.GetArtifactVersion(ctx, db, a.CurrentVersionID)
	if err != nil {
		return out
	}
	out["last_editor"] = map[string]any{
		"actor_id":   firstNonEmptyGateway(version.AuthorActorID, string(version.AuthorActorType)),
		"actor_type": string(version.AuthorActorType),
	}
	out["validation_state"] = version.ValidationState
	out["commit_state"] = string(version.CommitState)
	return out
}

func artifactIsSharedOrExported(a schema.Artifact) bool {
	if a.LifecycleState == schema.ArtifactLifecyclePublished {
		return true
	}
	if raw := strings.ToLower(artifactAttrString(a.Attributes, "sync_state", "share_state")); raw == "shared" || raw == "exported" || raw == "published" {
		return true
	}
	return artifactAttrTruthy(a.Attributes, "shared", "exported") ||
		artifactAttrString(a.Attributes, "shared_at", "exported_at", "share_url", "export_url") != ""
}

func artifactSyncState(a schema.Artifact) string {
	if raw := artifactAttrString(a.Attributes, "sync_state", "share_state"); raw != "" {
		return raw
	}
	if artifactIsSharedOrExported(a) {
		return "exported"
	}
	if a.LifecycleState == schema.ArtifactLifecycleArchived {
		return "archived"
	}
	return "local"
}

func artifactPendingExecutionState(a schema.Artifact) string {
	if raw := artifactAttrString(a.Attributes, "pending_execution_state", "execution_state"); raw != "" {
		return raw
	}
	if a.LifecycleState == schema.ArtifactLifecycleErrored {
		return "failed"
	}
	if strings.TrimSpace(a.SourceRunID) != "" || a.LifecycleState == schema.ArtifactLifecycleInProgress {
		return "running"
	}
	return ""
}

func artifactAttrString(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := attrs[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case []byte:
			if strings.TrimSpace(string(typed)) != "" {
				return string(typed)
			}
		}
	}
	return ""
}

func artifactAttrTruthy(attrs map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := attrs[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "1", "true", "yes", "shared", "exported":
				return true
			}
		}
	}
	return false
}

func inferLiveArtifactContext(ctx context.Context, db *sql.DB, chatID string, list []schema.Artifact) (string, string, error) {
	var workspaceID string
	var projectID string
	ids, err := store.ListArtifactIDsBySource(ctx, db, "conversation", chatID, 200)
	if err != nil {
		return "", "", err
	}
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	for _, item := range list {
		if _, ok := allowed[item.ID]; !ok {
			continue
		}
		if workspaceID == "" && strings.TrimSpace(item.WorkspaceID) != "" {
			workspaceID = item.WorkspaceID
		}
		if projectID == "" && strings.TrimSpace(item.ProjectID) != "" {
			projectID = item.ProjectID
		}
		if workspaceID != "" && projectID != "" {
			return workspaceID, projectID, nil
		}
	}
	for _, item := range list {
		if workspaceID == "" && strings.TrimSpace(item.WorkspaceID) != "" {
			workspaceID = item.WorkspaceID
		}
		if projectID == "" && strings.TrimSpace(item.ProjectID) != "" {
			projectID = item.ProjectID
		}
		if workspaceID != "" && projectID != "" {
			return workspaceID, projectID, nil
		}
	}
	return workspaceID, projectID, nil
}
