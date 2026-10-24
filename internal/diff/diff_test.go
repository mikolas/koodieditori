package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeDiff(t *testing.T) {
	oldText := "line 1\nline 2\nline 3\n"
	newText := "line 1\nline 2 modified\nline 3\nline 4\n"

	res := ComputeDiff(oldText, newText)

	if res.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", res.Additions)
	}
	if res.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", res.Deletions)
	}

	unified := res.UnifiedText()
	if !strings.Contains(unified, "+ line 2 modified") {
		t.Errorf("expected '+ line 2 modified' in unified diff, got:\n%s", unified)
	}
	if !strings.Contains(unified, "- line 2") {
		t.Errorf("expected '- line 2' in unified diff, got:\n%s", unified)
	}
	if !strings.Contains(unified, "+ line 4") {
		t.Errorf("expected '+ line 4' in unified diff, got:\n%s", unified)
	}
}

func TestAgentTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.go")
	file2 := filepath.Join(tmpDir, "file2.go")

	_ = os.WriteFile(file1, []byte("package main\n\nfunc main() {}\n"), 0644)
	_ = os.WriteFile(file2, []byte("package main\n"), 0644)

	tx := NewAgentTransaction()
	tx.AddChange(file1, "package main\n", "package main\n\nfunc main() {}\n")
	tx.AddChange(file2, "", "package main\n")

	if tx.FileCount() != 2 {
		t.Fatalf("expected 2 files in transaction, got %d", tx.FileCount())
	}

	summary := tx.CompactSummary()
	if !strings.Contains(summary, "[M]") || !strings.Contains(summary, "[A]") {
		t.Fatalf("expected summary to contain [M] and [A], got: %s", summary)
	}

	// Cycling
	f0 := tx.SelectedFile()
	if f0 == nil || f0.FilePath != file1 {
		t.Fatalf("expected file1 selected initially, got %v", f0)
	}
	tx.SelectNext()
	f1 := tx.SelectedFile()
	if f1 == nil || f1.FilePath != file2 {
		t.Fatalf("expected file2 selected after next, got %v", f1)
	}

	// Revert all
	if err := tx.RevertAll(); err != nil {
		t.Fatalf("RevertAll failed: %v", err)
	}
	if tx.FileCount() != 0 {
		t.Fatalf("expected 0 files after RevertAll, got %d", tx.FileCount())
	}

	// file1 should be reverted to "package main\n"
	content1, _ := os.ReadFile(file1)
	if string(content1) != "package main\n" {
		t.Fatalf("expected file1 reverted, got %q", string(content1))
	}
	// file2 was added, so it should be deleted
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Fatalf("expected added file2 to be removed on revert")
	}
}
