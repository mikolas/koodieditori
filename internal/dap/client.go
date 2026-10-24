package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ClientStatus represents lifecycle status of the debug adapter client.
type ClientStatus string

const (
	StatusStarting   ClientStatus = "starting"
	StatusReady      ClientStatus = "ready"
	StatusStopped    ClientStatus = "stopped"
	StatusTerminated ClientStatus = "terminated"
	StatusError      ClientStatus = "error"
)

// EventHandler is invoked whenever an asynchronous DAP event is received.
type EventHandler func(event Event)

// Client manages the subprocess lifecycle and bidirectional DAP communication.
type Client struct {
	command string
	args    []string
	dir     string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *TransportWriter

	seq atomic.Int32

	pendingMu sync.Mutex
	pending   map[int]chan *Response

	handlersMu    sync.RWMutex
	eventHandlers []EventHandler

	statusMu     sync.RWMutex
	status       ClientStatus
	statusDetail string

	capabilities Capabilities

	done chan struct{}
}

// NewClient initializes a new Client instance for an adapter command.
func NewClient(command string, args []string, dir string) *Client {
	return &Client{
		command: command,
		args:    args,
		dir:     dir,
		pending: make(map[int]chan *Response),
		status:  StatusStarting,
		done:    make(chan struct{}),
	}
}

// NewClientFromPipes initializes a Client with pre-configured pipes (useful for tests/mocking).
func NewClientFromPipes(stdin io.WriteCloser, stdout io.Reader) *Client {
	c := &Client{
		stdin:   stdin,
		writer:  NewTransportWriter(stdin),
		pending: make(map[int]chan *Response),
		status:  StatusReady,
		done:    make(chan struct{}),
	}
	go c.readLoop(bufio.NewReader(stdout))
	return c
}

// Status returns current lifecycle status.
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

// Capabilities returns adapter capabilities established during initialize.
func (c *Client) Capabilities() Capabilities {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.capabilities
}

// OnEvent registers an event handler for DAP notifications.
func (c *Client) OnEvent(handler EventHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.eventHandlers = append(c.eventHandlers, handler)
}

// Start launches the adapter subprocess and begins the message processing loop.
func (c *Client) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	if c.dir != "" {
		cmd.Dir = c.dir
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

	cmd.Stderr = nil // Discard stderr to prevent pipe deadlocks

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		c.setStatus(StatusError, err.Error())
		return fmt.Errorf("start adapter %q: %w", c.command, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.writer = NewTransportWriter(stdin)
	c.setStatus(StatusReady, "Adapter process running")

	go c.readLoop(bufio.NewReader(stdout))

	return nil
}

func (c *Client) readLoop(r *bufio.Reader) {
	defer func() {
		c.setStatus(StatusTerminated, "Adapter I/O closed")
		c.pendingMu.Lock()
		for seq, ch := range c.pending {
			close(ch)
			delete(c.pending, seq)
		}
		c.pendingMu.Unlock()
		select {
		case <-c.done:
		default:
			close(c.done)
		}
	}()

	for {
		raw, err := ReadMessage(r)
		if err != nil {
			return
		}

		var base ProtocolMessage
		if err := json.Unmarshal(raw, &base); err != nil {
			continue
		}

		switch base.Type {
		case "response":
			var resp Response
			if err := json.Unmarshal(raw, &resp); err == nil {
				c.pendingMu.Lock()
				ch, ok := c.pending[resp.RequestSeq]
				if ok {
					delete(c.pending, resp.RequestSeq)
				}
				c.pendingMu.Unlock()
				if ok && ch != nil {
					select {
					case ch <- &resp:
					default:
					}
				}
			}

		case "event":
			var ev Event
			if err := json.Unmarshal(raw, &ev); err == nil {
				c.handlersMu.RLock()
				handlers := append([]EventHandler(nil), c.eventHandlers...)
				c.handlersMu.RUnlock()

				for _, h := range handlers {
					go h(ev)
				}
			}
		}
	}
}

// SendRequest sends a DAP request and waits for its corresponding response.
func (c *Client) SendRequest(ctx context.Context, command string, args interface{}) (*Response, error) {
	if c.writer == nil {
		return nil, errors.New("debug adapter client is not running")
	}

	seq := int(c.seq.Add(1))
	req := Request{
		Seq:       seq,
		Type:      "request",
		Command:   command,
		Arguments: args,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[seq] = respCh
	c.pendingMu.Unlock()

	if err := c.writer.WriteMessage(data); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write message: %w", err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, ctx.Err()

	case resp, ok := <-respCh:
		if !ok || resp == nil {
			return nil, errors.New("debug adapter closed connection while waiting for response")
		}
		if !resp.Success {
			return resp, fmt.Errorf("command %q failed: %s", command, resp.Message)
		}
		return resp, nil
	}
}

// Initialize performs DAP handshake.
func (c *Client) Initialize(ctx context.Context, args InitializeArgs) (*Capabilities, error) {
	resp, err := c.SendRequest(ctx, "initialize", args)
	if err != nil {
		return nil, err
	}
	var caps Capabilities
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &caps)
	}
	c.statusMu.Lock()
	c.capabilities = caps
	c.statusMu.Unlock()
	return &caps, nil
}

// Launch sends a 'launch' request.
func (c *Client) Launch(ctx context.Context, args LaunchArgs) error {
	_, err := c.SendRequest(ctx, "launch", args)
	return err
}

// Attach sends an 'attach' request.
func (c *Client) Attach(ctx context.Context, args AttachArgs) error {
	_, err := c.SendRequest(ctx, "attach", args)
	return err
}

// SetBreakpoints sets breakpoints for a source file.
func (c *Client) SetBreakpoints(ctx context.Context, args SetBreakpointsArgs) ([]Breakpoint, error) {
	resp, err := c.SendRequest(ctx, "setBreakpoints", args)
	if err != nil {
		return nil, err
	}
	var body SetBreakpointsResponseBody
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	return body.Breakpoints, nil
}

// ConfigurationDone informs adapter that client finished setting initial configuration.
func (c *Client) ConfigurationDone(ctx context.Context) error {
	_, err := c.SendRequest(ctx, "configurationDone", struct{}{})
	return err
}

// Threads retrieves active threads from debuggee.
func (c *Client) Threads(ctx context.Context) ([]Thread, error) {
	resp, err := c.SendRequest(ctx, "threads", struct{}{})
	if err != nil {
		return nil, err
	}
	var body ThreadsResponseBody
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	return body.Threads, nil
}

// StackTrace retrieves stack frames for a thread.
func (c *Client) StackTrace(ctx context.Context, args StackTraceArgs) ([]StackFrame, error) {
	resp, err := c.SendRequest(ctx, "stackTrace", args)
	if err != nil {
		return nil, err
	}
	var body StackTraceResponseBody
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	return body.StackFrames, nil
}

// Scopes retrieves variable scopes for a stack frame.
func (c *Client) Scopes(ctx context.Context, args ScopesArgs) ([]Scope, error) {
	resp, err := c.SendRequest(ctx, "scopes", args)
	if err != nil {
		return nil, err
	}
	var body ScopesResponseBody
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	return body.Scopes, nil
}

// Variables retrieves variable contents for a scope or structured variable.
func (c *Client) Variables(ctx context.Context, args VariablesArgs) ([]Variable, error) {
	resp, err := c.SendRequest(ctx, "variables", args)
	if err != nil {
		return nil, err
	}
	var body VariablesResponseBody
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &body)
	}
	return body.Variables, nil
}

// Continue resumes execution.
func (c *Client) Continue(ctx context.Context, threadID int) error {
	_, err := c.SendRequest(ctx, "continue", ContinueArgs{ThreadID: threadID})
	return err
}

// Next steps over one line.
func (c *Client) Next(ctx context.Context, threadID int) error {
	_, err := c.SendRequest(ctx, "next", NextArgs{ThreadID: threadID})
	return err
}

// StepIn steps into a function.
func (c *Client) StepIn(ctx context.Context, threadID int) error {
	_, err := c.SendRequest(ctx, "stepIn", StepInArgs{ThreadID: threadID})
	return err
}

// StepOut steps out of current function.
func (c *Client) StepOut(ctx context.Context, threadID int) error {
	_, err := c.SendRequest(ctx, "stepOut", StepOutArgs{ThreadID: threadID})
	return err
}

// Pause pauses execution.
func (c *Client) Pause(ctx context.Context, threadID int) error {
	_, err := c.SendRequest(ctx, "pause", PauseArgs{ThreadID: threadID})
	return err
}

// Disconnect terminates or disconnects debug session.
func (c *Client) Disconnect(ctx context.Context, restart bool) error {
	_, err := c.SendRequest(ctx, "disconnect", DisconnectArgs{Restart: restart, TerminateDebuggee: true})
	return err
}

// Terminate requests adapter to terminate debuggee.
func (c *Client) Terminate(ctx context.Context, restart bool) error {
	_, err := c.SendRequest(ctx, "terminate", TerminateArgs{Restart: restart})
	return err
}

// Close gracefully terminates client and process.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = c.Disconnect(ctx, false)

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
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

	c.setStatus(StatusStopped, "Client closed")
	return nil
}
