package proposals

import (
	"encoding/json"

	"github.com/ceoai/navi/internal/schema"
)

const workspaceBoundaryActionType = "workspace_boundary_crossing"

// WorkspaceBoundaryAction is the canonical proposal payload for approvals that
// cross a workspace boundary.
type WorkspaceBoundaryAction struct {
	Type            string                     `json:"type"`
	ToolName        string                     `json:"tool_name"`
	Arguments       map[string]any             `json:"arguments,omitempty"`
	ChatID          string                     `json:"chat_id,omitempty"`
	RunID           string                     `json:"run_id,omitempty"`
	ICSGovernance   any                        `json:"ics_governance_handoff,omitempty"`
	ICSIntent       any                        `json:"ics_execution_intent,omitempty"`
	WorkspaceID     string                     `json:"workspace_id,omitempty"`
	TargetPath      string                     `json:"target_path,omitempty"`
	WorkspaceAction schema.WorkspaceActionType `json:"workspace_action,omitempty"`
	ActionTypes     []string                   `json:"action_types,omitempty"`
	RuleScope       string                     `json:"rule_scope,omitempty"`
}

func ParseWorkspaceBoundaryAction(proposal schema.Proposal) (WorkspaceBoundaryAction, bool) {
	var action WorkspaceBoundaryAction
	if err := json.Unmarshal([]byte(proposal.ProposedAction), &action); err != nil {
		return WorkspaceBoundaryAction{}, false
	}
	return action, action.Type == workspaceBoundaryActionType
}
