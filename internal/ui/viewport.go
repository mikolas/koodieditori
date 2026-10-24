package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/markdown"
	"github.com/mikolas/koodieditori/internal/treesitter"
)

// ViewportParams holds geometry and state for rendering the code viewport.
type ViewportParams struct {
	Buffer          *BufferItem
	FSM             *fsm.FSM
	Width           int
	Height          int
	TabWidth        int
	RelativeLines   bool
	Scrolloff       int
	Theme           *Theme
	DiagnosticSigns map[int]string // Line -> "E", "W", "I", "A"
	GitSigns        map[int]string // Line -> "+", "~", "-"
	SearchSpans     map[int][]SearchHighlight
	Breakpoints     map[int]bool // 0-based Line -> true
	StoppedLine     int          // 0-based stopped line index (-1 if none)
}

// SearchHighlight defines visual column span of a search match in a viewport line.
type SearchHighlight struct {
	StartCol  int
	EndCol    int
	IsCurrent bool
}

// AdjustScroll keeps cursor in view with scrolloff margins and handles horizontal scrolling.
func AdjustScroll(buf *BufferItem, viewportHeight int, codeWidth int, scrolloff int, tabWidth ...int) {
	if viewportHeight <= 0 || buf == nil || buf.PT == nil {
		return
	}

	effectiveScrolloff := scrolloff
	if effectiveScrolloff*2 >= viewportHeight {
		effectiveScrolloff = viewportHeight / 4
	}

	// Vertical scrolling
	if buf.CursorPos.Line < buf.ScrollY+effectiveScrolloff {
		buf.ScrollY = buf.CursorPos.Line - effectiveScrolloff
		if buf.ScrollY < 0 {
			buf.ScrollY = 0
		}
	} else if buf.CursorPos.Line >= buf.ScrollY+viewportHeight-effectiveScrolloff {
		buf.ScrollY = buf.CursorPos.Line - viewportHeight + effectiveScrolloff + 1
		if buf.ScrollY < 0 {
			buf.ScrollY = 0
		}
	}

	totalLines := buf.PT.LineCount()
	maxScrollY := totalLines - viewportHeight
	if maxScrollY < 0 {
		maxScrollY = 0
	}
	if buf.ScrollY > maxScrollY {
		buf.ScrollY = maxScrollY
	}
	if buf.ScrollY < 0 {
		buf.ScrollY = 0
	}

	// Horizontal scrolling
	if codeWidth > 0 {
		tw := 4
		if len(tabWidth) > 0 && tabWidth[0] > 0 {
			tw = tabWidth[0]
		}
		rawLine, _ := buf.PT.Line(buf.CursorPos.Line)
		visCol := RuneColToVisualCol(rawLine, buf.CursorPos.Col, tw)
		if visCol < buf.ScrollX {
			buf.ScrollX = visCol
		} else if visCol >= buf.ScrollX+codeWidth {
			buf.ScrollX = visCol - codeWidth + 1
		}
		if buf.ScrollX < 0 {
			buf.ScrollX = 0
		}
	}
}

// ScrollbarInfo stores calculated geometry for rendering the buffer scrollbar.
type ScrollbarInfo struct {
	Active      bool
	ThumbTop    int // Starting row in viewport (0-based) for the thumb
	ThumbHeight int // Height of thumb in rows
}

// CalculateScrollbar computes thumb position and height for a buffer.
// Returns an inactive scrollbar if content fits in the viewport.
func CalculateScrollbar(totalLines, viewportHeight, scrollY int) ScrollbarInfo {
	if viewportHeight <= 0 || totalLines <= viewportHeight {
		return ScrollbarInfo{Active: false}
	}

	thumbHeight := (viewportHeight * viewportHeight) / totalLines
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > viewportHeight {
		thumbHeight = viewportHeight
	}

	maxScrollY := totalLines - viewportHeight
	travel := viewportHeight - thumbHeight
	thumbTop := 0
	if maxScrollY > 0 && travel > 0 {
		thumbTop = (scrollY * travel) / maxScrollY
		if thumbTop < 0 {
			thumbTop = 0
		}
		if thumbTop > travel {
			thumbTop = travel
		}
	}

	return ScrollbarInfo{
		Active:      true,
		ThumbTop:    thumbTop,
		ThumbHeight: thumbHeight,
	}
}

// RenderViewport renders the code editor body (gutter + text lines + scrollbar).
func RenderViewport(p ViewportParams) string {
	buf := p.Buffer
	totalLines := buf.PT.LineCount()
	gutterWidth := CalculateGutterWidth(totalLines)

	scrollbarWidth := 0
	if totalLines > p.Height && p.Height > 0 {
		scrollbarWidth = 1
	}

	codeWidth := p.Width - gutterWidth - scrollbarWidth
	if codeWidth <= 0 {
		codeWidth = 10
	}

	AdjustScroll(buf, p.Height, codeWidth, p.Scrolloff, p.TabWidth)

	scrollbar := CalculateScrollbar(totalLines, p.Height, buf.ScrollY)

	theme := p.Theme
	if theme == nil {
		theme = DefaultTheme()
	}

	var linesOut []string

	isMd := markdown.IsMarkdownFile(buf.FilePath)
	inFence := false
	var fenceChar byte
	fenceLen := 0
	if isMd {
		for i := 0; i < buf.ScrollY && i < totalLines; i++ {
			lineStr, _ := buf.PT.Line(i)
			inFence, fenceChar, fenceLen = markdown.StepFenceState(lineStr, inFence, fenceChar, fenceLen)
		}
	}

	for row := 0; row < p.Height; row++ {
		lineIdx := buf.ScrollY + row

		scrollCell := ""
		if scrollbar.Active {
			if row >= scrollbar.ThumbTop && row < scrollbar.ThumbTop+scrollbar.ThumbHeight {
				scrollCell = theme.ScrollbarThumb.Render("█")
			} else {
				scrollCell = theme.ScrollbarTrack.Render("│")
			}
		}

		if lineIdx >= totalLines {
			// Lines beyond the buffer
			emptyGutter := RenderEmptyGutter(totalLines, theme)
			emptyLine := emptyGutter + strings.Repeat(" ", codeWidth) + scrollCell
			linesOut = append(linesOut, emptyLine)
			continue
		}

		// Prepare gutter info
		gInfo := GutterInfo{
			IsCurrent: (lineIdx == buf.CursorPos.Line),
		}
		if p.GitSigns != nil {
			gInfo.GitSign = p.GitSigns[lineIdx]
		}
		if p.DiagnosticSigns != nil {
			gInfo.DiagSign = p.DiagnosticSigns[lineIdx]
		}
		if p.Breakpoints != nil && p.Breakpoints[lineIdx] {
			gInfo.Breakpoint = true
		}
		if p.StoppedLine >= 0 && p.StoppedLine == lineIdx {
			gInfo.StoppedLine = true
		}

		gutterStr := RenderGutter(lineIdx, buf.CursorPos.Line, totalLines, p.RelativeLines, gInfo, theme)

		// Get raw line text
		lineText, _ := buf.PT.Line(lineIdx)

		displayRaw := lineText
		var rawToConcealed []int
		if isMd {
			wasInFence := inFence
			var isFenceDelim bool
			inFence, fenceChar, fenceLen, isFenceDelim = markdown.NextFenceState(lineText, inFence, fenceChar, fenceLen)
			if !wasInFence && !isFenceDelim {
				displayRaw, rawToConcealed = markdown.ConcealLineWithMapping(lineText, lineIdx == buf.CursorPos.Line)
			}
		}

		// Expand tabs to spaces
		expanded := expandTabs(displayRaw, p.TabWidth)

		var searchHls []SearchHighlight
		if p.SearchSpans != nil {
			searchHls = p.SearchSpans[lineIdx]
		}

		// Render text with selection and cursor
		renderedLine := renderLineWithConceal(expanded, lineText, displayRaw, rawToConcealed, p.TabWidth, lineIdx, buf, p.FSM.State(), codeWidth, theme, searchHls)
		linesOut = append(linesOut, gutterStr+renderedLine+scrollCell)
	}

	return strings.Join(linesOut, "\n")
}

func renderLineWithCursor(expanded string, rawLine string, tabWidth int, lineIdx int, buf *BufferItem, mode fsm.State, maxWidth int, theme *Theme, searchHls []SearchHighlight) string {
	displayRaw := rawLine
	var rawToConcealed []int
	if buf != nil && markdown.IsMarkdownFile(buf.FilePath) && lineIdx != buf.CursorPos.Line {
		if expanded != expandTabs(rawLine, tabWidth) {
			displayRaw, rawToConcealed = markdown.ConcealLineWithMapping(rawLine, false)
		}
	}
	return renderLineWithConceal(expanded, rawLine, displayRaw, rawToConcealed, tabWidth, lineIdx, buf, mode, maxWidth, theme, searchHls)
}

func renderLineWithConceal(expanded string, rawLine string, displayRaw string, rawToConcealed []int, tabWidth int, lineIdx int, buf *BufferItem, mode fsm.State, maxWidth int, theme *Theme, searchHls []SearchHighlight) string {
	runes := []rune(expanded)

	// Determine selection bounds on this line in visual display columns
	rawSelStart, rawSelEnd := getLineSelectionBounds(lineIdx, buf)
	visSelStart := -1
	visSelEnd := -1
	if rawSelStart >= 0 {
		if rawToConcealed != nil {
			bStart := RuneColToByteCol(rawLine, rawSelStart)
			cStart, _ := markdown.MapSpan(rawToConcealed, bStart, bStart)
			visSelStart = ByteColToVisualCol(displayRaw, cStart, tabWidth)
		} else {
			visSelStart = RuneColToVisualCol(rawLine, rawSelStart, tabWidth)
		}
	}
	if rawSelEnd >= 0 {
		if rawSelEnd >= 999999 {
			visSelEnd = 999999
		} else if rawToConcealed != nil {
			bEnd := RuneColToByteCol(rawLine, rawSelEnd)
			cEnd, _ := markdown.MapSpan(rawToConcealed, bEnd, bEnd)
			visSelEnd = ByteColToVisualCol(displayRaw, cEnd, tabWidth)
		} else {
			visSelEnd = RuneColToVisualCol(rawLine, rawSelEnd, tabWidth)
		}
	}

	var sb strings.Builder
	cursorCol := RuneColToVisualCol(rawLine, buf.CursorPos.Col, tabWidth)

	var rawSpans []treesitter.HighlightSpan
	if buf.Highlighter != nil {
		rawSpans = buf.Highlighter.GetLineHighlights(lineIdx)
	}
	visSpans := make([]treesitter.HighlightSpan, 0, len(rawSpans))
	for _, s := range rawSpans {
		startByte, endByte := s.StartCol, s.EndCol
		if rawToConcealed != nil {
			startByte, endByte = markdown.MapSpan(rawToConcealed, startByte, endByte)
		}
		if startByte >= endByte {
			continue
		}
		vStart := ByteColToVisualCol(displayRaw, startByte, tabWidth)
		vEnd := ByteColToVisualCol(displayRaw, endByte, tabWidth)
		if vStart < vEnd {
			visSpans = append(visSpans, treesitter.HighlightSpan{
				StartCol: vStart,
				EndCol:   vEnd,
				Type:     s.Type,
			})
		}
	}

	visSearchHls := make([]SearchHighlight, 0, len(searchHls))
	for _, h := range searchHls {
		var vStart, vEnd int
		if rawToConcealed != nil {
			bStart := RuneColToByteCol(rawLine, h.StartCol)
			bEnd := RuneColToByteCol(rawLine, h.EndCol)
			cStart, cEnd := markdown.MapSpan(rawToConcealed, bStart, bEnd)
			vStart = ByteColToVisualCol(displayRaw, cStart, tabWidth)
			vEnd = ByteColToVisualCol(displayRaw, cEnd, tabWidth)
		} else {
			vStart = RuneColToVisualCol(rawLine, h.StartCol, tabWidth)
			vEnd = RuneColToVisualCol(rawLine, h.EndCol, tabWidth)
		}
		if vStart < vEnd {
			visSearchHls = append(visSearchHls, SearchHighlight{
				StartCol:  vStart,
				EndCol:    vEnd,
				IsCurrent: h.IsCurrent,
			})
		}
	}

	scrollX := buf.ScrollX
	if scrollX < 0 {
		scrollX = 0
	}

	visCol := 0
	screenColsUsed := 0

	for _, r := range runes {
		rWidth := visualRuneWidth(r)
		if rWidth == 0 {
			continue
		}
		rVisStart := visCol
		rVisEnd := visCol + rWidth
		visCol += rWidth

		// Check horizontal scroll visibility
		if rVisEnd <= scrollX {
			continue
		}
		if rVisStart < scrollX {
			overlap := rVisEnd - scrollX
			for o := 0; o < overlap && screenColsUsed < maxWidth; o++ {
				sb.WriteString(" ")
				screenColsUsed++
			}
			continue
		}
		if rVisStart >= scrollX+maxWidth {
			break
		}
		if rVisEnd > scrollX+maxWidth {
			remaining := maxWidth - screenColsUsed
			for o := 0; o < remaining; o++ {
				sb.WriteString(" ")
				screenColsUsed++
			}
			break
		}

		isCursor := (lineIdx == buf.CursorPos.Line && cursorCol >= rVisStart && cursorCol < rVisEnd)
		isSelected := (visSelStart >= 0 && visSelEnd > visSelStart && rVisStart < visSelEnd && rVisEnd > visSelStart)
		isSearch, isCurSearch := getSearchHighlightAtCol(visSearchHls, rVisStart, rVisEnd)

		var styledChar string
		charStr := string(r)

		if isCursor {
			styledChar = theme.CursorBlock.Render(charStr)
		} else if isSelected {
			styledChar = theme.TextSelected.Render(charStr)
		} else if isCurSearch {
			styledChar = theme.SearchCurrent.Render(charStr)
		} else if isSearch {
			styledChar = theme.SearchMatch.Render(charStr)
		} else {
			token := getTokenAtCol(visSpans, rVisStart, rVisEnd)
			switch token {
			case treesitter.TokenKeyword:
				styledChar = theme.SynKeyword.Render(charStr)
			case treesitter.TokenDeclaration:
				styledChar = theme.SynDeclaration.Render(charStr)
			case treesitter.TokenFunction:
				styledChar = theme.SynFunction.Render(charStr)
			case treesitter.TokenTypeId:
				styledChar = theme.SynType.Render(charStr)
			case treesitter.TokenString:
				styledChar = theme.SynString.Render(charStr)
			case treesitter.TokenComment:
				styledChar = theme.SynComment.Render(charStr)
			case treesitter.TokenNumber:
				styledChar = theme.SynNumber.Render(charStr)
			case treesitter.TokenOperator:
				styledChar = theme.SynOperator.Render(charStr)
			case treesitter.TokenConstant:
				styledChar = theme.SynConstant.Render(charStr)
			case treesitter.TokenVariable:
				styledChar = theme.SynVariable.Render(charStr)
			default:
				styledChar = theme.TextNormal.Render(charStr)
			}
		}

		sb.WriteString(styledChar)
		screenColsUsed += rWidth
	}

	totalVisWidth := visCol
	if lineIdx == buf.CursorPos.Line && cursorCol >= totalVisWidth {
		for c := totalVisWidth; c < cursorCol; c++ {
			if c >= scrollX && c < scrollX+maxWidth && screenColsUsed < maxWidth {
				if visSelStart >= 0 && visSelEnd > visSelStart && c >= visSelStart && c < visSelEnd {
					sb.WriteString(theme.TextSelected.Render(" "))
				} else {
					sb.WriteString(" ")
				}
				screenColsUsed++
			}
		}
		if cursorCol >= scrollX && cursorCol < scrollX+maxWidth && screenColsUsed < maxWidth {
			sb.WriteString(theme.CursorBlock.Render(" "))
			screenColsUsed++
		}
	} else if visSelStart >= 0 && visSelEnd > totalVisWidth && totalVisWidth >= visSelStart {
		if totalVisWidth >= scrollX && totalVisWidth < scrollX+maxWidth && screenColsUsed < maxWidth {
			sb.WriteString(theme.TextSelected.Render(" "))
			screenColsUsed++
		}
	}

	remaining := maxWidth - screenColsUsed
	if remaining > 0 {
		sb.WriteString(strings.Repeat(" ", remaining))
	}

	return sb.String()
}

func getLineSelectionBounds(lineIdx int, buf *BufferItem) (int, int) {
	if buf.SelectionAnchor == nil {
		return -1, -1
	}

	start := *buf.SelectionAnchor
	end := buf.CursorPos

	// Ensure start <= end
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	if lineIdx < start.Line || lineIdx > end.Line {
		return -1, -1
	}

	if lineIdx == start.Line && lineIdx == end.Line {
		return start.Col, end.Col
	} else if lineIdx == start.Line {
		return start.Col, 999999
	} else if lineIdx == end.Line {
		return 0, end.Col
	}

	// Intermediate line: entire line is selected
	return 0, 999999
}

// visualRuneWidth returns the terminal column width of r, returning 0 for
// zero-width variation selectors, joiners, carriage returns, and combining marks.
func visualRuneWidth(r rune) int {
	if r == '\r' || r == '\uFE0E' || r == '\uFE0F' || r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return 1
	}
	return w
}

func expandTabs(s string, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	var sb strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			spaces := tabWidth - (col % tabWidth)
			sb.WriteString(strings.Repeat(" ", spaces))
			col += spaces
		} else {
			w := visualRuneWidth(r)
			if w == 0 {
				continue
			}
			sb.WriteRune(r)
			col += w
		}
	}
	return sb.String()
}

func getTokenAtCol(spans []treesitter.HighlightSpan, startCol, endCol int) treesitter.TokenType {
	for _, s := range spans {
		if startCol < s.EndCol && endCol > s.StartCol {
			return s.Type
		}
	}
	return treesitter.TokenNormal
}

// RuneColToByteCol converts a rune index in rawLine to its UTF-8 byte offset.
func RuneColToByteCol(rawLine string, targetRuneCol int) int {
	if targetRuneCol <= 0 {
		return 0
	}
	rIdx := 0
	for byteIdx := range rawLine {
		if rIdx >= targetRuneCol {
			return byteIdx
		}
		rIdx++
	}
	return len(rawLine)
}

// ByteColToVisualCol converts a UTF-8 byte offset in rawLine to its visual column in the tab-expanded string.
func ByteColToVisualCol(rawLine string, targetByteCol int, tabWidth int) int {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	byteOffset := 0
	visCol := 0
	for _, r := range rawLine {
		if byteOffset >= targetByteCol {
			break
		}
		runeBytes := utf8.RuneLen(r)
		if r == '\t' {
			spaces := tabWidth - (visCol % tabWidth)
			visCol += spaces
		} else {
			visCol += visualRuneWidth(r)
		}
		byteOffset += runeBytes
	}
	if byteOffset < targetByteCol {
		visCol += (targetByteCol - byteOffset)
	}
	return visCol
}

// RuneColToVisualCol converts a rune index in rawLine to its visual column in the tab-expanded string.
func RuneColToVisualCol(rawLine string, targetRuneCol int, tabWidth int) int {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	rIdx := 0
	visCol := 0
	for _, r := range rawLine {
		if rIdx >= targetRuneCol {
			break
		}
		if r == '\t' {
			spaces := tabWidth - (visCol % tabWidth)
			visCol += spaces
		} else {
			visCol += visualRuneWidth(r)
		}
		rIdx++
	}
	if rIdx < targetRuneCol {
		visCol += (targetRuneCol - rIdx)
	}
	return visCol
}

// VisualColToRuneCol converts a visual display column in the tab-expanded line back to a rune index in rawLine.
func VisualColToRuneCol(rawLine string, targetVisCol int, tabWidth int) int {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	visCol := 0
	rIdx := 0
	for _, r := range rawLine {
		if visCol >= targetVisCol {
			return rIdx
		}
		var w int
		if r == '\t' {
			w = tabWidth - (visCol % tabWidth)
		} else {
			w = visualRuneWidth(r)
		}
		if visCol+w > targetVisCol {
			return rIdx
		}
		visCol += w
		rIdx++
	}
	return rIdx
}

func getSearchHighlightAtCol(highlights []SearchHighlight, startCol, endCol int) (bool, bool) {
	for _, h := range highlights {
		if startCol < h.EndCol && endCol > h.StartCol {
			return true, h.IsCurrent
		}
	}
	return false, false
}
