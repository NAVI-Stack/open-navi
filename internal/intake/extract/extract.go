// Package extract is the Go boundary that invokes the Python extraction +
// resolution worker (CIP stage 7 + the resolution half of the synthesis seam,
// docs/design/intake-synthesis-seam.md §9). It reuses the JSON-RPC-over-stdio
// subprocess contract from internal/navi/skill/subprocess_runtime.go — one
// subprocess pattern across all P3 Python work, no new IPC.
//
// The dataflow is strictly Python -> Go: Python proposes ExtractionResult and
// ResolutionResult envelopes; Go (synthesize) decides. Python never writes the
// World Model. When the Python runtime is unavailable, Available() reports false
// and the synthesizer falls back to the deterministic entity spans P2 already
// recorded on each chunk — honest degradation, never a silent failure.
package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

const defaultTimeout = 30 * time.Second

// ExistingEntity is a World Model entity offered to the resolver as a match
// candidate. Go assembles these (personal-assistant scale) before the call;
// Python resolves against them but never reaches the store itself.
type ExistingEntity struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	BlockingKeys map[string]string `json:"blocking_keys,omitempty"`
}

// ProcessRequest is one chunk plus the existing-entity pool to resolve against.
type ProcessRequest struct {
	ChunkID          string
	Content          string
	ExistingEntities []ExistingEntity
}

// ProcessResult is the structured envelope returned by the worker.
type ProcessResult struct {
	Extraction  schema.ExtractionResult
	Resolutions []schema.ResolutionResult
}

// Runner invokes the Python worker. Command is the argv (e.g. ["python",
// "/abs/python/intake/worker.py"]); Timeout bounds each call.
type Runner struct {
	Command []string
	Timeout time.Duration
	Log     *slog.Logger
}

// jsonrpcRequest mirrors the skill subprocess runtime request shape.
type jsonrpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type processPayload struct {
	Extraction  schema.ExtractionResult   `json:"extraction"`
	Resolutions []schema.ResolutionResult `json:"resolutions"`
}

// NewRunner builds a Runner, locating the python executable and worker script by
// walking up from the current working directory. Both fields are empty when the
// runtime cannot be found; callers should check Available().
func NewRunner(log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	r := &Runner{Timeout: defaultTimeout, Log: log}
	py := locatePython()
	script := locateWorkerScript()
	if py != "" && script != "" {
		r.Command = []string{py, script}
	}
	return r
}

// Available reports whether the Python worker can be invoked.
func (r *Runner) Available() bool {
	if len(r.Command) == 0 {
		return false
	}
	if _, err := os.Stat(r.Command[len(r.Command)-1]); err != nil {
		return false
	}
	return true
}

// Process runs extraction + resolution for one chunk in a single subprocess call.
func (r *Runner) Process(ctx context.Context, req ProcessRequest) (ProcessResult, error) {
	if !r.Available() {
		return ProcessResult{}, fmt.Errorf("extract: python worker unavailable")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	existing := req.ExistingEntities
	if existing == nil {
		existing = []ExistingEntity{}
	}
	payload := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      uuid.NewString(),
		Method:  "process",
		Params: map[string]any{
			"chunk_id":          req.ChunkID,
			"content":           req.Content,
			"existing_entities": existing,
		},
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("extract: marshal request: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, r.Command[0], r.Command[1:]...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8")
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return ProcessResult{}, fmt.Errorf("extract: worker timeout after %s", timeout)
		}
		return ProcessResult{}, fmt.Errorf("extract: worker run: %w: %s", err, stderr.String())
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &resp); err != nil {
		return ProcessResult{}, fmt.Errorf("extract: decode response: %w (stderr: %s)", err, stderr.String())
	}
	if resp.ID != nil && fmt.Sprint(resp.ID) != payload.ID {
		return ProcessResult{}, fmt.Errorf("extract: response id %v does not match request %s", resp.ID, payload.ID)
	}
	if resp.Error != nil {
		return ProcessResult{}, fmt.Errorf("extract: worker error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	var pp processPayload
	if err := json.Unmarshal(resp.Result, &pp); err != nil {
		return ProcessResult{}, fmt.Errorf("extract: decode result: %w", err)
	}
	return ProcessResult{Extraction: pp.Extraction, Resolutions: pp.Resolutions}, nil
}

// locatePython resolves a python interpreter from PATH.
func locatePython() string {
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3", "py"}
	}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// locateWorkerScript walks up from the working directory to find
// python/intake/worker.py, so it resolves from both the repo root (navid) and a
// package test directory.
func locateWorkerScript() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(dir, "python", "intake", "worker.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fall back to an env override (set by navid in containers).
	if p := os.Getenv("NAVI_INTAKE_WORKER"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
