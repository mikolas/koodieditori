package buffer

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"
)

// BufferType indicates whether a piece points to the original or add buffer.
type BufferType uint8

const (
	BufferOriginal BufferType = iota
	BufferAdd
)

// Piece represents a contiguous slice of text in either the original or add buffer.
type Piece struct {
	Buffer BufferType
	Start  int // Byte offset in the respective buffer
	Length int // Byte length in the respective buffer
	Runes  int // Number of UTF-8 runes in this piece
	Lines  int // Number of newline ('\n') characters in this piece
}

// Position represents a 0-indexed (Line, Col) cursor or coordinate in runes.
type Position struct {
	Line int
	Col  int
}

// PieceTable is a high-performance text buffer implementation.
type PieceTable struct {
	mu sync.RWMutex

	original []byte
	add      []byte
	pieces   []Piece

	// Undo/Redo manager
	undoStack []undoEntry
	redoStack []undoEntry
	inTx      bool
	txPieces  []Piece

	// Read-only indicator
	readOnly bool
}

type undoEntry struct {
	pieces []Piece
}

// NewPieceTable creates an empty PieceTable.
func NewPieceTable() *PieceTable {
	return NewPieceTableFromBytes(nil)
}

// NewPieceTableFromString creates a PieceTable populated with initial text.
func NewPieceTableFromString(initialText string) *PieceTable {
	return NewPieceTableFromBytes([]byte(initialText))
}

// NewPieceTableFromBytes creates a PieceTable with initial byte content.
func NewPieceTableFromBytes(initialBytes []byte) *PieceTable {
	pt := &PieceTable{
		original: make([]byte, len(initialBytes)),
		add:      make([]byte, 0, 1024),
		pieces:   make([]Piece, 0, 16),
	}
	copy(pt.original, initialBytes)

	if len(initialBytes) > 0 {
		runes := utf8.RuneCount(initialBytes)
		lines := bytes.Count(initialBytes, []byte{'\n'})
		pt.pieces = append(pt.pieces, Piece{
			Buffer: BufferOriginal,
			Start:  0,
			Length: len(initialBytes),
			Runes:  runes,
			Lines:  lines,
		})
	}

	return pt
}

// SetReadOnly marks the buffer as read-only.
func (pt *PieceTable) SetReadOnly(ro bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.readOnly = ro
}

// IsReadOnly returns whether the buffer is read-only.
func (pt *PieceTable) IsReadOnly() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.readOnly
}

// Len returns the total byte length of the buffer.
func (pt *PieceTable) Len() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	total := 0
	for _, p := range pt.pieces {
		total += p.Length
	}
	return total
}

// RuneCount returns the total number of runes in the buffer.
func (pt *PieceTable) RuneCount() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	total := 0
	for _, p := range pt.pieces {
		total += p.Runes
	}
	return total
}

// LineCount returns the number of lines in the buffer (minimum 1).
func (pt *PieceTable) LineCount() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	newlines := 0
	for _, p := range pt.pieces {
		newlines += p.Lines
	}
	return newlines + 1
}

// Text returns the entire buffer content as a string.
func (pt *PieceTable) Text() string {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	var sb strings.Builder
	sb.Grow(pt.lenInternal())
	for _, p := range pt.pieces {
		sb.Write(pt.getPieceBytes(p))
	}
	return sb.String()
}

// Bytes returns the entire buffer content as a byte slice.
func (pt *PieceTable) Bytes() []byte {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	buf := make([]byte, 0, pt.lenInternal())
	for _, p := range pt.pieces {
		buf = append(buf, pt.getPieceBytes(p)...)
	}
	return buf
}

// Substr returns a substring given a byte offset and byte length.
func (pt *PieceTable) Substr(offset, length int) (string, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	totalLen := pt.lenInternal()
	if offset < 0 || offset > totalLen {
		return "", errors.New("offset out of range")
	}
	if length < 0 || offset+length > totalLen {
		return "", errors.New("length out of range")
	}
	if length == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.Grow(length)

	currOffset := 0
	remaining := length

	for _, p := range pt.pieces {
		pieceStart := currOffset
		pieceEnd := currOffset + p.Length

		if offset < pieceEnd && offset+length > pieceStart {
			// This piece overlaps with requested slice
			sliceStartInPiece := 0
			if offset > pieceStart {
				sliceStartInPiece = offset - pieceStart
			}

			sliceEndInPiece := p.Length
			if offset+length < pieceEnd {
				sliceEndInPiece = (offset + length) - pieceStart
			}

			pBytes := pt.getPieceBytes(p)
			sb.Write(pBytes[sliceStartInPiece:sliceEndInPiece])
			remaining -= (sliceEndInPiece - sliceStartInPiece)
			if remaining <= 0 {
				break
			}
		}
		currOffset = pieceEnd
	}

	return sb.String(), nil
}

// Insert inserts text at the specified byte offset.
func (pt *PieceTable) Insert(offset int, text string) error {
	if len(text) == 0 {
		return nil
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.readOnly {
		return errors.New("buffer is read-only")
	}

	totalLen := pt.lenInternal()
	if offset < 0 || offset > totalLen {
		return errors.New("offset out of range")
	}

	pt.saveUndoSnapshot()

	textBytes := []byte(text)
	textLen := len(textBytes)
	textRunes := utf8.RuneCount(textBytes)
	textLines := bytes.Count(textBytes, []byte{'\n'})

	addStart := len(pt.add)
	pt.add = append(pt.add, textBytes...)

	newPiece := Piece{
		Buffer: BufferAdd,
		Start:  addStart,
		Length: textLen,
		Runes:  textRunes,
		Lines:  textLines,
	}

	// Fast path: Empty pieces
	if len(pt.pieces) == 0 {
		pt.pieces = []Piece{newPiece}
		return nil
	}

	// Optimization: Coalesce sequential appends at the end of the add buffer
	lastIdx := len(pt.pieces) - 1
	if offset == totalLen {
		lastPiece := &pt.pieces[lastIdx]
		if lastPiece.Buffer == BufferAdd && lastPiece.Start+lastPiece.Length == addStart {
			lastPiece.Length += textLen
			lastPiece.Runes += textRunes
			lastPiece.Lines += textLines
			return nil
		}
		pt.pieces = append(pt.pieces, newPiece)
		return nil
	}

	// Insert at offset 0
	if offset == 0 {
		pt.pieces = append([]Piece{newPiece}, pt.pieces...)
		return nil
	}

	// Split piece at offset
	pieceIdx, offsetInPiece := pt.findPieceByOffset(offset)
	target := pt.pieces[pieceIdx]

	if offsetInPiece == 0 {
		// Insert right before pieceIdx
		pt.pieces = insertPieceAt(pt.pieces, pieceIdx, newPiece)
		return nil
	}

	if offsetInPiece == target.Length {
		// Insert right after pieceIdx
		pt.pieces = insertPieceAt(pt.pieces, pieceIdx+1, newPiece)
		return nil
	}

	// Split target into left and right pieces
	targetBytes := pt.getPieceBytes(target)
	leftBytes := targetBytes[:offsetInPiece]
	rightBytes := targetBytes[offsetInPiece:]

	leftPiece := Piece{
		Buffer: target.Buffer,
		Start:  target.Start,
		Length: len(leftBytes),
		Runes:  utf8.RuneCount(leftBytes),
		Lines:  bytes.Count(leftBytes, []byte{'\n'}),
	}

	rightPiece := Piece{
		Buffer: target.Buffer,
		Start:  target.Start + len(leftBytes),
		Length: len(rightBytes),
		Runes:  utf8.RuneCount(rightBytes),
		Lines:  bytes.Count(rightBytes, []byte{'\n'}),
	}

	// Replace target with leftPiece, newPiece, rightPiece
	newPieces := make([]Piece, 0, len(pt.pieces)+2)
	newPieces = append(newPieces, pt.pieces[:pieceIdx]...)
	newPieces = append(newPieces, leftPiece, newPiece, rightPiece)
	newPieces = append(newPieces, pt.pieces[pieceIdx+1:]...)
	pt.pieces = newPieces

	return nil
}

// Delete removes length bytes starting at offset.
func (pt *PieceTable) Delete(offset, length int) error {
	if length == 0 {
		return nil
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.readOnly {
		return errors.New("buffer is read-only")
	}

	totalLen := pt.lenInternal()
	if offset < 0 || offset >= totalLen {
		return errors.New("offset out of range")
	}
	if length < 0 || offset+length > totalLen {
		return errors.New("length out of range")
	}

	pt.saveUndoSnapshot()

	deleteEnd := offset + length
	currOffset := 0
	newPieces := make([]Piece, 0, len(pt.pieces))

	for _, p := range pt.pieces {
		pieceStart := currOffset
		pieceEnd := currOffset + p.Length

		if pieceEnd <= offset || pieceStart >= deleteEnd {
			// Piece is completely outside delete range
			newPieces = append(newPieces, p)
		} else {
			// Piece intersects with delete range
			pBytes := pt.getPieceBytes(p)

			// Keep left part if piece starts before delete offset
			if pieceStart < offset {
				leftLen := offset - pieceStart
				leftBytes := pBytes[:leftLen]
				newPieces = append(newPieces, Piece{
					Buffer: p.Buffer,
					Start:  p.Start,
					Length: leftLen,
					Runes:  utf8.RuneCount(leftBytes),
					Lines:  bytes.Count(leftBytes, []byte{'\n'}),
				})
			}

			// Keep right part if piece ends after deleteEnd
			if pieceEnd > deleteEnd {
				rightOffsetInPiece := deleteEnd - pieceStart
				rightBytes := pBytes[rightOffsetInPiece:]
				newPieces = append(newPieces, Piece{
					Buffer: p.Buffer,
					Start:  p.Start + rightOffsetInPiece,
					Length: len(rightBytes),
					Runes:  utf8.RuneCount(rightBytes),
					Lines:  bytes.Count(rightBytes, []byte{'\n'}),
				})
			}
		}
		currOffset = pieceEnd
	}

	pt.pieces = newPieces
	return nil
}

// Helper: lenInternal calculates length without acquiring locks
func (pt *PieceTable) lenInternal() int {
	total := 0
	for _, p := range pt.pieces {
		total += p.Length
	}
	return total
}

func (pt *PieceTable) getPieceBytes(p Piece) []byte {
	if p.Buffer == BufferOriginal {
		return pt.original[p.Start : p.Start+p.Length]
	}
	return pt.add[p.Start : p.Start+p.Length]
}

func (pt *PieceTable) findPieceByOffset(offset int) (pieceIdx int, offsetInPiece int) {
	curr := 0
	for i, p := range pt.pieces {
		if offset >= curr && offset < curr+p.Length {
			return i, offset - curr
		}
		curr += p.Length
	}
	if len(pt.pieces) > 0 && offset == curr {
		lastIdx := len(pt.pieces) - 1
		return lastIdx, pt.pieces[lastIdx].Length
	}
	return 0, 0
}

func insertPieceAt(pieces []Piece, idx int, item Piece) []Piece {
	if idx >= len(pieces) {
		return append(pieces, item)
	}
	res := make([]Piece, 0, len(pieces)+1)
	res = append(res, pieces[:idx]...)
	res = append(res, item)
	res = append(res, pieces[idx:]...)
	return res
}
