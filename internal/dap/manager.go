package dap

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DAPState represents high-level debugger session state.
type DAPState string

const (
	DAPStateInactive   DAPState = "INACTIVE"
	DAPStateStarting   DAPState = "STARTING"
	DAPStateRunning    DAPState = "RUNNING"
	DAPStateStopped    DAPState = "STOPPED"
	DAPStateTerminated DAPState = "TERMINATED"
)

// AdapterConfig holds command and launch arguments for a specific language adapter.
type AdapterConfig struct {
	Command   string                 `toml:"command"`
	Args      []string               `toml:"args"`
	Filetypes []string               `toml:"filetypes"`
	Launch    map[string]interface{} `toml:"launch"`
}

// ManagerEventType represents notifications sent to UI.
type ManagerEventType string

const (
	ManagerEventStopped            ManagerEventType = "stopped"
	ManagerEventContinued          ManagerEventType = "continued"
	ManagerEventOutput             ManagerEventType = "output"
	ManagerEventTerminated         ManagerEventType = "terminated"
	ManagerEventBreakpointsUpdated ManagerEventType = "breakpoints_updated"
)

// ManagerEvent is sent on EventCh when debugger state changes.
type ManagerEvent struct {
	Type         ManagerEventType
	State        DAPState
	StoppedFrame *StackFrame
	Reason       string
	Message      string
	ThreadID     int
}

// Manager coordinates debug adapters, breakpoints, and session flow.
type Manager struct {
	mu sync.RWMutex

	adapters map[string]AdapterConfig

	client *Client
	state  DAPState

	// Breakpoints: file -> line -> true (1-indexed lines)
	breakpoints map[string]map[int]bool

	activeFile string
	activeDir  string

	currentThreadID int
	stoppedFrame    *StackFrame
	stackFrames     []StackFrame
	variables       []Variable
	outputLogs      []string

	EventCh chan ManagerEvent
}

// NewManager creates a DAP Manager with default language configurations.
func NewManager(customAdapters map[string]AdapterConfig) *Manager {
	adapters := DefaultAdapters()
	for k, v := range customAdapters {
		adapters[k] = v
	}

	return &Manager{
		adapters:    adapters,
		breakpoints: make(map[string]map[int]bool),
		state:       DAPStateInactive,
		EventCh:     make(chan ManagerEvent, 50),
	}
}

// DefaultAdapters provides pre-configured DAP adapters for Go, C/C++, and Java.
func DefaultAdapters() map[string]AdapterConfig {
	return map[string]AdapterConfig{
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
	}
}

// State returns current debugger state.
func (m *Manager) State() DAPState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// StoppedFrame returns currently paused stack frame (or nil).
func (m *Manager) StoppedFrame() *StackFrame {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stoppedFrame == nil {
		return nil
	}
	cp := *m.stoppedFrame
	return &cp
}

// StackFrames returns current thread's stack frames.
func (m *Manager) StackFrames() []StackFrame {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]StackFrame, len(m.stackFrames))
	copy(res, m.stackFrames)
	return res
}

// Variables returns inspected variables of the top frame.
func (m *Manager) Variables() []Variable {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]Variable, len(m.variables))
	copy(res, m.variables)
	return res
}

// OutputLogs returns collected console and debug logs.
func (m *Manager) OutputLogs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]string, len(m.outputLogs))
	copy(res, m.outputLogs)
	return res
}

// ToggleBreakpoint toggles a breakpoint at 1-indexed line for file.
func (m *Manager) ToggleBreakpoint(filePath string, line int) bool {
	m.mu.Lock()
	cleanPath := filepath.Clean(filePath)
	if m.breakpoints[cleanPath] == nil {
		m.breakpoints[cleanPath] = make(map[int]bool)
	}

	exists := m.breakpoints[cleanPath][line]
	var nowSet bool
	if exists {
		delete(m.breakpoints[cleanPath], line)
		if len(m.breakpoints[cleanPath]) == 0 {
			delete(m.breakpoints, cleanPath)
		}
		nowSet = false
	} else {
		m.breakpoints[cleanPath][line] = true
		nowSet = true
	}

	client := m.client
	isActive := (m.state == DAPStateRunning || m.state == DAPStateStopped)
	m.mu.Unlock()

	// If session is active, sync breakpoints immediately
	if client != nil && isActive {
		go m.syncBreakpointsForFile(cleanPath)
	}

	m.emitEvent(ManagerEvent{
		Type:    ManagerEventBreakpointsUpdated,
		State:   m.State(),
		Message: fmt.Sprintf("Breakpoint on %s:%d %s", filepath.Base(cleanPath), line, map[bool]string{true: "set", false: "cleared"}[nowSet]),
	})

	return nowSet
}

// HasBreakpoint checks if a breakpoint exists at 1-indexed line.
func (m *Manager) HasBreakpoint(filePath string, line int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clean := filepath.Clean(filePath)
	if fileBps, ok := m.breakpoints[clean]; ok {
		return fileBps[line]
	}
	return false
}

// GetBreakpoints returns a sorted slice of 1-indexed breakpoint lines for file.
func (m *Manager) GetBreakpoints(filePath string) []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clean := filepath.Clean(filePath)
	var lines []int
	if fileBps, ok := m.breakpoints[clean]; ok {
		for l := range fileBps {
			lines = append(lines, l)
		}
		sort.Ints(lines)
	}
	return lines
}

// DetectAdapter locates the matching AdapterConfig based on file extension.
func (m *Manager) DetectAdapter(filePath string) (string, AdapterConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	for lang, cfg := range m.adapters {
		for _, ft := range cfg.Filetypes {
			if strings.EqualFold(ext, ft) {
				return lang, cfg, true
			}
		}
	}
	return "", AdapterConfig{}, false
}

// Start launches a new debugging session for target file in workspaceDir.
func (m *Manager) Start(filePath, workspaceDir string) error {
	m.mu.Lock()
	if m.state == DAPStateRunning || m.state == DAPStateStopped {
		m.mu.Unlock()
		return fmt.Errorf("a debug session is already active (state: %s)", m.state)
	}

	lang, cfg, ok := m.detectAdapterUnlocked(filePath)
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no DAP adapter configured for file %q", filePath)
	}

	m.activeFile = filepath.Clean(filePath)
	m.activeDir = workspaceDir
	if m.activeDir == "" {
		m.activeDir = filepath.Dir(m.activeFile)
	}

	m.state = DAPStateStarting
	m.stoppedFrame = nil
	m.stackFrames = nil
	m.variables = nil
	m.outputLogs = []string{fmt.Sprintf("Starting %s debugger via %q...", lang, cfg.Command)}

	client := NewClient(cfg.Command, cfg.Args, m.activeDir)
	m.client = client
	m.mu.Unlock()

	client.OnEvent(m.handleDAPEvent)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Start(ctx); err != nil {
		m.mu.Lock()
		m.state = DAPStateTerminated
		m.outputLogs = append(m.outputLogs, fmt.Sprintf("Failed to launch adapter: %v", err))
		m.mu.Unlock()
		return err
	}

	// 1. Initialize
	_, err := client.Initialize(ctx, InitializeArgs{
		ClientID:             "koodieditori",
		ClientName:           "Koodieditori",
		AdapterID:            lang,
		PathFormat:           "path",
		LinesStartAt1:        true,
		ColumnsStartAt1:      true,
		SupportsVariableType: true,
	})
	if err != nil {
		_ = client.Close()
		m.mu.Lock()
		m.state = DAPStateTerminated
		m.mu.Unlock()
		return fmt.Errorf("initialize adapter: %w", err)
	}

	// 2. Sync all current breakpoints
	m.syncAllBreakpoints(ctx)

	// 3. Launch
	launchArgs := make(LaunchArgs)
	for k, v := range cfg.Launch {
		launchArgs[k] = v
	}
	// For Go delve, set program to package dir
	if lang == "go" {
		launchArgs["program"] = filepath.Dir(m.activeFile)
	}

	if err := client.Launch(ctx, launchArgs); err != nil {
		// Some adapters require configurationDone first, try configurationDone if launch fails
		_ = client.ConfigurationDone(ctx)
		_ = client.Launch(ctx, launchArgs)
	} else {
		_ = client.ConfigurationDone(ctx)
	}

	m.mu.Lock()
	m.state = DAPStateRunning
	m.mu.Unlock()

	m.emitEvent(ManagerEvent{
		Type:    ManagerEventContinued,
		State:   DAPStateRunning,
		Message: fmt.Sprintf("%s debugger running", lang),
	})

	return nil
}

func (m *Manager) detectAdapterUnlocked(filePath string) (string, AdapterConfig, bool) {
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	for lang, cfg := range m.adapters {
		for _, ft := range cfg.Filetypes {
			if strings.EqualFold(ext, ft) {
				return lang, cfg, true
			}
		}
	}
	return "", AdapterConfig{}, false
}

func (m *Manager) syncAllBreakpoints(ctx context.Context) {
	m.mu.RLock()
	files := make([]string, 0, len(m.breakpoints))
	for f := range m.breakpoints {
		files = append(files, f)
	}
	m.mu.RUnlock()

	for _, f := range files {
		m.syncBreakpointsForFileWithContext(ctx, f)
	}
}

func (m *Manager) syncBreakpointsForFile(cleanPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.syncBreakpointsForFileWithContext(ctx, cleanPath)
}

func (m *Manager) syncBreakpointsForFileWithContext(ctx context.Context, cleanPath string) {
	m.mu.RLock()
	client := m.client
	lines := make([]SourceBreakpoint, 0)
	if bps, ok := m.breakpoints[cleanPath]; ok {
		for line := range bps {
			lines = append(lines, SourceBreakpoint{Line: line})
		}
	}
	m.mu.RUnlock()

	if client == nil {
		return
	}

	_, _ = client.SetBreakpoints(ctx, SetBreakpointsArgs{
		Source:      Source{Path: cleanPath, Name: filepath.Base(cleanPath)},
		Breakpoints: lines,
	})
}

func (m *Manager) handleDAPEvent(ev Event) {
	switch ev.Event {
	case "stopped":
		var body StoppedEventBody
		if len(ev.Body) > 0 {
			_ = json.Unmarshal(ev.Body, &body)
		}
		m.handleStopped(body)

	case "continued":
		m.mu.Lock()
		m.state = DAPStateRunning
		m.stoppedFrame = nil
		m.mu.Unlock()
		m.emitEvent(ManagerEvent{
			Type:    ManagerEventContinued,
			State:   DAPStateRunning,
			Message: "Running",
		})

	case "output":
		var body OutputEventBody
		if len(ev.Body) > 0 {
			_ = json.Unmarshal(ev.Body, &body)
		}
		out := strings.TrimRight(body.Output, "\r\n")
		if out != "" {
			m.mu.Lock()
			m.outputLogs = append(m.outputLogs, out)
			if len(m.outputLogs) > 500 {
				m.outputLogs = m.outputLogs[len(m.outputLogs)-500:]
			}
			m.mu.Unlock()

			m.emitEvent(ManagerEvent{
				Type:    ManagerEventOutput,
				State:   m.State(),
				Message: out,
			})
		}

	case "terminated", "exited":
		m.mu.Lock()
		m.state = DAPStateTerminated
		m.stoppedFrame = nil
		m.mu.Unlock()
		m.emitEvent(ManagerEvent{
			Type:    ManagerEventTerminated,
			State:   DAPStateTerminated,
			Message: "Debugger terminated",
		})
	}
}

func (m *Manager) handleStopped(body StoppedEventBody) {
	m.mu.Lock()
	m.state = DAPStateStopped
	m.currentThreadID = body.ThreadID
	client := m.client
	m.mu.Unlock()

	var topFrame *StackFrame
	var frames []StackFrame
	var vars []Variable

	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		threadID := body.ThreadID
		if threadID == 0 {
			threads, err := client.Threads(ctx)
			if err == nil && len(threads) > 0 {
				threadID = threads[0].ID
			}
		}

		if threadID != 0 {
			stFrames, err := client.StackTrace(ctx, StackTraceArgs{
				ThreadID:   threadID,
				StartFrame: 0,
				Levels:     20,
			})
			if err == nil && len(stFrames) > 0 {
				frames = stFrames
				topFrame = &stFrames[0]

				scopes, err := client.Scopes(ctx, ScopesArgs{FrameID: topFrame.ID})
				if err == nil && len(scopes) > 0 {
					scopeVars, err := client.Variables(ctx, VariablesArgs{
						VariablesReference: scopes[0].VariablesReference,
					})
					if err == nil {
						vars = scopeVars
					}
				}
			}
		}
	}

	m.mu.Lock()
	m.stoppedFrame = topFrame
	m.stackFrames = frames
	m.variables = vars
	m.mu.Unlock()

	msg := fmt.Sprintf("Stopped (%s)", body.Reason)
	if topFrame != nil {
		msg = fmt.Sprintf("Stopped at %s:%d (%s)", filepath.Base(topFrame.Source.Path), topFrame.Line, body.Reason)
	}

	m.emitEvent(ManagerEvent{
		Type:         ManagerEventStopped,
		State:        DAPStateStopped,
		StoppedFrame: topFrame,
		Reason:       body.Reason,
		Message:      msg,
		ThreadID:     body.ThreadID,
	})
}

// Continue resumes execution.
func (m *Manager) Continue() error {
	m.mu.RLock()
	client := m.client
	threadID := m.currentThreadID
	m.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("no active debugger session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.Continue(ctx, threadID)
}

// StepOver advances to the next line.
func (m *Manager) StepOver() error {
	m.mu.RLock()
	client := m.client
	threadID := m.currentThreadID
	m.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("no active debugger session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.Next(ctx, threadID)
}

// StepInto steps into function calls.
func (m *Manager) StepInto() error {
	m.mu.RLock()
	client := m.client
	threadID := m.currentThreadID
	m.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("no active debugger session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.StepIn(ctx, threadID)
}

// StepOut steps out of the current function.
func (m *Manager) StepOut() error {
	m.mu.RLock()
	client := m.client
	threadID := m.currentThreadID
	m.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("no active debugger session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.StepOut(ctx, threadID)
}

// Pause interrupts and pauses execution.
func (m *Manager) Pause() error {
	m.mu.RLock()
	client := m.client
	threadID := m.currentThreadID
	m.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("no active debugger session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.Pause(ctx, threadID)
}

// Stop ends the debug session.
func (m *Manager) Stop() error {
	m.mu.Lock()
	client := m.client
	m.client = nil
	m.state = DAPStateInactive
	m.stoppedFrame = nil
	m.stackFrames = nil
	m.variables = nil
	m.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}

	m.emitEvent(ManagerEvent{
		Type:    ManagerEventTerminated,
		State:   DAPStateInactive,
		Message: "Debugger stopped",
	})
	return nil
}

func (m *Manager) emitEvent(ev ManagerEvent) {
	select {
	case m.EventCh <- ev:
	default:
	}
}
