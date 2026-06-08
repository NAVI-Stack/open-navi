package schema

import (
	"fmt"
	"strings"
	"time"
)

// EntityProvenance holds canonical provenance for world-model entities (source, derivation, mutation history).
type EntityProvenance struct {
	Source             string    `json:"source"`
	Timestamp          time.Time `json:"timestamp"`
	Confidence         float64   `json:"confidence"`
	DerivationChain    []string  `json:"derivation_chain,omitempty"`
	ReinforcementCount int       `json:"reinforcement_count"`
	MutationHistory    []string  `json:"mutation_history,omitempty"`
	ProposalID         string    `json:"proposal_id,omitempty"`
}

// WorkspaceKind classifies the intent and shape of a workspace.
type WorkspaceKind string

const (
	WorkspaceKindGeneral    WorkspaceKind = "general"
	WorkspaceKindProject    WorkspaceKind = "project"
	WorkspaceKindRepository WorkspaceKind = "repository"
	WorkspaceKindFunctional WorkspaceKind = "functional"
)

var workspaceKindValues = map[WorkspaceKind]struct{}{
	WorkspaceKindGeneral:    {},
	WorkspaceKindProject:    {},
	WorkspaceKindRepository: {},
	WorkspaceKindFunctional: {},
}

// WorkspaceStatus tracks the lifecycle state of a workspace.
type WorkspaceStatus string

const (
	WorkspaceStatusActive    WorkspaceStatus = "active"
	WorkspaceStatusInactive  WorkspaceStatus = "inactive"
	WorkspaceStatusArchived  WorkspaceStatus = "archived"
	WorkspaceStatusSuspended WorkspaceStatus = "suspended"
)

var workspaceStatusValues = map[WorkspaceStatus]struct{}{
	WorkspaceStatusActive:    {},
	WorkspaceStatusInactive:  {},
	WorkspaceStatusArchived:  {},
	WorkspaceStatusSuspended: {},
}

// WorkspaceOperatingMode defines the system-level workspace posture.
type WorkspaceOperatingMode string

const (
	WorkspaceOperatingModeGlobal WorkspaceOperatingMode = "global"
	WorkspaceOperatingModeScoped WorkspaceOperatingMode = "scoped"
	WorkspaceOperatingModeHybrid WorkspaceOperatingMode = "hybrid"
)

var workspaceOperatingModeValues = map[WorkspaceOperatingMode]struct{}{
	WorkspaceOperatingModeGlobal: {},
	WorkspaceOperatingModeScoped: {},
	WorkspaceOperatingModeHybrid: {},
}

// BoundaryPolicyOutOfScopeDefault defines the default behavior for out-of-scope actions.
type BoundaryPolicyOutOfScopeDefault string

const (
	BoundaryPolicyOutOfScopePrompt BoundaryPolicyOutOfScopeDefault = "prompt"
	BoundaryPolicyOutOfScopeDeny   BoundaryPolicyOutOfScopeDefault = "deny"
)

var boundaryPolicyOutOfScopeValues = map[BoundaryPolicyOutOfScopeDefault]struct{}{
	BoundaryPolicyOutOfScopePrompt: {},
	BoundaryPolicyOutOfScopeDeny:   {},
}

// WhitelistRuleStatus tracks the lifecycle state of a workspace whitelist rule.
type WhitelistRuleStatus string

const (
	WhitelistRuleStatusActive  WhitelistRuleStatus = "active"
	WhitelistRuleStatusRevoked WhitelistRuleStatus = "revoked"
)

var whitelistRuleStatusValues = map[WhitelistRuleStatus]struct{}{
	WhitelistRuleStatusActive:  {},
	WhitelistRuleStatusRevoked: {},
}

// WorkspaceActionType is the exact deny-mask surface for workspace-local actions.
type WorkspaceActionType string

const (
	WorkspaceActionRead       WorkspaceActionType = "read"
	WorkspaceActionWrite      WorkspaceActionType = "write"
	WorkspaceActionCreate     WorkspaceActionType = "create"
	WorkspaceActionModify     WorkspaceActionType = "modify"
	WorkspaceActionRenameMove WorkspaceActionType = "rename_move"
	WorkspaceActionDelete     WorkspaceActionType = "delete"
	WorkspaceActionExecute    WorkspaceActionType = "execute"
)

var workspaceActionTypeValues = map[WorkspaceActionType]struct{}{
	WorkspaceActionRead:       {},
	WorkspaceActionWrite:      {},
	WorkspaceActionCreate:     {},
	WorkspaceActionModify:     {},
	WorkspaceActionRenameMove: {},
	WorkspaceActionDelete:     {},
	WorkspaceActionExecute:    {},
}

// AllowedActions is a workspace-local deny mask over in-scope resources.
// It does not grant permissions; it narrows what is permitted after governance passes.
type AllowedActions struct {
	Read       bool `json:"read"`
	Write      bool `json:"write"`
	Create     bool `json:"create"`
	Modify     bool `json:"modify"`
	RenameMove bool `json:"rename_move"`
	Delete     bool `json:"delete"`
	Execute    bool `json:"execute"`
}

// BoundaryPolicy defines the default behavior for out-of-scope actions.
type BoundaryPolicy struct {
	OutOfScopeDefault BoundaryPolicyOutOfScopeDefault `json:"out_of_scope_default"`
}

// WhitelistRule provides durable approval for specific out-of-scope actions.
type WhitelistRule struct {
	RuleID      string              `json:"rule_id"`
	Scope       string              `json:"scope"`
	ActionTypes []string            `json:"action_types"`
	Status      WhitelistRuleStatus `json:"status"`
	CreatedAt   time.Time           `json:"created_at"`
	RevokedAt   *time.Time          `json:"revoked_at,omitempty"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	CreatedBy   string              `json:"created_by"`
}

// Workspace is a durable execution boundary plus a context boundary.
type Workspace struct {
	ID             string          `json:"workspace_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	Kind           WorkspaceKind   `json:"workspace_kind"`
	Status         WorkspaceStatus `json:"status"`
	LocalRoots     []string        `json:"local_roots"`
	RepoRoots      []string        `json:"repo_roots"`
	ProtectedPaths []string        `json:"protected_paths"`
	AllowedActions AllowedActions  `json:"allowed_actions"`
	BoundaryPolicy BoundaryPolicy  `json:"boundary_policy"`
	AuditEnabled   bool            `json:"audit_enabled"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	CreatedBy      string          `json:"created_by"`

	// Optional fields
	Tags             []string        `json:"tags,omitempty"`
	RelatedProjectID string          `json:"related_project_id,omitempty"`
	Notes            string          `json:"notes,omitempty"`
	WhitelistRules   []WhitelistRule `json:"whitelist_rules,omitempty"`
	Metadata         string          `json:"metadata,omitempty"` // JSON-encoded
}

func normalizeWorldModelEnum(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseWorldModelEnum[T ~string](raw string, allowed map[T]struct{}, name string) (T, error) {
	normalized := T(normalizeWorldModelEnum(raw))
	if _, ok := allowed[normalized]; !ok {
		return "", fmt.Errorf("invalid %s %q", name, raw)
	}
	return normalized, nil
}

func (v WorkspaceKind) IsValid() bool {
	_, ok := workspaceKindValues[v]
	return ok
}

func ParseWorkspaceKind(raw string) (WorkspaceKind, error) {
	return parseWorldModelEnum(raw, workspaceKindValues, "workspace_kind")
}

func (v WorkspaceStatus) IsValid() bool {
	_, ok := workspaceStatusValues[v]
	return ok
}

func ParseWorkspaceStatus(raw string) (WorkspaceStatus, error) {
	return parseWorldModelEnum(raw, workspaceStatusValues, "workspace_status")
}

func (v WorkspaceOperatingMode) IsValid() bool {
	_, ok := workspaceOperatingModeValues[v]
	return ok
}

func ParseWorkspaceOperatingMode(raw string) (WorkspaceOperatingMode, error) {
	return parseWorldModelEnum(raw, workspaceOperatingModeValues, "workspace_operating_mode")
}

func (v BoundaryPolicyOutOfScopeDefault) IsValid() bool {
	_, ok := boundaryPolicyOutOfScopeValues[v]
	return ok
}

func ParseBoundaryPolicyOutOfScopeDefault(raw string) (BoundaryPolicyOutOfScopeDefault, error) {
	return parseWorldModelEnum(raw, boundaryPolicyOutOfScopeValues, "boundary_policy.out_of_scope_default")
}

func (v WhitelistRuleStatus) IsValid() bool {
	_, ok := whitelistRuleStatusValues[v]
	return ok
}

func ParseWhitelistRuleStatus(raw string) (WhitelistRuleStatus, error) {
	return parseWorldModelEnum(raw, whitelistRuleStatusValues, "whitelist_rule.status")
}

func (v WorkspaceActionType) IsValid() bool {
	_, ok := workspaceActionTypeValues[v]
	return ok
}

func ParseWorkspaceActionType(raw string) (WorkspaceActionType, error) {
	return parseWorldModelEnum(raw, workspaceActionTypeValues, "workspace_action_type")
}

func WorkspaceActionForCommandType(cmdType CommandType) WorkspaceActionType {
	switch cmdType {
	case CommandTypeQuery:
		return WorkspaceActionRead
	case CommandTypeCreate:
		return WorkspaceActionCreate
	case CommandTypeUpdate:
		return WorkspaceActionModify
	case CommandTypeDelete:
		return WorkspaceActionDelete
	default:
		return WorkspaceActionExecute
	}
}

func (a AllowedActions) Permits(actionType WorkspaceActionType) bool {
	switch actionType {
	case WorkspaceActionRead:
		return a.Read
	case WorkspaceActionWrite:
		return a.Write
	case WorkspaceActionCreate:
		return a.Create
	case WorkspaceActionModify:
		return a.Modify
	case WorkspaceActionRenameMove:
		return a.RenameMove
	case WorkspaceActionDelete:
		return a.Delete
	case WorkspaceActionExecute:
		return a.Execute
	default:
		return false
	}
}

// ApprovalOutcome tracks the result of a boundary crossing or governance prompt.
type ApprovalOutcome string

const (
	ApprovalOutcomeNA                ApprovalOutcome = "na"
	ApprovalOutcomeDenied            ApprovalOutcome = "denied"
	ApprovalOutcomeAllowOnce         ApprovalOutcome = "allow_once"
	ApprovalOutcomeAlwaysAllow       ApprovalOutcome = "always_allow"
	ApprovalOutcomeSwitchedWorkspace ApprovalOutcome = "switched_workspace"
)

// StateKind is the design's four state kinds. Owner-set and explicit require user confirmation to modify.
type StateKind string

const (
	StateKindExplicit StateKind = "explicit"  // directly set by user; authoritative
	StateKindOwnerSet StateKind = "owner_set" // subset of explicit: owner-authored; requires confirmation to modify
	StateKindInferred StateKind = "inferred"  // learned from behavior; reflection may update within scope
	StateKindDerived  StateKind = "derived"   // assembled at query time; never stored
)

// LifecycleAction is the data lifecycle operation (design: Archive, Forget, Supersede, Tombstone).
type LifecycleAction string

const (
	LifecycleArchive   LifecycleAction = "archive"   // default for Delete; soft-delete / hide
	LifecycleForget    LifecycleAction = "forget"    // Memory: deletion path
	LifecycleSupersede LifecycleAction = "supersede" // Knowledge: replace with newer fact
	LifecycleTombstone LifecycleAction = "tombstone" // hard-delete record
	LifecycleDeprecate LifecycleAction = "deprecate" // Knowledge: soft-delete without replacement
)

// Contact represents a node in NAVI's contact graph (people and entities).
// It starts minimal and can grow via the Metadata field.
type Contact struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`        // "person", "organization", "service", "navi"
	OwnerType  string    `json:"owner_type"`  // "navi" (NAVI's own) or "owner" (user's imported)
	TrustLevel string    `json:"trust_level"` // "unknown", "low", "medium", "high", "verified"
	Metadata   string    `json:"metadata"`    // JSON-encoded attributes for extensibility
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	ContactOwnerTypeNavi  = "navi"
	ContactOwnerTypeOwner = "owner"

	ContactKindPerson       = "person"
	ContactKindOrganization = "organization"
	ContactKindService      = "service"
	ContactKindNavi         = "navi" // another NAVI agent (NAVI-to-NAVI identity)

	ContactTrustLevelUnknown  = "unknown"
	ContactTrustLevelLow      = "low"
	ContactTrustLevelMedium   = "medium"
	ContactTrustLevelHigh     = "high"
	ContactTrustLevelVerified = "verified"
)

// WorldModelEvent represents scheduling and temporal awareness (e.g. calendar).
// Distinct from bus events; used for calendar, reminders, and time-based context.
type WorldModelEvent struct {
	ID        string     `json:"id"`
	OwnerID   string     `json:"owner_id"`
	Title     string     `json:"title"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Kind      string     `json:"kind"`     // e.g. "appointment", "reminder", "deadline"
	Source    string     `json:"source"`   // provenance: "user_input", "inference", etc.
	Metadata  string     `json:"metadata"` // JSON for extensibility
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Memory represents a significant experience or moment with emotional weight.
// Durable by default; deletion path (Forget) is required and deferred to lifecycle model.
type Memory struct {
	ID           string    `json:"id"`
	Scope        string    `json:"scope"` // "owner", "session"
	ScopeID      string    `json:"scope_id"`
	Summary      string    `json:"summary"`
	Details      string    `json:"details,omitempty"`
	Keywords     []string  `json:"keywords,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Embedding    []float64 `json:"embedding,omitempty"`
	Significance string    `json:"significance,omitempty"` // optional weight or category
	Source       string    `json:"source"`                 // provenance
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ConfigurationEntry represents explicit or inferred configuration in the world model.
type ConfigurationEntry struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`    // "owner", "session", "global", or domain-specific
	ScopeID   string    `json:"scope_id"` // identifier within scope (e.g. owner id, session id)
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Source    string    `json:"source"` // "explicit", "owner_set", "inferred"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Priority represents stated or observed goals and intentions.
type Priority struct {
	ID          string    `json:"id"`
	Scope       string    `json:"scope"`    // "owner", "directive", "session"
	ScopeID     string    `json:"scope_id"` // identifier within scope
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Source      string    `json:"source"` // "owner_set", "observed"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProposalPriority indicates whether a proposal is blocking an active action
// or queued for later review.
type ProposalPriority string

const (
	ProposalPriorityBlocking ProposalPriority = "blocking"
	ProposalPriorityQueued   ProposalPriority = "queued"
)

// ProposalStatus tracks lifecycle state of a proposal in the queue.
type ProposalStatus string

const (
	ProposalStatusPending    ProposalStatus = "pending"
	ProposalStatusApproved   ProposalStatus = "approved"
	ProposalStatusDeclined   ProposalStatus = "declined"
	ProposalStatusExpired    ProposalStatus = "expired"
	ProposalStatusSuperseded ProposalStatus = "superseded"
)

// ResolutionType indicates how a proposal was resolved.
type ResolutionType string

const (
	ResolutionTypeNA             ResolutionType = "na"
	ResolutionTypeApprovedOnce   ResolutionType = "approved_once"
	ResolutionTypeApprovedAlways ResolutionType = "approved_always"
	ResolutionTypeSwitchedWorkspace ResolutionType = "switched_workspace"
	ResolutionTypeDenied         ResolutionType = "denied"
)

// Proposal models a queued action that requires owner or system resolution
// before execution proceeds.
type Proposal struct {
	ProposalID       string           `json:"proposal_id"`
	SourceProcess    string           `json:"source_process"`
	SourceTrigger    string           `json:"source_trigger"`
	BoundaryKey      string           `json:"boundary_key,omitempty"`
	ProposedAction   string           `json:"proposed_action"`
	AffectedEntities string           `json:"affected_entities"`
	Rationale        string           `json:"rationale"`
	Priority         ProposalPriority `json:"priority"`
	Status           ProposalStatus   `json:"status"`
	CreatedAt        time.Time        `json:"created_at"`
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
	ResolvedAt       *time.Time       `json:"resolved_at,omitempty"`
	ResolvedBy       string           `json:"resolved_by,omitempty"`
	ResolutionType   ResolutionType   `json:"resolution_type,omitempty"`
	ResolutionNote   string           `json:"resolution_note,omitempty"`
}

// ExecutionOutcomeOutcome enumerates the high-level result of a command attempt.
type ExecutionOutcomeOutcome string

const (
	ExecutionOutcomeSucceeded            ExecutionOutcomeOutcome = "succeeded"
	ExecutionOutcomeFailed               ExecutionOutcomeOutcome = "failed"
	ExecutionOutcomePartiallySucceeded   ExecutionOutcomeOutcome = "partially_succeeded"
	ExecutionOutcomeCancelled            ExecutionOutcomeOutcome = "cancelled"
	ExecutionOutcomeTimedOut             ExecutionOutcomeOutcome = "timed_out"
	ExecutionOutcomeRejectedPreExecution ExecutionOutcomeOutcome = "rejected_pre_execution"
)

// CompensationStatus tracks the state of any compensation work for side effects.
type CompensationStatus string

const (
	CompensationStatusNotRequired CompensationStatus = "not_required"
	CompensationStatusPending     CompensationStatus = "pending"
	CompensationStatusCompleted   CompensationStatus = "completed"
	CompensationStatusFailed      CompensationStatus = "failed"
	CompensationStatusNotPossible CompensationStatus = "not_possible"
)

// RecoveryStatus tracks whether a partially succeeded command still has open recovery work.
type RecoveryStatus string

const (
	RecoveryStatusNotRequired RecoveryStatus = "not_required"
	RecoveryStatusOpen        RecoveryStatus = "open"
	RecoveryStatusResolved    RecoveryStatus = "resolved"
)

// UserModel is a composite projection assembled at query time from Contacts (owner),
// Knowledge (facts), Memories, Configuration, and Priorities. Never stored as a single entity.
type UserModel struct {
	OwnerContact  *Contact             `json:"owner_contact,omitempty"`
	Facts         []Fact               `json:"facts,omitempty"`
	Memories      []Memory             `json:"memories,omitempty"`
	Configuration []ConfigurationEntry `json:"configuration,omitempty"`
	Priorities    []Priority           `json:"priorities,omitempty"`
}

// Fact is a single fact from the Knowledge store (facts table). Used in UserModel.
// Deprecated facts are soft-deleted (excluded from normal reads); deprecation without replacement.
type Fact struct {
	ID         string    `json:"id"`
	Scope      string    `json:"scope"`
	ScopeID    string    `json:"scope_id"`
	Category   string    `json:"category"`
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Keywords   []string  `json:"keywords,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Embedding  []float64 `json:"embedding,omitempty"`
	Source     string    `json:"source"`
	Deprecated bool      `json:"deprecated,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// KnowledgeLink connects two knowledge nodes that were determined to be related.
// Links are stored canonically, but exposed as adjacent nodes from the caller's perspective.
type KnowledgeLink struct {
	EntityType      string    `json:"entity_type"`
	EntityID        string    `json:"entity_id"`
	RelatedType     string    `json:"related_type"`
	RelatedID       string    `json:"related_id"`
	Similarity      float64   `json:"similarity"`
	Source          string    `json:"source"`
	RelationshipTag string    `json:"relationship_tag,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// KnowledgeHit is a semantic-search result spanning facts and memories.
type KnowledgeHit struct {
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Score      float64         `json:"score"`
	Summary    string          `json:"summary"`
	Scope      string          `json:"scope"`
	ScopeID    string          `json:"scope_id"`
	Keywords   []string        `json:"keywords,omitempty"`
	Tags       []string        `json:"tags,omitempty"`
	Memory     *Memory         `json:"memory,omitempty"`
	Fact       *Fact           `json:"fact,omitempty"`
	Links      []KnowledgeLink `json:"links,omitempty"`
}

// ExecutionOutcome captures a single command attempt's outcome, mapped to
// the execution_outcomes table.
type ExecutionOutcome struct {
	AttemptID            string                  `json:"attempt_id"`
	CommandID            string                  `json:"command_id"`
	AttemptNumber        int                     `json:"attempt_number"`
	RetryOf              string                  `json:"retry_of,omitempty"`
	CommandType          CommandType             `json:"command_type"`
	StartTime            time.Time               `json:"start_time"`
	EndTime              *time.Time              `json:"end_time,omitempty"`
	Outcome              ExecutionOutcomeOutcome `json:"outcome"`
	FailureClass         FailureClass            `json:"failure_class,omitempty"`
	FailureReason        string                  `json:"failure_reason,omitempty"`
	AffectedEntities     string                  `json:"affected_entities"`
	Retryable            bool                    `json:"retryable"`
	CompensationRequired bool                    `json:"compensation_required"`
	CompensationStatus   CompensationStatus      `json:"compensation_status"`
	RecoveryStatus       RecoveryStatus          `json:"recovery_status"`
	ProposalID           string                  `json:"proposal_id,omitempty"`

	// Correlation context for operator/runs API (optional; populated when context available).
	RunID            string   `json:"run_id,omitempty"`
	RuntimeSessionID string   `json:"runtime_session_id,omitempty"`
	CorrelationID    string   `json:"correlation_id,omitempty"`
	ParentRunID      string   `json:"parent_run_id,omitempty"`
	SkillIDs         []string `json:"skill_ids,omitempty"`
	ConnectorIDs     []string `json:"connector_ids,omitempty"`
	LLMProvider      string   `json:"llm_provider,omitempty"`
	LLMModel         string   `json:"llm_model,omitempty"`
	LLMTaskClass     string   `json:"llm_task_class,omitempty"`
	LLMComplexity    string   `json:"llm_complexity,omitempty"`

	// Workspace audit fields (§18.1)
	WorkspaceID      string          `json:"workspace_id,omitempty"`
	BoundaryCrossing bool            `json:"boundary_crossing"`
	ApprovalRequired bool            `json:"approval_required"`
	ApprovalOutcome  ApprovalOutcome `json:"approval_outcome,omitempty"`
	ArtifactID       string          `json:"artifact_id,omitempty"`
}
