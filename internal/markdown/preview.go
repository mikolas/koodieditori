package markdown

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// reTrailingMarginSpaces matches Glamour's trailing foreground-only space padding
// (e.g. repeated "\x1b[38;5;252m \x1b[0m" or plain spaces at end of line).
var reTrailingMarginSpaces = regexp.MustCompile(`(?:(?:\x1b\[(?:0|38;[0-9;]+|3[0-7]|9[0-7])m)* +(?:\x1b\[0m)*)+$`)

// RenderPreview renders markdown content into terminal ANSI text using Glamour,
// automatically rendering any embedded ```mermaid ``` code blocks into box-art diagrams.
func RenderPreview(content string, width int, mermaidMode MermaidMode) (string, error) {
	if width <= 0 {
		width = 80
	}

	normalized := preprocessMarkdownInput(content, 4)

	// Replace ```mermaid code blocks with pre-rendered diagram text
	processed := reMermaidBlock.ReplaceAllStringFunc(normalized, func(match string) string {
		sub := reMermaidBlock.FindStringSubmatch(match)
		if len(sub) > 1 {
			diagramCode := sub[1]
			rendered := RenderMermaid(diagramCode, mermaidMode, width-4)
			return "```text\n" + rendered + "\n```"
		}
		return match
	})

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return sanitizePreviewOutput(processed, width, 4), err
	}

	out, err := r.Render(processed)
	if err != nil {
		return sanitizePreviewOutput(processed, width, 4), err
	}

	return sanitizePreviewOutput(out, width, 4), nil
}

// preprocessMarkdownInput normalizes line endings, strips emoji variation selectors
// that cause width disagreements between runewidth and uniseg/Kitty, and expands tabs.
func preprocessMarkdownInput(content string, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		var sb strings.Builder
		sb.Grow(len(line))
		col := 0
		for _, ch := range line {
			if ch == '\uFE0E' || ch == '\uFE0F' {
				continue
			}
			if ch == '\t' {
				spaces := tabWidth - (col % tabWidth)
				sb.WriteString(strings.Repeat(" ", spaces))
				col += spaces
				continue
			}
			if ch < 0x20 && ch != '\x1b' {
				continue
			}
			sb.WriteRune(ch)
			w := runewidth.RuneWidth(ch)
			if w > 0 {
				col += w
			}
		}
		lines[i] = sb.String()
	}
	return strings.Join(lines, "\n")
}

// sanitizePreviewOutput cleans each rendered line from Glamour so it never contains
// unexpanded tabs, carriage returns, variation selectors, or excessive trailing ANSI padding.
func sanitizePreviewOutput(out string, maxWidth int, tabWidth int) string {
	out = strings.TrimRight(out, "\r\n")
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = sanitizePreviewLine(line, maxWidth, tabWidth)
	}
	return strings.Join(lines, "\n")
}

func sanitizePreviewLine(line string, maxWidth int, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = 4
	}

	var sb strings.Builder
	sb.Grow(len(line))
	col := 0
	inEsc := false
	inOSC := false

	for i := 0; i < len(line); {
		b := line[i]
		if inOSC {
			sb.WriteByte(b)
			if b == '\x07' {
				inOSC = false
			} else if b == '\x1b' && i+1 < len(line) && line[i+1] == '\\' {
				sb.WriteByte(line[i+1])
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
			if i+1 < len(line) {
				next := line[i+1]
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

		// Decode UTF-8 rune
		r, size := decodeRuneInString(line[i:])
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

	cleaned := sb.String()
	// Strip Glamour's redundant trailing foreground-only space padding
	cleaned = reTrailingMarginSpaces.ReplaceAllString(cleaned, "")
	if maxWidth > 0 && ansi.StringWidth(cleaned) > maxWidth {
		cleaned = ansi.Truncate(cleaned, maxWidth, "")
	}
	if strings.Contains(cleaned, "\x1b") && !strings.HasSuffix(cleaned, "\x1b[0m") {
		cleaned += "\x1b[0m"
	}
	return cleaned
}

func decodeRuneInString(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 1
}
