package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestHelperProcess runs as the mock LSP server when GO_WANT_HELPER_PROCESS == "1".
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	reader := bufio.NewReader(os.Stdin)
	writer := NewTransportWriter(os.Stdout)

	for {
		msg, err := ReadMessage(reader)
		if err != nil {
			os.Exit(0)
		}

		var req Request
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			res := InitializeResult{
				Capabilities: ServerCapabilities{
					DefinitionProvider: true,
					CompletionProvider: true,
				},
				ServerInfo: &ServerInfo{
					Name:    "mock-lsp",
					Version: "1.0",
				},
			}
			resBytes, _ := json.Marshal(res)
			resp := Response{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  resBytes,
			}
			payload, _ := json.Marshal(resp)
			_ = writer.WriteMessage(payload)

			// Also push a diagnostic notification to verify notification dispatch
			diagNotif := Notification{
				JSONRPC: "2.0",
				Method:  "textDocument/publishDiagnostics",
				Params: json.RawMessage(`{
					"uri": "file:///mock/main.go",
					"diagnostics": [
						{
							"range": {"start": {"line": 2, "character": 0}, "end": {"line": 2, "character": 4}},
							"severity": 1,
							"message": "mock error"
						}
					]
				}`),
			}
			notifPayload, _ := json.Marshal(diagNotif)
			_ = writer.WriteMessage(notifPayload)

		case "textDocument/definition":
			locs := []Location{
				{
					URI: "file:///mock/target.go",
					Range: Range{
						Start: Position{Line: 42, Character: 10},
						End:   Position{Line: 42, Character: 15},
					},
				},
			}
			resBytes, _ := json.Marshal(locs)
			resp := Response{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  resBytes,
			}
			payload, _ := json.Marshal(resp)
			_ = writer.WriteMessage(payload)

		case "textDocument/completion":
			items := []CompletionItem{
				{
					Label:  "PrintLn",
					Kind:   CompletionFunction,
					Detail: "func PrintLn(s string)",
				},
			}
			resBytes, _ := json.Marshal(items)
			resp := Response{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  resBytes,
			}
			payload, _ := json.Marshal(resp)
			_ = writer.WriteMessage(payload)

		case "shutdown":
			resp := Response{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  json.RawMessage("null"),
			}
			payload, _ := json.Marshal(resp)
			_ = writer.WriteMessage(payload)

		case "exit":
			os.Exit(0)
		}
	}
}

func TestClientIntegration(t *testing.T) {
	// Use current test executable as the child process
	client := NewClient("mock-lsp", os.Args[0], []string{"-test.run=TestHelperProcess", "--"}, "")

	diagReceived := make(chan struct{}, 1)
	client.OnNotification("textDocument/publishDiagnostics", func(payload json.RawMessage) {
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(payload, &params); err == nil && len(params.Diagnostics) > 0 {
			select {
			case diagReceived <- struct{}{}:
			default:
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set helper process env
	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start() failed: %v", err)
	}
	defer client.Close()

	// Wait for client to become ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := client.Status()
		if status == StatusReady {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, detail := client.Status()
	if status != StatusReady {
		t.Fatalf("expected StatusReady, got %s (detail: %s)", status, detail)
	}

	// Verify notification was received
	select {
	case <-diagReceived:
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for publishDiagnostics notification")
	}

	// Test GotoDefinition
	locs, err := client.GotoDefinition(ctx, "file:///mock/main.go", 10, 5)
	if err != nil {
		t.Fatalf("GotoDefinition() failed: %v", err)
	}
	if len(locs) != 1 || locs[0].Range.Start.Line != 42 {
		t.Errorf("GotoDefinition() = %+v, want line 42", locs)
	}

	// Test Completion
	items, err := client.Completion(ctx, "file:///mock/main.go", 10, 5)
	if err != nil {
		t.Fatalf("Completion() failed: %v", err)
	}
	if len(items) != 1 || items[0].Label != "PrintLn" {
		t.Errorf("Completion() = %+v, want PrintLn", items)
	}
}
