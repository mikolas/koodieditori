package buffer

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Line returns the text of the line at 0-indexed lineNum (excluding trailing '\n').
func (pt *PieceTable) Line(lineNum int) (string, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	lineCount := pt.LineCount()
	if lineNum < 0 || lineNum >= lineCount {
		return "", errors.New("line number out of range")
	}

	targetLine := 0
	var sb strings.Builder

	for _, p := range pt.pieces {
		pBytes := pt.getPieceBytes(p)
		startInPiece := 0

		for i, b := range pBytes {
			if b == '\n' {
				if targetLine == lineNum {
					// Found the end of our target line
					sb.Write(pBytes[startInPiece:i])
					res := sb.String()
					if strings.HasSuffix(res, "\r") {
						res = strings.TrimSuffix(res, "\r")
					}
					return res, nil
				}
				targetLine++
				startInPiece = i + 1
			}
		}

		if targetLine == lineNum {
			// Current line extends into or ends in this piece
			sb.Write(pBytes[startInPiece:])
		}
	}

	if targetLine == lineNum {
		res := sb.String()
		if strings.HasSuffix(res, "\r") {
			res = strings.TrimSuffix(res, "\r")
		}
		return res, nil
	}

	return "", errors.New("line not found")
}

// LineOffset returns the byte offset in the buffer where lineNum (0-indexed) begins.
func (pt *PieceTable) LineOffset(lineNum int) (int, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if lineNum < 0 || lineNum >= pt.LineCount() {
		return 0, errors.New("line number out of range")
	}

	if lineNum == 0 {
		return 0, nil
	}

	currentLine := 0
	byteOffset := 0

	for _, p := range pt.pieces {
		pBytes := pt.getPieceBytes(p)
		for i, b := range pBytes {
			if b == '\n' {
				currentLine++
				if currentLine == lineNum {
					return byteOffset + i + 1, nil
				}
			}
		}
		byteOffset += p.Length
	}

	return 0, errors.New("line offset not found")
}

// OffsetToPosition converts a byte offset to a 0-indexed Position (Line, Col in runes).
func (pt *PieceTable) OffsetToPosition(offset int) (Position, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	totalLen := pt.lenInternal()
	if offset < 0 || offset > totalLen {
		return Position{}, errors.New("offset out of range")
	}

	line := 0
	lineStartOffset := 0
	currOffset := 0

	for _, p := range pt.pieces {
		pBytes := pt.getPieceBytes(p)
		pieceEnd := currOffset + p.Length

		if offset <= pieceEnd {
			// Offset is within this piece
			offsetInPiece := offset - currOffset
			for i := 0; i < offsetInPiece; i++ {
				if pBytes[i] == '\n' {
					line++
					lineStartOffset = currOffset + i + 1
				}
			}
			break
		}

		// Offset is beyond this piece, scan piece for newlines
		for i, b := range pBytes {
			if b == '\n' {
				line++
				lineStartOffset = currOffset + i + 1
			}
		}
		currOffset = pieceEnd
	}

	// Calculate rune column from lineStartOffset to offset
	colRunes := 0
	if offset > lineStartOffset {
		subBytes, err := pt.substrInternal(lineStartOffset, offset-lineStartOffset)
		if err == nil {
			colRunes = utf8.RuneCount(subBytes)
		}
	}

	return Position{Line: line, Col: colRunes}, nil
}

// PositionToOffset converts a 0-indexed Position (Line, Col in runes) to a byte offset.
func (pt *PieceTable) PositionToOffset(pos Position) (int, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	lineStart, err := pt.lineOffsetInternal(pos.Line)
	if err != nil {
		return 0, err
	}

	// Read line bytes to find the byte offset corresponding to Col runes
	lineText, err := pt.lineInternal(pos.Line)
	if err != nil {
		return 0, err
	}

	byteIdx := 0
	runeIdx := 0
	for byteIdx < len(lineText) {
		if runeIdx == pos.Col {
			break
		}
		_, size := utf8.DecodeRuneInString(lineText[byteIdx:])
		byteIdx += size
		runeIdx++
	}

	return lineStart + byteIdx, nil
}

// substrInternal retrieves bytes without lock
func (pt *PieceTable) substrInternal(offset, length int) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	buf := make([]byte, 0, length)
	currOffset := 0
	remaining := length

	for _, p := range pt.pieces {
		pieceStart := currOffset
		pieceEnd := currOffset + p.Length

		if offset < pieceEnd && offset+length > pieceStart {
			sliceStartInPiece := 0
			if offset > pieceStart {
				sliceStartInPiece = offset - pieceStart
			}

			sliceEndInPiece := p.Length
			if offset+length < pieceEnd {
				sliceEndInPiece = (offset + length) - pieceStart
			}

			pBytes := pt.getPieceBytes(p)
			buf = append(buf, pBytes[sliceStartInPiece:sliceEndInPiece]...)
			remaining -= (sliceEndInPiece - sliceStartInPiece)
			if remaining <= 0 {
				break
			}
		}
		currOffset = pieceEnd
	}
	return buf, nil
}

func (pt *PieceTable) lineOffsetInternal(lineNum int) (int, error) {
	if lineNum < 0 || lineNum >= pt.lineCountInternal() {
		return 0, errors.New("line number out of range")
	}
	if lineNum == 0 {
		return 0, nil
	}

	currentLine := 0
	byteOffset := 0
	for _, p := range pt.pieces {
		pBytes := pt.getPieceBytes(p)
		for i, b := range pBytes {
			if b == '\n' {
				currentLine++
				if currentLine == lineNum {
					return byteOffset + i + 1, nil
				}
			}
		}
		byteOffset += p.Length
	}
	return 0, errors.New("line offset not found")
}

func (pt *PieceTable) lineInternal(lineNum int) (string, error) {
	lineCount := pt.lineCountInternal()
	if lineNum < 0 || lineNum >= lineCount {
		return "", errors.New("line number out of range")
	}

	targetLine := 0
	var sb strings.Builder

	for _, p := range pt.pieces {
		pBytes := pt.getPieceBytes(p)
		startInPiece := 0

		for i, b := range pBytes {
			if b == '\n' {
				if targetLine == lineNum {
					sb.Write(pBytes[startInPiece:i])
					res := sb.String()
					if strings.HasSuffix(res, "\r") {
						res = strings.TrimSuffix(res, "\r")
					}
					return res, nil
				}
				targetLine++
				startInPiece = i + 1
			}
		}

		if targetLine == lineNum {
			sb.Write(pBytes[startInPiece:])
		}
	}

	if targetLine == lineNum {
		res := sb.String()
		if strings.HasSuffix(res, "\r") {
			res = strings.TrimSuffix(res, "\r")
		}
		return res, nil
	}
	return "", errors.New("line not found")
}

func (pt *PieceTable) lineCountInternal() int {
	newlines := 0
	for _, p := range pt.pieces {
		newlines += p.Lines
	}
	return newlines + 1
}
