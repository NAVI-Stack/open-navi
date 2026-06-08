package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ceoai/navi/internal/store"
)

func TestConsoleAppearanceDefaultsAndPersistence(t *testing.T) {
	srv, _, db := testServer(t)
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatal(err)
	}

	res := doReq(t, srv, http.MethodGet, "/api/console/appearance", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET default: expected 200, got %d", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["persisted"] != false {
		t.Fatalf("expected default response to report persisted=false, got %#v", got["persisted"])
	}
	appearance, ok := got["appearance"].(map[string]any)
	if !ok {
		t.Fatalf("expected appearance object, got %#v", got["appearance"])
	}
	if appearance["selected_preset_id"] != "default" {
		t.Fatalf("expected default selected preset, got %#v", appearance["selected_preset_id"])
	}

	payload := map[string]any{
		"schema_version":     "console.appearance.v1",
		"selected_preset_id": "night-shift",
		"theme":              testConsoleAppearanceTheme("#14b8a6"),
		"presets":            []any{map[string]any{"id": "night-shift", "name": "Night Shift", "theme": testConsoleAppearanceTheme("#14b8a6")}},
		"unexpected_ignored": "nope",
	}
	res = doReq(t, srv, http.MethodPatch, "/api/console/appearance", "", "", "127.0.0.1:1234", payload)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH with unknown field should be rejected, got %d", res.StatusCode)
	}
	delete(payload, "unexpected_ignored")

	res = doReq(t, srv, http.MethodPatch, "/api/console/appearance", "", "", "127.0.0.1:1234", payload)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH valid: expected 200, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodGet, "/api/console/appearance", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET persisted: expected 200, got %d", res.StatusCode)
	}
	got = map[string]any{}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["persisted"] != true {
		t.Fatalf("expected persisted=true after PATCH, got %#v", got["persisted"])
	}
	appearance = got["appearance"].(map[string]any)
	if appearance["selected_preset_id"] != "night-shift" {
		t.Fatalf("expected selected preset to persist, got %#v", appearance["selected_preset_id"])
	}
}

func TestConsoleAppearanceRequiresAuth(t *testing.T) {
	srv, _, db := testServer(t)
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatal(err)
	}
	res := doReq(t, srv, http.MethodGet, "/api/console/appearance", "", "", "192.168.1.10:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected remote unauthenticated request to be rejected, got %d", res.StatusCode)
	}
}

func testConsoleAppearanceTheme(accent string) map[string]any {
	tokens := map[string]any{
		"light": map[string]string{
			"background": "#f8fafc",
			"surface":    "#ffffff",
			"text":       "#0f172a",
			"muted":      "#64748b",
			"border":     "#dbe3ef",
			"danger":     "#dc2626",
			"warning":    "#d97706",
			"success":    "#059669",
		},
		"dark": map[string]string{
			"background": "#0b1020",
			"surface":    "#111827",
			"text":       "#e5e7eb",
			"muted":      "#9ca3af",
			"border":     "#1f2937",
			"danger":     "#f87171",
			"warning":    "#fbbf24",
			"success":    "#34d399",
		},
	}
	return map[string]any{
		"mode":       "system",
		"accent":     accent,
		"density":    "compact",
		"font_scale": "default",
		"tokens":     tokens,
	}
}
