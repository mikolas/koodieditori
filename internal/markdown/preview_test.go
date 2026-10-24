package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

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
