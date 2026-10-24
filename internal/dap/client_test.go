package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

// mockAdapter simulates a DAP adapter over in-memory pipes.
func runMockAdapter(t *testing.T, r io.Reader, w io.Writer) {
	reader := bufio.NewReader(r)
	writer := NewTransportWriter(w)

	for {
		msgBytes, err := ReadMessage(reader)
		if err != nil {
			return
		}

		var req Request
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			continue
		}

		switch req.Command {
		case "initialize":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "initialize",
				Body:       json.RawMessage(`{"supportsConfigurationDoneRequest": true}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

			// Emit initialized event
			ev := Event{
				Seq:   999,
				Type:  "event",
				Event: "initialized",
			}
			evBytes, _ := json.Marshal(ev)
			_ = writer.WriteMessage(evBytes)

		case "launch":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "launch",
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "setBreakpoints":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "setBreakpoints",
				Body:       json.RawMessage(`{"breakpoints": [{"id": 1, "verified": true, "line": 42}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "configurationDone":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "configurationDone",
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

			// Emit stopped event
			ev := Event{
				Seq:   1001,
				Type:  "event",
				Event: "stopped",
				Body:  json.RawMessage(`{"reason": "breakpoint", "threadId": 1}`),
			}
			evBytes, _ := json.Marshal(ev)
			_ = writer.WriteMessage(evBytes)

		case "threads":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "threads",
				Body:       json.RawMessage(`{"threads": [{"id": 1, "name": "main"}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "stackTrace":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "stackTrace",
				Body:       json.RawMessage(`{"stackFrames": [{"id": 10, "name": "main.main", "line": 42, "column": 1, "source": {"path": "/tmp/main.go", "name": "main.go"}}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "scopes":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "scopes",
				Body:       json.RawMessage(`{"scopes": [{"name": "Locals", "variablesReference": 100}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "variables":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    "variables",
				Body:       json.RawMessage(`{"variables": [{"name": "count", "value": "42", "type": "int"}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		case "continue", "next", "stepIn", "stepOut", "pause", "disconnect", "terminate":
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    true,
				Command:    req.Command,
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)

		default:
			resp := Response{
				Seq:        req.Seq + 100,
				Type:       "response",
				RequestSeq: req.Seq,
				Success:    false,
				Command:    req.Command,
				Message:    fmt.Sprintf("unknown command %s", req.Command),
			}
			respBytes, _ := json.Marshal(resp)
			_ = writer.WriteMessage(respBytes)
		}
	}
}

func TestClientLifecycleAndRequests(t *testing.T) {
	// Setup pipes
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	defer clientToServerR.Close()
	defer serverToClientR.Close()

	go runMockAdapter(t, clientToServerR, serverToClientW)

	client := NewClientFromPipes(clientToServerW, serverToClientR)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Initialize
	caps, err := client.Initialize(ctx, InitializeArgs{
		ClientID: "test",
		AdapterID: "go",
	})
	if err != nil {
		t.Fatalf("initialize error: %v", err)
	}
	if !caps.SupportsConfigurationDoneRequest {
		t.Errorf("expected capability supportsConfigurationDoneRequest true")
	}

	// 2. SetBreakpoints
	bps, err := client.SetBreakpoints(ctx, SetBreakpointsArgs{
		Source: Source{Path: "/tmp/main.go"},
		Breakpoints: []SourceBreakpoint{
			{Line: 42},
		},
	})
	if err != nil {
		t.Fatalf("setBreakpoints error: %v", err)
	}
	if len(bps) != 1 || !bps[0].Verified || bps[0].Line != 42 {
		t.Errorf("unexpected breakpoints response: %+v", bps)
	}

	// 3. Launch
	if err := client.Launch(ctx, LaunchArgs{"program": "."}); err != nil {
		t.Fatalf("launch error: %v", err)
	}

	// 4. Threads
	threads, err := client.Threads(ctx)
	if err != nil {
		t.Fatalf("threads error: %v", err)
	}
	if len(threads) != 1 || threads[0].Name != "main" {
		t.Errorf("unexpected threads: %+v", threads)
	}

	// 5. StackTrace
	frames, err := client.StackTrace(ctx, StackTraceArgs{ThreadID: 1})
	if err != nil {
		t.Fatalf("stackTrace error: %v", err)
	}
	if len(frames) != 1 || frames[0].Line != 42 {
		t.Errorf("unexpected stack frames: %+v", frames)
	}

	// 6. Scopes & Variables
	scopes, err := client.Scopes(ctx, ScopesArgs{FrameID: frames[0].ID})
	if err != nil {
		t.Fatalf("scopes error: %v", err)
	}
	if len(scopes) != 1 || scopes[0].Name != "Locals" {
		t.Errorf("unexpected scopes: %+v", scopes)
	}

	vars, err := client.Variables(ctx, VariablesArgs{VariablesReference: scopes[0].VariablesReference})
	if err != nil {
		t.Fatalf("variables error: %v", err)
	}
	if len(vars) != 1 || vars[0].Name != "count" || vars[0].Value != "42" {
		t.Errorf("unexpected variables: %+v", vars)
	}

	// 7. Stepping commands
	if err := client.Next(ctx, 1); err != nil {
		t.Errorf("next error: %v", err)
	}
	if err := client.StepIn(ctx, 1); err != nil {
		t.Errorf("stepIn error: %v", err)
	}
	if err := client.StepOut(ctx, 1); err != nil {
		t.Errorf("stepOut error: %v", err)
	}
	if err := client.Continue(ctx, 1); err != nil {
		t.Errorf("continue error: %v", err)
	}
}
