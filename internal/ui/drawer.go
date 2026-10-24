package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/tooling"
)

// DrawerMode determines the rendered content inside the drawer.
type DrawerMode int

const (
	DrawerModeOutput DrawerMode = iota
	DrawerModeMarkdown
	DrawerModeDiffReview
	DrawerModeSearchReplace
	DrawerModeDebug
)

// Drawer manages the ephemeral bottom output / preview panel.
type Drawer struct {
	Open         bool
	Maximized    bool
	Height       int // Target content rows (excluding border header)
	Mode         DrawerMode
	Content      []string
	ParsedErrors []*tooling.DiagnosticError
	CursorLine   int // 0-indexed cursor position in Content
	ScrollY      int // 0-indexed vertical scroll offset in Content
	IsDragging   bool
	Title        string
}

// NewDrawer creates a new default Drawer instance.
func NewDrawer() *Drawer {
	return &Drawer{
		Open:       false,
		Maximized:  false,
		Height:     8,
		Mode:       DrawerModeOutput,
		Content:    make([]string, 0),
		Title:      "Output & Diagnostics (Esc to close, Enter to jump)",
		CursorLine: 0,
		ScrollY:    0,
	}
}

// SetContent replaces the drawer's content and parses compiler diagnostics.
func (d *Drawer) SetContent(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	// Trim trailing blank line if present from split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = sanitizeDrawerLine(l, 4)
	}
	d.Content = lines
	d.ParsedErrors = tooling.ParseOutput(strings.Join(lines, "\n"))
	d.CursorLine = 0
	d.ScrollY = 0
}

// AppendContent appends lines of text to the existing drawer content.
func (d *Drawer) AppendContent(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	newLines := strings.Split(text, "\n")
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}
	for i, l := range newLines {
		newLines[i] = sanitizeDrawerLine(l, 4)
	}
	d.Content = append(d.Content, newLines...)
	newErrors := tooling.ParseOutput(strings.Join(newLines, "\n"))
	d.ParsedErrors = append(d.ParsedErrors, newErrors...)
}

// Toggle toggles the drawer between open and closed.
func (d *Drawer) Toggle() {
	d.Open = !d.Open
	if !d.Open {
		d.Maximized = false
		d.IsDragging = false
	}
}

// Close closes the drawer and restores non-maximized state.
func (d *Drawer) Close() {
	d.Open = false
	d.Maximized = false
	d.IsDragging = false
}

// ToggleMaximize toggles fullscreen drawer expansion.
func (d *Drawer) ToggleMaximize() {
	d.Maximized = !d.Maximized
}

// Grow increases the drawer height by delta rows.
func (d *Drawer) Grow(delta int) {
	d.Height += delta
	if d.Height < 3 {
		d.Height = 3
	}
}

// Shrink decreases the drawer height by delta rows.
func (d *Drawer) Shrink(delta int) {
	d.Height -= delta
	if d.Height < 3 {
		d.Height = 3
	}
}

// MoveCursor moves the drawer cursor by delta lines and keeps it in view.
func (d *Drawer) MoveCursor(delta int, visibleRows int) {
	if len(d.Content) == 0 {
		d.CursorLine = 0
		d.ScrollY = 0
		return
	}

	d.CursorLine += delta
	if d.CursorLine < 0 {
		d.CursorLine = 0
	}
	if d.CursorLine >= len(d.Content) {
		d.CursorLine = len(d.Content) - 1
	}

	if visibleRows <= 0 {
		visibleRows = d.Height
	}
	if visibleRows <= 0 {
		visibleRows = 5
	}

	// Adjust scroll window
	if d.CursorLine < d.ScrollY {
		d.ScrollY = d.CursorLine
	} else if d.CursorLine >= d.ScrollY+visibleRows {
		d.ScrollY = d.CursorLine - visibleRows + 1
	}
	maxScrollY := len(d.Content) - visibleRows
	if maxScrollY < 0 {
		maxScrollY = 0
	}
	if d.ScrollY > maxScrollY {
		d.ScrollY = maxScrollY
	}
	if d.ScrollY < 0 {
		d.ScrollY = 0
	}
}

// Scroll scrolls the drawer viewport by delta lines and keeps CursorLine within the visible window.
func (d *Drawer) Scroll(delta int, visibleRows int) {
	if len(d.Content) == 0 {
		d.CursorLine = 0
		d.ScrollY = 0
		return
	}
	if visibleRows <= 0 {
		visibleRows = d.Height
	}
	if visibleRows <= 0 {
		visibleRows = 5
	}

	d.ScrollY += delta
	maxScrollY := len(d.Content) - visibleRows
	if maxScrollY < 0 {
		maxScrollY = 0
	}
	if d.ScrollY > maxScrollY {
		d.ScrollY = maxScrollY
	}
	if d.ScrollY < 0 {
		d.ScrollY = 0
	}

	d.CursorLine += delta
	if d.CursorLine < d.ScrollY {
		d.CursorLine = d.ScrollY
	}
	if d.CursorLine >= d.ScrollY+visibleRows {
		d.CursorLine = d.ScrollY + visibleRows - 1
	}
	if d.CursorLine >= len(d.Content) {
		d.CursorLine = len(d.Content) - 1
	}
	if d.CursorLine < 0 {
		d.CursorLine = 0
	}
}

// CursorToEnd moves the cursor to the last line of content.
func (d *Drawer) CursorToEnd(visibleRows int) {
	if len(d.Content) == 0 {
		return
	}
	d.CursorLine = len(d.Content) - 1
	if visibleRows <= 0 {
		visibleRows = d.Height
	}
	d.ScrollY = len(d.Content) - visibleRows
	if d.ScrollY < 0 {
		d.ScrollY = 0
	}
}

// CursorToStart moves the cursor to the first line of content.
func (d *Drawer) CursorToStart() {
	d.CursorLine = 0
	d.ScrollY = 0
}

// CurrentDiagnostic returns the diagnostic associated with the active CursorLine, if any.
func (d *Drawer) CurrentDiagnostic() *tooling.DiagnosticError {
	if len(d.Content) == 0 || d.CursorLine < 0 || d.CursorLine >= len(d.Content) {
		return nil
	}

	activeLineText := d.Content[d.CursorLine]
	// First check if current line directly parses as a diagnostic
	if diag := tooling.ParseLine(activeLineText); diag != nil {
		return diag
	}

	// Otherwise check if any parsed diagnostic has matching raw text
	for _, diag := range d.ParsedErrors {
		if strings.TrimSpace(diag.RawLine) == strings.TrimSpace(activeLineText) {
			return diag
		}
	}

	// Fallback: if user is on a line near a parsed diagnostic, find the closest error before cursor
	for i := d.CursorLine; i >= 0; i-- {
		if diag := tooling.ParseLine(d.Content[i]); diag != nil {
			return diag
		}
	}

	return nil
}

// Render renders the complete drawer: top border row + content rows.
func (d *Drawer) Render(width int, contentHeight int, theme *Theme, isFocused bool) string {
	if width <= 0 || contentHeight <= 0 {
		return ""
	}

	// 1. Render Header Border: ─── Title ───
	titleText := d.Title
	if d.Maximized {
		titleText = fmt.Sprintf(" %s [MAXIMIZED] (Esc to close, Z to restore) ", d.Title)
		if ansi.StringWidth(titleText) > width {
			titleText = fmt.Sprintf(" [MAXIMIZED] %s ", d.Title)
		}
	} else {
		titleText = fmt.Sprintf(" %s ", d.Title)
	}
	titleText = sanitizeDrawerLine(titleText, 4)

	titleWidth := ansi.StringWidth(titleText)
	var headerLine string
	if width <= titleWidth+6 {
		truncatedTitle := ansi.Truncate(titleText, width, "")
		visW := ansi.StringWidth(truncatedTitle)
		headerLine = theme.DrawerTitle.Render(truncatedTitle)
		if visW < width {
			headerLine += theme.BorderLine.Render(strings.Repeat("─", width-visW))
		}
	} else {
		leftDashes := 4
		rightDashes := width - leftDashes - titleWidth
		if rightDashes < 0 {
			rightDashes = 0
		}
		headerLine = theme.BorderLine.Render(strings.Repeat("─", leftDashes)) +
			theme.DrawerTitle.Render(titleText) +
			theme.BorderLine.Render(strings.Repeat("─", rightDashes))
	}
	headerLine = clampAndPadLine(headerLine, width)

	// 2. Render Content Rows
	var rows []string
	rows = append(rows, headerLine)

	for r := 0; r < contentHeight; r++ {
		lineIdx := d.ScrollY + r
		if lineIdx >= len(d.Content) {
			// Empty blank row in drawer
			rows = append(rows, strings.Repeat(" ", width))
			continue
		}

		rawLine := sanitizeDrawerLine(d.Content[lineIdx], 4)
		diag := tooling.ParseLine(rawLine)
		isCursor := (lineIdx == d.CursorLine && isFocused)

		prefix := "  "
		if isCursor {
			prefix = "▶ "
		}

		prefixWidth := ansi.StringWidth(prefix)
		availWidth := width - prefixWidth
		if availWidth < 0 {
			availWidth = 0
		}

		// Truncate line text with ANSI awareness
		truncated := ansi.Truncate(rawLine, availWidth, "")
		if strings.Contains(truncated, "\x1b") && !strings.HasSuffix(truncated, "\x1b[0m") {
			truncated += "\x1b[0m"
		}
		visWidth := ansi.StringWidth(truncated)
		padding := availWidth - visWidth
		if padding < 0 {
			padding = 0
		}

		var styledLine string
		if d.Mode == DrawerModeMarkdown {
			if isCursor {
				styledLine = theme.TextSelected.Render(prefix) + truncated + strings.Repeat(" ", padding)
			} else {
				styledLine = prefix + truncated + strings.Repeat(" ", padding)
			}
		} else if d.Mode == DrawerModeDiffReview {
			if isCursor {
				styledLine = theme.TextSelected.Render(prefix + truncated + strings.Repeat(" ", padding))
			} else if strings.HasPrefix(rawLine, "+") {
				styledLine = theme.DiffAdded.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "-") {
				styledLine = theme.DiffDeleted.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "[M]") || strings.HasPrefix(rawLine, "[A]") || strings.HasPrefix(rawLine, "[D]") {
				styledLine = theme.DrawerTitle.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else {
				styledLine = theme.TextNormal.Render(prefix+truncated) + strings.Repeat(" ", padding)
			}
		} else if d.Mode == DrawerModeSearchReplace {
			if strings.HasPrefix(rawLine, "  Find:") || strings.HasPrefix(rawLine, "  Replace:") {
				styledLine = theme.DrawerTitle.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "  Flags:") || strings.HasPrefix(rawLine, "  Matches:") {
				styledLine = theme.StatusInfo.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "  +") || strings.HasPrefix(rawLine, "+") {
				styledLine = theme.DiffAdded.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "  -") || strings.HasPrefix(rawLine, "-") {
				styledLine = theme.DiffDeleted.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if isCursor {
				styledLine = theme.TextSelected.Render(prefix + truncated + strings.Repeat(" ", padding))
			} else {
				styledLine = theme.TextNormal.Render(prefix+truncated) + strings.Repeat(" ", padding)
			}
		} else if d.Mode == DrawerModeDebug {
			if strings.HasPrefix(rawLine, "───") || strings.HasPrefix(rawLine, "CALL STACK:") || strings.HasPrefix(rawLine, "VARIABLES:") || strings.HasPrefix(rawLine, "OUTPUT:") || strings.HasPrefix(rawLine, "CONTROLS:") {
				styledLine = theme.DrawerTitle.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "▶") || strings.HasPrefix(rawLine, "  ▶") {
				styledLine = theme.OverlayTitle.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "  [STOPPED") {
				styledLine = theme.DiagError.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if strings.HasPrefix(rawLine, "  [RUNNING") {
				styledLine = theme.DiffAdded.Render(prefix+truncated) + strings.Repeat(" ", padding)
			} else if isCursor {
				styledLine = theme.TextSelected.Render(prefix + truncated + strings.Repeat(" ", padding))
			} else {
				styledLine = theme.TextNormal.Render(prefix+truncated) + strings.Repeat(" ", padding)
			}
		} else {
			if isCursor {
				styledLine = theme.TextSelected.Render(prefix + truncated + strings.Repeat(" ", padding))
			} else if diag != nil {
				switch diag.Severity {
				case "error":
					styledLine = theme.DiagError.Render(prefix+truncated) + strings.Repeat(" ", padding)
				case "warning":
					styledLine = theme.DiagWarning.Render(prefix+truncated) + strings.Repeat(" ", padding)
				default:
					styledLine = theme.DiagInfo.Render(prefix+truncated) + strings.Repeat(" ", padding)
				}
			} else {
				styledLine = theme.TextNormal.Render(prefix+truncated) + strings.Repeat(" ", padding)
			}
		}

		styledLine = clampAndPadLine(styledLine, width)
		rows = append(rows, styledLine)
	}

	return strings.Join(rows, "\n")
}

// sanitizeDrawerLine expands tabs with ANSI awareness and removes carriage returns,
// variation selectors, and raw control characters so terminal column widths never drift.
func sanitizeDrawerLine(s string, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	if !strings.ContainsAny(s, "\t\r\uFE0E\uFE0F") {
		hasCtrl := false
		for i := 0; i < len(s); i++ {
			if s[i] < 0x20 && s[i] != '\x1b' {
				hasCtrl = true
				break
			}
		}
		if !hasCtrl {
			return s
		}
	}

	var sb strings.Builder
	sb.Grow(len(s))
	col := 0
	inEsc := false
	inOSC := false

	for i := 0; i < len(s); {
		b := s[i]
		if inOSC {
			sb.WriteByte(b)
			if b == '\x07' {
				inOSC = false
			} else if b == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				sb.WriteByte(s[i+1])
				i += 2
				inOSC = false
				continue
			}
			i++
			continue
		}
		if inEsc {
			sb.WriteByte(b)
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' || b == '@' {
				inEsc = false
			}
			i++
			continue
		}
		if b == '\x1b' {
			sb.WriteByte(b)
			if i+1 < len(s) {
				next := s[i+1]
				if next == '[' {
					sb.WriteByte(next)
					inEsc = true
					i += 2
					continue
				} else if next == ']' || next == '_' {
					sb.WriteByte(next)
					inOSC = true
					i += 2
					continue
				}
			}
			i++
			continue
		}

		var r rune
		var size int
		for _, decoded := range s[i:] {
			r = decoded
			size = len(string(decoded))
			break
		}
		if size <= 0 {
			size = 1
		}
		i += size

		if r == '\r' || r == '\uFE0E' || r == '\uFE0F' {
			continue
		}
		if r == '\t' {
			spaces := tabWidth - (col % tabWidth)
			sb.WriteString(strings.Repeat(" ", spaces))
			col += spaces
			continue
		}
		if r < 0x20 {
			continue
		}

		sb.WriteRune(r)
		w := runewidth.RuneWidth(r)
		if w > 0 {
			col += w
		}
	}

	return sb.String()
}

// clampAndPadLine guarantees that a rendered line has exact visual width == width
// and ends with an SGR reset before any trailing padding spaces.
func clampAndPadLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	if strings.Contains(line, "\x1b") && !strings.HasSuffix(line, "\x1b[0m") {
		line += "\x1b[0m"
	}
	if curW := ansi.StringWidth(line); curW < width {
		line += strings.Repeat(" ", width-curW)
	}
	return line
}

func truncateStringWidth(s string, maxWidth int) string {
	runes := []rune(s)
	curW := 0
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if curW+rw > maxWidth {
			return string(runes[:i])
		}
		curW += rw
	}
	return s
}
