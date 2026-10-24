package ui

import (
	"strings"
	"testing"

	"github.com/mikolas/koodieditori/internal/config"
	"github.com/mikolas/koodieditori/internal/lsp"
)

func TestThemeDefaults(t *testing.T) {
	theme := DefaultTheme()
	if theme == nil {
		t.Fatal("expected DefaultTheme() to return non-nil")
	}

	// Verify Mode styles
	if theme.ModeNormal.Render("NORMAL") == "" {
		t.Error("expected ModeNormal to render text")
	}
	if theme.ModeInsert.Render("INSERT") == "" {
		t.Error("expected ModeInsert to render text")
	}

	// Verify Syntax styles
	if theme.SynKeyword.Render("if") == "" {
		t.Error("expected SynKeyword to render text")
	}
	if theme.SynDeclaration.Render("func") == "" {
		t.Error("expected SynDeclaration to render text")
	}
	if theme.SynFunction.Render("main") == "" {
		t.Error("expected SynFunction to render text")
	}
	if theme.SynType.Render("int") == "" {
		t.Error("expected SynType to render text")
	}
	if theme.SynComment.Render("// comment") == "" {
		t.Error("expected SynComment to render text")
	}

	// Verify Overlay styles
	if theme.OverlayBox.Render("overlay") == "" {
		t.Error("expected OverlayBox to render text")
	}
	if theme.OverlayTitle.Render("Title") == "" {
		t.Error("expected OverlayTitle to render text")
	}
}

func TestLoadTheme(t *testing.T) {
	tests := []struct {
		name     string
		cfgName  string
		expected string
	}{
		{"default/terminal", "terminal", "terminal"},
		{"empty", "", "terminal"},
		{"vscode", "vscode", "vscode"},
		{"vscode-dark", "vscode-dark", "vscode"},
		{"gruvbox", "gruvbox", "gruvbox"},
		{"gruvbox-dark", "gruvbox-dark", "gruvbox"},
		{"nord", "nord", "nord"},
		{"custom", "custom", "custom"},
		{"unknown", "solarized", "terminal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ThemeConfig{Name: tt.cfgName}
			th := LoadTheme(cfg)
			if th == nil {
				t.Fatalf("LoadTheme(%q) returned nil", tt.cfgName)
			}
			// Check that rendering works
			rendered := th.ModeNormal.Render("TEST")
			if rendered == "" {
				t.Errorf("LoadTheme(%q) failed to render ModeNormal", tt.cfgName)
			}
		})
	}
}

func TestCustomOverrides(t *testing.T) {
	cfg := config.ThemeConfig{
		Name: "custom",
		Colors: map[string]string{
			"background":     "#112233",
			"foreground":     "#aabbcc",
			"border":         "#445566",
			"tab_active_bg":  "#334455",
			"tab_active_fg":  "#ffffff",
			"tab_inactive_bg": "#111122",
			"tab_inactive_fg": "#778899",
			"gutter_fg":      "#556677",
			"gutter_active":  "#ffffff",
			"cursor":         "#ff0055",
			"selection_bg":   "#335577",
			"selection_fg":   "#ffffff",
		},
		Syntax: map[string]string{
			"keyword":  "#ff79c6",
			"function": "#50fa7b",
			"type":     "#8be9fd",
			"string":   "#f1fa8c",
			"comment":  "#6272a4",
			"number":   "#bd93f9",
			"operator": "#ffb86c",
			"constant": "#ff5555",
		},
	}

	th := LoadTheme(cfg)
	if th == nil {
		t.Fatal("expected LoadTheme with custom overrides to return non-nil")
	}

	kw := th.SynKeyword.Render("package")
	if !strings.Contains(kw, "package") {
		t.Errorf("expected SynKeyword to contain 'package', got %q", kw)
	}

	box := th.OverlayBox.Render("content")
	if !strings.Contains(box, "content") {
		t.Errorf("expected OverlayBox to contain 'content', got %q", box)
	}
}

func TestLSPOverlayThemeIntegration(t *testing.T) {
	terminalTheme := NewTerminalTheme()
	gruvboxTheme := NewGruvboxTheme()
	nordTheme := NewNordTheme()

	for _, th := range []*Theme{terminalTheme, gruvboxTheme, nordTheme, nil} {
		// Hover box
		hover := RenderHoverBox("func Println(a ...any) (n int, err error)\nPrintln formats using default formats.", 80, 20, th)
		if !strings.Contains(hover, "Println") {
			t.Errorf("RenderHoverBox missing expected doc text")
		}
		if !strings.Contains(hover, "Hover Doc") {
			t.Errorf("RenderHoverBox missing header title")
		}

		// Completion box
		items := []lsp.CompletionItem{
			{Label: "Println", Detail: "func(a ...any)", Kind: lsp.CompletionFunction},
			{Label: "Printf", Detail: "func(format string, a ...any)", Kind: lsp.CompletionFunction},
		}
		comp := RenderCompletionBox(items, 0, 80, th)
		if !strings.Contains(comp, "Println") || !strings.Contains(comp, "Autocomplete") {
			t.Errorf("RenderCompletionBox missing expected autocomplete content")
		}

		// Code Action box
		actions := []lsp.CodeAction{
			{Title: "Import package fmt", IsPreferred: true},
		}
		ca := RenderCodeActionBox(actions, 0, 80, th)
		if !strings.Contains(ca, "Import package fmt") || !strings.Contains(ca, "Code Actions") {
			t.Errorf("RenderCodeActionBox missing expected code action content")
		}
	}
}

func TestAppLoadsThemeFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Theme.Name = "nord"

	app := NewApp(cfg, nil)
	if app.Theme == nil {
		t.Fatal("expected app.Theme to be initialized")
	}

	// Verify app renders with the configured theme
	normal := app.Theme.ModeNormal.Render("NORMAL")
	if !strings.Contains(normal, "NORMAL") {
		t.Errorf("expected ModeNormal to render, got %q", normal)
	}
}
