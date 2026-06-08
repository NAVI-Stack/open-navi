package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHubClientSearch(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		entries := []HubIndexEntry{
			{SkillID: "test-skill-1", Description: "A great test skill", Tags: []string{"test", "demo"}},
			{SkillID: "email-skill", Description: "Send and receive emails", Tags: []string{"communication", "inbox"}},
			{SkillID: "github-tools", Description: "PR and issue management for GitHub", Tags: []string{"dev", "vcs"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewHubClient(srv.URL + "/index.json")
	ctx := context.Background()

	t.Run("ExactTag", func(t *testing.T) {
		res, err := client.Search(ctx, "inbox")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].SkillID != "email-skill" {
			t.Fatalf("expected email-skill, got %v", res)
		}
	})

	t.Run("KeywordInDescription", func(t *testing.T) {
		res, err := client.Search(ctx, "issue management")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].SkillID != "github-tools" {
			t.Fatalf("expected github-tools, got %v", res)
		}
	})

	t.Run("EmptyMatch", func(t *testing.T) {
		res, err := client.Search(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Fatalf("expected empty, got %v", res)
		}
	})
	
	t.Run("AllOnEmptyQuery", func(t *testing.T) {
		res, err := client.Search(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 3 {
			t.Fatalf("expected 3 results, got %d", len(res))
		}
	})
}

func TestHubClientHttpError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewHubClient(srv.URL)
	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatalf("expected error on 500 response")
	}
}

func TestHubClientDownload(t *testing.T) {
	t.Parallel()

	// Create an in-memory tar.gz archive
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	fileContent := []byte("display:\n  name: Test Download\n")
	hdr := &tar.Header{
		Name: "test-skill/SKILL.yaml",
		Mode: 0600,
		Size: int64(len(fileContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}
	if _, err := tw.Write(fileContent); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}
	tw.Close()
	gzw.Close()

	archiveBytes := buf.Bytes()
	archiveSHA := fmt.Sprintf("%x", sha256.Sum256(archiveBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/archive.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archiveBytes)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewHubClient("http://unused") // URL doesn't matter for download itself if we give direct URL
	ctx := context.Background()

	tmpDir := t.TempDir()

	entry := HubIndexEntry{
		SkillID:     "test-download",
		DownloadURL: srv.URL + "/archive.tar.gz",
		SHA256:      archiveSHA,
	}

	err := client.Download(ctx, entry, tmpDir)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify extracted content
	b, err := os.ReadFile(filepath.Join(tmpDir, "test-skill", "SKILL.yaml"))
	if err != nil {
		t.Fatalf("readFile failed: %v", err)
	}
	if string(b) != string(fileContent) {
		t.Fatalf("expected %q, got %q", string(fileContent), string(b))
	}
}
