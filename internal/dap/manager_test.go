package dap

import (
	"io"
	"testing"
	"time"
)

func TestManagerBreakpoints(t *testing.T) {
	mgr := NewManager(nil)

	file := "/path/to/main.go"

	// Toggle on
	isSet := mgr.ToggleBreakpoint(file, 10)
	if !isSet {
		t.Errorf("expected breakpoint 10 to be set")
	}
	if !mgr.HasBreakpoint(file, 10) {
		t.Errorf("expected HasBreakpoint true for line 10")
	}

	// Toggle second line
	mgr.ToggleBreakpoint(file, 25)

	bps := mgr.GetBreakpoints(file)
	if len(bps) != 2 || bps[0] != 10 || bps[1] != 25 {
		t.Errorf("expected [10, 25], got %v", bps)
	}

	// Toggle off
	isSet = mgr.ToggleBreakpoint(file, 10)
	if isSet {
		t.Errorf("expected breakpoint 10 to be toggled off")
	}
	if mgr.HasBreakpoint(file, 10) {
		t.Errorf("expected HasBreakpoint false for line 10")
	}

	bps = mgr.GetBreakpoints(file)
	if len(bps) != 1 || bps[0] != 25 {
		t.Errorf("expected [25], got %v", bps)
	}
}

func TestManagerAdapterDetection(t *testing.T) {
	mgr := NewManager(nil)

	// Go
	lang, cfg, ok := mgr.DetectAdapter("main.go")
	if !ok || lang != "go" || cfg.Command != "dlv" {
		t.Errorf("expected Go adapter dlv, got lang=%s, cfg=%+v, ok=%v", lang, cfg, ok)
	}

	// C++
	lang, cfg, ok = mgr.DetectAdapter("src/server.cpp")
	if !ok || lang != "cpp" || cfg.Command != "lldb-dap" {
		t.Errorf("expected C++ adapter lldb-dap, got lang=%s, cfg=%+v, ok=%v", lang, cfg, ok)
	}

	// C Header
	lang, _, ok = mgr.DetectAdapter("include/header.h")
	if !ok || lang != "cpp" {
		t.Errorf("expected C++ adapter for .h, got lang=%s, ok=%v", lang, ok)
	}

	// Java
	lang, cfg, ok = mgr.DetectAdapter("App.java")
	if !ok || lang != "java" || cfg.Command != "java-debug" {
		t.Errorf("expected Java adapter java-debug, got lang=%s, cfg=%+v, ok=%v", lang, cfg, ok)
	}

	// Unsupported
	_, _, ok = mgr.DetectAdapter("README.md")
	if ok {
		t.Errorf("expected false for markdown file")
	}
}

func TestManagerSessionWithMockClient(t *testing.T) {
	mgr := NewManager(nil)

	// Setup mock pipes
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	defer clientToServerR.Close()
	defer serverToClientR.Close()

	go runMockAdapter(t, clientToServerR, serverToClientW)

	mockClient := NewClientFromPipes(clientToServerW, serverToClientR)
	defer mockClient.Close()

	mgr.mu.Lock()
	mgr.client = mockClient
	mgr.state = DAPStateRunning
	mgr.mu.Unlock()

	mockClient.OnEvent(mgr.handleDAPEvent)

	// Set a breakpoint
	mgr.ToggleBreakpoint("/tmp/main.go", 42)

	// Simulate stopped event from adapter
	mgr.handleDAPEvent(Event{
		Seq:   200,
		Type:  "event",
		Event: "stopped",
		Body:  []byte(`{"reason": "breakpoint", "threadId": 1}`),
	})

	// Wait for stopped event on EventCh
	var stoppedEv *ManagerEvent
	timeout := time.After(2 * time.Second)
	for stoppedEv == nil {
		select {
		case ev := <-mgr.EventCh:
			if ev.Type == ManagerEventStopped {
				stoppedEv = &ev
			}
		case <-timeout:
			t.Fatalf("timed out waiting for ManagerEventStopped")
		}
	}

	if stoppedEv.StoppedFrame == nil || stoppedEv.StoppedFrame.Line != 42 {
		t.Errorf("expected stopped frame at line 42, got %+v", stoppedEv.StoppedFrame)
	}

	if mgr.State() != DAPStateStopped {
		t.Errorf("expected state STOPPED, got %s", mgr.State())
	}

	// Check stack frames & variables populated
	frames := mgr.StackFrames()
	if len(frames) == 0 {
		t.Errorf("expected stack frames")
	}

	vars := mgr.Variables()
	if len(vars) == 0 || vars[0].Name != "count" {
		t.Errorf("expected variable count, got %+v", vars)
	}

	// Test stepping
	if err := mgr.StepOver(); err != nil {
		t.Errorf("StepOver error: %v", err)
	}
	if err := mgr.StepInto(); err != nil {
		t.Errorf("StepInto error: %v", err)
	}
	if err := mgr.StepOut(); err != nil {
		t.Errorf("StepOut error: %v", err)
	}
	if err := mgr.Continue(); err != nil {
		t.Errorf("Continue error: %v", err)
	}

	// Stop
	if err := mgr.Stop(); err != nil {
		t.Errorf("Stop error: %v", err)
	}
	if mgr.State() != DAPStateInactive {
		t.Errorf("expected state INACTIVE after stop, got %s", mgr.State())
	}
}
