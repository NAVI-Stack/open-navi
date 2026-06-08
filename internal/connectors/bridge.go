package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/open-navi/navi/connectors"
)

// GatewayBridgeConnector wraps an out-of-process connector registered via the
// gateway API. It implements the Connector interface so the Manager treats it
// identically to in-process connectors.
type GatewayBridgeConnector struct {
	name        string
	callbackURL string
	token       string
	client      *http.Client
	running     atomic.Bool
	maxMsgLen   int
}

// BridgeConfig holds configuration for creating a bridge connector.
type BridgeConfig struct {
	Name             string
	CallbackURL      string
	Token            string
	MaxMessageLength int
}

// NewGatewayBridgeConnector creates a bridge connector for an out-of-process service.
func NewGatewayBridgeConnector(cfg BridgeConfig) *GatewayBridgeConnector {
	return &GatewayBridgeConnector{
		name:        cfg.Name,
		callbackURL: cfg.CallbackURL,
		token:       cfg.Token,
		client:      &http.Client{Timeout: 30 * time.Second},
		maxMsgLen:   cfg.MaxMessageLength,
	}
}

func (b *GatewayBridgeConnector) Name() string    { return b.name }
func (b *GatewayBridgeConnector) IsRunning() bool { return b.running.Load() }

func (b *GatewayBridgeConnector) Start(_ context.Context) error {
	b.running.Store(true)
	return nil
}

func (b *GatewayBridgeConnector) Stop(_ context.Context) error {
	b.running.Store(false)
	return nil
}

// Send delivers a message to the out-of-process connector via HTTP POST.
// HTTP status codes are classified into connector errors:
//   - 429 → ErrRateLimit
//   - 5xx → ErrTemporary
//   - 4xx → ErrSendFailed
func (b *GatewayBridgeConnector) Send(ctx context.Context, msg connectors.OutboundMessage) error {
	if !b.running.Load() {
		return fmt.Errorf("bridge %s: %w", b.name, connectors.ErrNotRunning)
	}

	body, _ := json.Marshal(map[string]string{
		"channel": msg.Channel,
		"chat_id": msg.ChatID,
		"content": msg.Content,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", b.callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("bridge %s: %w: %v", b.name, connectors.ErrSendFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("bridge %s: %w: %v", b.name, connectors.ErrTemporary, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == 429:
		return fmt.Errorf("bridge %s: %w: HTTP 429", b.name, connectors.ErrRateLimit)
	case resp.StatusCode >= 500:
		return fmt.Errorf("bridge %s: %w: HTTP %d", b.name, connectors.ErrTemporary, resp.StatusCode)
	default:
		return fmt.Errorf("bridge %s: %w: HTTP %d", b.name, connectors.ErrSendFailed, resp.StatusCode)
	}
}

// MaxMessageLength implements connectors.MessageLengthProvider.
func (b *GatewayBridgeConnector) MaxMessageLength() int {
	return b.maxMsgLen
}
