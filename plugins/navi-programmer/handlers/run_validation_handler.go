package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ceoai/navi/internal/coderalias"
	coreskill "github.com/ceoai/navi/internal/navi/skill"
	coresandbox "github.com/ceoai/navi/internal/sandbox"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const (
	RunValidationSkillID      = "navi-programmer.run-validation"
	defaultValidationProfile  = "local-validation-network-disabled"
	defaultValidationTimeout  = 30000
	maxValidationTimeout      = 600000
	defaultMaxValidationBytes = 131072
	maxValidationBytes        = 5242880
)

var (
	validationExecutables = stringSet(
		"bun", "cargo", "dotnet", "eslint", "go", "gradle", "javac", "jest",
		"make", "mvn", "node", "npm", "pnpm", "poetry", "py", "pytest",
		"python", "python3", "ruff", "tsc", "uv", "vitest", "yarn",
	)
	bannedValidationExecutables = stringSet("bash", "cmd", "fish", "git", "npx", "powershell", "pwsh", "sh", "ssh", "zsh")
	packageManagers             = stringSet("bun", "npm", "pnpm", "poetry", "yarn")
	networkishPackageActions    = stringSet("add", "audit", "ci", "dlx", "get", "global", "install", "link", "publish", "remove", "uninstall", "update", "upgrade")
	validationTargetNames       = stringSet("build", "check", "ci", "compile", "e2e", "format", "fmt", "lint", "mypy", "pytest", "ruff", "smoke", "static", "test", "tsc", "typecheck", "unit", "validate", "vet")
	allowedPythonModules        = stringSet("compileall", "mypy", "py_compile", "pytest", "ruff", "unittest")
	allowedGoActions            = stringSet("build", "test", "vet")
	allowedCargoActions         = stringSet("build", "check", "clippy", "fmt", "test")
	shellOperatorTokens         = stringSet("|", "||", "&", "&&", ";", "<", ">", ">>", "2>", "2>>")

	secretNameRE = regexp.MustCompile(`(?i)(secret|token|password|passwd|credential|apikey|api_key|private|key)`)
	envNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

var RegisterInternalHandler = coreskill.RegisterInternalHandler

// ProgrammerHandlerConfig configures NAVI Programmer internal skill handlers.
type ProgrammerHandlerConfig struct {
	DB             *sql.DB
	WorkspaceDir   string
	Runner         coresandbox.Runner
	DefaultProfile schema.SandboxProfile
}

// RegisterProgrammerHandlers registers internal handlers for NAVI Programmer
// skills that need direct access to NAVI core services.
func RegisterProgrammerHandlers(skillID string, cfg ProgrammerHandlerConfig) {
	if strings.TrimSpace(skillID) == "" {
		skillID = RunValidationSkillID
	}
	handler := &runValidationHandler{cfg: cfg}
	if handler.cfg.Runner == nil {
		handler.cfg.Runner = coresandbox.NewDockerRunner()
	}
	RegisterInternalHandler(skillID, "run_command", handler.runCommand)
	RegisterInternalHandler(skillID, "record_not_run", handler.recordNotRun)
	// Also register under the Coder-facing skill alias (NEW-A) so a direct
	// GetInternalHandler("navi-coder.run-validation", ...) resolves to the same
	// implementation. The alias id comes from the single source of truth so it
	// never drifts. Legacy registration above is unchanged.
	if alias := coderalias.CoderSkillID(skillID); alias != skillID {
		RegisterInternalHandler(alias, "run_command", handler.runCommand)
		RegisterInternalHandler(alias, "record_not_run", handler.recordNotRun)
	}
}

type runValidationHandler struct {
	cfg ProgrammerHandlerConfig
}

func (h *runValidationHandler) runCommand(ctx context.Context, _ *coreskill.SkillEntry, _ *coreskill.Interface, args map[string]any) (any, error) {
	startedAt := time.Now().UTC()
	root, err := h.resolveRoot(args)
	if err != nil {
		return nil, err
	}
	workdirAbs, workdirRel, err := resolveValidationWorkdir(root, anyOrDefault(args["cwd"], "."))
	if err != nil {
		return nil, err
	}
	_ = workdirAbs
	command, err := parseValidationCommand(args["command"])
	if err != nil {
		return nil, err
	}
	normalizedExecutable, err := validateValidationCommandPolicy(command)
	if err != nil {
		return nil, err
	}
	timeoutMS, err := clampValidationInt(args["timeout_ms"], defaultValidationTimeout, 1000, maxValidationTimeout)
	if err != nil {
		return nil, err
	}
	maxOutputBytes, err := clampValidationInt(args["max_output_bytes"], defaultMaxValidationBytes, 1024, maxValidationBytes)
	if err != nil {
		return nil, err
	}
	expectedExitCodes, err := parseExpectedExitCodes(args["expected_exit_codes"])
	if err != nil {
		return nil, err
	}
	profileID := validationProfileID(args)
	profile, err := h.resolveSandboxProfile(ctx, profileID)
	if err != nil && !boolArg(args["dry_run"]) {
		return sandboxBlockedValidationResult(baseValidationResult(root, workdirRel, command, normalizedExecutable, args, profileID, nil), err, startedAt, maxOutputBytes), nil
	}
	env, envPassthrough, err := buildValidationEnv(args, profile.EnvAllowlist)
	if err != nil {
		return nil, err
	}
	base := baseValidationResult(root, workdirRel, command, normalizedExecutable, args, profileID, envPassthrough)
	if profile.ID != "" {
		base["sandbox_profile"] = profile.ID
		base["sandbox_profile_id"] = profile.ID
		base["sandbox_runtime"] = string(profile.Runtime)
		base["network_mode"] = string(profile.NetworkMode)
	}
	if boolArg(args["dry_run"]) {
		return dryRunValidationResult(base), nil
	}

	prepareStarted := time.Now()
	handle, err := h.runner().Prepare(ctx, coresandbox.PrepareRequest{
		Profile:       profile,
		WorkspaceRoot: root,
		RunID:         stringArg(args["run_id"]),
	})
	if err != nil {
		return sandboxBlockedValidationResult(base, err, startedAt, maxOutputBytes), nil
	}
	defer func() {
		_ = h.runner().Cleanup(ctx, handle)
	}()

	result, err := h.runner().Run(ctx, handle, coresandbox.Command{
		Args:      command,
		Workdir:   workdirRel,
		Env:       env,
		TimeoutMS: timeoutMS,
	})
	if err != nil {
		return sandboxBlockedValidationResult(base, err, startedAt, maxOutputBytes), nil
	}
	if result.DurationMS <= 0 {
		result.DurationMS = int(time.Since(prepareStarted).Milliseconds())
	}
	return sandboxValidationResult(base, result, expectedExitCodes, maxOutputBytes, startedAt), nil
}

func (h *runValidationHandler) recordNotRun(_ context.Context, _ *coreskill.SkillEntry, _ *coreskill.Interface, args map[string]any) (any, error) {
	reason := strings.TrimSpace(stringArg(args["reason"]))
	if reason == "" {
		return nil, fmt.Errorf("reason is required")
	}
	root, err := h.resolveRoot(args)
	if err != nil {
		return nil, err
	}
	_, workdirRel, err := resolveValidationWorkdir(root, anyOrDefault(args["cwd"], "."))
	if err != nil {
		return nil, err
	}
	recommended := []string{}
	if raw, ok := args["recommended_command"]; ok && raw != nil {
		recommended, err = parseValidationCommand(raw)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"root":                root,
		"cwd":                 workdirRel,
		"verdict":             "not_run",
		"validation_kind":     stringWithDefault(args["validation_kind"], "other"),
		"reason":              reason,
		"blocked_by":          stringArg(args["blocked_by"]),
		"recommended_command": recommended,
		"duration_ms":         0,
		"exit_code":           nil,
		"stdout":              "",
		"stderr":              "",
		"stdout_truncated":    false,
		"stderr_truncated":    false,
		"timed_out":           false,
	}, nil
}

func (h *runValidationHandler) runner() coresandbox.Runner {
	if h.cfg.Runner != nil {
		return h.cfg.Runner
	}
	return coresandbox.NewDockerRunner()
}

func (h *runValidationHandler) resolveRoot(args map[string]any) (string, error) {
	raw := strings.TrimSpace(firstNonEmpty(stringArg(args["root"]), stringArg(args["repo_root"]), os.Getenv("NAVI_WORKSPACE_DIR"), h.cfg.WorkspaceDir))
	if raw == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve current working directory: %w", err)
		}
		raw = cwd
	}
	root, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("root does not exist: %s", root)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory: %s", root)
	}
	return root, nil
}

func (h *runValidationHandler) resolveSandboxProfile(ctx context.Context, profileID string) (schema.SandboxProfile, error) {
	if h.cfg.DB != nil {
		return store.GetSandboxProfile(ctx, h.cfg.DB, profileID)
	}
	if strings.TrimSpace(h.cfg.DefaultProfile.ID) != "" {
		if profileID != "" && profileID != h.cfg.DefaultProfile.ID {
			return schema.SandboxProfile{}, fmt.Errorf("sandbox profile not found: %s", profileID)
		}
		return h.cfg.DefaultProfile, nil
	}
	return schema.SandboxProfile{}, fmt.Errorf("sandbox profile not found: %s", profileID)
}

func resolveValidationWorkdir(root string, pathValue any) (string, string, error) {
	raw := strings.TrimSpace(fmt.Sprint(pathValue))
	if raw == "" {
		raw = "."
	}
	candidate := filepath.Clean(raw)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	resolved, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve cwd: %w", err)
	}
	if !pathWithinRoot(resolved, root) {
		return "", "", fmt.Errorf("cwd escapes root: %s", raw)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("cwd does not exist: %s", raw)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("cwd is not a directory: %s", raw)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == "." || rel == "" {
		return resolved, ".", nil
	}
	return resolved, filepath.ToSlash(rel), nil
}

func parseValidationCommand(value any) ([]string, error) {
	var command []string
	switch v := value.(type) {
	case []string:
		command = append(command, v...)
	case []any:
		for _, part := range v {
			text := fmt.Sprint(part)
			if text != "" {
				command = append(command, text)
			}
		}
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("command is required")
		}
		parts, err := splitValidationCommand(v)
		if err != nil {
			return nil, err
		}
		command = parts
	default:
		return nil, fmt.Errorf("command must be an argv array or string")
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	for _, token := range command {
		if shellOperatorTokens[token] {
			return nil, fmt.Errorf("shell operator is not allowed: %s", token)
		}
	}
	return command, nil
}

func splitValidationCommand(value string) ([]string, error) {
	var (
		out     []string
		current strings.Builder
		quote   rune
		escaped bool
	)
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	flush()
	return out, nil
}

func validateValidationCommandPolicy(command []string) (string, error) {
	exe := normalizeValidationExecutable(command[0])
	if strings.ContainsAny(command[0], `/\`) {
		return "", fmt.Errorf("path-style executables are not allowed; use a named validation tool")
	}
	if bannedValidationExecutables[exe] {
		return "", fmt.Errorf("executable is not allowed: %s", exe)
	}
	if !validationExecutables[exe] {
		return "", fmt.Errorf("executable is outside validation allowlist: %s", exe)
	}
	if packageManagers[exe] {
		if err := validatePackageManagerCommand(exe, command); err != nil {
			return "", err
		}
	}
	switch exe {
	case "make":
		if err := validateMakeCommand(command); err != nil {
			return "", err
		}
	case "python", "python3", "py":
		if err := validatePythonCommand(command); err != nil {
			return "", err
		}
	case "node":
		if err := validateNodeCommand(command); err != nil {
			return "", err
		}
	case "go":
		if err := validateGoCommand(command); err != nil {
			return "", err
		}
	case "cargo":
		if err := validateCargoCommand(command); err != nil {
			return "", err
		}
	case "uv":
		if err := validateUVCommand(command); err != nil {
			return "", err
		}
	}
	return exe, nil
}

func validatePackageManagerCommand(exe string, command []string) error {
	action := firstNonOption(command[1:])
	if action == "" {
		return fmt.Errorf("%s requires an explicit validation action", exe)
	}
	action = strings.ToLower(action)
	if networkishPackageActions[action] {
		return fmt.Errorf("%s %s is not a validation command", exe, action)
	}
	if action == "run" {
		script := firstNonOption(command[2:])
		if script == "" {
			return fmt.Errorf("%s run requires a script name", exe)
		}
		if !isValidationTarget(script) {
			return fmt.Errorf("%s run %s is not an allowed validation target", exe, script)
		}
		return nil
	}
	if !validationTargetNames[action] {
		return fmt.Errorf("%s %s is not an allowed validation target", exe, action)
	}
	return nil
}

func validateMakeCommand(command []string) error {
	targets := make([]string, 0, len(command)-1)
	for _, part := range command[1:] {
		if !strings.HasPrefix(part, "-") {
			targets = append(targets, part)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("make requires an explicit validation target")
	}
	for _, target := range targets {
		if strings.Contains(target, "=") {
			continue
		}
		if !isValidationTarget(target) {
			return fmt.Errorf("make target is not an allowed validation target: %s", target)
		}
	}
	return nil
}

func validatePythonCommand(command []string) error {
	lowered := lowerSlice(command[1:])
	for _, part := range lowered {
		if part == "-c" {
			return fmt.Errorf("python -c is not allowed by the validation runner")
		}
	}
	for idx, part := range lowered {
		if part != "-m" {
			continue
		}
		module := ""
		if idx+1 < len(lowered) {
			module = lowered[idx+1]
		}
		if module == "pip" || module == "venv" || module == "ensurepip" {
			return fmt.Errorf("python -m %s is not a validation command", module)
		}
		if !allowedPythonModules[module] {
			return fmt.Errorf("python -m %s is not an allowed validation module", module)
		}
		return nil
	}
	return fmt.Errorf("python requires -m with an allowed validation module")
}

func validateNodeCommand(command []string) error {
	lowered := lowerSlice(command[1:])
	for _, part := range lowered {
		if part == "-e" || part == "--eval" || part == "-p" || part == "--print" {
			return fmt.Errorf("node inline evaluation is not allowed by the validation runner")
		}
	}
	for _, part := range lowered {
		if part == "--test" {
			return nil
		}
	}
	return fmt.Errorf("node is limited to the built-in --test validation path")
}

func validateGoCommand(command []string) error {
	action := strings.ToLower(firstNonOption(command[1:]))
	if action == "" || !allowedGoActions[action] {
		return fmt.Errorf("go %s is not an allowed validation command", missingAction(action))
	}
	return nil
}

func validateCargoCommand(command []string) error {
	action := strings.ToLower(firstNonOption(command[1:]))
	if action == "" || !allowedCargoActions[action] {
		return fmt.Errorf("cargo %s is not an allowed validation command", missingAction(action))
	}
	return nil
}

func validateUVCommand(command []string) error {
	action := strings.ToLower(firstNonOption(command[1:]))
	if action == "" {
		return nil
	}
	if action == "add" || action == "build" || action == "lock" || action == "pip" || action == "publish" || action == "remove" || action == "sync" {
		return fmt.Errorf("uv %s is not allowed by the validation runner", action)
	}
	if action == "run" {
		target := firstNonOption(command[2:])
		if target == "" {
			return fmt.Errorf("uv run requires a validation-shaped target")
		}
		for _, part := range command[2:] {
			if isValidationTarget(part) {
				return nil
			}
		}
		return fmt.Errorf("uv run requires a validation-shaped target")
	}
	return nil
}

func buildValidationEnv(args map[string]any, profileEnvAllowlist []string) (map[string]string, []string, error) {
	env := map[string]string{}
	allowlist, err := parseEnvAllowlist(args["env_allowlist"])
	if err != nil {
		return nil, nil, err
	}
	for _, name := range allowlist {
		if isSecretName(name) {
			return nil, nil, fmt.Errorf("secret-like env name is not allowed: %s", name)
		}
		if value, ok := os.LookupEnv(name); ok {
			env[name] = value
		}
	}
	if explicit, ok := args["env"]; ok && explicit != nil {
		values := map[string]any{}
		switch typed := explicit.(type) {
		case map[string]any:
			values = typed
		case map[string]string:
			for name, value := range typed {
				values[name] = value
			}
		default:
			return nil, nil, fmt.Errorf("env must be an object")
		}
		for name, value := range values {
			if !envNameRE.MatchString(name) {
				return nil, nil, fmt.Errorf("invalid env name: %s", name)
			}
			if isSecretName(name) {
				return nil, nil, fmt.Errorf("secret-like env name is not allowed: %s", name)
			}
			env[name] = fmt.Sprint(value)
		}
	}
	profileAllowsEnv := stringSet(profileEnvAllowlist...)
	if profileAllowsEnv["NAVI_VALIDATION"] {
		env["NAVI_VALIDATION"] = "1"
	}
	if profileAllowsEnv["NO_COLOR"] {
		env["NO_COLOR"] = "1"
	}
	keys := make([]string, 0, len(allowlist))
	keys = append(keys, allowlist...)
	sort.Strings(keys)
	return env, keys, nil
}

func parseEnvAllowlist(value any) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	raw, ok := value.([]any)
	if !ok {
		if values, ok := value.([]string); ok {
			raw = make([]any, 0, len(values))
			for _, item := range values {
				raw = append(raw, item)
			}
		} else {
			return nil, fmt.Errorf("env_allowlist must be an array")
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name := fmt.Sprint(item)
		if !envNameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid env_allowlist name: %s", name)
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func baseValidationResult(root, cwd string, command []string, normalizedExecutable string, args map[string]any, profileID string, envPassthrough []string) map[string]any {
	if envPassthrough == nil {
		envPassthrough = []string{}
	}
	return map[string]any{
		"root":               root,
		"cwd":                cwd,
		"command":            command,
		"command_display":    strings.Join(command, " "),
		"validation_kind":    stringWithDefault(args["validation_kind"], "other"),
		"sandbox_profile":    profileID,
		"sandbox_profile_id": profileID,
		"policy": map[string]any{
			"shell":                 false,
			"normalized_executable": normalizedExecutable,
			"network_access":        "sandbox_profile",
			"env_passthrough":       envPassthrough,
			"execution_backend":     "navi_core_sandbox",
		},
	}
}

func dryRunValidationResult(base map[string]any) map[string]any {
	return mergeValidationResult(base, map[string]any{
		"verdict":          "not_run",
		"reason":           "dry_run",
		"would_run":        true,
		"exit_code":        nil,
		"duration_ms":      0,
		"stdout":           "",
		"stderr":           "",
		"stdout_truncated": false,
		"stderr_truncated": false,
		"timed_out":        false,
	})
}

func sandboxBlockedValidationResult(base map[string]any, err error, startedAt time.Time, maxOutputBytes int) map[string]any {
	code := "sandbox_blocked"
	var sandboxErr *coresandbox.Error
	if errors.As(err, &sandboxErr) && strings.TrimSpace(sandboxErr.Code) != "" {
		code = sandboxErr.Code
	}
	stderr, truncated := truncateValidationText(err.Error(), maxOutputBytes)
	return mergeValidationResult(base, map[string]any{
		"verdict":          "not_run",
		"reason":           "sandbox_blocked",
		"blocked_by":       code,
		"exit_code":        nil,
		"duration_ms":      int(time.Since(startedAt).Milliseconds()),
		"stdout":           "",
		"stderr":           stderr,
		"stdout_truncated": false,
		"stderr_truncated": truncated,
		"timed_out":        false,
		"started_at":       formatValidationTime(startedAt),
		"ended_at":         formatValidationTime(time.Now().UTC()),
	})
}

func sandboxValidationResult(base map[string]any, result coresandbox.Result, expectedExitCodes []int, maxOutputBytes int, startedAt time.Time) map[string]any {
	stdout, stdoutTruncated := truncateValidationText(result.Stdout, maxOutputBytes)
	stderr, stderrTruncated := truncateValidationText(result.Stderr, maxOutputBytes)
	verdict := classifyValidationVerdict(result.ExitCode, expectedExitCodes, result.TimedOut, stdoutTruncated, stderrTruncated)
	var exitCode any = result.ExitCode
	if result.TimedOut {
		exitCode = nil
	}
	return mergeValidationResult(base, map[string]any{
		"sandbox_profile":     result.SandboxProfileID,
		"sandbox_profile_id":  result.SandboxProfileID,
		"sandbox_runtime":     result.Runtime,
		"network_mode":        result.NetworkMode,
		"verdict":             verdict,
		"exit_code":           exitCode,
		"duration_ms":         result.DurationMS,
		"stdout":              stdout,
		"stderr":              stderr,
		"stdout_truncated":    stdoutTruncated,
		"stderr_truncated":    stderrTruncated,
		"timed_out":           result.TimedOut,
		"started_at":          formatValidationTime(startedAt),
		"ended_at":            formatValidationTime(time.Now().UTC()),
		"expected_exit_codes": expectedExitCodes,
	})
}

func mergeValidationResult(base map[string]any, values map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(values))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func parseExpectedExitCodes(value any) ([]int, error) {
	if value == nil {
		return []int{0}, nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected_exit_codes must be an array")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("expected_exit_codes must not be empty")
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		parsed, err := strconv.Atoi(fmt.Sprint(item))
		if err != nil {
			return nil, fmt.Errorf("expected_exit_codes must contain integers")
		}
		out = append(out, parsed)
	}
	return out, nil
}

func classifyValidationVerdict(exitCode int, expected []int, timedOut, stdoutTruncated, stderrTruncated bool) string {
	if timedOut {
		return "timed_out"
	}
	for _, code := range expected {
		if exitCode == code {
			if stdoutTruncated || stderrTruncated {
				return "ambiguous"
			}
			return "passed"
		}
	}
	return "failed"
}

func truncateValidationText(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxValidationBytes
	}
	data := []byte(value)
	if len(data) <= maxBytes {
		return value, false
	}
	return string(data[:maxBytes]), true
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func normalizeValidationExecutable(value string) string {
	name := strings.ToLower(filepath.Base(value))
	if runtime.GOOS == "windows" {
		for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1"} {
			name = strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func firstNonOption(values []string) string {
	for _, value := range values {
		if !strings.HasPrefix(value, "-") {
			return value
		}
	}
	return ""
}

func isValidationTarget(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "_", "-"))
	for _, part := range strings.FieldsFunc(normalized, func(r rune) bool { return r == '-' || r == ':' }) {
		if validationTargetNames[part] {
			return true
		}
	}
	return false
}

func isSecretName(name string) bool {
	return secretNameRE.MatchString(name)
}

func lowerSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.ToLower(value))
	}
	return out
}

func missingAction(action string) string {
	if action == "" {
		return "<missing>"
	}
	return action
}

func clampValidationInt(value any, def, min, max int) (int, error) {
	if value == nil {
		return def, nil
	}
	parsed, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil {
		return 0, fmt.Errorf("expected integer, got %v", value)
	}
	if parsed < min {
		return min, nil
	}
	if parsed > max {
		return max, nil
	}
	return parsed, nil
}

func validationProfileID(args map[string]any) string {
	return firstNonEmpty(stringArg(args["sandbox_profile"]), defaultValidationProfile)
}

func boolArg(value any) bool {
	parsed, ok := value.(bool)
	return ok && parsed
}

func stringArg(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringWithDefault(value any, def string) string {
	if got := stringArg(value); got != "" {
		return got
	}
	return def
}

func anyOrDefault(value any, def any) any {
	if value == nil {
		return def
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatValidationTime(value time.Time) string {
	return value.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func stringSet(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
