package ui

import (
	"fmt"
	"path/filepath"

	"github.com/mikolas/koodieditori/internal/dap"
	"github.com/mikolas/koodieditori/internal/fsm"
)

// handleDAPEvent processes asynchronous DAP events dispatched from the Manager.
func (a *App) handleDAPEvent(ev dap.ManagerEvent) {
	switch ev.Type {
	case dap.ManagerEventStopped:
		if ev.StoppedFrame != nil && ev.StoppedFrame.Source.Path != "" {
			a.openOrSwitchToFile(ev.StoppedFrame.Source.Path)
			if curr := a.CurrentBuffer(); curr != nil {
				targetLine := ev.StoppedFrame.Line - 1
				if targetLine < 0 {
					targetLine = 0
				}
				curr.CursorPos.Line = targetLine
				curr.CursorPos.Col = 0
			}
		}
		a.StatusMessage = ev.Message
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDebug {
			a.refreshDebugDrawer()
		}

	case dap.ManagerEventContinued:
		a.StatusMessage = "Debuggee running..."
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDebug {
			a.refreshDebugDrawer()
		}

	case dap.ManagerEventOutput:
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDebug {
			a.refreshDebugDrawer()
		}

	case dap.ManagerEventTerminated:
		a.StatusMessage = "Debugger terminated"
		if a.Drawer != nil && a.Drawer.Open && a.Drawer.Mode == DrawerModeDebug {
			a.refreshDebugDrawer()
		}

	case dap.ManagerEventBreakpointsUpdated:
		a.StatusMessage = ev.Message
	}
}

// refreshDebugDrawer updates the drawer's text content with DAP call stack, variables, and logs.
func (a *App) refreshDebugDrawer() {
	if a.Drawer == nil || a.DAPManager == nil {
		return
	}

	state := a.DAPManager.State()
	a.Drawer.Title = fmt.Sprintf("Debugger: [%s] (Esc to close, F5/Space d c continue)", state)

	var lines []string
	lines = append(lines, fmt.Sprintf("  STATUS: [%s]", state))

	stopped := a.DAPManager.StoppedFrame()
	if stopped != nil {
		lines = append(lines, fmt.Sprintf("  ▶ PAUSED AT: %s:%d in %s", filepath.Base(stopped.Source.Path), stopped.Line, stopped.Name))
	}
	lines = append(lines, "")

	// 1. Call Stack
	frames := a.DAPManager.StackFrames()
	lines = append(lines, "CALL STACK:")
	if len(frames) == 0 {
		lines = append(lines, "  (no active frames)")
	} else {
		for i, f := range frames {
			prefix := "    "
			if i == 0 {
				prefix = "  ▶ "
			}
			lines = append(lines, fmt.Sprintf("%s%s (%s:%d)", prefix, f.Name, filepath.Base(f.Source.Path), f.Line))
		}
	}
	lines = append(lines, "")

	// 2. Variables
	vars := a.DAPManager.Variables()
	lines = append(lines, "VARIABLES:")
	if len(vars) == 0 {
		lines = append(lines, "  (no variables in scope)")
	} else {
		for _, v := range vars {
			typeStr := ""
			if v.Type != "" {
				typeStr = fmt.Sprintf(" (%s)", v.Type)
			}
			lines = append(lines, fmt.Sprintf("    %s = %s%s", v.Name, v.Value, typeStr))
		}
	}
	lines = append(lines, "")

	// 3. Console Output Logs
	logs := a.DAPManager.OutputLogs()
	if len(logs) > 0 {
		lines = append(lines, "OUTPUT:")
		start := 0
		if len(logs) > 10 {
			start = len(logs) - 10
		}
		for _, l := range logs[start:] {
			lines = append(lines, "    "+l)
		}
		lines = append(lines, "")
	}

	lines = append(lines, "CONTROLS: [F5/Space d c] Continue  [F10/Space d n] Step Over  [F11/Space d i] Step Into  [Shift+F11/Space d o] Step Out  [Shift+F5/Space d q] Stop")

	a.Drawer.Content = lines
}

// startDebugging launches the appropriate DAP adapter for the active buffer.
func (a *App) startDebugging() {
	curr := a.CurrentBuffer()
	if curr == nil || a.DAPManager == nil {
		return
	}

	if curr.FilePath == "" {
		a.StatusMessage = "Cannot debug unnamed buffer; save file first"
		return
	}

	a.StatusMessage = fmt.Sprintf("Starting debugger for %s...", filepath.Base(curr.FilePath))
	workspaceDir := a.WorkspacePath
	if workspaceDir == "" {
		workspaceDir = filepath.Dir(curr.FilePath)
	}

	// Open debug drawer
	if a.Drawer == nil {
		a.Drawer = NewDrawer()
	}
	a.Drawer.Mode = DrawerModeDebug
	a.Drawer.Open = true
	a.refreshDebugDrawer()
	a.FSM.SetState(fsm.StateDrawer)

	go func() {
		if err := a.DAPManager.Start(curr.FilePath, workspaceDir); err != nil {
			a.mu.Lock()
			a.StatusMessage = fmt.Sprintf("Debug start failed: %v", err)
			a.refreshDebugDrawer()
			a.mu.Unlock()
		}
	}()
}

// continueDebugging resumes execution or launches if inactive.
func (a *App) continueDebugging() {
	if a.DAPManager == nil {
		return
	}
	st := a.DAPManager.State()
	if st == dap.DAPStateInactive || st == dap.DAPStateTerminated {
		a.startDebugging()
		return
	}
	if err := a.DAPManager.Continue(); err != nil {
		a.StatusMessage = fmt.Sprintf("Continue error: %v", err)
	}
}
