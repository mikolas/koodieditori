package watcher

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mikolas/koodieditori/internal/buffer"
)

// ReconciliationStatus indicates the result of reconciling a file with a buffer.
type ReconciliationStatus int

const (
	ReconcileNotOpen ReconciliationStatus = iota
	ReconcileCleanReload
	ReconcileConflict
	ReconcileFileNotFound
	ReconcileError
)

func (s ReconciliationStatus) String() string {
	switch s {
	case ReconcileNotOpen:
		return "NotOpen"
	case ReconcileCleanReload:
		return "CleanReload"
	case ReconcileConflict:
		return "Conflict"
	case ReconcileFileNotFound:
		return "FileNotFound"
	case ReconcileError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ReconcileResult represents the outcome of buffer reconciliation with disk.
type ReconcileResult struct {
	Status      ReconciliationStatus
	FilePath    string
	DiskContent string
	Message     string
	Error       error
}

// ReconcileBuffer reconciles a single buffer against its on-disk state.
// If the buffer is clean (!isDirty), it replaces the PieceTable content with the disk file.
// If the buffer is dirty (isDirty), it flags a collision conflict.
func ReconcileBuffer(bufPath string, isDirty bool, pt *buffer.PieceTable) ReconcileResult {
	if bufPath == "" || pt == nil {
		return ReconcileResult{
			Status:   ReconcileNotOpen,
			FilePath: bufPath,
		}
	}

	cleanPath := filepath.Clean(bufPath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ReconcileResult{
				Status:   ReconcileFileNotFound,
				FilePath: cleanPath,
				Message:  fmt.Sprintf("File removed on disk: %s", filepath.Base(cleanPath)),
			}
		}
		return ReconcileResult{
			Status:   ReconcileError,
			FilePath: cleanPath,
			Error:    err,
			Message:  fmt.Sprintf("Failed to read %s: %v", filepath.Base(cleanPath), err),
		}
	}

	diskContent := string(data)

	// If buffer is dirty, local unsaved edits conflict with disk modification
	if isDirty {
		return ReconcileResult{
			Status:      ReconcileConflict,
			FilePath:    cleanPath,
			DiskContent: diskContent,
			Message:     fmt.Sprintf("Collision detected: %s modified on disk with unsaved local edits", filepath.Base(cleanPath)),
		}
	}

	// Buffer is clean: hot-reload from disk
	pt.BeginTransaction()
	oldLen := pt.Len()
	if oldLen > 0 {
		_ = pt.Delete(0, oldLen)
	}
	_ = pt.Insert(0, diskContent)
	pt.CommitTransaction()

	return ReconcileResult{
		Status:      ReconcileCleanReload,
		FilePath:    cleanPath,
		DiskContent: diskContent,
		Message:     fmt.Sprintf("Reloaded %s", filepath.Base(cleanPath)),
	}
}
