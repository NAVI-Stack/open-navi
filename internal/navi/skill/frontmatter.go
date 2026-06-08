package skill

import (
	"bufio"
	"fmt"
	"strings"
)

// ParseFrontmatter extracts name, description, and navi metadata from the YAML
// block between --- delimiters at the top of a SKILL.md file.
//
// Supported frontmatter keys:
//
//	name: skill-name
//	description: "what it does"
//	metadata:
//	  navi:
//	    emoji: "🔧"
//	    requires:
//	      bins: ["git", "gh"]
//	      env: ["GITHUB_TOKEN"]
func ParseFrontmatter(content string) (name, description string, meta SkillMetadata, err error) {
	reader := bufio.NewReader(strings.NewReader(content))

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "", meta, fmt.Errorf("empty or invalid file")
	}
	if strings.TrimSpace(line) != "---" {
		return "", "", meta, fmt.Errorf("no frontmatter found")
	}

	inDescription := false
	var descBuilder strings.Builder
	var inBins, inEnv bool
	var currentSection []string // tracks nesting path

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingSpaces(line)

		// Multi-line description continuation
		if inDescription {
			if strings.HasSuffix(trimmed, `"`) && !strings.HasSuffix(trimmed, `\"`) {
				descBuilder.WriteString(" ")
				descBuilder.WriteString(strings.TrimSuffix(trimmed, `"`))
				inDescription = false
			} else {
				descBuilder.WriteString(" ")
				descBuilder.WriteString(trimmed)
			}
			continue
		}

		// List items (- value) go into whichever list is active
		if strings.HasPrefix(trimmed, "- ") {
			val := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			if inBins {
				meta.RequiresBins = append(meta.RequiresBins, val)
			} else if inEnv {
				meta.RequiresEnv = append(meta.RequiresEnv, val)
			}
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Reset list tracking whenever we move to a new key at current indent or shallower
		inBins = false
		inEnv = false

		// Maintain section path
		depth := indent / 2
		if depth < len(currentSection) {
			currentSection = currentSection[:depth]
		}
		if depth == len(currentSection) {
			currentSection = append(currentSection, key)
		} else {
			// deeper nesting; append
			for len(currentSection) < depth {
				currentSection = append(currentSection, "")
			}
			currentSection = append(currentSection[:depth], key)
		}

		// Top-level keys
		if depth == 0 {
			switch key {
			case "name":
				name = unquote(val)
			case "description":
				if val != "" {
					if strings.HasPrefix(val, `"`) && !strings.HasSuffix(val, `"`) {
						inDescription = true
						descBuilder.WriteString(strings.TrimPrefix(val, `"`))
					} else {
						description = unquote(val)
					}
				}
			}
			continue
		}

		// metadata.navi.* keys
		if depth >= 2 && sectionMatches(currentSection, "metadata", "navi") {
			switch key {
			case "emoji":
				meta.Emoji = unquote(val)
			case "always":
				meta.Always = val == "true"
			}
		}

		// metadata.navi.requires.* keys — inline list or block list trigger
		if depth >= 3 && sectionMatches(currentSection, "metadata", "navi", "requires") {
			switch key {
			case "bins":
				if val != "" {
					meta.RequiresBins = parseInlineList(val)
				} else {
					inBins = true
				}
			case "env":
				if val != "" {
					meta.RequiresEnv = parseInlineList(val)
				} else {
					inEnv = true
				}
			}
		}
	}

	if descBuilder.Len() > 0 && description == "" {
		description = strings.TrimSpace(descBuilder.String())
	}

	return name, description, meta, nil
}

// StripFrontmatter removes the YAML frontmatter block and returns only the body.
func StripFrontmatter(content string) string {
	parts := strings.SplitN(content, "---", 3)
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[2])
	}
	return strings.TrimSpace(content)
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func leadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		if ch == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}

func parseInlineList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		item = unquote(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// sectionMatches returns true if the section path starts with the given segments.
func sectionMatches(section []string, segments ...string) bool {
	for i, seg := range segments {
		if i >= len(section) || section[i] != seg {
			return false
		}
	}
	return true
}
