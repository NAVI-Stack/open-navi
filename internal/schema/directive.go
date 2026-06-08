package schema

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type DirectiveMode string
type DirectiveStatus string

// DirectiveMode defines the operational posture of NAVI for a given task.
// Modes regulate the assistant's level of initiative, autonomy, and responsibility
// in an always-on environment — they are behavioral states, not development lifecycle phases.
const (
	// DirectiveModeChat — Conversational presence. Responds to dialogue only; no actions taken.
	DirectiveModeChat DirectiveMode = "CHAT"
	// DirectiveModeAdvise — Reasoning partner. Provides analysis, suggestions, and planning but takes no actions.
	DirectiveModeAdvise DirectiveMode = "ADVISE"
	// DirectiveModeAssist — Performs routine support tasks within safe, pre-approved boundaries.
	DirectiveModeAssist DirectiveMode = "ASSIST"
	// DirectiveModeAct — Executes workflows and automations with defined permissions.
	DirectiveModeAct DirectiveMode = "ACT"
	// DirectiveModeWatch — Monitors systems or conditions and triggers alerts or actions when thresholds are met.
	DirectiveModeWatch DirectiveMode = "WATCH"
)

const (
	DirectiveStatusActive   DirectiveStatus = "ACTIVE"
	DirectiveStatusPaused   DirectiveStatus = "PAUSED"
	DirectiveStatusComplete DirectiveStatus = "COMPLETE"
	DirectiveStatusStalled  DirectiveStatus = "STALLED"
	DirectiveStatusClosed   DirectiveStatus = "CLOSED"
)

// Validate reports whether the directive mode is one of the canonical values.
func (m DirectiveMode) Validate() error {
	switch m {
	case DirectiveModeChat, DirectiveModeAdvise, DirectiveModeAssist, DirectiveModeAct, DirectiveModeWatch:
		return nil
	default:
		return fmt.Errorf("directive: invalid mode %q", m)
	}
}

type Directive struct {
	DirectiveID string          `json:"directive_id"`
	Title       string          `json:"title"`
	Mode        DirectiveMode   `json:"mode"`
	Status      DirectiveStatus `json:"status"`
	CreatedVia  string          `json:"created_via"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DirectiveMessage struct {
	MessageID   string    `json:"message_id"`
	DirectiveID string    `json:"directive_id"`
	Role        string    `json:"role"` // "owner" | "navi"
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	TokensUsed  int       `json:"tokens_used,omitempty"`
	Model       string    `json:"model,omitempty"`
}

// NewDirective constructs a Directive with a generated ID and current timestamps.
func NewDirective(title string, mode DirectiveMode, via string) Directive {
	now := time.Now().UTC()
	return Directive{
		DirectiveID: uuid.New().String(),
		Title:       title,
		Mode:        mode,
		Status:      DirectiveStatusActive,
		CreatedVia:  via,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Validate checks the directive for required fields and valid enum values.
func (d *Directive) Validate() error {
	if d.DirectiveID == "" {
		return fmt.Errorf("directive: directive_id is required")
	}
	if d.Title == "" {
		return fmt.Errorf("directive: title is required")
	}
	if err := d.Mode.Validate(); err != nil {
		return err
	}
	switch d.Status {
	case DirectiveStatusActive, DirectiveStatusPaused, DirectiveStatusComplete, DirectiveStatusStalled, DirectiveStatusClosed:
		// valid
	default:
		return fmt.Errorf("directive: invalid status %q", d.Status)
	}
	if d.CreatedVia == "" {
		return fmt.Errorf("directive: created_via is required")
	}
	return nil
}

// Validate checks the directive message for required fields.
func (m *DirectiveMessage) Validate() error {
	if m.MessageID == "" {
		return fmt.Errorf("directive_message: message_id is required")
	}
	if m.DirectiveID == "" {
		return fmt.Errorf("directive_message: directive_id is required")
	}
	if m.Role != "owner" && m.Role != "navi" {
		return fmt.Errorf("directive_message: role must be 'owner' or 'navi', got %q", m.Role)
	}
	if m.Content == "" {
		return fmt.Errorf("directive_message: content is required")
	}
	return nil
}

// NewDirectiveMessage constructs a DirectiveMessage with a generated ID and current timestamp.
func NewDirectiveMessage(directiveID, role, content string) DirectiveMessage {
	return DirectiveMessage{
		MessageID:   uuid.New().String(),
		DirectiveID: directiveID,
		Role:        role,
		Content:     content,
		CreatedAt:   time.Now().UTC(),
	}
}
