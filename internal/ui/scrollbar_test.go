package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
)

func TestCalculateScrollbar(t *testing.T) {
	// 1. Inactive when content fits in viewport
	info := CalculateScrollbar(10, 20, 0)
	if info.Active {
		t.Fatalf("expected scrollbar inactive when totalLines <= viewportHeight")
	}

	info = CalculateScrollbar(20, 20, 0)
	if info.Active {
		t.Fatalf("expected scrollbar inactive when totalLines == viewportHeight")
	}

	info = CalculateScrollbar(50, 0, 0)
	if info.Active {
		t.Fatalf("expected scrollbar inactive when viewportHeight <= 0")
	}

	// 2. Active when content exceeds viewport
	totalLines := 100
	vpHeight := 20
	maxScroll := totalLines - vpHeight // 80

	// Top of buffer
	infoTop := CalculateScrollbar(totalLines, vpHeight, 0)
	if !infoTop.Active {
		t.Fatalf("expected scrollbar active")
	}
	if infoTop.ThumbTop != 0 {
		t.Errorf("expected ThumbTop 0 at top, got %d", infoTop.ThumbTop)
	}
	if infoTop.ThumbHeight < 1 {
		t.Errorf("expected ThumbHeight >= 1, got %d", infoTop.ThumbHeight)
	}

	// Bottom of buffer
	infoBottom := CalculateScrollbar(totalLines, vpHeight, maxScroll)
	if !infoBottom.Active {
		t.Fatalf("expected scrollbar active at bottom")
	}
	if infoBottom.ThumbTop+infoBottom.ThumbHeight != vpHeight {
		t.Errorf("expected thumb bottom to touch viewport bottom (%d), got ThumbTop=%d, ThumbHeight=%d (sum=%d)",
			vpHeight, infoBottom.ThumbTop, infoBottom.ThumbHeight, infoBottom.ThumbTop+infoBottom.ThumbHeight)
	}

	// Middle of buffer
	infoMid := CalculateScrollbar(totalLines, vpHeight, maxScroll/2)
	if !infoMid.Active {
		t.Fatalf("expected scrollbar active at mid")
	}
	if infoMid.ThumbTop <= 0 || infoMid.ThumbTop >= vpHeight-infoMid.ThumbHeight {
		t.Errorf("expected ThumbTop between top and bottom, got %d", infoMid.ThumbTop)
	}

	// 3. Minimum thumb height is at least 1 even for huge files
	infoHuge := CalculateScrollbar(10000, 10, 0)
	if infoHuge.ThumbHeight != 1 {
		t.Errorf("expected minimum thumb height 1, got %d", infoHuge.ThumbHeight)
	}

	// 4. Clamping out-of-bounds scrollY
	infoNeg := CalculateScrollbar(100, 20, -10)
	if infoNeg.ThumbTop != 0 {
		t.Errorf("expected clamped ThumbTop 0 for negative scroll, got %d", infoNeg.ThumbTop)
	}

	infoExcess := CalculateScrollbar(100, 20, 500)
	if infoExcess.ThumbTop+infoExcess.ThumbHeight != vpHeight {
		t.Errorf("expected clamped ThumbTop at bottom, got %d", infoExcess.ThumbTop)
	}
}

func TestRenderViewportScrollbar(t *testing.T) {
	theme := DefaultTheme()

	// Short buffer (no scrollbar)
	bufShort := NewBufferItem(1, "short.go", "line 1\nline 2\nline 3\n")
	pShort := ViewportParams{
		Buffer:   bufShort,
		FSM:      fsm.NewFSM(),
		Width:    40,
		Height:   10,
		TabWidth: 4,
		Theme:    theme,
	}
	renderedShort := RenderViewport(pShort)
	if strings.Contains(renderedShort, "█") || strings.Contains(renderedShort, "│") {
		t.Errorf("expected no scrollbar characters when buffer fits in viewport, got:\n%s", renderedShort)
	}

	// Long buffer (scrollbar rendered)
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "code line"
	}
	bufLong := NewBufferItem(2, "long.go", strings.Join(lines, "\n"))
	pLong := ViewportParams{
		Buffer:   bufLong,
		FSM:      fsm.NewFSM(),
		Width:    40,
		Height:   10,
		TabWidth: 4,
		Theme:    theme,
	}

	// Top
	bufLong.ScrollY = 0
	renderedLongTop := RenderViewport(pLong)
	if !strings.Contains(renderedLongTop, "█") {
		t.Errorf("expected thumb block '█' in rendered viewport, got:\n%s", renderedLongTop)
	}
	if !strings.Contains(renderedLongTop, "│") {
		t.Errorf("expected track rail '│' in rendered viewport, got:\n%s", renderedLongTop)
	}

	topLines := strings.Split(renderedLongTop, "\n")
	if len(topLines) != 10 {
		t.Fatalf("expected 10 viewport lines, got %d", len(topLines))
	}
	// First line should have thumb '█'
	if !strings.Contains(topLines[0], "█") {
		t.Errorf("expected top row to have thumb '█', got: %q", topLines[0])
	}

	// Bottom
	maxScroll := 50 - 10
	bufLong.ScrollY = maxScroll
	bufLong.CursorPos.Line = 49 // Keep cursor at bottom to avoid scrolloff pulling it up
	renderedLongBottom := RenderViewport(pLong)
	botLines := strings.Split(renderedLongBottom, "\n")
	if len(botLines) != 10 {
		t.Fatalf("expected 10 viewport lines, got %d", len(botLines))
	}
	// Last line should have thumb '█'
	if !strings.Contains(botLines[9], "█") {
		t.Errorf("expected bottom row to have thumb '█', got: %q", botLines[9])
	}
}

func TestScrollbarMouseClickAndDrag(t *testing.T) {
	app := NewApp(nil, nil)
	app.Width = 80
	app.Height = 24
	buf := app.CurrentBuffer()

	// Fill buffer with 100 lines
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "const x = 42"
	}
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, strings.Join(lines, "\n"))
	buf.CursorPos = buffer.Position{Line: 0, Col: 0}
	buf.ScrollY = 0

	vpHeight := app.getViewportHeight()
	maxScrollY := 100 - vpHeight
	scrollbarX := app.Width - 1 // rightmost column

	viewportTop := 1
	if !app.Config.Bufferline.Show {
		viewportTop = 0
	}

	// 1. Click bottom of scrollbar
	app.handleMouse(tea.MouseMsg{
		X:      scrollbarX,
		Y:      viewportTop + vpHeight - 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if buf.ScrollY != maxScrollY {
		t.Errorf("expected ScrollY to jump to maxScrollY (%d) on bottom click, got %d", maxScrollY, buf.ScrollY)
	}
	// Cursor position and selection should remain unchanged
	if buf.CursorPos.Line != 0 || buf.CursorPos.Col != 0 {
		t.Errorf("expected cursor pos unchanged, got Line=%d Col=%d", buf.CursorPos.Line, buf.CursorPos.Col)
	}
	if buf.SelectionAnchor != nil {
		t.Errorf("expected selection anchor nil after scrollbar click")
	}

	// Release mouse
	app.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
	})
	if app.isScrollbarDragging {
		t.Errorf("expected isScrollbarDragging false after release")
	}

	// 2. Click top of scrollbar
	app.handleMouse(tea.MouseMsg{
		X:      scrollbarX,
		Y:      viewportTop,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if buf.ScrollY != 0 {
		t.Errorf("expected ScrollY 0 on top click, got %d", buf.ScrollY)
	}
	app.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
	})

	// 3. Dragging the scrollbar
	// Press at middle
	midRow := viewportTop + vpHeight/2
	app.handleMouse(tea.MouseMsg{
		X:      scrollbarX,
		Y:      midRow,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if !app.isScrollbarDragging {
		t.Fatalf("expected isScrollbarDragging true after press on scrollbar")
	}
	midScroll := buf.ScrollY
	if midScroll <= 0 || midScroll >= maxScrollY {
		t.Errorf("expected ScrollY in middle range, got %d", midScroll)
	}

	// Drag down by moving mouse
	app.handleMouse(tea.MouseMsg{
		X:      scrollbarX - 5, // slightly off-column during drag
		Y:      midRow + 3,
		Action: tea.MouseActionMotion,
	})
	if buf.ScrollY <= midScroll {
		t.Errorf("expected ScrollY to increase when dragging downwards, got %d (was %d)", buf.ScrollY, midScroll)
	}

	// Release drag
	app.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
	})
	if app.isScrollbarDragging {
		t.Errorf("expected isScrollbarDragging false after release")
	}
}

func TestThemeScrollbarStyles(t *testing.T) {
	themes := []string{"terminal", "vscode", "gruvbox", "nord"}
	for _, themeName := range themes {
		cfg := config.ThemeConfig{Name: themeName}
		th := LoadTheme(cfg)
		if th.ScrollbarTrack.Render("│") == "" {
			t.Errorf("expected ScrollbarTrack non-empty for theme %s", themeName)
		}
		if th.ScrollbarThumb.Render("█") == "" {
			t.Errorf("expected ScrollbarThumb non-empty for theme %s", themeName)
		}
	}

	// Test custom overrides
	cfg := config.ThemeConfig{
		Name: "terminal",
		Colors: map[string]string{
			"scrollbar_track": "#222222",
			"scrollbar_thumb": "#ffffff",
		},
	}
	th := LoadTheme(cfg)
	if th.ScrollbarTrack.Render("│") == "" || th.ScrollbarThumb.Render("█") == "" {
		t.Errorf("expected custom scrollbar overrides to render non-empty text")
	}
}
