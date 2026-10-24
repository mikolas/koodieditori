package treesitter

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/yaml"
)

// Supported language identifiers
const (
	LangGo       = "go"
	LangC        = "c"
	LangCPP      = "cpp"
	LangJava     = "java"
	LangMarkdown = "markdown"
	LangJSON     = "json"
	LangTOML     = "toml"
	LangYAML     = "yaml"
	LangXML      = "xml"
	LangBash     = "bash"
	LangZsh      = "zsh"
	LangMakefile = "makefile"
	LangCMake    = "cmake"
	LangUnknown  = "text"
)

// DetectLanguage returns the Tree-sitter language (if available) and identifier for a given filename.
func DetectLanguage(filePath string) (*sitter.Language, string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	base := strings.ToLower(filepath.Base(filePath))

	// 1. Exact base filename matches
	switch base {
	case "makefile", "gnumakefile":
		return nil, LangMakefile
	case "cmakelists.txt", "cmakecache.txt":
		return nil, LangCMake
	case "cargo.lock":
		return toml.GetLanguage(), LangTOML
	case ".bashrc", ".bash_aliases", ".bash_profile", ".bash_logout":
		return bash.GetLanguage(), LangBash
	case ".zshrc", ".zshenv", ".zlogin", ".zprofile", ".zlogout":
		return bash.GetLanguage(), LangZsh
	case ".clang-format", ".clang-tidy":
		return yaml.GetLanguage(), LangYAML
	case "pom.xml", ".classpath", ".project":
		return nil, LangXML
	case "go.mod", "go.sum":
		return golang.GetLanguage(), LangGo
	}

	// 2. Extension matches
	switch ext {
	case ".go":
		return golang.GetLanguage(), LangGo
	case ".c", ".h":
		return c.GetLanguage(), LangC
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return cpp.GetLanguage(), LangCPP
	case ".java":
		return java.GetLanguage(), LangJava
	case ".md", ".markdown", ".mdown", ".mkd":
		return nil, LangMarkdown
	case ".json", ".jsonc", ".json5", ".ndjson":
		return nil, LangJSON
	case ".toml":
		return toml.GetLanguage(), LangTOML
	case ".yaml", ".yml":
		return yaml.GetLanguage(), LangYAML
	case ".xml", ".plist", ".svg", ".xsd", ".wsdl", ".xaml", ".csproj", ".fsproj", ".vcxproj":
		return nil, LangXML
	case ".sh", ".bash":
		return bash.GetLanguage(), LangBash
	case ".zsh":
		return bash.GetLanguage(), LangZsh
	case ".mk", ".mak", ".make":
		return nil, LangMakefile
	case ".cmake":
		return nil, LangCMake
	default:
		return nil, LangUnknown
	}
}

// DetectLanguageWithContent detects language with optional shebang/first-line inspection.
func DetectLanguageWithContent(filePath string, firstLine string) (*sitter.Language, string) {
	lang, id := DetectLanguage(filePath)
	if id != LangUnknown {
		return lang, id
	}

	trimmed := strings.TrimSpace(firstLine)
	if strings.HasPrefix(trimmed, "#!") {
		switch {
		case strings.Contains(trimmed, "bash"):
			return bash.GetLanguage(), LangBash
		case strings.Contains(trimmed, "zsh"):
			return bash.GetLanguage(), LangZsh
		case strings.Contains(trimmed, "sh"):
			return bash.GetLanguage(), LangBash
		}
	} else if strings.HasPrefix(trimmed, "<?xml") {
		return nil, LangXML
	}

	return nil, LangUnknown
}
