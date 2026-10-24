package search

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/mikolas/koodieditori/internal/buffer"
)

// SearchOptions holds parameters controlling search and replace operations.
type SearchOptions struct {
	Query         string
	Replacement   string
	IsRegex       bool
	CaseSensitive bool
	WholeWord     bool
}

// Match represents a single search match in a buffer or file.
type Match struct {
	FilePath    string
	Line        int    // 0-indexed line number
	Col         int    // 0-indexed rune column within the line
	StartOffset int    // 0-indexed byte offset in the full text
	EndOffset   int    // 0-indexed byte offset end
	RuneLength  int    // Number of runes in the match
	Text        string // Matched string
	LineContent string // Complete raw line content containing the match
}

// CompileRegex compiles a search regex based on the SearchOptions.
func CompileRegex(opts SearchOptions) (*regexp.Regexp, error) {
	if opts.Query == "" {
		return nil, nil
	}

	pattern := opts.Query
	if !opts.IsRegex {
		pattern = regexp.QuoteMeta(opts.Query)
	}

	if opts.WholeWord {
		pattern = `\b` + pattern + `\b`
	}

	if !opts.CaseSensitive {
		pattern = `(?i)` + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}
	return re, nil
}

// FindMatches searches text and returns all matching positions.
func FindMatches(text string, opts SearchOptions, filePath string) ([]Match, error) {
	if opts.Query == "" || len(text) == 0 {
		return nil, nil
	}

	re, err := CompileRegex(opts)
	if err != nil {
		return nil, err
	}
	if re == nil {
		return nil, nil
	}

	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return nil, nil
	}

	// Precompute line offsets for fast position mapping
	lineOffsets := computeLineOffsets(text)
	matches := make([]Match, 0, len(locs))

	for _, loc := range locs {
		start := loc[0]
		end := loc[1]
		lineIdx, colIdx, lineStr := offsetToPosition(text, lineOffsets, start)
		matchedStr := text[start:end]

		matches = append(matches, Match{
			FilePath:    filePath,
			Line:        lineIdx,
			Col:         colIdx,
			StartOffset: start,
			EndOffset:   end,
			RuneLength:  utf8.RuneCountInString(matchedStr),
			Text:        matchedStr,
			LineContent: lineStr,
		})
	}

	return matches, nil
}

// FindInPieceTable searches a piece table and returns all matches.
func FindInPieceTable(pt *buffer.PieceTable, opts SearchOptions, filePath string) ([]Match, error) {
	if pt == nil || opts.Query == "" {
		return nil, nil
	}
	text := pt.Text()
	return FindMatches(text, opts, filePath)
}

var reSlashDigits = regexp.MustCompile(`\\(\d+)`)

// NormalizeReplacement converts $1 or \1 capture references to ${1} format
// so that adjacent underscores or characters (e.g. $1_suffix) do not cause
// capture group name corruption in Go regex templates.
func NormalizeReplacement(repl string) string {
	// First convert \1, \2 -> ${1}, ${2}
	repl = reSlashDigits.ReplaceAllString(repl, `${$1}`)

	var sb strings.Builder
	for i := 0; i < len(repl); i++ {
		if repl[i] == '$' && i+1 < len(repl) {
			if repl[i+1] == '{' {
				// Already ${...}
				sb.WriteByte(repl[i])
				continue
			}
			if repl[i+1] >= '0' && repl[i+1] <= '9' {
				j := i + 1
				for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
					j++
				}
				sb.WriteString("${" + repl[i+1:j] + "}")
				i = j - 1
				continue
			}
		}
		sb.WriteByte(repl[i])
	}
	return sb.String()
}

// ReplaceAll replaces all occurrences in the piece table atomically.
// Returns the count of replaced occurrences.
func ReplaceAll(pt *buffer.PieceTable, opts SearchOptions, replacement string) (int, error) {
	if pt == nil || opts.Query == "" {
		return 0, nil
	}

	re, err := CompileRegex(opts)
	if err != nil {
		return 0, err
	}
	if re == nil {
		return 0, nil
	}

	text := pt.Text()
	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return 0, nil
	}

	normRepl := NormalizeReplacement(replacement)

	pt.BeginTransaction()
	defer pt.CommitTransaction()

	// Replace in reverse order so prior offsets remain valid
	for i := len(locs) - 1; i >= 0; i-- {
		start := locs[i][0]
		end := locs[i][1]
		length := end - start

		matchedStr := text[start:end]
		var repl string
		if opts.IsRegex {
			repl = re.ReplaceAllString(matchedStr, normRepl)
		} else {
			repl = replacement
		}

		if length > 0 {
			pt.Delete(start, length)
		}
		if len(repl) > 0 {
			pt.Insert(start, repl)
		}
	}

	return len(locs), nil
}

// ReplaceOne replaces a single match in the piece table atomically.
func ReplaceOne(pt *buffer.PieceTable, m Match, opts SearchOptions, replacement string) error {
	if pt == nil {
		return fmt.Errorf("buffer is nil")
	}

	re, err := CompileRegex(opts)
	if err != nil {
		return err
	}

	pt.BeginTransaction()
	defer pt.CommitTransaction()

	length := m.EndOffset - m.StartOffset
	var repl string
	if opts.IsRegex && re != nil {
		repl = re.ReplaceAllString(m.Text, NormalizeReplacement(replacement))
	} else {
		repl = replacement
	}

	if length > 0 {
		pt.Delete(m.StartOffset, length)
	}
	if len(repl) > 0 {
		pt.Insert(m.StartOffset, repl)
	}

	return nil
}

// PreviewLineReplacement returns the original line and preview replacement line.
func PreviewLineReplacement(lineContent string, m Match, opts SearchOptions, replacement string) (string, string) {
	re, err := CompileRegex(opts)
	if err != nil || re == nil {
		return lineContent, lineContent
	}

	normRepl := NormalizeReplacement(replacement)

	var newLine string
	if opts.IsRegex {
		newLine = re.ReplaceAllString(lineContent, normRepl)
	} else {
		pattern := opts.Query
		if opts.WholeWord {
			pattern = `\b` + regexp.QuoteMeta(pattern) + `\b`
			r, err := regexp.Compile(pattern)
			if err == nil {
				newLine = r.ReplaceAllString(lineContent, replacement)
			} else {
				newLine = strings.ReplaceAll(lineContent, opts.Query, replacement)
			}
		} else if !opts.CaseSensitive {
			r, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(pattern))
			if err == nil {
				newLine = r.ReplaceAllString(lineContent, replacement)
			} else {
				newLine = strings.ReplaceAll(lineContent, opts.Query, replacement)
			}
		} else {
			newLine = strings.ReplaceAll(lineContent, opts.Query, replacement)
		}
	}

	return lineContent, newLine
}

var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"target":       true,
	"build":        true,
	".gradle":      true,
	".ccls-cache":  true,
	".idea":        true,
	".vscode":      true,
	"dist":         true,
	"bin":          true,
}

func isIgnoredRelPath(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, part := range parts {
		if ignoredDirs[part] || (strings.HasPrefix(part, ".") && part != ".") {
			return true
		}
	}
	return false
}

// ProjectSearch recursively searches files in rootPath, excluding ignored directories.
func ProjectSearch(rootPath string, opts SearchOptions, maxResults int) ([]Match, error) {
	if opts.Query == "" {
		return nil, nil
	}
	if maxResults <= 0 {
		maxResults = 1000
	}

	var results []Match

	// 1. Try ripgrep (rg) if installed for fast workspace scanning
	if rgPath, err := exec.LookPath("rg"); err == nil && rgPath != "" {
		args := []string{"-l", "--no-messages"}
		if !opts.IsRegex {
			args = append(args, "-F")
		}
		if opts.CaseSensitive {
			args = append(args, "-s")
		} else {
			args = append(args, "-i")
		}
		if opts.WholeWord {
			args = append(args, "-w")
		}
		args = append(args, "-e", opts.Query, ".")

		cmd := exec.Command(rgPath, args...)
		cmd.Dir = rootPath
		if out, err := cmd.Output(); err == nil && len(out) > 0 {
			scanner := bufio.NewScanner(strings.NewReader(string(out)))
			for scanner.Scan() {
				relFile := strings.TrimSpace(scanner.Text())
				if relFile == "" || isIgnoredRelPath(relFile) {
					continue
				}
				fullPath := filepath.Join(rootPath, relFile)
				data, readErr := os.ReadFile(fullPath)
				if readErr != nil || isBinary(data) {
					continue
				}
				matches, mErr := FindMatches(string(data), opts, fullPath)
				if mErr == nil && len(matches) > 0 {
					results = append(results, matches...)
					if len(results) >= maxResults {
						return results[:maxResults], nil
					}
				}
			}
			return results, nil
		}
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if ignoredDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files larger than 2MB or non-text files
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Quick binary content check
		if isBinary(data) {
			return nil
		}

		matches, err := FindMatches(string(data), opts, path)
		if err == nil && len(matches) > 0 {
			results = append(results, matches...)
			if len(results) >= maxResults {
				return fs.SkipAll
			}
		}

		return nil
	})

	return results, err
}

// ASTSearch executes a Tree-sitter query on the given code and language.
func ASTSearch(code []byte, lang *sitter.Language, queryStr string) ([]Match, error) {
	if lang == nil || len(code) == 0 || queryStr == "" {
		return nil, nil
	}

	q, err := sitter.NewQuery([]byte(queryStr), lang)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter query error: %w", err)
	}
	defer q.Close()

	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, code)
	if err != nil || tree == nil {
		return nil, fmt.Errorf("failed to parse syntax tree: %w", err)
	}
	defer tree.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, tree.RootNode())

	text := string(code)
	lineOffsets := computeLineOffsets(text)
	var matches []Match

	for {
		m, ok := qc.NextMatch()
		if !ok || m == nil {
			break
		}
		for _, c := range m.Captures {
			node := c.Node
			startByte := int(node.StartByte())
			endByte := int(node.EndByte())
			if startByte >= len(text) || endByte > len(text) {
				continue
			}

			lineIdx, colIdx, lineStr := offsetToPosition(text, lineOffsets, startByte)
			matchedStr := text[startByte:endByte]

			matches = append(matches, Match{
				Line:        lineIdx,
				Col:         colIdx,
				StartOffset: startByte,
				EndOffset:   endByte,
				RuneLength:  utf8.RuneCountInString(matchedStr),
				Text:        matchedStr,
				LineContent: lineStr,
			})
		}
	}

	return matches, nil
}

// Helpers

func computeLineOffsets(text string) []int {
	offsets := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func offsetToPosition(text string, lineOffsets []int, offset int) (int, int, string) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}

	// Binary search for line index
	low := 0
	high := len(lineOffsets) - 1
	lineIdx := 0

	for low <= high {
		mid := (low + high) / 2
		if lineOffsets[mid] <= offset {
			lineIdx = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	lineStart := lineOffsets[lineIdx]
	lineEnd := len(text)
	if lineIdx+1 < len(lineOffsets) {
		lineEnd = lineOffsets[lineIdx+1]
		if lineEnd > 0 && text[lineEnd-1] == '\n' {
			lineEnd--
			if lineEnd > 0 && text[lineEnd-1] == '\r' {
				lineEnd--
			}
		}
	}

	lineContent := ""
	if lineStart <= lineEnd && lineStart <= len(text) {
		lineContent = text[lineStart:min(lineEnd, len(text))]
	}

	colRunes := utf8.RuneCountInString(text[lineStart:offset])
	return lineIdx, colRunes, lineContent
}

func isBinary(data []byte) bool {
	checkLen := min(1024, len(data))
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
