package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSocketPath returns the standard Unix domain socket path for a session.
func DefaultSocketPath(session string) string {
	if session == "" {
		session = "default"
	}

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		uid := os.Getuid()
		runtimeDir = filepath.Join(os.TempDir(), fmt.Sprintf("koodi-%d", uid))
	}

	koodiDir := filepath.Join(runtimeDir, "koodi")
	return filepath.Join(koodiDir, fmt.Sprintf("koodi-%s.sock", session))
}

// Request is the JSON-RPC-style IPC message sent from client to editor.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the response sent back to the client.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// OpenParams defines parameters for the "open" method.
type OpenParams struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
}

// DiffParams defines parameters for the "diff" method.
type DiffParams struct {
	File string `json:"file"`
}

// ReloadParams defines parameters for the "reload" method.
type ReloadParams struct {
	All  bool   `json:"all,omitempty"`
	File string `json:"file,omitempty"`
}

// NotifyParams defines parameters for the "notify" method.
type NotifyParams struct {
	Message string `json:"message"`
	Level   string `json:"level,omitempty"` // "info", "warn", "error"
}

// StatusBadgeParams defines parameters for the "status-badge" method.
type StatusBadgeParams struct {
	Badge string `json:"badge"`
	Busy  bool   `json:"busy,omitempty"`
}

// ActionParams defines parameters for the "action" method.
type ActionParams struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args,omitempty"`
}
