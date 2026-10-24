package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/lsp"
)

// RenderHoverBox renders a styled floating box containing documentation/hover text.
func RenderHoverBox(content string, maxWidth, maxHeight int, theme *Theme) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	if theme == nil {
		theme = DefaultTheme()
	}

	boxWidth := maxWidth - 4
	if boxWidth > 70 {
		boxWidth = 70
	}
	if boxWidth < 30 {
		boxWidth = 30
	}

	boxHeight := maxHeight - 4
	if boxHeight > 15 {
		boxHeight = 15
	}
	if boxHeight < 4 {
		boxHeight = 4
	}

	lines := strings.Split(content, "\n")
	if len(lines) > boxHeight {
		lines = lines[:boxHeight]
		lines = append(lines, theme.SynComment.Render("... [more lines truncated]"))
	}

	body := strings.Join(lines, "\n")

	style := theme.OverlayBox.
		Padding(0, 1).
		Width(boxWidth)

	header := theme.OverlayTitle.Render(" Hover Doc (Esc/q to close) ")
	return header + "\n" + style.Render(body)
}

// RenderCompletionBox renders a floating popup list for autocomplete items.
func RenderCompletionBox(items []lsp.CompletionItem, selectedIdx int, maxWidth int, theme *Theme) string {
	if len(items) == 0 {
		return ""
	}
	if theme == nil {
		theme = DefaultTheme()
	}

	maxDisplay := 7
	startIdx := 0
	if selectedIdx >= maxDisplay {
		startIdx = selectedIdx - maxDisplay + 1
	}
	endIdx := startIdx + maxDisplay
	if endIdx > len(items) {
		endIdx = len(items)
	}

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		item := items[i]
		isSel := (i == selectedIdx)

		cursor := "  "
		if isSel {
			cursor = "▶ "
		}

		kindIcon := fmt.Sprintf("[%s]", item.Kind.Icon())
		label := item.Label
		detail := item.Detail
		if detail != "" && !strings.HasPrefix(detail, "(") {
			detail = " " + detail
		}

		itemStr := fmt.Sprintf("%s%-6s %-18s %s", cursor, kindIcon, label, detail)
		if runewidth.StringWidth(itemStr) > maxWidth-4 {
			itemStr = runewidth.Truncate(itemStr, maxWidth-6, "...")
		}

		if isSel {
			lines = append(lines, theme.TextSelected.Render(itemStr))
		} else {
			lines = append(lines, theme.TextNormal.Render(itemStr))
		}
	}

	content := strings.Join(lines, "\n")

	style := theme.OverlayBox.Padding(0, 0)

	title := theme.OverlayTitle.Render(" Autocomplete (Tab/Enter: apply, Esc: cancel) ")
	return title + "\n" + style.Render(content)
}

// RenderCodeActionBox renders a floating selection box for available code actions.
func RenderCodeActionBox(actions []lsp.CodeAction, selectedIdx int, maxWidth int, theme *Theme) string {
	if len(actions) == 0 {
		return ""
	}
	if theme == nil {
		theme = DefaultTheme()
	}

	var lines []string
	for i, act := range actions {
		isSel := (i == selectedIdx)
		cursor := "  "
		if isSel {
			cursor = "▶ "
		}

		itemStr := fmt.Sprintf("%s%d. %s", cursor, i+1, act.Title)
		if act.IsPreferred {
			itemStr += " [preferred]"
		}

		if runewidth.StringWidth(itemStr) > maxWidth-4 {
			itemStr = runewidth.Truncate(itemStr, maxWidth-6, "...")
		}

		if isSel {
			lines = append(lines, theme.TextSelected.Render(itemStr))
		} else {
			lines = append(lines, theme.TextNormal.Render(itemStr))
		}
	}

	content := strings.Join(lines, "\n")

	style := theme.OverlayBox.Padding(0, 1)

	title := theme.OverlayTitle.Render(" Code Actions (Enter: apply, Esc: cancel) ")
	return title + "\n" + style.Render(content)
}

// OverlayBoxOnLines composites a rendered box over the target slice of lines at (topRow, leftCol).
func OverlayBoxOnLines(lines []string, boxStr string, topRow, leftCol, maxCols int) []string {
	if boxStr == "" || len(lines) == 0 {
		return lines
	}

	boxLines := strings.Split(boxStr, "\n")
	out := make([]string, len(lines))
	copy(out, lines)

	for bIdx, bLine := range boxLines {
		targetRow := topRow + bIdx
		if targetRow < 0 || targetRow >= len(out) {
			continue
		}

		boxW := runewidth.StringWidth(StripAnsi(bLine))
		baseLine := out[targetRow]
		baseW := runewidth.StringWidth(StripAnsi(baseLine))

		// If baseLine is shorter than leftCol, pad it
		if baseW < leftCol {
			baseLine = baseLine + strings.Repeat(" ", leftCol-baseW)
		}

		// Composite: prefix up to leftCol, then boxLine, then suffix after leftCol+boxW
		prefix := runewidth.Truncate(StripAnsi(baseLine), leftCol, "")
		suffix := ""
		if baseW > leftCol+boxW {
			raw := StripAnsi(baseLine)
			if len(raw) > leftCol+boxW {
				suffix = raw[leftCol+boxW:]
			}
		}

		combined := prefix + bLine + suffix
		if maxCols > 0 && runewidth.StringWidth(StripAnsi(combined)) > maxCols {
			combined = runewidth.Truncate(combined, maxCols, "")
		}
		out[targetRow] = combined
	}

	return out
}
