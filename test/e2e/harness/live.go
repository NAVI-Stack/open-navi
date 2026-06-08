package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nhooyr.io/websocket"
)

type LiveEvent struct {
	Type    string
	Seq     int64
	Payload json.RawMessage
}

type LiveClient struct {
	conn *websocket.Conn
}

func NewLiveClient(ctx context.Context, gatewayURL, apiKey, chatID string, afterSeq int64) (*LiveClient, error) {
	u, err := url.Parse(gatewayURL)
	if err != nil {
		return nil, err
	}
	wsURL := "ws://" + u.Host + "/ws/live"
	if strings.EqualFold(u.Scheme, "https") {
		wsURL = "wss://" + u.Host + "/ws/live"
	}
	header := http.Header{}
	header.Set("X-API-Key", apiKey)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, err
	}
	req := map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{
			"chat_id":   chatID,
			"after_seq": afterSeq,
			"stream":    "user",
		},
	}
	data, _ := json.Marshal(req)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "connect write failed")
		return nil, err
	}
	return &LiveClient{conn: conn}, nil
}

func (c *LiveClient) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *LiveClient) WaitForEvent(ctx context.Context, eventType string, timeout time.Duration) (*LiveEvent, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		_, msg, err := c.conn.Read(waitCtx)
		if err != nil {
			return nil, err
		}
		var base struct {
			Type  string `json:"type"`
			Event struct {
				Type    string          `json:"type"`
				Seq     int64           `json:"seq"`
				Payload json.RawMessage `json:"payload"`
			} `json:"event"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			continue
		}
		if base.Type != "event" {
			continue
		}
		if base.Event.Type == eventType {
			return &LiveEvent{
				Type:    base.Event.Type,
				Seq:     base.Event.Seq,
				Payload: base.Event.Payload,
			}, nil
		}
	}
}

func (c *LiveClient) WaitForAny(ctx context.Context, timeout time.Duration, eventTypes ...string) (*LiveEvent, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	set := make(map[string]struct{}, len(eventTypes))
	for _, typ := range eventTypes {
		set[typ] = struct{}{}
	}
	for {
		_, msg, err := c.conn.Read(waitCtx)
		if err != nil {
			return nil, err
		}
		var base struct {
			Type  string `json:"type"`
			Event struct {
				Type    string          `json:"type"`
				Seq     int64           `json:"seq"`
				Payload json.RawMessage `json:"payload"`
			} `json:"event"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			continue
		}
		if base.Type != "event" {
			continue
		}
		if _, ok := set[base.Event.Type]; ok {
			return &LiveEvent{
				Type:    base.Event.Type,
				Seq:     base.Event.Seq,
				Payload: base.Event.Payload,
			}, nil
		}
	}
}

func DecodePayload[T any](ev *LiveEvent) (*T, error) {
	if ev == nil {
		return nil, fmt.Errorf("event is nil")
	}
	var out T
	if err := json.Unmarshal(ev.Payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
