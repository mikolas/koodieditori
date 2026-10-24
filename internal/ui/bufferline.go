package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/treesitter"
)

// BufferItem represents an open text buffer in the editor session.
type BufferItem struct {
	ID               int
	FilePath         string
	FileName         string
	LangName         string
	Highlighter      *treesitter.Highlighter
	PT               *buffer.PieceTable
	CursorPos        buffer.Position
	SelectionAnchor  *buffer.Position
	ScrollY          int
	ScrollX          int
	Dirty            bool
	AgentActive      bool
	DiagnosticsCount int
	ReadOnly         bool

	// Tab click hit-testing ranges [startX, endX)
	StartX int
	EndX   int
}

// NewBufferItem creates a new buffer item.
func NewBufferItem(id int, path string, initialText string) *BufferItem {
	name := "[No Name]"
	if path != "" {
		name = filepath.Base(path)
	}

	hl := treesitter.NewHighlighter()
	firstLine := ""
	if idx := strings.IndexByte(initialText, '\n'); idx != -1 {
		firstLine = initialText[:idx]
	} else {
		firstLine = initialText
	}
	lang, langName := treesitter.DetectLanguageWithContent(path, firstLine)
	hl.Parse([]byte(initialText), lang, langName, path)

	return &BufferItem{
		ID:          id,
		FilePath:    path,
		FileName:    name,
		LangName:    langName,
		Highlighter: hl,
		PT:          buffer.NewPieceTableFromString(initialText),
		CursorPos:   buffer.Position{Line: 0, Col: 0},
	}
}

// RenderBufferline renders Row 0 of the screen displaying tabs and workspace info.
func RenderBufferline(items []*BufferItem, activeIdx int, width int, workspaceName string, theme *Theme) string {
	if width <= 0 {
		return ""
	}
	var tabsBuilder strings.Builder
	currX := 0

	// Right-aligned workspace indicator
	var wsStr string
	if workspaceName != "" {
		wsStr = theme.WorkspaceTag.Render(fmt.Sprintf("[ws: %s]", workspaceName))
	}
	wsWidth := runewidth.StringWidth(StripAnsi(wsStr))
	if wsWidth > width/2 {
		wsStr = ""
		wsWidth = 0
	}
	availTabsWidth := width - wsWidth
	if availTabsWidth < 1 {
		availTabsWidth = width
	}

	// Pre-render tabs to calculate widths and ensure activeIdx is scrolled into view
	renderedTabs := make([]string, len(items))
	tabWidths := make([]int, len(items))
	for i, item := range items {
		item.StartX = -1
		item.EndX = -1

		tabNum := i + 1
		var flags strings.Builder
		if item.Dirty {
			flags.WriteString("*")
		}
		if item.AgentActive {
			flags.WriteString(" [A]")
		}
		if item.DiagnosticsCount > 0 {
			flags.WriteString(fmt.Sprintf(" !%d", item.DiagnosticsCount))
		}

		tabTitle := fmt.Sprintf("[%d: %s%s]", tabNum, item.FileName, flags.String())
		if i == activeIdx {
			renderedTabs[i] = theme.TabActive.Render(tabTitle)
		} else {
			renderedTabs[i] = theme.TabInactive.Render(tabTitle)
		}
		tabWidths[i] = runewidth.StringWidth(StripAnsi(renderedTabs[i]))
	}

	ellipsis := theme.TabInactive.Render("...")
	ellipsisWidth := runewidth.StringWidth(StripAnsi(ellipsis))

	// Determine starting tab index so activeIdx is always visible
	startIdx := 0
	if activeIdx >= 0 && activeIdx < len(items) {
		for startIdx < activeIdx {
			needed := 0
			if startIdx > 0 {
				needed += ellipsisWidth + 1
			}
			for j := startIdx; j <= activeIdx; j++ {
				needed += tabWidths[j] + 1
			}
			if needed <= availTabsWidth {
				break
			}
			startIdx++
		}
	}

	if startIdx > 0 && currX+ellipsisWidth+1 <= availTabsWidth {
		tabsBuilder.WriteString(ellipsis)
		tabsBuilder.WriteString(" ")
		currX += ellipsisWidth + 1
	}

	for i := startIdx; i < len(items); i++ {
		item := items[i]
		renderedTab := renderedTabs[i]
		tabVisualWidth := tabWidths[i]

		reserveRight := 0
		if i < len(items)-1 {
			reserveRight = ellipsisWidth
		}

		if currX+tabVisualWidth+reserveRight > availTabsWidth && i > startIdx {
			if currX+ellipsisWidth <= availTabsWidth {
				tabsBuilder.WriteString(ellipsis)
				currX += ellipsisWidth
			}
			break
		}

		if currX+tabVisualWidth > availTabsWidth {
			renderedTab = ansi.Truncate(renderedTab, availTabsWidth-currX, "")
			tabVisualWidth = runewidth.StringWidth(StripAnsi(renderedTab))
		}

		item.StartX = currX
		item.EndX = currX + tabVisualWidth
		currX = item.EndX

		tabsBuilder.WriteString(renderedTab)
		if currX < availTabsWidth {
			tabsBuilder.WriteString(" ")
			currX++
		}
	}

	tabsStr := tabsBuilder.String()
	tabsWidth := runewidth.StringWidth(StripAnsi(tabsStr))

	remainingSpace := width - tabsWidth - wsWidth
	if remainingSpace < 0 {
		remainingSpace = 0
	}

	middlePadding := strings.Repeat(" ", remainingSpace)
	fullLine := ansi.Truncate(tabsStr+middlePadding+wsStr, width, "")

	return theme.BufferlineBg.Render(fullLine)
}

// HitTestTab returns the index of the tab clicked at coordinate x, or -1 if none.
func HitTestTab(items []*BufferItem, x int) int {
	for i, item := range items {
		if x >= item.StartX && x < item.EndX {
			return i
		}
	}
	return -1
}
