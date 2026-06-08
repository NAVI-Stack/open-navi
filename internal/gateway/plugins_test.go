package gateway

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/open-navi/navi/internal/navi/plugin"
)

func testPluginManifests() func() []plugin.Manifest {
	return func() []plugin.Manifest {
		return []plugin.Manifest{{
			ID:        "test.plugin",
			Name:      "Test Plugin",
			Version:   "1.0.0",
			Kind:      plugin.KindDomain,
			Status:    "active",
			TrustTier: "builtin",
		}}
	}
}

func TestPluginEnableDisable(t *testing.T) {
	var gotID string
	var gotEnabled bool
	called := 0
	srv := NewServer(Config{
		PluginManifests: testPluginManifests(),
		SetPluginEnabled: func(pluginID string, enabled bool) error {
			gotID = pluginID
			gotEnabled = enabled
			called++
			return nil
		},
		Addr: ":0",
	})

	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/disable", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("disable: expected 200, got %d", res.StatusCode)
	}
	if gotID != "test.plugin" || gotEnabled != false {
		t.Fatalf("disable: expected (test.plugin,false), got (%q,%v)", gotID, gotEnabled)
	}

	res = doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/enable", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("enable: expected 200, got %d", res.StatusCode)
	}
	if gotID != "test.plugin" || gotEnabled != true {
		t.Fatalf("enable: expected (test.plugin,true), got (%q,%v)", gotID, gotEnabled)
	}
	if called != 2 {
		t.Fatalf("expected hook called twice, got %d", called)
	}
}

func TestPluginEnableUnknownPlugin(t *testing.T) {
	srv := NewServer(Config{
		PluginManifests:  testPluginManifests(),
		SetPluginEnabled: func(string, bool) error { return nil },
		Addr:             ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/does.not.exist/enable", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown plugin: expected 404, got %d", res.StatusCode)
	}
}

func TestPluginEnableHookMissing(t *testing.T) {
	srv := NewServer(Config{
		PluginManifests: testPluginManifests(),
		Addr:            ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/enable", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("missing hook: expected 501, got %d", res.StatusCode)
	}
}

func TestPluginValidate(t *testing.T) {
	srv := NewServer(Config{
		PluginManifests: testPluginManifests(),
		Addr:            ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/validate", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("validate: expected 200, got %d", res.StatusCode)
	}
	var out struct {
		PluginID string   `json:"pluginId"`
		Valid    bool     `json:"valid"`
		Reasons  []string `json:"reasons"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("validate: decode: %v", err)
	}
	if out.PluginID != "test.plugin" || !out.Valid {
		t.Fatalf("validate: expected valid test.plugin, got %#v", out)
	}
}

func TestPluginValidateInvalidManifest(t *testing.T) {
	srv := NewServer(Config{
		PluginManifests: func() []plugin.Manifest {
			return []plugin.Manifest{{
				ID:   "bad.plugin",
				Kind: "not-a-real-kind",
			}}
		},
		Addr: ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/bad.plugin/validate", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("validate invalid: expected 200, got %d", res.StatusCode)
	}
	var out struct {
		Valid   bool     `json:"valid"`
		Reasons []string `json:"reasons"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("validate invalid: decode: %v", err)
	}
	if out.Valid || len(out.Reasons) == 0 {
		t.Fatalf("validate invalid: expected invalid with reasons, got %#v", out)
	}
}

func TestPluginReload(t *testing.T) {
	called := 0
	srv := NewServer(Config{
		PluginManifests: testPluginManifests(),
		ReloadPlugins:   func() error { called++; return nil },
		Addr:            ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/reload", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reload: expected 200, got %d", res.StatusCode)
	}
	if called != 1 {
		t.Fatalf("reload: expected hook called once, got %d", called)
	}
}

func TestPluginReloadHookMissing(t *testing.T) {
	srv := NewServer(Config{
		PluginManifests: testPluginManifests(),
		Addr:            ":0",
	})
	res := doJSONReq(t, srv, http.MethodPost, "/api/plugins/test.plugin/reload", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("reload missing hook: expected 501, got %d", res.StatusCode)
	}
}
