package gateway

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/store"
)

const (
	consoleAppearanceSettingKey = "console_appearance"
	consoleAppearanceSchema     = "console.appearance.v1"
	consoleAppearanceMaxBytes   = 64 * 1024
)

var consoleAppearanceHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type consoleAppearanceState struct {
	SchemaVersion    string                    `json:"schema_version"`
	SelectedPresetID string                    `json:"selected_preset_id"`
	Theme            consoleAppearanceTheme    `json:"theme"`
	Presets          []consoleAppearancePreset `json:"presets"`
	UpdatedAt        string                    `json:"updated_at,omitempty"`
}

type consoleAppearancePreset struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Theme     consoleAppearanceTheme `json:"theme"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
}

type consoleAppearanceTheme struct {
	Mode      string                       `json:"mode"`
	Accent    string                       `json:"accent"`
	Density   string                       `json:"density"`
	FontScale string                       `json:"font_scale"`
	Tokens    consoleAppearanceTokenGroups `json:"tokens"`
}

type consoleAppearanceTokenGroups struct {
	Light consoleAppearanceTokens `json:"light"`
	Dark  consoleAppearanceTokens `json:"dark"`
}

type consoleAppearanceTokens struct {
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
	Muted      string `json:"muted"`
	Border     string `json:"border"`
	Danger     string `json:"danger"`
	Warning    string `json:"warning"`
	Success    string `json:"success"`
}

func (s *Server) handleGetConsoleAppearance(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.DB == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "database not configured", nil)
		return
	}
	state, persisted, err := loadConsoleAppearance(r, s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"appearance": state,
		"persisted":  persisted,
	})
}

func (s *Server) handlePatchConsoleAppearance(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.DB == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "database not configured", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, consoleAppearanceMaxBytes)
	var req consoleAppearanceState
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	req.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := normalizeConsoleAppearance(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	blob, err := json.Marshal(req)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if len(blob) > consoleAppearanceMaxBytes {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "appearance payload too large", nil)
		return
	}
	if err := store.SetSetting(r.Context(), s.cfg.DB, consoleAppearanceSettingKey, string(blob)); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"appearance": req,
		"persisted":  true,
	})
}

func loadConsoleAppearance(r *http.Request, db *sql.DB) (consoleAppearanceState, bool, error) {
	raw, found, err := store.GetSetting(r.Context(), db, consoleAppearanceSettingKey)
	if err != nil {
		return consoleAppearanceState{}, false, fmt.Errorf("console appearance lookup failed: %w", err)
	}
	if !found || strings.TrimSpace(raw) == "" {
		return defaultConsoleAppearance(), false, nil
	}
	var state consoleAppearanceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return consoleAppearanceState{}, true, fmt.Errorf("stored console appearance is invalid JSON: %w", err)
	}
	if err := normalizeConsoleAppearance(&state); err != nil {
		return consoleAppearanceState{}, true, fmt.Errorf("stored console appearance is invalid: %w", err)
	}
	return state, true, nil
}

func defaultConsoleAppearance() consoleAppearanceState {
	return consoleAppearanceState{
		SchemaVersion:    consoleAppearanceSchema,
		SelectedPresetID: "default",
		Theme: consoleAppearanceTheme{
			Mode:      "system",
			Accent:    "#3b82f6",
			Density:   "compact",
			FontScale: "default",
			Tokens: consoleAppearanceTokenGroups{
				Light: consoleAppearanceTokens{
					Background: "#f8fafc",
					Surface:    "#ffffff",
					Text:       "#0f172a",
					Muted:      "#64748b",
					Border:     "#dbe3ef",
					Danger:     "#dc2626",
					Warning:    "#d97706",
					Success:    "#059669",
				},
				Dark: consoleAppearanceTokens{
					Background: "#0b1020",
					Surface:    "#111827",
					Text:       "#e5e7eb",
					Muted:      "#9ca3af",
					Border:     "#1f2937",
					Danger:     "#f87171",
					Warning:    "#fbbf24",
					Success:    "#34d399",
				},
			},
		},
		Presets: []consoleAppearancePreset{},
	}
}

func normalizeConsoleAppearance(state *consoleAppearanceState) error {
	if strings.TrimSpace(state.SchemaVersion) == "" {
		state.SchemaVersion = consoleAppearanceSchema
	}
	if state.SchemaVersion != consoleAppearanceSchema {
		return fmt.Errorf("unsupported schema_version %q", state.SchemaVersion)
	}
	if strings.TrimSpace(state.SelectedPresetID) == "" {
		state.SelectedPresetID = "default"
	}
	state.SelectedPresetID = cleanAppearanceID(state.SelectedPresetID)
	if state.SelectedPresetID == "" {
		state.SelectedPresetID = "default"
	}
	if err := validateConsoleTheme(&state.Theme); err != nil {
		return err
	}
	if len(state.Presets) > 12 {
		return fmt.Errorf("at most 12 appearance presets are supported")
	}
	seen := map[string]struct{}{}
	for i := range state.Presets {
		preset := &state.Presets[i]
		preset.ID = cleanAppearanceID(preset.ID)
		preset.Name = strings.TrimSpace(preset.Name)
		if preset.ID == "" {
			return fmt.Errorf("preset id required")
		}
		if preset.ID == "default" {
			return fmt.Errorf("default preset is built in and cannot be overwritten")
		}
		if _, exists := seen[preset.ID]; exists {
			return fmt.Errorf("duplicate preset id %q", preset.ID)
		}
		seen[preset.ID] = struct{}{}
		if preset.Name == "" {
			return fmt.Errorf("preset name required")
		}
		if len(preset.Name) > 48 {
			return fmt.Errorf("preset name %q is too long", preset.Name)
		}
		if err := validateConsoleTheme(&preset.Theme); err != nil {
			return fmt.Errorf("preset %q: %w", preset.Name, err)
		}
	}
	if state.SelectedPresetID != "default" {
		if _, exists := seen[state.SelectedPresetID]; !exists {
			return fmt.Errorf("selected_preset_id %q does not match a saved preset", state.SelectedPresetID)
		}
	}
	return nil
}

func validateConsoleTheme(theme *consoleAppearanceTheme) error {
	switch theme.Mode {
	case "system", "light", "dark":
	default:
		return fmt.Errorf("unsupported mode %q", theme.Mode)
	}
	switch theme.Density {
	case "", "compact":
		theme.Density = "compact"
	case "comfortable":
	default:
		return fmt.Errorf("unsupported density %q", theme.Density)
	}
	switch theme.FontScale {
	case "", "default":
		theme.FontScale = "default"
	case "small", "large":
	default:
		return fmt.Errorf("unsupported font_scale %q", theme.FontScale)
	}
	if !consoleAppearanceHexColor.MatchString(theme.Accent) {
		return fmt.Errorf("accent must be a #RRGGBB color")
	}
	if err := validateConsoleTokens("light", theme.Tokens.Light); err != nil {
		return err
	}
	if err := validateConsoleTokens("dark", theme.Tokens.Dark); err != nil {
		return err
	}
	return nil
}

func validateConsoleTokens(group string, tokens consoleAppearanceTokens) error {
	values := map[string]string{
		"background": tokens.Background,
		"surface":    tokens.Surface,
		"text":       tokens.Text,
		"muted":      tokens.Muted,
		"border":     tokens.Border,
		"danger":     tokens.Danger,
		"warning":    tokens.Warning,
		"success":    tokens.Success,
	}
	for key, value := range values {
		if !consoleAppearanceHexColor.MatchString(value) {
			return fmt.Errorf("%s token %s must be a #RRGGBB color", group, key)
		}
	}
	return nil
}

func cleanAppearanceID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAllowed {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
