package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFuzzyScore(t *testing.T) {
	// Empty pattern
	ok, score := FuzzyScore("", "anything")
	if !ok || score != 0 {
		t.Fatalf("expected empty pattern to match with score 0, got ok=%v, score=%d", ok, score)
	}

	// Exact match has higher score than substring
	okExact, scoreExact := FuzzyScore("main.go", "main.go")
	if !okExact {
		t.Fatal("expected exact match to succeed")
	}

	okSub, scoreSub := FuzzyScore("main", "main.go")
	if !okSub {
		t.Fatal("expected substring match to succeed")
	}

	if scoreExact <= scoreSub {
		t.Fatalf("expected exact score (%d) > substring score (%d)", scoreExact, scoreSub)
	}

	// Character ordering: "abc" should match "a_b_c" but not "cba"
	okOrder, _ := FuzzyScore("abc", "a_b_c")
	if !okOrder {
		t.Fatal("expected 'abc' to match 'a_b_c'")
	}

	okReverse, _ := FuzzyScore("abc", "cba")
	if okReverse {
		t.Fatal("expected 'abc' NOT to match 'cba'")
	}

	// Word boundary bonus: "fb" in "foo_bar" should score higher than in "afb"
	_, scoreWordBoundary := FuzzyScore("fb", "foo_bar")
	_, scoreMidWord := FuzzyScore("fb", "afbc")
	if scoreWordBoundary <= scoreMidWord {
		t.Fatalf("expected word boundary match score (%d) > mid-word score (%d)", scoreWordBoundary, scoreMidWord)
	}

	// CamelCase bonus
	_, scoreCamel := FuzzyScore("ft", "FileTree")
	_, scoreNonCamel := FuzzyScore("ft", "after")
	if scoreCamel <= scoreNonCamel {
		t.Fatalf("expected camelCase match score (%d) > non-camel score (%d)", scoreCamel, scoreNonCamel)
	}
}

func TestPickerOpenFilesAndFilter(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "buffer.go"), []byte("package buffer"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "filetree.go"), []byte("package ui"), 0644)

	p := NewPicker(tmpDir)
	p.OpenFiles(tmpDir)

	if !p.Open {
		t.Fatal("expected picker to be open")
	}
	if len(p.Items) < 3 {
		t.Fatalf("expected at least 3 items loaded, got %d", len(p.Items))
	}
	if len(p.Filtered) != len(p.Items) {
		t.Fatalf("expected initial filtered count to match items count")
	}

	// Filter with query "main"
	p.InsertChar("m")
	p.InsertChar("a")
	p.InsertChar("i")
	p.InsertChar("n")

	if len(p.Filtered) != 1 {
		t.Fatalf("expected exactly 1 match for 'main', got %d", len(p.Filtered))
	}
	item := p.SelectedItem()
	if item == nil || !strings.Contains(item.Label, "main.go") {
		t.Fatalf("expected main.go selected, got %v", item)
	}

	// Delete characters back to empty query
	p.DeleteChar()
	p.DeleteChar()
	p.DeleteChar()
	p.DeleteChar()

	if len(p.Filtered) != len(p.Items) {
		t.Fatalf("expected all items restored after deleting query, got %d", len(p.Filtered))
	}

	// Move cursor
	p.MoveCursor(1, 10)
	if p.SelectedIndex != 1 {
		t.Fatalf("expected selected index 1, got %d", p.SelectedIndex)
	}
	p.MoveCursor(-10, 10)
	if p.SelectedIndex != 0 {
		t.Fatalf("expected selected index clamped to 0, got %d", p.SelectedIndex)
	}
	p.MoveCursor(100, 10)
	if p.SelectedIndex != len(p.Filtered)-1 {
		t.Fatalf("expected selected index clamped to max, got %d", p.SelectedIndex)
	}

	// Close
	p.Close()
	if p.Open {
		t.Fatal("expected picker to be closed")
	}
}

func TestPickerOpenBuffers(t *testing.T) {
	buf1 := &BufferItem{ID: 1, FileName: "main.go", FilePath: "/tmp/main.go", Dirty: false}
	buf2 := &BufferItem{ID: 2, FileName: "app.go", FilePath: "/tmp/app.go", Dirty: true}
	buffers := []*BufferItem{buf1, buf2}

	p := NewPicker(".")
	p.OpenBuffers(buffers, 1)

	if !p.Open {
		t.Fatal("expected picker to be open")
	}
	if p.Mode != PickerModeBuffers {
		t.Fatalf("expected mode PickerModeBuffers, got %v", p.Mode)
	}
	if len(p.Items) != 2 {
		t.Fatalf("expected 2 buffer items, got %d", len(p.Items))
	}

	// Active buffer (buf2) should be prioritized at the top
	if !strings.Contains(p.Items[0].Label, "app.go") {
		t.Fatalf("expected active buffer app.go at index 0, got %s", p.Items[0].Label)
	}
	if !strings.Contains(p.Items[0].Label, "[+]") {
		t.Fatalf("expected dirty indicator [+] on app.go, got %s", p.Items[0].Label)
	}

	// Search for main
	p.InsertChar("m")
	if len(p.Filtered) != 1 || !strings.Contains(p.Filtered[0].Label, "main.go") {
		t.Fatalf("expected main.go match, got %v", p.Filtered)
	}
}

func TestPickerRender(t *testing.T) {
	p := NewPicker(".")
	p.Open = true
	p.Mode = PickerModeFiles
	p.Items = []PickerItem{
		{Label: "file1.go", Value: "/path/file1.go"},
		{Label: "file2.go", Value: "/path/file2.go"},
	}
	p.Filter()

	theme := DefaultTheme()
	rows := p.Render(80, 24, theme)

	if len(rows) == 0 {
		t.Fatal("expected non-empty rendered rows for picker modal")
	}

	// Check top border title
	topRowStripped := ansi.Strip(rows[0])
	if !strings.Contains(topRowStripped, "Files") {
		t.Errorf("expected top border to contain title 'Files', got %q", topRowStripped)
	}
	if !strings.HasPrefix(topRowStripped, "┌") || !strings.HasSuffix(topRowStripped, "┐") {
		t.Errorf("expected top border box corners ┌ and ┐, got %q", topRowStripped)
	}

	// Check query prompt in second row
	queryRowStripped := ansi.Strip(rows[1])
	if !strings.Contains(queryRowStripped, "> _") {
		t.Errorf("expected query prompt '> _', got %q", queryRowStripped)
	}

	// Check bottom border
	bottomRowStripped := ansi.Strip(rows[len(rows)-1])
	if !strings.Contains(bottomRowStripped, "Enter") {
		t.Errorf("expected bottom border instructions, got %q", bottomRowStripped)
	}
	if !strings.HasPrefix(bottomRowStripped, "└") || !strings.HasSuffix(bottomRowStripped, "┘") {
		t.Errorf("expected bottom border box corners └ and ┘, got %q", bottomRowStripped)
	}
}

func TestPickerGrepQueryThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("foo bar baz hello world\n"), 0644)

	p := NewPicker(tmpDir)
	p.OpenGrep(tmpDir)

	if !p.Open || p.Mode != PickerModeGrep {
		t.Fatal("expected picker open in grep mode")
	}

	// 1-character query: should not run search
	p.InsertChar("f")
	if len(p.Items) != 0 || len(p.Filtered) != 0 {
		t.Fatalf("expected 0 items for 1-char query 'f', got %d", len(p.Items))
	}

	// 2-character query: should run search and find matches
	p.InsertChar("o")
	if len(p.Filtered) == 0 {
		t.Fatalf("expected matches for 2-char query 'fo', got 0")
	}
	if !strings.Contains(p.Filtered[0].Label, "foo bar") {
		t.Fatalf("expected match to contain 'foo bar', got %q", p.Filtered[0].Label)
	}

	// Backspace back to 1-char: items should be cleared
	p.DeleteChar()
	if len(p.Filtered) != 0 {
		t.Fatalf("expected 0 items after deleting back to 1 char, got %d", len(p.Filtered))
	}
}

