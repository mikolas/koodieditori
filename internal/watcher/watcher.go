package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// IgnoredDirectoryNames defines default directory names excluded from watching.
var IgnoredDirectoryNames = map[string]bool{
	"target":       true,
	"build":        true,
	".gradle":      true,
	".ccls-cache":  true,
	"node_modules": true,
	".git":         true,
	".idea":        true,
	".vscode":      true,
	"out":          true,
	"dist":         true,
	".cache":       true,
	"bin":          true,
}

// FileChangeInfo contains the file path and the inotify operation.
type FileChangeInfo struct {
	Path string
	Op   fsnotify.Op
}

// ChangeEvent aggregates debounced file changes.
type ChangeEvent struct {
	Changes []FileChangeInfo
}

// Watcher monitors filesystem directories with filtering and debouncing.
type Watcher struct {
	fsw        *fsnotify.Watcher
	debounce   time.Duration
	events     chan ChangeEvent
	errors     chan error
	done       chan struct{}
	mu         sync.Mutex
	pending    map[string]fsnotify.Op
	timer      *time.Timer
	timerActive bool
	closed     bool
}

// NewWatcher creates a new Watcher with the specified debounce duration.
func NewWatcher(debounce time.Duration) (*Watcher, error) {
	if debounce < 50*time.Millisecond {
		debounce = 50 * time.Millisecond
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fsw:      fsw,
		debounce: debounce,
		events:   make(chan ChangeEvent, 64),
		errors:   make(chan error, 16),
		done:     make(chan struct{}),
		pending:  make(map[string]fsnotify.Op),
	}

	go w.loop()

	return w, nil
}

// Events returns the channel of aggregated ChangeEvents.
func (w *Watcher) Events() <-chan ChangeEvent {
	return w.events
}

// Errors returns the channel of filesystem watcher errors.
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// AddRecursive walks rootPath and watches all non-ignored directories.
func (w *Watcher) AddRecursive(rootPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	cleanRoot := filepath.Clean(rootPath)
	return filepath.WalkDir(cleanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if shouldIgnoreDir(base) && path != cleanRoot {
				return filepath.SkipDir
			}
			_ = w.fsw.Add(path)
		}
		return nil
	})
}

// RemoveRecursive removes all directories under rootPath from watching.
func (w *Watcher) RemoveRecursive(rootPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	cleanRoot := filepath.Clean(rootPath)
	return filepath.WalkDir(cleanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = w.fsw.Remove(path)
		}
		return nil
	})
}

// Close stops the watcher and frees all resources.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	close(w.done)
	w.mu.Unlock()

	return w.fsw.Close()
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			default:
			}

		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(ev)
		}
	}
}

func (w *Watcher) handleEvent(ev fsnotify.Event) {
	// Filter ignored directories and files
	cleanPath := filepath.Clean(ev.Name)
	if IsIgnoredPath(cleanPath) {
		return
	}

	// If a new directory was created, watch it automatically
	if ev.Has(fsnotify.Create) {
		if fi, err := os.Stat(cleanPath); err == nil && fi.IsDir() {
			base := filepath.Base(cleanPath)
			if !shouldIgnoreDir(base) {
				w.mu.Lock()
				if !w.closed {
					_ = w.fsw.Add(cleanPath)
				}
				w.mu.Unlock()
			}
			return
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	w.pending[cleanPath] = w.pending[cleanPath] | ev.Op

	if !w.timerActive {
		w.timerActive = true
		if w.timer == nil {
			w.timer = time.AfterFunc(w.debounce, w.flush)
		} else {
			w.timer.Reset(w.debounce)
		}
	} else {
		w.timer.Reset(w.debounce)
	}
}

func (w *Watcher) flush() {
	w.mu.Lock()
	w.timerActive = false
	if len(w.pending) == 0 || w.closed {
		w.mu.Unlock()
		return
	}

	changes := make([]FileChangeInfo, 0, len(w.pending))
	for p, op := range w.pending {
		changes = append(changes, FileChangeInfo{Path: p, Op: op})
	}
	w.pending = make(map[string]fsnotify.Op)
	w.mu.Unlock()

	select {
	case w.events <- ChangeEvent{Changes: changes}:
	case <-w.done:
	}
}

// IsIgnoredPath returns true if any component of the path is an ignored directory.
func IsIgnoredPath(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if shouldIgnoreDir(p) {
			return true
		}
	}
	return false
}

func shouldIgnoreDir(name string) bool {
	return IgnoredDirectoryNames[name] || strings.HasPrefix(name, ".") && name != "." && name != ".."
}
