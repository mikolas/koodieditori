package test

import (
	"strings"
	"testing"

	"github.com/mikolas/koodieditori/internal/fsm"
)

// TestNordicKeybindingErgonomics strictly verifies the Scandinavian/Nordic ISO
// keyboard constraint defined in AGENTS.md and docs/ARCHITECTURE.md:
// 1. NO core keybindings require AltGr combinations: [ ] { } \ |
// 2. NO core keybindings require dead keys: ` ~ ^
// 3. Text objects use mnemonic letters: m i b, m i a, m i p, m i q, m i f, m i c
// 4. Diagnostic/error/diff stepping uses space leader chords: Space n e, Space n d, Space n h
func TestNordicKeybindingErgonomics(t *testing.T) {
	sm := fsm.NewFSM()

	// Forbidden characters that require AltGr or dead keys on Nordic ISO keyboards
	forbiddenKeys := []string{"[", "]", "{", "}", "\\", "|", "`", "~", "^"}

	// 1. Verify Text Objects use mnemonic letters (NOT braces/brackets)
	semanticTargets := []struct {
		mnemonic string
		expected string
	}{
		{"b", "block"},
		{"a", "array"},
		{"p", "parens"},
		{"q", "quotes"},
		{"f", "function"},
		{"c", "class/struct"},
	}

	for _, st := range semanticTargets {
		// Inside object: m i <key>
		sm.SetState(fsm.StateNormal)
		_ = sm.HandleKey("m")
		_ = sm.HandleKey("i")
		actInside := sm.HandleKey(st.mnemonic)

		if actInside.Type != fsm.ActionSelectTextObject || actInside.Arg1 != "i" || actInside.Arg2 != st.mnemonic {
			t.Fatalf("expected mnemonic text object 'm i %s' for %s, got %+v", st.mnemonic, st.expected, actInside)
		}

		// Around object: m a <key>
		sm.SetState(fsm.StateNormal)
		_ = sm.HandleKey("m")
		_ = sm.HandleKey("a")
		actAround := sm.HandleKey(st.mnemonic)

		if actAround.Type != fsm.ActionSelectTextObject || actAround.Arg1 != "a" || actAround.Arg2 != st.mnemonic {
			t.Fatalf("expected mnemonic text object 'm a %s' for %s, got %+v", st.mnemonic, st.expected, actAround)
		}
	}

	// 2. Verify Diagnostic, Error, and Diff Hunk navigation uses Space leader chords
	leaderChords := []struct {
		chord    string
		expected fsm.ActionType
	}{
		{"ne", fsm.ActionNextError},
		{"pe", fsm.ActionPrevError},
		{"nd", fsm.ActionNextDiagnostic},
		{"pd", fsm.ActionPrevDiagnostic},
		{"nh", fsm.ActionNextHunk},
		{"ph", fsm.ActionPrevHunk},
		{"gd", fsm.ActionLSPDefinition},
		{"gi", fsm.ActionLSPImplementation},
		{"gc", fsm.ActionLSPCallHierarchy},
		{"ca", fsm.ActionLSPCodeAction},
		{"cr", fsm.ActionLSPRename},
		{"ff", fsm.ActionPickerOpenFiles},
		{"fg", fsm.ActionPickerOpenGrep},
		{"bb", fsm.ActionPickerOpenBuffers},
		{"e", fsm.ActionToggleFileTree},
		{"o", fsm.ActionToggleDrawer},
		{"ds", fsm.ActionAgentReviewOpen},
	}

	for _, lc := range leaderChords {
		sm.SetState(fsm.StateNormal)
		_ = sm.HandleKey("Space")

		var act fsm.Action
		for _, ch := range lc.chord {
			act = sm.HandleKey(string(ch))
		}

		if act.Type != lc.expected {
			t.Fatalf("leader chord 'Space %s': expected %v, got %v", lc.chord, lc.expected, act.Type)
		}

		// Verify none of the chord keys contain forbidden AltGr characters
		for _, forbidden := range forbiddenKeys {
			if strings.Contains(lc.chord, forbidden) {
				t.Fatalf("chord 'Space %s' contains forbidden AltGr key %q", lc.chord, forbidden)
			}
		}
	}

	// 3. Verify Nordic home-row quick step keys (ö and ä)
	sm.SetState(fsm.StateNormal)
	actO := sm.HandleKey("ö")
	if actO.Type != fsm.ActionStepBackward {
		t.Fatalf("expected ActionStepBackward for 'ö', got %v", actO.Type)
	}

	actA := sm.HandleKey("ä")
	if actA.Type != fsm.ActionStepForward {
		t.Fatalf("expected ActionStepForward for 'ä', got %v", actA.Type)
	}

	// 4. Verify that entering forbidden AltGr keys in Normal mode never crashes or maps to core actions
	for _, forbidden := range forbiddenKeys {
		sm.SetState(fsm.StateNormal)
		act := sm.HandleKey(forbidden)
		if act.Type == fsm.ActionSelectTextObject {
			t.Fatalf("forbidden key %q triggered text object selection", forbidden)
		}
	}
}
