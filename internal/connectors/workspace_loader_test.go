package connectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgconn "github.com/ceoai/navi/connectors"
)

func TestWorkspaceConnectorLifecycleWithSubprocess(t *testing.T) {
	workspaceRoot := t.TempDir()
	connectorDir := filepath.Join(workspaceRoot, "echo")
	if err := os.MkdirAll(connectorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := fmt.Sprintf(`
name: echo
display_name: Echo Connector
type: subprocess
command: %q
args:
  - -test.run=TestWorkspaceConnectorHelperProcess
  - --
  - connector-helper
capabilities:
  - name: messaging.send
`, os.Args[0])
	if err := os.WriteFile(filepath.Join(connectorDir, "CONNECTOR.yaml"), []byte(strings.TrimSpace(manifest)), 0o644); err != nil {
		t.Fatalf("WriteFile(CONNECTOR.yaml): %v", err)
	}

	reg := NewRegistry()
	specs, err := RegisterWorkspaceConnectorDrivers(workspaceRoot, reg)
	if err != nil {
		t.Fatalf("RegisterWorkspaceConnectorDrivers: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 workspace connector, got %d", len(specs))
	}
	if specs[0].Manifest.Name != "echo" {
		t.Fatalf("unexpected connector name %q", specs[0].Manifest.Name)
	}

	if err := reg.Create("echo", nil, nil, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	conn, ok := reg.Get("echo").(*SubprocessConnector)
	if !ok {
		t.Fatalf("expected subprocess connector, got %T", reg.Get("echo"))
	}

	inboundCh := make(chan pkgconn.InboundMessage, 1)
	conn.SetInboundHandler(func(_ context.Context, msg pkgconn.InboundMessage) error {
		inboundCh <- msg
		return nil
	})

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"echo": 1000}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	mgr.StartAll(ctx)
	defer mgr.StopAll(context.Background(), 2*time.Second)

	waitForFileContains(t, filepath.Join(connectorDir, "started.txt"), "start", 3*time.Second)

	msg := pkgconn.OutboundMessage{ChatID: "chat-1", Content: "hello from navi"}
	if err := mgr.Dispatch(context.Background(), "echo", msg); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	waitForFileContains(t, filepath.Join(connectorDir, "outbound.ndjson"), `"Content":"hello from navi"`, 3*time.Second)

	select {
	case inbound := <-inboundCh:
		if inbound.ChatID != "chat-1" {
			t.Fatalf("unexpected inbound chat id %q", inbound.ChatID)
		}
		if inbound.Content != "hello from helper" {
			t.Fatalf("unexpected inbound content %q", inbound.Content)
		}
		if inbound.SourceMessageRef != "msg-1" {
			t.Fatalf("unexpected inbound source ref %q", inbound.SourceMessageRef)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for inbound subprocess message")
	}
}

func TestWorkspaceConnectorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if len(os.Args) < 3 || os.Args[len(os.Args)-1] != "connector-helper" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var frame struct {
			Type string         `json:"type"`
			Msg  map[string]any `json:"msg"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		switch frame.Type {
		case "start":
			_ = os.WriteFile("started.txt", []byte("start"), 0o644)
			_, _ = os.Stdout.Write(append([]byte(`{"type":"message","chat_id":"chat-1","content":"hello from helper","source_ref":"msg-1"}`), '\n'))
		case "send":
			f, err := os.OpenFile("outbound.ndjson", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err == nil {
				_ = json.NewEncoder(f).Encode(frame.Msg)
				_ = f.Close()
			}
		case "stop":
			return
		}
	}
	os.Exit(0)
}

func waitForFileContains(t *testing.T, path, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), needle) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("timed out waiting for %q in %s; got %q", needle, path, string(data))
}
