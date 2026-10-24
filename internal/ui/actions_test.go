package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/lsp"
)

func TestAppYankAndPaste(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	buf := app.CurrentBuffer()
	if buf == nil {
		t.Fatal("expected active buffer")
	}

	// Setup initial buffer text:
	// Line 0: "first line\n"
	// Line 1: "second line\n"
	// Line 2: "third line\n"
	_ = buf.PT.Insert(0, "first line\nsecond line\nthird line\n")
	buf.CursorPos = buffer.Position{Line: 1, Col: 0}

	// 1. Line-wise yank without selection
	app.executeAction(fsm.Action{Type: fsm.ActionYank})
	if app.YankRegister != "second line\n" {
		t.Fatalf("expected YankRegister %q, got %q", "second line\n", app.YankRegister)
	}

	// 2. Paste after (ActionPasteAfter) -> should paste as a new line after Line 1
	app.executeAction(fsm.Action{Type: fsm.ActionPasteAfter})
	expectedText := "first line\nsecond line\nsecond line\nthird line\n"
	if buf.PT.Text() != expectedText {
		t.Fatalf("after paste after, expected %q, got %q", expectedText, buf.PT.Text())
	}

	// 3. Paste before (ActionPasteBefore) at Line 1 -> should paste before Line 1
	buf.CursorPos = buffer.Position{Line: 1, Col: 0}
	app.executeAction(fsm.Action{Type: fsm.ActionPasteBefore})
	expectedText2 := "first line\nsecond line\nsecond line\nsecond line\nthird line\n"
	if buf.PT.Text() != expectedText2 {
		t.Fatalf("after paste before, expected %q, got %q", expectedText2, buf.PT.Text())
	}

	// 4. Character-wise yank with selection
	// Select "line" from "first line" (col 6 to 10)
	anchor := buffer.Position{Line: 0, Col: 6}
	buf.SelectionAnchor = &anchor
	buf.CursorPos = buffer.Position{Line: 0, Col: 10}
	app.FSM.SetState(fsm.StateSelect)

	app.executeAction(fsm.Action{Type: fsm.ActionYank})
	if app.YankRegister != "line" {
		t.Fatalf("expected YankRegister 'line', got %q", app.YankRegister)
	}
	if buf.SelectionAnchor != nil {
		t.Fatalf("expected SelectionAnchor to be cleared after yank")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected state StateNormal after yank, got %v", app.FSM.State())
	}

	// 5. Character-wise paste after at col 0 of line 0
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	app.executeAction(fsm.Action{Type: fsm.ActionPasteAfter})
	line0, _ := buf.PT.Line(0)
	if !strings.HasPrefix(line0, "flineirst") {
		t.Fatalf("expected 'flineirst...', got %q", line0)
	}

	// 6. Paste with empty register
	app.YankRegister = ""
	app.executeAction(fsm.Action{Type: fsm.ActionPasteAfter})
	if !strings.Contains(app.StatusMessage, "empty") {
		t.Fatalf("expected status message mentioning empty register, got %q", app.StatusMessage)
	}
}

func TestAppIndentAndOutdent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Editor.TabWidth = 4
	app := NewApp(cfg, nil)
	defer app.Close()

	buf := app.CurrentBuffer()
	_ = buf.PT.Insert(0, "line1\nline2\nline3\n")

	// 1. Indent single line
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	app.executeAction(fsm.Action{Type: fsm.ActionIndent})
	line0, _ := buf.PT.Line(0)
	if line0 != "    line1" {
		t.Fatalf("expected '    line1', got %q", line0)
	}

	// 2. Outdent single line
	app.executeAction(fsm.Action{Type: fsm.ActionOutdent})
	line0, _ = buf.PT.Line(0)
	if line0 != "line1" {
		t.Fatalf("expected 'line1', got %q", line0)
	}

	// 3. Multi-line indent with selection
	anchor := buffer.Position{Line: 0, Col: 0}
	buf.SelectionAnchor = &anchor
	buf.CursorPos = buffer.Position{Line: 2, Col: 0}

	app.executeAction(fsm.Action{Type: fsm.ActionIndent})
	l0, _ := buf.PT.Line(0)
	l1, _ := buf.PT.Line(1)
	l2, _ := buf.PT.Line(2)
	if l0 != "    line1" || l1 != "    line2" || l2 != "    line3" {
		t.Fatalf("expected all lines indented with 4 spaces, got %q, %q, %q", l0, l1, l2)
	}

	// 4. Multi-line outdent
	app.executeAction(fsm.Action{Type: fsm.ActionOutdent})
	l0, _ = buf.PT.Line(0)
	l1, _ = buf.PT.Line(1)
	l2, _ = buf.PT.Line(2)
	if l0 != "line1" || l1 != "line2" || l2 != "line3" {
		t.Fatalf("expected all lines outdented, got %q, %q, %q", l0, l1, l2)
	}

	// 5. Outdent line with tab character
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, "\thello\n")
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	buf.SelectionAnchor = nil
	app.executeAction(fsm.Action{Type: fsm.ActionOutdent})
	l0, _ = buf.PT.Line(0)
	if l0 != "hello" {
		t.Fatalf("expected 'hello' after tab outdent, got %q", l0)
	}
}

func TestAppUndoRedoClampsCursor(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	buf := app.CurrentBuffer()
	// Insert short text
	buf.PT.BeginTransaction()
	_ = buf.PT.Insert(0, "short\n")
	buf.PT.CommitTransaction()

	// Insert long text with multiple lines
	buf.PT.BeginTransaction()
	_ = buf.PT.Insert(buf.PT.Len(), "long line with lots of characters\nanother line\nyet another line\n")
	buf.PT.CommitTransaction()

	// Place cursor at line 3, col 15
	buf.CursorPos = buffer.Position{Line: 3, Col: 15}

	// Undo the second insertion
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})

	// Cursor should now be clamped to line 1 or line 0 and col within line length
	if buf.CursorPos.Line >= buf.PT.LineCount() {
		t.Fatalf("cursor line %d out of bounds (LineCount=%d)", buf.CursorPos.Line, buf.PT.LineCount())
	}
	lineLen := app.getLineLength(buf, buf.CursorPos.Line)
	if buf.CursorPos.Col > lineLen {
		t.Fatalf("cursor col %d out of bounds (lineLen=%d)", buf.CursorPos.Col, lineLen)
	}

	// Redo
	app.executeAction(fsm.Action{Type: fsm.ActionRedo})
	if buf.CursorPos.Line >= buf.PT.LineCount() {
		t.Fatalf("cursor line %d out of bounds after redo", buf.CursorPos.Line)
	}
}

func TestAppSelectTextObjectFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	buf := app.CurrentBuffer()
	_ = buf.PT.Insert(0, "plain text without any block or brackets\n")
	buf.CursorPos = buffer.Position{Line: 0, Col: 5}

	// Try to select inside block 'b' ({}) which doesn't exist
	app.executeAction(fsm.Action{
		Type: fsm.ActionSelectTextObject,
		Arg1: "inner",
		Arg2: "b",
	})

	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected state to return to StateNormal, got %v", app.FSM.State())
	}
	if buf.SelectionAnchor != nil {
		t.Fatalf("expected nil SelectionAnchor when text object not found")
	}
	if !strings.Contains(app.StatusMessage, "No enclosing b text object found") {
		t.Fatalf("expected status message about no enclosing b, got %q", app.StatusMessage)
	}
}

func TestAppNordicStepForwardBackward(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	testPath := "/tmp/test_nordic.go"
	app.Buffers[0].FilePath = testPath
	app.Buffers[0].FileName = "test_nordic.go"
	_ = app.Buffers[0].PT.Insert(0, "package main\n\nfunc first() {}\nfunc second() {}\n")

	uri := lsp.PathToURI(testPath)
	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 0},
				End:   lsp.Position{Line: 0, Character: 5},
			},
			Severity: lsp.SeverityWarning,
			Message:  "diag 1",
		},
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 2, Character: 5},
				End:   lsp.Position{Line: 2, Character: 10},
			},
			Severity: lsp.SeverityError,
			Message:  "diag 2",
		},
	}

	app.Update(LSPDiagnosticsMsg{
		Update: lsp.DiagnosticsUpdate{
			URI:         uri,
			FilePath:    testPath,
			Diagnostics: diags,
		},
	})

	// 1. Step forward with 'ä' (ActionStepForward) when drawer closed -> steps LSP diagnostic
	app.Buffers[0].CursorPos = buffer.Position{Line: 0, Col: 0}
	app.executeAction(fsm.Action{Type: fsm.ActionStepForward})
	if app.Buffers[0].CursorPos.Line != 2 {
		t.Fatalf("expected cursor at line 2 for diag 2, got %d", app.Buffers[0].CursorPos.Line)
	}

	// 2. Step backward with 'ö' (ActionStepBackward) -> steps to diag 1
	app.executeAction(fsm.Action{Type: fsm.ActionStepBackward})
	if app.Buffers[0].CursorPos.Line != 0 {
		t.Fatalf("expected cursor at line 0 for diag 1, got %d", app.Buffers[0].CursorPos.Line)
	}
}

func TestAppNextPrevHunkAndDiffStepping(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.go")
	f2 := filepath.Join(tmpDir, "f2.go")

	_ = os.WriteFile(f1, []byte("line 1\n"), 0644)
	_ = os.WriteFile(f2, []byte("func a() {}\n"), 0644)

	app := NewApp(nil, []string{f1, f2})
	defer app.Close()
	app.WorkspacePath = tmpDir

	// Initially no hunks
	app.executeAction(fsm.Action{Type: fsm.ActionNextHunk})
	if !strings.Contains(app.StatusMessage, "No diff hunks active") {
		t.Fatalf("expected status 'No diff hunks active', got %q", app.StatusMessage)
	}

	// Add agent transaction changes
	app.AgentTx.AddChange(f1, "line 1\n", "line 1 modified\n")
	app.AgentTx.AddChange(f2, "func a() {}\n", "func a() { return }\n")

	// Trigger ActionNextHunk -> should open diff review drawer
	app.executeAction(fsm.Action{Type: fsm.ActionNextHunk})
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeDiffReview {
		t.Fatalf("expected drawer to open in DiffReview mode")
	}

	// Step forward / backward in diff drawer with ActionStepForward ('ä') and ActionStepBackward ('ö')
	app.executeAction(fsm.Action{Type: fsm.ActionStepForward})
	// Now step backward
	app.executeAction(fsm.Action{Type: fsm.ActionStepBackward})
}

func TestAppCustomKeyRemapsIntegration(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Keys = map[string]map[string]string{
		"normal": {
			"ctrl+w": "save_all",
			"F5":     "toggle_tree",
		},
		"insert": {
			"ctrl+s": "save",
		},
	}

	app := NewApp(cfg, nil)
	defer app.Close()

	// 1. Normal mode remaps
	act := app.FSM.HandleKey("ctrl+w")
	if act.Type != fsm.ActionSaveAll {
		t.Fatalf("expected ActionSaveAll for remapped ctrl+w, got %v", act.Type)
	}

	act = app.FSM.HandleKey("F5")
	if act.Type != fsm.ActionToggleFileTree {
		t.Fatalf("expected ActionToggleFileTree for remapped F5, got %v", act.Type)
	}

	// 2. Insert mode remap
	app.FSM.SetState(fsm.StateInsert)
	act = app.FSM.HandleKey("ctrl+s")
	if act.Type != fsm.ActionSave {
		t.Fatalf("expected ActionSave for remapped ctrl+s in Insert mode, got %v", act.Type)
	}
}

