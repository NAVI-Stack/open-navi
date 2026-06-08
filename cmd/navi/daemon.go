package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const daemonDefaultTimeout = 30 * time.Second
const daemonDistributionChannelEnv = "NAVI_DISTRIBUTION_CHANNEL"

type daemonPaths struct {
	Dir       string
	PIDPath   string
	StatePath string
	LogPath   string
}

type daemonState struct {
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	Command    []string  `json:"command"`
	WorkDir    string    `json:"work_dir"`
	GatewayURL string    `json:"gateway_url"`
	LogPath    string    `json:"log_path"`
}

func runDaemonCommand(ctx context.Context, args []string) int {
	if !requirePackagedDaemonControl(os.Stderr, os.Environ()) {
		return 1
	}
	if len(args) == 0 {
		printDaemonUsage(os.Stderr)
		return 1
	}
	verb := args[0]
	subArgs := args[1:]
	switch verb {
	case "start":
		return runDaemonStart(ctx, subArgs)
	case "stop":
		return runDaemonStop(ctx, subArgs)
	case "restart":
		return runDaemonRestart(ctx, subArgs)
	case "status":
		return runDaemonStatus(ctx, subArgs)
	case "logs":
		return runDaemonLogs(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown daemon command %q.\n", verb)
		printDaemonUsage(os.Stderr)
		return 1
	}
}

func requirePackagedDaemonControl(w io.Writer, env []string) bool {
	if daemonControlAllowed(env) {
		return true
	}
	fmt.Fprintln(w, "NAVI local daemon lifecycle is available only through the packaged Node or Python CLI.")
	fmt.Fprintln(w, "Install and run one of:")
	fmt.Fprintln(w, "  npm install open-navi")
	fmt.Fprintln(w, "  pip install open-navi")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Developer builds may set NAVI_DISTRIBUTION_CHANNEL=dev explicitly.")
	return false
}

func daemonControlAllowed(env []string) bool {
	switch daemonDistributionChannel(env) {
	case "npm", "node", "pip", "python", "dev":
		return true
	default:
		return false
	}
}

func daemonDistributionChannel(env []string) string {
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, daemonDistributionChannelEnv) {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func printDaemonUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: navi daemon start|stop|restart|status|logs [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Local daemon mode runs navid on this host through the npm or pip CLI. Docker mode remains separate:")
	fmt.Fprintln(w, "  docker compose -f compose.yml up -d --build")
}

func runDaemonStart(ctx context.Context, args []string) int {
	var cfg Config
	var explicitBin, cwd, logPath, gatewayURL, listenAddr string
	var foreground bool
	timeout := daemonDefaultTimeout
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&gatewayURL, "url", "", "Gateway URL to wait for")
	fs.StringVar(&listenAddr, "addr", "", "Gateway listen address for the local daemon, e.g. :6284 or 127.0.0.1:6285")
	fs.StringVar(&explicitBin, "bin", "", "Path to navid binary")
	fs.StringVar(&cwd, "cwd", "", "Working directory for navid; defaults to repo root/current directory")
	fs.StringVar(&logPath, "log", "", "Log file path; defaults to ~/.navi/daemon/navid.log")
	fs.DurationVar(&timeout, "timeout", timeout, "How long to wait for /health")
	fs.BoolVar(&foreground, "foreground", false, "Run navid in the foreground")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg.GatewayURL = gatewayURL
	resolveConfig(&cfg, false)
	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(listenAddr) != "" {
		cfg.GatewayURL = gatewayURLFromListenAddr(listenAddr)
	}
	paths, err := getDaemonPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}
	if logPath == "" {
		logPath = paths.LogPath
	}
	cwd, err = resolveDaemonCWD(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}

	if err := probeDaemonHealth(ctx, cfg.GatewayURL); err == nil {
		if state, stateErr := readDaemonState(paths); stateErr == nil && state.PID > 0 && processExists(state.PID) {
			fmt.Printf("Local NaviD daemon is already running (pid %d) at %s\n", state.PID, cfg.GatewayURL)
		} else {
			fmt.Printf("NaviD gateway is already reachable at %s; not starting another daemon.\n", cfg.GatewayURL)
			fmt.Println("This may be Docker mode or an unmanaged foreground navid process.")
		}
		return 0
	}
	if state, stateErr := readDaemonState(paths); stateErr == nil && state.PID > 0 && processExists(state.PID) {
		fmt.Fprintf(os.Stderr, "daemon start: managed navid process %d is already running but %s/health is not healthy\n", state.PID, strings.TrimRight(cfg.GatewayURL, "/"))
		fmt.Fprintf(os.Stderr, "Check logs: %s\n", state.LogPath)
		return 1
	}

	bin, cmdArgs, err := resolveDaemonCommand(explicitBin, cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}

	if foreground {
		cmd := exec.Command(bin, cmdArgs...)
		cmd.Dir = cwd
		cmd.Env = daemonCommandEnv(os.Environ(), listenAddr)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "daemon foreground: %v\n", err)
			return 1
		}
		return 0
	}

	if err := os.MkdirAll(paths.Dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: create %s: %v\n", paths.Dir, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: create log dir: %v\n", err)
		return 1
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: open log %s: %v\n", logPath, err)
		return 1
	}

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Dir = cwd
	cmd.Env = daemonCommandEnv(os.Environ(), listenAddr)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureDaemonProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}
	_ = logFile.Close()

	state := daemonState{
		PID:        cmd.Process.Pid,
		StartedAt:  time.Now().UTC(),
		Command:    append([]string{bin}, cmdArgs...),
		WorkDir:    cwd,
		GatewayURL: cfg.GatewayURL,
		LogPath:    logPath,
	}
	if err := writeDaemonState(paths, state); err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: write state: %v\n", err)
		return 1
	}

	if timeout > 0 {
		if err := waitForDaemonHealth(ctx, cfg.GatewayURL, timeout); err != nil {
			fmt.Fprintf(os.Stderr, "NaviD started as pid %d, but health did not become ready: %v\n", state.PID, err)
			fmt.Fprintf(os.Stderr, "Log: %s\n", logPath)
			return 1
		}
	}
	fmt.Printf("NaviD local daemon started (pid %d)\n", state.PID)
	fmt.Printf("Gateway: %s\n", cfg.GatewayURL)
	fmt.Printf("Log:     %s\n", logPath)
	return 0
}

func daemonCommandEnv(base []string, listenAddr string) []string {
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		return base
	}
	const key = "NAVI_GATEWAY_ADDR"
	next := make([]string, 0, len(base)+1)
	replaced := false
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			next = append(next, key+"="+listenAddr)
			replaced = true
			continue
		}
		next = append(next, entry)
	}
	if !replaced {
		next = append(next, key+"="+listenAddr)
	}
	return next
}

func gatewayURLFromListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "http://localhost:6284"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + addr
}

func runDaemonStop(ctx context.Context, args []string) int {
	timeout := 15 * time.Second
	fs := flag.NewFlagSet("daemon stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.DurationVar(&timeout, "timeout", timeout, "How long to wait before force-killing")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	paths, err := getDaemonPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon stop: %v\n", err)
		return 1
	}
	state, err := readDaemonState(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon stop: no managed local daemon state found")
		fmt.Fprintln(os.Stderr, "If NaviD is running through Docker, stop it with: docker compose -f compose.yml down")
		return 1
	}
	if state.PID <= 0 || !processExists(state.PID) {
		_ = removeDaemonState(paths)
		fmt.Println("No running managed local daemon found; stale state removed.")
		return 0
	}

	if err := terminateProcess(state.PID); err != nil {
		fmt.Fprintf(os.Stderr, "daemon stop: graceful stop failed for pid %d: %v\n", state.PID, err)
	}
	if err := waitForProcessExit(ctx, state.PID, timeout); err != nil {
		if proc, findErr := os.FindProcess(state.PID); findErr == nil {
			_ = proc.Kill()
		}
		if waitErr := waitForProcessExit(ctx, state.PID, 5*time.Second); waitErr != nil {
			fmt.Fprintf(os.Stderr, "daemon stop: pid %d did not exit: %v\n", state.PID, waitErr)
			return 1
		}
	}
	_ = removeDaemonState(paths)
	fmt.Printf("Stopped local NaviD daemon (pid %d)\n", state.PID)
	return 0
}

func runDaemonRestart(ctx context.Context, args []string) int {
	paths, err := getDaemonPaths()
	if err == nil {
		if state, stateErr := readDaemonState(paths); stateErr == nil && state.PID > 0 && processExists(state.PID) {
			if code := runDaemonStop(ctx, nil); code != 0 {
				return code
			}
		}
	}
	return runDaemonStart(ctx, args)
}

func runDaemonStatus(ctx context.Context, args []string) int {
	var cfg Config
	var jsonOut bool
	fs := flag.NewFlagSet("daemon status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.GatewayURL, "url", "", "Gateway URL to probe")
	fs.BoolVar(&jsonOut, "json", false, "Output status as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	resolveConfig(&cfg, false)

	paths, err := getDaemonPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon status: %v\n", err)
		return 1
	}
	state, stateErr := readDaemonState(paths)
	managed := stateErr == nil && state.PID > 0
	processRunning := managed && processExists(state.PID)
	healthErr := probeDaemonHealth(ctx, cfg.GatewayURL)
	gatewayReachable := healthErr == nil

	if jsonOut {
		out := map[string]any{
			"managed":           managed,
			"process_running":   processRunning,
			"gateway_reachable": gatewayReachable,
			"gateway_url":       cfg.GatewayURL,
			"state_path":        paths.StatePath,
			"pid_path":          paths.PIDPath,
			"log_path":          paths.LogPath,
		}
		if state != nil {
			out["pid"] = state.PID
			out["started_at"] = state.StartedAt
			out["command"] = state.Command
			out["work_dir"] = state.WorkDir
			out["log_path"] = state.LogPath
		}
		if healthErr != nil {
			out["health_error"] = healthErr.Error()
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		if gatewayReachable {
			return 0
		}
		return 1
	}

	if managed {
		stateLine := "stopped"
		if processRunning {
			stateLine = "running"
		}
		fmt.Printf("Managed local daemon: %s", stateLine)
		if state.PID > 0 {
			fmt.Printf(" (pid %d)", state.PID)
		}
		fmt.Println()
		fmt.Printf("Work dir: %s\n", state.WorkDir)
		fmt.Printf("Log:      %s\n", state.LogPath)
	} else {
		fmt.Println("Managed local daemon: not found")
	}
	if gatewayReachable {
		fmt.Printf("Gateway: reachable at %s\n", cfg.GatewayURL)
		if !managed {
			fmt.Println("Runtime mode: Docker, foreground navid, or another unmanaged process")
		} else {
			fmt.Println("Runtime mode: local daemon")
		}
		return 0
	}
	fmt.Printf("Gateway: not reachable at %s", cfg.GatewayURL)
	if healthErr != nil {
		fmt.Printf(" (%v)", healthErr)
	}
	fmt.Println()
	return 1
}

func runDaemonLogs(args []string) int {
	var logPath string
	lineCount := 80
	fs := flag.NewFlagSet("daemon logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&logPath, "log", "", "Log file path; defaults to ~/.navi/daemon/navid.log")
	fs.IntVar(&lineCount, "n", lineCount, "Number of lines to print")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	paths, err := getDaemonPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon logs: %v\n", err)
		return 1
	}
	if logPath == "" {
		if state, stateErr := readDaemonState(paths); stateErr == nil && strings.TrimSpace(state.LogPath) != "" {
			logPath = state.LogPath
		} else {
			logPath = paths.LogPath
		}
	}
	lines, err := tailLines(logPath, lineCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon logs: %v\n", err)
		return 1
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return 0
}

func getDaemonPaths() (daemonPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return daemonPaths{}, err
	}
	dir := filepath.Join(home, ".navi", "daemon")
	return daemonPaths{
		Dir:       dir,
		PIDPath:   filepath.Join(dir, "navid.pid"),
		StatePath: filepath.Join(dir, "navid.json"),
		LogPath:   filepath.Join(dir, "navid.log"),
	}, nil
}

func resolveDaemonCWD(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if looksLikeDaemonCWD(cwd) {
		return cwd, nil
	}
	if exe, err := os.Executable(); err == nil {
		parent := filepath.Dir(filepath.Dir(exe))
		if looksLikeDaemonCWD(parent) {
			return parent, nil
		}
	}
	return cwd, nil
}

func looksLikeDaemonCWD(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "runtime.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "cmd", "navid", "main.go")); err == nil {
		return true
	}
	return false
}

func resolveDaemonCommand(explicitBin, cwd string) (string, []string, error) {
	if strings.TrimSpace(explicitBin) != "" {
		bin, err := filepath.Abs(explicitBin)
		if err != nil {
			return "", nil, err
		}
		if !isExecutableFile(bin) {
			return "", nil, fmt.Errorf("%s is not an executable file", bin)
		}
		return bin, nil, nil
	}
	for _, candidate := range daemonBinaryCandidates(cwd) {
		if isExecutableFile(candidate) {
			return candidate, nil, nil
		}
	}
	if path, err := exec.LookPath("navid"); err == nil {
		return path, nil, nil
	}
	return "", nil, errors.New("navid binary not found; run `make build` or pass `navi daemon start -bin <path-to-navid>`")
}

func daemonBinaryCandidates(cwd string) []string {
	names := []string{"navid"}
	if runtime.GOOS == "windows" {
		names = []string{"navid.exe", "navid"}
	}
	var dirs []string
	if cwd != "" {
		dirs = append(dirs, filepath.Join(cwd, "bin"))
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	seen := map[string]struct{}{}
	var out []string
	for _, dir := range dirs {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func readDaemonState(paths daemonPaths) (*daemonState, error) {
	data, err := os.ReadFile(paths.StatePath)
	if err != nil {
		return nil, err
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.LogPath) == "" {
		state.LogPath = paths.LogPath
	}
	return &state, nil
}

func writeDaemonState(paths daemonPaths, state daemonState) error {
	if err := os.MkdirAll(paths.Dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(paths.StatePath, data, 0600); err != nil {
		return err
	}
	return os.WriteFile(paths.PIDPath, []byte(strconv.Itoa(state.PID)+"\n"), 0600)
}

func removeDaemonState(paths daemonPaths) error {
	errState := os.Remove(paths.StatePath)
	errPID := os.Remove(paths.PIDPath)
	if errState != nil && !errors.Is(errState, os.ErrNotExist) {
		return errState
	}
	if errPID != nil && !errors.Is(errPID, os.ErrNotExist) {
		return errPID
	}
	return nil
}

func daemonHealthURL(gatewayURL string) string {
	base := strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	if base == "" {
		base = "http://localhost:6284"
	}
	return base + "/health"
}

func probeDaemonHealth(ctx context.Context, gatewayURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, daemonHealthURL(gatewayURL), nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func waitForDaemonHealth(ctx context.Context, gatewayURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeDaemonHealth(ctx, gatewayURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("timed out after %s", timeout)
}

func waitForProcessExit(ctx context.Context, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out after %s", timeout)
}

func tailLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, n)
	for scanner.Scan() {
		if len(lines) == n {
			copy(lines, lines[1:])
			lines[n-1] = scanner.Text()
			continue
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
