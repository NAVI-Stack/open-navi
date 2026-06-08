package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/open-navi/navi/internal/navi"
	navistore "github.com/open-navi/navi/internal/navi/store"
	"github.com/open-navi/navi/internal/schema"
)

func (s *Server) handleNaviListConversationEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	if chatID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chat id required", nil)
		return
	}
	if _, err := endpoints.EnsureConsoleEndpoint(r.Context(), chatID); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := endpoints.ListConversationEndpoints(r.Context(), chatID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"endpoints": list})
}

func (s *Server) handleNaviCreateConversationEndpoint(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	var req struct {
		ID                  string         `json:"endpoint_id"`
		ConnectorKind       string         `json:"connector_kind"`
		ConnectorInstanceID string         `json:"connector_instance_id"`
		ExternalChatID      string         `json:"external_chat_id"`
		ExternalThreadID    string         `json:"external_thread_id"`
		DisplayName         string         `json:"display_name"`
		ReceiveEnabled      *bool          `json:"receive_enabled"`
		SendEnabled         *bool          `json:"send_enabled"`
		MirrorEnabled       *bool          `json:"mirror_enabled"`
		Status              string         `json:"status"`
		Metadata            map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	endpoint, err := endpoints.UpsertConnectorEndpoint(r.Context(), navi.UpsertConnectorEndpointInput{
		ID:                  req.ID,
		ChatID:              chatID,
		ConnectorKind:       req.ConnectorKind,
		ConnectorInstanceID: req.ConnectorInstanceID,
		ExternalChatID:      req.ExternalChatID,
		ExternalThreadID:    req.ExternalThreadID,
		DisplayName:         req.DisplayName,
		ReceiveEnabled:      req.ReceiveEnabled,
		SendEnabled:         req.SendEnabled,
		MirrorEnabled:       req.MirrorEnabled,
		Status:              navi.ConversationEndpointStatus(req.Status),
		Metadata:            req.Metadata,
	})
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, map[string]any{"endpoint": endpoint})
}

func (s *Server) handleNaviPatchConversationEndpoint(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	endpointID := strings.TrimSpace(r.PathValue("endpoint_id"))
	existing, err := endpoints.GetConversationEndpoint(r.Context(), endpointID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, navi.ErrConversationEndpointNotFound) {
			statusCode = http.StatusNotFound
		}
		replyError(w, statusCode, err.Error())
		return
	}
	if string(existing.ChatID) != chatID {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "endpoint does not belong to chat", nil)
		return
	}
	var req struct {
		DisplayName    *string        `json:"display_name"`
		ReceiveEnabled *bool          `json:"receive_enabled"`
		SendEnabled    *bool          `json:"send_enabled"`
		MirrorEnabled  *bool          `json:"mirror_enabled"`
		Status         *string        `json:"status"`
		Metadata       map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	var status *navi.ConversationEndpointStatus
	if req.Status != nil {
		value := navi.ConversationEndpointStatus(strings.TrimSpace(*req.Status))
		status = &value
	}
	endpoint, err := endpoints.UpdateConversationEndpoint(r.Context(), endpointID, navi.UpdateConversationEndpointInput{
		DisplayName:    req.DisplayName,
		ReceiveEnabled: req.ReceiveEnabled,
		SendEnabled:    req.SendEnabled,
		MirrorEnabled:  req.MirrorEnabled,
		Status:         status,
		Metadata:       req.Metadata,
	})
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, navi.ErrConversationEndpointNotFound) {
			statusCode = http.StatusNotFound
		}
		replyError(w, statusCode, err.Error())
		return
	}
	if string(endpoint.ChatID) != chatID {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "endpoint does not belong to chat", nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"endpoint": endpoint})
}

func (s *Server) handleNaviGetDeliveryPolicy(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	policy, err := endpoints.GetChatDeliveryPolicy(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"delivery_policy": policy})
}

func (s *Server) handleNaviPatchDeliveryPolicy(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	var req struct {
		DefaultMode string         `json:"default_mode"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	policy, err := endpoints.SetChatDeliveryPolicy(r.Context(), strings.TrimSpace(r.PathValue("id")), navi.DeliveryPolicyMode(req.DefaultMode), req.Metadata)
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"delivery_policy": policy})
}

func (s *Server) handleNaviExplicitEndpointSend(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	chats, err := s.chatStoreForEndpointRoutes()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	var req struct {
		Content     string   `json:"content"`
		EndpointIDs []string `json:"endpoint_ids"`
		Source      string   `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "content required", nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "console"
	}
	messageID, err := chats.AppendSystemAssistantMessage(r.Context(), chatID, content, string(navi.ExperienceModeStandard), source, string(schema.AssistantMessageKindReply))
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	service := navi.DeliveryService{Endpoints: endpoints, Dispatcher: s.connectorDispatcher()}
	deliveries, err := service.DeliverAssistantMessage(r.Context(), navi.AssistantDeliveryRequest{
		ChatID:              chatID,
		MessageID:           messageID,
		Content:             content,
		ExplicitEndpointIDs: req.EndpointIDs,
		CorrelationID:       chatID,
	})
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, map[string]any{"message_id": messageID, "deliveries": deliveries})
}

func (s *Server) handleConnectorEndpointResolve(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.endpointStore()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	chats, err := s.chatStoreForEndpointRoutes()
	if err != nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", err.Error(), nil)
		return
	}
	var req struct {
		ConnectorKind       string `json:"connector_kind"`
		ConnectorInstanceID string `json:"connector_instance_id"`
		ExternalChatID      string `json:"external_chat_id"`
		ExternalThreadID    string `json:"external_thread_id"`
		DisplayName         string `json:"display_name"`
		OwnerID             string `json:"owner_id"`
		WorkspaceID         string `json:"workspace_id"`
		ProjectID           string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	resolver := navi.EndpointResolver{Chats: chats, Endpoints: endpoints}
	origin, err := resolver.ResolveConnectorOrigin(r.Context(), navi.ConnectorEndpointOriginInput{
		ConnectorKind:       req.ConnectorKind,
		ConnectorInstanceID: req.ConnectorInstanceID,
		ExternalChatID:      req.ExternalChatID,
		ExternalThreadID:    req.ExternalThreadID,
		DisplayName:         req.DisplayName,
		OwnerID:             req.OwnerID,
		WorkspaceID:         req.WorkspaceID,
		ProjectID:           req.ProjectID,
	})
	if err != nil {
		if errors.Is(err, navi.ErrConversationEndpointReceiveDisabled) {
			replyErrorAPI(w, http.StatusConflict, "ENDPOINT_RECEIVE_DISABLED", err.Error(), nil)
			return
		}
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, origin)
}

func (s *Server) endpointStore() (navi.ConversationEndpointStore, error) {
	if s.cfg.ConversationEndpoints != nil {
		return s.cfg.ConversationEndpoints, nil
	}
	if s.cfg.DB == nil {
		return nil, errors.New("conversation endpoint store not configured")
	}
	return navistore.NewSQLiteStore(s.cfg.DB), nil
}

func (s *Server) chatStoreForEndpointRoutes() (navi.ChatStore, error) {
	if store, ok := s.cfg.ConversationEndpoints.(navi.ChatStore); ok {
		return store, nil
	}
	if s.cfg.DB == nil {
		return nil, errors.New("chat store not configured")
	}
	return navistore.NewSQLiteStore(s.cfg.DB), nil
}

func (s *Server) connectorDispatcher() navi.ConnectorDispatcher {
	if s.cfg.ConnectorDispatcher != nil {
		return s.cfg.ConnectorDispatcher
	}
	if s.cfg.Manager != nil {
		return s.cfg.Manager
	}
	return nil
}
