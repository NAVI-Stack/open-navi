package tool

import (
	"strings"
	"testing"
	"time"
)

func TestActiveToolSetContainsToolVersionByCanonicalID(t *testing.T) {
	reg := NewRegistry()
	alpha := validTestTool("navi.repo.search")
	alpha.SchemaVersion = "1.2.3"
	if err := reg.Register(alpha); err != nil {
		t.Fatalf("register alpha: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	set := broker.BuildActiveToolSet(
		BrokerInput{
			UserInput: "search the repo",
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			Intent:          BrokerIntentCodingQuery,
			SelectedToolIDs: []string{"navi.repo.search"},
			BrokerReason:    "coder search set",
		},
		ActiveToolSetSpec{
			Scope:       ActiveToolSetScopeSession,
			ChatID:      "sess-1",
			Mode:        "coder",
			Environment: "development",
			ToolIDs:     []string{"navi.repo.search"},
			LoadReason:  "coder_session_baseline",
		},
	)

	if strings.TrimSpace(set.ActiveToolSetID) == "" {
		t.Fatal("expected active tool set id")
	}
	if !set.ContainsTool("navi.repo.search") {
		t.Fatalf("expected canonical tool id membership, got %+v", set.Tools)
	}
	if !set.ContainsToolVersion("navi.repo.search", "1.2.3") {
		t.Fatalf("expected schema version membership, got %+v", set.Tools)
	}
	if set.ContainsToolVersion("navi.repo.search", "9.9.9") {
		t.Fatalf("did not expect mismatched schema version to match, got %+v", set.Tools)
	}
	if set.Scope != ActiveToolSetScopeSession {
		t.Fatalf("expected session scope, got %s", set.Scope)
	}
	if set.Provenance.RegistrySnapshotID == "" {
		t.Fatalf("expected registry snapshot provenance, got %+v", set.Provenance)
	}
}

func TestToolBrokerBuildActiveToolSetFromSelectedTools(t *testing.T) {
	reg := NewRegistry()
	for _, toolID := range []string{"navi.files.read", "navi.repo.search"} {
		if err := reg.Register(validTestTool(toolID)); err != nil {
			t.Fatalf("register %q: %v", toolID, err)
		}
	}

	broker := NewToolBrokerFromRegistry(reg)
	resolution := broker.Resolve(BrokerInput{
		UserInput:     "search the repo for a symbol",
		SessionMode:   DiscoverySessionModeCoder,
		Environment:   "development",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	})
	activeSet := broker.BuildActiveToolSet(
		BrokerInput{
			UserInput:     "search the repo for a symbol",
			SessionMode:   DiscoverySessionModeCoder,
			Environment:   "development",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		resolution,
		ActiveToolSetSpec{
			Scope:       ActiveToolSetScopeProviderCall,
			ChatID:      "sess-2",
			Mode:        "coder",
			Environment: "development",
			LoadReason:  "provider_call_surface",
		},
	)

	if len(activeSet.Tools) != len(resolution.SelectedToolIDs) {
		t.Fatalf("expected active set tools to match broker selection, set=%+v selected=%+v", activeSet.Tools, resolution.SelectedToolIDs)
	}
	if activeSet.Constraints.MaxParallelCalls != 1 {
		t.Fatalf("expected default max parallel calls of 1, got %+v", activeSet.Constraints)
	}
	if activeSet.Constraints.MaxToolsExposed <= 0 {
		t.Fatalf("expected max tools exposed constraint, got %+v", activeSet.Constraints)
	}
}

func TestActiveToolSetTracksLifecycleProvenanceAndExpiry(t *testing.T) {
	reg := NewRegistry()
	toolEntry := validTestTool("navi.repo.search")
	toolEntry.SchemaVersion = "2.0.0"
	if err := reg.Register(toolEntry); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	loadedAt := time.Date(2026, time.April, 17, 12, 0, 0, 0, time.UTC)
	expiresAt := loadedAt.Add(30 * time.Minute)
	broker := NewToolBrokerFromRegistry(reg)
	set := broker.BuildActiveToolSet(
		BrokerInput{
			UserInput:       "search the repo",
			SessionMode:     DiscoverySessionModeCoder,
			Environment:     "development",
			UserAuthority:   ToolAuthorityOwner,
			ActiveToolSetID: "ats-prev",
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			Intent:          BrokerIntentCodingQuery,
			SelectedToolIDs: []string{"navi.repo.search"},
			BrokerReason:    "coder search set",
		},
		ActiveToolSetSpec{
			Scope:       ActiveToolSetScopeProviderCall,
			ChatID:      "sess-life",
			WorkflowID:  "run-life",
			Mode:        "coder",
			Environment: "development",
			ToolIDs:     []string{"navi.repo.search"},
			LoadedAt:    loadedAt,
			ExpiresAt:   &expiresAt,
			LoadReason:  "provider_call_surface",
		},
	)

	if set.LoadedAt != loadedAt {
		t.Fatalf("expected loaded_at provenance, got %+v", set)
	}
	if set.ExpiresAt == nil || !set.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at on active set, got %+v", set)
	}
	if len(set.Tools) != 1 || set.Tools[0].ExpiresAt == nil || !set.Tools[0].ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected tool-level expiry to follow the active set, got %+v", set.Tools)
	}
	if set.Provenance.RegistrySnapshotID != reg.Snapshot().ID {
		t.Fatalf("expected registry snapshot provenance, got %+v", set.Provenance)
	}
	if set.Provenance.PreviousActiveSetID != "ats-prev" {
		t.Fatalf("expected previous active set linkage, got %+v", set.Provenance)
	}
	if set.IsExpiredAt(loadedAt.Add(29 * time.Minute)) {
		t.Fatalf("expected active set to remain fresh before expiry, got %+v", set)
	}
	if !set.ContainsToolVersionAt("navi.repo.search", "2.0.0", loadedAt.Add(29*time.Minute)) {
		t.Fatalf("expected fresh active set to contain tool version, got %+v", set.Tools)
	}
	if !set.IsExpiredAt(expiresAt) {
		t.Fatalf("expected active set to expire at its deadline, got %+v", set)
	}
	if set.ContainsToolVersionAt("navi.repo.search", "2.0.0", expiresAt) {
		t.Fatalf("expected expired active set to fail closed for tool membership, got %+v", set.Tools)
	}
	if set.ContainsToolVersionAt("navi.repo.search", "3.0.0", loadedAt.Add(5*time.Minute)) {
		t.Fatalf("expected schema-version drift to fail closed, got %+v", set.Tools)
	}
}

func TestToolBrokerBuildActiveToolSetRecordsDroppedToolIDs(t *testing.T) {
	reg := NewRegistry()
	validTool := validTestTool("navi.repo.search")
	validTool.SchemaVersion = "1.0.0"
	if err := reg.Register(validTool); err != nil {
		t.Fatalf("register valid tool: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	set := broker.BuildActiveToolSet(
		BrokerInput{
			UserInput:     "search the repo",
			SessionMode:   DiscoverySessionModeCoder,
			Environment:   "development",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			Intent:          BrokerIntentCodingQuery,
			SelectedToolIDs: []string{"navi.repo.search", "navi.repo.missing"},
			BrokerReason:    "mixed active set",
		},
		ActiveToolSetSpec{
			Scope:       ActiveToolSetScopeProviderCall,
			ChatID:      "sess-dropped",
			Mode:        "coder",
			Environment: "development",
			LoadReason:  "provider_call_surface",
		},
	)

	if len(set.Tools) != 1 || set.Tools[0].ToolID != "navi.repo.search" {
		t.Fatalf("expected only valid tool to load, got %+v", set.Tools)
	}
	if !strings.Contains(strings.Join(set.Provenance.DroppedToolIDs, ","), "navi.repo.missing") {
		t.Fatalf("expected dropped tool id to be recorded, got %+v", set.Provenance)
	}
	if set.ContainsTool("navi.repo.missing") {
		t.Fatalf("expected missing tool to fail closed, got %+v", set.Tools)
	}
	if set.Provenance.RegistrySnapshotID == "" || set.Constraints.MaxToolsExposed <= 0 {
		t.Fatalf("expected provenance and constraints to remain intact, got set=%+v", set)
	}
}
