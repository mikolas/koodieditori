package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/muesli/termenv"
)

func TestVisualColToRuneCol(t *testing.T) {
	tabWidth := 4

	// 1. Line with leading tab: "\thello"
	// Rune 0: '\t' (visCol 0..3)
	// Rune 1: 'h'  (visCol 4)
	// Rune 2: 'e'  (visCol 5)
	raw := "\thello"
	for vis := 0; vis < 4; vis++ {
		r := VisualColToRuneCol(raw, vis, tabWidth)
		if r != 0 {
			t.Fatalf("expected visCol %d to map to rune 0 ('\\t'), got %d", vis, r)
		}
	}
	if r := VisualColToRuneCol(raw, 4, tabWidth); r != 1 {
		t.Fatalf("expected visCol 4 to map to rune 1 ('h'), got %d", r)
	}
	if r := VisualColToRuneCol(raw, 5, tabWidth); r != 2 {
		t.Fatalf("expected visCol 5 to map to rune 2 ('e'), got %d", r)
	}
	if r := VisualColToRuneCol(raw, 100, tabWidth); r != 6 {
		t.Fatalf("expected visCol 100 to map to rune 6 (EOL), got %d", r)
	}

	// 2. Line with wide characters: "日本語" (width 2 each)
	// Rune 0: '日' (visCol 0..1)
	// Rune 1: '本' (visCol 2..3)
	// Rune 2: '語' (visCol 4..5)
	rawWide := "日本語"
	if r := VisualColToRuneCol(rawWide, 0, tabWidth); r != 0 {
		t.Fatalf("expected vis 0 -> rune 0, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 1, tabWidth); r != 0 {
		t.Fatalf("expected vis 1 -> rune 0, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 2, tabWidth); r != 1 {
		t.Fatalf("expected vis 2 -> rune 1, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 3, tabWidth); r != 1 {
		t.Fatalf("expected vis 3 -> rune 1, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 4, tabWidth); r != 2 {
		t.Fatalf("expected vis 4 -> rune 2, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 5, tabWidth); r != 2 {
		t.Fatalf("expected vis 5 -> rune 2, got %d", r)
	}
	if r := VisualColToRuneCol(rawWide, 6, tabWidth); r != 3 {
		t.Fatalf("expected vis 6 -> rune 3 (EOL), got %d", r)
	}

	// 3. Line with multibyte 1-column characters: "äöå" (2 bytes each, 1 visual column)
	rawNordic := "äöå"
	if r := VisualColToRuneCol(rawNordic, 0, tabWidth); r != 0 {
		t.Fatalf("expected vis 0 -> rune 0, got %d", r)
	}
	if r := VisualColToRuneCol(rawNordic, 1, tabWidth); r != 1 {
		t.Fatalf("expected vis 1 -> rune 1, got %d", r)
	}
	if r := VisualColToRuneCol(rawNordic, 2, tabWidth); r != 2 {
		t.Fatalf("expected vis 2 -> rune 2, got %d", r)
	}
	if r := VisualColToRuneCol(rawNordic, 3, tabWidth); r != 3 {
		t.Fatalf("expected vis 3 -> rune 3 (EOL), got %d", r)
	}
}

func TestTabbedAndWideViewportRendering(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	theme := DefaultTheme()

	// 1. Line with tabs: "\tval := 1"
	// Cursor on 'v' (rune 1, visual column 4)
	buf := NewBufferItem(1, "test.go", "\tval := 1\n")
	buf.CursorPos = buffer.Position{Line: 0, Col: 1}

	expanded := expandTabs("\tval := 1", 4)
	rendered := renderLineWithCursor(expanded, "\tval := 1", 4, 0, buf, fsm.StateNormal, 30, theme, nil)

	// 'v' must be rendered with CursorBlock
	vCursor := theme.CursorBlock.Render("v")
	if !strings.Contains(rendered, vCursor) {
		t.Fatalf("expected 'v' to be styled with CursorBlock at visual column 4, got:\n%q", rendered)
	}

	// 2. Line with wide characters: "日本語 = true"
	// Rune 0: '日' (0..2), Rune 1: '本' (2..4), Rune 2: '語' (4..6), Rune 3: ' ' (6..7), Rune 4: '=' (7..8)
	// Cursor on '=' (rune 4, visual column 7)
	bufWide := NewBufferItem(2, "wide.go", "日本語 = true\n")
	bufWide.CursorPos = buffer.Position{Line: 0, Col: 4}

	expandedWide := expandTabs("日本語 = true", 4)
	renderedWide := renderLineWithCursor(expandedWide, "日本語 = true", 4, 0, bufWide, fsm.StateNormal, 30, theme, nil)

	// '=' must be rendered with CursorBlock
	eqCursor := theme.CursorBlock.Render("=")
	if !strings.Contains(renderedWide, eqCursor) {
		t.Fatalf("expected '=' to be styled with CursorBlock at visual column 7, got:\n%q", renderedWide)
	}

	// 3. Selection covering tab: selection from rune 0 to rune 1 (the tab)
	vscodeTheme := NewVSCodeTheme()
	bufSel := NewBufferItem(3, "sel.go", "\tcode\n")
	anchor := buffer.Position{Line: 0, Col: 0}
	bufSel.SelectionAnchor = &anchor
	bufSel.CursorPos = buffer.Position{Line: 0, Col: 1}

	expandedSel := expandTabs("\tcode", 4)
	renderedSel := renderLineWithCursor(expandedSel, "\tcode", 4, 0, bufSel, fsm.StateSelect, 30, vscodeTheme, nil)

	// All 4 spaces of the tab should be selected
	selSpace := vscodeTheme.TextSelected.Render(" ")
	if strings.Count(renderedSel, selSpace) < 4 {
		t.Fatalf("expected at least 4 selected spaces for tab selection, got count %d", strings.Count(renderedSel, selSpace))
	}

	// 'c' at col 4 is the cursor in SELECT mode, so it is rendered with CursorBlock (not TextSelected)
	cSel := vscodeTheme.TextSelected.Render("c")
	if strings.Contains(renderedSel, cSel) {
		t.Fatalf("expected 'c' to NOT be styled with TextSelected")
	}

	// 'o' at col 5 should be neither selected nor cursor
	oSel := vscodeTheme.TextSelected.Render("o")
	if strings.Contains(renderedSel, oSel) {
		t.Fatalf("expected 'o' to NOT be selected")
	}
}

func TestHorizontalScrollWithWideAndTabs(t *testing.T) {
	theme := DefaultTheme()
	raw := "\t日本語 and English"
	buf := NewBufferItem(1, "scroll.go", raw+"\n")

	// ScrollX = 4 (skips the tab of 4 spaces)
	buf.ScrollX = 4
	buf.CursorPos = buffer.Position{Line: 0, Col: 1} // Cursor on '日'

	expanded := expandTabs(raw, 4)
	rendered := renderLineWithCursor(expanded, raw, 4, 0, buf, fsm.StateNormal, 20, theme, nil)

	// The tab spaces (vis 0..3) are scrolled out.
	// '日' should now be at the very start of the rendered view with CursorBlock!
	jpCursor := theme.CursorBlock.Render("日")
	if !strings.HasPrefix(rendered, jpCursor) {
		t.Fatalf("expected view to start with scrolled cursor '日', got:\n%q", rendered)
	}

	// Test AdjustScroll with horizontal scrolling past codeWidth
	buf.CursorPos = buffer.Position{Line: 0, Col: 5} // 'a' in "and" (vis col 11)
	AdjustScroll(buf, 10, 8, 2, 4)
	if buf.ScrollX <= 0 {
		t.Fatalf("expected ScrollX > 0 when cursor exceeds codeWidth, got %d", buf.ScrollX)
	}
}

func TestMouseClickTabAndWideCoordinateMapping(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Editor.Mouse = true
	cfg.Editor.TabWidth = 4

	app := NewApp(cfg, nil)
	defer app.Close()

	// Buffer with tab-indented and wide-character lines
	// Line 0: "\thello"
	// Line 1: "日本語"
	buf := app.CurrentBuffer()
	_ = buf.PT.Insert(0, "\thello\n日本語\n")

	gutterWidth := CalculateGutterWidth(buf.PT.LineCount())

	// 1. Mouse click at Line 0, inside the tab (screen col = gutterWidth + 2)
	// Visual col 2 -> Rune index 0 ('\t')
	app.handleMouse(tea.MouseMsg{
		X:      gutterWidth + 2,
		Y:      1, // line 0 in editor viewport (Y=0 is bufferline)
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if buf.CursorPos.Line != 0 || buf.CursorPos.Col != 0 {
		t.Fatalf("expected cursor at Line 0, Col 0 for click inside tab, got Line %d, Col %d", buf.CursorPos.Line, buf.CursorPos.Col)
	}

	// 2. Mouse click at Line 0, on 'h' (screen col = gutterWidth + 4)
	// Visual col 4 -> Rune index 1 ('h')
	app.handleMouse(tea.MouseMsg{
		X:      gutterWidth + 4,
		Y:      1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if buf.CursorPos.Line != 0 || buf.CursorPos.Col != 1 {
		t.Fatalf("expected cursor at Line 0, Col 1 for click on 'h', got Line %d, Col %d", buf.CursorPos.Line, buf.CursorPos.Col)
	}

	// 3. Mouse click at Line 1, on '本' (screen col = gutterWidth + 2 or 3)
	// '日' is visCol 0..1, '本' is visCol 2..3 -> Rune index 1
	app.handleMouse(tea.MouseMsg{
		X:      gutterWidth + 3,
		Y:      2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if buf.CursorPos.Line != 1 || buf.CursorPos.Col != 1 {
		t.Fatalf("expected cursor at Line 1, Col 1 for click on '本', got Line %d, Col %d", buf.CursorPos.Line, buf.CursorPos.Col)
	}
}
