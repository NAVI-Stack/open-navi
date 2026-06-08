package store

import (
	"context"
	"testing"
)

func TestWebhookRegistrationsRoundTripAndDefaults(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if err := UpsertWebhookRegistration(ctx, db, WebhookRegistration{
		Source:  "GitHub",
		ChatID:  "sess-webhook",
		Enabled: true,
		Signature: WebhookSignatureConfig{
			Type:   WebhookSignatureHMACSHA256,
			Secret: "topsecret",
		},
	}); err != nil {
		t.Fatalf("UpsertWebhookRegistration: %v", err)
	}

	reg, found, err := GetWebhookRegistration(ctx, db, "github")
	if err != nil {
		t.Fatalf("GetWebhookRegistration: %v", err)
	}
	if !found {
		t.Fatal("expected registration to exist")
	}
	if reg.Source != "github" {
		t.Fatalf("expected normalized source github, got %q", reg.Source)
	}
	if reg.SourceChannel != "webhook" {
		t.Fatalf("expected default source_channel webhook, got %q", reg.SourceChannel)
	}
	if reg.EventHeader != "X-GitHub-Event" {
		t.Fatalf("expected github event header default, got %q", reg.EventHeader)
	}
	if reg.DeliveryIDHeader != "X-GitHub-Delivery" {
		t.Fatalf("expected github delivery header default, got %q", reg.DeliveryIDHeader)
	}
	if reg.Signature.Header != "X-Hub-Signature-256" {
		t.Fatalf("expected github signature header default, got %q", reg.Signature.Header)
	}
	if reg.Signature.Prefix != "sha256=" {
		t.Fatalf("expected github signature prefix default, got %q", reg.Signature.Prefix)
	}
}

func TestWebhookRegistrationsDelete(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if err := UpsertWebhookRegistration(ctx, db, WebhookRegistration{
		Source:  "buildkite",
		ChatID:  "sess-buildkite",
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertWebhookRegistration: %v", err)
	}

	if err := DeleteWebhookRegistration(ctx, db, "buildkite"); err != nil {
		t.Fatalf("DeleteWebhookRegistration: %v", err)
	}

	_, found, err := GetWebhookRegistration(ctx, db, "buildkite")
	if err != nil {
		t.Fatalf("GetWebhookRegistration after delete: %v", err)
	}
	if found {
		t.Fatal("expected registration to be deleted")
	}
}
