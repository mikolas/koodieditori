package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mikolas/koodieditori/internal/buffer"
)

func TestIsIgnoredPath(t *testing.T) {
	cases := []struct {
		path     string
		expected bool
	}{
		{"src/main.go", false},
		{"target/debug/app", true},
		{"build/libs/app.jar", true},
		{".gradle/cache/file", true},
		{".ccls-cache/index", true},
		{"node_modules/express/index.js", true},
		{".git/HEAD", true},
		{".idea/workspace.xml", true},
		{".vscode/settings.json", true},
		{"vendor/foo.go", false},
	}

	for _, c := range cases {
		got := IsIgnoredPath(c.path)
		if got != c.expected {
			t.Errorf("IsIgnoredPath(%q) = %v; expected %v", c.path, got, c.expected)
		}
	}
}

func TestWatcherDebounceAndEvents(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	_ = os.MkdirAll(srcDir, 0755)

	ignoredDir := filepath.Join(tmpDir, "target")
	_ = os.MkdirAll(ignoredDir, 0755)

	w, err := NewWatcher(60 * time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	if err := w.AddRecursive(tmpDir); err != nil {
		t.Fatalf("failed to add recursive path: %v", err)
	}

	// 1. Write file in ignored directory (should produce NO events)
	ignoredFile := filepath.Join(ignoredDir, "artifact.o")
	_ = os.WriteFile(ignoredFile, []byte("ignored"), 0644)

	// 2. Rapid multiple writes to a normal source file
	srcFile := filepath.Join(srcDir, "main.go")
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(srcFile, []byte("package main"), 0644)
		time.Sleep(5 * time.Millisecond)
	}

	// Expect a single aggregated event after debounce
	select {
	case ev := <-w.Events():
		if len(ev.Changes) == 0 {
			t.Fatal("expected non-empty changes")
		}
		foundSrc := false
		for _, c := range ev.Changes {
			if filepath.Clean(c.Path) == filepath.Clean(srcFile) {
				foundSrc = true
			}
			if filepath.Clean(c.Path) == filepath.Clean(ignoredFile) {
				t.Fatalf("unexpected change event for ignored file %q", c.Path)
			}
		}
		if !foundSrc {
			t.Fatalf("expected change for %q in event", srcFile)
		}

	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for debounced change event")
	}
}

func TestReconcileBufferClean(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "hello.txt")
	_ = os.WriteFile(file, []byte("Hello, World!"), 0644)

	pt := buffer.NewPieceTableFromBytes([]byte("Initial content"))
	res := ReconcileBuffer(file, false, pt)

	if res.Status != ReconcileCleanReload {
		t.Fatalf("expected ReconcileCleanReload, got %v", res.Status)
	}
	if pt.Text() != "Hello, World!" {
		t.Fatalf("expected buffer reloaded with 'Hello, World!', got %q", pt.Text())
	}
}

func TestReconcileBufferConflict(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "hello.txt")
	_ = os.WriteFile(file, []byte("Modified on disk"), 0644)

	pt := buffer.NewPieceTableFromBytes([]byte("Local unsaved edits"))
	res := ReconcileBuffer(file, true, pt)

	if res.Status != ReconcileConflict {
		t.Fatalf("expected ReconcileConflict, got %v", res.Status)
	}
	// Buffer content must NOT be overwritten
	if pt.Text() != "Local unsaved edits" {
		t.Fatalf("expected buffer preserved with local edits, got %q", pt.Text())
	}
}

func TestReconcileBufferFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "nonexistent.txt")

	pt := buffer.NewPieceTableFromBytes([]byte("Something"))
	res := ReconcileBuffer(file, false, pt)

	if res.Status != ReconcileFileNotFound {
		t.Fatalf("expected ReconcileFileNotFound, got %v", res.Status)
	}
}
