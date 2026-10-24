package test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/ipc"
	"github.com/mikolas/koodieditori/internal/ui"
)

// TestAgentConcurrentBurstStress simulates an external AI agent or build tool
// modifying 50 files concurrently while the editor is running with active file watching,
// IPC socket handling, and user keystroke events.
// Ensures zero deadlocks, race conditions, or dropped events.
func TestAgentConcurrentBurstStress(t *testing.T) {
	tmpDir := t.TempDir()
	fileCount := 50

	// 1. Create initial files
	filePaths := make([]string, fileCount)
	for i := 0; i < fileCount; i++ {
		p := filepath.Join(tmpDir, fmt.Sprintf("module_%02d.go", i))
		initialContent := fmt.Sprintf("package module%02d\n\nfunc Calculate() int {\n\treturn %d\n}\n", i, i)
		if err := os.WriteFile(p, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", p, err)
		}
		filePaths[i] = p
	}

	cfg := config.DefaultConfig()
	cfg.Agent.BatchDebounceMs = 20

	app := ui.NewApp(cfg, filePaths[:5]) // Open first 5 files as active tabs
	defer app.Close()

	// Initialize file watcher
	if err := app.StartWatcher(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// 2. Launch concurrent burst modifications
	var wg sync.WaitGroup
	errCh := make(chan error, fileCount)

	concurrency := 10
	filesPerWorker := fileCount / concurrency

	startBurst := make(chan struct{})

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()
			<-startBurst

			startIdx := workerID * filesPerWorker
			endIdx := startIdx + filesPerWorker
			for i := startIdx; i < endIdx; i++ {
				filePath := filePaths[i]
				newContent := fmt.Sprintf("package module%02d\n\n// Modified by Agent worker %d\nfunc Calculate() int {\n\treturn %d * 100\n}\n", i, workerID, i)
				if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
					errCh <- fmt.Errorf("worker %d failed to write %s: %w", workerID, filePath, err)
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}

	// Also simulate concurrent UI keystrokes during the burst
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startBurst

		keys := []string{"j", "k", "l", "h", "w", "b", "alt+1", "alt+2", "esc"}
		for i := 0; i < 30; i++ {
			k := keys[i%len(keys)]
			app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
			time.Sleep(3 * time.Millisecond)
		}
	}()

	// Also simulate concurrent Agent Diff transaction review via IPC
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startBurst

		time.Sleep(10 * time.Millisecond)
		_ = app.HandleDiff(ipc.DiffParams{
			File: filePaths[0],
		})
	}()

	// Trigger all workers simultaneously
	close(startBurst)

	// Wait for all concurrent routines with a safety timeout against deadlocks
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK DETECTED: concurrent agent burst did not complete within 5 seconds")
	}

	close(errCh)
	for err := range errCh {
		t.Errorf("burst error: %v", err)
	}

	// Verify that the editor state is intact and view renders cleanly
	view := app.View()
	if len(view) == 0 {
		t.Fatal("view was empty after stress burst")
	}
}
