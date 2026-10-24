package fsm

import "strings"

// State represents the modal interaction state of the editor.
type State uint8

const (
	StateNormal State = iota
	StateInsert
	StateSelect
	StateLeader
	StateMatchPending
	StatePicker
	StateDrawer
	StateDiffReview
	StateTree
	StateHover
	StateCompletion
	StateCodeAction
	StateRename
	StateSearch
	StateHelp
)

func (s State) String() string {
	switch s {
	case StateNormal:
		return "NORMAL"
	case StateInsert:
		return "INSERT"
	case StateSelect:
		return "SELECT"
	case StateLeader:
		return "LEADER"
	case StateMatchPending:
		return "MATCH"
	case StatePicker:
		return "PICKER"
	case StateDrawer:
		return "DRAWER"
	case StateDiffReview:
		return "DIFF_REVIEW"
	case StateTree:
		return "TREE"
	case StateHover:
		return "HOVER"
	case StateCompletion:
		return "COMPLETION"
	case StateCodeAction:
		return "ACTION"
	case StateRename:
		return "RENAME"
	case StateSearch:
		return "SEARCH"
	case StateHelp:
		return "HELP"
	default:
		return "UNKNOWN"
	}
}

// ActionType represents an action triggered by an input event.
type ActionType uint16

const (
	ActionNone ActionType = iota

	// Cursor & Navigation
	ActionMoveLeft
	ActionMoveRight
	ActionMoveUp
	ActionMoveDown
	ActionMoveWordForward
	ActionMoveWordBackward
	ActionPageUp
	ActionPageDown
	ActionMoveLineStart
	ActionMoveLineEnd
	ActionMoveBufferStart
	ActionMoveBufferEnd

	// Selection
	ActionSelectWord
	ActionSelectWordBackward
	ActionSelectLine
	ActionExtendSelection
	ActionCollapseSelection
	ActionSelectTextObject // e.g. m i b, m a f

	// Mutations
	ActionInsertChar
	ActionInsertNewline
	ActionInsertTab
	ActionBackspace
	ActionDeleteChar
	ActionDeleteSelection
	ActionChangeSelection // Delete selection and switch to insert mode
	ActionIndent
	ActionOutdent
	ActionYank
	ActionPasteAfter
	ActionPasteBefore
	ActionUndo
	ActionRedo
	ActionPipeSelection // '!' in select mode

	// Buffer & Tabs
	ActionTabNext
	ActionTabPrev
	ActionTabSwitch
	ActionBufferClose
	ActionBufferForceClose
	ActionSave
	ActionSaveAll

	// Drawers & Panes
	ActionToggleFileTree
	ActionToggleDrawer
	ActionToggleMarkdownPreview
	ActionDrawerClose
	ActionDrawerJump
	ActionDrawerMaximize

	// Quickfix & Diagnostics (Nordic Space n/p chords)
	ActionNextError
	ActionPrevError
	ActionNextDiagnostic
	ActionPrevDiagnostic
	ActionNextHunk
	ActionPrevHunk
	ActionStepForward  // 'ä'
	ActionStepBackward // 'ö'

	// Agent & Diff
	ActionAgentReviewOpen
	ActionAgentAcceptAll
	ActionAgentRevertAll

	// Tooling & LSP
	ActionAlternateFile // C++ .cpp <-> .hpp
	ActionLSPDefinition
	ActionLSPImplementation
	ActionLSPCallHierarchy
	ActionLSPCodeAction
	ActionLSPRename
	ActionLSPHover
	ActionLSPComplete
	ActionLSPCancel
	ActionLSPSelectNext
	ActionLSPSelectPrev
	ActionLSPSelectConfirm

	// History & Jumplist
	ActionJumplistBack
	ActionJumplistForward

	// Markdown specific
	ActionToggleChecklist
	ActionToggleMermaidMode
	ActionToggleMarkdownFold

	// Picker / Fuzzy Finder
	ActionPickerOpenFiles
	ActionPickerOpenGrep
	ActionPickerOpenBuffers
	ActionPickerSelect
	ActionPickerCancel
	ActionPickerNext
	ActionPickerPrev
	ActionPickerInput
	ActionPickerBackspace

	// Pane resize
	ActionResizeTreeGrow
	ActionResizeTreeShrink
	ActionResizeDrawerGrow
	ActionResizeDrawerShrink

	// FileTree navigation & actions
	ActionTreeUp
	ActionTreeDown
	ActionTreeToggleOrOpen
	ActionTreeCollapseOrParent
	ActionTreeTop
	ActionTreeBottom
	ActionTreePageUp
	ActionTreePageDown
	ActionTreeRefresh
	ActionTreeCreateFile
	ActionTreeCreateDir
	ActionTreeDelete

	// Pane focus & window cycling
	ActionPaneFocusTree
	ActionPaneFocusBuffer
	ActionPaneFocusDrawer
	ActionPaneCycle

	// Search & Replace
	ActionSearchOpen
	ActionSearchInput
	ActionSearchBackspace
	ActionSearchConfirm
	ActionSearchCancel
	ActionSearchNext
	ActionSearchPrev
	ActionSearchClearHighlight
	ActionReplaceOpen
	ActionProjectReplaceOpen
	ActionReplaceApplyAll
	ActionReplaceApplyCurrent
	ActionReplaceToggleCase
	ActionReplaceToggleWord
	ActionReplaceToggleRegex
	ActionReplaceNextField
	ActionReplacePrevField

	// DAP Debugger
	ActionDAPToggleBreakpoint
	ActionDAPStart
	ActionDAPContinue
	ActionDAPStepOver
	ActionDAPStepInto
	ActionDAPStepOut
	ActionDAPPause
	ActionDAPStop
	ActionDAPToggleView

	// Help View
	ActionHelpToggle
	ActionHelpOpen
	ActionHelpCancel
	ActionHelpNext
	ActionHelpPrev
	ActionHelpSelect
	ActionHelpInput
	ActionHelpBackspace
	ActionHelpPageUp
	ActionHelpPageDown
)

// Action encapsulates the action to perform along with any argument payload.
type Action struct {
	Type   ActionType
	String string
	Int    int
	Arg1   string
	Arg2   string
}

// ParseActionType parses a human-readable action name string into an ActionType.
func ParseActionType(name string) ActionType {
	cleaned := strings.ToLower(strings.TrimSpace(name))
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	switch cleaned {
	// Navigation
	case "move_left", "left":
		return ActionMoveLeft
	case "move_right", "right":
		return ActionMoveRight
	case "move_up", "up":
		return ActionMoveUp
	case "move_down", "down":
		return ActionMoveDown
	case "move_word_forward", "word_forward":
		return ActionMoveWordForward
	case "move_word_backward", "word_backward":
		return ActionMoveWordBackward
	case "page_up", "pgup":
		return ActionPageUp
	case "page_down", "pgdn", "pgdown":
		return ActionPageDown
	case "move_line_start", "line_start", "home":
		return ActionMoveLineStart
	case "move_line_end", "line_end", "end":
		return ActionMoveLineEnd
	case "move_buffer_start", "buffer_start":
		return ActionMoveBufferStart
	case "move_buffer_end", "buffer_end":
		return ActionMoveBufferEnd

	// Selection
	case "select_word":
		return ActionSelectWord
	case "select_word_backward":
		return ActionSelectWordBackward
	case "select_line":
		return ActionSelectLine
	case "extend_selection":
		return ActionExtendSelection
	case "collapse_selection":
		return ActionCollapseSelection
	case "select_text_object":
		return ActionSelectTextObject

	// Mutations
	case "insert_char":
		return ActionInsertChar
	case "insert_newline", "newline":
		return ActionInsertNewline
	case "insert_tab", "tab":
		return ActionInsertTab
	case "backspace":
		return ActionBackspace
	case "delete_char", "delete":
		return ActionDeleteChar
	case "delete_selection":
		return ActionDeleteSelection
	case "change_selection":
		return ActionChangeSelection
	case "indent":
		return ActionIndent
	case "outdent":
		return ActionOutdent
	case "yank", "copy":
		return ActionYank
	case "paste_after", "paste":
		return ActionPasteAfter
	case "paste_before":
		return ActionPasteBefore
	case "undo":
		return ActionUndo
	case "redo":
		return ActionRedo
	case "pipe_selection":
		return ActionPipeSelection

	// Buffers & Tabs
	case "tab_next", "next_tab", "buffer_next":
		return ActionTabNext
	case "tab_prev", "prev_tab", "buffer_prev":
		return ActionTabPrev
	case "tab_switch":
		return ActionTabSwitch
	case "buffer_close", "close_buffer":
		return ActionBufferClose
	case "buffer_force_close", "force_close_buffer":
		return ActionBufferForceClose
	case "save":
		return ActionSave
	case "save_all":
		return ActionSaveAll

	// Drawers & Panes
	case "toggle_file_tree", "toggle_tree":
		return ActionToggleFileTree
	case "toggle_drawer":
		return ActionToggleDrawer
	case "toggle_markdown_preview":
		return ActionToggleMarkdownPreview
	case "drawer_close":
		return ActionDrawerClose
	case "drawer_jump":
		return ActionDrawerJump
	case "drawer_maximize":
		return ActionDrawerMaximize

	// Quickfix & Stepping
	case "next_error":
		return ActionNextError
	case "prev_error":
		return ActionPrevError
	case "next_diagnostic":
		return ActionNextDiagnostic
	case "prev_diagnostic":
		return ActionPrevDiagnostic
	case "next_hunk":
		return ActionNextHunk
	case "prev_hunk":
		return ActionPrevHunk
	case "step_forward":
		return ActionStepForward
	case "step_backward":
		return ActionStepBackward

	// Agent & Diff
	case "agent_review_open", "agent_review":
		return ActionAgentReviewOpen
	case "agent_accept_all":
		return ActionAgentAcceptAll
	case "agent_revert_all":
		return ActionAgentRevertAll

	// Tooling & LSP
	case "alternate_file":
		return ActionAlternateFile
	case "lsp_definition", "definition":
		return ActionLSPDefinition
	case "lsp_implementation", "implementation":
		return ActionLSPImplementation
	case "lsp_call_hierarchy", "call_hierarchy":
		return ActionLSPCallHierarchy
	case "lsp_code_action", "code_action":
		return ActionLSPCodeAction
	case "lsp_rename", "rename":
		return ActionLSPRename
	case "lsp_hover", "hover":
		return ActionLSPHover
	case "lsp_complete", "completion":
		return ActionLSPComplete
	case "lsp_cancel":
		return ActionLSPCancel

	// Jumplist
	case "jumplist_back":
		return ActionJumplistBack
	case "jumplist_forward":
		return ActionJumplistForward

	// Markdown
	case "toggle_checklist":
		return ActionToggleChecklist
	case "toggle_mermaid_mode":
		return ActionToggleMermaidMode
	case "toggle_markdown_fold":
		return ActionToggleMarkdownFold

	// Picker
	case "picker_open_files", "open_files":
		return ActionPickerOpenFiles
	case "picker_open_grep", "live_grep":
		return ActionPickerOpenGrep
	case "picker_open_buffers", "open_buffers":
		return ActionPickerOpenBuffers
	case "picker_select":
		return ActionPickerSelect
	case "picker_cancel":
		return ActionPickerCancel
	case "picker_next":
		return ActionPickerNext
	case "picker_prev":
		return ActionPickerPrev

	// Panes
	case "pane_focus_tree":
		return ActionPaneFocusTree
	case "pane_focus_buffer":
		return ActionPaneFocusBuffer
	case "pane_focus_drawer":
		return ActionPaneFocusDrawer
	case "pane_cycle":
		return ActionPaneCycle
	case "resize_tree_grow":
		return ActionResizeTreeGrow
	case "resize_tree_shrink":
		return ActionResizeTreeShrink
	case "resize_drawer_grow":
		return ActionResizeDrawerGrow
	case "resize_drawer_shrink":
		return ActionResizeDrawerShrink

	// FileTree
	case "tree_up":
		return ActionTreeUp
	case "tree_down":
		return ActionTreeDown
	case "tree_toggle_or_open":
		return ActionTreeToggleOrOpen
	case "tree_collapse_or_parent":
		return ActionTreeCollapseOrParent
	case "tree_refresh":
		return ActionTreeRefresh
	case "tree_create_file":
		return ActionTreeCreateFile
	case "tree_create_dir":
		return ActionTreeCreateDir
	case "tree_delete":
		return ActionTreeDelete

	// Search & Replace
	case "search_open", "search":
		return ActionSearchOpen
	case "search_cancel":
		return ActionSearchCancel
	case "search_next":
		return ActionSearchNext
	case "search_prev":
		return ActionSearchPrev
	case "search_clear_highlight":
		return ActionSearchClearHighlight
	case "replace_open", "replace":
		return ActionReplaceOpen
	case "project_replace_open":
		return ActionProjectReplaceOpen

	// DAP
	case "dap_toggle_breakpoint", "toggle_breakpoint":
		return ActionDAPToggleBreakpoint
	case "dap_start":
		return ActionDAPStart
	case "dap_continue", "debug_continue":
		return ActionDAPContinue
	case "dap_step_over", "debug_step_over":
		return ActionDAPStepOver
	case "dap_step_into", "debug_step_into":
		return ActionDAPStepInto
	case "dap_step_out", "debug_step_out":
		return ActionDAPStepOut
	case "dap_pause", "debug_pause":
		return ActionDAPPause
	case "dap_stop", "debug_stop":
		return ActionDAPStop
	case "dap_toggle_view", "debug_toggle_view":
		return ActionDAPToggleView

	// Help
	case "help_toggle", "help":
		return ActionHelpToggle
	case "help_open":
		return ActionHelpOpen
	case "help_cancel":
		return ActionHelpCancel

	default:
		return ActionNone
	}
}

