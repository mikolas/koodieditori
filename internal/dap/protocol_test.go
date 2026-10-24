package dap

import (
	"encoding/json"
	"testing"
)

func TestProtocolSerialization(t *testing.T) {
	req := Request{
		Seq:     1,
		Type:    "request",
		Command: "initialize",
		Arguments: InitializeArgs{
			ClientID:   "test",
			ClientName: "Test",
			AdapterID:  "go",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request error: %v", err)
	}

	var req2 Request
	if err := json.Unmarshal(data, &req2); err != nil {
		t.Fatalf("unmarshal request error: %v", err)
	}

	if req2.Command != "initialize" || req2.Seq != 1 {
		t.Errorf("unexpected request unmarshaled: %+v", req2)
	}

	resp := Response{
		Seq:        2,
		Type:       "response",
		RequestSeq: 1,
		Success:    true,
		Command:    "initialize",
		Body:       json.RawMessage(`{"supportsConfigurationDoneRequest": true}`),
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response error: %v", err)
	}

	var resp2 Response
	if err := json.Unmarshal(respData, &resp2); err != nil {
		t.Fatalf("unmarshal response error: %v", err)
	}

	if !resp2.Success || resp2.RequestSeq != 1 {
		t.Errorf("unexpected response: %+v", resp2)
	}

	var caps Capabilities
	if err := json.Unmarshal(resp2.Body, &caps); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	if !caps.SupportsConfigurationDoneRequest {
		t.Errorf("expected supportsConfigurationDoneRequest true")
	}
}
