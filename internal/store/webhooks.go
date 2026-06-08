package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const webhookRegistrationsSettingKey = "gateway.webhook_registrations.v1"

type WebhookSignatureType string

const (
	WebhookSignatureNone        WebhookSignatureType = "none"
	WebhookSignatureHMACSHA256  WebhookSignatureType = "hmac-sha256"
	WebhookSignatureHeaderValue WebhookSignatureType = "header-value"
)

type WebhookSignatureConfig struct {
	Type   WebhookSignatureType `json:"type"`
	Header string               `json:"header,omitempty"`
	Prefix string               `json:"prefix,omitempty"`
	Secret string               `json:"secret,omitempty"`
}

type WebhookRegistration struct {
	Source           string                 `json:"source"`
	Description      string                 `json:"description,omitempty"`
	ChatID           string                 `json:"chat_id"`
	Enabled          bool                   `json:"enabled"`
	SourceChannel    string                 `json:"source_channel,omitempty"`
	EventHeader      string                 `json:"event_header,omitempty"`
	DeliveryIDHeader string                 `json:"delivery_id_header,omitempty"`
	Signature        WebhookSignatureConfig `json:"signature"`
}

func NormalizeWebhookSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	source = strings.ReplaceAll(source, " ", "-")
	return source
}

func ListWebhookRegistrations(ctx context.Context, db *sql.DB) ([]WebhookRegistration, error) {
	if db == nil {
		return nil, nil
	}
	raw, found, err := GetSetting(ctx, db, webhookRegistrationsSettingKey)
	if err != nil {
		return nil, fmt.Errorf("store: list webhook registrations: %w", err)
	}
	if !found || strings.TrimSpace(raw) == "" {
		return []WebhookRegistration{}, nil
	}
	var regs []WebhookRegistration
	if err := json.Unmarshal([]byte(raw), &regs); err != nil {
		return nil, fmt.Errorf("store: decode webhook registrations: %w", err)
	}
	for i := range regs {
		regs[i] = normalizeWebhookRegistration(regs[i])
	}
	sort.Slice(regs, func(i, j int) bool {
		return regs[i].Source < regs[j].Source
	})
	return regs, nil
}

func GetWebhookRegistration(ctx context.Context, db *sql.DB, source string) (WebhookRegistration, bool, error) {
	source = NormalizeWebhookSource(source)
	if source == "" {
		return WebhookRegistration{}, false, nil
	}
	regs, err := ListWebhookRegistrations(ctx, db)
	if err != nil {
		return WebhookRegistration{}, false, err
	}
	for _, reg := range regs {
		if reg.Source == source {
			return reg, true, nil
		}
	}
	return WebhookRegistration{}, false, nil
}

func UpsertWebhookRegistration(ctx context.Context, db *sql.DB, reg WebhookRegistration) error {
	if db == nil {
		return fmt.Errorf("store: upsert webhook registration: nil db")
	}
	reg = normalizeWebhookRegistration(reg)
	if reg.Source == "" {
		return fmt.Errorf("store: upsert webhook registration: source required")
	}
	regs, err := ListWebhookRegistrations(ctx, db)
	if err != nil {
		return err
	}
	replaced := false
	for i := range regs {
		if regs[i].Source == reg.Source {
			regs[i] = reg
			replaced = true
			break
		}
	}
	if !replaced {
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(i, j int) bool {
		return regs[i].Source < regs[j].Source
	})
	blob, err := json.Marshal(regs)
	if err != nil {
		return fmt.Errorf("store: encode webhook registrations: %w", err)
	}
	if err := SetSetting(ctx, db, webhookRegistrationsSettingKey, string(blob)); err != nil {
		return fmt.Errorf("store: persist webhook registrations: %w", err)
	}
	return nil
}

func DeleteWebhookRegistration(ctx context.Context, db *sql.DB, source string) error {
	if db == nil {
		return fmt.Errorf("store: delete webhook registration: nil db")
	}
	source = NormalizeWebhookSource(source)
	regs, err := ListWebhookRegistrations(ctx, db)
	if err != nil {
		return err
	}
	out := make([]WebhookRegistration, 0, len(regs))
	for _, reg := range regs {
		if reg.Source != source {
			out = append(out, reg)
		}
	}
	blob, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("store: encode webhook registrations: %w", err)
	}
	if err := SetSetting(ctx, db, webhookRegistrationsSettingKey, string(blob)); err != nil {
		return fmt.Errorf("store: persist webhook registrations: %w", err)
	}
	return nil
}

func normalizeWebhookRegistration(reg WebhookRegistration) WebhookRegistration {
	reg.Source = NormalizeWebhookSource(reg.Source)
	reg.SourceChannel = strings.TrimSpace(reg.SourceChannel)
	reg.ChatID = strings.TrimSpace(reg.ChatID)
	reg.Description = strings.TrimSpace(reg.Description)
	reg.EventHeader = strings.TrimSpace(reg.EventHeader)
	reg.DeliveryIDHeader = strings.TrimSpace(reg.DeliveryIDHeader)
	reg.Signature.Type = WebhookSignatureType(strings.TrimSpace(string(reg.Signature.Type)))
	reg.Signature.Header = strings.TrimSpace(reg.Signature.Header)
	reg.Signature.Prefix = strings.TrimSpace(reg.Signature.Prefix)
	reg.Signature.Secret = strings.TrimSpace(reg.Signature.Secret)

	if reg.Source == "github" {
		if reg.EventHeader == "" {
			reg.EventHeader = "X-GitHub-Event"
		}
		if reg.DeliveryIDHeader == "" {
			reg.DeliveryIDHeader = "X-GitHub-Delivery"
		}
		if reg.Signature.Type == WebhookSignatureHMACSHA256 {
			if reg.Signature.Header == "" {
				reg.Signature.Header = "X-Hub-Signature-256"
			}
			if reg.Signature.Prefix == "" {
				reg.Signature.Prefix = "sha256="
			}
		}
	}
	if reg.Signature.Type == "" {
		reg.Signature.Type = WebhookSignatureNone
	}
	if reg.SourceChannel == "" {
		reg.SourceChannel = "webhook"
	}
	return reg
}

func NormalizeWebhookRegistration(reg WebhookRegistration) WebhookRegistration {
	return normalizeWebhookRegistration(reg)
}
