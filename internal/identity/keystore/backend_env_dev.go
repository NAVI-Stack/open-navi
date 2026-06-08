package keystore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// EnvDevStore is a development-only backend. It is only used when explicitly
// selected via backend = "env/dev".
type EnvDevStore struct {
	envKey string
}

func NewEnvDevStore(envKey string) (Store, error) {
	key := strings.TrimSpace(envKey)
	if key == "" {
		key = "NAVI_IDENTITY_PRIVATE_KEY_B64"
	}
	return &EnvDevStore{envKey: key}, nil
}

func (s *EnvDevStore) Save(_ context.Context, keyID string, privateKey []byte) error {
	if strings.TrimSpace(keyID) == "" {
		return fmt.Errorf("keystore: key id is required")
	}
	if len(privateKey) == 0 {
		return fmt.Errorf("keystore: private key is required")
	}
	return os.Setenv(s.envKey, base64.RawStdEncoding.EncodeToString(privateKey))
}

func (s *EnvDevStore) Load(_ context.Context, keyID string) ([]byte, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, fmt.Errorf("keystore: key id is required")
	}
	raw := strings.TrimSpace(os.Getenv(s.envKey))
	if raw == "" {
		return nil, ErrNotFound
	}
	privateKey, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("env/dev decode: %w", err))
	}
	return privateKey, nil
}

func (s *EnvDevStore) Exists(ctx context.Context, keyID string) (bool, error) {
	_, err := s.Load(ctx, keyID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}
