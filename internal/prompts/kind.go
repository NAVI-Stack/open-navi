package prompts

// Kind identifies a prompt template file.
type Kind string

const (
	KindChatSystem            Kind = "chat/system"
	KindOrchestratorDirective Kind = "orchestrator/directive"
	KindOrchestratorImplement Kind = "orchestrator/implement"
	KindOrchestratorDecompose Kind = "orchestrator/decompose"
	KindSummarizerSystem      Kind = "summarizer/system"
	KindSummarizerUser        Kind = "summarizer/user"
	KindHeartbeatCycle        Kind = "heartbeat/cycle"
	KindCoderSystem           Kind = "agents/coder/system"
	KindCriticSystem          Kind = "agents/critic/system"
	KindScoutSystem           Kind = "agents/scout/system"
	KindStrategistSystem      Kind = "agents/strategist/system"

	KindNCOSIdentityRules      Kind = "ncos/identity_rules"
	KindNCOSChatBehavior       Kind = "ncos/chat_behavior"
	KindNCOSDiagnosticsRules   Kind = "ncos/diagnostics_rules"
	KindNCOSSecurityRules      Kind = "ncos/security_rules"
	KindNCOSProviderStateRules Kind = "ncos/provider_state_rules"
	KindNCOSModelStyleRules    Kind = "ncos/model_style_rules"
	KindDirectiveModes         Kind = "ncos/directive_modes"
	KindIntrospectionGrounding Kind = "ncos/introspection_grounding"
	KindNCOSProgrammerWorkflow Kind = "ncos/programmer_workflow"

	// KindNCOSCoderWorkflow is the forward-facing Coder alias for
	// KindNCOSProgrammerWorkflow (migration ticket NEW-A). It is intentionally
	// NOT a member of AllKinds(): there is no ncos/coder_workflow.md on disk or
	// in embed. It is normalized to KindNCOSProgrammerWorkflow on the read path
	// (Render, Snapshot, IsKnownKind) so it renders the existing Programmer
	// template. This is the compatibility seam for the eventual atomic prompt-kind
	// rename (NEW-D); the legacy constant, value, file, and callers are untouched.
	KindNCOSCoderWorkflow Kind = "ncos/coder_workflow"
)

// promptKindAliases maps Coder-facing prompt-kind aliases (NEW-A) to their
// legacy canonical kind. Kept local to the prompts package, which owns the
// ncos/* namespace.
var promptKindAliases = map[Kind]Kind{
	KindNCOSCoderWorkflow: KindNCOSProgrammerWorkflow,
}

// canonicalKind resolves a Coder-facing prompt-kind alias to its legacy
// canonical kind so it renders the existing template. All other kinds pass
// through unchanged.
func canonicalKind(k Kind) Kind {
	if canonical, ok := promptKindAliases[k]; ok {
		return canonical
	}
	return k
}

// AllKinds is every template managed by the prompt manager (bootstrap + reload).
func AllKinds() []Kind {
	return []Kind{
		KindChatSystem,
		KindOrchestratorDirective,
		KindOrchestratorImplement,
		KindOrchestratorDecompose,
		KindSummarizerSystem,
		KindSummarizerUser,
		KindHeartbeatCycle,
		KindCoderSystem,
		KindCriticSystem,
		KindScoutSystem,
		KindStrategistSystem,
		KindNCOSIdentityRules,
		KindNCOSChatBehavior,
		KindNCOSDiagnosticsRules,
		KindNCOSSecurityRules,
		KindNCOSProviderStateRules,
		KindNCOSModelStyleRules,
		KindDirectiveModes,
		KindIntrospectionGrounding,
		KindNCOSProgrammerWorkflow,
	}
}

func kindRelPath(k Kind) string {
	return string(k) + ".md"
}

// IsKnownKind reports whether k is managed by the prompt manager. Coder-facing
// aliases (NEW-A) are recognized via their legacy canonical kind.
func IsKnownKind(k Kind) bool {
	k = canonicalKind(k)
	for _, ak := range AllKinds() {
		if ak == k {
			return true
		}
	}
	return false
}
