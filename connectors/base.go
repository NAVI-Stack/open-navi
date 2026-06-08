package connectors

import (
	"strings"
	"sync/atomic"
)

// BaseConnector provides common functionality that concrete connectors can embed.
// It implements Name(), IsRunning(), SetRunning(), and IsAllowed() so that
// connector implementations don't need to re-implement these patterns.
//
// Usage:
//
//	type MyBot struct {
//	    connectors.BaseConnector
//	    // ... bot-specific fields
//	}
//
//	func NewMyBot(name string, allowList []string) *MyBot {
//	    return &MyBot{
//	        BaseConnector: connectors.NewBaseConnector(name, allowList),
//	    }
//	}
type BaseConnector struct {
	name      string
	running   atomic.Bool
	allowList []string
}

// NewBaseConnector creates a BaseConnector with the given name and allow-list.
func NewBaseConnector(name string, allowList []string) BaseConnector {
	return BaseConnector{
		name:      name,
		allowList: allowList,
	}
}

// Name returns the connector identifier.
func (c *BaseConnector) Name() string {
	return c.name
}

// IsRunning returns whether the connector is currently accepting sends.
func (c *BaseConnector) IsRunning() bool {
	return c.running.Load()
}

// SetRunning atomically sets the running state.
func (c *BaseConnector) SetRunning(running bool) {
	c.running.Store(running)
}

// IsAllowed checks whether a sender ID is permitted by the allow-list.
// If the allow-list is empty, all senders are permitted.
func (c *BaseConnector) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}

	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}

	for _, allowed := range c.allowList {
		trimmed := strings.TrimPrefix(allowed, "@")
		if senderID == allowed ||
			senderID == trimmed ||
			idPart == allowed ||
			idPart == trimmed ||
			(userPart != "" && (userPart == allowed || userPart == trimmed)) {
			return true
		}
	}

	return false
}
