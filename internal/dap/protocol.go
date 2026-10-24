package dap

import "encoding/json"

// ProtocolMessage is the base schema for all DAP messages.
type ProtocolMessage struct {
	Seq  int    `json:"seq"`
	Type string `json:"type"` // "request", "response", "event"
}

// Request represents a DAP request from client to adapter.
type Request struct {
	Seq       int         `json:"seq"`
	Type      string      `json:"type"` // "request"
	Command   string      `json:"command"`
	Arguments interface{} `json:"arguments,omitempty"`
}

// Response represents a DAP response from adapter to client.
type Response struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"` // "response"
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// Event represents an asynchronous DAP notification event from adapter.
type Event struct {
	Seq   int             `json:"seq"`
	Type  string          `json:"type"` // "event"
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// InitializeArgs contains client configuration sent in 'initialize' request.
type InitializeArgs struct {
	ClientID             string `json:"clientID"`
	ClientName           string `json:"clientName"`
	AdapterID            string `json:"adapterID"`
	PathFormat           string `json:"pathFormat"` // "path"
	LinesStartAt1        bool   `json:"linesStartAt1"`
	ColumnsStartAt1      bool   `json:"columnsStartAt1"`
	SupportsVariableType bool   `json:"supportsVariableType"`
}

// Capabilities describes features supported by the debug adapter.
type Capabilities struct {
	SupportsConfigurationDoneRequest bool `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints      bool `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints   bool `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers        bool `json:"supportsEvaluateForHovers,omitempty"`
	SupportsStepBack                 bool `json:"supportsStepBack,omitempty"`
	SupportsSetVariable              bool `json:"supportsSetVariable,omitempty"`
	SupportsRestartFrame             bool `json:"supportsRestartFrame,omitempty"`
	SupportsGotoTargetsRequest       bool `json:"supportsGotoTargetsRequest,omitempty"`
	SupportsStepInTargetsRequest     bool `json:"supportsStepInTargetsRequest,omitempty"`
	SupportsCompletionsRequest       bool `json:"supportsCompletionsRequest,omitempty"`
	SupportsModulesRequest           bool `json:"supportsModulesRequest,omitempty"`
	SupportsTerminateRequest         bool `json:"supportsTerminateRequest,omitempty"`
}

// Source represents a source code file location.
type Source struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// SourceBreakpoint describes a breakpoint set in a source file.
type SourceBreakpoint struct {
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
	LogMessage   string `json:"logMessage,omitempty"`
}

// SetBreakpointsArgs specifies arguments for 'setBreakpoints'.
type SetBreakpointsArgs struct {
	Source      Source             `json:"source"`
	Breakpoints []SourceBreakpoint `json:"breakpoints"`
}

// Breakpoint represents the verified or unverified state of a breakpoint.
type Breakpoint struct {
	ID        int    `json:"id,omitempty"`
	Verified  bool   `json:"verified"`
	Message   string `json:"message,omitempty"`
	Source    Source `json:"source,omitempty"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

// SetBreakpointsResponseBody holds the result of 'setBreakpoints'.
type SetBreakpointsResponseBody struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// LaunchArgs contains common launch arguments for debug adapters.
type LaunchArgs map[string]interface{}

// AttachArgs contains common attach arguments for debug adapters.
type AttachArgs map[string]interface{}

// Thread represents a debuggee execution thread.
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ThreadsResponseBody is the body returned by 'threads'.
type ThreadsResponseBody struct {
	Threads []Thread `json:"threads"`
}

// StackTraceArgs specifies parameters for 'stackTrace'.
type StackTraceArgs struct {
	ThreadID   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

// StackFrame describes one frame on a thread's call stack.
type StackFrame struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Source    Source `json:"source,omitempty"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

// StackTraceResponseBody is the body returned by 'stackTrace'.
type StackTraceResponseBody struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

// ScopesArgs specifies parameters for 'scopes'.
type ScopesArgs struct {
	FrameID int `json:"frameId"`
}

// Scope represents a variable scope (e.g. Locals, Globals).
type Scope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
	Expensive          bool   `json:"expensive"`
}

// ScopesResponseBody is the body returned by 'scopes'.
type ScopesResponseBody struct {
	Scopes []Scope `json:"scopes"`
}

// VariablesArgs specifies parameters for 'variables'.
type VariablesArgs struct {
	VariablesReference int `json:"variablesReference"`
}

// Variable represents a single variable in a scope.
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference,omitempty"`
}

// VariablesResponseBody is the body returned by 'variables'.
type VariablesResponseBody struct {
	Variables []Variable `json:"variables"`
}

// Flow control argument types
type ContinueArgs struct {
	ThreadID int `json:"threadId"`
}

type NextArgs struct {
	ThreadID int `json:"threadId"`
}

type StepInArgs struct {
	ThreadID int `json:"threadId"`
}

type StepOutArgs struct {
	ThreadID int `json:"threadId"`
}

type PauseArgs struct {
	ThreadID int `json:"threadId"`
}

type DisconnectArgs struct {
	Restart           bool `json:"restart,omitempty"`
	TerminateDebuggee bool `json:"terminateDebuggee,omitempty"`
}

type TerminateArgs struct {
	Restart bool `json:"restart,omitempty"`
}

// Event body schemas

// StoppedEventBody is payload for the 'stopped' event.
type StoppedEventBody struct {
	Reason            string `json:"reason"` // "breakpoint", "step", "pause", "exception"
	Description       string `json:"description,omitempty"`
	ThreadID          int    `json:"threadId,omitempty"`
	Text              string `json:"text,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped,omitempty"`
}

// OutputEventBody is payload for the 'output' event.
type OutputEventBody struct {
	Category string `json:"category,omitempty"` // "console", "stdout", "stderr"
	Output   string `json:"output"`
}

// ExitedEventBody is payload for the 'exited' event.
type ExitedEventBody struct {
	ExitCode int `json:"exitCode"`
}
