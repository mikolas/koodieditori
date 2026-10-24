package buffer

// BeginTransaction marks the start of an atomic edit transaction (e.g. typing a continuous word).
func (pt *PieceTable) BeginTransaction() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.inTx {
		pt.inTx = true
		pt.txPieces = clonePieces(pt.pieces)
	}
}

// CommitTransaction finalizes the current atomic edit transaction.
func (pt *PieceTable) CommitTransaction() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.inTx {
		if !piecesEqual(pt.txPieces, pt.pieces) {
			pt.undoStack = append(pt.undoStack, undoEntry{pieces: pt.txPieces})
			pt.redoStack = nil // Clear redo on new edit
		}
		pt.inTx = false
		pt.txPieces = nil
	}
}

// CanUndo returns true if there are actions to undo.
func (pt *PieceTable) CanUndo() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.undoStack) > 0 || (pt.inTx && !piecesEqual(pt.txPieces, pt.pieces))
}

// CanRedo returns true if there are actions to redo.
func (pt *PieceTable) CanRedo() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.redoStack) > 0
}

// Undo reverts the buffer to the previous state.
func (pt *PieceTable) Undo() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// If currently in a transaction, commit or discard it first
	if pt.inTx {
		if !piecesEqual(pt.txPieces, pt.pieces) {
			pt.redoStack = append(pt.redoStack, undoEntry{pieces: clonePieces(pt.pieces)})
			pt.pieces = pt.txPieces
			pt.inTx = false
			pt.txPieces = nil
			return true
		}
		pt.inTx = false
		pt.txPieces = nil
	}

	if len(pt.undoStack) == 0 {
		return false
	}

	// Pop from undo stack
	lastIdx := len(pt.undoStack) - 1
	prev := pt.undoStack[lastIdx]
	pt.undoStack = pt.undoStack[:lastIdx]

	// Push current state to redo stack
	pt.redoStack = append(pt.redoStack, undoEntry{pieces: clonePieces(pt.pieces)})

	// Restore state
	pt.pieces = prev.pieces
	return true
}

// Redo reapplies the last undone edit.
func (pt *PieceTable) Redo() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if len(pt.redoStack) == 0 {
		return false
	}

	// Pop from redo stack
	lastIdx := len(pt.redoStack) - 1
	next := pt.redoStack[lastIdx]
	pt.redoStack = pt.redoStack[:lastIdx]

	// Push current state to undo stack
	pt.undoStack = append(pt.undoStack, undoEntry{pieces: clonePieces(pt.pieces)})

	// Restore state
	pt.pieces = next.pieces
	return true
}

func (pt *PieceTable) saveUndoSnapshot() {
	if pt.inTx {
		return
	}
	pt.undoStack = append(pt.undoStack, undoEntry{pieces: clonePieces(pt.pieces)})
	pt.redoStack = nil // Clear redo on new modification
}

func clonePieces(src []Piece) []Piece {
	dst := make([]Piece, len(src))
	copy(dst, src)
	return dst
}

func piecesEqual(a, b []Piece) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
