package schema

// CommandType enumerates the primitive command types used by NAVI.
// These align with the conceptual design's command set and are intended to be
// stable over time.
type CommandType string

const (
	CommandTypeQuery    CommandType = "query"
	CommandTypeCreate   CommandType = "create"
	CommandTypeUpdate   CommandType = "update"
	CommandTypeDelete   CommandType = "delete"
	CommandTypeInvoke   CommandType = "invoke"
	CommandTypeSend     CommandType = "send"
	CommandTypeAcquire  CommandType = "acquire"
	CommandTypeSchedule CommandType = "schedule"
	CommandTypeDelegate CommandType = "delegate"
	CommandTypeCompose  CommandType = "compose"
)

// IdempotencyExpectation describes whether a command is safe to retry (design: Idempotency Expectations table).
type IdempotencyExpectation string

const (
	IdempotencyIdempotent     IdempotencyExpectation = "idempotent"       // repeated calls return same result
	IdempotencyNotIdempotent  IdempotencyExpectation = "not_idempotent"   // may produce duplicates; guard with dedup key
	IdempotencyVersionAware   IdempotencyExpectation = "version_aware"    // concurrent updates must detect conflicts
	IdempotencyDependsOnSkill IdempotencyExpectation = "depends_on_skill" // declared by the Skill contract
)

// Idempotency returns the design idempotency expectation for the command type.
func (c CommandType) Idempotency() IdempotencyExpectation {
	switch c {
	case CommandTypeQuery, CommandTypeDelete:
		return IdempotencyIdempotent
	case CommandTypeUpdate:
		return IdempotencyVersionAware
	case CommandTypeCreate, CommandTypeSend, CommandTypeSchedule:
		return IdempotencyNotIdempotent
	case CommandTypeInvoke, CommandTypeDelegate, CommandTypeCompose:
		return IdempotencyDependsOnSkill
	case CommandTypeAcquire:
		return IdempotencyIdempotent // only when target is versioned/content-addressed
	default:
		return IdempotencyNotIdempotent
	}
}

// FailureClass classifies failures according to their origin and recovery path.
// This mirrors the Failure Model taxonomy in the conceptual design.
type FailureClass string

const (
	FailureClassValidationRejection     FailureClass = "validation_rejection"
	FailureClassPermissionDenial        FailureClass = "permission_denial"
	FailureClassPolicyBlocked           FailureClass = "policy_blocked"
	FailureClassApprovalPending         FailureClass = "approval_pending"
	FailureClassVersionConflict         FailureClass = "version_conflict"
	FailureClassRendererMissing         FailureClass = "renderer_missing"
	FailureClassPatchApplyFailed        FailureClass = "patch_apply_failed"
	FailureClassSchemaInvalid           FailureClass = "schema_invalid"
	FailureClassSandboxBlocked          FailureClass = "sandbox_blocked"
	FailureClassStorageFailure          FailureClass = "storage_failure"
	FailureClassExportFailure           FailureClass = "export_failure"
	FailureClassSyncFailure             FailureClass = "sync_failure"
	FailureClassUnknown                 FailureClass = "unknown"
	FailureClassConnectorUnavailable    FailureClass = "connector_unavailable"
	FailureClassExecutionFailure        FailureClass = "execution_failure"
	FailureClassPartialExecution        FailureClass = "partial_execution"
	FailureClassTimeout                 FailureClass = "timeout"
	FailureClassPluginCrash             FailureClass = "plugin_crash"
	FailureClassContradictionStaleState FailureClass = "contradiction_stale_state"
)

// ReversibilityClass describes how a command or plugin operation can be
// compensated on failure. This maps to the Failure Model's reversibility
// classes and is intended for plugin/connector metadata.
type ReversibilityClass string

const (
	ReversibilityInternal     ReversibilityClass = "reversible_internal"
	ReversibilityCompensable  ReversibilityClass = "compensable_external"
	ReversibilityIrreversible ReversibilityClass = "irreversible"
)

// ComposeFailureMode defines how a failed Compose command is handled.
type ComposeFailureMode string

const (
	ComposeFailureRollback       ComposeFailureMode = "rollback"        // attempt to revert completed steps
	ComposeFailureCompensation   ComposeFailureMode = "compensation"    // run compensating commands for external effects
	ComposeFailureSurfacePartial ComposeFailureMode = "surface_partial" // report partial success and stop
)

// DegradationVisibility is the user-visible handling of a failure (design: Silent Retry / Advisory / Blocking / Deferred Recovery).
type DegradationVisibility string

const (
	DegradationSilentRetry      DegradationVisibility = "silent_retry"      // retry without surfacing to user
	DegradationAdvisory         DegradationVisibility = "advisory"          // surface as insight; do not block
	DegradationBlocking         DegradationVisibility = "blocking"          // block action; require user acknowledgment
	DegradationDeferredRecovery DegradationVisibility = "deferred_recovery" // create recovery proposal for later
)

// DegradationVisibilityFor selects the user-visible handling from failure class and impact.
// Used by the governor or loop to decide retry vs surface vs block vs proposal.
func DegradationVisibilityFor(failureClass FailureClass, userFacing, irreversible bool) DegradationVisibility {
	if irreversible {
		return DegradationBlocking
	}
	switch failureClass {
	case FailureClassValidationRejection, FailureClassPermissionDenial, FailureClassPluginCrash, FailureClassContradictionStaleState:
		return DegradationBlocking
	case FailureClassPartialExecution:
		if userFacing {
			return DegradationDeferredRecovery
		}
		return DegradationAdvisory
	case FailureClassConnectorUnavailable, FailureClassTimeout:
		if !userFacing {
			return DegradationSilentRetry
		}
		return DegradationAdvisory
	case FailureClassExecutionFailure:
		if userFacing {
			return DegradationBlocking
		}
		return DegradationAdvisory
	default:
		if userFacing {
			return DegradationBlocking
		}
		return DegradationAdvisory
	}
}
