package fsm

import (
	"testing"
)

func TestFSMInitialState(t *testing.T) {
	f := NewFSM()
	if f.State() != StateNormal {
		t.Fatalf("expected initial state NORMAL, got %v", f.State())
	}
}

func TestFSMNormalToInsert(t *testing.T) {
	f := NewFSM()

	// 'i' enters insert mode
	act := f.HandleKey("i")
	if f.State() != StateInsert {
		t.Fatalf("expected state INSERT, got %v", f.State())
	}
	if act.Type != ActionNone {
		t.Fatalf("expected ActionNone, got %v", act.Type)
	}

	// Esc returns to normal
	act = f.HandleKey("esc")
	if f.State() != StateNormal {
		t.Fatalf("expected state NORMAL, got %v", f.State())
	}
	if act.Type != ActionMoveLeft {
		t.Fatalf("expected ActionMoveLeft on Esc, got %v", act.Type)
	}
}

func TestFSMFastHomeRowExit(t *testing.T) {
	f := NewFSM()
	f.HandleKey("i")

	// Type 'j', then 'k' in quick succession (<200ms)
	f.HandleKey("j")
	act := f.HandleKey("k")

	if f.State() != StateNormal {
		t.Fatalf("expected fast home-row exit to NORMAL, got %v", f.State())
	}
	if act.Type != ActionBackspace {
		t.Fatalf("expected ActionBackspace to erase 'j', got %v", act.Type)
	}
}

func TestFSMSelectionFirst(t *testing.T) {
	f := NewFSM()

	// 'w' selects word and transitions to SELECT mode
	act := f.HandleKey("w")
	if f.State() != StateSelect {
		t.Fatalf("expected state SELECT, got %v", f.State())
	}
	if act.Type != ActionSelectWord {
		t.Fatalf("expected ActionSelectWord, got %v", act.Type)
	}

	// 'd' in SELECT deletes selection and returns to NORMAL
	act = f.HandleKey("d")
	if f.State() != StateNormal {
		t.Fatalf("expected state NORMAL after delete, got %v", f.State())
	}
	if act.Type != ActionDeleteSelection {
		t.Fatalf("expected ActionDeleteSelection, got %v", act.Type)
	}

	// 'x' selects line, 'c' changes into INSERT
	f.HandleKey("x")
	if f.State() != StateSelect {
		t.Fatalf("expected state SELECT, got %v", f.State())
	}
	act = f.HandleKey("c")
	if f.State() != StateInsert {
		t.Fatalf("expected state INSERT after change, got %v", f.State())
	}
	if act.Type != ActionChangeSelection {
		t.Fatalf("expected ActionChangeSelection, got %v", act.Type)
	}
}

func TestFSMTextObjects(t *testing.T) {
	f := NewFSM()

	// Press 'm' -> MATCH mode
	act := f.HandleKey("m")
	if f.State() != StateMatchPending {
		t.Fatalf("expected state MATCH, got %v", f.State())
	}

	// Press 'i' -> inside
	act = f.HandleKey("i")
	if f.State() != StateMatchPending {
		t.Fatalf("expected state MATCH waiting for target, got %v", f.State())
	}

	// Press 'b' -> block (curly braces)
	act = f.HandleKey("b")
	if f.State() != StateSelect {
		t.Fatalf("expected state SELECT after text object, got %v", f.State())
	}
	if act.Type != ActionSelectTextObject || act.Arg1 != "i" || act.Arg2 != "b" {
		t.Fatalf("expected ActionSelectTextObject(i, b), got %v (%s, %s)", act.Type, act.Arg1, act.Arg2)
	}
}

func TestFSMLadderChords(t *testing.T) {
	f := NewFSM()

	// Space f f -> Picker files
	f.HandleKey("Space")
	if f.State() != StateLeader {
		t.Fatalf("expected state LEADER, got %v", f.State())
	}
	f.HandleKey("f")
	if f.State() != StateLeader {
		t.Fatalf("expected state LEADER prefix, got %v", f.State())
	}
	act := f.HandleKey("f")
	if f.State() != StatePicker {
		t.Fatalf("expected state PICKER, got %v", f.State())
	}
	if act.Type != ActionPickerOpenFiles {
		t.Fatalf("expected ActionPickerOpenFiles, got %v", act.Type)
	}

	// Esc in picker -> Normal
	f.HandleKey("esc")
	if f.State() != StateNormal {
		t.Fatalf("expected state NORMAL after picker cancel, got %v", f.State())
	}

	// Space o -> Drawer toggle
	f.HandleKey("Space")
	act = f.HandleKey("o")
	if f.State() != StateDrawer {
		t.Fatalf("expected state DRAWER, got %v", f.State())
	}
	if act.Type != ActionToggleDrawer {
		t.Fatalf("expected ActionToggleDrawer, got %v", act.Type)
	}

	// q closes drawer -> Normal
	act = f.HandleKey("q")
	if f.State() != StateNormal {
		t.Fatalf("expected state NORMAL after drawer close, got %v", f.State())
	}
	if act.Type != ActionDrawerClose {
		t.Fatalf("expected ActionDrawerClose, got %v", act.Type)
	}
}

func TestFSMNordicKeyChords(t *testing.T) {
	f := NewFSM()

	// Direct stepping keys: 'ö' (backward) and 'ä' (forward)
	act := f.HandleKey("ö")
	if act.Type != ActionStepBackward {
		t.Fatalf("expected ActionStepBackward for 'ö', got %v", act.Type)
	}

	act = f.HandleKey("ä")
	if act.Type != ActionStepForward {
		t.Fatalf("expected ActionStepForward for 'ä', got %v", act.Type)
	}

	// Space n e -> Next Error
	f.HandleKey("Space")
	f.HandleKey("n")
	act = f.HandleKey("e")
	if act.Type != ActionNextError {
		t.Fatalf("expected ActionNextError, got %v", act.Type)
	}

	// Space p e -> Prev Error
	f.HandleKey("Space")
	f.HandleKey("p")
	act = f.HandleKey("e")
	if act.Type != ActionPrevError {
		t.Fatalf("expected ActionPrevError, got %v", act.Type)
	}

	// Space n d -> Next Diagnostic
	f.HandleKey("Space")
	f.HandleKey("n")
	act = f.HandleKey("d")
	if act.Type != ActionNextDiagnostic {
		t.Fatalf("expected ActionNextDiagnostic, got %v", act.Type)
	}

	// Space d s -> Agent Transaction Review
	f.HandleKey("Space")
	f.HandleKey("d")
	act = f.HandleKey("s")
	if f.State() != StateDiffReview {
		t.Fatalf("expected state DIFF_REVIEW, got %v", f.State())
	}
	if act.Type != ActionAgentReviewOpen {
		t.Fatalf("expected ActionAgentReviewOpen, got %v", act.Type)
	}
}

func TestFSMDirectTabSwitching(t *testing.T) {
	f := NewFSM()

	// Alt+1 .. Alt+9 direct tab switching
	act := f.HandleKey("alt+3")
	if act.Type != ActionTabSwitch || act.Int != 3 {
		t.Fatalf("expected ActionTabSwitch(3), got %v (%d)", act.Type, act.Int)
	}

	act = f.HandleKey("alt+9")
	if act.Type != ActionTabSwitch || act.Int != 9 {
		t.Fatalf("expected ActionTabSwitch(9), got %v (%d)", act.Type, act.Int)
	}
}

func TestFSMStateString(t *testing.T) {
	states := []State{
		StateNormal, StateInsert, StateSelect, StateLeader,
		StateMatchPending, StatePicker, StateDrawer, StateDiffReview,
		State(99),
	}
	for _, s := range states {
		if s.String() == "" {
			t.Fatalf("expected non-empty string for state %d", s)
		}
	}
}

func TestFSMLeaderAllChords(t *testing.T) {
	tests := []struct {
		keys     []string
		expected ActionType
	}{
		{[]string{"f", "g"}, ActionPickerOpenGrep},
		{[]string{"b", "b"}, ActionPickerOpenBuffers},
		{[]string{"b", "d"}, ActionBufferClose},
		{[]string{"e"}, ActionToggleFileTree},
		{[]string{"a"}, ActionAlternateFile},
		{[]string{"n", "h"}, ActionNextHunk},
		{[]string{"p", "h"}, ActionPrevHunk},
		{[]string{"d", "a"}, ActionAgentAcceptAll},
		{[]string{"d", "r"}, ActionAgentRevertAll},
		{[]string{"c", "a"}, ActionLSPCodeAction},
		{[]string{"c", "r"}, ActionLSPRename},
		{[]string{"g", "d"}, ActionLSPDefinition},
		{[]string{"g", "i"}, ActionLSPImplementation},
		{[]string{"g", "c"}, ActionLSPCallHierarchy},
		{[]string{"w", ">"}, ActionResizeTreeGrow},
		{[]string{"w", "<"}, ActionResizeTreeShrink},
		{[]string{"w", "+"}, ActionResizeDrawerGrow},
		{[]string{"w", "-"}, ActionResizeDrawerShrink},
		{[]string{"m", "p"}, ActionToggleMarkdownPreview},
		{[]string{"m", "t"}, ActionToggleChecklist},
		{[]string{"m", "d"}, ActionToggleMermaidMode},
		{[]string{"m", "f"}, ActionToggleMarkdownFold},
	}

	for _, tc := range tests {
		f := NewFSM()
		f.HandleKey("Space")
		var act Action
		for _, k := range tc.keys {
			act = f.HandleKey(k)
		}
		if act.Type != tc.expected {
			t.Errorf("for chord %v: expected action %v, got %v", tc.keys, tc.expected, act.Type)
		}
	}
}

func TestFSMPickerAndDrawerInteractions(t *testing.T) {
	// Picker
	f := NewFSM()
	f.SetState(StatePicker)

	act := f.HandleKey("a")
	if act.Type != ActionPickerInput || act.String != "a" {
		t.Fatalf("expected ActionPickerInput('a'), got %v", act)
	}
	act = f.HandleKey("backspace")
	if act.Type != ActionPickerBackspace {
		t.Fatalf("expected ActionPickerBackspace, got %v", act)
	}
	act = f.HandleKey("down")
	if act.Type != ActionPickerNext {
		t.Fatalf("expected ActionPickerNext, got %v", act)
	}
	act = f.HandleKey("ctrl+p")
	if act.Type != ActionPickerPrev {
		t.Fatalf("expected ActionPickerPrev, got %v", act)
	}
	act = f.HandleKey("enter")
	if act.Type != ActionPickerSelect || f.State() != StateNormal {
		t.Fatalf("expected ActionPickerSelect and return to NORMAL, got %v", act)
	}

	// Drawer
	f.SetState(StateDrawer)
	act = f.HandleKey("Z")
	if act.Type != ActionDrawerMaximize {
		t.Fatalf("expected ActionDrawerMaximize, got %v", act)
	}
	act = f.HandleKey("j")
	if act.Type != ActionMoveDown {
		t.Fatalf("expected ActionMoveDown in drawer, got %v", act)
	}
	act = f.HandleKey("enter")
	if act.Type != ActionDrawerJump || f.State() != StateNormal {
		t.Fatalf("expected ActionDrawerJump, got %v", act)
	}

	// DiffReview
	f.SetState(StateDiffReview)
	act = f.HandleKey("j")
	if act.Type != ActionMoveDown {
		t.Fatalf("expected ActionMoveDown in diff review, got %v", act)
	}
	act = f.HandleKey("enter")
	if act.Type != ActionDrawerJump {
		t.Fatalf("expected ActionDrawerJump in diff review, got %v", act)
	}
	act = f.HandleKey("q")
	if f.State() != StateNormal {
		t.Fatalf("expected return to NORMAL on 'q' in diff review, got %v", f.State())
	}
}

func TestFSMSelectModeExtraKeys(t *testing.T) {
	f := NewFSM()
	f.SetState(StateSelect)

	act := f.HandleKey(">")
	if act.Type != ActionIndent {
		t.Fatalf("expected ActionIndent, got %v", act)
	}
	act = f.HandleKey("<")
	if act.Type != ActionOutdent {
		t.Fatalf("expected ActionOutdent, got %v", act)
	}
	act = f.HandleKey("!")
	if act.Type != ActionPipeSelection || f.State() != StateNormal {
		t.Fatalf("expected ActionPipeSelection and return to NORMAL, got %v", act)
	}
}

func TestFSMRemainingKeyBranches(t *testing.T) {
	// Normal mode arrows & keys
	f := NewFSM()
	keys := map[string]ActionType{
		"left":  ActionMoveLeft,
		"right": ActionMoveRight,
		"up":    ActionMoveUp,
		"down":  ActionMoveDown,
		"o":     ActionInsertNewline,
		"b":     ActionSelectWordBackward,
		"p":     ActionPasteAfter,
		"P":     ActionPasteBefore,
		"u":     ActionUndo,
		"U":     ActionRedo,
		"H":     ActionTabPrev,
		"L":     ActionTabNext,
	}
	for k, expected := range keys {
		f.SetState(StateNormal)
		act := f.HandleKey(k)
		if act.Type != expected {
			t.Errorf("for key %q: expected action %v, got %v", k, expected, act.Type)
		}
	}

	// Insert mode keys
	insertKeys := map[string]ActionType{
		"enter":     ActionInsertNewline,
		"tab":       ActionInsertTab,
		"backspace": ActionBackspace,
		"delete":    ActionDeleteChar,
		"left":      ActionMoveLeft,
		"right":     ActionMoveRight,
		"up":        ActionMoveUp,
		"down":      ActionMoveDown,
	}
	for k, expected := range insertKeys {
		f.SetState(StateInsert)
		act := f.HandleKey(k)
		if act.Type != expected {
			t.Errorf("insert mode for key %q: expected action %v, got %v", k, expected, act.Type)
		}
	}

	// Select mode extend keys
	selectKeys := map[string]ActionType{
		"left":  ActionExtendSelection,
		"right": ActionExtendSelection,
		"up":    ActionExtendSelection,
		"down":  ActionExtendSelection,
		"h":     ActionExtendSelection,
		"l":     ActionExtendSelection,
		"j":     ActionExtendSelection,
		"k":     ActionExtendSelection,
		"w":     ActionSelectWord,
		"b":     ActionSelectWordBackward,
		"x":     ActionSelectLine,
	}
	for k, expected := range selectKeys {
		f.SetState(StateSelect)
		act := f.HandleKey(k)
		if act.Type != expected {
			t.Errorf("select mode for key %q: expected action %v, got %v", k, expected, act.Type)
		}
	}

	// Invalid leader reset
	f.SetState(StateNormal)
	f.HandleKey("Space")
	act := f.HandleKey("z") // invalid leader chord
	if f.State() != StateNormal {
		t.Fatalf("expected return to NORMAL on invalid leader chord, got %v", f.State())
	}
	if act.Type != ActionNone {
		t.Fatalf("expected ActionNone on invalid chord, got %v", act.Type)
	}

	// Leader Esc
	f.SetState(StateNormal)
	f.HandleKey("Space")
	act = f.HandleKey("esc")
	if f.State() != StateNormal {
		t.Fatalf("expected return to NORMAL on leader esc, got %v", f.State())
	}
}

func TestFSMNavigationKeys(t *testing.T) {
	f := NewFSM()

	normalTests := []struct {
		key      string
		expected ActionType
	}{
		{"pgup", ActionPageUp},
		{"pageup", ActionPageUp},
		{"ctrl+b", ActionPageUp},
		{"ctrl+u", ActionPageUp},
		{"pgdown", ActionPageDown},
		{"pagedown", ActionPageDown},
		{"ctrl+f", ActionPageDown},
		{"ctrl+d", ActionPageDown},
		{"home", ActionMoveLineStart},
		{"0", ActionMoveLineStart},
		{"end", ActionMoveLineEnd},
		{"$", ActionMoveLineEnd},
		{"ctrl+home", ActionMoveBufferStart},
		{"ctrl+end", ActionMoveBufferEnd},
		{"G", ActionMoveBufferEnd},
		{"ctrl+left", ActionMoveWordBackward},
		{"alt+left", ActionMoveWordBackward},
		{"ctrl+right", ActionMoveWordForward},
		{"alt+right", ActionMoveWordForward},
	}

	for _, tt := range normalTests {
		f.SetState(StateNormal)
		act := f.HandleKey(tt.key)
		if act.Type != tt.expected {
			t.Errorf("normal mode %q: expected %v, got %v", tt.key, tt.expected, act.Type)
		}
	}

	insertTests := []struct {
		key      string
		expected ActionType
	}{
		{"pgup", ActionPageUp},
		{"pageup", ActionPageUp},
		{"pgdown", ActionPageDown},
		{"pagedown", ActionPageDown},
		{"home", ActionMoveLineStart},
		{"end", ActionMoveLineEnd},
		{"ctrl+home", ActionMoveBufferStart},
		{"ctrl+end", ActionMoveBufferEnd},
		{"ctrl+left", ActionMoveWordBackward},
		{"ctrl+right", ActionMoveWordForward},
	}

	for _, tt := range insertTests {
		f.SetState(StateInsert)
		act := f.HandleKey(tt.key)
		if act.Type != tt.expected {
			t.Errorf("insert mode %q: expected %v, got %v", tt.key, tt.expected, act.Type)
		}
	}

	selectTests := []struct {
		key      string
		expected ActionType
	}{
		{"pgup", ActionPageUp},
		{"pageup", ActionPageUp},
		{"pgdown", ActionPageDown},
		{"pagedown", ActionPageDown},
		{"home", ActionMoveLineStart},
		{"end", ActionMoveLineEnd},
		{"ctrl+home", ActionMoveBufferStart},
		{"ctrl+end", ActionMoveBufferEnd},
	}

	for _, tt := range selectTests {
		f.SetState(StateSelect)
		act := f.HandleKey(tt.key)
		if act.Type != tt.expected {
			t.Errorf("select mode %q: expected %v, got %v", tt.key, tt.expected, act.Type)
		}
	}
}

func TestFSMTreeNavigationAndPaneCycling(t *testing.T) {
	f := NewFSM()

	// 1. Verify StateTree string representation
	if StateTree.String() != "TREE" {
		t.Fatalf("expected 'TREE', got %q", StateTree.String())
	}

	// 2. Space e enters StateTree from StateNormal
	f.HandleKey("Space")
	act := f.HandleKey("e")
	if f.State() != StateTree {
		t.Fatalf("expected state StateTree after 'Space e', got %v", f.State())
	}
	if act.Type != ActionToggleFileTree {
		t.Fatalf("expected ActionToggleFileTree, got %v", act.Type)
	}

	// 3. Navigation inside StateTree
	treeKeys := []struct {
		key      string
		expected ActionType
	}{
		{"j", ActionTreeDown},
		{"down", ActionTreeDown},
		{"k", ActionTreeUp},
		{"up", ActionTreeUp},
		{"enter", ActionTreeToggleOrOpen},
		{"l", ActionTreeToggleOrOpen},
		{"right", ActionTreeToggleOrOpen},
		{"h", ActionTreeCollapseOrParent},
		{"left", ActionTreeCollapseOrParent},
		{"pgdown", ActionTreePageDown},
		{"ctrl+d", ActionTreePageDown},
		{"pgup", ActionTreePageUp},
		{"ctrl+u", ActionTreePageUp},
		{"home", ActionTreeTop},
		{"end", ActionTreeBottom},
		{"G", ActionTreeBottom},
		{"r", ActionTreeRefresh},
		{"a", ActionTreeCreateFile},
		{"A", ActionTreeCreateDir},
		{"d", ActionTreeDelete},
		{"ctrl+w", ActionPaneCycle},
	}

	for _, tt := range treeKeys {
		act := f.HandleKey(tt.key)
		if act.Type != tt.expected {
			t.Errorf("tree mode key %q: expected %v, got %v", tt.key, tt.expected, act.Type)
		}
		if f.State() != StateTree {
			t.Errorf("key %q unexpectedly changed state from StateTree to %v", tt.key, f.State())
		}
	}

	// gg navigation
	f.HandleKey("g")
	act = f.HandleKey("g")
	if act.Type != ActionTreeTop {
		t.Fatalf("expected ActionTreeTop for 'gg', got %v", act.Type)
	}

	// Esc in StateTree returns to StateNormal
	act = f.HandleKey("esc")
	if f.State() != StateNormal {
		t.Fatalf("expected StateNormal after Esc, got %v", f.State())
	}
	if act.Type != ActionPaneFocusBuffer {
		t.Fatalf("expected ActionPaneFocusBuffer on Esc, got %v", act.Type)
	}

	// 'q' in StateTree returns to StateNormal
	f.SetState(StateTree)
	act = f.HandleKey("q")
	if f.State() != StateNormal {
		t.Fatalf("expected StateNormal after 'q', got %v", f.State())
	}
	if act.Type != ActionPaneFocusBuffer {
		t.Fatalf("expected ActionPaneFocusBuffer on 'q', got %v", act.Type)
	}

	// 4. Pane chords (Space w w, Space w h, Space w l, Space w j)
	f.SetState(StateNormal)
	f.HandleKey("Space")
	act = f.HandleKey("w")
	act = f.HandleKey("w")
	if act.Type != ActionPaneCycle {
		t.Fatalf("expected ActionPaneCycle for 'Space w w', got %v", act.Type)
	}

	f.HandleKey("Space")
	f.HandleKey("w")
	act = f.HandleKey("h")
	if act.Type != ActionPaneFocusTree {
		t.Fatalf("expected ActionPaneFocusTree for 'Space w h', got %v", act.Type)
	}
	if f.State() != StateTree {
		t.Fatalf("expected state StateTree for 'Space w h', got %v", f.State())
	}

	f.HandleKey("Space")
	f.HandleKey("w")
	act = f.HandleKey("l")
	if act.Type != ActionPaneFocusBuffer {
		t.Fatalf("expected ActionPaneFocusBuffer for 'Space w l', got %v", act.Type)
	}
	if f.State() != StateNormal {
		t.Fatalf("expected state StateNormal for 'Space w l', got %v", f.State())
	}

	f.HandleKey("Space")
	f.HandleKey("w")
	act = f.HandleKey("j")
	if act.Type != ActionPaneFocusDrawer {
		t.Fatalf("expected ActionPaneFocusDrawer for 'Space w j', got %v", act.Type)
	}
	if f.State() != StateDrawer {
		t.Fatalf("expected state StateDrawer for 'Space w j', got %v", f.State())
	}

	// Direct ctrl+w in Normal and Drawer
	f.SetState(StateNormal)
	act = f.HandleKey("ctrl+w")
	if act.Type != ActionPaneCycle {
		t.Fatalf("expected ActionPaneCycle for 'ctrl+w' in Normal, got %v", act.Type)
	}

	f.SetState(StateDrawer)
	act = f.HandleKey("ctrl+w")
	if act.Type != ActionPaneCycle {
		t.Fatalf("expected ActionPaneCycle for 'ctrl+w' in Drawer, got %v", act.Type)
	}
}

func TestFSMLSPActionsAndModals(t *testing.T) {
	f := NewFSM()

	// Normal 'K' -> Hover
	act := f.HandleKey("K")
	if act.Type != ActionLSPHover {
		t.Fatalf("expected ActionLSPHover for 'K', got %v", act.Type)
	}

	// Space k -> Hover
	f.HandleKey("Space")
	act = f.HandleKey("k")
	if act.Type != ActionLSPHover {
		t.Fatalf("expected ActionLSPHover for 'Space k', got %v", act.Type)
	}

	// Insert ctrl+space -> Complete
	f.SetState(StateInsert)
	act = f.HandleKey("ctrl+space")
	if act.Type != ActionLSPComplete {
		t.Fatalf("expected ActionLSPComplete for 'ctrl+space', got %v", act.Type)
	}

	// StateHover navigation & escape
	f.SetState(StateHover)
	act = f.HandleKey("j")
	if act.Type != ActionMoveDown {
		t.Fatalf("expected ActionMoveDown in Hover, got %v", act.Type)
	}
	act = f.HandleKey("esc")
	if act.Type != ActionLSPCancel || f.State() != StateNormal {
		t.Fatalf("expected ActionLSPCancel and return to Normal, got %v", act.Type)
	}

	// StateCompletion navigation & selection
	f.SetState(StateCompletion)
	act = f.HandleKey("down")
	if act.Type != ActionLSPSelectNext {
		t.Fatalf("expected ActionLSPSelectNext, got %v", act.Type)
	}
	act = f.HandleKey("up")
	if act.Type != ActionLSPSelectPrev {
		t.Fatalf("expected ActionLSPSelectPrev, got %v", act.Type)
	}
	act = f.HandleKey("enter")
	if act.Type != ActionLSPSelectConfirm || f.State() != StateInsert {
		t.Fatalf("expected ActionLSPSelectConfirm and return to Insert, got %v", act.Type)
	}

	// StateCodeAction quick digit selection
	f.SetState(StateCodeAction)
	act = f.HandleKey("2")
	if act.Type != ActionLSPSelectConfirm || act.Int != 1 || f.State() != StateNormal {
		t.Fatalf("expected ActionLSPSelectConfirm with Int=1, got %v (int=%d)", act.Type, act.Int)
	}

	// StateRename typing and submit
	f.SetState(StateRename)
	act = f.HandleKey("x")
	if act.Type != ActionInsertChar || act.String != "x" {
		t.Fatalf("expected ActionInsertChar 'x', got %v", act.Type)
	}
	act = f.HandleKey("enter")
	if act.Type != ActionLSPSelectConfirm || f.State() != StateNormal {
		t.Fatalf("expected ActionLSPSelectConfirm and return to Normal, got %v", act.Type)
	}
}

func TestFSMSearchAndReplace(t *testing.T) {
	f := NewFSM()

	// 1. '/' enters StateSearch
	act := f.HandleKey("/")
	if act.Type != ActionSearchOpen || f.State() != StateSearch {
		t.Fatalf("expected ActionSearchOpen and StateSearch, got %v (%v)", act.Type, f.State())
	}

	// 2. Typing into search
	act = f.HandleKey("m")
	if act.Type != ActionSearchInput || act.String != "m" {
		t.Fatalf("expected ActionSearchInput 'm', got %v", act.Type)
	}
	act = f.HandleKey("backspace")
	if act.Type != ActionSearchBackspace {
		t.Fatalf("expected ActionSearchBackspace, got %v", act.Type)
	}

	// 3. Confirm search with enter
	act = f.HandleKey("enter")
	if act.Type != ActionSearchConfirm || f.State() != StateNormal {
		t.Fatalf("expected ActionSearchConfirm and return to StateNormal, got %v (%v)", act.Type, f.State())
	}

	// 4. In Normal mode: n and N step matches
	act = f.HandleKey("n")
	if act.Type != ActionSearchNext {
		t.Fatalf("expected ActionSearchNext for 'n', got %v", act.Type)
	}
	act = f.HandleKey("N")
	if act.Type != ActionSearchPrev {
		t.Fatalf("expected ActionSearchPrev for 'N', got %v", act.Type)
	}

	// 5. Space leader search & replace chords
	f.HandleKey("Space")
	act = f.HandleKey("s")
	if act.Type != ActionNone {
		t.Fatalf("expected waiting for second leader key, got %v", act.Type)
	}
	act = f.HandleKey("r")
	if act.Type != ActionReplaceOpen || f.State() != StateDrawer {
		t.Fatalf("expected ActionReplaceOpen and StateDrawer, got %v (%v)", act.Type, f.State())
	}

	// 6. In Drawer mode: field cycling & toggles
	act = f.HandleKey("tab")
	if act.Type != ActionReplaceNextField {
		t.Fatalf("expected ActionReplaceNextField for tab, got %v", act.Type)
	}
	act = f.HandleKey("alt+c")
	if act.Type != ActionReplaceToggleCase {
		t.Fatalf("expected ActionReplaceToggleCase for alt+c, got %v", act.Type)
	}
	act = f.HandleKey("alt+w")
	if act.Type != ActionReplaceToggleWord {
		t.Fatalf("expected ActionReplaceToggleWord for alt+w, got %v", act.Type)
	}
	act = f.HandleKey("alt+r")
	if act.Type != ActionReplaceToggleRegex {
		t.Fatalf("expected ActionReplaceToggleRegex for alt+r, got %v", act.Type)
	}

	// Close drawer with Esc
	act = f.HandleKey("esc")
	if act.Type != ActionDrawerClose || f.State() != StateNormal {
		t.Fatalf("expected ActionDrawerClose and StateNormal, got %v (%v)", act.Type, f.State())
	}

	// 7. Space c h clears highlights
	f.HandleKey("Space")
	f.HandleKey("c")
	act = f.HandleKey("h")
	if act.Type != ActionSearchClearHighlight {
		t.Fatalf("expected ActionSearchClearHighlight for 'Space c h', got %v", act.Type)
	}
}

func TestFSMHelpMode(t *testing.T) {
	// 1. Trigger via '?'
	f := NewFSM()
	act := f.HandleKey("?")
	if act.Type != ActionHelpToggle || f.State() != StateHelp {
		t.Fatalf("expected ActionHelpToggle and StateHelp for '?', got %v (%v)", act.Type, f.State())
	}

	// Dismiss via 'esc'
	act = f.HandleKey("esc")
	if act.Type != ActionHelpCancel || f.State() != StateNormal {
		t.Fatalf("expected ActionHelpCancel and StateNormal for 'esc', got %v (%v)", act.Type, f.State())
	}

	// 2. Trigger via 'f1'
	act = f.HandleKey("f1")
	if act.Type != ActionHelpToggle || f.State() != StateHelp {
		t.Fatalf("expected ActionHelpToggle and StateHelp for 'f1', got %v (%v)", act.Type, f.State())
	}
	act = f.HandleKey("q")
	if act.Type != ActionHelpCancel || f.State() != StateNormal {
		t.Fatalf("expected ActionHelpCancel and StateNormal for 'q', got %v (%v)", act.Type, f.State())
	}

	// 3. Trigger via leader chords 'Space ?' and 'Space h'
	f.HandleKey("Space")
	act = f.HandleKey("?")
	if act.Type != ActionHelpToggle || f.State() != StateHelp {
		t.Fatalf("expected ActionHelpToggle for 'Space ?', got %v (%v)", act.Type, f.State())
	}
	f.HandleKey("?") // toggles/dismisses from StateHelp
	if f.State() != StateNormal {
		t.Fatalf("expected StateNormal after '?', got %v", f.State())
	}

	f.HandleKey("Space")
	act = f.HandleKey("h")
	if act.Type != ActionHelpToggle || f.State() != StateHelp {
		t.Fatalf("expected ActionHelpToggle for 'Space h', got %v (%v)", act.Type, f.State())
	}

	// 4. Navigation inside StateHelp
	navTests := []struct {
		key      string
		expected ActionType
	}{
		{"down", ActionHelpNext},
		{"ctrl+n", ActionHelpNext},
		{"tab", ActionHelpNext},
		{"up", ActionHelpPrev},
		{"ctrl+p", ActionHelpPrev},
		{"shift+tab", ActionHelpPrev},
		{"btab", ActionHelpPrev},
		{"pgdown", ActionHelpPageDown},
		{"pagedown", ActionHelpPageDown},
		{"ctrl+d", ActionHelpPageDown},
		{"pgup", ActionHelpPageUp},
		{"pageup", ActionHelpPageUp},
		{"ctrl+u", ActionHelpPageUp},
		{"backspace", ActionHelpBackspace},
	}

	for _, tt := range navTests {
		act = f.HandleKey(tt.key)
		if act.Type != tt.expected {
			t.Errorf("key %q: expected action %v, got %v", tt.key, tt.expected, act.Type)
		}
		if f.State() != StateHelp {
			t.Errorf("key %q unexpectedly left StateHelp (state: %v)", tt.key, f.State())
		}
	}

	// 5. Input text inside StateHelp
	act = f.HandleKey("l")
	if act.Type != ActionHelpInput || act.String != "l" {
		t.Fatalf("expected ActionHelpInput with 'l', got %v (%q)", act.Type, act.String)
	}
	act = f.HandleKey(" ")
	if act.Type != ActionHelpInput || act.String != " " {
		t.Fatalf("expected ActionHelpInput with ' ', got %v (%q)", act.Type, act.String)
	}

	// 6. Enter confirms/selects and returns to StateNormal
	act = f.HandleKey("enter")
	if act.Type != ActionHelpSelect || f.State() != StateNormal {
		t.Fatalf("expected ActionHelpSelect and StateNormal for 'enter', got %v (%v)", act.Type, f.State())
	}
}

func TestFSMAutocompleteKeybindings(t *testing.T) {
	f := NewFSM()
	f.SetState(StateInsert)

	// Test ctrl+space
	act := f.HandleKey("ctrl+space")
	if act.Type != ActionLSPComplete {
		t.Fatalf("expected ActionLSPComplete for ctrl+space, got %v", act.Type)
	}

	// Test ctrl+@ (terminal representation of ctrl+space)
	act = f.HandleKey("ctrl+@")
	if act.Type != ActionLSPComplete {
		t.Fatalf("expected ActionLSPComplete for ctrl+@, got %v", act.Type)
	}

	// In StateCompletion, ctrl+space and ctrl+@ should be ignored (ActionNone)
	f.SetState(StateCompletion)
	act = f.HandleKey("ctrl+space")
	if act.Type != ActionNone {
		t.Fatalf("expected ActionNone for ctrl+space in StateCompletion, got %v", act.Type)
	}
	act = f.HandleKey("ctrl+@")
	if act.Type != ActionNone {
		t.Fatalf("expected ActionNone for ctrl+@ in StateCompletion, got %v", act.Type)
	}
}

func TestFSMDynamicKeyRemaps(t *testing.T) {
	f := NewFSM()

	remaps := map[string]map[string]string{
		"normal": {
			"F2":     "save",
			"ctrl+z": "undo",
		},
		"insert": {
			"ctrl+s": "save",
		},
		"select": {
			"c": "change_selection",
		},
	}

	f.SetKeyRemaps(remaps)

	// 1. Normal mode remaps
	f.SetState(StateNormal)
	act := f.HandleKey("F2")
	if act.Type != ActionSave {
		t.Fatalf("expected ActionSave for F2, got %v", act.Type)
	}

	act = f.HandleKey("ctrl+z")
	if act.Type != ActionUndo {
		t.Fatalf("expected ActionUndo for ctrl+z, got %v", act.Type)
	}

	// Unmapped key still works normally
	act = f.HandleKey("h")
	if act.Type != ActionMoveLeft {
		t.Fatalf("expected ActionMoveLeft for h, got %v", act.Type)
	}

	// 2. Insert mode remap: ctrl+s triggers ActionSave without inserting text
	f.SetState(StateInsert)
	act = f.HandleKey("ctrl+s")
	if act.Type != ActionSave {
		t.Fatalf("expected ActionSave for ctrl+s in Insert mode, got %v", act.Type)
	}

	// 3. Select mode remap: c triggers ActionChangeSelection and transitions to StateInsert
	f.SetState(StateSelect)
	act = f.HandleKey("c")
	if act.Type != ActionChangeSelection || f.State() != StateInsert {
		t.Fatalf("expected ActionChangeSelection and StateInsert for c, got %v (%v)", act.Type, f.State())
	}
}



