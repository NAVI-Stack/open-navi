package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	artifactsvc "github.com/open-navi/navi/internal/artifact"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type artifactExportRequest struct {
	VersionID        string `json:"version_id,omitempty"`
	Format           string `json:"format"`
	TargetKind       string `json:"target_kind"`
	TargetURI        string `json:"target_uri,omitempty"`
	ConfirmationMode string `json:"confirmation_mode,omitempty"`
}

type artifactShareRequest struct {
	VersionID        string `json:"version_id,omitempty"`
	Scope            string `json:"scope"`
	AccessLevel      string `json:"access_level"`
	ConfirmationMode string `json:"confirmation_mode,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
}

type artifactBranchRequest struct {
	BaseVersionID string `json:"base_version_id,omitempty"`
	Name          string `json:"name,omitempty"`
}

type artifactSaveRequest struct {
	Content               string `json:"content"`
	ExpectedBaseVersionID string `json:"expected_base_version_id,omitempty"`
	ChangeSummary         string `json:"change_summary,omitempty"`
}

type artifactRestoreVersionRequest struct {
	ExpectedBaseVersionID string `json:"expected_base_version_id,omitempty"`
	ChangeSummary         string `json:"change_summary,omitempty"`
}

func artifactExportToMap(item schema.ArtifactExport) map[string]any {
	return map[string]any{
		"export_id":             item.ExportID,
		"artifact_id":           item.ArtifactID,
		"version_id":            item.VersionID,
		"format":                string(item.Format),
		"target_kind":           string(item.TargetKind),
		"target_uri":            item.TargetURI,
		"content_ref":           contentRefToMap(item.ContentRef),
		"content_type":          item.ContentType,
		"status":                string(item.Status),
		"failure_class":         string(item.FailureClass),
		"failure_reason":        item.FailureReason,
		"created_by_actor_type": string(item.CreatedByActorType),
		"created_by_actor_id":   item.CreatedByActorID,
		"created_at":            item.CreatedAt.UTC().Format(time.RFC3339),
		"completed_at":          formatOptionalTime(item.CompletedAt),
	}
}

func artifactShareToMap(item schema.ArtifactShare) map[string]any {
	return map[string]any{
		"share_id":              item.ShareID,
		"artifact_id":           item.ArtifactID,
		"version_id":            item.VersionID,
		"scope":                 string(item.Scope),
		"access_level":          string(item.AccessLevel),
		"status":                string(item.Status),
		"share_url":             item.ShareURL,
		"created_by_actor_type": string(item.CreatedByActorType),
		"created_by_actor_id":   item.CreatedByActorID,
		"created_at":            item.CreatedAt.UTC().Format(time.RFC3339),
		"expires_at":            formatOptionalTime(item.ExpiresAt),
		"revoked_at":            formatOptionalTime(item.RevokedAt),
	}
}

func artifactBranchToMap(item schema.ArtifactBranch) map[string]any {
	return map[string]any{
		"branch_id":             item.ID,
		"artifact_id":           item.ArtifactID,
		"name":                  item.Name,
		"base_version_id":       item.BaseVersionID,
		"head_version_id":       item.HeadVersionID,
		"status":                item.Status,
		"created_by_actor_type": string(item.CreatedByActorType),
		"created_by_actor_id":   item.CreatedByActorID,
		"created_at":            item.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":            item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func attachArtifactRecords(ctx context.Context, db *sql.DB, artifactID string, out map[string]any) {
	if db == nil || strings.TrimSpace(artifactID) == "" || out == nil {
		return
	}
	if exports, err := store.ListArtifactExports(ctx, db, artifactID, 20); err == nil {
		items := make([]map[string]any, 0, len(exports))
		for _, item := range exports {
			items = append(items, artifactExportToMap(item))
		}
		out["exports"] = items
	}
	if shares, err := store.ListArtifactShares(ctx, db, artifactID, 20); err == nil {
		items := make([]map[string]any, 0, len(shares))
		for _, item := range shares {
			items = append(items, artifactShareToMap(item))
		}
		out["shares"] = items
	}
	if branches, err := store.ListArtifactBranches(ctx, db, artifactID, 20); err == nil {
		items := make([]map[string]any, 0, len(branches))
		for _, item := range branches {
			items = append(items, artifactBranchToMap(item))
		}
		out["branches"] = items
	}
	if failures, err := store.ListExecutionOutcomesByArtifactID(ctx, db, artifactID, 20); err == nil {
		items := make([]map[string]any, 0, len(failures))
		for _, item := range failures {
			items = append(items, runOutcomeToMap(&item))
		}
		out["recent_runs"] = items
	}
}

func (s *Server) handleCreateArtifactBranch(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact branching not configured", nil)
		return
	}
	var req artifactBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	item, err := s.cfg.ArtifactService.BranchArtifact(r.Context(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpBranch,
		TargetArtifactID:      strings.TrimSpace(r.PathValue("id")),
		ExpectedBaseVersionID: strings.TrimSpace(req.BaseVersionID),
		Reason:                strings.TrimSpace(req.Name),
		ActorType:             schema.ActorUser,
		ActorID:               OwnerIDFromCtx(r.Context()),
	})
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "BRANCH_FAILED", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, artifactBranchToMap(item))
}

func (s *Server) handleSaveArtifactContent(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil || s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact saving not configured", nil)
		return
	}
	artifactID := strings.TrimSpace(r.PathValue("id"))
	if artifactID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id required", nil)
		return
	}
	artifact, err := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if artifact == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found", nil)
		return
	}

	var req artifactSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	baseVersionID := firstNonEmptyGateway(strings.TrimSpace(req.ExpectedBaseVersionID), artifact.CurrentVersionID)
	version, err := s.cfg.ArtifactService.UpdateContent(r.Context(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpReplace,
		OperationID:           uuid.New().String(),
		TargetArtifactID:      artifactID,
		ExpectedBaseVersionID: baseVersionID,
		Payload:               req.Content,
		Reason:                firstNonEmptyGateway(strings.TrimSpace(req.ChangeSummary), "Saved from artifact workspace"),
		ActorType:             schema.ActorUser,
		ActorID:               OwnerIDFromCtx(r.Context()),
	})
	if err != nil {
		if strings.Contains(err.Error(), "version_conflict") {
			latest, latestErr := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
			details := map[string]any{
				"expected_base_version_id": baseVersionID,
			}
			if latestErr == nil && latest != nil {
				details["current_head_version_id"] = latest.CurrentVersionID
				details["artifact"] = artifactToMap(latest)
			}
			replyErrorAPI(w, http.StatusConflict, "VERSION_CONFLICT", err.Error(), details)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error(), nil)
		return
	}

	updated, err := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated == nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "artifact vanished after save", nil)
		return
	}
	resp := artifactToMap(updated)
	versions, err := s.cfg.WorldModel.ListArtifactVersions(r.Context(), artifactID, 20)
	if err == nil {
		versionMaps := make([]map[string]any, 0, len(versions))
		for _, item := range versions {
			versionMaps = append(versionMaps, artifactVersionToMap(item))
		}
		resp["versions"] = versionMaps
	}
	attachArtifactRecords(r.Context(), s.cfg.DB, artifactID, resp)
	resp["saved_version_id"] = version.ID
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRestoreArtifactVersion(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil || s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact restore not configured", nil)
		return
	}
	artifactID := strings.TrimSpace(r.PathValue("id"))
	versionID := strings.TrimSpace(r.PathValue("versionID"))
	if artifactID == "" || versionID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id and version id required", nil)
		return
	}
	artifact, err := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if artifact == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found", nil)
		return
	}

	var req artifactRestoreVersionRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
			return
		}
	}
	baseVersionID := firstNonEmptyGateway(strings.TrimSpace(req.ExpectedBaseVersionID), artifact.CurrentVersionID)
	version, err := s.cfg.ArtifactService.RestoreVersion(r.Context(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpRestoreVersion,
		OperationID:           uuid.New().String(),
		TargetArtifactID:      artifactID,
		TargetVersionID:       versionID,
		ExpectedBaseVersionID: baseVersionID,
		Reason:                firstNonEmptyGateway(strings.TrimSpace(req.ChangeSummary), "Restored from version history"),
		ActorType:             schema.ActorUser,
		ActorID:               OwnerIDFromCtx(r.Context()),
	})
	if err != nil {
		if strings.Contains(err.Error(), "version_conflict") {
			latest, latestErr := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
			details := map[string]any{
				"expected_base_version_id": baseVersionID,
				"restore_version_id":       versionID,
			}
			if latestErr == nil && latest != nil {
				details["current_head_version_id"] = latest.CurrentVersionID
				details["artifact"] = artifactToMap(latest)
			}
			replyErrorAPI(w, http.StatusConflict, "VERSION_CONFLICT", err.Error(), details)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "RESTORE_FAILED", err.Error(), nil)
		return
	}

	updated, err := s.cfg.WorldModel.GetArtifact(r.Context(), artifactID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated == nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "artifact vanished after restore", nil)
		return
	}
	resp := artifactToMap(updated)
	versions, err := s.cfg.WorldModel.ListArtifactVersions(r.Context(), artifactID, 20)
	if err == nil {
		versionMaps := make([]map[string]any, 0, len(versions))
		for _, item := range versions {
			versionMaps = append(versionMaps, artifactVersionToMap(item))
		}
		resp["versions"] = versionMaps
	}
	attachArtifactRecords(r.Context(), s.cfg.DB, artifactID, resp)
	resp["saved_version_id"] = version.ID
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListArtifactExports(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id required", nil)
		return
	}
	items, err := store.ListArtifactExports(r.Context(), s.cfg.DB, id, 50)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, artifactExportToMap(item))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (s *Server) handleCreateArtifactExport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact export not configured", nil)
		return
	}
	var req artifactExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	item, err := s.cfg.ArtifactService.ExportArtifact(r.Context(), artifactsvc.ExportRequest{
		ArtifactID:       strings.TrimSpace(r.PathValue("id")),
		VersionID:        strings.TrimSpace(req.VersionID),
		Format:           schema.ExportFormat(strings.ToLower(strings.TrimSpace(req.Format))),
		TargetKind:       schema.ArtifactExportTargetKind(strings.ToLower(strings.TrimSpace(req.TargetKind))),
		TargetURI:        strings.TrimSpace(req.TargetURI),
		ActorType:        schema.ActorUser,
		ActorID:          OwnerIDFromCtx(r.Context()),
		ConfirmationMode: strings.TrimSpace(req.ConfirmationMode),
	})
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "requires confirmation") {
			code = http.StatusForbidden
		} else if strings.Contains(err.Error(), "unsupported") {
			code = http.StatusBadRequest
		}
		replyErrorAPI(w, code, "EXPORT_FAILED", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, artifactExportToMap(item))
}

func (s *Server) handleDownloadArtifactExport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact export not configured", nil)
		return
	}
	payload, item, err := s.cfg.ArtifactService.ReadExportContent(r.Context(), strings.TrimSpace(r.PathValue("exportID")))
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	contentType := item.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+artifactsvc.ExportFilename("artifact", item.Format)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) handleListArtifactShares(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id required", nil)
		return
	}
	items, err := store.ListArtifactShares(r.Context(), s.cfg.DB, id, 50)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, artifactShareToMap(item))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (s *Server) handleCreateArtifactShare(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact sharing not configured", nil)
		return
	}
	var req artifactShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	var expiresAt *time.Time
	if raw := strings.TrimSpace(req.ExpiresAt); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "expires_at must be RFC3339", nil)
			return
		}
		expiresAt = &parsed
	}
	item, err := s.cfg.ArtifactService.ShareArtifact(r.Context(), artifactsvc.ShareRequest{
		ArtifactID:       strings.TrimSpace(r.PathValue("id")),
		VersionID:        strings.TrimSpace(req.VersionID),
		Scope:            schema.ArtifactShareScope(strings.ToLower(strings.TrimSpace(req.Scope))),
		AccessLevel:      schema.ArtifactShareAccessLevel(strings.ToLower(strings.TrimSpace(req.AccessLevel))),
		ActorType:        schema.ActorUser,
		ActorID:          OwnerIDFromCtx(r.Context()),
		ConfirmationMode: strings.TrimSpace(req.ConfirmationMode),
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "requires confirmation") {
			code = http.StatusForbidden
		}
		replyErrorAPI(w, code, "SHARE_FAILED", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, artifactShareToMap(item))
}
