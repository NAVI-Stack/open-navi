package filetools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

const (
	ReadFileToolName  = "navi.files.read"
	ListDirToolName   = "navi.files.list"
	WriteFileToolName = "navi.files.write"

	maxReadBytes   = 128 * 1024
	maxListEntries = 256
)

// IsFileTool reports whether name is a workspace file tool.
func IsFileTool(name string) bool {
	switch name {
	case ReadFileToolName, ListDirToolName, WriteFileToolName:
		return true
	default:
		return false
	}
}

// PathChecker is implemented by governor.Governor and enforces path policy
// before file I/O occurs.
type PathChecker interface {
	CheckPath(targetPath string) error
}

// Tools returns the LLM tool definitions for workspace file access.
func Tools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        ReadFileToolName,
			Description: "Read a UTF-8 text file from the workspace using a relative path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file inside the workspace.",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        ListDirToolName,
			Description: "List files and directories inside the workspace using a relative path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative directory path inside the workspace. Defaults to the workspace root.",
					},
				},
			},
		},
		{
			Name:        WriteFileToolName,
			Description: "Create or overwrite a text file in the workspace using a relative path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file inside the workspace.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Complete file contents to write.",
					},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

// Execute runs the named file tool against the configured workspace.
func Execute(ctx context.Context, name string, args map[string]any, workspaceDir string, checker PathChecker) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if args == nil {
		args = map[string]any{}
	}

	switch name {
	case ReadFileToolName:
		return executeReadFile(args["path"], workspaceDir, checker)
	case ListDirToolName:
		return executeListDir(args["path"], workspaceDir, checker)
	case WriteFileToolName:
		return executeWriteFile(args["path"], args["content"], workspaceDir, checker)
	default:
		return "", fmt.Errorf("unknown file tool %q", name)
	}
}

func executeReadFile(rawPath any, workspaceDir string, checker PathChecker) (string, error) {
	targetPath, displayPath, err := resolveWorkspacePath(workspaceDir, stringArg(rawPath), checker, false)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", displayPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read file %s: path is a directory", displayPath)
	}

	f, err := os.Open(targetPath)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", displayPath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxReadBytes+1))
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", displayPath, err)
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + "\n\n[truncated: file exceeds 131072 bytes]", nil
	}
	return string(data), nil
}

func executeListDir(rawPath any, workspaceDir string, checker PathChecker) (string, error) {
	targetPath, displayPath, err := resolveWorkspacePath(workspaceDir, stringArg(rawPath), checker, true)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return "", fmt.Errorf("list dir %s: %w", displayPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("list dir %s: path is not a directory", displayPath)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return "", fmt.Errorf("list dir %s: %w", displayPath, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	if len(entries) == 0 {
		return fmt.Sprintf("%s\n(empty)", displayPath), nil
	}

	var sb strings.Builder
	sb.WriteString(displayPath)
	sb.WriteByte('\n')
	for i, entry := range entries {
		if i >= maxListEntries {
			sb.WriteString(fmt.Sprintf("... (%d more entries)", len(entries)-maxListEntries))
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		sb.WriteString(name)
		if i < len(entries)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}

func executeWriteFile(rawPath, rawContent any, workspaceDir string, checker PathChecker) (string, error) {
	targetPath, displayPath, err := resolveWorkspacePath(workspaceDir, stringArg(rawPath), checker, false)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(displayPath, "/") || displayPath == "." {
		return "", fmt.Errorf("write file %s: expected a file path", displayPath)
	}

	content, ok := rawContent.(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("write file %s: %w", displayPath, err)
	}

	action := "updated"
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		action = "created"
	}

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write file %s: %w", displayPath, err)
	}

	return fmt.Sprintf("%s %s (%d bytes)", action, displayPath, len(content)), nil
}

func resolveWorkspacePath(workspaceDir, rawPath string, checker PathChecker, allowEmpty bool) (string, string, error) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return "", "", fmt.Errorf("workspace directory is not configured")
	}

	displayPath := strings.TrimSpace(rawPath)
	if displayPath == "" {
		if !allowEmpty {
			return "", "", fmt.Errorf("path is required")
		}
		displayPath = "."
	}

	if checker != nil {
		if err := checker.CheckPath(displayPath); err != nil {
			return "", "", err
		}
	}

	cleanPath := filepath.Clean(displayPath)
	if filepath.IsAbs(cleanPath) {
		return "", "", fmt.Errorf("path access denied: absolute paths are not allowed")
	}
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path access denied: workspace escape blocked")
	}

	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace: %w", err)
	}
	targetPath := filepath.Join(absWorkspace, cleanPath)
	relPath, err := filepath.Rel(absWorkspace, targetPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path access denied: workspace escape blocked")
	}

	display := filepath.ToSlash(cleanPath)
	if display == "" {
		display = "."
	}
	return targetPath, display, nil
}

func stringArg(v any) string {
	s, _ := v.(string)
	return s
}
