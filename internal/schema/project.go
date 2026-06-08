package schema

import "time"

// ProjectKind classifies how NAVI should treat a project at the capability
// boundary. Coding projects require a usable workspace before mutative code work.
type ProjectKind string

const (
	ProjectKindGeneral ProjectKind = "general"
	ProjectKindCoding  ProjectKind = "coding"
)

var validProjectKinds = map[ProjectKind]struct{}{
	ProjectKindGeneral: {},
	ProjectKindCoding:  {},
}

func (k ProjectKind) IsValid() bool {
	_, ok := validProjectKinds[k]
	return ok
}

func ParseProjectKind(value string) (ProjectKind, error) {
	return parseWorldModelEnum(value, validProjectKinds, "project_kind")
}

// ProjectStatus is the durable lifecycle state for a project.
type ProjectStatus string

const (
	ProjectStatusDraft     ProjectStatus = "draft"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusOnHold    ProjectStatus = "on_hold"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

var validProjectStatuses = map[ProjectStatus]struct{}{
	ProjectStatusDraft:     {},
	ProjectStatusActive:    {},
	ProjectStatusOnHold:    {},
	ProjectStatusCompleted: {},
	ProjectStatusArchived:  {},
}

func (s ProjectStatus) IsValid() bool {
	_, ok := validProjectStatuses[s]
	return ok
}

func ParseProjectStatus(value string) (ProjectStatus, error) {
	return parseWorldModelEnum(value, validProjectStatuses, "project_status")
}

// ProjectHealth captures a lightweight human-facing status signal.
type ProjectHealth string

const (
	ProjectHealthOnTrack ProjectHealth = "on_track"
	ProjectHealthAtRisk  ProjectHealth = "at_risk"
	ProjectHealthBlocked ProjectHealth = "blocked"
	ProjectHealthUnknown ProjectHealth = "unknown"
)

var validProjectHealthValues = map[ProjectHealth]struct{}{
	ProjectHealthOnTrack: {},
	ProjectHealthAtRisk:  {},
	ProjectHealthBlocked: {},
	ProjectHealthUnknown: {},
}

func (h ProjectHealth) IsValid() bool {
	_, ok := validProjectHealthValues[h]
	return ok
}

func ParseProjectHealth(value string) (ProjectHealth, error) {
	return parseWorldModelEnum(value, validProjectHealthValues, "project_health")
}

// MemoryScope controls whether a project's memories are shared with or isolated
// from chats and agents outside the project boundary.
type MemoryScope string

const (
	MemoryScopeDefault     MemoryScope = "default"
	MemoryScopeProjectOnly MemoryScope = "project_only"
)

var validMemoryScopes = map[MemoryScope]struct{}{
	MemoryScopeDefault:     {},
	MemoryScopeProjectOnly: {},
}

func (m MemoryScope) IsValid() bool {
	_, ok := validMemoryScopes[m]
	return ok
}

func ParseMemoryScope(value string) (MemoryScope, error) {
	return parseWorldModelEnum(value, validMemoryScopes, "memory_scope")
}

// Project is NAVI's first-class container for centered work. WorkspaceID is the
// authoritative project-owned workspace binding; legacy workspace.project links
// remain readable for compatibility.
type Project struct {
	ID               string                 `json:"project_id"`
	Title            string                 `json:"title"`
	Slug             string                 `json:"slug"`
	Description      string                 `json:"description,omitempty"`
	Kind             ProjectKind            `json:"project_kind"`
	Status           ProjectStatus          `json:"status"`
	Health           ProjectHealth          `json:"health"`
	WorkspaceID      string                 `json:"workspace_id,omitempty"`
	SandboxProfileID string                 `json:"sandbox_profile_id,omitempty"`
	Icon             string                 `json:"icon,omitempty"`
	Color            string                 `json:"color,omitempty"`
	MemoryScope      MemoryScope            `json:"memory_scope"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CreatedBy        string                 `json:"created_by"`
	Attributes       map[string]interface{} `json:"attributes,omitempty"`
}

// ProjectReadiness is the deterministic gate used before assigning project work
// to coding-capable execution.
type ProjectReadiness struct {
	ProjectID                string   `json:"project_id"`
	ProjectKind              string   `json:"project_kind"`
	Status                   string   `json:"status"`
	ReadyForChat             bool     `json:"ready_for_chat"`
	ReadyForPlanning         bool     `json:"ready_for_planning"`
	ReadyForCoding           bool     `json:"ready_for_coding"`
	WorkspaceBindingRequired bool     `json:"workspace_binding_required"`
	WorkspaceID              string   `json:"workspace_id,omitempty"`
	SandboxProfileID         string   `json:"sandbox_profile_id,omitempty"`
	DegradedReasons          []string `json:"degraded_reasons"`
}
