package fsm

import (
	"strings"
	"time"
)

// FSM represents the Finite State Machine controlling editor interaction.
type FSM struct {
	state          State
	previousState  State
	leaderBuffer   string
	matchBuffer    string
	lastInputTime  time.Time
	lastChar       string
	leaderTimeout  time.Duration
	drawerEditable bool
	treeEditable   bool
	remaps         map[State]map[string]ActionType
}

// NewFSM creates an FSM initialized in StateNormal.
func NewFSM() *FSM {
	return &FSM{
		state:         StateNormal,
		previousState: StateNormal,
		leaderTimeout: 2 * time.Second,
	}
}

// State returns the current interaction state.
func (f *FSM) State() State {
	return f.state
}

// DrawerEditable returns whether the drawer currently receives text input.
func (f *FSM) DrawerEditable() bool {
	return f.drawerEditable
}

// SetDrawerEditable sets whether the drawer receives text input.
func (f *FSM) SetDrawerEditable(editable bool) {
	f.drawerEditable = editable
}

// TreeEditable returns whether the file tree currently receives text input (e.g., new file/folder prompt).
func (f *FSM) TreeEditable() bool {
	return f.treeEditable
}

// SetTreeEditable sets whether the file tree receives text input.
func (f *FSM) SetTreeEditable(editable bool) {
	f.treeEditable = editable
}

// SetState forces a specific state (used by external overlays or modals).
func (f *FSM) SetState(s State) {
	f.previousState = f.state
	f.state = s
	if s != StateDrawer {
		f.drawerEditable = false
	}
	if s != StateTree {
		f.treeEditable = false
	}
	f.clearBuffers()
}

// SetKeyRemaps configures dynamic key remaps from TOML config.
// The map structure is: mode (e.g. "normal", "insert", "select") -> key -> action name.
func (f *FSM) SetKeyRemaps(rawRemaps map[string]map[string]string) {
	if rawRemaps == nil {
		f.remaps = nil
		return
	}
	remaps := make(map[State]map[string]ActionType)
	for modeStr, keyMap := range rawRemaps {
		var st State
		switch strings.ToLower(strings.TrimSpace(modeStr)) {
		case "normal":
			st = StateNormal
		case "insert":
			st = StateInsert
		case "select":
			st = StateSelect
		case "tree":
			st = StateTree
		case "drawer":
			st = StateDrawer
		case "picker":
			st = StatePicker
		case "diff_review":
			st = StateDiffReview
		case "search":
			st = StateSearch
		case "help":
			st = StateHelp
		default:
			continue
		}
		if remaps[st] == nil {
			remaps[st] = make(map[string]ActionType)
		}
		for k, actName := range keyMap {
			actType := ParseActionType(actName)
			if actType != ActionNone {
				remaps[st][k] = actType
			}
		}
	}
	f.remaps = remaps
}

func (f *FSM) lookupRemap(st State, key string) (ActionType, bool) {
	if f.remaps == nil {
		return ActionNone, false
	}
	m := f.remaps[st]
	if m == nil {
		return ActionNone, false
	}
	if act, ok := m[key]; ok {
		return act, true
	}
	if act, ok := m[strings.ToLower(key)]; ok {
		return act, true
	}
	return ActionNone, false
}

// HandleKey processes a single key input string and returns the corresponding Action.
func (f *FSM) HandleKey(key string) Action {
	now := time.Now()

	// Check leader timeout
	if f.state == StateLeader && now.Sub(f.lastInputTime) > f.leaderTimeout {
		f.state = StateNormal
		f.clearBuffers()
	}

	defer func() {
		f.lastInputTime = now
		f.lastChar = key
	}()

	// Check custom dynamic remaps before default dispatch (unless in an active text prompt)
	inEditablePrompt := (f.state == StateDrawer && f.drawerEditable) || (f.state == StateTree && f.treeEditable)
	if !inEditablePrompt {
		if actType, ok := f.lookupRemap(f.state, key); ok {
			switch actType {
			case ActionSelectWord, ActionSelectWordBackward, ActionSelectLine, ActionExtendSelection:
				f.state = StateSelect
			case ActionInsertNewline:
				if f.state == StateNormal {
					f.state = StateInsert
				}
			case ActionChangeSelection:
				f.state = StateInsert
			case ActionSearchOpen:
				f.state = StateSearch
			case ActionDrawerClose, ActionPickerCancel, ActionHelpCancel, ActionSearchCancel:
				f.state = StateNormal
			}
			return Action{Type: actType}
		}
	}

	switch f.state {
	case StateNormal:
		return f.handleNormal(key)
	case StateInsert:
		return f.handleInsert(key, now)
	case StateSelect:
		return f.handleSelect(key)
	case StateLeader:
		return f.handleLeader(key)
	case StateMatchPending:
		return f.handleMatch(key)
	case StateDrawer:
		return f.handleDrawer(key)
	case StatePicker:
		return f.handlePicker(key)
	case StateDiffReview:
		return f.handleDiffReview(key)
	case StateTree:
		return f.handleTree(key)
	case StateHover:
		return f.handleHover(key)
	case StateCompletion:
		return f.handleCompletion(key)
	case StateCodeAction:
		return f.handleCodeAction(key)
	case StateRename:
		return f.handleRename(key)
	case StateSearch:
		return f.handleSearch(key)
	case StateHelp:
		return f.handleHelp(key)
	default:
		f.state = StateNormal
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleNormal(key string) Action {
	switch key {
	// Mode transitions
	case "i":
		f.state = StateInsert
		return Action{Type: ActionNone}
	case "a":
		f.state = StateInsert
		return Action{Type: ActionMoveRight}
	case "o":
		f.state = StateInsert
		return Action{Type: ActionInsertNewline}
	case "w":
		f.state = StateSelect
		return Action{Type: ActionSelectWord}
	case "b":
		f.state = StateSelect
		return Action{Type: ActionSelectWordBackward}
	case "x":
		f.state = StateSelect
		return Action{Type: ActionSelectLine}
	case "v":
		f.state = StateSelect
		return Action{Type: ActionNone}
	case "/":
		f.state = StateSearch
		return Action{Type: ActionSearchOpen}
	case "n":
		return Action{Type: ActionSearchNext}
	case "N":
		return Action{Type: ActionSearchPrev}
	case "Space", " ":
		f.state = StateLeader
		f.leaderBuffer = ""
		return Action{Type: ActionNone}
	case "m":
		f.state = StateMatchPending
		f.matchBuffer = ""
		return Action{Type: ActionNone}

	// Basic Navigation
	case "h", "left":
		return Action{Type: ActionMoveLeft}
	case "l", "right":
		return Action{Type: ActionMoveRight}
	case "j", "down":
		return Action{Type: ActionMoveDown}
	case "k", "up":
		return Action{Type: ActionMoveUp}
	case "pgup", "pageup", "ctrl+b", "ctrl+u":
		return Action{Type: ActionPageUp}
	case "pgdown", "pagedown", "ctrl+f", "ctrl+d":
		return Action{Type: ActionPageDown}
	case "home", "0":
		return Action{Type: ActionMoveLineStart}
	case "end", "$":
		return Action{Type: ActionMoveLineEnd}
	case "ctrl+home":
		return Action{Type: ActionMoveBufferStart}
	case "ctrl+end", "G":
		return Action{Type: ActionMoveBufferEnd}
	case "ctrl+right", "alt+right":
		return Action{Type: ActionMoveWordForward}
	case "ctrl+left", "alt+left":
		return Action{Type: ActionMoveWordBackward}

	// Mutations
	case "d":
		return Action{Type: ActionDeleteChar}
	case "y":
		return Action{Type: ActionYank}
	case "p":
		return Action{Type: ActionPasteAfter}
	case "P":
		return Action{Type: ActionPasteBefore}
	case "u":
		return Action{Type: ActionUndo}
	case "U":
		return Action{Type: ActionRedo}

	// Buffer Tabs
	case "H":
		return Action{Type: ActionTabPrev}
	case "L":
		return Action{Type: ActionTabNext}

	// Nordic Quick Step Keys (ö / ä)
	case "ö":
		return Action{Type: ActionStepBackward}
	case "ä":
		return Action{Type: ActionStepForward}

	// History / Jumplist
	case "ctrl+o":
		return Action{Type: ActionJumplistBack}
	case "ctrl+i":
		return Action{Type: ActionJumplistForward}

	// Window & Pane Cycling
	case "ctrl+w":
		return Action{Type: ActionPaneCycle}

	// LSP Hover
	case "K":
		return Action{Type: ActionLSPHover}

	// File Save
	case "ctrl+s":
		return Action{Type: ActionSave}

	// DAP Debugger Function Keys
	case "f5":
		return Action{Type: ActionDAPContinue}
	case "f9":
		return Action{Type: ActionDAPToggleBreakpoint}
	case "f10":
		return Action{Type: ActionDAPStepOver}
	case "f11":
		return Action{Type: ActionDAPStepInto}
	case "shift+f11", "f12":
		return Action{Type: ActionDAPStepOut}
	case "shift+f5":
		return Action{Type: ActionDAPStop}

	// Help View
	case "?", "f1":
		f.state = StateHelp
		return Action{Type: ActionHelpToggle}

	default:
		// Alt+1 .. Alt+9 direct tab switching
		if strings.HasPrefix(key, "alt+") && len(key) == 5 {
			ch := key[4]
			if ch >= '1' && ch <= '9' {
				return Action{Type: ActionTabSwitch, Int: int(ch - '0')}
			}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleInsert(key string, now time.Time) Action {
	// Fast home-row exit: "j" followed quickly by "k" (<200ms)
	if key == "k" && f.lastChar == "j" && now.Sub(f.lastInputTime) < 200*time.Millisecond {
		f.state = StateNormal
		return Action{Type: ActionBackspace} // Erase the 'j' that was typed, then transition
	}

	switch key {
	case "esc":
		f.state = StateNormal
		return Action{Type: ActionMoveLeft}
	case "enter":
		return Action{Type: ActionInsertNewline}
	case "tab":
		return Action{Type: ActionInsertTab}
	case "ctrl+space", "ctrl+@":
		return Action{Type: ActionLSPComplete}
	case "ctrl+s":
		return Action{Type: ActionSave}
	case "backspace":
		return Action{Type: ActionBackspace}
	case "delete":
		return Action{Type: ActionDeleteChar}
	case "left":
		return Action{Type: ActionMoveLeft}
	case "right":
		return Action{Type: ActionMoveRight}
	case "up":
		return Action{Type: ActionMoveUp}
	case "down":
		return Action{Type: ActionMoveDown}
	case "pgup", "pageup":
		return Action{Type: ActionPageUp}
	case "pgdown", "pagedown":
		return Action{Type: ActionPageDown}
	case "home":
		return Action{Type: ActionMoveLineStart}
	case "end":
		return Action{Type: ActionMoveLineEnd}
	case "ctrl+home":
		return Action{Type: ActionMoveBufferStart}
	case "ctrl+end":
		return Action{Type: ActionMoveBufferEnd}
	case "ctrl+right", "alt+right":
		return Action{Type: ActionMoveWordForward}
	case "ctrl+left", "alt+left":
		return Action{Type: ActionMoveWordBackward}
	case "Space", " ":
		return Action{Type: ActionInsertChar, String: " "}
	default:
		// Printable character insertion
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionInsertChar, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleSelect(key string) Action {
	switch key {
	case "esc":
		f.state = StateNormal
		return Action{Type: ActionCollapseSelection}
	case "d":
		f.state = StateNormal
		return Action{Type: ActionDeleteSelection}
	case "c":
		f.state = StateInsert
		return Action{Type: ActionChangeSelection}
	case "y":
		f.state = StateNormal
		return Action{Type: ActionYank}
	case ">":
		return Action{Type: ActionIndent}
	case "<":
		return Action{Type: ActionOutdent}
	case "!":
		f.state = StateNormal
		return Action{Type: ActionPipeSelection}
	case "h", "left":
		return Action{Type: ActionExtendSelection, String: "left"}
	case "l", "right":
		return Action{Type: ActionExtendSelection, String: "right"}
	case "j", "down":
		return Action{Type: ActionExtendSelection, String: "down"}
	case "k", "up":
		return Action{Type: ActionExtendSelection, String: "up"}
	case "pgup", "pageup":
		return Action{Type: ActionPageUp}
	case "pgdown", "pagedown":
		return Action{Type: ActionPageDown}
	case "home":
		return Action{Type: ActionMoveLineStart}
	case "end":
		return Action{Type: ActionMoveLineEnd}
	case "ctrl+home":
		return Action{Type: ActionMoveBufferStart}
	case "ctrl+end":
		return Action{Type: ActionMoveBufferEnd}
	case "w":
		return Action{Type: ActionSelectWord}
	case "b":
		return Action{Type: ActionSelectWordBackward}
	case "x":
		return Action{Type: ActionSelectLine}
	default:
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleMatch(key string) Action {
	f.matchBuffer += key

	if len(f.matchBuffer) == 1 {
		// First key must be 'i' (inside) or 'a' (around)
		if key != "i" && key != "a" {
			f.state = StateNormal
			f.matchBuffer = ""
			return Action{Type: ActionNone}
		}
		return Action{Type: ActionNone}
	}

	if len(f.matchBuffer) == 2 {
		kind := string(f.matchBuffer[0])   // "i" or "a"
		target := string(f.matchBuffer[1]) // "b", "a", "p", "q", "f", "c", "s"
		f.state = StateSelect
		f.matchBuffer = ""

		return Action{
			Type: ActionSelectTextObject,
			Arg1: kind,
			Arg2: target,
		}
	}

	f.state = StateNormal
	f.matchBuffer = ""
	return Action{Type: ActionNone}
}

func (f *FSM) handleLeader(key string) Action {
	if key == "esc" {
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionNone}
	}

	f.leaderBuffer += key

	switch f.leaderBuffer {
	// Fuzzy finders
	case "ff":
		f.state = StatePicker
		f.leaderBuffer = ""
		return Action{Type: ActionPickerOpenFiles}
	case "fg":
		f.state = StatePicker
		f.leaderBuffer = ""
		return Action{Type: ActionPickerOpenGrep}
	case "bb":
		f.state = StatePicker
		f.leaderBuffer = ""
		return Action{Type: ActionPickerOpenBuffers}
	case "bd":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionBufferClose}
	case "bD":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionBufferForceClose}
	case "fs":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionSave}
	case "fa":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionSaveAll}

	// Layout & Drawers
	case "e":
		if f.state == StateTree {
			f.state = StateNormal
		} else {
			f.state = StateTree
		}
		f.leaderBuffer = ""
		return Action{Type: ActionToggleFileTree}
	case "o":
		f.state = StateDrawer
		f.leaderBuffer = ""
		return Action{Type: ActionToggleDrawer}
	case "?", "h":
		f.state = StateHelp
		f.leaderBuffer = ""
		return Action{Type: ActionHelpToggle}
	case "mp":
		f.state = StateDrawer
		f.leaderBuffer = ""
		return Action{Type: ActionToggleMarkdownPreview}
	case "mt":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionToggleChecklist}
	case "md":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionToggleMermaidMode}
	case "mf":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionToggleMarkdownFold}

	// Tooling
	case "a":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionAlternateFile}

	// Diagnostics & Errors
	case "ne":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionNextError}
	case "pe":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPrevError}
	case "nd":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionNextDiagnostic}
	case "pd":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPrevDiagnostic}
	case "nh":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionNextHunk}
	case "ph":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPrevHunk}

	// Agent & Review
	case "ds":
		f.state = StateDiffReview
		f.leaderBuffer = ""
		return Action{Type: ActionAgentReviewOpen}
	case "da":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionAgentAcceptAll}
	case "dr":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionAgentRevertAll}

	// Debugger (DAP)
	case "db":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPToggleBreakpoint}
	case "dd":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPStart}
	case "dc":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPContinue}
	case "dn":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPStepOver}
	case "di":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPStepInto}
	case "do":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPStepOut}
	case "dp":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPPause}
	case "dq":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionDAPStop}
	case "dv":
		f.state = StateDrawer
		f.leaderBuffer = ""
		return Action{Type: ActionDAPToggleView}

	// LSP
	case "ca":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPCodeAction}
	case "cr":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPRename}
	case "gd":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPDefinition}
	case "gi":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPImplementation}
	case "gc":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPCallHierarchy}
	case "k":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionLSPHover}

	// Resizing
	case "w>":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionResizeTreeGrow}
	case "w<":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionResizeTreeShrink}
	case "w+":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionResizeDrawerGrow}
	case "w-":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionResizeDrawerShrink}

	// Window & Pane Navigation
	case "ww":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPaneCycle}
	case "wh":
		f.state = StateTree
		f.leaderBuffer = ""
		return Action{Type: ActionPaneFocusTree}
	case "wl":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPaneFocusBuffer}
	case "wj":
		f.state = StateDrawer
		f.leaderBuffer = ""
		return Action{Type: ActionPaneFocusDrawer}
	case "wk":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionPaneFocusBuffer}

	// Search & Replace
	case "sr":
		f.state = StateDrawer
		f.drawerEditable = true
		f.leaderBuffer = ""
		return Action{Type: ActionReplaceOpen}
	case "sp":
		f.state = StateDrawer
		f.drawerEditable = true
		f.leaderBuffer = ""
		return Action{Type: ActionProjectReplaceOpen}
	case "sf", "s/":
		f.state = StateSearch
		f.leaderBuffer = ""
		return Action{Type: ActionSearchOpen}
	case "ch":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionSearchClearHighlight}
	case "ns":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionSearchNext}
	case "ps":
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionSearchPrev}

	default:
		// Prefix matching: If the buffer is a prefix of any valid chord, keep waiting
		if f.isLeaderPrefix(f.leaderBuffer) {
			return Action{Type: ActionNone}
		}
		// Invalid chord, reset
		f.state = StateNormal
		f.leaderBuffer = ""
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleDrawer(key string) Action {
	if f.drawerEditable {
		switch key {
		case "esc":
			f.state = StateNormal
			f.drawerEditable = false
			return Action{Type: ActionDrawerClose}
		case "enter":
			return Action{Type: ActionReplaceApplyAll}
		case "tab":
			return Action{Type: ActionReplaceNextField}
		case "shift+tab", "btab":
			return Action{Type: ActionReplacePrevField}
		case "alt+c":
			return Action{Type: ActionReplaceToggleCase}
		case "alt+w":
			return Action{Type: ActionReplaceToggleWord}
		case "alt+r", "ctrl+r":
			return Action{Type: ActionReplaceToggleRegex}
		case "alt+a", "ctrl+a":
			return Action{Type: ActionReplaceApplyAll}
		case "alt+y", "ctrl+y":
			return Action{Type: ActionReplaceApplyCurrent}
		case "up":
			return Action{Type: ActionReplacePrevField}
		case "down":
			return Action{Type: ActionReplaceNextField}
		case "pgup", "pageup":
			return Action{Type: ActionPageUp}
		case "pgdown", "pagedown":
			return Action{Type: ActionPageDown}
		case "backspace":
			return Action{Type: ActionBackspace}
		case "Space", " ":
			return Action{Type: ActionInsertChar, String: " "}
		default:
			if len([]rune(key)) == 1 && isPrintableRune(key) {
				return Action{Type: ActionInsertChar, String: key}
			}
			return Action{Type: ActionNone}
		}
	}

	// Read-only navigation mode (Output / Diagnostics)
	switch key {
	case "esc", "q":
		f.state = StateNormal
		return Action{Type: ActionDrawerClose}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionDrawerJump}
	case "Z":
		return Action{Type: ActionDrawerMaximize}
	case "j", "down":
		return Action{Type: ActionMoveDown}
	case "k", "up":
		return Action{Type: ActionMoveUp}
	case "d", "ctrl+d", "pgdown", "pagedown":
		return Action{Type: ActionPageDown}
	case "u", "ctrl+u", "pgup", "pageup":
		return Action{Type: ActionPageUp}
	case "G", "ctrl+end":
		return Action{Type: ActionMoveBufferEnd}
	case "home", "ctrl+home":
		return Action{Type: ActionMoveBufferStart}
	case "f5":
		return Action{Type: ActionDAPContinue}
	case "f9":
		return Action{Type: ActionDAPToggleBreakpoint}
	case "f10":
		return Action{Type: ActionDAPStepOver}
	case "f11":
		return Action{Type: ActionDAPStepInto}
	case "shift+f11", "f12":
		return Action{Type: ActionDAPStepOut}
	case "shift+f5":
		return Action{Type: ActionDAPStop}
	case "ctrl+w":
		return Action{Type: ActionPaneCycle}
	case "Space", " ":
		f.state = StateLeader
		f.leaderBuffer = ""
		return Action{Type: ActionNone}
	default:
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleTree(key string) Action {
	if f.treeEditable {
		switch key {
		case "esc":
			f.treeEditable = false
			return Action{Type: ActionLSPCancel}
		case "enter":
			f.treeEditable = false
			return Action{Type: ActionLSPSelectConfirm}
		case "backspace":
			return Action{Type: ActionBackspace}
		case "Space", " ":
			return Action{Type: ActionInsertChar, String: " "}
		default:
			if len([]rune(key)) == 1 && isPrintableRune(key) {
				return Action{Type: ActionInsertChar, String: key}
			}
			return Action{Type: ActionNone}
		}
	}

	switch key {
	case "esc", "q":
		f.state = StateNormal
		return Action{Type: ActionPaneFocusBuffer}
	case "j", "down":
		return Action{Type: ActionTreeDown}
	case "k", "up":
		return Action{Type: ActionTreeUp}
	case "enter", "l", "right":
		return Action{Type: ActionTreeToggleOrOpen}
	case "h", "left":
		return Action{Type: ActionTreeCollapseOrParent}
	case "pgdown", "pagedown", "ctrl+d":
		return Action{Type: ActionTreePageDown}
	case "pgup", "pageup", "ctrl+u":
		return Action{Type: ActionTreePageUp}
	case "home":
		return Action{Type: ActionTreeTop}
	case "end", "G":
		return Action{Type: ActionTreeBottom}
	case "g":
		if f.lastChar == "g" {
			return Action{Type: ActionTreeTop}
		}
		return Action{Type: ActionNone}
	case "r":
		return Action{Type: ActionTreeRefresh}
	case "a":
		return Action{Type: ActionTreeCreateFile}
	case "A":
		return Action{Type: ActionTreeCreateDir}
	case "d":
		return Action{Type: ActionTreeDelete}
	case "ctrl+w":
		return Action{Type: ActionPaneCycle}
	case "Space", " ":
		f.state = StateLeader
		f.leaderBuffer = ""
		return Action{Type: ActionNone}
	default:
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handlePicker(key string) Action {
	switch key {
	case "esc":
		f.state = StateNormal
		return Action{Type: ActionPickerCancel}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionPickerSelect}
	case "down", "ctrl+n":
		return Action{Type: ActionPickerNext}
	case "up", "ctrl+p":
		return Action{Type: ActionPickerPrev}
	case "backspace":
		return Action{Type: ActionPickerBackspace}
	case "Space", " ":
		return Action{Type: ActionPickerInput, String: " "}
	default:
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionPickerInput, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleHelp(key string) Action {
	switch key {
	case "esc", "q", "?":
		f.state = StateNormal
		return Action{Type: ActionHelpCancel}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionHelpSelect}
	case "down", "ctrl+n", "tab":
		return Action{Type: ActionHelpNext}
	case "up", "ctrl+p", "shift+tab", "btab":
		return Action{Type: ActionHelpPrev}
	case "pgdown", "pagedown", "ctrl+d":
		return Action{Type: ActionHelpPageDown}
	case "pgup", "pageup", "ctrl+u":
		return Action{Type: ActionHelpPageUp}
	case "backspace":
		return Action{Type: ActionHelpBackspace}
	case "Space", " ":
		return Action{Type: ActionHelpInput, String: " "}
	default:
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionHelpInput, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleDiffReview(key string) Action {
	switch key {
	case "esc", "q":
		f.state = StateNormal
		return Action{Type: ActionNone}
	case "j", "down":
		return Action{Type: ActionMoveDown}
	case "k", "up":
		return Action{Type: ActionMoveUp}
	case "enter":
		return Action{Type: ActionDrawerJump}
	default:
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleHover(key string) Action {
	switch key {
	case "esc", "q":
		f.state = StateNormal
		return Action{Type: ActionLSPCancel}
	case "j", "down":
		return Action{Type: ActionMoveDown}
	case "k", "up":
		return Action{Type: ActionMoveUp}
	default:
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleCompletion(key string) Action {
	switch key {
	case "esc":
		f.state = StateInsert
		return Action{Type: ActionLSPCancel}
	case "enter", "tab":
		f.state = StateInsert
		return Action{Type: ActionLSPSelectConfirm}
	case "down", "ctrl+n":
		return Action{Type: ActionLSPSelectNext}
	case "up", "ctrl+p":
		return Action{Type: ActionLSPSelectPrev}
	case "backspace":
		return Action{Type: ActionBackspace}
	case "ctrl+space", "ctrl+@":
		return Action{Type: ActionNone}
	default:
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionInsertChar, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleCodeAction(key string) Action {
	switch key {
	case "esc", "q":
		f.state = StateNormal
		return Action{Type: ActionLSPCancel}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionLSPSelectConfirm}
	case "j", "down":
		return Action{Type: ActionLSPSelectNext}
	case "k", "up":
		return Action{Type: ActionLSPSelectPrev}
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			f.state = StateNormal
			return Action{Type: ActionLSPSelectConfirm, Int: int(key[0] - '1')}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleRename(key string) Action {
	switch key {
	case "esc":
		f.state = StateNormal
		return Action{Type: ActionLSPCancel}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionLSPSelectConfirm}
	case "backspace":
		return Action{Type: ActionBackspace}
	default:
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionInsertChar, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) handleSearch(key string) Action {
	switch key {
	case "esc":
		f.state = StateNormal
		return Action{Type: ActionSearchCancel}
	case "enter":
		f.state = StateNormal
		return Action{Type: ActionSearchConfirm}
	case "backspace":
		return Action{Type: ActionSearchBackspace}
	default:
		if len([]rune(key)) == 1 && isPrintableRune(key) {
			return Action{Type: ActionSearchInput, String: key}
		}
		return Action{Type: ActionNone}
	}
}

func (f *FSM) isLeaderPrefix(prefix string) bool {
	validChords := []string{
		"ff", "fg", "fs", "fa", "bb", "bd", "bD", "e", "o", "a", "k", "?", "h",
		"mp", "mt", "md", "mf",
		"ne", "pe", "nd", "pd", "nh", "ph", "ns", "ps",
		"ds", "da", "dr",
		"db", "dd", "dc", "dn", "di", "do", "dp", "dq", "dv",
		"ca", "cr", "gd", "gi", "gc", "ch",
		"sr", "sp", "sf", "s/",
		"w>", "w<", "w+", "w-",
		"ww", "wh", "wl", "wj", "wk",
	}
	for _, chord := range validChords {
		if strings.HasPrefix(chord, prefix) {
			return true
		}
	}
	return false
}

func (f *FSM) clearBuffers() {
	f.leaderBuffer = ""
	f.matchBuffer = ""
}

func isPrintableRune(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < 32 && r != '\t' && r != '\n' {
			return false
		}
	}
	return true
}
