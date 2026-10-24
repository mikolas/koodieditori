package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/fsm"
)

func TestDeleteCharRuneSafety(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	curr := app.CurrentBuffer()
	// Text contains 2-byte ä, ö and 4-byte emoji 🚀
	_ = curr.PT.Insert(0, "äö🚀abc")

	// Cursor at 0 ('ä', 2 bytes)
	curr.CursorPos.Col = 0
	app.executeAction(fsm.Action{Type: fsm.ActionDeleteChar})

	got := curr.PT.Text()
	expected := "ö🚀abc"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("buffer contains invalid UTF-8 after deleting multibyte character")
	}

	// Delete 'ö' (2 bytes)
	app.executeAction(fsm.Action{Type: fsm.ActionDeleteChar})
	got = curr.PT.Text()
	expected = "🚀abc"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("buffer contains invalid UTF-8 after deleting 'ö'")
	}

	// Delete '🚀' (4 bytes)
	app.executeAction(fsm.Action{Type: fsm.ActionDeleteChar})
	got = curr.PT.Text()
	expected = "abc"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("buffer contains invalid UTF-8 after deleting 4-byte emoji")
	}
}

func TestRenameInputRuneSafeBackspace(t *testing.T) {
	cfg := config.DefaultConfig()
	app := NewApp(cfg, nil)
	defer app.Close()

	app.RenameActive = true
	app.RenameInput = "mäki"

	// Press backspace
	app.executeAction(fsm.Action{Type: fsm.ActionBackspace})
	if app.RenameInput != "mäk" {
		t.Fatalf("expected 'mäk', got %q", app.RenameInput)
	}

	// Press backspace again ('k')
	app.executeAction(fsm.Action{Type: fsm.ActionBackspace})
	if app.RenameInput != "mä" {
		t.Fatalf("expected 'mä', got %q", app.RenameInput)
	}

	// Press backspace on 'ä' (multibyte)
	app.executeAction(fsm.Action{Type: fsm.ActionBackspace})
	if app.RenameInput != "m" {
		t.Fatalf("expected 'm', got %q", app.RenameInput)
	}
	if !utf8.ValidString(app.RenameInput) {
		t.Fatal("RenameInput corrupted by backspace on multibyte character")
	}
}
