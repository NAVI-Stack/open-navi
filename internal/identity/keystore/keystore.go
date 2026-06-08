package keystore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	BackendFileEncrypted = "file_encrypted"
	BackendEnvDev        = "env/dev"
	BackendHardware      = "hardware"
)

var (
	ErrNotFound         = errors.New("keystore: key not found")
	ErrNotImplemented   = errors.New("keystore: backend not implemented")
	ErrDecryptionFailed = errors.New("keystore: decryption failed")
)

// Store persists and loads private key bytes by key ID.
type Store interface {
	Save(ctx context.Context, keyID string, privateKey []byte) error
	Load(ctx context.Context, keyID string) ([]byte, error)
	Exists(ctx context.Context, keyID string) (bool, error)
}

// Config controls keystore backend selection and encryption parameters.
type Config struct {
	Backend string
	DataDir string

	// Passphrase is optional. When empty, the file backend derives a KEK from
	// OS-bound secret material.
	Passphrase string

	// PBKDF2 parameters are versioned and stored in key metadata header.
	PBKDF2Iterations int
	PBKDF2SaltBytes  int
}

// New returns a keystore backend. Default backend is file_encrypted.
func New(cfg Config) (Store, error) {
	backend := strings.TrimSpace(cfg.Backend)
	if backend == "" {
		backend = BackendFileEncrypted
	}

	switch backend {
	case BackendFileEncrypted:
		dataDir := strings.TrimSpace(cfg.DataDir)
		if dataDir == "" {
			dataDir = "."
		}
		return NewFileEncryptedStore(filepath.Join(dataDir, "identity", "private.key.enc"), FileEncryptedConfig{
			Passphrase:       cfg.Passphrase,
			PBKDF2Iterations: cfg.PBKDF2Iterations,
			PBKDF2SaltBytes:  cfg.PBKDF2SaltBytes,
		}), nil
	case BackendEnvDev:
		return NewEnvDevStore(os.Getenv("NAVI_IDENTITY_PRIVATE_KEY_ENV"))
	case BackendHardware:
		return HardwareStore{}, nil
	default:
		return nil, fmt.Errorf("keystore: unknown backend %q", backend)
	}
}
