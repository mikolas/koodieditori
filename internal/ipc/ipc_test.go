package ipc

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

type mockHandler struct {
	mu           sync.Mutex
	lastOpen     OpenParams
	lastDiff     DiffParams
	lastReload   ReloadParams
	lastNotify   NotifyParams
	lastBadge    StatusBadgeParams
	lastAction   ActionParams
	failWith     error
}

func (m *mockHandler) HandleOpen(p OpenParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastOpen = p
	return nil
}

func (m *mockHandler) HandleDiff(p DiffParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastDiff = p
	return nil
}

func (m *mockHandler) HandleReload(p ReloadParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastReload = p
	return nil
}

func (m *mockHandler) HandleNotify(p NotifyParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastNotify = p
	return nil
}

func (m *mockHandler) HandleStatusBadge(p StatusBadgeParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastBadge = p
	return nil
}

func (m *mockHandler) HandleAction(p ActionParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWith != nil {
		return m.failWith
	}
	m.lastAction = p
	return nil
}

func TestIPCServerAndClient(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "koodi-test.sock")

	handler := &mockHandler{}
	srv, err := NewServer(sockPath, handler)
	if err != nil {
		t.Fatalf("failed to create IPC server: %v", err)
	}
	defer srv.Close()

	client, err := NewClient(sockPath)
	if err != nil {
		t.Fatalf("failed to create IPC client: %v", err)
	}
	defer client.Close()

	// 1. Open
	if err := client.Open("src/main.cpp", 42, 10); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	handler.mu.Lock()
	if handler.lastOpen.File != "src/main.cpp" || handler.lastOpen.Line != 42 || handler.lastOpen.Col != 10 {
		t.Fatalf("unexpected lastOpen: %+v", handler.lastOpen)
	}
	handler.mu.Unlock()

	// 2. Diff
	if err := client.Diff("src/engine.go"); err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	handler.mu.Lock()
	if handler.lastDiff.File != "src/engine.go" {
		t.Fatalf("unexpected lastDiff: %+v", handler.lastDiff)
	}
	handler.mu.Unlock()

	// 3. Reload
	if err := client.Reload(true, ""); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	handler.mu.Lock()
	if !handler.lastReload.All {
		t.Fatalf("unexpected lastReload: %+v", handler.lastReload)
	}
	handler.mu.Unlock()

	// 4. Notify
	if err := client.Notify("Build succeeded", "info"); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}
	handler.mu.Lock()
	if handler.lastNotify.Message != "Build succeeded" || handler.lastNotify.Level != "info" {
		t.Fatalf("unexpected lastNotify: %+v", handler.lastNotify)
	}
	handler.mu.Unlock()

	// 5. StatusBadge
	if err := client.StatusBadge("[Agent: Refactoring...]", true); err != nil {
		t.Fatalf("StatusBadge failed: %v", err)
	}
	handler.mu.Lock()
	if handler.lastBadge.Badge != "[Agent: Refactoring...]" || !handler.lastBadge.Busy {
		t.Fatalf("unexpected lastBadge: %+v", handler.lastBadge)
	}
	handler.mu.Unlock()

	// 6. Action
	if err := client.Action("drawer:toggle", map[string]string{"mode": "markdown"}); err != nil {
		t.Fatalf("Action failed: %v", err)
	}
	handler.mu.Lock()
	if handler.lastAction.Name != "drawer:toggle" || handler.lastAction.Args["mode"] != "markdown" {
		t.Fatalf("unexpected lastAction: %+v", handler.lastAction)
	}
	handler.mu.Unlock()

	// 7. Error handling
	handler.mu.Lock()
	handler.failWith = errors.New("file locked")
	handler.mu.Unlock()

	if err := client.Open("fail.txt", 1, 1); err == nil {
		t.Fatal("expected error, got nil")
	}
}
