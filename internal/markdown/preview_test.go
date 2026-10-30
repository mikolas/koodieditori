package markdown

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var reColorSGR = regexp.MustCompile(`\x1b\[[0-9;]*?(?:3[0-8]|4[0-8]|9[0-7]|10[0-7])(?:;[0-9]+)*m`)

func TestRenderPreview(t *testing.T) {
	doc := `# Project Roadmap

Here is a checklist:
- [x] Phase 1
- [ ] Phase 2

Here is a diagram:
` + "```mermaid" + `
flowchart LR
    A["Client"] --> B["Server"]
` + "```" + `

End of document.
`
	out, err := RenderPreview(doc, 80, MermaidModeBoxArt)
	if err != nil {
		t.Fatalf("unexpected error rendering preview: %v", err)
	}

	if !strings.Contains(out, "Project") || !strings.Contains(out, "Roadmap") {
		t.Fatal("expected 'Project' and 'Roadmap' in rendered markdown preview")
	}
	if !strings.Contains(out, "Client") || !strings.Contains(out, "Server") {
		t.Fatal("expected mermaid diagram nodes in rendered preview")
	}
}

func TestRenderPreviewMonochromeTextStylingOnly(t *testing.T) {
	doc := `# Heading One

## Heading Two

Paragraph with **bold text**, *italic text*, ` + "`inline_code()`" + `, and a [Link Title](https://example.com).

> A blockquote line

` + "```go" + `
package main

func main() {
	println("hello")
}
` + "```" + `
`
	out, err := RenderPreview(doc, 80, MermaidModeBoxArt)
	if err != nil {
		t.Fatalf("unexpected error rendering preview: %v", err)
	}

	// Verify no foreground or background color SGR codes are emitted
	if match := reColorSGR.FindString(out); match != "" {
		t.Fatalf("expected monochrome preview without color SGR sequences, found %q in:\n%q", match, out)
	}

	// Verify typographic text styling (bold, italic, underline) is applied
	if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[1;4m") && !strings.Contains(out, "\x1b[4;1m") {
		t.Errorf("expected bold SGR styling in preview output, got:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[3m") {
		t.Errorf("expected italic SGR styling in preview output, got:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[4m") && !strings.Contains(out, "\x1b[1;4m") && !strings.Contains(out, "\x1b[4;1m") {
		t.Errorf("expected underline SGR styling in preview output, got:\n%q", out)
	}
}

func TestRenderPreviewSanitizesTabsAndVariationSelectors(t *testing.T) {
	doc := "# ⌨️⚡ Koodieditori\r\n\r\n```toml\r\n[languages.go]\r\n\tlsp_command = \"gopls\"\r\n\t\tdeep_tab = true\r\n```\r\n"
	const width = 70
	out, err := RenderPreview(doc, width, MermaidModeBoxArt)
	if err != nil {
		t.Fatalf("unexpected error rendering preview: %v", err)
	}

	if strings.ContainsAny(out, "\t\r\uFE0E\uFE0F") {
		t.Fatalf("expected no tabs, CRs, or variation selectors in preview output, got: %q", out)
	}

	for i, line := range strings.Split(out, "\n") {
		aw := ansi.StringWidth(line)
		rw := runewidth.StringWidth(ansi.Strip(line))
		if aw > width {
			t.Errorf("line %d exceeds max width %d: ansi.StringWidth=%d", i, width, aw)
		}
		if aw != rw {
			t.Errorf("line %d width mismatch between uniseg (%d) and runewidth (%d): %q", i, aw, rw, ansi.Strip(line))
		}
	}
}
