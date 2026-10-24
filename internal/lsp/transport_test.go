package lsp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
)

func TestTransportReadWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	writer := NewTransportWriter(&buf)

	msg1 := []byte(`{"jsonrpc":"2.0","id":1,"method":"test","params":{}}`)
	msg2 := []byte(`{"jsonrpc":"2.0","method":"notify"}`)

	if err := writer.WriteMessage(msg1); err != nil {
		t.Fatalf("failed to write msg1: %v", err)
	}
	if err := writer.WriteMessage(msg2); err != nil {
		t.Fatalf("failed to write msg2: %v", err)
	}

	reader := bufio.NewReader(&buf)

	read1, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("failed to read msg1: %v", err)
	}
	if !bytes.Equal(read1, msg1) {
		t.Errorf("read1 = %s, want %s", string(read1), string(msg1))
	}

	read2, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("failed to read msg2: %v", err)
	}
	if !bytes.Equal(read2, msg2) {
		t.Errorf("read2 = %s, want %s", string(read2), string(msg2))
	}
}

func TestTransportExtraHeaders(t *testing.T) {
	raw := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 13\r\n\r\nHello, World!"
	reader := bufio.NewReader(bytes.NewBufferString(raw))

	msg, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(msg) != "Hello, World!" {
		t.Errorf("got %q, want %q", string(msg), "Hello, World!")
	}
}

func TestTransportMissingContentLength(t *testing.T) {
	raw := "Content-Type: application/vscode-jsonrpc\r\n\r\nHello"
	reader := bufio.NewReader(bytes.NewBufferString(raw))

	_, err := ReadMessage(reader)
	if err == nil {
		t.Fatalf("expected error for missing Content-Length, got nil")
	}
}

func TestTransportConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	writer := NewTransportWriter(&buf)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			payload := []byte(`{"id":1}`)
			_ = writer.WriteMessage(payload)
		}(i)
	}

	wg.Wait()

	reader := bufio.NewReader(&buf)
	for i := 0; i < n; i++ {
		msg, err := ReadMessage(reader)
		if err != nil {
			t.Fatalf("failed to read message %d: %v", i, err)
		}
		if string(msg) != `{"id":1}` {
			t.Fatalf("corrupted message: %s", string(msg))
		}
	}
}
