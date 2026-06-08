package connectors

import "context"

// InboundMessage is a normalized inbound message emitted by a connector.
type InboundMessage struct {
	ChatID           string
	Content          string
	SourceMessageRef string
	SourceChannel    string
	RuntimeSessionID string
}

// InboundHandlerAware is implemented by connectors that can surface inbound
// messages directly into NAVI through an injected callback.
type InboundHandlerAware interface {
	SetInboundHandler(func(context.Context, InboundMessage) error)
}
