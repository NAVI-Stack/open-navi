package store

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

func TestDirectiveCRUD(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}

	d := schema.NewDirective("Test Directive", schema.DirectiveModeAct, "cli")

	// Create
	if err := SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}

	// Read
	got, found, err := GetDirective(ctx, db, d.DirectiveID)
	if err != nil {
		t.Fatalf("GetDirective: %v", err)
	}
	if !found {
		t.Fatal("directive not found")
	}
	if got.Title != d.Title {
		t.Fatalf("expected title %q, got %q", d.Title, got.Title)
	}

	// Active
	active, err := GetActiveDirectives(ctx, db)
	if err != nil {
		t.Fatalf("GetActiveDirectives: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active directive, got %d", len(active))
	}

	// Update status
	if err := UpdateDirectiveStatus(ctx, db, d.DirectiveID, schema.DirectiveStatusClosed); err != nil {
		t.Fatalf("UpdateDirectiveStatus: %v", err)
	}

	activeAfter, err := GetActiveDirectives(ctx, db)
	if err != nil {
		t.Fatalf("GetActiveDirectives: %v", err)
	}
	if len(activeAfter) != 0 {
		t.Fatalf("expected 0 active directives after close, got %d", len(activeAfter))
	}

	// Update mode
	if err := UpdateDirectiveMode(ctx, db, d.DirectiveID, schema.DirectiveModeChat); err != nil {
		t.Fatalf("UpdateDirectiveMode: %v", err)
	}

	got2, _, _ := GetDirective(ctx, db, d.DirectiveID)
	if got2.Mode != schema.DirectiveModeChat {
		t.Fatalf("expected mode chat, got %q", got2.Mode)
	}
}

func TestDirectiveMessages(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}

	d := schema.NewDirective("Test Messages", schema.DirectiveModeAct, "cli")
	if err := SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}

	// Insert messages out of chronological insertion order but with explicit timestamps
	now := time.Now().UTC()
	m1 := schema.DirectiveMessage{
		MessageID:   uuid.New().String(),
		DirectiveID: d.DirectiveID,
		Role:        "owner",
		Content:     "Hello Orchestrator",
		CreatedAt:   now.Add(-2 * time.Minute),
	}
	m2 := schema.DirectiveMessage{
		MessageID:   uuid.New().String(),
		DirectiveID: d.DirectiveID,
		Role:        "navi",
		Content:     "I am ready.",
		CreatedAt:   now.Add(-1 * time.Minute),
		TokensUsed:  150,
		Model:       "mock-v1",
	}

	if err := AppendMessage(ctx, db, m1); err != nil {
		t.Fatalf("AppendMessage 1: %v", err)
	}
	if err := AppendMessage(ctx, db, m2); err != nil {
		t.Fatalf("AppendMessage 2: %v", err)
	}

	// Read back messages. Should be newest first.
	msgs, err := GetMessages(ctx, db, d.DirectiveID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].MessageID != m2.MessageID {
		t.Fatalf("expected message 2 to be first (newest), got ID %q", msgs[0].MessageID)
	}
	if msgs[0].TokensUsed != 150 {
		t.Fatalf("expected 150 tokens used, got %d", msgs[0].TokensUsed)
	}
	if msgs[0].Model != "mock-v1" {
		t.Fatalf("expected 'mock-v1' model, got %q", msgs[0].Model)
	}

	if msgs[1].MessageID != m1.MessageID {
		t.Fatalf("expected message 1 to be second, got ID %q", msgs[1].MessageID)
	}
}

func TestUpdateDirectiveModeRejectsInvalidMode(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}

	d := schema.NewDirective("Invalid mode test", schema.DirectiveModeAct, "cli")
	if err := SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}

	if err := UpdateDirectiveMode(ctx, db, d.DirectiveID, schema.DirectiveMode("IMPLEMENT")); err == nil {
		t.Fatal("expected invalid directive mode to be rejected")
	}
}
