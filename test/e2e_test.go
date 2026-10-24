package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/lsp"
	"github.com/mikolas/koodieditori/internal/ui"
)

// TestEndToEndCompleteUserWorkflow tests the entire user lifecycle across all
// subsystems: buffer editing, selection-first mutations, fuzzy finder, file tree,
// ephemeral drawers, compiler error jumping, and LSP overlays.
func TestEndToEndCompleteUserWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "main.go")
	content1 := "package main\n\nfunc main() {\n\tmsg := \"hello\"\n\tprintln(msg)\n}\n"
	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to create file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "util.go")
	content2 := "package main\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to create file2: %v", err)
	}

	cfg := config.DefaultConfig()
	app := ui.NewApp(cfg, []string{file1, file2})
	defer app.Close()

	// Initial assertions
	if len(app.Buffers) != 2 {
		t.Fatalf("expected 2 buffers, got %d", len(app.Buffers))
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal initially, got %v", app.FSM.State())
	}

	// 1. Buffer Switching: Alt+2 and Alt+1
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	if app.ActiveBuf != 1 {
		t.Fatalf("expected active buffer 1 after Alt+2, got %d", app.ActiveBuf)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true})
	if app.ActiveBuf != 0 {
		t.Fatalf("expected active buffer 0 after Alt+1, got %d", app.ActiveBuf)
	}

	// 2. Selection-First Word Selection & Deletion
	// Move down to 'func main' and select word forward
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	if app.FSM.State() != fsm.StateSelect {
		t.Fatalf("expected StateSelect after 'w', got %v", app.FSM.State())
	}

	// Delete selection with 'd'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after 'd', got %v", app.FSM.State())
	}

	// Undo deletion with 'u'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	currText := app.CurrentBuffer().PT.Text()
	if !strings.Contains(currText, "func main()") {
		t.Fatalf("expected undo to restore 'func main()', got: %s", currText)
	}

	// 3. Fast Exit from Insert Mode ('jk')
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if app.FSM.State() != fsm.StateInsert {
		t.Fatalf("expected StateInsert, got %v", app.FSM.State())
	}

	// Type 'j' then 'k'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected fast exit to StateNormal on 'jk', got %v", app.FSM.State())
	}

	// 4. Fuzzy Finder Picker (Space f f)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if app.FSM.State() != fsm.StatePicker {
		t.Fatalf("expected StatePicker after Space f f, got %v", app.FSM.State())
	}

	// Dismiss picker with Esc
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after Esc on picker, got %v", app.FSM.State())
	}

	// 5. File Tree Explorer (Space e)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected StateTree after Space e, got %v", app.FSM.State())
	}

	// Dismiss tree with 'q'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after 'q' on tree, got %v", app.FSM.State())
	}

	// 6. Ephemeral Output Drawer & Error Jump
	app.Drawer.Open = true
	app.Drawer.Mode = ui.DrawerModeOutput
	app.Drawer.SetContent(file2 + ":3:5: error: syntax error\n")
	app.FSM.SetState(fsm.StateDrawer)

	// Jump to error with Enter
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Must switch to file2 and line 2 (0-indexed for 3rd line)
	if app.CurrentBuffer().FilePath != file2 {
		t.Fatalf("expected jump to %s, got %s", file2, app.CurrentBuffer().FilePath)
	}
	if app.CurrentBuffer().CursorPos.Line != 2 {
		t.Fatalf("expected cursor on line 2, got %d", app.CurrentBuffer().CursorPos.Line)
	}

	// Dismiss drawer with Esc
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after Esc on drawer, got %v", app.FSM.State())
	}

	// 7. LSP Diagnostics & Autocomplete Popover
	// Inject LSP completion item
	app.FSM.SetState(fsm.StateCompletion)
	app.CompletionItems = []lsp.CompletionItem{
		{Label: "Println", Detail: "func(a ...any)", Kind: lsp.CompletionFunction},
	}
	app.CompletionIndex = 0

	// Apply completion with Enter
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if app.FSM.State() != fsm.StateInsert && app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateInsert/Normal after completion applied, got %v", app.FSM.State())
	}

	// Render final complete viewport view
	view := app.View()
	if len(view) == 0 {
		t.Fatal("expected view to render successfully")
	}
}
