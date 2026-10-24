package treesitter

import (
	"strings"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/mikolas/koodieditori/internal/buffer"
)

// Range represents a text span from Start to End.
type Range struct {
	Start buffer.Position
	End   buffer.Position
}

// FindTextObject finds the target text object surrounding the cursor position.
// kind is "i" (inside) or "a" (around).
// target is "b" (block), "a" (array), "p" (parens), "q" (quotes), "f" (function), "c" (class), "s" (string/statement).
func FindTextObject(tree *sitter.Tree, pt *buffer.PieceTable, cursor buffer.Position, kind, target string) (Range, bool) {
	if tree != nil {
		root := tree.RootNode()
		if root != nil {
			colBytes := cursor.Col
			if pt != nil {
				if lineText, err := pt.Line(cursor.Line); err == nil {
					rIdx := 0
					bIdx := 0
					for bIdx < len(lineText) && rIdx < cursor.Col {
						_, size := utf8.DecodeRuneInString(lineText[bIdx:])
						bIdx += size
						rIdx++
					}
					colBytes = bIdx
				}
			}
			ptSitter := sitter.Point{
				Row:    uint32(cursor.Line),
				Column: uint32(colBytes),
			}
			if rng, ok := findASTObject(root, ptSitter, pt, kind, target); ok {
				return rng, true
			}
		}
	}

	// Fallback: Buffer-based text object scanner for brackets, quotes, and blocks
	return findBufferObject(pt, cursor, kind, target)
}

func findASTObject(root *sitter.Node, ptSitter sitter.Point, pt *buffer.PieceTable, kind, target string) (Range, bool) {
	node := root.NamedDescendantForPointRange(ptSitter, ptSitter)
	if node == nil {
		return Range{}, false
	}

	curr := node
	for curr != nil {
		nType := curr.Type()

		switch target {
		case "f": // Function
			if isFunctionNode(nType) {
				return nodeToRange(curr, pt, kind, "{", "}"), true
			}
		case "c": // Class / Struct
			if isClassNode(nType) {
				return nodeToRange(curr, pt, kind, "{", "}"), true
			}
		case "b": // Block
			if isBlockNode(nType) {
				return nodeToRange(curr, pt, kind, "{", "}"), true
			}
		case "a": // Array / Bracket
			if isArrayNode(nType) {
				return nodeToRange(curr, pt, kind, "[", "]"), true
			}
		case "p": // Parens
			if isParenNode(nType) {
				return nodeToRange(curr, pt, kind, "(", ")"), true
			}
		case "q", "s": // Quote / String
			if strings.Contains(nType, "string") || strings.Contains(nType, "literal") {
				return nodeToRange(curr, pt, kind, "\"", "\""), true
			}
		}

		curr = curr.Parent()
	}

	return Range{}, false
}

func nodeToRange(node *sitter.Node, pt *buffer.PieceTable, kind string, openDelim, closeDelim string) Range {
	sp := node.StartPoint()
	ep := node.EndPoint()

	startColRunes := byteColToRuneCol(pt, int(sp.Row), int(sp.Column))
	endColRunes := byteColToRuneCol(pt, int(ep.Row), int(ep.Column))

	start := buffer.Position{Line: int(sp.Row), Col: startColRunes}
	end := buffer.Position{Line: int(ep.Row), Col: endColRunes}

	if kind == "i" {
		// Inside: trim 1 column on start and end if on same line
		if start.Line == end.Line {
			if end.Col > start.Col+1 {
				start.Col++
				end.Col--
			}
		} else {
			start.Col++
			if end.Col > 0 {
				end.Col--
			}
		}
	}

	return Range{Start: start, End: end}
}

func byteColToRuneCol(pt *buffer.PieceTable, row int, colBytes int) int {
	if pt == nil {
		return colBytes
	}
	lineText, err := pt.Line(row)
	if err != nil || len(lineText) == 0 {
		return colBytes
	}
	if colBytes >= len(lineText) {
		return utf8.RuneCountInString(lineText)
	}
	return utf8.RuneCountInString(lineText[:colBytes])
}

func isFunctionNode(t string) bool {
	return strings.Contains(t, "function") || strings.Contains(t, "method") ||
		t == "func_literal" || t == "lambda_expression" || t == "rule"
}

func isClassNode(t string) bool {
	return strings.Contains(t, "class") || strings.Contains(t, "struct") ||
		strings.Contains(t, "interface") || t == "type_spec" || t == "type_declaration" ||
		t == "table" || t == "table_array_element"
}

func isBlockNode(t string) bool {
	return t == "block" || t == "compound_statement" || strings.Contains(t, "block") ||
		t == "subshell" || t == "block_mapping" || t == "mapping" || t == "do_group"
}

func isArrayNode(t string) bool {
	return strings.Contains(t, "array") || strings.Contains(t, "slice") ||
		strings.Contains(t, "subscript") || strings.Contains(t, "bracket") ||
		strings.Contains(t, "sequence") || t == "block_sequence"
}

func isParenNode(t string) bool {
	return strings.Contains(t, "paren") || strings.Contains(t, "parameter") ||
		strings.Contains(t, "argument") || t == "subshell" || t == "command_substitution"
}

// findBufferObject provides a bulletproof fallback searching delimiters directly in buffer.
func findBufferObject(pt *buffer.PieceTable, cursor buffer.Position, kind, target string) (Range, bool) {
	if pt == nil {
		return Range{}, false
	}

	var openRune, closeRune rune
	switch target {
	case "b":
		openRune, closeRune = '{', '}'
	case "a":
		openRune, closeRune = '[', ']'
	case "p":
		openRune, closeRune = '(', ')'
	case "q":
		openRune, closeRune = '"', '"'
	default:
		return Range{}, false
	}

	text := pt.Text()
	runes := []rune(text)
	totalRunes := len(runes)

	cursorOffset, err := pt.PositionToOffset(cursor)
	if err != nil {
		cursorOffset = 0
	}
	if cursorOffset >= totalRunes {
		cursorOffset = totalRunes - 1
	}
	if cursorOffset < 0 {
		return Range{}, false
	}

	// 1. Scan backwards for opening delimiter
	openIdx := -1
	depth := 0
	for i := cursorOffset; i >= 0; i-- {
		r := runes[i]
		if openRune == closeRune {
			if r == openRune {
				openIdx = i
				break
			}
		} else {
			if r == closeRune && i != cursorOffset {
				depth++
			} else if r == openRune {
				if depth == 0 {
					openIdx = i
					break
				}
				depth--
			}
		}
	}

	if openIdx == -1 {
		return Range{}, false
	}

	// 2. Scan forwards for closing delimiter
	closeIdx := -1
	depth = 0
	for i := openIdx + 1; i < totalRunes; i++ {
		r := runes[i]
		if openRune == closeRune {
			if r == closeRune {
				closeIdx = i
				break
			}
		} else {
			if r == openRune {
				depth++
			} else if r == closeRune {
				if depth == 0 {
					closeIdx = i
					break
				}
				depth--
			}
		}
	}

	if closeIdx == -1 {
		return Range{}, false
	}

	// Convert rune indices to Positions
	startPos, err1 := pt.OffsetToPosition(openIdx)
	endPos, err2 := pt.OffsetToPosition(closeIdx + 1) // +1 to include closing delimiter

	if err1 != nil || err2 != nil {
		return Range{}, false
	}

	if kind == "i" {
		// Inside: exclude the delimiters
		insideStart, _ := pt.OffsetToPosition(openIdx + 1)
		insideEnd, _ := pt.OffsetToPosition(closeIdx)
		return Range{Start: insideStart, End: insideEnd}, true
	}

	return Range{Start: startPos, End: endPos}, true
}
