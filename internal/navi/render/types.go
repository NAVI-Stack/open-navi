// Package render holds NAVI's data-driven UI render layer: render-intent
// classification, the renderer-neutral NaviDataView model, deterministic OpenUI
// Lang generation, and fallback-markdown generation.
//
// Canonical axiom: "NAVI owns meaning. OpenUI renders meaning." NaviDataView is
// the semantic, renderer-neutral source of truth. OpenUI Lang is only one render
// target derived from it; it is never the canonical state. Every payload carries
// fallback markdown so a failed or unavailable renderer still yields useful data.
//
// This package is deliberately decoupled from internal/navi and internal/runtime
// (it imports neither) so it can be marshaled into an opaque payload and carried
// through the run without creating an import cycle. See ADR-013 and
// docs/architecture/data-driven-ui-rendering.md.
package render

import "time"

// RenderIntent classifies how a chat response should be presented. For the first
// proving slice only PlainResponse and OpenUIDataRender are produced; the other
// values exist so the seam matches the documented router and can grow later.
type RenderIntent string

const (
	RenderIntentPlainResponse         RenderIntent = "plain_response"
	RenderIntentMarkdownStructured    RenderIntent = "markdown_structured"
	RenderIntentDeterministicSystem   RenderIntent = "deterministic_system_ui"
	RenderIntentNaviUISpec            RenderIntent = "navi_ui_spec"
	RenderIntentOpenUIDataRender      RenderIntent = "openui_data_render"
	RenderIntentOpenUIPrototypeRender RenderIntent = "openui_prototype_render"
	RenderIntentArtifactRender        RenderIntent = "artifact_render"
)

// RenderMode is the transport mode of a ChatMessageRenderPayload. The console
// reads this to choose a render lane.
type RenderMode string

const (
	RenderModeText       RenderMode = "text"
	RenderModeMarkdown   RenderMode = "markdown"
	RenderModeArtifact   RenderMode = "artifact"
	RenderModeNaviUI     RenderMode = "navi-ui"
	RenderModeOpenUI     RenderMode = "openui"
	RenderModeSystemCard RenderMode = "system-card"
	RenderModeProposal   RenderMode = "proposal"
)

// DataColumnType is the semantic type of a dataset column.
type DataColumnType string

const (
	ColumnString   DataColumnType = "string"
	ColumnNumber   DataColumnType = "number"
	ColumnDatetime DataColumnType = "datetime"
	ColumnBoolean  DataColumnType = "boolean"
	ColumnDuration DataColumnType = "duration"
)

// DataColumn describes one column of a NaviDataView dataset.
type DataColumn struct {
	Key   string         `json:"key"`
	Label string         `json:"label"`
	Type  DataColumnType `json:"type"`
}

// Dataset is the renderer-neutral tabular payload underlying a NaviDataView.
type Dataset struct {
	Columns []DataColumn     `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// SuggestedView is a renderer-neutral hint about how the data could be shown.
type SuggestedView struct {
	Type      string `json:"type"` // bar | line | pie | table | cards | timeline
	Rationale string `json:"rationale"`
}

// Governance carries the data-view's sensitivity and action policy. For
// read-only views AllowedActions is empty — generated UI must not invent
// privileged actions.
type Governance struct {
	DataSensitivity string   `json:"dataSensitivity"` // public | local | private | sensitive
	AllowedActions  []string `json:"allowedActions"`
	AuditLevel      string   `json:"auditLevel"` // none | basic | sensitive
}

// Fallback is the always-present degraded presentation.
type Fallback struct {
	Markdown string `json:"markdown,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// Trace carries provenance for debugging the render path.
type Trace struct {
	RunID          string   `json:"runId,omitempty"`
	MessageID      string   `json:"messageId,omitempty"`
	ChatID         string   `json:"chatId,omitempty"`
	SourceSkillIDs []string `json:"sourceSkillIds,omitempty"`
}

// NaviDataView is the canonical, renderer-neutral data-view model. NAVI owns it;
// OpenUI Lang is generated from it but never replaces it.
type NaviDataView struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Intent         string          `json:"intent"` // chart | table | dashboard | timeline | inspector | form
	Dataset        Dataset         `json:"dataset"`
	SuggestedViews []SuggestedView `json:"suggestedViews,omitempty"`
	Actions        []any           `json:"actions,omitempty"` // read-only slice for the slice; reserved for NaviActionBinding
	Governance     Governance      `json:"governance"`
	Fallback       Fallback        `json:"fallback"`
	Trace          Trace           `json:"trace,omitempty"`
}

// RenderError records a render-pipeline failure surfaced to the client/diagnostics.
type RenderError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// RenderPayload (the ChatMessageRenderPayload) is the chat transport that carries
// a generated UI alongside its canonical data view and fallback. It is persisted
// into the assistant message's metadata_json under "renderPayload" and
// interpreted by the console. OpenUILang is one render target; DataView is canonical.
type RenderPayload struct {
	Mode       RenderMode    `json:"mode"`
	SourceText string        `json:"sourceText,omitempty"`
	NaviUISpec any           `json:"naviUiSpec,omitempty"`
	OpenUILang string        `json:"openuiLang,omitempty"`
	DataView   *NaviDataView `json:"dataView,omitempty"`
	// Prototype carries the canonical (non-durable) prototype model when this is a
	// generated-UI prototype render. DataView is nil in that case — a prototype has
	// no real telemetry; NaviUIPrototype is the NAVI-owned meaning instead.
	Prototype *NaviUIPrototype `json:"prototype,omitempty"`
	// DataSourceKind labels the provenance of the rendered data. "placeholder" for
	// prototype renders so the console (and any client) can show an honest label.
	DataSourceKind   DataSourceKind `json:"dataSourceKind,omitempty"`
	FallbackMarkdown string         `json:"fallbackMarkdown,omitempty"`
	TraceID          string         `json:"traceId,omitempty"`
	Errors           []RenderError  `json:"errors,omitempty"`
}

// ToolEvent is one observed tool invocation, extracted from persisted
// assistant-message toolParts. It is the (current) chat-scoped data source for
// tool-usage views.
//
// Limitation: this is the FIRST source, not the final one. It lacks per-call
// durations and precise start times (UsedAt is the message timestamp). A richer
// future source is the event log / runtime traces (tool.call.* events). We never
// fabricate missing data — an empty input yields an honest empty/degraded view.
type ToolEvent struct {
	ToolName string
	IsError  bool
	UsedAt   time.Time
}
