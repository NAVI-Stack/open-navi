// Package instructions compiles explicit NCOS instruction layers.
package instructions

import "github.com/open-navi/navi/internal/navi/orchestration"

type SystemCoreInput struct {
	IdentityRules      []string
	ToolUseRules       []string
	ChatBehavior       []string
	DiagnosticsRules   []string
	SecurityRules      []string
	ProviderStateRules []string
	TimeContext        string
	SessionContext     string
	ModelStyleRules    []string
	Additional         []string
}

type RuntimeConstraintsInput struct {
	GovernanceNotes       []string
	AuthoritativeWarnings []string
	Additional            []string
}

type ExperienceOverlayInput struct {
	Fragment   string
	Additional []string
}

type TaskFrameInput struct {
	Objective    string
	Instructions []string
	Additional   []string
}

type OutputContractInput struct {
	Present               bool
	MustReply             bool
	AllowToolCalls        bool
	AllowScheduledReplies bool
	ExpectStreaming       bool
	Additional            []string
}

type CapabilitySurfaceInput struct {
	Surface         string
	ToolNames       []string
	SelectionReason string
	Additional      []string
}

type ExecutionFrameInput struct {
	Frame      orchestration.ExecutionFrame
	Additional []string
}
