package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/open-navi/navi/internal/cliui"
	"github.com/open-navi/navi/internal/coderalias"
	"github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/onboarding"
	"nhooyr.io/websocket"
)

// Config holds the CLI configuration
type Config struct {
	GatewayURL               string `json:"gateway_url"`
	APIKey                   string `json:"api_key"`      // X-API-Key for normal access
	OwnerSecret              string `json:"owner_secret"` // X-Owner-Secret for privileged operations (stored locally)
	NewRuntimeSession        bool   `json:"-"`
	ResumeLastRuntimeSession bool   `json:"-"` // if true, reuse last non-heartbeat runtime session; default false = new runtime session per CLI
	Debug                    bool   `json:"-"` // enable debug/trace output for events (default: false)
}

// RuntimeSessionInfo holds basic runtime session data.
type RuntimeSessionInfo struct {
	RuntimeSessionID string `json:"runtime_session_id"`
}

var httpClient = &http.Client{}

func main() {
	// Subcommand model:
	//   navi            -> chat (default)
	//   navi chat ...   -> interactive chat REPL
	//   navi ask  ...   -> one-shot question/answer
	//   navi status     -> show gateway / LLM status
	//   navi sessions   -> list recent runtime sessions
	//   navi models     -> inspect or change active provider/model
	//   navi logs       -> stream recent events (basic)
	//   navi daemon     -> manage the local host navid daemon
	args := os.Args[1:]
	subcommand := "chat"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subcommand = args[0]
		args = args[1:]
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	switch subcommand {
	case "chat":
		var cfg Config
		var saveFlag bool
		var shortSaveFlag bool
		fs := flag.NewFlagSet("chat", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.BoolVar(&cfg.NewRuntimeSession, "new", false, "Force create a new runtime session")
		fs.BoolVar(&cfg.ResumeLastRuntimeSession, "resume-session", false, "Reuse the last non-heartbeat runtime session instead of starting a new one")
		fs.BoolVar(&cfg.Debug, "debug", false, "Enable debug and trace output for WebSocket events")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication (X-API-Key)")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		fs.BoolVar(&shortSaveFlag, "s", false, "Alias for -save")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		saveRequested := saveFlag || shortSaveFlag
		resolveConfig(&cfg, saveRequested)
		runChat(ctx, &cfg)
	case "ask":
		var cfg Config
		var saveFlag bool
		var shortSaveFlag bool
		var message string
		fs := flag.NewFlagSet("ask", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.BoolVar(&cfg.NewRuntimeSession, "new", false, "Force create a new runtime session")
		fs.BoolVar(&cfg.ResumeLastRuntimeSession, "resume-session", false, "Reuse the last non-heartbeat runtime session instead of starting a new one")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication (X-API-Key)")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		fs.BoolVar(&shortSaveFlag, "s", false, "Alias for -save")
		fs.StringVar(&message, "m", "", "Message to send to NAVI")
		fs.StringVar(&message, "message", "", "Message to send to NAVI")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if strings.TrimSpace(message) == "" {
			fmt.Fprintln(os.Stderr, "Usage: navi ask -m \"your question\" [flags]")
			os.Exit(1)
		}
		saveRequested := saveFlag || shortSaveFlag
		resolveConfig(&cfg, saveRequested)
		if err := runAskOnce(ctx, &cfg, message); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "init":
		var cfg Config
		var quick bool
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.BoolVar(&quick, "quick", false, "Quick setup: owner + API key only")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		resolveConfig(&cfg, false)
		if err := runInit(ctx, &cfg, quick); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
	case "status":
		var cfg Config
		var saveFlag, jsonOut bool
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		fs.BoolVar(&jsonOut, "json", false, "Output status as JSON")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		resolveConfig(&cfg, saveFlag)
		runStatusCommand(ctx, &cfg, jsonOut)
	case "sessions":
		var cfg Config
		var saveFlag bool
		var shortSaveFlag bool
		fs := flag.NewFlagSet("sessions", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		fs.BoolVar(&shortSaveFlag, "s", false, "Alias for -save")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		saveRequested := saveFlag || shortSaveFlag
		resolveConfig(&cfg, saveRequested)
		runRuntimeSessionsCommand(ctx, &cfg, "")
	case "models":
		runOperatorSubcommand(ctx, args, "models", runModels)
	case "logs":
		var cfg Config
		var saveFlag, shortSaveFlag bool
		var jsonOutput bool
		fs := flag.NewFlagSet("logs", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		fs.BoolVar(&shortSaveFlag, "s", false, "Alias for -save")
		fs.BoolVar(&jsonOutput, "json", false, "Emit raw JSON events instead of formatted lines")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		resolveConfig(&cfg, saveFlag || shortSaveFlag)
		if err := runLogs(ctx, &cfg, jsonOutput); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "daemon":
		if code := runDaemonCommand(ctx, args); code != 0 {
			os.Exit(code)
		}
	case "proposals":
		runOperatorSubcommand(ctx, args, "proposals", runProposals)
	case "connectors":
		runOperatorSubcommand(ctx, args, "connectors", runConnectors)
	case "skills":
		runOperatorSubcommand(ctx, args, "skills", runSkills)
	case "activity":
		runOperatorSubcommand(ctx, args, "activity", runActivity)
	case "runs":
		runOperatorSubcommand(ctx, args, "runs", runRuns)
	case "dev":
		runOperatorSubcommand(ctx, args, "dev", runDev)
	case "doctor":
		runOperatorSubcommand(ctx, args, "doctor", runDoctor)
	case "console":
		var cfg Config
		var saveFlag bool
		fs := flag.NewFlagSet("console", flag.ExitOnError)
		fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
		fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication")
		fs.BoolVar(&saveFlag, "save", false, "Persist provided flags to ~/.navi/config.json")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		resolveConfig(&cfg, saveFlag)
		if err := runConsole(ctx, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "console error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q. Available: chat (default), ask, init, status, sessions, models, logs, daemon, proposals, connectors, skills, activity, runs, dev, doctor, console\n", subcommand)
		os.Exit(1)
	}
}

// runConsole opens the NAVI Console (the web UI served by the gateway at its
// root) in the owner's default browser. It first probes the gateway's public
// /health endpoint so a clear message is shown when NaviD is not running, then
// prints the URL and attempts to launch a browser. Failure to launch a browser
// is not fatal — the URL is always printed so it can be opened manually.
func runConsole(ctx context.Context, cfg *Config) error {
	consoleURL := strings.TrimRight(cfg.GatewayURL, "/")
	if consoleURL == "" {
		return fmt.Errorf("no gateway URL configured")
	}

	resp, err := doRequest(ctx, cfg, "GET", "/health", nil)
	if err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w\nIs NaviD running? (docker compose -f compose.yml up -d)", consoleURL, err)
	}
	resp.Body.Close()

	fmt.Printf("Opening NAVI Console: %s\n", consoleURL)
	if err := openBrowser(consoleURL); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser automatically (%v)\nOpen this URL manually: %s\n", err, consoleURL)
	}
	return nil
}

// openBrowser launches the platform's default handler for url. It returns an
// error if no launcher is available or the launcher fails to start.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default: // linux, bsd, etc.
		name = "xdg-open"
		args = []string{url}
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("no browser launcher (%s) found on PATH", name)
	}
	return exec.Command(name, args...).Start()
}

// resolveConfig merges flags, environment variables, and on-disk config into cfg.
// Precedence: flags > env > file > default.
func resolveConfig(cfg *Config, saveRequested bool) {
	// 1. Load from file if exists
	configDir, _ := os.UserHomeDir()
	configPath := filepath.Join(configDir, ".navi", "config.json")
	var fileCfg Config
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &fileCfg)
	}

	// 2. Resolve final values (Flags > Env > File > Default)
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = os.Getenv("NAVI_GATEWAY_URL")
		if cfg.GatewayURL == "" {
			cfg.GatewayURL = fileCfg.GatewayURL
			if cfg.GatewayURL == "" {
				cfg.GatewayURL = "http://localhost:6284"
			}
		}
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("NAVI_API_KEY")
		if cfg.APIKey == "" {
			cfg.APIKey = fileCfg.APIKey
		}
	}
	if cfg.OwnerSecret == "" {
		cfg.OwnerSecret = os.Getenv("NAVI_OWNER_SECRET")
		if cfg.OwnerSecret == "" {
			cfg.OwnerSecret = fileCfg.OwnerSecret
		}
	}

	if saveRequested {
		if _, err := saveLocalConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save CLI config: %v\n", err)
		}
	}
}

// runChat implements the default interactive chat REPL flow.
func runChat(ctx context.Context, cfg *Config) {
	cliui.ClearScreen()
	// Check onboarding state — if not yet claimed, run the init wizard first.
	if err := runSetupWizard(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Setup wizard failed: %v\n", err)
		os.Exit(1)
	}

	// Get or create a runtime session.
	runtimeSessionID, err := getOrCreateRuntimeSession(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize runtime session: %v\n", err)
		os.Exit(1)
	}
	if runtimeSessionID == "" {
		fmt.Fprintln(os.Stderr, "Failed to initialize runtime session: empty runtime session ID returned")
		os.Exit(1)
	}

	// Readiness confirmed only after successful runtime session initialization.
	fmt.Println("\nI am NAVI. I am ready.")

	// Start REPL
	runREPL(ctx, cfg, runtimeSessionID)
}

// runAskOnce sends a single message to NAVI and waits for a reply, then exits.
func runAskOnce(ctx context.Context, cfg *Config, message string) error {
	// We deliberately skip first-run prompts here to avoid interactive prompts
	// in one-shot mode. If setup is incomplete, the gateway will return an error.

	// Get or create a runtime session (reuse when possible unless -new is set).
	runtimeSessionID, err := getOrCreateRuntimeSession(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize runtime session: %w", err)
	}
	if runtimeSessionID == "" {
		return fmt.Errorf("failed to initialize runtime session: empty runtime session ID returned")
	}
	prevID, err := latestAssistantMessageID(ctx, cfg, runtimeSessionID)
	if err != nil {
		prevID = ""
	}

	u, _ := url.Parse(cfg.GatewayURL)
	wsURL := "ws://" + u.Host + "/ws/live"
	if u.Scheme == "https" {
		wsURL = "wss://" + u.Host + "/ws/live"
	}

	header := http.Header{}
	if cfg.APIKey != "" {
		header.Set("X-API-Key", cfg.APIKey)
	}

	updates := make(chan liveUpdate, 8)
	var debugEvents atomic.Bool
	var traceEvents atomic.Bool
	var armReplies atomic.Bool
	ctxWS, cancelWS := context.WithCancel(ctx)
	defer cancelWS()
	go runLiveReader(ctxWS, wsURL, header, runtimeSessionID, cfg, updates, &debugEvents, &traceEvents, &armReplies, prevID)

	if err := sendMessage(ctx, cfg, runtimeSessionID, message); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	armReplies.Store(true)

	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	streamingReply := false
	streamedViaTokens := false
	lastPartial := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if !deadline.Stop() {
				select {
				case <-deadline.C:
				default:
				}
			}
			deadline.Reset(30 * time.Second)
			switch update.Kind {
			case liveUpdateTokenDelta:
				if !streamingReply {
					fmt.Print("NAVI: ")
					streamingReply = true
				}
				streamedViaTokens = true
				fmt.Print(update.Content)
			case liveUpdatePartial:
				if !streamingReply {
					fmt.Print("NAVI: ")
					streamingReply = true
				}
				if strings.HasPrefix(update.Content, lastPartial) {
					fmt.Print(update.Content[len(lastPartial):])
				} else {
					fmt.Printf("\nNAVI: %s", update.Content)
				}
				lastPartial = update.Content
			case liveUpdateCompleted:
				if streamedViaTokens {
					fmt.Print("\n")
					return nil
				}
				if streamingReply {
					if strings.HasPrefix(update.Content, lastPartial) {
						fmt.Print(update.Content[len(lastPartial):])
					} else if update.Content != lastPartial {
						fmt.Printf("\nNAVI: %s", update.Content)
					}
					fmt.Print("\n")
				} else {
					fmt.Printf("NAVI: %s\n", update.Content)
				}
				return nil
			case liveUpdateFailed:
				return fmt.Errorf("run failed: %s", update.Content)
			case liveUpdateCancelled:
				return fmt.Errorf("run cancelled: %s", update.Content)
			}
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for reply from NAVI")
		}
	}
}

// saveLocalConfig writes the current CLI configuration to the user's ~/.navi/config.json.
// It returns the path it wrote to for logging purposes.
func saveLocalConfig(cfg *Config) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(homeDir, ".navi", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return configPath, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return configPath, err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return configPath, err
	}
	return configPath, nil
}

// runSetupWizard checks whether this NAVI instance has been claimed.
// If not, it runs the interactive onboarding wizard (Phases 1-3.5).
// On return, cfg.APIKey and cfg.OwnerSecret are populated and saved to disk.
func runSetupWizard(ctx context.Context, cfg *Config) error {
	status, err := fetchFirstRunStatus(ctx, cfg)
	if err != nil {
		return nil
	}

	if status.Complete && cfg.APIKey != "" {
		return nil
	}

	if status.Complete && cfg.APIKey == "" {
		fmt.Println()
		fmt.Println("NAVI is already configured. Paste your API key to continue.")
		fmt.Print("API Key: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			cfg.APIKey = strings.TrimSpace(scanner.Text())
			saveLocalConfig(cfg)
		}
		return nil
	}

	// Not claimed — run first-boot wizard.
	return runInit(ctx, cfg, false)
}

// runInit is the Phase 1–3.5 onboarding wizard.
// quick=true skips optional configuration (LLM, connectors, features).
type firstRunCLIStatus struct {
	Claimed       bool                     `json:"claimed"`
	Complete      bool                     `json:"complete"`
	FirstRunState onboarding.FirstRunState `json:"first_run_state"`
	CurrentStep   string                   `json:"current_step"`
	OwnerName     string                   `json:"owner_name"`
}

func fetchFirstRunStatus(ctx context.Context, cfg *Config) (firstRunCLIStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.GatewayURL+"/api/onboarding/status", nil)
	if err != nil {
		return firstRunCLIStatus{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return firstRunCLIStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return firstRunCLIStatus{}, fmt.Errorf("onboarding status returned %d", resp.StatusCode)
	}
	var status firstRunCLIStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return firstRunCLIStatus{}, err
	}
	if status.FirstRunState == "" {
		status.FirstRunState = onboarding.FirstRunUninitialized
	}
	return status, nil
}

type cliRecoveryPassport struct {
	OwnerID                string    `json:"owner_id"`
	OwnerName              string    `json:"owner_name"`
	InstanceID             string    `json:"instance_id"`
	OwnerSecretFingerprint string    `json:"owner_secret_fingerprint"`
	PrimaryAPIKey          string    `json:"primary_api_key"`
	APIKey                 string    `json:"api_key"`
	RecoverySeed           string    `json:"recovery_seed"`
	AdminSecret            string    `json:"admin_secret"`
	RecoverySavedPath      string    `json:"recovery_saved_path"`
	CreatedAt              time.Time `json:"created_at"`
}

func runInit(ctx context.Context, cfg *Config, quick bool) error {
	cliui.ClearScreen()
	status, err := fetchFirstRunStatus(ctx, cfg)
	if err != nil {
		return err
	}
	if status.Complete {
		fmt.Println("NAVI is already configured.")
		return nil
	}

	state := cliui.NewOnboardingState()
	state.OwnerName = firstNonEmptyString(status.OwnerName, "Owner")
	state.SelectedConnector = "none"
	if quick {
		state.OwnerName = "Owner"
		state.UseAutoGeneratedOwnerSecret = true
	} else if status.FirstRunState == onboarding.FirstRunUninitialized {
		state, err = cliui.RunOnboardingFlow(cfg.GatewayURL, cfg.APIKey)
		if err != nil {
			return err
		}
		if state.OwnerSecret == "" {
			state.UseAutoGeneratedOwnerSecret = true
		}
	} else {
		fmt.Println("Recovery has already been created. NAVI will not show those secrets again.")
		if status.FirstRunState == onboarding.FirstRunRecoveryCreated {
			if err := runCLIProviderSelection(ctx, cfg, state); err != nil {
				return err
			}
		}
		if err := runCLIConnectionSelection(ctx, cfg, state); err != nil {
			return err
		}
		action, err := cliui.RunReviewScreen(state)
		if err != nil {
			return err
		}
		if action != cliui.ReviewConfirm {
			return fmt.Errorf("onboarding cancelled")
		}
	}

	currentState := status.FirstRunState
	var passport cliRecoveryPassport
	if currentState == onboarding.FirstRunUninitialized {
		passport, err = createRecoveryPassport(ctx, cfg, state)
		if err != nil {
			return err
		}
		if passport.PrimaryAPIKey == "" {
			passport.PrimaryAPIKey = passport.APIKey
		}
		if passport.AdminSecret != "" && state.OwnerSecret == "" {
			state.OwnerSecret = passport.AdminSecret
		}

		cliui.DisplayPassport(passport.OwnerName, passport.InstanceID,
			passport.OwnerSecretFingerprint, state.OwnerSecret, passport.PrimaryAPIKey, passport.RecoverySeed)
		if passport.RecoverySavedPath != "" {
			fmt.Printf("Recovery passport saved by NAVI at %s\n", passport.RecoverySavedPath)
		}
		if !quick {
			if err := cliui.RunPassportSave(passport.OwnerName, passport.InstanceID,
				passport.OwnerSecretFingerprint, state.OwnerSecret, passport.PrimaryAPIKey, passport.RecoverySeed); err != nil {
				fmt.Fprintf(os.Stderr, "warning: manual passport save skipped: %v\n", err)
			}
		}

		cfg.APIKey = passport.PrimaryAPIKey
		if state.OwnerSecret != "" {
			cfg.OwnerSecret = state.OwnerSecret
		}
		configPath, err := saveLocalConfig(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save config: %v\n", err)
		} else {
			fmt.Printf("Config saved to %s\n", configPath)
		}
		currentState = onboarding.FirstRunRecoveryCreated

		if !quick {
			if err := cliui.RunPassportConfirmation(passport.RecoverySeed); err != nil {
				return err
			}
			apply, err := cliui.RunApplyScreen()
			if err != nil {
				return err
			}
			if !apply {
				fmt.Println(cliui.DimStyle.Render("Setup paused. Recovery was created; run navi init to continue."))
				return nil
			}
		}
	}

	cliui.DisplayInitializing()
	if currentState == onboarding.FirstRunRecoveryCreated {
		if err := configureOnboardingProvider(ctx, cfg, state, quick); err != nil {
			return err
		}
		currentState = onboarding.FirstRunProviderConfigured
	}
	if currentState == onboarding.FirstRunProviderConfigured {
		if err := configureOnboardingConnection(ctx, cfg, state); err != nil {
			fmt.Printf("  [!] Connection setup warning: %v\n", err)
			if err := skipOnboardingConnection(ctx, cfg); err != nil {
				return err
			}
		}
	}
	if err := completeOnboarding(ctx, cfg); err != nil {
		return err
	}

	ownerName := firstNonEmptyString(passport.OwnerName, state.OwnerName, status.OwnerName, "Owner")
	cliui.DisplayReady(ownerName)
	return nil
}

func createRecoveryPassport(ctx context.Context, cfg *Config, state *cliui.OnboardingState) (cliRecoveryPassport, error) {
	body, _ := json.Marshal(map[string]string{
		"owner_name":   state.OwnerName,
		"owner_handle": "",
		"owner_secret": state.OwnerSecret,
	})
	resp, err := doRequest(ctx, cfg, http.MethodPost, "/api/onboarding/recovery", bytes.NewReader(body))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return cliRecoveryPassport{}, fmt.Errorf("recovery setup failed: %w", err)
	}
	defer resp.Body.Close()
	var passport cliRecoveryPassport
	if err := json.NewDecoder(resp.Body).Decode(&passport); err != nil {
		return cliRecoveryPassport{}, fmt.Errorf("failed to decode recovery passport: %w", err)
	}
	return passport, nil
}

func configureOnboardingProvider(ctx context.Context, cfg *Config, state *cliui.OnboardingState, useDefaults bool) error {
	req := map[string]string{}
	if !useDefaults {
		req = map[string]string{
			"provider": state.SelectedProvider,
			"api_key":  state.ProviderAPIKey,
			"model":    state.SelectedModel,
		}
	}
	body, _ := json.Marshal(req)
	resp, err := doRequest(ctx, cfg, http.MethodPost, "/api/onboarding/provider", bytes.NewReader(body))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("AI provider setup failed: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	fmt.Printf("  [+] AI provider configured: %s", firstNonEmptyString(result.Provider, state.SelectedProvider, "default"))
	if result.Model != "" {
		fmt.Printf(" (%s)", result.Model)
	}
	fmt.Println()
	return nil
}

func configureOnboardingConnection(ctx context.Context, cfg *Config, state *cliui.OnboardingState) error {
	if state.SelectedConnector == "" || state.SelectedConnector == "none" {
		return skipOnboardingConnection(ctx, cfg)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"type":   state.SelectedConnector,
		"params": state.ConnectorParams,
	})
	resp, err := doRequest(ctx, cfg, http.MethodPost, "/api/onboarding/connection", bytes.NewReader(body))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return err
	}
	resp.Body.Close()
	fmt.Printf("  [+] Connection %q activated.\n", state.SelectedConnector)
	return nil
}

func skipOnboardingConnection(ctx context.Context, cfg *Config) error {
	body, _ := json.Marshal(map[string]bool{"skip": true})
	resp, err := doRequest(ctx, cfg, http.MethodPost, "/api/onboarding/connection", bytes.NewReader(body))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return err
	}
	resp.Body.Close()
	return nil
}

func completeOnboarding(ctx context.Context, cfg *Config) error {
	resp, err := doRequest(ctx, cfg, http.MethodPost, "/api/onboarding/complete", bytes.NewReader([]byte("{}")))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("completion failed: %w", err)
	}
	resp.Body.Close()
	return nil
}

func runCLIProviderSelection(ctx context.Context, cfg *Config, state *cliui.OnboardingState) error {
	catalog, err := cliui.FetchCatalog(cfg.GatewayURL, cfg.APIKey)
	if err != nil {
		fmt.Println(cliui.DimStyle.Render("Gateway catalog not reachable - using provider defaults."))
	}
	if err := cliui.ProviderSelectForm(catalog, state).Run(); err != nil {
		return err
	}
	if state.SelectedProvider == "manual" {
		if err := cliui.ManualProviderForm(state).Run(); err != nil {
			return err
		}
	}
	if state.SelectedProvider == "ollama" {
		ollamaModels, ollamaErr := cliui.FetchOllamaModels()
		if ollamaErr == nil && len(ollamaModels) > 0 {
			if err := cliui.OllamaModelSelectForm(ollamaModels, state).Run(); err != nil {
				return err
			}
		}
	} else if state.SelectedProvider != "manual" && err == nil {
		if mf := cliui.ModelSelectForm(catalog, state); mf != nil {
			if err := mf.Run(); err != nil {
				return err
			}
		}
	}
	if state.SelectedModel == "custom" || state.SelectedModel == "" {
		if err := cliui.ManualModelForm(state).Run(); err != nil {
			return err
		}
	}
	if kf := cliui.ProviderAPIKeyForm(state); kf != nil {
		if err := kf.Run(); err != nil {
			return err
		}
	}
	return nil
}

func runCLIConnectionSelection(ctx context.Context, cfg *Config, state *cliui.OnboardingState) error {
	schema, schemaErr := cliui.FetchSetupSchema(cfg.GatewayURL, cfg.APIKey)
	if schemaErr != nil {
		fmt.Println(cliui.DimStyle.Render("Gateway connector schema not reachable - using built-in connector list."))
		schema = connectors.BuiltinSetupDescriptors()
	}
	if err := cliui.ConnectorSelectForm(schema, state).Run(); err != nil {
		return err
	}
	if state.SelectedConnector == "none" {
		return nil
	}
	paramsForm, cFields := cliui.BuildConnectorParamsForm(schema, state)
	if paramsForm != nil {
		if err := paramsForm.Run(); err != nil {
			return err
		}
		for _, cf := range cFields {
			state.ConnectorParams[cf.Key] = cf.Value
			if cf.Label != "" {
				state.ConnectorParamLabels[cf.Key] = cf.Label
			}
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func authenticate(_ context.Context, _, _ string) (string, error) {
	// JWT authentication has been removed. Connectors and clients use X-API-Key.
	// This stub is kept to prevent compilation errors in any remaining call sites.
	return "", fmt.Errorf("JWT auth is not supported; use X-API-Key")
}

func doRequest(ctx context.Context, cfg *Config, method, urlPath string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, cfg.GatewayURL+urlPath, body)
	if err != nil {
		return nil, err
	}
	if cfg.APIKey != "" {
		req.Header.Set("X-API-Key", cfg.APIKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request %s %s: %w", method, urlPath, err)
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(b, &errResp); err == nil && errResp.Error != "" {
			return resp, fmt.Errorf("server error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return resp, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(b))
	}

	return resp, nil
}

const (
	wsBackoffInitial = 1 * time.Second
	wsBackoffMax     = 30 * time.Second
)

type liveReqFrame struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type liveResFrame struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Event json.RawMessage `json:"event,omitempty"`
}

type liveEventFrame struct {
	Type  string `json:"type"`
	Event struct {
		Type      string          `json:"type"`
		Timestamp time.Time       `json:"timestamp"`
		Payload   json.RawMessage `json:"payload"`
		Seq       int64           `json:"seq"`
	} `json:"event"`
}

type liveUpdateKind string

const (
	liveUpdateTokenDelta liveUpdateKind = "token_delta"
	liveUpdatePartial    liveUpdateKind = "partial"
	liveUpdateCompleted  liveUpdateKind = "completed"
	liveUpdateFailed     liveUpdateKind = "failed"
	liveUpdateProposal   liveUpdateKind = "proposal"
	liveUpdateCancelled  liveUpdateKind = "cancelled"
)

type liveUpdate struct {
	Kind      liveUpdateKind
	MessageID string
	Content   string
	Proposal  string
	ToolName  string
	Reason    string
	Arguments map[string]any
}

// runLiveReader connects to the live WebSocket, tracks event seq for resume,
// and reconnects with exponential backoff on disconnect. Runs until ctx is done.
func runLiveReader(ctx context.Context, wsURL string, header http.Header, chatID string, cfg *Config, updates chan<- liveUpdate, debugEvents, traceEvents, armReplies *atomic.Bool, skipCompletedID string) {
	_ = cfg
	var lastSeq atomic.Int64
	backoff := wsBackoffInitial

	for ctx.Err() == nil {
		c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
			HTTPHeader: header,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if lastSeq.Load() > 0 {
				fmt.Fprintf(os.Stderr, "\n[!] Connection lost. Reconnecting in %v... (%v)\n", backoff, err)
			} else {
				fmt.Fprintf(os.Stderr, "\n[!] Could not connect. Retrying in %v... (%v)\n", backoff, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < wsBackoffMax {
					backoff *= 2
				}
			}
			continue
		}

		backoff = wsBackoffInitial
		initMsg, _ := json.Marshal(liveReqFrame{
			Type:   "req",
			ID:     "connect",
			Method: "connect",
			Params: map[string]any{
				"chat_id":   chatID,
				"after_seq": lastSeq.Load(),
				"stream":    "user",
			},
		})
		if err := c.Write(ctx, websocket.MessageText, initMsg); err != nil {
			c.Close(websocket.StatusNormalClosure, "")
			continue
		}
		if lastSeq.Load() > 0 {
			fmt.Fprintf(os.Stderr, "\n[+] Reconnected.\n")
		}

		readDone := false
		for !readDone && ctx.Err() == nil {
			_, messageData, err := c.Read(ctx)
			if err != nil {
				c.Close(websocket.StatusAbnormalClosure, "")
				readDone = true
				fmt.Fprintf(os.Stderr, "\n[!] Connection lost. Reconnecting...\n")
				continue
			}

			if debugEvents != nil && debugEvents.Load() {
				fmt.Fprintf(os.Stderr, "[debug] ws event: %s\n", string(messageData))
			}

			var base struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(messageData, &base); err != nil {
				if debugEvents != nil && debugEvents.Load() {
					fmt.Fprintf(os.Stderr, "[debug] malformed event: %v\n", err)
				}
				continue
			}
			switch base.Type {
			case "res":
				var res liveResFrame
				if err := json.Unmarshal(messageData, &res); err != nil {
					continue
				}
				if !res.OK && debugEvents != nil && debugEvents.Load() {
					fmt.Fprintf(os.Stderr, "[debug] ws error: %s\n", res.Error)
				}
			case "event":
				var frame liveEventFrame
				if err := json.Unmarshal(messageData, &frame); err != nil {
					continue
				}
				if frame.Event.Seq > 0 {
					lastSeq.Store(frame.Event.Seq)
				}
				switch frame.Event.Type {
				case "assistant.token.delta":
					var payload struct {
						Delta string `json:"delta"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					if armReplies != nil && !armReplies.Load() {
						continue
					}
					if payload.Delta == "" {
						continue
					}
					select {
					case updates <- liveUpdate{Kind: liveUpdateTokenDelta, Content: payload.Delta}:
					case <-ctx.Done():
						return
					}
				case "assistant.message.partial":
					var payload struct {
						Content string `json:"content"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					if armReplies != nil && !armReplies.Load() {
						continue
					}
					select {
					case updates <- liveUpdate{Kind: liveUpdatePartial, Content: payload.Content}:
					case <-ctx.Done():
						return
					}
				case "assistant.message.completed":
					var payload struct {
						MessageID string `json:"message_id"`
						Content   string `json:"content"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					if payload.MessageID == skipCompletedID {
						continue
					}
					if armReplies != nil && !armReplies.Load() {
						continue
					}
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] assistant.message.completed\n")
					}
					select {
					case updates <- liveUpdate{Kind: liveUpdateCompleted, MessageID: payload.MessageID, Content: payload.Content}:
					case <-ctx.Done():
						return
					}
				case "proposal.waiting":
					var payload struct {
						ProposalID string         `json:"proposal_id"`
						ToolName   string         `json:"tool_name"`
						Arguments  map[string]any `json:"arguments"`
						Reason     string         `json:"reason"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					select {
					case updates <- liveUpdate{
						Kind:      liveUpdateProposal,
						Proposal:  payload.ProposalID,
						ToolName:  payload.ToolName,
						Reason:    payload.Reason,
						Arguments: payload.Arguments,
					}:
					case <-ctx.Done():
						return
					}
				case "run.failed":
					var payload struct {
						Error string `json:"error"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					if armReplies != nil && !armReplies.Load() {
						continue
					}
					select {
					case updates <- liveUpdate{Kind: liveUpdateFailed, Content: payload.Error}:
					case <-ctx.Done():
						return
					}
				case "run.cancelled":
					var payload struct {
						Reason string `json:"reason"`
					}
					if err := json.Unmarshal(frame.Event.Payload, &payload); err != nil {
						continue
					}
					if armReplies != nil && !armReplies.Load() {
						continue
					}
					select {
					case updates <- liveUpdate{Kind: liveUpdateCancelled, Content: payload.Reason}:
					case <-ctx.Done():
						return
					}
				case "fact.cost.recorded":
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] cost recorded\n")
					}
				case "fact.governor.tripped":
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] governor tripped\n")
					}
				case "fact.directive.replied":
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] directive replied\n")
					}
				case "navi.fact.skill_error":
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] skill error\n")
					}
				default:
					if traceEvents != nil && traceEvents.Load() {
						fmt.Printf("\n[trace] event %s\n", frame.Event.Type)
					}
				}
			}
		}
	}
}

func runREPL(ctx context.Context, cfg *Config, chatID string) {
	// Fetch brain status for the welcome line
	brainLine := ""
	if statusResp, err := doRequest(ctx, cfg, "GET", "/api/status", nil); err == nil {
		defer statusResp.Body.Close()
		var s struct {
			LLM struct {
				Status string `json:"status"`
			} `json:"llm"`
		}
		if json.NewDecoder(statusResp.Body).Decode(&s) == nil {
			brain := s.LLM.Status
			if brain == "" {
				brain = "not configured"
			}
			brainLine = fmt.Sprintf("  Brain:   %s", brain)
		}
	}

	fmt.Println("╔══════════════════════════════════╗")
	fmt.Println("║           NAVI  ·  Chat          ║")
	fmt.Println("╚══════════════════════════════════╝")
	if brainLine != "" {
		fmt.Println(brainLine)
	}
	fmt.Println("  /status  /connect  /sessions  /model  /debug  /exit")
	fmt.Println("──────────────────────────────────")

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// WebSocket URL for live stream
	u, _ := url.Parse(cfg.GatewayURL)
	wsURL := "ws://" + u.Host + "/ws/live"
	if u.Scheme == "https" {
		wsURL = "wss://" + u.Host + "/ws/live"
	}

	header := http.Header{}
	if cfg.APIKey != "" {
		header.Set("X-API-Key", cfg.APIKey)
	}

	// Channel for live runtime updates from NAVI
	updateCh := make(chan liveUpdate, 32)
	prevID, _ := latestAssistantMessageID(ctx, cfg, chatID)

	var debugEvents atomic.Bool
	var traceEvents atomic.Bool
	var armReplies atomic.Bool
	if cfg.Debug {
		debugEvents.Store(true)
		traceEvents.Store(true)
	}

	var liveCancel context.CancelFunc
	startLiveReader := func(targetChatID string) {
		if liveCancel != nil {
			liveCancel()
		}
		prevID, _ = latestAssistantMessageID(ctx, cfg, targetChatID)
		readerCtx, cancel := context.WithCancel(ctx)
		liveCancel = cancel
		go runLiveReader(readerCtx, wsURL, header, targetChatID, cfg, updateCh, &debugEvents, &traceEvents, &armReplies, prevID)
	}
	startLiveReader(chatID)
	defer func() {
		if liveCancel != nil {
			liveCancel()
		}
	}()

	// Scanner for user input — one persistent goroutine to avoid leaks.
	inputCh := make(chan string, 10)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			inputCh <- scanner.Text()
		}
	}()

	// Timeout channel for waiting on the first reply after a user message.
	timeoutCh := make(chan struct{}, 1)
	waitingForReply := false
	streamingReply := false
	streamedViaTokens := false
	lastPartial := ""

	fmt.Print("\nYou: ")
	for {
		select {
		case <-sigCh:
			fmt.Println("\nBye!")
			return
		case <-ctx.Done():
			return
		case update := <-updateCh:
			switch update.Kind {
			case liveUpdateTokenDelta:
				if !armReplies.Load() {
					continue
				}
				if waitingForReply {
					waitingForReply = false
				}
				if !streamingReply {
					fmt.Print("\nNAVI: ")
					streamingReply = true
				}
				streamedViaTokens = true
				fmt.Print(update.Content)
			case liveUpdatePartial:
				if !armReplies.Load() {
					continue
				}
				if waitingForReply {
					waitingForReply = false
				}
				if !streamingReply {
					fmt.Print("\nNAVI: ")
					streamingReply = true
				}
				next := update.Content
				if strings.HasPrefix(next, lastPartial) {
					fmt.Print(next[len(lastPartial):])
				} else {
					fmt.Printf("\nNAVI: %s", next)
				}
				lastPartial = next
			case liveUpdateCompleted:
				if !armReplies.Load() {
					continue
				}
				if waitingForReply {
					waitingForReply = false
				}
				armReplies.Store(false)
				if streamedViaTokens {
					fmt.Print("\n\nYou: ")
				} else if streamingReply {
					if strings.HasPrefix(update.Content, lastPartial) {
						fmt.Print(update.Content[len(lastPartial):])
					} else if update.Content != lastPartial {
						fmt.Printf("\nNAVI: %s", update.Content)
					}
					fmt.Print("\n\nYou: ")
				} else {
					fmt.Printf("\nNAVI: %s\n\nYou: ", update.Content)
				}
				streamingReply = false
				streamedViaTokens = false
				lastPartial = ""
			case liveUpdateProposal:
				armReplies.Store(false)
				waitingForReply = false
				streamingReply = false
				streamedViaTokens = false
				lastPartial = ""
				fmt.Printf("\n[approval] NAVI paused for approval:\n  proposal: %s\n  tool: %s\n  reason: %s\n  args: %v\n", update.Proposal, update.ToolName, update.Reason, update.Arguments)
				fmt.Print("\nUse the proposals API to approve or decline, then continue chatting.\nYou: ")
			case liveUpdateFailed:
				armReplies.Store(false)
				waitingForReply = false
				streamingReply = false
				streamedViaTokens = false
				lastPartial = ""
				fmt.Printf("\n[run failed] %s\n\nYou: ", update.Content)
			case liveUpdateCancelled:
				armReplies.Store(false)
				waitingForReply = false
				streamingReply = false
				streamedViaTokens = false
				lastPartial = ""
				msg := update.Content
				if msg == "" {
					msg = "cancelled"
				}
				fmt.Printf("\n[run cancelled] %s\n\nYou: ", msg)
			}
		case <-timeoutCh:
			if !waitingForReply {
				// Stale timeout from an earlier turn; ignore.
				continue
			}
			fmt.Print("\n(Still processing your request. I'll reply as soon as it's ready.)\n\nYou: ")
			// Keep waiting for the eventual reply and emit periodic reminders.
			go func() {
				time.Sleep(30 * time.Second)
				select {
				case timeoutCh <- struct{}{}:
				default:
				}
			}()
		case input := <-inputCh:
			text := strings.TrimSpace(input)
			if text == "" {
				fmt.Print("\nYou: ")
				continue
			}

			if strings.HasPrefix(text, "/") {
				switch {
				case text == "/exit" || text == "/quit":
					fmt.Println("Bye!")
					return
				case text == "/status":
					runStatusCommand(ctx, cfg, false)
				case text == "/sessions":
					runRuntimeSessionsCommand(ctx, cfg, chatID)
				case strings.HasPrefix(text, "/use "):
					target := strings.TrimSpace(strings.TrimPrefix(text, "/use "))
					if target == "" {
						fmt.Println("\n/use requires a chat ID, e.g. /use 1234-...")
					} else if newID, ok := runUseSessionCommand(ctx, cfg, target); ok {
						chatID = newID
						waitingForReply = false
						armReplies.Store(false)
						streamingReply = false
						streamedViaTokens = false
						lastPartial = ""
						startLiveReader(chatID)
						fmt.Printf("\nSwitched to chat %s\n", chatID)
					}
				case text == "/debug":
					enabled := !debugEvents.Load()
					debugEvents.Store(enabled)
					if enabled {
						fmt.Println("\nDebug mode enabled — raw WebSocket events will be logged.")
					} else {
						fmt.Println("\nDebug mode disabled.")
					}
				case text == "/connect telegram":
					getLine := func(prompt string) string {
						fmt.Print(prompt)
						return strings.TrimSpace(<-inputCh)
					}
					runConnectTelegram(ctx, cfg, getLine)
				case text == "/connect slack":
					getLine := func(prompt string) string {
						fmt.Print(prompt)
						return strings.TrimSpace(<-inputCh)
					}
					runConnectConnector(ctx, cfg, "slack", getLine)
				case text == "/connect":
					getLine := func(prompt string) string {
						fmt.Print(prompt)
						return strings.TrimSpace(<-inputCh)
					}
					typ := getLine("  Connector type? ")
					runConnectConnector(ctx, cfg, typ, getLine)
				case text == "/trace":
					enabled := !traceEvents.Load()
					traceEvents.Store(enabled)
					if enabled {
						fmt.Println("\nTrace mode enabled — compact step traces will be shown for live events.")
					} else {
						fmt.Println("\nTrace mode disabled.")
					}
				case strings.HasPrefix(text, "/approve"):
					note := strings.TrimSpace(strings.TrimPrefix(text, "/approve"))
					if note == "" {
						note = "approved"
					}
					msg := fmt.Sprintf("Operator approval: APPROVE. %s", note)
					if err := sendMessage(ctx, cfg, chatID, msg); err != nil {
						fmt.Printf("\n/approve error: %v\n", err)
					}
				case strings.HasPrefix(text, "/deny"):
					note := strings.TrimSpace(strings.TrimPrefix(text, "/deny"))
					if note == "" {
						note = "denied"
					}
					msg := fmt.Sprintf("Operator approval: DENY. %s", note)
					if err := sendMessage(ctx, cfg, chatID, msg); err != nil {
						fmt.Printf("\n/deny error: %v\n", err)
					}
				case text == "/model" || text == "/model list":
					runModelListCommand(ctx, cfg)
				case strings.HasPrefix(text, "/model set "):
					args := strings.TrimSpace(strings.TrimPrefix(text, "/model set "))
					if args == "" {
						fmt.Println("\n/model set requires <provider> <model> or <provider/model>, e.g. /model set ollama llama3.2")
					} else {
						runModelSetCommand(ctx, cfg, args)
					}
				default:
					fmt.Printf("\nUnknown command %q. Available: /status, /sessions, /use <id>, /connect [telegram|slack], /model [list|set], /debug, /trace, /approve <note>, /deny <note>, /exit\n", text)
				}
				fmt.Print("\nYou: ")
				continue
			}

			if text == "/exit" || text == "/quit" {
				fmt.Println("Bye!")
				return
			}
			if err := sendMessage(ctx, cfg, chatID, text); err != nil {
				fmt.Printf("\nError: %v\n", err)
				fmt.Print("\nYou: ")
				continue
			}

			// Start waiting for a reply with a bounded timeout.
			waitingForReply = true
			armReplies.Store(true)
			go func() {
				time.Sleep(30 * time.Second)
				select {
				case timeoutCh <- struct{}{}:
				default:
				}
			}()
			fmt.Print("\n(Waiting for NAVI...)\n")
		}
	}
}

// fetchAvailableConnectors queries the server for supported connector types.
// Returns empty slice if the request fails.
func fetchAvailableConnectors(ctx context.Context, cfg *Config) []connectors.SetupDescriptor {
	resp, err := doRequest(ctx, cfg, "GET", "/api/connectors/setup-schema", nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var schema []connectors.SetupDescriptor
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil
	}
	return schema
}

// runConnectConnector is the single entry point for connector configuration.
// It uses GET /api/connectors/setup-schema (server-authoritative) for prompts and param definitions.
func runConnectConnector(ctx context.Context, cfg *Config, connectorType string, getLine func(prompt string) string) {
	connectorType = strings.ToLower(strings.TrimSpace(connectorType))
	if connectorType == "" {
		fmt.Println("\n  [!] No connector type specified.")
		return
	}

	schemaResp, err := doRequest(ctx, cfg, "GET", "/api/connectors/setup-schema", nil)
	if err != nil {
		fmt.Printf("\n  [!] Could not fetch connector setup schema: %v\n", err)
		return
	}
	if schemaResp.StatusCode != http.StatusOK {
		schemaResp.Body.Close()
		fmt.Printf("\n  [!] Server returned %d for setup-schema.\n", schemaResp.StatusCode)
		return
	}
	var schema []connectors.SetupDescriptor
	if err := json.NewDecoder(schemaResp.Body).Decode(&schema); err != nil {
		schemaResp.Body.Close()
		fmt.Printf("\n  [!] Invalid setup-schema response: %v\n", err)
		return
	}
	schemaResp.Body.Close()

	var descriptor *connectors.SetupDescriptor
	for i := range schema {
		if schema[i].Type == connectorType {
			descriptor = &schema[i]
			break
		}
	}
	if descriptor == nil {
		var types []string
		for _, d := range schema {
			types = append(types, d.Type)
		}
		fmt.Printf("\n  [!] Unknown connector type %q. Server supports: %s\n", connectorType, strings.Join(types, ", "))
		return
	}

	params := runConnectConnectorFromSchema(*descriptor, getLine)
	if params == nil {
		return
	}

	fmt.Println("\n  Saving to NAVI...")
	body, _ := json.Marshal(map[string]interface{}{
		"type":   connectorType,
		"params": params,
	})
	resp, err := doRequest(ctx, cfg, "POST", "/api/setup/connector", bytes.NewReader(body))
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		fmt.Printf("\n  [!] Failed: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Printf("  [+] Connector %q activated.\n", connectorType)
	fmt.Println("  ─────────────────────────────────────────────────────────")
}

func runConnectConnectorFromSchema(d connectors.SetupDescriptor, getLine func(prompt string) string) map[string]string {
	fmt.Println()
	fmt.Printf("  ── Connector setup — %s ──────────────────────────────\n", d.DisplayName)
	fmt.Println("  Credentials are entered directly here and never sent to the LLM.")
	if d.SetupHint != "" {
		fmt.Printf("  %s\n", d.SetupHint)
	}
	fmt.Println()

	params := make(map[string]string)
	for _, p := range d.RequiredParams {
		v := getLine("  " + p.Label + ": ")
		if v == "" {
			fmt.Printf("\n  [!] Setup cancelled — %s is required.\n", p.Label)
			return nil
		}
		params[p.Key] = v
	}
	for _, p := range d.OptionalParams {
		v := getLine("  " + p.Label + " (optional): ")
		if v != "" {
			params[p.Key] = v
		}
	}
	return params
}

// runConnectTelegram is a trivial wrapper over the generic connector configure path.
func runConnectTelegram(ctx context.Context, cfg *Config, getLine func(prompt string) string) {
	runConnectConnector(ctx, cfg, "telegram", getLine)
}

func runStatusCommand(ctx context.Context, cfg *Config, jsonOut bool) {
	resp, err := doRequest(ctx, cfg, "GET", "/api/status", nil)
	if err != nil {
		if isAuthError(err) {
			fmt.Fprintf(os.Stderr, "status: auth failed: %v\n", err)
			os.Exit(3)
		}
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if jsonOut {
		fmt.Println(string(b))
		return
	}
	var status struct {
		Gateway struct {
			Reachable      bool
			Version, Build string
		} `json:"gateway"`
		Setup    struct{ Complete bool }           `json:"setup"`
		Identity struct{ AgentID, OwnerID string } `json:"identity"`
		Governor struct {
			Tripped               bool
			BudgetUsed, BudgetMax int
			CostUsed, CostCeiling float64
			DurationRemaining     string
		} `json:"governor"`
		Proposals  struct{ PendingCount int } `json:"proposals"`
		Connectors struct {
			Total    int `json:"total"`
			Running  int `json:"running"`
			Degraded int `json:"degraded"`
			Error    int `json:"error"`
		} `json:"connectors"`
		LLM struct {
			Configured bool
			Status     string
		} `json:"llm"`
		Degraded  bool   `json:"degraded"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(b, &status); err != nil {
		fmt.Fprintf(os.Stderr, "status decode error: %v\n", err)
		os.Exit(1)
	}
	version := status.Gateway.Version
	if status.Gateway.Build != "" {
		version += " (" + status.Gateway.Build + ")"
	}
	fmt.Printf("Gateway:   %s  %s\n", cfg.GatewayURL, version)
	fmt.Printf("Setup:     %v\n", status.Setup.Complete)
	fmt.Printf("Identity:  agent=%s owner=%s\n", status.Identity.AgentID, status.Identity.OwnerID)
	fmt.Printf("Governor:  tripped=%v budget=%d/%d cost=%.2f/%.2f remaining=%s\n",
		status.Governor.Tripped, status.Governor.BudgetUsed, status.Governor.BudgetMax,
		status.Governor.CostUsed, status.Governor.CostCeiling, status.Governor.DurationRemaining)
	fmt.Printf("Proposals: %d pending\n", status.Proposals.PendingCount)
	fmt.Printf("Connectors: %d total (%d running, %d degraded, %d error)\n",
		status.Connectors.Total, status.Connectors.Running, status.Connectors.Degraded, status.Connectors.Error)
	fmt.Printf("LLM:       configured=%v status=%s\n", status.LLM.Configured, status.LLM.Status)
	fmt.Printf("Degraded:  %v  updated_at=%s\n", status.Degraded, status.UpdatedAt)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "Unauthorized")
}

type operatorRunner func(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int

func runOperatorSubcommand(ctx context.Context, args []string, name string, run operatorRunner) {
	var cfg Config
	var jsonOut bool
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL")
	fs.StringVar(&cfg.APIKey, "api-key", "", "API key for authentication")
	fs.BoolVar(&jsonOut, "json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	resolveConfig(&cfg, false)
	code := run(ctx, &cfg, fs.Args(), jsonOut)
	if code != 0 {
		os.Exit(code)
	}
}

func runProposals(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	if len(subArgs) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: navi proposals list | approve <id> [--note] | decline <id> [--note]\n")
		return 1
	}
	verb := subArgs[0]
	switch verb {
	case "list":
		resp, err := doRequest(ctx, cfg, "GET", "/api/proposals", nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "proposals list: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var list []struct {
			ProposalID string `json:"proposal_id"`
			Status     string `json:"status"`
			Priority   string `json:"priority"`
			CreatedAt  string `json:"created_at"`
			Rationale  string `json:"rationale"`
		}
		if err := json.Unmarshal(b, &list); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		for _, p := range list {
			fmt.Printf("%s  %s  %s  %s  %s\n", p.ProposalID, p.Status, p.Priority, p.CreatedAt, p.Rationale)
		}
		return 0
	case "approve", "decline":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: navi proposals %s <id> [--note=...]\n", verb)
			return 1
		}
		id := subArgs[1]
		body := map[string]string{"action": verb, "resolution_note": ""}
		if len(subArgs) > 2 {
			body["resolution_note"] = strings.Join(subArgs[2:], " ")
		}
		reqBody, _ := json.Marshal(body)
		resp, err := doRequest(ctx, cfg, "POST", "/api/proposals/"+id+"/resolve", bytes.NewReader(reqBody))
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			if strings.Contains(err.Error(), "404") {
				return 2
			}
			fmt.Fprintf(os.Stderr, "proposals %s: %v\n", verb, err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
		} else {
			fmt.Printf("Proposal %s %sd.\n", id, verb)
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown verb %q. Use list | approve | decline.\n", verb)
		return 1
	}
}

func runConnectors(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	verb := "list"
	if len(subArgs) > 0 {
		verb = subArgs[0]
	}
	path := "/api/connectors"
	if verb == "health" {
		path = "/api/health/connectors"
	} else if verb == "disconnect" {
		if len(subArgs) < 2 || strings.TrimSpace(subArgs[1]) == "" {
			fmt.Fprintf(os.Stderr, "Usage: navi connectors disconnect <name>\n")
			return 1
		}
		name := strings.TrimSpace(subArgs[1])
		resp, err := doRequest(ctx, cfg, "DELETE", "/api/connectors/"+name, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "connectors disconnect: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		fmt.Printf("%s  disconnected\n", name)
		return 0
	} else if verb != "list" {
		fmt.Fprintf(os.Stderr, "Usage: navi connectors list | health | disconnect <name>\n")
		return 1
	}
	resp, err := doRequest(ctx, cfg, "GET", path, nil)
	if err != nil {
		if isAuthError(err) {
			return 3
		}
		fmt.Fprintf(os.Stderr, "connectors: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if jsonOut {
		fmt.Println(string(b))
		return 0
	}
	var list []map[string]any
	if err := json.Unmarshal(b, &list); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		return 1
	}
	for _, c := range list {
		name, _ := c["name"].(string)
		state, _ := c["state"].(string)
		fmt.Printf("%s  %s\n", name, state)
	}
	return 0
}

func runModels(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	verb := "list"
	if len(subArgs) > 0 {
		verb = subArgs[0]
	}

	switch verb {
	case "list":
		resp, err := doRequest(ctx, cfg, "GET", "/api/llm/catalog", nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "models list: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			Providers []struct {
				Key         string `json:"key"`
				DisplayName string `json:"display_name"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"providers"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		for _, p := range out.Providers {
			if p.DisplayName != "" && p.DisplayName != p.Key {
				fmt.Printf("%s (%s)\n", p.Key, p.DisplayName)
			} else {
				fmt.Println(p.Key)
			}
			for _, m := range p.Models {
				fmt.Printf("  %s\n", m.Name)
			}
		}
		return 0
	case "active":
		resp, err := doRequest(ctx, cfg, "GET", "/api/llm/active", nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "models active: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		fmt.Printf("%s/%s\n", out.Provider, out.Model)
		return 0
	case "set":
		if len(subArgs) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: navi models set <provider> <model>\n")
			return 1
		}
		body, _ := json.Marshal(map[string]string{
			"provider": strings.TrimSpace(subArgs[1]),
			"model":    strings.TrimSpace(strings.Join(subArgs[2:], " ")),
		})
		resp, err := doRequest(ctx, cfg, "PUT", "/api/llm/active", bytes.NewReader(body))
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "models set: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		fmt.Printf("Active model set to %s/%s\n", out.Provider, out.Model)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Usage: navi models list | active | set <provider> <model>\n")
		return 1
	}
}

func runSkills(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	if len(subArgs) == 0 || subArgs[0] == "list" {
		path := "/api/skills"
		resp, err := doRequest(ctx, cfg, "GET", path, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "skills list: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		for _, s := range out.Items {
			id, _ := s["id"].(string)
			name, _ := s["name"].(string)
			version, _ := s["version"].(string)
			state, _ := s["availability_state"].(string)
			fmt.Printf("%s  %s  %s  %s\n", id, name, version, state)
		}
		return 0
	}
	if subArgs[0] == "show" && len(subArgs) >= 2 {
		id := subArgs[1]
		resp, err := doRequest(ctx, cfg, "GET", "/api/skills/"+id, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			if strings.Contains(err.Error(), "404") {
				return 2
			}
			fmt.Fprintf(os.Stderr, "skills show: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
		} else {
			var s map[string]any
			if err := json.Unmarshal(b, &s); err != nil {
				fmt.Fprintf(os.Stderr, "decode: %v\n", err)
				return 1
			}
			for k, v := range s {
				fmt.Printf("%s: %v\n", k, v)
			}
		}
		return 0
	}
	if subArgs[0] == "inspect" && len(subArgs) >= 2 {
		// Alias for show to match documentation.
		return runSkills(ctx, cfg, []string{"show", subArgs[1]}, jsonOut)
	}
	if subArgs[0] == "reload" {
		resp, err := doRequest(ctx, cfg, "POST", "/api/skills/reload", nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "skills reload: %v\n", err)
			return 1
		}
		resp.Body.Close()
		if !jsonOut {
			fmt.Println("Skills reloaded.")
		}
		return 0
	}
	if subArgs[0] == "validate" && len(subArgs) >= 2 {
		id := subArgs[1]
		resp, err := doRequest(ctx, cfg, "POST", "/api/skills/"+id+"/validate", nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			if strings.Contains(err.Error(), "404") {
				return 2
			}
			fmt.Fprintf(os.Stderr, "skills validate: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			OK    bool                   `json:"ok"`
			Error string                 `json:"error"`
			Skill map[string]interface{} `json:"skill"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		if out.OK {
			fmt.Printf("Skill %s is valid.\n", id)
			return 0
		}
		fmt.Printf("Skill %s is invalid: %s\n", id, out.Error)
		return 1
	}
	if subArgs[0] == "invoke" && len(subArgs) >= 3 {
		return runSkillInvoke(ctx, cfg, subArgs[1], subArgs[2], subArgs[3:], jsonOut)
	}
	if subArgs[0] == "smoke" && len(subArgs) >= 2 {
		return runSkillSmoke(ctx, cfg, subArgs[1], subArgs[2:], jsonOut)
	}
	fmt.Fprintf(os.Stderr, "Usage: navi skills list | show <id> | inspect <id> | reload | validate <id> | invoke <id> <interface> [--args-json '{}'] | smoke <id>\n")
	return 1
}

func runSkillInvoke(ctx context.Context, cfg *Config, skillID, iface string, subArgs []string, jsonOut bool) int {
	var argsJSON string
	localJSON := jsonOut
	fs := flag.NewFlagSet("skills invoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&argsJSON, "args-json", "{}", "JSON object passed as skill arguments")
	fs.BoolVar(&localJSON, "json", jsonOut, "Output as JSON")
	if err := fs.Parse(subArgs); err != nil {
		fmt.Fprintf(os.Stderr, "skills invoke: %v\n", err)
		return 1
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		fmt.Fprintf(os.Stderr, "skills invoke: --args-json must be a JSON object: %v\n", err)
		return 1
	}
	if args == nil {
		args = map[string]any{}
	}
	body, _ := json.Marshal(map[string]any{"arguments": args})
	path := "/api/skills/" + url.PathEscape(skillID) + "/interfaces/" + url.PathEscape(iface) + "/invoke"
	resp, err := doRequest(ctx, cfg, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		if isAuthError(err) {
			return 3
		}
		if strings.Contains(err.Error(), "404") {
			return 2
		}
		fmt.Fprintf(os.Stderr, "skills invoke: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if localJSON {
		fmt.Println(string(raw))
		return 0
	}
	var result struct {
		Status  string          `json:"status"`
		Payload json.RawMessage `json:"payload"`
		Error   *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		fmt.Println(string(raw))
		return 0
	}
	fmt.Printf("%s/%s: %s\n", skillID, iface, result.Status)
	if result.Error != nil {
		fmt.Printf("%s: %s\n", result.Error.Type, result.Error.Message)
		return 1
	}
	if len(result.Payload) > 0 && string(result.Payload) != "null" {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, result.Payload, "", "  "); err == nil {
			fmt.Println(pretty.String())
		} else {
			fmt.Println(string(result.Payload))
		}
	}
	return 0
}

func runSkillSmoke(ctx context.Context, cfg *Config, skillID string, subArgs []string, jsonOut bool) int {
	localJSON := jsonOut
	fs := flag.NewFlagSet("skills smoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&localJSON, "json", jsonOut, "Output as JSON")
	if err := fs.Parse(subArgs); err != nil {
		fmt.Fprintf(os.Stderr, "skills smoke: %v\n", err)
		return 1
	}
	type smokeCall struct {
		Interface string         `json:"interface"`
		Arguments map[string]any `json:"arguments"`
	}
	// Accept Coder-facing skill aliases (navi-coder.*) by resolving to the legacy
	// canonical id. Unrelated ids (e.g. navi.coder.repo) pass through unchanged.
	skillID = coderalias.CanonicalSkillID(skillID)
	var calls []smokeCall
	switch skillID {
	case "navi-programmer.repo-inspect":
		calls = []smokeCall{{Interface: "list_tree", Arguments: map[string]any{"path": ".", "max_entries": 10}}}
	case "navi-programmer.run-validation":
		calls = []smokeCall{
			{Interface: "record_not_run", Arguments: map[string]any{"reason": "smoke test", "validation_kind": "smoke"}},
			{Interface: "run_command", Arguments: map[string]any{"command": []string{"go", "test", "./..."}, "validation_kind": "smoke", "dry_run": true}},
		}
	case "navi.coder.repo":
		calls = []smokeCall{{Interface: "inspect_repo", Arguments: map[string]any{"path": ".", "max_entries": 20}}}
	default:
		fmt.Fprintf(os.Stderr, "skills smoke: no smoke harness for %s\n", skillID)
		return 1
	}
	results := make([]map[string]any, 0, len(calls))
	exitCode := 0
	for _, call := range calls {
		result, code := invokeSkillForSmoke(ctx, cfg, skillID, call.Interface, call.Arguments)
		if code != 0 {
			exitCode = code
		}
		result["interface"] = call.Interface
		results = append(results, result)
	}
	if localJSON {
		data, _ := json.Marshal(map[string]any{"skill_id": skillID, "results": results})
		fmt.Println(string(data))
		return exitCode
	}
	for _, result := range results {
		fmt.Printf("%s/%s: %v\n", skillID, result["interface"], result["status"])
		if errObj, ok := result["error"].(map[string]any); ok {
			fmt.Printf("  %v: %v\n", errObj["type"], errObj["message"])
		}
	}
	return exitCode
}

func invokeSkillForSmoke(ctx context.Context, cfg *Config, skillID, iface string, args map[string]any) (map[string]any, int) {
	body, _ := json.Marshal(map[string]any{"arguments": args})
	path := "/api/skills/" + url.PathEscape(skillID) + "/interfaces/" + url.PathEscape(iface) + "/invoke"
	resp, err := doRequest(ctx, cfg, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return map[string]any{"status": "error", "error": map[string]any{"type": "RequestError", "message": err.Error()}}, 1
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]any{"status": "error", "error": map[string]any{"type": "DecodeError", "message": err.Error()}}, 1
	}
	if result["status"] != "success" {
		return result, 1
	}
	return result, 0
}

func runActivity(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	limit := 20
	if len(subArgs) >= 1 && subArgs[0] == "tail" {
		if len(subArgs) >= 2 {
			fmt.Sscanf(subArgs[1], "%d", &limit)
		}
	}
	path := fmt.Sprintf("/api/activity?limit=%d", limit)
	resp, err := doRequest(ctx, cfg, "GET", path, nil)
	if err != nil {
		if isAuthError(err) {
			return 3
		}
		fmt.Fprintf(os.Stderr, "activity: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if jsonOut {
		fmt.Println(string(b))
		return 0
	}
	var out struct {
		Items []struct {
			At            string `json:"at"`
			Type          string `json:"type"`
			Summary       string `json:"summary"`
			CorrelationID string `json:"correlation_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		return 1
	}
	for _, e := range out.Items {
		fmt.Printf("%s  %s  %s  %s\n", e.At, e.Type, e.Summary, e.CorrelationID)
	}
	return 0
}

func runRuns(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	if len(subArgs) == 0 || subArgs[0] == "list" {
		path := "/api/runs?limit=50"
		resp, err := doRequest(ctx, cfg, "GET", path, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "runs list: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
			return 0
		}
		var out struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			return 1
		}
		for _, r := range out.Items {
			runID, _ := r["run_id"].(string)
			typ, _ := r["type"].(string)
			status, _ := r["status"].(string)
			corr, _ := r["correlation_id"].(string)
			fmt.Printf("%s  %s  %s  %s\n", runID, typ, status, corr)
		}
		return 0
	}
	if subArgs[0] == "show" && len(subArgs) >= 2 {
		id := subArgs[1]
		resp, err := doRequest(ctx, cfg, "GET", "/api/runs/"+id, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			if strings.Contains(err.Error(), "404") {
				return 2
			}
			fmt.Fprintf(os.Stderr, "runs show: %v\n", err)
			return 1
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if jsonOut {
			fmt.Println(string(b))
		} else {
			var r map[string]any
			if err := json.Unmarshal(b, &r); err != nil {
				fmt.Fprintf(os.Stderr, "decode: %v\n", err)
				return 1
			}
			for k, v := range r {
				fmt.Printf("%s: %v\n", k, v)
			}
		}
		return 0
	}
	if subArgs[0] == "trace" && len(subArgs) >= 2 {
		id := subArgs[1]
		path := "/api/debug/events?limit=1000"
		resp, err := doRequest(ctx, cfg, "GET", path, nil)
		if err != nil {
			if isAuthError(err) {
				return 3
			}
			fmt.Fprintf(os.Stderr, "runs trace: %v\n", err)
			return 1
		}
		defer resp.Body.Close()

		var body struct {
			Items []struct {
				Type      string          `json:"type"`
				Timestamp time.Time       `json:"timestamp"`
				Payload   json.RawMessage `json:"payload"`
				Seq       int64           `json:"seq"`
				RunID     string          `json:"run_id"`
			} `json:"items"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			fmt.Fprintf(os.Stderr, "runs trace decode: %v\n", err)
			return 1
		}

		for _, ev := range body.Items {
			if ev.RunID != id {
				continue
			}
			ts := ev.Timestamp.Format(time.RFC3339)
			if jsonOut {
				data, _ := json.Marshal(ev)
				fmt.Println(string(data))
				continue
			}
			fmt.Printf("%s  %s  seq=%d\n", ts, ev.Type, ev.Seq)
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "Usage: navi runs list | show <id> | trace <id>\n")
	return 1
}

func runDev(ctx context.Context, cfg *Config, subArgs []string, jsonOut bool) int {
	if len(subArgs) < 2 || subArgs[0] != "events" || subArgs[1] != "tail" {
		fmt.Fprintf(os.Stderr, "Usage: navi dev events tail [N]\n")
		return 1
	}
	limit := 50
	if len(subArgs) >= 3 {
		fmt.Sscanf(subArgs[2], "%d", &limit)
	}
	path := fmt.Sprintf("/api/debug/events?limit=%d", limit)
	resp, err := doRequest(ctx, cfg, "GET", path, nil)
	if err != nil {
		if isAuthError(err) {
			return 3
		}
		fmt.Fprintf(os.Stderr, "dev events: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if jsonOut {
		fmt.Println(string(b))
		return 0
	}
	var out struct {
		Items []struct {
			Seq           int64  `json:"seq"`
			Type          string `json:"type"`
			CorrelationID string `json:"correlation_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		return 1
	}
	for _, e := range out.Items {
		fmt.Printf("%d  %s  %s\n", e.Seq, e.Type, e.CorrelationID)
	}
	return 0
}

func runDoctor(ctx context.Context, cfg *Config, _ []string, jsonOut bool) int {
	statusResp, err := doRequest(ctx, cfg, "GET", "/api/status", nil)
	if err != nil {
		if isAuthError(err) {
			return 3
		}
		fmt.Fprintf(os.Stderr, "doctor: %v\n", err)
		return 1
	}
	defer statusResp.Body.Close()
	var status map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		fmt.Fprintf(os.Stderr, "doctor: decode status: %v\n", err)
		return 1
	}
	checks := []struct {
		name string
		ok   bool
		msg  string
	}{
		{"gateway", status["gateway"] != nil, "reachable"},
		{"setup", getBool(status, "setup", "complete"), "complete"},
		{"governor_tripped", !getBool(status, "governor", "tripped"), "not tripped"},
		{"degraded", !getBool(status, "degraded"), "system healthy"},
	}
	if gov, _ := status["governor"].(map[string]any); gov != nil {
		if getBool(gov, "tripped") {
			checks[2].ok = false
			checks[2].msg = "governor tripped"
		}
	}
	if v, _ := status["degraded"].(bool); v {
		checks[3].ok = false
		checks[3].msg = "system degraded"
	}
	if jsonOut {
		out := make([]map[string]any, len(checks))
		for i, c := range checks {
			out[i] = map[string]any{"check": c.name, "result": "ok", "message": c.msg}
			if !c.ok {
				out[i]["result"] = "fail"
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(out)
		return 0
	}
	allOk := true
	for _, c := range checks {
		result := "ok"
		if !c.ok {
			result = "fail"
			allOk = false
		}
		fmt.Printf("%s: %s  %s\n", c.name, result, c.msg)
	}
	if !allOk {
		return 1
	}
	return 0
}

func getBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			return false
		}
		if sub, ok := v.(map[string]any); ok {
			m = sub
			continue
		}
		if b, ok := v.(bool); ok {
			return b
		}
		return false
	}
	return false
}

func runRuntimeSessionsCommand(ctx context.Context, cfg *Config, currentChatID string) {
	resp, err := doRequest(ctx, cfg, "GET", "/api/navi/runtime-sessions", nil)
	if err != nil {
		fmt.Printf("\n/sessions error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var sessions []struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		fmt.Printf("\n/sessions decode error: %v\n", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("\nNo runtime sessions found.")
		return
	}

	fmt.Println("\nSessions:")
	for _, s := range sessions {
		marker := " "
		if s.ChatID == currentChatID {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, s.ChatID)
	}
}

func runModelListCommand(ctx context.Context, cfg *Config) {
	activeResp, err := doRequest(ctx, cfg, "GET", "/api/llm/active", nil)
	if err != nil {
		fmt.Printf("\n/model error (active): %v\n", err)
		return
	}
	defer activeResp.Body.Close()
	var active struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(activeResp.Body).Decode(&active); err != nil {
		fmt.Printf("\n/model error (active decode): %v\n", err)
		return
	}
	fmt.Printf("\nCurrent: %s\n", active.Status)

	catalogResp, err := doRequest(ctx, cfg, "GET", "/api/llm/catalog", nil)
	if err != nil {
		fmt.Printf("\n/model error (catalog): %v\n", err)
		return
	}
	defer catalogResp.Body.Close()
	var catalog struct {
		Providers []struct {
			Key         string `json:"key"`
			DisplayName string `json:"display_name"`
			Models      []struct {
				Name string `json:"name"`
			} `json:"models"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(catalogResp.Body).Decode(&catalog); err != nil {
		fmt.Printf("\n/model error (catalog decode): %v\n", err)
		return
	}
	fmt.Println("Available:")
	for _, p := range catalog.Providers {
		disp := p.DisplayName
		if disp == "" {
			disp = p.Key
		}
		for _, m := range p.Models {
			current := ""
			if p.Key == active.Provider && m.Name == active.Model {
				current = " (current)"
			}
			fmt.Printf("  %s/%s%s\n", p.Key, m.Name, current)
		}
	}
}

var modelSetReservedWords = map[string]bool{
	"status": true, "list": true, "exit": true, "connect": true,
	"debug": true, "trace": true, "approve": true, "deny": true,
	"sessions": true, "use": true, "quit": true,
}

func runModelSetCommand(ctx context.Context, cfg *Config, args string) {
	var provider, model string
	if strings.Contains(args, "/") {
		parts := strings.SplitN(args, "/", 2)
		provider = strings.TrimSpace(parts[0])
		model = strings.TrimSpace(parts[1])
	} else {
		parts := strings.Fields(args)
		if len(parts) < 2 {
			fmt.Println("\n/model set requires <provider> <model> or <provider/model>, e.g. /model set ollama llama3.2")
			return
		}
		provider = parts[0]
		model = parts[1]
	}
	if provider == "" || model == "" {
		fmt.Println("\n/model set requires non-empty provider and model.")
		return
	}
	if modelSetReservedWords[strings.ToLower(model)] || modelSetReservedWords[strings.ToLower(provider)] {
		fmt.Printf("\n/model set: %q looks like a command, not a model name. Use e.g. /model set ollama llama3.1:latest\n", model)
		return
	}
	body, _ := json.Marshal(map[string]string{"provider": provider, "model": model})
	resp, err := doRequest(ctx, cfg, "PUT", "/api/llm/active", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("\n/model set error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("\n/model set failed: %s\n", string(b))
		return
	}
	var out struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Status   string `json:"status"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) == nil {
		fmt.Printf("\nSwitched to %s\n", out.Status)
	} else {
		fmt.Println("\nModel updated.")
	}
}

func runUseSessionCommand(ctx context.Context, cfg *Config, targetSession string) (string, bool) {
	resp, err := doRequest(ctx, cfg, "GET", fmt.Sprintf("/api/navi/sessions/%s", targetSession), nil)
	if err != nil {
		fmt.Printf("\n/use error: %v\n", err)
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("\n/use failed: %s\n", string(b))
		return "", false
	}

	return targetSession, true
}

func getOrCreateRuntimeSession(ctx context.Context, cfg *Config) (string, error) {
	if cfg.ResumeLastRuntimeSession {
		resp, err := doRequest(ctx, cfg, "GET", "/api/navi/runtime-sessions", nil)
		if err != nil {
			return "", fmt.Errorf("could not connect to gateway: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			var runtimeSessions []RuntimeSessionInfo
			if err := json.NewDecoder(resp.Body).Decode(&runtimeSessions); err == nil && len(runtimeSessions) > 0 {
				for i := 0; i < len(runtimeSessions); i++ {
					runtimeSessionID := runtimeSessions[i].RuntimeSessionID
					if runtimeSessionID != "heartbeat-auto" && runtimeSessionID != "" {
						resp.Body.Close()
						return runtimeSessionID, nil
					}
				}
			}
		}
		resp.Body.Close()
	}

	body := map[string]interface{}{}
	reqBody, _ := json.Marshal(body)
	resp, err := doRequest(ctx, cfg, "POST", "/api/navi/runtime-sessions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var runtimeSession struct {
		RuntimeSessionID string `json:"runtime_session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runtimeSession); err != nil {
		return "", err
	}

	if runtimeSession.RuntimeSessionID == "" {
		return "", fmt.Errorf("server returned success but empty runtime_session_id")
	}

	return runtimeSession.RuntimeSessionID, nil
}

func sendMessage(ctx context.Context, cfg *Config, runtimeSessionID, content string) error {
	now := time.Now().UTC().UnixNano()
	reqBody, _ := json.Marshal(map[string]string{
		"content":            content,
		"source_channel":     "cli",
		"source_message_ref": fmt.Sprintf("cli:%s:%d", runtimeSessionID, now),
		"idempotency_key":    fmt.Sprintf("cli:%s:%d", runtimeSessionID, now),
	})
	resp, err := doRequest(ctx, cfg, "POST", fmt.Sprintf("/api/navi/runtime-sessions/%s/message", runtimeSessionID), bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func fetchMessageText(ctx context.Context, cfg *Config, runtimeSessionID, messageID string) (string, error) {
	resp, err := doRequest(ctx, cfg, "GET", fmt.Sprintf("/api/navi/runtime-sessions/%s", runtimeSessionID), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var runtimeSession struct {
		Messages []struct {
			ID      string `json:"message_id"`
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runtimeSession); err != nil {
		return "", err
	}

	for _, msg := range runtimeSession.Messages {
		if msg.ID == messageID {
			return msg.Content, nil
		}
	}
	return "", fmt.Errorf("message %s not found", messageID)
}

// latestAssistantMessageID returns the ID of the most recent NAVI chat message.
func latestAssistantMessageID(ctx context.Context, cfg *Config, runtimeSessionID string) (string, error) {
	resp, err := doRequest(ctx, cfg, "GET", fmt.Sprintf("/api/navi/runtime-sessions/%s", runtimeSessionID), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var runtimeSession struct {
		Messages []struct {
			ID   string `json:"message_id"`
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runtimeSession); err != nil {
		return "", err
	}
	for i := len(runtimeSession.Messages) - 1; i >= 0; i-- {
		msg := runtimeSession.Messages[i]
		if msg.Role == "navi" {
			return msg.ID, nil
		}
	}
	return "", nil
}

// latestAssistantMessage returns the ID and content of the most recent NAVI chat message.
func latestAssistantMessage(ctx context.Context, cfg *Config, runtimeSessionID string) (string, string, error) {
	resp, err := doRequest(ctx, cfg, "GET", fmt.Sprintf("/api/navi/runtime-sessions/%s", runtimeSessionID), nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var runtimeSession struct {
		Messages []struct {
			ID      string `json:"message_id"`
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runtimeSession); err != nil {
		return "", "", err
	}
	for i := len(runtimeSession.Messages) - 1; i >= 0; i-- {
		msg := runtimeSession.Messages[i]
		if msg.Role == "navi" && msg.Content != "" {
			return msg.ID, msg.Content, nil
		}
	}
	return "", "", nil
}

// runLogs polls the operator debug event API and streams recent events.
func runLogs(ctx context.Context, cfg *Config, jsonOutput bool) error {
	fmt.Printf("Streaming events from %s/api/debug/events (Ctrl+C to stop)\n", cfg.GatewayURL)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	afterSeq := int64(0)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			fmt.Println("\nStopping event stream.")
			return nil
		case <-ticker.C:
			resp, err := doRequest(ctx, cfg, "GET", fmt.Sprintf("/api/debug/events?after_seq=%d&limit=100", afterSeq), nil)
			if err != nil {
				return err
			}
			var body struct {
				Items []struct {
					Type       string          `json:"type"`
					Timestamp  time.Time       `json:"timestamp"`
					Payload    json.RawMessage `json:"payload"`
					Seq        int64           `json:"seq"`
					RunID      string          `json:"run_id"`
					Visibility string          `json:"visibility"`
				} `json:"items"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				resp.Body.Close()
				return err
			}
			resp.Body.Close()

			for _, item := range body.Items {
				if item.Seq > afterSeq {
					afterSeq = item.Seq
				}
				if jsonOutput {
					data, _ := json.Marshal(item)
					fmt.Println(string(data))
					continue
				}
				ts := item.Timestamp.Format(time.RFC3339)
				fmt.Printf("%s | %-28s | run=%s | %s\n", ts, item.Type, item.RunID, item.Visibility)
			}
		}
	}
}
