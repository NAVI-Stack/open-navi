package onboarding

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/open-navi/navi/internal/store"
)

// FirstRunState is the compact state machine used by the backend-hosted
// first-run onboarding flow. The CLI and backend web UI both use this state.
type FirstRunState string

const (
	FirstRunUninitialized      FirstRunState = "uninitialized"
	FirstRunRecoveryCreated    FirstRunState = "recovery_created"
	FirstRunProviderConfigured FirstRunState = "provider_configured"
	FirstRunComplete           FirstRunState = "complete"
)

const (
	SettingKeyComplete           = "onboarding_complete"
	SettingKeySetupComplete      = "setup_complete"
	SettingKeyOnboardingMode     = "onboarding_mode"
	SettingKeyFirstRunState      = "onboarding_first_run_state"
	SettingKeyRecoverySavedPath  = "onboarding_recovery_saved_path"
	SettingKeyProviderConfigured = "onboarding_provider_configured"
	SettingKeyProviderName       = "onboarding_provider_name"
	SettingKeyProviderModel      = "onboarding_provider_model"
	SettingKeyProviderEndpoint   = "onboarding_provider_endpoint"
	SettingKeyConnectionStatus   = "onboarding_connection_status"
	SettingKeyConnectionType     = "onboarding_connection_type"
)

// FirstRunStatus is the safe public status view for browser onboarding. It
// intentionally contains no raw API keys, owner secrets, recovery seeds, or
// provider credentials.
type FirstRunStatus struct {
	State             FirstRunState
	Claimed           bool
	Complete          bool
	RecoverySavedPath string
	ProviderName      string
	ProviderModel     string
	ProviderEndpoint  string
	ConnectionStatus  string
	ConnectionType    string
}

// DeriveFirstRunStatus folds owner/setup settings into the first-run state
// machine used by both CLI and web onboarding.
func DeriveFirstRunStatus(ctx context.Context, db *sql.DB, _ string) (FirstRunStatus, error) {
	settings, err := store.GetAllSettings(ctx, db)
	if err != nil {
		return FirstRunStatus{}, err
	}
	claimed, err := store.OwnerExists(ctx, db)
	if err != nil {
		return FirstRunStatus{}, err
	}

	status := FirstRunStatus{
		State:             FirstRunUninitialized,
		Claimed:           claimed,
		RecoverySavedPath: strings.TrimSpace(settings[SettingKeyRecoverySavedPath]),
		ProviderName:      firstNonEmpty(settings[SettingKeyProviderName], settings["llm_provider"]),
		ProviderModel:     firstNonEmpty(settings[SettingKeyProviderModel], settings["llm_ollama_model"], settings["llm_openai_model"], settings["llm_anthropic_model"], settings["llm_openrouter_model"]),
		ProviderEndpoint:  strings.TrimSpace(settings[SettingKeyProviderEndpoint]),
		ConnectionStatus:  strings.TrimSpace(settings[SettingKeyConnectionStatus]),
		ConnectionType:    strings.TrimSpace(settings[SettingKeyConnectionType]),
	}

	if (settings[SettingKeyComplete] == "true" || settings[SettingKeySetupComplete] == "true") && claimed {
		status.State = FirstRunComplete
		status.Complete = true
		return status, nil
	}

	persisted := FirstRunState(strings.TrimSpace(settings[SettingKeyFirstRunState]))
	if persisted == FirstRunProviderConfigured && claimed {
		status.State = FirstRunProviderConfigured
		return status, nil
	}
	if persisted == FirstRunRecoveryCreated && claimed {
		status.State = FirstRunRecoveryCreated
		return status, nil
	}

	if settings[SettingKeyProviderConfigured] == "true" && claimed {
		status.State = FirstRunProviderConfigured
		return status, nil
	}
	if status.ProviderName != "" && claimed {
		status.State = FirstRunProviderConfigured
		return status, nil
	}
	if claimed {
		status.State = FirstRunRecoveryCreated
		return status, nil
	}
	return status, nil
}

// SetFirstRunState persists a validated first-run state transition.
func SetFirstRunState(ctx context.Context, db *sql.DB, current, next FirstRunState) error {
	if !CanTransitionFirstRun(current, next) {
		return fmt.Errorf("onboarding: invalid transition %s -> %s", current, next)
	}
	if err := store.SetSetting(ctx, db, SettingKeyFirstRunState, string(next)); err != nil {
		return err
	}
	if next == FirstRunComplete {
		if err := store.SetSetting(ctx, db, SettingKeyComplete, "true"); err != nil {
			return err
		}
		if err := store.SetSetting(ctx, db, SettingKeySetupComplete, "true"); err != nil {
			return err
		}
		return store.SetSetting(ctx, db, SettingKeyOnboardingMode, "false")
	}
	return store.SetSetting(ctx, db, SettingKeyOnboardingMode, "true")
}

// CanTransitionFirstRun enforces the linear first-run flow while allowing
// idempotent retries at the current step.
func CanTransitionFirstRun(current, next FirstRunState) bool {
	if current == next {
		return true
	}
	switch current {
	case "":
		return next == FirstRunUninitialized
	case FirstRunUninitialized:
		return next == FirstRunRecoveryCreated
	case FirstRunRecoveryCreated:
		return next == FirstRunProviderConfigured
	case FirstRunProviderConfigured:
		return next == FirstRunComplete
	case FirstRunComplete:
		return next == FirstRunComplete
	default:
		return false
	}
}

// DefaultRecoveryPassportPath returns the backend-local default path for the
// one-time owner passport material.
func DefaultRecoveryPassportPath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "."
	}
	return filepath.Join(dataDir, "recovery", "navi-owner-passport.txt")
}

// WriteRecoveryPassport writes recovery material with owner-only permissions
// where the host filesystem honors POSIX modes.
func WriteRecoveryPassport(path string, content []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("onboarding: recovery passport path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create recovery directory: %w", err)
	}
	_ = os.Chmod(dir, 0o700)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write recovery passport: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`navi_[A-Za-z0-9_-]{12,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)(secret|token|api[_-]?key|password)(\s*[=:]\s*)([^,\s;]+)`),
}

// RedactSecrets removes common secret-shaped values from strings before they
// are returned in errors or diagnostic contexts.
func RedactSecrets(text string) string {
	out := text
	out = redactionPatterns[0].ReplaceAllString(out, "[redacted]")
	out = redactionPatterns[1].ReplaceAllString(out, "[redacted]")
	out = redactionPatterns[2].ReplaceAllString(out, `${1}${2}[redacted]`)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
