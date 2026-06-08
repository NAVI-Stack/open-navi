package prompts

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartWatcher watches PromptsDir and calls Reload on debounced changes.
func (m *Manager) StartWatcher(ctx context.Context) error {
	if m == nil || m.dir == "" {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := addWatchRecursive(w, m.dir); err != nil {
		_ = w.Close()
		return err
	}

	const debounce = 300 * time.Millisecond
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	pending := false

	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-w.Errors:
				if err != nil {
					slog.Debug("prompts: watcher error", "error", err)
				}
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) && ev.Name != "" {
					if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
						_ = w.Add(ev.Name)
					}
				}
				if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
					pending = true
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(debounce)
				}
			case <-timer.C:
				if !pending {
					continue
				}
				pending = false
				if err := m.Reload(); err != nil {
					slog.Warn("prompts: reload after filesystem change failed", "error", err)
				}
			}
		}
	}()
	return nil
}

func addWatchRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}
