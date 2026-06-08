package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// ActivityType is the locked activity type enum for the operator feed.
type ActivityType string

const (
	ActivityProposalCreated    ActivityType = "proposal_created"
	ActivityProposalResolved   ActivityType = "proposal_resolved"
	ActivityRunStarted         ActivityType = "run_started"
	ActivityRunCompleted       ActivityType = "run_completed"
	ActivityRunFailed          ActivityType = "run_failed"
	ActivityConnectorError     ActivityType = "connector_error"
	ActivityConnectorRecovered ActivityType = "connector_recovered"
	ActivityGovernorTripped    ActivityType = "governor_tripped"
	ActivityGovernorRecovered  ActivityType = "governor_recovered"
	ActivityMessageSent       ActivityType = "message_sent"
	ActivityMessageReceived    ActivityType = "message_received"
	ActivityDirectiveExecuted  ActivityType = "directive_executed"
)

// ActivityEntry is the locked schema for a single activity feed item (operator-facing).
type ActivityEntry struct {
	At             string   `json:"at"`
	Type           string   `json:"type"`
	Summary        string   `json:"summary"`
	CorrelationID  string   `json:"correlation_id"`
	ID             string   `json:"id,omitempty"`
	ChatID         string   `json:"chat_id,omitempty"`
	ProposalID     string   `json:"proposal_id,omitempty"`
	RunID          string   `json:"run_id,omitempty"`
	DirectiveID    string   `json:"directive_id,omitempty"`
	ConnectorID    string   `json:"connector_id,omitempty"`
	EventSeq       int64    `json:"event_seq,omitempty"`
}

// MapEventToActivity maps a store event to an activity entry and summary per the deterministic template table.
// Returns (entry, true) when the event type is mapped; (zero, false) when skipped.
func MapEventToActivity(ev schema.Event) (ActivityEntry, bool) {
	at := ev.Timestamp.UTC().Format(time.RFC3339)
	entry := ActivityEntry{
		At:            at,
		CorrelationID: ev.CorrelationID,
		ID:            ev.ID,
		EventSeq:      ev.Seq,
	}
	var actType ActivityType
	var summary string
	switch ev.Type {
	case schema.FactGovernorTripped:
		actType = ActivityGovernorTripped
		summary = "Governor tripped"
	case schema.CmdTaskAssign:
		actType = ActivityRunStarted
		if m, ok := ev.Payload.(map[string]any); ok {
			if t, ok := m["task"].(map[string]any); ok {
				if id, _ := t["id"].(string); id != "" {
					entry.RunID = id
					summary = fmt.Sprintf("Run started: %s", id)
				}
			}
		}
		if summary == "" {
			summary = "Run started: (unknown)"
		}
	case schema.FactDirectiveReplied:
		actType = ActivityDirectiveExecuted
		if p, ok := ev.Payload.(map[string]any); ok {
			if id, _ := p["directive_id"].(string); id != "" {
				entry.DirectiveID = id
				summary = fmt.Sprintf("Directive executed: %s", id)
			}
		}
		if summary == "" {
			summary = "Directive executed: (unknown)"
		}
	case schema.CmdDirectiveMessage:
		actType = ActivityMessageSent
		if p, ok := ev.Payload.(map[string]any); ok {
			if id, _ := p["directive_id"].(string); id != "" {
				entry.DirectiveID = id
			}
		}
		summary = fmt.Sprintf("Message sent: %s", entry.DirectiveID)
		if entry.DirectiveID == "" {
			summary = "Message sent: (unknown)"
		}
	case schema.FactNaviReplied:
		actType = ActivityMessageSent
		summary = "Message sent: (session)"
	case schema.FactNaviActionRequested:
		actType = ActivityProposalCreated
		if p, ok := ev.Payload.(map[string]any); ok {
			if id, _ := p["proposal_id"].(string); id != "" {
				entry.ProposalID = id
				summary = fmt.Sprintf("Proposal created: %s", id)
			}
			if id, _ := p["chat_id"].(string); id != "" {
				entry.ChatID = id
			}
		}
		if summary == "" {
			summary = "Proposal created: (unknown)"
		}
	default:
		return ActivityEntry{}, false
	}
	entry.Type = string(actType)
	entry.Summary = summary
	return entry, true
}

// MapEventToActivityFromJSON is like MapEventToActivity but unmarshals payload from raw JSON when Payload is not already a map.
// Use when reading events from the store (payload is []byte or string).
func MapEventToActivityFromJSON(ev schema.Event, payloadJSON []byte) (ActivityEntry, bool) {
	if payloadJSON != nil {
		var m map[string]any
		if err := json.Unmarshal(payloadJSON, &m); err == nil {
			ev.Payload = m
		}
	}
	return MapEventToActivity(ev)
}
