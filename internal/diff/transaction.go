package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileStatus represents git-like status for a file in a transaction.
type FileStatus string

const (
	StatusModified FileStatus = "[M]"
	StatusAdded    FileStatus = "[A]"
	StatusDeleted  FileStatus = "[D]"
)

// FileChange represents a single file altered in an agent transaction.
type FileChange struct {
	FilePath   string
	Status     FileStatus
	OldContent string
	NewContent string
	Diff       DiffResult
}

// Summary returns a compact status representation like "[M] src/engine.cpp (+30/-8)".
func (fc FileChange) Summary() string {
	relPath := fc.FilePath
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, fc.FilePath); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		}
	}
	return fmt.Sprintf("%s %s %s", fc.Status, relPath, fc.Diff.FormatSummary())
}

// AgentTransaction groups a batch of file modifications performed by an agent.
type AgentTransaction struct {
	ID        string
	Timestamp time.Time
	Files     []FileChange
	Selected  int
	mu        sync.RWMutex
}

// NewAgentTransaction creates an empty AgentTransaction.
func NewAgentTransaction() *AgentTransaction {
	return &AgentTransaction{
		ID:        fmt.Sprintf("tx-%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Files:     make([]FileChange, 0),
		Selected:  0,
	}
}

// AddChange records a file modification into the transaction.
func (tx *AgentTransaction) AddChange(path string, oldContent, newContent string) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	status := StatusModified
	if len(oldContent) == 0 && len(newContent) > 0 {
		status = StatusAdded
	} else if len(oldContent) > 0 && len(newContent) == 0 {
		status = StatusDeleted
	}

	d := ComputeDiff(oldContent, newContent)

	// Check if file already exists in transaction
	for i, f := range tx.Files {
		if filepath.Clean(f.FilePath) == filepath.Clean(path) {
			tx.Files[i].NewContent = newContent
			tx.Files[i].Status = status
			tx.Files[i].Diff = d
			return
		}
	}

	tx.Files = append(tx.Files, FileChange{
		FilePath:   path,
		Status:     status,
		OldContent: oldContent,
		NewContent: newContent,
		Diff:       d,
	})
}

// CompactSummary returns the pipeline-delimited single line summary:
// e.g. "[M] src/engine.cpp (+30/-8) | [M] include/engine.hpp (+4/-0)"
func (tx *AgentTransaction) CompactSummary() string {
	tx.mu.RLock()
	defer tx.mu.RUnlock()

	if len(tx.Files) == 0 {
		return "No changes in transaction"
	}

	parts := make([]string, len(tx.Files))
	for i, f := range tx.Files {
		parts[i] = f.Summary()
	}
	return strings.Join(parts, " | ")
}

// SelectedFile returns the currently selected FileChange, if any.
func (tx *AgentTransaction) SelectedFile() *FileChange {
	tx.mu.RLock()
	defer tx.mu.RUnlock()

	if len(tx.Files) == 0 || tx.Selected < 0 || tx.Selected >= len(tx.Files) {
		return nil
	}
	f := tx.Files[tx.Selected]
	return &f
}

// SelectNext moves the selection cursor to the next changed file.
func (tx *AgentTransaction) SelectNext() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if len(tx.Files) == 0 {
		return
	}
	tx.Selected = (tx.Selected + 1) % len(tx.Files)
}

// SelectPrev moves the selection cursor to the previous changed file.
func (tx *AgentTransaction) SelectPrev() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if len(tx.Files) == 0 {
		return
	}
	tx.Selected = (tx.Selected - 1 + len(tx.Files)) % len(tx.Files)
}

// RevertAll reverts all files in the transaction on disk back to their OldContent.
func (tx *AgentTransaction) RevertAll() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	for _, f := range tx.Files {
		if f.Status == StatusAdded {
			_ = os.Remove(f.FilePath)
		} else {
			if err := os.WriteFile(f.FilePath, []byte(f.OldContent), 0644); err != nil {
				return err
			}
		}
	}
	tx.Files = nil
	tx.Selected = 0
	return nil
}

// AcceptAll finalizes all changes and clears the pending transaction.
func (tx *AgentTransaction) AcceptAll() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	tx.Files = nil
	tx.Selected = 0
}

// FileCount returns the number of files in the transaction.
func (tx *AgentTransaction) FileCount() int {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return len(tx.Files)
}
