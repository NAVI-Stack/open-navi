package skill

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type skillCtxKey int

const (
	testTransportKey skillCtxKey = iota
)

// NormalizationResult encapsulates a newly normalized skill along with warnings.
type NormalizationResult struct {
	Entry    SkillEntry
	Source   string // "openclaw" | "raw" | "url" | "yaml"
	Warnings []string
}

// Normalize detects the format of the input and applies the appropriate normalizer.
// Input can be a local path, an HTTPS URL, or raw text.
func Normalize(ctx context.Context, input, name string) (*NormalizationResult, error) {
	if strings.HasPrefix(input, "https://") {
		entry, err := NormalizeFromURL(ctx, input)
		if err != nil {
			return nil, err
		}
		if name != "" && entry.Skill.Name == "" {
			entry.Skill.Name = name
		}
		return &NormalizationResult{Entry: *entry, Source: "url"}, nil
	}

	if info, err := os.Stat(input); err == nil && info.IsDir() {
		yamlPath := filepath.Join(input, "SKILL.yaml")
		if _, err := os.Stat(yamlPath); err == nil {
			data, err := os.ReadFile(yamlPath)
			if err != nil {
				return nil, fmt.Errorf("navi: skill normalize: %w", err)
			}
			var spec OSS27Spec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return nil, fmt.Errorf("navi: skill normalize: %w", err)
			}
			applySpecDefaults(&spec)
			if err := validateSpec(&spec); err != nil {
				return nil, fmt.Errorf("navi: skill normalize: %w", err)
			}
			return &NormalizationResult{
				Entry: SkillEntry{
					Skill: Skill{
						ID:          spec.SkillID,
						Name:        spec.Display.Name,
						Description: spec.Display.Description,
						FilePath:    yamlPath,
						BaseDir:     input,
					},
					Spec: &spec,
				},
				Source: "yaml",
			}, nil
		}
		mdPath := filepath.Join(input, "SKILL.md")
		if _, err := os.Stat(mdPath); err == nil {
			entry, err := NormalizeOpenClawSkill(input)
			if err != nil {
				return nil, err
			}
			return &NormalizationResult{Entry: *entry, Source: "openclaw"}, nil
		}
	}

	entry := NormalizeRawText(name, input)
	return &NormalizationResult{Entry: entry, Source: "raw"}, nil
}

// NormalizeOpenClawSkill reads a directory expecting an OpenClaw-formatted SKILL.md.
// It parses the frontmatter and sanitizes openclaw-specific fields if necessary.
func NormalizeOpenClawSkill(dir string) (*SkillEntry, error) {
	mdPath := filepath.Join(dir, "SKILL.md")
	contentBytes, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("navi: skill normalize: %w", err)
	}

	content := string(contentBytes)
	name, description, meta, err := ParseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("navi: skill normalize: %w", err)
	}

	if name == "" {
		name = filepath.Base(dir)
	}

	return &SkillEntry{
		Skill: Skill{
			Name:        name,
			Description: description,
			FilePath:    mdPath,
			BaseDir:     dir,
			Body:        StripFrontmatter(content),
		},
		Metadata: meta, // Frontmatter already sanitizes fields we care about
	}, nil
}

// NormalizeRawText creates a skill from an arbitrary markdown/text string.
func NormalizeRawText(name, content string) SkillEntry {
	if name == "" {
		name = "custom-skill"
	}

	description := "Custom user-provided skill."
	lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
	if len(lines) > 0 && len(lines[0]) > 0 {
		description = strings.TrimSpace(lines[0])
		if len(description) > 100 {
			description = description[:97] + "..."
		}
	}

	return SkillEntry{
		Skill: Skill{
			Name:        name,
			Description: description,
			Body:        content,
		},
	}
}

// NormalizeFromURL fetched a SKILL.md payload from an HTTPS endpoint securely.
func NormalizeFromURL(ctx context.Context, rawURL string) (*SkillEntry, error) {
	if !strings.HasPrefix(rawURL, "https://") {
		return nil, fmt.Errorf("navi: skill normalize: only https URLs are supported")
	}

	var tr http.RoundTripper
	if testTr, ok := ctx.Value(testTransportKey).(http.RoundTripper); ok {
		tr = testTr
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("navi: skill normalize url: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("navi: skill normalize url request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("navi: skill normalize url returned status %d", resp.StatusCode)
	}

	// Enforce 256KB download limit
	lr := &io.LimitedReader{R: resp.Body, N: int64(maxSkillFileSize) + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("navi: skill normalize url read: %w", err)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf("navi: skill normalize url exceeded 256KB limit")
	}

	content := string(data)
	name, description, meta, _ := ParseFrontmatter(content)

	if name == "" {
		name = "remote-skill"
	}

	return &SkillEntry{
		Skill: Skill{
			Name:        name,
			Description: description,
			Body:        StripFrontmatter(content),
		},
		Metadata: meta,
	}, nil
}
