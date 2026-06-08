package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

var defaultCommandAllowlist = map[string]struct{}{
	"bun":     {},
	"cargo":   {},
	"dotnet":  {},
	"eslint":  {},
	"go":      {},
	"gradle":  {},
	"javac":   {},
	"jest":    {},
	"make":    {},
	"mvn":     {},
	"node":    {},
	"npm":     {},
	"pnpm":    {},
	"poetry":  {},
	"py":      {},
	"pytest":  {},
	"python":  {},
	"python3": {},
	"ruff":    {},
	"tsc":     {},
	"uv":      {},
	"vitest":  {},
	"yarn":    {},
}

var shellOperatorTokens = map[string]struct{}{
	"|": {}, "||": {}, "&": {}, "&&": {}, ";": {}, "<": {}, ">": {}, ">>": {}, "2>": {}, "2>>": {},
}

// Runner executes project commands inside a bounded sandbox.
type Runner interface {
	Prepare(ctx context.Context, req PrepareRequest) (Handle, error)
	Run(ctx context.Context, handle Handle, cmd Command) (Result, error)
	Cleanup(ctx context.Context, handle Handle) error
}

type PrepareRequest struct {
	Profile       schema.SandboxProfile
	WorkspaceRoot string
	RunID         string
	ExtraMounts   []Mount
}

type Handle struct {
	ID            string
	Profile       schema.SandboxProfile
	WorkspaceRoot string
	MountTarget   string
	ExecContainer string
	RunID         string
	ExtraMounts   []Mount
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type Command struct {
	Args      []string
	Workdir   string
	Env       map[string]string
	TimeoutMS int
	Stdin     []byte
}

type Result struct {
	SandboxProfileID string   `json:"sandbox_profile_id"`
	Runtime          string   `json:"runtime"`
	NetworkMode      string   `json:"network_mode"`
	Command          []string `json:"command"`
	CommandDisplay   string   `json:"command_display"`
	ExitCode         int      `json:"exit_code"`
	Stdout           string   `json:"stdout"`
	Stderr           string   `json:"stderr"`
	DurationMS       int      `json:"duration_ms"`
	TimedOut         bool     `json:"timed_out"`
	Verdict          string   `json:"verdict"`
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) FailureClass() schema.FailureClass {
	return schema.FailureClassSandboxBlocked
}

type commandResult struct {
	stdout   string
	stderr   string
	exitCode int
	timedOut bool
}

type commandFunc func(ctx context.Context, executable string, args []string, stdin []byte, timeout time.Duration) (commandResult, error)
type lookPathFunc func(file string) (string, error)

// DockerRunner runs commands with `docker run --rm`, mounting a single project
// workspace into the container and disabling network egress by default.
type DockerRunner struct {
	DockerBinary string
	lookPath     lookPathFunc
	runCommand   commandFunc
}

func NewDockerRunner() *DockerRunner {
	return &DockerRunner{DockerBinary: "docker"}
}

func (r *DockerRunner) Prepare(ctx context.Context, req PrepareRequest) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	profile := req.Profile
	if strings.TrimSpace(profile.ID) == "" {
		return Handle{}, sandboxError("profile_missing", "sandbox profile is required")
	}
	if profile.Status != schema.SandboxProfileStatusActive {
		return Handle{}, sandboxError("profile_inactive", "sandbox profile is not active")
	}
	if profile.Runtime != schema.SandboxRuntimeDocker {
		return Handle{}, sandboxError("runtime_unsupported", "sandbox profile runtime is not supported by DockerRunner")
	}
	if strings.TrimSpace(profile.Image) == "" {
		return Handle{}, sandboxError("image_missing", "sandbox profile image is required")
	}
	root, err := filepath.Abs(strings.TrimSpace(req.WorkspaceRoot))
	if err != nil {
		return Handle{}, fmt.Errorf("sandbox: resolve workspace root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Handle{}, sandboxError("workspace_unavailable", fmt.Sprintf("workspace root unavailable: %v", err))
	}
	if !info.IsDir() {
		return Handle{}, sandboxError("workspace_not_directory", "workspace root must be a directory")
	}
	docker := r.dockerBinary()
	if _, err := r.lookup(docker); err != nil {
		return Handle{}, sandboxError("docker_unavailable", "docker executable is unavailable")
	}
	mountTarget := strings.TrimSpace(profile.WorkspaceMountTarget)
	if mountTarget == "" {
		mountTarget = "/workspace"
	}
	extraMounts, err := normalizeExtraMounts(req.ExtraMounts)
	if err != nil {
		return Handle{}, err
	}
	return Handle{
		ID:            firstNonEmpty(req.RunID, profile.ID),
		Profile:       profile,
		WorkspaceRoot: root,
		MountTarget:   mountTarget,
		ExecContainer: dockerExecContainer(),
		RunID:         req.RunID,
		ExtraMounts:   extraMounts,
	}, nil
}

func (r *DockerRunner) Run(ctx context.Context, handle Handle, cmd Command) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(handle.WorkspaceRoot) == "" {
		return Result{}, sandboxError("handle_invalid", "sandbox handle is missing workspace root")
	}
	if err := validateCommand(handle.Profile, cmd); err != nil {
		return Result{}, err
	}
	timeout := commandTimeout(handle.Profile, cmd.TimeoutMS)
	dockerArgs, displayCommand, err := r.dockerArgs(handle, cmd)
	if err != nil {
		return Result{}, err
	}
	start := time.Now()
	ran, err := r.run(ctx, r.dockerBinary(), dockerArgs, cmd.Stdin, timeout)
	durationMS := int(time.Since(start).Milliseconds())
	result := Result{
		SandboxProfileID: handle.Profile.ID,
		Runtime:          string(handle.Profile.Runtime),
		NetworkMode:      string(handle.Profile.NetworkMode),
		Command:          cmd.Args,
		CommandDisplay:   displayCommand,
		ExitCode:         ran.exitCode,
		Stdout:           ran.stdout,
		Stderr:           ran.stderr,
		DurationMS:       durationMS,
		TimedOut:         ran.timedOut,
		Verdict:          verdictFor(ran.exitCode, ran.timedOut),
	}
	if err != nil {
		if ran.timedOut || errors.Is(err, context.DeadlineExceeded) {
			result.TimedOut = true
			result.Verdict = "timed_out"
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func (r *DockerRunner) Cleanup(ctx context.Context, handle Handle) error {
	return ctx.Err()
}

func (r *DockerRunner) dockerArgs(handle Handle, cmd Command) ([]string, string, error) {
	if strings.TrimSpace(handle.ExecContainer) != "" {
		return r.dockerExecArgs(handle, cmd)
	}
	return r.dockerRunArgs(handle, cmd)
}

func (r *DockerRunner) dockerRunArgs(handle Handle, cmd Command) ([]string, string, error) {
	profile := handle.Profile
	workdir, err := containerWorkdir(handle.MountTarget, cmd.Workdir)
	if err != nil {
		return nil, "", err
	}
	network := string(profile.NetworkMode)
	if network == "" {
		network = string(schema.SandboxNetworkNone)
	}
	args := []string{
		"run",
		"--rm",
	}
	if len(cmd.Stdin) > 0 {
		args = append(args, "-i")
	}
	args = append(args,
		"--network", network,
		"--mount", "type=bind,source="+handle.WorkspaceRoot+",target="+handle.MountTarget,
		"-w", workdir,
	)
	for _, mount := range handle.ExtraMounts {
		spec := "type=bind,source=" + mount.Source + ",target=" + mount.Target
		if mount.ReadOnly {
			spec += ",readonly"
		}
		args = append(args, "--mount", spec)
	}
	if strings.TrimSpace(profile.CPULimit) != "" {
		args = append(args, "--cpus", strings.TrimSpace(profile.CPULimit))
	}
	if strings.TrimSpace(profile.MemoryLimit) != "" {
		args = append(args, "--memory", strings.TrimSpace(profile.MemoryLimit))
	}
	for _, key := range allowedEnvKeys(profile, cmd.Env) {
		args = append(args, "-e", key+"="+cmd.Env[key])
	}
	args = append(args, profile.Image)
	args = append(args, cmd.Args...)
	return args, strings.Join(cmd.Args, " "), nil
}

func (r *DockerRunner) dockerExecArgs(handle Handle, cmd Command) ([]string, string, error) {
	workdir, err := localWorkdir(handle.WorkspaceRoot, cmd.Workdir)
	if err != nil {
		return nil, "", err
	}
	args := []string{"exec"}
	if len(cmd.Stdin) > 0 {
		args = append(args, "-i")
	}
	args = append(args, "-w", workdir)
	for _, key := range allowedEnvKeys(handle.Profile, cmd.Env) {
		args = append(args, "-e", key+"="+cmd.Env[key])
	}
	args = append(args, handle.ExecContainer)
	args = append(args, cmd.Args...)
	return args, strings.Join(cmd.Args, " "), nil
}

func validateCommand(profile schema.SandboxProfile, cmd Command) error {
	if len(cmd.Args) == 0 || strings.TrimSpace(cmd.Args[0]) == "" {
		return sandboxError("command_missing", "sandbox command is required")
	}
	for _, token := range cmd.Args {
		if _, ok := shellOperatorTokens[token]; ok {
			return sandboxError("shell_operator_blocked", "shell operators are not allowed in sandbox commands")
		}
	}
	executable := filepath.Base(strings.TrimSpace(cmd.Args[0]))
	if runtime.GOOS == "windows" {
		executable = strings.TrimSuffix(strings.ToLower(executable), ".exe")
	} else {
		executable = strings.ToLower(executable)
	}
	allowlist := commandAllowlist(profile.CommandAllowlist)
	if _, ok := allowlist[executable]; !ok {
		return sandboxError("command_not_allowlisted", fmt.Sprintf("sandbox command %q is not allowlisted", executable))
	}
	envAllowlist := envAllowlist(profile.EnvAllowlist)
	for key := range cmd.Env {
		if _, ok := envAllowlist[key]; !ok {
			return sandboxError("env_not_allowlisted", fmt.Sprintf("sandbox env var %q is not allowlisted", key))
		}
	}
	return nil
}

func commandAllowlist(values []string) map[string]struct{} {
	if len(values) == 0 {
		return defaultCommandAllowlist
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func envAllowlist(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func allowedEnvKeys(profile schema.SandboxProfile, env map[string]string) []string {
	allowed := envAllowlist(profile.EnvAllowlist)
	keys := make([]string, 0, len(env))
	for key := range env {
		if _, ok := allowed[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func containerWorkdir(mountTarget, workdir string) (string, error) {
	mountTarget = strings.TrimRight(strings.TrimSpace(mountTarget), "/")
	if mountTarget == "" {
		mountTarget = "/workspace"
	}
	workdir = strings.TrimSpace(workdir)
	if workdir == "" || workdir == "." {
		return mountTarget, nil
	}
	clean := filepath.ToSlash(filepath.Clean(workdir))
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", sandboxError("workdir_escape_blocked", "sandbox workdir must stay inside the mounted workspace")
	}
	return mountTarget + "/" + clean, nil
}

func localWorkdir(root, workdir string) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return "", sandboxError("workspace_unavailable", "workspace root is required")
	}
	workdir = strings.TrimSpace(workdir)
	if workdir == "" || workdir == "." {
		return root, nil
	}
	clean := filepath.Clean(workdir)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", sandboxError("workdir_escape_blocked", "sandbox workdir must stay inside the mounted workspace")
	}
	return filepath.Join(root, clean), nil
}

func commandTimeout(profile schema.SandboxProfile, requestedMS int) time.Duration {
	timeoutMS := requestedMS
	if timeoutMS <= 0 {
		timeoutMS = profile.DefaultTimeoutMS
	}
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	maxMS := profile.MaxTimeoutMS
	if maxMS <= 0 {
		maxMS = 600000
	}
	if timeoutMS > maxMS {
		timeoutMS = maxMS
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func verdictFor(exitCode int, timedOut bool) string {
	if timedOut {
		return "timed_out"
	}
	if exitCode == 0 {
		return "passed"
	}
	return "failed"
}

func (r *DockerRunner) dockerBinary() string {
	if strings.TrimSpace(r.DockerBinary) == "" {
		return "docker"
	}
	return strings.TrimSpace(r.DockerBinary)
}

func (r *DockerRunner) lookup(file string) (string, error) {
	if r.lookPath != nil {
		return r.lookPath(file)
	}
	return exec.LookPath(file)
}

func (r *DockerRunner) run(ctx context.Context, executable string, args []string, stdin []byte, timeout time.Duration) (commandResult, error) {
	if r.runCommand != nil {
		return r.runCommand(ctx, executable, args, stdin, timeout)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executable, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil
		} else if runCtx.Err() != nil {
			return commandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: -1, timedOut: true}, runCtx.Err()
		} else {
			return commandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: -1}, err
		}
	}
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}, err
}

func sandboxError(code, message string) *Error {
	return &Error{Code: code, Message: "sandbox: " + message}
}

func normalizeExtraMounts(mounts []Mount) ([]Mount, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	out := make([]Mount, 0, len(mounts))
	for _, mount := range mounts {
		source := strings.TrimSpace(mount.Source)
		target := strings.TrimSpace(mount.Target)
		if source == "" || target == "" {
			return nil, sandboxError("mount_invalid", "extra mount source and target are required")
		}
		if !strings.HasPrefix(target, "/") {
			return nil, sandboxError("mount_invalid", "extra mount target must be an absolute container path")
		}
		resolved, err := filepath.Abs(source)
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve extra mount source: %w", err)
		}
		if _, err := os.Stat(resolved); err != nil {
			return nil, sandboxError("mount_unavailable", fmt.Sprintf("extra mount source unavailable: %v", err))
		}
		out = append(out, Mount{
			Source:   resolved,
			Target:   filepath.ToSlash(filepath.Clean(target)),
			ReadOnly: mount.ReadOnly,
		})
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dockerExecContainer() string {
	return firstNonEmpty(os.Getenv("NAVI_SANDBOX_CONTAINER_NAME"))
}
