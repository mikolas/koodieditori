package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the complete Koodieditori configuration schema.
type Config struct {
	Editor       EditorConfig                 `toml:"editor"`
	Theme        ThemeConfig                  `toml:"theme"`
	Markdown     MarkdownConfig               `toml:"markdown"`
	OutputDrawer OutputDrawerConfig           `toml:"output_drawer"`
	Bufferline   BufferlineConfig             `toml:"bufferline"`
	Agent        AgentConfig                  `toml:"agent"`
	LSP          LSPConfig                    `toml:"lsp"`
	DAP          DAPConfig                    `toml:"dap"`
	Keys         map[string]map[string]string `toml:"keys"`
}

type EditorConfig struct {
	TabWidth       int    `toml:"tab_width"`
	ExpandTab      bool   `toml:"expand_tab"`
	LineNumbers    string `toml:"line_numbers"` // "relative", "absolute", "none"
	Mouse          bool   `toml:"mouse"`
	Scrolloff      int    `toml:"scrolloff"`
	KeyboardLayout string `toml:"keyboard_layout"` // "nordic", "iso", "ansi"
}

type ThemeConfig struct {
	Name      string            `toml:"name"`      // "terminal" (default), "gruvbox", "nord", "custom"
	Truecolor bool              `toml:"truecolor"` // false = pure ANSI 0-15; true = full 24-bit hex
	Colors    map[string]string `toml:"colors"`    // custom UI color overrides
	Syntax    map[string]string `toml:"syntax"`    // custom syntax color overrides
}

type MarkdownConfig struct {
	PreviewDrawerPercent int    `toml:"preview_drawer_percent"`
	MermaidMode          string `toml:"mermaid_mode"` // "auto", "box_art", "graphic"
	LiveSync             bool   `toml:"live_sync"`
}

type OutputDrawerConfig struct {
	HeightPercent   int    `toml:"height_percent"`
	AutoOpenOnError bool   `toml:"auto_open_on_error"`
	AutoDismissKey  string `toml:"auto_dismiss_key"`
}

type BufferlineConfig struct {
	Show            bool `toml:"show"`
	ShowAgentBadge  bool `toml:"show_agent_badge"`
	ShowDiagnostics bool `toml:"show_diagnostics"`
}

type AgentConfig struct {
	SocketEnabled        bool `toml:"socket_enabled"`
	AutoReloadUnmodified bool `toml:"auto_reload_unmodified"`
	BatchDebounceMs      int  `toml:"batch_debounce_ms"`
}

type LSPConfig struct {
	Servers map[string]LSPServerConfig `toml:"servers"`
}

type LSPServerConfig struct {
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	Roots     []string `toml:"roots"`
	Filetypes []string `toml:"filetypes"`
}

type DAPConfig struct {
	Adapters map[string]DAPAdapterConfig `toml:"adapters"`
}

type DAPAdapterConfig struct {
	Command   string                 `toml:"command"`
	Args      []string               `toml:"args"`
	Filetypes []string               `toml:"filetypes"`
	Launch    map[string]interface{} `toml:"launch"`
}

// DefaultConfig returns the canonical default configuration.
func DefaultConfig() *Config {
	return &Config{
		Editor: EditorConfig{
			TabWidth:       4,
			ExpandTab:      true,
			LineNumbers:    "absolute",
			Mouse:          true,
			Scrolloff:      4,
			KeyboardLayout: "nordic",
		},
		Theme: ThemeConfig{
			Name:      "terminal",
			Truecolor: false,
			Colors:    make(map[string]string),
			Syntax:    make(map[string]string),
		},
		Markdown: MarkdownConfig{
			PreviewDrawerPercent: 45,
			MermaidMode:          "auto",
			LiveSync:             true,
		},
		OutputDrawer: OutputDrawerConfig{
			HeightPercent:   35,
			AutoOpenOnError: true,
			AutoDismissKey:  "Esc",
		},
		Bufferline: BufferlineConfig{
			Show:            true,
			ShowAgentBadge:  true,
			ShowDiagnostics: true,
		},
		Agent: AgentConfig{
			SocketEnabled:        true,
			AutoReloadUnmodified: true,
			BatchDebounceMs:      200,
		},
		LSP: LSPConfig{
			Servers: map[string]LSPServerConfig{
				"gopls": {
					Command:   "gopls",
					Roots:     []string{"go.mod"},
					Filetypes: []string{"go"},
				},
				"clangd": {
					Command:   "clangd",
					Args:      []string{"--background-index", "--clang-tidy"},
					Roots:     []string{"compile_commands.json", "CMakeLists.txt", ".git"},
					Filetypes: []string{"c", "cpp", "h", "hpp"},
				},
				"jdtls": {
					Command:   "jdtls",
					Roots:     []string{"pom.xml", "build.gradle", ".git"},
					Filetypes: []string{"java"},
				},
				"bash-language-server": {
					Command:   "bash-language-server",
					Args:      []string{"start"},
					Roots:     []string{".git"},
					Filetypes: []string{"sh", "bash", "zsh"},
				},
				"vscode-json-language-server": {
					Command:   "vscode-json-language-server",
					Args:      []string{"--stdio"},
					Roots:     []string{"package.json", ".git"},
					Filetypes: []string{"json", "jsonc"},
				},
				"yaml-language-server": {
					Command:   "yaml-language-server",
					Args:      []string{"--stdio"},
					Roots:     []string{".git", "docker-compose.yml"},
					Filetypes: []string{"yaml", "yml"},
				},
				"taplo": {
					Command:   "taplo",
					Args:      []string{"lsp", "stdio"},
					Roots:     []string{"Cargo.toml", ".git"},
					Filetypes: []string{"toml"},
				},
				"lemminx": {
					Command:   "lemminx",
					Roots:     []string{"pom.xml", ".git"},
					Filetypes: []string{"xml"},
				},
				"cmake-language-server": {
					Command:   "cmake-language-server",
					Roots:     []string{"CMakeLists.txt", ".git"},
					Filetypes: []string{"cmake"},
				},
			},
		},
		DAP: DAPConfig{
			Adapters: map[string]DAPAdapterConfig{
				"go": {
					Command:   "dlv",
					Args:      []string{"dap"},
					Filetypes: []string{"go"},
					Launch: map[string]interface{}{
						"mode":    "debug",
						"program": ".",
					},
				},
				"cpp": {
					Command:   "lldb-dap",
					Args:      []string{},
					Filetypes: []string{"c", "cpp", "cc", "cxx", "h", "hpp"},
					Launch: map[string]interface{}{
						"program": "./a.out",
					},
				},
				"java": {
					Command:   "java-debug",
					Args:      []string{},
					Filetypes: []string{"java"},
					Launch: map[string]interface{}{
						"mainClass": "Main",
					},
				},
			},
		},
		Keys: make(map[string]map[string]string),
	}
}

// LoadConfig loads configuration from a specified TOML file.
func LoadConfig(filePath string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// FindAndLoadConfig probes for configuration files in:
// 1. Current directory `.koodi/config.toml` (project override)
// 2. `$XDG_CONFIG_HOME/koodi/config.toml` or `~/.config/koodi/config.toml`
// Falling back to DefaultConfig().
func FindAndLoadConfig() *Config {
	cfg := DefaultConfig()

	// 1. Check user global config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			configHome = filepath.Join(home, ".config")
		}
	}
	if configHome != "" {
		candidates := []string{
			filepath.Join(configHome, "koodi", "koodi.toml"),
			filepath.Join(configHome, "koodi", "config.toml"),
		}
		for _, p := range candidates {
			if data, err := os.ReadFile(p); err == nil {
				_, _ = toml.Decode(string(data), cfg)
				break
			}
		}
	}

	// 2. Check local project config override
	localCandidates := []string{
		filepath.Join(".koodi", "koodi.toml"),
		filepath.Join(".koodi", "config.toml"),
		"koodi.toml",
	}
	for _, p := range localCandidates {
		if data, err := os.ReadFile(p); err == nil {
			_, _ = toml.Decode(string(data), cfg)
			break
		}
	}

	return cfg
}
