package plugin

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/llm"
)

func TestInstall_FromCatalog(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)
	m := Manifest{ID: "test.plugin", Name: "Test", Version: "0.1.0"}
	tools := []llm.ToolDefinition{{Name: "test_tool", Description: "Test", Parameters: map[string]any{}}}
	reg.SetCatalogEntry(m.ID, m, tools)

	if err := reg.Install(ctx, m.ID, InstallOptions{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !reg.HasManifestID(m.ID) {
		t.Fatal("expected manifest registered after Install")
	}
	if err := reg.Install(ctx, m.ID, InstallOptions{}); err != nil {
		t.Fatalf("Install idempotent: %v", err)
	}
}

func TestInstall_NotInCatalog(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)
	err := reg.Install(ctx, "nonexistent", InstallOptions{})
	if err != ErrLifecycleNotImplemented {
		t.Errorf("Install not in catalog: want ErrLifecycleNotImplemented, got %v", err)
	}
}

func TestUpdate_FromCatalog(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)
	m := Manifest{ID: "test.plugin", Name: "Test", Version: "0.1.0"}
	tools := []llm.ToolDefinition{{Name: "test_tool", Description: "Test", Parameters: map[string]any{}}}
	reg.SetCatalogEntry(m.ID, m, tools)
	reg.RegisterPlugin(m, tools)
	reg.SetPluginEnabled(m.ID, false)

	m2 := Manifest{ID: m.ID, Name: "Test Updated", Version: "0.2.0"}
	reg.SetCatalogEntry(m.ID, m2, tools)
	if err := reg.Update(ctx, m.ID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	manifests := reg.Manifests()
	var found *Manifest
	for i := range manifests {
		if manifests[i].ID == m.ID {
			found = &manifests[i]
			break
		}
	}
	if found == nil || found.Name != "Test Updated" || found.Version != "0.2.0" {
		t.Errorf("Update: expected updated manifest, got %v", found)
	}
	if reg.IsPluginEnabled(m.ID) {
		t.Error("Update: expected plugin to remain disabled")
	}
}

func TestUpdate_NotInCatalog(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry(nil)
	err := reg.Update(ctx, "nonexistent")
	if err != ErrLifecycleNotImplemented {
		t.Errorf("Update not in catalog: want ErrLifecycleNotImplemented, got %v", err)
	}
}
