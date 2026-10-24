package ipc

import (
	"encoding/json"
	"fmt"
	"net"
)

// Client is a Unix domain socket client for interacting with a running Koodieditori instance.
type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
}

// NewClient establishes a connection to the Koodieditori IPC socket.
func NewClient(socketPath string) (*Client, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath("default")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Koodieditori socket at %s: %w", socketPath, err)
	}

	return &Client{
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Send transmits a Request and returns the Response.
func (c *Client) Send(method string, params any) (*Response, error) {
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		rawParams = data
	}

	req := Request{
		Method: method,
		Params: rawParams,
	}

	if err := c.enc.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !resp.Success {
		return &resp, fmt.Errorf("server error: %s", resp.Error)
	}

	return &resp, nil
}

// Open requests the editor to open a file at line and column.
func (c *Client) Open(file string, line, col int) error {
	_, err := c.Send("open", OpenParams{File: file, Line: line, Col: col})
	return err
}

// Diff requests the editor to open a diff view for a file.
func (c *Client) Diff(file string) error {
	_, err := c.Send("diff", DiffParams{File: file})
	return err
}

// Reload requests the editor to reload open buffer(s) from disk.
func (c *Client) Reload(all bool, file string) error {
	_, err := c.Send("reload", ReloadParams{All: all, File: file})
	return err
}

// Notify displays a notification message in the editor.
func (c *Client) Notify(message string, level string) error {
	_, err := c.Send("notify", NotifyParams{Message: message, Level: level})
	return err
}

// StatusBadge updates the agent status badge on the statusline.
func (c *Client) StatusBadge(badge string, busy bool) error {
	_, err := c.Send("status-badge", StatusBadgeParams{Badge: badge, Busy: busy})
	return err
}

// Action dispatches an arbitrary editor action by name.
func (c *Client) Action(name string, args map[string]string) error {
	_, err := c.Send("action", ActionParams{Name: name, Args: args})
	return err
}
