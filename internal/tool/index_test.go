package tool

import "testing"

func TestToolIndexExactLookupReturnsRegisteredToolOrRelatedMiss(t *testing.T) {
	reg := NewRegistry()
	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Aliases = []string{"read file", "open file"}
	readTool.CapabilityTags = []string{"files", "workspace", "read"}
	readTool.Governance.Domain = "files"
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	idx := NewIndex(reg)
	hit := idx.ExactLookup("navi.files.read", 3)
	if hit.SnapshotID == "" {
		t.Fatal("expected exact lookup snapshot id")
	}
	if hit.ExactMatch == nil || hit.ExactMatch.Tool == nil || hit.ExactMatch.Tool.ToolID != "navi.files.read" {
		t.Fatalf("expected exact tool match, got %#v", hit.ExactMatch)
	}

	miss := idx.ExactLookup("navi.files.open", 3)
	if miss.ExactMatch != nil {
		t.Fatalf("expected exact miss, got %#v", miss.ExactMatch)
	}
	if len(miss.Related) == 0 {
		t.Fatal("expected related matches on exact miss")
	}
	if miss.Related[0].Tool == nil || miss.Related[0].Tool.ToolID != "navi.files.read" {
		t.Fatalf("expected related match to point at read tool, got %#v", miss.Related[0].Tool)
	}
}

func TestToolIndexLexicalSearchRanksAliasCandidates(t *testing.T) {
	reg := NewRegistry()

	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Aliases = []string{"open file", "read file"}
	readTool.Description = "Read a file from the workspace."
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	replyTool := validTestTool("navi.messaging.send_reply")
	replyTool.DisplayName = "Send Reply"
	replyTool.Aliases = []string{"send reply"}
	replyTool.Description = "Send a queued reply to the owner."
	replyTool.Category = ToolCategoryWorkflowAction
	if err := reg.Register(replyTool); err != nil {
		t.Fatalf("register reply tool: %v", err)
	}

	results := NewIndex(reg).LexicalSearch("open file", 5)
	if len(results) == 0 {
		t.Fatal("expected lexical results")
	}
	if results[0].Tool == nil || results[0].Tool.ToolID != "navi.files.read" {
		t.Fatalf("expected alias-ranked read tool first, got %#v", results[0].Tool)
	}
	if results[0].MatchKind != "alias" {
		t.Fatalf("expected alias match kind, got %q", results[0].MatchKind)
	}
}

func TestToolIndexTagSearchUsesCapabilityTagsAndDomain(t *testing.T) {
	reg := NewRegistry()

	calendarTool := validTestTool("navi.domain.calendar.current_time_window")
	calendarTool.DisplayName = "Calendar Current Time Window"
	calendarTool.CapabilityTags = []string{"calendar", "time"}
	calendarTool.Governance.Domain = "calendar"
	if err := reg.Register(calendarTool); err != nil {
		t.Fatalf("register calendar tool: %v", err)
	}

	githubTool := validTestTool("navi.integration.github.repo_info")
	githubTool.DisplayName = "GitHub Repository Info"
	githubTool.CapabilityTags = []string{"github", "repo"}
	githubTool.Governance.Domain = "github"
	if err := reg.Register(githubTool); err != nil {
		t.Fatalf("register github tool: %v", err)
	}

	results := NewIndex(reg).TagSearch("calendar", 5)
	if len(results) != 1 {
		t.Fatalf("expected one tag result, got %+v", results)
	}
	if results[0].Tool == nil || results[0].Tool.ToolID != "navi.domain.calendar.current_time_window" {
		t.Fatalf("expected calendar tag match, got %#v", results[0].Tool)
	}
}

func TestToolIndexFiltersNonDiscoverableStatusesFromSearch(t *testing.T) {
	reg := NewRegistry()

	activeTool := validTestTool("test.registry.active")
	activeTool.DisplayName = "Searchable Tool"
	activeTool.Description = "Searchable active tool."
	if err := reg.Register(activeTool); err != nil {
		t.Fatalf("register active tool: %v", err)
	}

	deprecatedTool := validTestTool("test.registry.deprecated")
	deprecatedTool.DisplayName = "Deprecated Searchable Tool"
	deprecatedTool.Description = "Deprecated but still discoverable."
	deprecatedTool.Status = ToolStatusDeprecated
	if err := reg.Register(deprecatedTool); err != nil {
		t.Fatalf("register deprecated tool: %v", err)
	}

	suspendedTool := validTestTool("test.registry.suspended")
	suspendedTool.DisplayName = "Suspended Searchable Tool"
	suspendedTool.Description = "Should not appear in search."
	suspendedTool.Status = ToolStatusSuspended
	if err := reg.Register(suspendedTool); err != nil {
		t.Fatalf("register suspended tool: %v", err)
	}

	removedTool := validTestTool("test.registry.removed")
	removedTool.DisplayName = "Removed Searchable Tool"
	removedTool.Description = "Should not appear in search."
	removedTool.Status = ToolStatusRemoved
	if err := reg.Register(removedTool); err != nil {
		t.Fatalf("register removed tool: %v", err)
	}

	invalidTool := validTestTool("test.registry.invalid_status")
	invalidTool.DisplayName = "Invalid Searchable Tool"
	invalidTool.Description = "Should not appear in search."
	invalidTool.Status = ToolStatusInvalid
	if err := reg.Register(invalidTool); err != nil {
		t.Fatalf("register invalid tool: %v", err)
	}

	hiddenTool := validTestTool("test.registry.hidden_search")
	hiddenTool.DisplayName = "Hidden Searchable Tool"
	hiddenTool.Description = "Should not appear in search."
	hiddenTool.Hidden = true
	if err := reg.Register(hiddenTool); err != nil {
		t.Fatalf("register hidden tool: %v", err)
	}

	idx := NewIndex(reg)
	results := idx.LexicalSearch("searchable tool", 10)
	if len(results) != 2 {
		t.Fatalf("expected only active and deprecated results, got %+v", results)
	}
	for _, candidate := range results {
		if candidate.Tool == nil {
			t.Fatalf("expected candidate tool payload, got %+v", candidate)
		}
		switch candidate.Tool.ToolID {
		case "test.registry.active", "test.registry.deprecated":
		default:
			t.Fatalf("unexpected non-discoverable candidate %q", candidate.Tool.ToolID)
		}
		if !candidate.Discoverable {
			t.Fatalf("expected returned candidate %q to be discoverable", candidate.Tool.ToolID)
		}
	}

	exactSuspended := idx.ExactLookup("test.registry.suspended", 3)
	if exactSuspended.ExactMatch == nil || exactSuspended.ExactMatch.Tool == nil {
		t.Fatal("expected exact lookup to return suspended registered tool")
	}
	if exactSuspended.ExactMatch.Discoverable {
		t.Fatal("expected suspended exact lookup result to be marked non-discoverable")
	}
}
