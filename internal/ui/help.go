package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mikolas/koodieditori/internal/fsm"
)

// HelpItem represents a single command, keybinding, or interaction capability.
type HelpItem struct {
	Chord       string
	Title       string
	Category    string
	Description string
	Action      fsm.ActionType
}

// HelpModal manages the centered modal cheatsheet and command reference.
type HelpModal struct {
	Open          bool
	Query         string
	Items         []HelpItem
	Filtered      []HelpItem
	SelectedIndex int
	ScrollY       int
}

// NewHelpModal creates an initialized HelpModal with the default catalog.
func NewHelpModal() *HelpModal {
	items := makeDefaultHelpItems()
	return &HelpModal{
		Open:          false,
		Query:         "",
		Items:         items,
		Filtered:      items,
		SelectedIndex: 0,
		ScrollY:       0,
	}
}

// OpenModal opens the help modal and resets search state.
func (h *HelpModal) OpenModal() {
	h.Open = true
	h.Query = ""
	h.Filtered = h.Items
	h.SelectedIndex = 0
	h.ScrollY = 0
}

// Close closes the help modal.
func (h *HelpModal) Close() {
	h.Open = false
	h.Query = ""
	h.SelectedIndex = 0
	h.ScrollY = 0
}

// InsertChar appends character to search query and refilters.
func (h *HelpModal) InsertChar(ch string) {
	h.Query += ch
	h.Filter()
}

// DeleteChar deletes the last character in search query.
func (h *HelpModal) DeleteChar() {
	if len(h.Query) > 0 {
		runes := []rune(h.Query)
		h.Query = string(runes[:len(runes)-1])
		h.Filter()
	}
}

// Next moves selection down by 1.
func (h *HelpModal) Next() {
	if len(h.Filtered) == 0 {
		return
	}
	if h.SelectedIndex < len(h.Filtered)-1 {
		h.SelectedIndex++
	} else {
		h.SelectedIndex = 0
		h.ScrollY = 0
	}
}

// Prev moves selection up by 1.
func (h *HelpModal) Prev() {
	if len(h.Filtered) == 0 {
		return
	}
	if h.SelectedIndex > 0 {
		h.SelectedIndex--
	} else {
		h.SelectedIndex = len(h.Filtered) - 1
	}
}

// PageDown moves selection down by pageSize.
func (h *HelpModal) PageDown(pageSize int) {
	if len(h.Filtered) == 0 {
		return
	}
	h.SelectedIndex += pageSize
	if h.SelectedIndex >= len(h.Filtered) {
		h.SelectedIndex = len(h.Filtered) - 1
	}
}

// PageUp moves selection up by pageSize.
func (h *HelpModal) PageUp(pageSize int) {
	if len(h.Filtered) == 0 {
		return
	}
	h.SelectedIndex -= pageSize
	if h.SelectedIndex < 0 {
		h.SelectedIndex = 0
	}
}

// SelectedItem returns the currently highlighted help item, or nil.
func (h *HelpModal) SelectedItem() *HelpItem {
	if !h.Open || len(h.Filtered) == 0 || h.SelectedIndex < 0 || h.SelectedIndex >= len(h.Filtered) {
		return nil
	}
	return &h.Filtered[h.SelectedIndex]
}

// Filter filters the help items according to the query.
func (h *HelpModal) Filter() {
	q := strings.ToLower(strings.TrimSpace(h.Query))
	if q == "" {
		h.Filtered = h.Items
		h.SelectedIndex = 0
		h.ScrollY = 0
		return
	}

	type scoredItem struct {
		item  HelpItem
		score int
	}

	var matches []scoredItem
	for _, it := range h.Items {
		chordLow := strings.ToLower(it.Chord)
		titleLow := strings.ToLower(it.Title)
		catLow := strings.ToLower(it.Category)
		descLow := strings.ToLower(it.Description)

		score := 0
		if chordLow == q {
			score += 100
		} else if strings.HasPrefix(chordLow, q) {
			score += 60
		} else if strings.Contains(chordLow, q) {
			score += 40
		}

		if strings.HasPrefix(titleLow, q) {
			score += 50
		} else if strings.Contains(titleLow, q) {
			score += 30
		}

		if strings.Contains(catLow, q) {
			score += 20
		}
		if strings.Contains(descLow, q) {
			score += 10
		}

		if score > 0 {
			matches = append(matches, scoredItem{item: it, score: score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	res := make([]HelpItem, len(matches))
	for i, m := range matches {
		res[i] = m.item
	}

	h.Filtered = res
	h.SelectedIndex = 0
	h.ScrollY = 0
}

// Render renders the centered floating help cheatsheet modal.
func (h *HelpModal) Render(termWidth, termHeight int, theme *Theme) []string {
	if !h.Open || termWidth < 30 || termHeight < 8 {
		return nil
	}

	modalWidth := termWidth * 75 / 100
	if modalWidth < 68 {
		modalWidth = 68
	}
	if modalWidth > termWidth-4 {
		modalWidth = termWidth - 4
	}

	modalHeight := termHeight * 75 / 100
	if modalHeight < 12 {
		modalHeight = 12
	}
	if modalHeight > termHeight-4 {
		modalHeight = termHeight - 4
	}

	innerWidth := modalWidth - 2
	// Rows: TopBorder(1), Query(1), Header(1), FooterDivider(1), Footer(1), BottomBorder(1) = 6 fixed rows
	listHeight := modalHeight - 6
	if listHeight < 1 {
		listHeight = 1
	}

	// Update scroll window around selected item
	if h.SelectedIndex < h.ScrollY {
		h.ScrollY = h.SelectedIndex
	} else if h.SelectedIndex >= h.ScrollY+listHeight {
		h.ScrollY = h.SelectedIndex - listHeight + 1
	}
	if h.ScrollY < 0 {
		h.ScrollY = 0
	}

	rows := make([]string, 0, modalHeight)

	// 1. Top border
	title := " Help & Keyboard Reference (? or Esc to close) "
	rows = append(rows, renderModalTopBorder(title, innerWidth, theme))

	// 2. Query input
	queryText := fmt.Sprintf(" > Search help: %s_", h.Query)
	truncatedQuery := ansi.Truncate(queryText, innerWidth, "")
	qPad := innerWidth - ansi.StringWidth(truncatedQuery)
	if qPad < 0 {
		qPad = 0
	}
	rows = append(rows, theme.BorderLine.Render("│")+theme.TextSelected.Render(truncatedQuery)+strings.Repeat(" ", qPad)+theme.BorderLine.Render("│"))

	// 3. Column header divider
	colHeader := formatColumns("CHORD / KEY", "ACTION", "CATEGORY", "DESCRIPTION", innerWidth)
	rows = append(rows, theme.BorderLine.Render("├")+theme.DiffModified.Render(colHeader)+theme.BorderLine.Render("┤"))

	// 4. Items
	chordColWidth := 19
	titleColWidth := 21
	catColWidth := 13

	for i := 0; i < listHeight; i++ {
		idx := h.ScrollY + i
		if idx >= len(h.Filtered) {
			rows = append(rows, theme.BorderLine.Render("│")+strings.Repeat(" ", innerWidth)+theme.BorderLine.Render("│"))
			continue
		}

		item := h.Filtered[idx]
		isCursor := (idx == h.SelectedIndex)

		prefix := "  "
		if isCursor {
			prefix = "▶ "
		}

		chordStr := prefix + item.Chord
		chordStr = fitWidth(chordStr, chordColWidth)

		titleStr := fitWidth(item.Title, titleColWidth)
		catStr := fitWidth("["+item.Category+"]", catColWidth)

		remWidth := innerWidth - chordColWidth - titleColWidth - catColWidth - 1
		if remWidth < 5 {
			remWidth = 5
		}
		descStr := fitWidth(item.Description, remWidth)

		rowText := chordStr + " " + titleStr + " " + catStr + " " + descStr
		rowText = fitWidth(rowText, innerWidth)

		var styled string
		if isCursor {
			styled = theme.TextSelected.Render(rowText)
		} else {
			styled = theme.TextNormal.Render(rowText)
		}

		rows = append(rows, theme.BorderLine.Render("│")+styled+theme.BorderLine.Render("│"))
	}

	// 5. Footer divider
	rows = append(rows, theme.BorderLine.Render("├"+strings.Repeat("─", innerWidth)+"┤"))

	// 6. Footer instructions
	footerText := " [↑/↓/Tab]: Navigate  [Enter]: Execute Action  [Esc/q/?]: Dismiss"
	footerTrunc := ansi.Truncate(footerText, innerWidth, "")
	fPad := innerWidth - ansi.StringWidth(footerTrunc)
	if fPad < 0 {
		fPad = 0
	}
	rows = append(rows, theme.BorderLine.Render("│")+theme.StatusInfo.Render(footerTrunc)+strings.Repeat(" ", fPad)+theme.BorderLine.Render("│"))

	// 7. Bottom border
	rows = append(rows, theme.BorderLine.Render("└"+strings.Repeat("─", innerWidth)+"┘"))

	return rows
}

func formatColumns(chord, title, cat, desc string, totalWidth int) string {
	chordW := 19
	titleW := 21
	catW := 13

	s1 := fitWidth("  "+chord, chordW)
	s2 := fitWidth(title, titleW)
	s3 := fitWidth(cat, catW)

	rem := totalWidth - chordW - titleW - catW - 1
	if rem < 5 {
		rem = 5
	}
	s4 := fitWidth(desc, rem)

	return fitWidth(s1+" "+s2+" "+s3+" "+s4, totalWidth)
}

func fitWidth(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-w)
}

func makeDefaultHelpItems() []HelpItem {
	return []HelpItem{
		// 1. Navigation & Viewport
		{"h / Left", "Move Left", "Navigation", "Move cursor left 1 column", fsm.ActionMoveLeft},
		{"l / Right", "Move Right", "Navigation", "Move cursor right 1 column", fsm.ActionMoveRight},
		{"k / Up", "Move Up", "Navigation", "Move cursor up 1 row", fsm.ActionMoveUp},
		{"j / Down", "Move Down", "Navigation", "Move cursor down 1 row", fsm.ActionMoveDown},
		{"0 / Home", "Line Start", "Navigation", "Move to beginning of current line", fsm.ActionMoveLineStart},
		{"$ / End", "Line End", "Navigation", "Move to end of current line", fsm.ActionMoveLineEnd},
		{"Ctrl+Home", "Buffer Start", "Navigation", "Jump to the very beginning of buffer", fsm.ActionMoveBufferStart},
		{"Ctrl+End", "Buffer End", "Navigation", "Jump to the end of buffer", fsm.ActionMoveBufferEnd},
		{"Ctrl+u / PgUp", "Page Up", "Navigation", "Scroll viewport up by page", fsm.ActionPageUp},
		{"Ctrl+d / PgDn", "Page Down", "Navigation", "Scroll viewport down by page", fsm.ActionPageDown},
		{"Ctrl+o", "Jumplist Back", "Navigation", "Jump back in location history", fsm.ActionJumplistBack},
		{"Ctrl+i", "Jumplist Forward", "Navigation", "Jump forward in location history", fsm.ActionJumplistForward},
		{"Alt+Left", "Word Backward", "Navigation", "Move cursor word backward", fsm.ActionMoveWordBackward},
		{"Alt+Right", "Word Forward", "Navigation", "Move cursor word forward", fsm.ActionMoveWordForward},

		// 2. Selection-First & Mutations
		{"w", "Select Word", "Select & Edit", "Select word forward (enters SELECT mode)", fsm.ActionSelectWord},
		{"b", "Select Word Back", "Select & Edit", "Select word backward (enters SELECT mode)", fsm.ActionSelectWordBackward},
		{"x", "Select Line", "Select & Edit", "Select entire line (enters SELECT mode)", fsm.ActionSelectLine},
		{"i", "Insert Mode", "Select & Edit", "Enter INSERT mode before cursor", fsm.ActionNone},
		{"a", "Append Mode", "Select & Edit", "Enter INSERT mode after cursor", fsm.ActionNone},
		{"o", "Open Below", "Select & Edit", "Enter INSERT mode on newline below", fsm.ActionNone},
		{"jk", "Fast Exit", "Select & Edit", "Fast exit INSERT -> NORMAL (<200ms)", fsm.ActionNone},
		{"Esc", "Cancel / Normal", "Select & Edit", "Collapse selection / return to NORMAL", fsm.ActionCollapseSelection},
		{"d", "Delete", "Select & Edit", "Delete selection (or character under cursor)", fsm.ActionDeleteSelection},
		{"c", "Change", "Select & Edit", "Delete selection and switch to INSERT", fsm.ActionChangeSelection},
		{"y", "Yank", "Select & Edit", "Yank selection to clipboard register", fsm.ActionYank},
		{"p / P", "Paste", "Select & Edit", "Paste clipboard after / before cursor", fsm.ActionPasteAfter},
		{"u", "Undo", "Select & Edit", "Undo last buffer transaction", fsm.ActionUndo},
		{"U", "Redo", "Select & Edit", "Redo previously undone transaction", fsm.ActionRedo},
		{"> / <", "Indent / Outdent", "Select & Edit", "Indent or outdent selected lines", fsm.ActionIndent},
		{"!", "Shell Pipe", "Select & Edit", "Filter selection through external shell command", fsm.ActionPipeSelection},

		// 3. Nordic Semantic Text Objects
		{"m i b / m a b", "Block Object", "Objects", "Inside / Around Block { } (or Shell/YAML block)", fsm.ActionNone},
		{"m i a / m a a", "Array Object", "Objects", "Inside / Around Array [ ] (or TOML/YAML array)", fsm.ActionNone},
		{"m i p / m a p", "Parens Object", "Objects", "Inside / Around Parens ( ) (or subshell/CMake)", fsm.ActionNone},
		{"m i q / m a q", "Quote Object", "Objects", "Inside / Around Quotes \"...\" or '...'", fsm.ActionNone},
		{"m i f / m a f", "Function Object", "Objects", "Inside / Around Function AST node (or Make rule)", fsm.ActionNone},
		{"m i c / m a c", "Class/Struct Object", "Objects", "Inside / Around Class / Struct / TOML table", fsm.ActionNone},
		{"m i s", "Statement Object", "Objects", "Inside Statement AST node", fsm.ActionNone},

		// 4. Space Leader Catalog
		{"Space f f", "Find Files", "Leader", "Fuzzy File Finder (git ls-files & fallback)", fsm.ActionPickerOpenFiles},
		{"Space f g", "Live Grep", "Leader", "Real-time workspace search (ripgrep)", fsm.ActionPickerOpenGrep},
		{"Space f s", "Save File", "Leader", "Atomically save active buffer to disk", fsm.ActionSave},
		{"Ctrl+S", "Save File", "Leader", "Quick save active buffer (Normal & Insert)", fsm.ActionSave},
		{"Space f a", "Save All", "Leader", "Save all modified buffers to disk", fsm.ActionSaveAll},
		{"Space b b", "Switch Buffer", "Leader", "Buffer Switcher fuzzy finder modal", fsm.ActionPickerOpenBuffers},
		{"Space b d", "Close Buffer", "Leader", "Close current buffer tab (prompts if unsaved)", fsm.ActionBufferClose},
		{"Space b D", "Force Close", "Leader", "Force close buffer without saving", fsm.ActionBufferForceClose},
		{"Space e", "Toggle Tree", "Leader", "Toggle File Tree sidebar", fsm.ActionToggleFileTree},
		{"Tree: a", "New File", "File Tree", "Create new file in selected folder", fsm.ActionTreeCreateFile},
		{"Tree: A", "New Folder", "File Tree", "Create new directory in selected folder", fsm.ActionTreeCreateDir},
		{"Tree: d (x2)", "Delete Item", "File Tree", "Delete selected file or directory (press twice)", fsm.ActionTreeDelete},
		{"Space o", "Output Drawer", "Leader", "Toggle Ephemeral Output & Build drawer", fsm.ActionToggleDrawer},
		{"Space a", "Alternate File", "Leader", "Switch C/C++ alternate file (.cpp <-> .hpp)", fsm.ActionAlternateFile},
		{"/", "Search Buffer", "Leader", "In-buffer interactive live search", fsm.ActionSearchOpen},
		{"Space s r", "Replace Buffer", "Leader", "Search & Replace in current buffer drawer", fsm.ActionReplaceOpen},
		{"Space s p", "Replace Project", "Leader", "Project-wide Search & Replace drawer", fsm.ActionProjectReplaceOpen},
		{"Space c h", "Clear Highlights", "Leader", "Clear active search highlight styling", fsm.ActionSearchClearHighlight},
		{"Space n s / n", "Next Match", "Leader", "Jump to next search match", fsm.ActionSearchNext},
		{"Space p s / N", "Prev Match", "Leader", "Jump to previous search match", fsm.ActionSearchPrev},
		{"Space ? / ? / F1", "Help Cheatsheet", "Leader", "Toggle this Help & Keyboard reference", fsm.ActionHelpToggle},

		// 5. LSP Intelligence
		{"Space g d", "Definition", "LSP", "Go to Definition (records to Jumplist)", fsm.ActionLSPDefinition},
		{"Space g i", "Implementation", "LSP", "Go to Implementation (interfaces & types)", fsm.ActionLSPImplementation},
		{"Space g c", "Call Hierarchy", "LSP", "Display incoming and outgoing call tree", fsm.ActionLSPCallHierarchy},
		{"Space k / K", "Hover Docs", "LSP", "Hover documentation and type signatures", fsm.ActionLSPHover},
		{"Ctrl+Space", "Autocomplete", "LSP", "Trigger autocomplete suggestion popover", fsm.ActionLSPComplete},
		{"Space c a", "Code Actions", "LSP", "Quickfix menu (missing imports, types)", fsm.ActionLSPCodeAction},
		{"Space c r", "Rename Symbol", "LSP", "Rename symbol across entire open workspace", fsm.ActionLSPRename},
		{"Space n d", "Next Diagnostic", "LSP", "Jump to next LSP error or warning", fsm.ActionNextDiagnostic},
		{"Space p d", "Prev Diagnostic", "LSP", "Jump to previous LSP diagnostic", fsm.ActionPrevDiagnostic},
		{"Space n e", "Next Error", "LSP", "Jump to next compiler build error", fsm.ActionNextError},
		{"Space p e", "Prev Error", "LSP", "Jump to previous compiler build error", fsm.ActionPrevError},
		{"ä / ö", "Quick Step", "LSP", "Nordic quick step forward / backward", fsm.ActionStepForward},

		// 6. DAP Interactive Debugger
		{"Space d b / F9", "Breakpoint", "Debugger", "Toggle Breakpoint on current line", fsm.ActionDAPToggleBreakpoint},
		{"Space d c / F5", "Continue / Start", "Debugger", "Start execution or continue to next break", fsm.ActionDAPContinue},
		{"Space d n / F10", "Step Over", "Debugger", "Step over next statement/line", fsm.ActionDAPStepOver},
		{"Space d i / F11", "Step Into", "Debugger", "Step into function call", fsm.ActionDAPStepInto},
		{"Space d o / S-F11", "Step Out", "Debugger", "Step out of current function stack", fsm.ActionDAPStepOut},
		{"Space d p", "Pause Target", "Debugger", "Pause running execution", fsm.ActionDAPPause},
		{"Space d q / S-F5", "Stop Debugger", "Debugger", "Stop and disconnect debug session", fsm.ActionDAPStop},
		{"Space d v", "Debug Inspector", "Debugger", "Toggle Call Stack and Variables drawer", fsm.ActionDAPToggleView},

		// 7. Markdown & Mermaid
		{"Space m p", "Markdown Preview", "Markdown", "Toggle Markdown & Mermaid live preview drawer", fsm.ActionToggleMarkdownPreview},
		{"Space m t", "Toggle Checklist", "Markdown", "Toggle checklist checkbox (- [ ] <-> - [x])", fsm.ActionToggleChecklist},
		{"Space m d", "Mermaid Mode", "Markdown", "Toggle Mermaid Box-art vs Kitty graphics", fsm.ActionToggleMermaidMode},

		// 8. Agent & Batch Review
		{"Space d s", "Diff Review", "Agent", "Open Agent Batch Diff Review drawer", fsm.ActionAgentReviewOpen},
		{"Space d a", "Accept All", "Agent", "Accept all changes in pending agent burst", fsm.ActionAgentAcceptAll},
		{"Space d r", "Revert All", "Agent", "Revert all changes in pending agent burst", fsm.ActionAgentRevertAll},

		// 9. Panes & Layout
		{"Ctrl+w", "Cycle Focus", "Layout", "Cycle focus (Buffer -> Drawer -> Tree)", fsm.ActionPaneCycle},
		{"H / L", "Tab Prev / Next", "Layout", "Switch to previous or next buffer tab", fsm.ActionTabPrev},
		{"Alt+1 .. Alt+9", "Direct Tab Switch", "Layout", "Direct jump to buffer tab 1 through 9", fsm.ActionTabSwitch},
		{"Z", "Maximize Drawer", "Layout", "Maximize / restore ephemeral drawer height", fsm.ActionDrawerMaximize},
		{"Space w > / <", "Resize Tree", "Layout", "Grow or shrink File Tree width (+/- 3 cols)", fsm.ActionResizeTreeGrow},
		{"Space w + / -", "Resize Drawer", "Layout", "Grow or shrink Output Drawer (+/- 3 rows)", fsm.ActionResizeDrawerGrow},

		// 10. Mouse & Touchpad
		{"Scrollbar Click", "Jump Viewport", "Mouse", "Jump viewport directly to clicked position", fsm.ActionNone},
		{"Scrollbar Drag", "Scrub Viewport", "Mouse", "Smoothly scrub viewport through buffer", fsm.ActionNone},
		{"Gutter Click", "Breakpoint Click", "Mouse", "Toggle breakpoint on clicked line (Col 1)", fsm.ActionNone},
		{"Code Drag", "Drag Selection", "Mouse", "Drag-select text into SELECT mode", fsm.ActionNone},
		{"Wheel Up/Down", "Smooth Scroll", "Mouse", "Smoothly scroll viewport by 3 lines", fsm.ActionNone},
		{"Tab Middle Click", "Close Tab", "Mouse", "Close clicked buffer tab", fsm.ActionNone},
		{"Border Drag", "Resize Panes", "Mouse", "Drag File Tree or Drawer border to resize", fsm.ActionNone},
	}
}
