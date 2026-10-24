package markdown

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

// MermaidMode specifies the diagram rendering mode.
type MermaidMode int

const (
	MermaidModeBoxArt MermaidMode = iota
	MermaidModeKitty
)

var (
	reMermaidBlock = regexp.MustCompile("(?s)```mermaid\\s*\\n(.*?)\\n```")
	reFlowchartHdr = regexp.MustCompile(`^(?:flowchart|graph)\s+(TD|TB|LR|RL|BT)`)
	reSequenceHdr  = regexp.MustCompile(`^sequenceDiagram`)
	reParticipant  = regexp.MustCompile(`^participant\s+([a-zA-Z0-9_-]+)(?:\s+as\s+(.*))?`)
	reSeqMsg       = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*(->>|-->>|->|-->)\s*([a-zA-Z0-9_-]+)\s*:\s*(.*)$`)
	reEdge         = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*(?:\["?(.*?)"?\]|\("?(.*?)"?\)|\{"?(.*?)"?\})?\s*(-->|---|-.->|==>)(?:\|(.*?)\|)?\s*([a-zA-Z0-9_-]+)(?:\["?(.*?)"?\]|\("?(.*?)"?\)|\{"?(.*?)"?\})?$`)
	reNodeOnly     = regexp.MustCompile(`^([a-zA-Z0-9_-]+)(?:\["?(.*?)"?\]|\("?(.*?)"?\)|\{"?(.*?)"?\})$`)
)

// DetectKittyGraphics returns true if the terminal supports Kitty Graphics Protocol.
func DetectKittyGraphics() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := os.Getenv("TERM")
	if term == "xterm-kitty" || strings.HasPrefix(term, "foot") {
		return true
	}
	return false
}

// FlowNode represents a node in a flowchart.
type FlowNode struct {
	ID    string
	Label string
	Shape string // "rect", "round", "diamond"
}

// FlowEdge represents a directed or undirected connection between two nodes.
type FlowEdge struct {
	From  string
	To    string
	Label string
	Style string
}

// RenderMermaid parses mermaid diagram syntax and returns a formatted box-art string.
func RenderMermaid(code string, mode MermaidMode, maxWidth int) string {
	clean := strings.TrimSpace(code)
	lines := strings.Split(clean, "\n")
	if len(lines) == 0 {
		return ""
	}

	header := strings.TrimSpace(lines[0])

	if reSequenceHdr.MatchString(header) {
		return renderSequenceDiagram(lines[1:], maxWidth)
	}

	dir := "TD"
	if m := reFlowchartHdr.FindStringSubmatch(header); len(m) > 1 {
		dir = m[1]
	}

	return renderFlowchart(lines[1:], dir, maxWidth)
}

func renderFlowchart(lines []string, dir string, maxWidth int) string {
	nodes := make(map[string]*FlowNode)
	var order []string
	var edges []FlowEdge

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "%%") {
			continue
		}

		if m := reEdge.FindStringSubmatch(l); len(m) > 0 {
			fromID := m[1]
			fromLabel := m[2]
			if fromLabel == "" {
				fromLabel = m[3]
			}
			if fromLabel == "" {
				fromLabel = m[4]
			}
			if fromLabel == "" {
				fromLabel = fromID
			}

			edgeStyle := m[5]
			edgeLabel := m[6]
			toID := m[7]
			toLabel := m[8]
			if toLabel == "" {
				toLabel = m[9]
			}
			if toLabel == "" {
				toLabel = m[10]
			}
			if toLabel == "" {
				toLabel = toID
			}

			if _, exists := nodes[fromID]; !exists {
				nodes[fromID] = &FlowNode{ID: fromID, Label: fromLabel, Shape: "rect"}
				order = append(order, fromID)
			} else if fromLabel != fromID {
				nodes[fromID].Label = fromLabel
			}

			if _, exists := nodes[toID]; !exists {
				nodes[toID] = &FlowNode{ID: toID, Label: toLabel, Shape: "rect"}
				order = append(order, toID)
			} else if toLabel != toID {
				nodes[toID].Label = toLabel
			}

			edges = append(edges, FlowEdge{
				From:  fromID,
				To:    toID,
				Label: edgeLabel,
				Style: edgeStyle,
			})
			continue
		}

		if m := reNodeOnly.FindStringSubmatch(l); len(m) > 0 {
			id := m[1]
			label := m[2]
			if label == "" {
				label = m[3]
			}
			if label == "" {
				label = m[4]
			}
			if label == "" {
				label = id
			}
			if _, exists := nodes[id]; !exists {
				nodes[id] = &FlowNode{ID: id, Label: label, Shape: "rect"}
				order = append(order, id)
			} else {
				nodes[id].Label = label
			}
		}
	}

	if len(order) == 0 {
		return "  (empty diagram)"
	}

	if dir == "LR" {
		return renderFlowchartLR(nodes, order, edges)
	}
	return renderFlowchartTD(nodes, order, edges)
}

func renderFlowchartTD(nodes map[string]*FlowNode, order []string, edges []FlowEdge) string {
	var sb strings.Builder

	edgeMap := make(map[string][]FlowEdge)
	for _, e := range edges {
		edgeMap[e.From] = append(edgeMap[e.From], e)
	}

	for i, id := range order {
		node := nodes[id]
		label := node.Label
		w := runewidth.StringWidth(label)
		boxWidth := w + 4

		top := "┌" + strings.Repeat("─", boxWidth-2) + "┐"
		mid := "│ " + label + " │"
		bot := "└" + strings.Repeat("─", boxWidth-2) + "┘"

		indent := "  "
		sb.WriteString(indent + top + "\n")
		sb.WriteString(indent + mid + "\n")
		sb.WriteString(indent + bot + "\n")

		// If this node has outgoing edges or isn't the last node
		if i < len(order)-1 {
			outgoing := edgeMap[id]
			centerPad := strings.Repeat(" ", boxWidth/2)
			if len(outgoing) > 0 && outgoing[0].Label != "" {
				sb.WriteString(indent + centerPad + "│ (" + outgoing[0].Label + ")\n")
			} else {
				sb.WriteString(indent + centerPad + "│\n")
			}
			sb.WriteString(indent + centerPad + "▼\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func renderFlowchartLR(nodes map[string]*FlowNode, order []string, edges []FlowEdge) string {
	edgeMap := make(map[string]FlowEdge)
	for _, e := range edges {
		edgeMap[e.From+"->"+e.To] = e
	}

	var rowTop, rowMid, rowBot strings.Builder
	rowTop.WriteString("  ")
	rowMid.WriteString("  ")
	rowBot.WriteString("  ")

	for i, id := range order {
		node := nodes[id]
		label := node.Label
		w := runewidth.StringWidth(label)
		boxWidth := w + 4

		rowTop.WriteString("┌" + strings.Repeat("─", boxWidth-2) + "┐")
		rowMid.WriteString("│ " + label + " │")
		rowBot.WriteString("└" + strings.Repeat("─", boxWidth-2) + "┘")

		if i < len(order)-1 {
			nextID := order[i+1]
			arrow := " ──▶ "
			if e, ok := edgeMap[id+"->"+nextID]; ok && e.Label != "" {
				arrow = fmt.Sprintf(" ─[%s]─▶ ", e.Label)
			}
			arrowLen := runewidth.StringWidth(arrow)
			rowTop.WriteString(strings.Repeat(" ", arrowLen))
			rowMid.WriteString(arrow)
			rowBot.WriteString(strings.Repeat(" ", arrowLen))
		}
	}

	return rowTop.String() + "\n" + rowMid.String() + "\n" + rowBot.String()
}

func renderSequenceDiagram(lines []string, maxWidth int) string {
	type SeqMessage struct {
		From string
		To   string
		Text string
	}

	var participants []string
	partMap := make(map[string]bool)
	var messages []SeqMessage

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "%%") {
			continue
		}

		if m := reParticipant.FindStringSubmatch(l); len(m) > 0 {
			name := m[1]
			if !partMap[name] {
				partMap[name] = true
				participants = append(participants, name)
			}
			continue
		}

		if m := reSeqMsg.FindStringSubmatch(l); len(m) > 0 {
			from := m[1]
			to := m[3]
			text := strings.TrimSpace(m[4])
			if !partMap[from] {
				partMap[from] = true
				participants = append(participants, from)
			}
			if !partMap[to] {
				partMap[to] = true
				participants = append(participants, to)
			}
			messages = append(messages, SeqMessage{From: from, To: to, Text: text})
		}
	}

	if len(participants) == 0 {
		return "  (empty sequence diagram)"
	}

	colWidth := 20
	var sb strings.Builder

	// Participant headers
	var topRow, midRow, botRow strings.Builder
	topRow.WriteString("  ")
	midRow.WriteString("  ")
	botRow.WriteString("  ")

	for _, p := range participants {
		pLen := runewidth.StringWidth(p)
		boxW := pLen + 4
		if boxW > colWidth {
			colWidth = boxW + 2
		}
		top := "┌" + strings.Repeat("─", boxW-2) + "┐"
		mid := "│ " + p + " │"
		bot := "└─┬─" + strings.Repeat("─", max(0, boxW-4)) + "┘"

		spacing := colWidth - boxW
		if spacing < 2 {
			spacing = 2
		}

		topRow.WriteString(top + strings.Repeat(" ", spacing))
		midRow.WriteString(mid + strings.Repeat(" ", spacing))
		botRow.WriteString(bot + strings.Repeat(" ", spacing))
	}

	sb.WriteString(topRow.String() + "\n")
	sb.WriteString(midRow.String() + "\n")
	sb.WriteString(botRow.String() + "\n")

	// Helper to find participant center column
	getCol := func(name string) int {
		for i, p := range participants {
			if p == name {
				return 2 + (i * colWidth) + 2
			}
		}
		return 0
	}

	totalWidth := 2 + len(participants)*colWidth

	// Draw lifelines with messages
	for _, msg := range messages {
		// Empty lifeline gap
		line := make([]rune, totalWidth)
		for i := range line {
			line[i] = ' '
		}
		for _, p := range participants {
			c := getCol(p)
			if c < len(line) {
				line[c] = '│'
			}
		}
		sb.WriteString(string(line) + "\n")

		// Message row
		fromCol := getCol(msg.From)
		toCol := getCol(msg.To)

		msgRunes := make([]rune, totalWidth)
		for i := range msgRunes {
			msgRunes[i] = ' '
		}
		for _, p := range participants {
			c := getCol(p)
			if c < len(msgRunes) {
				msgRunes[c] = '│'
			}
		}

		if fromCol < toCol {
			// Left to right arrow: ├── text ──▶│
			span := toCol - fromCol
			arrowStr := fmt.Sprintf("── %s ──▶", msg.Text)
			if runewidth.StringWidth(arrowStr) > span {
				arrowStr = truncateStringWidth(arrowStr, span)
			}
			copy(msgRunes[fromCol+1:], []rune(arrowStr))
			for k := fromCol + 1 + len([]rune(arrowStr)); k < toCol; k++ {
				msgRunes[k] = '─'
			}
			msgRunes[fromCol] = '├'
			msgRunes[toCol] = '▶'
		} else if fromCol > toCol {
			// Right to left arrow: │◀── text ──┤
			span := fromCol - toCol
			arrowStr := fmt.Sprintf("◀── %s ──", msg.Text)
			if runewidth.StringWidth(arrowStr) > span {
				arrowStr = truncateStringWidth(arrowStr, span)
			}
			copy(msgRunes[toCol+1:], []rune(arrowStr))
			for k := toCol + 1 + len([]rune(arrowStr)); k < fromCol; k++ {
				msgRunes[k] = '─'
			}
			msgRunes[toCol] = '◀'
			msgRunes[fromCol] = '┤'
		}

		sb.WriteString(string(msgRunes) + "\n")
	}

	// Bottom lifeline
	line := make([]rune, totalWidth)
	for i := range line {
		line[i] = ' '
	}
	for _, p := range participants {
		c := getCol(p)
		if c < len(line) {
			line[c] = '┴'
		}
	}
	sb.WriteString(string(line))

	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateStringWidth(s string, maxWidth int) string {
	runes := []rune(s)
	curW := 0
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if curW+rw > maxWidth {
			return string(runes[:i])
		}
		curW += rw
	}
	return s
}
