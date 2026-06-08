package model

import (
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

const (
	QuirkSingleSystemMessage  = "single_system_message"
	QuirkNoSystemRole         = "no_system_role"
	QuirkPlainFunctionalStyle = "plain_functional_style"
	QuirkRepeatGuardOptions   = "repeat_guard_options"
	// QuirkSuppressToolsForChat strips all tools from chat-turn requests.
	// Ollama models emit verbose tool-routing reasoning as chat text when tool
	// definitions are present; removing them prevents boundary leakage.
	QuirkSuppressToolsForChat = "suppress_tools_for_chat"
)

func NormalizeProfile(profile orchestration.ModelProfile) orchestration.ModelProfile {
	if !profile.SupportsSystemRole {
		profile.SupportsSystemRole = true
	}
	if !profile.SupportsMultiSystemMessages {
		profile.SupportsMultiSystemMessages = true
	}
	profile.Quirks = uniqueClean(profile.Quirks)
	profile.Metadata = cloneStringMap(profile.Metadata)
	return profile
}

func ProfileSupportsSingleSystemMessage(profile orchestration.ModelProfile) bool {
	profile = NormalizeProfile(profile)
	return hasQuirk(profile, QuirkSingleSystemMessage) || !profile.SupportsMultiSystemMessages
}

func ProfileSupportsSystemRole(profile orchestration.ModelProfile) bool {
	profile = NormalizeProfile(profile)
	return profile.SupportsSystemRole && !hasQuirk(profile, QuirkNoSystemRole)
}

func ProfileHasQuirk(profile orchestration.ModelProfile, quirk string) bool {
	return hasQuirk(NormalizeProfile(profile), quirk)
}

func hasQuirk(profile orchestration.ModelProfile, quirk string) bool {
	for _, candidate := range profile.Quirks {
		if strings.EqualFold(candidate, quirk) {
			return true
		}
	}
	return false
}

func uniqueClean(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
