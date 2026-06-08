package schema

import "fmt"

// kindForEventType is the authoritative mapping of every event type to its
// expected kind. ValidateEventKind enforces this at the bus boundary.
// Every EventType constant must appear here — omitting one creates a silent bypass.
var kindForEventType = map[EventType]EventKind{
	// Commands
	CmdAgentRegister:    EventKindCommand,
	CmdAgentRetire:      EventKindCommand,
	CmdDirectiveMessage: EventKindCommand,
	CmdTaskAssign:       EventKindCommand,

	// Facts
	FactCostRecorded:              EventKindFact,
	FactGovernorTripped:           EventKindFact,
	FactAgentBudgetExceeded:       EventKindFact,
	FactDirectiveReplied:          EventKindFact,
	FactNaviReplied:               EventKindFact,
	FactNaviReplyChunk:            EventKindFact,
	FactNaviExperienceModeChanged: EventKindFact,
	FactNaviExperienceSnapshot:    EventKindFact,
	FactNaviSkillLoaded:           EventKindFact,
	FactNaviSkillError:            EventKindFact,
	FactNaviHeartbeatDone:         EventKindFact,
	FactNaviActionRequested:       EventKindFact,
	FactReflectionQueued:          EventKindFact,
	FactSubconsciousInterruption:  EventKindFact,

	// Run lifecycle facts
	FactRunStarted:             EventKindFact,
	FactRunPhaseChanged:        EventKindFact,
	FactInferenceDecisionTrace: EventKindFact,
	FactRunPaused:              EventKindFact,
	FactRunResumed:       EventKindFact,
	FactRunCompleted:     EventKindFact,
	FactRunFailed:        EventKindFact,
	FactRunCancelled:     EventKindFact,
	FactInterruptRaised:  EventKindFact,
	FactInterruptApplied: EventKindFact,

	// Tool call lifecycle facts
	FactToolCallStarted:   EventKindFact,
	FactToolCallProgress:  EventKindFact,
	FactToolCallCompleted: EventKindFact,
	FactToolCallFailed:    EventKindFact,

	// Message intake facts
	FactMessageReceived:                 EventKindFact,
	FactMessageClassified:               EventKindFact,
	FactMessageMerged:                   EventKindFact,
	FactMessageDeferred:                 EventKindFact,
	FactMessageSuperseded:               EventKindFact,
	FactAssistantTokenDelta:             EventKindFact,
	FactAssistantMessagePartial:         EventKindFact,
	FactAssistantMessageCompleted:       EventKindFact,
	FactContextRead:                     EventKindFact,
	FactGovernanceBlocked:               EventKindFact,
	FactProposalWaiting:                 EventKindFact,
	FactProposalResolved:                EventKindFact,
	FactDegradationNoted:                EventKindFact,
	FactRecoveryRequired:                EventKindFact,
	FactArtifactCreated:                 EventKindFact,
	FactArtifactUpdated:                 EventKindFact,
	FactArtifactArchived:                EventKindFact,
	FactArtifactRestored:                EventKindFact,
	FactArtifactBranched:                EventKindFact,
	FactArtifactDeleted:                 EventKindFact,
	FactArtifactVersionCommitted:        EventKindFact,
	FactArtifactVersionRestoreRequested: EventKindFact,
	FactArtifactLifecycleChanged:        EventKindFact,
	FactArtifactExecutionStarted:        EventKindFact,
	FactArtifactExecutionCompleted:      EventKindFact,
	FactArtifactExecutionFailed:         EventKindFact,
	FactArtifactPatchFailed:             EventKindFact,
	FactArtifactRendererResolved:        EventKindFact,
	FactArtifactRendererMissing:         EventKindFact,
	FactArtifactWorkspaceLoaded:         EventKindFact,
	FactArtifactWorkspaceLoadFailed:     EventKindFact,
	FactArtifactExportCompleted:         EventKindFact,
	FactArtifactSyncCompleted:           EventKindFact,
	FactArtifactSyncFailed:              EventKindFact,
	FactArtifactConflictDetected:        EventKindFact,
	FactArtifactMaterialized:            EventKindFact,
}

// ValidateEventKind returns an error if the event's Kind does not match the
// authoritative mapping for its Type. Unknown event types are also rejected.
func ValidateEventKind(e Event) error {
	expected, ok := kindForEventType[e.Type]
	if !ok {
		return fmt.Errorf("validate: unknown event type %q", e.Type)
	}
	if e.Kind != expected {
		return fmt.Errorf("validate: event type %q must have kind %q, got %q", e.Type, expected, e.Kind)
	}
	return nil
}
