package harness

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type OutputChunk struct {
	Stream string
	Text   string
	At     time.Duration
}

type CommandResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	Duration   time.Duration
	Transcript []OutputChunk
}

type CLIRunner struct {
	stack   *Stack
	binary  string
	homeDir string
}

func NewCLIRunner(ctx context.Context, stack *Stack) (*CLIRunner, error) {
	binary, err := stack.BuildCLI(ctx)
	if err != nil {
		return nil, err
	}
	homeDir, err := os.MkdirTemp(stack.tempDir, "cli-home-*")
	if err != nil {
		return nil, err
	}
	return &CLIRunner{
		stack:   stack,
		binary:  binary,
		homeDir: homeDir,
	}, nil
}

func (r *CLIRunner) Ask(ctx context.Context, message string, extraArgs ...string) (*CommandResult, error) {
	args := []string{"ask", "-url", r.stack.gatewayURL, "-api-key", r.stack.apiKey, "-m", message}
	args = append(args, extraArgs...)
	return r.run(ctx, args...)
}

func (r *CLIRunner) AskWithAPIKey(ctx context.Context, apiKey, message string, extraArgs ...string) (*CommandResult, error) {
	args := []string{"ask", "-url", r.stack.gatewayURL, "-api-key", apiKey, "-m", message}
	args = append(args, extraArgs...)
	return r.run(ctx, args...)
}

func (r *CLIRunner) Run(ctx context.Context, args ...string) (*CommandResult, error) {
	return r.RunWithAPIKey(ctx, r.stack.apiKey, args...)
}

func (r *CLIRunner) StartChat(ctx context.Context, extraArgs ...string) (*ChatSession, error) {
	return r.StartChatWithAPIKey(ctx, r.stack.apiKey, extraArgs...)
}

func (r *CLIRunner) RunWithAPIKey(ctx context.Context, apiKey string, args ...string) (*CommandResult, error) {
	base := []string{"-url", r.stack.gatewayURL, "-api-key", apiKey}
	switch {
	case len(args) > 0 && args[0] == "models":
		args = append([]string{"models"}, append(base, args[1:]...)...)
	case len(args) > 0 && args[0] == "connectors":
		args = append([]string{"connectors"}, append(base, args[1:]...)...)
	case len(args) > 0 && isOperatorSubcommand(args[0]):
		args = append([]string{args[0]}, append(base, args[1:]...)...)
	}
	return r.run(ctx, args...)
}

func (r *CLIRunner) StartChatWithAPIKey(ctx context.Context, apiKey string, extraArgs ...string) (*ChatSession, error) {
	args := []string{"chat", "-url", r.stack.gatewayURL, "-api-key", apiKey, "-new"}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, r.binary, args...)
	cmd.Dir = r.stack.repoRoot
	cmd.Env = append(os.Environ(), "HOME="+r.homeDir)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	session := &ChatSession{
		cmd:       cmd,
		ptmx:      ptmx,
		startedAt: time.Now(),
		done:      make(chan *CommandResult, 1),
		closed:    make(chan struct{}),
	}
	go session.readLoop()
	go session.waitLoop()
	return session, nil
}

func (r *CLIRunner) run(ctx context.Context, args ...string) (*CommandResult, error) {
	cmd := exec.CommandContext(ctx, r.binary, args...)
	cmd.Dir = r.stack.repoRoot
	cmd.Env = append(os.Environ(), "HOME="+r.homeDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var transcriptMu sync.Mutex
	transcript := make([]OutputChunk, 0, 16)
	readPipe := func(name string, pipe io.Reader, target *bytes.Buffer) {
		reader := bufio.NewReader(pipe)
		for {
			chunk := make([]byte, 4096)
			n, err := reader.Read(chunk)
			if n > 0 {
				target.Write(chunk[:n])
				transcriptMu.Lock()
				transcript = append(transcript, OutputChunk{
					Stream: name,
					Text:   string(chunk[:n]),
					At:     time.Since(start),
				})
				transcriptMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		readPipe("stdout", stdout, &stdoutBuf)
	}()
	go func() {
		defer wg.Done()
		readPipe("stderr", stderr, &stderrBuf)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, waitErr
		}
	}

	return &CommandResult{
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		ExitCode:   exitCode,
		Duration:   time.Since(start),
		Transcript: transcript,
	}, nil
}

func isOperatorSubcommand(name string) bool {
	switch name {
	case "status", "sessions", "logs", "proposals", "connectors", "skills", "activity", "runs", "dev", "doctor", "models":
		return true
	default:
		return false
	}
}

type ChatSession struct {
	cmd       *exec.Cmd
	ptmx      *os.File
	startedAt time.Time

	mu         sync.Mutex
	output     bytes.Buffer
	transcript []OutputChunk
	done       chan *CommandResult
	closed     chan struct{}
}

func (s *ChatSession) readLoop() {
	reader := bufio.NewReader(s.ptmx)
	for {
		chunk := make([]byte, 4096)
		n, err := reader.Read(chunk)
		if n > 0 {
			s.mu.Lock()
			s.output.Write(chunk[:n])
			s.transcript = append(s.transcript, OutputChunk{
				Stream: "pty",
				Text:   string(chunk[:n]),
				At:     time.Since(s.startedAt),
			})
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *ChatSession) waitLoop() {
	waitErr := s.cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	s.mu.Lock()
	result := &CommandResult{
		Stdout:     s.output.String(),
		ExitCode:   exitCode,
		Duration:   time.Since(s.startedAt),
		Transcript: append([]OutputChunk(nil), s.transcript...),
	}
	s.mu.Unlock()
	s.done <- result
	close(s.closed)
}

func (s *ChatSession) SendLine(line string) error {
	_, err := io.WriteString(s.ptmx, line+"\n")
	return err
}

func (s *ChatSession) WaitFor(substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(s.Output(), substr) {
			return nil
		}
		select {
		case <-s.closed:
			if strings.Contains(s.Output(), substr) {
				return nil
			}
			return fmt.Errorf("process exited before output contained %q", substr)
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %q; output=%s", substr, s.Output())
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *ChatSession) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String()
}

func (s *ChatSession) Transcript() []OutputChunk {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OutputChunk(nil), s.transcript...)
}

func (s *ChatSession) Done() <-chan struct{} {
	return s.closed
}

func (s *ChatSession) CloseGracefully(timeout time.Duration) (*CommandResult, error) {
	_ = s.SendLine("/exit")
	select {
	case res := <-s.done:
		return res, nil
	case <-time.After(timeout):
		return s.Kill()
	}
}

func (s *ChatSession) Kill() (*CommandResult, error) {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case res := <-s.done:
		return res, nil
	case <-time.After(5 * time.Second):
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		select {
		case res := <-s.done:
			return res, nil
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("failed to stop chat session")
		}
	}
}

func (r *CLIRunner) PersistedConfigPath() string {
	return filepath.Join(r.homeDir, ".navi", "config.json")
}

func (r *CLIRunner) Cleanup() error {
	return os.RemoveAll(r.homeDir)
}
