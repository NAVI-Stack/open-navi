package prompts

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

const maxPromptFileSize = 512 * 1024

// RenderOptions controls per-call overrides (tests and advanced callers).
type RenderOptions struct {
	RuntimeOverride string
}

// Snapshot is raw template source for introspection (ops / debugging).
// Source is one of:
// - "disk"
// - "embed"
// - "empty_fallback" (whitespace-only file healed from embed)
// - "missing_fallback" (missing disk file served from embed)
// - "oversized_fallback" (oversized disk file served from embed)
type Snapshot struct {
	Kind    Kind
	Source  string
	RelPath string
	Raw     string
}

// PromptSnapshot is kept as a compatibility alias for older callers/docs.
type PromptSnapshot = Snapshot

// Manager loads prompt templates from PromptsDir (writable) with embedded defaults.
// Empty dir means embed-only (no disk I/O for templates).
type Manager struct {
	dir string
	mu  sync.RWMutex
	tpl map[Kind]*template.Template
}

// New creates a manager. dir may be empty for embed-only mode.
func New(dir string) *Manager {
	return &Manager{dir: dir, tpl: make(map[Kind]*template.Template)}
}

// Dir returns the configured prompts directory (may be empty).
func (m *Manager) Dir() string {
	if m == nil {
		return ""
	}
	return m.dir
}

// EnsureDefaults creates parent directories and copies any missing template files from embed.
func (m *Manager) EnsureDefaults() error {
	if m == nil || m.dir == "" {
		return nil
	}
	for _, k := range AllKinds() {
		if err := m.ensureOne(k); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureOne(k Kind) error {
	rel := kindRelPath(k)
	dst := filepath.Join(m.dir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("prompts: mkdir %q: %w", filepath.Dir(dst), err)
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("prompts: stat %q: %w", dst, err)
	}
	em := path.Join("defaults", filepath.ToSlash(rel))
	b, err := fs.ReadFile(defaultFS, em)
	if err != nil {
		return fmt.Errorf("prompts: embedded %s: %w", em, err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return fmt.Errorf("prompts: write %q: %w", dst, err)
	}
	return nil
}

func (m *Manager) readBytesForKind(k Kind) ([]byte, error) {
	rel := kindRelPath(k)
	em := path.Join("defaults", filepath.ToSlash(rel))

	if m.dir == "" {
		b, err := fs.ReadFile(defaultFS, em)
		if err != nil {
			return nil, fmt.Errorf("prompts: embedded %s: %w", em, err)
		}
		return b, nil
	}

	dst := filepath.Join(m.dir, rel)
	b, err := os.ReadFile(dst)
	if err == nil {
		if len(b) > maxPromptFileSize {
			return nil, fmt.Errorf("prompts: %s exceeds max size %d", k, maxPromptFileSize)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			fb, ferr := fs.ReadFile(defaultFS, em)
			if ferr != nil {
				return nil, fmt.Errorf("prompts: empty file %q and embedded fallback failed: %w", dst, ferr)
			}
			_ = os.WriteFile(dst, fb, 0o644)
			return fb, nil
		}
		return b, nil
	}
	if os.IsNotExist(err) {
		if err := m.ensureOne(k); err != nil {
			return nil, err
		}
		b, err = os.ReadFile(dst)
		if err != nil {
			return nil, fmt.Errorf("prompts: read %q after seed: %w", dst, err)
		}
		if len(b) > maxPromptFileSize {
			return nil, fmt.Errorf("prompts: %s exceeds max size %d", k, maxPromptFileSize)
		}
		return b, nil
	}
	return nil, fmt.Errorf("prompts: read %q: %w", dst, err)
}

// Snapshot returns the raw template body for kind (from disk or embedded fallback).
func (m *Manager) Snapshot(kind Kind) (Snapshot, bool) {
	if m == nil || !IsKnownKind(kind) {
		return Snapshot{}, false
	}
	kind = canonicalKind(kind) // resolve Coder-facing alias (NEW-A) to its template
	rel := kindRelPath(kind)
	em := path.Join("defaults", filepath.ToSlash(rel))
	if m.dir == "" {
		b, err := fs.ReadFile(defaultFS, em)
		if err != nil {
			return Snapshot{}, false
		}
		return Snapshot{Kind: kind, Source: "embed", RelPath: rel, Raw: string(b)}, true
	}
	dst := filepath.Join(m.dir, rel)
	b, err := os.ReadFile(dst)
	if err == nil && len(b) > 0 && len(bytes.TrimSpace(b)) > 0 && len(b) <= maxPromptFileSize {
		return Snapshot{Kind: kind, Source: "disk", RelPath: rel, Raw: string(b)}, true
	}
	fb, ferr := fs.ReadFile(defaultFS, em)
	if ferr != nil {
		return Snapshot{}, false
	}
	src := "embed"
	switch {
	case err == nil && len(b) > maxPromptFileSize:
		src = "oversized_fallback"
	case err == nil && len(bytes.TrimSpace(b)) == 0:
		src = "empty_fallback"
	case os.IsNotExist(err):
		src = "missing_fallback"
	}
	return Snapshot{Kind: kind, Source: src, RelPath: rel, Raw: string(fb)}, true
}

func (m *Manager) parseAll() (map[Kind]*template.Template, error) {
	return m.parseAllWith(m.readBytesForKind)
}

func (m *Manager) parseAllEmbedded() (map[Kind]*template.Template, error) {
	return m.parseAllWith(func(k Kind) ([]byte, error) {
		rel := kindRelPath(k)
		em := path.Join("defaults", filepath.ToSlash(rel))
		b, err := fs.ReadFile(defaultFS, em)
		if err != nil {
			return nil, fmt.Errorf("prompts: embedded %s: %w", em, err)
		}
		return b, nil
	})
}

func (m *Manager) parseAllWith(loader func(Kind) ([]byte, error)) (map[Kind]*template.Template, error) {
	out := make(map[Kind]*template.Template)
	for _, k := range AllKinds() {
		b, err := loader(k)
		if err != nil {
			return nil, err
		}
		tpl, err := template.New(string(k)).Parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("prompts: parse %s: %w", k, err)
		}
		out[k] = tpl
	}
	return out, nil
}

// Reload reparses all templates from disk (or embed if missing/empty). Swaps atomically on success.
func (m *Manager) Reload() error {
	if m == nil {
		return fmt.Errorf("prompts: nil manager")
	}
	next, err := m.parseAll()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.tpl = next
	m.mu.Unlock()
	return nil
}

// ReloadEmbedded reparses bundled default templates and swaps them atomically.
// Unlike New(""), this keeps the configured prompt directory so watcher/reload
// can recover automatically once on-disk files become valid again.
func (m *Manager) ReloadEmbedded() error {
	if m == nil {
		return fmt.Errorf("prompts: nil manager")
	}
	next, err := m.parseAllEmbedded()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.tpl = next
	m.mu.Unlock()
	return nil
}

// Render executes the template for kind with data.
func (m *Manager) Render(kind Kind, data any, opts RenderOptions) (string, error) {
	if m == nil {
		return "", fmt.Errorf("prompts: nil manager")
	}
	if strings.TrimSpace(opts.RuntimeOverride) != "" {
		return opts.RuntimeOverride, nil
	}
	kind = canonicalKind(kind) // resolve Coder-facing alias (NEW-A) to its template
	m.mu.RLock()
	tpl := m.tpl[kind]
	m.mu.RUnlock()
	if tpl == nil {
		if err := m.Reload(); err != nil {
			return "", err
		}
		m.mu.RLock()
		tpl = m.tpl[kind]
		m.mu.RUnlock()
		if tpl == nil {
			return "", fmt.Errorf("prompts: no template for %s", kind)
		}
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompts: execute %s: %w", kind, err)
	}
	s := buf.String()
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("prompts: empty render for %s", kind)
	}
	return s, nil
}

// RenderLines renders a template and splits the result into non-empty, non-comment lines.
func (m *Manager) RenderLines(kind Kind, data any, opts RenderOptions) ([]string, error) {
	raw, err := m.Render(kind, data, opts)
	if err != nil {
		return nil, err
	}
	return SplitRuleLines(raw), nil
}

// SplitRuleLines splits rendered text into non-empty, non-comment lines.
func SplitRuleLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// RenderMap renders a template and parses key=value lines into a map.
func (m *Manager) RenderMap(kind Kind, data any, opts RenderOptions) (map[string]string, error) {
	raw, err := m.Render(kind, data, opts)
	if err != nil {
		return nil, err
	}
	return ParseKeyValueLines(raw), nil
}

// ParseKeyValueLines parses lines of "key=value" format into a map.
func ParseKeyValueLines(s string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			out[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	return out
}

var (
	embeddedOnce sync.Once
	embeddedMgr  *Manager
	embeddedErr  error
)

// EmbeddedManager returns a singleton embed-only manager for tests and nil Config fallback.
func EmbeddedManager() (*Manager, error) {
	embeddedOnce.Do(func() {
		m := New("")
		embeddedErr = m.Reload()
		embeddedMgr = m
	})
	return embeddedMgr, embeddedErr
}
