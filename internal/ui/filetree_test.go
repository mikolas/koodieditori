package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFileTreeInitAndExpand(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	_ = os.MkdirAll(srcDir, 0755)
	file1 := filepath.Join(srcDir, "main.go")
	_ = os.WriteFile(file1, []byte("package main\n"), 0644)
	file2 := filepath.Join(tmpDir, "README.md")
	_ = os.WriteFile(file2, []byte("# Readme\n"), 0644)

	ft := NewFileTree(tmpDir, 26)
	if ft.RootNode == nil {
		t.Fatal("expected non-nil root node")
	}

	// Initially, root is expanded
	initialCount := len(ft.VisibleNodes)
	if initialCount == 0 {
		t.Fatal("expected visible nodes in tree")
	}

	// Find the src directory node and toggle expand
	srcIdx := -1
	for i, n := range ft.VisibleNodes {
		if n.Name == "src" && n.IsDir {
			srcIdx = i
			break
		}
	}
	if srcIdx == -1 {
		t.Fatalf("could not find src directory in visible nodes: %v", ft.VisibleNodes)
	}

	ft.CursorIndex = srcIdx
	path, isFile := ft.ToggleExpand()
	if isFile || path != "" {
		t.Fatalf("expected directory toggle, got path=%q, isFile=%v", path, isFile)
	}

	// After expanding src, main.go should now be visible
	foundMain := false
	for _, n := range ft.VisibleNodes {
		if n.Name == "main.go" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Fatal("expected main.go visible after expanding src")
	}

	// Toggle expand again to collapse
	ft.CursorIndex = srcIdx
	ft.ToggleExpand()
	foundMainAfterCollapse := false
	for _, n := range ft.VisibleNodes {
		if n.Name == "main.go" {
			foundMainAfterCollapse = true
			break
		}
	}
	if foundMainAfterCollapse {
		t.Fatal("expected main.go hidden after collapsing src")
	}
}

func TestFileTreeCreateAndDelete(t *testing.T) {
	tmpDir := t.TempDir()
	ft := NewFileTree(tmpDir, 26)

	// Create new file
	createdPath, err := ft.CreateFile("hello.go")
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	if _, err := os.Stat(createdPath); os.IsNotExist(err) {
		t.Fatalf("expected created file to exist on disk: %s", createdPath)
	}

	// Verify file is in visible nodes
	found := false
	for i, n := range ft.VisibleNodes {
		if n.Name == "hello.go" {
			found = true
			ft.CursorIndex = i
			break
		}
	}
	if !found {
		t.Fatal("expected hello.go in visible nodes")
	}

	// Delete file
	if err := ft.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected failed: %v", err)
	}
	if _, err := os.Stat(createdPath); !os.IsNotExist(err) {
		t.Fatal("expected deleted file to be removed from disk")
	}
}

func TestFileTreeRender(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte("package main"), 0644)

	ft := NewFileTree(tmpDir, 26)
	theme := DefaultTheme()

	renderedRows := ft.Render(10, theme, true)
	if len(renderedRows) != 10 {
		t.Fatalf("expected 10 rendered rows, got %d", len(renderedRows))
	}

	for i, row := range renderedRows {
		width := ansi.StringWidth(row)
		if width != 26 {
			t.Errorf("row %d width = %d, expected 26: %q", i, width, row)
		}
		if !strings.HasSuffix(ansi.Strip(row), "│") {
			t.Errorf("row %d missing border character │: %q", i, row)
		}
	}
}

func TestFileTreeResizeAndCursor(t *testing.T) {
	ft := NewFileTree(".", 26)
	initialWidth := ft.Width

	ft.Grow(5)
	if ft.Width != initialWidth+5 {
		t.Fatalf("expected width %d, got %d", initialWidth+5, ft.Width)
	}

	ft.Shrink(20)
	if ft.Width < 15 {
		t.Fatalf("expected width clamped to minimum 15, got %d", ft.Width)
	}

	// Cursor clamping
	ft.MoveCursor(-100, 10)
	if ft.CursorIndex != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", ft.CursorIndex)
	}

	ft.CursorToBottom(10)
	if ft.CursorIndex != len(ft.VisibleNodes)-1 {
		t.Fatalf("expected cursor at bottom, got %d (len %d)", ft.CursorIndex, len(ft.VisibleNodes))
	}

	ft.CursorToTop()
	if ft.CursorIndex != 0 {
		t.Fatalf("expected cursor at top (0), got %d", ft.CursorIndex)
	}
}

func TestFileTreeCollapseOrParent(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "pkg")
	_ = os.MkdirAll(subDir, 0755)
	file := filepath.Join(subDir, "code.go")
	_ = os.WriteFile(file, []byte("package pkg\n"), 0644)

	ft := NewFileTree(tmpDir, 26)

	// Find pkg dir and expand it
	pkgIdx := -1
	for i, n := range ft.VisibleNodes {
		if n.Name == "pkg" {
			pkgIdx = i
			break
		}
	}
	if pkgIdx == -1 {
		t.Fatalf("could not find pkg in visible nodes: %v", ft.VisibleNodes)
	}

	ft.CursorIndex = pkgIdx
	ft.ToggleExpand()

	// Locate code.go inside pkg
	codeIdx := -1
	for i, n := range ft.VisibleNodes {
		if n.Name == "code.go" {
			codeIdx = i
			break
		}
	}
	if codeIdx == -1 {
		t.Fatal("expected code.go visible after expanding pkg")
	}

	// When on code.go, CollapseOrParent should jump cursor to parent (pkg)
	ft.CursorIndex = codeIdx
	ft.CollapseOrParent()
	if ft.SelectedNode().Name != "pkg" {
		t.Fatalf("expected cursor moved to parent 'pkg', got %q", ft.SelectedNode().Name)
	}

	// When on pkg (which is expanded), CollapseOrParent should collapse it
	ft.CollapseOrParent()
	if ft.SelectedNode().Expanded {
		t.Fatal("expected pkg to be collapsed after CollapseOrParent")
	}
}
