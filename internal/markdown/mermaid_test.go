package markdown

import (
	"strings"
	"testing"
)

func TestRenderMermaidFlowchartTD(t *testing.T) {
	chart := `flowchart TD
    Start["Start Process"] --> Build["Compile Code"]
    Build --> Test["Run Tests"]
    Test --> Deploy["Deploy Server"]
`
	rendered := RenderMermaid(chart, MermaidModeBoxArt, 80)
	if !strings.Contains(rendered, "Start Process") {
		t.Fatalf("expected 'Start Process' in rendered chart, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Compile Code") {
		t.Fatalf("expected 'Compile Code' in rendered chart, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "▼") {
		t.Fatalf("expected downward arrow ▼ in TD chart, got:\n%s", rendered)
	}
}

func TestRenderMermaidFlowchartLR(t *testing.T) {
	chart := `flowchart LR
    A["Client"] --> B["API Gateway"] --> C["Service"]
`
	rendered := RenderMermaid(chart, MermaidModeBoxArt, 80)
	if !strings.Contains(rendered, "Client") {
		t.Fatalf("expected 'Client' in rendered chart, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "API Gateway") {
		t.Fatalf("expected 'API Gateway' in rendered chart, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "▶") {
		t.Fatalf("expected rightward arrow ▶ in LR chart, got:\n%s", rendered)
	}
}

func TestRenderMermaidSequenceDiagram(t *testing.T) {
	seq := `sequenceDiagram
    participant Alice
    participant Bob
    Alice->>Bob: Hello Bob
    Bob-->>Alice: Hi Alice
`
	rendered := RenderMermaid(seq, MermaidModeBoxArt, 80)
	if !strings.Contains(rendered, "Alice") || !strings.Contains(rendered, "Bob") {
		t.Fatalf("expected participants Alice and Bob, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Hello Bob") {
		t.Fatalf("expected message 'Hello Bob', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Hi Alice") {
		t.Fatalf("expected message 'Hi Alice', got:\n%s", rendered)
	}
}

func TestDetectKittyGraphics(t *testing.T) {
	// Function returns bool without panicking
	_ = DetectKittyGraphics()
}
