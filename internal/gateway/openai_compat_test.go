package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
	corestore "github.com/open-navi/navi/internal/store"
)

func TestOAIWaitForReplyIgnoresProactiveAssistantMessages(t *testing.T) {
	db := testDB(t)

	ctx := context.Background()
	proactive := schema.NewRunEvent(
		schema.FactAssistantMessageCompleted,
		schema.EventKindFact,
		"sess-oai",
		schema.AgentNavi,
		"",
		schema.VisibilityUser,
		schema.AssistantMessageCompletedPayload{
			RuntimeSessionID: "sess-oai",
			MessageID:        "msg-proactive",
			Content:          "Background check complete.",
			MessageKind:      string(schema.AssistantMessageKindProactive),
		},
	)
	if err := corestore.AppendEvent(ctx, db, proactive); err != nil {
		t.Fatalf("AppendEvent proactive: %v", err)
	}
	normal := schema.NewRunEvent(
		schema.FactAssistantMessageCompleted,
		schema.EventKindFact,
		"sess-oai",
		schema.AgentNavi,
		"run-1",
		schema.VisibilityUser,
		schema.AssistantMessageCompletedPayload{
			RunID:            "run-1",
			RuntimeSessionID: "sess-oai",
			MessageID:        "msg-reply",
			Content:          "Normal reply",
			MessageKind:      string(schema.AssistantMessageKindReply),
		},
	)
	if err := corestore.AppendEvent(ctx, db, normal); err != nil {
		t.Fatalf("AppendEvent normal: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	got, err := oaiWaitForReply(waitCtx, db, "sess-oai", 0)
	if err != nil {
		t.Fatalf("oaiWaitForReply: %v", err)
	}
	if got != "Normal reply" {
		t.Fatalf("expected normal reply, got %q", got)
	}
}
