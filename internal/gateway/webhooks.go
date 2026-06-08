package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/store"
)

const maxWebhookBodyBytes = 1 << 20

type webhookRegistrationResponse struct {
	Source           string `json:"source"`
	Description      string `json:"description,omitempty"`
	ChatID           string `json:"chat_id"`
	Enabled          bool   `json:"enabled"`
	SourceChannel    string `json:"source_channel,omitempty"`
	EventHeader      string `json:"event_header,omitempty"`
	DeliveryIDHeader string `json:"delivery_id_header,omitempty"`
	Signature        struct {
		Type             store.WebhookSignatureType `json:"type"`
		Header           string                     `json:"header,omitempty"`
		Prefix           string                     `json:"prefix,omitempty"`
		SecretConfigured bool                       `json:"secret_configured"`
	} `json:"signature"`
}

type webhookRegistrationRequest struct {
	Description      string                       `json:"description,omitempty"`
	ChatID           string                       `json:"chat_id"`
	Enabled          bool                         `json:"enabled"`
	SourceChannel    string                       `json:"source_channel,omitempty"`
	EventHeader      string                       `json:"event_header,omitempty"`
	DeliveryIDHeader string                       `json:"delivery_id_header,omitempty"`
	Signature        store.WebhookSignatureConfig `json:"signature"`
}

func (s *Server) handleListWebhookRegistrations(w http.ResponseWriter, r *http.Request) {
	regs, err := store.ListWebhookRegistrations(r.Context(), s.cfg.DB)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]webhookRegistrationResponse, 0, len(regs))
	for _, reg := range regs {
		items = append(items, redactWebhookRegistration(reg))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePutWebhookRegistration(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	source := store.NormalizeWebhookSource(r.PathValue("source"))
	if source == "" {
		replyError(w, http.StatusBadRequest, "source required")
		return
	}
	var req webhookRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	reg := store.WebhookRegistration{
		Source:           source,
		Description:      req.Description,
		ChatID:           req.ChatID,
		Enabled:          req.Enabled,
		SourceChannel:    req.SourceChannel,
		EventHeader:      req.EventHeader,
		DeliveryIDHeader: req.DeliveryIDHeader,
		Signature:        req.Signature,
	}
	existing, found, err := store.GetWebhookRegistration(r.Context(), s.cfg.DB, source)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if found && strings.TrimSpace(reg.Signature.Secret) == "" {
		reg.Signature.Secret = existing.Signature.Secret
	}
	reg = store.NormalizeWebhookRegistration(reg)
	if err := validateWebhookRegistration(r.Context(), s, reg); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.UpsertWebhookRegistration(r.Context(), s.cfg.DB, reg); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	saved, _, err := store.GetWebhookRegistration(r.Context(), s.cfg.DB, source)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, redactWebhookRegistration(saved))
}

func (s *Server) handleDeleteWebhookRegistration(w http.ResponseWriter, r *http.Request) {
	source := store.NormalizeWebhookSource(r.PathValue("source"))
	if source == "" {
		replyError(w, http.StatusBadRequest, "source required")
		return
	}
	if err := store.DeleteWebhookRegistration(r.Context(), s.cfg.DB, source); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"status": "deleted", "source": source})
}

func (s *Server) handleWebhookIngress(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	source := store.NormalizeWebhookSource(r.PathValue("source"))
	reg, found, err := store.GetWebhookRegistration(r.Context(), s.cfg.DB, source)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found || !reg.Enabled {
		http.NotFound(w, r)
		return
	}
	hidden, err := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, reg.ChatID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		http.NotFound(w, r)
		return
	}
	body, err := readWebhookBody(r)
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateWebhookSignature(reg, r, body); err != nil {
		replyError(w, http.StatusUnauthorized, err.Error())
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		replyError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	eventName := strings.TrimSpace(r.Header.Get(reg.EventHeader))
	deliveryID := strings.TrimSpace(r.Header.Get(reg.DeliveryIDHeader))
	item := &naviruntime.InboxItem{
		ChatID:           reg.ChatID,
		SourceChannel:    reg.SourceChannel,
		SourceMessageRef: webhookMessageRef(source, deliveryID),
		ActorType:        "webhook",
		PayloadType:      "json",
		QueueAction:      "append",
		Status:           naviruntime.InboxStatusPending,
		Content:          renderWebhookContent(source, eventName, deliveryID, body),
		Structured:       json.RawMessage(body),
		CorrelationID:    webhookCorrelationID(source, deliveryID),
		IdempotencyKey:   deliveryID,
		ClassifiedReason: "external webhook ingress",
		Confidence:       1,
		ReceivedAt:       time.Now().UTC(),
	}
	accepted, err := s.cfg.Navi.SendSignal(r.Context(), item)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusAccepted, map[string]any{
		"status":        "queued",
		"source":        source,
		"chat_id":       reg.ChatID,
		"event":         eventName,
		"delivery_id":   deliveryID,
		"inbox_item_id": itemID(accepted),
	})
}

func validateWebhookRegistration(ctx context.Context, srv *Server, reg store.WebhookRegistration) error {
	if strings.TrimSpace(reg.ChatID) == "" {
		return fmt.Errorf("chat_id required")
	}
	hidden, err := chatHiddenFromPublicAPI(ctx, srv.cfg.DB, reg.ChatID)
	if err != nil {
		return err
	}
	if hidden {
		return fmt.Errorf("chat_id must refer to a user-facing chat")
	}
	if _, err := srv.cfg.Navi.GetChat(ctx, reg.ChatID); err != nil {
		return fmt.Errorf("chat_id not found")
	}
	switch reg.Signature.Type {
	case store.WebhookSignatureNone:
		return nil
	case store.WebhookSignatureHMACSHA256, store.WebhookSignatureHeaderValue:
		if strings.TrimSpace(reg.Signature.Header) == "" {
			return fmt.Errorf("signature.header required")
		}
		if strings.TrimSpace(reg.Signature.Secret) == "" {
			return fmt.Errorf("signature.secret required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported signature.type %q", reg.Signature.Type)
	}
}

func validateWebhookSignature(reg store.WebhookRegistration, r *http.Request, body []byte) error {
	switch reg.Signature.Type {
	case store.WebhookSignatureNone:
		return nil
	case store.WebhookSignatureHeaderValue:
		got := strings.TrimSpace(r.Header.Get(reg.Signature.Header))
		if got == "" {
			return fmt.Errorf("missing signature header")
		}
		if !hmac.Equal([]byte(got), []byte(reg.Signature.Secret)) {
			return fmt.Errorf("invalid signature")
		}
		return nil
	case store.WebhookSignatureHMACSHA256:
		got := strings.TrimSpace(r.Header.Get(reg.Signature.Header))
		if got == "" {
			return fmt.Errorf("missing signature header")
		}
		mac := hmac.New(sha256.New, []byte(reg.Signature.Secret))
		mac.Write(body)
		expected := reg.Signature.Prefix + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(got), []byte(expected)) {
			return fmt.Errorf("invalid signature")
		}
		return nil
	default:
		return fmt.Errorf("unsupported signature type")
	}
}

func readWebhookBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("request body required")
	}
	if len(body) > maxWebhookBodyBytes {
		return nil, fmt.Errorf("request body too large")
	}
	return body, nil
}

func renderWebhookContent(source, eventName, deliveryID string, raw []byte) string {
	pretty := string(raw)
	var buf strings.Builder
	if compact, err := prettyWebhookPayload(raw); err == nil {
		pretty = compact
	}
	buf.WriteString("Webhook received")
	if source != "" {
		buf.WriteString(" from ")
		buf.WriteString(source)
	}
	buf.WriteString(".")
	if eventName != "" {
		buf.WriteString("\nEvent: ")
		buf.WriteString(eventName)
	}
	if deliveryID != "" {
		buf.WriteString("\nDelivery: ")
		buf.WriteString(deliveryID)
	}
	buf.WriteString("\nPayload:\n")
	buf.WriteString(pretty)
	return buf.String()
}

func prettyWebhookPayload(raw []byte) (string, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	out := string(pretty)
	if len(out) > 8192 {
		return out[:8192] + "\n... [truncated]", nil
	}
	return out, nil
}

func webhookCorrelationID(source, deliveryID string) string {
	if deliveryID != "" {
		return "webhook:" + source + ":" + deliveryID
	}
	return "webhook:" + source
}

func webhookMessageRef(source, deliveryID string) string {
	if deliveryID != "" {
		return source + ":" + deliveryID
	}
	return source
}

func redactWebhookRegistration(reg store.WebhookRegistration) webhookRegistrationResponse {
	out := webhookRegistrationResponse{
		Source:           reg.Source,
		Description:      reg.Description,
		ChatID:           reg.ChatID,
		Enabled:          reg.Enabled,
		SourceChannel:    reg.SourceChannel,
		EventHeader:      reg.EventHeader,
		DeliveryIDHeader: reg.DeliveryIDHeader,
	}
	out.Signature.Type = reg.Signature.Type
	out.Signature.Header = reg.Signature.Header
	out.Signature.Prefix = reg.Signature.Prefix
	out.Signature.SecretConfigured = strings.TrimSpace(reg.Signature.Secret) != ""
	return out
}
