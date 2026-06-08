package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/presence"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"nhooyr.io/websocket"
)

// defaultLiveOrigins are used when origin_patterns is empty so we never accept all origins.
var defaultLiveOrigins = []string{
	"localhost:6284", "localhost:5173", "localhost:3000",
	"127.0.0.1:6284", "127.0.0.1:5173", "127.0.0.1:3000",
}

type liveRequestFrame struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type liveResponseFrame struct {
	Type   string `json:"type"`
	ID     string `json:"id,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

type liveEventFrame struct {
	Type  string       `json:"type"`
	Event schema.Event `json:"event"`
}

type livePresenceEventFrame struct {
	Type   string    `json:"type"`
	SentAt time.Time `json:"sent_at"`
	Data   any       `json:"data"`
}

type liveConnectParams struct {
	ChatID   string `json:"chat_id"`
	AfterSeq int64  `json:"after_seq"`
	Replay   *bool  `json:"replay,omitempty"`
	Stream   string `json:"stream"`
}

type liveSendMessageParams struct {
	Content          string `json:"content"`
	SourceChannel    string `json:"source_channel,omitempty"`
	SourceMessageRef string `json:"source_message_ref,omitempty"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

type liveResolveProposalParams struct {
	ProposalID        string `json:"proposal_id"`
	Action            string `json:"action,omitempty"`
	Resolution        string `json:"resolution,omitempty"`
	Note              string `json:"note,omitempty"`
	SwitchWorkspaceID string `json:"switch_workspace_id,omitempty"`
}

type liveAckNotificationParams struct {
	NotificationID string `json:"notification_id,omitempty"`
}

type liveGetArtifactsParams struct {
	View        string `json:"view,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type liveArtifactParams struct {
	ArtifactID string `json:"artifact_id"`
	VersionID  string `json:"version_id,omitempty"`
}

type liveTerminalSummary struct {
	Type      schema.EventType `json:"type"`
	Seq       int64            `json:"seq"`
	Timestamp time.Time        `json:"timestamp"`
	Payload   any              `json:"payload"`
}

// handleLive binds /ws/live to one chat-scoped runtime stream per connection.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{}
	if len(s.cfg.OriginPatterns) > 0 {
		opts.OriginPatterns = s.cfg.OriginPatterns
	} else {
		opts.OriginPatterns = defaultLiveOrigins
	}
	c, err := websocket.Accept(w, r, opts)
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "internal error")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	_, raw, err := c.Read(readCtx)
	if err != nil {
		c.Close(websocket.StatusPolicyViolation, "missing connect frame")
		return
	}

	var connectReq liveRequestFrame
	if err := json.Unmarshal(raw, &connectReq); err != nil || connectReq.Type != "req" || connectReq.Method != "connect" {
		c.Close(websocket.StatusPolicyViolation, "invalid connect frame")
		return
	}

	var connect liveConnectParams
	if err := json.Unmarshal(connectReq.Params, &connect); err != nil || connect.ChatID == "" {
		c.Close(websocket.StatusPolicyViolation, "invalid connect params")
		return
	}
	if connect.Stream == "" {
		connect.Stream = "user"
	}
	hidden, err := chatHiddenFromPublicAPI(ctx, s.cfg.DB, connect.ChatID)
	if err != nil {
		c.Close(websocket.StatusInternalError, "failed to resolve chat access")
		return
	}
	if hidden {
		c.Close(websocket.StatusPolicyViolation, "internal chats are not available on live streams")
		return
	}
	allowed := visibilitiesForStream(connect.Stream)

	seq := connect.AfterSeq
	if connect.Replay != nil && !*connect.Replay {
		maxSeq, maxErr := chatMaxSeq(ctx, s.cfg.DB, connect.ChatID, allowed)
		if maxErr != nil {
			c.Close(websocket.StatusInternalError, "failed to resolve stream cursor")
			return
		}
		seq = maxSeq
	}
	if err := writeLiveFrame(ctx, c, liveResponseFrame{
		Type: "res",
		ID:   connectReq.ID,
		OK:   true,
		Result: map[string]any{
			"chat_id":    connect.ChatID,
			"stream":     connect.Stream,
			"after_seq":  connect.AfterSeq,
			"cursor_seq": seq,
		},
	}); err != nil {
		return
	}
	if err := replayLiveEvents(ctx, c, s.cfg.DB, connect.ChatID, &seq, allowed); err != nil {
		return
	}

	reqCh := make(chan liveRequestFrame, 8)
	errCh := make(chan error, 1)
	go func() {
		defer close(reqCh)
		for {
			_, msg, readErr := c.Read(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			var req liveRequestFrame
			if json.Unmarshal(msg, &req) != nil || req.Type != "req" {
				continue
			}
			reqCh <- req
		}
	}()

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	var lastNaviRevision int64
	var lastSnapshotRevision int64
	var lastAttention presence.PresenceAttention
	presenceSvc := s.presenceService()
	if presenceSvc != nil {
		lastNaviRevision = presenceSvc.NaviRevision()
		lastSnapshotRevision = presenceSvc.SnapshotRevision()
		envelope := presenceSvc.NaviPresence(ctx)
		lastAttention = naviAttentionFromEnvelope(envelope)
		snapshot := presenceSvc.Snapshot(ctx)
		now := time.Now().UTC()
		_ = writeLiveFrame(ctx, c, livePresenceEventFrame{
			Type:   "presence.snapshot",
			SentAt: now,
			Data:   snapshot,
		})
		_ = writeLiveFrame(ctx, c, livePresenceEventFrame{
			Type:   "presence.transport.state",
			SentAt: now,
			Data: map[string]any{
				"state":              snapshot.Transport.State,
				"last_ws_message_at": now,
				"stale_after_ms":     snapshot.Transport.StaleAfterMS,
			},
		})
	}

	for {
		select {
		case <-ctx.Done():
			c.Close(websocket.StatusNormalClosure, "")
			return
		case <-ticker.C:
			if err := replayLiveEvents(ctx, c, s.cfg.DB, connect.ChatID, &seq, allowed); err != nil {
				return
			}
			if presenceSvc != nil {
				naviRevision := presenceSvc.NaviRevision()
				snapshotRevision := presenceSvc.SnapshotRevision()

				naviChanged := naviRevision != lastNaviRevision
				snapshotChanged := snapshotRevision != lastSnapshotRevision

				if snapshotChanged {
					// A snapshot change is the most comprehensive update
					snapshot := presenceSvc.Snapshot(ctx)
					_ = writeLiveFrame(ctx, c, livePresenceEventFrame{
						Type:   "presence.snapshot",
						SentAt: time.Now().UTC(),
						Data:   snapshot,
					})
					lastSnapshotRevision = snapshotRevision
					lastNaviRevision = naviRevision

					// Update lastAttention from the new snapshot
					if snapshot.Navi != nil {
						if p, ok := snapshot.Navi.Payload.(presence.NaviPresencePayload); ok {
							lastAttention = p.Attention
						}
					}
				} else if naviChanged {
					// Rare case where only Navi changed but snapshot revision didn't catch it
					navi := presenceSvc.NaviPresence(ctx)
					_ = writeLiveFrame(ctx, c, livePresenceEventFrame{
						Type:   "presence.navi.updated",
						SentAt: time.Now().UTC(),
						Data:   navi,
					})
					lastNaviRevision = naviRevision

					// Also check if attention changed
					if p, ok := navi.Payload.(presence.NaviPresencePayload); ok {
						if !reflect.DeepEqual(p.Attention, lastAttention) {
							_ = writeLiveFrame(ctx, c, livePresenceEventFrame{
								Type:   "presence.navi.attention",
								SentAt: time.Now().UTC(),
								Data:   p.Attention,
							})
							lastAttention = p.Attention
						}
					}
				}
			}
		case readErr := <-errCh:
			if readErr != nil {
				c.Close(websocket.StatusNormalClosure, "")
			}
			return
		case req, ok := <-reqCh:
			if !ok {
				c.Close(websocket.StatusNormalClosure, "")
				return
			}
			if req.Method == "connect" {
				_ = writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "already connected"})
				continue
			}
			if err := s.handleLiveRequest(ctx, connect.ChatID, req, c); err != nil {
				_ = writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: err.Error()})
				continue
			}
		}
	}
}

func (s *Server) handleLiveRequest(ctx context.Context, chatID string, req liveRequestFrame, c *websocket.Conn) error {
	switch req.Method {
	case "sendMessage":
		if s.cfg.Navi == nil {
			return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "NAVI not enabled"})
		}
		var params liveSendMessageParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		if params.SourceChannel == "" {
			params.SourceChannel = "web"
		}
		ingressCtx, cancel := newNaviMessageIngressContext(ctx)
		defer cancel()
		item, err := s.cfg.Navi.SendMessageInput(ingressCtx, chatID, naviruntime.MessageInput{
			Content:          params.Content,
			SourceChannel:    params.SourceChannel,
			SourceMessageRef: params.SourceMessageRef,
			IdempotencyKey:   params.IdempotencyKey,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ingressCtx.Err(), context.DeadlineExceeded) {
				return context.DeadlineExceeded
			}
			return err
		}
		s.triggerConnectorInbound(ctx, params.SourceChannel, params.SourceMessageRef)
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Result: map[string]any{
				"chat_id":       chatID,
				"inbox_item_id": itemID(item),
			},
		})
	case "cancelRun":
		if s.cfg.Navi == nil {
			return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "NAVI not enabled"})
		}
		if err := s.cfg.Navi.CancelRun(chatID); err != nil {
			return err
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: true, Result: map[string]any{"chat_id": chatID}})
	case "resumeRun":
		if s.cfg.Navi == nil {
			return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "NAVI not enabled"})
		}
		if err := s.cfg.Navi.ResumeRun(chatID); err != nil {
			return err
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: true, Result: map[string]any{"chat_id": chatID}})
	case "resolveProposal":
		if s.cfg.Navi == nil {
			return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "NAVI not enabled"})
		}
		var params liveResolveProposalParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		action := strings.TrimSpace(params.Action)
		if action == "" {
			action = strings.TrimSpace(params.Resolution)
		}
		if action == "" {
			action = "approve"
		}

		status := schema.ProposalStatusApproved
		approvalOutcome := ""
		switch action {
		case "approve":
			if err := s.cfg.Navi.ResolveProposal(ctx, params.ProposalID, schema.ProposalStatusApproved, schema.ResolutionTypeApprovedOnce, "owner", params.Note); err != nil {
				return err
			}
		case "decline":
			status = schema.ProposalStatusDeclined
			if err := s.cfg.Navi.ResolveProposal(ctx, params.ProposalID, schema.ProposalStatusDeclined, schema.ResolutionTypeDenied, "owner", params.Note); err != nil {
				return err
			}
		case "deny":
			status = schema.ProposalStatusDeclined
			approvalOutcome = string(schema.ApprovalOutcomeDenied)
			if err := s.cfg.Navi.ResolveBoundaryProposal(ctx, params.ProposalID, schema.ApprovalOutcomeDenied, "", params.Note); err != nil {
				return err
			}
		case "allow_once":
			approvalOutcome = string(schema.ApprovalOutcomeAllowOnce)
			if err := s.cfg.Navi.ResolveBoundaryProposal(ctx, params.ProposalID, schema.ApprovalOutcomeAllowOnce, "", params.Note); err != nil {
				return err
			}
		case "always_allow":
			approvalOutcome = string(schema.ApprovalOutcomeAlwaysAllow)
			if err := s.cfg.Navi.ResolveBoundaryProposal(ctx, params.ProposalID, schema.ApprovalOutcomeAlwaysAllow, "", params.Note); err != nil {
				return err
			}
		case "switch_workspace":
			approvalOutcome = string(schema.ApprovalOutcomeSwitchedWorkspace)
			if err := s.cfg.Navi.ResolveBoundaryProposal(ctx, params.ProposalID, schema.ApprovalOutcomeSwitchedWorkspace, params.SwitchWorkspaceID, params.Note); err != nil {
				return err
			}
		default:
			return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "action must be approve, decline, deny, allow_once, always_allow, or switch_workspace"})
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Result: map[string]any{
				"proposal_id":      params.ProposalID,
				"status":           string(status),
				"approval_outcome": approvalOutcome,
			},
		})
	case "ackNotification":
		var params liveAckNotificationParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return err
			}
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Result: map[string]any{
				"acknowledged":    true,
				"notification_id": params.NotificationID,
			},
		})
	case "getPresence", "getChatRuntimeSummary":
		summary, err := liveChatRuntimeSummary(ctx, s.cfg.DB, chatID)
		if err != nil {
			return err
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type:   "res",
			ID:     req.ID,
			OK:     true,
			Result: summary,
		})
	case "getArtifacts":
		var params liveGetArtifactsParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return err
			}
		}
		ownerID, _ := store.GetOwnerID(ctx, s.cfg.DB)
		artifacts, err := store.ListArtifactsForOwner(ctx, s.cfg.DB, ownerID, 200)
		if err != nil {
			return err
		}
		artifacts, err = liveFilterArtifacts(ctx, s.cfg.DB, chatID, artifacts, params)
		if err != nil {
			return err
		}
		result := make([]map[string]any, 0, len(artifacts))
		for i := range artifacts {
			result = append(result, artifactListItemMap(ctx, s.cfg.DB, artifacts[i]))
		}
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type:   "res",
			ID:     req.ID,
			OK:     true,
			Result: result,
		})
	case "getArtifact":
		startedAt := time.Now().UTC()
		var params liveArtifactParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		artifact, versions, err := s.liveArtifactDetail(ctx, params.ArtifactID)
		if err != nil {
			return err
		}
		resp := artifactToMap(artifact)
		versionMaps := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			versionMaps = append(versionMaps, artifactVersionToMap(v))
		}
		resp["versions"] = versionMaps
		attachArtifactRecords(ctx, s.cfg.DB, artifact.ID, resp)
		s.emitArtifactWorkspaceEvent(ctx, schema.FactArtifactWorkspaceLoaded, artifact, nil, "loaded", "", "", startedAt)
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type:   "res",
			ID:     req.ID,
			OK:     true,
			Result: resp,
		})
	case "getArtifactContent":
		startedAt := time.Now().UTC()
		var params liveArtifactParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		artifact, version, err := s.liveResolveArtifactVersion(ctx, params.ArtifactID, params.VersionID)
		if err != nil {
			return err
		}
		if s.cfg.ArtifactService == nil {
			return fmt.Errorf("artifact content storage not configured")
		}
		content, err := s.cfg.ArtifactService.ReadVersionContent(ctx, version.ID)
		if err != nil {
			s.emitArtifactWorkspaceEvent(ctx, schema.FactArtifactWorkspaceLoadFailed, artifact, &version, "failed", schema.FailureClassStorageFailure, "read_version_content", startedAt)
			return err
		}
		s.emitArtifactWorkspaceEvent(ctx, schema.FactArtifactWorkspaceLoaded, artifact, &version, "loaded", "", "", startedAt)
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Result: map[string]any{
				"artifact_id": artifact.ID,
				"version_id":  version.ID,
				"content":     content,
				"content_ref": contentRefToMap(&version.ContentRef),
			},
		})
	case "getArtifactDiff":
		startedAt := time.Now().UTC()
		var params liveArtifactParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return err
		}
		artifact, version, err := s.liveResolveArtifactVersion(ctx, params.ArtifactID, params.VersionID)
		if err != nil {
			return err
		}
		if s.cfg.ArtifactService == nil {
			return fmt.Errorf("artifact content storage not configured")
		}
		diff, err := s.cfg.ArtifactService.ReadVersionDiff(ctx, version.ID)
		if err != nil {
			s.emitArtifactWorkspaceEvent(ctx, schema.FactArtifactWorkspaceLoadFailed, artifact, &version, "failed", schema.FailureClassStorageFailure, "read_version_diff", startedAt)
			return err
		}
		s.emitArtifactWorkspaceEvent(ctx, schema.FactArtifactWorkspaceLoaded, artifact, &version, "loaded", "", "", startedAt)
		return writeLiveFrame(ctx, c, liveResponseFrame{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Result: map[string]any{
				"artifact_id": artifact.ID,
				"version_id":  version.ID,
				"diff":        decodeFlexibleJSON(diff),
				"diff_ref":    contentRefToMap(version.DiffRef),
			},
		})
	default:
		return writeLiveFrame(ctx, c, liveResponseFrame{Type: "res", ID: req.ID, OK: false, Error: "unsupported method"})
	}
}

func liveFilterArtifacts(ctx context.Context, db *sql.DB, chatID string, list []schema.Artifact, params liveGetArtifactsParams) ([]schema.Artifact, error) {
	view := strings.TrimSpace(params.View)
	switch view {
	case "", "recent":
		return list, nil
	case "current_workspace":
		workspaceID := strings.TrimSpace(params.WorkspaceID)
		if workspaceID == "" {
			inferredWorkspaceID, _, err := inferLiveArtifactContext(ctx, db, chatID, list)
			if err != nil {
				return nil, err
			}
			workspaceID = inferredWorkspaceID
		}
		if workspaceID == "" {
			return []schema.Artifact{}, nil
		}
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if item.WorkspaceID == workspaceID {
				out = append(out, item)
			}
		}
		return out, nil
	case "drafts":
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if item.LifecycleState == schema.ArtifactLifecycleDraft || item.LifecycleState == schema.ArtifactLifecycleInProgress {
				out = append(out, item)
			}
		}
		return out, nil
	case "archived":
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if item.LifecycleState == schema.ArtifactLifecycleArchived {
				out = append(out, item)
			}
		}
		return out, nil
	case "current_project":
		projectID := strings.TrimSpace(params.ProjectID)
		if projectID == "" {
			_, inferredProjectID, err := inferLiveArtifactContext(ctx, db, chatID, list)
			if err != nil {
				return nil, err
			}
			projectID = inferredProjectID
		}
		if projectID == "" {
			return []schema.Artifact{}, nil
		}
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if item.ProjectID == projectID {
				out = append(out, item)
			}
		}
		return out, nil
	case "shared_exported":
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if artifactIsSharedOrExported(item) {
				out = append(out, item)
			}
		}
		return out, nil
	case "failed":
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if item.LifecycleState == schema.ArtifactLifecycleErrored || artifactPendingExecutionState(item) == "failed" || strings.TrimSpace(item.LastError) != "" {
				out = append(out, item)
			}
		}
		return out, nil
	case "current_conversation":
		ids, err := store.ListArtifactIDsBySource(ctx, db, "conversation", chatID, 200)
		if err != nil {
			return nil, err
		}
		allowed := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			allowed[id] = struct{}{}
		}
		out := make([]schema.Artifact, 0, len(list))
		for _, item := range list {
			if _, ok := allowed[item.ID]; ok {
				out = append(out, item)
			}
		}
		return out, nil
	default:
		return list, nil
	}
}

func (s *Server) liveArtifactDetail(ctx context.Context, artifactID string) (*schema.Artifact, []schema.ArtifactVersion, error) {
	if s.cfg.WorldModel == nil {
		return nil, nil, fmt.Errorf("world model not configured")
	}
	artifact, err := s.cfg.WorldModel.GetArtifact(ctx, artifactID)
	if err != nil {
		return nil, nil, err
	}
	if artifact == nil {
		return nil, nil, fmt.Errorf("artifact not found")
	}
	versions, err := s.cfg.WorldModel.ListArtifactVersions(ctx, artifactID, 20)
	if err != nil {
		return nil, nil, err
	}
	return artifact, versions, nil
}

func (s *Server) liveResolveArtifactVersion(ctx context.Context, artifactID, versionID string) (*schema.Artifact, schema.ArtifactVersion, error) {
	artifact, _, err := s.liveArtifactDetail(ctx, artifactID)
	if err != nil {
		return nil, schema.ArtifactVersion{}, err
	}
	if strings.TrimSpace(versionID) == "" {
		versionID = artifact.CurrentVersionID
	}
	if strings.TrimSpace(versionID) == "" {
		return nil, schema.ArtifactVersion{}, fmt.Errorf("artifact version not found")
	}
	version, err := store.GetArtifactVersion(ctx, s.cfg.DB, versionID)
	if err != nil {
		return nil, schema.ArtifactVersion{}, err
	}
	if version.ArtifactID != artifact.ID {
		return nil, schema.ArtifactVersion{}, fmt.Errorf("artifact version not found")
	}
	return artifact, version, nil
}

func liveChatRuntimeSummary(ctx context.Context, db *sql.DB, chatID string) (map[string]any, error) {
	summary := map[string]any{
		"chat_id": chatID,
	}
	var projectID sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT project_id
		FROM navi_chats
		WHERE chat_id = ? AND deleted_at IS NULL
	`, chatID).Scan(&projectID); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("live chat runtime summary project lookup: %w", err)
	}
	if projectID.Valid && strings.TrimSpace(projectID.String) != "" {
		projectIDValue := strings.TrimSpace(projectID.String)
		summary["project_id"] = projectIDValue
		if project, err := store.GetProject(ctx, db, projectIDValue); err == nil {
			summary["project"] = projectToMap(project)
		}
	}
	var pendingCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM navi_inbox
		WHERE chat_id = ? AND status = 'pending'
	`, chatID).Scan(&pendingCount); err != nil {
		return nil, fmt.Errorf("live chat runtime summary pending count: %w", err)
	}
	var deferredCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM navi_inbox
		WHERE chat_id = ? AND status = 'deferred'
	`, chatID).Scan(&deferredCount); err != nil {
		return nil, fmt.Errorf("live chat runtime summary deferred count: %w", err)
	}
	summary["pending_count"] = pendingCount
	summary["deferred_count"] = deferredCount

	var (
		runtimeSessionID  sql.NullString
		runID             sql.NullString
		status            sql.NullString
		phase             sql.NullString
		blockedProposalID sql.NullString
		interruptClass    sql.NullString
		interruptReason   sql.NullString
		updatedAt         sql.NullTime
		mainArtifactID    sql.NullString
		artifactIDs       sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT runtime_session_id, run_id, status, current_phase, blocked_on_proposal_id, interrupt_class, interrupt_reason, updated_at, main_artifact_id, artifact_ids
		FROM runtime_runs
		WHERE chat_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, chatID).Scan(&runtimeSessionID, &runID, &status, &phase, &blockedProposalID, &interruptClass, &interruptReason, &updatedAt, &mainArtifactID, &artifactIDs)
	switch {
	case err == nil:
		var ids []string
		if artifactIDs.Valid && artifactIDs.String != "" {
			_ = json.Unmarshal([]byte(artifactIDs.String), &ids)
		}
		summary["run"] = map[string]any{
			"runtime_session_id":     runtimeSessionID.String,
			"run_id":                 runID.String,
			"status":                 status.String,
			"phase":                  phase.String,
			"blocked_on_proposal_id": blockedProposalID.String,
			"interrupt_class":        interruptClass.String,
			"interrupt_reason":       interruptReason.String,
			"updated_at":             updatedAt.Time,
			"main_artifact_id":       mainArtifactID.String,
			"artifact_ids":           ids,
		}
		if runID.Valid && strings.TrimSpace(runID.String) != "" {
			terminal, termErr := latestRunTerminalEvent(ctx, db, chatID, runID.String)
			if termErr != nil {
				return nil, fmt.Errorf("live chat runtime summary terminal lookup: %w", termErr)
			}
			if terminal != nil {
				summary["terminal_event"] = terminal
			}
		}
	case err == sql.ErrNoRows:
	default:
		return nil, fmt.Errorf("live chat runtime summary run lookup: %w", err)
	}
	return summary, nil
}

func latestRunTerminalEvent(ctx context.Context, db *sql.DB, chatID, runID string) (*liveTerminalSummary, error) {
	if db == nil || strings.TrimSpace(chatID) == "" || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	row := db.QueryRowContext(ctx, `
		SELECT type, seq, timestamp, payload
		FROM events
		WHERE correlation_id = ? AND run_id = ? AND type IN (?, ?, ?)
		ORDER BY seq DESC
		LIMIT 1
	`, chatID, runID, string(schema.FactRunCompleted), string(schema.FactRunFailed), string(schema.FactRunCancelled))
	var (
		eventType schema.EventType
		seq       int64
		tsText    string
		payload   string
	)
	if err := row.Scan((*string)(&eventType), &seq, &tsText, &payload); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ts, err := time.Parse(time.RFC3339Nano, tsText)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, tsText)
		if err != nil {
			return nil, err
		}
	}
	var decoded any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return nil, err
	}
	return &liveTerminalSummary{
		Type:      eventType,
		Seq:       seq,
		Timestamp: ts,
		Payload:   decoded,
	}, nil
}

func replayLiveEvents(ctx context.Context, c *websocket.Conn, db *sql.DB, chatID string, seq *int64, allowed []schema.EventVisibility) error {
	events, err := store.SessionEventsSince(ctx, db, chatID, *seq, allowed, 200)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if isLiveTerminalEvent(ev.Type) {
			slog.Debug("gateway: replaying terminal live event",
				"chat_id", chatID,
				"run_id", ev.RunID,
				"event_type", ev.Type,
				"seq", ev.Seq,
			)
		}
		if err := writeLiveFrame(ctx, c, liveEventFrame{Type: "event", Event: ev}); err != nil {
			return err
		}
		*seq = ev.Seq
	}
	return nil
}

func writeLiveFrame(ctx context.Context, c *websocket.Conn, frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return c.Write(ctx, websocket.MessageText, data)
}

func visibilitiesForStream(stream string) []schema.EventVisibility {
	switch stream {
	case "operator":
		return []schema.EventVisibility{schema.VisibilityUser, schema.VisibilityOperator}
	case "audit":
		return []schema.EventVisibility{schema.VisibilityUser, schema.VisibilityOperator, schema.VisibilityAudit}
	default:
		return []schema.EventVisibility{schema.VisibilityUser}
	}
}

func isLiveTerminalEvent(eventType schema.EventType) bool {
	switch eventType {
	case schema.FactAssistantMessageCompleted, schema.FactRunCompleted, schema.FactRunFailed, schema.FactRunCancelled:
		return true
	default:
		return false
	}
}

func itemID(item *naviruntime.InboxItem) string {
	if item == nil {
		return ""
	}
	return item.ID
}

func chatMaxSeq(ctx context.Context, db *sql.DB, chatID string, allowed []schema.EventVisibility) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("database not configured")
	}
	query := `SELECT COALESCE(MAX(seq), 0) FROM events WHERE correlation_id = ?`
	args := []any{chatID}
	if len(allowed) > 0 {
		query += ` AND visibility IN (` + placeholders(len(allowed)) + `)`
		for _, v := range allowed {
			args = append(args, string(v))
		}
	}
	var seq int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func naviAttentionFromEnvelope(envelope presence.PresenceEnvelope) presence.PresenceAttention {
	if payload, ok := envelope.Payload.(presence.NaviPresencePayload); ok {
		return payload.Attention
	}
	return presence.PresenceAttention{}
}
