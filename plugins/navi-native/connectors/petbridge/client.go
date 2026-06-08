package petbridge

import (
	"fmt"
)

// Client handles communication with the PET desktop bridge.
// STUB: This is a contract placeholder. The real implementation will use
// WebSocket/IPC to relay requests to the Tauri desktop shell.
type Client struct {
	bridgeToken string
	bridgeURL   string
}

func NewClient(config Config) *Client {
	bridgeURL := config.BridgeURL
	if bridgeURL == "" {
		bridgeURL = "ws://localhost:6285/bridge"
	}

	return &Client{
		bridgeToken: config.BridgeToken,
		bridgeURL:   bridgeURL,
	}
}

// --- I/O Types ---

type FileReadInput struct {
	Path string `json:"path"`
}

type FileReadOutput struct {
	Content   string `json:"content"`
	BytesRead int    `json:"bytes_read"`
	Truncated bool   `json:"truncated"`
}

type FileWriteInput struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type FileWriteOutput struct {
	Action       string `json:"action"`
	BytesWritten int    `json:"bytes_written"`
	Path         string `json:"path"`
}

type ListDirInput struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
}

type DirEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type ListDirOutput struct {
	Path      string     `json:"path"`
	Entries   []DirEntry `json:"entries"`
	Truncated bool       `json:"truncated"`
}

type FolderPickInput struct {
	Title       string `json:"title,omitempty"`
	DefaultPath string `json:"default_path,omitempty"`
}

type FolderPickOutput struct {
	SelectedPath string `json:"selected_path"`
	Cancelled    bool   `json:"cancelled"`
}

// --- Stub Operations ---

func (c *Client) FileRead(input FileReadInput) (*FileReadOutput, error) {
	if input.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	// STUB: Will relay to PET desktop shell via bridge protocol.
	return nil, fmt.Errorf("pet bridge not connected: file_read requires a running PET desktop instance")
}

func (c *Client) FileWrite(input FileWriteInput) (*FileWriteOutput, error) {
	if input.Path == "" || input.Content == "" {
		return nil, fmt.Errorf("path and content are required")
	}
	// STUB: Will relay to PET desktop shell via bridge protocol.
	return nil, fmt.Errorf("pet bridge not connected: file_write requires a running PET desktop instance")
}

func (c *Client) ListDir(input ListDirInput) (*ListDirOutput, error) {
	if input.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if input.Limit <= 0 {
		input.Limit = 256
	}
	// STUB: Will relay to PET desktop shell via bridge protocol.
	return nil, fmt.Errorf("pet bridge not connected: list_dir requires a running PET desktop instance")
}

func (c *Client) FolderPick(input FolderPickInput) (*FolderPickOutput, error) {
	// STUB: Will relay to PET desktop shell via bridge protocol.
	return nil, fmt.Errorf("pet bridge not connected: folder_pick requires a running PET desktop instance")
}
