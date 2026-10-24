package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/search"
	"github.com/mikolas/koodieditori/internal/watcher"
)

// PickerMode specifies the candidate source for the fuzzy finder.
type PickerMode int

const (
	PickerModeFiles PickerMode = iota
	PickerModeBuffers
	PickerModeGrep
)

// PickerItem represents a candidate in the picker list.
type PickerItem struct {
	Label  string // Display text
	Value  string // File path or buffer identifier
	Detail string // Subtext, line number, or match snippet
	Score  int
}

// Picker manages the centered modal fuzzy finder.
type Picker struct {
	Open          bool
	Mode          PickerMode
	Query         string
	Items         []PickerItem
	Filtered      []PickerItem
	SelectedIndex int
	ScrollY       int
	WorkspacePath string
}

// NewPicker creates a new Picker initialized for workspacePath.
func NewPicker(workspacePath string) *Picker {
	if workspacePath == "" {
		workspacePath = "."
	}
	absPath, err := filepath.Abs(workspacePath)
	if err != nil {
		absPath = workspacePath
	}

	return &Picker{
		Open:          false,
		Mode:          PickerModeFiles,
		Query:         "",
		Items:         nil,
		Filtered:      nil,
		SelectedIndex: 0,
		ScrollY:       0,
		WorkspacePath: absPath,
	}
}

// OpenFiles opens the picker in File Finder mode (Space f f).
func (p *Picker) OpenFiles(workspacePath string) {
	if workspacePath != "" {
		p.WorkspacePath = workspacePath
	}
	p.Mode = PickerModeFiles
	p.Query = ""
	p.Open = true
	p.SelectedIndex = 0
	p.ScrollY = 0

	p.loadFiles()
	p.Filter()
}

// OpenBuffers opens the picker in Buffer Switcher mode (Space b b).
func (p *Picker) OpenBuffers(buffers []*BufferItem, activeIdx int) {
	p.Mode = PickerModeBuffers
	p.Query = ""
	p.Open = true
	p.SelectedIndex = 0
	p.ScrollY = 0

	p.loadBuffers(buffers, activeIdx)
	p.Filter()
}

// OpenGrep opens the picker in Live Grep mode (Space f g).
func (p *Picker) OpenGrep(workspacePath string) {
	if workspacePath != "" {
		p.WorkspacePath = workspacePath
	}
	p.Mode = PickerModeGrep
	p.Query = ""
	p.Open = true
	p.SelectedIndex = 0
	p.ScrollY = 0
	p.Items = nil
	p.Filtered = nil
}

// Close closes the picker modal.
func (p *Picker) Close() {
	p.Open = false
	p.Query = ""
	p.Items = nil
	p.Filtered = nil
	p.SelectedIndex = 0
	p.ScrollY = 0
}

// InsertChar appends a character to the search query and refilters candidates.
func (p *Picker) InsertChar(key string) {
	p.Query += key
	if p.Mode == PickerModeGrep {
		p.executeGrep(p.Query)
	} else {
		p.Filter()
	}
}

// DeleteChar deletes the last character in the search query and refilters candidates.
func (p *Picker) DeleteChar() {
	if len(p.Query) > 0 {
		runes := []rune(p.Query)
		p.Query = string(runes[:len(runes)-1])
		if p.Mode == PickerModeGrep {
			p.executeGrep(p.Query)
		} else {
			p.Filter()
		}
	}
}

// MoveCursor moves candidate selection by delta and keeps it within view.
func (p *Picker) MoveCursor(delta int, visibleRows int) {
	if len(p.Filtered) == 0 {
		p.SelectedIndex = 0
		p.ScrollY = 0
		return
	}

	p.SelectedIndex += delta
	if p.SelectedIndex < 0 {
		p.SelectedIndex = 0
	}
	if p.SelectedIndex >= len(p.Filtered) {
		p.SelectedIndex = len(p.Filtered) - 1
	}

	if visibleRows <= 0 {
		visibleRows = 10
	}

	if p.SelectedIndex < p.ScrollY {
		p.ScrollY = p.SelectedIndex
	} else if p.SelectedIndex >= p.ScrollY+visibleRows {
		p.ScrollY = p.SelectedIndex - visibleRows + 1
	}
	if p.ScrollY < 0 {
		p.ScrollY = 0
	}
}

// SelectedItem returns the currently highlighted candidate.
func (p *Picker) SelectedItem() *PickerItem {
	if len(p.Filtered) == 0 || p.SelectedIndex < 0 || p.SelectedIndex >= len(p.Filtered) {
		return nil
	}
	item := p.Filtered[p.SelectedIndex]
	return &item
}

// Filter scores and filters the candidate items against the current query.
func (p *Picker) Filter() {
	if p.Query == "" {
		p.Filtered = make([]PickerItem, len(p.Items))
		copy(p.Filtered, p.Items)
		p.SelectedIndex = 0
		p.ScrollY = 0
		return
	}

	var matched []PickerItem
	for _, item := range p.Items {
		ok, score := FuzzyScore(p.Query, item.Label)
		if ok {
			item.Score = score
			matched = append(matched, item)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Score != matched[j].Score {
			return matched[i].Score > matched[j].Score
		}
		return len(matched[i].Label) < len(matched[j].Label)
	})

	p.Filtered = matched
	p.SelectedIndex = 0
	p.ScrollY = 0
}

// FuzzyScore calculates fuzzy matching similarity between pattern and target.
// Returns whether the target matched and an integer relevance score.
func FuzzyScore(pattern, target string) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	pRunes := []rune(pattern)
	tRunes := []rune(target)

	pLen := len(pRunes)
	tLen := len(tRunes)

	if pLen > tLen {
		return false, 0
	}

	pIdx := 0
	score := 0
	consecutive := 0
	firstMatchIdx := -1

	for tIdx := 0; tIdx < tLen && pIdx < pLen; tIdx++ {
		pChar := unicode.ToLower(pRunes[pIdx])
		tChar := unicode.ToLower(tRunes[tIdx])

		if pChar == tChar {
			if firstMatchIdx == -1 {
				firstMatchIdx = tIdx
			}
			score += 10
			score += (consecutive * 5)
			consecutive++

			// Exact case bonus
			if pRunes[pIdx] == tRunes[tIdx] {
				score += 5
			}

			// Word boundary bonus: start of string, or previous char is delimiter (non-letter, non-digit)
			if tIdx == 0 || (!unicode.IsLetter(tRunes[tIdx-1]) && !unicode.IsDigit(tRunes[tIdx-1])) {
				score += 20
			} else if unicode.IsUpper(tRunes[tIdx]) && !unicode.IsUpper(tRunes[tIdx-1]) {
				// CamelCase bonus
				score += 15
			}

			pIdx++
		} else {
			consecutive = 0
		}
	}

	if pIdx < pLen {
		return false, 0
	}

	// Exact match bonus
	if strings.EqualFold(pattern, target) {
		score += 100
	}

	// Earlier match bonus
	if firstMatchIdx >= 0 {
		score -= firstMatchIdx
	}

	// Compactness penalty
	score -= (tLen - pLen)

	return true, score
}

func (p *Picker) loadFiles() {
	var files []string

	// 1. Try 'git ls-files' if inside a git repo
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = p.WorkspacePath
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !watcher.IsIgnoredPath(line) {
				files = append(files, line)
			}
		}
	}

	// 2. Try 'rg --files' if ripgrep is installed
	if len(files) == 0 {
		if rgPath, err := exec.LookPath("rg"); err == nil && rgPath != "" {
			cmdRg := exec.Command(rgPath, "--files", "--hidden", "--glob", "!.git/*")
			cmdRg.Dir = p.WorkspacePath
			if out, err := cmdRg.Output(); err == nil && len(out) > 0 {
				scanner := bufio.NewScanner(strings.NewReader(string(out)))
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line != "" && !watcher.IsIgnoredPath(line) {
						files = append(files, line)
					}
				}
			}
		}
	}

	// 3. Fallback: Pure Go directory walk
	if len(files) == 0 {
		_ = filepath.WalkDir(p.WorkspacePath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				base := filepath.Base(path)
				if watcher.IsIgnoredPath(base) && path != p.WorkspacePath {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(p.WorkspacePath, path)
			if err == nil && !watcher.IsIgnoredPath(rel) {
				files = append(files, rel)
			}
			return nil
		})
	}

	items := make([]PickerItem, len(files))
	for i, f := range files {
		items[i] = PickerItem{
			Label: f,
			Value: filepath.Join(p.WorkspacePath, f),
		}
	}
	p.Items = items
}

func (p *Picker) loadBuffers(buffers []*BufferItem, activeIdx int) {
	var items []PickerItem

	// Prioritize active buffer, then others (MRU)
	for i, b := range buffers {
		status := ""
		if b.Dirty {
			status = " [+]"
		}
		label := b.FileName
		if label == "" {
			label = "[No Name]"
		}
		items = append(items, PickerItem{
			Label:  fmt.Sprintf("[%d] %s%s", b.ID, label, status),
			Value:  b.FilePath,
			Detail: b.FilePath,
			Score:  0,
		})
		if i == activeIdx {
			// Put active buffer at top initially
			items[0], items[len(items)-1] = items[len(items)-1], items[0]
		}
	}
	p.Items = items
}

func (p *Picker) executeGrep(query string) {
	if len(strings.TrimSpace(query)) < 2 {
		p.Items = nil
		p.Filtered = nil
		p.SelectedIndex = 0
		p.ScrollY = 0
		return
	}

	var results []PickerItem

	// 1. Try ripgrep (rg) if available
	if rgPath, err := exec.LookPath("rg"); err == nil && rgPath != "" {
		cmd := exec.Command(rgPath, "-n", "-i", "--no-heading", "--color=never", "--max-count=200", query)
		cmd.Dir = p.WorkspacePath
		if out, err := cmd.Output(); err == nil && len(out) > 0 {
			scanner := bufio.NewScanner(strings.NewReader(string(out)))
			for scanner.Scan() {
				line := scanner.Text()
				parts := strings.SplitN(line, ":", 3)
				if len(parts) == 3 {
					relPath := parts[0]
					lineNum := parts[1]
					textSnippet := strings.TrimSpace(parts[2])
					results = append(results, PickerItem{
						Label:  fmt.Sprintf("%s:%s: %s", relPath, lineNum, textSnippet),
						Value:  fmt.Sprintf("%s:%s", filepath.Join(p.WorkspacePath, relPath), lineNum),
						Detail: textSnippet,
					})
				}
			}
		}
	}

	// 2. Fallback: Pure Go project search if rg returned no results or is unavailable
	if len(results) == 0 {
		matches, err := search.ProjectSearch(p.WorkspacePath, search.SearchOptions{Query: query}, 200)
		if err == nil {
			for _, m := range matches {
				relPath := m.FilePath
				if rel, err := filepath.Rel(p.WorkspacePath, m.FilePath); err == nil {
					relPath = rel
				}
				results = append(results, PickerItem{
					Label:  fmt.Sprintf("%s:%d: %s", relPath, m.Line+1, strings.TrimSpace(m.LineContent)),
					Value:  fmt.Sprintf("%s:%d", m.FilePath, m.Line+1),
					Detail: strings.TrimSpace(m.LineContent),
				})
			}
		}
	}

	p.Items = results
	p.Filtered = results
	p.SelectedIndex = 0
	p.ScrollY = 0
}

// Render renders the centered floating modal window.
func (p *Picker) Render(termWidth, termHeight int, theme *Theme) []string {
	if !p.Open || termWidth < 20 || termHeight < 6 {
		return nil
	}

	modalWidth := termWidth * 66 / 100
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > termWidth-4 {
		modalWidth = termWidth - 4
	}

	modalHeight := termHeight * 66 / 100
	if modalHeight < 10 {
		modalHeight = 10
	}
	if modalHeight > termHeight-4 {
		modalHeight = termHeight - 4
	}

	innerWidth := modalWidth - 2
	listHeight := modalHeight - 4 // border top (1), query input (1), divider (1), border bottom (1)
	if listHeight < 1 {
		listHeight = 1
	}

	title := " Files "
	switch p.Mode {
	case PickerModeBuffers:
		title = " Buffers "
	case PickerModeGrep:
		title = " Live Grep "
	}

	rows := make([]string, 0, modalHeight)

	// 1. Top border: ┌─── Title ───┐
	topBorder := renderModalTopBorder(title, innerWidth, theme)
	rows = append(rows, topBorder)

	// 2. Query input: │ > query_ │
	queryDisplay := fmt.Sprintf(" > %s_", p.Query)
	truncatedQuery := ansi.Truncate(queryDisplay, innerWidth, "")
	qPad := innerWidth - ansi.StringWidth(truncatedQuery)
	if qPad < 0 {
		qPad = 0
	}
	rows = append(rows, theme.BorderLine.Render("│")+theme.TextSelected.Render(truncatedQuery)+strings.Repeat(" ", qPad)+theme.BorderLine.Render("│"))

	// 3. Divider: ├───┤
	rows = append(rows, theme.BorderLine.Render("├"+strings.Repeat("─", innerWidth)+"┤"))

	// 4. Candidate items
	for i := 0; i < listHeight; i++ {
		idx := p.ScrollY + i
		if idx >= len(p.Filtered) {
			// Blank item row
			rows = append(rows, theme.BorderLine.Render("│")+strings.Repeat(" ", innerWidth)+theme.BorderLine.Render("│"))
			continue
		}

		item := p.Filtered[idx]
		isCursor := (idx == p.SelectedIndex)

		prefix := "  "
		if isCursor {
			prefix = "▶ "
		}

		itemText := prefix + item.Label
		truncated := ansi.Truncate(itemText, innerWidth, "")
		tWidth := ansi.StringWidth(truncated)
		pad := innerWidth - tWidth
		if pad < 0 {
			pad = 0
		}

		var styled string
		if isCursor {
			styled = theme.TextSelected.Render(truncated) + strings.Repeat(" ", pad)
		} else {
			styled = theme.TextNormal.Render(truncated) + strings.Repeat(" ", pad)
		}

		rows = append(rows, theme.BorderLine.Render("│")+styled+theme.BorderLine.Render("│"))
	}

	// 5. Bottom border: └─── (Enter: Select, Esc: Close) ───┘
	bottomHint := " (Enter: Select, Esc: Close) "
	bottomBorder := renderModalBottomBorder(bottomHint, innerWidth, theme)
	rows = append(rows, bottomBorder)

	return rows
}

func renderModalTopBorder(title string, innerWidth int, theme *Theme) string {
	tWidth := runewidth.StringWidth(title)
	leftDashes := 3
	rightDashes := innerWidth - leftDashes - tWidth
	if rightDashes < 0 {
		rightDashes = 0
	}
	return theme.BorderLine.Render("┌"+strings.Repeat("─", leftDashes)) +
		theme.DrawerTitle.Render(title) +
		theme.BorderLine.Render(strings.Repeat("─", rightDashes)+"┐")
}

func renderModalBottomBorder(hint string, innerWidth int, theme *Theme) string {
	hWidth := runewidth.StringWidth(hint)
	leftDashes := 2
	rightDashes := innerWidth - leftDashes - hWidth
	if rightDashes < 0 {
		rightDashes = 0
	}
	return theme.BorderLine.Render("└"+strings.Repeat("─", leftDashes)) +
		theme.DrawerTitle.Render(hint) +
		theme.BorderLine.Render(strings.Repeat("─", rightDashes)+"┘")
}
