package bus

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// EmbeddedServer wraps a NATS server running in-process.
type EmbeddedServer struct {
	srv *server.Server
}

// ClientURL returns the URL that clients can connect to.
func (e *EmbeddedServer) ClientURL() string {
	return e.srv.ClientURL()
}

// Shutdown stops the embedded NATS server.
func (e *EmbeddedServer) Shutdown() {
	e.srv.Shutdown()
}

// StartEmbedded starts an in-process NATS JetStream server.
// dataDir is used as the JetStream store directory (a "jetstream" subdirectory
// is created inside it). Use "." for the current working directory.
// Returns an EmbeddedServer whose ClientURL() can be passed to Connect().
func StartEmbedded(dataDir string) (*EmbeddedServer, error) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // OS-assigned port
		JetStream: true,
		StoreDir:  dataDir + "/jetstream",
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("embedded NATS: new server: %w", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("embedded NATS: timed out waiting for ready")
	}
	return &EmbeddedServer{srv: srv}, nil
}

// Connect connects to a NATS server, creates a JetStream context,
// and returns a JetStreamBus instance.
func Connect(url string, db *sql.DB) (*JetStreamBus, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	if err := EnsureStreams(js); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ensure streams: %w", err)
	}

	return NewJetStreamBus(conn, js, db), nil
}
