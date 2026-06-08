package examplemessaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(config Config) *Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.example-messaging.local"
	}

	return &Client{
		apiKey:  config.APIKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

type SendMessageInput struct {
	Recipient      string `json:"recipient"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type SendMessageOutput struct {
	MessageID      string `json:"message_id"`
	Delivered      bool   `json:"delivered"`
	ProviderStatus string `json:"provider_status"`
}

type FetchUpdatesInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type Update struct {
	UpdateID   string `json:"update_id"`
	ReceivedAt string `json:"received_at"`
	Sender     string `json:"sender"`
	Body       string `json:"body"`
}

type FetchUpdatesOutput struct {
	NextCursor string   `json:"next_cursor,omitempty"`
	Updates    []Update `json:"updates"`
}

func (c *Client) SendMessage(input SendMessageInput) (*SendMessageOutput, error) {
	if input.Recipient == "" || input.Body == "" {
		return nil, fmt.Errorf("recipient and body are required")
	}

	var out SendMessageOutput
	if err := c.postJSON("/messages", input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) FetchUpdates(input FetchUpdatesInput) (*FetchUpdatesOutput, error) {
	if input.Limit <= 0 {
		input.Limit = 25
	}

	var out FetchUpdatesOutput
	if err := c.postJSON("/updates/fetch", input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) postJSON(path string, input any, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("example messaging API returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(output)
}
