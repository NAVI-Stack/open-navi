package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestWatcher(t *testing.T, root string, debounce time.Duration) *Watcher {
	t.Helper()
	w, err := New(Config{Root: root, Debounce: debounce})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestWatcher_DetectsWriteDebounced(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newTestWatcher(t, root, 80*time.Millisecond)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	path := filepath.Join(root, "note.md")
	// Several rapid writes should collapse to a single settled event.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != Written {
			t.Errorf("kind: got %v want Written", ev.Kind)
		}
		if filepath.Base(ev.Path) != "note.md" {
			t.Errorf("path: got %q", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event within timeout")
	}

	// No second event should arrive from the collapsed batch.
	select {
	case ev := <-w.Events():
		t.Fatalf("unexpected second event: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatcher_StartNonEmptyTreeReturnsPromptly(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"contacts", "knowledge", "memories", "artifacts", "_index"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(root, "contacts", "alex.md"):                 "# Alex",
		filepath.Join(root, "_index", "contacts-by-recency.md"):    "# All contacts",
		filepath.Join(root, "_index", "knowledge-by-topic.md"):     "# Knowledge",
		filepath.Join(root, "knowledge", "general", "topic.md"):    "# Topic",
		filepath.Join(root, "memories", "2026", "06", "memory.md"): "# Memory",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newTestWatcher(t, root, 60*time.Millisecond)

	started := make(chan error, 1)
	go func() {
		started <- w.Start(ctx)
	}()

	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start blocked while registering a non-empty tree")
	}
}

func TestWatcher_MuteSuppressesProjectorWrite(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newTestWatcher(t, root, 60*time.Millisecond)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	path := filepath.Join(root, "projected.md")
	w.Mute(path, time.Second)
	if err := os.WriteFile(path, []byte("projector"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		t.Fatalf("muted projector write leaked as event: %+v", ev)
	case <-time.After(500 * time.Millisecond):
		// success — muted write produced no owner-edit event
	}
}

func TestWatcher_DetectsRemove(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gone.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newTestWatcher(t, root, 60*time.Millisecond)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		if ev.Kind != Removed {
			t.Errorf("kind: got %v want Removed", ev.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no remove event")
	}
}
