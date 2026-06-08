package render

import "strings"

// This file defines NaviUIPrototype — the canonical, renderer-neutral model for
// an in-chat generated UI *prototype* (a dashboard/component/panel/display mockup).
//
// Canonical axiom (same as NaviDataView): "NAVI owns meaning. OpenUI renders
// meaning." NaviUIPrototype is the NAVI-owned source of truth; OpenUI Lang is one
// render target derived from it (see prototype_openui.go) and is never canonical.
//
// Scope (Block 2 — in-chat prototype rendering): prototypes are NOT persisted as
// artifacts, are NOT live, use clearly-labeled PLACEHOLDER data, and are read-only
// (no privileged actions). NaviUIPrototype is therefore deliberately minimal.

// PrototypePurpose is the kind of prototype surface requested.
type PrototypePurpose string

const (
	PurposeDashboard PrototypePurpose = "dashboard"
	PurposeComponent PrototypePurpose = "component"
	PurposeWorkflow  PrototypePurpose = "workflow"
	PurposeDemo      PrototypePurpose = "demo"
	PurposeInspector PrototypePurpose = "inspector"
	PurposeDisplay   PrototypePurpose = "display"
	PurposePanel     PrototypePurpose = "panel"
)

// DataSourceKind labels whether rendered data is real, placeholder, or mixed.
// Block 2 prototypes are always DataSourcePlaceholder. The value is also surfaced
// on RenderPayload so the console can render an honest "placeholder" label.
type DataSourceKind string

const (
	DataSourceReal        DataSourceKind = "real"
	DataSourcePlaceholder DataSourceKind = "placeholder"
	DataSourceMixed       DataSourceKind = "mixed"
)

// PlaceholderPolicy records how placeholder data may be used for a prototype.
type PlaceholderPolicy string

const (
	PlaceholderNone     PlaceholderPolicy = "none"
	PlaceholderAllowed  PlaceholderPolicy = "allowed"
	PlaceholderRequired PlaceholderPolicy = "required"
)

// PrototypeGovernance carries the prototype's action policy. Prototype UI is
// read-only for this slice, so AllowedActions is always empty — generated UI must
// not invoke privileged actions.
type PrototypeGovernance struct {
	AllowedActions []string `json:"allowedActions"`
	AuditLevel     string   `json:"auditLevel"` // none | basic | sensitive
}

// PrototypeTrace carries provenance for debugging the prototype render path.
type PrototypeTrace struct {
	RunID     string `json:"runId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
}

// NaviUIPrototype is the canonical, renderer-neutral prototype model. NAVI owns
// it; OpenUI Lang is generated from it but never replaces it. It is NOT durable
// state and is never persisted as an artifact in this slice.
type NaviUIPrototype struct {
	ID                string              `json:"id"`
	Title             string              `json:"title"`
	Purpose           PrototypePurpose    `json:"purpose"`
	Prompt            string              `json:"prompt"`
	DataSourceKind    DataSourceKind      `json:"dataSourceKind"`
	PlaceholderPolicy PlaceholderPolicy   `json:"placeholderPolicy"`
	ComponentIntent   string              `json:"componentIntent"` // "unknown" for the slice
	FallbackMarkdown  string              `json:"fallbackMarkdown"`
	Governance        PrototypeGovernance `json:"governance"`
	Trace             *PrototypeTrace     `json:"trace,omitempty"`
}

// NormalizePrototypePurpose is the canonical sanitizer for a purpose value. It is
// called on every construction path (BuildPrototype / BuildPrototypePayload) so the
// canonical model never trusts raw classifier or model-provided output. Unknown,
// empty, or garbage values default to PurposeComponent.
func NormalizePrototypePurpose(raw string) PrototypePurpose {
	switch PrototypePurpose(strings.ToLower(strings.TrimSpace(raw))) {
	case PurposeDashboard:
		return PurposeDashboard
	case PurposeComponent:
		return PurposeComponent
	case PurposeWorkflow:
		return PurposeWorkflow
	case PurposeDemo:
		return PurposeDemo
	case PurposeInspector:
		return PurposeInspector
	case PurposeDisplay:
		return PurposeDisplay
	case PurposePanel:
		return PurposePanel
	default:
		return PurposeComponent
	}
}
