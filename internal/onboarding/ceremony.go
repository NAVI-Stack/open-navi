package onboarding

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/store"
)

const (
	CeremonyJourneyID = "navi_ceremony.v1"
	CeremonyVersion   = "v1"

	SettingKeyCeremonyJourneyState = "ceremony_journey_state"
)

type CeremonyJourneyStatus string

const (
	CeremonyStatusNotStarted CeremonyJourneyStatus = "not_started"
	CeremonyStatusInProgress CeremonyJourneyStatus = "in_progress"
	CeremonyStatusCompleted  CeremonyJourneyStatus = "completed"
	CeremonyStatusSkipped    CeremonyJourneyStatus = "skipped"
)

type CeremonyJourneyState struct {
	JourneyID      string                `json:"journeyId"`
	Status         CeremonyJourneyStatus `json:"status"`
	CurrentStep    string                `json:"currentStep,omitempty"`
	CompletedSteps []string              `json:"completedSteps"`
	StartedAt      string                `json:"startedAt,omitempty"`
	CompletedAt    string                `json:"completedAt,omitempty"`
	SkippedAt      string                `json:"skippedAt,omitempty"`
	Version        string                `json:"version"`
	ChatID         string                `json:"chat_id,omitempty"`
}

func DefaultCeremonyJourneyState() CeremonyJourneyState {
	return CeremonyJourneyState{
		JourneyID:      CeremonyJourneyID,
		Status:         CeremonyStatusNotStarted,
		CompletedSteps: []string{},
		Version:        CeremonyVersion,
	}
}

func LoadCeremonyJourneyState(ctx context.Context, db *sql.DB) (CeremonyJourneyState, error) {
	if db == nil {
		return DefaultCeremonyJourneyState(), nil
	}
	raw, found, err := store.GetSetting(ctx, db, SettingKeyCeremonyJourneyState)
	if err != nil {
		return CeremonyJourneyState{}, err
	}
	if !found || strings.TrimSpace(raw) == "" {
		return DefaultCeremonyJourneyState(), nil
	}
	var state CeremonyJourneyState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return CeremonyJourneyState{}, fmt.Errorf("onboarding: decode ceremony journey state: %w", err)
	}
	return normalizeCeremonyJourneyState(state), nil
}

func SaveCeremonyJourneyState(ctx context.Context, db *sql.DB, state CeremonyJourneyState) (CeremonyJourneyState, error) {
	if db == nil {
		return normalizeCeremonyJourneyState(state), nil
	}
	state = normalizeCeremonyJourneyState(state)
	raw, err := json.Marshal(state)
	if err != nil {
		return CeremonyJourneyState{}, fmt.Errorf("onboarding: encode ceremony journey state: %w", err)
	}
	if err := store.SetSetting(ctx, db, SettingKeyCeremonyJourneyState, string(raw)); err != nil {
		return CeremonyJourneyState{}, err
	}
	return state, nil
}

func StartCeremonyJourney(ctx context.Context, db *sql.DB, currentStep string) (CeremonyJourneyState, error) {
	state, err := LoadCeremonyJourneyState(ctx, db)
	if err != nil {
		return CeremonyJourneyState{}, err
	}
	if state.StartedAt == "" {
		state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	state.Status = CeremonyStatusInProgress
	state.CurrentStep = strings.TrimSpace(currentStep)
	if state.CurrentStep == "" {
		state.CurrentStep = "transition"
	}
	return SaveCeremonyJourneyState(ctx, db, state)
}

func CompleteCeremonyJourney(ctx context.Context, db *sql.DB) (CeremonyJourneyState, error) {
	state, err := LoadCeremonyJourneyState(ctx, db)
	if err != nil {
		return CeremonyJourneyState{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if state.StartedAt == "" {
		state.StartedAt = now
	}
	state.Status = CeremonyStatusCompleted
	state.CurrentStep = ""
	state.CompletedSteps = []string{"owner_recognition", "navi_presence", "trust_boundaries", "personalization_seed", "pact_summary"}
	state.CompletedAt = now
	state.SkippedAt = ""
	return SaveCeremonyJourneyState(ctx, db, state)
}

func SkipCeremonyJourney(ctx context.Context, db *sql.DB) (CeremonyJourneyState, error) {
	state, err := LoadCeremonyJourneyState(ctx, db)
	if err != nil {
		return CeremonyJourneyState{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if state.StartedAt == "" {
		state.StartedAt = now
	}
	state.Status = CeremonyStatusSkipped
	state.CurrentStep = ""
	state.SkippedAt = now
	return SaveCeremonyJourneyState(ctx, db, state)
}

func UpdateCeremonyJourneyChatID(ctx context.Context, db *sql.DB, chatID string) (CeremonyJourneyState, error) {
	state, err := LoadCeremonyJourneyState(ctx, db)
	if err != nil {
		return CeremonyJourneyState{}, err
	}
	state.ChatID = strings.TrimSpace(chatID)
	return SaveCeremonyJourneyState(ctx, db, state)
}

func CeremonyRequiresRouting(state CeremonyJourneyState) bool {
	state = normalizeCeremonyJourneyState(state)
	return state.Status != CeremonyStatusCompleted && state.Status != CeremonyStatusSkipped
}

func normalizeCeremonyJourneyState(state CeremonyJourneyState) CeremonyJourneyState {
	if strings.TrimSpace(state.JourneyID) == "" {
		state.JourneyID = CeremonyJourneyID
	}
	if strings.TrimSpace(state.Version) == "" {
		state.Version = CeremonyVersion
	}
	switch state.Status {
	case CeremonyStatusNotStarted, CeremonyStatusInProgress, CeremonyStatusCompleted, CeremonyStatusSkipped:
	default:
		state.Status = CeremonyStatusNotStarted
	}
	if state.CompletedSteps == nil {
		state.CompletedSteps = []string{}
	}
	return state
}
