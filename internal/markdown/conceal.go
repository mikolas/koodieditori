package markdown

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	reTaskUnchecked = regexp.MustCompile(`^(\s*[-*])\s+\[ \]\s*(.*)$`)
	reTaskChecked   = regexp.MustCompile(`^(\s*[-*])\s+\[[xX]\]\s*(.*)$`)
	reTaskPrefix    = regexp.MustCompile(`^(\s*[-*])(\s+)(\[[ xX]\])(\s*)`)
	reListPrefix    = regexp.MustCompile(`^(\s*)([*-]|\d+\.)(\s+)`)
)

// IsMarkdownFile returns true if the file path has a standard markdown extension.
func IsMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	default:
		return false
	}
}

// ToggleCheckbox toggles task checkbox state between unchecked ([ ]) and checked ([x]).
// Returns the updated line and a boolean indicating whether a checkbox was found.
func ToggleCheckbox(line string) (string, bool) {
	if m := reTaskUnchecked.FindStringSubmatch(line); len(m) > 0 {
		return m[1] + " [x] " + m[2], true
	}
	if m := reTaskChecked.FindStringSubmatch(line); len(m) > 0 {
		return m[1] + " [ ] " + m[2], true
	}
	return line, false
}

// ParseOpeningFence checks if a line opens a fenced code block (``` or ~~~).
func ParseOpeningFence(line string) (isOpen bool, fenceChar byte, fenceLen int, infoString string) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 {
		return false, 0, 0, ""
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return false, 0, 0, ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	if n < 3 {
		return false, 0, 0, ""
	}
	rest := trimmed[n:]
	// In CommonMark, a backtick code fence's info string cannot contain backticks
	// (e.g. ```inline``` on a single line is inline code, not a fence).
	if ch == '`' && strings.ContainsRune(rest, '`') {
		return false, 0, 0, ""
	}
	return true, ch, n, strings.TrimSpace(rest)
}

// IsClosingFence checks if a line closes an active fenced code block opened by (fenceChar, fenceLen).
func IsClosingFence(line string, fenceChar byte, fenceLen int) bool {
	if fenceLen < 3 {
		return false
	}
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < fenceLen || trimmed[0] != fenceChar {
		return false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == fenceChar {
		n++
	}
	if n < fenceLen {
		return false
	}
	return strings.TrimSpace(trimmed[n:]) == ""
}

// NextFenceState advances fenced code block state across a single line and reports
// whether the current line is itself an opening or closing fence delimiter.
func NextFenceState(line string, inFence bool, fenceChar byte, fenceLen int) (nextInFence bool, nextChar byte, nextLen int, isDelimiter bool) {
	if !inFence {
		if ok, ch, n, _ := ParseOpeningFence(line); ok {
			return true, ch, n, true
		}
		return false, 0, 0, false
	}
	if IsClosingFence(line, fenceChar, fenceLen) {
		return false, 0, 0, true
	}
	return true, fenceChar, fenceLen, false
}

// StepFenceState advances fenced code block state across a single line.
func StepFenceState(line string, inFence bool, fenceChar byte, fenceLen int) (nextInFence bool, nextChar byte, nextLen int) {
	nextInFence, nextChar, nextLen, _ = NextFenceState(line, inFence, fenceChar, fenceLen)
	return nextInFence, nextChar, nextLen
}

// MatchTaskPrefix matches a Markdown task list item prefix (e.g. "- [ ] " or "  * [x] ").
func MatchTaskPrefix(line string) (prefixEnd, space1End, boxStart, boxEnd, contentStart int, checked, ok bool) {
	idx := reTaskPrefix.FindStringSubmatchIndex(line)
	if idx == nil {
		return 0, 0, 0, 0, 0, false, false
	}
	bStart, bEnd := idx[6], idx[7]
	isChecked := (bEnd-bStart >= 2 && (line[bStart+1] == 'x' || line[bStart+1] == 'X'))
	return idx[3], idx[5], bStart, bEnd, idx[1], isChecked, true
}

// MatchListPrefix matches a standard unordered/ordered list marker (e.g. "- ", "* ", "1. ").
func MatchListPrefix(line string) (markerStart, markerEnd, contentStart int, ok bool) {
	idx := reListPrefix.FindStringSubmatchIndex(line)
	if idx == nil {
		return 0, 0, 0, false
	}
	return idx[4], idx[5], idx[1], true
}

// ConcealLine applies markdown visual concealment to inactive lines:
// - Checkboxes: "- [ ] " -> "☐ " (U+2610), "- [x] " -> "☑ " (U+2611)
// - Links: "[title](url)" -> "title"
// - Bold/Italic/Code delimiters are removed for clean rendering.
// Active cursor lines are never concealed to allow seamless editing of raw syntax.
func ConcealLine(line string, isCursorLine bool) string {
	concealed, _ := ConcealLineWithMapping(line, isCursorLine)
	return concealed
}

// ConcealLineWithMapping applies markdown visual concealment to inactive lines
// and returns a byte-offset mapping slice rawToConcealed of length len(line)+1.
// For any byte offset b in line (0 <= b <= len(line)), rawToConcealed[b] gives
// the corresponding byte offset in the returned concealed string.
// If no concealment is performed (e.g. isCursorLine is true), rawToConcealed is nil.
func ConcealLineWithMapping(line string, isCursorLine bool) (string, []int) {
	if isCursorLine || line == "" {
		return line, nil
	}
	if isOpen, _, _, _ := ParseOpeningFence(line); isOpen {
		return line, nil
	}

	cb := concealBuilder{
		rawToConcealed: make([]int, len(line)+1),
	}
	cb.buf.Grow(len(line))

	startPos := 0
	if prefixEnd, space1End, boxStart, boxEnd, contentStart, checked, ok := MatchTaskPrefix(line); ok {
		cb.keepRange(line, 0, prefixEnd)
		cb.replaceRange(prefixEnd, space1End, " ")
		if checked {
			cb.replaceRange(boxStart, boxEnd, "☑")
		} else {
			cb.replaceRange(boxStart, boxEnd, "☐")
		}
		cb.replaceRange(boxEnd, contentStart, " ")
		startPos = contentStart
	}

	cb.concealInline(line, startPos, len(line))
	return cb.buf.String(), cb.rawToConcealed
}

// MapSpan maps a half-open byte span [startByte, endByte) from the raw line
// into the corresponding half-open byte span in the concealed line.
func MapSpan(rawToConcealed []int, startByte, endByte int) (int, int) {
	if rawToConcealed == nil {
		return startByte, endByte
	}
	maxIdx := len(rawToConcealed) - 1
	if startByte < 0 {
		startByte = 0
	} else if startByte > maxIdx {
		startByte = maxIdx
	}
	if endByte < 0 {
		endByte = 0
	} else if endByte > maxIdx {
		endByte = maxIdx
	}
	return rawToConcealed[startByte], rawToConcealed[endByte]
}

type concealBuilder struct {
	buf            strings.Builder
	rawToConcealed []int
}

func (c *concealBuilder) keepByte(line string, idx int) {
	c.rawToConcealed[idx] = c.buf.Len()
	c.buf.WriteByte(line[idx])
	c.rawToConcealed[idx+1] = c.buf.Len()
}

func (c *concealBuilder) keepRange(line string, start, end int) {
	for i := start; i < end; i++ {
		c.rawToConcealed[i] = c.buf.Len()
		c.buf.WriteByte(line[i])
	}
	c.rawToConcealed[end] = c.buf.Len()
}

func (c *concealBuilder) concealRange(start, end int) {
	cur := c.buf.Len()
	for i := start; i <= end; i++ {
		c.rawToConcealed[i] = cur
	}
}

func (c *concealBuilder) replaceRange(start, end int, replacement string) {
	startOut := c.buf.Len()
	c.buf.WriteString(replacement)
	endOut := c.buf.Len()
	for i := start; i < end; i++ {
		c.rawToConcealed[i] = startOut
	}
	c.rawToConcealed[end] = endOut
}

func (c *concealBuilder) concealInline(line string, start, end int) {
	c.rawToConcealed[start] = c.buf.Len()
	for i := start; i < end; {
		ch := line[i]

		// 1. Escaped markdown punctuation: keep literal
		if ch == '\\' && i+1 < end && IsMarkdownPunct(line[i+1]) {
			c.keepRange(line, i, i+2)
			i += 2
			continue
		}

		// 2. Inline code span (`code` or ``code``): highest inline precedence; inner text is literal
		if ch == '`' {
			tickLen := 0
			for i+tickLen < end && line[i+tickLen] == '`' {
				tickLen++
			}
			closeIdx := FindClosingBackticks(line, i+tickLen, end, tickLen)
			if closeIdx > i+tickLen {
				c.concealRange(i, i+tickLen)
				c.keepRange(line, i+tickLen, closeIdx)
				c.concealRange(closeIdx, closeIdx+tickLen)
				i = closeIdx + tickLen
				continue
			}
			c.keepRange(line, i, i+tickLen)
			i += tickLen
			continue
		}

		// 3. Links: [title](url) -> title
		if ch == '[' {
			if titleStart, titleEnd, _, _, linkEnd, ok := MatchLink(line, i, end); ok {
				c.concealRange(i, titleStart)
				c.concealInline(line, titleStart, titleEnd)
				c.concealRange(titleEnd, linkEnd)
				i = linkEnd
				continue
			}
		}

		// 4. Bold (**text** or __text__) and Italic (*text* or _text_)
		if ch == '*' || ch == '_' {
			run := 0
			for i+run < end && line[i+run] == ch {
				run++
			}
			if run == 2 {
				if closeIdx, ok := MatchEmphasis(line, i, end, ch, 2); ok {
					c.concealRange(i, i+2)
					c.concealInline(line, i+2, closeIdx)
					c.concealRange(closeIdx, closeIdx+2)
					i = closeIdx + 2
					continue
				}
			} else if run == 1 {
				if closeIdx, ok := MatchEmphasis(line, i, end, ch, 1); ok {
					c.concealRange(i, i+1)
					c.concealInline(line, i+1, closeIdx)
					c.concealRange(closeIdx, closeIdx+1)
					i = closeIdx + 1
					continue
				}
			}
			c.keepRange(line, i, i+run)
			i += run
			continue
		}

		c.keepByte(line, i)
		i++
	}
}

// FindClosingBackticks finds a closing backtick run of exact length tickLen within line[from:end].
func FindClosingBackticks(line string, from, end, tickLen int) int {
	for j := from; j < end; {
		if line[j] == '`' {
			run := 0
			for j+run < end && line[j+run] == '`' {
				run++
			}
			if run == tickLen {
				return j
			}
			j += run
		} else {
			j++
		}
	}
	return -1
}

// MatchLink matches a markdown link [title](url) starting at '[' at index start within line[:end].
func MatchLink(line string, start, end int) (titleStart, titleEnd, urlStart, urlEnd, linkEnd int, ok bool) {
	if start >= end || line[start] != '[' {
		return 0, 0, 0, 0, 0, false
	}
	closeBracket := -1
	for j := start + 1; j < end; {
		if line[j] == '\\' && j+1 < end {
			j += 2
			continue
		}
		if line[j] == '`' {
			tickLen := 0
			for j+tickLen < end && line[j+tickLen] == '`' {
				tickLen++
			}
			if closeTick := FindClosingBackticks(line, j+tickLen, end, tickLen); closeTick >= 0 {
				j = closeTick + tickLen
				continue
			}
			j += tickLen
			continue
		}
		if line[j] == '[' {
			return 0, 0, 0, 0, 0, false
		}
		if line[j] == ']' {
			closeBracket = j
			break
		}
		j++
	}
	if closeBracket <= start+1 || closeBracket+1 >= end || line[closeBracket+1] != '(' {
		return 0, 0, 0, 0, 0, false
	}

	closeParen := -1
	parenDepth := 1
	for k := closeBracket + 2; k < end; {
		if line[k] == '\\' && k+1 < end {
			k += 2
			continue
		}
		if line[k] == '(' {
			parenDepth++
		} else if line[k] == ')' {
			parenDepth--
			if parenDepth == 0 {
				closeParen = k
				break
			}
		}
		k++
	}
	if closeParen <= closeBracket+2 {
		return 0, 0, 0, 0, 0, false
	}

	return start + 1, closeBracket, closeBracket + 2, closeParen, closeParen + 1, true
}

// MatchEmphasis matches bold (** / __) or italic (* / _) starting at start within line[:end].
// Enforces word-boundary and intraword rules so identifiers (vasmm68k_mot), globs (disasm/*.asm),
// and inline code spans are never misidentified as emphasis.
func MatchEmphasis(line string, start, end int, delimChar byte, delimLen int) (int, bool) {
	if start+delimLen >= end {
		return -1, false
	}
	if start > 0 && line[start-1] == delimChar {
		return -1, false
	}
	if start+delimLen < end && line[start+delimLen] == delimChar {
		return -1, false
	}

	// Check left context of opening delimiter
	if start > 0 {
		prevRune, _ := utf8.DecodeLastRuneInString(line[:start])
		if isWordRune(prevRune) {
			return -1, false
		}
		if !unicode.IsSpace(prevRune) && !isOpenEmphasisPunct(prevRune, delimLen) {
			return -1, false
		}
	}

	// Character immediately inside opening delimiter must not be whitespace
	firstInner, _ := utf8.DecodeRuneInString(line[start+delimLen : end])
	if firstInner == utf8.RuneError || unicode.IsSpace(firstInner) {
		return -1, false
	}

	for j := start + delimLen; j <= end-delimLen; {
		if line[j] == '\\' && j+1 < end {
			j += 2
			continue
		}
		// Never match a closing delimiter inside an inline backtick code span
		if line[j] == '`' {
			tickLen := 0
			for j+tickLen < end && line[j+tickLen] == '`' {
				tickLen++
			}
			if closeTick := FindClosingBackticks(line, j+tickLen, end, tickLen); closeTick >= 0 {
				j = closeTick + tickLen
				continue
			}
			j += tickLen
			continue
		}
		// Never match a closing delimiter inside a link URL
		if line[j] == '[' {
			if _, _, _, _, linkEnd, ok := MatchLink(line, j, end); ok {
				j = linkEnd
				continue
			}
		}

		if line[j] == delimChar {
			run := 0
			for j+run < end && line[j+run] == delimChar {
				run++
			}
			if run == delimLen && j > start+delimLen {
				lastInner, _ := utf8.DecodeLastRuneInString(line[start+delimLen : j])
				if lastInner != utf8.RuneError && !unicode.IsSpace(lastInner) && canCloseEmphasisAt(line, j, delimLen) {
					return j, true
				}
			}
			j += run
			continue
		}
		j++
	}
	return -1, false
}

func canCloseEmphasisAt(line string, closePos int, delimLen int) bool {
	afterPos := closePos + delimLen
	if afterPos >= len(line) {
		return true
	}
	nextRune, _ := utf8.DecodeRuneInString(line[afterPos:])
	if isWordRune(nextRune) {
		return false
	}
	return unicode.IsSpace(nextRune) || isCloseEmphasisPunct(nextRune)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isOpenEmphasisPunct(r rune, delimLen int) bool {
	switch r {
	case '(', '[', '{', '"', '\'', '<', '«', '“', '‘':
		return true
	case '/', '-':
		return delimLen == 2
	default:
		return false
	}
}

func isCloseEmphasisPunct(r rune) bool {
	switch r {
	case ')', ']', '}', '.', ',', ';', ':', '!', '?', '"', '\'', '>', '/', '-', '»', '”', '’':
		return true
	default:
		return false
	}
}

// IsMarkdownPunct returns true if b is an ASCII punctuation character escapable in Markdown.
func IsMarkdownPunct(b byte) bool {
	switch b {
	case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '+', '-', '.', '!', '|', '~':
		return true
	default:
		return false
	}
}
