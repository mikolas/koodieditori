package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/ipc"
)

func TestSaveActiveBufferAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")
	initialContent := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filePath, []byte(initialContent), 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{filePath})
	defer app.Close()

	curr := app.CurrentBuffer()
	if curr == nil {
		t.Fatal("expected active buffer")
	}

	// Insert text to make it dirty
	_ = curr.PT.Insert(curr.PT.Len(), "// newly inserted comment\n")
	curr.Dirty = true

	// Save active buffer via ActionSave
	app.executeAction(fsm.Action{Type: fsm.ActionSave})

	if curr.Dirty {
		t.Fatal("expected buffer to be clean after save")
	}

	diskData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file after save: %v", err)
	}

	expected := "package main\n\nfunc main() {}\n// newly inserted comment\n"
	if string(diskData) != expected {
		t.Fatalf("expected disk content %q, got %q", expected, string(diskData))
	}

	// Verify permissions preserved (0755)
	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if fi.Mode().Perm() != 0755 {
		t.Fatalf("expected permissions 0755, got %v", fi.Mode().Perm())
	}
}

func TestSaveBufferUnnamed(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil) // empty untitled buffer
	defer app.Close()

	curr := app.CurrentBuffer()
	curr.Dirty = true

	app.executeAction(fsm.Action{Type: fsm.ActionSave})

	if !curr.Dirty {
		t.Fatal("unnamed buffer should remain dirty")
	}
	if app.StatusMessage == "" {
		t.Fatal("expected status message warning about unnamed buffer")
	}
}

func TestSaveAllBuffers(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.txt")
	f2 := filepath.Join(tmpDir, "f2.txt")
	_ = os.WriteFile(f1, []byte("1"), 0644)
	_ = os.WriteFile(f2, []byte("2"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{f1, f2})
	defer app.Close()

	for _, b := range app.Buffers {
		_ = b.PT.Insert(b.PT.Len(), "+more")
		b.Dirty = true
	}

	app.executeAction(fsm.Action{Type: fsm.ActionSaveAll})

	for _, b := range app.Buffers {
		if b.Dirty {
			t.Fatalf("buffer %s should be clean after SaveAll", b.FileName)
		}
	}

	d1, _ := os.ReadFile(f1)
	d2, _ := os.ReadFile(f2)
	if string(d1) != "1+more" || string(d2) != "2+more" {
		t.Fatalf("unexpected content after SaveAll: %q, %q", string(d1), string(d2))
	}
}

func TestBufferCloseDirtyProtection(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.txt")
	f2 := filepath.Join(tmpDir, "f2.txt")
	_ = os.WriteFile(f1, []byte("a"), 0644)
	_ = os.WriteFile(f2, []byte("b"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{f1, f2})
	defer app.Close()

	// Make active buffer dirty
	curr := app.CurrentBuffer()
	curr.Dirty = true

	// Normal close should NOT close dirty buffer
	app.executeAction(fsm.Action{Type: fsm.ActionBufferClose})
	if len(app.Buffers) != 2 {
		t.Fatalf("expected 2 buffers remaining, got %d", len(app.Buffers))
	}

	// Force close should close it
	app.executeAction(fsm.Action{Type: fsm.ActionBufferForceClose})
	if len(app.Buffers) != 1 {
		t.Fatalf("expected 1 buffer remaining after force close, got %d", len(app.Buffers))
	}
}

func TestCtrlCSafetyPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.txt")
	_ = os.WriteFile(f1, []byte("a"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{f1})
	defer app.Close()

	app.Buffers[0].Dirty = true

	// First Ctrl+C: should NOT quit
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatal("expected first Ctrl+C with dirty buffer not to return tea.Quit")
	}
	if !app.quitConfirmPending {
		t.Fatal("expected quitConfirmPending to be true")
	}

	// Second Ctrl+C immediately: should quit
	_, cmd2 := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd2 == nil {
		t.Fatal("expected second Ctrl+C to return tea.Quit command")
	}
}

func TestIsSameFileNoBasenameAliasing(t *testing.T) {
	tmpDir := t.TempDir()
	dirA := filepath.Join(tmpDir, "a")
	dirB := filepath.Join(tmpDir, "b")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.MkdirAll(dirB, 0755)

	fileA := filepath.Join(dirA, "main.go")
	fileB := filepath.Join(dirB, "main.go")
	_ = os.WriteFile(fileA, []byte("package a"), 0644)
	_ = os.WriteFile(fileB, []byte("package b"), 0644)

	// Even though both have base "main.go", they must NOT be considered the same file!
	if isSameFile(fileA, fileB) {
		t.Fatalf("isSameFile aliased different files with same base: %s and %s", fileA, fileB)
	}

	// Same file should match
	if !isSameFile(fileA, fileA) {
		t.Fatal("expected fileA to match fileA")
	}
	relA, _ := filepath.Rel(tmpDir, fileA)
	if !isSameFile(fileA, filepath.Join(tmpDir, relA)) {
		t.Fatal("expected relative and absolute paths of fileA to match")
	}
}

func TestAgentRevertOnlyTouchesTransactionFiles(t *testing.T) {
	tmpDir := t.TempDir()
	agentFile := filepath.Join(tmpDir, "agent.txt")
	userFile := filepath.Join(tmpDir, "user.txt")
	_ = os.WriteFile(agentFile, []byte("disk-new"), 0644)
	_ = os.WriteFile(userFile, []byte("disk-orig"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{agentFile, userFile})
	defer app.Close()

	// User buffer has manual unsaved edits
	app.Buffers[1].PT.Insert(app.Buffers[1].PT.Len(), "-unsaved")
	app.Buffers[1].Dirty = true

	// Simulate an agent transaction affecting only agentFile
	app.openOrSwitchToFile(agentFile)
	app.openAgentReviewDrawer()
	if app.AgentTx != nil {
		app.AgentTx.AddChange(agentFile, "disk-old", "disk-new")
	}

	// Revert the agent transaction
	app.revertAgentTransaction()

	// user.txt must NOT have been reloaded or wiped!
	if !app.Buffers[1].Dirty {
		t.Fatal("userFile dirty status was wiped by agent revert of another file!")
	}
	if app.Buffers[1].PT.Text() != "disk-orig-unsaved" {
		t.Fatalf("userFile content was corrupted by agent revert: got %q", app.Buffers[1].PT.Text())
	}
}

func TestApplyReloadDirtyProtection(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.txt")
	_ = os.WriteFile(f1, []byte("disk-content"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{f1})
	defer app.Close()

	app.Buffers[0].PT.Insert(0, "local-")
	app.Buffers[0].Dirty = true

	// External reload request should NOT clobber local edits
	_ = app.applyReload(ipc.ReloadParams{All: true})

	if !app.Buffers[0].Dirty {
		t.Fatal("applyReload should not mark dirty buffer as clean")
	}
	if app.Buffers[0].PT.Text() != "local-disk-content" {
		t.Fatalf("applyReload clobbered local edits: got %q", app.Buffers[0].PT.Text())
	}
}

func TestTreeDeleteConfirmation(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "file.txt")
	_ = os.WriteFile(f1, []byte("hello"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	app.WorkspacePath = tmpDir
	app.FileTree = NewFileTree(tmpDir, 25)
	app.FileTree.Open = true
	app.FileTree.Refresh()

	// Select file.txt (index 1 under root)
	if len(app.FileTree.VisibleNodes) > 1 {
		app.FileTree.CursorIndex = 1
	}

	// First 'd': must NOT delete
	app.executeAction(fsm.Action{Type: fsm.ActionTreeDelete})
	if _, err := os.Stat(f1); os.IsNotExist(err) {
		t.Fatal("single 'd' press immediately deleted file without confirmation!")
	}

	// Second 'd' within 3s: deletes file
	app.executeAction(fsm.Action{Type: fsm.ActionTreeDelete})
	if _, err := os.Stat(f1); err == nil {
		t.Fatal("expected file to be deleted after second 'd'")
	}
}
