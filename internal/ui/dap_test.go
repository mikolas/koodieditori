package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikolas/koodieditori/internal/buffer"
	"github.com/mikolas/koodieditori/internal/dap"
	"github.com/mikolas/koodieditori/internal/fsm"
)

func TestDAPGutterBreakpointAndStoppedLineRendering(t *testing.T) {
	theme := DefaultTheme()
	totalLines := 10

	// 1. Line without breakpoint
	infoNormal := GutterInfo{IsCurrent: false}
	gStrNormal := RenderGutter(2, 0, totalLines, false, infoNormal, theme)
	if strings.Contains(gStrNormal, "●") || strings.Contains(gStrNormal, "▶") {
		t.Errorf("expected clean gutter without breakpoint icon, got %q", gStrNormal)
	}

	// 2. Line with breakpoint (should show '●')
	infoBp := GutterInfo{Breakpoint: true, IsCurrent: false}
	gStrBp := RenderGutter(2, 0, totalLines, false, infoBp, theme)
	if !strings.Contains(gStrBp, "●") {
		t.Errorf("expected breakpoint icon '●' in gutter, got %q", gStrBp)
	}

	// 3. Line where debugger is stopped (should show '▶')
	infoStopped := GutterInfo{StoppedLine: true, IsCurrent: false}
	gStrStopped := RenderGutter(2, 0, totalLines, false, infoStopped, theme)
	if !strings.Contains(gStrStopped, "▶") {
		t.Errorf("expected execution arrow '▶' in gutter, got %q", gStrStopped)
	}

	// 4. Viewport rendering with Breakpoints map and StoppedLine
	buf := NewBufferItem(1, "main.go", "package main\n\nfunc main() {\n\tx := 1\n\ty := 2\n}\n")
	vpParams := ViewportParams{
		Buffer:      buf,
		FSM:         fsm.NewFSM(),
		Width:       40,
		Height:      6,
		TabWidth:    4,
		Theme:       theme,
		Breakpoints: map[int]bool{3: true}, // Line 4 (index 3) has breakpoint
		StoppedLine: 3,                    // Stopped at line 4
	}
	rendered := RenderViewport(vpParams)
	if !strings.Contains(rendered, "▶") {
		t.Errorf("expected rendered viewport to show stopped execution arrow '▶', got:\n%s", rendered)
	}
}

func TestDAPMouseGutterClickToggleBreakpoint(t *testing.T) {
	app := NewApp(nil, []string{"main.go"})
	app.Width = 80
	app.Height = 24

	buf := app.CurrentBuffer()
	_ = buf.PT.Delete(0, buf.PT.Len())
	_ = buf.PT.Insert(0, "line 1\nline 2\nline 3\nline 4\nline 5\n")

	// Click on gutter (X=1, Y=3 -> line index 2, 1-indexed line 3)
	// ViewportTop is 1 (due to Bufferline)
	app.handleMouse(tea.MouseMsg{
		X:      1, // Gutter column
		Y:      3, // viewportTop (1) + line index (2) -> line 3
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if !app.DAPManager.HasBreakpoint("main.go", 3) {
		t.Errorf("expected breakpoint to be set on line 3 after gutter click")
	}

	// Click again on same row to toggle off
	app.handleMouse(tea.MouseMsg{
		X:      1,
		Y:      3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if app.DAPManager.HasBreakpoint("main.go", 3) {
		t.Errorf("expected breakpoint to be cleared on line 3 after second gutter click")
	}
}

func TestDAPKeyboardActions(t *testing.T) {
	app := NewApp(nil, []string{"main.go"})
	buf := app.CurrentBuffer()
	buf.CursorPos = buffer.Position{Line: 4, Col: 0}

	// 1. Toggle Breakpoint via Space d b
	app.FSM.HandleKey("Space")
	act := app.FSM.HandleKey("db")
	if act.Type != fsm.ActionDAPToggleBreakpoint {
		t.Fatalf("expected ActionDAPToggleBreakpoint, got %v", act.Type)
	}
	app.executeAction(act)
	if !app.DAPManager.HasBreakpoint("main.go", 5) {
		t.Errorf("expected breakpoint set on line 5")
	}

	// 2. Toggle Breakpoint via F9
	actF9 := app.FSM.HandleKey("f9")
	if actF9.Type != fsm.ActionDAPToggleBreakpoint {
		t.Fatalf("expected ActionDAPToggleBreakpoint on F9, got %v", actF9.Type)
	}
	app.executeAction(actF9)
	if app.DAPManager.HasBreakpoint("main.go", 5) {
		t.Errorf("expected breakpoint toggled off on line 5 after F9")
	}

	// 3. F5 -> Continue / Start
	actF5 := app.FSM.HandleKey("f5")
	if actF5.Type != fsm.ActionDAPContinue {
		t.Errorf("expected ActionDAPContinue on F5, got %v", actF5.Type)
	}

	// 4. F10 -> Step Over
	actF10 := app.FSM.HandleKey("f10")
	if actF10.Type != fsm.ActionDAPStepOver {
		t.Errorf("expected ActionDAPStepOver on F10, got %v", actF10.Type)
	}

	// 5. F11 -> Step Into
	actF11 := app.FSM.HandleKey("f11")
	if actF11.Type != fsm.ActionDAPStepInto {
		t.Errorf("expected ActionDAPStepInto on F11, got %v", actF11.Type)
	}

	// 6. Shift+F11 / F12 -> Step Out
	actShiftF11 := app.FSM.HandleKey("shift+f11")
	if actShiftF11.Type != fsm.ActionDAPStepOut {
		t.Errorf("expected ActionDAPStepOut on shift+f11, got %v", actShiftF11.Type)
	}

	// 7. Shift+F5 -> Stop
	actShiftF5 := app.FSM.HandleKey("shift+f5")
	if actShiftF5.Type != fsm.ActionDAPStop {
		t.Errorf("expected ActionDAPStop on shift+f5, got %v", actShiftF5.Type)
	}

	// 8. Space d v -> Toggle Debug Drawer
	app.FSM.HandleKey("Space")
	actView := app.FSM.HandleKey("dv")
	if actView.Type != fsm.ActionDAPToggleView {
		t.Fatalf("expected ActionDAPToggleView, got %v", actView.Type)
	}
	app.executeAction(actView)
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeDebug {
		t.Errorf("expected debug drawer to open in DrawerModeDebug")
	}
}

func TestDAPDebugDrawerContent(t *testing.T) {
	app := NewApp(nil, []string{"main.go"})
	app.Width = 80
	app.Height = 24

	// Open debug drawer
	app.executeAction(fsm.Action{Type: fsm.ActionDAPToggleView})
	if !app.Drawer.Open || app.Drawer.Mode != DrawerModeDebug {
		t.Fatalf("expected debug drawer open")
	}

	content := strings.Join(app.Drawer.Content, "\n")
	if !strings.Contains(content, "STATUS:") {
		t.Errorf("expected drawer content to contain 'STATUS:', got:\n%s", content)
	}
	if !strings.Contains(content, "CALL STACK:") {
		t.Errorf("expected drawer content to contain 'CALL STACK:', got:\n%s", content)
	}
	if !strings.Contains(content, "VARIABLES:") {
		t.Errorf("expected drawer content to contain 'VARIABLES:', got:\n%s", content)
	}
	if !strings.Contains(content, "CONTROLS:") {
		t.Errorf("expected drawer content to contain 'CONTROLS:', got:\n%s", content)
	}
}

func TestDAPLanguageAdaptersSupported(t *testing.T) {
	mgr := dap.NewManager(nil)

	// 1. Go
	goLang, goCfg, ok := mgr.DetectAdapter("service/handler.go")
	if !ok || goLang != "go" || goCfg.Command != "dlv" {
		t.Errorf("expected Go adapter dlv, got lang=%s, cfg=%+v, ok=%v", goLang, goCfg, ok)
	}

	// 2. C / C++
	cppLang, cppCfg, ok := mgr.DetectAdapter("native/engine.cpp")
	if !ok || cppLang != "cpp" || cppCfg.Command != "lldb-dap" {
		t.Errorf("expected C++ adapter lldb-dap, got lang=%s, cfg=%+v, ok=%v", cppLang, cppCfg, ok)
	}

	cLang, _, ok := mgr.DetectAdapter("src/main.c")
	if !ok || cLang != "cpp" {
		t.Errorf("expected C adapter matching cpp group, got lang=%s, ok=%v", cLang, ok)
	}

	// 3. Java
	javaLang, javaCfg, ok := mgr.DetectAdapter("src/main/java/App.java")
	if !ok || javaLang != "java" || javaCfg.Command != "java-debug" {
		t.Errorf("expected Java adapter java-debug, got lang=%s, cfg=%+v, ok=%v", javaLang, javaCfg, ok)
	}
}
