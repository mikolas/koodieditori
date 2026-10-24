package treesitter

import (
	"context"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mikolas/koodieditori/internal/markdown"
	sitter "github.com/smacker/go-tree-sitter"
)

// TokenType represents a semantic highlighting category (aligned with VS Code semantic tokens).
type TokenType uint8

const (
	TokenNormal TokenType = iota
	TokenKeyword     // Control flow: if, else, for, return, switch, case... (VS Code Purple)
	TokenDeclaration // Declaration: func, package, import, var, const, type, struct... (VS Code Blue)
	TokenFunction    // Functions & methods: declarations and calls (VS Code Yellow)
	TokenTypeId      // Types & Structs (VS Code Teal)
	TokenString      // Strings & character literals (VS Code Terracotta/Green)
	TokenComment     // Comments (VS Code Muted Slate/Green)
	TokenNumber      // Numbers (VS Code Light Green/Magenta)
	TokenOperator    // Operators (VS Code Subtle Foreground)
	TokenConstant    // Constants: true, false, nil, iota (VS Code Blue)
	TokenVariable    // Variables & parameters (VS Code Light Blue)
)

// HighlightSpan represents a highlighted span on a single line.
type HighlightSpan struct {
	StartCol int
	EndCol   int
	Type     TokenType
}

// Highlighter parses code using Tree-sitter and produces line-level highlight spans.
type Highlighter struct {
	mu     sync.RWMutex
	parser *sitter.Parser
	tree   *sitter.Tree
	lang   *sitter.Language
	lines  map[int][]HighlightSpan
}

// NewHighlighter creates a new syntax highlighter.
func NewHighlighter() *Highlighter {
	return &Highlighter{
		parser: sitter.NewParser(),
		lines:  make(map[int][]HighlightSpan),
	}
}

// Close frees the Tree-sitter resources.
func (h *Highlighter) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tree != nil {
		h.tree.Close()
		h.tree = nil
	}
	if h.parser != nil {
		h.parser.Close()
		h.parser = nil
	}
}

// Parse parses the given source code with Tree-sitter or Chroma fallback.
// extra optionally specifies langId (e.g. "json", "xml", "makefile") and filePath.
func (h *Highlighter) Parse(source []byte, lang *sitter.Language, extra ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	langId := ""
	filePath := ""
	if len(extra) > 0 {
		langId = extra[0]
	}
	if len(extra) > 1 {
		filePath = extra[1]
	}

	if lang == nil {
		if h.tree != nil {
			h.tree.Close()
			h.tree = nil
		}
		if langId == LangMarkdown || markdown.IsMarkdownFile(filePath) {
			h.parseMarkdown(source)
		} else if langId != "" || filePath != "" {
			h.parseWithChroma(source, langId, filePath)
		} else {
			h.lines = make(map[int][]HighlightSpan)
		}
		return
	}

	h.lang = lang
	h.parser.SetLanguage(lang)

	if h.tree != nil {
		h.tree.Close()
	}

	tree, err := h.parser.ParseCtx(context.Background(), nil, source)
	if err != nil || tree == nil {
		h.lines = make(map[int][]HighlightSpan)
		return
	}
	h.tree = tree

	// Extract highlight spans from AST
	newLines := make(map[int][]HighlightSpan)
	h.collectSpans(tree.RootNode(), newLines)
	h.lines = newLines
}

func (h *Highlighter) parseWithChroma(source []byte, langId, filePath string) {
	h.lines = tokenizeWithChroma(source, langId, filePath)
}

func tokenizeWithChroma(source []byte, langId, filePath string) map[int][]HighlightSpan {
	var lexer chroma.Lexer
	if langId != "" && langId != LangUnknown {
		lexer = lexers.Get(langId)
	}
	if lexer == nil && filePath != "" {
		lexer = lexers.Match(filePath)
	}
	if lexer == nil {
		return make(map[int][]HighlightSpan)
	}

	iterator, err := lexer.Tokenise(nil, string(source))
	if err != nil {
		return make(map[int][]HighlightSpan)
	}

	newLines := make(map[int][]HighlightSpan)
	lineIdx := 0
	colIdx := 0

	for _, token := range iterator.Tokens() {
		tokType := mapChromaToken(token.Type)
		val := token.Value

		linesInToken := strings.Split(val, "\n")
		for i, lineStr := range linesInToken {
			if i > 0 {
				lineIdx++
				colIdx = 0
			}
			byteCount := len(lineStr)
			if byteCount > 0 && tokType != TokenNormal {
				newLines[lineIdx] = append(newLines[lineIdx], HighlightSpan{
					StartCol: colIdx,
					EndCol:   colIdx + byteCount,
					Type:     tokType,
				})
			}
			colIdx += byteCount
		}
	}

	return newLines
}

func (h *Highlighter) parseMarkdown(source []byte) {
	lines := strings.Split(string(source), "\n")
	newLines := make(map[int][]HighlightSpan)

	inFence := false
	var fenceChar byte
	fenceLen := 0
	fenceLang := ""
	fenceStartLine := 0
	var fenceLines []string

	for lineIdx, line := range lines {
		if !inFence {
			if isOpen, ch, n, info := markdown.ParseOpeningFence(line); isOpen {
				inFence = true
				fenceChar = ch
				fenceLen = n
				fenceLang = info
				fenceStartLine = lineIdx + 1
				fenceLines = nil
				if len(line) > 0 {
					newLines[lineIdx] = []HighlightSpan{{
						StartCol: 0,
						EndCol:   len(line),
						Type:     TokenComment,
					}}
				}
				continue
			}
			if spans := parseMarkdownLineSpans(line); len(spans) > 0 {
				newLines[lineIdx] = spans
			}
		} else {
			if markdown.IsClosingFence(line, fenceChar, fenceLen) {
				if len(fenceLines) > 0 && fenceLang != "" {
					h.highlightFencedCodeBlock(strings.Join(fenceLines, "\n"), fenceLang, fenceStartLine, newLines)
				}
				if len(line) > 0 {
					newLines[lineIdx] = []HighlightSpan{{
						StartCol: 0,
						EndCol:   len(line),
						Type:     TokenComment,
					}}
				}
				inFence = false
				fenceChar = 0
				fenceLen = 0
				fenceLang = ""
				fenceLines = nil
				continue
			}
			fenceLines = append(fenceLines, line)
		}
	}

	if inFence && len(fenceLines) > 0 && fenceLang != "" {
		h.highlightFencedCodeBlock(strings.Join(fenceLines, "\n"), fenceLang, fenceStartLine, newLines)
	}

	h.lines = newLines
}

func (h *Highlighter) highlightFencedCodeBlock(code string, langTag string, startLine int, outLines map[int][]HighlightSpan) {
	tag := strings.ToLower(strings.TrimSpace(langTag))
	if fields := strings.Fields(tag); len(fields) > 0 {
		tag = fields[0]
	}
	var subLang *sitter.Language
	switch tag {
	case "go", "golang":
		subLang, _ = DetectLanguage("a.go")
	case "c", "h":
		subLang, _ = DetectLanguage("a.c")
	case "cpp", "c++", "cc", "cxx", "hpp":
		subLang, _ = DetectLanguage("a.cpp")
	case "java":
		subLang, _ = DetectLanguage("a.java")
	case "bash", "sh", "shell", "zsh":
		subLang, _ = DetectLanguage("a.sh")
	case "toml":
		subLang, _ = DetectLanguage("a.toml")
	case "yaml", "yml":
		subLang, _ = DetectLanguage("a.yaml")
	}

	if subLang != nil {
		subParser := sitter.NewParser()
		defer subParser.Close()
		subParser.SetLanguage(subLang)
		tree, err := subParser.ParseCtx(context.Background(), nil, []byte(code))
		if err == nil && tree != nil {
			defer tree.Close()
			subLines := make(map[int][]HighlightSpan)
			h.collectSpans(tree.RootNode(), subLines)
			for r, spans := range subLines {
				outLines[startLine+r] = append(outLines[startLine+r], spans...)
			}
		}
		return
	}

	subLines := tokenizeWithChroma([]byte(code), tag, "")
	for r, spans := range subLines {
		outLines[startLine+r] = append(outLines[startLine+r], spans...)
	}
}

func parseMarkdownLineSpans(line string) []HighlightSpan {
	if len(line) == 0 {
		return nil
	}

	trimmedLeft := strings.TrimLeft(line, " \t")
	indentLen := len(line) - len(trimmedLeft)

	// 1. ATX Headings (# .. ######)
	if len(trimmedLeft) > 0 && trimmedLeft[0] == '#' {
		hashes := 0
		for hashes < len(trimmedLeft) && trimmedLeft[hashes] == '#' {
			hashes++
		}
		if hashes <= 6 && (hashes == len(trimmedLeft) || trimmedLeft[hashes] == ' ' || trimmedLeft[hashes] == '\t') {
			var spans []HighlightSpan
			contentStart := indentLen + hashes
			parseMarkdownInlineSpans(line, contentStart, len(line), &spans)
			spans = append(spans, HighlightSpan{
				StartCol: indentLen,
				EndCol:   len(line),
				Type:     TokenDeclaration,
			})
			return spans
		}
	}

	// 2. Blockquotes (> ...)
	if len(trimmedLeft) > 0 && trimmedLeft[0] == '>' {
		markerEnd := indentLen + 1
		var spans []HighlightSpan
		spans = append(spans, HighlightSpan{
			StartCol: indentLen,
			EndCol:   markerEnd,
			Type:     TokenKeyword,
		})
		parseMarkdownInlineSpans(line, markerEnd, len(line), &spans)
		if markerEnd < len(line) {
			spans = append(spans, HighlightSpan{
				StartCol: markerEnd,
				EndCol:   len(line),
				Type:     TokenComment,
			})
		}
		return spans
	}

	// 3. Task list items (- [ ] / - [x])
	var spans []HighlightSpan
	inlineStart := 0
	if prefixEnd, _, boxStart, boxEnd, contentStart, _, ok := markdown.MatchTaskPrefix(line); ok {
		if prefixEnd > 0 {
			spans = append(spans, HighlightSpan{
				StartCol: prefixEnd - 1,
				EndCol:   prefixEnd,
				Type:     TokenKeyword,
			})
		}
		spans = append(spans, HighlightSpan{
			StartCol: boxStart,
			EndCol:   boxEnd,
			Type:     TokenKeyword,
		})
		inlineStart = contentStart
	} else if markerStart, markerEnd, contentStart, ok := markdown.MatchListPrefix(line); ok {
		// 4. Unordered / Ordered list items (- , * , 1. )
		spans = append(spans, HighlightSpan{
			StartCol: markerStart,
			EndCol:   markerEnd,
			Type:     TokenKeyword,
		})
		inlineStart = contentStart
	}

	parseMarkdownInlineSpans(line, inlineStart, len(line), &spans)
	return spans
}

func parseMarkdownInlineSpans(line string, start, end int, spans *[]HighlightSpan) {
	for i := start; i < end; {
		ch := line[i]

		// 1. Escaped markdown punctuation
		if ch == '\\' && i+1 < end && markdown.IsMarkdownPunct(line[i+1]) {
			i += 2
			continue
		}

		// 2. Inline code span (`code` or ``code``)
		if ch == '`' {
			tickLen := 0
			for i+tickLen < end && line[i+tickLen] == '`' {
				tickLen++
			}
			closeIdx := markdown.FindClosingBackticks(line, i+tickLen, end, tickLen)
			if closeIdx > i+tickLen {
				*spans = append(*spans, HighlightSpan{
					StartCol: i,
					EndCol:   closeIdx + tickLen,
					Type:     TokenString,
				})
				i = closeIdx + tickLen
				continue
			}
			i += tickLen
			continue
		}

		// 3. Links: [title](url)
		if ch == '[' {
			if titleStart, titleEnd, urlStart, urlEnd, linkEnd, ok := markdown.MatchLink(line, i, end); ok {
				parseMarkdownInlineSpans(line, titleStart, titleEnd, spans)
				*spans = append(*spans, HighlightSpan{
					StartCol: titleStart,
					EndCol:   titleEnd,
					Type:     TokenDeclaration,
				})
				*spans = append(*spans, HighlightSpan{
					StartCol: urlStart,
					EndCol:   urlEnd,
					Type:     TokenKeyword,
				})
				i = linkEnd
				continue
			}
		}

		// 4. Bold (**text** / __text__) and Italic (*text* / _text_)
		if ch == '*' || ch == '_' {
			run := 0
			for i+run < end && line[i+run] == ch {
				run++
			}
			if run == 2 {
				if closeIdx, ok := markdown.MatchEmphasis(line, i, end, ch, 2); ok {
					parseMarkdownInlineSpans(line, i+2, closeIdx, spans)
					*spans = append(*spans, HighlightSpan{
						StartCol: i,
						EndCol:   closeIdx + 2,
						Type:     TokenConstant,
					})
					i = closeIdx + 2
					continue
				}
			} else if run == 1 {
				if closeIdx, ok := markdown.MatchEmphasis(line, i, end, ch, 1); ok {
					parseMarkdownInlineSpans(line, i+1, closeIdx, spans)
					*spans = append(*spans, HighlightSpan{
						StartCol: i,
						EndCol:   closeIdx + 1,
						Type:     TokenComment,
					})
					i = closeIdx + 1
					continue
				}
			}
			i += run
			continue
		}

		i++
	}
}

func mapChromaToken(t chroma.TokenType) TokenType {
	switch {
	case t.InCategory(chroma.Comment):
		return TokenComment
	case t.InSubCategory(chroma.LiteralString):
		return TokenString
	case t.InSubCategory(chroma.LiteralNumber):
		return TokenNumber
	case t == chroma.KeywordConstant || t == chroma.NameConstant:
		return TokenConstant
	case t == chroma.KeywordType || t == chroma.NameClass:
		return TokenTypeId
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return TokenFunction
	case t == chroma.NameVariable || t == chroma.NameVariableClass || t == chroma.NameVariableGlobal || t == chroma.NameVariableInstance || t == chroma.NameVariableMagic:
		return TokenVariable
	case t == chroma.KeywordDeclaration || t == chroma.KeywordNamespace:
		return TokenDeclaration
	case t.InCategory(chroma.Keyword):
		return TokenKeyword
	case t.InCategory(chroma.Operator):
		return TokenOperator
	case t == chroma.NameTag:
		return TokenDeclaration
	case t == chroma.NameAttribute:
		return TokenKeyword
	case t == chroma.NameLabel:
		return TokenFunction
	case t == chroma.GenericHeading || t == chroma.GenericSubheading:
		return TokenDeclaration
	case t == chroma.GenericStrong:
		return TokenConstant
	default:
		return TokenNormal
	}
}

// Tree returns the active Tree-sitter tree for AST queries (e.g. text objects).
func (h *Highlighter) Tree() *sitter.Tree {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tree
}

// GetLineHighlights returns the highlight spans for lineIdx (0-indexed).
func (h *Highlighter) GetLineHighlights(lineIdx int) []HighlightSpan {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lines[lineIdx]
}

func (h *Highlighter) collectSpans(node *sitter.Node, lines map[int][]HighlightSpan) {
	if node == nil {
		return
	}

	nodeType := node.Type()
	childCount := int(node.ChildCount())

	// 1. Strings and comments are atomic token spans even if they contain internal child nodes (e.g. escape sequences)
	if isAtomicLiteralOrComment(nodeType) {
		token := classifyAtomic(nodeType)
		h.appendNodeSpan(node, token, lines)
		return
	}

	// 2. Leaf nodes (no children): keywords, identifiers, types, numbers, operators
	if childCount == 0 {
		token := classifyLeafNode(node)
		if token != TokenNormal {
			h.appendNodeSpan(node, token, lines)
		}
		return
	}

	// 3. For composite container nodes (type_declaration, function_declaration, call_expression, etc.),
	// do NOT emit container spans that would clobber children! Recurse directly into child nodes.
	for i := 0; i < childCount; i++ {
		h.collectSpans(node.Child(i), lines)
	}
}

func (h *Highlighter) appendNodeSpan(node *sitter.Node, token TokenType, lines map[int][]HighlightSpan) {
	startPoint := node.StartPoint()
	endPoint := node.EndPoint()

	startRow := int(startPoint.Row)
	endRow := int(endPoint.Row)
	startCol := int(startPoint.Column)
	endCol := int(endPoint.Column)

	if startRow == endRow {
		lines[startRow] = append(lines[startRow], HighlightSpan{
			StartCol: startCol,
			EndCol:   endCol,
			Type:     token,
		})
	} else {
		// Multi-line token (e.g. multiline comment or raw string literal)
		for r := startRow; r <= endRow; r++ {
			sc := 0
			ec := 999999
			if r == startRow {
				sc = startCol
			}
			if r == endRow {
				ec = endCol
			}
			lines[r] = append(lines[r], HighlightSpan{
				StartCol: sc,
				EndCol:   ec,
				Type:     token,
			})
		}
	}
}

func isAtomicLiteralOrComment(t string) bool {
	switch t {
	case "comment", "line_comment", "block_comment",
		"string_literal", "interpreted_string_literal", "raw_string_literal",
		"rune_literal", "char_literal", "character_literal", "system_lib_string",
		"raw_string", "ansi_c_string", "heredoc_body", "string_scalar":
		return true
	}
	return false
}

func classifyAtomic(t string) TokenType {
	if strings.Contains(t, "comment") {
		return TokenComment
	}
	return TokenString
}

func classifyLeafNode(node *sitter.Node) TokenType {
	t := node.Type()

	// Comments & Strings
	if strings.Contains(t, "comment") {
		return TokenComment
	}
	if strings.Contains(t, "string") || strings.Contains(t, "char_literal") {
		return TokenString
	}

	// Numbers
	if strings.Contains(t, "integer") || strings.Contains(t, "float") || strings.Contains(t, "number") {
		return TokenNumber
	}
	switch t {
	case "int_literal", "float_literal", "imaginary_literal", "number_literal",
		"decimal_integer_literal", "hex_integer_literal", "decimal_floating_point_literal",
		"hex_literal", "offset_date_time", "local_date_time", "local_date", "local_time":
		return TokenNumber
	}

	// Types
	switch t {
	case "type_identifier", "primitive_type", "sized_type_specifier",
		"integral_type", "floating_point_type", "boolean_type", "void_type":
		return TokenTypeId
	}

	// Constants
	switch t {
	case "true", "false", "nil", "iota", "null", "nullptr", "this", "super",
		"boolean_scalar", "null_scalar", "boolean":
		return TokenConstant
	}

	// Control Flow Keywords (VS Code Purple)
	switch t {
	case "if", "else", "for", "return", "switch", "case", "default",
		"break", "continue", "goto", "defer", "go", "select",
		"while", "do", "try", "catch", "finally", "throw", "throws",
		"then", "elif", "fi", "esac", "done", "until", "in":
		return TokenKeyword
	}

	// Declaration / Storage Keywords (VS Code Blue)
	switch t {
	case "func", "package", "import", "var", "const", "type", "struct", "interface",
		"map", "chan", "class", "enum", "public", "private", "protected",
		"static", "final", "abstract", "void", "new", "delete",
		"template", "namespace", "using", "auto", "virtual", "override",
		"extends", "implements", "instanceof", "synchronized", "volatile",
		"function":
		return TokenDeclaration
	}

	// Operators
	switch t {
	case "+", "-", "*", "/", "%", "=", "+=", "-=", "*=", "/=",
		"==", "!=", "<", ">", "<=", ">=", "&&", "||", "!", "&", "|", "^", "~",
		":=", "<-", "->", "=>", "::", "?", ":":
		return TokenOperator
	}

	// Identifiers, variables, properties
	switch t {
	case "variable_name", "special_variable_name", "bare_key", "dotted_key", "property":
		return TokenVariable
	case "command_name":
		return TokenFunction
	case "word":
		if node.Parent() != nil && node.Parent().Type() == "command_name" {
			return TokenFunction
		}
	}

	// Identifiers: check if function declaration, method, or function call
	if t == "identifier" || t == "field_identifier" {
		if isFunctionToken(node) {
			return TokenFunction
		}
		return TokenVariable
	}

	return TokenNormal
}

func isFunctionToken(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	pType := parent.Type()
	switch pType {
	case "function_declaration", "method_declaration":
		nameNode := parent.ChildByFieldName("name")
		if nameNode != nil && nameNode.StartByte() == node.StartByte() {
			return true
		}
	case "function_declarator":
		nameNode := parent.ChildByFieldName("declarator")
		if nameNode != nil && nameNode.StartByte() == node.StartByte() {
			return true
		}
	case "call_expression":
		fnNode := parent.ChildByFieldName("function")
		if fnNode != nil && fnNode.StartByte() == node.StartByte() {
			return true
		}
	case "method_invocation":
		nameNode := parent.ChildByFieldName("name")
		if nameNode != nil && nameNode.StartByte() == node.StartByte() {
			return true
		}
	case "selector_expression", "field_expression":
		grandParent := parent.Parent()
		if grandParent != nil && (grandParent.Type() == "call_expression" || grandParent.Type() == "method_invocation") {
			fieldNode := parent.ChildByFieldName("field")
			if fieldNode != nil && fieldNode.StartByte() == node.StartByte() {
				return true
			}
		}
	case "scoped_identifier":
		grandParent := parent.Parent()
		if grandParent != nil && grandParent.Type() == "call_expression" {
			nameNode := parent.ChildByFieldName("name")
			if nameNode != nil && nameNode.StartByte() == node.StartByte() {
				return true
			}
		}
	}
	return false
}
