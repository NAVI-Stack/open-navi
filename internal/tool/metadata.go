package tool

var defaultToolInteractionModes = []ToolInteractionMode{
	ToolInteractionConversation,
	ToolInteractionAction,
}

// IsKnownToolExposureClass reports whether the supplied exposure class is part
// of the settled Phase 2 capability-surface contract.
func IsKnownToolExposureClass(class ToolExposureClass) bool {
	switch class {
	case ToolExposureUserFacing, ToolExposureInternal, ToolExposureDevelopment, ToolExposureTest:
		return true
	default:
		return false
	}
}

// IsKnownToolInteractionMode reports whether the supplied interaction mode is
// part of the settled Phase 2 capability-surface contract.
func IsKnownToolInteractionMode(mode ToolInteractionMode) bool {
	switch mode {
	case ToolInteractionConversation, ToolInteractionAction:
		return true
	default:
		return false
	}
}

// NormalizedExposureClass returns the effective exposure class for a tool.
// Empty metadata defaults to user-facing so existing registry behavior is
// preserved until the resolver starts consuming this field.
func (t *Tool) NormalizedExposureClass() ToolExposureClass {
	if t == nil || t.Metadata.ExposureClass == "" {
		return ToolExposureUserFacing
	}
	return t.Metadata.ExposureClass
}

// HasInvalidExposureClass reports whether the tool carries an explicit
// unsupported exposure class. Empty metadata is not treated as invalid.
func (t *Tool) HasInvalidExposureClass() bool {
	if t == nil || t.Metadata.ExposureClass == "" {
		return false
	}
	return !IsKnownToolExposureClass(t.Metadata.ExposureClass)
}

// NormalizedInteractionModes returns the effective interaction modes in a
// deterministic contract order. Empty metadata defaults to both conversation
// and action so current runtime behavior is preserved until the resolver
// consumes explicit interaction policy.
func (t *Tool) NormalizedInteractionModes() []ToolInteractionMode {
	if t == nil || len(t.Metadata.InteractionModes) == 0 {
		return append([]ToolInteractionMode(nil), defaultToolInteractionModes...)
	}

	seen := make(map[ToolInteractionMode]struct{}, len(t.Metadata.InteractionModes))
	for _, mode := range t.Metadata.InteractionModes {
		if IsKnownToolInteractionMode(mode) {
			seen[mode] = struct{}{}
		}
	}

	out := make([]ToolInteractionMode, 0, len(defaultToolInteractionModes))
	for _, mode := range defaultToolInteractionModes {
		if _, ok := seen[mode]; ok {
			out = append(out, mode)
		}
	}
	return out
}

// HasInvalidInteractionModes reports whether the tool carries any explicit
// unsupported interaction modes.
func (t *Tool) HasInvalidInteractionModes() bool {
	if t == nil {
		return false
	}
	for _, mode := range t.Metadata.InteractionModes {
		if !IsKnownToolInteractionMode(mode) {
			return true
		}
	}
	return false
}

// SupportsInteractionMode reports whether the effective interaction policy
// includes the given request class.
func (t *Tool) SupportsInteractionMode(mode ToolInteractionMode) bool {
	for _, candidate := range t.NormalizedInteractionModes() {
		if candidate == mode {
			return true
		}
	}
	return false
}

// RequiresToolCapableModel reports the effective tool-capable-model
// requirement. Registry tools default to requiring a tool-capable model,
// preserving current semantics until a future ticket explicitly introduces a
// different class of registry-backed capability.
func (t *Tool) RequiresToolCapableModel() bool {
	if t == nil {
		return true
	}
	if t.Metadata.RequiresToolCapableModel == nil {
		return true
	}
	return *t.Metadata.RequiresToolCapableModel
}
