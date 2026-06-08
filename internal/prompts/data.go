package prompts

// ChatSystemData is passed to KindChatSystem.
type ChatSystemData struct {
	ExperienceControl  string
	SkillsPrompt       string
	CurrentTimeRFC3339 string
	Timezone           string
	LocalTimeSuffix    bool
	ChatID             string
	IsOllama           bool
	FactsBlock         string
	SummaryBlock       string
}

// OrchestratorDirectiveData is passed to KindOrchestratorDirective.
type OrchestratorDirectiveData struct {
	Title           string
	ModeDescription string
}

// OrchestratorImplementData is passed to KindOrchestratorImplement.
type OrchestratorImplementData struct {
	Title    string
	MaxTasks int
}

// OrchestratorDecomposeData is passed to KindOrchestratorDecompose (workspace listing task).
type OrchestratorDecomposeData struct {
	Title   string
	DirList string
}

// SummarizerUserData is passed to KindSummarizerUser.
type SummarizerUserData struct {
	ChatID       string
	MessagesText string
}

// HeartbeatTaskLine is one numbered task for KindHeartbeatCycle.
type HeartbeatTaskLine struct {
	N           int
	Description string
}

// HeartbeatCycleData is passed to KindHeartbeatCycle.
type HeartbeatCycleData struct {
	Timestamp string
	TZLabel   string
	IsLocal   bool
	Tasks     []HeartbeatTaskLine
}

// CoderSystemData is passed to KindCoderSystem.
type CoderSystemData struct {
	Title       string
	Description string
	Surfaces    string
}

// CriticSystemData is passed to KindCriticSystem.
type CriticSystemData struct {
	Title       string
	Description string
}

// NCOSRulesData is passed to NCOS rule templates (identity, chat behavior, etc.).
type NCOSRulesData struct {
	CurrentTime string
}

// ScoutUserContent is formatted in runner; system uses KindScoutSystem (static).
// Strategist uses KindStrategistSystem (static) + user message built in runner.
