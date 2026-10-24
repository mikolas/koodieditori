package dap

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestTransportReadWrite(t *testing.T) {
	var buf bytes.Buffer
	tw := NewTransportWriter(&buf)

	msg := []byte(`{"seq":1,"type":"request","command":"initialize"}`)
	if err := tw.WriteMessage(msg); err != nil {
		t.Fatalf("WriteMessage error: %v", err)
	}

	raw := buf.String()
	if !strings.HasPrefix(raw, "Content-Length: 49\r\n\r\n") {
		t.Fatalf("unexpected framed message header: %q", raw)
	}

	reader := bufio.NewReader(&buf)
	readMsg, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage error: %v", err)
	}

	if string(readMsg) != string(msg) {
		t.Fatalf("expected payload %q, got %q", string(msg), string(readMsg))
	}
}
