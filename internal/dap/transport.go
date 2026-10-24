package dap

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// ReadMessage reads a single Content-Length framed DAP message from r.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headerName := strings.TrimSpace(parts[0])
			headerValue := strings.TrimSpace(parts[1])
			if strings.EqualFold(headerName, "Content-Length") {
				cl, err := strconv.Atoi(headerValue)
				if err != nil {
					return nil, fmt.Errorf("invalid Content-Length: %w", err)
				}
				contentLength = cl
			}
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading message body of %d bytes: %w", contentLength, err)
	}

	return buf, nil
}

// TransportWriter provides thread-safe framed message writing to an io.Writer.
type TransportWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewTransportWriter creates a new thread-safe framed message writer.
func NewTransportWriter(w io.Writer) *TransportWriter {
	return &TransportWriter{w: w}
}

// WriteMessage frames and writes payload to the underlying writer.
func (tw *TransportWriter) WriteMessage(payload []byte) error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := tw.w.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := tw.w.Write(payload); err != nil {
		return err
	}
	if flusher, ok := tw.w.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}
