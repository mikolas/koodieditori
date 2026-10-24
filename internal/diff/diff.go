package diff

import (
	"fmt"
	"strings"
)

// ChangeType specifies whether a diff line is unchanged, added, or deleted.
type ChangeType int

const (
	DiffEqual ChangeType = iota
	DiffAdd
	DiffDelete
)

// DiffLine represents a single line in a diff view.
type DiffLine struct {
	Type    ChangeType
	Content string
}

// DiffResult holds computed diff metrics and line-by-line diff output.
type DiffResult struct {
	Additions int
	Deletions int
	Lines     []DiffLine
}

// UnifiedText returns standard git-style unified diff text.
func (d DiffResult) UnifiedText() string {
	var sb strings.Builder
	for _, l := range d.Lines {
		switch l.Type {
		case DiffAdd:
			sb.WriteString("+ " + l.Content + "\n")
		case DiffDelete:
			sb.WriteString("- " + l.Content + "\n")
		case DiffEqual:
			sb.WriteString("  " + l.Content + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// FormatSummary returns a compact git-style string like "(+30/-8)".
func (d DiffResult) FormatSummary() string {
	return fmt.Sprintf("(+%d/-%d)", d.Additions, d.Deletions)
}

// ComputeDiff calculates line-by-line differences between oldText and newText using LCS.
func ComputeDiff(oldText, newText string) DiffResult {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	n := len(oldLines)
	m := len(newLines)

	// Special cases: empty inputs
	if n == 0 && m == 0 {
		return DiffResult{}
	}
	if n == 0 {
		lines := make([]DiffLine, m)
		for i, l := range newLines {
			lines[i] = DiffLine{Type: DiffAdd, Content: l}
		}
		return DiffResult{Additions: m, Deletions: 0, Lines: lines}
	}
	if m == 0 {
		lines := make([]DiffLine, n)
		for i, l := range oldLines {
			lines[i] = DiffLine{Type: DiffDelete, Content: l}
		}
		return DiffResult{Additions: 0, Deletions: n, Lines: lines}
	}

	// Compute LCS table
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if oldLines[i] == newLines[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else {
				if dp[i+1][j] >= dp[i][j+1] {
					dp[i+1][j+1] = dp[i+1][j]
				} else {
					dp[i+1][j+1] = dp[i][j+1]
				}
			}
		}
	}

	// Backtrack to build diff
	var lines []DiffLine
	additions := 0
	deletions := 0

	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			lines = append(lines, DiffLine{Type: DiffEqual, Content: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			lines = append(lines, DiffLine{Type: DiffAdd, Content: newLines[j-1]})
			additions++
			j--
		} else if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			lines = append(lines, DiffLine{Type: DiffDelete, Content: oldLines[i-1]})
			deletions++
			i--
		}
	}

	// Reverse lines to restore original order
	for k := 0; k < len(lines)/2; k++ {
		opp := len(lines) - 1 - k
		lines[k], lines[opp] = lines[opp], lines[k]
	}

	return DiffResult{
		Additions: additions,
		Deletions: deletions,
		Lines:     lines,
	}
}

func splitLines(s string) []string {
	if len(s) == 0 {
		return nil
	}
	raw := strings.Split(s, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	return raw
}
