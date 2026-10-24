package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mikolas/koodieditori/internal/config"
)

func TestDetectServer(t *testing.T) {
	cfg := config.DefaultConfig().LSP
	mgr := NewManager(cfg, "/tmp/workspace")

	// Go file
	srv, _, ext, ok := mgr.DetectServer("cmd/koodi/main.go")
	if !ok || srv != "gopls" || ext != "go" {
		t.Errorf("DetectServer(main.go) = (%s, %s, %v), want gopls, go, true", srv, ext, ok)
	}

	// C++ file
	srv, _, ext, ok = mgr.DetectServer("src/engine.cpp")
	if !ok || srv != "clangd" {
		t.Errorf("DetectServer(engine.cpp) = (%s, %s, %v), want clangd", srv, ext, ok)
	}

	// Java file
	srv, _, ext, ok = mgr.DetectServer("src/main/java/App.java")
	if !ok || srv != "jdtls" || ext != "java" {
		t.Errorf("DetectServer(App.java) = (%s, %s, %v), want jdtls", srv, ext, ok)
	}

	// Unknown file
	_, _, _, ok = mgr.DetectServer("readme.txt")
	if ok {
		t.Errorf("DetectServer(readme.txt) should be false")
	}
}

func TestFindRoot(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "pkg", "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create root marker in tmpDir
	marker := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(marker, []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig().LSP
	mgr := NewManager(cfg, tmpDir)

	filePath := filepath.Join(subDir, "file.go")
	root := mgr.FindRoot(filePath, []string{"go.mod"})

	if root != tmpDir {
		t.Errorf("FindRoot() = %q, want %q", root, tmpDir)
	}
}

func TestDiagnosticsStorageAndCounts(t *testing.T) {
	cfg := config.DefaultConfig().LSP
	mgr := NewManager(cfg, "/tmp")

	ch := mgr.SubscribeDiagnostics(10)
	defer mgr.UnsubscribeDiagnostics(ch)

	testURI := "file:///test/main.go"
	diags := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 1, Character: 5}},
			Severity: SeverityError,
			Message:  "error 1",
		},
		{
			Range:    Range{Start: Position{Line: 5, Character: 2}, End: Position{Line: 5, Character: 10}},
			Severity: SeverityWarning,
			Message:  "warning 1",
		},
	}

	mgr.saveDiagnostics(PublishDiagnosticsParams{
		URI:         testURI,
		Diagnostics: diags,
	})

	select {
	case update := <-ch:
		if update.URI != testURI || len(update.Diagnostics) != 2 {
			t.Errorf("unexpected update: %+v", update)
		}
	default:
		t.Fatalf("expected diagnostics update on subscription channel")
	}

	errs, warns, infos := mgr.CountDiagnostics(testURI)
	if errs != 1 || warns != 1 || infos != 0 {
		t.Errorf("CountDiagnostics() = (%d, %d, %d), want (1, 1, 0)", errs, warns, infos)
	}

	totErrs, totWarns, _ := mgr.TotalDiagnosticsCount()
	if totErrs != 1 || totWarns != 1 {
		t.Errorf("TotalDiagnosticsCount() = (%d, %d), want (1, 1)", totErrs, totWarns)
	}
}

func TestLanguageIDForExt(t *testing.T) {
	cases := map[string]string{
		"go":       "go",
		"golang":   "go",
		"c":        "c",
		"cpp":      "cpp",
		"cxx":      "cpp",
		"py":       "python",
		"rs":       "rust",
		"sh":       "shellscript",
		"bash":     "shellscript",
		"zsh":      "shellscript",
		"json":     "json",
		"toml":     "toml",
		"yaml":     "yaml",
		"xml":      "xml",
		"makefile": "makefile",
		"cmake":    "cmake",
		"custom":   "custom",
	}

	for ext, want := range cases {
		got := LanguageIDForExt(ext)
		if got != want {
			t.Errorf("LanguageIDForExt(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestGetClientStatus(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")

	cfg := config.DefaultConfig().LSP
	mgr := NewManager(cfg, tmpDir)

	// 1. Unknown file
	srv, _, running := mgr.GetClientStatus(filepath.Join(tmpDir, "unknown.xyz"))
	if running || srv != "" {
		t.Errorf("GetClientStatus(unknown.xyz) = (%s, %v), want empty, false", srv, running)
	}

	// 2. Go file detected, not yet running
	srv, _, running = mgr.GetClientStatus(filePath)
	if srv != "gopls" || running {
		t.Errorf("GetClientStatus(main.go) = (%s, %v), want gopls, false", srv, running)
	}

	// 3. Client registered and marked ready
	rootDir := mgr.FindRoot(filePath, cfg.Servers["gopls"].Roots)
	key := "gopls:" + rootDir
	client := NewClient("gopls", "gopls", nil, rootDir)
	client.setStatus(StatusReady, "ok")
	mgr.mu.Lock()
	mgr.clients[key] = client
	mgr.mu.Unlock()

	srv, st, running := mgr.GetClientStatus(filePath)
	if srv != "gopls" || !running || st != StatusReady {
		t.Errorf("GetClientStatus(main.go) = (%s, %v, %v), want gopls, StatusReady, true", srv, st, running)
	}

	// 4. Client errored
	client.setStatus(StatusError, "not installed")
	srv, st, running = mgr.GetClientStatus(filePath)
	if srv != "gopls" || !running || st != StatusError {
		t.Errorf("GetClientStatus(main.go) = (%s, %v, %v), want gopls, StatusError, true", srv, st, running)
	}
}
