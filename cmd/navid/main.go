package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ceoai/navi/internal/backlog"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/coder"
	"github.com/ceoai/navi/internal/cognitive"
	"github.com/ceoai/navi/internal/command"
	"github.com/ceoai/navi/internal/config"
	"github.com/ceoai/navi/internal/connectors"
	"github.com/ceoai/navi/internal/critic"
	cpkg "github.com/ceoai/navi/internal/cron"
	"github.com/ceoai/navi/internal/gateway"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/identity"
	"github.com/ceoai/navi/internal/identity/keystore"
	"github.com/ceoai/navi/internal/intake"
	"github.com/ceoai/navi/internal/intake/embed"
	intakepolicy "github.com/ceoai/navi/internal/intake/policy"
	"github.com/ceoai/navi/internal/intake/retrieve"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/navi/heartbeat"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/reflection"
	navistore "github.com/ceoai/navi/internal/navi/store"
	"github.com/ceoai/navi/internal/onboarding"
	"github.com/ceoai/navi/internal/orchestrator"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/scout"
	"github.com/ceoai/navi/internal/store"
	"github.com/ceoai/navi/internal/strategist"
	"github.com/ceoai/navi/internal/worldmodel"
	figure "github.com/common-nighthawk/go-figure"
)

var version = "dev"
var build = "" // set via ldflags at release, e.g. -ldflags "-X main.build=abc123"

// worldModelConfigPriorityReader adapts *worldmodel.WorldModel to governor.ConfigPriorityReader for owner-scoped governance checks.
type worldModelConfigPriorityReader struct {
	wm *worldmodel.WorldModel
}

func (r worldModelConfigPriorityReader) ListConfiguration(ctx context.Context, ownerID string, limit int) ([]schema.ConfigurationEntry, error) {
	return r.wm.ListConfigurationByScope(ctx, "owner", ownerID, limit)
}

func (r worldModelConfigPriorityReader) ListPriorities(ctx context.Context, ownerID string, limit int) ([]schema.Priority, error) {
	return r.wm.ListPrioritiesByScope(ctx, "owner", ownerID, limit)
}

func logBanner(logLine func(string), phrase, font string) {
	fig := figure.NewFigure(phrase, font, false)
	for _, line := range fig.Slicify() {
		logLine(line)
	}
}

func main() {
	logBanner(func(s string) { log.Println(s) }, "NAVI", "")

	cfg, err := config.LoadOrDefault("config/runtime.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	level := slog.LevelInfo
	if cfg.Navi.Debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Open SQLite.
	log.Printf("Opening SQLite at %s...", cfg.SQLite.Path)
	db, err := store.Open(cfg.SQLite.Path)
	if err != nil {
		log.Fatalf("failed to open sqlite: %v", err)
	}

	if err := store.CreateTables(ctx, db); err != nil {
		log.Fatalf("failed to create tables: %v", err)
	}
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		log.Fatalf("failed to create navi tables: %v", err)
	}
	if err := ensureDefaultSandboxProfiles(ctx, db); err != nil {
		log.Fatalf("failed to seed default sandbox profiles: %v", err)
	}
	dataDir := filepath.Dir(cfg.SQLite.Path)
	if dataDir == "" || dataDir == "." {
		dataDir = "."
	}
	keyStore, err := keystore.New(keystore.Config{
		Backend:    strings.TrimSpace(os.Getenv("NAVI_IDENTITY_KEYSTORE_BACKEND")),
		DataDir:    dataDir,
		Passphrase: os.Getenv("NAVI_IDENTITY_KEYSTORE_PASSPHRASE"),
	})
	if err != nil {
		log.Fatalf("failed to initialize identity keystore: %v", err)
	}
	agentIdentity, createdIdentity, err := identity.EnsureInitialized(ctx, db, keyStore)
	if err != nil {
		log.Fatalf("failed to initialize local identity: %v", err)
	}
	if createdIdentity {
		slog.Info("generated local agent identity", "key_type", agentIdentity.KeyType, "fingerprint", agentIdentity.Fingerprint)
	} else {
		slog.Debug("loaded local agent identity", "key_type", agentIdentity.KeyType, "fingerprint", agentIdentity.Fingerprint)
	}

	// Load persisted settings so wizard-configured keys survive restarts.
	persistedSettings, _ := store.GetAllSettings(ctx, db)
	if persistedSettings == nil {
		persistedSettings = map[string]string{}
	}
	if v := persistedSettings["llm_anthropic_key"]; v != "" {
		cfg.LLM.AnthropicKey = v
	}
	if v := persistedSettings["llm_openai_key"]; v != "" {
		cfg.LLM.OpenAIKey = v
	}
	if v := persistedSettings["llm_openrouter_key"]; v != "" {
		cfg.LLM.OpenRouterKey = v
	}
	if v := persistedSettings["llm_anthropic_model"]; v != "" {
		cfg.LLM.AnthropicModel = v
	}
	if v := persistedSettings["llm_openai_model"]; v != "" {
		cfg.LLM.OpenAIModel = v
	}
	if v := persistedSettings["llm_openrouter_model"]; v != "" {
		cfg.LLM.OpenRouterModel = v
	}
	if v := persistedSettings["llm_ollama_url"]; v != "" {
		cfg.LLM.OllamaURL = v
	}
	if v := persistedSettings["brave_search_key"]; v != "" {
		cfg.LLM.BraveSearchKey = v
	}
	loadedTelegramAccounts := false
	if v := persistedSettings[config.TelegramSettingKeyAccounts]; v != "" {
		accounts, err := config.ParseTelegramAccountsJSON(v)
		if err != nil {
			log.Printf("navi: failed to parse %s: %v", config.TelegramSettingKeyAccounts, err)
		} else if len(accounts) > 0 {
			cfg.Connectors.Telegram.Accounts = accounts
			loadedTelegramAccounts = true
		}
	}
	if !loadedTelegramAccounts {
		if v := persistedSettings[config.TelegramSettingKeyBotToken]; v != "" && cfg.Connectors.Telegram.BotToken == "" {
			cfg.Connectors.Telegram.BotToken = v
		}
		if v := persistedSettings[config.TelegramSettingKeyOwnerChatID]; v != "" && cfg.Connectors.Telegram.OwnerChatID == 0 {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				cfg.Connectors.Telegram.OwnerChatID = id
			}
		}
	}
	if cfg.Gateway.SharedSecret == "" {
		if v := persistedSettings["gateway_shared_secret"]; v != "" {
			cfg.Gateway.SharedSecret = v
		} else {
			b := make([]byte, 16)
			rand.Read(b)
			cfg.Gateway.SharedSecret = base64.RawURLEncoding.EncodeToString(b)
			store.SetSetting(ctx, db, "gateway_shared_secret", cfg.Gateway.SharedSecret)
		}
	}

	// Boot-time onboarding: derive first-run state for CLI and backend web.
	onboardingStatus, err := onboarding.DeriveFirstRunStatus(ctx, db, dataDir)
	if err != nil {
		slog.Warn("onboarding state derivation failed", "error", err)
		_ = store.SetSetting(ctx, db, onboarding.SettingKeyOnboardingMode, "true")
	} else {
		_ = store.SetSetting(ctx, db, onboarding.SettingKeyFirstRunState, string(onboardingStatus.State))
		if onboardingStatus.State != onboarding.FirstRunComplete {
			_ = store.SetSetting(ctx, db, onboarding.SettingKeyOnboardingMode, "true")
		} else {
			_ = store.SetSetting(ctx, db, onboarding.SettingKeyOnboardingMode, "false")
		}
	}
	if onboardingStatus.State == onboarding.FirstRunUninitialized {
		logBanner(func(s string) { slog.Info(s) }, "First Boot", "")
		slog.Info("No owner has been configured yet.")
		slog.Info("")
		slog.Info(fmt.Sprintf("Open:  %s/onboarding  in your browser", localGatewayURL(cfg.Gateway.Addr)))
		slog.Info("Or run:  navi init  in a separate terminal")
		slog.Info("")
		slog.Info("The first person to complete setup becomes the owner.")
	} else if onboardingStatus.State == onboarding.FirstRunRecoveryCreated {
		slog.Info("Onboarding recovery has been created. Continue setup in the browser or run: navi init")
	} else if onboardingStatus.State == onboarding.FirstRunProviderConfigured {
		slog.Info("Onboarding provider is configured. Finish setup in the browser or run: navi init")
	}
	if v := persistedSettings["llm_ollama_model"]; v != "" {
		if !strings.Contains(v, "qwen") || cfg.LLM.OllamaModel == "" {
			cfg.LLM.OllamaModel = v
		} else if cfg.LLM.OllamaModel != "" {
			_ = store.SetSetting(ctx, db, "llm_ollama_model", cfg.LLM.OllamaModel)
		}
	}

	// Initialize World Model façade.
	wm := worldmodel.New(db)
	if auditReport, err := store.DetectProjectReferenceOrphans(ctx, db); err != nil {
		slog.Warn("project reference audit failed", "error", err)
	} else if auditReport.HasOrphans() {
		slog.Warn("project reference audit found orphaned rows", "count", len(auditReport.Orphans))
		for _, orphan := range auditReport.Orphans {
			slog.Warn("orphaned project reference",
				"table", orphan.Table,
				"row_id", orphan.RowID,
				"field", orphan.Field,
				"project_id", orphan.ProjectID,
				"reason", orphan.Reason,
			)
		}
	}

	// Start embedded NATS when configured (default for local dev).
	if cfg.NATS.URL == "embedded" {
		dataDir := filepath.Dir(cfg.SQLite.Path)
		if dataDir == "" || dataDir == "." {
			dataDir = "."
		}
		log.Println("Starting embedded NATS server...")
		embeddedSrv, err := bus.StartEmbedded(dataDir)
		if err != nil {
			log.Fatalf("failed to start embedded NATS: %v", err)
		}
		defer embeddedSrv.Shutdown()
		cfg.NATS.URL = embeddedSrv.ClientURL()
		log.Printf("Embedded NATS ready at %s", cfg.NATS.URL)
	}

	// Connect to NATS.
	log.Printf("Connecting to NATS at %s...", cfg.NATS.URL)
	natsBus, err := bus.Connect(cfg.NATS.URL, db)
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}

	saveErrorRecord := func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
		return store.SaveErrorRecord(ctx, db, store.ErrorRecord{
			Component:    component,
			ChatID:       chatID,
			RunID:        runID,
			ErrorType:    errorType,
			ErrorMessage: message,
			ContextJSON:  contextJSON,
		})
	}

	connectorRegistry := connectors.NewRegistry()
	connectorMgr := connectors.NewManager(connectorRegistry, connectors.ManagerConfig{
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			return store.SaveExecutionOutcome(ctx, db, eo)
		},
		SaveErrorRecord: saveErrorRecord,
		RecordDeliveryAttempt: func(ctx context.Context, deliveryID, status string, attemptDelta int, lastError string) error {
			_, err := navistore.NewSQLiteStore(db).UpdateMessageDelivery(ctx, deliveryID, navi.UpdateMessageDeliveryInput{
				Status:       navi.MessageDeliveryStatus(status),
				AttemptDelta: attemptDelta,
				LastError:    lastError,
			})
			return err
		},
	})

	pluginRegistry := plugin.NewRegistry(nil)
	plugin.RegisterBuiltin(pluginRegistry)
	pluginLoader := plugin.NewLoader(
		filepath.Join(cfg.Navi.WorkspaceDir, "plugins"),
		"",
		"plugins",
	)
	if err := pluginLoader.LoadIntoRegistry(pluginRegistry, connectorRegistry); err != nil {
		log.Printf("plugin loader: %v", err)
	}
	activeConnectorIDs := pluginRegistry.ActivePluginConnectorIDs()
	llm.SetActiveProviderKeys(pluginRegistry.ActivePluginProviderIDs())

	// CIP P1: intake emitter publishes serialised IntakeRecords to NAVI_REFINERY.
	intakeEmitter := intake.NewEmitter(natsBus.JS)
	intakeEmitFn := func(_ context.Context, data []byte) error {
		return intakeEmitter.EmitBytes(data)
	}

	telegramPluginActive := activeConnectorIDs["telegram"]
	registerTelegramFactory := func(acct config.ResolvedTelegramAccount) {
		if !telegramPluginActive {
			return
		}
		registerTelegramAccountFactory(connectorRegistry, acct, saveErrorRecord, intakeEmitFn)
	}

	var telegramConnectorNames []string
	if telegramPluginActive {
		registerTelegramSetupFactory(connectorRegistry, saveErrorRecord, intakeEmitFn)
		telegramAccounts, err := config.ResolveTelegramAccounts(cfg.Connectors.Telegram)
		if err != nil {
			log.Fatalf("invalid telegram configuration: %v", err)
		}
		if len(telegramAccounts) > 0 {
			cfg.Connectors.Telegram.Accounts = resolvedToTelegramAccounts(telegramAccounts)
		}
		telegramConnectorNames = make([]string, 0, len(telegramAccounts))
		for _, acct := range telegramAccounts {
			registerTelegramFactory(acct)
			telegramConnectorNames = append(telegramConnectorNames, acct.ConnectorName)
		}
	}

	if activeConnectorIDs["slack"] {
		registerSlackFactory(connectorRegistry, saveErrorRecord)
	}

	// Enrich builtin connector drivers with metadata from active plugin manifests.
	for _, m := range pluginRegistry.Manifests() {
		if m.Connector == nil || !activeConnectorIDs[m.Connector.DriverID] {
			continue
		}
		if m.Connector.Kind == "builtin" {
			// manifestDriver implements the rich metadata interface.
			d := plugin.DriverFromManifest(m)
			connectorRegistry.ExtendDriver(m.Connector.DriverID, d.SetupDescriptor(), d.Capabilities())
		}
	}

	// Build LLM provider chain. Ollama is always available as a fallback if running.
	baseProvider, provErr := llm.FromConfig(cfg)
	if provErr != nil {
		log.Printf("Warning: LLM not fully configured (%v) — configure with navi init or /onboarding.", provErr)
	}
	llmSettingStore := llm.NewSettingStore(
		func(ctx context.Context, key string) (string, bool, error) {
			return store.GetSetting(ctx, db, key)
		},
		func(ctx context.Context, key, value string) error {
			return store.SetSetting(ctx, db, key, value)
		},
	)
	cp, err := llm.NewControlPlane(llm.ControlPlaneOptions{
		Config:       cfg,
		SettingStore: llmSettingStore,
		BaseProvider: baseProvider,
	})
	if err != nil {
		log.Fatalf("LLM control plane: %v", err)
	}
	dp := cp.DynamicProvider()

	// startConnector persists connector config and starts the connector via the registry.
	startConnector := makeStartConnector(cfg, db, natsBus, connectorRegistry, connectorMgr, registerTelegramFactory, activeConnectorIDs)

	// Build directive adapter and governor.
	orchestratorAdapter, err := orchestrator.NewLLMDirectiveAdapter(dp, "chat", db, wm, cfg.Navi.WorkspaceDir, nil)
	if err != nil {
		log.Fatalf("orchestrator adapter: %v", err)
	}
	orchestratorAdapter.SetSaveExecutionOutcome(func(ctx context.Context, eo schema.ExecutionOutcome) error {
		return store.SaveExecutionOutcome(ctx, db, eo)
	})
	govCfg := governor.GovernorConfig{
		MaxActionBudget:    cfg.Governor.MaxActionBudget,
		MaxRetries:         cfg.Governor.MaxRetries,
		CostCeiling:        cfg.Governor.CostCeiling,
		AutonomousDuration: 72 * time.Hour,
		MaxRepetitions:     cfg.Governor.MaxRepetitions,
	}
	if govCfg.MaxActionBudget <= 0 {
		govCfg.MaxActionBudget = 1000
	}
	if govCfg.MaxRetries < 0 {
		govCfg.MaxRetries = 5
	}
	if govCfg.CostCeiling <= 0 {
		govCfg.CostCeiling = 50.0
	}
	if cfg.Governor.AutonomousDuration != "" {
		if d, err := time.ParseDuration(cfg.Governor.AutonomousDuration); err == nil {
			govCfg.AutonomousDuration = d
		}
	}
	gov := governor.NewGovernor(govCfg, cfg.Navi.WorkspaceDir)

	// Build and start NAVI agent.
	log.Println("Starting NAVI Agent...")

	compReg := store.NewCompensatorRegistry()
	compReg.Register(schema.CommandTypeCreate, &store.CreateCompensator{DB: db})
	directiveWriter := cognitive.StoreDirectiveWriter(db)
	// Shared SQLite store: backs ChatStore + RuntimeSessionStore + runtime store
	// + ICS state + compaction. Also wrapped as a cron ChatAppender so
	// auto-promoted send_reply jobs deliver their payload as an assistant
	// message in the target chat when they fire.
	chatStore := navistore.NewSQLiteStore(db)
	chatAppender := cronChatAppender{store: chatStore}
	var hbService *heartbeat.HeartbeatService
	cronSvc := cpkg.NewService(cpkg.Deps{
		DB:              db,
		DirectiveWriter: directiveWriter,
		ChatAppender:    chatAppender,
		Log:             slog.Default(),
		Cron:            cfg.EffectiveCronConfig(),
		Now:             time.Now,
		OnFailureAlert: func(alertCtx context.Context, job cpkg.Job, text string) {
			title := strings.TrimSpace(job.Name)
			if title == "" {
				title = job.ID
			}
			d := schema.NewDirective("Cron failure: "+title, schema.DirectiveModeAdvise, "scheduler")
			if err := directiveWriter.SaveDirective(alertCtx, d); err != nil {
				slog.Warn("cron: failure alert directive", "job_id", job.ID, "error", err)
				return
			}
			if err := directiveWriter.AppendMessage(alertCtx, schema.NewDirectiveMessage(d.DirectiveID, "owner", text)); err != nil {
				slog.Warn("cron: failure alert message", "job_id", job.ID, "error", err)
				return
			}
			if hbService != nil {
				hbService.RequestWake("cron failure alert: "+title, int(heartbeat.WakePriorityRetry), "")
			}
		},
	})
	executionRecorder := cognitive.StoreExecutionRecorder(db, compReg.CompensatorFor)
	llmCallTimeout := time.Duration(0)
	if raw := strings.TrimSpace(cfg.Navi.LLMCallTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			log.Printf("navi: invalid llm_call_timeout %q; using default foreground timeout: %v", raw, err)
		} else {
			llmCallTimeout = parsed
		}
	}
	effectiveLLMCallTimeout := llmCallTimeout
	if effectiveLLMCallTimeout <= 0 {
		effectiveLLMCallTimeout = 5 * time.Minute
	}
	log.Printf("navi: effective llm_call_timeout=%s coordinator_run_timeout=%s", effectiveLLMCallTimeout, effectiveLLMCallTimeout+30*time.Second)

	naviConfig := navi.Config{
		LLM:                   dp,
		Bus:                   natsBus,
		Chats:                 chatStore,
		RuntimeSessions:       chatStore,
		ConversationEndpoints: chatStore,
		ConnectorDispatcher:   connectorMgr,
		DB:                    db,
		WorldModel:            wm,
		SkillsDir:             "skills",
		BraveSearchKey:        cfg.LLM.BraveSearchKey,
		ExperienceProfilesDir: cfg.Navi.ExperienceProfilesDir,
		WorkspaceDir:          cfg.Navi.WorkspaceDir,
		ArtifactsDir:          cfg.Navi.ArtifactsDir,
		Governor:              gov,

		InitialExperienceMode: navi.NormalizeExperienceMode(navi.ExperienceMode(cfg.Navi.InitialExperienceMode)),
		Model:                 "chat",
		LLMCallTimeout:        llmCallTimeout,

		ListLLMs: func(ctx context.Context) (any, error) {
			return cp.Catalog(ctx)
		},
		GetActiveLLM: func(ctx context.Context) (provider, model string, err error) {
			active, err := cp.GetActive(ctx)
			if err != nil {
				return "", "", err
			}
			return active.Provider, active.Model, nil
		},
		SetActiveLLM: func(ctx context.Context, provider, model string) (string, string, error) {
			active, err := cp.SetActive(ctx, provider, model)
			if err != nil {
				return "", "", err
			}
			return active.Provider, active.Model, nil
		},
		RouteLLM: cp.Route,
		IsSetupDone: func(ctx context.Context) bool {
			val, found, _ := store.GetSetting(ctx, db, "setup_complete")
			return found && val == "true"
		},
		OnStartConnector: startConnector,
		// CIP P4: provenance-bearing retrieval feeds the Contextualize step. The
		// embedder must match the one synthesis persisted with (stub-hash-v1) so the
		// brute-force cosine index queries the same vector space.
		ContextRetriever: &retrieve.ContextProvider{
			DB:       db,
			Embedder: embed.NewStubEmbedder(),
			Log:      slog.Default(),
		},
		FactsBlock: func(ctx context.Context, chatID string) (string, error) {
			ownerID, err := store.GetOwnerID(ctx, db)
			if err != nil || ownerID == "" {
				return "", err
			}
			return wm.ContextBlockForChat(ctx, ownerID, chatID, 30, 20, 50, 20)
		},
		SaveProposal: func(ctx context.Context, p schema.Proposal) error { return store.SaveProposal(ctx, db, p) },
		GetProposal:  func(ctx context.Context, id string) (schema.Proposal, error) { return store.GetProposal(ctx, db, id) },
		ResolveProposal: func(ctx context.Context, id string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, note string) error {
			if status == schema.ProposalStatusApproved {
				p, err := store.GetProposal(ctx, db, id)
				if err != nil {
					return err
				}
				if p.ProposedAction == "tombstone" && p.AffectedEntities != "" {
					var refs []string
					if err := json.Unmarshal([]byte(p.AffectedEntities), &refs); err == nil {
						exec := command.NewExecutor(func(ctx context.Context, eo schema.ExecutionOutcome) error {
							return store.SaveExecutionOutcome(ctx, db, eo)
						})
						for _, ref := range refs {
							ref = strings.TrimSpace(ref)
							idx := strings.Index(ref, ":")
							if idx <= 0 || idx == len(ref)-1 {
								continue
							}
							entityType, entityID := ref[:idx], ref[idx+1:]
							_, err := exec.Execute(ctx, command.Descriptor{Type: schema.CommandTypeDelete}, func(ctx context.Context) (any, error) {
								return nil, wm.TombstoneEntity(ctx, entityType, entityID, id)
							})
							if err != nil {
								return fmt.Errorf("tombstone %s: %w", ref, err)
							}
						}
					}
				}
			}
			return store.ResolveProposal(ctx, db, id, status, resolutionType, resolvedBy, note)
		},
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			if err := store.SaveExecutionOutcome(ctx, db, eo); err != nil {
				return err
			}
			if eo.CompensationRequired {
				_ = store.RunCompensation(ctx, db, eo, compReg.CompensatorFor(eo))
			}
			return nil
		},
		SaveErrorRecord:         saveErrorRecord,
		AutonomyResolver:        &cfg.Autonomy,
		DomainForSkill:          func(skillName string) string { return "general" },
		GovConfigPriorityReader: worldModelConfigPriorityReader{wm: wm},
		ResolveOwnerID:          func(ctx context.Context, _ string) string { id, _ := store.GetOwnerID(ctx, db); return id },
		ConnectorHealth: func() []navi.ConnectorHealthInfo {
			var out []navi.ConnectorHealthInfo
			for _, h := range connectorMgr.Health() {
				state := h.Status
				switch state {
				case "healthy", "connected", "configured":
					state = "running"
				case "down":
					state = "error"
				}
				out = append(out, navi.ConnectorHealthInfo{Name: h.Name, Status: state})
			}
			return out
		},
		PluginRegistry:              pluginRegistry,
		RegisterPluginSkillHandlers: registerBuiltinSkillHandlers,
		Debug:                       cfg.Navi.Debug,
		Cron:                        cronSvc,
	}
	naviAgent, err := navi.New(naviConfig)
	if err != nil {
		log.Fatalf("failed to initialize NAVI: %v", err)
	}

	naviCtx, naviCancel := context.WithCancel(ctx)
	if err := naviAgent.Start(naviCtx); err != nil {
		log.Fatalf("failed to start NAVI: %v", err)
	}

	// Heartbeat service queues HEARTBEAT.md as an internal runtime signal without
	// appending to user-facing chat.
	if cfg.Navi.HeartbeatEnabled {
		hbInterval, err := time.ParseDuration(cfg.Navi.HeartbeatInterval)
		if err != nil {
			log.Printf("heartbeat: invalid interval %q, using default 30m: %v", cfg.Navi.HeartbeatInterval, err)
			hbInterval = 30 * time.Minute
		}
		hbService = heartbeat.New(cfg.Navi.WorkspaceDir, hbInterval)
		hbService.SetBus(natsBus)
		hbService.SetHandler(func(hbCtx context.Context, runtimeSessionID, prompt string) error {
			_, err := naviAgent.SendSignal(hbCtx, &naviruntime.InboxItem{
				RuntimeSessionID: runtimeSessionID,
				SourceChannel:    "heartbeat",
				ActorType:        "system",
				PayloadType:      "text",
				QueueAction:      "append",
				Status:           naviruntime.InboxStatusPending,
				Content:          prompt,
				CorrelationID:    runtimeSessionID,
			})
			return err
		})
		log.Printf("Starting Heartbeat Service (interval=%s, workspace=%s)...", hbInterval, cfg.Navi.WorkspaceDir)
		hbService.Start(ctx)
	}

	if hbService != nil {
		cronSvc.SetHeartbeat(hbService)
	}
	if err := cronSvc.Start(ctx); err != nil {
		log.Fatalf("cron: %v", err)
	}

	// Backlog poller — when no active directives, creates one from the first PENDING task in the backlog file.
	backlogPath := os.Getenv("NAVI_BACKLOG_PATH")
	if backlogPath == "" {
		backlogPath = "docs/tasks/autonomous-agent-readiness-backlog.md"
	}
	backlogInterval := 30 * time.Minute
	if cfg.Navi.HeartbeatEnabled {
		if d, err := time.ParseDuration(cfg.Navi.HeartbeatInterval); err == nil {
			backlogInterval = d
		}
	}
	go func() {
		ticker := time.NewTicker(backlogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := backlog.CreateDirectiveFromBacklog(ctx, db, directiveWriter, backlogPath); err != nil {
					slog.Warn("backlog poller", "error", err)
				}
				// Proposal janitor — periodically expires old proposals.
				if count, err := store.ExpireProposals(ctx, db); err != nil {
					slog.Warn("proposal janitor", "error", err)
				} else if count > 0 {
					slog.Info("proposal janitor: expired old proposals", "count", count)
				}
				// Execution outcome retention — delete rows older than configured days.
				if cfg.Governor.ExecutionOutcomeRetentionDays > 0 {
					cutoff := time.Now().UTC().AddDate(0, 0, -cfg.Governor.ExecutionOutcomeRetentionDays)
					if n, err := store.DeleteExecutionOutcomesOlderThan(ctx, db, cutoff); err != nil {
						slog.Warn("execution outcome retention", "error", err)
					} else if n > 0 {
						slog.Info("execution outcome retention: deleted old rows", "count", n)
					}
				}
			}
		}
	}()

	// Orchestrator loop — drives the directive conversation loop.
	log.Println("Starting Orchestrator...")
	orchestratorLoopCtx, orchestratorLoopCancel := context.WithCancel(ctx)
	orchestratorConfig := orchestrator.LoopConfig{
		Bus:          natsBus,
		LLM:          orchestratorAdapter,
		Governor:     gov,
		TickInterval: 2 * time.Second,
	}
	go func() {
		if err := orchestrator.RunLoop(orchestratorLoopCtx, orchestratorConfig); err != nil {
			log.Printf("Orchestrator loop stopped: %v", err)
		}
	}()

	// workerGovernanceOK routes worker execution through ICS and blocks the task only when ICS emits a blocked/pause disposition.
	workerGovernanceOK := func(ctx context.Context, agent *navi.NAVI, task schema.Task, actorKind string, cmdType schema.CommandType, domain string) bool {
		decision, err := agent.AuthorizeWorkerTask(ctx, task, actorKind, cmdType, domain)
		if err != nil {
			task.Status = schema.TaskStatusBlocked
			task.UpdatedAt = time.Now().UTC()
			_ = executionRecorder.UpdateTask(ctx, task)
			slog.Warn("worker ICS authorization failed", "task_id", task.ID, "actor", actorKind, "error", err)
			return false
		}
		if decision.RuntimeDisposition != "pause_for_proposal" && decision.RuntimeDisposition != "block_with_reply" {
			return true
		}

		proposalID := ""
		if decision.Proposal != nil {
			proposalID = decision.Proposal.ProposalID
		} else {
			proposalID = decision.Rationale.DecisionTrace.ProposalID
		}
		reason := strings.TrimSpace(decision.ReplyMessage)
		if reason == "" {
			reason = strings.TrimSpace(decision.Rationale.DecisionTrace.GovernanceOutcome)
		}
		task.Status = schema.TaskStatusBlocked
		task.UpdatedAt = time.Now().UTC()
		_ = executionRecorder.UpdateTask(ctx, task)
		slog.Debug(
			"worker ICS blocked task",
			"task_id", task.ID,
			"actor", actorKind,
			"disposition", decision.RuntimeDisposition,
			"proposal_id", proposalID,
			"reason", reason,
		)
		return false
	}

	// Coder worker — subscribes to CmdTaskAssign and executes tasks with file tools + LLM.
	coderRunner := coder.NewRunner(coder.Config{
		WorkspaceDir:      cfg.Navi.WorkspaceDir,
		Governor:          gov,
		LLM:               dp,
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: executionRecorder,
		CompensatorFor:    compReg.CompensatorFor,
	})
	if err := natsBus.Subscribe(ctx, schema.CmdTaskAssign, "coder-worker", func(ctx context.Context, ev schema.Event) {
		data, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("coder: marshal payload", "error", err)
			return
		}
		var payload schema.TaskAssignedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("coder: unmarshal TaskAssignedPayload", "error", err)
			return
		}
		if payload.Task.AssignedTo != schema.AgentCoder {
			return
		}
		if naviAgent != nil && !workerGovernanceOK(ctx, naviAgent, payload.Task, "coder-worker", schema.CommandTypeDelegate, governor.AutonomyDomainCoding) {
			return
		}
		taskStatus := schema.TaskStatusCompleted
		if err := coderRunner.Execute(ctx, payload.Task); err != nil {
			slog.Warn("coder: execute task failed", "task_id", payload.Task.ID, "error", err)
			payload.Task.Status = schema.TaskStatusFailed
			payload.Task.UpdatedAt = time.Now().UTC()
			_ = executionRecorder.UpdateTask(ctx, payload.Task)
			taskStatus = schema.TaskStatusFailed
		}
		if wm != nil {
			ownerID, _ := store.GetOwnerID(ctx, db)
			_ = wm.RecordInteractionEvent(ctx, ownerID, "", map[string]any{
				"agent": "coder", "task_id": payload.Task.ID, "directive_id": payload.Task.DirectiveID,
				"status": taskStatus,
			})
		}
	}); err != nil {
		log.Printf("coder worker subscribe: %v", err)
	}

	criticRunner := critic.NewRunner(critic.Config{
		WorkspaceDir:      cfg.Navi.WorkspaceDir,
		Governor:          gov,
		LLM:               dp,
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: executionRecorder,
		DirectiveWriter:   directiveWriter,
		CompensatorFor:    compReg.CompensatorFor,
	})
	if err := natsBus.Subscribe(ctx, schema.CmdTaskAssign, "critic-worker", func(ctx context.Context, ev schema.Event) {
		data, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("critic: marshal payload", "error", err)
			return
		}
		var payload schema.TaskAssignedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("critic: unmarshal TaskAssignedPayload", "error", err)
			return
		}
		if payload.Task.AssignedTo != schema.AgentCritic {
			return
		}
		if naviAgent != nil && !workerGovernanceOK(ctx, naviAgent, payload.Task, "critic-worker", schema.CommandTypeDelegate, governor.AutonomyDomainCoding) {
			return
		}
		taskStatus := schema.TaskStatusCompleted
		if err := criticRunner.Execute(ctx, payload.Task); err != nil {
			slog.Warn("critic: execute task failed", "task_id", payload.Task.ID, "error", err)
			payload.Task.Status = schema.TaskStatusFailed
			payload.Task.UpdatedAt = time.Now().UTC()
			_ = executionRecorder.UpdateTask(ctx, payload.Task)
			taskStatus = schema.TaskStatusFailed
		}
		if wm != nil {
			ownerID, _ := store.GetOwnerID(ctx, db)
			_ = wm.RecordInteractionEvent(ctx, ownerID, "", map[string]any{
				"agent": "critic", "task_id": payload.Task.ID, "directive_id": payload.Task.DirectiveID,
				"status": taskStatus,
			})
		}
	}); err != nil {
		log.Printf("critic worker subscribe: %v", err)
	}

	strategistRunner := strategist.NewRunner(strategist.Config{
		LLM:               dp,
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: executionRecorder,
		DirectiveWriter:   directiveWriter,
		CompensatorFor:    compReg.CompensatorFor,
	})
	if err := natsBus.Subscribe(ctx, schema.CmdTaskAssign, "strategist-worker", func(ctx context.Context, ev schema.Event) {
		data, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("strategist: marshal payload", "error", err)
			return
		}
		var payload schema.TaskAssignedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("strategist: unmarshal TaskAssignedPayload", "error", err)
			return
		}
		if payload.Task.AssignedTo != schema.AgentStrategist {
			return
		}
		if naviAgent != nil && !workerGovernanceOK(ctx, naviAgent, payload.Task, "strategist-worker", schema.CommandTypeDelegate, governor.AutonomyDomainCoding) {
			return
		}
		taskStatus := schema.TaskStatusCompleted
		if err := strategistRunner.Execute(ctx, payload.Task); err != nil {
			slog.Warn("strategist: execute task failed", "task_id", payload.Task.ID, "error", err)
			payload.Task.Status = schema.TaskStatusFailed
			payload.Task.UpdatedAt = time.Now().UTC()
			_ = executionRecorder.UpdateTask(ctx, payload.Task)
			taskStatus = schema.TaskStatusFailed
		}
		if wm != nil {
			ownerID, _ := store.GetOwnerID(ctx, db)
			_ = wm.RecordInteractionEvent(ctx, ownerID, "", map[string]any{
				"agent": "strategist", "task_id": payload.Task.ID, "directive_id": payload.Task.DirectiveID,
				"status": taskStatus,
			})
		}
	}); err != nil {
		log.Printf("strategist worker subscribe: %v", err)
	}

	scoutRunner := scout.NewRunner(scout.Config{
		LLM:               dp,
		Model:             "chat",
		DB:                db,
		Skills:            naviAgent.Skills(),
		ExecutionRecorder: executionRecorder,
		DirectiveWriter:   directiveWriter,
		CompensatorFor:    compReg.CompensatorFor,
	})
	if err := natsBus.Subscribe(ctx, schema.CmdTaskAssign, "scout-worker", func(ctx context.Context, ev schema.Event) {
		data, err := json.Marshal(ev.Payload)
		if err != nil {
			slog.Warn("scout: marshal payload", "error", err)
			return
		}
		var payload schema.TaskAssignedPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("scout: unmarshal TaskAssignedPayload", "error", err)
			return
		}
		if payload.Task.AssignedTo != schema.AgentScout {
			return
		}
		if naviAgent != nil && !workerGovernanceOK(ctx, naviAgent, payload.Task, "scout-worker", schema.CommandTypeDelegate, governor.AutonomyDomainMemory) {
			return
		}
		taskStatus := schema.TaskStatusCompleted
		if err := scoutRunner.Execute(ctx, payload.Task); err != nil {
			slog.Warn("scout: execute task failed", "task_id", payload.Task.ID, "error", err)
			payload.Task.Status = schema.TaskStatusFailed
			payload.Task.UpdatedAt = time.Now().UTC()
			_ = executionRecorder.UpdateTask(ctx, payload.Task)
			taskStatus = schema.TaskStatusFailed
		}
		if wm != nil {
			ownerID, _ := store.GetOwnerID(ctx, db)
			_ = wm.RecordInteractionEvent(ctx, ownerID, "", map[string]any{
				"agent": "scout", "task_id": payload.Task.ID, "directive_id": payload.Task.DirectiveID,
				"status": taskStatus,
			})
		}
	}); err != nil {
		log.Printf("scout worker subscribe: %v", err)
	}

	// Reflection worker — consumes navi.fact.reflection_queued (Conscious → Subconscious).
	refWorker := reflection.NewWorker(wm)
	refWorker.SetSaveProposal(func(ctx context.Context, p schema.Proposal) error {
		return store.SaveProposal(ctx, db, p)
	})
	refWorker.SetExperienceControlBuilder(func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error) {
		if naviAgent == nil {
			engine := experience.NewEngine(experience.NewStoreGovernanceBoundsProvider(db))
			return engine.Build(ctx, experience.DefaultStandardProfile(), req)
		}
		return naviAgent.BuildExperienceControl(ctx, navi.ExperienceMode(mode), req)
	})
	refWorker.SetContradictionChecker(reflection.NewDefaultContradictionChecker(
		func(ctx context.Context, directiveID string, limit int) ([]schema.DirectiveMessage, error) {
			return store.GetMessages(ctx, db, directiveID, limit)
		}, 10))
	refWorker.SetSaveExecutionOutcome(func(ctx context.Context, eo schema.ExecutionOutcome) error {
		return store.SaveExecutionOutcome(ctx, db, eo)
	})
	go func() {
		if err := refWorker.Run(ctx, natsBus); err != nil {
			slog.Warn("reflection worker", "error", err)
		}
	}()

	// CIP P1: intake worker — consumes navi.refinery.queue, runs Admit, dedupes, persists.
	intakeWorker := intake.NewWorker(natsBus.JS, db, gov.RecordAction, slog.Default())
	// CIP P3: enable the synthesis stages (Score → Embed → Extract → Resolve →
	// Synthesize → Fold). Steady-state intake runs in Delta mode (per-write
	// Proposals). All World Model writes flow through the governor via the
	// world_model_mutation effect, with autonomy resolved by cfg.Autonomy.
	intakeWorker.SetPipeline(intake.PipelineConfig{
		Synthesis: intake.NewSynthesisConfig(db, governor.MutationPipelineOptions{
			Resolver: &cfg.Autonomy,
		}, schema.JobModeDelta, slog.Default()),
	})
	// CIP P5: per-connector sync policy (defaults + config/connectors YAML +
	// runtime overrides) and per-connector cost-ceiling enforcement via the
	// existing Governor. Policy supplies the inputs; the Governor decides.
	intakePolicyFiles, err := intakepolicy.LoadDir("config/connectors")
	if err != nil {
		slog.Warn("intake: failed to load connector policy files; using defaults", "error", err)
		intakePolicyFiles = nil
	}
	intakePolicyResolver := intakepolicy.NewResolver(db, intakePolicyFiles)
	intakeWorker.SetPolicy(intakePolicyResolver, gov.RecordConnectorCost, gov.ResetConnectorCost)
	go func() {
		if err := intakeWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("intake worker stopped", "error", err)
		}
	}()

	// NAVI-VAULT-V1: Memory Vault — the second mouth on the intake pipeline. It
	// projects the World Model to owner-editable Markdown and round-trips owner
	// edits through the SAME governor seam intake uses (governor.EvaluateMutation),
	// tagged ContentTrust: owner. Owner edits become structured diffs, never raw
	// writes; hard floors (merge/deprecate/forget) still raise Proposals.
	if cfg.Vault.Enabled {
		vaultWorker, err := startVaultWorker(ctx, cfg, db)
		if err != nil {
			slog.Warn("vault worker not started", "error", err)
		} else if vaultWorker != nil {
			defer vaultWorker.Close()
		}
	}

	// Gateway server.
	log.Printf("Starting API Gateway on %s...", cfg.Gateway.Addr)
	gwConfig := gateway.Config{
		DB:                    db,
		DirectiveWriter:       directiveWriter,
		DataDir:               dataDir,
		WorkspaceDir:          cfg.Navi.WorkspaceDir,
		Bus:                   natsBus,
		Governor:              gov,
		IntakePolicies:        intakePolicyResolver,
		Registry:              connectorRegistry,
		Manager:               connectorMgr,
		ConversationEndpoints: chatStore,
		ConnectorDispatcher:   connectorMgr,
		Navi:                  naviAgent,
		SkillRegistry:         naviAgent.Skills(),
		WorldModel:            wm,
		RefWorker:             refWorker,
		PluginManifests:       func() []plugin.Manifest { return pluginRegistry.Manifests() },
		SetPluginEnabled: func(pluginID string, enabled bool) error {
			pluginRegistry.SetPluginEnabled(pluginID, enabled)
			return nil
		},
		ReloadPlugins: func() error {
			return pluginLoader.LoadIntoRegistry(pluginRegistry, connectorRegistry)
		},
		IsPluginEnabled:           pluginRegistry.IsPluginEnabled,
		OriginPatterns:            cfg.Gateway.OriginPatterns,
		Addr:                      cfg.Gateway.Addr,
		StaticDir:                 cfg.Gateway.StaticDir,
		Version:                   version,
		Build:                     build,
		SharedSecret:              cfg.Gateway.SharedSecret,
		SaveErrorRecord:           saveErrorRecord,
		LLM:                       cp,
		SetupSchema:               connectorRegistry.SetupDescriptors(),
		ConnectorSetupDescriptors: connectorRegistry.SetupDescriptors,
		ExperienceBuilder: func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error) {
			if naviAgent == nil {
				engine := experience.NewEngine(experience.NewStoreGovernanceBoundsProvider(db))
				return engine.Build(ctx, experience.DefaultStandardProfile(), req)
			}
			return naviAgent.BuildExperienceControl(ctx, navi.ExperienceMode(mode), req)
		},
		Presence:         naviAgent.PresenceService(),
		OnSetupConnector: startConnector,
	}
	gwServer := gateway.NewServer(gwConfig)
	go func() {
		if err := gwServer.Start(); err != nil {
			log.Fatalf("Gateway server failed: %v", err)
		}
	}()

	waitForGateway(cfg.Gateway.Addr, 10*time.Second)

	for _, connectorName := range telegramConnectorNames {
		if err := connectorRegistry.Create(connectorName, cfg, natsBus, nil); err != nil {
			log.Printf("create %s connector: %v", connectorName, err)
		}
	}
	if cfg.Connectors.Slack.BotToken != "" {
		if err := connectorRegistry.Create("slack", cfg, natsBus, nil); err != nil {
			log.Printf("create slack connector: %v", err)
		}
	}
	connectorMgr.StartAll(ctx)
	connectorMgr.StartJanitor(ctx)

	// Wait for termination signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v. Initiating graceful shutdown...", sig)

	// Graceful shutdown.
	shutdownCtx, sdCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sdCancel()
	log.Println("Shutting down connectors...")
	connectorMgr.StopAll(shutdownCtx, 15*time.Second)

	log.Println("Shutting down Gateway...")
	gwServer.Stop(shutdownCtx)

	log.Println("Shutting down Orchestrator...")
	orchestratorLoopCancel()

	log.Println("Shutting down Cron...")
	cronSvc.Stop()

	if hbService != nil {
		hbService.Stop()
	}

	log.Println("Shutting down NAVI Agent...")
	naviAgent.Stop()
	naviCancel()

	log.Println("Shutting down NATS connection...")
	natsBus.Close()

	log.Println("Shutting down SQLite...")
	db.Close()

	log.Println("Shutdown complete.")
}

func nonEmptyOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func localGatewayURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "http://localhost:6284"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func parseTelegramChatIDsCSV(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		idStr := strings.TrimSpace(part)
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func upsertTelegramAccount(accounts []config.TelegramAccountConfig, account config.TelegramAccountConfig) []config.TelegramAccountConfig {
	targetConnector := config.TelegramConnectorName(account.Name)
	next := make([]config.TelegramAccountConfig, 0, len(accounts)+1)
	replaced := false
	for _, existing := range accounts {
		if config.TelegramConnectorName(existing.Name) == targetConnector {
			next = append(next, account)
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if !replaced {
		next = append(next, account)
	}
	return next
}

func resolvedToTelegramAccounts(accounts []config.ResolvedTelegramAccount) []config.TelegramAccountConfig {
	out := make([]config.TelegramAccountConfig, 0, len(accounts))
	for _, acct := range accounts {
		out = append(out, config.TelegramAccountConfig{
			Name:          acct.AccountName,
			BotToken:      acct.BotToken,
			OwnerChatID:   acct.OwnerChatID,
			AllowFrom:     acct.AllowFrom,
			PairingCode:   acct.PairingCode,
			GatewayURL:    acct.GatewayURL,
			APIURL:        acct.APIURL,
			WebhookURL:    acct.WebhookURL,
			WebhookSecret: acct.WebhookSecret,
		})
	}
	return out
}

func makeStartConnector(
	cfg *config.Config,
	db *sql.DB,
	natsBus bus.Bus,
	connectorRegistry *connectors.Registry,
	connectorMgr *connectors.Manager,
	registerTelegramFactory func(config.ResolvedTelegramAccount),
	activeConnectorIDs map[string]bool,
) func(context.Context, string, map[string]string) error {
	return func(ctx context.Context, connType string, params map[string]string) error {
		if activeConnectorIDs != nil && !activeConnectorIDs[connType] {
			return fmt.Errorf("connector plugin %s is not active", connType)
		}
		switch connType {
		case "telegram":
			botToken := strings.TrimSpace(params["bot_token"])
			if botToken == "" {
				return fmt.Errorf("telegram bot_token is required")
			}

			var (
				chatID         int64
				ownerChatIDStr = strings.TrimSpace(params["owner_chat_id"])
			)
			if ownerChatIDStr != "" {
				parsed, parseErr := strconv.ParseInt(ownerChatIDStr, 10, 64)
				if parseErr != nil {
					return fmt.Errorf("invalid telegram owner_chat_id: %w", parseErr)
				}
				chatID = parsed
			}

			account := config.TelegramAccountConfig{
				Name:          strings.TrimSpace(params["account"]),
				BotToken:      botToken,
				OwnerChatID:   chatID,
				AllowFrom:     parseTelegramChatIDsCSV(params["allow_from"]),
				PairingCode:   strings.TrimSpace(params["pairing_code"]),
				GatewayURL:    nonEmptyOr(strings.TrimSpace(params["gateway_url"]), cfg.Connectors.Telegram.GatewayURL),
				APIURL:        nonEmptyOr(strings.TrimSpace(params["api_url"]), cfg.Connectors.Telegram.APIURL),
				WebhookURL:    strings.TrimSpace(params["webhook_url"]),
				WebhookSecret: strings.TrimSpace(params["webhook_secret"]),
			}
			nextAccounts := upsertTelegramAccount(cfg.Connectors.Telegram.Accounts, account)
			nextTelegramCfg := cfg.Connectors.Telegram
			nextTelegramCfg.Accounts = nextAccounts
			resolvedAccounts, err := config.ResolveTelegramAccounts(nextTelegramCfg)
			if err != nil {
				return err
			}

			targetConnector := ""
			for _, resolved := range resolvedAccounts {
				registerTelegramFactory(resolved)
				if resolved.BotToken == botToken {
					targetConnector = resolved.ConnectorName
				}
			}
			if targetConnector == "" {
				return fmt.Errorf("failed to resolve telegram connector name")
			}
			if connectorRegistry.Get(targetConnector) != nil {
				return fmt.Errorf("telegram connector %q already exists", targetConnector)
			}

			encodedAccounts, err := config.EncodeTelegramAccountsJSON(nextAccounts)
			if err != nil {
				return err
			}
			if err := store.SetSetting(ctx, db, config.TelegramSettingKeyAccounts, encodedAccounts); err != nil {
				return fmt.Errorf("save telegram accounts: %w", err)
			}
			cfg.Connectors.Telegram.Accounts = nextAccounts

			// Backward-compatible keys for single/default account users.
			if targetConnector == "telegram" {
				if err := store.SetSetting(ctx, db, config.TelegramSettingKeyBotToken, botToken); err != nil {
					return fmt.Errorf("save telegram token: %w", err)
				}
				if err := store.SetSetting(ctx, db, config.TelegramSettingKeyOwnerChatID, ownerChatIDStr); err != nil {
					return fmt.Errorf("save telegram chat id: %w", err)
				}
				cfg.Connectors.Telegram.BotToken = botToken
				cfg.Connectors.Telegram.OwnerChatID = chatID
			}

			if err := connectorRegistry.Create(targetConnector, cfg, natsBus, nil); err != nil {
				return err
			}
			connectorMgr.StartOne(ctx, targetConnector)
			return nil
		case "slack":
			botToken := params["bot_token"]
			appToken := params["app_token"]
			if err := store.SetSetting(ctx, db, "slack_bot_token", botToken); err != nil {
				return fmt.Errorf("save slack bot token: %w", err)
			}
			if appToken != "" {
				_ = store.SetSetting(ctx, db, "slack_app_token", appToken)
			}
			cfg.Connectors.Slack.BotToken = botToken
			cfg.Connectors.Slack.AppToken = appToken
			if err := connectorRegistry.Create("slack", cfg, natsBus, nil); err != nil {
				return err
			}
			connectorMgr.StartOne(ctx, "slack")
			return nil
		default:
			driver, ok := connectorRegistry.GetDriver(connType)
			if !ok {
				return fmt.Errorf("unknown connector type: %s", connType)
			}
			if driver.Kind() == "builtin" {
				return fmt.Errorf("builtin connector %s missing special setup logic", connType)
			}

			if err := connectorRegistry.Create(connType, cfg, natsBus, params); err != nil {
				return err
			}
			connectorMgr.StartOne(ctx, connType)
			return nil
		}
	}
}

// cronChatAppender adapts the chat store to cron's ChatAppender interface so
// promoted send_reply jobs can deliver an assistant message directly into a
// chat when their scheduled time arrives. Uses the system-channel append path
// because the cron tick has no active run.
//
// The scheduled job's chat_id payload field is treated as a chat_id for
// this tranche (legacy payload naming); the job-schema rename is deferred to
// a later tranche. TODO(chat-target-tranche): rename the cron job field
// when the payload schema is reworked.
type cronChatAppender struct {
	store *navistore.SQLiteStore
}

func (a cronChatAppender) AppendAssistantMessage(ctx context.Context, chatID, content string) error {
	if a.store == nil {
		return fmt.Errorf("cron chat appender: no store configured")
	}
	if strings.TrimSpace(chatID) == "" {
		// No chat target: do NOT fabricate a chat, do NOT use runtime_session_id
		// as a fake chat_id. The cron executor records this as a runtime/history
		// outcome instead of a transcript write.
		return cpkg.ErrNoChatTarget
	}
	if _, err := a.store.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not found") {
			return cpkg.ErrNoChatTarget
		}
		return err
	}
	_, err := a.store.AppendSystemAssistantMessage(ctx, chatID, content, "", "scheduler", string(schema.AssistantMessageKindReply))
	return err
}

func waitForGateway(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Printf("warning: gateway did not become ready within %s", timeout)
}
