package ui

import (
	"fmt"
	"math"
	"strings"
)

// GutterInfo encapsulates per-line metadata for gutter rendering.
type GutterInfo struct {
	GitSign     string // "+", "~", "-"
	DiagSign    string // "E", "W", "I", "A"
	Breakpoint  bool   // Breakpoint set on this line
	StoppedLine bool   // Debugger stopped at this line
	IsCurrent   bool
}

// CalculateGutterWidth returns the total gutter width based on total lines.
// Default is 6 characters for lines <= 999: 1 git + 1 diag + 3 digits + 1 sep.
func CalculateGutterWidth(totalLines int) int {
	digits := 3
	if totalLines > 999 {
		digits = len(fmt.Sprintf("%d", totalLines))
	}
	return 2 + digits + 1 // git (1) + diag (1) + digits + sep (1)
}

// RenderGutter renders a single line of the gutter.
func RenderGutter(lineIdx int, cursorLine int, totalLines int, relative bool, info GutterInfo, theme *Theme) string {
	digitsWidth := 3
	if totalLines > 999 {
		digitsWidth = len(fmt.Sprintf("%d", totalLines))
	}

	// 1. Git diff sign (Col 0)
	var gitCol string
	switch info.GitSign {
	case "+":
		gitCol = theme.DiffAdded.Render("+")
	case "~":
		gitCol = theme.DiffModified.Render("~")
	case "-":
		gitCol = theme.DiffDeleted.Render("-")
	default:
		gitCol = " "
	}

	// 2. Diagnostic / Breakpoint / Agent sign (Col 1)
	var diagCol string
	if info.StoppedLine {
		diagCol = theme.OverlayTitle.Render("▶")
	} else if info.Breakpoint {
		diagCol = theme.DiagError.Render("●")
	} else {
		switch info.DiagSign {
		case "E":
			diagCol = theme.DiagError.Render("E")
		case "W":
			diagCol = theme.DiagWarning.Render("W")
		case "I":
			diagCol = theme.DiagInfo.Render("I")
		case "A":
			diagCol = theme.DiagAgentBadge.Render("A")
		default:
			diagCol = " "
		}
	}

	// 3. Line Number (Cols 2..4 or dynamic)
	var numStr string
	if relative {
		if lineIdx == cursorLine {
			numStr = fmt.Sprintf("%*d", digitsWidth, lineIdx+1) // Absolute number on current line
		} else {
			dist := int(math.Abs(float64(lineIdx - cursorLine)))
			numStr = fmt.Sprintf("%*d", digitsWidth, dist)
		}
	} else {
		numStr = fmt.Sprintf("%*d", digitsWidth, lineIdx+1)
	}

	var styledNum string
	if info.IsCurrent {
		styledNum = theme.GutterActive.Render(numStr)
	} else {
		styledNum = theme.GutterNormal.Render(numStr)
	}

	// 4. Separator (Col 5)
	sep := theme.GutterSep.Render(" ")

	return gitCol + diagCol + styledNum + sep
}

// RenderEmptyGutter renders a gutter spacer for lines beyond buffer length.
func RenderEmptyGutter(totalLines int, theme *Theme) string {
	width := CalculateGutterWidth(totalLines)
	tilde := theme.GutterNormal.Render("~")
	pad := width - 1
	if pad < 0 {
		pad = 0
	}
	return tilde + strings.Repeat(" ", pad)
}
