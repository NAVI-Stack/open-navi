package schema

// AgentType identifies the role of an agent in the system.
type AgentType string

const (
	// AgentNavi is the primary persistent personal assistant.
	AgentNavi AgentType = "navi"
	// AgentHeartbeat is the background autonomy loop that runs scheduled tasks.
	AgentHeartbeat AgentType = "heartbeat"
	// AgentConnector represents an inbound/outbound channel connector (Telegram, Slack, etc.).
	AgentConnector AgentType = "connector"
	// AgentCoder is the task worker that executes code-edit tasks (ReadFile, ListDir, WriteFile).
	AgentCoder AgentType = "coder"
	// AgentCritic is the task worker that reviews surfaces and produces structured feedback.
	AgentCritic AgentType = "critic"
	// AgentStrategist is the task worker that produces design documents from directives.
	AgentStrategist AgentType = "strategist"
	// AgentScout is the task worker for web research and external knowledge routing.
	AgentScout AgentType = "scout"
)
