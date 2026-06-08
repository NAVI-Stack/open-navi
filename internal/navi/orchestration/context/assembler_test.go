package context

import (
	stdcontext "context"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

func TestAssemblerAssemblesPhaseOneSourcesWithProvenance(t *testing.T) {
	assembler := NewAssembler(ResolverFunc(func(stdcontext.Context, orchestration.CanonicalRunRequest) (AssembleInput, error) {
		return AssembleInput{
			Summaries: []SummaryInput{{
				Block:     "## Earlier session summary\n- unresolved item",
				SourceRef: "chat:sess-1",
			}},
			Facts: []FactsInput{{
				Block:     "## Facts\n- owner prefers concise replies",
				SourceRef: "facts:sess-1",
			}},
			RuntimeStates: []RuntimeStateInput{{
				Block:     "## Runtime State\n- Active LLM: ollama/llama3.1",
				SourceRef: "run:run-1",
			}},
			Proposals: []ProposalInput{{
				ProposalID:     "prop-1",
				Status:         "pending",
				ProposedAction: "write workspace file",
				Summary:        "Approval required for scoped write",
				SourceRef:      "proposal:prop-1",
			}},
			Capabilities: []CapabilitySnapshotInput{{
				Surface:         "runtime",
				ToolNames:       []string{"read_file", "send_reply"},
				SelectionReason: "foreground run",
				SourceRef:       "surface:runtime",
			}},
		}, nil
	}), Config{})

	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			ChatID: "sess-1",
			RunID:     "run-1",
		},
		Conversation: []orchestration.ConversationTurn{
			{ID: "m1", Role: "user", Content: "first", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "m2", Role: "assistant", Content: "second", CreatedAt: time.Unix(2, 0).UTC()},
		},
	}

	pack, err := assembler.Assemble(stdcontext.Background(), req)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pack.Items) < 6 {
		t.Fatalf("expected assembled items, got %d", len(pack.Items))
	}
	if pack.AssembledAt.IsZero() {
		t.Fatal("expected assembled timestamp")
	}

	var sawConversation, sawSummary, sawFacts, sawRuntime, sawProposal, sawCapability bool
	for _, item := range pack.Items {
		if item.SourceRef == "" {
			t.Fatalf("expected source ref on item %+v", item)
		}
		if item.ContextClass == "" {
			t.Fatalf("expected context class on item %+v", item)
		}
		switch item.ContextClass {
		case orchestration.ContextClassConversation:
			sawConversation = true
			if item.Source != orchestration.ContextSourceHistory {
				t.Fatalf("unexpected conversation provenance: %+v", item)
			}
			role := item.Attributes["role"]
			if role == "user" && item.Trust != orchestration.TrustLabelUserSupplied {
				t.Fatalf("expected user conversation item to be user supplied, got %+v", item)
			}
			if role != "user" && item.Trust != orchestration.TrustLabelDerived {
				t.Fatalf("expected non-user conversation item to be derived, got %+v", item)
			}
		case orchestration.ContextClassChatSummary:
			sawSummary = true
		case orchestration.ContextClassFacts:
			sawFacts = true
		case orchestration.ContextClassRuntimeState:
			sawRuntime = true
		case orchestration.ContextClassProposal:
			sawProposal = true
		case orchestration.ContextClassCapabilitySurface:
			sawCapability = true
		}
	}
	if !(sawConversation && sawSummary && sawFacts && sawRuntime && sawProposal && sawCapability) {
		t.Fatalf("missing one or more expected context classes: %+v", pack.Items)
	}
}

func TestAssemblerSelectsRecentMessagesOnlyAndKeepsChronologicalOrder(t *testing.T) {
	assembler := NewAssembler(nil, Config{RecentMessageLimit: 3})
	req := orchestration.CanonicalRunRequest{}
	for i := 1; i <= 5; i++ {
		req.Conversation = append(req.Conversation, orchestration.ConversationTurn{
			ID:        "m" + string(rune('0'+i)),
			Role:      "user",
			Content:   strings.Repeat("x", i),
			CreatedAt: time.Unix(int64(i), 0).UTC(),
		})
	}

	pack, err := assembler.Assemble(stdcontext.Background(), req)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pack.Items) != 3 {
		t.Fatalf("expected 3 recent messages, got %d", len(pack.Items))
	}
	if pack.Items[0].Content != "xxx" || pack.Items[1].Content != "xxxx" || pack.Items[2].Content != "xxxxx" {
		t.Fatalf("expected chronological recent messages, got %+v", pack.Items)
	}
}

func TestAssemblerConversationTrustTracksMessageRole(t *testing.T) {
	assembler := NewAssembler(nil, Config{})
	req := orchestration.CanonicalRunRequest{
		Conversation: []orchestration.ConversationTurn{
			{ID: "u1", Role: "user", Content: "owner input", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "a1", Role: "assistant", Content: "prior model reply", CreatedAt: time.Unix(2, 0).UTC()},
		},
	}

	pack, err := assembler.Assemble(stdcontext.Background(), req)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pack.Items) != 2 {
		t.Fatalf("expected 2 conversation items, got %d", len(pack.Items))
	}
	if pack.Items[0].Trust != orchestration.TrustLabelUserSupplied {
		t.Fatalf("expected user message to be user supplied, got %+v", pack.Items[0])
	}
	if pack.Items[1].Trust != orchestration.TrustLabelDerived {
		t.Fatalf("expected assistant message to be derived, got %+v", pack.Items[1])
	}
}

func TestAssemblerBudgetReportsTruncation(t *testing.T) {
	assembler := NewAssembler(ResolverFunc(func(stdcontext.Context, orchestration.CanonicalRunRequest) (AssembleInput, error) {
		return AssembleInput{
			RuntimeStates: []RuntimeStateInput{{Block: "runtime state", SourceRef: "run"}},
			Summaries:     []SummaryInput{{Block: strings.Repeat("s", 20), SourceRef: "summary"}},
			Facts:         []FactsInput{{Block: strings.Repeat("f", 20), SourceRef: "facts"}},
		}, nil
	}), Config{MaxItems: 2, MaxChars: 30})

	pack, err := assembler.Assemble(stdcontext.Background(), orchestration.CanonicalRunRequest{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if !pack.Budget.Truncated {
		t.Fatalf("expected truncated budget, got %+v", pack.Budget)
	}
	if pack.Budget.ExcludedItems == 0 {
		t.Fatalf("expected excluded items, got %+v", pack.Budget)
	}
	if len(pack.Warnings) == 0 {
		t.Fatal("expected budget warning")
	}
}

func TestAssemblerBudgetHonorsTokenLimit(t *testing.T) {
	assembler := NewAssembler(ResolverFunc(func(stdcontext.Context, orchestration.CanonicalRunRequest) (AssembleInput, error) {
		return AssembleInput{
			RuntimeStates: []RuntimeStateInput{{Block: strings.Repeat("r", 20), SourceRef: "run"}},
			Summaries:     []SummaryInput{{Block: strings.Repeat("s", 20), SourceRef: "summary"}},
			Facts:         []FactsInput{{Block: strings.Repeat("f", 20), SourceRef: "facts"}},
		}, nil
	}), Config{MaxItems: 5, MaxChars: 1000, MaxTokens: 10})

	pack, err := assembler.Assemble(stdcontext.Background(), orchestration.CanonicalRunRequest{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if pack.Budget.UsedTokens > pack.Budget.MaxTokens {
		t.Fatalf("expected used tokens to honor max token budget, got %+v", pack.Budget)
	}
	if !pack.Budget.Truncated {
		t.Fatalf("expected token budget to truncate context, got %+v", pack.Budget)
	}
}

func TestAssemblerDefaultProposalAndScratchpadSources(t *testing.T) {
	assembler := NewAssembler(nil, Config{})
	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			RunID:            "run-7",
			ResumeProposalID: "proposal-7",
			ResumeReason:     "proposal approved; resume execution",
			Scratchpad: map[string]string{
				"current_user_message": "hello",
			},
		},
	}

	pack, err := assembler.Assemble(stdcontext.Background(), req)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	var sawProposal, sawScratchpad bool
	for _, item := range pack.Items {
		switch item.ContextClass {
		case orchestration.ContextClassProposal:
			sawProposal = true
		case orchestration.ContextClassScratchpad:
			sawScratchpad = true
		}
	}
	if !(sawProposal && sawScratchpad) {
		t.Fatalf("expected default proposal and scratchpad items, got %+v", pack.Items)
	}
}

func TestAssemblerIncludesResumeReasonInContextPack(t *testing.T) {
	assembler := NewAssembler(nil, Config{})
	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			RunID:            "run-8",
			ResumeProposalID: "proposal-8",
			ResumeReason:     "owner approved the file write",
		},
	}

	pack, err := assembler.Assemble(stdcontext.Background(), req)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	found := false
	for _, item := range pack.Items {
		if item.ContextClass == orchestration.ContextClassProposal && strings.Contains(item.Content, "resume_reason: owner approved the file write") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected resume reason to be materialized in the assembled context pack, got %+v", pack.Items)
	}
}

func TestFormatScratchpadStateSortsKeysDeterministically(t *testing.T) {
	got := formatScratchpadState(map[string]string{
		"zeta":  "last",
		"alpha": "first",
		"mid":   "middle",
	})
	want := "scratchpad:\nalpha: first\nmid: middle\nzeta: last"
	if got != want {
		t.Fatalf("expected deterministic scratchpad ordering, got %q", got)
	}
}

func TestAssemblerAssignsExpectedSourceAndTrustLabels(t *testing.T) {
	assembler := NewAssembler(ResolverFunc(func(stdcontext.Context, orchestration.CanonicalRunRequest) (AssembleInput, error) {
		return AssembleInput{
			RuntimeStates: []RuntimeStateInput{{
				Block:     "## Runtime State\n- active model",
				SourceRef: "run:abc",
			}},
			Proposals: []ProposalInput{{
				ProposalID:     "prop-abc",
				Status:         "pending",
				ProposedAction: "write file",
				SourceRef:      "proposal:abc",
			}},
			Capabilities: []CapabilitySnapshotInput{{
				Surface:         "runtime",
				ToolNames:       []string{"read_file"},
				SelectionReason: "selected for run",
				SourceRef:       "surface:runtime",
			}},
		}, nil
	}), Config{})

	pack, err := assembler.Assemble(stdcontext.Background(), orchestration.CanonicalRunRequest{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	var sawRuntime, sawProposal, sawCapability bool
	for _, item := range pack.Items {
		switch item.ContextClass {
		case orchestration.ContextClassRuntimeState:
			sawRuntime = true
			if item.Source != orchestration.ContextSourceRuntime || item.Trust != orchestration.TrustLabelAuthoritative {
				t.Fatalf("unexpected runtime item provenance: %+v", item)
			}
		case orchestration.ContextClassProposal:
			sawProposal = true
			if item.Source != orchestration.ContextSourceGovernance || item.Trust != orchestration.TrustLabelAuthoritative {
				t.Fatalf("unexpected proposal provenance: %+v", item)
			}
		case orchestration.ContextClassCapabilitySurface:
			sawCapability = true
			if item.Source != orchestration.ContextSourceCapability || item.Trust != orchestration.TrustLabelAuthoritative {
				t.Fatalf("unexpected capability provenance: %+v", item)
			}
		}
	}
	if !(sawRuntime && sawProposal && sawCapability) {
		t.Fatalf("expected runtime/proposal/capability items, got %+v", pack.Items)
	}
}

func TestAssemblerBudgetPrioritizesHighRankedContextClasses(t *testing.T) {
	assembler := NewAssembler(ResolverFunc(func(stdcontext.Context, orchestration.CanonicalRunRequest) (AssembleInput, error) {
		return AssembleInput{
			RuntimeStates: []RuntimeStateInput{{
				Block:     "runtime state",
				SourceRef: "run:1",
			}},
			Facts: []FactsInput{{
				Block:     "fact block",
				SourceRef: "facts:1",
			}},
			RecentMessages: []RecentMessagesInput{{
				SourceRef: "chat:1",
				Messages: []orchestration.ConversationTurn{
					{ID: "m1", Role: "user", Content: "recent user message", CreatedAt: time.Unix(1, 0).UTC()},
				},
			}},
		}, nil
	}), Config{MaxItems: 2, MaxChars: 1000})

	pack, err := assembler.Assemble(stdcontext.Background(), orchestration.CanonicalRunRequest{})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pack.Items) != 2 {
		t.Fatalf("expected 2 items after budgeting, got %d", len(pack.Items))
	}
	if pack.Items[0].ContextClass != orchestration.ContextClassRuntimeState || pack.Items[1].ContextClass != orchestration.ContextClassFacts {
		t.Fatalf("expected higher-ranked runtime/facts items to survive truncation, got %+v", pack.Items)
	}
	if !pack.Budget.Truncated || pack.Budget.ExcludedItems != 1 {
		t.Fatalf("expected one excluded low-ranked item, got %+v", pack.Budget)
	}
}
