package skill

import (
	"context"
	"strings"
	"sync"
)

// InternalHandler is a function that implements an internal skill interface.
// It returns a payload that will be wrapped in a SkillExecutionResult, or an error.
// The returned error is wrapped in a SkillResultError.
type InternalHandler func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error)

var (
	internalHandlers       = make(map[string]InternalHandler)
	internalPrefixHandlers = make(map[string]InternalHandler)
	internalHandlersMu     sync.RWMutex
)

// RegisterInternalHandler adds an internal handler for a specific skill_id and interface name.
// Key format: "skill_id/interface"
func RegisterInternalHandler(id, iface string, handler InternalHandler) {
	internalHandlersMu.Lock()
	defer internalHandlersMu.Unlock()
	key := id + "/" + iface
	internalHandlers[key] = handler
}

// RegisterInternalPrefixHandler adds an internal handler for a skill_id prefix and interface name.
func RegisterInternalPrefixHandler(prefix, iface string, handler InternalHandler) {
	internalHandlersMu.Lock()
	defer internalHandlersMu.Unlock()
	key := prefix + "/" + iface
	internalPrefixHandlers[key] = handler
}

// GetInternalHandler retrieves a registered internal handler.
func GetInternalHandler(id, iface string) (InternalHandler, bool) {
	internalHandlersMu.RLock()
	defer internalHandlersMu.RUnlock()

	// Try exact match first
	key := id + "/" + iface
	if h, ok := internalHandlers[key]; ok {
		return h, true
	}

	// Try prefix matches
	for pKey, h := range internalPrefixHandlers {
		pParts := strings.SplitN(pKey, "/", 2)
		if len(pParts) == 2 && strings.HasPrefix(id, pParts[0]) && iface == pParts[1] {
			return h, true
		}
	}

	return nil, false
}

func init() {
	// Register the legacy/onboarding connector handler
	RegisterInternalPrefixHandler("connector.", "setup", handleConnectorSetup)
}

func handleConnectorSetup(_ context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
	connType := strings.TrimPrefix(entry.Spec.SkillID, "connector.")
	connType = strings.Trim(connType, ".")
	if connType == "" {
		connType = "telegram"
	}
	message := "To connect this connector securely, run /connect " + connType + " in the CLI so credentials stay out of chat history."
	if redirect, _ := args["redirect"].(string); redirect != "" && redirect != "cli" {
		message = "Connector setup only supports redirect=cli. Run /connect " + connType + "."
	}
	return map[string]any{
		"status":  "ok",
		"message": message,
	}, nil
}
