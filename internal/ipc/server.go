package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Handler processes incoming IPC requests from external clients.
type Handler interface {
	HandleOpen(params OpenParams) error
	HandleDiff(params DiffParams) error
	HandleReload(params ReloadParams) error
	HandleNotify(params NotifyParams) error
	HandleStatusBadge(params StatusBadgeParams) error
	HandleAction(params ActionParams) error
}

// Server listens on a Unix domain socket and dispatches RPC requests to a Handler.
type Server struct {
	socketPath string
	listener   net.Listener
	handler    Handler
	done       chan struct{}
	mu         sync.Mutex
	closed     bool
}

// NewServer creates and starts an IPC server listening on socketPath.
func NewServer(socketPath string, handler Handler) (*Server, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath("default")
	}

	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove stale socket if it exists
	_ = os.Remove(socketPath)

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket %s: %w", socketPath, err)
	}

	// Restrict permissions to owner only
	_ = os.Chmod(socketPath, 0600)

	s := &Server{
		socketPath: socketPath,
		listener:   l,
		handler:    handler,
		done:       make(chan struct{}),
	}

	go s.acceptLoop()

	return s, nil
}

// SocketPath returns the active socket path.
func (s *Server) SocketPath() string {
	return s.socketPath
}

// Close terminates the server and removes the socket file.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()

	err := s.listener.Close()
	_ = os.Remove(s.socketPath)
	return err
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				return
			}
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			if err != io.EOF {
				_ = encoder.Encode(Response{
					Success: false,
					Error:   fmt.Sprintf("invalid JSON request: %v", err),
				})
			}
			return
		}

		resp := s.dispatch(req)
		if err := encoder.Encode(resp); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(req Request) Response {
	if s.handler == nil {
		return Response{Success: false, Error: "no handler configured"}
	}

	switch req.Method {
	case "open":
		var p OpenParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Success: false, Error: fmt.Sprintf("invalid open params: %v", err)}
		}
		if err := s.handler.HandleOpen(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true, Message: fmt.Sprintf("Opened %s", p.File)}

	case "diff":
		var p DiffParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Success: false, Error: fmt.Sprintf("invalid diff params: %v", err)}
		}
		if err := s.handler.HandleDiff(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true, Message: fmt.Sprintf("Opened diff for %s", p.File)}

	case "reload":
		var p ReloadParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &p)
		}
		if err := s.handler.HandleReload(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true, Message: "Reload complete"}

	case "notify":
		var p NotifyParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Success: false, Error: fmt.Sprintf("invalid notify params: %v", err)}
		}
		if err := s.handler.HandleNotify(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true}

	case "status-badge":
		var p StatusBadgeParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Success: false, Error: fmt.Sprintf("invalid status-badge params: %v", err)}
		}
		if err := s.handler.HandleStatusBadge(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true}

	case "action":
		var p ActionParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return Response{Success: false, Error: fmt.Sprintf("invalid action params: %v", err)}
		}
		if err := s.handler.HandleAction(p); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true}

	default:
		return Response{Success: false, Error: fmt.Sprintf("unknown method %q", req.Method)}
	}
}
