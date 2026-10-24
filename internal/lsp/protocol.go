package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

// JSON-RPC 2.0 Base Protocol

// ID represents a JSON-RPC 2.0 request or response identifier (number or string).
type ID struct {
	Num   int64
	Str   string
	IsStr bool
}

func (id ID) String() string {
	if id.IsStr {
		return id.Str
	}
	return strconv.FormatInt(id.Num, 10)
}

func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsStr {
		return json.Marshal(id.Str)
	}
	return json.Marshal(id.Num)
}

func (id *ID) UnmarshalJSON(data []byte) error {
	var num int64
	if err := json.Unmarshal(data, &num); err == nil {
		id.Num = num
		id.IsStr = false
		id.Str = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		id.Str = str
		id.IsStr = true
		id.Num = 0
		return nil
	}
	return fmt.Errorf("invalid json-rpc id: %s", string(data))
}

func NewIntID(n int64) ID {
	return ID{Num: n, IsStr: false}
}

func NewStringID(s string) ID {
	return ID{Str: s, IsStr: true}
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *ID             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ResponseError) Error() string {
	return e.Message
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// LSP Position & Range (0-indexed)

type Position struct {
	Line      int `json:"line"`      // 0-based line
	Character int `json:"character"` // 0-based character offset (UTF-16 code units per LSP spec)
}

// RuneColToUTF16Col converts a 0-based rune column on a line to 0-based UTF-16 code units.
func RuneColToUTF16Col(line string, runeCol int) int {
	if runeCol <= 0 {
		return 0
	}
	utf16Col := 0
	rIdx := 0
	for _, r := range line {
		if rIdx >= runeCol {
			break
		}
		if utf16.IsSurrogate(r) || r > 0xFFFF {
			utf16Col += 2
		} else {
			utf16Col += 1
		}
		rIdx++
	}
	if rIdx < runeCol {
		utf16Col += (runeCol - rIdx)
	}
	return utf16Col
}

// UTF16ColToRuneCol converts a 0-based UTF-16 code unit offset on a line to 0-based rune column.
func UTF16ColToRuneCol(line string, utf16Col int) int {
	if utf16Col <= 0 {
		return 0
	}
	curUTF16 := 0
	rIdx := 0
	for _, r := range line {
		if curUTF16 >= utf16Col {
			return rIdx
		}
		var width int
		if utf16.IsSurrogate(r) || r > 0xFFFF {
			width = 2
		} else {
			width = 1
		}
		if curUTF16+width > utf16Col {
			return rIdx
		}
		curUTF16 += width
		rIdx++
	}
	if curUTF16 < utf16Col {
		rIdx += (utf16Col - curUTF16)
	}
	return rIdx
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// LocationLink is returned by some language servers for definition/implementation.
type LocationLink struct {
	OriginSelectionRange *Range `json:"originSelectionRange,omitempty"`
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

// Diagnostics

type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

func (s DiagnosticSeverity) String() string {
	switch s {
	case SeverityError:
		return "Error"
	case SeverityWarning:
		return "Warning"
	case SeverityInformation:
		return "Info"
	case SeverityHint:
		return "Hint"
	default:
		return "Unknown"
	}
}

type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Documents

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength int    `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// Completion

type CompletionItemKind int

const (
	CompletionText          CompletionItemKind = 1
	CompletionMethod        CompletionItemKind = 2
	CompletionFunction      CompletionItemKind = 3
	CompletionConstructor   CompletionItemKind = 4
	CompletionField         CompletionItemKind = 5
	CompletionVariable      CompletionItemKind = 6
	CompletionClass         CompletionItemKind = 7
	CompletionInterface     CompletionItemKind = 8
	CompletionModule        CompletionItemKind = 9
	CompletionProperty      CompletionItemKind = 10
	CompletionUnit          CompletionItemKind = 11
	CompletionValue         CompletionItemKind = 12
	CompletionEnum          CompletionItemKind = 13
	CompletionKeyword       CompletionItemKind = 14
	CompletionSnippet       CompletionItemKind = 15
	CompletionColor         CompletionItemKind = 16
	CompletionFile          CompletionItemKind = 17
	CompletionReference     CompletionItemKind = 18
	CompletionFolder        CompletionItemKind = 19
	CompletionEnumMember    CompletionItemKind = 20
	CompletionConstant      CompletionItemKind = 21
	CompletionStruct        CompletionItemKind = 22
	CompletionEvent         CompletionItemKind = 23
	CompletionOperator      CompletionItemKind = 24
	CompletionTypeParameter CompletionItemKind = 25
)

func (k CompletionItemKind) Icon() string {
	switch k {
	case CompletionFunction, CompletionMethod:
		return "fn"
	case CompletionVariable, CompletionField, CompletionProperty:
		return "var"
	case CompletionClass, CompletionStruct, CompletionInterface:
		return "type"
	case CompletionKeyword:
		return "kw"
	case CompletionSnippet:
		return "snip"
	case CompletionConstant:
		return "const"
	default:
		return "item"
	}
}

type CompletionItem struct {
	Label         string             `json:"label"`
	Kind          CompletionItemKind `json:"kind,omitempty"`
	Detail        string             `json:"detail,omitempty"`
	Documentation any                `json:"documentation,omitempty"` // string or MarkupContent
	InsertText    string             `json:"insertText,omitempty"`
	FilterText    string             `json:"filterText,omitempty"`
	SortText      string             `json:"sortText,omitempty"`
	TextEdit      *TextEdit          `json:"textEdit,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Hover

type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

type Hover struct {
	Contents any    `json:"contents"` // MarkupContent, MarkedString, or []MarkedString
	Range    *Range `json:"range,omitempty"`
}

func ExtractHoverText(contents any) string {
	if contents == nil {
		return ""
	}
	switch v := contents.(type) {
	case string:
		return v
	case map[string]any:
		if val, ok := v["value"].(string); ok {
			return val
		}
	case []any:
		var parts []string
		for _, item := range v {
			if s := ExtractHoverText(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// TextEdit & WorkspaceEdit

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type OptionalVersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version,omitempty"`
}

type TextDocumentEdit struct {
	TextDocument OptionalVersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                              `json:"edits"`
}

type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []TextDocumentEdit    `json:"documentChanges,omitempty"`
}

// Code Actions

type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
}

type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// Rename

type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// Initialize

type ClientCapabilities struct {
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    *WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

type TextDocumentSyncClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	WillSave            bool `json:"willSave,omitempty"`
	WillSaveWaitUntil   bool `json:"willSaveWaitUntil,omitempty"`
	DidSave             bool `json:"didSave,omitempty"`
}

type TextDocumentClientCapabilities struct {
	Synchronization *TextDocumentSyncClientCapabilities `json:"synchronization,omitempty"`
	Completion      *CompletionClientCapabilities       `json:"completion,omitempty"`
	Hover           *HoverClientCapabilities            `json:"hover,omitempty"`
	Definition      *DefinitionClientCapabilities       `json:"definition,omitempty"`
	Implementation  *ImplementationClientCapabilities   `json:"implementation,omitempty"`
	CodeAction      *CodeActionClientCapabilities       `json:"codeAction,omitempty"`
}

type CompletionClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	CompletionItem      struct {
		SnippetSupport bool `json:"snippetSupport,omitempty"`
	} `json:"completionItem,omitempty"`
}

type HoverClientCapabilities struct {
	ContentFormat []string `json:"contentFormat,omitempty"` // ["markdown", "plaintext"]
}

type DefinitionClientCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

type ImplementationClientCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

type CodeActionClientCapabilities struct {
	CodeActionLiteralSupport struct {
		CodeActionKind struct {
			ValueSet []string `json:"valueSet"`
		} `json:"codeActionKind"`
	} `json:"codeActionLiteralSupport,omitempty"`
}

type WorkspaceClientCapabilities struct {
	ApplyEdit bool `json:"applyEdit,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type InitializeParams struct {
	ProcessID    *int               `json:"processId"`
	RootURI      string             `json:"rootUri,omitempty"`
	RootPath     string             `json:"rootPath,omitempty"`
	Capabilities ClientCapabilities `json:"capabilities"`
	ClientInfo   *ClientInfo        `json:"clientInfo,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ServerCapabilities struct {
	TextDocumentSync   any  `json:"textDocumentSync,omitempty"` // int or TextDocumentSyncOptions
	DefinitionProvider any  `json:"definitionProvider,omitempty"`
	ImplementationProvider any `json:"implementationProvider,omitempty"`
	HoverProvider      any  `json:"hoverProvider,omitempty"`
	CompletionProvider any  `json:"completionProvider,omitempty"`
	CodeActionProvider any  `json:"codeActionProvider,omitempty"`
	RenameProvider     any  `json:"renameProvider,omitempty"`
}

// URI Helpers

// PathToURI converts an absolute or relative filesystem path to a file:// URI.
func PathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// On Unix, abs starts with "/". url.URL with Scheme "file" and Path abs gives "file:///path".
	u := url.URL{
		Scheme: "file",
		Path:   abs,
	}
	return u.String()
}

// URIToPath converts a file:// URI back to a filesystem path.
func URIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	return filepath.Clean(u.Path)
}
