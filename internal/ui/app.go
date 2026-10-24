package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/dap"
	"github.com/mikolas/koodieditori/internal/diff"
	"github.com/mikolas/koodieditori/internal/fsm"
	"github.com/mikolas/koodieditori/internal/ipc"
	"github.com/mikolas/koodieditori/internal/lsp"
	"github.com/mikolas/koodieditori/internal/markdown"
	"github.com/mikolas/koodieditori/internal/search"
	"github.com/mikolas/koodieditori/internal/tooling"
	"github.com/mikolas/koodieditori/internal/treesitter"
	"github.com/mikolas/koodieditori/internal/watcher"
)

// IPCMsg conveys an IPC request to the Bubble Tea main event loop.
type IPCMsg struct {
	Type   string
	Params any
	Done   chan error
}

// WatcherMsg conveys inotify filesystem change events to the Bubble Tea loop.
type WatcherMsg struct {
	Event watcher.ChangeEvent
}

// LSP message types for async Bubble Tea commands
type LSPDiagnosticsMsg struct {
	Update lsp.DiagnosticsUpdate
}

type LSPDefinitionMsg struct {
	Locations []lsp.Location
	Err       error
}

type LSPImplementationMsg struct {
	Locations []lsp.Location
	Err       error
}

type LSPHoverMsg struct {
	Hover *lsp.Hover
	Err   error
}

type LSPCompletionMsg struct {
	Items  []lsp.CompletionItem
	Prefix string
	Err    error
}

type LSPCodeActionMsg struct {
	Actions []lsp.CodeAction
	Err     error
}

type LSPRenameMsg struct {
	Edit *lsp.WorkspaceEdit
	Err  error
}

type LSPStatusMsg struct {
	FilePath   string
	ServerName string
	Status     lsp.ClientStatus
}

// App is the root Bubble Tea application model for Koodieditori.
type App struct {
	Config    *config.Config
	Theme     *Theme
	FSM       *fsm.FSM
	Buffers   []*BufferItem
	ActiveBuf int

	// Terminal geometry
	Width  int
	Height int

	// Workspace
	WorkspacePath string
	WorkspaceName string
	StatusMessage string
	AgentStatus   string
	AgentBusy     bool

	// Yank register (clipboard)
	YankRegister string

	// Output Drawer & Tooling
	Drawer   *Drawer
	Jumplist *tooling.Jumplist

	// Markdown & Mermaid
	MermaidMode markdown.MermaidMode

	// Agentic CLI Coexistence & IPC
	Watcher   *watcher.Watcher
	IPCServer *ipc.Server
	AgentTx   *diff.AgentTransaction
	Program   *tea.Program

	// File Tree & Fuzzy Finder
	FileTree  *FileTree
	Picker    *Picker
	HelpModal *HelpModal

	// LSP Client & Code Intelligence
	LSPManager       *lsp.Manager
	lspDiagCh        chan lsp.DiagnosticsUpdate
	HoverContent     string
	CompletionItems  []lsp.CompletionItem
	CompletionIndex  int
	CompletionPrefix string
	CodeActions      []lsp.CodeAction
	CodeActionIndex  int
	RenameActive     bool
	RenameInput      string
	RenameOriginal   string
	RenamePos        lsp.Position

	// LSP debouncing
	lspDebounceMu    sync.Mutex
	lspDebounceTimer *time.Timer
	lspPendingFile   string
	lspPendingText   string
	lspPendingVer    int

	// In-Buffer & Project Search / Replace
	SearchActive     bool
	SearchQuery      string
	SearchMatches    []search.Match
	SearchIndex      int
	SearchOptions    search.SearchOptions
	SearchHistory    []string
	SearchHistoryIdx int
	ReplaceText      string
	ReplaceScope     string // "buffer" or "project"
	ReplaceFocused   int    // 0: Find, 1: Replace, 2: Preview diff

	// Scrollbar interaction
	isScrollbarDragging bool

	// DAP Debugger
	DAPManager *dap.Manager

	// Safety confirmations & FileTree prompts
	quitConfirmPending bool
	quitConfirmTime    time.Time
	treeDeletePending  bool
	treeDeletePath     string
	treeDeleteTime     time.Time
	treePromptMode     string // "" | "file" | "dir"
	treePromptInput    string

	// Insert session undo grouping
	insertBuffer *BufferItem

	mu sync.RWMutex
}

var projectRootMarkers = []string{
	".git",
	"go.mod",
	"Cargo.toml",
	"package.json",
	"CMakeLists.txt",
	"Makefile",
	"pom.xml",
	"build.gradle",
	"pyproject.toml",
	"compile_commands.json",
}

func findProjectRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	cur := filepath.Clean(abs)
	for {
		for _, marker := range projectRootMarkers {
			candidate := filepath.Join(cur, marker)
			if _, err := os.Stat(candidate); err == nil {
				return cur
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == "" {
			break
		}
		cur = parent
	}
	return filepath.Clean(abs)
}

// NewApp creates a new App model with initial configuration and buffers.
func NewApp(cfg *config.Config, initialFiles []string) *App {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	mermaidMode := markdown.MermaidModeBoxArt
	if markdown.DetectKittyGraphics() {
		mermaidMode = markdown.MermaidModeKitty
	}

	// Resolve workspace directory and initial files
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if absCwd, err := filepath.Abs(cwd); err == nil {
		cwd = filepath.Clean(absCwd)
	}

	workspacePath := cwd
	explicitDirGiven := false
	var filesToOpen []string

	for _, arg := range initialFiles {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		info, err := os.Stat(arg)
		if err == nil && info.IsDir() {
			if !explicitDirGiven {
				absDir, err := filepath.Abs(arg)
				if err == nil {
					workspacePath = filepath.Clean(absDir)
				} else {
					workspacePath = filepath.Clean(arg)
				}
				explicitDirGiven = true
			}
		} else {
			filesToOpen = append(filesToOpen, arg)
		}
	}

	if !explicitDirGiven && len(filesToOpen) > 0 {
		firstFile := filesToOpen[0]
		absFile, err := filepath.Abs(firstFile)
		if err != nil {
			absFile = firstFile
		}
		fileDir := filepath.Dir(absFile)
		rel, err := filepath.Rel(cwd, fileDir)
		if err == nil && !strings.HasPrefix(rel, "..") {
			workspacePath = cwd
		} else {
			workspacePath = findProjectRoot(fileDir)
		}
	}

	workspaceName := filepath.Base(workspacePath)
	if workspaceName == "" || workspaceName == "." || workspaceName == string(filepath.Separator) {
		workspaceName = "root"
	}

	app := &App{
		Config:        cfg,
		Theme:         LoadTheme(cfg.Theme),
		FSM:           fsm.NewFSM(),
		Buffers:       make([]*BufferItem, 0),
		ActiveBuf:     0,
		Width:         80,
		Height:        24,
		WorkspacePath: workspacePath,
		WorkspaceName: workspaceName,
		Drawer:        NewDrawer(),
		Jumplist:      tooling.NewJumplist(100),
		MermaidMode:   mermaidMode,
		AgentTx:       diff.NewAgentTransaction(),
		FileTree:      NewFileTree(workspacePath, 26),
		Picker:        NewPicker(workspacePath),
		HelpModal:     NewHelpModal(),
		SearchOptions: search.SearchOptions{CaseSensitive: false, WholeWord: false, IsRegex: false},
		ReplaceScope:  "buffer",
		SearchHistory: make([]string, 0),
	}

	if cfg != nil && len(cfg.Keys) > 0 {
		app.FSM.SetKeyRemaps(cfg.Keys)
	}

	// Initialize LSP Manager
	lspMgr := lsp.NewManager(cfg.LSP, workspacePath)
	diagCh := lspMgr.SubscribeDiagnostics(100)
	app.LSPManager = lspMgr
	app.lspDiagCh = diagCh

	// Initialize DAP Manager
	dapAdapters := make(map[string]dap.AdapterConfig)
	for k, v := range cfg.DAP.Adapters {
		dapAdapters[k] = dap.AdapterConfig{
			Command:   v.Command,
			Args:      v.Args,
			Filetypes: v.Filetypes,
			Launch:    v.Launch,
		}
	}
	app.DAPManager = dap.NewManager(dapAdapters)

	if len(filesToOpen) == 0 {
		app.Buffers = append(app.Buffers, NewBufferItem(1, "", ""))
		if explicitDirGiven {
			app.FileTree.Open = true
			app.FSM.SetState(fsm.StateTree)
		}
	} else {
		for i, f := range filesToOpen {
			content := ""
			if data, err := os.ReadFile(f); err == nil {
				content = string(data)
			}
			app.Buffers = append(app.Buffers, NewBufferItem(i+1, f, content))
		}
	}

	return app
}

// DAPEventMsg conveys an asynchronous DAP notification to the Bubble Tea loop.
type DAPEventMsg struct {
	Event dap.ManagerEvent
}

// Init initializes the Bubble Tea loop.
func (a *App) Init() tea.Cmd {
	if curr := a.CurrentBuffer(); curr != nil && curr.FilePath != "" {
		a.ensureLSPForBuffer(curr)
	}
	return tea.Batch(a.waitForDiagnostics(), a.waitForDAPEvents())
}

func (a *App) waitForDAPEvents() tea.Cmd {
	return func() tea.Msg {
		if a.DAPManager == nil {
			return nil
		}
		ev, ok := <-a.DAPManager.EventCh
		if !ok {
			return nil
		}
		return DAPEventMsg{Event: ev}
	}
}

func (a *App) waitForDiagnostics() tea.Cmd {
	return func() tea.Msg {
		if a.lspDiagCh == nil {
			return nil
		}
		update, ok := <-a.lspDiagCh
		if !ok {
			return nil
		}
		return LSPDiagnosticsMsg{Update: update}
	}
}

// Update processes Bubble Tea messages (keys, mouse, resize).
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.Width = msg.Width
		a.Height = msg.Height
		if a.Drawer != nil && a.Drawer.Open {
			if a.Drawer.Mode == DrawerModeMarkdown {
				a.refreshMarkdownPreview()
			} else if a.Drawer.Mode == DrawerModeDiffReview {
				a.refreshDiffReview()
			} else if a.Drawer.Mode == DrawerModeSearchReplace {
				a.refreshSearchReplace()
			} else if a.Drawer.Mode == DrawerModeDebug {
				a.refreshDebugDrawer()
			}
		}
		return a, nil

	case WatcherMsg:
		a.handleWatcherEvent(msg.Event)
		return a, nil

	case IPCMsg:
		err := a.handleIPCMsg(msg)
		if msg.Done != nil {
			msg.Done <- err
		}
		return a, nil

	case LSPStatusMsg:
		return a, nil

	case LSPDiagnosticsMsg:
		if a.LSPManager != nil {
			a.LSPManager.StoreDiagnostics(msg.Update.URI, msg.Update.Diagnostics)
		}
		for _, b := range a.Buffers {
			if isSameFile(b.FilePath, msg.Update.FilePath) {
				b.DiagnosticsCount = len(msg.Update.Diagnostics)
			}
		}
		return a, a.waitForDiagnostics()

	case LSPDefinitionMsg:
		if msg.Err != nil {
			a.StatusMessage = "Definition: " + msg.Err.Error()
			return a, nil
		}
		if len(msg.Locations) == 0 {
			a.StatusMessage = "No definition found"
			return a, nil
		}
		loc := msg.Locations[0]
		destPath := lsp.URIToPath(loc.URI)
		curr := a.CurrentBuffer()
		if a.Jumplist != nil && curr != nil {
			a.Jumplist.Record(tooling.Location{
				FilePath: curr.FilePath,
				Line:     curr.CursorPos.Line,
				Col:      curr.CursorPos.Col,
			})
		}
		a.openOrSwitchToFile(destPath)
		if destBuf := a.CurrentBuffer(); destBuf != nil {
			destBuf.CursorPos.Line = loc.Range.Start.Line
			lineText, _ := destBuf.PT.Line(loc.Range.Start.Line)
			destBuf.CursorPos.Col = lsp.UTF16ColToRuneCol(lineText, loc.Range.Start.Character)
		}
		a.StatusMessage = fmt.Sprintf("Definition: %s:%d:%d", filepath.Base(destPath), loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		return a, nil

	case LSPImplementationMsg:
		if msg.Err != nil {
			a.StatusMessage = "Implementation: " + msg.Err.Error()
			return a, nil
		}
		if len(msg.Locations) == 0 {
			a.StatusMessage = "No implementation found"
			return a, nil
		}
		loc := msg.Locations[0]
		destPath := lsp.URIToPath(loc.URI)
		curr := a.CurrentBuffer()
		if a.Jumplist != nil && curr != nil {
			a.Jumplist.Record(tooling.Location{
				FilePath: curr.FilePath,
				Line:     curr.CursorPos.Line,
				Col:      curr.CursorPos.Col,
			})
		}
		a.openOrSwitchToFile(destPath)
		if destBuf := a.CurrentBuffer(); destBuf != nil {
			destBuf.CursorPos.Line = loc.Range.Start.Line
			lineText, _ := destBuf.PT.Line(loc.Range.Start.Line)
			destBuf.CursorPos.Col = lsp.UTF16ColToRuneCol(lineText, loc.Range.Start.Character)
		}
		a.StatusMessage = fmt.Sprintf("Implementation: %s:%d:%d", filepath.Base(destPath), loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		return a, nil

	case LSPHoverMsg:
		if msg.Err != nil {
			a.StatusMessage = "Hover: " + msg.Err.Error()
			return a, nil
		}
		if msg.Hover == nil || msg.Hover.Contents == nil {
			a.StatusMessage = "No hover info available"
			return a, nil
		}
		text := lsp.ExtractHoverText(msg.Hover.Contents)
		if text == "" {
			a.StatusMessage = "No hover documentation"
			return a, nil
		}
		a.HoverContent = text
		a.FSM.SetState(fsm.StateHover)
		return a, nil

	case LSPCompletionMsg:
		if msg.Err != nil {
			a.StatusMessage = fmt.Sprintf("Autocomplete: %v", msg.Err)
			return a, nil
		}
		if len(msg.Items) == 0 {
			a.StatusMessage = "No completions available"
			return a, nil
		}
		a.CompletionItems = msg.Items
		a.CompletionIndex = 0
		a.CompletionPrefix = msg.Prefix
		a.FSM.SetState(fsm.StateCompletion)
		return a, nil

	case LSPCodeActionMsg:
		if msg.Err != nil {
			a.StatusMessage = "Code actions: " + msg.Err.Error()
			return a, nil
		}
		if len(msg.Actions) == 0 {
			a.StatusMessage = "No code actions available"
			return a, nil
		}
		a.CodeActions = msg.Actions
		a.CodeActionIndex = 0
		a.FSM.SetState(fsm.StateCodeAction)
		return a, nil

	case LSPRenameMsg:
		if msg.Err != nil {
			a.StatusMessage = "Rename failed: " + msg.Err.Error()
			return a, nil
		}
		if msg.Edit == nil {
			a.StatusMessage = "Rename: no changes returned"
			return a, nil
		}
		a.applyWorkspaceEdit(msg.Edit)
		a.StatusMessage = fmt.Sprintf("Renamed '%s' to '%s'", a.RenameOriginal, a.RenameInput)
		a.RenameActive = false
		return a, nil

	case DAPEventMsg:
		a.handleDAPEvent(msg.Event)
		return a, a.waitForDAPEvents()

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if a.hasDirtyBuffers() {
				now := time.Now()
				if !a.quitConfirmPending || now.Sub(a.quitConfirmTime) > 2*time.Second {
					a.quitConfirmPending = true
					a.quitConfirmTime = now
					a.StatusMessage = "⚠ Unsaved changes in buffer(s)! Press Ctrl+C again within 2s to quit without saving"
					return a, nil
				}
			}
			return a, tea.Quit
		}
		a.quitConfirmPending = false
		prevState := a.FSM.State()
		keyStr := normalizeKey(msg)
		act := a.FSM.HandleKey(keyStr)

		// Begin transaction when transitioning into StateInsert
		if prevState != fsm.StateInsert && a.FSM.State() == fsm.StateInsert {
			curr := a.CurrentBuffer()
			if curr != nil && curr.PT != nil {
				curr.PT.BeginTransaction()
				a.insertBuffer = curr
			}
		}

		cmd := a.executeAction(act)

		// Commit transaction when transitioning out of StateInsert
		if prevState == fsm.StateInsert && a.FSM.State() != fsm.StateInsert {
			if a.insertBuffer != nil && a.insertBuffer.PT != nil {
				a.insertBuffer.PT.CommitTransaction()
				a.insertBuffer = nil
			} else {
				curr := a.CurrentBuffer()
				if curr != nil && curr.PT != nil {
					curr.PT.CommitTransaction()
				}
			}
			a.flushLSPBufferChanged(a.CurrentBuffer())
		}
		return a, cmd

	case tea.MouseMsg:
		a.handleMouse(msg)
		return a, nil
	}

	return a, nil
}

// View renders the deterministic layout: Bufferline, Viewport, Drawer (if open), Statusline.
func (a *App) View() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.Width <= 0 || a.Height <= 0 {
		return ""
	}

	currBuf := a.CurrentBuffer()
	if currBuf == nil {
		return ""
	}

	// 1. Row 0: Bufferline Tabs
	bufferlineHeight := 1
	var bufferline string
	if a.Config.Bufferline.Show {
		bufferline = RenderBufferline(a.Buffers, a.ActiveBuf, a.Width, a.WorkspaceName, a.Theme)
	} else {
		bufferlineHeight = 0
	}

	// 2. Row H-1: Statusline & Diagnostics
	diagSigns := make(map[int]string)
	var lspStatusMsg string
	var diagStatusMsg string
	if a.LSPManager != nil && currBuf.FilePath != "" {
		diags := a.LSPManager.GetDiagnostics(currBuf.FilePath)
		for _, d := range diags {
			// Suppress gutter diagnostic sign on the active editing line while in INSERT mode
			// to eliminate distracting 'E' flashing on every keystroke
			if a.FSM.State() == fsm.StateInsert && d.Range.Start.Line == currBuf.CursorPos.Line {
				continue
			}
			sign := "I"
			if d.Severity == lsp.SeverityError {
				sign = "E"
			} else if d.Severity == lsp.SeverityWarning {
				sign = "W"
			}
			existing, exists := diagSigns[d.Range.Start.Line]
			if !exists || sign == "E" || (sign == "W" && existing != "E") {
				diagSigns[d.Range.Start.Line] = sign
			}
		}

		errs, warns, _ := a.LSPManager.CountDiagnostics(currBuf.FilePath)
		if (errs > 0 || warns > 0) && a.FSM.State() != fsm.StateInsert {
			diagStatusMsg = fmt.Sprintf("E:%d W:%d", errs, warns)
		}

		if srvName, st, running := a.LSPManager.GetClientStatus(currBuf.FilePath); srvName != "" {
			if !running {
				lspStatusMsg = srvName + ": idle"
			} else {
				switch st {
				case lsp.StatusReady:
					lspStatusMsg = srvName + ": ok"
				case lsp.StatusStarting:
					lspStatusMsg = srvName + ": starting"
				case lsp.StatusError:
					lspStatusMsg = srvName + ": err"
				case lsp.StatusStopped:
					lspStatusMsg = srvName + ": stopped"
				default:
					lspStatusMsg = srvName + ": " + string(st)
				}
			}
		}
	}

	statusMsg := a.StatusMessage
	if statusMsg == "" {
		statusMsg = diagStatusMsg
	}
	if a.FSM.State() == fsm.StateRename {
		statusMsg = fmt.Sprintf("Rename '%s' to: %s_", a.RenameOriginal, a.RenameInput)
	}

	searchHighlights := make(map[int][]SearchHighlight)
	if (a.SearchActive || a.FSM.State() == fsm.StateSearch) && len(a.SearchMatches) > 0 {
		for idx, m := range a.SearchMatches {
			if m.FilePath == "" || isSameFile(m.FilePath, currBuf.FilePath) {
				isCur := (idx == a.SearchIndex)
				searchHighlights[m.Line] = append(searchHighlights[m.Line], SearchHighlight{
					StartCol:  m.Col,
					EndCol:    m.Col + m.RuneLength,
					IsCurrent: isCur,
				})
			}
		}
	}

	displayPath := currBuf.FilePath
	if a.WorkspacePath != "" && currBuf.FilePath != "" {
		if rel, err := filepath.Rel(a.WorkspacePath, currBuf.FilePath); err == nil && !strings.HasPrefix(rel, "..") {
			displayPath = rel
		}
	}

	statuslineHeight := 1
	statusState := StatuslineState{
		Mode:           a.FSM.State(),
		FilePath:       displayPath,
		IsDirty:        currBuf.Dirty,
		IsReadOnly:     currBuf.ReadOnly,
		FileType:       currBuf.FileName,
		Line:           currBuf.CursorPos.Line,
		Col:            currBuf.CursorPos.Col,
		TotalLines:     currBuf.PT.LineCount(),
		AgentStatus:    a.AgentStatus,
		AgentBusy:      a.AgentBusy,
		DiagnosticsMsg: statusMsg,
		LSPMsg:         lspStatusMsg,
	}
	if a.FSM.State() == fsm.StateSearch {
		statusState.SearchPrompt = "/ " + a.SearchQuery + "█"
		if len(a.SearchMatches) > 0 {
			statusState.SearchMatchMsg = fmt.Sprintf("[%d matches]", len(a.SearchMatches))
		} else if a.SearchQuery != "" {
			statusState.SearchMatchMsg = "[Pattern not found]"
		}
	} else if a.FSM.State() == fsm.StateTree && a.treePromptMode != "" {
		if a.treePromptMode == "dir" {
			statusState.SearchPrompt = "New Folder: " + a.treePromptInput + "█"
		} else {
			statusState.SearchPrompt = "New File: " + a.treePromptInput + "█"
		}
	} else if a.SearchActive && len(a.SearchMatches) > 0 {
		statusState.SearchMatchMsg = fmt.Sprintf("[%d/%d matches]", a.SearchIndex+1, len(a.SearchMatches))
	}
	if a.DAPManager != nil {
		dState := a.DAPManager.State()
		if dState == dap.DAPStateStopped {
			if statusState.DiagnosticsMsg != "" {
				statusState.DiagnosticsMsg = "[DEBUG: PAUSED] " + statusState.DiagnosticsMsg
			} else {
				statusState.DiagnosticsMsg = "[DEBUG: PAUSED]"
			}
		} else if dState == dap.DAPStateRunning {
			if statusState.DiagnosticsMsg != "" {
				statusState.DiagnosticsMsg = "[DEBUG: RUNNING] " + statusState.DiagnosticsMsg
			} else {
				statusState.DiagnosticsMsg = "[DEBUG: RUNNING]"
			}
		}
	}
	statusline := RenderStatusline(statusState, a.Width, a.Theme)

	availHeight := a.Height - bufferlineHeight - statuslineHeight
	if availHeight < 1 {
		availHeight = 1
	}

	treeOpen := (a.FileTree != nil && a.FileTree.Open)
	treeWidth := 0
	if treeOpen {
		treeWidth = a.FileTree.Width
		if treeWidth >= a.Width-15 {
			treeWidth = a.Width - 15
		}
		if treeWidth < 10 {
			treeWidth = 10
		}
	}
	editorWidth := a.Width - treeWidth
	if editorWidth < 10 {
		editorWidth = 10
	}

	// 3. Drawer calculation
	var drawerRendered string
	drawerContentHeight := 0
	viewportHeight := availHeight

	if a.Drawer != nil && a.Drawer.Open {
		if a.Drawer.Maximized {
			drawerContentHeight = availHeight - 1 // 1 row for header border
			if drawerContentHeight < 1 {
				drawerContentHeight = 1
			}
			viewportHeight = 0
		} else {
			drawerContentHeight = a.Drawer.Height
			if drawerContentHeight <= 0 {
				drawerContentHeight = a.Height * 35 / 100
			}
			maxDrawerH := availHeight - 3 // at least 2 viewport rows + 1 drawer border
			if maxDrawerH < 1 {
				maxDrawerH = 1
			}
			if drawerContentHeight > maxDrawerH {
				drawerContentHeight = maxDrawerH
			}
			if drawerContentHeight < 2 {
				drawerContentHeight = 2
			}
			viewportHeight = availHeight - (drawerContentHeight + 1)
		}
		isDrawerFocused := (a.FSM.State() == fsm.StateDrawer)
		drawerRendered = a.Drawer.Render(editorWidth, drawerContentHeight, a.Theme, isDrawerFocused)
	}

	var viewport string
	if viewportHeight > 0 {
		var fileBreakpoints map[int]bool
		stoppedLine := -1
		if a.DAPManager != nil && currBuf.FilePath != "" {
			bps := a.DAPManager.GetBreakpoints(currBuf.FilePath)
			if len(bps) > 0 {
				fileBreakpoints = make(map[int]bool, len(bps))
				for _, line1 := range bps {
					fileBreakpoints[line1-1] = true
				}
			}
			if stopped := a.DAPManager.StoppedFrame(); stopped != nil {
				if isSameFile(stopped.Source.Path, currBuf.FilePath) {
					stoppedLine = stopped.Line - 1
				}
			}
		}

		vpParams := ViewportParams{
			Buffer:          currBuf,
			FSM:             a.FSM,
			Width:           editorWidth,
			Height:          viewportHeight,
			TabWidth:        a.Config.Editor.TabWidth,
			RelativeLines:   (a.Config.Editor.LineNumbers == "relative"),
			Scrolloff:       a.Config.Editor.Scrolloff,
			Theme:           a.Theme,
			DiagnosticSigns: diagSigns,
			SearchSpans:     searchHighlights,
			Breakpoints:     fileBreakpoints,
			StoppedLine:     stoppedLine,
		}
		viewport = RenderViewport(vpParams)
	}

	var editorLines []string
	if viewportHeight > 0 && len(viewport) > 0 {
		editorLines = append(editorLines, strings.Split(viewport, "\n")...)
	}
	if a.Drawer != nil && a.Drawer.Open && len(drawerRendered) > 0 {
		editorLines = append(editorLines, strings.Split(drawerRendered, "\n")...)
	}
	for len(editorLines) < availHeight {
		editorLines = append(editorLines, strings.Repeat(" ", editorWidth))
	}
	if len(editorLines) > availHeight {
		editorLines = editorLines[:availHeight]
	}

	// 4. LSP Floating Overlays
	if a.FSM.State() == fsm.StateHover && a.HoverContent != "" {
		hoverBox := RenderHoverBox(a.HoverContent, editorWidth, availHeight, a.Theme)
		cursorRow := currBuf.CursorPos.Line - currBuf.ScrollY
		if cursorRow < 0 {
			cursorRow = 0
		}
		boxTop := cursorRow + 1
		if boxTop+10 >= availHeight {
			boxTop = cursorRow - 10
			if boxTop < 0 {
				boxTop = 0
			}
		}
		editorLines = OverlayBoxOnLines(editorLines, hoverBox, boxTop, 6, editorWidth)
	} else if a.FSM.State() == fsm.StateCompletion && len(a.CompletionItems) > 0 {
		compBox := RenderCompletionBox(a.CompletionItems, a.CompletionIndex, editorWidth, a.Theme)
		cursorRow := currBuf.CursorPos.Line - currBuf.ScrollY
		if cursorRow < 0 {
			cursorRow = 0
		}
		boxTop := cursorRow + 1
		if boxTop+9 >= availHeight {
			boxTop = cursorRow - 9
			if boxTop < 0 {
				boxTop = 0
			}
		}
		gutterW := CalculateGutterWidth(currBuf.PT.LineCount())
		lineText, _ := currBuf.PT.Line(currBuf.CursorPos.Line)
		visCursorCol := RuneColToVisualCol(lineText, currBuf.CursorPos.Col, a.Config.Editor.TabWidth)
		boxLeft := gutterW + visCursorCol - currBuf.ScrollX
		if boxLeft < gutterW {
			boxLeft = gutterW
		}
		if boxLeft+35 > editorWidth {
			boxLeft = editorWidth - 35
			if boxLeft < 0 {
				boxLeft = 0
			}
		}
		editorLines = OverlayBoxOnLines(editorLines, compBox, boxTop, boxLeft, editorWidth)
	} else if a.FSM.State() == fsm.StateCodeAction && len(a.CodeActions) > 0 {
		actionBox := RenderCodeActionBox(a.CodeActions, a.CodeActionIndex, editorWidth, a.Theme)
		cursorRow := currBuf.CursorPos.Line - currBuf.ScrollY
		if cursorRow < 0 {
			cursorRow = 0
		}
		boxTop := cursorRow + 1
		if boxTop+8 >= availHeight {
			boxTop = cursorRow - 8
			if boxTop < 0 {
				boxTop = 0
			}
		}
		editorLines = OverlayBoxOnLines(editorLines, actionBox, boxTop, 6, editorWidth)
	}

	var middleLines []string
	if treeOpen {
		isTreeFocused := (a.FSM.State() == fsm.StateTree)
		savedTreeWidth := a.FileTree.Width
		a.FileTree.Width = treeWidth
		treeLines := a.FileTree.Render(availHeight, a.Theme, isTreeFocused)
		a.FileTree.Width = savedTreeWidth
		for len(treeLines) < availHeight {
			treeLines = append(treeLines, strings.Repeat(" ", treeWidth-1)+a.Theme.BorderLine.Render("│"))
		}
		if len(treeLines) > availHeight {
			treeLines = treeLines[:availHeight]
		}
		middleLines = make([]string, availHeight)
		for i := 0; i < availHeight; i++ {
			middleLines[i] = treeLines[i] + editorLines[i]
		}
	} else {
		middleLines = editorLines
	}

	var allLines []string
	if bufferlineHeight > 0 {
		allLines = append(allLines, bufferline)
	}
	allLines = append(allLines, middleLines...)
	allLines = append(allLines, statusline)

	if a.Picker != nil && a.Picker.Open {
		modalLines := a.Picker.Render(a.Width, a.Height, a.Theme)
		if len(modalLines) > 0 {
			allLines = overlayModal(allLines, modalLines, a.Width, a.Height)
		}
	} else if a.HelpModal != nil && a.HelpModal.Open {
		modalLines := a.HelpModal.Render(a.Width, a.Height, a.Theme)
		if len(modalLines) > 0 {
			allLines = overlayModal(allLines, modalLines, a.Width, a.Height)
		}
	}

	if a.Width > 0 {
		for i, line := range allLines {
			if strings.Contains(line, "\x1b_G") {
				continue
			}
			if ansi.StringWidth(line) > a.Width || strings.ContainsAny(line, "\t\r\uFE0E\uFE0F") {
				allLines[i] = clampAndPadLine(sanitizeDrawerLine(line, 4), a.Width)
			}
		}
	}

	return strings.Join(allLines, "\n")
}

func overlayModal(bgLines []string, modalLines []string, termWidth, termHeight int) []string {
	if len(modalLines) == 0 || len(bgLines) == 0 {
		return bgLines
	}

	modalHeight := len(modalLines)
	topY := (termHeight - modalHeight) / 2
	if topY < 0 {
		topY = 0
	}

	modalWidth := ansi.StringWidth(modalLines[0])
	leftX := (termWidth - modalWidth) / 2
	if leftX < 0 {
		leftX = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for r := 0; r < modalHeight; r++ {
		y := topY + r
		if y >= len(result) {
			break
		}

		bgLine := result[y]
		leftPart := ansi.Cut(bgLine, 0, leftX)
		leftW := ansi.StringWidth(leftPart)
		if leftW < leftX {
			leftPart += strings.Repeat(" ", leftX-leftW)
		}

		rightX := leftX + modalWidth
		rightPart := ansi.Cut(bgLine, rightX, termWidth)
		expectedRightW := termWidth - rightX
		if expectedRightW < 0 {
			expectedRightW = 0
		}
		rightW := ansi.StringWidth(rightPart)
		if rightW < expectedRightW {
			rightPart += strings.Repeat(" ", expectedRightW-rightW)
		}

		result[y] = leftPart + modalLines[r] + rightPart
	}

	return result
}

// CurrentBuffer returns the currently focused BufferItem.
func (a *App) CurrentBuffer() *BufferItem {
	if len(a.Buffers) == 0 {
		return nil
	}
	if a.ActiveBuf < 0 || a.ActiveBuf >= len(a.Buffers) {
		a.ActiveBuf = 0
	}
	return a.Buffers[a.ActiveBuf]
}

// getAvailHeight returns the middle workspace height between bufferline and statusline.
func (a *App) getAvailHeight() int {
	h := a.Height - 1 // statusline
	if a.Config.Bufferline.Show {
		h--
	}
	if h < 1 {
		return 1
	}
	return h
}

// getDrawerContentHeight returns the rendered content row count of the drawer.
func (a *App) getDrawerContentHeight() int {
	availHeight := a.getAvailHeight()
	if a.Drawer == nil {
		return 8
	}
	if a.Drawer.Maximized {
		drawerContentHeight := availHeight - 1
		if drawerContentHeight < 1 {
			drawerContentHeight = 1
		}
		return drawerContentHeight
	}
	drawerContentHeight := a.Drawer.Height
	if drawerContentHeight <= 0 {
		drawerContentHeight = a.Height * 35 / 100
	}
	maxDrawerH := availHeight - 3
	if maxDrawerH < 1 {
		maxDrawerH = 1
	}
	if drawerContentHeight > maxDrawerH {
		drawerContentHeight = maxDrawerH
	}
	if drawerContentHeight < 2 {
		drawerContentHeight = 2
	}
	return drawerContentHeight
}

// getViewportHeight returns the vertical lines allocated for the code viewport.
func (a *App) getViewportHeight() int {
	h := a.getAvailHeight()
	if a.Drawer != nil && a.Drawer.Open {
		if a.Drawer.Maximized {
			return 1
		}
		drawerH := a.getDrawerContentHeight()
		h -= (drawerH + 1)
	}
	if h < 1 {
		return 1
	}
	return h
}

// getViewportWidth returns the horizontal columns allocated for the code viewport.
func (a *App) getViewportWidth() int {
	w := a.Width
	if a.FileTree != nil && a.FileTree.Open {
		treeWidth := a.FileTree.Width
		if a.Width > 25 && treeWidth >= a.Width-15 {
			treeWidth = a.Width - 15
		}
		if treeWidth < 10 {
			treeWidth = 10
		}
		w -= treeWidth
	}
	if w < 10 {
		return 10
	}
	return w
}

func (a *App) refreshActiveDrawer() {
	if a.Drawer == nil || !a.Drawer.Open {
		return
	}
	switch a.Drawer.Mode {
	case DrawerModeMarkdown:
		a.refreshMarkdownPreview()
	case DrawerModeDiffReview:
		a.refreshDiffReview()
	case DrawerModeSearchReplace:
		a.refreshSearchReplace()
	case DrawerModeDebug:
		a.refreshDebugDrawer()
	}
}

func (a *App) executeAction(act fsm.Action) tea.Cmd {
	// 0. LSP Modal Selection Actions
	switch act.Type {
	case fsm.ActionLSPSelectNext:
		if len(a.CompletionItems) > 0 {
			a.CompletionIndex = (a.CompletionIndex + 1) % len(a.CompletionItems)
		} else if len(a.CodeActions) > 0 {
			a.CodeActionIndex = (a.CodeActionIndex + 1) % len(a.CodeActions)
		}
		return nil

	case fsm.ActionLSPSelectPrev:
		if len(a.CompletionItems) > 0 {
			a.CompletionIndex = (a.CompletionIndex - 1 + len(a.CompletionItems)) % len(a.CompletionItems)
		} else if len(a.CodeActions) > 0 {
			a.CodeActionIndex = (a.CodeActionIndex - 1 + len(a.CodeActions)) % len(a.CodeActions)
		}
		return nil

	case fsm.ActionLSPSelectConfirm:
		if len(a.CompletionItems) > 0 {
			item := a.CompletionItems[a.CompletionIndex]
			a.applyCompletionItem(item)
			a.CompletionItems = nil
			return nil
		}
		if len(a.CodeActions) > 0 {
			idx := a.CodeActionIndex
			if act.Int >= 0 && act.Int < len(a.CodeActions) {
				idx = act.Int
			}
			a.applyCodeAction(a.CodeActions[idx])
			a.CodeActions = nil
			return nil
		}
		if a.RenameActive {
			a.RenameActive = false
			return a.requestRenameCmd()
		}
		if a.treePromptMode != "" {
			mode := a.treePromptMode
			name := strings.TrimSpace(a.treePromptInput)
			a.treePromptMode = ""
			a.treePromptInput = ""
			a.FSM.SetTreeEditable(false)
			if name == "" || a.FileTree == nil {
				return nil
			}
			if mode == "dir" {
				createdPath, err := a.FileTree.CreateDir(name)
				if err != nil {
					a.StatusMessage = fmt.Sprintf("Create folder error: %v", err)
				} else {
					a.StatusMessage = fmt.Sprintf("Created folder %s", filepath.Base(createdPath))
				}
				return nil
			}
			createdPath, err := a.FileTree.CreateFile(name)
			if err != nil {
				a.StatusMessage = fmt.Sprintf("Create file error: %v", err)
			} else {
				a.openOrSwitchToFile(createdPath)
				a.FSM.SetState(fsm.StateNormal)
				a.StatusMessage = fmt.Sprintf("Created file %s", filepath.Base(createdPath))
			}
			return nil
		}
		return nil

	case fsm.ActionLSPCancel:
		a.CompletionItems = nil
		a.CodeActions = nil
		a.HoverContent = ""
		a.RenameActive = false
		if a.treePromptMode != "" {
			a.treePromptMode = ""
			a.treePromptInput = ""
			a.FSM.SetTreeEditable(false)
		}
		return nil
	}

	if a.RenameActive {
		switch act.Type {
		case fsm.ActionInsertChar:
			a.RenameInput += act.String
			return nil
		case fsm.ActionBackspace:
			runes := []rune(a.RenameInput)
			if len(runes) > 0 {
				a.RenameInput = string(runes[:len(runes)-1])
			}
			return nil
		}
	}

	if a.treePromptMode != "" {
		switch act.Type {
		case fsm.ActionInsertChar:
			a.treePromptInput += act.String
			return nil
		case fsm.ActionBackspace:
			runes := []rune(a.treePromptInput)
			if len(runes) > 0 {
				a.treePromptInput = string(runes[:len(runes)-1])
			}
			return nil
		}
	}

	// 1. Drawer-specific key navigation
	if a.Drawer != nil && a.Drawer.Open && a.FSM.State() == fsm.StateDrawer {
		switch act.Type {
		case fsm.ActionInsertChar:
			if a.Drawer.Mode == DrawerModeSearchReplace {
				a.handleSearchReplaceInput(act.String)
				return nil
			}
		case fsm.ActionBackspace:
			if a.Drawer.Mode == DrawerModeSearchReplace {
				a.handleSearchReplaceBackspace()
				return nil
			}
		case fsm.ActionReplaceNextField:
			a.cycleReplaceField(1)
			return nil
		case fsm.ActionReplacePrevField:
			a.cycleReplaceField(-1)
			return nil
		case fsm.ActionReplaceToggleCase:
			a.toggleSearchCase()
			return nil
		case fsm.ActionReplaceToggleWord:
			a.toggleSearchWord()
			return nil
		case fsm.ActionReplaceToggleRegex:
			a.toggleSearchRegex()
			return nil
		case fsm.ActionReplaceApplyAll:
			a.applyReplaceAll()
			return nil
		case fsm.ActionReplaceApplyCurrent:
			a.applyReplaceCurrent()
			return nil
		case fsm.ActionMoveDown:
			a.Drawer.MoveCursor(1, a.getDrawerContentHeight())
			return nil
		case fsm.ActionMoveUp:
			a.Drawer.MoveCursor(-1, a.getDrawerContentHeight())
			return nil
		case fsm.ActionPageDown:
			drawerH := a.getDrawerContentHeight()
			pageSize := drawerH / 2
			if pageSize < 1 {
				pageSize = 1
			}
			a.Drawer.MoveCursor(pageSize, drawerH)
			return nil
		case fsm.ActionPageUp:
			drawerH := a.getDrawerContentHeight()
			pageSize := drawerH / 2
			if pageSize < 1 {
				pageSize = 1
			}
			a.Drawer.MoveCursor(-pageSize, drawerH)
			return nil
		case fsm.ActionMoveBufferEnd:
			a.Drawer.CursorToEnd(a.getDrawerContentHeight())
			return nil
		case fsm.ActionMoveBufferStart:
			a.Drawer.CursorToStart()
			return nil
		case fsm.ActionDrawerMaximize:
			a.Drawer.ToggleMaximize()
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
			return nil
		case fsm.ActionDrawerClose:
			a.Drawer.Close()
			a.FSM.SetState(fsm.StateNormal)
			a.FSM.SetDrawerEditable(false)
			return nil
		case fsm.ActionStepForward:
			if a.Drawer.Mode == DrawerModeDiffReview && a.AgentTx != nil {
				a.AgentTx.SelectNext()
				a.refreshDiffReview()
				return nil
			}
		case fsm.ActionStepBackward:
			if a.Drawer.Mode == DrawerModeDiffReview && a.AgentTx != nil {
				a.AgentTx.SelectPrev()
				a.refreshDiffReview()
				return nil
			}
		case fsm.ActionDrawerJump:
			if a.Drawer.Mode == DrawerModeSearchReplace {
				a.applyReplaceAll()
				return nil
			}
			a.jumpToCurrentDrawerDiagnostic()
			return nil
		}
	}

	// 2. Search & Replace Actions (Global / Prompt)
	switch act.Type {
	case fsm.ActionSearchOpen:
		a.openSearchPrompt()
		return nil
	case fsm.ActionSearchInput:
		a.handleSearchInput(act.String)
		return nil
	case fsm.ActionSearchBackspace:
		a.handleSearchBackspace()
		return nil
	case fsm.ActionSearchConfirm:
		a.confirmSearch()
		return nil
	case fsm.ActionSearchCancel:
		a.cancelSearch()
		return nil
	case fsm.ActionSearchNext:
		a.jumpToNextSearchMatch()
		return nil
	case fsm.ActionSearchPrev:
		a.jumpToPrevSearchMatch()
		return nil
	case fsm.ActionSearchClearHighlight:
		a.clearSearchHighlight()
		return nil
	case fsm.ActionReplaceOpen:
		a.openSearchReplace(false)
		return nil
	case fsm.ActionProjectReplaceOpen:
		a.openSearchReplace(true)
		return nil
	case fsm.ActionReplaceApplyAll:
		a.applyReplaceAll()
		return nil
	case fsm.ActionReplaceApplyCurrent:
		a.applyReplaceCurrent()
		return nil
	case fsm.ActionReplaceToggleCase:
		a.toggleSearchCase()
		return nil
	case fsm.ActionReplaceToggleWord:
		a.toggleSearchWord()
		return nil
	case fsm.ActionReplaceToggleRegex:
		a.toggleSearchRegex()
		return nil
	case fsm.ActionReplaceNextField:
		a.cycleReplaceField(1)
		return nil
	case fsm.ActionReplacePrevField:
		a.cycleReplaceField(-1)
		return nil
	}

	buf := a.CurrentBuffer()
	if buf == nil {
		return nil
	}

	if a.FSM.State() == fsm.StateSelect && buf.SelectionAnchor == nil {
		anchor := buf.CursorPos
		buf.SelectionAnchor = &anchor
	}

	switch act.Type {
	case fsm.ActionMoveLeft:
		if buf.CursorPos.Col > 0 {
			buf.CursorPos.Col--
		}
	case fsm.ActionMoveRight:
		lineLen := a.getLineLength(buf, buf.CursorPos.Line)
		if buf.CursorPos.Col < lineLen {
			buf.CursorPos.Col++
		}
	case fsm.ActionMoveUp:
		if buf.CursorPos.Line > 0 {
			buf.CursorPos.Line--
			lineLen := a.getLineLength(buf, buf.CursorPos.Line)
			if buf.CursorPos.Col > lineLen {
				buf.CursorPos.Col = lineLen
			}
		}
	case fsm.ActionMoveDown:
		totalLines := buf.PT.LineCount()
		if buf.CursorPos.Line < totalLines-1 {
			buf.CursorPos.Line++
			lineLen := a.getLineLength(buf, buf.CursorPos.Line)
			if buf.CursorPos.Col > lineLen {
				buf.CursorPos.Col = lineLen
			}
		}

	case fsm.ActionMoveLineStart:
		buf.CursorPos.Col = 0

	case fsm.ActionMoveLineEnd:
		buf.CursorPos.Col = a.getLineLength(buf, buf.CursorPos.Line)

	case fsm.ActionMoveWordForward:
		lineText, _ := buf.PT.Line(buf.CursorPos.Line)
		runes := []rune(lineText)
		end := buf.CursorPos.Col
		for end < len(runes) && isWordChar(runes[end]) {
			end++
		}
		for end < len(runes) && !isWordChar(runes[end]) {
			end++
		}
		buf.CursorPos.Col = end

	case fsm.ActionMoveWordBackward:
		lineText, _ := buf.PT.Line(buf.CursorPos.Line)
		runes := []rune(lineText)
		start := buf.CursorPos.Col
		if start > len(runes) {
			start = len(runes)
		}
		for start > 0 && !isWordChar(runes[start-1]) {
			start--
		}
		for start > 0 && isWordChar(runes[start-1]) {
			start--
		}
		buf.CursorPos.Col = start

	case fsm.ActionPageUp:
		vpHeight := a.getViewportHeight()
		pageSize := vpHeight - 2
		if pageSize < 1 {
			pageSize = 1
		}
		buf.CursorPos.Line -= pageSize
		if buf.CursorPos.Line < 0 {
			buf.CursorPos.Line = 0
		}
		buf.ScrollY -= pageSize
		if buf.ScrollY < 0 {
			buf.ScrollY = 0
		}
		lineLen := a.getLineLength(buf, buf.CursorPos.Line)
		if buf.CursorPos.Col > lineLen {
			buf.CursorPos.Col = lineLen
		}

	case fsm.ActionPageDown:
		vpHeight := a.getViewportHeight()
		pageSize := vpHeight - 2
		if pageSize < 1 {
			pageSize = 1
		}
		totalLines := buf.PT.LineCount()
		buf.CursorPos.Line += pageSize
		if buf.CursorPos.Line >= totalLines {
			buf.CursorPos.Line = totalLines - 1
		}
		if buf.CursorPos.Line < 0 {
			buf.CursorPos.Line = 0
		}
		buf.ScrollY += pageSize
		maxScrollY := totalLines - vpHeight
		if maxScrollY < 0 {
			maxScrollY = 0
		}
		if buf.ScrollY > maxScrollY {
			buf.ScrollY = maxScrollY
		}
		if buf.ScrollY < 0 {
			buf.ScrollY = 0
		}
		lineLen := a.getLineLength(buf, buf.CursorPos.Line)
		if buf.CursorPos.Col > lineLen {
			buf.CursorPos.Col = lineLen
		}

	case fsm.ActionMoveBufferStart:
		buf.CursorPos.Line = 0
		buf.CursorPos.Col = 0
		buf.ScrollY = 0
		buf.ScrollX = 0

	case fsm.ActionMoveBufferEnd:
		totalLines := buf.PT.LineCount()
		if totalLines > 0 {
			buf.CursorPos.Line = totalLines - 1
		} else {
			buf.CursorPos.Line = 0
		}
		buf.CursorPos.Col = a.getLineLength(buf, buf.CursorPos.Line)
		vpHeight := a.getViewportHeight()
		maxScrollY := totalLines - vpHeight
		if maxScrollY < 0 {
			maxScrollY = 0
		}
		buf.ScrollY = maxScrollY

	case fsm.ActionExtendSelection:
		switch act.String {
		case "left":
			if buf.CursorPos.Col > 0 {
				buf.CursorPos.Col--
			}
		case "right":
			lineLen := a.getLineLength(buf, buf.CursorPos.Line)
			if buf.CursorPos.Col < lineLen {
				buf.CursorPos.Col++
			}
		case "up":
			if buf.CursorPos.Line > 0 {
				buf.CursorPos.Line--
				lineLen := a.getLineLength(buf, buf.CursorPos.Line)
				if buf.CursorPos.Col > lineLen {
					buf.CursorPos.Col = lineLen
				}
			}
		case "down":
			totalLines := buf.PT.LineCount()
			if buf.CursorPos.Line < totalLines-1 {
				buf.CursorPos.Line++
				lineLen := a.getLineLength(buf, buf.CursorPos.Line)
				if buf.CursorPos.Col > lineLen {
					buf.CursorPos.Col = lineLen
				}
			}
		}

	case fsm.ActionSelectWord:
		if buf.SelectionAnchor == nil {
			anchor := buf.CursorPos
			buf.SelectionAnchor = &anchor
		}
		lineText, _ := buf.PT.Line(buf.CursorPos.Line)
		runes := []rune(lineText)
		// Find word end
		end := buf.CursorPos.Col
		for end < len(runes) && isWordChar(runes[end]) {
			end++
		}
		if end == buf.CursorPos.Col && end < len(runes) {
			end++
		}
		buf.CursorPos.Col = end

	case fsm.ActionSelectWordBackward:
		if buf.SelectionAnchor == nil {
			anchor := buf.CursorPos
			buf.SelectionAnchor = &anchor
		}
		lineText, _ := buf.PT.Line(buf.CursorPos.Line)
		runes := []rune(lineText)
		start := buf.CursorPos.Col
		if start > len(runes) {
			start = len(runes)
		}
		for start > 0 && !isWordChar(runes[start-1]) {
			start--
		}
		for start > 0 && isWordChar(runes[start-1]) {
			start--
		}
		buf.CursorPos.Col = start

	case fsm.ActionSelectLine:
		buf.SelectionAnchor = &buffer.Position{Line: buf.CursorPos.Line, Col: 0}
		lineLen := a.getLineLength(buf, buf.CursorPos.Line)
		buf.CursorPos.Col = lineLen

	case fsm.ActionCollapseSelection:
		buf.SelectionAnchor = nil

	case fsm.ActionSelectTextObject:
		var tree *sitter.Tree
		if buf.Highlighter != nil {
			tree = buf.Highlighter.Tree()
		}
		rng, ok := treesitter.FindTextObject(tree, buf.PT, buf.CursorPos, act.Arg1, act.Arg2)
		if ok {
			anchor := rng.Start
			buf.SelectionAnchor = &anchor
			buf.CursorPos = rng.End
			a.FSM.SetState(fsm.StateSelect)
		} else {
			a.FSM.SetState(fsm.StateNormal)
			a.StatusMessage = fmt.Sprintf("No enclosing %s text object found", act.Arg2)
		}

	case fsm.ActionInsertChar:
		offset, err := buf.PT.PositionToOffset(buf.CursorPos)
		if err == nil {
			_ = buf.PT.Insert(offset, act.String)
			buf.CursorPos.Col += utf8.RuneCountInString(act.String)
			buf.Dirty = true
		}

	case fsm.ActionInsertNewline:
		offset, err := buf.PT.PositionToOffset(buf.CursorPos)
		if err == nil {
			_ = buf.PT.Insert(offset, "\n")
			buf.CursorPos.Line++
			buf.CursorPos.Col = 0
			buf.Dirty = true
		}

	case fsm.ActionInsertTab:
		offset, err := buf.PT.PositionToOffset(buf.CursorPos)
		if err == nil {
			spaces := strings.Repeat(" ", a.Config.Editor.TabWidth)
			_ = buf.PT.Insert(offset, spaces)
			buf.CursorPos.Col += a.Config.Editor.TabWidth
			buf.Dirty = true
		}

	case fsm.ActionBackspace:
		if buf.CursorPos.Col > 0 {
			// Delete 1 character before cursor
			prevPos := buffer.Position{Line: buf.CursorPos.Line, Col: buf.CursorPos.Col - 1}
			offStart, err1 := buf.PT.PositionToOffset(prevPos)
			offEnd, err2 := buf.PT.PositionToOffset(buf.CursorPos)
			if err1 == nil && err2 == nil && offEnd > offStart {
				_ = buf.PT.Delete(offStart, offEnd-offStart)
				buf.CursorPos.Col--
				buf.Dirty = true
			}
		} else if buf.CursorPos.Line > 0 {
			// Merge with previous line
			prevLineIdx := buf.CursorPos.Line - 1
			prevLineLen := a.getLineLength(buf, prevLineIdx)
			off, err := buf.PT.PositionToOffset(buffer.Position{Line: buf.CursorPos.Line, Col: 0})
			if err == nil && off > 0 {
				_ = buf.PT.Delete(off-1, 1) // Delete the newline
				buf.CursorPos.Line = prevLineIdx
				buf.CursorPos.Col = prevLineLen
				buf.Dirty = true
			}
		}

	case fsm.ActionDeleteChar:
		offset, err := buf.PT.PositionToOffset(buf.CursorPos)
		if err == nil && offset < buf.PT.Len() {
			substr, subErr := buf.PT.Substr(offset, min(4, buf.PT.Len()-offset))
			if subErr == nil && len(substr) > 0 {
				_, size := utf8.DecodeRuneInString(substr)
				if size > 0 {
					_ = buf.PT.Delete(offset, size)
					buf.Dirty = true
					lineLen := a.getLineLength(buf, buf.CursorPos.Line)
					if buf.CursorPos.Col > lineLen {
						buf.CursorPos.Col = max(0, lineLen)
					}
				}
			}
		}

	case fsm.ActionDeleteSelection, fsm.ActionChangeSelection:
		if buf.SelectionAnchor != nil {
			start := *buf.SelectionAnchor
			end := buf.CursorPos
			if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
				start, end = end, start
			}
			offStart, err1 := buf.PT.PositionToOffset(start)
			offEnd, err2 := buf.PT.PositionToOffset(end)
			if err1 == nil && err2 == nil && offEnd > offStart {
				_ = buf.PT.Delete(offStart, offEnd-offStart)
				buf.CursorPos = start
				buf.SelectionAnchor = nil
				buf.Dirty = true
			}
		}

	case fsm.ActionUndo:
		if buf.PT.Undo() {
			buf.Dirty = true
			a.clampCursorToBounds(buf)
			a.reparseBuffer(buf)
			a.notifyLSPBufferChanged(buf)
		}
	case fsm.ActionRedo:
		if buf.PT.Redo() {
			buf.Dirty = true
			a.clampCursorToBounds(buf)
			a.reparseBuffer(buf)
			a.notifyLSPBufferChanged(buf)
		}

	case fsm.ActionYank:
		if buf.SelectionAnchor != nil {
			start := *buf.SelectionAnchor
			end := buf.CursorPos
			if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
				start, end = end, start
			}
			offStart, err1 := buf.PT.PositionToOffset(start)
			offEnd, err2 := buf.PT.PositionToOffset(end)
			if err1 == nil && err2 == nil && offEnd > offStart {
				str, err := buf.PT.Substr(offStart, offEnd-offStart)
				if err == nil {
					a.YankRegister = str
					a.StatusMessage = fmt.Sprintf("Yanked %d character(s)", utf8.RuneCountInString(str))
				}
			}
			buf.SelectionAnchor = nil
			a.FSM.SetState(fsm.StateNormal)
		} else {
			lineText, err := buf.PT.Line(buf.CursorPos.Line)
			if err == nil {
				a.YankRegister = lineText + "\n"
				a.StatusMessage = fmt.Sprintf("Yanked line %d", buf.CursorPos.Line+1)
			}
		}

	case fsm.ActionPasteAfter, fsm.ActionPasteBefore:
		if len(a.YankRegister) == 0 {
			a.StatusMessage = "Clipboard register is empty"
			return nil
		}
		targetPos := buf.CursorPos
		if act.Type == fsm.ActionPasteAfter {
			if strings.HasSuffix(a.YankRegister, "\n") {
				targetPos = buffer.Position{Line: buf.CursorPos.Line + 1, Col: 0}
				if targetPos.Line > buf.PT.LineCount() {
					targetPos.Line = buf.PT.LineCount()
				}
			} else {
				targetPos.Col++
			}
		} else {
			if strings.HasSuffix(a.YankRegister, "\n") {
				targetPos = buffer.Position{Line: buf.CursorPos.Line, Col: 0}
			}
		}
		off, err := buf.PT.PositionToOffset(targetPos)
		if err == nil {
			buf.PT.BeginTransaction()
			_ = buf.PT.Insert(off, a.YankRegister)
			buf.PT.CommitTransaction()
			buf.Dirty = true
			a.reparseBuffer(buf)
			a.notifyLSPBufferChanged(buf)
			a.StatusMessage = fmt.Sprintf("Pasted %d character(s)", utf8.RuneCountInString(a.YankRegister))
		}

	case fsm.ActionIndent, fsm.ActionOutdent:
		startLine := buf.CursorPos.Line
		endLine := buf.CursorPos.Line
		if buf.SelectionAnchor != nil {
			startLine = min(buf.SelectionAnchor.Line, buf.CursorPos.Line)
			endLine = max(buf.SelectionAnchor.Line, buf.CursorPos.Line)
		}
		indentStr := strings.Repeat(" ", a.Config.Editor.TabWidth)
		buf.PT.BeginTransaction()
		for l := startLine; l <= endLine; l++ {
			lineOff, err := buf.PT.LineOffset(l)
			if err != nil {
				continue
			}
			if act.Type == fsm.ActionIndent {
				_ = buf.PT.Insert(lineOff, indentStr)
			} else {
				lineText, lErr := buf.PT.Line(l)
				if lErr == nil {
					spacesToRemove := 0
					for _, ch := range lineText {
						if ch == ' ' && spacesToRemove < a.Config.Editor.TabWidth {
							spacesToRemove++
						} else if ch == '\t' && spacesToRemove == 0 {
							spacesToRemove = 1
							break
						} else {
							break
						}
					}
					if spacesToRemove > 0 {
						_ = buf.PT.Delete(lineOff, spacesToRemove)
					}
				}
			}
		}
		buf.PT.CommitTransaction()
		buf.Dirty = true
		a.reparseBuffer(buf)
		a.notifyLSPBufferChanged(buf)

	case fsm.ActionNextHunk:
		if a.AgentTx != nil && a.AgentTx.FileCount() > 0 {
			a.AgentTx.SelectNext()
			if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
				a.refreshDiffReview()
			} else {
				a.openAgentReviewDrawer()
			}
		} else {
			a.StatusMessage = "No diff hunks active"
		}

	case fsm.ActionPrevHunk:
		if a.AgentTx != nil && a.AgentTx.FileCount() > 0 {
			a.AgentTx.SelectPrev()
			if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
				a.refreshDiffReview()
			} else {
				a.openAgentReviewDrawer()
			}
		} else {
			a.StatusMessage = "No diff hunks active"
		}

	case fsm.ActionStepForward:
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview && a.AgentTx != nil {
			a.AgentTx.SelectNext()
			a.refreshDiffReview()
			return nil
		}
		a.jumpToNextLSPDiagnostic()

	case fsm.ActionStepBackward:
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview && a.AgentTx != nil {
			a.AgentTx.SelectPrev()
			a.refreshDiffReview()
			return nil
		}
		a.jumpToPrevLSPDiagnostic()

	case fsm.ActionTabNext:
		if len(a.Buffers) > 1 {
			a.ActiveBuf = (a.ActiveBuf + 1) % len(a.Buffers)
			a.ensureLSPForBuffer(a.CurrentBuffer())
			if a.Drawer != nil && a.Drawer.Open {
				if a.Drawer.Mode == DrawerModeMarkdown {
					a.refreshMarkdownPreview()
				} else if a.Drawer.Mode == DrawerModeDiffReview {
					a.refreshDiffReview()
				} else if a.Drawer.Mode == DrawerModeSearchReplace {
					a.refreshSearchReplace()
				}
			}
		}
	case fsm.ActionTabPrev:
		if len(a.Buffers) > 1 {
			a.ActiveBuf = (a.ActiveBuf - 1 + len(a.Buffers)) % len(a.Buffers)
			a.ensureLSPForBuffer(a.CurrentBuffer())
			if a.Drawer != nil && a.Drawer.Open {
				if a.Drawer.Mode == DrawerModeMarkdown {
					a.refreshMarkdownPreview()
				} else if a.Drawer.Mode == DrawerModeDiffReview {
					a.refreshDiffReview()
				} else if a.Drawer.Mode == DrawerModeSearchReplace {
					a.refreshSearchReplace()
				}
			}
		}
	case fsm.ActionTabSwitch:
		targetIdx := act.Int - 1
		if targetIdx >= 0 && targetIdx < len(a.Buffers) {
			a.ActiveBuf = targetIdx
			a.ensureLSPForBuffer(a.CurrentBuffer())
			if a.Drawer != nil && a.Drawer.Open {
				if a.Drawer.Mode == DrawerModeMarkdown {
					a.refreshMarkdownPreview()
				} else if a.Drawer.Mode == DrawerModeDiffReview {
					a.refreshDiffReview()
				} else if a.Drawer.Mode == DrawerModeSearchReplace {
					a.refreshSearchReplace()
				}
			}
		}

	case fsm.ActionSave:
		_ = a.SaveActiveBuffer()

	case fsm.ActionSaveAll:
		_, _ = a.SaveAllBuffers()

	case fsm.ActionBufferClose:
		curr := a.CurrentBuffer()
		if curr != nil && curr.Dirty {
			a.StatusMessage = fmt.Sprintf("⚠ '%s' has unsaved changes. Save with Ctrl+S / Space f s, or force close with Space b D", curr.FileName)
			return nil
		}
		a.closeCurrentBuffer()

	case fsm.ActionBufferForceClose:
		a.closeCurrentBuffer()

	case fsm.ActionToggleDrawer:
		if a.Drawer != nil {
			if a.Drawer.Open && a.Drawer.Mode == DrawerModeOutput {
				a.Drawer.Close()
				a.FSM.SetState(fsm.StateNormal)
				a.FSM.SetDrawerEditable(false)
			} else {
				a.Drawer.Mode = DrawerModeOutput
				a.Drawer.Title = "Output & Diagnostics (Esc to close, Enter to jump)"
				a.Drawer.Open = true
				a.FSM.SetState(fsm.StateDrawer)
			}
		}

	case fsm.ActionToggleMarkdownPreview:
		a.toggleMarkdownPreview()

	case fsm.ActionToggleChecklist:
		a.toggleCurrentLineChecklist()

	case fsm.ActionToggleMermaidMode:
		a.toggleMermaidMode()

	case fsm.ActionAgentReviewOpen:
		a.openAgentReviewDrawer()

	case fsm.ActionAgentAcceptAll:
		a.acceptAgentTransaction()

	case fsm.ActionAgentRevertAll:
		a.revertAgentTransaction()

	// DAP Debugger Actions
	case fsm.ActionDAPToggleBreakpoint:
		curr := a.CurrentBuffer()
		if curr != nil && a.DAPManager != nil && curr.FilePath != "" {
			line1 := curr.CursorPos.Line + 1
			set := a.DAPManager.ToggleBreakpoint(curr.FilePath, line1)
			if set {
				a.StatusMessage = fmt.Sprintf("Breakpoint set: %s:%d", filepath.Base(curr.FilePath), line1)
			} else {
				a.StatusMessage = fmt.Sprintf("Breakpoint cleared: %s:%d", filepath.Base(curr.FilePath), line1)
			}
		}
		return nil

	case fsm.ActionDAPStart:
		a.startDebugging()
		return nil

	case fsm.ActionDAPContinue:
		a.continueDebugging()
		return nil

	case fsm.ActionDAPStepOver:
		if a.DAPManager != nil {
			_ = a.DAPManager.StepOver()
		}
		return nil

	case fsm.ActionDAPStepInto:
		if a.DAPManager != nil {
			_ = a.DAPManager.StepInto()
		}
		return nil

	case fsm.ActionDAPStepOut:
		if a.DAPManager != nil {
			_ = a.DAPManager.StepOut()
		}
		return nil

	case fsm.ActionDAPPause:
		if a.DAPManager != nil {
			_ = a.DAPManager.Pause()
		}
		return nil

	case fsm.ActionDAPStop:
		if a.DAPManager != nil {
			_ = a.DAPManager.Stop()
			a.StatusMessage = "Debugger stopped"
		}
		return nil

	case fsm.ActionDAPToggleView:
		if a.Drawer == nil {
			a.Drawer = NewDrawer()
		}
		if a.Drawer.Open && a.Drawer.Mode == DrawerModeDebug {
			a.Drawer.Close()
			a.FSM.SetState(fsm.StateNormal)
		} else {
			a.Drawer.Mode = DrawerModeDebug
			a.Drawer.Open = true
			a.refreshDebugDrawer()
			a.FSM.SetState(fsm.StateDrawer)
		}
		return nil

	case fsm.ActionDrawerClose:
		if a.Drawer != nil {
			a.Drawer.Close()
		}
		a.FSM.SetState(fsm.StateNormal)
		a.FSM.SetDrawerEditable(false)

	case fsm.ActionDrawerMaximize:
		if a.Drawer != nil {
			a.Drawer.ToggleMaximize()
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
		}

	case fsm.ActionResizeDrawerGrow:
		if a.Drawer != nil {
			a.Drawer.Grow(3)
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
		}

	case fsm.ActionResizeDrawerShrink:
		if a.Drawer != nil {
			a.Drawer.Shrink(3)
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
		}

	case fsm.ActionDrawerJump:
		a.jumpToCurrentDrawerDiagnostic()

	case fsm.ActionNextError:
		a.jumpToNextDiagnosticError()

	case fsm.ActionPrevError:
		a.jumpToPrevDiagnosticError()

	case fsm.ActionAlternateFile:
		a.switchAlternateFile()

	case fsm.ActionJumplistBack:
		a.jumplistBack()

	case fsm.ActionJumplistForward:
		a.jumplistForward()

	// File Tree & Fuzzy Finder
	case fsm.ActionToggleFileTree:
		if a.FileTree != nil {
			if !a.FileTree.Open {
				a.FileTree.Open = true
				a.FileTree.Refresh()
				a.FSM.SetState(fsm.StateTree)
				a.refreshMarkdownPreview()
			} else {
				if a.FSM.State() == fsm.StateTree {
					a.FileTree.Open = false
					a.FSM.SetState(fsm.StateNormal)
					a.refreshMarkdownPreview()
				} else {
					a.FSM.SetState(fsm.StateTree)
				}
			}
		}

	case fsm.ActionTreeDown:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.MoveCursor(1, a.getAvailHeight())
		}

	case fsm.ActionTreeUp:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.MoveCursor(-1, a.getAvailHeight())
		}

	case fsm.ActionTreePageDown:
		if a.FileTree != nil && a.FileTree.Open {
			availH := a.getAvailHeight()
			pageSize := availH / 2
			if pageSize < 1 {
				pageSize = 1
			}
			a.FileTree.MoveCursor(pageSize, availH)
		}

	case fsm.ActionTreePageUp:
		if a.FileTree != nil && a.FileTree.Open {
			availH := a.getAvailHeight()
			pageSize := availH / 2
			if pageSize < 1 {
				pageSize = 1
			}
			a.FileTree.MoveCursor(-pageSize, availH)
		}

	case fsm.ActionTreeTop:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.CursorToTop()
		}

	case fsm.ActionTreeBottom:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.CursorToBottom(a.getAvailHeight())
		}

	case fsm.ActionTreeToggleOrOpen:
		if a.FileTree != nil && a.FileTree.Open {
			path, isFile := a.FileTree.ToggleExpand()
			if isFile && path != "" {
				a.openOrSwitchToFile(path)
				a.FSM.SetState(fsm.StateNormal)
			}
		}

	case fsm.ActionTreeCollapseOrParent:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.CollapseOrParent()
		}

	case fsm.ActionTreeRefresh:
		if a.FileTree != nil && a.FileTree.Open {
			a.FileTree.Refresh()
			a.StatusMessage = "File tree refreshed"
		}

	case fsm.ActionTreeCreateFile:
		if a.FileTree != nil {
			if !a.FileTree.Open {
				a.FileTree.Open = true
				a.FileTree.Refresh()
				a.refreshMarkdownPreview()
			}
			a.treePromptMode = "file"
			a.treePromptInput = ""
			a.FSM.SetState(fsm.StateTree)
			a.FSM.SetTreeEditable(true)
		}

	case fsm.ActionTreeCreateDir:
		if a.FileTree != nil {
			if !a.FileTree.Open {
				a.FileTree.Open = true
				a.FileTree.Refresh()
				a.refreshMarkdownPreview()
			}
			a.treePromptMode = "dir"
			a.treePromptInput = ""
			a.FSM.SetState(fsm.StateTree)
			a.FSM.SetTreeEditable(true)
		}

	case fsm.ActionTreeDelete:
		if a.FileTree != nil && a.FileTree.Open {
			node := a.FileTree.SelectedNode()
			if node != nil {
				name := node.Name
				now := time.Now()
				if !a.treeDeletePending || a.treeDeletePath != node.Path || now.Sub(a.treeDeleteTime) > 3*time.Second {
					a.treeDeletePending = true
					a.treeDeletePath = node.Path
					a.treeDeleteTime = now
					a.StatusMessage = fmt.Sprintf("⚠ Confirm deletion: press 'd' again within 3s to permanently delete %s", name)
					return nil
				}
				a.treeDeletePending = false
				a.treeDeletePath = ""
				if err := a.FileTree.DeleteSelected(); err != nil {
					a.StatusMessage = fmt.Sprintf("Delete error: %v", err)
				} else {
					a.StatusMessage = fmt.Sprintf("Deleted %s", name)
				}
			}
		}

	case fsm.ActionPaneFocusTree:
		if a.FileTree != nil {
			if !a.FileTree.Open {
				a.FileTree.Open = true
				a.FileTree.Refresh()
				a.refreshMarkdownPreview()
			}
			a.FSM.SetState(fsm.StateTree)
		}

	case fsm.ActionPaneFocusBuffer:
		a.FSM.SetState(fsm.StateNormal)

	case fsm.ActionPaneFocusDrawer:
		if a.Drawer != nil && a.Drawer.Open {
			a.FSM.SetState(fsm.StateDrawer)
		}

	case fsm.ActionPaneCycle:
		switch a.FSM.State() {
		case fsm.StateTree:
			a.FSM.SetState(fsm.StateNormal)
		case fsm.StateNormal:
			if a.Drawer != nil && a.Drawer.Open {
				a.FSM.SetState(fsm.StateDrawer)
			} else if a.FileTree != nil && a.FileTree.Open {
				a.FSM.SetState(fsm.StateTree)
			}
		case fsm.StateDrawer:
			if a.FileTree != nil && a.FileTree.Open {
				a.FSM.SetState(fsm.StateTree)
			} else {
				a.FSM.SetState(fsm.StateNormal)
			}
		default:
			a.FSM.SetState(fsm.StateNormal)
		}

	case fsm.ActionResizeTreeGrow:
		if a.FileTree != nil {
			a.FileTree.Grow(3)
			a.refreshMarkdownPreview()
		}

	case fsm.ActionResizeTreeShrink:
		if a.FileTree != nil {
			a.FileTree.Shrink(3)
			a.refreshMarkdownPreview()
		}

	case fsm.ActionPickerOpenFiles:
		if a.Picker != nil {
			a.Picker.OpenFiles(a.WorkspacePath)
			a.FSM.SetState(fsm.StatePicker)
		}

	case fsm.ActionPickerOpenBuffers:
		if a.Picker != nil {
			a.Picker.OpenBuffers(a.Buffers, a.ActiveBuf)
			a.FSM.SetState(fsm.StatePicker)
		}

	case fsm.ActionPickerOpenGrep:
		if a.Picker != nil {
			a.Picker.OpenGrep(a.WorkspacePath)
			a.FSM.SetState(fsm.StatePicker)
		}

	case fsm.ActionPickerCancel:
		if a.Picker != nil {
			a.Picker.Close()
		}
		a.FSM.SetState(fsm.StateNormal)

	case fsm.ActionPickerSelect:
		if a.Picker != nil && a.Picker.Open {
			item := a.Picker.SelectedItem()
			if item != nil {
				if a.Picker.Mode == PickerModeGrep {
					parts := strings.SplitN(item.Value, ":", 2)
					filePath := parts[0]
					lineNum := 0
					if len(parts) > 1 {
						_, _ = fmt.Sscanf(parts[1], "%d", &lineNum)
						if lineNum > 0 {
							lineNum--
						}
					}
					a.openOrSwitchToFile(filePath)
					if targetBuf := a.CurrentBuffer(); targetBuf != nil {
						targetBuf.CursorPos.Line = lineNum
						targetBuf.CursorPos.Col = 0
					}
				} else {
					a.openOrSwitchToFile(item.Value)
				}
			}
			a.Picker.Close()
		}
		a.FSM.SetState(fsm.StateNormal)

	case fsm.ActionPickerNext:
		if a.Picker != nil && a.Picker.Open {
			a.Picker.MoveCursor(1, 10)
		}

	case fsm.ActionPickerPrev:
		if a.Picker != nil && a.Picker.Open {
			a.Picker.MoveCursor(-1, 10)
		}

	case fsm.ActionPickerInput:
		if a.Picker != nil && a.Picker.Open {
			a.Picker.InsertChar(act.String)
		}

	case fsm.ActionPickerBackspace:
		if a.Picker != nil && a.Picker.Open {
			a.Picker.DeleteChar()
		}

	case fsm.ActionHelpToggle:
		if a.HelpModal != nil {
			if a.HelpModal.Open {
				a.HelpModal.Close()
				a.FSM.SetState(fsm.StateNormal)
			} else {
				a.HelpModal.OpenModal()
				a.FSM.SetState(fsm.StateHelp)
			}
		}

	case fsm.ActionHelpOpen:
		if a.HelpModal != nil {
			a.HelpModal.OpenModal()
			a.FSM.SetState(fsm.StateHelp)
		}

	case fsm.ActionHelpCancel:
		if a.HelpModal != nil {
			a.HelpModal.Close()
		}
		a.FSM.SetState(fsm.StateNormal)

	case fsm.ActionHelpNext:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.Next()
		}

	case fsm.ActionHelpPrev:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.Prev()
		}

	case fsm.ActionHelpPageDown:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.PageDown(10)
		}

	case fsm.ActionHelpPageUp:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.PageUp(10)
		}

	case fsm.ActionHelpInput:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.InsertChar(act.String)
		}

	case fsm.ActionHelpBackspace:
		if a.HelpModal != nil && a.HelpModal.Open {
			a.HelpModal.DeleteChar()
		}

	case fsm.ActionHelpSelect:
		if a.HelpModal != nil && a.HelpModal.Open {
			item := a.HelpModal.SelectedItem()
			a.HelpModal.Close()
			a.FSM.SetState(fsm.StateNormal)
			if item != nil && item.Action != fsm.ActionNone && item.Action != fsm.ActionHelpToggle && item.Action != fsm.ActionHelpOpen {
				return a.executeAction(fsm.Action{Type: item.Action})
			}
		}

	case fsm.ActionNextDiagnostic:
		a.jumpToNextLSPDiagnostic()

	case fsm.ActionPrevDiagnostic:
		a.jumpToPrevLSPDiagnostic()

	case fsm.ActionLSPDefinition:
		return a.requestDefinitionCmd()

	case fsm.ActionLSPImplementation:
		return a.requestImplementationCmd()

	case fsm.ActionLSPHover:
		return a.requestHoverCmd()

	case fsm.ActionLSPComplete:
		return a.requestCompletionCmd()

	case fsm.ActionLSPCodeAction:
		return a.requestCodeActionCmd()

	case fsm.ActionLSPRename:
		curr := a.CurrentBuffer()
		if curr != nil {
			word := a.getWordAtCursor(curr)
			a.RenameOriginal = word
			a.RenameInput = word
			lineText, _ := curr.PT.Line(curr.CursorPos.Line)
			utf16Col := lsp.RuneColToUTF16Col(lineText, curr.CursorPos.Col)
			a.RenamePos = lsp.Position{Line: curr.CursorPos.Line, Character: utf16Col}
			a.RenameActive = true
			a.FSM.SetState(fsm.StateRename)
			a.StatusMessage = fmt.Sprintf("Rename '%s' to: ", word)
		}
	}

	if buf.Dirty {
		a.reparseBuffer(buf)
		a.notifyLSPBufferChanged(buf)
		if a.Drawer != nil && a.Drawer.Open {
			if a.Drawer.Mode == DrawerModeMarkdown {
				a.refreshMarkdownPreview()
			} else if a.Drawer.Mode == DrawerModeDiffReview {
				a.refreshDiffReview()
			} else if a.Drawer.Mode == DrawerModeSearchReplace {
				a.refreshSearchReplace()
			}
		}
	}

	return nil
}

// LSP Command Helpers

func (a *App) requestDefinitionCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		a.StatusMessage = "No active buffer for LSP"
		return nil
	}
	filePath := currBuf.FilePath
	line := currBuf.CursorPos.Line
	lineText, _ := currBuf.PT.Line(line)
	col := lsp.RuneColToUTF16Col(lineText, currBuf.CursorPos.Col)
	uri := lsp.PathToURI(filePath)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPDefinitionMsg{Err: fmt.Errorf("no LSP server available: %v", err)}
		}
		locs, err := client.GotoDefinition(ctx, uri, line, col)
		return LSPDefinitionMsg{Locations: locs, Err: err}
	}
}

func (a *App) requestImplementationCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		a.StatusMessage = "No active buffer for LSP"
		return nil
	}
	filePath := currBuf.FilePath
	line := currBuf.CursorPos.Line
	lineText, _ := currBuf.PT.Line(line)
	col := lsp.RuneColToUTF16Col(lineText, currBuf.CursorPos.Col)
	uri := lsp.PathToURI(filePath)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPImplementationMsg{Err: fmt.Errorf("no LSP server available: %v", err)}
		}
		locs, err := client.GotoImplementation(ctx, uri, line, col)
		return LSPImplementationMsg{Locations: locs, Err: err}
	}
}

func (a *App) requestHoverCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		a.StatusMessage = "No active buffer for LSP"
		return nil
	}
	filePath := currBuf.FilePath
	line := currBuf.CursorPos.Line
	lineText, _ := currBuf.PT.Line(line)
	col := lsp.RuneColToUTF16Col(lineText, currBuf.CursorPos.Col)
	uri := lsp.PathToURI(filePath)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPHoverMsg{Err: fmt.Errorf("no LSP server available")}
		}
		hover, err := client.Hover(ctx, uri, line, col)
		return LSPHoverMsg{Hover: hover, Err: err}
	}
}

func (a *App) requestCompletionCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		return nil
	}
	filePath := currBuf.FilePath
	line := currBuf.CursorPos.Line
	lineText, _ := currBuf.PT.Line(line)
	col := lsp.RuneColToUTF16Col(lineText, currBuf.CursorPos.Col)
	uri := lsp.PathToURI(filePath)
	prefix := a.getCurrentWordPrefix(currBuf)

	// Flush any pending debounced edits immediately so LSP server has current buffer text
	a.flushLSPBufferChanged(currBuf)

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPCompletionMsg{Err: fmt.Errorf("no LSP server available: %v", err)}
		}
		items, err := client.Completion(ctx, uri, line, col)
		return LSPCompletionMsg{Items: items, Prefix: prefix, Err: err}
	}
}

func (a *App) requestCodeActionCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		a.StatusMessage = "No active buffer for LSP"
		return nil
	}
	filePath := currBuf.FilePath
	line := currBuf.CursorPos.Line
	lineText, _ := currBuf.PT.Line(line)
	col := lsp.RuneColToUTF16Col(lineText, currBuf.CursorPos.Col)
	uri := lsp.PathToURI(filePath)

	allDiags := a.LSPManager.GetDiagnostics(filePath)
	var lineDiags []lsp.Diagnostic
	for _, d := range allDiags {
		if d.Range.Start.Line <= line && line <= d.Range.End.Line {
			lineDiags = append(lineDiags, d)
		}
	}

	rng := lsp.Range{
		Start: lsp.Position{Line: line, Character: col},
		End:   lsp.Position{Line: line, Character: col},
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPCodeActionMsg{Err: fmt.Errorf("no LSP server available")}
		}
		actions, err := client.CodeAction(ctx, uri, rng, lineDiags)
		return LSPCodeActionMsg{Actions: actions, Err: err}
	}
}

func (a *App) requestRenameCmd() tea.Cmd {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" || a.LSPManager == nil {
		return nil
	}
	filePath := currBuf.FilePath
	uri := lsp.PathToURI(filePath)
	newName := a.RenameInput
	pos := a.RenamePos

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		client, err := a.LSPManager.GetOrStartClient(ctx, filePath)
		if err != nil || client == nil {
			return LSPRenameMsg{Err: fmt.Errorf("no LSP server available")}
		}
		edit, err := client.Rename(ctx, uri, pos.Line, pos.Character, newName)
		return LSPRenameMsg{Edit: edit, Err: err}
	}
}

func (a *App) applyCompletionItem(item lsp.CompletionItem) {
	curr := a.CurrentBuffer()
	if curr == nil {
		return
	}
	curr.PT.BeginTransaction()
	defer curr.PT.CommitTransaction()

	insertText := item.InsertText
	if insertText == "" {
		insertText = item.Label
	}

	if item.TextEdit != nil {
		startLineText, _ := curr.PT.Line(item.TextEdit.Range.Start.Line)
		startRuneCol := lsp.UTF16ColToRuneCol(startLineText, item.TextEdit.Range.Start.Character)
		endLineText, _ := curr.PT.Line(item.TextEdit.Range.End.Line)
		endRuneCol := lsp.UTF16ColToRuneCol(endLineText, item.TextEdit.Range.End.Character)

		startOff, _ := curr.PT.PositionToOffset(buffer.Position{Line: item.TextEdit.Range.Start.Line, Col: startRuneCol})
		endOff, _ := curr.PT.PositionToOffset(buffer.Position{Line: item.TextEdit.Range.End.Line, Col: endRuneCol})
		if endOff > startOff {
			curr.PT.Delete(startOff, endOff-startOff)
		}
		curr.PT.Insert(startOff, item.TextEdit.NewText)
		curr.CursorPos, _ = curr.PT.OffsetToPosition(startOff + len(item.TextEdit.NewText))
	} else {
		if len(a.CompletionPrefix) > 0 {
			currOff, _ := curr.PT.PositionToOffset(curr.CursorPos)
			startOff := currOff - len(a.CompletionPrefix)
			if startOff >= 0 {
				curr.PT.Delete(startOff, len(a.CompletionPrefix))
				curr.PT.Insert(startOff, insertText)
				curr.CursorPos, _ = curr.PT.OffsetToPosition(startOff + len(insertText))
			}
		} else {
			currOff, _ := curr.PT.PositionToOffset(curr.CursorPos)
			curr.PT.Insert(currOff, insertText)
			curr.CursorPos, _ = curr.PT.OffsetToPosition(currOff + len(insertText))
		}
	}
	curr.Dirty = true
	a.reparseBuffer(curr)
	a.notifyLSPBufferChanged(curr)
}

func (a *App) applyCodeAction(act lsp.CodeAction) {
	if act.Edit != nil {
		a.applyWorkspaceEdit(act.Edit)
		a.StatusMessage = fmt.Sprintf("Applied action: %s", act.Title)
	}
}

func (a *App) applyWorkspaceEdit(edit *lsp.WorkspaceEdit) {
	if edit == nil {
		return
	}
	if len(edit.DocumentChanges) > 0 {
		for _, docEdit := range edit.DocumentChanges {
			if docEdit.TextDocument.URI == "" || len(docEdit.Edits) == 0 {
				continue
			}
			a.applyFileEdits(docEdit.TextDocument.URI, docEdit.Edits)
		}
		return
	}

	for uri, edits := range edit.Changes {
		if len(edits) == 0 {
			continue
		}
		a.applyFileEdits(uri, edits)
	}
}

func (a *App) applyFileEdits(uri string, edits []lsp.TextEdit) {
	fp := lsp.URIToPath(uri)
	a.openOrSwitchToFile(fp)
	buf := a.CurrentBuffer()
	if buf == nil {
		return
	}
	buf.PT.BeginTransaction()
	for i := len(edits) - 1; i >= 0; i-- {
		te := edits[i]
		startLineText, _ := buf.PT.Line(te.Range.Start.Line)
		startRuneCol := lsp.UTF16ColToRuneCol(startLineText, te.Range.Start.Character)
		endLineText, _ := buf.PT.Line(te.Range.End.Line)
		endRuneCol := lsp.UTF16ColToRuneCol(endLineText, te.Range.End.Character)

		startOffset, _ := buf.PT.PositionToOffset(buffer.Position{Line: te.Range.Start.Line, Col: startRuneCol})
		endOffset, _ := buf.PT.PositionToOffset(buffer.Position{Line: te.Range.End.Line, Col: endRuneCol})
		if endOffset > startOffset {
			buf.PT.Delete(startOffset, endOffset-startOffset)
		}
		if te.NewText != "" {
			buf.PT.Insert(startOffset, te.NewText)
		}
	}
	buf.PT.CommitTransaction()
	buf.Dirty = true
	a.reparseBuffer(buf)
	a.notifyLSPBufferChanged(buf)
}

func (a *App) jumpToNextLSPDiagnostic() {
	currBuf := a.CurrentBuffer()
	if currBuf == nil {
		return
	}
	if a.LSPManager == nil {
		a.jumpToNextDiagnosticError()
		return
	}

	diags := a.LSPManager.GetDiagnostics(currBuf.FilePath)
	if len(diags) == 0 {
		a.jumpToNextDiagnosticError()
		return
	}

	var chosen *lsp.Diagnostic
	for i := range diags {
		if diags[i].Range.Start.Line > currBuf.CursorPos.Line {
			chosen = &diags[i]
			break
		}
	}
	if chosen == nil {
		chosen = &diags[0]
	}

	currBuf.CursorPos.Line = chosen.Range.Start.Line
	lineText, _ := currBuf.PT.Line(chosen.Range.Start.Line)
	currBuf.CursorPos.Col = lsp.UTF16ColToRuneCol(lineText, chosen.Range.Start.Character)
	a.StatusMessage = fmt.Sprintf("[%s] %s", chosen.Severity, chosen.Message)
}

func (a *App) jumpToPrevLSPDiagnostic() {
	currBuf := a.CurrentBuffer()
	if currBuf == nil {
		return
	}
	if a.LSPManager == nil {
		a.jumpToPrevDiagnosticError()
		return
	}

	diags := a.LSPManager.GetDiagnostics(currBuf.FilePath)
	if len(diags) == 0 {
		a.jumpToPrevDiagnosticError()
		return
	}

	var chosen *lsp.Diagnostic
	for i := len(diags) - 1; i >= 0; i-- {
		if diags[i].Range.Start.Line < currBuf.CursorPos.Line {
			chosen = &diags[i]
			break
		}
	}
	if chosen == nil {
		chosen = &diags[len(diags)-1]
	}

	currBuf.CursorPos.Line = chosen.Range.Start.Line
	lineText, _ := currBuf.PT.Line(chosen.Range.Start.Line)
	currBuf.CursorPos.Col = lsp.UTF16ColToRuneCol(lineText, chosen.Range.Start.Character)
	a.StatusMessage = fmt.Sprintf("[%s] %s", chosen.Severity, chosen.Message)
}

func (a *App) notifyLSPBufferChanged(buf *BufferItem) {
	if a.LSPManager == nil || buf == nil || buf.FilePath == "" {
		return
	}
	fp := buf.FilePath
	text := buf.PT.Text()

	a.lspDebounceMu.Lock()
	defer a.lspDebounceMu.Unlock()

	a.lspPendingFile = fp
	a.lspPendingText = text
	a.lspPendingVer++
	ver := a.lspPendingVer

	if a.lspDebounceTimer != nil {
		a.lspDebounceTimer.Stop()
	}

	a.lspDebounceTimer = time.AfterFunc(250*time.Millisecond, func() {
		a.lspDebounceMu.Lock()
		pendingFP := a.lspPendingFile
		pendingText := a.lspPendingText
		a.lspDebounceMu.Unlock()

		if a.LSPManager != nil && pendingFP != "" {
			_ = a.LSPManager.DidChange(context.Background(), pendingFP, ver, pendingText)
		}
	})
}

func (a *App) flushLSPBufferChanged(buf *BufferItem) {
	if a.LSPManager == nil || buf == nil || buf.FilePath == "" {
		return
	}
	a.lspDebounceMu.Lock()
	if a.lspDebounceTimer != nil {
		a.lspDebounceTimer.Stop()
		a.lspDebounceTimer = nil
	}
	fp := buf.FilePath
	text := buf.PT.Text()
	a.lspPendingFile = ""
	a.lspPendingText = ""
	ver := a.lspPendingVer
	a.lspDebounceMu.Unlock()

	_ = a.LSPManager.DidChange(context.Background(), fp, ver, text)
}

func (a *App) ensureLSPForBuffer(buf *BufferItem) {
	if a.LSPManager == nil || buf == nil || buf.FilePath == "" {
		return
	}
	fp := buf.FilePath
	srvName, _, _, ok := a.LSPManager.DetectServer(fp)
	if !ok {
		return
	}

	text := buf.PT.Text()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client, err := a.LSPManager.GetOrStartClient(ctx, fp)
		if err != nil || client == nil {
			if a.Program != nil {
				a.Program.Send(LSPStatusMsg{FilePath: fp, ServerName: srvName, Status: lsp.StatusError})
			}
			return
		}

		if a.Program != nil {
			st, _ := client.Status()
			a.Program.Send(LSPStatusMsg{FilePath: fp, ServerName: client.ServerName(), Status: st})
		}

		if err := client.WaitReady(ctx); err != nil {
			if a.Program != nil {
				st, _ := client.Status()
				a.Program.Send(LSPStatusMsg{FilePath: fp, ServerName: client.ServerName(), Status: st})
			}
			return
		}

		_ = a.LSPManager.DidOpen(ctx, fp, text, 1)
		if a.Program != nil {
			st, _ := client.Status()
			a.Program.Send(LSPStatusMsg{FilePath: fp, ServerName: client.ServerName(), Status: st})
		}
	}()
}

func (a *App) getWordAtCursor(buf *BufferItem) string {
	if buf == nil {
		return ""
	}
	line, _ := buf.PT.Line(buf.CursorPos.Line)
	runes := []rune(line)
	col := buf.CursorPos.Col
	if col > len(runes) {
		col = len(runes)
	}
	if col < len(runes) && !isIdentifierRune(runes[col]) && col > 0 && isIdentifierRune(runes[col-1]) {
		col--
	}
	if col >= len(runes) || !isIdentifierRune(runes[col]) {
		return ""
	}
	start := col
	for start > 0 && isIdentifierRune(runes[start-1]) {
		start--
	}
	end := col
	for end < len(runes) && isIdentifierRune(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

func (a *App) getCurrentWordPrefix(buf *BufferItem) string {
	if buf == nil {
		return ""
	}
	line, _ := buf.PT.Line(buf.CursorPos.Line)
	runes := []rune(line)
	col := buf.CursorPos.Col
	if col > len(runes) {
		col = len(runes)
	}
	start := col
	for start > 0 && isIdentifierRune(runes[start-1]) {
		start--
	}
	return string(runes[start:col])
}

func isIdentifierRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// Search & Replace Engine Helpers

func (a *App) openSearchPrompt() {
	a.SearchActive = true
	a.SearchQuery = ""
	a.SearchMatches = nil
	a.SearchIndex = -1
	a.FSM.SetState(fsm.StateSearch)
	a.StatusMessage = "Search: Type query, Enter to confirm, Esc to cancel"
}

func (a *App) handleSearchInput(ch string) {
	a.SearchQuery += ch
	a.updateSearchMatches()
	if len(a.SearchMatches) > 0 && a.SearchIndex >= 0 {
		a.jumpToMatch(a.SearchIndex)
	}
}

func (a *App) handleSearchBackspace() {
	runes := []rune(a.SearchQuery)
	if len(runes) > 0 {
		a.SearchQuery = string(runes[:len(runes)-1])
		a.updateSearchMatches()
		if len(a.SearchMatches) > 0 && a.SearchIndex >= 0 {
			a.jumpToMatch(a.SearchIndex)
		}
	} else {
		a.SearchQuery = ""
		a.SearchMatches = nil
		a.SearchIndex = -1
	}
}

func (a *App) confirmSearch() {
	a.FSM.SetState(fsm.StateNormal)
	if a.SearchQuery != "" {
		if len(a.SearchHistory) == 0 || a.SearchHistory[len(a.SearchHistory)-1] != a.SearchQuery {
			a.SearchHistory = append(a.SearchHistory, a.SearchQuery)
		}
		a.SearchHistoryIdx = len(a.SearchHistory)
	}
	if len(a.SearchMatches) > 0 {
		if a.SearchIndex < 0 {
			a.SearchIndex = 0
		}
		a.jumpToMatch(a.SearchIndex)
		a.StatusMessage = fmt.Sprintf("[%d/%d matches] '%s'", a.SearchIndex+1, len(a.SearchMatches), a.SearchQuery)
	} else if a.SearchQuery != "" {
		a.StatusMessage = fmt.Sprintf("Pattern not found: '%s'", a.SearchQuery)
	}
}

func (a *App) cancelSearch() {
	a.FSM.SetState(fsm.StateNormal)
	a.SearchActive = false
	a.SearchQuery = ""
	a.SearchMatches = nil
	a.SearchIndex = -1
	a.StatusMessage = "Search cancelled"
}

func (a *App) jumpToNextSearchMatch() {
	if len(a.SearchMatches) == 0 {
		if a.SearchQuery != "" {
			a.updateSearchMatches()
		}
	}
	if len(a.SearchMatches) == 0 {
		a.StatusMessage = "No search matches"
		return
	}
	a.SearchActive = true
	a.SearchIndex = (a.SearchIndex + 1) % len(a.SearchMatches)
	a.jumpToMatch(a.SearchIndex)
	a.StatusMessage = fmt.Sprintf("[%d/%d matches] '%s'", a.SearchIndex+1, len(a.SearchMatches), a.SearchQuery)
}

func (a *App) jumpToPrevSearchMatch() {
	if len(a.SearchMatches) == 0 {
		if a.SearchQuery != "" {
			a.updateSearchMatches()
		}
	}
	if len(a.SearchMatches) == 0 {
		a.StatusMessage = "No search matches"
		return
	}
	a.SearchActive = true
	a.SearchIndex = (a.SearchIndex - 1 + len(a.SearchMatches)) % len(a.SearchMatches)
	a.jumpToMatch(a.SearchIndex)
	a.StatusMessage = fmt.Sprintf("[%d/%d matches] '%s'", a.SearchIndex+1, len(a.SearchMatches), a.SearchQuery)
}

func (a *App) clearSearchHighlight() {
	a.SearchActive = false
	a.SearchMatches = nil
	a.SearchIndex = -1
	a.StatusMessage = "Search highlights cleared"
}

func (a *App) jumpToMatch(idx int) {
	if idx < 0 || idx >= len(a.SearchMatches) {
		return
	}
	m := a.SearchMatches[idx]
	buf := a.CurrentBuffer()
	if buf == nil {
		return
	}
	if m.FilePath != "" && !isSameFile(m.FilePath, buf.FilePath) {
		a.openOrSwitchToFile(m.FilePath)
		buf = a.CurrentBuffer()
		if buf == nil {
			return
		}
	}
	buf.CursorPos.Line = m.Line
	buf.CursorPos.Col = m.Col
}

func (a *App) updateSearchMatches() {
	buf := a.CurrentBuffer()
	if buf == nil || a.SearchQuery == "" {
		a.SearchMatches = nil
		a.SearchIndex = -1
		return
	}
	opts := a.SearchOptions
	opts.Query = a.SearchQuery
	filePath := buf.FilePath
	matches, err := search.FindInPieceTable(buf.PT, opts, filePath)
	if err != nil {
		a.SearchMatches = nil
		a.SearchIndex = -1
		return
	}
	a.SearchMatches = matches
	if len(matches) > 0 {
		currPos := buf.CursorPos
		closestIdx := 0
		found := false
		for i, m := range matches {
			if m.Line > currPos.Line || (m.Line == currPos.Line && m.Col >= currPos.Col) {
				closestIdx = i
				found = true
				break
			}
		}
		if !found {
			closestIdx = 0
		}
		a.SearchIndex = closestIdx
	} else {
		a.SearchIndex = -1
	}
}

func (a *App) openSearchReplace(projectWide bool) {
	if a.Drawer == nil {
		a.Drawer = NewDrawer()
	}
	a.Drawer.Open = true
	a.Drawer.Mode = DrawerModeSearchReplace
	if projectWide {
		a.ReplaceScope = "project"
		a.Drawer.Title = "Project Search & Replace (Esc: Close, Enter: Replace All, Alt+y: Current)"
	} else {
		a.ReplaceScope = "buffer"
		a.Drawer.Title = "Search & Replace (Esc: Close, Enter: Replace All, Alt+y: Current)"
	}
	a.FSM.SetState(fsm.StateDrawer)
	a.FSM.SetDrawerEditable(true)
	a.ReplaceFocused = 0

	buf := a.CurrentBuffer()
	if a.SearchQuery == "" && buf != nil {
		w := a.getWordAtCursor(buf)
		if w != "" {
			a.SearchQuery = w
		}
	}
	a.refreshSearchReplace()
}

func (a *App) handleSearchReplaceInput(ch string) {
	if a.ReplaceFocused == 0 {
		a.SearchQuery += ch
	} else if a.ReplaceFocused == 1 {
		a.ReplaceText += ch
	}
	a.refreshSearchReplace()
}

func (a *App) handleSearchReplaceBackspace() {
	if a.ReplaceFocused == 0 {
		runes := []rune(a.SearchQuery)
		if len(runes) > 0 {
			a.SearchQuery = string(runes[:len(runes)-1])
		}
	} else if a.ReplaceFocused == 1 {
		runes := []rune(a.ReplaceText)
		if len(runes) > 0 {
			a.ReplaceText = string(runes[:len(runes)-1])
		}
	}
	a.refreshSearchReplace()
}

func (a *App) cycleReplaceField(delta int) {
	a.ReplaceFocused = (a.ReplaceFocused + delta + 2) % 2
	a.refreshSearchReplace()
}

func (a *App) toggleSearchCase() {
	a.SearchOptions.CaseSensitive = !a.SearchOptions.CaseSensitive
	a.refreshSearchReplace()
}

func (a *App) toggleSearchWord() {
	a.SearchOptions.WholeWord = !a.SearchOptions.WholeWord
	a.refreshSearchReplace()
}

func (a *App) toggleSearchRegex() {
	a.SearchOptions.IsRegex = !a.SearchOptions.IsRegex
	a.refreshSearchReplace()
}

func (a *App) applyReplaceCurrent() {
	buf := a.CurrentBuffer()
	if buf == nil || a.SearchQuery == "" {
		return
	}
	opts := a.SearchOptions
	opts.Query = a.SearchQuery
	opts.Replacement = a.ReplaceText
	matches, err := search.FindInPieceTable(buf.PT, opts, buf.FilePath)
	if err != nil || len(matches) == 0 {
		a.StatusMessage = "No match to replace"
		return
	}

	targetIdx := 0
	currPos := buf.CursorPos
	for i, m := range matches {
		if m.Line > currPos.Line || (m.Line == currPos.Line && m.Col >= currPos.Col) {
			targetIdx = i
			break
		}
	}
	targetMatch := matches[targetIdx]

	err = search.ReplaceOne(buf.PT, targetMatch, opts, a.ReplaceText)
	if err != nil {
		a.StatusMessage = fmt.Sprintf("Replace error: %v", err)
		return
	}

	buf.Dirty = true
	a.reparseBuffer(buf)
	a.notifyLSPBufferChanged(buf)

	buf.CursorPos.Line = targetMatch.Line
	buf.CursorPos.Col = targetMatch.Col

	a.StatusMessage = "Replaced 1 occurrence"
	a.refreshSearchReplace()
}

func (a *App) applyReplaceAll() {
	if a.SearchQuery == "" {
		return
	}
	opts := a.SearchOptions
	opts.Query = a.SearchQuery
	opts.Replacement = a.ReplaceText

	if a.ReplaceScope == "project" {
		matches, err := search.ProjectSearch(a.WorkspacePath, opts, 2000)
		if err != nil {
			a.StatusMessage = fmt.Sprintf("Project search error: %v", err)
			return
		}
		if len(matches) == 0 {
			a.StatusMessage = "No matches found in project"
			return
		}

		totalReplaced := 0
		filesModified := 0
		filesMap := make(map[string]bool)
		for _, m := range matches {
			filesMap[m.FilePath] = true
		}

		for filePath := range filesMap {
			var targetBuf *BufferItem
			for _, b := range a.Buffers {
				if isSameFile(b.FilePath, filePath) {
					targetBuf = b
					break
				}
			}

			if targetBuf != nil {
				cnt, repErr := search.ReplaceAll(targetBuf.PT, opts, a.ReplaceText)
				if repErr == nil && cnt > 0 {
					targetBuf.Dirty = true
					a.reparseBuffer(targetBuf)
					a.notifyLSPBufferChanged(targetBuf)
					totalReplaced += cnt
					filesModified++
				}
			} else {
				data, readErr := os.ReadFile(filePath)
				if readErr == nil {
					pt := buffer.NewPieceTableFromString(string(data))
					cnt, repErr := search.ReplaceAll(pt, opts, a.ReplaceText)
					if repErr == nil && cnt > 0 {
						if writeErr := atomicWriteFile(filePath, []byte(pt.Text()), 0644); writeErr == nil {
							totalReplaced += cnt
							filesModified++
						}
					}
				}
			}
		}

		a.StatusMessage = fmt.Sprintf("Replaced %d occurrence(s) across %d file(s)", totalReplaced, filesModified)
		a.refreshSearchReplace()
		return
	}

	buf := a.CurrentBuffer()
	if buf == nil {
		return
	}

	cnt, err := search.ReplaceAll(buf.PT, opts, a.ReplaceText)
	if err != nil {
		a.StatusMessage = fmt.Sprintf("Replace error: %v", err)
	} else {
		buf.Dirty = true
		a.reparseBuffer(buf)
		a.notifyLSPBufferChanged(buf)
		a.StatusMessage = fmt.Sprintf("Replaced %d occurrence(s)", cnt)
	}
	a.refreshSearchReplace()
}

func (a *App) refreshSearchReplace() {
	if a.Drawer == nil || !a.Drawer.Open || a.Drawer.Mode != DrawerModeSearchReplace {
		return
	}

	opts := a.SearchOptions
	opts.Query = a.SearchQuery
	opts.Replacement = a.ReplaceText

	cursorFind := ""
	cursorRepl := ""
	if a.ReplaceFocused == 0 {
		cursorFind = "█"
	} else if a.ReplaceFocused == 1 {
		cursorRepl = "█"
	}

	caseBadge := "[Alt+c] Case: OFF"
	if opts.CaseSensitive {
		caseBadge = "[Alt+c] Case: ON"
	}
	wordBadge := "[Alt+w] Word: OFF"
	if opts.WholeWord {
		wordBadge = "[Alt+w] Word: ON"
	}
	regexBadge := "[Alt+r] Regex: OFF"
	if opts.IsRegex {
		regexBadge = "[Alt+r] Regex: ON"
	}

	scopeBadge := fmt.Sprintf("Scope: %s", a.ReplaceScope)

	var lines []string
	lines = append(lines, fmt.Sprintf("  Find:    %s%s", a.SearchQuery, cursorFind))
	lines = append(lines, fmt.Sprintf("  Replace: %s%s", a.ReplaceText, cursorRepl))
	lines = append(lines, fmt.Sprintf("  Flags:   %s  %s  %s  [%s]", caseBadge, wordBadge, regexBadge, scopeBadge))

	if a.ReplaceScope == "project" {
		matches, _ := search.ProjectSearch(a.WorkspacePath, opts, 200)
		a.SearchMatches = matches
		lines = append(lines, fmt.Sprintf("  Matches: %d match(es) across project", len(matches)))
		lines = append(lines, "  Preview Diff (Enter: Replace All | Tab: Switch Field | Esc: Close):")

		shown := 0
		lastFile := ""
		for _, m := range matches {
			if shown >= 30 {
				lines = append(lines, fmt.Sprintf("  ... and %d more match(es)", len(matches)-shown))
				break
			}
			relPath := m.FilePath
			if a.WorkspacePath != "" {
				if r, err := filepath.Rel(a.WorkspacePath, m.FilePath); err == nil {
					relPath = r
				}
			}
			if relPath != lastFile {
				lines = append(lines, fmt.Sprintf("--- %s ---", relPath))
				lastFile = relPath
			}
			oldLine, newLine := search.PreviewLineReplacement(m.LineContent, m, opts, a.ReplaceText)
			lines = append(lines, fmt.Sprintf("  - %d: %s", m.Line+1, oldLine))
			lines = append(lines, fmt.Sprintf("  + %d: %s", m.Line+1, newLine))
			shown++
		}
	} else {
		buf := a.CurrentBuffer()
		var matches []search.Match
		if buf != nil && opts.Query != "" {
			matches, _ = search.FindInPieceTable(buf.PT, opts, buf.FilePath)
		}
		a.SearchMatches = matches
		lines = append(lines, fmt.Sprintf("  Matches: %d match(es)", len(matches)))
		lines = append(lines, "  Preview Diff (Enter: Replace All | Alt+y: Replace Current | Tab: Switch Field):")

		shown := 0
		for _, m := range matches {
			if shown >= 20 {
				lines = append(lines, fmt.Sprintf("  ... and %d more match(es)", len(matches)-shown))
				break
			}
			oldLine, newLine := search.PreviewLineReplacement(m.LineContent, m, opts, a.ReplaceText)
			lines = append(lines, fmt.Sprintf("  - Line %d: %s", m.Line+1, oldLine))
			lines = append(lines, fmt.Sprintf("  + Line %d: %s", m.Line+1, newLine))
			shown++
		}
	}

	a.Drawer.Content = lines
}

func (a *App) jumpToCurrentDrawerDiagnostic() {
	if a.Drawer == nil {
		return
	}
	diag := a.Drawer.CurrentDiagnostic()
	if diag != nil {
		a.jumpToDiagnostic(diag)
		a.Drawer.Close()
		a.FSM.SetState(fsm.StateNormal)
	}
}

func (a *App) jumpToDiagnostic(diag *tooling.DiagnosticError) {
	if diag == nil {
		return
	}
	currBuf := a.CurrentBuffer()
	if currBuf != nil && a.Jumplist != nil {
		a.Jumplist.Record(tooling.Location{
			FilePath: currBuf.FilePath,
			Line:     currBuf.CursorPos.Line,
			Col:      currBuf.CursorPos.Col,
		})
	}

	a.openOrSwitchToFile(diag.FilePath)

	targetBuf := a.CurrentBuffer()
	if targetBuf != nil {
		totalLines := targetBuf.PT.LineCount()
		if diag.Line >= totalLines {
			targetBuf.CursorPos.Line = max(0, totalLines-1)
		} else {
			targetBuf.CursorPos.Line = max(0, diag.Line)
		}
		lineLen := a.getLineLength(targetBuf, targetBuf.CursorPos.Line)
		targetBuf.CursorPos.Col = max(0, diag.Col)
		if targetBuf.CursorPos.Col > lineLen {
			targetBuf.CursorPos.Col = lineLen
		}

		if a.Jumplist != nil {
			a.Jumplist.Record(tooling.Location{
				FilePath: targetBuf.FilePath,
				Line:     targetBuf.CursorPos.Line,
				Col:      targetBuf.CursorPos.Col,
			})
		}
	}
	a.StatusMessage = fmt.Sprintf("[%s] %s", strings.ToUpper(diag.Severity), diag.Message)
}

func (a *App) jumpToNextDiagnosticError() {
	if a.Drawer == nil || len(a.Drawer.ParsedErrors) == 0 {
		a.StatusMessage = "No compiler diagnostics"
		return
	}
	currBuf := a.CurrentBuffer()
	var nextDiag *tooling.DiagnosticError

	if currBuf != nil {
		for _, diag := range a.Drawer.ParsedErrors {
			if isSameFile(diag.FilePath, currBuf.FilePath) && diag.Line > currBuf.CursorPos.Line {
				nextDiag = diag
				break
			}
		}
	}
	if nextDiag == nil {
		nextDiag = a.Drawer.ParsedErrors[0]
	}
	a.jumpToDiagnostic(nextDiag)
}

func (a *App) jumpToPrevDiagnosticError() {
	if a.Drawer == nil || len(a.Drawer.ParsedErrors) == 0 {
		a.StatusMessage = "No compiler diagnostics"
		return
	}
	currBuf := a.CurrentBuffer()
	var prevDiag *tooling.DiagnosticError

	if currBuf != nil {
		for i := len(a.Drawer.ParsedErrors) - 1; i >= 0; i-- {
			diag := a.Drawer.ParsedErrors[i]
			if isSameFile(diag.FilePath, currBuf.FilePath) && diag.Line < currBuf.CursorPos.Line {
				prevDiag = diag
				break
			}
		}
	}
	if prevDiag == nil {
		prevDiag = a.Drawer.ParsedErrors[len(a.Drawer.ParsedErrors)-1]
	}
	a.jumpToDiagnostic(prevDiag)
}

func (a *App) switchAlternateFile() {
	currBuf := a.CurrentBuffer()
	if currBuf == nil || currBuf.FilePath == "" {
		a.StatusMessage = "No active file for alternate toggle"
		return
	}
	altPath, err := tooling.FindAlternate(currBuf.FilePath, a.WorkspacePath)
	if err != nil {
		a.StatusMessage = fmt.Sprintf("Alternate: %v", err)
		return
	}
	if a.Jumplist != nil {
		a.Jumplist.Record(tooling.Location{
			FilePath: currBuf.FilePath,
			Line:     currBuf.CursorPos.Line,
			Col:      currBuf.CursorPos.Col,
		})
	}
	a.openOrSwitchToFile(altPath)
	a.StatusMessage = fmt.Sprintf("Switched to alternate: %s", altPath)
}

func (a *App) jumplistBack() {
	if a.Jumplist == nil {
		return
	}
	currBuf := a.CurrentBuffer()
	if currBuf != nil {
		a.Jumplist.Record(tooling.Location{
			FilePath: currBuf.FilePath,
			Line:     currBuf.CursorPos.Line,
			Col:      currBuf.CursorPos.Col,
		})
	}
	loc, ok := a.Jumplist.Back()
	if !ok {
		a.StatusMessage = "At oldest jump location"
		return
	}
	a.openOrSwitchToFile(loc.FilePath)
	targetBuf := a.CurrentBuffer()
	if targetBuf != nil {
		targetBuf.CursorPos.Line = loc.Line
		targetBuf.CursorPos.Col = loc.Col
	}
}

func (a *App) jumplistForward() {
	if a.Jumplist == nil {
		return
	}
	loc, ok := a.Jumplist.Forward()
	if !ok {
		a.StatusMessage = "At newest jump location"
		return
	}
	a.openOrSwitchToFile(loc.FilePath)
	targetBuf := a.CurrentBuffer()
	if targetBuf != nil {
		targetBuf.CursorPos.Line = loc.Line
		targetBuf.CursorPos.Col = loc.Col
	}
}

func (a *App) openOrSwitchToFile(targetPath string) {
	if targetPath == "" {
		return
	}
	cleanTarget := filepath.Clean(targetPath)
	for i, b := range a.Buffers {
		if filepath.Clean(b.FilePath) == cleanTarget || b.FileName == targetPath || b.FilePath == targetPath {
			a.ActiveBuf = i
			a.ensureLSPForBuffer(b)
			a.refreshActiveDrawer()
			return
		}
	}

	content := ""
	if data, err := os.ReadFile(targetPath); err == nil {
		content = string(data)
	} else if data, err := os.ReadFile(cleanTarget); err == nil {
		content = string(data)
	}

	// If the only buffer is an unmodified empty untitled buffer, replace it
	if len(a.Buffers) == 1 && a.Buffers[0].FilePath == "" && !a.Buffers[0].Dirty && a.Buffers[0].PT.Len() == 0 {
		newBuf := NewBufferItem(1, targetPath, content)
		a.Buffers[0] = newBuf
		a.ActiveBuf = 0
		a.reparseBuffer(newBuf)
		a.ensureLSPForBuffer(newBuf)
		a.refreshActiveDrawer()
		return
	}

	newBuf := NewBufferItem(len(a.Buffers)+1, targetPath, content)
	a.Buffers = append(a.Buffers, newBuf)
	a.ActiveBuf = len(a.Buffers) - 1
	a.reparseBuffer(newBuf)
	a.ensureLSPForBuffer(newBuf)
	a.refreshActiveDrawer()
}

func (a *App) toggleMarkdownPreview() {
	if a.Drawer == nil {
		return
	}
	if a.Drawer.Open && a.Drawer.Mode == DrawerModeMarkdown {
		a.Drawer.Close()
		a.FSM.SetState(fsm.StateNormal)
		return
	}

	currBuf := a.CurrentBuffer()
	if currBuf == nil {
		return
	}

	previewWidth := a.getViewportWidth() - 4
	if previewWidth <= 0 {
		previewWidth = 80
	}
	rendered, err := markdown.RenderPreview(currBuf.PT.Text(), previewWidth, a.MermaidMode)
	if err != nil {
		rendered = fmt.Sprintf("Error rendering markdown: %v\n\n%s", err, currBuf.PT.Text())
	}

	a.Drawer.Mode = DrawerModeMarkdown
	a.Drawer.Title = "Markdown Preview (Esc/q to close, Z to maximize)"
	a.Drawer.SetContent(rendered)
	a.Drawer.Open = true
	a.FSM.SetState(fsm.StateDrawer)
}

func (a *App) refreshMarkdownPreview() {
	if a.Drawer == nil || !a.Drawer.Open || a.Drawer.Mode != DrawerModeMarkdown {
		return
	}
	currBuf := a.CurrentBuffer()
	if currBuf == nil {
		return
	}
	previewWidth := a.getViewportWidth() - 4
	if previewWidth <= 0 {
		previewWidth = 80
	}
	rendered, err := markdown.RenderPreview(currBuf.PT.Text(), previewWidth, a.MermaidMode)
	if err != nil {
		rendered = fmt.Sprintf("Error rendering markdown: %v\n\n%s", err, currBuf.PT.Text())
	}

	oldCursor := a.Drawer.CursorLine
	oldScroll := a.Drawer.ScrollY
	a.Drawer.SetContent(rendered)
	if oldCursor < len(a.Drawer.Content) {
		a.Drawer.CursorLine = oldCursor
	} else if len(a.Drawer.Content) > 0 {
		a.Drawer.CursorLine = len(a.Drawer.Content) - 1
	}
	if oldScroll < len(a.Drawer.Content) {
		a.Drawer.ScrollY = oldScroll
	}
	a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
}

func (a *App) toggleMermaidMode() {
	if a.MermaidMode == markdown.MermaidModeBoxArt {
		a.MermaidMode = markdown.MermaidModeKitty
		a.StatusMessage = "Mermaid: Kitty graphics mode"
	} else {
		a.MermaidMode = markdown.MermaidModeBoxArt
		a.StatusMessage = "Mermaid: Unicode box-art mode"
	}
	if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeMarkdown {
		a.refreshMarkdownPreview()
	}
}

func (a *App) toggleCurrentLineChecklist() {
	buf := a.CurrentBuffer()
	if buf == nil {
		return
	}
	lineIdx := buf.CursorPos.Line
	lineText, err := buf.PT.Line(lineIdx)
	if err != nil {
		return
	}

	newLine, toggled := markdown.ToggleCheckbox(lineText)
	if !toggled {
		return
	}

	lineStart, err := buf.PT.LineOffset(lineIdx)
	if err != nil {
		return
	}

	buf.PT.BeginTransaction()
	if len(lineText) > 0 {
		_ = buf.PT.Delete(lineStart, len(lineText))
	}
	_ = buf.PT.Insert(lineStart, newLine)
	buf.PT.CommitTransaction()

	newLineLen := utf8.RuneCountInString(newLine)
	if buf.CursorPos.Col > newLineLen {
		buf.CursorPos.Col = newLineLen
	}
	buf.Dirty = true
}

func isSameFile(p1, p2 string) bool {
	if p1 == "" || p2 == "" {
		return false
	}
	if p1 == p2 {
		return true
	}
	c1 := filepath.Clean(p1)
	c2 := filepath.Clean(p2)
	if c1 == c2 {
		return true
	}
	abs1, err1 := filepath.Abs(c1)
	abs2, err2 := filepath.Abs(c2)
	if err1 == nil && err2 == nil && abs1 == abs2 {
		return true
	}
	return false
}

// atomicWriteFile writes data to a temporary file in the same directory, fsyncs,
// applies permissions, and atomically renames it to target.
func atomicWriteFile(filePath string, data []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	cleanPath := filepath.Clean(filePath)
	if fi, err := os.Stat(cleanPath); err == nil {
		perm = fi.Mode().Perm()
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".koodi-save-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, cleanPath)
}

// SaveActiveBuffer saves the currently active buffer to disk atomically.
func (a *App) SaveActiveBuffer() error {
	curr := a.CurrentBuffer()
	if curr == nil {
		a.StatusMessage = "No active buffer to save"
		return errors.New("no active buffer")
	}
	return a.SaveBuffer(curr)
}

// SaveBuffer atomically saves the specified buffer to disk and notifies LSP.
func (a *App) SaveBuffer(buf *BufferItem) error {
	if buf == nil {
		return errors.New("nil buffer")
	}
	if buf.FilePath == "" {
		a.StatusMessage = "Cannot save unnamed buffer; file path required"
		return errors.New("unnamed buffer")
	}

	text := buf.PT.Text()
	data := []byte(text)

	if err := atomicWriteFile(buf.FilePath, data, 0644); err != nil {
		a.StatusMessage = fmt.Sprintf("Save error: %v", err)
		return err
	}

	buf.Dirty = false
	if a.LSPManager != nil {
		_ = a.LSPManager.DidSave(context.Background(), buf.FilePath)
	}

	a.StatusMessage = fmt.Sprintf("Saved %s (%d lines, %d bytes)", buf.FileName, buf.PT.LineCount(), len(data))
	return nil
}

// SaveAllBuffers saves all dirty buffers with valid file paths to disk.
func (a *App) SaveAllBuffers() (int, error) {
	saved := 0
	var firstErr error
	for _, b := range a.Buffers {
		if b.Dirty && b.FilePath != "" {
			if err := a.SaveBuffer(b); err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				saved++
			}
		}
	}
	if saved > 0 {
		a.StatusMessage = fmt.Sprintf("Saved %d modified buffer(s)", saved)
	} else if firstErr == nil {
		a.StatusMessage = "All buffers up to date"
	}
	return saved, firstErr
}

// hasDirtyBuffers returns true if any open buffer has unsaved changes.
func (a *App) hasDirtyBuffers() bool {
	for _, b := range a.Buffers {
		if b.Dirty {
			return true
		}
	}
	return false
}

// closeCurrentBuffer removes the active buffer tab and notifies LSP.
func (a *App) closeCurrentBuffer() {
	if len(a.Buffers) == 0 {
		return
	}
	closedPath := ""
	if curr := a.CurrentBuffer(); curr != nil {
		closedPath = curr.FilePath
	}

	if len(a.Buffers) > 1 {
		a.Buffers = append(a.Buffers[:a.ActiveBuf], a.Buffers[a.ActiveBuf+1:]...)
		if a.ActiveBuf >= len(a.Buffers) {
			a.ActiveBuf = len(a.Buffers) - 1
		}
		a.ensureLSPForBuffer(a.CurrentBuffer())
	} else if len(a.Buffers) == 1 {
		a.Buffers[0] = NewBufferItem(1, "", "")
		a.ActiveBuf = 0
	}

	if a.LSPManager != nil && closedPath != "" {
		_ = a.LSPManager.DidClose(context.Background(), closedPath)
	}
}

func (a *App) reparseBuffer(buf *BufferItem) {
	if buf == nil || buf.Highlighter == nil {
		return
	}
	bytes := buf.PT.Bytes()
	firstLine := ""
	if idx := strings.IndexByte(string(bytes), '\n'); idx != -1 {
		firstLine = string(bytes[:idx])
	} else {
		firstLine = string(bytes)
	}
	lang, langName := treesitter.DetectLanguageWithContent(buf.FilePath, firstLine)
	buf.LangName = langName
	buf.Highlighter.Parse(bytes, lang, langName, buf.FilePath)
}

// -----------------------------------------------------------------------------
// Agentic CLI Coexistence & IPC
// -----------------------------------------------------------------------------

func (a *App) openAgentReviewDrawer() {
	if a.Drawer == nil {
		return
	}
	if a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
		a.Drawer.Close()
		a.FSM.SetState(fsm.StateNormal)
		return
	}

	content := a.renderDiffReviewContent()
	a.Drawer.Mode = DrawerModeDiffReview
	a.Drawer.Title = "Agent Review (Space d a: Accept, Space d r: Revert, Esc/q: Close)"
	a.Drawer.SetContent(content)
	a.Drawer.Open = true
	a.FSM.SetState(fsm.StateDrawer)
}

func (a *App) refreshDiffReview() {
	if a.Drawer == nil || !a.Drawer.Open || a.Drawer.Mode != DrawerModeDiffReview {
		return
	}
	content := a.renderDiffReviewContent()
	oldCursor := a.Drawer.CursorLine
	oldScroll := a.Drawer.ScrollY
	a.Drawer.SetContent(content)
	if oldCursor < len(a.Drawer.Content) {
		a.Drawer.CursorLine = oldCursor
	}
	if oldScroll < len(a.Drawer.Content) {
		a.Drawer.ScrollY = oldScroll
	}
}

func (a *App) renderDiffReviewContent() string {
	if a.AgentTx == nil || a.AgentTx.FileCount() == 0 {
		return "No pending agent transactions.\n\nFiles modified by autonomous CLI agents will appear here."
	}

	var sb strings.Builder
	sb.WriteString("=== AGENT TRANSACTION: " + a.AgentTx.CompactSummary() + " ===\n\n")

	for i, f := range a.AgentTx.Files {
		marker := "  "
		if i == a.AgentTx.Selected {
			marker = "▶ "
		}
		sb.WriteString(fmt.Sprintf("%s%s %s %s\n", marker, f.Status, f.FilePath, f.Diff.FormatSummary()))
	}

	sb.WriteString("\n------------------------------------------------------------\n")
	sel := a.AgentTx.SelectedFile()
	if sel != nil {
		sb.WriteString(fmt.Sprintf("Diff for %s:\n\n", sel.FilePath))
		sb.WriteString(sel.Diff.UnifiedText())
	}

	return sb.String()
}

func (a *App) acceptAgentTransaction() {
	if a.AgentTx != nil {
		a.AgentTx.AcceptAll()
		a.AgentStatus = ""
		a.StatusMessage = "Agent transaction accepted"
	}
	if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
		a.Drawer.Close()
		a.FSM.SetState(fsm.StateNormal)
	}
}

func (a *App) revertAgentTransaction() {
	if a.AgentTx != nil {
		revertedFiles := make(map[string]bool)
		for _, f := range a.AgentTx.Files {
			abs, err := filepath.Abs(filepath.Clean(f.FilePath))
			if err == nil {
				revertedFiles[abs] = true
			} else {
				revertedFiles[filepath.Clean(f.FilePath)] = true
			}
		}

		if err := a.AgentTx.RevertAll(); err != nil {
			a.StatusMessage = fmt.Sprintf("Revert error: %v", err)
			return
		}
		a.AgentStatus = ""
		a.StatusMessage = "Agent transaction reverted"

		// Reload only buffers that were part of the reverted transaction
		for _, b := range a.Buffers {
			if b.FilePath == "" {
				continue
			}
			abs, err := filepath.Abs(filepath.Clean(b.FilePath))
			matched := false
			if err == nil && revertedFiles[abs] {
				matched = true
			} else if revertedFiles[filepath.Clean(b.FilePath)] {
				matched = true
			}
			if matched {
				_ = watcher.ReconcileBuffer(b.FilePath, false, b.PT)
				b.Dirty = false
				a.reparseBuffer(b)
			}
		}
	}
	if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
		a.Drawer.Close()
		a.FSM.SetState(fsm.StateNormal)
	}
}

// StartWatcher starts the background inotify watcher on WorkspacePath.
func (a *App) StartWatcher() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Watcher != nil {
		return nil
	}
	debounce := 200 * time.Millisecond
	if a.Config != nil && a.Config.Agent.BatchDebounceMs > 0 {
		debounce = time.Duration(a.Config.Agent.BatchDebounceMs) * time.Millisecond
	}
	w, err := watcher.NewWatcher(debounce)
	if err != nil {
		return err
	}
	if err := w.AddRecursive(a.WorkspacePath); err != nil {
		_ = w.Close()
		return err
	}
	a.Watcher = w

	go func() {
		for {
			select {
			case ev, ok := <-w.Events():
				if !ok {
					return
				}
				if a.Program != nil {
					a.Program.Send(WatcherMsg{Event: ev})
				} else {
					a.mu.Lock()
					a.handleWatcherEvent(ev)
					a.mu.Unlock()
				}
			case _, ok := <-w.Errors():
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

func (a *App) handleWatcherEvent(ev watcher.ChangeEvent) {
	for _, c := range ev.Changes {
		for _, b := range a.Buffers {
			if isSameFile(b.FilePath, c.Path) {
				oldContent := b.PT.Text()
				res := watcher.ReconcileBuffer(b.FilePath, b.Dirty, b.PT)
				if res.Status == watcher.ReconcileCleanReload {
					if oldContent == res.DiskContent {
						break
					}
					b.Dirty = false
					a.StatusMessage = fmt.Sprintf("⚡ Reloaded %s", b.FileName)
					a.reparseBuffer(b)
					if a.AgentTx == nil {
						a.AgentTx = diff.NewAgentTransaction()
					}
					a.AgentTx.AddChange(c.Path, oldContent, res.DiskContent)
				} else if res.Status == watcher.ReconcileConflict {
					a.StatusMessage = fmt.Sprintf("⚠ Collision on %s: modified on disk with unsaved edits", b.FileName)
					if a.AgentTx == nil {
						a.AgentTx = diff.NewAgentTransaction()
					}
					a.AgentTx.AddChange(c.Path, oldContent, res.DiskContent)
				}
				break
			}
		}
	}
	if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDiffReview {
		a.refreshDiffReview()
	}
	if a.FileTree != nil && a.FileTree.Open {
		a.FileTree.Refresh()
	}
}

// StartIPCServer starts the Unix domain socket server.
func (a *App) StartIPCServer(socketPath string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.IPCServer != nil {
		return nil
	}
	srv, err := ipc.NewServer(socketPath, a)
	if err != nil {
		return err
	}
	a.IPCServer = srv
	return nil
}

// Close terminates the file watcher, IPC server, and LSP clients.
func (a *App) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.insertBuffer != nil && a.insertBuffer.PT != nil {
		a.insertBuffer.PT.CommitTransaction()
		a.insertBuffer = nil
	}

	a.lspDebounceMu.Lock()
	if a.lspDebounceTimer != nil {
		a.lspDebounceTimer.Stop()
		a.lspDebounceTimer = nil
	}
	a.lspDebounceMu.Unlock()

	if a.Watcher != nil {
		_ = a.Watcher.Close()
		a.Watcher = nil
	}
	if a.IPCServer != nil {
		_ = a.IPCServer.Close()
		a.IPCServer = nil
	}
	if a.LSPManager != nil {
		a.LSPManager.StopAll()
	}
}

// ipc.Handler implementation
func (a *App) HandleOpen(p ipc.OpenParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "open", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyOpen(p)
}

func (a *App) HandleDiff(p ipc.DiffParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "diff", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyDiff(p)
}

func (a *App) HandleReload(p ipc.ReloadParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "reload", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyReload(p)
}

func (a *App) HandleNotify(p ipc.NotifyParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "notify", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyNotify(p)
}

func (a *App) HandleStatusBadge(p ipc.StatusBadgeParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "status-badge", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyStatusBadge(p)
}

func (a *App) HandleAction(p ipc.ActionParams) error {
	if a.Program != nil {
		done := make(chan error, 1)
		a.Program.Send(IPCMsg{Type: "action", Params: p, Done: done})
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("timeout waiting for editor")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applyAction(p)
}

func (a *App) handleIPCMsg(msg IPCMsg) error {
	switch msg.Type {
	case "open":
		p, ok := msg.Params.(ipc.OpenParams)
		if !ok {
			return errors.New("invalid open params")
		}
		return a.applyOpen(p)
	case "diff":
		p, ok := msg.Params.(ipc.DiffParams)
		if !ok {
			return errors.New("invalid diff params")
		}
		return a.applyDiff(p)
	case "reload":
		p, ok := msg.Params.(ipc.ReloadParams)
		if !ok {
			return errors.New("invalid reload params")
		}
		return a.applyReload(p)
	case "notify":
		p, ok := msg.Params.(ipc.NotifyParams)
		if !ok {
			return errors.New("invalid notify params")
		}
		return a.applyNotify(p)
	case "status-badge":
		p, ok := msg.Params.(ipc.StatusBadgeParams)
		if !ok {
			return errors.New("invalid status-badge params")
		}
		return a.applyStatusBadge(p)
	case "action":
		p, ok := msg.Params.(ipc.ActionParams)
		if !ok {
			return errors.New("invalid action params")
		}
		return a.applyAction(p)
	default:
		return fmt.Errorf("unknown IPC message type %q", msg.Type)
	}
}

func (a *App) applyOpen(p ipc.OpenParams) error {
	if p.File == "" {
		return errors.New("file path required")
	}
	a.openOrSwitchToFile(p.File)
	currBuf := a.CurrentBuffer()
	if currBuf != nil {
		if p.Line > 0 {
			totalLines := currBuf.PT.LineCount()
			targetLine := p.Line - 1
			if targetLine >= totalLines {
				targetLine = max(0, totalLines-1)
			}
			currBuf.CursorPos.Line = targetLine
		}
		if p.Col > 0 {
			lineLen := a.getLineLength(currBuf, currBuf.CursorPos.Line)
			targetCol := p.Col - 1
			if targetCol > lineLen {
				targetCol = lineLen
			}
			currBuf.CursorPos.Col = targetCol
		}
	}
	a.StatusMessage = fmt.Sprintf("Opened %s", p.File)
	return nil
}

func (a *App) applyDiff(p ipc.DiffParams) error {
	if p.File == "" {
		return errors.New("file path required")
	}

	diskData, err := os.ReadFile(p.File)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", p.File, err)
	}
	diskContent := string(diskData)

	oldContent := ""
	for _, b := range a.Buffers {
		if isSameFile(b.FilePath, p.File) {
			oldContent = b.PT.Text()
			break
		}
	}

	d := diff.ComputeDiff(oldContent, diskContent)
	if a.AgentTx == nil {
		a.AgentTx = diff.NewAgentTransaction()
	}
	a.AgentTx.AddChange(p.File, oldContent, diskContent)

	a.Drawer.Mode = DrawerModeDiffReview
	a.Drawer.Title = fmt.Sprintf("Diff: %s %s (Esc/q to close)", filepath.Base(p.File), d.FormatSummary())
	a.Drawer.SetContent(d.UnifiedText())
	a.Drawer.Open = true
	a.FSM.SetState(fsm.StateDrawer)
	return nil
}

func (a *App) applyReload(p ipc.ReloadParams) error {
	reloadedCount := 0
	collisionCount := 0
	for _, b := range a.Buffers {
		if b.FilePath == "" {
			continue
		}
		if p.All || isSameFile(b.FilePath, p.File) {
			res := watcher.ReconcileBuffer(b.FilePath, b.Dirty, b.PT)
			if res.Status == watcher.ReconcileCleanReload {
				b.Dirty = false
				a.reparseBuffer(b)
				reloadedCount++
			} else if res.Status == watcher.ReconcileConflict {
				collisionCount++
			}
		}
	}
	if collisionCount > 0 {
		a.StatusMessage = fmt.Sprintf("Reloaded %d buffer(s), %d conflict(s) skipped (unsaved local edits)", reloadedCount, collisionCount)
	} else {
		a.StatusMessage = fmt.Sprintf("Reloaded %d buffer(s)", reloadedCount)
	}
	return nil
}

func (a *App) applyNotify(p ipc.NotifyParams) error {
	levelTag := ""
	switch strings.ToLower(p.Level) {
	case "warn", "warning":
		levelTag = "[WARN] "
	case "error", "err":
		levelTag = "[ERROR] "
	case "info":
		levelTag = "[INFO] "
	}
	a.StatusMessage = levelTag + p.Message
	return nil
}

func (a *App) applyStatusBadge(p ipc.StatusBadgeParams) error {
	a.AgentStatus = p.Badge
	a.AgentBusy = p.Busy
	return nil
}

func (a *App) applyAction(p ipc.ActionParams) error {
	switch p.Name {
	case "drawer:toggle", "toggle_drawer":
		a.executeAction(fsm.Action{Type: fsm.ActionToggleDrawer})
	case "drawer:close", "close_drawer":
		a.executeAction(fsm.Action{Type: fsm.ActionDrawerClose})
	case "drawer:maximize":
		a.executeAction(fsm.Action{Type: fsm.ActionDrawerMaximize})
	case "markdown:preview", "toggle_preview":
		a.executeAction(fsm.Action{Type: fsm.ActionToggleMarkdownPreview})
	case "markdown:checklist", "toggle_checklist":
		a.executeAction(fsm.Action{Type: fsm.ActionToggleChecklist})
	case "markdown:mermaid", "toggle_mermaid":
		a.executeAction(fsm.Action{Type: fsm.ActionToggleMermaidMode})
	case "agent:review", "diff_review":
		a.executeAction(fsm.Action{Type: fsm.ActionAgentReviewOpen})
	case "agent:accept_all", "accept_all":
		a.executeAction(fsm.Action{Type: fsm.ActionAgentAcceptAll})
	case "agent:revert_all", "revert_all":
		a.executeAction(fsm.Action{Type: fsm.ActionAgentRevertAll})
	case "tab:next":
		a.executeAction(fsm.Action{Type: fsm.ActionTabNext})
	case "tab:prev":
		a.executeAction(fsm.Action{Type: fsm.ActionTabPrev})
	case "alternate", "alternate_file":
		a.executeAction(fsm.Action{Type: fsm.ActionAlternateFile})
	case "tree:toggle", "filetree:toggle", "toggle_tree":
		a.executeAction(fsm.Action{Type: fsm.ActionToggleFileTree})
	case "picker:files", "picker_files":
		a.executeAction(fsm.Action{Type: fsm.ActionPickerOpenFiles})
	case "picker:buffers", "picker_buffers":
		a.executeAction(fsm.Action{Type: fsm.ActionPickerOpenBuffers})
	case "picker:grep", "picker_grep":
		a.executeAction(fsm.Action{Type: fsm.ActionPickerOpenGrep})
	case "help:toggle", "toggle_help", "help":
		a.executeAction(fsm.Action{Type: fsm.ActionHelpToggle})
	default:
		return fmt.Errorf("unrecognized action %q", p.Name)
	}
	return nil
}

func (a *App) handleMouse(msg tea.MouseMsg) {
	if !a.Config.Editor.Mouse {
		return
	}

	// 0. Active FileTree dragging takes priority
	if a.FileTree != nil && a.FileTree.IsDragging {
		if msg.Action == tea.MouseActionRelease {
			a.FileTree.IsDragging = false
			a.refreshMarkdownPreview()
			return
		}
		if msg.Action == tea.MouseActionMotion {
			newW := msg.X + 1
			if newW < 15 {
				newW = 15
			}
			maxW := a.Width - 15
			if maxW > 50 {
				maxW = 50
			}
			if newW > maxW {
				newW = maxW
			}
			if a.FileTree.Width != newW {
				a.FileTree.Width = newW
				a.refreshMarkdownPreview()
			}
			return
		}
	}

	// Active drawer dragging
	if a.Drawer != nil && a.Drawer.IsDragging {
		if msg.Action == tea.MouseActionRelease {
			a.Drawer.IsDragging = false
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
			return
		}
		if msg.Action == tea.MouseActionMotion {
			newH := (a.Height - 1) - msg.Y - 1
			if newH < 3 {
				newH = 3
			}
			maxH := a.Height - 5
			if newH > maxH {
				newH = maxH
			}
			a.Drawer.Height = newH
			a.Drawer.MoveCursor(0, a.getDrawerContentHeight())
			return
		}
	}

	// Active scrollbar dragging
	if a.isScrollbarDragging {
		if msg.Action == tea.MouseActionRelease {
			a.isScrollbarDragging = false
			return
		}
		if msg.Action == tea.MouseActionMotion {
			buf := a.CurrentBuffer()
			if buf != nil {
				totalLines := buf.PT.LineCount()
				vpHeight := a.getViewportHeight()
				if totalLines > vpHeight && vpHeight > 1 {
					viewportTop := 1
					if !a.Config.Bufferline.Show {
						viewportTop = 0
					}
					clickRow := msg.Y - viewportTop
					if clickRow < 0 {
						clickRow = 0
					}
					if clickRow >= vpHeight {
						clickRow = vpHeight - 1
					}
					maxScrollY := totalLines - vpHeight
					buf.ScrollY = (clickRow * maxScrollY) / (vpHeight - 1)
				}
			}
			return
		}
	}

	// 1. Picker modal mouse interaction (when open)
	if a.Picker != nil && a.Picker.Open {
		modalW := a.Width * 66 / 100
		if modalW < 40 {
			modalW = 40
		}
		if modalW > a.Width-4 {
			modalW = a.Width - 4
		}
		modalH := a.Height * 66 / 100
		if modalH < 10 {
			modalH = 10
		}
		if modalH > a.Height-4 {
			modalH = a.Height - 4
		}
		topY := (a.Height - modalH) / 2
		leftX := (a.Width - modalW) / 2
		bottomY := topY + modalH - 1
		rightX := leftX + modalW - 1

		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.X < leftX || msg.X > rightX || msg.Y < topY || msg.Y > bottomY {
				a.Picker.Close()
				a.FSM.SetState(fsm.StateNormal)
				return
			}
			// Candidate list rows start at topY + 3
			if msg.Y >= topY+3 && msg.Y < bottomY {
				clickedIdx := a.Picker.ScrollY + (msg.Y - (topY + 3))
				if clickedIdx >= 0 && clickedIdx < len(a.Picker.Filtered) {
					a.Picker.SelectedIndex = clickedIdx
					a.executeAction(fsm.Action{Type: fsm.ActionPickerSelect})
					return
				}
			}
		}
		if msg.Button == tea.MouseButtonWheelUp {
			a.Picker.MoveCursor(-1, modalH-4)
			return
		}
		if msg.Button == tea.MouseButtonWheelDown {
			a.Picker.MoveCursor(1, modalH-4)
			return
		}
		return
	}

	// 1b. Help modal mouse interaction (when open)
	if a.HelpModal != nil && a.HelpModal.Open {
		modalW := a.Width * 75 / 100
		if modalW < 68 {
			modalW = 68
		}
		if modalW > a.Width-4 {
			modalW = a.Width - 4
		}
		modalH := a.Height * 75 / 100
		if modalH < 12 {
			modalH = 12
		}
		if modalH > a.Height-4 {
			modalH = a.Height - 4
		}
		topY := (a.Height - modalH) / 2
		leftX := (a.Width - modalW) / 2
		bottomY := topY + modalH - 1
		rightX := leftX + modalW - 1

		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.X < leftX || msg.X > rightX || msg.Y < topY || msg.Y > bottomY {
				a.HelpModal.Close()
				a.FSM.SetState(fsm.StateNormal)
				return
			}
			// Candidate list rows start at topY + 3 and end before footer divider (bottomY - 2)
			if msg.Y >= topY+3 && msg.Y < bottomY-2 {
				clickedIdx := a.HelpModal.ScrollY + (msg.Y - (topY + 3))
				if clickedIdx >= 0 && clickedIdx < len(a.HelpModal.Filtered) {
					a.HelpModal.SelectedIndex = clickedIdx
					a.executeAction(fsm.Action{Type: fsm.ActionHelpSelect})
					return
				}
			}
		}
		if msg.Button == tea.MouseButtonWheelUp {
			a.HelpModal.Prev()
			return
		}
		if msg.Button == tea.MouseButtonWheelDown {
			a.HelpModal.Next()
			return
		}
		return
	}

	buf := a.CurrentBuffer()
	if buf == nil {
		return
	}

	// 2. Click on Row 0: Tab switching
	if msg.Y == 0 && a.Config.Bufferline.Show {
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			tabIdx := HitTestTab(a.Buffers, msg.X)
			if tabIdx >= 0 && tabIdx < len(a.Buffers) {
				a.ActiveBuf = tabIdx
				a.ensureLSPForBuffer(a.CurrentBuffer())
				a.refreshActiveDrawer()
			}
		}
		return
	}

	viewportTop := 1
	if !a.Config.Bufferline.Show {
		viewportTop = 0
	}

	// 3. FileTree interaction
	treeOpen := (a.FileTree != nil && a.FileTree.Open)
	treeWidth := 0
	if treeOpen {
		treeWidth = a.FileTree.Width
		if treeWidth >= a.Width-15 {
			treeWidth = a.Width - 15
		}
		if treeWidth < 10 {
			treeWidth = 10
		}

		if msg.Y >= viewportTop && msg.Y < a.Height-1 {
			// Drag handle on right border: X == treeWidth - 1
			if msg.X == treeWidth-1 {
				if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
					a.FileTree.IsDragging = true
					return
				}
			}
			// Clicks/scroll inside FileTree body
			if msg.X < treeWidth-1 {
				availH := a.Height - viewportTop - 1
				if msg.Button == tea.MouseButtonWheelUp {
					a.FileTree.MoveCursor(-3, availH)
					return
				}
				if msg.Button == tea.MouseButtonWheelDown {
					a.FileTree.MoveCursor(3, availH)
					return
				}
				if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
					clickedRow := a.FileTree.ScrollY + (msg.Y - viewportTop)
					if clickedRow >= 0 && clickedRow < len(a.FileTree.VisibleNodes) {
						a.FileTree.CursorIndex = clickedRow
						nodePath, isFile := a.FileTree.ToggleExpand()
						if isFile && nodePath != "" {
							a.openOrSwitchToFile(nodePath)
							a.FSM.SetState(fsm.StateNormal)
						} else {
							a.FSM.SetState(fsm.StateTree)
						}
					}
					return
				}
				return
			}
		}
	}

	// For drawer and viewport, calculate X relative to editor area
	editorX := msg.X - treeWidth
	if editorX < 0 {
		return
	}

	// 4. Drawer mouse events & dragging
	drawerTopRow := -1
	if a.Drawer != nil && a.Drawer.Open {
		if a.Drawer.Maximized {
			drawerTopRow = viewportTop
		} else {
			drawerTopRow = viewportTop + a.getViewportHeight()
		}

		// Drag handle interaction on drawer top border
		if msg.Y == drawerTopRow {
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				a.Drawer.IsDragging = true
				return
			}
		}

		// Inside drawer body
		if msg.Y > drawerTopRow && msg.Y < a.Height-1 {
			if msg.Button == tea.MouseButtonWheelUp {
				a.Drawer.Scroll(-3, a.getDrawerContentHeight())
				return
			}
			if msg.Button == tea.MouseButtonWheelDown {
				a.Drawer.Scroll(3, a.getDrawerContentHeight())
				return
			}
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				row := a.Drawer.ScrollY + (msg.Y - drawerTopRow - 1)
				if row >= len(a.Drawer.Content) {
					row = max(0, len(a.Drawer.Content)-1)
				}
				a.Drawer.CursorLine = row
				a.FSM.SetState(fsm.StateDrawer)
				return
			}
			return
		}
	}

	// 5. Viewport mouse events
	viewportBottom := a.Height - 1
	if drawerTopRow >= 0 {
		viewportBottom = drawerTopRow
	}

	if msg.Y >= viewportTop && msg.Y < viewportBottom {
		totalLines := buf.PT.LineCount()
		gutterWidth := CalculateGutterWidth(totalLines)
		codeX := editorX - gutterWidth
		if codeX < 0 {
			codeX = 0
		}

		targetLine := buf.ScrollY + (msg.Y - viewportTop)
		if targetLine >= totalLines {
			targetLine = totalLines - 1
		}
		if targetLine < 0 {
			targetLine = 0
		}

		targetLineText, _ := buf.PT.Line(targetLine)
		targetRuneCol := VisualColToRuneCol(targetLineText, codeX+buf.ScrollX, a.Config.Editor.TabWidth)
		lineLen := a.getLineLength(buf, targetLine)
		if targetRuneCol > lineLen {
			targetRuneCol = lineLen
		}

		// Wheel / Touchpad 2-finger scrolling
		if msg.Button == tea.MouseButtonWheelUp {
			buf.ScrollY -= 3
			if buf.ScrollY < 0 {
				buf.ScrollY = 0
			}
			return
		}
		if msg.Button == tea.MouseButtonWheelDown {
			buf.ScrollY += 3
			maxScrollY := totalLines - a.getViewportHeight()
			if maxScrollY < 0 {
				maxScrollY = 0
			}
			if buf.ScrollY > maxScrollY {
				buf.ScrollY = maxScrollY
			}
			return
		}

		// Scrollbar click / drag start
		editorWidth := a.Width - treeWidth
		vpHeight := viewportBottom - viewportTop
		if vpHeight <= 0 {
			vpHeight = a.getViewportHeight()
		}
		if totalLines > vpHeight && editorX >= editorWidth-1 {
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				a.isScrollbarDragging = true
				if vpHeight > 1 {
					clickRow := msg.Y - viewportTop
					if clickRow < 0 {
						clickRow = 0
					}
					if clickRow >= vpHeight {
						clickRow = vpHeight - 1
					}
					maxScrollY := totalLines - vpHeight
					buf.ScrollY = (clickRow * maxScrollY) / (vpHeight - 1)
				}
				return
			}
		}

		// Gutter click: toggle breakpoint
		if editorX < gutterWidth && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if a.DAPManager != nil && buf.FilePath != "" {
				line1 := targetLine + 1
				set := a.DAPManager.ToggleBreakpoint(buf.FilePath, line1)
				if set {
					a.StatusMessage = fmt.Sprintf("Breakpoint set: %s:%d", filepath.Base(buf.FilePath), line1)
				} else {
					a.StatusMessage = fmt.Sprintf("Breakpoint cleared: %s:%d", filepath.Base(buf.FilePath), line1)
				}
				return
			}
		}

		// Click to position cursor
		if msg.Button == tea.MouseButtonLeft {
			if msg.Action == tea.MouseActionPress {
				buf.CursorPos = buffer.Position{Line: targetLine, Col: targetRuneCol}
				buf.SelectionAnchor = nil
				a.FSM.SetState(fsm.StateNormal)
			} else if msg.Action == tea.MouseActionMotion {
				// Drag to select
				if buf.SelectionAnchor == nil {
					anchor := buf.CursorPos
					buf.SelectionAnchor = &anchor
				}
				buf.CursorPos = buffer.Position{Line: targetLine, Col: targetRuneCol}
				a.FSM.SetState(fsm.StateSelect)
			}
		}
	}
}

func (a *App) getLineLength(buf *BufferItem, lineIdx int) int {
	lineText, err := buf.PT.Line(lineIdx)
	if err != nil {
		return 0
	}
	return utf8.RuneCountInString(lineText)
}

func (a *App) clampCursorToBounds(buf *BufferItem) {
	if buf == nil {
		return
	}
	lineCount := buf.PT.LineCount()
	if buf.CursorPos.Line >= lineCount {
		buf.CursorPos.Line = max(0, lineCount-1)
	}
	if buf.CursorPos.Line < 0 {
		buf.CursorPos.Line = 0
	}
	lineLen := a.getLineLength(buf, buf.CursorPos.Line)
	if buf.CursorPos.Col > lineLen {
		buf.CursorPos.Col = max(0, lineLen)
	}
	if buf.CursorPos.Col < 0 {
		buf.CursorPos.Col = 0
	}
}

func normalizeKey(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyEsc:
		return "esc"
	case tea.KeyEnter:
		return "enter"
	case tea.KeyBackspace:
		return "backspace"
	case tea.KeyDelete:
		return "delete"
	case tea.KeyTab:
		return "tab"
	case tea.KeySpace:
		return "Space"
	case tea.KeyLeft:
		return "left"
	case tea.KeyRight:
		return "right"
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyPgUp:
		return "pgup"
	case tea.KeyPgDown:
		return "pgdown"
	case tea.KeyHome:
		return "home"
	case tea.KeyEnd:
		return "end"
	case tea.KeyCtrlHome:
		return "ctrl+home"
	case tea.KeyCtrlEnd:
		return "ctrl+end"
	case tea.KeyCtrlLeft:
		return "ctrl+left"
	case tea.KeyCtrlRight:
		return "ctrl+right"
	case tea.KeyCtrlO:
		return "ctrl+o"
	case tea.KeyCtrlW:
		return "ctrl+w"
	case tea.KeyCtrlA:
		return "ctrl+a"
	case tea.KeyCtrlR:
		return "ctrl+r"
	case tea.KeyCtrlY:
		return "ctrl+y"
	case tea.KeyCtrlS:
		return "ctrl+s"
	case tea.KeyCtrlQ:
		return "ctrl+q"
	case tea.KeyCtrlAt:
		return "ctrl+space"
	default:
		str := msg.String()
		if str == " " {
			return "Space"
		}
		if str == "ctrl+@" || str == "ctrl+space" {
			return "ctrl+space"
		}
		if strings.HasPrefix(str, "alt+") {
			return str
		}
		return str
	}
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' ||
		r == 'ä' || r == 'ö' || r == 'å' || r == 'Ä' || r == 'Ö' || r == 'Å'
}
