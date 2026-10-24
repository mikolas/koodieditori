package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/ipc"
	"github.com/mikolas/koodieditori/internal/lsp"
	"github.com/mikolas/koodieditori/internal/markdown"
	"github.com/mikolas/koodieditori/internal/tooling"
	"github.com/mikolas/koodieditori/internal/treesitter"
	"github.com/mikolas/koodieditori/internal/watcher"
)

func TestBufferlineRenderingAndHitTest(t *testing.T) {
	theme := DefaultTheme()
	items := []*BufferItem{
		NewBufferItem(1, "main.go", "package main"),
		NewBufferItem(2, "server.cpp", "int main() {}"),
	}
	items[0].Dirty = true
	items[1].AgentActive = true

	rendered := RenderBufferline(items, 0, 80, "koodieditori", theme)
	raw := StripAnsi(rendered)

	if !strings.Contains(raw, "[1: main.go*]") {
		t.Fatalf("expected dirty flag on tab 1, got %q", raw)
	}
	if !strings.Contains(raw, "[2: server.cpp [A]]") {
		t.Fatalf("expected agent badge on tab 2, got %q", raw)
	}
	if !strings.Contains(raw, "[ws: koodieditori]") {
		t.Fatalf("expected workspace tag, got %q", raw)
	}

	// Test Hit testing: tab 0 should be at x = 0
	hit0 := HitTestTab(items, 2)
	if hit0 != 0 {
		t.Fatalf("expected hit on tab 0, got %d", hit0)
	}

	// Hit on tab 1
	hit1 := HitTestTab(items, items[1].StartX+2)
	if hit1 != 1 {
		t.Fatalf("expected hit on tab 1, got %d", hit1)
	}
}

func TestGutterGeometry(t *testing.T) {
	theme := DefaultTheme()

	// Under 999 lines, width must strictly be 6 cols
	width := CalculateGutterWidth(50)
	if width != 6 {
		t.Fatalf("expected gutter width 6 for 50 lines, got %d", width)
	}

	// Render line 10 with relative numbering when cursor is at line 10 (dist 0 -> absolute 11)
	info := GutterInfo{GitSign: "+", DiagSign: "E", IsCurrent: true}
	gStr := RenderGutter(10, 10, 50, true, info, theme)
	raw := StripAnsi(gStr)

	if len(raw) != 6 {
		t.Fatalf("expected raw gutter width 6, got %d (%q)", len(raw), raw)
	}
	if !strings.HasPrefix(raw, "+E") {
		t.Fatalf("expected prefix '+E', got %q", raw)
	}

	// Relative line at distance 3
	infoRel := GutterInfo{IsCurrent: false}
	gRel := RenderGutter(13, 10, 50, true, infoRel, theme)
	rawRel := StripAnsi(gRel)
	if !strings.Contains(rawRel, "3") {
		t.Fatalf("expected relative line distance 3, got %q", rawRel)
	}
}

func TestStatuslineRendering(t *testing.T) {
	theme := DefaultTheme()
	s := StatuslineState{
		Mode:        fsm.StateNormal,
		FilePath:    "src/main.go",
		IsDirty:     true,
		Line:        41,
		Col:         11,
		TotalLines:  100,
		AgentStatus: "Idle",
		KeyBuffer:   "Space n _",
	}

	line := RenderStatusline(s, 100, theme)
	raw := StripAnsi(line)

	if !strings.Contains(raw, "NORMAL") {
		t.Fatalf("expected NORMAL mode in statusline, got %q", raw)
	}
	if !strings.Contains(raw, "src/main.go [+]") {
		t.Fatalf("expected dirty file path in statusline, got %q", raw)
	}
	if !strings.Contains(raw, "Ln 42, Col 12 (42%)") {
		t.Fatalf("expected cursor pos in statusline, got %q", raw)
	}
	if !strings.Contains(raw, "[Agent: Idle]") {
		t.Fatalf("expected agent status, got %q", raw)
	}
	if !strings.Contains(raw, "Space n _") {
		t.Fatalf("expected key buffer, got %q", raw)
	}
}

func TestAppModelLifecycleAndTyping(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	buf := app.CurrentBuffer()
	if buf == nil {
		t.Fatal("expected active buffer")
	}

	// Switch to insert mode: press 'i'
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if app.FSM.State() != fsm.StateInsert {
		t.Fatalf("expected INSERT mode, got %v", app.FSM.State())
	}

	// Type "hello"
	for _, ch := range "hello" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if buf.PT.Text() != "hello" {
		t.Fatalf("expected buffer text 'hello', got %q", buf.PT.Text())
	}
	if buf.CursorPos.Col != 5 {
		t.Fatalf("expected cursor col 5, got %d", buf.CursorPos.Col)
	}
	if !buf.Dirty {
		t.Fatal("expected buffer to be marked dirty")
	}

	// Press Esc: return to NORMAL
	app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected NORMAL mode after Esc, got %v", app.FSM.State())
	}

	// Render view: should not panic or be empty
	view := app.View()
	if len(view) == 0 {
		t.Fatal("expected non-empty view rendering")
	}
}

func TestAppMouseInteraction(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"file1.go", "file2.go"})
	app.Width = 80
	app.Height = 24

	// First render to compute tab coordinates
	_ = app.View()

	// Click on second tab in Row 0
	tab2X := app.Buffers[1].StartX + 2
	app.Update(tea.MouseMsg{
		X:      tab2X,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if app.ActiveBuf != 1 {
		t.Fatalf("expected ActiveBuf 1 after mouse click, got %d", app.ActiveBuf)
	}
}

func TestEmptyGutterPadding(t *testing.T) {
	theme := DefaultTheme()
	// Must not panic on any line count
	for _, count := range []int{0, 1, 5, 50, 1000, 50000} {
		out := RenderEmptyGutter(count, theme)
		if len(out) == 0 {
			t.Fatalf("expected non-empty gutter for %d lines", count)
		}
	}
}

func TestAppNavigationAndMutations(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	buf := app.CurrentBuffer()

	// Initial buffer has empty line
	app.executeAction(fsm.Action{Type: fsm.ActionInsertChar, String: "line 1"})
	app.executeAction(fsm.Action{Type: fsm.ActionInsertNewline})
	app.executeAction(fsm.Action{Type: fsm.ActionInsertChar, String: "line 2"})

	if buf.PT.LineCount() != 2 {
		t.Fatalf("expected 2 lines, got %d", buf.PT.LineCount())
	}

	// Move Up
	app.executeAction(fsm.Action{Type: fsm.ActionMoveUp})
	if buf.CursorPos.Line != 0 {
		t.Fatalf("expected cursor on Line 0, got %d", buf.CursorPos.Line)
	}

	// Move Down
	app.executeAction(fsm.Action{Type: fsm.ActionMoveDown})
	if buf.CursorPos.Line != 1 {
		t.Fatalf("expected cursor on Line 1, got %d", buf.CursorPos.Line)
	}

	// Move Left / Right
	app.executeAction(fsm.Action{Type: fsm.ActionMoveLeft})
	if buf.CursorPos.Col != 5 {
		t.Fatalf("expected col 5, got %d", buf.CursorPos.Col)
	}
	app.executeAction(fsm.Action{Type: fsm.ActionMoveRight})
	if buf.CursorPos.Col != 6 {
		t.Fatalf("expected col 6, got %d", buf.CursorPos.Col)
	}

	// Insert Tab
	app.executeAction(fsm.Action{Type: fsm.ActionInsertTab})
	if buf.CursorPos.Col != 10 {
		t.Fatalf("expected col 10 after tab, got %d", buf.CursorPos.Col)
	}

	// Delete Char
	app.CursorTo(1, 0)
	app.executeAction(fsm.Action{Type: fsm.ActionDeleteChar})
	l2, _ := buf.PT.Line(1)
	if !strings.HasPrefix(l2, "ine 2") {
		t.Fatalf("expected 'ine 2' after delete char, got %q", l2)
	}

	// Backspace at line start: merge with line 0
	app.CursorTo(1, 0)
	app.executeAction(fsm.Action{Type: fsm.ActionBackspace})
	if buf.PT.LineCount() != 1 {
		t.Fatalf("expected 1 line after merge, got %d", buf.PT.LineCount())
	}

	// Undo and Redo
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})
	if buf.PT.LineCount() != 2 {
		t.Fatalf("expected 2 lines after undo, got %d", buf.PT.LineCount())
	}
	app.executeAction(fsm.Action{Type: fsm.ActionRedo})
	if buf.PT.LineCount() != 1 {
		t.Fatalf("expected 1 line after redo, got %d", buf.PT.LineCount())
	}
}

func (a *App) CursorTo(line, col int) {
	buf := a.CurrentBuffer()
	if buf != nil {
		buf.CursorPos.Line = line
		buf.CursorPos.Col = col
	}
}

func TestAppTabCycling(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"f1.go", "f2.go", "f3.go"})

	if app.ActiveBuf != 0 {
		t.Fatalf("expected initial ActiveBuf 0, got %d", app.ActiveBuf)
	}

	// Next tab
	app.executeAction(fsm.Action{Type: fsm.ActionTabNext})
	if app.ActiveBuf != 1 {
		t.Fatalf("expected ActiveBuf 1 after TabNext, got %d", app.ActiveBuf)
	}

	// Next tab
	app.executeAction(fsm.Action{Type: fsm.ActionTabNext})
	if app.ActiveBuf != 2 {
		t.Fatalf("expected ActiveBuf 2, got %d", app.ActiveBuf)
	}

	// Wrap around
	app.executeAction(fsm.Action{Type: fsm.ActionTabNext})
	if app.ActiveBuf != 0 {
		t.Fatalf("expected wrap around to 0, got %d", app.ActiveBuf)
	}

	// Prev tab
	app.executeAction(fsm.Action{Type: fsm.ActionTabPrev})
	if app.ActiveBuf != 2 {
		t.Fatalf("expected ActiveBuf 2 after TabPrev, got %d", app.ActiveBuf)
	}

	// Direct switch
	app.executeAction(fsm.Action{Type: fsm.ActionTabSwitch, Int: 2})
	if app.ActiveBuf != 1 {
		t.Fatalf("expected ActiveBuf 1 after TabSwitch(2), got %d", app.ActiveBuf)
	}
}

func TestMouseScrollingAndDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	buf := app.CurrentBuffer()
	for i := 0; i < 50; i++ {
		_ = buf.PT.Insert(buf.PT.Len(), "line\n")
	}

	// Wheel Down
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelDown,
	})
	if buf.ScrollY != 3 {
		t.Fatalf("expected ScrollY 3 after wheel down, got %d", buf.ScrollY)
	}

	// Wheel Up
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelUp,
	})
	if buf.ScrollY != 0 {
		t.Fatalf("expected ScrollY 0 after wheel up, got %d", buf.ScrollY)
	}

	// Mouse disabled
	app.Config.Editor.Mouse = false
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelDown,
	})
	if buf.ScrollY != 0 {
		t.Fatalf("expected no scroll when mouse is disabled, got %d", buf.ScrollY)
	}
}

func TestAdjustScrollAndSelectionBounds(t *testing.T) {
	buf := NewBufferItem(1, "test.go", "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10")
	viewportHeight := 5
	scrolloff := 2

	// Move cursor down to line 8
	buf.CursorPos.Line = 8
	AdjustScroll(buf, viewportHeight, 80, scrolloff)
	if buf.ScrollY <= 0 {
		t.Fatalf("expected ScrollY > 0 when cursor is at bottom, got %d", buf.ScrollY)
	}

	// Move cursor back up to line 1
	buf.CursorPos.Line = 1
	AdjustScroll(buf, viewportHeight, 80, scrolloff)
	if buf.ScrollY > 1 {
		t.Fatalf("expected ScrollY to decrease, got %d", buf.ScrollY)
	}

	// Selection bounds across multiple lines
	buf.SelectionAnchor = &buffer.Position{Line: 1, Col: 2}
	buf.CursorPos = buffer.Position{Line: 3, Col: 4}

	// Line 1: start.Col to 999999
	s1, e1 := getLineSelectionBounds(1, buf)
	if s1 != 2 || e1 != 999999 {
		t.Fatalf("expected (2, 999999) on start line, got (%d, %d)", s1, e1)
	}

	// Line 2: intermediate line 0 to 999999
	s2, e2 := getLineSelectionBounds(2, buf)
	if s2 != 0 || e2 != 999999 {
		t.Fatalf("expected (0, 999999) on intermediate line, got (%d, %d)", s2, e2)
	}

	// Line 3: end line 0 to end.Col
	s3, e3 := getLineSelectionBounds(3, buf)
	if s3 != 0 || e3 != 4 {
		t.Fatalf("expected (0, 4) on end line, got (%d, %d)", s3, e3)
	}

	// Line 4: outside selection
	s4, e4 := getLineSelectionBounds(4, buf)
	if s4 != -1 || e4 != -1 {
		t.Fatalf("expected (-1, -1) on outside line, got (%d, %d)", s4, e4)
	}
}

func TestExpandTabs(t *testing.T) {
	out := expandTabs("a\tb", 4)
	if out != "a   b" {
		t.Fatalf("expected 'a   b', got %q", out)
	}

	outDefault := expandTabs("a\tb", 0)
	if outDefault != "a   b" {
		t.Fatalf("expected default 4 spaces, got %q", outDefault)
	}
}

func TestAppSelectionActions(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	buf := app.CurrentBuffer()
	_ = buf.PT.Insert(0, "select and delete this text")

	// Select word
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	app.executeAction(fsm.Action{Type: fsm.ActionSelectWord})
	if buf.SelectionAnchor == nil || buf.CursorPos.Col != 6 {
		t.Fatalf("expected word selection up to col 6, got %d", buf.CursorPos.Col)
	}

	// Delete selection
	app.executeAction(fsm.Action{Type: fsm.ActionDeleteSelection})
	if strings.HasPrefix(buf.PT.Text(), "select") {
		t.Fatalf("expected 'select' to be deleted, got %q", buf.PT.Text())
	}
	if buf.SelectionAnchor != nil {
		t.Fatal("expected SelectionAnchor to be cleared after delete")
	}

	// Select line
	app.executeAction(fsm.Action{Type: fsm.ActionSelectLine})
	if buf.SelectionAnchor == nil {
		t.Fatal("expected line selection anchor")
	}

	// Collapse selection
	app.executeAction(fsm.Action{Type: fsm.ActionCollapseSelection})
	if buf.SelectionAnchor != nil {
		t.Fatal("expected SelectionAnchor to be nil after collapse")
	}
}

func TestAppTextObjectSelection(t *testing.T) {
	cfg := config.DefaultConfig()
	code := `package main

func main() {
	a := 1
	b := 2
}
`
	app := NewApp(cfg, nil)
	buf := app.CurrentBuffer()
	buf.FilePath = "main.go"
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, code)
	app.reparseBuffer(buf)

	// Place cursor inside main() at line 3, col 2 ("a := 1")
	buf.CursorPos = buffer.Position{Line: 3, Col: 2}

	// Trigger ActionSelectTextObject: inside block ("i", "b")
	app.executeAction(fsm.Action{
		Type: fsm.ActionSelectTextObject,
		Arg1: "i",
		Arg2: "b",
	})

	if app.FSM.State() != fsm.StateSelect {
		t.Fatalf("expected state SELECT after text object, got %v", app.FSM.State())
	}
	if buf.SelectionAnchor == nil {
		t.Fatal("expected SelectionAnchor to be set")
	}

	// Render view: should show highlighted keywords and functions
	view := app.View()
	if !strings.Contains(view, "package") || !strings.Contains(view, "main") {
		t.Fatalf("view should contain syntax elements, got %q", view)
	}
}

func TestAppPageUpDownAndHomeEnd(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	buf := app.CurrentBuffer()
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("line %03d: the quick brown fox jumps over the lazy dog", i))
	}
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, strings.Join(lines, "\n"))

	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	buf.ScrollY = 0

	// 1. PageDown via Bubble Tea message
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = m.(*App)

	if buf.CursorPos.Line <= 0 {
		t.Fatalf("expected cursor line > 0 after PageDown, got %d", buf.CursorPos.Line)
	}
	firstPgDownLine := buf.CursorPos.Line

	// View should render without panic
	view := app.View()
	if len(view) == 0 {
		t.Fatal("expected rendered view after PageDown")
	}

	// 2. Second PageDown
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = m.(*App)
	if buf.CursorPos.Line <= firstPgDownLine {
		t.Fatalf("expected cursor line > %d after second PageDown, got %d", firstPgDownLine, buf.CursorPos.Line)
	}

	// 3. PageUp
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	app = m.(*App)
	if buf.CursorPos.Line >= 100 || buf.CursorPos.Line < 0 {
		t.Fatalf("invalid cursor line after PageUp: %d", buf.CursorPos.Line)
	}

	// 4. End and Home
	buf.CursorPos.Col = 0
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnd})
	app = m.(*App)
	lineLen := app.getLineLength(buf, buf.CursorPos.Line)
	if buf.CursorPos.Col != lineLen {
		t.Fatalf("expected cursor at col %d after End, got %d", lineLen, buf.CursorPos.Col)
	}

	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyHome})
	app = m.(*App)
	if buf.CursorPos.Col != 0 {
		t.Fatalf("expected cursor at col 0 after Home, got %d", buf.CursorPos.Col)
	}

	// 5. PageDown in Select mode extends selection
	app.FSM.SetState(fsm.StateSelect)
	buf.SelectionAnchor = &buffer.Position{Line: buf.CursorPos.Line, Col: 0}
	anchorLine := buf.SelectionAnchor.Line
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = m.(*App)

	if buf.SelectionAnchor == nil || buf.SelectionAnchor.Line != anchorLine {
		t.Fatalf("expected selection anchor preserved at line %d, got %v", anchorLine, buf.SelectionAnchor)
	}
	if buf.CursorPos.Line <= anchorLine {
		t.Fatalf("expected cursor line to advance past %d, got %d", anchorLine, buf.CursorPos.Line)
	}
}

func TestAppBufferStartEndAndWordNav(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	buf := app.CurrentBuffer()
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("line %02d alpha beta gamma", i))
	}
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, strings.Join(lines, "\n"))

	// Move buffer end (ctrl+end or G)
	app.executeAction(fsm.Action{Type: fsm.ActionMoveBufferEnd})
	if buf.CursorPos.Line != 49 {
		t.Fatalf("expected cursor at line 49 after MoveBufferEnd, got %d", buf.CursorPos.Line)
	}

	// Move buffer start (ctrl+home)
	app.executeAction(fsm.Action{Type: fsm.ActionMoveBufferStart})
	if buf.CursorPos.Line != 0 || buf.CursorPos.Col != 0 {
		t.Fatalf("expected cursor at line 0, col 0 after MoveBufferStart, got %v", buf.CursorPos)
	}

	// Word forward & backward
	app.executeAction(fsm.Action{Type: fsm.ActionMoveWordForward})
	if buf.CursorPos.Col <= 0 {
		t.Fatalf("expected cursor col > 0 after MoveWordForward, got %d", buf.CursorPos.Col)
	}
	app.executeAction(fsm.Action{Type: fsm.ActionMoveWordBackward})
	if buf.CursorPos.Col != 0 {
		t.Fatalf("expected cursor col == 0 after MoveWordBackward, got %d", buf.CursorPos.Col)
	}

	// Extend selection
	app.FSM.SetState(fsm.StateSelect)
	buf.SelectionAnchor = nil
	app.executeAction(fsm.Action{Type: fsm.ActionExtendSelection, String: "right"})
	if buf.SelectionAnchor == nil || buf.CursorPos.Col != 1 {
		t.Fatalf("expected SelectionAnchor set and Col=1, got anchor=%v col=%d", buf.SelectionAnchor, buf.CursorPos.Col)
	}
	app.executeAction(fsm.Action{Type: fsm.ActionExtendSelection, String: "down"})
	if buf.CursorPos.Line != 1 {
		t.Fatalf("expected Line=1 after extend down, got %d", buf.CursorPos.Line)
	}
}

func TestNormalizeKeyExtended(t *testing.T) {
	tests := []struct {
		msg      tea.KeyMsg
		expected string
	}{
		{tea.KeyMsg{Type: tea.KeyPgUp}, "pgup"},
		{tea.KeyMsg{Type: tea.KeyPgDown}, "pgdown"},
		{tea.KeyMsg{Type: tea.KeyHome}, "home"},
		{tea.KeyMsg{Type: tea.KeyEnd}, "end"},
		{tea.KeyMsg{Type: tea.KeyCtrlHome}, "ctrl+home"},
		{tea.KeyMsg{Type: tea.KeyCtrlEnd}, "ctrl+end"},
		{tea.KeyMsg{Type: tea.KeyCtrlLeft}, "ctrl+left"},
		{tea.KeyMsg{Type: tea.KeyCtrlRight}, "ctrl+right"},
	}

	for _, tt := range tests {
		got := normalizeKey(tt.msg)
		if got != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, got)
		}
	}
}

func TestAppDrawerOpenCloseAndMaximize(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	// Open drawer via Space o
	app.executeAction(fsm.Action{Type: fsm.ActionToggleDrawer})
	if !app.Drawer.Open {
		t.Fatal("expected drawer open after toggle")
	}
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected state DRAWER, got %v", app.FSM.State())
	}

	// Verify view rendering with drawer open
	view := app.View()
	if !strings.Contains(view, "Output & Diagnostics") {
		t.Fatal("expected view to contain drawer header")
	}

	// Maximize drawer via Z
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerMaximize})
	if !app.Drawer.Maximized {
		t.Fatal("expected drawer to be maximized")
	}
	maxView := app.View()
	if !strings.Contains(maxView, "[MAXIMIZED]") {
		t.Fatal("expected view to show [MAXIMIZED]")
	}

	// Close drawer via q/esc
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerClose})
	if app.Drawer.Open {
		t.Fatal("expected drawer to be closed")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected state NORMAL after drawer close, got %v", app.FSM.State())
	}
}

func TestAppDrawerErrorJump(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "calculator.cpp")
	_ = os.WriteFile(sourceFile, []byte("int add(int a, int b) {\n    return a + b;\n}\nint bad() {\n    return missing;\n}\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.WorkspacePath = tmpDir
	app.Width = 80
	app.Height = 24

	compilerOutput := fmt.Sprintf("%s:5:12: error: use of undeclared identifier 'missing'", sourceFile)
	app.Drawer.SetContent(compilerOutput)

	// Open drawer
	app.executeAction(fsm.Action{Type: fsm.ActionToggleDrawer})
	if !app.Drawer.Open {
		t.Fatal("expected drawer open")
	}

	// Hit enter on the error line
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerJump})

	// Verify drawer closed and editor buffer navigated to error
	if app.Drawer.Open {
		t.Fatal("expected drawer closed after jump")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected state NORMAL after jump, got %v", app.FSM.State())
	}

	currBuf := app.CurrentBuffer()
	if currBuf == nil {
		t.Fatal("expected non-nil active buffer")
	}
	if filepath.Clean(currBuf.FilePath) != filepath.Clean(sourceFile) {
		t.Fatalf("expected active buffer %q, got %q", sourceFile, currBuf.FilePath)
	}
	// Line 5 is 0-indexed line 4, Col 12 is 0-indexed col 11
	if currBuf.CursorPos.Line != 4 {
		t.Fatalf("expected line 4, got %d", currBuf.CursorPos.Line)
	}
	if currBuf.CursorPos.Col != 11 {
		t.Fatalf("expected col 11, got %d", currBuf.CursorPos.Col)
	}
}

func TestAppDiagnosticsStepping(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(sourceFile, []byte("package main\n\nfunc main() {\n    foo()\n    bar()\n}\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{sourceFile})
	app.WorkspacePath = tmpDir
	app.Width = 80
	app.Height = 24

	compilerOutput := fmt.Sprintf("%s:4:5: undefined: foo\n%s:5:5: undefined: bar\n", sourceFile, sourceFile)
	app.Drawer.SetContent(compilerOutput)

	buf := app.CurrentBuffer()
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}

	// Step next error
	app.executeAction(fsm.Action{Type: fsm.ActionNextError})
	if buf.CursorPos.Line != 3 {
		t.Fatalf("expected line 3 after next error, got %d", buf.CursorPos.Line)
	}

	// Step next error again
	app.executeAction(fsm.Action{Type: fsm.ActionNextError})
	if buf.CursorPos.Line != 4 {
		t.Fatalf("expected line 4 after second next error, got %d", buf.CursorPos.Line)
	}

	// Step prev error
	app.executeAction(fsm.Action{Type: fsm.ActionPrevError})
	if buf.CursorPos.Line != 3 {
		t.Fatalf("expected line 3 after prev error, got %d", buf.CursorPos.Line)
	}
}

func TestAppAlternateFileSwitching(t *testing.T) {
	tmpDir := t.TempDir()
	cppFile := filepath.Join(tmpDir, "widget.cpp")
	hppFile := filepath.Join(tmpDir, "widget.hpp")
	_ = os.WriteFile(cppFile, []byte("// widget implementation\n"), 0644)
	_ = os.WriteFile(hppFile, []byte("// widget header\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{cppFile})
	app.WorkspacePath = tmpDir

	if app.CurrentBuffer().FileName != "widget.cpp" {
		t.Fatalf("expected widget.cpp active, got %s", app.CurrentBuffer().FileName)
	}

	// Toggle alternate: Space a
	app.executeAction(fsm.Action{Type: fsm.ActionAlternateFile})
	if filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(hppFile) {
		t.Fatalf("expected widget.hpp active, got %s", app.CurrentBuffer().FilePath)
	}

	// Toggle back: Space a
	app.executeAction(fsm.Action{Type: fsm.ActionAlternateFile})
	if filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(cppFile) {
		t.Fatalf("expected widget.cpp active, got %s", app.CurrentBuffer().FilePath)
	}
}

func TestAppJumplistNavigation(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "file1.txt")
	f2 := filepath.Join(tmpDir, "file2.txt")
	_ = os.WriteFile(f1, []byte("line 1\nline 2\nline 3\n"), 0644)
	_ = os.WriteFile(f2, []byte("alpha\nbeta\ngamma\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{f1, f2})

	// Start at f1, line 0
	app.ActiveBuf = 0
	app.CurrentBuffer().CursorPos = buffer.Position{Line: 0, Col: 0}

	// Jump to f1, line 2
	app.Jumplist.Record(tooling.Location{FilePath: f1, Line: 0, Col: 0})
	app.CurrentBuffer().CursorPos = buffer.Position{Line: 2, Col: 0}

	// Jump to f2, line 1
	app.Jumplist.Record(tooling.Location{FilePath: f1, Line: 2, Col: 0})
	app.ActiveBuf = 1
	app.CurrentBuffer().CursorPos = buffer.Position{Line: 1, Col: 0}
	app.Jumplist.Record(tooling.Location{FilePath: f2, Line: 1, Col: 0})

	// Jump back (Ctrl+o) -> should return to f1, line 2
	app.executeAction(fsm.Action{Type: fsm.ActionJumplistBack})
	if filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(f1) || app.CurrentBuffer().CursorPos.Line != 2 {
		t.Fatalf("expected f1 at line 2 after jumplist back, got %s line %d", app.CurrentBuffer().FilePath, app.CurrentBuffer().CursorPos.Line)
	}

	// Jump back (Ctrl+o) -> should return to f1, line 0
	app.executeAction(fsm.Action{Type: fsm.ActionJumplistBack})
	if filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(f1) || app.CurrentBuffer().CursorPos.Line != 0 {
		t.Fatalf("expected f1 at line 0 after jumplist back, got %s line %d", app.CurrentBuffer().FilePath, app.CurrentBuffer().CursorPos.Line)
	}

	// Jump forward (Ctrl+i) -> should return to f1, line 2
	app.executeAction(fsm.Action{Type: fsm.ActionJumplistForward})
	if filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(f1) || app.CurrentBuffer().CursorPos.Line != 2 {
		t.Fatalf("expected f1 at line 2 after jumplist forward, got %s line %d", app.CurrentBuffer().FilePath, app.CurrentBuffer().CursorPos.Line)
	}
}

func TestAppDrawerMouseInteraction(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24

	app.Drawer.SetContent("error 1\nerror 2\nerror 3\nerror 4\nerror 5")
	app.Drawer.Open = true
	app.Drawer.Height = 6

	drawerTopRow := 1 + app.getViewportHeight()

	// 1. Mouse click on drawer top border starts drag
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      drawerTopRow,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if !app.Drawer.IsDragging {
		t.Fatal("expected drawer dragging after border click")
	}

	// 2. Mouse drag motion changes drawer height
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      drawerTopRow - 2,
		Action: tea.MouseActionMotion,
	})
	if app.Drawer.Height <= 6 {
		t.Fatalf("expected drawer height to expand after dragging upward, got %d", app.Drawer.Height)
	}

	// 3. Mouse release stops drag
	app.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
	})
	if app.Drawer.IsDragging {
		t.Fatal("expected dragging false after mouse release")
	}

	// 4. Mouse click inside drawer focuses drawer and sets cursor line
	app.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      app.Height - 3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected state DRAWER after clicking inside drawer, got %v", app.FSM.State())
	}
}

func TestMarkdownViewportConcealment(t *testing.T) {
	content := "- [ ] First task\n- [x] Done task\nCheck [link](https://example.com) and `code`"
	buf := NewBufferItem(1, "README.md", content)
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}

	theme := DefaultTheme()
	params := ViewportParams{
		Buffer:   buf,
		FSM:      fsm.NewFSM(),
		Width:    80,
		Height:   5,
		TabWidth: 4,
		Theme:    theme,
	}

	rendered := RenderViewport(params)

	// Line 0 is active cursor line -> MUST NOT be concealed
	if !strings.Contains(rendered, "- [ ] First task") {
		t.Errorf("expected cursor line 0 to have unconcealed '- [ ] First task', got:\n%s", rendered)
	}

	// Line 1 is inactive line -> MUST be concealed
	if !strings.Contains(rendered, "☑ Done task") {
		t.Errorf("expected inactive line 1 to have concealed '☑ Done task', got:\n%s", rendered)
	}

	// Line 2 is inactive line -> [link](https://...) and `code` should be concealed to 'link' and 'code'
	if strings.Contains(rendered, "[link](") || strings.Contains(rendered, "`code`") {
		t.Errorf("expected inactive line 2 markdown delimiters concealed, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Check link and code") {
		t.Errorf("expected inactive line 2 to contain 'Check link and code', got:\n%s", rendered)
	}

	// Now move cursor to Line 1
	buf.CursorPos.Line = 1
	renderedCursor1 := RenderViewport(params)

	// Now Line 0 should be concealed ("☐ First task")
	if !strings.Contains(renderedCursor1, "☐ First task") {
		t.Errorf("expected inactive line 0 to have concealed '☐ First task', got:\n%s", renderedCursor1)
	}
	// Line 1 is active -> unconcealed "- [x] Done task"
	if !strings.Contains(renderedCursor1, "- [x] Done task") {
		t.Errorf("expected active line 1 to have unconcealed '- [x] Done task', got:\n%s", renderedCursor1)
	}
}

func TestAppToggleChecklist(t *testing.T) {
	content := "- [ ] First task\n- [x] Second task\nRegular text"
	app := NewApp(nil, nil)
	buf := app.CurrentBuffer()
	buf.FilePath = "notes.md"
	buf.FileName = "notes.md"
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, content)
	buf.CursorPos = buffer.Position{Line: 0, Col: 2}

	// Toggle Line 0 (- [ ] -> - [x])
	app.executeAction(fsm.Action{Type: fsm.ActionToggleChecklist})
	line0, _ := buf.PT.Line(0)
	if line0 != "- [x] First task" {
		t.Fatalf("expected '- [x] First task', got %q", line0)
	}

	// Toggle again (- [x] -> - [ ])
	app.executeAction(fsm.Action{Type: fsm.ActionToggleChecklist})
	line0, _ = buf.PT.Line(0)
	if line0 != "- [ ] First task" {
		t.Fatalf("expected '- [ ] First task', got %q", line0)
	}

	// Test Undo
	app.executeAction(fsm.Action{Type: fsm.ActionUndo})
	line0, _ = buf.PT.Line(0)
	if line0 != "- [x] First task" {
		t.Fatalf("expected undo to restore '- [x] First task', got %q", line0)
	}

	// Move to line 2 (no checkbox) -> toggle should do nothing
	buf.CursorPos.Line = 2
	app.executeAction(fsm.Action{Type: fsm.ActionToggleChecklist})
	line2, _ := buf.PT.Line(2)
	if line2 != "Regular text" {
		t.Fatalf("expected 'Regular text' unchanged, got %q", line2)
	}
}

func TestAppMarkdownPreviewDrawer(t *testing.T) {
	mdContent := "# My Document\n\nSome body text\n\n```mermaid\nflowchart TD\n    A[Start] --> B[End]\n```"
	app := NewApp(nil, nil)
	buf := app.CurrentBuffer()
	buf.FilePath = "doc.md"
	buf.FileName = "doc.md"
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, mdContent)

	if app.Drawer.Open {
		t.Fatal("expected drawer closed initially")
	}

	// Open markdown preview
	app.executeAction(fsm.Action{Type: fsm.ActionToggleMarkdownPreview})
	if !app.Drawer.Open {
		t.Fatal("expected drawer open after toggle preview")
	}
	if app.Drawer.Mode != DrawerModeMarkdown {
		t.Fatalf("expected DrawerModeMarkdown, got %v", app.Drawer.Mode)
	}
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected StateDrawer, got %v", app.FSM.State())
	}

	// Check drawer content contains rendered markdown and mermaid box-art
	drawerText := ansi.Strip(strings.Join(app.Drawer.Content, "\n"))
	if !strings.Contains(drawerText, "My Document") {
		t.Fatalf("expected drawer text to contain 'My Document', got:\n%s", drawerText)
	}
	if !strings.Contains(drawerText, "Start") || !strings.Contains(drawerText, "End") {
		t.Fatalf("expected drawer text to contain diagram nodes Start and End, got:\n%s", drawerText)
	}

	// Close drawer with esc / ActionDrawerClose
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerClose})
	if app.Drawer.Open {
		t.Fatal("expected drawer closed")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal, got %v", app.FSM.State())
	}
}

func TestAppMermaidModeToggle(t *testing.T) {
	app := NewApp(nil, nil)
	app.MermaidMode = markdown.MermaidModeBoxArt

	// Toggle mode
	app.executeAction(fsm.Action{Type: fsm.ActionToggleMermaidMode})
	if app.MermaidMode != markdown.MermaidModeKitty {
		t.Fatalf("expected Kitty mode, got %v", app.MermaidMode)
	}
	if !strings.Contains(app.StatusMessage, "Kitty") {
		t.Fatalf("expected status message mentioning Kitty, got %q", app.StatusMessage)
	}

	// Toggle back
	app.executeAction(fsm.Action{Type: fsm.ActionToggleMermaidMode})
	if app.MermaidMode != markdown.MermaidModeBoxArt {
		t.Fatalf("expected BoxArt mode, got %v", app.MermaidMode)
	}
	if !strings.Contains(app.StatusMessage, "box-art") {
		t.Fatalf("expected status message mentioning box-art, got %q", app.StatusMessage)
	}
}

func TestAppMarkdownPreviewAutoRefresh(t *testing.T) {
	app := NewApp(nil, nil)
	buf := app.CurrentBuffer()
	buf.FilePath = "doc.md"
	buf.FileName = "doc.md"
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, "# Initial Title\n")

	// Open preview
	app.executeAction(fsm.Action{Type: fsm.ActionToggleMarkdownPreview})
	initialDrawerText := ansi.Strip(strings.Join(app.Drawer.Content, "\n"))
	if !strings.Contains(initialDrawerText, "Initial Title") {
		t.Fatalf("expected 'Initial Title' in preview, got:\n%s", initialDrawerText)
	}

	// Insert text into buffer
	buf.CursorPos = buffer.Position{Line: 0, Col: 15}
	app.FSM.SetState(fsm.StateInsert)
	app.executeAction(fsm.Action{Type: fsm.ActionInsertChar, String: " Updated"})

	// Verify preview drawer refreshed automatically
	refreshedDrawerText := ansi.Strip(strings.Join(app.Drawer.Content, "\n"))
	if !strings.Contains(refreshedDrawerText, "Updated") {
		t.Fatalf("expected 'Updated' in preview after buffer edit, got:\n%s", refreshedDrawerText)
	}
}

func TestAppIPCIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "server.go")
	_ = os.WriteFile(sourceFile, []byte("package main\n\nfunc start() {\n    // server\n}\n"), 0644)

	sockPath := filepath.Join(tmpDir, "koodi-test.sock")
	app := NewApp(nil, nil)
	app.WorkspacePath = tmpDir

	if err := app.StartIPCServer(sockPath); err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer app.Close()

	client, err := ipc.NewClient(sockPath)
	if err != nil {
		t.Fatalf("failed to connect IPC client: %v", err)
	}
	defer client.Close()

	// 1. Test Open
	if err := client.Open(sourceFile, 3, 5); err != nil {
		t.Fatalf("client.Open failed: %v", err)
	}
	currBuf := app.CurrentBuffer()
	if currBuf == nil || filepath.Clean(currBuf.FilePath) != filepath.Clean(sourceFile) {
		t.Fatalf("expected active buffer %q, got %v", sourceFile, currBuf)
	}
	if currBuf.CursorPos.Line != 2 { // 0-indexed
		t.Fatalf("expected line 2, got %d", currBuf.CursorPos.Line)
	}
	if currBuf.CursorPos.Col != 4 { // 0-indexed
		t.Fatalf("expected col 4, got %d", currBuf.CursorPos.Col)
	}

	// 2. Test Notify
	if err := client.Notify("Task completed successfully", "info"); err != nil {
		t.Fatalf("client.Notify failed: %v", err)
	}
	if !strings.Contains(app.StatusMessage, "Task completed successfully") {
		t.Fatalf("expected status message updated, got %q", app.StatusMessage)
	}

	// 3. Test StatusBadge
	if err := client.StatusBadge("[Agent: Refactoring]", true); err != nil {
		t.Fatalf("client.StatusBadge failed: %v", err)
	}
	if app.AgentStatus != "[Agent: Refactoring]" || !app.AgentBusy {
		t.Fatalf("expected AgentStatus and AgentBusy set, got %q, %v", app.AgentStatus, app.AgentBusy)
	}

	// 4. Test Action
	if err := client.Action("drawer:toggle", nil); err != nil {
		t.Fatalf("client.Action failed: %v", err)
	}
	if !app.Drawer.Open {
		t.Fatal("expected drawer open after action")
	}

	// 5. Test Diff
	_ = os.WriteFile(sourceFile, []byte("package main\n\nfunc start() {\n    // server modified\n}\n"), 0644)
	if err := client.Diff(sourceFile); err != nil {
		t.Fatalf("client.Diff failed: %v", err)
	}
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeDiffReview {
		t.Fatalf("expected drawer open in DiffReview mode, got open=%v, mode=%v", app.Drawer.Open, app.Drawer.Mode)
	}
	drawerText := strings.Join(app.Drawer.Content, "\n")
	if !strings.Contains(drawerText, "+     // server modified") {
		t.Fatalf("expected diff to contain added line, got:\n%s", drawerText)
	}

	// 6. Test Reload
	if err := client.Reload(true, ""); err != nil {
		t.Fatalf("client.Reload failed: %v", err)
	}
	if !strings.Contains(currBuf.PT.Text(), "server modified") {
		t.Fatalf("expected buffer reloaded with disk content, got %q", currBuf.PT.Text())
	}
}

func TestAppWatcherReconciliationAndCollisions(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "clean.go")
	file2 := filepath.Join(tmpDir, "dirty.go")

	_ = os.WriteFile(file1, []byte("original clean content\n"), 0644)
	_ = os.WriteFile(file2, []byte("original dirty content\n"), 0644)

	app := NewApp(nil, []string{file1, file2})
	app.WorkspacePath = tmpDir

	// Mark buffer 1 clean, buffer 2 dirty
	app.Buffers[0].Dirty = false
	app.Buffers[1].Dirty = true
	_ = app.Buffers[1].PT.Insert(0, "// local edit\n")

	// Modify both files on disk
	_ = os.WriteFile(file1, []byte("disk clean modified\n"), 0644)
	_ = os.WriteFile(file2, []byte("disk dirty modified\n"), 0644)

	// Dispatch simulated watcher event
	ev := watcher.ChangeEvent{
		Changes: []watcher.FileChangeInfo{
			{Path: file1},
			{Path: file2},
		},
	}
	app.handleWatcherEvent(ev)

	// Clean buffer 0 should be reloaded automatically
	if app.Buffers[0].PT.Text() != "disk clean modified\n" {
		t.Fatalf("expected buffer 0 reloaded with disk content, got %q", app.Buffers[0].PT.Text())
	}
	if app.Buffers[0].Dirty {
		t.Fatal("expected buffer 0 to remain clean after hot reload")
	}

	// Dirty buffer 1 should NOT be overwritten (collision preserved)
	if !strings.Contains(app.Buffers[1].PT.Text(), "// local edit") {
		t.Fatalf("expected buffer 1 local edits preserved, got %q", app.Buffers[1].PT.Text())
	}

	// Agent transaction should contain recorded changes
	if app.AgentTx == nil || app.AgentTx.FileCount() < 2 {
		t.Fatalf("expected at least 2 files in agent transaction, got %v", app.AgentTx)
	}
}

func TestAppAgentReviewDrawerAndAcceptRevert(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.go")
	f2 := filepath.Join(tmpDir, "f2.go")

	_ = os.WriteFile(f1, []byte("line 1\n"), 0644)
	_ = os.WriteFile(f2, []byte("func a() {}\n"), 0644)

	app := NewApp(nil, []string{f1, f2})
	app.WorkspacePath = tmpDir

	// Simulate agent transaction modifications
	_ = os.WriteFile(f1, []byte("line 1 modified\nline 2\n"), 0644)
	_ = os.WriteFile(f2, []byte("func a() { return }\n"), 0644)

	app.AgentTx.AddChange(f1, "line 1\n", "line 1 modified\nline 2\n")
	app.AgentTx.AddChange(f2, "func a() {}\n", "func a() { return }\n")

	// Open Agent Review drawer (Space d s)
	app.executeAction(fsm.Action{Type: fsm.ActionAgentReviewOpen})
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeDiffReview {
		t.Fatalf("expected drawer open in DiffReview mode, open=%v, mode=%v", app.Drawer.Open, app.Drawer.Mode)
	}
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected StateDrawer, got %v", app.FSM.State())
	}

	content := strings.Join(app.Drawer.Content, "\n")
	if !strings.Contains(content, "[M]") {
		t.Fatalf("expected [M] status in review drawer, got:\n%s", content)
	}

	// Step forward / backward in drawer (ActionStepForward / ActionStepBackward)
	app.executeAction(fsm.Action{Type: fsm.ActionStepForward})
	if app.AgentTx.Selected != 1 {
		t.Fatalf("expected selected index 1 after step forward, got %d", app.AgentTx.Selected)
	}

	// Test Revert All (Space d r)
	app.executeAction(fsm.Action{Type: fsm.ActionAgentRevertAll})
	if app.AgentTx.FileCount() != 0 {
		t.Fatalf("expected 0 files in transaction after revert, got %d", app.AgentTx.FileCount())
	}
	if app.Drawer.Open {
		t.Fatal("expected drawer closed after revert")
	}

	// Verify files reverted on disk and in buffers
	data1, _ := os.ReadFile(f1)
	if string(data1) != "line 1\n" {
		t.Fatalf("expected f1 reverted to 'line 1\\n', got %q", string(data1))
	}
	if app.Buffers[0].PT.Text() != "line 1\n" {
		t.Fatalf("expected buffer 0 reloaded with reverted content, got %q", app.Buffers[0].PT.Text())
	}

	// Add change again and test Accept All (Space d a)
	app.AgentTx.AddChange(f1, "line 1\n", "new accepted content\n")
	app.executeAction(fsm.Action{Type: fsm.ActionAgentReviewOpen})
	app.executeAction(fsm.Action{Type: fsm.ActionAgentAcceptAll})
	if app.AgentTx.FileCount() != 0 {
		t.Fatalf("expected 0 files after accept all, got %d", app.AgentTx.FileCount())
	}
	if app.Drawer.Open {
		t.Fatal("expected drawer closed after accept")
	}
	if !strings.Contains(app.StatusMessage, "accepted") {
		t.Fatalf("expected status message mentioning accepted, got %q", app.StatusMessage)
	}
}

func TestAppFileTreeToggleAndLayout(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello world\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24
	app.WorkspacePath = tmpDir
	app.FileTree = NewFileTree(tmpDir, 26)

	if app.FileTree.Open {
		t.Fatal("expected filetree initially closed")
	}
	if app.getViewportWidth() != 80 {
		t.Fatalf("expected viewport width 80 when tree is closed, got %d", app.getViewportWidth())
	}

	// Toggle open (Space e)
	app.executeAction(fsm.Action{Type: fsm.ActionToggleFileTree})
	if !app.FileTree.Open {
		t.Fatal("expected filetree open after ActionToggleFileTree")
	}

	expectedVpWidth := 80 - app.FileTree.Width
	if app.getViewportWidth() != expectedVpWidth {
		t.Fatalf("expected viewport width %d, got %d", expectedVpWidth, app.getViewportWidth())
	}

	// Render view with tree open
	rendered := app.View()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected 24 rendered lines, got %d", len(lines))
	}

	// Check middle lines contain the separator │
	foundSeparator := false
	for i := 1; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], "│") {
			foundSeparator = true
			break
		}
	}
	if !foundSeparator {
		t.Fatal("expected tree separator border │ in rendered middle lines")
	}

	// Grow tree (Space w >)
	wBefore := app.FileTree.Width
	app.executeAction(fsm.Action{Type: fsm.ActionResizeTreeGrow})
	if app.FileTree.Width != wBefore+3 {
		t.Fatalf("expected tree width %d, got %d", wBefore+3, app.FileTree.Width)
	}

	// Shrink tree (Space w <)
	app.executeAction(fsm.Action{Type: fsm.ActionResizeTreeShrink})
	if app.FileTree.Width != wBefore {
		t.Fatalf("expected tree width restored to %d, got %d", wBefore, app.FileTree.Width)
	}

	// Toggle closed
	app.executeAction(fsm.Action{Type: fsm.ActionToggleFileTree})
	if app.FileTree.Open {
		t.Fatal("expected filetree closed after second ActionToggleFileTree")
	}
}

func TestAppFileTreeMouseInteraction(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	_ = os.MkdirAll(subDir, 0755)
	fileA := filepath.Join(subDir, "test.go")
	_ = os.WriteFile(fileA, []byte("package sub\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24
	app.WorkspacePath = tmpDir
	app.FileTree = NewFileTree(tmpDir, 26)
	app.FileTree.Open = true

	// 1. Mouse Drag on border (X == 25)
	app.handleMouse(tea.MouseMsg{
		X:      25,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if !app.FileTree.IsDragging {
		t.Fatal("expected FileTree.IsDragging == true after press on border")
	}

	// Motion drag to X = 32
	app.handleMouse(tea.MouseMsg{
		X:      32,
		Y:      5,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
	})
	if app.FileTree.Width != 33 {
		t.Fatalf("expected FileTree.Width 33 after motion, got %d", app.FileTree.Width)
	}

	// Release
	app.handleMouse(tea.MouseMsg{
		X:      32,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	if app.FileTree.IsDragging {
		t.Fatal("expected FileTree.IsDragging == false after release")
	}

	// 2. Click on directory in tree: toggle expand
	// Find sub directory in visible nodes
	subRow := -1
	for i, n := range app.FileTree.VisibleNodes {
		if n.Name == "sub" {
			subRow = i
			break
		}
	}
	if subRow == -1 {
		t.Fatalf("could not find 'sub' in visible nodes: %v", app.FileTree.VisibleNodes)
	}

	// Click on row (viewportTop = 1, so Y = subRow + 1)
	app.handleMouse(tea.MouseMsg{
		X:      5,
		Y:      subRow + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})

	// After clicking, 'sub' should be expanded and test.go visible
	foundFile := false
	for _, n := range app.FileTree.VisibleNodes {
		if n.Name == "test.go" {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Fatal("expected test.go visible after expanding sub directory via mouse click")
	}

	// 3. Click on test.go: should open buffer
	testRow := -1
	for i, n := range app.FileTree.VisibleNodes {
		if n.Name == "test.go" {
			testRow = i
			break
		}
	}
	if testRow == -1 {
		t.Fatal("could not find test.go row")
	}

	app.handleMouse(tea.MouseMsg{
		X:      5,
		Y:      testRow + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})

	curr := app.CurrentBuffer()
	if curr == nil || !strings.Contains(curr.FilePath, "test.go") {
		t.Fatalf("expected current buffer to be test.go after clicking in filetree, got %v", curr)
	}
}

func TestAppPickerFiles(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "alpha.go")
	f2 := filepath.Join(tmpDir, "beta.go")
	_ = os.WriteFile(f1, []byte("package alpha\n"), 0644)
	_ = os.WriteFile(f2, []byte("package beta\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24
	app.WorkspacePath = tmpDir
	app.Picker = NewPicker(tmpDir)

	// Open files picker (Space f f)
	app.executeAction(fsm.Action{Type: fsm.ActionPickerOpenFiles})
	if !app.Picker.Open {
		t.Fatal("expected picker to be open")
	}
	if app.FSM.State() != fsm.StatePicker {
		t.Fatalf("expected StatePicker, got %v", app.FSM.State())
	}

	// Verify modal rendered in View()
	viewStr := app.View()
	rawView := ansi.Strip(viewStr)
	if !strings.Contains(rawView, "Files") {
		t.Fatalf("expected modal title 'Files' in View(), got:\n%s", rawView)
	}

	// Type query "beta"
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "b"})
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "e"})
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "t"})
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "a"})

	if len(app.Picker.Filtered) != 1 {
		t.Fatalf("expected exactly 1 filtered match for 'beta', got %d", len(app.Picker.Filtered))
	}

	// Select item (Enter)
	app.executeAction(fsm.Action{Type: fsm.ActionPickerSelect})
	if app.Picker.Open {
		t.Fatal("expected picker to close after selection")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after selection, got %v", app.FSM.State())
	}

	currBuf := app.CurrentBuffer()
	if currBuf == nil || !strings.Contains(currBuf.FilePath, "beta.go") {
		t.Fatalf("expected beta.go open in active buffer, got %v", currBuf)
	}
}

func TestAppPickerBuffers(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"alpha.go", "beta.go", "gamma.go"})
	app.Width = 80
	app.Height = 24

	// Initially active buffer is 0 (alpha.go)
	if app.ActiveBuf != 0 {
		t.Fatalf("expected ActiveBuf 0, got %d", app.ActiveBuf)
	}

	// Open buffer picker (Space b b)
	app.executeAction(fsm.Action{Type: fsm.ActionPickerOpenBuffers})
	if !app.Picker.Open || app.Picker.Mode != PickerModeBuffers {
		t.Fatalf("expected picker open in Buffers mode, open=%v, mode=%v", app.Picker.Open, app.Picker.Mode)
	}

	// Filter for "gamma"
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "g"})
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "a"})
	app.executeAction(fsm.Action{Type: fsm.ActionPickerInput, String: "m"})

	// Select
	app.executeAction(fsm.Action{Type: fsm.ActionPickerSelect})
	if app.Picker.Open {
		t.Fatal("expected picker closed after selection")
	}
	if app.CurrentBuffer().FileName != "gamma.go" {
		t.Fatalf("expected active buffer gamma.go, got %s", app.CurrentBuffer().FileName)
	}
}

func TestAppPickerMouseInteraction(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"file1.go", "file2.go"})
	app.Width = 80
	app.Height = 24
	app.executeAction(fsm.Action{Type: fsm.ActionPickerOpenBuffers})

	// Modal bounds: W = 80, H = 24
	// modalW = 80 * 66 / 100 = 52. leftX = (80 - 52)/2 = 14. rightX = 14 + 52 - 1 = 65.
	// modalH = 24 * 66 / 100 = 15. topY = (24 - 15)/2 = 4. bottomY = 4 + 15 - 1 = 18.
	// Click outside modal: X = 2, Y = 2
	app.handleMouse(tea.MouseMsg{
		X:      2,
		Y:      2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if app.Picker.Open {
		t.Fatal("expected click outside picker modal to close it")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal, got %v", app.FSM.State())
	}

	// Reopen and click on candidate item (topY + 3 = 7)
	app.executeAction(fsm.Action{Type: fsm.ActionPickerOpenBuffers})
	app.handleMouse(tea.MouseMsg{
		X:      30,
		Y:      7, // candidate item row 0
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if app.Picker.Open {
		t.Fatal("expected click on candidate item in picker to select and close")
	}
}

func TestAppFileTreeKeyboardNavigation(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "pkg")
	_ = os.MkdirAll(subDir, 0755)
	fileA := filepath.Join(subDir, "alpha.go")
	_ = os.WriteFile(fileA, []byte("package pkg\n"), 0644)
	fileB := filepath.Join(tmpDir, "root.go")
	_ = os.WriteFile(fileB, []byte("package main\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 80
	app.Height = 24
	app.WorkspacePath = tmpDir
	app.FileTree = NewFileTree(tmpDir, 26)

	// 1. Open tree via Space e -> sets StateTree
	app.executeAction(fsm.Action{Type: fsm.ActionToggleFileTree})
	if !app.FileTree.Open || app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected tree open and StateTree, open=%v state=%v", app.FileTree.Open, app.FSM.State())
	}

	// 2. Cursor navigation: move down
	initCursor := app.FileTree.CursorIndex
	app.executeAction(fsm.Action{Type: fsm.ActionTreeDown})
	if app.FileTree.CursorIndex != initCursor+1 {
		t.Fatalf("expected cursor moved down to %d, got %d", initCursor+1, app.FileTree.CursorIndex)
	}

	app.executeAction(fsm.Action{Type: fsm.ActionTreeUp})
	if app.FileTree.CursorIndex != initCursor {
		t.Fatalf("expected cursor moved back up to %d, got %d", initCursor, app.FileTree.CursorIndex)
	}

	// 3. Find pkg directory and expand it using ActionTreeToggleOrOpen
	for i, n := range app.FileTree.VisibleNodes {
		if n.Name == "pkg" {
			app.FileTree.CursorIndex = i
			break
		}
	}
	app.executeAction(fsm.Action{Type: fsm.ActionTreeToggleOrOpen})
	if !app.FileTree.SelectedNode().Expanded {
		t.Fatal("expected pkg to expand on ActionTreeToggleOrOpen")
	}
	if app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected still in StateTree after expanding directory, got %v", app.FSM.State())
	}

	// 4. Move to alpha.go file inside pkg and open it
	for i, n := range app.FileTree.VisibleNodes {
		if n.Name == "alpha.go" {
			app.FileTree.CursorIndex = i
			break
		}
	}
	app.executeAction(fsm.Action{Type: fsm.ActionTreeToggleOrOpen})
	// Opening a file should switch active buffer AND transition state to StateNormal
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after opening file, got %v", app.FSM.State())
	}
	if app.CurrentBuffer() == nil || !strings.Contains(app.CurrentBuffer().FilePath, "alpha.go") {
		t.Fatalf("expected active buffer to be alpha.go, got %v", app.CurrentBuffer())
	}

	// 5. While in StateNormal, pressing Space e re-focuses tree into StateTree without closing it
	app.executeAction(fsm.Action{Type: fsm.ActionToggleFileTree})
	if !app.FileTree.Open || app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected tree open and focused in StateTree, open=%v state=%v", app.FileTree.Open, app.FSM.State())
	}

	// 6. Test collapse using ActionTreeCollapseOrParent
	for i, n := range app.FileTree.VisibleNodes {
		if n.Name == "pkg" {
			app.FileTree.CursorIndex = i
			break
		}
	}
	app.executeAction(fsm.Action{Type: fsm.ActionTreeCollapseOrParent})
	if app.FileTree.SelectedNode().Expanded {
		t.Fatal("expected pkg directory collapsed after ActionTreeCollapseOrParent")
	}

	// 7. Test Esc/q (ActionPaneFocusBuffer) returns focus to buffer leaving tree open
	app.executeAction(fsm.Action{Type: fsm.ActionPaneFocusBuffer})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after focus buffer, got %v", app.FSM.State())
	}
	if !app.FileTree.Open {
		t.Fatal("expected tree to remain open when focus returns to buffer")
	}

	// 8. Test top and bottom navigation
	app.executeAction(fsm.Action{Type: fsm.ActionPaneFocusTree})
	app.executeAction(fsm.Action{Type: fsm.ActionTreeBottom})
	if app.FileTree.CursorIndex != len(app.FileTree.VisibleNodes)-1 {
		t.Fatalf("expected cursor at bottom, got %d", app.FileTree.CursorIndex)
	}
	app.executeAction(fsm.Action{Type: fsm.ActionTreeTop})
	if app.FileTree.CursorIndex != 0 {
		t.Fatalf("expected cursor at top, got %d", app.FileTree.CursorIndex)
	}
}

func TestAppPaneFocusCycling(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"main.go"})
	app.Width = 80
	app.Height = 24

	// Open tree and drawer
	app.FileTree.Open = true
	app.Drawer.Open = true
	app.Drawer.Mode = DrawerModeOutput

	// Initially in StateNormal (Buffer focused)
	app.FSM.SetState(fsm.StateNormal)

	// Cycle 1: Normal -> Drawer
	app.executeAction(fsm.Action{Type: fsm.ActionPaneCycle})
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected StateDrawer after cycle 1, got %v", app.FSM.State())
	}

	// Cycle 2: Drawer -> Tree
	app.executeAction(fsm.Action{Type: fsm.ActionPaneCycle})
	if app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected StateTree after cycle 2, got %v", app.FSM.State())
	}

	// Cycle 3: Tree -> Normal
	app.executeAction(fsm.Action{Type: fsm.ActionPaneCycle})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal after cycle 3, got %v", app.FSM.State())
	}

	// Direct directional focus actions
	app.executeAction(fsm.Action{Type: fsm.ActionPaneFocusTree})
	if app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected StateTree, got %v", app.FSM.State())
	}

	app.executeAction(fsm.Action{Type: fsm.ActionPaneFocusDrawer})
	if app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected StateDrawer, got %v", app.FSM.State())
	}

	app.executeAction(fsm.Action{Type: fsm.ActionPaneFocusBuffer})
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal, got %v", app.FSM.State())
	}
}

func TestCoordinateProjectionAndTabHighlighting(t *testing.T) {
	tabWidth := 4

	// 1. Test ByteColToVisualCol with tabs
	raw := "\treturn a + b"
	if vis := ByteColToVisualCol(raw, 0, tabWidth); vis != 0 {
		t.Errorf("expected byte 0 to map to vis 0, got %d", vis)
	}
	if vis := ByteColToVisualCol(raw, 1, tabWidth); vis != 4 {
		t.Errorf("expected byte 1 to map to vis 4, got %d", vis)
	}
	if vis := ByteColToVisualCol(raw, 7, tabWidth); vis != 10 {
		t.Errorf("expected byte 7 to map to vis 10, got %d", vis)
	}

	// 2. Test with multibyte Finnish characters: 'ä' is 2 bytes
	rawMulti := "ä := 1"
	if vis := ByteColToVisualCol(rawMulti, 0, tabWidth); vis != 0 {
		t.Errorf("expected byte 0 to map to vis 0, got %d", vis)
	}
	if vis := ByteColToVisualCol(rawMulti, 2, tabWidth); vis != 1 {
		t.Errorf("expected byte 2 to map to vis 1, got %d", vis)
	}
	if vis := ByteColToVisualCol(rawMulti, 3, tabWidth); vis != 2 {
		t.Errorf("expected byte 3 to map to vis 2, got %d", vis)
	}

	// 3. Test RuneColToVisualCol
	if vis := RuneColToVisualCol(raw, 0, tabWidth); vis != 0 {
		t.Errorf("expected rune 0 to map to vis 0, got %d", vis)
	}
	if vis := RuneColToVisualCol(raw, 1, tabWidth); vis != 4 {
		t.Errorf("expected rune 1 to map to vis 4, got %d", vis)
	}
}

func TestTabIndentedViewportHighlighting(t *testing.T) {
	theme := NewVSCodeTheme()
	code := "\tfunc add(a int) {\n\t\tfmt.Println(\"ok\")\n\t}\n"
	buf := NewBufferItem(1, "main.go", code)

	lang, _ := treesitter.DetectLanguage("main.go")
	buf.Highlighter.Parse([]byte(code), lang)

	// Render line 0: "\tfunc add(a int) {"
	rendered0 := renderLineWithCursor("    func add(a int) {", "\tfunc add(a int) {", 4, 0, buf, fsm.StateNormal, 80, theme, nil)

	// Verify that 'func' is styled consistently with SynDeclaration
	fStyled := theme.SynDeclaration.Render("f")
	uStyled := theme.SynDeclaration.Render("u")
	if !strings.Contains(rendered0, fStyled) || !strings.Contains(rendered0, uStyled) {
		t.Fatalf("expected 'func' to be styled with SynDeclaration without being split in the middle")
	}

	// Verify that function name 'add' is styled with SynFunction (yellow)
	addStyled := theme.SynFunction.Render("a")
	if !strings.Contains(rendered0, addStyled) {
		t.Fatalf("expected 'add' to be styled with SynFunction")
	}

	// Verify that indent spaces (cols 0..3) have TokenNormal and are not covered by any keyword span
	spans0 := buf.Highlighter.GetLineHighlights(0)
	for _, s := range spans0 {
		vStart := ByteColToVisualCol("\tfunc add(a int) {", s.StartCol, 4)
		vEnd := ByteColToVisualCol("\tfunc add(a int) {", s.EndCol, 4)
		if vStart < 4 {
			t.Fatalf("expected all spans on indented line to start >= 4, got vStart %d for type %v", vStart, s.Type)
		}
		if s.Type == treesitter.TokenDeclaration {
			if vStart != 4 || vEnd != 8 {
				t.Fatalf("expected 'func' span to be [4, 8), got [%d, %d)", vStart, vEnd)
			}
		}
		if s.Type == treesitter.TokenFunction {
			if vStart != 9 || vEnd != 12 {
				t.Fatalf("expected 'add' function span to be [9, 12), got [%d, %d)", vStart, vEnd)
			}
		}
	}
}

func TestAppSearchPromptAndMatchStepping(t *testing.T) {
	app := NewApp(nil, nil)
	content := "func helloWorld() {\n\tprintln(\"hello world\")\n\treturn hello\n}\n"
	app.Buffers = []*BufferItem{NewBufferItem(1, "main.go", content)}
	app.ActiveBuf = 0
	app.Width = 80
	app.Height = 24

	// 1. Open search prompt via '/'
	act := app.FSM.HandleKey("/")
	if act.Type != fsm.ActionSearchOpen || app.FSM.State() != fsm.StateSearch {
		t.Fatalf("expected ActionSearchOpen and StateSearch, got %v (%v)", act.Type, app.FSM.State())
	}
	app.executeAction(act)
	if !app.SearchActive || app.SearchQuery != "" {
		t.Fatalf("expected SearchActive=true and empty SearchQuery, got %v, %q", app.SearchActive, app.SearchQuery)
	}

	// 2. Type "hello" into search prompt
	for _, ch := range "hello" {
		kAct := app.FSM.HandleKey(string(ch))
		if kAct.Type != fsm.ActionSearchInput {
			t.Fatalf("expected ActionSearchInput for %q, got %v", string(ch), kAct.Type)
		}
		app.executeAction(kAct)
	}
	if app.SearchQuery != "hello" {
		t.Fatalf("expected SearchQuery='hello', got %q", app.SearchQuery)
	}
	if len(app.SearchMatches) != 3 {
		t.Fatalf("expected 3 matches for 'hello', got %d", len(app.SearchMatches))
	}

	// 3. Confirm search with enter
	cAct := app.FSM.HandleKey("enter")
	if cAct.Type != fsm.ActionSearchConfirm || app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected ActionSearchConfirm and StateNormal, got %v (%v)", cAct.Type, app.FSM.State())
	}
	app.executeAction(cAct)

	// 4. Verify View() statusline contains match info
	renderedView := app.View()
	if !strings.Contains(renderedView, "matches") {
		t.Fatalf("expected View() to contain match indicator, got:\n%s", renderedView)
	}

	// 5. Step forward with 'n'
	initialIdx := app.SearchIndex
	nAct := app.FSM.HandleKey("n")
	if nAct.Type != fsm.ActionSearchNext {
		t.Fatalf("expected ActionSearchNext for 'n', got %v", nAct.Type)
	}
	app.executeAction(nAct)
	if app.SearchIndex != (initialIdx+1)%3 {
		t.Fatalf("expected SearchIndex to advance to %d, got %d", (initialIdx+1)%3, app.SearchIndex)
	}

	// 6. Step backward with 'N'
	prevAct := app.FSM.HandleKey("N")
	if prevAct.Type != fsm.ActionSearchPrev {
		t.Fatalf("expected ActionSearchPrev for 'N', got %v", prevAct.Type)
	}
	app.executeAction(prevAct)
	if app.SearchIndex != initialIdx {
		t.Fatalf("expected SearchIndex to return to %d, got %d", initialIdx, app.SearchIndex)
	}

	// 7. Clear highlights via Space c h
	app.FSM.HandleKey("Space")
	app.FSM.HandleKey("c")
	clearAct := app.FSM.HandleKey("h")
	if clearAct.Type != fsm.ActionSearchClearHighlight {
		t.Fatalf("expected ActionSearchClearHighlight, got %v", clearAct.Type)
	}
	app.executeAction(clearAct)
	if app.SearchActive || len(app.SearchMatches) > 0 {
		t.Fatalf("expected search highlights to be cleared, got active=%v, matches=%d", app.SearchActive, len(app.SearchMatches))
	}
}

func TestAppSearchReplaceDrawer(t *testing.T) {
	app := NewApp(nil, nil)
	content := "alpha beta alpha gamma\nalpha delta\n"
	app.Buffers = []*BufferItem{NewBufferItem(1, "test.txt", content)}
	app.ActiveBuf = 0
	app.Width = 80
	app.Height = 24

	// 1. Open Search & Replace drawer via Space s r
	app.FSM.HandleKey("Space")
	app.FSM.HandleKey("s")
	openAct := app.FSM.HandleKey("r")
	if openAct.Type != fsm.ActionReplaceOpen || app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected ActionReplaceOpen and StateDrawer, got %v (%v)", openAct.Type, app.FSM.State())
	}
	app.executeAction(openAct)

	if app.Drawer == nil || !app.Drawer.Open || app.Drawer.Mode != DrawerModeSearchReplace {
		t.Fatalf("expected Drawer to be open in DrawerModeSearchReplace")
	}

	// 2. Set Find and Replace text
	app.SearchQuery = "alpha"
	app.ReplaceText = "omega"
	app.refreshSearchReplace()

	// Verify matches found
	if len(app.SearchMatches) != 3 {
		t.Fatalf("expected 3 matches for 'alpha', got %d", len(app.SearchMatches))
	}

	// Verify drawer content shows Find, Replace, Flags, Matches, and preview diffs
	drawerContent := strings.Join(app.Drawer.Content, "\n")
	if !strings.Contains(drawerContent, "Find:    alpha") || !strings.Contains(drawerContent, "Replace: omega") {
		t.Fatalf("expected drawer to show Find and Replace text, got:\n%s", drawerContent)
	}
	if !strings.Contains(drawerContent, "Matches: 3 match(es)") {
		t.Fatalf("expected drawer to show 3 matches, got:\n%s", drawerContent)
	}
	if !strings.Contains(drawerContent, "- Line 1:") || !strings.Contains(drawerContent, "+ Line 1:") {
		t.Fatalf("expected preview diff lines in drawer, got:\n%s", drawerContent)
	}

	// 3. Apply Replace Current (alt+y)
	currAct := app.FSM.HandleKey("alt+y")
	if currAct.Type != fsm.ActionReplaceApplyCurrent {
		t.Fatalf("expected ActionReplaceApplyCurrent for alt+y, got %v", currAct.Type)
	}
	app.executeAction(currAct)

	bufText := app.CurrentBuffer().PT.Text()
	if !strings.HasPrefix(bufText, "omega beta alpha gamma") {
		t.Fatalf("expected first occurrence replaced with 'omega', got %q", bufText)
	}

	// 4. Apply Replace All (enter)
	allAct := app.FSM.HandleKey("enter")
	if allAct.Type != fsm.ActionReplaceApplyAll {
		t.Fatalf("expected ActionReplaceApplyAll for enter, got %v", allAct.Type)
	}
	app.executeAction(allAct)

	bufTextAfter := app.CurrentBuffer().PT.Text()
	expectedText := "omega beta omega gamma\nomega delta\n"
	if bufTextAfter != expectedText {
		t.Fatalf("expected all 'alpha' replaced with 'omega', got %q, expected %q", bufTextAfter, expectedText)
	}

	// 5. Test Undo restores buffer
	undoAct := fsm.Action{Type: fsm.ActionUndo}
	app.executeAction(undoAct)
	// Since replace all was an atomic transaction, undo restores the state before replace all
	if app.CurrentBuffer().PT.Text() != "omega beta alpha gamma\nalpha delta\n" {
		t.Fatalf("expected single undo to restore state before replace all, got %q", app.CurrentBuffer().PT.Text())
	}

	// Close drawer with Esc
	closeAct := app.FSM.HandleKey("esc")
	if closeAct.Type != fsm.ActionDrawerClose || app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected ActionDrawerClose and StateNormal, got %v (%v)", closeAct.Type, app.FSM.State())
	}
	app.executeAction(closeAct)
	if app.Drawer.Open {
		t.Fatalf("expected drawer to be closed after esc")
	}
}

func TestAppProjectSearchReplace(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "file1.txt")
	f2 := filepath.Join(tempDir, "file2.txt")
	_ = os.WriteFile(f1, []byte("target item here\nsecond target\n"), 0644)
	_ = os.WriteFile(f2, []byte("another target item\n"), 0644)

	app := NewApp(nil, nil)
	app.WorkspacePath = tempDir
	app.Width = 80
	app.Height = 24

	// Open project search & replace via Space s p
	app.FSM.HandleKey("Space")
	app.FSM.HandleKey("s")
	openAct := app.FSM.HandleKey("p")
	if openAct.Type != fsm.ActionProjectReplaceOpen || app.FSM.State() != fsm.StateDrawer {
		t.Fatalf("expected ActionProjectReplaceOpen and StateDrawer, got %v (%v)", openAct.Type, app.FSM.State())
	}
	app.executeAction(openAct)

	if app.ReplaceScope != "project" {
		t.Fatalf("expected ReplaceScope='project', got %q", app.ReplaceScope)
	}

	app.SearchQuery = "target"
	app.ReplaceText = "replacement"
	app.refreshSearchReplace()

	if len(app.SearchMatches) != 3 {
		t.Fatalf("expected 3 matches across project, got %d", len(app.SearchMatches))
	}

	// Apply Replace All
	app.executeAction(fsm.Action{Type: fsm.ActionReplaceApplyAll})

	// Verify on disk
	d1, _ := os.ReadFile(f1)
	expected1 := "replacement item here\nsecond replacement\n"
	if string(d1) != expected1 {
		t.Fatalf("expected %q, got %q", expected1, string(d1))
	}

	d2, _ := os.ReadFile(f2)
	expected2 := "another replacement item\n"
	if string(d2) != expected2 {
		t.Fatalf("expected %q, got %q", expected2, string(d2))
	}
}

func TestAppHelpModalIntegration(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Width = 100
	app.Height = 30

	// 1. Toggle Help Modal open
	app.executeAction(fsm.Action{Type: fsm.ActionHelpToggle})
	if !app.HelpModal.Open {
		t.Fatal("expected HelpModal to be open after ActionHelpToggle")
	}
	if app.FSM.State() != fsm.StateHelp {
		t.Fatalf("expected StateHelp, got %v", app.FSM.State())
	}

	// Verify modal overlay rendered in View()
	viewStr := app.View()
	rawView := ansi.Strip(viewStr)
	if !strings.Contains(rawView, "Help & Keyboard Reference") {
		t.Fatalf("expected Help title in View(), got:\n%s", rawView)
	}

	// 2. Filter by "tree"
	for _, ch := range "tree" {
		app.executeAction(fsm.Action{Type: fsm.ActionHelpInput, String: string(ch)})
	}
	if len(app.HelpModal.Filtered) == 0 {
		t.Fatal("expected filtered results for 'tree'")
	}

	// 3. Select item (Enter) - should execute the action (Toggle Tree)
	app.executeAction(fsm.Action{Type: fsm.ActionHelpSelect})
	if app.HelpModal.Open {
		t.Fatal("expected HelpModal to close after selection")
	}

	// 4. Test IPC action
	if err := app.applyAction(ipc.ActionParams{Name: "help:toggle"}); err != nil {
		t.Fatalf("unexpected error from IPC help:toggle: %v", err)
	}
	if !app.HelpModal.Open {
		t.Fatal("expected HelpModal to open from IPC action")
	}

	// Dismiss with cancel
	app.executeAction(fsm.Action{Type: fsm.ActionHelpCancel})
	if app.HelpModal.Open {
		t.Fatal("expected HelpModal to close after ActionHelpCancel")
	}
}

func TestNewAppDirectoryArgument(t *testing.T) {
	tmpDir := t.TempDir()
	subFile := filepath.Join(tmpDir, "hello.txt")
	_ = os.WriteFile(subFile, []byte("world\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{tmpDir})

	// Workspace path and name
	if app.WorkspacePath != tmpDir {
		t.Fatalf("expected WorkspacePath=%q, got %q", tmpDir, app.WorkspacePath)
	}
	if app.WorkspaceName != filepath.Base(tmpDir) {
		t.Fatalf("expected WorkspaceName=%q, got %q", filepath.Base(tmpDir), app.WorkspaceName)
	}

	// FileTree and Picker should be rooted at tmpDir
	if app.FileTree == nil || app.FileTree.RootPath != tmpDir {
		t.Fatalf("expected FileTree.RootPath=%q, got %v", tmpDir, app.FileTree)
	}
	if app.Picker == nil || app.Picker.WorkspacePath != tmpDir {
		t.Fatalf("expected Picker.WorkspacePath=%q, got %v", tmpDir, app.Picker)
	}

	// Because a directory argument was explicitly given, FileTree should be open and focused in TREE mode
	if !app.FileTree.Open {
		t.Fatal("expected FileTree.Open=true when directory argument passed")
	}
	if app.FSM.State() != fsm.StateTree {
		t.Fatalf("expected FSM state StateTree, got %v", app.FSM.State())
	}

	// Buffers should have 1 empty untitled buffer, not a buffer for the directory
	if len(app.Buffers) != 1 {
		t.Fatalf("expected 1 buffer, got %d", len(app.Buffers))
	}
	if app.Buffers[0].FilePath != "" {
		t.Fatalf("expected empty buffer FilePath, got %q", app.Buffers[0].FilePath)
	}
}

func TestNewAppFileInProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "myproj")
	_ = os.MkdirAll(filepath.Join(projDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module myproj\n"), 0644)
	sourceFile := filepath.Join(projDir, "src", "main.go")
	_ = os.WriteFile(sourceFile, []byte("package main\n"), 0644)

	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{sourceFile})

	// Should resolve workspace to project root with go.mod
	if app.WorkspacePath != projDir {
		t.Fatalf("expected WorkspacePath=%q, got %q", projDir, app.WorkspacePath)
	}
	if app.WorkspaceName != "myproj" {
		t.Fatalf("expected WorkspaceName=myproj, got %q", app.WorkspaceName)
	}

	// FileTree should not be open
	if app.FileTree.Open {
		t.Fatal("expected FileTree.Open=false when file argument passed")
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected StateNormal, got %v", app.FSM.State())
	}

	// Buffer 0 should be sourceFile
	if len(app.Buffers) != 1 || app.Buffers[0].FilePath != sourceFile {
		t.Fatalf("expected buffer for %q, got %+v", sourceFile, app.Buffers)
	}
}

func TestOpenOrSwitchToFileReplacesEmptyUntitledBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp(nil, []string{tmpDir})

	if len(app.Buffers) != 1 || app.Buffers[0].FilePath != "" {
		t.Fatalf("expected 1 empty untitled buffer, got %d", len(app.Buffers))
	}

	file1 := filepath.Join(tmpDir, "file1.txt")
	_ = os.WriteFile(file1, []byte("first content\n"), 0644)

	app.openOrSwitchToFile(file1)

	// Should replace empty scratch buffer, keeping total buffers at 1
	if len(app.Buffers) != 1 {
		t.Fatalf("expected 1 buffer after opening first file, got %d", len(app.Buffers))
	}
	if app.Buffers[0].FilePath != file1 {
		t.Fatalf("expected buffer FilePath=%q, got %q", file1, app.Buffers[0].FilePath)
	}

	// Opening a second file should now append
	file2 := filepath.Join(tmpDir, "file2.txt")
	_ = os.WriteFile(file2, []byte("second content\n"), 0644)

	app.openOrSwitchToFile(file2)
	if len(app.Buffers) != 2 {
		t.Fatalf("expected 2 buffers after opening second file, got %d", len(app.Buffers))
	}
	if app.ActiveBuf != 1 {
		t.Fatalf("expected ActiveBuf=1, got %d", app.ActiveBuf)
	}
}

func TestAppLSPDynamicStatuslineIndicator(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	app := NewApp(cfg, []string{tmpDir})
	app.Width = 100
	app.Height = 24

	goFile := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(goFile, []byte("package main\n"), 0644)

	// Configure buffer without triggering background warmup
	app.Buffers[0].FilePath = goFile
	app.Buffers[0].FileName = "main.go"
	app.Buffers[0].PT = buffer.NewPieceTableFromString("package main\n")

	// 1. Initially without client process, statusline displays idle
	view := ansi.Strip(app.View())
	if !strings.Contains(view, "[gopls: idle]") {
		t.Fatalf("expected [gopls: idle] in statusline, got:\n%s", view)
	}

	// 2. Register mock client in Starting state
	rootDir := app.LSPManager.FindRoot(goFile, cfg.LSP.Servers["gopls"].Roots)
	mockClient := lsp.NewClient("gopls", "gopls", nil, rootDir)
	mockClient.SetStatusForTesting(lsp.StatusStarting, "initializing")
	app.LSPManager.RegisterClientForTesting("gopls", rootDir, mockClient)

	// Deliver LSPStatusMsg to trigger redraw
	app.Update(LSPStatusMsg{FilePath: goFile, ServerName: "gopls", Status: lsp.StatusStarting})
	view = ansi.Strip(app.View())
	if !strings.Contains(view, "[gopls: starting]") {
		t.Fatalf("expected [gopls: starting] in statusline, got:\n%s", view)
	}

	// 3. Update client to Ready state
	mockClient.SetStatusForTesting(lsp.StatusReady, "ok")
	app.Update(LSPStatusMsg{FilePath: goFile, ServerName: "gopls", Status: lsp.StatusReady})
	view = ansi.Strip(app.View())
	if !strings.Contains(view, "[gopls: ok]") {
		t.Fatalf("expected [gopls: ok] in statusline, got:\n%s", view)
	}

	// 4. Update client to Error state
	mockClient.SetStatusForTesting(lsp.StatusError, "crashed")
	app.Update(LSPStatusMsg{FilePath: goFile, ServerName: "gopls", Status: lsp.StatusError})
	view = ansi.Strip(app.View())
	if !strings.Contains(view, "[gopls: err]") {
		t.Fatalf("expected [gopls: err] in statusline, got:\n%s", view)
	}
}

func TestAppBufferCloseAction(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, []string{"file1.go", "file2.go"})
	if len(app.Buffers) != 2 {
		t.Fatalf("expected 2 buffers, got %d", len(app.Buffers))
	}
	app.ActiveBuf = 0

	// Close first buffer
	app.executeAction(fsm.Action{Type: fsm.ActionBufferClose})
	if len(app.Buffers) != 1 {
		t.Fatalf("expected 1 buffer remaining, got %d", len(app.Buffers))
	}
	if app.Buffers[0].FileName != "file2.go" {
		t.Fatalf("expected remaining buffer file2.go, got %q", app.Buffers[0].FileName)
	}

	// Close last buffer -> resets to empty unnamed buffer
	app.executeAction(fsm.Action{Type: fsm.ActionBufferClose})
	if len(app.Buffers) != 1 {
		t.Fatalf("expected 1 buffer, got %d", len(app.Buffers))
	}
	if app.Buffers[0].FilePath != "" {
		t.Fatalf("expected empty buffer, got %q", app.Buffers[0].FilePath)
	}
}

func TestNormalizeKeyCtrlSpace(t *testing.T) {
	// 1. Terminal sends tea.KeyCtrlAt (NUL)
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlAt}
	if got := normalizeKey(keyMsg); got != "ctrl+space" {
		t.Fatalf("expected ctrl+space for KeyCtrlAt, got %q", got)
	}

	// 2. Terminal sends string "ctrl+@"
	keyMsgStrAt := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+@")}
	if got := normalizeKey(keyMsgStrAt); got != "ctrl+space" {
		t.Fatalf("expected ctrl+space for string 'ctrl+@', got %q", got)
	}

	// 3. String "ctrl+space"
	keyMsgStrSpace := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+space")}
	if got := normalizeKey(keyMsgStrSpace); got != "ctrl+space" {
		t.Fatalf("expected ctrl+space for string 'ctrl+space', got %q", got)
	}
}

func TestDiagnosticGutterSuppressionInInsertMode(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	testFile := "/test/workspace/main.go"
	app.Buffers = []*BufferItem{
		NewBufferItem(1, testFile, "package main\n\nfunc main() {\n}\n"),
	}
	app.ActiveBuf = 0
	app.Width = 80
	app.Height = 24

	// Register diagnostics on line 0 (error) and line 2 (warning)
	app.LSPManager.SaveDiagnostics(testFile, []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 5}},
			Severity: lsp.SeverityError,
			Message:  "syntax error",
		},
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 2, Character: 0}, End: lsp.Position{Line: 2, Character: 4}},
			Severity: lsp.SeverityWarning,
			Message:  "unused variable",
		},
	})

	// 1. In NORMAL mode at line 0: line 0 displays 'E', line 2 displays 'W', statusline shows E:1 W:1
	app.FSM.SetState(fsm.StateNormal)
	app.Buffers[0].CursorPos.Line = 0
	view := ansi.Strip(app.View())
	if !strings.Contains(view, "E:1 W:1") {
		t.Fatalf("expected E:1 W:1 in statusline in NORMAL mode, got:\n%s", view)
	}

	// 2. In INSERT mode at line 0:
	// Line 0 diagnostic 'E' MUST be suppressed so it doesn't flash violently while typing!
	// Statusline must NOT display 'E:1 W:1'.
	app.FSM.SetState(fsm.StateInsert)
	view = ansi.Strip(app.View())
	if strings.Contains(view, "E:1 W:1") {
		t.Fatalf("did not expect E:1 W:1 in statusline while in INSERT mode, got:\n%s", view)
	}
	// Check that line 0 in the viewport does not show 'E' in gutter
	lines := strings.Split(view, "\n")
	foundLine0 := false
	for _, l := range lines {
		if strings.Contains(l, "package main") {
			foundLine0 = true
			if strings.Contains(l, "E") {
				t.Fatalf("expected gutter 'E' to be suppressed on line 0 in INSERT mode, got line: %q", l)
			}
			break
		}
	}
	if !foundLine0 {
		t.Fatalf("could not find line 0 in view:\n%s", view)
	}

	// 3. Move cursor to line 2 while still in INSERT mode:
	// Line 0 'E' now displays because cursor is no longer on line 0.
	// Line 2 'W' is suppressed because cursor is on line 2.
	app.Buffers[0].CursorPos.Line = 2
	view = ansi.Strip(app.View())
	lines = strings.Split(view, "\n")
	foundLine0 = false
	foundLine2 := false
	for _, l := range lines {
		if strings.Contains(l, "package main") {
			foundLine0 = true
			if !strings.Contains(l, "E") {
				t.Fatalf("expected gutter 'E' to be present on line 0 when cursor is on line 2, got line: %q", l)
			}
		}
		if strings.Contains(l, "func main") {
			foundLine2 = true
			if strings.Contains(l, "W") {
				t.Fatalf("expected gutter 'W' to be suppressed on line 2 in INSERT mode, got line: %q", l)
			}
		}
	}
	if !foundLine0 || !foundLine2 {
		t.Fatalf("could not find lines in view:\n%s", view)
	}

	// 4. Return to NORMAL mode (Esc):
	// Both diagnostics display normally.
	app.FSM.SetState(fsm.StateNormal)
	view = ansi.Strip(app.View())
	if !strings.Contains(view, "E:1 W:1") {
		t.Fatalf("expected E:1 W:1 in statusline after returning to NORMAL mode, got:\n%s", view)
	}
}

func TestLSPCompletionMsgHandling(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	app.Buffers = []*BufferItem{
		NewBufferItem(1, "/test/file.go", "package main\n"),
	}
	app.ActiveBuf = 0
	app.FSM.SetState(fsm.StateInsert)

	// 1. Error message
	app.Update(LSPCompletionMsg{Err: fmt.Errorf("connection refused")})
	if !strings.Contains(app.StatusMessage, "Autocomplete: connection refused") {
		t.Fatalf("expected Autocomplete error message, got %q", app.StatusMessage)
	}

	// 2. Empty completions
	app.Update(LSPCompletionMsg{Items: []lsp.CompletionItem{}})
	if app.StatusMessage != "No completions available" {
		t.Fatalf("expected 'No completions available', got %q", app.StatusMessage)
	}

	// 3. Valid completions
	app.Update(LSPCompletionMsg{
		Items: []lsp.CompletionItem{
			{Label: "Println", Detail: "func(a ...any) (n int, err error)"},
			{Label: "Printf", Detail: "func(format string, a ...any) (n int, err error)"},
		},
		Prefix: "Print",
	})
	if app.FSM.State() != fsm.StateCompletion {
		t.Fatalf("expected StateCompletion, got %v", app.FSM.State())
	}
	if len(app.CompletionItems) != 2 {
		t.Fatalf("expected 2 completion items, got %d", len(app.CompletionItems))
	}
	if app.CompletionPrefix != "Print" {
		t.Fatalf("expected prefix 'Print', got %q", app.CompletionPrefix)
	}
}

func TestLSPDebounceAndFlush(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	buf := NewBufferItem(1, "/test/workspace/foo.go", "package main\n")
	app.Buffers = []*BufferItem{buf}
	app.ActiveBuf = 0

	// Notify change (starts debounce timer)
	app.notifyLSPBufferChanged(buf)

	app.lspDebounceMu.Lock()
	pendingFile := app.lspPendingFile
	hasTimer := app.lspDebounceTimer != nil
	app.lspDebounceMu.Unlock()

	if pendingFile != "/test/workspace/foo.go" {
		t.Fatalf("expected pendingFile to be set, got %q", pendingFile)
	}
	if !hasTimer {
		t.Fatalf("expected debounce timer to be active")
	}

	// Flush change immediately
	app.flushLSPBufferChanged(buf)

	app.lspDebounceMu.Lock()
	pendingFileAfter := app.lspPendingFile
	hasTimerAfter := app.lspDebounceTimer != nil
	app.lspDebounceMu.Unlock()

	if pendingFileAfter != "" {
		t.Fatalf("expected pendingFile to be cleared after flush, got %q", pendingFileAfter)
	}
	if hasTimerAfter {
		t.Fatalf("expected debounce timer to be stopped after flush")
	}
}

func TestMarkdownConcealAndHighlightAlignment(t *testing.T) {
	theme := NewGruvboxTheme()
	md := "review [AGENTS.md](AGENTS.md) and [ARCHITECTURE.md](ARCHITECTURE.md) now\n" +
		"1. **Branching & Commits:** Never commit directly to `main`/`master`.\n" +
		"2. **100% Load-Image Parity:** (`vasmm68k_mot` + `scripts/verify_parity.py`).\n" +
		"Licensed under the [MIT License](LICENSE). Copyright (c) 2026.\n" +
		"cursor line\n"

	buf := NewBufferItem(1, "README.md", md)
	// Place cursor on line 4 so lines 0..3 are inactive (concealed)
	buf.CursorPos.Line = 4

	// 1. Check line 0: links "[AGENTS.md](AGENTS.md) and [ARCHITECTURE.md](ARCHITECTURE.md)"
	raw0, _ := buf.PT.Line(0)
	disp0, map0 := markdown.ConcealLineWithMapping(raw0, false)
	if disp0 != "review AGENTS.md and ARCHITECTURE.md now" {
		t.Fatalf("unexpected concealed line 0: %q", disp0)
	}
	spans0 := buf.Highlighter.GetLineHighlights(0)
	for _, s := range spans0 {
		cStart, cEnd := markdown.MapSpan(map0, s.StartCol, s.EndCol)
		if cStart >= cEnd {
			continue
		}
		seg := disp0[cStart:cEnd]
		if seg != "AGENTS.md" && seg != "ARCHITECTURE.md" {
			t.Fatalf("expected only 'AGENTS.md' and 'ARCHITECTURE.md' spans after conceal mapping on line 0, got %q (type %v)", seg, s.Type)
		}
	}

	// 2. Check line 1: bold + inline code "`main`/`master`"
	raw1, _ := buf.PT.Line(1)
	disp1, map1 := markdown.ConcealLineWithMapping(raw1, false)
	if disp1 != "1. Branching & Commits: Never commit directly to main/master." {
		t.Fatalf("unexpected concealed line 1: %q", disp1)
	}
	var stringSegs1 []string
	for _, s := range buf.Highlighter.GetLineHighlights(1) {
		cStart, cEnd := markdown.MapSpan(map1, s.StartCol, s.EndCol)
		if cStart < cEnd && s.Type == treesitter.TokenString {
			stringSegs1 = append(stringSegs1, disp1[cStart:cEnd])
		}
	}
	if len(stringSegs1) != 2 || stringSegs1[0] != "main" || stringSegs1[1] != "master" {
		t.Fatalf("expected TokenString segments ['main', 'master'] on concealed line 1, got %v", stringSegs1)
	}

	// 3. Check line 2: underscores in inline code "`vasmm68k_mot` + `scripts/verify_parity.py`"
	raw2, _ := buf.PT.Line(2)
	disp2, map2 := markdown.ConcealLineWithMapping(raw2, false)
	if disp2 != "2. 100% Load-Image Parity: (vasmm68k_mot + scripts/verify_parity.py)." {
		t.Fatalf("unexpected concealed line 2: %q", disp2)
	}
	var stringSegs2 []string
	for _, s := range buf.Highlighter.GetLineHighlights(2) {
		cStart, cEnd := markdown.MapSpan(map2, s.StartCol, s.EndCol)
		if cStart < cEnd && s.Type == treesitter.TokenString {
			stringSegs2 = append(stringSegs2, disp2[cStart:cEnd])
		}
	}
	if len(stringSegs2) != 2 || stringSegs2[0] != "vasmm68k_mot" || stringSegs2[1] != "scripts/verify_parity.py" {
		t.Fatalf("expected TokenString segments ['vasmm68k_mot', 'scripts/verify_parity.py'] on concealed line 2, got %v", stringSegs2)
	}

	// 4. Check line 3: "[MIT License](LICENSE)"
	raw3, _ := buf.PT.Line(3)
	disp3, map3 := markdown.ConcealLineWithMapping(raw3, false)
	if disp3 != "Licensed under the MIT License. Copyright (c) 2026." {
		t.Fatalf("unexpected concealed line 3: %q", disp3)
	}
	for _, s := range buf.Highlighter.GetLineHighlights(3) {
		cStart, cEnd := markdown.MapSpan(map3, s.StartCol, s.EndCol)
		if cStart >= cEnd {
			continue
		}
		if disp3[cStart:cEnd] != "MIT License" {
			t.Fatalf("expected only 'MIT License' highlighted on concealed line 3, got %q", disp3[cStart:cEnd])
		}
	}

	// 5. Render full viewport and ensure plain text matches concealed lines
	vp := RenderViewport(ViewportParams{
		Buffer:   buf,
		FSM:      fsm.NewFSM(),
		Width:    100,
		Height:   6,
		TabWidth: 4,
		Theme:    theme,
	})
	stripped := StripAnsi(vp)
	if !strings.Contains(stripped, "vasmm68k_mot + scripts/verify_parity.py") {
		t.Fatalf("expected viewport to preserve underscores in vasmm68k_mot + scripts/verify_parity.py, got:\n%s", stripped)
	}
}

func TestStatuslineNoDiffStatsAndSingleLineInvariants(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "file_one.go")
	f2 := filepath.Join(tmpDir, "file_two.go")
	_ = os.WriteFile(f1, []byte("package main\n"), 0644)
	_ = os.WriteFile(f2, []byte("package main\n"), 0644)

	app := NewApp(nil, []string{f1, f2})
	app.WorkspacePath = tmpDir
	app.Width = 60
	app.Height = 20

	// Modify files on disk and trigger watcher event
	_ = os.WriteFile(f1, []byte("package main\n// changed 1\n"), 0644)
	_ = os.WriteFile(f2, []byte("package main\n// changed 2\n"), 0644)
	app.handleWatcherEvent(watcher.ChangeEvent{
		Changes: []watcher.FileChangeInfo{{Path: f1}, {Path: f2}},
	})

	// Statusline must NOT get populated with changed file diff stats
	if app.AgentStatus != "" {
		t.Fatalf("expected AgentStatus to remain empty after watcher events, got %q", app.AgentStatus)
	}

	// Open many buffers and select the last one: bufferline must stay 1 line and show active tab
	var items []*BufferItem
	for i := 1; i <= 10; i++ {
		items = append(items, NewBufferItem(i, fmt.Sprintf("long_filename_%d.go", i), ""))
	}
	bl := RenderBufferline(items, 9, 60, "koodieditori", app.Theme)
	if strings.Contains(bl, "\n") {
		t.Fatalf("expected RenderBufferline to never wrap onto multiple lines, got:\n%s", bl)
	}
	if !strings.Contains(StripAnsi(bl), "long_filename_10.go") {
		t.Fatalf("expected active tab 10 to be scrolled into view in bufferline, got %q", StripAnsi(bl))
	}
}

func TestMarkdownPreviewScrollWithFileTreeSync(t *testing.T) {
	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "README.md")

	var docBuilder strings.Builder
	docBuilder.WriteString("# ⌨️⚡ Koodieditori Architecture\n\n")
	docBuilder.WriteString("A fast terminal code editor with **Markdown Preview** and `FileTree`.\n\n")
	docBuilder.WriteString("```toml\n[languages.go]\n\tlsp_command = \"gopls\"\n\tbuild_cmd = \"go build ./...\"\n\t\tdeep_indent = \"value\"\n```\n\n")
	docBuilder.WriteString("```mermaid\nflowchart LR\n    A[\"FileTree\"] --> B[\"MarkdownDrawer\"]\n```\n\n")
	for i := 1; i <= 60; i++ {
		docBuilder.WriteString(fmt.Sprintf("## Section %d — Detailed Specification\n\n", i))
		docBuilder.WriteString(fmt.Sprintf("- [x] Task item %d with **bold**, *italic*, and `inline_code_%d` across a long line that wraps in Glamour.\n", i, i))
		docBuilder.WriteString("```go\nfunc example() {\n\tfmt.Println(\"tab indented line\")\n}\n```\n\n")
	}
	_ = os.WriteFile(mdFile, []byte(docBuilder.String()), 0644)

	app := NewApp(nil, []string{mdFile})
	app.WorkspacePath = tmpDir
	app.Width = 120
	app.Height = 36
	app.FileTree = NewFileTree(tmpDir, 26)
	app.FileTree.Open = true

	// Open Markdown Preview
	app.executeAction(fsm.Action{Type: fsm.ActionToggleMarkdownPreview})
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeMarkdown {
		t.Fatalf("expected markdown preview drawer open")
	}

	assertFrameGeometry := func(step string) {
		t.Helper()
		view := app.View()
		lines := strings.Split(view, "\n")
		if len(lines) != app.Height {
			t.Fatalf("%s: expected %d lines in View(), got %d", step, app.Height, len(lines))
		}
		for i, line := range lines {
			if strings.ContainsAny(line, "\t\r\uFE0E\uFE0F") {
				t.Fatalf("%s: line %d contains forbidden control/tab/variation chars: %q", step, i, line)
			}
			aw := ansi.StringWidth(line)
			rw := runewidth.StringWidth(ansi.Strip(line))
			if aw != app.Width || rw != app.Width {
				t.Fatalf("%s: line %d width mismatch: expected %d, got ansi=%d runewidth=%d\nline=%q",
					step, i, app.Width, aw, rw, ansi.Strip(line))
			}
			// Middle rows must keep FileTree border '│' aligned at column treeWidth-1 (25)
			if i >= 1 && i < app.Height-1 && app.FileTree.Open {
				treePart := ansi.Strip(ansi.Cut(line, 0, app.FileTree.Width))
				if !strings.HasSuffix(treePart, "│") {
					t.Fatalf("%s: row %d FileTree border misaligned, treePart=%q", step, i, treePart)
				}
			}
		}
	}

	assertFrameGeometry("initial")

	// Scroll down for a while with MoveDown and PageDown
	for step := 0; step < 120; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionMoveDown})
		assertFrameGeometry(fmt.Sprintf("down-%d", step))
	}
	for step := 0; step < 10; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionPageDown})
		assertFrameGeometry(fmt.Sprintf("pagedown-%d", step))
	}

	// Scroll back up with MoveUp and PageUp
	for step := 0; step < 120; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionMoveUp})
		assertFrameGeometry(fmt.Sprintf("up-%d", step))
	}
	for step := 0; step < 10; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionPageUp})
		assertFrameGeometry(fmt.Sprintf("pageup-%d", step))
	}

	// Test maximized drawer scrolling down and up
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerMaximize})
	if !app.Drawer.Maximized {
		t.Fatalf("expected drawer maximized")
	}
	for step := 0; step < 40; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionMoveDown})
		assertFrameGeometry(fmt.Sprintf("max-down-%d", step))
	}
	for step := 0; step < 40; step++ {
		app.executeAction(fsm.Action{Type: fsm.ActionMoveUp})
		assertFrameGeometry(fmt.Sprintf("max-up-%d", step))
	}

	// Unmaximize and test mouse wheel scrolling inside drawer
	app.executeAction(fsm.Action{Type: fsm.ActionDrawerMaximize})
	for step := 0; step < 10; step++ {
		app.handleMouse(tea.MouseMsg{
			X:      50,
			Y:      app.Height - 3,
			Button: tea.MouseButtonWheelDown,
		})
		assertFrameGeometry(fmt.Sprintf("wheel-down-%d", step))
	}
	scrollAfterDown := app.Drawer.ScrollY
	if scrollAfterDown <= 0 {
		t.Fatalf("expected positive ScrollY after mouse wheel down, got %d", scrollAfterDown)
	}
	app.handleMouse(tea.MouseMsg{
		X:      50,
		Y:      app.Height - 3,
		Button: tea.MouseButtonWheelUp,
	})
	assertFrameGeometry("wheel-up-1")
	if app.Drawer.ScrollY >= scrollAfterDown {
		t.Fatalf("expected ScrollY to decrease immediately on first wheel up, was %d, got %d", scrollAfterDown, app.Drawer.ScrollY)
	}
}

func TestFileTreeCreateDirAndFilePrompts(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp(nil, nil)
	app.WorkspacePath = tmpDir
	app.Width = 100
	app.Height = 24
	app.FileTree = NewFileTree(tmpDir, 26)
	app.FileTree.Open = true
	app.FSM.SetState(fsm.StateTree)

	sendKey := func(k string) {
		act := app.FSM.HandleKey(k)
		app.executeAction(act)
	}

	// 1. Press 'A' to create a new folder "subpkg"
	sendKey("A")
	if !app.FSM.TreeEditable() {
		t.Fatal("expected FSM.TreeEditable() true after pressing 'A'")
	}
	view := StripAnsi(app.View())
	if !strings.Contains(view, "New Folder:") {
		t.Fatalf("expected statusline to show 'New Folder:' prompt, got:\n%s", view)
	}

	for _, r := range "subpkgx" {
		sendKey(string(r))
	}
	sendKey("backspace")
	sendKey("enter")

	expectedDir := filepath.Join(tmpDir, "subpkg")
	fi, err := os.Stat(expectedDir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected directory %s to be created on disk, err=%v", expectedDir, err)
	}
	if app.FileTree.SelectedNode() == nil || filepath.Clean(app.FileTree.SelectedNode().Path) != filepath.Clean(expectedDir) {
		t.Fatalf("expected FileTree cursor to select newly created directory %s", expectedDir)
	}

	// 2. Press 'a' while "subpkg" is selected to create "hello.go" inside "subpkg"
	sendKey("a")
	if !app.FSM.TreeEditable() {
		t.Fatal("expected FSM.TreeEditable() true after pressing 'a'")
	}
	view = StripAnsi(app.View())
	if !strings.Contains(view, "New File:") {
		t.Fatalf("expected statusline to show 'New File:' prompt, got:\n%s", view)
	}

	for _, r := range "hello.go" {
		sendKey(string(r))
	}
	sendKey("enter")

	expectedFile := filepath.Join(expectedDir, "hello.go")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Fatalf("expected file %s to be created on disk, err=%v", expectedFile, err)
	}
	if app.CurrentBuffer() == nil || filepath.Clean(app.CurrentBuffer().FilePath) != filepath.Clean(expectedFile) {
		t.Fatalf("expected newly created file %s to be opened as active buffer", expectedFile)
	}
	if app.FSM.State() != fsm.StateNormal {
		t.Fatalf("expected state NORMAL after opening newly created file, got %v", app.FSM.State())
	}

	// 3. Test cancelling prompt with Esc
	app.FSM.SetState(fsm.StateTree)
	sendKey("A")
	for _, r := range "aborted_dir" {
		sendKey(string(r))
	}
	sendKey("esc")
	if app.FSM.TreeEditable() {
		t.Fatal("expected FSM.TreeEditable() false after Esc")
	}
	if _, err := os.Stat(filepath.Join(expectedDir, "aborted_dir")); !os.IsNotExist(err) {
		t.Fatal("expected cancelled directory not to be created")
	}
}



