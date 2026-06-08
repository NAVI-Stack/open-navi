package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coreskill "github.com/open-navi/navi/internal/navi/skill"
	storepkg "github.com/open-navi/navi/internal/store"
)

func TestContactHandlersReturnPersistedTimestamps(t *testing.T) {
	db := storepkg.InitTestDB(t)
	const skillID = "test-navi-contacts-timestamps"
	RegisterContactsHandler(skillID, db)

	createHandler, ok := coreskill.GetInternalHandler(skillID, "create_contact")
	if !ok {
		t.Fatal("create_contact handler not registered")
	}
	createPayload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name": "Ada Lovelace",
	})
	if err != nil {
		t.Fatalf("create_contact: %v", err)
	}
	created := createPayload.(map[string]any)
	id := created["id"].(string)
	createdAt := parseRFC3339Field(t, created, "created_at")
	if createdAt.IsZero() {
		t.Fatal("create_contact returned a zero created_at")
	}
	time.Sleep(1100 * time.Millisecond)

	updateHandler, ok := coreskill.GetInternalHandler(skillID, "update_contact")
	if !ok {
		t.Fatal("update_contact handler not registered")
	}
	updatePayload, err := updateHandler(context.Background(), nil, nil, map[string]any{
		"id":   id,
		"name": "Augusta Ada Lovelace",
	})
	if err != nil {
		t.Fatalf("update_contact: %v", err)
	}
	updated := updatePayload.(map[string]any)
	updatedAtRaw, ok := updated["updated_at"].(string)
	if !ok || updatedAtRaw == "" {
		t.Fatalf("expected updated_at string in %#v", updated)
	}
	updatedAt := parseRFC3339Field(t, updated, "updated_at")
	if !updatedAt.After(createdAt) {
		t.Fatalf("expected updated_at %s to be after created_at %s", updatedAt, createdAt)
	}

	saved, err := storepkg.GetContact(context.Background(), db, id)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if updatedAtRaw != saved.UpdatedAt.Format(time.RFC3339) {
		t.Fatalf("update_contact returned updated_at %s, stored value is %s", updatedAtRaw, saved.UpdatedAt.Format(time.RFC3339))
	}
}

func TestCreateContactWithTrustLevel(t *testing.T) {
	db := storepkg.InitTestDB(t)
	const skillID = "test-navi-contacts-trust"
	RegisterContactsHandler(skillID, db)

	createHandler, ok := coreskill.GetInternalHandler(skillID, "create_contact")
	if !ok {
		t.Fatal("create_contact handler not registered")
	}
	payload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name":        "NAVI-Alpha",
		"kind":        "navi",
		"trust_level": "high",
	})
	if err != nil {
		t.Fatalf("create_contact: %v", err)
	}
	created := payload.(map[string]any)
	if created["trust_level"] != "high" {
		t.Fatalf("expected trust_level 'high' in create response, got %#v", created["trust_level"])
	}
	if created["kind"] != "navi" {
		t.Fatalf("expected kind 'navi', got %#v", created["kind"])
	}

	id := created["id"].(string)
	getHandler, ok := coreskill.GetInternalHandler(skillID, "get_contact")
	if !ok {
		t.Fatal("get_contact handler not registered")
	}
	getPayload, err := getHandler(context.Background(), nil, nil, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_contact: %v", err)
	}
	got := getPayload.(map[string]any)
	if got["trust_level"] != "high" {
		t.Fatalf("expected persisted trust_level 'high', got %#v", got["trust_level"])
	}

	// Verify via store directly
	saved, err := storepkg.GetContact(context.Background(), db, id)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if saved.TrustLevel != "high" {
		t.Fatalf("stored trust_level = %q, want %q", saved.TrustLevel, "high")
	}
}

func TestLinkIdentityToContact(t *testing.T) {
	db := storepkg.InitTestDB(t)
	const skillID = "test-navi-contacts-link"
	RegisterContactsHandler(skillID, db)

	createHandler, ok := coreskill.GetInternalHandler(skillID, "create_contact")
	if !ok {
		t.Fatal("create_contact handler not registered")
	}
	payload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name": "NAVI-Beta",
		"kind": "navi",
	})
	if err != nil {
		t.Fatalf("create_contact: %v", err)
	}
	id := payload.(map[string]any)["id"].(string)

	linkHandler, ok := coreskill.GetInternalHandler(skillID, "link_identity")
	if !ok {
		t.Fatal("link_identity handler not registered")
	}
	linkPayload, err := linkHandler(context.Background(), nil, nil, map[string]any{
		"contact_id":       id,
		"identifier_type":  "navi_id",
		"identifier_value": "navi://agent/beta-001",
	})
	if err != nil {
		t.Fatalf("link_identity: %v", err)
	}
	result := linkPayload.(map[string]any)
	if result["success"] != true {
		t.Fatalf("expected success=true, got %#v", result)
	}

	// Verify via get_contact
	getHandler, ok := coreskill.GetInternalHandler(skillID, "get_contact")
	if !ok {
		t.Fatal("get_contact handler not registered")
	}
	getPayload, err := getHandler(context.Background(), nil, nil, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_contact: %v", err)
	}
	got := getPayload.(map[string]any)

	// metadata is returned as ContactMetadata (struct), re-marshal/unmarshal to check navi_id
	metaBytes, err := json.Marshal(got["metadata"])
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	var meta ContactMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.NaviID != "navi://agent/beta-001" {
		t.Fatalf("expected navi_id = 'navi://agent/beta-001', got %q", meta.NaviID)
	}
	if meta.InteractionCount != 1 {
		t.Fatalf("expected interaction_count 1 after link, got %d", meta.InteractionCount)
	}

	// Linking the same navi_id again should return success=false (deduplicate)
	linkPayload2, err := linkHandler(context.Background(), nil, nil, map[string]any{
		"contact_id":       id,
		"identifier_type":  "navi_id",
		"identifier_value": "navi://agent/beta-001",
	})
	if err != nil {
		t.Fatalf("second link_identity: %v", err)
	}
	result2 := linkPayload2.(map[string]any)
	if result2["success"] != false {
		t.Fatalf("expected success=false for duplicate navi_id link, got %#v", result2)
	}
}

func TestLinkIdentityRejectsIdentifierAlreadyOnAnotherContact(t *testing.T) {
	db := storepkg.InitTestDB(t)
	const skillID = "test-navi-contacts-cross-contact-link"
	RegisterContactsHandler(skillID, db)

	createHandler, ok := coreskill.GetInternalHandler(skillID, "create_contact")
	if !ok {
		t.Fatal("create_contact handler not registered")
	}
	firstPayload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name": "First Contact",
	})
	if err != nil {
		t.Fatalf("create first contact: %v", err)
	}
	secondPayload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name": "Second Contact",
	})
	if err != nil {
		t.Fatalf("create second contact: %v", err)
	}
	firstID := firstPayload.(map[string]any)["id"].(string)
	secondID := secondPayload.(map[string]any)["id"].(string)

	linkHandler, ok := coreskill.GetInternalHandler(skillID, "link_identity")
	if !ok {
		t.Fatal("link_identity handler not registered")
	}
	if _, err := linkHandler(context.Background(), nil, nil, map[string]any{
		"contact_id":       firstID,
		"identifier_type":  "email",
		"identifier_value": "shared@example.com",
	}); err != nil {
		t.Fatalf("link identity to first contact: %v", err)
	}

	duplicatePayload, err := linkHandler(context.Background(), nil, nil, map[string]any{
		"contact_id":       secondID,
		"identifier_type":  "email",
		"identifier_value": "shared@example.com",
	})
	if err != nil {
		t.Fatalf("link duplicate identity to second contact: %v", err)
	}
	duplicate := duplicatePayload.(map[string]any)
	if duplicate["success"] != false {
		t.Fatalf("expected duplicate identity to be rejected, got %#v", duplicate)
	}

	savedSecond, err := storepkg.GetContact(context.Background(), db, secondID)
	if err != nil {
		t.Fatalf("GetContact(second): %v", err)
	}
	var meta ContactMetadata
	if err := json.Unmarshal([]byte(savedSecond.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal second metadata: %v", err)
	}
	if len(meta.Emails) != 0 {
		t.Fatalf("expected duplicate email not to be stored on second contact, got %#v", meta.Emails)
	}
}

func TestRecordInteraction(t *testing.T) {
	db := storepkg.InitTestDB(t)
	const skillID = "test-navi-contacts-interaction"
	RegisterContactsHandler(skillID, db)

	createHandler, ok := coreskill.GetInternalHandler(skillID, "create_contact")
	if !ok {
		t.Fatal("create_contact handler not registered")
	}
	payload, err := createHandler(context.Background(), nil, nil, map[string]any{
		"name": "Charlie Contact",
	})
	if err != nil {
		t.Fatalf("create_contact: %v", err)
	}
	id := payload.(map[string]any)["id"].(string)

	recordHandler, ok := coreskill.GetInternalHandler(skillID, "record_interaction")
	if !ok {
		t.Fatal("record_interaction handler not registered")
	}

	for i, args := range []map[string]any{
		{"contact_id": id, "kind": "message_sent", "note": "hello"},
		{"contact_id": id, "kind": "meeting_held"},
	} {
		r, err := recordHandler(context.Background(), nil, nil, args)
		if err != nil {
			t.Fatalf("record_interaction #%d: %v", i+1, err)
		}
		result := r.(map[string]any)
		if result["success"] != true {
			t.Fatalf("record_interaction #%d: expected success=true", i+1)
		}
		wantCount := i + 1
		gotCount, ok := result["interaction_count"].(int)
		if !ok {
			t.Fatalf("record_interaction #%d: interaction_count type %T, want int", i+1, result["interaction_count"])
		}
		if gotCount != wantCount {
			t.Fatalf("record_interaction #%d: expected count=%d, got %d", i+1, wantCount, gotCount)
		}
	}

	// Verify via get_contact
	getHandler, ok := coreskill.GetInternalHandler(skillID, "get_contact")
	if !ok {
		t.Fatal("get_contact handler not registered")
	}
	getPayload, err := getHandler(context.Background(), nil, nil, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_contact: %v", err)
	}
	got := getPayload.(map[string]any)

	metaBytes, err := json.Marshal(got["metadata"])
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	var meta ContactMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.InteractionCount != 2 {
		t.Fatalf("expected interaction_count=2, got %d", meta.InteractionCount)
	}
	if len(meta.InteractionHistory) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(meta.InteractionHistory))
	}
	if meta.InteractionHistory[0].Kind != "message_sent" {
		t.Fatalf("expected first event kind 'message_sent', got %q", meta.InteractionHistory[0].Kind)
	}
	if meta.InteractionHistory[0].Note != "hello" {
		t.Fatalf("expected first event note 'hello', got %q", meta.InteractionHistory[0].Note)
	}
	if meta.LastInteraction == "" {
		t.Fatal("expected last_interaction to be set")
	}
}

func parseRFC3339Field(t *testing.T, payload map[string]any, key string) time.Time {
	t.Helper()
	raw, ok := payload[key].(string)
	if !ok || raw == "" {
		t.Fatalf("expected %s string in %#v", key, payload)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse %s: %v", key, err)
	}
	return parsed
}
