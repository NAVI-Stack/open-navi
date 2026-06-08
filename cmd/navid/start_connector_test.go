package main

import (
	"context"
	"path/filepath"
	"testing"

	pkgconn "github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/config"
	intconnectors "github.com/ceoai/navi/internal/connectors"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/store"
)

type testConnector struct {
	name    string
	running bool
}

func (c *testConnector) Name() string { return c.name }
func (c *testConnector) Start(context.Context) error {
	c.running = true
	return nil
}
func (c *testConnector) Stop(context.Context) error {
	c.running = false
	return nil
}
func (c *testConnector) Send(context.Context, pkgconn.OutboundMessage) error { return nil }
func (c *testConnector) IsRunning() bool                                     { return c.running }

func TestMakeStartConnectorUsesCanonicalTelegramAccountPath(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/navi.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	cfg := &config.Config{}
	cfg.Connectors.Telegram.GatewayURL = "http://localhost:6284"
	cfg.Connectors.Telegram.APIURL = "https://api.telegram.org"

	natsBus := bus.NewMemBus(db)
	registry := intconnectors.NewRegistry()
	manager := intconnectors.NewManager(registry, intconnectors.ManagerConfig{})

	var resolved []config.ResolvedTelegramAccount
	registerTelegramFactory := func(acct config.ResolvedTelegramAccount) {
		resolved = append(resolved, acct)
		registry.RegisterFactory(acct.ConnectorName, func(_ *config.Config, _ bus.Bus) (pkgconn.Connector, error) {
			return &testConnector{name: acct.ConnectorName}, nil
		})
	}

	startConnector := makeStartConnector(cfg, db, natsBus, registry, manager, registerTelegramFactory, map[string]bool{"telegram": true})
	err = startConnector(context.Background(), "telegram", map[string]string{
		"bot_token":      "test-token",
		"owner_chat_id":  "42",
		"account":        "ops",
		"allow_from":     "100,200,100",
		"pairing_code":   "pair-code",
		"gateway_url":    "http://gateway.internal:6284",
		"api_url":        "https://telegram.internal",
		"webhook_url":    "https://example.com/webhook",
		"webhook_secret": "hook-secret",
	})
	if err != nil {
		t.Fatalf("start connector: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected one resolved account, got %d", len(resolved))
	}
	if resolved[0].ConnectorName != "telegram-ops" {
		t.Fatalf("expected connector telegram-ops, got %q", resolved[0].ConnectorName)
	}
	if resolved[0].GatewayURL != "http://gateway.internal:6284" {
		t.Fatalf("unexpected gateway url %q", resolved[0].GatewayURL)
	}
	if resolved[0].APIURL != "https://telegram.internal" {
		t.Fatalf("unexpected api url %q", resolved[0].APIURL)
	}
	if resolved[0].WebhookURL != "https://example.com/webhook" {
		t.Fatalf("unexpected webhook url %q", resolved[0].WebhookURL)
	}
	if resolved[0].WebhookSecret != "hook-secret" {
		t.Fatalf("unexpected webhook secret %q", resolved[0].WebhookSecret)
	}
	if len(resolved[0].AllowFrom) != 2 {
		t.Fatalf("expected deduped allow_from list, got %#v", resolved[0].AllowFrom)
	}

	rawAccounts, found, err := store.GetSetting(context.Background(), db, config.TelegramSettingKeyAccounts)
	if err != nil {
		t.Fatalf("get telegram accounts: %v", err)
	}
	if !found {
		t.Fatal("expected telegram_accounts setting to be saved")
	}
	accounts, err := config.ParseTelegramAccountsJSON(rawAccounts)
	if err != nil {
		t.Fatalf("parse telegram accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "ops" {
		t.Fatalf("unexpected persisted accounts %#v", accounts)
	}
	if got := registry.Get("telegram-ops"); got == nil {
		t.Fatal("expected named telegram connector instance to be created")
	}
}

func TestMakeStartConnectorPreservesLegacySingleAccountSettings(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/navi.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	cfg := &config.Config{}
	cfg.Connectors.Telegram.GatewayURL = "http://localhost:6284"
	cfg.Connectors.Telegram.APIURL = "https://api.telegram.org"

	natsBus := bus.NewMemBus(db)
	registry := intconnectors.NewRegistry()
	manager := intconnectors.NewManager(registry, intconnectors.ManagerConfig{})

	registerTelegramFactory := func(acct config.ResolvedTelegramAccount) {
		registry.RegisterFactory(acct.ConnectorName, func(_ *config.Config, _ bus.Bus) (pkgconn.Connector, error) {
			return &testConnector{name: acct.ConnectorName}, nil
		})
	}

	startConnector := makeStartConnector(cfg, db, natsBus, registry, manager, registerTelegramFactory, map[string]bool{"telegram": true})
	err = startConnector(context.Background(), "telegram", map[string]string{
		"bot_token":     "default-token",
		"owner_chat_id": "99",
	})
	if err != nil {
		t.Fatalf("start connector: %v", err)
	}

	if got := registry.Get("telegram"); got == nil {
		t.Fatal("expected default telegram connector instance to be created")
	}

	botToken, found, err := store.GetSetting(context.Background(), db, config.TelegramSettingKeyBotToken)
	if err != nil || !found {
		t.Fatalf("expected legacy bot token setting, found=%v err=%v", found, err)
	}
	if botToken != "default-token" {
		t.Fatalf("unexpected legacy bot token %q", botToken)
	}

	ownerChatID, found, err := store.GetSetting(context.Background(), db, config.TelegramSettingKeyOwnerChatID)
	if err != nil || !found {
		t.Fatalf("expected legacy owner chat id setting, found=%v err=%v", found, err)
	}
	if ownerChatID != "99" {
		t.Fatalf("unexpected legacy owner chat id %q", ownerChatID)
	}
}

func TestMakeStartConnectorRejectsInactivePluginConnector(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/navi.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	cfg := &config.Config{}
	natsBus := bus.NewMemBus(db)
	registry := intconnectors.NewRegistry()
	manager := intconnectors.NewManager(registry, intconnectors.ManagerConfig{})

	startConnector := makeStartConnector(cfg, db, natsBus, registry, manager, func(config.ResolvedTelegramAccount) {}, map[string]bool{"telegram": false})
	err = startConnector(context.Background(), "telegram", map[string]string{"bot_token": "test-token"})
	if err == nil {
		t.Fatal("expected inactive telegram plugin to block connector setup")
	}
}

func TestRegisterTelegramSetupFactoryKeepsTelegramInSetupSchemaWithoutAccounts(t *testing.T) {
	registry := intconnectors.NewRegistry()
	registerTelegramSetupFactory(registry, nil, nil)

	// Load telegram manifest from repo-owned plugin.yaml (not from RegisterBuiltin).
	pluginsDir := filepath.Join("..", "..", "plugins")
	pluginLoader := plugin.NewLoader(pluginsDir, "", "")
	pluginRegistry := plugin.NewRegistry(nil)
	if err := pluginLoader.LoadIntoRegistry(pluginRegistry, nil); err != nil {
		t.Fatalf("LoadIntoRegistry: %v", err)
	}
	for _, m := range pluginRegistry.Manifests() {
		if m.Connector == nil || m.Connector.Kind != "builtin" {
			continue
		}
		d := plugin.DriverFromManifest(m)
		registry.ExtendDriver(m.Connector.DriverID, d.SetupDescriptor(), d.Capabilities())
	}

	descs := registry.SetupDescriptors()
	found := false
	for _, desc := range descs {
		if desc.Type == "telegram" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected telegram to remain available in setup schema without configured accounts")
	}
}
