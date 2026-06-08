package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	defaultOwnerName   = "E2E Owner"
	defaultOwnerHandle = "e2e-owner"
	defaultOwnerSecret = "navi-e2e-owner-secret"
	defaultSharedKey   = "navi-e2e-shared-secret"
)

type Stack struct {
	repoRoot     string
	tempDir      string
	projectName  string
	composeFile  string
	runtimeFile  string
	dataDir      string
	artifactsDir string
	httpClient   *http.Client

	gatewayURL      string
	fakeOllamaURL   string
	fakeTelegramURL string
	apiKey          string
	ownerSecret     string

	cliOnce   sync.Once
	cliBinary string
	cliErr    error
}

type ClaimResponse struct {
	APIKey      string `json:"primary_api_key"`
	AdminSecret string `json:"admin_secret"`
}

type APIKeyResponse struct {
	APIKey string `json:"api_key"`
}

type FakeOllamaScenario struct {
	Mode       string   `json:"mode"`
	Models     []string `json:"models,omitempty"`
	Chunks     []string `json:"chunks,omitempty"`
	DelayMS    int      `json:"delay_ms,omitempty"`
	StatusCode int      `json:"status_code,omitempty"`
	Message    string   `json:"message,omitempty"`
	ToolName   string   `json:"tool_name,omitempty"`
}

type FakeTelegramMode struct {
	HealthStatus int `json:"health_status,omitempty"`
	SendStatus   int `json:"send_status,omitempty"`
	EditStatus   int `json:"edit_status,omitempty"`
	DelayMS      int `json:"delay_ms,omitempty"`
	PollWaitMS   int `json:"poll_wait_ms,omitempty"`
}

type FakeTelegramInbound struct {
	ChatID          int64  `json:"chat_id"`
	Text            string `json:"text"`
	MessageID       int64  `json:"message_id,omitempty"`
	MessageThreadID int64  `json:"message_thread_id,omitempty"`
}

func FindRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		next := filepath.Dir(wd)
		if next == wd {
			return "", fmt.Errorf("could not find go.mod from %s", wd)
		}
		wd = next
	}
}

func NewStack(repoRoot string) (*Stack, error) {
	tempDir, err := os.MkdirTemp("", "navi-e2e-*")
	if err != nil {
		return nil, err
	}
	projectName := "navi-e2e-" + strings.ToLower(strings.ReplaceAll(uuid.NewString()[:8], "_", ""))
	artifactsDir := filepath.Join(repoRoot, "test", "e2e", "artifacts", projectName)
	s := &Stack{
		repoRoot:     repoRoot,
		tempDir:      tempDir,
		projectName:  projectName,
		composeFile:  filepath.Join(tempDir, "compose.yaml"),
		runtimeFile:  filepath.Join(tempDir, "runtime.yaml"),
		dataDir:      filepath.Join(tempDir, "data"),
		artifactsDir: artifactsDir,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		ownerSecret:  defaultOwnerSecret,
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.artifactsDir, 0o755); err != nil {
		return nil, err
	}
	if err := s.writeRuntimeConfig(); err != nil {
		return nil, err
	}
	if err := s.writeComposeFile(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Stack) Cleanup() error {
	return os.RemoveAll(s.tempDir)
}

func (s *Stack) RepoRoot() string {
	return s.repoRoot
}

func (s *Stack) GatewayURL() string {
	return s.gatewayURL
}

func (s *Stack) FakeOllamaURL() string {
	return s.fakeOllamaURL
}

func (s *Stack) FakeTelegramURL() string {
	return s.fakeTelegramURL
}

func (s *Stack) APIKey() string {
	return s.apiKey
}

func (s *Stack) OwnerSecret() string {
	return s.ownerSecret
}

func (s *Stack) ArtifactsDir() string {
	return s.artifactsDir
}

func (s *Stack) Up(ctx context.Context) error {
	if _, err := s.compose(ctx, "up", "--build", "-d"); err != nil {
		return err
	}
	gatewayURL, err := s.resolveServiceURL(ctx, "navid", "6284")
	if err != nil {
		return err
	}
	ollamaURL, err := s.resolveServiceURL(ctx, "fake-ollama", "11434")
	if err != nil {
		return err
	}
	telegramURL, err := s.resolveServiceURL(ctx, "fake-telegram", "8080")
	if err != nil {
		return err
	}
	s.gatewayURL = gatewayURL
	s.fakeOllamaURL = ollamaURL
	s.fakeTelegramURL = telegramURL
	if err := waitForOK(ctx, s.httpClient, s.gatewayURL+"/health"); err != nil {
		return err
	}
	if err := waitForOK(ctx, s.httpClient, s.fakeOllamaURL+"/health"); err != nil {
		return err
	}
	if err := waitForOK(ctx, s.httpClient, s.fakeTelegramURL+"/health"); err != nil {
		return err
	}
	return nil
}

func (s *Stack) Down(ctx context.Context) error {
	_, err := s.compose(ctx, "down", "--volumes", "--remove-orphans")
	return err
}

func (s *Stack) Restart(ctx context.Context, services ...string) error {
	args := []string{"restart"}
	if len(services) == 0 {
		args = append(args, "navid")
	} else {
		args = append(args, services...)
	}
	if _, err := s.compose(ctx, args...); err != nil {
		return err
	}

	if gatewayURL, err := s.resolveServiceURL(ctx, "navid", "6284"); err == nil {
		s.gatewayURL = gatewayURL
	}
	if ollamaURL, err := s.resolveServiceURL(ctx, "fake-ollama", "11434"); err == nil {
		s.fakeOllamaURL = ollamaURL
	}
	if telegramURL, err := s.resolveServiceURL(ctx, "fake-telegram", "8080"); err == nil {
		s.fakeTelegramURL = telegramURL
	}

	if err := waitForOK(ctx, s.httpClient, s.gatewayURL+"/health"); err != nil {
		return err
	}
	if s.fakeOllamaURL != "" {
		if err := waitForOK(ctx, s.httpClient, s.fakeOllamaURL+"/health"); err != nil {
			return err
		}
	}
	if s.fakeTelegramURL != "" {
		if err := waitForOK(ctx, s.httpClient, s.fakeTelegramURL+"/health"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stack) Claim(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"owner_name":   defaultOwnerName,
		"owner_handle": defaultOwnerHandle,
		"owner_secret": s.ownerSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL+"/api/onboarding/claim", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("claim failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	var out ClaimResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	s.apiKey = out.APIKey
	if out.AdminSecret != "" {
		s.ownerSecret = out.AdminSecret
	}
	return nil
}

func (s *Stack) ResetAndClaim(ctx context.Context) error {
	needsRestart := false
	if s.apiKey != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL+"/api/instance/reset", nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", s.apiKey)
		req.Header.Set("X-Owner-Secret", s.ownerSecret)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("reset failed: %s", resp.Status)
		}
		needsRestart = true
	}
	if needsRestart {
		if err := s.Restart(ctx, "navid"); err != nil {
			return err
		}
	}
	s.apiKey = ""
	if err := s.ResetFakes(ctx); err != nil {
		return err
	}
	return s.Claim(ctx)
}

func (s *Stack) NewAPIKey(ctx context.Context, name string, scopes []string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"name":   name,
		"scopes": scopes,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gatewayURL+"/api/keys", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.apiKey)
	req.Header.Set("X-Owner-Secret", s.ownerSecret)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create key failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	var out APIKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.APIKey, nil
}

func (s *Stack) ResetFakes(ctx context.Context) error {
	if err := s.adminPOST(ctx, s.fakeOllamaURL+"/__admin/reset", nil); err != nil {
		return err
	}
	return s.adminPOST(ctx, s.fakeTelegramURL+"/__admin/reset", nil)
}

func (s *Stack) SetOllamaScenario(ctx context.Context, scenario FakeOllamaScenario) error {
	return s.adminPOST(ctx, s.fakeOllamaURL+"/__admin/scenario", scenario)
}

func (s *Stack) OllamaRequests(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	err := s.adminGET(ctx, s.fakeOllamaURL+"/__admin/requests", &out)
	return out, err
}

func (s *Stack) SetTelegramMode(ctx context.Context, mode FakeTelegramMode) error {
	return s.adminPOST(ctx, s.fakeTelegramURL+"/__admin/mode", mode)
}

func (s *Stack) TelegramInjectMessage(ctx context.Context, msg FakeTelegramInbound) error {
	return s.adminPOST(ctx, s.fakeTelegramURL+"/__admin/inbound", msg)
}

func (s *Stack) TelegramOutbound(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	err := s.adminGET(ctx, s.fakeTelegramURL+"/__admin/outbound", &out)
	return out, err
}

func (s *Stack) TelegramRequests(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	err := s.adminGET(ctx, s.fakeTelegramURL+"/__admin/requests", &out)
	return out, err
}

func (s *Stack) BuildCLI(ctx context.Context) (string, error) {
	s.cliOnce.Do(func() {
		s.cliBinary = filepath.Join(s.tempDir, "bin", "navi")
		if err := os.MkdirAll(filepath.Dir(s.cliBinary), 0o755); err != nil {
			s.cliErr = err
			return
		}
		cmd := exec.CommandContext(ctx, "go", "build", "-o", s.cliBinary, "./cmd/navi")
		cmd.Dir = s.repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			s.cliErr = fmt.Errorf("build cli: %w\n%s", err, string(out))
		}
	})
	return s.cliBinary, s.cliErr
}

func (s *Stack) AttachArtifacts(testName string, failed func() bool) func() {
	return func() {
		if !failed() {
			return
		}
		_ = s.dumpArtifacts(context.Background(), sanitizeName(testName))
	}
}

func (s *Stack) Logs(ctx context.Context, services ...string) (string, error) {
	args := []string{"logs", "--no-color"}
	args = append(args, services...)
	return s.compose(ctx, args...)
}

func (s *Stack) APIRequest(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.gatewayURL+path, reader)
	if err != nil {
		return err
	}
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(data))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Stack) dumpArtifacts(ctx context.Context, name string) error {
	caseDir := filepath.Join(s.artifactsDir, name)
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return err
	}
	if out, err := s.compose(ctx, "ps"); err == nil {
		_ = os.WriteFile(filepath.Join(caseDir, "compose-ps.txt"), []byte(out), 0o644)
	}
	if out, err := s.compose(ctx, "logs", "--no-color"); err == nil {
		_ = os.WriteFile(filepath.Join(caseDir, "compose-logs.txt"), []byte(out), 0o644)
	}
	if s.gatewayURL != "" {
		s.dumpHTTP(ctx, caseDir, "health.json", http.MethodGet, s.gatewayURL+"/health", "", nil)
		s.dumpHTTP(ctx, caseDir, "onboarding-status.json", http.MethodGet, s.gatewayURL+"/api/onboarding/status", "", nil)
		if s.apiKey != "" {
			s.dumpHTTP(ctx, caseDir, "status.json", http.MethodGet, s.gatewayURL+"/api/status", s.apiKey, nil)
			s.dumpHTTP(ctx, caseDir, "runs.json", http.MethodGet, s.gatewayURL+"/api/runs?limit=50", s.apiKey, nil)
			s.dumpHTTP(ctx, caseDir, "activity.json", http.MethodGet, s.gatewayURL+"/api/activity?limit=50", s.apiKey, nil)
			s.dumpHTTP(ctx, caseDir, "connectors.json", http.MethodGet, s.gatewayURL+"/api/connectors", s.apiKey, nil)
			s.dumpHTTP(ctx, caseDir, "connector-health.json", http.MethodGet, s.gatewayURL+"/api/health/connectors", s.apiKey, nil)
		}
	}
	if s.fakeOllamaURL != "" {
		s.dumpHTTP(ctx, caseDir, "fake-ollama-requests.json", http.MethodGet, s.fakeOllamaURL+"/__admin/requests", "", nil)
	}
	if s.fakeTelegramURL != "" {
		s.dumpHTTP(ctx, caseDir, "fake-telegram-outbound.json", http.MethodGet, s.fakeTelegramURL+"/__admin/outbound", "", nil)
		s.dumpHTTP(ctx, caseDir, "fake-telegram-requests.json", http.MethodGet, s.fakeTelegramURL+"/__admin/requests", "", nil)
	}
	return nil
}

func (s *Stack) dumpHTTP(ctx context.Context, dir, name, method, url, apiKey string, body []byte) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	_ = os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

func (s *Stack) compose(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "-p", s.projectName, "-f", s.composeFile}, args...)...)
	cmd.Dir = s.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func (s *Stack) resolveServiceURL(ctx context.Context, service, containerPort string) (string, error) {
	out, err := s.compose(ctx, "port", service, containerPort)
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(out)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("parse compose port %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func (s *Stack) adminPOST(ctx context.Context, url string, payload any) error {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin post %s failed: status=%d body=%s", url, resp.StatusCode, string(b))
	}
	return nil
}

func (s *Stack) adminGET(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin get %s failed: status=%d body=%s", url, resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Stack) writeRuntimeConfig() error {
	cfg := map[string]any{
		"gateway": map[string]any{
			"addr":          ":6284",
			"shared_secret": defaultSharedKey,
			"static_dir":    "web",
			"origin_patterns": []string{
				"http://localhost:6284",
				"http://127.0.0.1:6284",
			},
		},
		"nats": map[string]any{
			"url": "embedded",
		},
		"sqlite": map[string]any{
			"path": "/navi/data/navi.db",
		},
		"connectors": map[string]any{
			"telegram": map[string]any{
				"bot_token":     "",
				"owner_chat_id": 0,
				"allow_from":    []int64{},
				"pairing_code":  "",
				"gateway_url":   "http://localhost:6284",
				"api_url":       "http://fake-telegram:8080",
				"accounts":      []any{},
			},
			"slack": map[string]any{
				"bot_token":        "",
				"app_token":        "",
				"gateway_url":      "http://localhost:6284",
				"allowed_user_ids": []string{},
			},
		},
		"llm": map[string]any{
			"ollama_url":           "http://fake-ollama:11434/v1",
			"ollama_model":         "llama3:latest",
			"ollama_discuss_model": "llama3:latest",
			"orchestrator_model":   "llama3:latest",
			"coder_model":          "llama3:latest",
			"chat_model":           "llama3:latest",
			"providers": map[string]any{
				"ollama": map[string]any{
					"display_name": "Ollama",
					"models":       []string{"llama3:latest", "mistral:latest"},
				},
			},
		},
		"lsp": map[string]any{
			"workspace_root": "",
		},
		"navi": map[string]any{
			"workspace_dir":      "/navi/data/workspace",
			"personas_dir":       "config/personas",
			"initial_persona":    "navi",
			"heartbeat_enabled":  false,
			"heartbeat_interval": "30m",
			"debug":              true,
		},
		"governor": map[string]any{
			"max_action_budget":   1000,
			"max_retries":         5,
			"cost_ceiling":        50.0,
			"autonomous_duration": "72h",
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(s.runtimeFile, data, 0o644)
}

func (s *Stack) writeComposeFile() error {
	compose := map[string]any{
		"services": map[string]any{
			"navid": map[string]any{
				"build": map[string]any{
					"context":    s.repoRoot,
					"dockerfile": "Dockerfile",
				},
				"ports": []string{"127.0.0.1::6284"},
				"volumes": []string{
					fmt.Sprintf("%s:/navi/data", s.dataDir),
					fmt.Sprintf("%s:/navi/config/runtime.yaml:ro", s.runtimeFile),
				},
				"environment": map[string]any{
					"NAVI_SQLITE_PATH":           "/navi/data/navi.db",
					"NAVI_GATEWAY_SHARED_SECRET": defaultSharedKey,
				},
				"depends_on": []string{"fake-ollama", "fake-telegram"},
				"healthcheck": map[string]any{
					"test":         []string{"CMD", "wget", "-qO-", "http://localhost:6284/health"},
					"interval":     "5s",
					"timeout":      "5s",
					"retries":      12,
					"start_period": "5s",
				},
			},
			"fake-ollama": map[string]any{
				"build": map[string]any{
					"context":    s.repoRoot,
					"dockerfile": "test/e2e/fakes/ollama/Dockerfile",
				},
				"ports": []string{"127.0.0.1::11434"},
			},
			"fake-telegram": map[string]any{
				"build": map[string]any{
					"context":    s.repoRoot,
					"dockerfile": "test/e2e/fakes/telegram/Dockerfile",
				},
				"ports": []string{"127.0.0.1::8080"},
			},
		},
	}
	data, err := yaml.Marshal(compose)
	if err != nil {
		return err
	}
	return os.WriteFile(s.composeFile, data, 0o644)
}

func waitForOK(ctx context.Context, client *http.Client, url string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", url)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-", "\\", "-")
	return replacer.Replace(name)
}
