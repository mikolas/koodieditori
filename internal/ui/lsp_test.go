package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/lsp"
)

func TestAppLSPDiagnosticsUpdateAndStepping(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)

	testPath := "/tmp/test_main.go"
	app.Buffers[0].FilePath = testPath
	app.Buffers[0].FileName = "test_main.go"
	app.Buffers[0].PT.Insert(0, "package main\n\nfunc main() {\n\tinvalid()\n}\n")

	uri := lsp.PathToURI(testPath)
	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 3, Character: 1},
				End:   lsp.Position{Line: 3, Character: 8},
			},
			Severity: lsp.SeverityError,
			Message:  "undefined: invalid",
		},
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 0},
				End:   lsp.Position{Line: 0, Character: 12},
			},
			Severity: lsp.SeverityWarning,
			Message:  "package comment missing",
		},
	}

	// Deliver diagnostics update
	app.Update(LSPDiagnosticsMsg{
		Update: lsp.DiagnosticsUpdate{
			URI:         uri,
			FilePath:    testPath,
			Diagnostics: diags,
		},
	})

	if app.Buffers[0].DiagnosticsCount != 2 {
		t.Fatalf("expected DiagnosticsCount=2, got %d", app.Buffers[0].DiagnosticsCount)
	}

	// Trigger next diagnostic (Space n d)
	app.FSM.HandleKey("Space")
	app.FSM.HandleKey("n")
	act := app.FSM.HandleKey("d")
	if act.Type != fsm.ActionNextDiagnostic {
		t.Fatalf("expected ActionNextDiagnostic, got %v", act.Type)
	}

	app.executeAction(act)
	// Cursor should jump to line 0 or line 3
	curr := app.CurrentBuffer()
	if curr.CursorPos.Line != 0 && curr.CursorPos.Line != 3 {
		t.Fatalf("expected cursor at diagnostic line (0 or 3), got line %d", curr.CursorPos.Line)
	}
	if !strings.Contains(app.StatusMessage, "undefined: invalid") && !strings.Contains(app.StatusMessage, "package comment missing") {
		t.Fatalf("expected diagnostic message in StatusMessage, got %q", app.StatusMessage)
	}
}

func TestLSPDiagnosticsNoInfiniteLoop(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	testPath := "/tmp/loop_test.go"
	uri := lsp.PathToURI(testPath)
	app.Buffers = []*BufferItem{
		NewBufferItem(1, testPath, "package main\n"),
	}
	app.ActiveBuf = 0

	// Deliver diagnostics update
	app.Update(LSPDiagnosticsMsg{
		Update: lsp.DiagnosticsUpdate{
			URI:      uri,
			FilePath: testPath,
			Diagnostics: []lsp.Diagnostic{
				{Message: "test error", Severity: lsp.SeverityError},
			},
		},
	})

	// Verify that app.lspDiagCh has NO messages queued (which would cause an infinite loop)
	select {
	case unexpected := <-app.lspDiagCh:
		t.Fatalf("unexpected recursive diagnostics broadcast detected: %v", unexpected)
	default:
		// Channel is clear, no loop!
	}
}

func TestAppLSPDefinitionAndJumplist(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)

	srcPath := "/tmp/src_file.go"
	destPath := "/tmp/dest_file.go"

	app.Buffers[0].FilePath = srcPath
	app.Buffers[0].FileName = "src_file.go"
	app.Buffers[0].CursorPos.Line = 5
	app.Buffers[0].CursorPos.Col = 10

	// Deliver definition message
	destURI := lsp.PathToURI(destPath)
	loc := lsp.Location{
		URI: destURI,
		Range: lsp.Range{
			Start: lsp.Position{Line: 42, Character: 4},
			End:   lsp.Position{Line: 42, Character: 12},
		},
	}

	app.Update(LSPDefinitionMsg{
		Locations: []lsp.Location{loc},
	})

	// Check active buffer switched or opened destPath
	curr := app.CurrentBuffer()
	if curr == nil || filepath.Clean(curr.FilePath) != filepath.Clean(destPath) {
		t.Fatalf("expected active buffer to be %s, got %v", destPath, curr)
	}
	if curr.CursorPos.Line != 42 || curr.CursorPos.Col != 4 {
		t.Fatalf("expected cursor at line 42 col 4, got line %d col %d", curr.CursorPos.Line, curr.CursorPos.Col)
	}

	// Verify Jumplist recorded origin so jumplistBack (ctrl+o) can jump back
	app.jumplistBack()
	curr = app.CurrentBuffer()
	if curr == nil || filepath.Clean(curr.FilePath) != filepath.Clean(srcPath) {
		t.Fatalf("expected jump back to %s, got %v", srcPath, curr)
	}
	if curr.CursorPos.Line != 5 || curr.CursorPos.Col != 10 {
		t.Fatalf("expected cursor back at line 5 col 10, got line %d col %d", curr.CursorPos.Line, curr.CursorPos.Col)
	}
}

func TestAppLSPHoverOverlay(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	hoverText := "```go\nfunc Println(a ...any) (n int, err error)\n```\nPrintln formats using the default formats for its operands."
	app.Update(LSPHoverMsg{
		Hover: &lsp.Hover{
			Contents: hoverText,
		},
	})

	if app.FSM.State() != fsm.StateHover {
		t.Fatalf("expected StateHover, got %v", app.FSM.State())
	}
	if app.HoverContent != hoverText {
		t.Fatalf("expected HoverContent %q, got %q", hoverText, app.HoverContent)
	}

	view := app.View()
	if !strings.Contains(view, "Hover Doc") || !strings.Contains(view, "func Println") {
		t.Fatalf("expected Hover Doc popup in View, got:\n%s", view)
	}

	// Dismiss with Esc
	app.FSM.HandleKey("esc")
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected return to StateNormal after Esc, got %v", app.FSM.State())
	}
}

func TestAppLSPCompletionOverlayAndApply(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	curr := app.CurrentBuffer()
	curr.PT.Insert(0, "fmt.Pr")
	curr.CursorPos.Line = 0
	curr.CursorPos.Col = 6 // after "fmt.Pr"

	items := []lsp.CompletionItem{
		{
			Label:      "Printf",
			Kind:       lsp.CompletionFunction,
			Detail:     "func(format string, a ...any) (n int, err error)",
			InsertText: "Printf",
		},
		{
			Label:      "Println",
			Kind:       lsp.CompletionFunction,
			Detail:     "func(a ...any) (n int, err error)",
			InsertText: "Println",
		},
	}

	// Deliver completion items
	app.Update(LSPCompletionMsg{
		Items:  items,
		Prefix: "Pr",
	})

	if app.FSM.State() != fsm.StateCompletion {
		t.Fatalf("expected StateCompletion, got %v", app.FSM.State())
	}
	if len(app.CompletionItems) != 2 {
		t.Fatalf("expected 2 completion items, got %d", len(app.CompletionItems))
	}

	view := app.View()
	if !strings.Contains(view, "Autocomplete") || !strings.Contains(view, "Printf") {
		t.Fatalf("expected Autocomplete popup in View, got:\n%s", view)
	}

	// Navigate down to Println
	act := app.FSM.HandleKey("down")
	if act.Type != fsm.ActionLSPSelectNext {
		t.Fatalf("expected ActionLSPSelectNext, got %v", act.Type)
	}
	app.executeAction(act)
	if app.CompletionIndex != 1 {
		t.Fatalf("expected CompletionIndex=1, got %d", app.CompletionIndex)
	}

	// Select and confirm Println
	confirmAct := app.FSM.HandleKey("enter")
	if confirmAct.Type != fsm.ActionLSPSelectConfirm {
		t.Fatalf("expected ActionLSPSelectConfirm, got %v", confirmAct.Type)
	}
	app.executeAction(confirmAct)

	// Buffer should now contain "fmt.Println"
	if curr.PT.Text() != "fmt.Println" {
		t.Fatalf("expected buffer text 'fmt.Println', got %q", curr.PT.Text())
	}
	if app.FSM.State() != fsm.StateInsert {
		t.Fatalf("expected return to StateInsert after completion, got %v", app.FSM.State())
	}
}

func TestAppLSPCodeActionApply(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	tmpFile := filepath.Join(t.TempDir(), "action_test.go")
	if err := os.WriteFile(tmpFile, []byte("package main\n\nx := 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app.openOrSwitchToFile(tmpFile)

	actions := []lsp.CodeAction{
		{
			Title:       "Remove unused variable",
			IsPreferred: true,
			Edit: &lsp.WorkspaceEdit{
				Changes: map[string][]lsp.TextEdit{
					lsp.PathToURI(tmpFile): {
						{
							Range: lsp.Range{
								Start: lsp.Position{Line: 2, Character: 0},
								End:   lsp.Position{Line: 3, Character: 0},
							},
							NewText: "",
						},
					},
				},
			},
		},
	}

	app.Update(LSPCodeActionMsg{
		Actions: actions,
	})

	if app.FSM.State() != fsm.StateCodeAction {
		t.Fatalf("expected StateCodeAction, got %v", app.FSM.State())
	}

	// Confirm action 1
	act := app.FSM.HandleKey("1")
	app.executeAction(act)

	buf := app.CurrentBuffer()
	if strings.Contains(buf.PT.Text(), "x := 1") {
		t.Fatalf("expected 'x := 1' to be removed by code action, got %q", buf.PT.Text())
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after code action, got %v", app.FSM.State())
	}
}

func TestAppLSPRenameWorkspaceEdit(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)

	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "file1.go")
	f2 := filepath.Join(tmpDir, "file2.go")

	_ = os.WriteFile(f1, []byte("func OldName() {}\n"), 0644)
	_ = os.WriteFile(f2, []byte("func Call() { OldName() }\n"), 0644)

	app.openOrSwitchToFile(f1)

	// Simulate Rename message from server
	edit := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			lsp.PathToURI(f1): {
				{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 5}, End: lsp.Position{Line: 0, Character: 12}},
					NewText: "NewAwesomeName",
				},
			},
			lsp.PathToURI(f2): {
				{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 14}, End: lsp.Position{Line: 0, Character: 21}},
					NewText: "NewAwesomeName",
				},
			},
		},
	}

	app.RenameOriginal = "OldName"
	app.RenameInput = "NewAwesomeName"

	app.Update(LSPRenameMsg{
		Edit: edit,
	})

	// Verify both buffers were updated
	app.openOrSwitchToFile(f1)
	if !strings.Contains(app.CurrentBuffer().PT.Text(), "NewAwesomeName") {
		t.Fatalf("expected f1 to contain NewAwesomeName, got %q", app.CurrentBuffer().PT.Text())
	}

	app.openOrSwitchToFile(f2)
	if !strings.Contains(app.CurrentBuffer().PT.Text(), "NewAwesomeName") {
		t.Fatalf("expected f2 to contain NewAwesomeName, got %q", app.CurrentBuffer().PT.Text())
	}
}

func TestAppLSPDocumentChangesWorkspaceEdit(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "doc1.go")
	f2 := filepath.Join(tmpDir, "doc2.go")

	_ = os.WriteFile(f1, []byte("package foo\nfunc OldFoo() {}\n"), 0644)
	_ = os.WriteFile(f2, []byte("package bar\nfunc Bar() { OldFoo() }\n"), 0644)

	app.openOrSwitchToFile(f1)

	// Simulate Rename message with DocumentChanges format
	edit := &lsp.WorkspaceEdit{
		DocumentChanges: []lsp.TextDocumentEdit{
			{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{
					URI: lsp.PathToURI(f1),
				},
				Edits: []lsp.TextEdit{
					{
						Range:   lsp.Range{Start: lsp.Position{Line: 1, Character: 5}, End: lsp.Position{Line: 1, Character: 11}},
						NewText: "RenamedFoo",
					},
				},
			},
			{
				TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{
					URI: lsp.PathToURI(f2),
				},
				Edits: []lsp.TextEdit{
					{
						Range:   lsp.Range{Start: lsp.Position{Line: 1, Character: 13}, End: lsp.Position{Line: 1, Character: 19}},
						NewText: "RenamedFoo",
					},
				},
			},
		},
	}

	app.applyWorkspaceEdit(edit)

	// Verify both buffers received the DocumentChanges edits
	app.openOrSwitchToFile(f1)
	if !strings.Contains(app.CurrentBuffer().PT.Text(), "RenamedFoo") {
		t.Fatalf("expected f1 to contain RenamedFoo, got %q", app.CurrentBuffer().PT.Text())
	}

	app.openOrSwitchToFile(f2)
	if !strings.Contains(app.CurrentBuffer().PT.Text(), "RenamedFoo") {
		t.Fatalf("expected f2 to contain RenamedFoo, got %q", app.CurrentBuffer().PT.Text())
	}
}

func TestAppLSPUTF16PositionEditsAndStepping(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "utf16_test.go")
	// Line 0: "// 🚀 Launch rocket\n"
	// Rune indices:
	// 0: '/', 1: '/', 2: ' ', 3: '🚀' (surrogate pair, 2 utf16 units), 4: ' ', 5: 'L'
	// UTF-16 character offsets:
	// 0: '/', 1: '/', 2: ' ', 3..4: '🚀', 5: ' ', 6: 'L', 12: ' ' (after "Launch")
	_ = os.WriteFile(f, []byte("// 🚀 Launch rocket\nfunc TestRocket() {}\n"), 0644)

	app.openOrSwitchToFile(f)
	buf := app.CurrentBuffer()

	// 1. Test jumpToNextLSPDiagnostic with UTF-16 character offset 6 (points to 'L')
	uri := lsp.PathToURI(f)
	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 6},
				End:   lsp.Position{Line: 0, Character: 12},
			},
			Severity: lsp.SeverityError,
			Message:  "typo in Launch",
		},
	}
	app.Update(LSPDiagnosticsMsg{
		Update: lsp.DiagnosticsUpdate{
			URI:         uri,
			FilePath:    f,
			Diagnostics: diags,
		},
	})

	buf.CursorPos.Line = 0
	buf.CursorPos.Col = 0
	app.jumpToNextLSPDiagnostic()

	// UTF-16 character 6 must convert to rune column 5
	if buf.CursorPos.Col != 5 {
		t.Fatalf("expected cursor Col=5 (at 'L'), got Col=%d", buf.CursorPos.Col)
	}

	// 2. Test applyFileEdits with UTF-16 character offsets 6..12 replacing "Launch" with "Deploy"
	edit := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			uri: {
				{
					Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 6}, End: lsp.Position{Line: 0, Character: 12}},
					NewText: "Deploy",
				},
			},
		},
	}
	app.applyWorkspaceEdit(edit)

	expectedText := "// 🚀 Deploy rocket\nfunc TestRocket() {}\n"
	if buf.PT.Text() != expectedText {
		t.Fatalf("expected buffer text %q, got %q", expectedText, buf.PT.Text())
	}
}

