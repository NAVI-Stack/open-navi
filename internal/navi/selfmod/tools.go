package selfmod

import (
	"context"
	"fmt"

	"github.com/open-navi/navi/internal/llm"
)

const (
	GitStatusToolName = "navi.selfmod.git_status"
	GitDiffToolName   = "navi.selfmod.git_diff"
	GitCommitToolName = "navi.selfmod.git_commit"
	GoBuildToolName   = "navi.selfmod.go_build"
	GoTestToolName    = "navi.selfmod.go_test"
)

// IsSelfModTool reports whether name is a self-modification / workspace dev tool.
func IsSelfModTool(name string) bool {
	switch name {
	case GitStatusToolName, GitDiffToolName, GitCommitToolName, GoBuildToolName, GoTestToolName:
		return true
	default:
		return false
	}
}

// Tools returns the tool definitions for codebase self-modification.
func Tools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        GitStatusToolName,
			Description: "Run 'git status --porcelain' in the workspace to see staged and unstaged changes.",
		},
		{
			Name:        GitDiffToolName,
			Description: "Run 'git diff' in the workspace to see details of unstaged changes.",
		},
		{
			Name:        GitCommitToolName,
			Description: "Commit staged changes with a descriptive message.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "The commit message.",
					},
				},
				"required": []string{"message"},
			},
		},
		{
			Name:        GoBuildToolName,
			Description: "Run 'go build ./...' in the workspace to verify compilation.",
		},
		{
			Name:        GoTestToolName,
			Description: "Run 'go test ./...' in the workspace or in specific subdirectories.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to test (e.g. './internal/navi/...'). Defaults to './...'.",
					},
				},
			},
		},
	}
}

// Execute runs the named self-modification tool.
func Execute(ctx context.Context, name string, args map[string]any, exec *Executor) (string, error) {
	if exec == nil {
		return "", fmt.Errorf("self-modification executor not configured")
	}

	switch name {
	case GitStatusToolName:
		return exec.RunGitCommand(ctx, "status", "--porcelain")
	case GitDiffToolName:
		return exec.RunGitCommand(ctx, "diff")
	case GitCommitToolName:
		msg, _ := args["message"].(string)
		if msg == "" {
			return "", fmt.Errorf("commit message required")
		}
		return exec.RunGitCommand(ctx, "commit", "-m", msg)
	case GoBuildToolName:
		return exec.RunGoCommand(ctx, "build", "./...")
	case GoTestToolName:
		path, _ := args["path"].(string)
		if path == "" {
			path = "./..."
		}
		return exec.RunGoCommand(ctx, "test", path)
	default:
		return "", fmt.Errorf("unknown self-modification tool %q", name)
	}
}
