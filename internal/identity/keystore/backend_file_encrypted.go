package keystore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	fileFormatVersion = 1
	aeadAlgorithm     = "AES-256-GCM"

	kdfNamePBKDF2SHA256 = "PBKDF2-SHA256"
	kdfVersionV1        = 1

	defaultPBKDF2Iterations = 600_000
	defaultPBKDF2SaltBytes  = 16
)

type FileEncryptedConfig struct {
	Passphrase string

	// Stored in metadata header for future migrations.
	PBKDF2Iterations int
	PBKDF2SaltBytes  int
}

type FileEncryptedStore struct {
	path string
	cfg  FileEncryptedConfig
}

type keyFileEnvelope struct {
	Header     keyFileHeader `json:"header"`
	Nonce      string        `json:"nonce"`
	Ciphertext string        `json:"ciphertext"`
}

type keyFileHeader struct {
	Version   int          `json:"version"`
	Algorithm string       `json:"algorithm"`
	KeyID     string       `json:"key_id"`
	CreatedAt string       `json:"created_at"`
	KDF       keyFileKDFV1 `json:"kdf"`
}

type keyFileKDFV1 struct {
	Name       string `json:"name"`
	Version    int    `json:"version"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	KeyLen     int    `json:"key_len"`
	Source     string `json:"source"`
}

func NewFileEncryptedStore(path string, cfg FileEncryptedConfig) *FileEncryptedStore {
	iterations := cfg.PBKDF2Iterations
	if iterations <= 0 {
		iterations = defaultPBKDF2Iterations
	}
	saltBytes := cfg.PBKDF2SaltBytes
	if saltBytes <= 0 {
		saltBytes = defaultPBKDF2SaltBytes
	}
	return &FileEncryptedStore{
		path: path,
		cfg: FileEncryptedConfig{
			Passphrase:       cfg.Passphrase,
			PBKDF2Iterations: iterations,
			PBKDF2SaltBytes:  saltBytes,
		},
	}
}

func (s *FileEncryptedStore) Save(_ context.Context, keyID string, privateKey []byte) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return fmt.Errorf("keystore: key id is required")
	}
	if len(privateKey) == 0 {
		return fmt.Errorf("keystore: private key is required")
	}

	dir := filepath.Dir(s.path)
	if err := ensureDirPermissions(dir); err != nil {
		return err
	}

	salt := make([]byte, s.cfg.PBKDF2SaltBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("keystore: read kdf salt: %w", err)
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("keystore: read gcm nonce: %w", err)
	}

	ikm, source, err := s.kekInputMaterial()
	if err != nil {
		return err
	}
	kek := derivePBKDF2Key(ikm, salt, s.cfg.PBKDF2Iterations, 32)

	header := keyFileHeader{
		Version:   fileFormatVersion,
		Algorithm: aeadAlgorithm,
		KeyID:     keyID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		KDF: keyFileKDFV1{
			Name:       kdfNamePBKDF2SHA256,
			Version:    kdfVersionV1,
			Iterations: s.cfg.PBKDF2Iterations,
			Salt:       base64.RawStdEncoding.EncodeToString(salt),
			KeyLen:     32,
			Source:     source,
		},
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return fmt.Errorf("keystore: init aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("keystore: init aes-gcm: %w", err)
	}
	aad, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("keystore: encode header aad: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, privateKey, aad)

	envelope := keyFileEnvelope{
		Header:     header,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("keystore: encode key envelope: %w", err)
	}
	raw = append(raw, '\n')

	if err := writeAtomic0600(s.path, raw); err != nil {
		return err
	}
	return nil
}

func (s *FileEncryptedStore) Load(_ context.Context, keyID string) ([]byte, error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return nil, fmt.Errorf("keystore: key id is required")
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("keystore: read encrypted key file: %w", err)
	}
	if err := ensureExistingPathPermissions(filepath.Dir(s.path), s.path); err != nil {
		return nil, err
	}

	var env keyFileEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: parse key envelope: %w", err))
	}
	if err := validateEnvelope(env); err != nil {
		return nil, errors.Join(ErrDecryptionFailed, err)
	}
	if env.Header.KeyID != keyID {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: key id mismatch"))
	}

	salt, err := base64.RawStdEncoding.DecodeString(env.Header.KDF.Salt)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: decode kdf salt: %w", err))
	}
	nonce, err := base64.RawStdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: decode gcm nonce: %w", err))
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: decode ciphertext: %w", err))
	}

	ikm, _, err := s.kekInputMaterial()
	if err != nil {
		return nil, err
	}
	kek := derivePBKDF2Key(ikm, salt, env.Header.KDF.Iterations, env.Header.KDF.KeyLen)

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("keystore: init aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("keystore: init aes-gcm: %w", err)
	}
	aad, err := json.Marshal(env.Header)
	if err != nil {
		return nil, fmt.Errorf("keystore: encode header aad: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.Join(ErrDecryptionFailed, fmt.Errorf("keystore: aes-gcm open: %w", err))
	}
	return plaintext, nil
}

func (s *FileEncryptedStore) Exists(_ context.Context, keyID string) (bool, error) {
	if strings.TrimSpace(keyID) == "" {
		return false, fmt.Errorf("keystore: key id is required")
	}
	_, err := os.Stat(s.path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("keystore: stat encrypted key file: %w", err)
}

func (s *FileEncryptedStore) kekInputMaterial() ([]byte, string, error) {
	if s.cfg.Passphrase != "" {
		return []byte(s.cfg.Passphrase), "passphrase", nil
	}
	if secret := strings.TrimSpace(os.Getenv("NAVI_IDENTITY_KEYSTORE_OS_SECRET")); secret != "" {
		return []byte(secret), "os-secret-env", nil
	}
	// OS-bound source. This stays local to host/user context and avoids a plain
	// passphrase requirement for default file_encrypted backend.
	seed, err := osBoundSecretSeed()
	if err != nil {
		return nil, "", fmt.Errorf("keystore: derive os-bound secret source: %w", err)
	}
	return seed, "os-bound", nil
}

func osBoundSecretSeed() ([]byte, error) {
	hostname, _ := os.Hostname()
	currentUser, _ := user.Current()
	exe, _ := os.Executable()
	configDir, _ := os.UserConfigDir()
	homeDir, _ := os.UserHomeDir()

	username := ""
	uid := ""
	if currentUser != nil {
		username = currentUser.Username
		uid = currentUser.Uid
	}

	combined := strings.Join([]string{
		"goos=" + runtime.GOOS,
		"goarch=" + runtime.GOARCH,
		"hostname=" + hostname,
		"username=" + username,
		"uid=" + uid,
		"exe=" + exe,
		"config_dir=" + configDir,
		"home_dir=" + homeDir,
	}, "|")
	if strings.TrimSpace(combined) == "" {
		return nil, fmt.Errorf("empty os-bound source")
	}
	sum := sha256.Sum256([]byte(combined))
	return sum[:], nil
}

func derivePBKDF2Key(input, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2.Key(input, salt, iterations, keyLen, sha256.New)
}

func validateEnvelope(env keyFileEnvelope) error {
	if env.Header.Version != fileFormatVersion {
		return fmt.Errorf("keystore: unsupported format version %d", env.Header.Version)
	}
	if env.Header.Algorithm != aeadAlgorithm {
		return fmt.Errorf("keystore: unsupported algorithm %q", env.Header.Algorithm)
	}
	if env.Header.KDF.Name != kdfNamePBKDF2SHA256 {
		return fmt.Errorf("keystore: unsupported kdf %q", env.Header.KDF.Name)
	}
	if env.Header.KDF.Version != kdfVersionV1 {
		return fmt.Errorf("keystore: unsupported kdf version %d", env.Header.KDF.Version)
	}
	if env.Header.KDF.Iterations <= 0 {
		return fmt.Errorf("keystore: invalid kdf iterations")
	}
	if env.Header.KDF.KeyLen != 32 {
		return fmt.Errorf("keystore: unsupported kdf key length %d", env.Header.KDF.KeyLen)
	}
	if strings.TrimSpace(env.Header.KeyID) == "" {
		return fmt.Errorf("keystore: missing key id")
	}
	if strings.TrimSpace(env.Nonce) == "" || strings.TrimSpace(env.Ciphertext) == "" {
		return fmt.Errorf("keystore: missing encrypted payload")
	}
	return nil
}

func ensureDirPermissions(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("keystore: create key directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("keystore: set key directory permissions: %w", err)
	}
	return ensureExistingDirPermissions(dir)
}

func ensureExistingPathPermissions(dir, filePath string) error {
	if err := ensureExistingDirPermissions(dir); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("keystore: stat key file: %w", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		return fmt.Errorf("keystore: insecure key file permissions %o (expected 600)", perm)
	}
	return nil
}

func ensureExistingDirPermissions(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("keystore: stat key directory: %w", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		return fmt.Errorf("keystore: insecure key directory permissions %o (expected 700)", perm)
	}
	return nil
}

func writeAtomic0600(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "private.key.enc.tmp-*")
	if err != nil {
		return fmt.Errorf("keystore: create temp key file: %w", err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("keystore: write temp key file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("keystore: fsync temp key file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("keystore: chmod temp key file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("keystore: close temp key file: %w", err)
	}
	closed = true

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("keystore: atomic rename key file: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	if err := ensureExistingPathPermissions(dir, path); err != nil {
		return err
	}
	return nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("keystore: open key directory for fsync: %w", err)
	}
	defer f.Close()

	if err := f.Sync(); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("keystore: fsync key directory: %w", err)
	}
	return nil
}
