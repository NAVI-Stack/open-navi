package handlers

import (
	"context"
	"testing"

	coreskill "github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/store"
)

var GetInternalHandler = coreskill.GetInternalHandler

func TestSelfDiagnosticHandlerRecentErrors(t *testing.T) {
	db := store.InitTestDB(t)
	if err := store.SaveErrorRecord(context.Background(), db, store.ErrorRecord{
		Component:    "runtime",
		ChatID:       "sess-diag",
		ErrorType:    "llm_timeout",
		ErrorMessage: "deadline exceeded",
	}); err != nil {
		t.Fatalf("SaveErrorRecord: %v", err)
	}

	RegisterSelfDiagnosticHandler(db)
	handler, ok := GetInternalHandler("self-diagnostic", "recent_errors")
	if !ok {
		t.Fatal("expected self-diagnostic handler")
	}
	payload, err := handler(context.Background(), nil, &Interface{Name: "recent_errors"}, map[string]any{"chat_id": "sess-diag"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	items, ok := result["items"].([]store.ErrorRecord)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one recent error, got %#v", result["items"])
	}
}
