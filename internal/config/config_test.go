package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// toYAMLPath converts a path to a form safe for YAML (avoids backslash escapes on Windows).
func toYAMLPath(p string) string {
	return filepath.ToSlash(p)
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
gateway:
  addr: ":9090"
  shared_secret: "test-shared"
  static_dir: "public"
nats:
  url: "nats://remote:4222"
sqlite:
  path: "` + toYAMLPath(dir) + `/test.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(filepath.Join(dir, "prompts")) + `"
  max_response_tokens: 1234
connectors:
  telegram:
    bot_token: "tg-token"
    owner_chat_id: 12345
    gateway_url: "http://example.com"
  slack:
    bot_token: "sl-token"
    app_token: "sl-app"
    gateway_url: "http://example.com"
    allowed_user_ids: ["U123"]
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Gateway.Addr != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Gateway.Addr)
	}
	if cfg.Gateway.SharedSecret != "test-shared" {
		t.Errorf("expected test-shared, got %s", cfg.Gateway.SharedSecret)
	}
	if cfg.NATS.URL != "nats://remote:4222" {
		t.Errorf("expected nats://remote:4222, got %s", cfg.NATS.URL)
	}
	wantSQLite := filepath.Join(dir, "test.db")
	if filepath.ToSlash(cfg.SQLite.Path) != filepath.ToSlash(wantSQLite) {
		t.Errorf("expected sqlite path %s, got %s", wantSQLite, cfg.SQLite.Path)
	}
	if cfg.Connectors.Telegram.BotToken != "tg-token" {
		t.Errorf("expected tg-token, got %s", cfg.Connectors.Telegram.BotToken)
	}
	if cfg.Navi.MaxResponseTokens != 1234 {
		t.Errorf("expected max_response_tokens 1234, got %d", cfg.Navi.MaxResponseTokens)
	}
	if cfg.Connectors.Telegram.OwnerChatID != 12345 {
		t.Errorf("expected 12345, got %d", cfg.Connectors.Telegram.OwnerChatID)
	}
	if cfg.Connectors.Slack.BotToken != "sl-token" {
		t.Errorf("expected sl-token, got %s", cfg.Connectors.Slack.BotToken)
	}
	if len(cfg.Connectors.Slack.AllowedUserIDs) != 1 || cfg.Connectors.Slack.AllowedUserIDs[0] != "U123" {
		t.Errorf("expected [U123], got %v", cfg.Connectors.Slack.AllowedUserIDs)
	}
}

func TestEnvOverrideAppliesMaxResponseTokens(t *testing.T) {
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
sqlite:
  path: "` + toYAMLPath(dir) + `/navi.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
llm:
  ollama_url: "http://localhost:11434/v1"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	os.Setenv("NAVI_MAX_RESPONSE_TOKENS", "2222")
	defer os.Unsetenv("NAVI_MAX_RESPONSE_TOKENS")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Navi.MaxResponseTokens != 2222 {
		t.Fatalf("expected env max_response_tokens 2222, got %d", cfg.Navi.MaxResponseTokens)
	}
}

func TestEffectiveCronConfigUsesFullBackoffDefaults(t *testing.T) {
	want := []int64{30_000, 60_000, 5 * 60_000, 15 * 60_000, 60 * 60_000}
	cfg := Config{}.EffectiveCronConfig()
	if !reflect.DeepEqual(cfg.Retry.BackoffMs, want) {
		t.Fatalf("unexpected backoff defaults: got %v want %v", cfg.Retry.BackoffMs, want)
	}

	def := defaults()
	if !reflect.DeepEqual(def.Cron.Retry.BackoffMs, want) {
		t.Fatalf("unexpected defaults() backoff: got %v want %v", def.Cron.Retry.BackoffMs, want)
	}
}

func TestEffectiveDimensions_RespectsDomainDimensionOverrides(t *testing.T) {
	c := AutonomyConfig{
		GlobalPreset: "balanced",
		DomainDimensionOverrides: map[string]AutonomyDimensions{
			"coding": {
				ExecutionThreshold: "high",
				InsightSurfacing:   "", // fallback to preset
			},
		},
	}
	dims := c.EffectiveDimensions("coding")
	if dims.ExecutionThreshold != "high" {
		t.Errorf("expected execution_threshold high from override, got %q", dims.ExecutionThreshold)
	}
	if dims.InsightSurfacing != "medium" {
		t.Errorf("expected insight_surfacing medium from preset (empty override), got %q", dims.InsightSurfacing)
	}
	// Domain with no override uses preset only.
	other := c.EffectiveDimensions("memory")
	if other.ExecutionThreshold != "medium" || other.InsightSurfacing != "medium" {
		t.Errorf("expected memory to use balanced preset, got %+v", other)
	}
}

func TestLoadSucceedsWithEmptySharedSecret(t *testing.T) {
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
gateway:
  addr: ":8080"
  shared_secret: ""
sqlite:
  path: "` + toYAMLPath(dir) + `/navi.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("NAVI_GATEWAY_SHARED_SECRET")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for empty shared_secret, got: %v", err)
	}
	if cfg.Gateway.SharedSecret != "" {
		t.Errorf("expected empty shared_secret in loaded config, got %q", cfg.Gateway.SharedSecret)
	}
}

func TestEnvOverrideTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
gateway:
  addr: ":8080"
  shared_secret: "yaml-shared"
sqlite:
  path: "` + toYAMLPath(dir) + `/navi.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
connectors:
  telegram:
    bot_token: "yaml-tg"
  slack:
    bot_token: "yaml-sl"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	os.Setenv("NAVI_GATEWAY_SHARED_SECRET", "env-shared")
	os.Setenv("NAVI_GATEWAY_ADDR", ":6285")
	os.Setenv("NAVI_TELEGRAM_BOT_TOKEN", "env-tg")
	os.Setenv("NAVI_SLACK_BOT_TOKEN", "env-sl")
	defer func() {
		os.Unsetenv("NAVI_GATEWAY_SHARED_SECRET")
		os.Unsetenv("NAVI_GATEWAY_ADDR")
		os.Unsetenv("NAVI_TELEGRAM_BOT_TOKEN")
		os.Unsetenv("NAVI_SLACK_BOT_TOKEN")
	}()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Gateway.SharedSecret != "env-shared" {
		t.Errorf("expected env-shared, got %s", cfg.Gateway.SharedSecret)
	}
	if cfg.Gateway.Addr != ":6285" {
		t.Errorf("expected gateway addr :6285, got %s", cfg.Gateway.Addr)
	}
	if cfg.Connectors.Telegram.BotToken != "env-tg" {
		t.Errorf("expected env-tg, got %s", cfg.Connectors.Telegram.BotToken)
	}
	if cfg.Connectors.Slack.BotToken != "env-sl" {
		t.Errorf("expected env-sl, got %s", cfg.Connectors.Slack.BotToken)
	}
}

func TestResolveTelegramAccountsLegacy(t *testing.T) {
	accounts, err := ResolveTelegramAccounts(TelegramConfig{
		BotToken:    "tg-token",
		OwnerChatID: 12345,
		GatewayURL:  "http://localhost:6284",
		AllowFrom:   []int64{111, 222, 111},
	})
	if err != nil {
		t.Fatalf("ResolveTelegramAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].ConnectorName != "telegram" {
		t.Fatalf("expected connector name telegram, got %q", accounts[0].ConnectorName)
	}
	if accounts[0].OwnerChatID != 12345 {
		t.Fatalf("expected owner chat 12345, got %d", accounts[0].OwnerChatID)
	}
	if len(accounts[0].AllowFrom) != 2 {
		t.Fatalf("expected deduped allow_from length 2, got %d", len(accounts[0].AllowFrom))
	}
}

func TestResolveTelegramAccountsDuplicateToken(t *testing.T) {
	_, err := ResolveTelegramAccounts(TelegramConfig{
		Accounts: []TelegramAccountConfig{
			{Name: "one", BotToken: "same-token"},
			{Name: "two", BotToken: "same-token"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate telegram bot token") {
		t.Fatalf("expected duplicate token error, got %v", err)
	}
}

func TestResolveTelegramAccountsCarriesGroups(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		accounts, err := ResolveTelegramAccounts(TelegramConfig{
			BotToken: "tg-token",
			Groups:   map[string]TelegramGroupConfig{"*": {RequireMention: boolPtr(false)}},
		})
		if err != nil {
			t.Fatalf("ResolveTelegramAccounts: %v", err)
		}
		if len(accounts) != 1 {
			t.Fatalf("expected 1 account, got %d", len(accounts))
		}
		g, ok := accounts[0].Groups["*"]
		if !ok || g.RequireMention == nil || *g.RequireMention != false {
			t.Fatalf("legacy account did not carry wildcard group config: %+v", accounts[0].Groups)
		}
	})

	t.Run("account overrides channel fallback", func(t *testing.T) {
		accounts, err := ResolveTelegramAccounts(TelegramConfig{
			Groups: map[string]TelegramGroupConfig{"*": {RequireMention: boolPtr(false)}},
			Accounts: []TelegramAccountConfig{
				{Name: "fallback", BotToken: "tok-a"},
				{Name: "own", BotToken: "tok-b", Groups: map[string]TelegramGroupConfig{"*": {RequireMention: boolPtr(true)}}},
			},
		})
		if err != nil {
			t.Fatalf("ResolveTelegramAccounts: %v", err)
		}
		byName := map[string]ResolvedTelegramAccount{}
		for _, a := range accounts {
			byName[a.AccountName] = a
		}
		if g := byName["fallback"].Groups["*"]; g.RequireMention == nil || *g.RequireMention != false {
			t.Fatalf("fallback account should inherit channel groups, got %+v", byName["fallback"].Groups)
		}
		if g := byName["own"].Groups["*"]; g.RequireMention == nil || *g.RequireMention != true {
			t.Fatalf("own account should use its own groups, got %+v", byName["own"].Groups)
		}
	})
}

func boolPtr(b bool) *bool { return &b }

func TestValidateRejectsNoLLM(t *testing.T) {
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
gateway:
  addr: ":8080"
sqlite:
  path: "` + toYAMLPath(dir) + `/navi.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
llm:
  anthropic_key: ""
  openai_key: ""
  openrouter_key: ""
  ollama_url: ""
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error when no LLM provider set")
	}
	if !strings.Contains(err.Error(), "at least one LLM provider") {
		t.Errorf("expected LLM error, got: %v", err)
	}
}

func TestLoadAllowsMissingExperienceProfilesDir(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	content := `
gateway:
  addr: ":8080"
sqlite:
  path: "` + toYAMLPath(dir) + `/navi.db"
navi:
  experience_profiles_dir: "` + toYAMLPath(dir) + `/nonexistent_personas"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
llm:
  ollama_url: "http://localhost:11434/v1"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected missing experience_profiles_dir to be allowed, got: %v", err)
	}
	if cfg.Navi.ExperienceProfilesDir == "" {
		t.Fatal("expected configured experience_profiles_dir to be retained for experience loading")
	}
}

func TestValidateRejectsUnwritableSQLiteParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission check not reliable on Windows")
	}
	dir := t.TempDir()
	personasDir := filepath.Join(dir, "personas")
	if err := os.MkdirAll(personasDir, 0755); err != nil {
		t.Fatal(err)
	}
	readOnly := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(readOnly, 0444); err != nil {
		t.Fatal(err)
	}
	promptsDir := filepath.Join(dir, "prompts_ok")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
gateway:
  addr: ":8080"
sqlite:
  path: "` + toYAMLPath(readOnly) + `/navi.db"
navi:
  personas_dir: "` + toYAMLPath(personasDir) + `"
  prompts_dir: "` + toYAMLPath(promptsDir) + `"
llm:
  ollama_url: "http://localhost:11434/v1"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error when sqlite parent is not writable")
	}
	if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("expected writable error, got: %v", err)
	}
}

func TestTelegramAccountsJSONRoundTrip(t *testing.T) {
	in := []TelegramAccountConfig{
		{
			Name:        "primary",
			BotToken:    "token-1",
			OwnerChatID: 1001,
			AllowFrom:   []int64{1002},
		},
	}
	raw, err := EncodeTelegramAccountsJSON(in)
	if err != nil {
		t.Fatalf("EncodeTelegramAccountsJSON: %v", err)
	}
	out, err := ParseTelegramAccountsJSON(raw)
	if err != nil {
		t.Fatalf("ParseTelegramAccountsJSON: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 account, got %d", len(out))
	}
	if out[0].Name != "primary" || out[0].BotToken != "token-1" || out[0].OwnerChatID != 1001 {
		t.Fatalf("unexpected round trip payload: %+v", out[0])
	}
}
