package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/treesitter"
)

func TestFindMatches(t *testing.T) {
	text := "Hello world!\nhello Nordic äiti öljy\nHELLO WORLD again."

	// 1. Case-insensitive search
	opts1 := SearchOptions{Query: "hello", CaseSensitive: false}
	matches1, err := FindMatches(text, opts1, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches1) != 3 {
		t.Fatalf("expected 3 matches for 'hello', got %d", len(matches1))
	}
	if matches1[0].Line != 0 || matches1[0].Col != 0 {
		t.Fatalf("expected match 0 at (0, 0), got (%d, %d)", matches1[0].Line, matches1[0].Col)
	}
	if matches1[1].Line != 1 || matches1[1].Col != 0 {
		t.Fatalf("expected match 1 at (1, 0), got (%d, %d)", matches1[1].Line, matches1[1].Col)
	}
	if matches1[2].Line != 2 || matches1[2].Col != 0 {
		t.Fatalf("expected match 2 at (2, 0), got (%d, %d)", matches1[2].Line, matches1[2].Col)
	}

	// 2. Case-sensitive search
	opts2 := SearchOptions{Query: "Hello", CaseSensitive: true}
	matches2, err := FindMatches(text, opts2, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches2) != 1 {
		t.Fatalf("expected 1 match for 'Hello', got %d", len(matches2))
	}

	// 3. Multibyte UTF-8 search (Nordic runes)
	opts3 := SearchOptions{Query: "öljy", CaseSensitive: true}
	matches3, err := FindMatches(text, opts3, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches3) != 1 {
		t.Fatalf("expected 1 match for 'öljy', got %d", len(matches3))
	}
	if matches3[0].Line != 1 {
		t.Fatalf("expected line 1, got %d", matches3[0].Line)
	}
	// "hello Nordic äiti öljy" -> hello(5) +(1) Nordic(6) +(1) äiti(4) +(1) = col 18
	if matches3[0].Col != 18 {
		t.Fatalf("expected col 18 for 'öljy', got %d", matches3[0].Col)
	}

	// 4. Whole word matching
	textWord := "word words subword word"
	optsWord := SearchOptions{Query: "word", WholeWord: true}
	matchesWord, err := FindMatches(textWord, optsWord, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matchesWord) != 2 {
		t.Fatalf("expected 2 whole-word matches, got %d", len(matchesWord))
	}

	// 5. Regex search with character class
	textRegex := "port: 8080, port: 9000, port: abc"
	optsRegex := SearchOptions{Query: `port:\s*(\d+)`, IsRegex: true}
	matchesRegex, err := FindMatches(textRegex, optsRegex, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matchesRegex) != 2 {
		t.Fatalf("expected 2 regex matches, got %d", len(matchesRegex))
	}
}

func TestReplaceAllAndUndo(t *testing.T) {
	initial := "foo bar foo\nsecond foo line\nlast FOO line"
	pt := buffer.NewPieceTableFromString(initial)

	// 1. Replace all case-sensitive "foo" with "baz"
	opts := SearchOptions{Query: "foo", CaseSensitive: true}
	count, err := ReplaceAll(pt, opts, "baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 replacements, got %d", count)
	}

	expected := "baz bar baz\nsecond baz line\nlast FOO line"
	if pt.Text() != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, pt.Text())
	}

	// 2. Undo should restore initial text in one step
	if !pt.Undo() {
		t.Fatalf("undo failed")
	}
	if pt.Text() != initial {
		t.Fatalf("expected undo to restore initial text, got:\n%s", pt.Text())
	}

	// 3. Regex Replace with Capture Groups
	code := "func user_get() int {}\nfunc user_post() int {}\nfunc item_get() int {}"
	ptCode := buffer.NewPieceTableFromString(code)

	regexOpts := SearchOptions{
		Query:   `func (\w+)_(\w+)\(\)`,
		IsRegex: true,
	}
	// Swap order: func ActionResource()
	rCount, err := ReplaceAll(ptCode, regexOpts, "func $2_$1()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rCount != 3 {
		t.Fatalf("expected 3 replacements, got %d", rCount)
	}

	expectedCode := "func get_user() int {}\nfunc post_user() int {}\nfunc get_item() int {}"
	if ptCode.Text() != expectedCode {
		t.Fatalf("expected:\n%s\ngot:\n%s", expectedCode, ptCode.Text())
	}
}

func TestReplaceOne(t *testing.T) {
	initial := "alpha beta alpha gamma alpha"
	pt := buffer.NewPieceTableFromString(initial)

	opts := SearchOptions{Query: "alpha", CaseSensitive: true}
	matches, err := FindInPieceTable(pt, opts, "test.txt")
	if err != nil || len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d (err: %v)", len(matches), err)
	}

	// Replace the middle match (index 1)
	err = ReplaceOne(pt, matches[1], opts, "OMEGA")
	if err != nil {
		t.Fatalf("replace one failed: %v", err)
	}

	expected := "alpha beta OMEGA gamma alpha"
	if pt.Text() != expected {
		t.Fatalf("expected %q, got %q", expected, pt.Text())
	}
}

func TestPreviewLineReplacement(t *testing.T) {
	line := "fmt.Printf(\"log: %s\", msg)"
	m := Match{Text: "Printf"}
	opts := SearchOptions{Query: "Printf"}

	oldLine, newLine := PreviewLineReplacement(line, m, opts, "Println")
	if oldLine != line {
		t.Fatalf("expected oldLine to match original, got %s", oldLine)
	}
	if !strings.Contains(newLine, "fmt.Println(\"log: %s\", msg)") {
		t.Fatalf("expected replacement in newLine, got %s", newLine)
	}
}

func TestProjectSearch(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(file1, []byte("package main\nfunc Target() {}\n"), 0644)

	file2 := filepath.Join(tmpDir, "util.go")
	_ = os.WriteFile(file2, []byte("package main\nfunc Target() int { return 1 }\n"), 0644)

	ignoredDir := filepath.Join(tmpDir, "target")
	_ = os.Mkdir(ignoredDir, 0755)
	_ = os.WriteFile(filepath.Join(ignoredDir, "artifact.go"), []byte("func Target() {}\n"), 0644)

	opts := SearchOptions{Query: "Target"}
	results, err := ProjectSearch(tmpDir, opts, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match file1 and file2, but NOT target/artifact.go
	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(results))
	}
}

func TestASTSearch(t *testing.T) {
	code := []byte("package main\n\nfunc Alpha() {}\nfunc Beta() int { return 42 }\n")
	lang, _ := treesitter.DetectLanguage("main.go")
	if lang == nil {
		t.Skip("Tree-sitter Go grammar not available")
	}

	query := `(function_declaration name: (identifier) @name)`
	matches, err := ASTSearch(code, lang, query)
	if err != nil {
		t.Fatalf("ast search failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 function names, got %d", len(matches))
	}
	if matches[0].Text != "Alpha" || matches[1].Text != "Beta" {
		t.Fatalf("expected Alpha and Beta, got %s and %s", matches[0].Text, matches[1].Text)
	}
}
