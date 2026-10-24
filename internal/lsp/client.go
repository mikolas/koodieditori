package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ClientStatus represents the lifecycle state of the LSP client.
type ClientStatus string

const (
	StatusStarting ClientStatus = "starting"
	StatusReady    ClientStatus = "ready"
	StatusError    ClientStatus = "error"
	StatusStopped  ClientStatus = "stopped"
)

// Client manages the subprocess lifecycle and bidirectional JSON-RPC 2.0 communication
// with a language server (e.g. gopls, clangd, jdtls).
type Client struct {
	serverName string
	command    string
	args       []string
	rootDir    string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *TransportWriter

	seq atomic.Int64

	pendingMu sync.Mutex
	pending   map[string]chan *Response

	handlersMu           sync.RWMutex
	notificationHandlers map[string][]func(payload json.RawMessage)

	statusMu     sync.RWMutex
	status       ClientStatus
	statusDetail string

	ready     chan struct{}
	readyOnce sync.Once

	openDocsMu sync.RWMutex
	openDocs   map[string]bool

	capabilities ServerCapabilities
	serverInfo   *ServerInfo

	done chan struct{}
}

// NewClient initializes a new Client instance without starting the process.
func NewClient(serverName, command string, args []string, rootDir string) *Client {
	return &Client{
		serverName:           serverName,
		command:              command,
		args:                 args,
		rootDir:              rootDir,
		pending:              make(map[string]chan *Response),
		notificationHandlers: make(map[string][]func(json.RawMessage)),
		status:               StatusStarting,
		ready:                make(chan struct{}),
		openDocs:             make(map[string]bool),
		done:                 make(chan struct{}),
	}
}

// ServerName returns the configured identifier for this language server (e.g. "gopls").
func (c *Client) ServerName() string {
	return c.serverName
}

// RootDir returns the root directory associated with this language server instance.
func (c *Client) RootDir() string {
	return c.rootDir
}

// Status returns current lifecycle status and any detail message.
func (c *Client) Status() (ClientStatus, string) {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status, c.statusDetail
}

func (c *Client) setStatus(status ClientStatus, detail string) {
	c.statusMu.Lock()
	c.status = status
	c.statusDetail = detail
	c.statusMu.Unlock()
}

// SetStatusForTesting sets the client status for test suites.
func (c *Client) SetStatusForTesting(status ClientStatus, detail string) {
	c.setStatus(status, detail)
}

// Start launches the language server subprocess and begins the I/O read loop.
func (c *Client) Start(ctx context.Context) error {
	cmd := exec.Command(c.command, c.args...)
	if c.rootDir != "" {
		cmd.Dir = c.rootDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.setStatus(StatusError, err.Error())
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		c.setStatus(StatusError, err.Error())
		return fmt.Errorf("stdout pipe: %w", err)
	}

	cmd.Stderr = nil // Ignore or let stderr be discarded to prevent blocking

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		c.setStatus(StatusError, err.Error())
		c.readyOnce.Do(func() {
			close(c.ready)
		})
		return fmt.Errorf("starting %s: %w", c.command, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.writer = NewTransportWriter(stdin)

	// Launch reader loop
	go c.readLoop(bufio.NewReader(stdout))

	// Perform initialize handshake in background
	go func() {
		defer c.readyOnce.Do(func() {
			close(c.ready)
		})
		initCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := c.initialize(initCtx); err != nil {
			c.setStatus(StatusError, err.Error())
		} else {
			c.setStatus(StatusReady, "ok")
		}
	}()

	return nil
}

// WaitReady blocks until the initialize handshake completes or the context expires.
func (c *Client) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		c.statusMu.RLock()
		st := c.status
		detail := c.statusDetail
		c.statusMu.RUnlock()
		if st == StatusError {
			return fmt.Errorf("lsp server error: %s", detail)
		}
		if st == StatusStopped {
			return errors.New("lsp server stopped")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) readLoop(r *bufio.Reader) {
	for {
		select {
		case <-c.done:
			return
		default:
		}

		msg, err := ReadMessage(r)
		if err != nil {
			// Subprocess died or pipe closed
			c.cleanupPending(err)
			c.setStatus(StatusStopped, "connection closed")
			return
		}

		c.handleIncoming(msg)
	}
}

func (c *Client) handleIncoming(msg []byte) {
	// Parse as generic JSON object
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(msg, &raw); err != nil {
		return
	}

	_, hasID := raw["id"]
	methodRaw, hasMethod := raw["method"]

	if hasID && !hasMethod {
		// Response to a client request
		var resp Response
		if err := json.Unmarshal(msg, &resp); err == nil && resp.ID != nil {
			idKey := resp.ID.String()
			c.pendingMu.Lock()
			ch, ok := c.pending[idKey]
			delete(c.pending, idKey)
			c.pendingMu.Unlock()

			if ok {
				ch <- &resp
			}
		}
		return
	}

	if hasID && hasMethod {
		// Server-initiated request (e.g. workspace/configuration)
		// Respond with null result to satisfy JSON-RPC
		var req Request
		if err := json.Unmarshal(msg, &req); err == nil {
			go c.respondSuccess(req.ID, nil)
		}
		return
	}

	if hasMethod && !hasID {
		// Server notification
		var method string
		_ = json.Unmarshal(methodRaw, &method)
		params := raw["params"]

		c.handlersMu.RLock()
		handlers := append([]func(json.RawMessage){}, c.notificationHandlers[method]...)
		wildcards := append([]func(json.RawMessage){}, c.notificationHandlers["*"]...)
		c.handlersMu.RUnlock()

		for _, h := range handlers {
			go h(params)
		}
		for _, h := range wildcards {
			go h(params)
		}
	}
}

func (c *Client) respondSuccess(id ID, result any) {
	resBytes, _ := json.Marshal(result)
	resp := Response{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  resBytes,
	}
	payload, err := json.Marshal(resp)
	if err == nil {
		_ = c.writer.WriteMessage(payload)
	}
}

func (c *Client) cleanupPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for idKey, ch := range c.pending {
		delete(c.pending, idKey)
		id := NewStringID(idKey)
		ch <- &Response{
			JSONRPC: "2.0",
			ID:      &id,
			Error: &ResponseError{
				Code:    -32000,
				Message: err.Error(),
			},
		}
	}
}

// OnNotification registers a callback handler for notifications with the specified method.
func (c *Client) OnNotification(method string, handler func(payload json.RawMessage)) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.notificationHandlers[method] = append(c.notificationHandlers[method], handler)
}

// Call sends a JSON-RPC request and awaits the server response or context cancellation.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	if method != "initialize" {
		if err := c.WaitReady(ctx); err != nil {
			return fmt.Errorf("lsp server not ready (%s): %w", method, err)
		}
	}

	idNum := c.seq.Add(1)
	id := NewIntID(idNum)
	idKey := id.String()

	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshaling params: %w", err)
		}
		paramsRaw = b
	}

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[idKey] = respCh
	c.pendingMu.Unlock()

	if c.writer == nil {
		c.pendingMu.Lock()
		delete(c.pending, idKey)
		c.pendingMu.Unlock()
		return errors.New("client writer is not initialized")
	}

	if err := c.writer.WriteMessage(payload); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, idKey)
		c.pendingMu.Unlock()
		return fmt.Errorf("writing request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, idKey)
		c.pendingMu.Unlock()
		return ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("unmarshaling result: %w", err)
			}
		}
		return nil
	}
}

// Notify sends a one-way JSON-RPC notification.
func (c *Client) Notify(method string, params any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshaling params: %w", err)
		}
		paramsRaw = b
	}

	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}

	if c.writer == nil {
		return errors.New("client writer is not initialized")
	}
	return c.writer.WriteMessage(payload)
}

// initialize performs the LSP initialize -> initialized handshake.
func (c *Client) initialize(ctx context.Context) error {
	pid := os.Getpid()
	rootURI := PathToURI(c.rootDir)

	params := InitializeParams{
		ProcessID: &pid,
		RootURI:   rootURI,
		RootPath:  c.rootDir,
		ClientInfo: &ClientInfo{
			Name:    "koodieditori",
			Version: "0.1.0",
		},
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				Synchronization: &TextDocumentSyncClientCapabilities{
					DidSave: true,
				},
				Completion: &CompletionClientCapabilities{
					DynamicRegistration: true,
				},
				Hover: &HoverClientCapabilities{
					ContentFormat: []string{"markdown", "plaintext"},
				},
				Definition: &DefinitionClientCapabilities{
					LinkSupport: true,
				},
				Implementation: &ImplementationClientCapabilities{
					LinkSupport: true,
				},
			},
		},
	}

	var res InitializeResult
	if err := c.Call(ctx, "initialize", params, &res); err != nil {
		return fmt.Errorf("initialize call failed: %w", err)
	}

	c.capabilities = res.Capabilities
	c.serverInfo = res.ServerInfo

	// Send initialized notification
	if err := c.Notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("sending initialized notification: %w", err)
	}

	return nil
}

// Document Synchronization Helpers

// DidOpen informs the LSP server that a document has been opened.
func (c *Client) DidOpen(uri, languageID, text string, version int) error {
	c.openDocsMu.Lock()
	if c.openDocs[uri] {
		c.openDocsMu.Unlock()
		return nil
	}
	c.openDocs[uri] = true
	c.openDocsMu.Unlock()

	return c.Notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    version,
			Text:       text,
		},
	})
}

// DidChange informs the LSP server of full-document changes.
func (c *Client) DidChange(uri string, version int, fullText string) error {
	return c.Notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     uri,
			Version: version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{
				Text: fullText,
			},
		},
	})
}

// DidSave informs the LSP server that a document was saved.
func (c *Client) DidSave(uri string) error {
	return c.Notify("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: uri,
		},
	})
}

// DidClose informs the LSP server that a document was closed.
func (c *Client) DidClose(uri string) error {
	c.openDocsMu.Lock()
	delete(c.openDocs, uri)
	c.openDocsMu.Unlock()

	return c.Notify("textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: uri,
		},
	})
}

// IsOpen returns whether a document URI is tracked as open by this client.
func (c *Client) IsOpen(uri string) bool {
	c.openDocsMu.RLock()
	defer c.openDocsMu.RUnlock()
	return c.openDocs[uri]
}

// Feature Requests

// GotoDefinition requests the definition location(s) for a symbol at line, col (0-indexed).
func (c *Client) GotoDefinition(ctx context.Context, uri string, line, col int) ([]Location, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}

	var raw json.RawMessage
	if err := c.Call(ctx, "textDocument/definition", params, &raw); err != nil {
		return nil, err
	}

	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Definition can be a single Location, a []Location, or a []LocationLink
	var single Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []Location{single}, nil
	}

	var locs []Location
	if err := json.Unmarshal(raw, &locs); err == nil && len(locs) > 0 {
		return locs, nil
	}

	var links []LocationLink
	if err := json.Unmarshal(raw, &links); err == nil && len(links) > 0 {
		out := make([]Location, 0, len(links))
		for _, link := range links {
			out = append(out, Location{
				URI:   link.TargetURI,
				Range: link.TargetSelectionRange,
			})
		}
		return out, nil
	}

	return nil, nil
}

// GotoImplementation requests implementation location(s) for an interface or symbol.
func (c *Client) GotoImplementation(ctx context.Context, uri string, line, col int) ([]Location, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}

	var raw json.RawMessage
	if err := c.Call(ctx, "textDocument/implementation", params, &raw); err != nil {
		return nil, err
	}

	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var single Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []Location{single}, nil
	}

	var locs []Location
	if err := json.Unmarshal(raw, &locs); err == nil && len(locs) > 0 {
		return locs, nil
	}

	var links []LocationLink
	if err := json.Unmarshal(raw, &links); err == nil && len(links) > 0 {
		out := make([]Location, 0, len(links))
		for _, link := range links {
			out = append(out, Location{
				URI:   link.TargetURI,
				Range: link.TargetSelectionRange,
			})
		}
		return out, nil
	}

	return nil, nil
}

// Hover requests hover documentation at line, col (0-indexed).
func (c *Client) Hover(ctx context.Context, uri string, line, col int) (*Hover, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}

	var hover Hover
	if err := c.Call(ctx, "textDocument/hover", params, &hover); err != nil {
		return nil, err
	}

	if hover.Contents == nil {
		return nil, nil
	}
	return &hover, nil
}

// Completion requests autocomplete suggestions at line, col (0-indexed).
func (c *Client) Completion(ctx context.Context, uri string, line, col int) ([]CompletionItem, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
	}

	var raw json.RawMessage
	if err := c.Call(ctx, "textDocument/completion", params, &raw); err != nil {
		return nil, err
	}

	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Completion response can be []CompletionItem or CompletionList
	var list CompletionList
	if err := json.Unmarshal(raw, &list); err == nil {
		return list.Items, nil
	}

	var items []CompletionItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}

	return nil, nil
}

// CodeAction requests available code actions for a range.
func (c *Client) CodeAction(ctx context.Context, uri string, rng Range, diags []Diagnostic) ([]CodeAction, error) {
	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        rng,
		Context: CodeActionContext{
			Diagnostics: diags,
		},
	}

	var actions []CodeAction
	if err := c.Call(ctx, "textDocument/codeAction", params, &actions); err != nil {
		return nil, err
	}

	return actions, nil
}

// Rename requests a workspace edit to rename a symbol.
func (c *Client) Rename(ctx context.Context, uri string, line, col int, newName string) (*WorkspaceEdit, error) {
	params := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: col},
		NewName:      newName,
	}

	var edit WorkspaceEdit
	if err := c.Call(ctx, "textDocument/rename", params, &edit); err != nil {
		return nil, err
	}

	return &edit, nil
}

// Close gracefully stops the language server process.
func (c *Client) Close() error {
	c.readyOnce.Do(func() {
		close(c.ready)
	})

	c.openDocsMu.Lock()
	c.openDocs = make(map[string]bool)
	c.openDocsMu.Unlock()

	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}

	c.setStatus(StatusStopped, "shutting down")

	// Attempt graceful shutdown if writer is active
	if c.writer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = c.Call(ctx, "shutdown", nil, nil)
		_ = c.Notify("exit", nil)
	}

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// Wait or kill
		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}

	c.cleanupPending(errors.New("client closed"))
	return nil
}
