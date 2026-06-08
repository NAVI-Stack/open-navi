package blob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store defines the interface for artifact content storage.
type Store interface {
	// Put writes content to the store and returns a relative path/key.
	Put(ctx context.Context, key string, r io.Reader) error
	// Get returns a reader for the content at key.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes content from the store.
	Delete(ctx context.Context, key string) error
	// Exists checks if content exists at key.
	Exists(ctx context.Context, key string) (bool, error)
}

// FilesystemStore implements Store using the local file system.
type FilesystemStore struct {
	rootDir string
}

// NewFilesystemStore creates a new FilesystemStore.
func NewFilesystemStore(rootDir string) (*FilesystemStore, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("blob: failed to create root dir: %w", err)
	}
	return &FilesystemStore{rootDir: rootDir}, nil
}

func (s *FilesystemStore) Put(ctx context.Context, key string, r io.Reader) error {
	path := filepath.Join(s.rootDir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("blob: failed to create parent dirs: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("blob: failed to create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("blob: failed to write content: %w", err)
	}

	return nil
}

func (s *FilesystemStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.rootDir, key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob: content not found: %s", key)
		}
		return nil, fmt.Errorf("blob: failed to open file: %w", err)
	}
	return f, nil
}

func (s *FilesystemStore) Delete(ctx context.Context, key string) error {
	path := filepath.Join(s.rootDir, key)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("blob: failed to remove file: %w", err)
	}
	return nil
}

func (s *FilesystemStore) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(s.rootDir, key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("blob: failed to stat file: %w", err)
}
