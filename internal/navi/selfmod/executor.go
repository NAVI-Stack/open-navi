package selfmod

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Executor handles running system commands in the workspace.
type Executor struct {
	WorkspaceDir string
}

// NewExecutor creates a new Executor.
func NewExecutor(workspaceDir string) *Executor {
	return &Executor{WorkspaceDir: workspaceDir}
}

// RunGoCommand executes a 'go' command in the workspace.
func (e *Executor) RunGoCommand(ctx context.Context, args ...string) (string, error) {
	if e.WorkspaceDir == "" {
		return "", fmt.Errorf("workspace directory not configured")
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = e.WorkspaceDir
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("go %s failed: %w", strings.Join(args, " "), err)
	}

	return string(output), nil
}

// RunGitCommand executes a 'git' command in the workspace.
func (e *Executor) RunGitCommand(ctx context.Context, args ...string) (string, error) {
	if e.WorkspaceDir == "" {
		return "", fmt.Errorf("workspace directory not configured")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = e.WorkspaceDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}

	return string(output), nil
}
