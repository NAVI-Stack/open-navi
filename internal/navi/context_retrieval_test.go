package navi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

type fakeRetriever struct {
	block     string
	err       error
	gotQuery  string
	gotBudget int
	calls     int
}

func (f *fakeRetriever) RetrieveContext(_ context.Context, query string, budget int) (string, error) {
	f.calls++
	f.gotQuery = query
	f.gotBudget = budget
	return f.block, f.err
}

func reqWithMessage(chatID, msg string) orchestration.CanonicalRunRequest {
	req := orchestration.CanonicalRunRequest{UserMessage: msg}
	req.Frame.ChatID = chatID
	return req
}

func TestRetrievedContextFacts_InjectsBlock(t *testing.T) {
	fr := &fakeRetriever{block: "## Retrieved context\n- the budget is approved"}
	l := &AgentLoop{cfg: LoopConfig{ContextRetriever: fr}}

	fact, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("chat-1", "what about the budget"))
	if !ok {
		t.Fatal("expected a Facts injection")
	}
	if !strings.Contains(fact.Block, "budget is approved") {
		t.Errorf("block not threaded through: %q", fact.Block)
	}
	if !strings.Contains(fact.SourceRef, "chat-1") {
		t.Errorf("source ref missing chat id: %q", fact.SourceRef)
	}
	if fr.gotQuery != "what about the budget" {
		t.Errorf("query = %q, want the user message", fr.gotQuery)
	}
	if fr.gotBudget != defaultContextRetrievalBudgetTokens {
		t.Errorf("budget = %d, want default %d", fr.gotBudget, defaultContextRetrievalBudgetTokens)
	}
}

func TestRetrievedContextFacts_NilRetrieverPreservesBehavior(t *testing.T) {
	l := &AgentLoop{cfg: LoopConfig{}}
	if _, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("c", "hi")); ok {
		t.Error("nil retriever must not inject")
	}
}

func TestRetrievedContextFacts_EmptyReturnsNoInjection(t *testing.T) {
	fr := &fakeRetriever{block: "   "}
	l := &AgentLoop{cfg: LoopConfig{ContextRetriever: fr}}
	if _, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("c", "hi")); ok {
		t.Error("empty retrieval must not inject (turn-time behavior preserved)")
	}
}

func TestRetrievedContextFacts_EmptyQuerySkips(t *testing.T) {
	fr := &fakeRetriever{block: "x"}
	l := &AgentLoop{cfg: LoopConfig{ContextRetriever: fr}}
	if _, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("c", "   ")); ok {
		t.Error("empty query must not retrieve")
	}
	if fr.calls != 0 {
		t.Error("retriever should not be called for an empty query")
	}
}

func TestRetrievedContextFacts_ErrorIsNonFatal(t *testing.T) {
	fr := &fakeRetriever{err: errors.New("boom")}
	l := &AgentLoop{cfg: LoopConfig{ContextRetriever: fr}}
	if _, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("c", "hi")); ok {
		t.Error("a retrieval error must degrade to no injection, not propagate")
	}
}

func TestRetrievedContextFacts_CustomBudget(t *testing.T) {
	fr := &fakeRetriever{block: "x"}
	l := &AgentLoop{cfg: LoopConfig{ContextRetriever: fr, ContextRetrievalBudgetTokens: 4096}}
	if _, ok := l.retrievedContextFacts(context.Background(), reqWithMessage("c", "hi")); !ok {
		t.Fatal("expected injection")
	}
	if fr.gotBudget != 4096 {
		t.Errorf("budget = %d, want 4096 (config override)", fr.gotBudget)
	}
}
