package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Editor.TabWidth != 4 {
		t.Fatalf("expected TabWidth 4, got %d", cfg.Editor.TabWidth)
	}
	if cfg.Editor.KeyboardLayout != "nordic" {
		t.Fatalf("expected KeyboardLayout 'nordic', got %q", cfg.Editor.KeyboardLayout)
	}
	if cfg.Editor.LineNumbers != "absolute" {
		t.Fatalf("expected LineNumbers 'absolute', got %q", cfg.Editor.LineNumbers)
	}
	if cfg.Theme.Name != "terminal" {
		t.Fatalf("expected Theme 'terminal', got %q", cfg.Theme.Name)
	}
	if cfg.Theme.Truecolor != false {
		t.Fatalf("expected Truecolor false by default, got %v", cfg.Theme.Truecolor)
	}
	if _, ok := cfg.LSP.Servers["gopls"]; !ok {
		t.Fatal("expected default gopls server configuration")
	}
	if _, ok := cfg.LSP.Servers["clangd"]; !ok {
		t.Fatal("expected default clangd server configuration")
	}
	if _, ok := cfg.LSP.Servers["jdtls"]; !ok {
		t.Fatal("expected default jdtls server configuration")
	}
}

func TestLoadConfigCustom(t *testing.T) {
	tomlData := `
[editor]
tab_width = 2
line_numbers = "absolute"
keyboard_layout = "iso"

[theme]
name = "nord"
truecolor = true

[theme.colors]
background = "#1a1b26"
foreground = "#c0caf5"

[theme.syntax]
keyword = "#bb9af7"

[markdown]
preview_drawer_percent = 50
`
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(confPath, []byte(tomlData), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(confPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Editor.TabWidth != 2 {
		t.Fatalf("expected TabWidth 2, got %d", cfg.Editor.TabWidth)
	}
	if cfg.Editor.LineNumbers != "absolute" {
		t.Fatalf("expected LineNumbers 'absolute', got %q", cfg.Editor.LineNumbers)
	}
	if cfg.Editor.KeyboardLayout != "iso" {
		t.Fatalf("expected KeyboardLayout 'iso', got %q", cfg.Editor.KeyboardLayout)
	}
	if cfg.Theme.Name != "nord" {
		t.Fatalf("expected Theme 'nord', got %q", cfg.Theme.Name)
	}
	if !cfg.Theme.Truecolor {
		t.Fatalf("expected Truecolor true, got %v", cfg.Theme.Truecolor)
	}
	if cfg.Theme.Colors["background"] != "#1a1b26" {
		t.Fatalf("expected background '#1a1b26', got %q", cfg.Theme.Colors["background"])
	}
	if cfg.Theme.Syntax["keyword"] != "#bb9af7" {
		t.Fatalf("expected keyword '#bb9af7', got %q", cfg.Theme.Syntax["keyword"])
	}
	if cfg.Markdown.PreviewDrawerPercent != 50 {
		t.Fatalf("expected PreviewDrawerPercent 50, got %d", cfg.Markdown.PreviewDrawerPercent)
	}
	// Fallbacks preserved
	if cfg.Editor.Scrolloff != 4 {
		t.Fatalf("expected fallback Scrolloff 4, got %d", cfg.Editor.Scrolloff)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	// Non-existent file
	_, err := LoadConfig("/non/existent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for non-existent config file")
	}

	// Invalid TOML content
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.toml")
	_ = os.WriteFile(badFile, []byte("invalid = = toml"), 0644)
	_, err = LoadConfig(badFile)
	if err == nil {
		t.Fatal("expected decode error for invalid TOML")
	}
}

func TestFindAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configHome := filepath.Join(tmpDir, "config")
	koodiDir := filepath.Join(configHome, "koodi")
	_ = os.MkdirAll(koodiDir, 0755)

	globalConfig := `
[editor]
tab_width = 8
`
	_ = os.WriteFile(filepath.Join(koodiDir, "config.toml"), []byte(globalConfig), 0644)

	t.Setenv("XDG_CONFIG_HOME", configHome)

	cfg := FindAndLoadConfig()
	if cfg.Editor.TabWidth != 8 {
		t.Fatalf("expected TabWidth 8 from XDG_CONFIG_HOME, got %d", cfg.Editor.TabWidth)
	}
}
