// Package watcher monitors the Vault directory for owner edits and delivers
// debounced change batches, while muting the projector's own writes so
// reprojection never re-enters as a fake owner edit (memory-projection-v1.md §9).
//
// Two mechanisms keep projector writes out of the owner-edit stream:
//
//   - A time-based mute window: Mute(path, d) tells the watcher to ignore events
//     for a path until the window elapses. The worker brackets every projector
//     write with a mute.
//   - Debounce: rapid follow-up saves (editors often write-rename-truncate) are
//     collapsed into one batch per path after a quiet window (default 750ms).
package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// EventKind classifies a settled change.
type EventKind int

const (
	// Written means the file was created or modified by the owner.
	Written EventKind = iota
	// Removed means the file was deleted by the owner.
	Removed
)

// Event is a settled, debounced change for one path (absolute).
type Event struct {
	Path string
	Kind EventKind
}

// Config tunes the watcher.
type Config struct {
	Root         string        // vault root directory (watched recursively)
	Debounce     time.Duration // quiet window before a change settles (default 750ms)
	Logger       *slog.Logger
	IgnoreSuffix []string // path suffixes to ignore (e.g. editor swap files)
}

// Watcher delivers debounced Events on Events().
type Watcher struct {
	cfg    Config
	fsw    *fsnotify.Watcher
	log    *slog.Logger
	out    chan Event
	debMu  sync.Mutex
	timers map[string]*time.Timer
	muteMu sync.Mutex
	mutes  map[string]time.Time
}

// New constructs a Watcher rooted at cfg.Root. The root must exist.
func New(cfg Config) (*Watcher, error) {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 750 * time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		cfg:    cfg,
		fsw:    fsw,
		log:    cfg.Logger,
		out:    make(chan Event, 64),
		timers: map[string]*time.Timer{},
		mutes:  map[string]time.Time{},
	}
	return w, nil
}

// Events returns the channel of settled changes.
func (w *Watcher) Events() <-chan Event { return w.out }

// Mute suppresses owner-edit detection for path for the given duration. The
// worker calls this immediately before writing a file via the projector so the
// resulting fsnotify event is not mistaken for an owner edit.
func (w *Watcher) Mute(path string, d time.Duration) {
	abs, _ := filepath.Abs(path)
	w.muteMu.Lock()
	w.mutes[abs] = time.Now().Add(d)
	w.muteMu.Unlock()
}

func (w *Watcher) muted(path string) bool {
	w.muteMu.Lock()
	defer w.muteMu.Unlock()
	until, ok := w.mutes[path]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(w.mutes, path)
		return false
	}
	return true
}

// Start begins watching (recursively) until ctx is cancelled. It adds existing
// subdirectories and newly created ones.
func (w *Watcher) Start(ctx context.Context) error {
	go w.loop(ctx)
	if err := w.addRecursive(w.cfg.Root); err != nil {
		_ = w.Close()
		return err
	}
	return nil
}

// Close releases the underlying fsnotify watcher.
func (w *Watcher) Close() error { return w.fsw.Close() }

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries; do not abort the whole walk
		}
		if d.IsDir() {
			if err := w.fsw.Add(path); err != nil {
				w.log.Debug("vault watcher: add dir failed", "path", path, "err", err)
			}
		}
		return nil
	})
}

func (w *Watcher) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ctx, ev)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.log.Debug("vault watcher: error", "err", err)
		}
	}
}

func (w *Watcher) handle(ctx context.Context, ev fsnotify.Event) {
	// Track new directories so recursive watching keeps up.
	if ev.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			_ = w.addRecursive(ev.Name)
			return
		}
	}
	if !w.relevant(ev.Name) {
		return
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return
	}
	w.schedule(ctx, ev.Name)
}

func (w *Watcher) relevant(path string) bool {
	if !strings.HasSuffix(path, ".md") {
		return false
	}
	for _, suf := range w.cfg.IgnoreSuffix {
		if suf != "" && strings.HasSuffix(path, suf) {
			return false
		}
	}
	return true
}

// schedule (re)arms the debounce timer for a path. When it fires, the file's
// current existence decides Written vs Removed, and the mute is checked last so a
// projector write that lands inside its mute window is dropped.
func (w *Watcher) schedule(ctx context.Context, path string) {
	abs, _ := filepath.Abs(path)
	w.debMu.Lock()
	if t, ok := w.timers[abs]; ok {
		t.Stop()
	}
	w.timers[abs] = time.AfterFunc(w.cfg.Debounce, func() {
		w.debMu.Lock()
		delete(w.timers, abs)
		w.debMu.Unlock()
		if w.muted(abs) {
			w.log.Debug("vault watcher: muted (projector write)", "path", abs)
			return
		}
		kind := Written
		if _, err := os.Stat(abs); err != nil {
			kind = Removed
		}
		select {
		case w.out <- Event{Path: abs, Kind: kind}:
		case <-ctx.Done():
		}
	})
	w.debMu.Unlock()
}
