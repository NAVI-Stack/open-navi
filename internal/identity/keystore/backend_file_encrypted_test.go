package keystore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileEncryptedStorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode bits are not reliably enforced on windows")
	}

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "identity", "private.key.enc")
	store := NewFileEncryptedStore(path, FileEncryptedConfig{
		Passphrase:       "perm-test-passphrase",
		PBKDF2Iterations: 1000,
	})
	key := make([]byte, 64)
	for i := range key {
		key[i] = byte(i)
	}

	if err := store.Save(ctx, "test-key-id", key); err != nil {
		t.Fatalf("save error: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir error: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected directory mode 700, got %o", got)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file error: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected file mode 600, got %o", got)
	}
}

func TestFileEncryptedStoreRejectsInsecurePermissionsOnLoad(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode bits are not reliably enforced on windows")
	}

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "identity", "private.key.enc")
	store := NewFileEncryptedStore(path, FileEncryptedConfig{
		Passphrase:       "perm-test-passphrase",
		PBKDF2Iterations: 1000,
	})
	if err := store.Save(ctx, "test-key-id", []byte("secret")); err != nil {
		t.Fatalf("save error: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod file error: %v", err)
	}

	_, err := store.Load(ctx, "test-key-id")
	if err == nil {
		t.Fatalf("expected load failure for insecure permissions")
	}
}

func TestFileEncryptedStoreDecryptionFailureWrongPassphrase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "identity", "private.key.enc")

	s1 := NewFileEncryptedStore(path, FileEncryptedConfig{
		Passphrase:       "passphrase-a",
		PBKDF2Iterations: 1000,
	})
	s2 := NewFileEncryptedStore(path, FileEncryptedConfig{
		Passphrase:       "passphrase-b",
		PBKDF2Iterations: 1000,
	})

	if err := s1.Save(ctx, "test-key-id", []byte("secret")); err != nil {
		t.Fatalf("save error: %v", err)
	}
	_, err := s2.Load(ctx, "test-key-id")
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got %v", err)
	}
}

func TestFileEncryptedStoreDecryptionFailureOnTamper(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "identity", "private.key.enc")
	store := NewFileEncryptedStore(path, FileEncryptedConfig{
		Passphrase:       "tamper-passphrase",
		PBKDF2Iterations: 1000,
	})

	if err := store.Save(ctx, "test-key-id", []byte("secret")); err != nil {
		t.Fatalf("save error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}
	var env keyFileEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	ciphertext[0] ^= 0x01
	env.Ciphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal tampered envelope: %v", err)
	}
	if err := os.WriteFile(path, append(tampered, '\n'), 0o600); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	_, err = store.Load(ctx, "test-key-id")
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got %v", err)
	}
}
