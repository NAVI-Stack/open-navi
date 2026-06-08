package hooks

import "context"

// HookName identifies a lifecycle hook point.
type HookName string

const (
	HookBeforeToolCall    HookName = "before_tool_call"
	HookAfterToolCall     HookName = "after_tool_call"
	HookMessageReceived   HookName = "message_received"
	HookMessageSending    HookName = "message_sending"
	HookSessionStart      HookName = "session_start"
	HookSessionEnd        HookName = "session_end"
	HookBeforePromptBuild HookName = "before_prompt_build"
)

// HookHandler is invoked at a hook point. Priority is used for ordering (lower = runs first).
// Returning a non-nil error stops the chain and returns that error.
// Returning (newPayload, nil) passes the new payload to the next handler.
type HookHandler func(ctx context.Context, payload interface{}) (interface{}, error)

// Registration holds a single hook registration.
type Registration struct {
	Priority int
	Handler  HookHandler
}
