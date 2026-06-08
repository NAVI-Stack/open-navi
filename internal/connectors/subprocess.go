package connectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"

	"github.com/ceoai/navi/connectors"
)

// SubprocessConnector runs an out-of-process connector binary and communicates
// via JSON on stdin/stdout. Stderr is piped to the NAVI logs.
type SubprocessConnector struct {
	BaseConnector
	command        string
	args           []string
	workDir        string
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	inboundHandler func(context.Context, connectors.InboundMessage) error
	mu             sync.Mutex
	done           chan struct{}
	doneOnce       sync.Once
}

// NewSubprocessConnector creates a persistent connector that runs a command.
func NewSubprocessConnector(name string, command string, args []string, allowList []string) *SubprocessConnector {
	return &SubprocessConnector{
		BaseConnector: NewBaseConnector(name, allowList),
		command:       command,
		args:          args,
		done:          make(chan struct{}),
	}
}

// SetWorkDir configures the subprocess working directory.
func (c *SubprocessConnector) SetWorkDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workDir = dir
}

// SetInboundHandler injects the callback used for inbound message delivery.
func (c *SubprocessConnector) SetInboundHandler(handler func(context.Context, connectors.InboundMessage) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inboundHandler = handler
}

// Start launches the subprocess and monitors its lifecycle.
func (c *SubprocessConnector) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.IsRunning() {
		return nil
	}

	c.cmd = exec.CommandContext(ctx, c.command, c.args...)
	c.cmd.Dir = c.workDir
	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	// Stderr to system logs
	c.cmd.Stderr = log.Writer()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	c.done = make(chan struct{})
	c.doneOnce = sync.Once{}
	c.SetRunning(true)

	// Monitor stdout for inbound events or log messages
	go c.readLoop(ctx)

	// Monitor process wait
	go func() {
		_ = c.cmd.Wait()
		c.SetRunning(false)
		c.doneOnce.Do(func() { close(c.done) })
	}()

	if err := c.writeFrameLocked(map[string]any{"type": "start"}); err != nil {
		return fmt.Errorf("send start frame: %w", err)
	}

	return nil
}

// Stop terminates the subprocess.
func (c *SubprocessConnector) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.IsRunning() {
		return nil
	}

	_ = c.writeFrameLocked(map[string]any{"type": "stop"})
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	select {
	case <-c.done:
	case <-ctx.Done():
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	}

	c.SetRunning(false)
	return nil
}

// Send delivers a message to the subprocess via stdin as a JSON RPC-like request.
func (c *SubprocessConnector) Send(ctx context.Context, msg connectors.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("subprocess %s: %w", c.Name(), connectors.ErrNotRunning)
	}

	payload, err := json.Marshal(map[string]any{
		"type": "send",
		"msg":  msg,
	})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := fmt.Fprintln(c.stdin, string(payload)); err != nil {
		return fmt.Errorf("write to stdin: %w", err)
	}

	return nil
}

func (c *SubprocessConnector) writeFrameLocked(payload any) error {
	if c.stdin == nil {
		return fmt.Errorf("stdin unavailable")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.stdin, string(body))
	return err
}

func (c *SubprocessConnector) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var frame struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &frame); err != nil {
			// Not a JSON event, log it as a raw line
			log.Printf("subprocess %s: %s", c.Name(), string(line))
			continue
		}

		switch frame.Type {
		case "message":
			var inbound struct {
				Type             string `json:"type"`
				ChatID           string `json:"chat_id"`
				Content          string `json:"content"`
				SourceRef        string `json:"source_ref"`
				SourceChannel    string `json:"source_channel"`
				RuntimeSessionID string `json:"runtime_session_id"`
			}
			if err := json.Unmarshal(line, &inbound); err != nil {
				log.Printf("subprocess %s invalid message frame: %v", c.Name(), err)
				continue
			}
			c.mu.Lock()
			handler := c.inboundHandler
			c.mu.Unlock()
			if handler == nil {
				log.Printf("subprocess %s inbound dropped: no handler configured", c.Name())
				continue
			}
			runtimeSessID := inbound.RuntimeSessionID
			if runtimeSessID == "" {
				runtimeSessID = inbound.ChatID
			}
			msg := connectors.InboundMessage{
				ChatID:           inbound.ChatID,
				Content:          inbound.Content,
				SourceMessageRef: inbound.SourceRef,
				SourceChannel:    inbound.SourceChannel,
				RuntimeSessionID: runtimeSessID,
			}
			if err := handler(ctx, msg); err != nil {
				log.Printf("subprocess %s inbound handler failed: %v", c.Name(), err)
			}
		case "log":
			var entry struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(line, &entry); err == nil && entry.Message != "" {
				log.Printf("subprocess %s: %s", c.Name(), entry.Message)
				continue
			}
			log.Printf("subprocess %s: %s", c.Name(), string(line))
		default:
			log.Printf("subprocess %s event: %s", c.Name(), frame.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("subprocess %s stdout error: %v", c.Name(), err)
	}
}
