package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
)

func TestInsertModeUndoGrouping(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	buf := app.CurrentBuffer()
	if buf == nil {
		t.Fatal("expected active buffer")
	}

	initialText := "initial content\n"
	_ = buf.PT.Insert(0, initialText)

	// 1. Enter Insert mode via 'i'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if app.FSM.State() != fsm.StateInsert {
		t.Fatalf("expected StateInsert, got %v", app.FSM.State())
	}

	// Type "hello world"
	for _, ch := range "hello world" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	expectedAfterType := "hello worldinitial content\n"
	if buf.PT.Text() != expectedAfterType {
		t.Fatalf("expected %q after typing, got %q", expectedAfterType, buf.PT.Text())
	}

	// 2. Exit Insert mode with Esc -> should commit transaction
	app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after Esc, got %v", app.FSM.State())
	}

	// 3. Press 'u' (undo) -> should revert entire "hello world" in a single undo!
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})
	if buf.PT.Text() != initialText {
		t.Fatalf("expected single undo to revert back to %q, got %q", initialText, buf.PT.Text())
	}

	// 4. Press redo -> should restore "hello world"
	app.executeAction(fsm.Action{Type: fsm.ActionRedo})
	if buf.PT.Text() != expectedAfterType {
		t.Fatalf("expected redo to restore %q, got %q", expectedAfterType, buf.PT.Text())
	}

	// Undo again
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})
	if buf.PT.Text() != initialText {
		t.Fatalf("expected undo to restore %q, got %q", initialText, buf.PT.Text())
	}

	// 5. Test 'o' (open line below) + typing
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if app.FSM.State() != fsm.StateInsert {
		t.Fatalf("expected StateInsert after 'o', got %v", app.FSM.State())
	}

	for _, ch := range "line 2" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Exit insert mode
	app.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Single undo should revert both newline and "line 2"
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})
	if buf.PT.Text() != initialText {
		t.Fatalf("expected 'o' + typing to be undone in single step back to %q, got %q", initialText, buf.PT.Text())
	}
}

func TestFileTreeQueryGitStatusesNoGitStatus(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repository
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tmpDir
	if err := cmdInit.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure test user
	_ = exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run()
	_ = exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()

	// Create and commit initial file
	trackedFile := filepath.Join(tmpDir, "tracked.go")
	_ = os.WriteFile(trackedFile, []byte("package main\n"), 0644)
	_ = exec.Command("git", "-C", tmpDir, "add", "tracked.go").Run()
	_ = exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run()

	// Modify tracked.go
	_ = os.WriteFile(trackedFile, []byte("package main\n// modified\n"), 0644)

	// Create untracked file
	untrackedFile := filepath.Join(tmpDir, "untracked.go")
	_ = os.WriteFile(untrackedFile, []byte("package main\n// untracked\n"), 0644)

	ft := NewFileTree(tmpDir, 25)
	statuses := ft.queryGitStatuses()

	if statuses[trackedFile] != "M" {
		t.Fatalf("expected tracked.go to have status 'M', got %q", statuses[trackedFile])
	}
	if statuses[untrackedFile] != "U" {
		t.Fatalf("expected untracked.go to have status 'U', got %q", statuses[untrackedFile])
	}
}
