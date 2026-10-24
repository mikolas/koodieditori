package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mikolas/koodieditori/internal/config"
)

// DiagnosticsUpdate contains document diagnostics emitted by a server.
type DiagnosticsUpdate struct {
	URI         string
	FilePath    string
	Diagnostics []Diagnostic
}

// Manager coordinates language servers across different filetypes and workspaces.
type Manager struct {
	cfg          config.LSPConfig
	workspaceDir string

	mu      sync.RWMutex
	clients map[string]*Client // key: "serverName:rootDir"

	diagsMu     sync.RWMutex
	diagnostics map[string][]Diagnostic // key: normalized URI

	listenersMu sync.RWMutex
	listeners   []chan DiagnosticsUpdate
}

// NewManager creates an LSP Manager with given configuration and workspace directory.
func NewManager(cfg config.LSPConfig, workspaceDir string) *Manager {
	if workspaceDir == "" {
		workspaceDir, _ = os.Getwd()
	}
	return &Manager{
		cfg:          cfg,
		workspaceDir: workspaceDir,
		clients:      make(map[string]*Client),
		diagnostics:  make(map[string][]Diagnostic),
	}
}

// SubscribeDiagnostics returns a channel receiving diagnostics updates.
func (m *Manager) SubscribeDiagnostics(bufferSize int) chan DiagnosticsUpdate {
	ch := make(chan DiagnosticsUpdate, bufferSize)
	m.listenersMu.Lock()
	m.listeners = append(m.listeners, ch)
	m.listenersMu.Unlock()
	return ch
}

// UnsubscribeDiagnostics removes a listener channel.
func (m *Manager) UnsubscribeDiagnostics(ch chan DiagnosticsUpdate) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()
	for i, c := range m.listeners {
		if c == ch {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			break
		}
	}
}

func (m *Manager) broadcastDiagnostics(update DiagnosticsUpdate) {
	m.listenersMu.RLock()
	defer m.listenersMu.RUnlock()
	for _, ch := range m.listeners {
		select {
		case ch <- update:
		default:
		}
	}
}

// DetectServer returns the server configuration matching the given file extension/language.
func (m *Manager) DetectServer(filePath string) (serverName string, serverCfg config.LSPServerConfig, ext string, ok bool) {
	ext = strings.TrimPrefix(filepath.Ext(filePath), ".")
	extLower := strings.ToLower(ext)
	baseLower := strings.ToLower(filepath.Base(filePath))

	if baseLower == "cmakelists.txt" || extLower == "cmake" {
		extLower = "cmake"
	} else if baseLower == "makefile" || baseLower == "gnumakefile" || extLower == "mk" {
		extLower = "makefile"
	} else if baseLower == ".bashrc" || baseLower == ".bash_profile" || extLower == "bash" {
		extLower = "bash"
	} else if baseLower == ".zshrc" || baseLower == ".zshenv" || extLower == "zsh" {
		extLower = "zsh"
	}

	for name, s := range m.cfg.Servers {
		for _, ft := range s.Filetypes {
			if strings.EqualFold(ft, extLower) {
				return name, s, extLower, true
			}
		}
	}

	// Also check common aliases
	switch extLower {
	case "golang":
		extLower = "go"
	case "cc", "cxx", "c", "h", "hpp":
		extLower = "cpp"
	case "yml":
		extLower = "yaml"
	case "jsonc", "json5":
		extLower = "json"
	}

	for name, s := range m.cfg.Servers {
		for _, ft := range s.Filetypes {
			if strings.EqualFold(ft, extLower) {
				return name, s, extLower, true
			}
		}
	}

	return "", config.LSPServerConfig{}, extLower, false
}

// LanguageIDForExt maps file extensions or aliases to standard LSP language IDs.
func LanguageIDForExt(ext string) string {
	switch strings.ToLower(ext) {
	case "go", "golang":
		return "go"
	case "c":
		return "c"
	case "cpp", "cc", "cxx", "hpp", "h":
		return "cpp"
	case "java":
		return "java"
	case "rs", "rust":
		return "rust"
	case "py", "python":
		return "python"
	case "sh", "bash", "zsh":
		return "shellscript"
	case "json", "jsonc", "json5":
		return "json"
	case "toml":
		return "toml"
	case "yaml", "yml":
		return "yaml"
	case "xml":
		return "xml"
	case "makefile", "mk":
		return "makefile"
	case "cmake":
		return "cmake"
	case "md", "markdown":
		return "markdown"
	default:
		return ext
	}
}

// FindRoot locates the project root for a given file according to server root markers.
func (m *Manager) FindRoot(filePath string, rootMarkers []string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}

	dir := filepath.Dir(abs)
	for {
		for _, marker := range rootMarkers {
			candidate := filepath.Join(dir, marker)
			if _, err := os.Stat(candidate); err == nil {
				return dir
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}

	if m.workspaceDir != "" {
		return m.workspaceDir
	}
	return filepath.Dir(abs)
}

// GetClientStatus queries the language server status for a given file path without spawning subprocesses.
func (m *Manager) GetClientStatus(filePath string) (serverName string, status ClientStatus, running bool) {
	sName, serverCfg, _, ok := m.DetectServer(filePath)
	if !ok {
		return "", "", false
	}
	serverName = sName

	rootDir := m.FindRoot(filePath, serverCfg.Roots)
	key := fmt.Sprintf("%s:%s", serverName, rootDir)

	m.mu.RLock()
	client, exists := m.clients[key]
	m.mu.RUnlock()

	if !exists || client == nil {
		return serverName, StatusStarting, false
	}

	st, _ := client.Status()
	return serverName, st, true
}

// RegisterClientForTesting registers a client into the manager for testing.
func (m *Manager) RegisterClientForTesting(serverName, rootDir string, client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", serverName, rootDir)
	m.clients[key] = client
}

// GetOrStartClient returns an active LSP client for the file, launching one if necessary.
func (m *Manager) GetOrStartClient(ctx context.Context, filePath string) (*Client, error) {
	serverName, serverCfg, _, ok := m.DetectServer(filePath)
	if !ok {
		return nil, nil // No LSP configured for this file type
	}

	rootDir := m.FindRoot(filePath, serverCfg.Roots)
	key := fmt.Sprintf("%s:%s", serverName, rootDir)

	m.mu.RLock()
	client, exists := m.clients[key]
	m.mu.RUnlock()

	if exists && client != nil {
		st, _ := client.Status()
		if st != StatusError && st != StatusStopped {
			return client, nil
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check after acquire lock
	if client, exists = m.clients[key]; exists && client != nil {
		st, _ := client.Status()
		if st != StatusError && st != StatusStopped {
			return client, nil
		}
	}

	args := append([]string{}, serverCfg.Args...)

	// Specific language server provisions
	if serverName == "jdtls" {
		// Provision isolated data cache under ~/.cache/koodi/jdtls/<hash>
		cacheRoot := filepath.Join(os.TempDir(), "koodi-jdtls")
		if home, err := os.UserHomeDir(); err == nil {
			cacheRoot = filepath.Join(home, ".cache", "koodi", "jdtls")
		}
		h := sha256.Sum256([]byte(rootDir))
		hashStr := hex.EncodeToString(h[:8])
		projectCache := filepath.Join(cacheRoot, hashStr)
		_ = os.MkdirAll(projectCache, 0755)

		// Check if -data already present in args
		hasData := false
		for _, a := range args {
			if a == "-data" {
				hasData = true
				break
			}
		}
		if !hasData {
			args = append(args, "-data", projectCache)
		}
	}

	client = NewClient(serverName, serverCfg.Command, args, rootDir)

	// Register publishDiagnostics handler
	client.OnNotification("textDocument/publishDiagnostics", func(payload json.RawMessage) {
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(payload, &params); err == nil {
			m.saveDiagnostics(params)
		}
	})

	if err := client.Start(ctx); err != nil {
		m.clients[key] = client
		return nil, err
	}

	m.clients[key] = client
	return client, nil
}

// SaveDiagnostics explicitly stores diagnostics for a given URI or file path and broadcasts updates.
func (m *Manager) SaveDiagnostics(filePathOrURI string, diags []Diagnostic) {
	uri := filePathOrURI
	if !strings.HasPrefix(filePathOrURI, "file://") {
		uri = PathToURI(filePathOrURI)
	}
	m.saveDiagnostics(PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

// StoreDiagnostics stores diagnostics for a URI without re-broadcasting to listeners.
func (m *Manager) StoreDiagnostics(filePathOrURI string, diags []Diagnostic) {
	uri := filePathOrURI
	if !strings.HasPrefix(filePathOrURI, "file://") {
		uri = PathToURI(filePathOrURI)
	}
	m.diagsMu.Lock()
	m.diagnostics[uri] = diags
	m.diagsMu.Unlock()
}

func (m *Manager) saveDiagnostics(params PublishDiagnosticsParams) {
	m.diagsMu.Lock()
	prev := m.diagnostics[params.URI]
	if diagnosticsEqual(prev, params.Diagnostics) {
		m.diagsMu.Unlock()
		return
	}
	m.diagnostics[params.URI] = params.Diagnostics
	m.diagsMu.Unlock()

	fp := URIToPath(params.URI)
	m.broadcastDiagnostics(DiagnosticsUpdate{
		URI:         params.URI,
		FilePath:    fp,
		Diagnostics: params.Diagnostics,
	})
}

func diagnosticsEqual(a, b []Diagnostic) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Range != b[i].Range ||
			a[i].Severity != b[i].Severity ||
			a[i].Message != b[i].Message ||
			a[i].Source != b[i].Source ||
			fmt.Sprint(a[i].Code) != fmt.Sprint(b[i].Code) {
			return false
		}
	}
	return true
}

// GetDiagnostics returns stored diagnostics for a file URI or path.
func (m *Manager) GetDiagnostics(filePathOrURI string) []Diagnostic {
	uri := filePathOrURI
	if !strings.HasPrefix(filePathOrURI, "file://") {
		uri = PathToURI(filePathOrURI)
	}

	m.diagsMu.RLock()
	defer m.diagsMu.RUnlock()

	diags := m.diagnostics[uri]
	if len(diags) == 0 {
		return nil
	}
	out := make([]Diagnostic, len(diags))
	copy(out, diags)
	return out
}

// CountDiagnostics counts errors, warnings, and info items for a file.
func (m *Manager) CountDiagnostics(filePathOrURI string) (errors, warnings, infos int) {
	diags := m.GetDiagnostics(filePathOrURI)
	for _, d := range diags {
		switch d.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		default:
			infos++
		}
	}
	return errors, warnings, infos
}

// TotalDiagnosticsCount returns the total counts across all tracked files.
func (m *Manager) TotalDiagnosticsCount() (errors, warnings, infos int) {
	m.diagsMu.RLock()
	defer m.diagsMu.RUnlock()

	for _, list := range m.diagnostics {
		for _, d := range list {
			switch d.Severity {
			case SeverityError:
				errors++
			case SeverityWarning:
				warnings++
			default:
				infos++
			}
		}
	}
	return errors, warnings, infos
}

// Document Synchronization

// DidOpen notifies the appropriate server that a document was opened.
func (m *Manager) DidOpen(ctx context.Context, filePath, text string, version int) error {
	client, err := m.GetOrStartClient(ctx, filePath)
	if err != nil || client == nil {
		return err
	}
	if err := client.WaitReady(ctx); err != nil {
		return err
	}
	_, _, ext, _ := m.DetectServer(filePath)
	langID := LanguageIDForExt(ext)
	return client.DidOpen(PathToURI(filePath), langID, text, version)
}

// DidChange notifies the server of document changes.
func (m *Manager) DidChange(ctx context.Context, filePath string, version int, fullText string) error {
	client, err := m.GetOrStartClient(ctx, filePath)
	if err != nil || client == nil {
		return err
	}
	if err := client.WaitReady(ctx); err != nil {
		return err
	}
	uri := PathToURI(filePath)
	if !client.IsOpen(uri) {
		_, _, ext, _ := m.DetectServer(filePath)
		langID := LanguageIDForExt(ext)
		_ = client.DidOpen(uri, langID, fullText, version)
		return nil
	}
	return client.DidChange(uri, version, fullText)
}

// DidSave notifies the server that a document was saved.
func (m *Manager) DidSave(ctx context.Context, filePath string) error {
	client, err := m.GetOrStartClient(ctx, filePath)
	if err != nil || client == nil {
		return err
	}
	if err := client.WaitReady(ctx); err != nil {
		return err
	}
	return client.DidSave(PathToURI(filePath))
}

// DidClose notifies the server that a document was closed.
func (m *Manager) DidClose(ctx context.Context, filePath string) error {
	client, err := m.GetOrStartClient(ctx, filePath)
	if err != nil || client == nil {
		return err
	}
	return client.DidClose(PathToURI(filePath))
}

// StopAll stops all active LSP clients.
func (m *Manager) StopAll() {
	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.clients = make(map[string]*Client)
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			_ = client.Close()
		}(c)
	}
	wg.Wait()
}
