package ui

import (
	"strings"
	"testing"
)

func TestHelpModalLifecycleAndFiltering(t *testing.T) {
	h := NewHelpModal()
	if h.Open {
		t.Fatalf("expected HelpModal to be closed initially")
	}
	if len(h.Items) < 30 {
		t.Fatalf("expected at least 30 default help items, got %d", len(h.Items))
	}

	h.OpenModal()
	if !h.Open {
		t.Fatalf("expected HelpModal to be open after OpenModal()")
	}
	if len(h.Filtered) != len(h.Items) {
		t.Fatalf("expected Filtered to match Items on open, got %d vs %d", len(h.Filtered), len(h.Items))
	}

	// Filter by "breakpoint"
	h.InsertChar("b")
	h.InsertChar("r")
	h.InsertChar("e")
	h.InsertChar("a")
	h.InsertChar("k")
	if len(h.Filtered) == 0 {
		t.Fatalf("expected matches for 'break', got 0")
	}
	found := false
	for _, it := range h.Filtered {
		if strings.Contains(strings.ToLower(it.Title), "breakpoint") || strings.Contains(strings.ToLower(it.Description), "breakpoint") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected breakpoint item in filtered results")
	}

	// Delete characters back
	for i := 0; i < 5; i++ {
		h.DeleteChar()
	}
	if h.Query != "" {
		t.Fatalf("expected empty query after deletes, got %q", h.Query)
	}
	if len(h.Filtered) != len(h.Items) {
		t.Fatalf("expected full list restored after clearing query, got %d", len(h.Filtered))
	}

	// Filter by nonexistent string
	h.InsertChar("z")
	h.InsertChar("z")
	h.InsertChar("q")
	h.InsertChar("q")
	if len(h.Filtered) != 0 {
		t.Fatalf("expected 0 matches for 'zzqq', got %d", len(h.Filtered))
	}
	if h.SelectedItem() != nil {
		t.Fatalf("expected SelectedItem to be nil when filtered list is empty")
	}

	h.Close()
	if h.Open {
		t.Fatalf("expected HelpModal to be closed after Close()")
	}
}

func TestHelpModalNavigation(t *testing.T) {
	h := NewHelpModal()
	h.OpenModal()

	// Initial selection
	if h.SelectedIndex != 0 {
		t.Fatalf("expected SelectedIndex 0, got %d", h.SelectedIndex)
	}
	initial := h.SelectedItem()
	if initial == nil {
		t.Fatalf("expected non-nil SelectedItem")
	}

	// Next
	h.Next()
	if h.SelectedIndex != 1 {
		t.Fatalf("expected SelectedIndex 1, got %d", h.SelectedIndex)
	}

	// Prev
	h.Prev()
	if h.SelectedIndex != 0 {
		t.Fatalf("expected SelectedIndex 0 after Prev, got %d", h.SelectedIndex)
	}

	// Prev wrapping to bottom
	h.Prev()
	if h.SelectedIndex != len(h.Filtered)-1 {
		t.Fatalf("expected SelectedIndex %d after wrap, got %d", len(h.Filtered)-1, h.SelectedIndex)
	}

	// Next wrapping back to top
	h.Next()
	if h.SelectedIndex != 0 {
		t.Fatalf("expected SelectedIndex 0 after next wrap, got %d", h.SelectedIndex)
	}

	// PageDown & PageUp
	h.PageDown(5)
	if h.SelectedIndex != 5 {
		t.Fatalf("expected SelectedIndex 5 after PageDown, got %d", h.SelectedIndex)
	}
	h.PageUp(3)
	if h.SelectedIndex != 2 {
		t.Fatalf("expected SelectedIndex 2 after PageUp, got %d", h.SelectedIndex)
	}
}

func TestHelpModalRender(t *testing.T) {
	theme := DefaultTheme()
	h := NewHelpModal()

	// When closed, returns nil
	rows := h.Render(80, 24, theme)
	if rows != nil {
		t.Fatalf("expected nil render output when closed")
	}

	h.OpenModal()
	rows = h.Render(80, 24, theme)
	if len(rows) == 0 {
		t.Fatalf("expected non-empty rows when open")
	}

	// Verify top border contains title
	if !strings.Contains(rows[0], "Help & Keyboard Reference") {
		t.Fatalf("top border does not contain title: %q", rows[0])
	}

	// Verify search bar row
	if !strings.Contains(rows[1], "Search help:") {
		t.Fatalf("row 1 does not contain search bar prompt: %q", rows[1])
	}

	// Verify header divider row
	if !strings.Contains(rows[2], "CHORD / KEY") {
		t.Fatalf("row 2 does not contain column headers: %q", rows[2])
	}

	// Verify footer instructions row
	footerIdx := len(rows) - 2
	if !strings.Contains(rows[footerIdx], "Navigate") || !strings.Contains(rows[footerIdx], "Execute") {
		t.Fatalf("footer row does not contain instructions: %q", rows[footerIdx])
	}
}
