package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mikolas/koodieditori/internal/config"
)

// Theme holds all Lip Gloss styles used across Koodieditori components.
type Theme struct {
	// Mode badges
	ModeNormal lipgloss.Style
	ModeInsert lipgloss.Style
	ModeSelect lipgloss.Style
	ModeLeader lipgloss.Style
	ModeDrawer lipgloss.Style
	ModePicker lipgloss.Style
	ModeSearch lipgloss.Style

	// Search & Replace
	SearchMatch   lipgloss.Style
	SearchCurrent lipgloss.Style

	// Bufferline / Tabs
	TabActive     lipgloss.Style
	TabInactive   lipgloss.Style
	TabDirty      lipgloss.Style
	TabAgentBadge lipgloss.Style
	WorkspaceTag  lipgloss.Style
	BufferlineBg  lipgloss.Style

	// Gutter
	GutterNormal   lipgloss.Style
	GutterActive   lipgloss.Style
	GutterSep      lipgloss.Style
	DiffAdded      lipgloss.Style
	DiffModified   lipgloss.Style
	DiffDeleted    lipgloss.Style
	DiagError      lipgloss.Style
	DiagWarning    lipgloss.Style
	DiagInfo       lipgloss.Style
	DiagAgentBadge lipgloss.Style

	// Viewport & Text
	TextNormal   lipgloss.Style
	TextSelected lipgloss.Style
	CursorBlock  lipgloss.Style
	Whitespace   lipgloss.Style

	// Statusline
	StatuslineBg    lipgloss.Style
	StatusfilePath  lipgloss.Style
	StatusInfo      lipgloss.Style
	StatusAgentIdle lipgloss.Style
	StatusAgentBusy lipgloss.Style
	StatusKeyBuffer lipgloss.Style

	// Borders & Drawers
	BorderLine  lipgloss.Style
	DrawerTitle lipgloss.Style

	// Syntax Highlighting (aligned with VS Code semantic tokens)
	SynKeyword     lipgloss.Style // Control flow: if, else, return, for, switch (VS Code Purple)
	SynDeclaration lipgloss.Style // Declarations: func, package, import, var, type, struct (VS Code Blue)
	SynFunction    lipgloss.Style // Functions & methods: declarations and calls (VS Code Yellow)
	SynType        lipgloss.Style // Types & structs (VS Code Teal)
	SynString      lipgloss.Style // Strings (VS Code Terracotta/Green)
	SynComment     lipgloss.Style // Comments (VS Code Muted Slate)
	SynNumber      lipgloss.Style // Numbers (VS Code Sage Green)
	SynOperator    lipgloss.Style // Operators (Unpainted / Muted)
	SynConstant    lipgloss.Style // Constants: true, false, nil (VS Code Blue)
	SynVariable    lipgloss.Style // Variables & parameters (VS Code Light Blue)

	// Overlays & Floating Popovers
	OverlayBox   lipgloss.Style
	OverlayTitle lipgloss.Style

	// Scrollbar
	ScrollbarTrack lipgloss.Style
	ScrollbarThumb lipgloss.Style
}

// DefaultTheme returns the default theme: native terminal ANSI palette.
func DefaultTheme() *Theme {
	return NewTerminalTheme()
}

// LoadTheme loads a Theme by configuration name and applies custom overrides if specified.
func LoadTheme(cfg config.ThemeConfig) *Theme {
	var t *Theme
	switch strings.ToLower(cfg.Name) {
	case "vscode", "vscode-dark":
		t = NewVSCodeTheme()
	case "gruvbox", "gruvbox-dark":
		t = NewGruvboxTheme()
	case "nord":
		t = NewNordTheme()
	case "custom":
		t = NewCustomTheme(cfg)
		return t
	default: // "terminal", "", or unknown
		t = NewTerminalTheme()
	}

	if len(cfg.Colors) > 0 || len(cfg.Syntax) > 0 {
		applyOverrides(t, cfg.Colors, cfg.Syntax)
	}
	return t
}

// NewTerminalTheme creates a theme that inherits the terminal's native ANSI palette (0-15)
// and default background. It automatically harmonizes with any terminal theme (Kitty, Foot, WezTerm)
// with zero external configuration dependencies.
func NewTerminalTheme() *Theme {
	return &Theme{
		// Modes: clean, bold colored indicators without harsh solid neon background blocks
		ModeNormal: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")), // Green
		ModeInsert: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")), // Blue
		ModeSelect: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")), // Magenta
		ModeLeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Yellow
		ModeDrawer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")), // Cyan
		ModePicker: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Yellow
		ModeSearch: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Yellow

		// Search & Replace
		SearchMatch:   lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")),
		SearchCurrent: lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Underline(true),

		// Tabs
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")), // Bright foreground
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")),             // Muted ANSI 8
		TabDirty:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")),  // Red
		TabAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")),  // Magenta
		WorkspaceTag:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")),             // Muted ANSI 8
		BufferlineBg:  lipgloss.NewStyle(),                                             // Unset: inherit terminal background

		// Gutter: subtle peripheral reference
		GutterNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")), // Muted ANSI 8
		GutterActive:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")),
		GutterSep:      lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		DiffAdded:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")), // Green
		DiffModified:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Yellow
		DiffDeleted:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")), // Red
		DiagError:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")), // Red
		DiagWarning:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Yellow
		DiagInfo:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")), // Blue
		DiagAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")), // Magenta

		// Viewport & Text: universal transparent background with reverse selection
		TextNormal:   lipgloss.NewStyle(),            // Default terminal foreground & background
		TextSelected: lipgloss.NewStyle().Reverse(true),
		CursorBlock:  lipgloss.NewStyle().Reverse(true),
		Whitespace:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")),

		// Statusline: clean, low visual weight
		StatuslineBg:    lipgloss.NewStyle(),
		StatusfilePath:  lipgloss.NewStyle().Bold(true).Padding(0, 1),
		StatusInfo:      lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1),
		StatusAgentIdle: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1),
		StatusAgentBusy: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Padding(0, 1),
		StatusKeyBuffer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Padding(0, 1),

		// Borders & Drawers
		BorderLine:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		DrawerTitle: lipgloss.NewStyle().Bold(true),

		// Syntax: balanced semantic hierarchy aligned with VS Code
		SynKeyword:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")), // Control flow: Magenta (ANSI 5)
		SynDeclaration: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")), // Declarations: Blue (ANSI 4)
		SynFunction:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")), // Functions: Yellow / Gold (ANSI 3)
		SynType:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")), // Types: Cyan / Mint (ANSI 6)
		SynString:      lipgloss.NewStyle().Foreground(lipgloss.Color("2")),             // Strings: Green (ANSI 2)
		SynComment:     lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("8")), // Comments: Muted slate (ANSI 8)
		SynNumber:      lipgloss.NewStyle().Foreground(lipgloss.Color("5")),             // Numbers: Magenta
		SynOperator:    lipgloss.NewStyle(),                                             // Operators: Normal text
		SynConstant:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")), // Constants: Blue (ANSI 4)
		SynVariable:    lipgloss.NewStyle(),                                             // Variables: Normal text

		// Overlays & Floating Popovers
		OverlayBox:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")),
		OverlayTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")),

		// Scrollbar
		ScrollbarTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		ScrollbarThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	}
}

// NewVSCodeTheme creates the classic 24-bit VS Code Dark+ theme.
func NewVSCodeTheme() *Theme {
	return &Theme{
		ModeNormal: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#4ec9b0")).Padding(0, 1),
		ModeInsert: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#569cd6")).Padding(0, 1),
		ModeSelect: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#c586c0")).Padding(0, 1),
		ModeLeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#dcdcaa")).Padding(0, 1),
		ModeDrawer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#ce9178")).Padding(0, 1),
		ModePicker: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#dcdcaa")).Padding(0, 1),
		ModeSearch: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e1e")).Background(lipgloss.Color("#dcdcaa")).Padding(0, 1),

		// Search & Replace
		SearchMatch:   lipgloss.NewStyle().Background(lipgloss.Color("#515c6a")).Foreground(lipgloss.Color("#ffffff")),
		SearchCurrent: lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#ea5c00")).Foreground(lipgloss.Color("#ffffff")),

		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#1e1e1e")).Padding(0, 1),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#969696")).Background(lipgloss.Color("#2d2d2d")).Padding(0, 1),
		TabDirty:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f44747")),
		TabAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c586c0")),
		WorkspaceTag:  lipgloss.NewStyle().Foreground(lipgloss.Color("#6e7681")).Background(lipgloss.Color("#252526")).Padding(0, 1),
		BufferlineBg:  lipgloss.NewStyle().Background(lipgloss.Color("#252526")),

		GutterNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#858585")),
		GutterActive:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c6c6c6")),
		GutterSep:      lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
		DiffAdded:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4ec9b0")),
		DiffModified:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#dcdcaa")),
		DiffDeleted:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f44747")),
		DiagError:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f44747")),
		DiagWarning:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cca700")),
		DiagInfo:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#75beff")),
		DiagAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c586c0")),

		TextNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")),
		TextSelected: lipgloss.NewStyle().Background(lipgloss.Color("#264f78")).Foreground(lipgloss.Color("#ffffff")),
		CursorBlock:  lipgloss.NewStyle().Reverse(true),
		Whitespace:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3b3a32")),

		StatuslineBg:    lipgloss.NewStyle().Background(lipgloss.Color("#007acc")).Foreground(lipgloss.Color("#ffffff")),
		StatusfilePath:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Padding(0, 1),
		StatusInfo:      lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(0, 1),
		StatusAgentIdle: lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")).Padding(0, 1),
		StatusAgentBusy: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#68217a")).Padding(0, 1),
		StatusKeyBuffer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#dcdcaa")).Padding(0, 1),

		BorderLine:  lipgloss.NewStyle().Foreground(lipgloss.Color("#454545")),
		DrawerTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")),

		SynKeyword:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c586c0")), // Control flow: Magenta
		SynDeclaration: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#569cd6")), // Declarations: Blue
		SynFunction:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#dcdcaa")), // Functions: Yellow
		SynType:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4ec9b0")), // Types: Mint/Teal
		SynString:      lipgloss.NewStyle().Foreground(lipgloss.Color("#ce9178")),             // Strings: Terracotta
		SynComment:     lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#6a9955")), // Comments: Muted green
		SynNumber:      lipgloss.NewStyle().Foreground(lipgloss.Color("#b5cea8")),             // Numbers: Sage green
		SynOperator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4")),             // Operators: Subtle foreground
		SynConstant:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#569cd6")), // Constants: Blue
		SynVariable:    lipgloss.NewStyle().Foreground(lipgloss.Color("#9cdcfe")),             // Variables: Light blue

		OverlayBox:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#007acc")).Background(lipgloss.Color("#252526")).Foreground(lipgloss.Color("#cccccc")),
		OverlayTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#007acc")),

		// Scrollbar
		ScrollbarTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("#252526")),
		ScrollbarThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#5a5d5e")),
	}
}

// NewGruvboxTheme creates the classic 24-bit Gruvbox-Dark theme.
func NewGruvboxTheme() *Theme {
	return &Theme{
		ModeNormal: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#b8bb26")).Padding(0, 1),
		ModeInsert: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#83a598")).Padding(0, 1),
		ModeSelect: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#d3869b")).Padding(0, 1),
		ModeLeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#fabd2f")).Padding(0, 1),
		ModeDrawer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#fe8019")).Padding(0, 1),
		ModePicker: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#fabd2f")).Padding(0, 1),
		ModeSearch: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1d2021")).Background(lipgloss.Color("#fabd2f")).Padding(0, 1),

		// Search & Replace
		SearchMatch:   lipgloss.NewStyle().Background(lipgloss.Color("#79740e")).Foreground(lipgloss.Color("#ebdbb2")),
		SearchCurrent: lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#fabd2f")).Foreground(lipgloss.Color("#282828")),

		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebdbb2")).Background(lipgloss.Color("#3c3836")).Padding(0, 1),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")).Background(lipgloss.Color("#1d2021")).Padding(0, 1),
		TabDirty:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fb4934")),
		TabAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d3869b")),
		WorkspaceTag:  lipgloss.NewStyle().Foreground(lipgloss.Color("#665c54")).Background(lipgloss.Color("#1d2021")).Padding(0, 1),
		BufferlineBg:  lipgloss.NewStyle().Background(lipgloss.Color("#1d2021")),

		GutterNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#504945")),
		GutterActive:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebdbb2")),
		GutterSep:      lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836")),
		DiffAdded:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b8bb26")),
		DiffModified:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fabd2f")),
		DiffDeleted:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fb4934")),
		DiagError:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fb4934")),
		DiagWarning:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fabd2f")),
		DiagInfo:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#83a598")),
		DiagAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d3869b")),

		TextNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#ebdbb2")),
		TextSelected: lipgloss.NewStyle().Background(lipgloss.Color("#504945")).Foreground(lipgloss.Color("#fbf1c7")),
		CursorBlock:  lipgloss.NewStyle().Reverse(true),
		Whitespace:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836")),

		StatuslineBg:    lipgloss.NewStyle().Background(lipgloss.Color("#282828")).Foreground(lipgloss.Color("#ebdbb2")),
		StatusfilePath:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebdbb2")).Padding(0, 1),
		StatusInfo:      lipgloss.NewStyle().Foreground(lipgloss.Color("#a89984")).Padding(0, 1),
		StatusAgentIdle: lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")).Padding(0, 1),
		StatusAgentBusy: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d3869b")).Padding(0, 1),
		StatusKeyBuffer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fabd2f")).Padding(0, 1),

		BorderLine:  lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836")),
		DrawerTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebdbb2")),

		SynKeyword:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fb4934")),
		SynDeclaration: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#83a598")),
		SynFunction:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fabd2f")),
		SynType:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8ec07c")),
		SynString:      lipgloss.NewStyle().Foreground(lipgloss.Color("#b8bb26")),
		SynComment:     lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#928374")),
		SynNumber:      lipgloss.NewStyle().Foreground(lipgloss.Color("#d3869b")),
		SynOperator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#fe8019")),
		SynConstant:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d3869b")),
		SynVariable:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ebdbb2")),

		OverlayBox:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#fabd2f")).Background(lipgloss.Color("#282828")).Foreground(lipgloss.Color("#ebdbb2")),
		OverlayTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fabd2f")),

		// Scrollbar
		ScrollbarTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836")),
		ScrollbarThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")),
	}
}

// NewNordTheme creates the cool arctic Nord theme.
func NewNordTheme() *Theme {
	return &Theme{
		ModeNormal: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#a3be8c")).Padding(0, 1),
		ModeInsert: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#88c0d0")).Padding(0, 1),
		ModeSelect: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#b48ead")).Padding(0, 1),
		ModeLeader: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#ebcb8b")).Padding(0, 1),
		ModeDrawer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#d08770")).Padding(0, 1),
		ModePicker: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#ebcb8b")).Padding(0, 1),
		ModeSearch: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2e3440")).Background(lipgloss.Color("#ebcb8b")).Padding(0, 1),

		// Search & Replace
		SearchMatch:   lipgloss.NewStyle().Background(lipgloss.Color("#434c5e")).Foreground(lipgloss.Color("#eceff4")),
		SearchCurrent: lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#ebcb8b")).Foreground(lipgloss.Color("#2e3440")),

		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eceff4")).Background(lipgloss.Color("#434c5e")).Padding(0, 1),
		TabInactive:   lipgloss.NewStyle().Foreground(lipgloss.Color("#d8dee9")).Background(lipgloss.Color("#2e3440")).Padding(0, 1),
		TabDirty:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#bf616a")),
		TabAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b48ead")),
		WorkspaceTag:  lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")).Background(lipgloss.Color("#2e3440")).Padding(0, 1),
		BufferlineBg:  lipgloss.NewStyle().Background(lipgloss.Color("#2e3440")),

		GutterNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),
		GutterActive:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d8dee9")),
		GutterSep:      lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4252")),
		DiffAdded:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a3be8c")),
		DiffModified:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebcb8b")),
		DiffDeleted:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#bf616a")),
		DiagError:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#bf616a")),
		DiagWarning:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebcb8b")),
		DiagInfo:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0d0")),
		DiagAgentBadge: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b48ead")),

		TextNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("#d8dee9")),
		TextSelected: lipgloss.NewStyle().Background(lipgloss.Color("#434c5e")).Foreground(lipgloss.Color("#eceff4")),
		CursorBlock:  lipgloss.NewStyle().Reverse(true),
		Whitespace:   lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4252")),

		StatuslineBg:    lipgloss.NewStyle().Background(lipgloss.Color("#3b4252")).Foreground(lipgloss.Color("#d8dee9")),
		StatusfilePath:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eceff4")).Padding(0, 1),
		StatusInfo:      lipgloss.NewStyle().Foreground(lipgloss.Color("#e5e9f0")).Padding(0, 1),
		StatusAgentIdle: lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")).Padding(0, 1),
		StatusAgentBusy: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b48ead")).Padding(0, 1),
		StatusKeyBuffer: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ebcb8b")).Padding(0, 1),

		BorderLine:  lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),
		DrawerTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eceff4")),

		SynKeyword:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#81a1c1")),
		SynDeclaration: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#81a1c1")),
		SynFunction:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0d0")),
		SynType:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8fbcbb")),
		SynString:      lipgloss.NewStyle().Foreground(lipgloss.Color("#a3be8c")),
		SynComment:     lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#4c566a")),
		SynNumber:      lipgloss.NewStyle().Foreground(lipgloss.Color("#b48ead")),
		SynOperator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#81a1c1")),
		SynConstant:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b48ead")),
		SynVariable:    lipgloss.NewStyle().Foreground(lipgloss.Color("#d8dee9")),

		OverlayBox:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#88c0d0")).Background(lipgloss.Color("#2e3440")).Foreground(lipgloss.Color("#eceff4")),
		OverlayTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0d0")),

		// Scrollbar
		ScrollbarTrack: lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4252")),
		ScrollbarThumb: lipgloss.NewStyle().Foreground(lipgloss.Color("#88c0d0")),
	}
}

// NewCustomTheme creates a custom theme populated from user TOML config values.
func NewCustomTheme(cfg config.ThemeConfig) *Theme {
	t := NewTerminalTheme()
	if cfg.Truecolor {
		t = NewGruvboxTheme()
	}
	applyOverrides(t, cfg.Colors, cfg.Syntax)
	return t
}

func applyOverrides(t *Theme, colors, syntax map[string]string) {
	// UI Colors
	for k, v := range colors {
		if strings.TrimSpace(v) == "" {
			continue
		}
		c := lipgloss.Color(v)
		switch strings.ToLower(k) {
		case "background", "bg":
			t.BufferlineBg = t.BufferlineBg.Background(c)
			t.StatuslineBg = t.StatuslineBg.Background(c)
			t.OverlayBox = t.OverlayBox.Background(c)
		case "foreground", "fg":
			t.TextNormal = t.TextNormal.Foreground(c)
			t.StatusfilePath = t.StatusfilePath.Foreground(c)
			t.OverlayBox = t.OverlayBox.Foreground(c)
		case "cursor":
			t.CursorBlock = lipgloss.NewStyle().Background(c).Reverse(false)
		case "selection_bg", "selection":
			t.TextSelected = t.TextSelected.Background(c)
		case "selection_fg":
			t.TextSelected = t.TextSelected.Foreground(c)
		case "gutter_fg", "gutter":
			t.GutterNormal = t.GutterNormal.Foreground(c)
			t.GutterSep = t.GutterSep.Foreground(c)
		case "gutter_active":
			t.GutterActive = t.GutterActive.Foreground(c)
		case "border":
			t.BorderLine = t.BorderLine.Foreground(c)
			t.OverlayBox = t.OverlayBox.BorderForeground(c)
		case "tab_active_bg":
			t.TabActive = t.TabActive.Background(c)
		case "tab_active_fg":
			t.TabActive = t.TabActive.Foreground(c)
		case "tab_inactive_bg":
			t.TabInactive = t.TabInactive.Background(c)
		case "tab_inactive_fg":
			t.TabInactive = t.TabInactive.Foreground(c)
		case "search_match":
			t.SearchMatch = t.SearchMatch.Background(c)
		case "search_current":
			t.SearchCurrent = t.SearchCurrent.Background(c)
		case "scrollbar_track":
			t.ScrollbarTrack = t.ScrollbarTrack.Foreground(c)
		case "scrollbar_thumb":
			t.ScrollbarThumb = t.ScrollbarThumb.Foreground(c)
		}
	}

	// Syntax Colors
	for k, v := range syntax {
		if strings.TrimSpace(v) == "" {
			continue
		}
		c := lipgloss.Color(v)
		switch strings.ToLower(k) {
		case "keyword":
			t.SynKeyword = t.SynKeyword.Foreground(c)
		case "declaration":
			t.SynDeclaration = t.SynDeclaration.Foreground(c)
		case "function", "func":
			t.SynFunction = t.SynFunction.Foreground(c)
		case "type":
			t.SynType = t.SynType.Foreground(c)
		case "string":
			t.SynString = t.SynString.Foreground(c)
		case "comment":
			t.SynComment = t.SynComment.Foreground(c)
		case "number":
			t.SynNumber = t.SynNumber.Foreground(c)
		case "operator":
			t.SynOperator = t.SynOperator.Foreground(c)
		case "constant":
			t.SynConstant = t.SynConstant.Foreground(c)
		case "variable", "var":
			t.SynVariable = t.SynVariable.Foreground(c)
		}
	}
}
