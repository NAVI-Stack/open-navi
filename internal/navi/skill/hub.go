package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HubIndexEntry represents a single skill published in the registry index.
type HubIndexEntry struct {
	SkillID     string   `json:"skill_id"`
	Semver      string   `json:"semver"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	TrustTier   string   `json:"trust_tier"` // "builtin", "verified", "community", "local"
	DownloadURL string   `json:"download_url"`
	SHA256      string   `json:"sha256,omitempty"` // optional integrity check
}

// Hub defines the interface for interacting with a remote skill registry.
type Hub interface {
	Search(ctx context.Context, query string) ([]HubIndexEntry, error)
	Download(ctx context.Context, entry HubIndexEntry, destDir string) error
}

// HubClient implements the Hub interface using an HTTP backend.
type HubClient struct {
	indexURL   string
	httpClient *http.Client

	mu        sync.RWMutex
	cache     []HubIndexEntry
	cacheTime time.Time
	cacheTTL  time.Duration
}

// NewHubClient creates a new HubClient pointing to the specific index JSON URL.
func NewHubClient(indexURL string) *HubClient {
	return &HubClient{
		indexURL: indexURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		cacheTTL: 5 * time.Minute,
	}
}

func (c *HubClient) fetchIndex(ctx context.Context) ([]HubIndexEntry, error) {
	c.mu.RLock()
	if time.Since(c.cacheTime) < c.cacheTTL && c.cache != nil {
		defer c.mu.RUnlock()
		return c.cache, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(c.cacheTime) < c.cacheTTL && c.cache != nil {
		return c.cache, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create hub index request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch hub index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub index returned status %d", resp.StatusCode)
	}

	var entries []HubIndexEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode hub index: %w", err)
	}

	c.cache = entries
	c.cacheTime = time.Now()
	return entries, nil
}

// Search filters the registry index based on a query keyword in the skill_id, description, or tags.
func (c *HubClient) Search(ctx context.Context, query string) ([]HubIndexEntry, error) {
	entries, err := c.fetchIndex(ctx)
	if err != nil {
		return nil, err
	}

	if query == "" {
		return entries, nil
	}

	q := strings.ToLower(query)
	var results []HubIndexEntry
	for _, entry := range entries {
		match := false
		if strings.Contains(strings.ToLower(entry.SkillID), q) {
			match = true
		} else if strings.Contains(strings.ToLower(entry.Description), q) {
			match = true
		} else {
			for _, tag := range entry.Tags {
				if strings.Contains(strings.ToLower(tag), q) {
					match = true
					break
				}
			}
		}

		if match {
			results = append(results, entry)
		}
	}
	return results, nil
}

// Download fetches the skill archive, verifies its SHA256 (if provided),
// and extracts it to destDir. It supports .tar.gz and .zip files.
func (c *HubClient) Download(ctx context.Context, entry HubIndexEntry, destDir string) error {
	if entry.DownloadURL == "" {
		return fmt.Errorf("skill %q has no download URL", entry.SkillID)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", entry.DownloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %q: %w", entry.SkillID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %q returned status %d", entry.SkillID, resp.StatusCode)
	}

	var buf bytes.Buffer
	hasher := sha256.New()
	reader := io.TeeReader(resp.Body, hasher)

	if _, err := io.Copy(&buf, reader); err != nil {
		return fmt.Errorf("read download content: %w", err)
	}

	if entry.SHA256 != "" {
		actualSHA := fmt.Sprintf("%x", hasher.Sum(nil))
		if actualSHA != entry.SHA256 {
			return fmt.Errorf("sha256 mismatch for %q: expected %s, got %s", entry.SkillID, entry.SHA256, actualSHA)
		}
	}

	// Create destDir if it doesn't exist
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	lowerURL := strings.ToLower(entry.DownloadURL)
	if strings.HasSuffix(lowerURL, ".tar.gz") || strings.HasSuffix(lowerURL, ".tgz") {
		return extractTarGz(&buf, destDir)
	} else if strings.HasSuffix(lowerURL, ".zip") {
		return extractZip(buf.Bytes(), destDir)
	}

	// Default to tar.gz if unknown
	return extractTarGz(&buf, destDir)
}

func extractTarGz(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Simple protection against directory traversal
		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func extractZip(data []byte, dest string) error {
	br := bytes.NewReader(data)
	zr, err := zip.NewReader(br, int64(len(data)))
	if err != nil {
		return err
	}

	for _, f := range zr.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("invalid file path in archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		frc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, f.Mode())
		if err != nil {
			frc.Close()
			return err
		}

		_, err = io.Copy(outFile, frc)
		outFile.Close()
		frc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
