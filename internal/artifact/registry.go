package artifact

import (
	"sort"
	"strings"
	"sync"

	"github.com/open-navi/navi/internal/schema"
)

const (
	PatchStrategyText       = "text"
	PatchStrategyBlock      = "block"
	PatchStrategyTextRange  = "text_range"
	PatchStrategyAST        = "ast"
	PatchStrategyFunction   = "function"
	PatchStrategyStructured = "structured_object"
	PatchStrategyRowCell    = "row_cell"
	PatchStrategyFull       = "full_replace"
)

// RendererDescriptor describes how a client should render an artifact subtype.
type RendererDescriptor struct {
	Type            schema.ArtifactType `json:"type"`
	Subtype         string              `json:"subtype"`
	ComponentID     string              `json:"component_id"` // Frontend component name
	DiffViewerID    string              `json:"diff_viewer_id,omitempty"`
	Icon            string              `json:"icon"`
	Label           string              `json:"label"`
	DisplayModes    []string            `json:"display_modes,omitempty"`
	ExportFormats   []string            `json:"export_formats,omitempty"`
	PatchStrategies []string            `json:"patch_strategies,omitempty"`
	SupportsDiff    bool                `json:"supports_diff"`
	SupportsHistory bool                `json:"supports_history"`
	SupportsPrint   bool                `json:"supports_print"`
}

// EditorDescriptor describes how a client should edit an artifact subtype.
type EditorDescriptor struct {
	Type              schema.ArtifactType `json:"type"`
	Subtype           string              `json:"subtype"`
	EditorID          string              `json:"editor_id"` // Frontend editor component name
	DiffViewerID      string              `json:"diff_viewer_id,omitempty"`
	Features          []string            `json:"features,omitempty"`         // e.g. "syntax_highlighting", "autocomplete"
	ParseFormats      []string            `json:"parse_formats,omitempty"`    // e.g. "markdown", "json", "csv"
	ExportFormats     []string            `json:"export_formats,omitempty"`   // e.g. "md", "pdf"
	PatchStrategies   []string            `json:"patch_strategies,omitempty"` // ordered by preference
	ValidateStrategy  string              `json:"validate_strategy,omitempty"`
	SupportsPatch     bool                `json:"supports_patch"`
	SupportsReplace   bool                `json:"supports_replace"`
	SupportsPreview   bool                `json:"supports_preview"`
	SupportsStructured bool               `json:"supports_structured"`
}

// Registry manages the mapping of artifact subtypes to renderers and editors.
type Registry struct {
	mu        sync.RWMutex
	renderers map[string]RendererDescriptor
	editors   map[string]EditorDescriptor
}

// NewRegistry creates a new Registry with default mappings.
func NewRegistry() *Registry {
	r := &Registry{
		renderers: make(map[string]RendererDescriptor),
		editors:   make(map[string]EditorDescriptor),
	}
	r.registerDefaults()
	return r
}

func (r *Registry) registerDefaults() {
	// Markdown
	r.RegisterRenderer(RendererDescriptor{
		Type:            schema.ArtifactTypeDocument,
		Subtype:         "markdown",
		ComponentID:     "MarkdownRenderer",
		DiffViewerID:    "UnifiedDiffViewer",
		Icon:            "description",
		Label:           "Document",
		DisplayModes:    []string{"read", "edit", "diff", "history", "inspector"},
		ExportFormats:   []string{"md", "pdf"},
		PatchStrategies: []string{PatchStrategyBlock, PatchStrategyTextRange, PatchStrategyFull},
		SupportsDiff:    true,
		SupportsHistory: true,
		SupportsPrint:   true,
	})
	r.RegisterEditor(EditorDescriptor{
		Type:               schema.ArtifactTypeDocument,
		Subtype:            "markdown",
		EditorID:           "MarkdownEditor",
		DiffViewerID:       "UnifiedDiffViewer",
		Features:           []string{"preview", "headings", "tables", "code_blocks"},
		ParseFormats:       []string{"markdown"},
		ExportFormats:      []string{"md", "pdf"},
		PatchStrategies:    []string{PatchStrategyBlock, PatchStrategyTextRange, PatchStrategyFull},
		ValidateStrategy:   "markdown",
		SupportsPatch:      true,
		SupportsReplace:    true,
		SupportsPreview:    true,
		SupportsStructured: true,
	})
	r.RegisterRendererAlias("text", "markdown")
	r.RegisterRendererAlias("summary", "markdown")
	r.RegisterRendererAlias("workflow_draft", "markdown")
	r.RegisterEditorAlias("text", "markdown")
	r.RegisterEditorAlias("summary", "markdown")
	r.RegisterEditorAlias("workflow_draft", "markdown")

	// Source Code (generic)
	r.RegisterRenderer(RendererDescriptor{
		Type:            schema.ArtifactTypeCode,
		Subtype:         "code",
		ComponentID:     "CodeRenderer",
		DiffViewerID:    "UnifiedDiffViewer",
		Icon:            "code",
		Label:           "Source Code",
		DisplayModes:    []string{"read", "edit", "diff", "history", "inspector"},
		ExportFormats:   []string{"txt", "formatted"},
		PatchStrategies: []string{PatchStrategyAST, PatchStrategyFunction, PatchStrategyText, PatchStrategyFull},
		SupportsDiff:    true,
		SupportsHistory: true,
		SupportsPrint:   true,
	})
	r.RegisterEditor(EditorDescriptor{
		Type:               schema.ArtifactTypeCode,
		Subtype:            "code",
		EditorID:           "CodeEditor",
		DiffViewerID:       "UnifiedDiffViewer",
		Features:           []string{"syntax_highlighting", "line_numbers"},
		ParseFormats:       []string{"plain_text"},
		ExportFormats:      []string{"txt", "formatted"},
		PatchStrategies:    []string{PatchStrategyAST, PatchStrategyFunction, PatchStrategyText, PatchStrategyFull},
		ValidateStrategy:   "language_aware",
		SupportsPatch:      true,
		SupportsReplace:    true,
		SupportsPreview:    true,
		SupportsStructured: false,
	})

	// Data / JSON
	r.RegisterRenderer(RendererDescriptor{
		Type:            schema.ArtifactTypeData,
		Subtype:         "json",
		ComponentID:     "JSONRenderer",
		DiffViewerID:    "UnifiedDiffViewer",
		Icon:            "data_object",
		Label:           "JSON Data",
		DisplayModes:    []string{"read", "edit", "diff", "history", "inspector"},
		ExportFormats:   []string{"json"},
		PatchStrategies: []string{PatchStrategyStructured, PatchStrategyFull},
		SupportsDiff:    true,
		SupportsHistory: true,
	})
	r.RegisterEditor(EditorDescriptor{
		Type:               schema.ArtifactTypeData,
		Subtype:            "json",
		EditorID:           "JSONEditor",
		DiffViewerID:       "UnifiedDiffViewer",
		Features:           []string{"tree_view", "schema_validation"},
		ParseFormats:       []string{"json"},
		ExportFormats:      []string{"json"},
		PatchStrategies:    []string{PatchStrategyStructured, PatchStrategyFull},
		ValidateStrategy:   "json",
		SupportsPatch:      true,
		SupportsReplace:    true,
		SupportsPreview:    true,
		SupportsStructured: true,
	})

	// Data / Table
	r.RegisterRenderer(RendererDescriptor{
		Type:            schema.ArtifactTypeData,
		Subtype:         "table",
		ComponentID:     "DataTableRenderer",
		DiffViewerID:    "UnifiedDiffViewer",
		Icon:            "table_chart",
		Label:           "Data Table",
		DisplayModes:    []string{"read", "edit", "diff", "history", "inspector"},
		ExportFormats:   []string{"csv", "json"},
		PatchStrategies: []string{PatchStrategyRowCell, PatchStrategyStructured, PatchStrategyFull},
		SupportsDiff:    true,
		SupportsHistory: true,
	})
	r.RegisterEditor(EditorDescriptor{
		Type:               schema.ArtifactTypeData,
		Subtype:            "table",
		EditorID:           "DataTableEditor",
		DiffViewerID:       "UnifiedDiffViewer",
		Features:           []string{"table_editing", "column_types"},
		ParseFormats:       []string{"csv", "json_array"},
		ExportFormats:      []string{"csv", "json"},
		PatchStrategies:    []string{PatchStrategyRowCell, PatchStrategyStructured, PatchStrategyFull},
		ValidateStrategy:   "tabular",
		SupportsPatch:      true,
		SupportsReplace:    true,
		SupportsPreview:    true,
		SupportsStructured: true,
	})
	r.RegisterRendererAlias("csv", "table")
	r.RegisterEditorAlias("csv", "table")
}

// RegisterRenderer adds a renderer descriptor.
func (r *Registry) RegisterRenderer(desc RendererDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renderers[desc.Subtype] = desc
}

// RegisterRendererAlias maps one subtype to an existing renderer contract.
func (r *Registry) RegisterRendererAlias(alias, target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if desc, ok := r.renderers[target]; ok {
		aliasDesc := desc
		aliasDesc.Subtype = alias
		r.renderers[alias] = aliasDesc
	}
}

// RegisterEditor adds an editor descriptor.
func (r *Registry) RegisterEditor(desc EditorDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.editors[desc.Subtype] = desc
}

// RegisterEditorAlias maps one subtype to an existing editor contract.
func (r *Registry) RegisterEditorAlias(alias, target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if desc, ok := r.editors[target]; ok {
		aliasDesc := desc
		aliasDesc.Subtype = alias
		r.editors[alias] = aliasDesc
	}
}

// GetRenderer returns the renderer for a subtype, or a raw fallback.
func (r *Registry) GetRenderer(subtype string) RendererDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	subtype = normalizeSubtype(subtype)
	if d, ok := r.renderers[subtype]; ok {
		return d
	}
	// Raw Fallback
	return RendererDescriptor{
		Subtype:       subtype,
		ComponentID:   "RawRenderer",
		DiffViewerID:  "UnifiedDiffViewer",
		Icon:          "insert_drive_file",
		Label:         "Raw File",
		DisplayModes:  []string{"read", "edit"},
		ExportFormats: []string{"txt"},
	}
}

// GetEditor returns the editor for a subtype, or a raw fallback.
func (r *Registry) GetEditor(subtype string) EditorDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	subtype = normalizeSubtype(subtype)
	if d, ok := r.editors[subtype]; ok {
		return d
	}
	// Raw Fallback
	return EditorDescriptor{
		Subtype:         subtype,
		EditorID:        "RawEditor",
		ParseFormats:    []string{"plain_text"},
		ExportFormats:   []string{"txt"},
		PatchStrategies: []string{PatchStrategyFull},
		SupportsReplace: true,
	}
}

// ListRenderers returns registered renderer contracts ordered by subtype.
func (r *Registry) ListRenderers() []RendererDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]RendererDescriptor, 0, len(r.renderers))
	for _, desc := range r.renderers {
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Subtype < out[j].Subtype
	})
	return out
}

// ListEditors returns registered editor contracts ordered by subtype.
func (r *Registry) ListEditors() []EditorDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]EditorDescriptor, 0, len(r.editors))
	for _, desc := range r.editors {
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Subtype < out[j].Subtype
	})
	return out
}

func normalizeSubtype(subtype string) string {
	return strings.TrimSpace(strings.ToLower(subtype))
}
