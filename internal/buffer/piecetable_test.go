package buffer

import (
	"fmt"
	"testing"
)

func TestPieceTableEmpty(t *testing.T) {
	pt := NewPieceTable()
	if pt.Len() != 0 {
		t.Fatalf("expected len 0, got %d", pt.Len())
	}
	if pt.RuneCount() != 0 {
		t.Fatalf("expected rune count 0, got %d", pt.RuneCount())
	}
	if pt.LineCount() != 1 {
		t.Fatalf("expected line count 1, got %d", pt.LineCount())
	}
	if pt.Text() != "" {
		t.Fatalf("expected empty text, got %q", pt.Text())
	}

	line, err := pt.Line(0)
	if err != nil {
		t.Fatalf("failed to get line 0: %v", err)
	}
	if line != "" {
		t.Fatalf("expected empty line 0, got %q", line)
	}
}

func TestPieceTableInitialText(t *testing.T) {
	initial := "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n"
	pt := NewPieceTableFromString(initial)

	if pt.Text() != initial {
		t.Fatalf("expected %q, got %q", initial, pt.Text())
	}
	if pt.LineCount() != 6 {
		t.Fatalf("expected line count 6, got %d", pt.LineCount())
	}

	line0, err := pt.Line(0)
	if err != nil || line0 != "package main" {
		t.Fatalf("expected 'package main', got %q (err: %v)", line0, err)
	}

	line2, err := pt.Line(2)
	if err != nil || line2 != "func main() {" {
		t.Fatalf("expected 'func main() {', got %q (err: %v)", line2, err)
	}
}

func TestPieceTableInsertions(t *testing.T) {
	pt := NewPieceTable()

	// Insert at start
	if err := pt.Insert(0, "world"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if pt.Text() != "world" {
		t.Fatalf("expected 'world', got %q", pt.Text())
	}

	// Insert before start
	if err := pt.Insert(0, "hello "); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if pt.Text() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", pt.Text())
	}

	// Insert in the middle
	if err := pt.Insert(5, " beautiful"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if pt.Text() != "hello beautiful world" {
		t.Fatalf("expected 'hello beautiful world', got %q", pt.Text())
	}

	// Insert at the end
	if err := pt.Insert(pt.Len(), "!"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if pt.Text() != "hello beautiful world!" {
		t.Fatalf("expected 'hello beautiful world!', got %q", pt.Text())
	}
}

func TestPieceTableDeletions(t *testing.T) {
	pt := NewPieceTableFromString("hello beautiful world!")

	// Delete from middle: remove " beautiful" (10 bytes starting at index 5)
	if err := pt.Delete(5, 10); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if pt.Text() != "hello world!" {
		t.Fatalf("expected 'hello world!', got %q", pt.Text())
	}

	// Delete from end: remove "!"
	if err := pt.Delete(pt.Len()-1, 1); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if pt.Text() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", pt.Text())
	}

	// Delete from start: remove "hello "
	if err := pt.Delete(0, 6); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if pt.Text() != "world" {
		t.Fatalf("expected 'world', got %q", pt.Text())
	}

	// Delete everything
	if err := pt.Delete(0, pt.Len()); err != nil {
		t.Fatalf("delete all failed: %v", err)
	}
	if pt.Text() != "" {
		t.Fatalf("expected empty string, got %q", pt.Text())
	}
}

func TestPieceTableScandinavianAndMultibyte(t *testing.T) {
	// Nordic characters: ä, ö, å (2 bytes each in UTF-8)
	text := "Mökkiranta Åke älä jätä"
	pt := NewPieceTableFromString(text)

	// UTF-8 byte length is greater than rune count
	if pt.Len() <= pt.RuneCount() {
		t.Fatalf("expected byte len (%d) > rune count (%d)", pt.Len(), pt.RuneCount())
	}

	// Insert another Scandinavian word
	if err := pt.Insert(len("Mökki"), "lämmin "); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	expected := "Mökkilämmin ranta Åke älä jätä"
	if pt.Text() != expected {
		t.Fatalf("expected %q, got %q", expected, pt.Text())
	}

	// Verify PositionToOffset and OffsetToPosition
	// Line 0, Col 0 -> offset 0
	off, err := pt.PositionToOffset(Position{Line: 0, Col: 0})
	if err != nil || off != 0 {
		t.Fatalf("expected offset 0, got %d (err: %v)", off, err)
	}

	// Test multi-line Scandinavian text
	multi := "Rivi 1: äöå\nRivi 2: Räksä\nRivi 3: loppu"
	ptMulti := NewPieceTableFromString(multi)

	pos2, err := ptMulti.OffsetToPosition(len("Rivi 1: äöå\n"))
	if err != nil {
		t.Fatalf("OffsetToPosition failed: %v", err)
	}
	if pos2.Line != 1 || pos2.Col != 0 {
		t.Fatalf("expected Line 1, Col 0, got Line %d, Col %d", pos2.Line, pos2.Col)
	}

	// Line 1, Col 8 (which is after "Räksä")
	offAfterRäksä, err := ptMulti.PositionToOffset(Position{Line: 1, Col: 8})
	if err != nil {
		t.Fatalf("PositionToOffset failed: %v", err)
	}
	posBack, err := ptMulti.OffsetToPosition(offAfterRäksä)
	if err != nil || posBack.Line != 1 || posBack.Col != 8 {
		t.Fatalf("roundtrip mismatch: expected (1, 8), got (%d, %d)", posBack.Line, posBack.Col)
	}
}

func TestPieceTableUndoRedo(t *testing.T) {
	pt := NewPieceTableFromString("initial")

	// Step 1: insert " text"
	if err := pt.Insert(pt.Len(), " text"); err != nil {
		t.Fatalf("insert 1 failed: %v", err)
	}
	if pt.Text() != "initial text" {
		t.Fatalf("expected 'initial text', got %q", pt.Text())
	}

	// Step 2: insert " more"
	if err := pt.Insert(pt.Len(), " more"); err != nil {
		t.Fatalf("insert 2 failed: %v", err)
	}
	if pt.Text() != "initial text more" {
		t.Fatalf("expected 'initial text more', got %q", pt.Text())
	}

	// Undo step 2
	if !pt.Undo() {
		t.Fatal("expected Undo to succeed")
	}
	if pt.Text() != "initial text" {
		t.Fatalf("after undo 1: expected 'initial text', got %q", pt.Text())
	}

	// Undo step 1
	if !pt.Undo() {
		t.Fatal("expected Undo to succeed")
	}
	if pt.Text() != "initial" {
		t.Fatalf("after undo 2: expected 'initial', got %q", pt.Text())
	}

	// No more undo
	if pt.Undo() {
		t.Fatal("expected Undo to return false at root")
	}

	// Redo step 1
	if !pt.Redo() {
		t.Fatal("expected Redo to succeed")
	}
	if pt.Text() != "initial text" {
		t.Fatalf("after redo 1: expected 'initial text', got %q", pt.Text())
	}

	// Redo step 2
	if !pt.Redo() {
		t.Fatal("expected Redo to succeed")
	}
	if pt.Text() != "initial text more" {
		t.Fatalf("after redo 2: expected 'initial text more', got %q", pt.Text())
	}
}

func TestPieceTableTransactions(t *testing.T) {
	pt := NewPieceTableFromString("start ")

	// Simulate user typing a whole word as a single transaction
	pt.BeginTransaction()
	for _, ch := range "typing" {
		_ = pt.Insert(pt.Len(), string(ch))
	}
	pt.CommitTransaction()

	if pt.Text() != "start typing" {
		t.Fatalf("expected 'start typing', got %q", pt.Text())
	}

	// A single undo should revert the entire transaction
	if !pt.Undo() {
		t.Fatal("expected Undo to succeed")
	}
	if pt.Text() != "start " {
		t.Fatalf("expected 'start ', got %q", pt.Text())
	}

	// Redo restores the whole word
	if !pt.Redo() {
		t.Fatal("expected Redo to succeed")
	}
	if pt.Text() != "start typing" {
		t.Fatalf("expected 'start typing', got %q", pt.Text())
	}
}

func TestPieceTableReadOnly(t *testing.T) {
	pt := NewPieceTableFromString("protected")
	pt.SetReadOnly(true)

	if err := pt.Insert(0, "a"); err == nil {
		t.Fatal("expected error inserting into read-only buffer")
	}
	if err := pt.Delete(0, 1); err == nil {
		t.Fatal("expected error deleting from read-only buffer")
	}

	pt.SetReadOnly(false)
	if err := pt.Insert(0, "not "); err != nil {
		t.Fatalf("insert failed after unsetting read-only: %v", err)
	}
}

func TestPieceTableStress(t *testing.T) {
	pt := NewPieceTable()

	// Perform 1000 sequential appends
	for i := 0; i < 1000; i++ {
		str := fmt.Sprintf("line %d: code\n", i)
		if err := pt.Insert(pt.Len(), str); err != nil {
			t.Fatalf("stress insert failed at %d: %v", i, err)
		}
	}

	if pt.LineCount() != 1001 {
		t.Fatalf("expected 1001 lines, got %d", pt.LineCount())
	}

	line500, err := pt.Line(500)
	if err != nil || line500 != "line 500: code" {
		t.Fatalf("expected 'line 500: code', got %q (err: %v)", line500, err)
	}
}

func TestPieceTableErrorsAndEdgeCases(t *testing.T) {
	pt := NewPieceTableFromString("line 1\nline 2\nline 3")

	// Bytes()
	b := pt.Bytes()
	if string(b) != "line 1\nline 2\nline 3" {
		t.Fatalf("unexpected Bytes: %q", string(b))
	}

	// Substr out of range
	_, err := pt.Substr(-1, 2)
	if err == nil {
		t.Fatal("expected error for negative offset")
	}
	_, err = pt.Substr(0, -1)
	if err == nil {
		t.Fatal("expected error for negative length")
	}
	_, err = pt.Substr(0, 500)
	if err == nil {
		t.Fatal("expected error for length exceeding buffer")
	}
	s, err := pt.Substr(0, 0)
	if err != nil || s != "" {
		t.Fatalf("expected empty substring for len 0, got %q (err: %v)", s, err)
	}

	// Line out of range
	_, err = pt.Line(-1)
	if err == nil {
		t.Fatal("expected error for line < 0")
	}
	_, err = pt.Line(100)
	if err == nil {
		t.Fatal("expected error for line >= LineCount")
	}

	// LineOffset out of range
	_, err = pt.LineOffset(-1)
	if err == nil {
		t.Fatal("expected error for LineOffset < 0")
	}
	_, err = pt.LineOffset(100)
	if err == nil {
		t.Fatal("expected error for LineOffset >= LineCount")
	}

	// OffsetToPosition out of range
	_, err = pt.OffsetToPosition(-1)
	if err == nil {
		t.Fatal("expected error for OffsetToPosition < 0")
	}
	_, err = pt.OffsetToPosition(500)
	if err == nil {
		t.Fatal("expected error for OffsetToPosition > Len")
	}

	// PositionToOffset out of range
	_, err = pt.PositionToOffset(Position{Line: -1, Col: 0})
	if err == nil {
		t.Fatal("expected error for PositionToOffset line < 0")
	}
	_, err = pt.PositionToOffset(Position{Line: 100, Col: 0})
	if err == nil {
		t.Fatal("expected error for PositionToOffset line >= LineCount")
	}

	// Insert and Delete out of range
	if err := pt.Insert(-1, "test"); err == nil {
		t.Fatal("expected error inserting at negative offset")
	}
	if err := pt.Insert(500, "test"); err == nil {
		t.Fatal("expected error inserting beyond Len")
	}
	if err := pt.Delete(-1, 1); err == nil {
		t.Fatal("expected error deleting at negative offset")
	}
	if err := pt.Delete(0, -1); err == nil {
		t.Fatal("expected error deleting negative length")
	}
	if err := pt.Delete(0, 500); err == nil {
		t.Fatal("expected error deleting beyond Len")
	}
}

func TestPieceTableCanUndoRedo(t *testing.T) {
	pt := NewPieceTableFromString("initial")
	if pt.CanUndo() {
		t.Fatal("expected CanUndo == false on clean buffer")
	}
	if pt.CanRedo() {
		t.Fatal("expected CanRedo == false on clean buffer")
	}

	_ = pt.Insert(pt.Len(), " edit")
	if !pt.CanUndo() {
		t.Fatal("expected CanUndo == true after edit")
	}

	pt.Undo()
	if !pt.CanRedo() {
		t.Fatal("expected CanRedo == true after undo")
	}

	pt.Redo()
	if pt.CanRedo() {
		t.Fatal("expected CanRedo == false after redoing all")
	}
}

func TestPieceTableUndoWhileInTransaction(t *testing.T) {
	pt := NewPieceTableFromString("base")
	pt.BeginTransaction()
	_ = pt.Insert(pt.Len(), " draft")

	// Undo while transaction is still open
	if !pt.Undo() {
		t.Fatal("expected Undo to succeed while in transaction")
	}
	if pt.Text() != "base" {
		t.Fatalf("expected 'base' after undo in transaction, got %q", pt.Text())
	}
}
