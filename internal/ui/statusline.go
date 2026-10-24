package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/fsm"
)

// StatuslineState holds all metadata required to render the statusline.
type StatuslineState struct {
	Mode           fsm.State
	FilePath       string
	IsDirty        bool
	IsReadOnly     bool
	FileType       string
	Encoding       string
	Line           int
	Col            int
	TotalLines     int
	DiagnosticsMsg string
	LSPMsg         string
	AgentStatus    string
	AgentBusy      bool
	KeyBuffer      string
	SearchPrompt   string
	SearchMatchMsg string
}

// RenderStatusline renders Row H-1 according to the deterministic UI spec.
func RenderStatusline(s StatuslineState, width int, theme *Theme) string {
	// 1. Mode Badge
	var modeBadge string
	switch s.Mode {
	case fsm.StateInsert:
		modeBadge = theme.ModeInsert.Render(" INSERT ")
	case fsm.StateSelect:
		modeBadge = theme.ModeSelect.Render(" SELECT ")
	case fsm.StateLeader:
		modeBadge = theme.ModeLeader.Render(" LEADER ")
	case fsm.StateDrawer:
		modeBadge = theme.ModeDrawer.Render(" DRAWER ")
	case fsm.StatePicker:
		modeBadge = theme.ModePicker.Render(" PICKER ")
	case fsm.StateMatchPending:
		modeBadge = theme.ModeSelect.Render(" MATCH ")
	case fsm.StateTree:
		modeBadge = theme.ModeDrawer.Render(" TREE ")
	case fsm.StateHover:
		modeBadge = theme.ModeDrawer.Render(" HOVER ")
	case fsm.StateCompletion:
		modeBadge = theme.ModeInsert.Render(" CMP ")
	case fsm.StateCodeAction:
		modeBadge = theme.ModeSelect.Render(" ACTION ")
	case fsm.StateRename:
		modeBadge = theme.ModeLeader.Render(" RENAME ")
	case fsm.StateSearch:
		modeBadge = theme.ModeSearch.Render(" SEARCH ")
	case fsm.StateHelp:
		modeBadge = theme.ModePicker.Render(" HELP ")
	default:
		modeBadge = theme.ModeNormal.Render(" NORMAL ")
	}

	// 2. File Path & Flags (or Search Prompt if searching)
	var filePathBadge string
	if s.Mode == fsm.StateSearch || s.SearchPrompt != "" {
		prompt := s.SearchPrompt
		if prompt == "" {
			prompt = "/"
		}
		if s.SearchMatchMsg != "" {
			filePathBadge = theme.StatusfilePath.Render(prompt + " " + s.SearchMatchMsg)
		} else {
			filePathBadge = theme.StatusfilePath.Render(prompt)
		}
	} else {
		flags := ""
		if s.IsDirty {
			flags += " [+]"
		}
		if s.IsReadOnly {
			flags += " [RO]"
		}
		pathText := s.FilePath
		if pathText == "" {
			pathText = "[No Name]"
		}
		if s.SearchMatchMsg != "" {
			flags += " " + s.SearchMatchMsg
		}
		filePathBadge = theme.StatusfilePath.Render(pathText + flags)
	}

	// 3. File info (type & encoding)
	fileType := s.FileType
	if fileType == "" {
		fileType = "Plain Text"
	}
	encoding := s.Encoding
	if encoding == "" {
		encoding = "utf-8"
	}
	fileInfo := theme.StatusInfo.Render(fmt.Sprintf("%s | %s", fileType, encoding))

	// 3b. LSP status indicator
	var lspBadge string
	if s.LSPMsg != "" {
		if strings.HasSuffix(s.LSPMsg, ": err") {
			lspBadge = theme.DiagError.Render(fmt.Sprintf(" [%s]", s.LSPMsg))
		} else if strings.HasSuffix(s.LSPMsg, ": starting") {
			lspBadge = theme.DiagWarning.Render(fmt.Sprintf(" [%s]", s.LSPMsg))
		} else {
			lspBadge = theme.StatusInfo.Render(fmt.Sprintf(" [%s]", s.LSPMsg))
		}
	}

	// 4. Cursor position & Percentage
	pct := 0
	if s.TotalLines > 0 {
		pct = int(float64(s.Line+1) / float64(s.TotalLines) * 100)
	}
	posInfo := theme.StatusInfo.Render(fmt.Sprintf("Ln %d, Col %d (%d%%)", s.Line+1, s.Col+1, pct))

	var diagBadge string
	if s.DiagnosticsMsg != "" {
		diagBadge = theme.DiagError.Render(" " + s.DiagnosticsMsg)
	}

	// Left group
	leftSegments := modeBadge + filePathBadge + fileInfo + lspBadge + posInfo + diagBadge
	leftWidth := runewidth.StringWidth(StripAnsi(leftSegments))

	// 5. Agent Status Badge
	var agentBadge string
	if s.AgentStatus != "" {
		if s.AgentBusy {
			agentBadge = theme.StatusAgentBusy.Render(fmt.Sprintf("[Agent: %s]", s.AgentStatus))
		} else {
			agentBadge = theme.StatusAgentIdle.Render(fmt.Sprintf("[Agent: %s]", s.AgentStatus))
		}
	}

	// 6. Pending Key Buffer (e.g. "Space n _")
	var keyBadge string
	if s.KeyBuffer != "" {
		keyBadge = theme.StatusKeyBuffer.Render(s.KeyBuffer)
	}

	// Right group
	rightSegments := agentBadge + keyBadge
	rightWidth := runewidth.StringWidth(StripAnsi(rightSegments))

	if leftWidth+rightWidth > width {
		// 1. Drop fileInfo (least critical on narrow widths)
		leftSegments = modeBadge + filePathBadge + lspBadge + posInfo + diagBadge
		leftWidth = runewidth.StringWidth(StripAnsi(leftSegments))
	}
	if leftWidth+rightWidth > width {
		// 2. Drop diagBadge
		leftSegments = modeBadge + filePathBadge + lspBadge + posInfo
		leftWidth = runewidth.StringWidth(StripAnsi(leftSegments))
	}
	if leftWidth+rightWidth > width {
		// 3. Drop lspBadge
		leftSegments = modeBadge + filePathBadge + posInfo
		leftWidth = runewidth.StringWidth(StripAnsi(leftSegments))
	}
	if leftWidth+rightWidth > width {
		availLeft := width - rightWidth
		if availLeft < 0 {
			availLeft = 0
		}
		leftSegments = ansi.Truncate(leftSegments, availLeft, "")
		leftWidth = runewidth.StringWidth(StripAnsi(leftSegments))
	}

	remaining := width - leftWidth - rightWidth
	if remaining < 0 {
		remaining = 0
	}
	spacer := strings.Repeat(" ", remaining)

	fullLine := ansi.Truncate(leftSegments+spacer+rightSegments, width, "")
	return theme.StatuslineBg.Render(fullLine)
}
