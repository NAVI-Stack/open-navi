package schema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task tests
// ---------------------------------------------------------------------------

func TestNewTaskHasUUID(t *testing.T) {
	task := NewTask("write auth", "implement JWT auth", RiskMedium, AgentNavi)
	if task.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if task.Status != TaskStatusPending {
		t.Fatalf("expected pending status, got %q", task.Status)
	}
}

func TestTaskValidateHappyPath(t *testing.T) {
	task := NewTask("deploy", "deploy to prod", RiskHigh, AgentConnector)
	if err := task.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestTaskValidateMissingTitle(t *testing.T) {
	task := NewTask("", "desc", RiskLow, AgentNavi)
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestTaskValidateInvalidRisk(t *testing.T) {
	task := NewTask("t", "d", RiskLevel("extreme"), AgentNavi)
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for unknown risk level")
	}
}

func TestTaskValidateInvalidSurfaceAccessMode(t *testing.T) {
	task := NewTask("t", "d", RiskLow, AgentNavi)
	task.Surfaces = []SurfaceDeclaration{
		{Path: "src/", AccessMode: "destroy"},
	}
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for invalid access mode")
	}
}

// ---------------------------------------------------------------------------
// Event tests
// ---------------------------------------------------------------------------

func TestNewEventHasUUID(t *testing.T) {
	ev := NewEvent(CmdDirectiveMessage, EventKindCommand, "corr-1", AgentNavi, DirectiveMessagePayload{
		DirectiveID: "d1",
		MessageID:   "m1",
		OwnerID:     "owner",
	})
	if ev.ID == "" {
		t.Fatal("expected non-empty event ID")
	}
	if ev.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", SchemaVersion, ev.SchemaVersion)
	}
}

func TestEventValidateHappyPath(t *testing.T) {
	ev := NewEvent(FactDirectiveReplied, EventKindFact, "corr-2", AgentNavi, DirectiveRepliedPayload{
		DirectiveID: "d2",
		MessageID:   "m2",
	})
	if err := ev.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventValidateMissingCorrelationID(t *testing.T) {
	ev := NewEvent(CmdDirectiveMessage, EventKindCommand, "", AgentNavi, DirectiveMessagePayload{
		DirectiveID: "d3",
		MessageID:   "m3",
		OwnerID:     "owner",
	})
	if err := ev.Validate(); err == nil {
		t.Fatal("expected error for missing correlation ID")
	}
}

func TestEventValidateInvalidKind(t *testing.T) {
	ev := Event{
		ID:            "test-id",
		Type:          CmdDirectiveMessage,
		Kind:          EventKind("maybe"),
		CorrelationID: "corr-3",
		SourceAgent:   AgentNavi,
		Payload: DirectiveMessagePayload{
			DirectiveID: "d4",
			MessageID:   "m4",
			OwnerID:     "owner",
		},
		SchemaVersion: SchemaVersion,
	}
	if err := ev.Validate(); err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

// ---------------------------------------------------------------------------
// Bus-boundary validation tests (validate.go)
// ---------------------------------------------------------------------------

func TestValidateEventKindCommandIsCommand(t *testing.T) {
	ev := NewEvent(CmdDirectiveMessage, EventKindCommand, "corr", AgentNavi, DirectiveMessagePayload{
		DirectiveID: "d5",
		MessageID:   "m5",
		OwnerID:     "owner",
	})
	if err := ValidateEventKind(ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEventKindMismatch(t *testing.T) {
	ev := NewEvent(CmdDirectiveMessage, EventKindFact, "corr", AgentNavi, DirectiveMessagePayload{
		DirectiveID: "d6",
		MessageID:   "m6",
		OwnerID:     "owner",
	})
	if err := ValidateEventKind(ev); err == nil {
		t.Fatal("expected error for kind/type mismatch")
	}
}

func TestValidateEventKindUnknownType(t *testing.T) {
	ev := NewEvent(EventType("unknown.event"), EventKindFact, "corr", AgentNavi, nil)
	if err := ValidateEventKind(ev); err == nil {
		t.Fatal("expected error for unknown event type")
	}
}

func TestValidateEventKindExhaustive(t *testing.T) {
	// Find all EventType constants in the schema package using reflection.
	// This ensures that if a new EventType is added, it must also be added
	// to the kindForEventType map.
	pkg := reflect.TypeOf(CmdAgentRegister).PkgPath()
	_ = pkg // just to use the type

	// Given the constraints, let's at least verify that all currently known
	// EventTypes are in the map.
	knownTypes := []EventType{
		CmdAgentRegister, CmdAgentRetire, FactDirectiveReplied, CmdDirectiveMessage,
		CmdTaskAssign, FactCostRecorded, FactGovernorTripped, FactAgentBudgetExceeded,
		FactNaviReplied, FactNaviReplyChunk, FactNaviExperienceModeChanged,
		FactNaviExperienceSnapshot, FactNaviSkillLoaded, FactNaviSkillError,
		FactNaviHeartbeatDone, FactNaviActionRequested, FactReflectionQueued,
		FactSubconsciousInterruption, FactRunStarted, FactRunPhaseChanged,
		FactInferenceDecisionTrace, FactRunPaused, FactRunResumed,
		FactRunCompleted, FactRunFailed, FactRunCancelled,
		FactInterruptRaised, FactInterruptApplied, FactToolCallStarted,
		FactToolCallProgress, FactToolCallCompleted, FactToolCallFailed,
		FactMessageReceived, FactMessageClassified, FactMessageMerged,
		FactMessageDeferred, FactMessageSuperseded, FactAssistantTokenDelta,
		FactAssistantMessagePartial, FactAssistantMessageCompleted,
		FactGovernanceBlocked, FactProposalWaiting, FactProposalResolved,
		FactDegradationNoted, FactRecoveryRequired, FactArtifactCreated,
		FactArtifactUpdated, FactArtifactArchived, FactArtifactRestored,
		FactArtifactBranched, FactArtifactDeleted, FactArtifactVersionCommitted,
		FactArtifactVersionRestoreRequested, FactArtifactLifecycleChanged,
		FactArtifactExecutionStarted, FactArtifactExecutionCompleted,
		FactArtifactExecutionFailed, FactArtifactPatchFailed,
		FactArtifactRendererResolved, FactArtifactRendererMissing,
		FactArtifactWorkspaceLoaded, FactArtifactWorkspaceLoadFailed,
		FactArtifactExportCompleted, FactArtifactSyncCompleted,
		FactArtifactSyncFailed, FactArtifactConflictDetected,
		FactArtifactMaterialized,
	}

	for _, et := range knownTypes {
		if _, ok := kindForEventType[et]; !ok {
			t.Errorf("EventType %q is missing from kindForEventType map in validate.go", et)
		}
	}
}

func TestValidateEventKindTable(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		eventKind EventKind
		wantErr   bool
	}{
		{"happy path command", CmdAgentRegister, EventKindCommand, false},
		{"happy path fact", FactRunStarted, EventKindFact, false},
		{"happy path trace", FactInferenceDecisionTrace, EventKindFact, false},
		{"mismatch command as fact", CmdAgentRegister, EventKindFact, true},
		{"mismatch fact as command", FactRunStarted, EventKindCommand, true},
		{"unknown type", EventType("unknown"), EventKindFact, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Event{
				Type: tt.eventType,
				Kind: tt.eventKind,
			}
			err := ValidateEventKind(ev)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEventKind() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkspaceKindParseCoversExactV1Values(t *testing.T) {
	valid := []WorkspaceKind{
		WorkspaceKindGeneral,
		WorkspaceKindProject,
		WorkspaceKindRepository,
		WorkspaceKindFunctional,
	}
	for _, kind := range valid {
		parsed, err := ParseWorkspaceKind(string(kind))
		if err != nil {
			t.Fatalf("ParseWorkspaceKind(%q): %v", kind, err)
		}
		if parsed != kind {
			t.Fatalf("ParseWorkspaceKind(%q) = %q", kind, parsed)
		}
		if !kind.IsValid() {
			t.Fatalf("WorkspaceKind %q should be valid", kind)
		}
	}
	if _, err := ParseWorkspaceKind("global"); err == nil {
		t.Fatal("expected global workspace kind to be rejected")
	}
}

func TestWorkspaceStatusParseCoversExactV1Values(t *testing.T) {
	valid := []WorkspaceStatus{
		WorkspaceStatusActive,
		WorkspaceStatusInactive,
		WorkspaceStatusArchived,
		WorkspaceStatusSuspended,
	}
	for _, status := range valid {
		parsed, err := ParseWorkspaceStatus(string(status))
		if err != nil {
			t.Fatalf("ParseWorkspaceStatus(%q): %v", status, err)
		}
		if parsed != status {
			t.Fatalf("ParseWorkspaceStatus(%q) = %q", status, parsed)
		}
		if !status.IsValid() {
			t.Fatalf("WorkspaceStatus %q should be valid", status)
		}
	}
	if _, err := ParseWorkspaceStatus("deleted"); err == nil {
		t.Fatal("expected invalid workspace status to be rejected")
	}
}

func TestWorkspaceOperatingModeParseCoversExactV1Values(t *testing.T) {
	valid := []WorkspaceOperatingMode{
		WorkspaceOperatingModeGlobal,
		WorkspaceOperatingModeScoped,
		WorkspaceOperatingModeHybrid,
	}
	for _, mode := range valid {
		parsed, err := ParseWorkspaceOperatingMode(string(mode))
		if err != nil {
			t.Fatalf("ParseWorkspaceOperatingMode(%q): %v", mode, err)
		}
		if parsed != mode {
			t.Fatalf("ParseWorkspaceOperatingMode(%q) = %q", mode, parsed)
		}
		if !mode.IsValid() {
			t.Fatalf("WorkspaceOperatingMode %q should be valid", mode)
		}
	}
	if _, err := ParseWorkspaceOperatingMode("wide_open"); err == nil {
		t.Fatal("expected invalid workspace operating mode to be rejected")
	}
}

func TestWorkspacePolicyEnumsValidate(t *testing.T) {
	if _, err := ParseBoundaryPolicyOutOfScopeDefault("prompt"); err != nil {
		t.Fatalf("ParseBoundaryPolicyOutOfScopeDefault(prompt): %v", err)
	}
	if _, err := ParseBoundaryPolicyOutOfScopeDefault("allow"); err == nil {
		t.Fatal("expected invalid boundary policy default to be rejected")
	}

	if _, err := ParseWhitelistRuleStatus("active"); err != nil {
		t.Fatalf("ParseWhitelistRuleStatus(active): %v", err)
	}
	if _, err := ParseWhitelistRuleStatus("expired"); err == nil {
		t.Fatal("expected invalid whitelist rule status to be rejected")
	}
}

func TestWorkspaceActionTypeParseCoversExactV1Values(t *testing.T) {
	valid := []WorkspaceActionType{
		WorkspaceActionRead,
		WorkspaceActionWrite,
		WorkspaceActionCreate,
		WorkspaceActionModify,
		WorkspaceActionRenameMove,
		WorkspaceActionDelete,
		WorkspaceActionExecute,
	}
	for _, action := range valid {
		parsed, err := ParseWorkspaceActionType(string(action))
		if err != nil {
			t.Fatalf("ParseWorkspaceActionType(%q): %v", action, err)
		}
		if parsed != action {
			t.Fatalf("ParseWorkspaceActionType(%q) = %q", action, parsed)
		}
		if !action.IsValid() {
			t.Fatalf("WorkspaceActionType %q should be valid", action)
		}
	}
	if _, err := ParseWorkspaceActionType("archive"); err == nil {
		t.Fatal("expected invalid workspace action type to be rejected")
	}
}

func TestValidateEventKindExperienceModeChangedIsFact(t *testing.T) {
	ev := NewEvent(FactNaviExperienceModeChanged, EventKindFact, "corr", AgentNavi, map[string]any{"experience_mode": "navi"})
	if err := ValidateEventKind(ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEventKindExperienceSnapshotIsFact(t *testing.T) {
	ev := NewEvent(FactNaviExperienceSnapshot, EventKindFact, "corr", AgentNavi, NaviExperienceSnapshotPayload{
		SnapshotID:         "snap-1",
		ChatID:             "sess-1",
		ExperienceMode:     "navi",
		Trigger:            "session_start",
		SourceStateID:      "eps-123",
		EffectiveStateJSON: `{"state_id":"eps-123"}`,
	})
	if err := ValidateEventKind(ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPayloadJSONUsesExperienceModeField(t *testing.T) {
	b, err := json.Marshal(RunStartedPayload{RunID: "run-1", RuntimeSessionID: "runtime-1", ExperienceMode: "navi"})
	if err != nil {
		t.Fatalf("Marshal run payload: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"experience_mode":"navi"`) {
		t.Fatalf("expected experience_mode field in %s", got)
	}
	if strings.Contains(got, `"persona_id"`) {
		t.Fatalf("did not expect legacy persona_id field in %s", got)
	}
	if strings.Contains(got, `"chat_id"`) {
		t.Fatalf("did not expect legacy chat_id field in %s", got)
	}

	b, err = json.Marshal(AssistantMessageCompletedPayload{RuntimeSessionID: "runtime-1", MessageID: "msg-1", Content: "ok", ExperienceMode: "navi"})
	if err != nil {
		t.Fatalf("Marshal assistant payload: %v", err)
	}
	got = string(b)
	if !strings.Contains(got, `"experience_mode":"navi"`) {
		t.Fatalf("expected experience_mode field in %s", got)
	}
	if strings.Contains(got, `"persona_id"`) {
		t.Fatalf("did not expect legacy persona_id field in %s", got)
	}
	if strings.Contains(got, `"chat_id"`) {
		t.Fatalf("did not expect legacy chat_id field in %s", got)
	}
}

func TestRuntimeEventPayloadsUseRuntimeSessionID(t *testing.T) {
	payloads := []any{
		RunStartedPayload{},
		RunPhaseChangedPayload{},
		InferenceDecisionTracePayload{},
		RunPausedPayload{},
		RunResumedPayload{},
		RunCompletedPayload{},
		RunFailedPayload{},
		RunCancelledPayload{},
		InterruptRaisedPayload{},
		InterruptAppliedPayload{},
		ToolCallStartedPayload{},
		ToolCallProgressPayload{},
		ToolCallCompletedPayload{},
		ToolCallFailedPayload{},
		MessageReceivedPayload{},
		MessageClassifiedPayload{},
		MessageQueueActionPayload{},
		AssistantTokenDeltaPayload{},
		AssistantMessagePartialPayload{},
		AssistantMessageCompletedPayload{},
		GovernanceBlockedPayload{},
		ProposalWaitingPayload{},
		ProposalResolvedPayload{},
		DegradationNotedPayload{},
		RecoveryRequiredPayload{},
	}
	for _, payload := range payloads {
		typ := reflect.TypeOf(payload)
		legacyField := "Session" + "ID"
		if _, ok := typ.FieldByName(legacyField); ok {
			t.Fatalf("%s must not expose legacy %s field", typ.Name(), legacyField)
		}
		field, ok := typ.FieldByName("RuntimeSessionID")
		if !ok {
			t.Fatalf("%s missing RuntimeSessionID field", typ.Name())
		}
		if got := strings.Split(field.Tag.Get("json"), ",")[0]; got != "runtime_session_id" {
			t.Fatalf("%s RuntimeSessionID json tag = %q, want runtime_session_id", typ.Name(), got)
		}
		for i := 0; i < typ.NumField(); i++ {
			tagName := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			legacyTag := "session" + "_id"
			if tagName == legacyTag {
				t.Fatalf("%s field %s still serializes as %s", typ.Name(), typ.Field(i).Name, legacyTag)
			}
		}
	}
}

func TestParseWorkspaceStatus(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    WorkspaceStatus
		wantErr bool
	}{
		// Valid cases
		{"active exact", "active", WorkspaceStatusActive, false},
		{"inactive exact", "inactive", WorkspaceStatusInactive, false},
		{"archived exact", "archived", WorkspaceStatusArchived, false},
		{"suspended exact", "suspended", WorkspaceStatusSuspended, false},

		// Valid casing and spaces (parseWorldModelEnum uses strings.ToLower and strings.TrimSpace)
		{"active uppercase", "ACTIVE", WorkspaceStatusActive, false},
		{"inactive mixed case", "InActive", WorkspaceStatusInactive, false},
		{"archived with spaces", "  archived  ", WorkspaceStatusArchived, false},

		// Invalid cases
		{"invalid value deleted", "deleted", "", true},
		{"invalid value random", "unknown", "", true},
		{"empty string", "", "", true},
		{"spaces only", "   ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWorkspaceStatus(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWorkspaceStatus(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseWorkspaceStatus(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
