package lsp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPathToURIAndURIToPath(t *testing.T) {
	testPath := "/home/user/project/main.go"
	uri := PathToURI(testPath)

	if uri != "file:///home/user/project/main.go" {
		t.Errorf("PathToURI(%q) = %q, want file:///home/user/project/main.go", testPath, uri)
	}

	back := URIToPath(uri)
	if filepath.Clean(back) != filepath.Clean(testPath) {
		t.Errorf("URIToPath(%q) = %q, want %q", uri, back, testPath)
	}

	// Relative path resolution
	relURI := PathToURI("main.go")
	if !filepath.IsAbs(URIToPath(relURI)) {
		t.Errorf("URIToPath on resolved relative path should be absolute: %s", URIToPath(relURI))
	}
}

func TestExtractHoverText(t *testing.T) {
	// Plain string
	if got := ExtractHoverText("func Foo()"); got != "func Foo()" {
		t.Errorf("got %q, want %q", got, "func Foo()")
	}

	// MarkupContent map
	mc := map[string]any{
		"kind":  "markdown",
		"value": "```go\nfunc Bar()\n```",
	}
	if got := ExtractHoverText(mc); got != "```go\nfunc Bar()\n```" {
		t.Errorf("got %q", got)
	}

	// Slice of content
	slice := []any{
		"Header line",
		map[string]any{"kind": "markdown", "value": "Detail line"},
	}
	expected := "Header line\n\nDetail line"
	if got := ExtractHoverText(slice); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestDiagnosticSerialization(t *testing.T) {
	diag := Diagnostic{
		Range: Range{
			Start: Position{Line: 10, Character: 4},
			End:   Position{Line: 10, Character: 12},
		},
		Severity: SeverityError,
		Message:  "syntax error: unexpected newline",
		Source:   "compiler",
	}

	data, err := json.Marshal(diag)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Diagnostic
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Message != diag.Message || decoded.Severity != SeverityError {
		t.Errorf("decoded diag mismatch: %+v", decoded)
	}
}

func TestJSONRPCIDSupport(t *testing.T) {
	// 1. Integer ID in Request and Response
	intReqJSON := `{"jsonrpc":"2.0","id":42,"method":"test"}`
	var req Request
	if err := json.Unmarshal([]byte(intReqJSON), &req); err != nil {
		t.Fatalf("failed to unmarshal int ID request: %v", err)
	}
	if req.ID.IsStr || req.ID.Num != 42 || req.ID.String() != "42" {
		t.Fatalf("expected int ID 42, got %+v", req.ID)
	}

	// 2. String ID in Request and Response (e.g. UUID)
	strRespJSON := `{"jsonrpc":"2.0","id":"uuid-9876-abcd","result":{}}`
	var resp Response
	if err := json.Unmarshal([]byte(strRespJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal string ID response: %v", err)
	}
	if resp.ID == nil || !resp.ID.IsStr || resp.ID.Str != "uuid-9876-abcd" || resp.ID.String() != "uuid-9876-abcd" {
		t.Fatalf("expected string ID 'uuid-9876-abcd', got %+v", resp.ID)
	}

	// 3. Marshaling
	marshaledReq, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(marshaledReq) != intReqJSON {
		t.Fatalf("expected marshaled request %q, got %q", intReqJSON, string(marshaledReq))
	}
}

func TestUTF16PositionConversion(t *testing.T) {
	// Line with ASCII and BMP characters: "ä := 1"
	// Rune 0: 'ä' (BMP, 1 rune, 1 utf16 code unit, 2 utf8 bytes)
	// Rune 1: ' '
	lineBMP := "ä := 1"
	if u := RuneColToUTF16Col(lineBMP, 0); u != 0 {
		t.Errorf("expected rune 0 -> utf16 0, got %d", u)
	}
	if u := RuneColToUTF16Col(lineBMP, 1); u != 1 {
		t.Errorf("expected rune 1 -> utf16 1, got %d", u)
	}
	if r := UTF16ColToRuneCol(lineBMP, 1); r != 1 {
		t.Errorf("expected utf16 1 -> rune 1, got %d", r)
	}

	// Line with non-BMP characters (surrogate pairs): "🚀 := 1"
	// Rune 0: '🚀' (U+1F680, 1 rune, 2 UTF-16 code units)
	// Rune 1: ' '  (UTF-16 code unit index 2)
	// Rune 2: ':'  (UTF-16 code unit index 3)
	lineNonBMP := "🚀 := 1"
	if u := RuneColToUTF16Col(lineNonBMP, 0); u != 0 {
		t.Errorf("expected rune 0 -> utf16 0, got %d", u)
	}
	if u := RuneColToUTF16Col(lineNonBMP, 1); u != 2 {
		t.Errorf("expected rune 1 (after rocket) -> utf16 2, got %d", u)
	}
	if u := RuneColToUTF16Col(lineNonBMP, 2); u != 3 {
		t.Errorf("expected rune 2 (':') -> utf16 3, got %d", u)
	}

	// Reverse conversion from UTF-16 to Rune column
	if r := UTF16ColToRuneCol(lineNonBMP, 0); r != 0 {
		t.Errorf("expected utf16 0 -> rune 0, got %d", r)
	}
	// Middle of surrogate pair snaps to rune 0
	if r := UTF16ColToRuneCol(lineNonBMP, 1); r != 0 {
		t.Errorf("expected utf16 1 -> rune 0, got %d", r)
	}
	if r := UTF16ColToRuneCol(lineNonBMP, 2); r != 1 {
		t.Errorf("expected utf16 2 -> rune 1, got %d", r)
	}
	if r := UTF16ColToRuneCol(lineNonBMP, 3); r != 2 {
		t.Errorf("expected utf16 3 -> rune 2, got %d", r)
	}
}
