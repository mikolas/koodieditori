package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/mikolas/koodieditori/internal/watcher"
)

// FileTreeNode represents a directory or file in the FileTree.
type FileTreeNode struct {
	Path      string
	Name      string
	IsDir     bool
	Expanded  bool
	Level     int
	Children  []*FileTreeNode
	GitStatus string // "M", "A", "U", "D", ""
}

// FileTree manages the left-docked project navigation sidebar.
type FileTree struct {
	RootPath     string
	RootNode     *FileTreeNode
	Open         bool
	Width        int // Sidebar width including separator border (default: 26)
	CursorIndex  int
	ScrollY      int
	IsDragging   bool
	VisibleNodes []*FileTreeNode
}

// NewFileTree creates a new FileTree initialized at rootPath.
func NewFileTree(rootPath string, width int) *FileTree {
	if width <= 0 {
		width = 26
	}
	if width < 15 {
		width = 15
	}
	if width > 50 {
		width = 50
	}
	if rootPath == "" {
		rootPath = "."
	}

	cleanRoot, err := filepath.Abs(rootPath)
	if err != nil {
		cleanRoot = rootPath
	}

	ft := &FileTree{
		RootPath:    cleanRoot,
		Open:        false,
		Width:       width,
		CursorIndex: 0,
		ScrollY:     0,
	}

	ft.Refresh()
	return ft
}

// Toggle toggles sidebar open/closed.
func (ft *FileTree) Toggle() {
	ft.Open = !ft.Open
	if ft.Open {
		ft.Refresh()
	}
}

// Grow increases sidebar width.
func (ft *FileTree) Grow(delta int) {
	ft.Width += delta
	if ft.Width > 50 {
		ft.Width = 50
	}
}

// Shrink decreases sidebar width.
func (ft *FileTree) Shrink(delta int) {
	ft.Width -= delta
	if ft.Width < 15 {
		ft.Width = 15
	}
}

// MoveCursor moves tree selection cursor by delta lines and keeps it in view.
func (ft *FileTree) MoveCursor(delta int, visibleRows int) {
	if len(ft.VisibleNodes) == 0 {
		ft.CursorIndex = 0
		ft.ScrollY = 0
		return
	}

	ft.CursorIndex += delta
	if ft.CursorIndex < 0 {
		ft.CursorIndex = 0
	}
	if ft.CursorIndex >= len(ft.VisibleNodes) {
		ft.CursorIndex = len(ft.VisibleNodes) - 1
	}

	if visibleRows <= 0 {
		visibleRows = 10
	}

	// Adjust vertical scroll offset
	if ft.CursorIndex < ft.ScrollY {
		ft.ScrollY = ft.CursorIndex
	} else if ft.CursorIndex >= ft.ScrollY+visibleRows {
		ft.ScrollY = ft.CursorIndex - visibleRows + 1
	}
	if ft.ScrollY < 0 {
		ft.ScrollY = 0
	}
}

// SelectedNode returns the currently highlighted node, if any.
func (ft *FileTree) SelectedNode() *FileTreeNode {
	if len(ft.VisibleNodes) == 0 || ft.CursorIndex < 0 || ft.CursorIndex >= len(ft.VisibleNodes) {
		return nil
	}
	return ft.VisibleNodes[ft.CursorIndex]
}

// ToggleExpand toggles directory expansion for the selected node.
// If the selected node is a file, returns its path and true to indicate it should be opened.
func (ft *FileTree) ToggleExpand() (string, bool) {
	node := ft.SelectedNode()
	if node == nil {
		return "", false
	}

	if node.IsDir {
		node.Expanded = !node.Expanded
		if node.Expanded && len(node.Children) == 0 {
			ft.loadChildren(node)
		}
		ft.updateVisibleNodes()
		return "", false
	}

	// It's a file
	return node.Path, true
}

// CollapseOrParent collapses the directory if currently expanded.
// If already collapsed or if on a file, moves the cursor to the parent directory.
func (ft *FileTree) CollapseOrParent() {
	node := ft.SelectedNode()
	if node == nil {
		return
	}

	if node.IsDir && node.Expanded {
		node.Expanded = false
		ft.updateVisibleNodes()
		return
	}

	// Move cursor to parent directory
	parentPath := filepath.Dir(node.Path)
	for i, n := range ft.VisibleNodes {
		if filepath.Clean(n.Path) == filepath.Clean(parentPath) {
			ft.CursorIndex = i
			if ft.CursorIndex < ft.ScrollY {
				ft.ScrollY = ft.CursorIndex
			}
			return
		}
	}
}

// CursorToTop jumps the cursor to the top of the tree.
func (ft *FileTree) CursorToTop() {
	ft.CursorIndex = 0
	ft.ScrollY = 0
}

// CursorToBottom jumps the cursor to the bottom of the tree.
func (ft *FileTree) CursorToBottom(visibleRows int) {
	if len(ft.VisibleNodes) == 0 {
		ft.CursorIndex = 0
		ft.ScrollY = 0
		return
	}
	ft.CursorIndex = len(ft.VisibleNodes) - 1
	if visibleRows <= 0 {
		visibleRows = 10
	}
	if ft.CursorIndex >= ft.ScrollY+visibleRows {
		ft.ScrollY = ft.CursorIndex - visibleRows + 1
	}
	if ft.ScrollY < 0 {
		ft.ScrollY = 0
	}
}

// CreateFile creates a new file inside the directory of the selected node.
func (ft *FileTree) CreateFile(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("file name cannot be empty")
	}

	dir := ft.RootPath
	if node := ft.SelectedNode(); node != nil {
		if node.IsDir {
			dir = node.Path
			node.Expanded = true
		} else {
			dir = filepath.Dir(node.Path)
		}
	}

	targetPath := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	_ = f.Close()

	ft.Refresh()
	// Position cursor on newly created file
	for i, n := range ft.VisibleNodes {
		if filepath.Clean(n.Path) == filepath.Clean(targetPath) {
			ft.CursorIndex = i
			break
		}
	}
	return targetPath, nil
}

// CreateDir creates a new directory inside the directory of the selected node.
func (ft *FileTree) CreateDir(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("directory name cannot be empty")
	}

	dir := ft.RootPath
	if node := ft.SelectedNode(); node != nil {
		if node.IsDir {
			dir = node.Path
			node.Expanded = true
		} else {
			dir = filepath.Dir(node.Path)
		}
	}

	targetPath := filepath.Join(dir, name)
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return "", err
	}

	ft.Refresh()
	// Position cursor on newly created directory
	for i, n := range ft.VisibleNodes {
		if filepath.Clean(n.Path) == filepath.Clean(targetPath) {
			ft.CursorIndex = i
			break
		}
	}
	return targetPath, nil
}

// DeleteSelected deletes the selected file or directory from disk.
func (ft *FileTree) DeleteSelected() error {
	node := ft.SelectedNode()
	if node == nil {
		return nil
	}
	if filepath.Clean(node.Path) == filepath.Clean(ft.RootPath) {
		return fmt.Errorf("cannot delete root workspace directory")
	}

	if err := os.RemoveAll(node.Path); err != nil {
		return err
	}

	ft.Refresh()
	return nil
}

// Refresh re-scans the root directory and updates git status flags.
func (ft *FileTree) Refresh() {
	rootBase := filepath.Base(ft.RootPath)
	if rootBase == "." || rootBase == "/" {
		rootBase = "workspace"
	}

	// Preserve expanded state of directories across refreshes
	expandedMap := make(map[string]bool)
	ft.collectExpanded(ft.RootNode, expandedMap)

	gitStatuses := ft.queryGitStatuses()

	rootNode := &FileTreeNode{
		Path:     ft.RootPath,
		Name:     rootBase,
		IsDir:    true,
		Expanded: true,
		Level:    0,
	}

	ft.loadChildrenRecursive(rootNode, expandedMap, gitStatuses)
	ft.RootNode = rootNode
	ft.updateVisibleNodes()

	if ft.CursorIndex >= len(ft.VisibleNodes) {
		ft.CursorIndex = max(0, len(ft.VisibleNodes)-1)
	}
}

func (ft *FileTree) collectExpanded(node *FileTreeNode, m map[string]bool) {
	if node == nil {
		return
	}
	if node.IsDir && node.Expanded {
		m[node.Path] = true
	}
	for _, child := range node.Children {
		ft.collectExpanded(child, m)
	}
}

func (ft *FileTree) loadChildren(node *FileTreeNode) {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return
	}

	var dirs []*FileTreeNode
	var files []*FileTreeNode

	for _, entry := range entries {
		name := entry.Name()
		if watcher.IsIgnoredPath(name) {
			continue
		}

		childPath := filepath.Join(node.Path, name)
		childNode := &FileTreeNode{
			Path:     childPath,
			Name:     name,
			IsDir:    entry.IsDir(),
			Expanded: false,
			Level:    node.Level + 1,
		}

		if entry.IsDir() {
			dirs = append(dirs, childNode)
		} else {
			files = append(files, childNode)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	node.Children = append(dirs, files...)
}

func (ft *FileTree) loadChildrenRecursive(node *FileTreeNode, expandedMap map[string]bool, gitStatuses map[string]string) {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return
	}

	var dirs []*FileTreeNode
	var files []*FileTreeNode

	for _, entry := range entries {
		name := entry.Name()
		if watcher.IsIgnoredPath(name) {
			continue
		}

		childPath := filepath.Join(node.Path, name)
		childNode := &FileTreeNode{
			Path:      childPath,
			Name:      name,
			IsDir:     entry.IsDir(),
			Expanded:  expandedMap[childPath],
			Level:     node.Level + 1,
			GitStatus: gitStatuses[filepath.Clean(childPath)],
		}

		if entry.IsDir() {
			if childNode.Expanded {
				ft.loadChildrenRecursive(childNode, expandedMap, gitStatuses)
			}
			dirs = append(dirs, childNode)
		} else {
			files = append(files, childNode)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	node.Children = append(dirs, files...)
}

func (ft *FileTree) updateVisibleNodes() {
	ft.VisibleNodes = make([]*FileTreeNode, 0)
	if ft.RootNode == nil {
		return
	}
	ft.flattenNode(ft.RootNode)
}

func (ft *FileTree) flattenNode(node *FileTreeNode) {
	ft.VisibleNodes = append(ft.VisibleNodes, node)
	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			ft.flattenNode(child)
		}
	}
}

func (ft *FileTree) queryGitStatuses() map[string]string {
	res := make(map[string]string)
	if ft.RootPath == "" {
		return res
	}

	// 1. Query modified, added, deleted files using git diff-index (Rule 6 compliant: no git status execution)
	cmdDiff := exec.Command("git", "diff-index", "--name-status", "HEAD")
	cmdDiff.Dir = ft.RootPath
	if out, err := cmdDiff.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if len(l) < 2 {
				continue
			}
			parts := strings.Fields(l)
			if len(parts) >= 2 {
				status := parts[0]
				relPath := parts[1]
				absPath := filepath.Clean(filepath.Join(ft.RootPath, relPath))
				badge := ""
				switch {
				case strings.HasPrefix(status, "M"):
					badge = "M"
				case strings.HasPrefix(status, "A"):
					badge = "A"
				case strings.HasPrefix(status, "D"):
					badge = "D"
				default:
					badge = status
				}
				res[absPath] = badge
			}
		}
	}

	// 2. Query untracked files using git ls-files --others --exclude-standard
	cmdUntracked := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmdUntracked.Dir = ft.RootPath
	if out, err := cmdUntracked.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			relPath := strings.TrimSpace(l)
			if relPath != "" {
				absPath := filepath.Clean(filepath.Join(ft.RootPath, relPath))
				res[absPath] = "U"
			}
		}
	}

	return res
}

// Render renders the FileTree lines for height rows.
// Each line is guaranteed to have display width == ft.Width.
func (ft *FileTree) Render(height int, theme *Theme, isFocused bool) []string {
	if height <= 0 || ft.Width <= 0 {
		return nil
	}

	contentWidth := ft.Width - 1 // Leave 1 col for vertical border │
	if contentWidth < 1 {
		contentWidth = 1
	}

	rows := make([]string, 0, height)
	totalNodes := len(ft.VisibleNodes)

	for r := 0; r < height; r++ {
		nodeIdx := ft.ScrollY + r
		if nodeIdx >= totalNodes {
			// Blank row in file tree
			rowText := strings.Repeat(" ", contentWidth) + theme.BorderLine.Render("│")
			rows = append(rows, rowText)
			continue
		}

		node := ft.VisibleNodes[nodeIdx]
		isCursor := (nodeIdx == ft.CursorIndex && isFocused)

		// Indentation
		indent := strings.Repeat("  ", node.Level)

		// Icon
		icon := "  "
		if node.IsDir {
			if node.Expanded {
				icon = "▾ "
			} else {
				icon = "▸ "
			}
		}

		// Git status badge
		gitBadge := " "
		if node.GitStatus != "" {
			gitBadge = node.GitStatus + " "
		}

		// Cursor prefix
		cursorPrefix := " "
		if isCursor {
			cursorPrefix = "▶"
		}

		rawLine := sanitizeDrawerLine(cursorPrefix+indent+icon+node.Name, 4)
		badgeWidth := runewidth.StringWidth(gitBadge)

		availForName := contentWidth - badgeWidth
		if availForName < 1 {
			availForName = 1
		}

		truncated := ansi.Truncate(rawLine, availForName, "")
		tWidth := ansi.StringWidth(truncated)
		pad := contentWidth - tWidth - badgeWidth
		if pad < 0 {
			pad = 0
		}

		// Styling
		var styledText string
		if isCursor {
			styledText = theme.TextSelected.Render(truncated + strings.Repeat(" ", pad))
		} else if node.IsDir {
			styledText = theme.DrawerTitle.Render(truncated) + strings.Repeat(" ", pad)
		} else {
			styledText = theme.TextNormal.Render(truncated) + strings.Repeat(" ", pad)
		}

		var styledBadge string
		switch node.GitStatus {
		case "M":
			styledBadge = theme.DiffModified.Render(gitBadge)
		case "A":
			styledBadge = theme.DiffAdded.Render(gitBadge)
		case "D":
			styledBadge = theme.DiffDeleted.Render(gitBadge)
		case "U":
			styledBadge = theme.DiagInfo.Render(gitBadge)
		default:
			styledBadge = gitBadge
		}

		border := theme.BorderLine.Render("│")
		rowStr := clampAndPadLine(styledText+styledBadge, contentWidth) + border
		rows = append(rows, rowStr)
	}

	return rows
}
