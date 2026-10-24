package buffer

import (
	"strings"
	"testing"
)

func TestLineCRLFTrimming(t *testing.T) {
	crlfText := "line 1\r\nline 2\r\nline 3\r\n"
	pt := NewPieceTableFromString(crlfText)

	if pt.LineCount() != 4 {
		t.Fatalf("expected 4 lines, got %d", pt.LineCount())
	}

	l0, err := pt.Line(0)
	if err != nil || l0 != "line 1" {
		t.Fatalf("expected 'line 1' without \\r, got %q (err: %v)", l0, err)
	}
	if strings.Contains(l0, "\r") {
		t.Fatal("line 0 still contains carriage return \\r")
	}

	l1, err := pt.Line(1)
	if err != nil || l1 != "line 2" {
		t.Fatalf("expected 'line 2' without \\r, got %q (err: %v)", l1, err)
	}

	l2, err := pt.Line(2)
	if err != nil || l2 != "line 3" {
		t.Fatalf("expected 'line 3' without \\r, got %q (err: %v)", l2, err)
	}
}
