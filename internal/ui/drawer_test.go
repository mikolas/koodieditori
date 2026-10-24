package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func TestDrawerLifecycleAndContent(t *testing.T) {
	d := NewDrawer()
	if d.Open {
		t.Fatal("expected drawer initially closed")
	}

	output := `Building test...
src/main.cpp:10:2: error: unknown type
src/main.cpp:20: warning: unused var
Build finished.`

	d.SetContent(output)
	if len(d.Content) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(d.Content))
	}
	if len(d.ParsedErrors) != 2 {
		t.Fatalf("expected 2 parsed errors, got %d", len(d.ParsedErrors))
	}

	// Toggle open
	d.Toggle()
	if !d.Open {
		t.Fatal("expected drawer open after toggle")
	}

	// Move cursor
	d.MoveCursor(1, 4)
	if d.CursorLine != 1 {
		t.Fatalf("expected cursor at line 1, got %d", d.CursorLine)
	}

	diag := d.CurrentDiagnostic()
	if diag == nil || diag.Line != 9 {
		t.Fatalf("expected diagnostic at line 9 (0-indexed), got %+v", diag)
	}

	// Maximize and restore
	d.ToggleMaximize()
	if !d.Maximized {
		t.Fatal("expected maximized")
	}
	d.ToggleMaximize()
	if d.Maximized {
		t.Fatal("expected unmaximized")
	}

	// Resize
	origHeight := d.Height
	d.Grow(3)
	if d.Height != origHeight+3 {
		t.Fatalf("expected height %d, got %d", origHeight+3, d.Height)
	}
	d.Shrink(100)
	if d.Height != 3 {
		t.Fatalf("expected height clamped to 3, got %d", d.Height)
	}

	// Close
	d.Close()
	if d.Open {
		t.Fatal("expected drawer closed")
	}
}

func TestDrawerRender(t *testing.T) {
	theme := DefaultTheme()
	d := NewDrawer()
	d.SetContent("src/foo.cpp:5:1: error: boom\nregular log line")
	d.Open = true

	rendered := d.Render(60, 4, theme, true)
	if !strings.Contains(rendered, "Output & Diagnostics") {
		t.Fatal("expected title in rendered header")
	}
	if !strings.Contains(rendered, "boom") {
		t.Fatal("expected error text in rendered body")
	}

	// Maximized title
	d.Maximized = true
	maxRendered := d.Render(60, 4, theme, true)
	if !strings.Contains(maxRendered, "[MAXIMIZED]") {
		t.Fatal("expected [MAXIMIZED] in header")
	}
}

func TestDrawerRenderExactWidthAndScroll(t *testing.T) {
	theme := DefaultTheme()
	d := NewDrawer()
	d.Mode = DrawerModeMarkdown
	d.Title = "Markdown Preview (Esc/q to close, Z to maximize)"
	d.SetContent("# ⌨️⚡ Heading\n\tindented with tab\n\t\tdouble tab line with \x1b[38;5;212mANSI\x1b[0m\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10")
	d.Open = true

	for _, width := range []int{30, 60, 94, 120} {
		for _, maximized := range []bool{false, true} {
			d.Maximized = maximized
			out := d.Render(width, 5, theme, true)
			rows := strings.Split(out, "\n")
			if len(rows) != 6 {
				t.Fatalf("expected 6 rows (1 header + 5 content), got %d", len(rows))
			}
			for i, row := range rows {
				if strings.ContainsAny(row, "\t\r\uFE0E\uFE0F") {
					t.Fatalf("width=%d max=%v row=%d contains forbidden characters: %q", width, maximized, i, row)
				}
				aw := ansi.StringWidth(row)
				rw := runewidth.StringWidth(ansi.Strip(row))
				if aw != width || rw != width {
					t.Fatalf("width=%d max=%v row=%d expected width %d, got ansi=%d runewidth=%d: %q", width, maximized, i, width, aw, rw, ansi.Strip(row))
				}
			}
		}
	}

	// Verify immediate directional Scroll
	d.ScrollY = 0
	d.CursorLine = 0
	d.Scroll(3, 4)
	if d.ScrollY != 3 {
		t.Fatalf("expected ScrollY=3 after Scroll(3, 4), got %d", d.ScrollY)
	}
	d.Scroll(-2, 4)
	if d.ScrollY != 1 {
		t.Fatalf("expected ScrollY=1 after Scroll(-2, 4), got %d", d.ScrollY)
	}
}
