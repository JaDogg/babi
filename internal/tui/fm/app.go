// Package fm implements a two-pane file manager TUI using bubbletea and lipgloss.
package fm

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#02A699", Dark: "#04D18A"}
	colorError   = lipgloss.AdaptiveColor{Light: "#CC3333", Dark: "#FF5555"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#626262"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#DEDEDE", Dark: "#333333"}
	colorBg      = lipgloss.AdaptiveColor{Light: "#F0EEFF", Dark: "#1E1A2E"}

	styleHeaderBg    = lipgloss.NewStyle().Background(colorPrimary)
	styleHeaderTitle = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 2)
	styleHeaderRight = lipgloss.NewStyle().Background(colorPrimary).Foreground(lipgloss.Color("#CCC8FF")).Padding(0, 2)
	styleFooterBar   = lipgloss.NewStyle().Background(colorFaint).Foreground(colorMuted).Padding(0, 2)
	styleFooterKey   = lipgloss.NewStyle().Background(colorMuted).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	styleActiveRow   = lipgloss.NewStyle().Background(colorBg).Foreground(colorPrimary).Bold(true)
	styleDimRow      = lipgloss.NewStyle().Foreground(colorMuted)
	stylePaneDiv     = lipgloss.NewStyle().Foreground(colorFaint)
	styleDirName     = lipgloss.NewStyle().Bold(true)
	styleSuccess     = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError       = lipgloss.NewStyle().Foreground(colorError)
	styleSubtle      = lipgloss.NewStyle().Foreground(colorMuted)
)

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

// renderHeader renders the top bar with a title on the left and a right-hand
// label (typically the current directory). Both sides are padded to fill width.
func renderHeader(width int, title, right string) string {
	titleStr := styleHeaderTitle.Render(title)
	rightStr := styleHeaderRight.Render(right)

	titleW := lipgloss.Width(titleStr)
	rightW := lipgloss.Width(rightStr)
	gap := width - titleW - rightW
	if gap < 0 {
		gap = 0
	}
	fill := styleHeaderBg.Render(strings.Repeat(" ", gap))
	return titleStr + fill + rightStr
}

// renderFooter renders a footer bar from key/description pairs.
// pairs should be an even-length list: key, description, key, description…
func renderFooter(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		k := styleFooterKey.Render(pairs[i])
		d := styleSubtle.Render(" " + pairs[i+1] + "  ")
		parts = append(parts, k+d)
	}
	content := strings.Join(parts, "")
	// pad to full width
	content += strings.Repeat(" ", max(0, width-lipgloss.Width(content)))
	return styleFooterBar.Render(content)
}

// padToHeight ensures s has at least height lines by appending blank lines.
func padToHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

// humanSize converts a byte count to a human-readable string.
func humanSize(n int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Pane
// ---------------------------------------------------------------------------

type pane struct {
	dir     string
	entries []os.DirEntry
	cursor  int
	offset  int // scroll offset
}

// load reads and sorts directory entries for the pane.
func (p *pane) load() error {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return err
	}
	p.entries = entries
	// clamp cursor
	if p.cursor >= len(p.entries)+1 { // +1 for ".."
		p.cursor = max(0, len(p.entries))
	}
	return nil
}

// selectedName returns the filesystem name of the currently highlighted item.
// Returns ".." when cursor == 0.
func (p *pane) selectedName() string {
	if p.cursor == 0 {
		return ".."
	}
	return p.entries[p.cursor-1].Name()
}

// selectedPath returns the full absolute path of the highlighted item.
func (p *pane) selectedPath() string {
	if p.cursor == 0 {
		return filepath.Dir(p.dir)
	}
	return filepath.Join(p.dir, p.entries[p.cursor-1].Name())
}

// selectedIsDir reports whether the highlighted item is a directory.
func (p *pane) selectedIsDir() bool {
	if p.cursor == 0 {
		return true
	}
	return p.entries[p.cursor-1].IsDir()
}

// renderPane renders the full pane content (excluding outer borders).
// active controls the cursor style; listHeight is the number of rows
// available for entries (between the two dividers).
func (p *pane) renderPane(paneWidth int, listHeight int, active bool) string {
	var sb strings.Builder

	// ── directory path ──
	dirLine := truncate(p.dir, paneWidth)
	sb.WriteString(styleSubtle.Render(dirLine))
	sb.WriteString("\n")

	// ── top divider ──
	sb.WriteString(stylePaneDiv.Render(strings.Repeat("─", paneWidth)))
	sb.WriteString("\n")

	// Build visible window of entries (cursor 0 = "..")
	total := len(p.entries) + 1 // +1 for ".."

	// Keep cursor in view
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+listHeight {
		p.offset = p.cursor - listHeight + 1
	}
	if p.offset < 0 {
		p.offset = 0
	}

	for row := 0; row < listHeight; row++ {
		idx := p.offset + row
		if idx >= total {
			sb.WriteString(strings.Repeat(" ", paneWidth))
			sb.WriteString("\n")
			continue
		}

		isCursor := idx == p.cursor
		var line string
		if idx == 0 {
			line = renderEntryRow("..", true, -1, paneWidth, isCursor, active)
		} else {
			entry := p.entries[idx-1]
			var size int64
			if !entry.IsDir() {
				if fi, err := entry.Info(); err == nil {
					size = fi.Size()
				}
			}
			line = renderEntryRow(entry.Name(), entry.IsDir(), size, paneWidth, isCursor, active)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// ── bottom divider ──
	sb.WriteString(stylePaneDiv.Render(strings.Repeat("─", paneWidth)))
	sb.WriteString("\n")

	// ── item count ──
	count := len(p.entries)
	countStr := fmt.Sprintf("%d item", count)
	if count != 1 {
		countStr += "s"
	}
	sb.WriteString(styleSubtle.Render(truncate(countStr, paneWidth)))

	return sb.String()
}

// renderEntryRow formats a single row in the file list.
func renderEntryRow(name string, isDir bool, size int64, width int, isCursor, activePane bool) string {
	const prefixLen = 2 // "> " or "  "

	var prefix string
	if isCursor {
		prefix = "> "
	} else {
		prefix = "  "
	}

	inner := width - prefixLen

	var rightLabel string
	if name == ".." {
		rightLabel = ""
	} else if isDir {
		rightLabel = "<DIR>"
	} else {
		rightLabel = humanSize(size)
	}

	// Name column: left-aligned, right label right-aligned
	nameField := name
	if isDir && name != ".." {
		nameField = name + "/"
	}

	// Compose the content
	var content string
	if rightLabel == "" {
		content = padRight(nameField, inner)
	} else {
		rightLen := utf8.RuneCountInString(rightLabel)
		nameLen := inner - rightLen - 1 // 1 space gap
		if nameLen < 1 {
			nameLen = 1
		}
		content = padRight(nameField, nameLen) + " " + rightLabel
	}
	content = truncate(content, inner)

	row := prefix + content

	isHidden := strings.HasPrefix(name, ".") && name != ".."

	switch {
	case isCursor && activePane:
		return styleActiveRow.Render(padRight(row, width))
	case isCursor && !activePane:
		return styleDimRow.Render(padRight(row, width))
	case isHidden:
		return styleDimRow.Render(padRight(row, width))
	case isDir && name != "..":
		return styleDirName.Render(padRight(row, width))
	default:
		return padRight(row, width)
	}
}

// ---------------------------------------------------------------------------
// String utilities
// ---------------------------------------------------------------------------

func padRight(s string, n int) string {
	l := utf8.RuneCountInString(s)
	if l >= n {
		return s
	}
	return s + strings.Repeat(" ", n-l)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}

// ---------------------------------------------------------------------------
// File operations
// ---------------------------------------------------------------------------

// copyDirRecursive copies a directory tree from src to dst.
func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// openFile launches a file with the system default application.
func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type opMode int

const (
	opNone opMode = iota
	opConfirmDelete
	opRename
	opNewDir
)

// Model is the root bubbletea model for the file manager.
type Model struct {
	left   pane
	right  pane
	active int    // 0=left, 1=right
	op     opMode
	input  string // for rename/newdir prompts
	status string // transient status message
	width  int
	height int
}

// New creates a new file manager Model with both panes starting in dir.
func New(dir string) (Model, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Model{}, err
	}
	m := Model{
		left:   pane{dir: abs},
		right:  pane{dir: abs},
		active: 0,
	}
	if err := m.left.load(); err != nil {
		return Model{}, err
	}
	if err := m.right.load(); err != nil {
		return Model{}, err
	}
	return m, nil
}

// activePane returns a pointer to the currently active pane.
func (m *Model) activePane() *pane {
	if m.active == 0 {
		return &m.left
	}
	return &m.right
}

// otherPane returns a pointer to the inactive pane.
func (m *Model) otherPane() *pane {
	if m.active == 0 {
		return &m.right
	}
	return &m.left
}

// reloadBoth refreshes directory listings in both panes.
func (m *Model) reloadBoth() {
	_ = m.left.load()
	_ = m.right.load()
}

// ---------------------------------------------------------------------------
// tea.Model implementation
// ---------------------------------------------------------------------------

// Init satisfies tea.Model; the file manager needs no initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles all key events and window resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// When in an input mode, route to input handler.
		if m.op == opRename || m.op == opNewDir {
			return m.handleInputKey(msg)
		}
		if m.op == opConfirmDelete {
			return m.handleConfirmKey(msg)
		}
		return m.handleNormalKey(msg)
	}

	return m, nil
}

// handleNormalKey processes keys in normal navigation mode.
func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ap := m.activePane()
	total := len(ap.entries) + 1 // +1 for ".."

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.active = 1 - m.active
		m.status = ""

	case "j", "down":
		m.status = ""
		if ap.cursor < total-1 {
			ap.cursor++
		}

	case "k", "up":
		m.status = ""
		if ap.cursor > 0 {
			ap.cursor--
		}

	case "enter", "right", "l":
		m.status = ""
		if ap.selectedIsDir() {
			newDir := ap.selectedPath()
			ap.dir = newDir
			ap.cursor = 0
			ap.offset = 0
			_ = ap.load()
		} else {
			_ = openFile(ap.selectedPath())
		}

	case "backspace", "left", "h":
		m.status = ""
		parent := filepath.Dir(ap.dir)
		if parent != ap.dir {
			oldDir := ap.dir
			ap.dir = parent
			ap.cursor = 0
			ap.offset = 0
			_ = ap.load()
			// Try to position cursor on the directory we just came from
			base := filepath.Base(oldDir)
			for i, e := range ap.entries {
				if e.Name() == base {
					ap.cursor = i + 1
					break
				}
			}
		}

	case "c":
		m.status = ""
		src := ap.selectedPath()
		name := ap.selectedName()
		if name == ".." {
			break
		}
		dst := filepath.Join(m.otherPane().dir, name)
		var err error
		if ap.selectedIsDir() {
			err = copyDirRecursive(src, dst)
		} else {
			err = copyFile(src, dst)
		}
		if err != nil {
			m.status = styleError.Render("Error: " + err.Error())
		} else {
			m.status = styleSuccess.Render("Copied " + name)
		}
		m.reloadBoth()

	case "m":
		m.status = ""
		src := ap.selectedPath()
		name := ap.selectedName()
		if name == ".." {
			break
		}
		dst := filepath.Join(m.otherPane().dir, name)
		err := os.Rename(src, dst)
		if err != nil {
			// Fallback: copy then delete
			if ap.selectedIsDir() {
				err = copyDirRecursive(src, dst)
			} else {
				err = copyFile(src, dst)
			}
			if err == nil {
				err = os.RemoveAll(src)
			}
		}
		if err != nil {
			m.status = styleError.Render("Error: " + err.Error())
		} else {
			m.status = styleSuccess.Render("Moved " + name)
			if ap.cursor > 0 {
				ap.cursor--
			}
		}
		m.reloadBoth()

	case "d":
		name := ap.selectedName()
		if name == ".." {
			break
		}
		m.op = opConfirmDelete
		m.status = ""

	case "r":
		name := ap.selectedName()
		if name == ".." {
			break
		}
		m.op = opRename
		m.input = name

	case "n":
		m.op = opNewDir
		m.input = ""
	}

	return m, nil
}

// handleConfirmKey processes keys in the delete confirmation prompt.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		ap := m.activePane()
		path := ap.selectedPath()
		name := ap.selectedName()
		err := os.RemoveAll(path)
		m.op = opNone
		if err != nil {
			m.status = styleError.Render("Error: " + err.Error())
		} else {
			m.status = styleSuccess.Render("Deleted " + name)
			if ap.cursor > 0 {
				ap.cursor--
			}
		}
		m.reloadBoth()
	case "n", "N", "esc":
		m.op = opNone
		m.status = ""
	}
	return m, nil
}

// handleInputKey processes keystrokes for rename / new-dir prompts.
func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.op = opNone
		m.input = ""
		m.status = ""

	case "enter":
		ap := m.activePane()
		if m.op == opRename {
			oldPath := ap.selectedPath()
			newPath := filepath.Join(ap.dir, m.input)
			err := os.Rename(oldPath, newPath)
			m.op = opNone
			if err != nil {
				m.status = styleError.Render("Error: " + err.Error())
			} else {
				m.status = styleSuccess.Render("Renamed to " + m.input)
			}
			m.input = ""
			m.reloadBoth()
		} else if m.op == opNewDir {
			newPath := filepath.Join(ap.dir, m.input)
			err := os.MkdirAll(newPath, 0o755)
			m.op = opNone
			if err != nil {
				m.status = styleError.Render("Error: " + err.Error())
			} else {
				m.status = styleSuccess.Render("Created " + m.input)
			}
			m.input = ""
			m.reloadBoth()
		}

	case "backspace":
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}

	default:
		// Append printable rune(s)
		if len(msg.Runes) > 0 {
			m.input += string(msg.Runes)
		}
	}
	return m, nil
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	// 1 separator column between panes
	paneWidth := (m.width - 1) / 2

	// Vertical space budget:
	//   header: 1 line
	//   footer: 1 line
	//   pane header (dir path): 1 line
	//   top divider: 1 line
	//   bottom divider: 1 line
	//   pane footer (item count): 1 line
	const fixedRows = 1 + 1 + 1 + 1 + 1 + 1
	listHeight := m.height - fixedRows
	if listHeight < 1 {
		listHeight = 1
	}

	// Active pane directory for the header right label
	currentDir := m.activePane().dir

	// ── Header ──────────────────────────────────────────────────────
	header := renderHeader(m.width, "babi fm", currentDir)

	// ── Panes ───────────────────────────────────────────────────────
	leftContent := m.left.renderPane(paneWidth, listHeight, m.active == 0)
	rightContent := m.right.renderPane(paneWidth, listHeight, m.active == 1)

	// Split each pane's rendered output into lines and zip them side-by-side
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	sep := stylePaneDiv.Render("│")

	var body strings.Builder
	totalLines := max(len(leftLines), len(rightLines))
	for i := 0; i < totalLines; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		// Ensure each side is exactly paneWidth wide (visual width)
		l = ensureWidth(l, paneWidth)
		r = ensureWidth(r, paneWidth)
		body.WriteString(l + sep + r + "\n")
	}

	// ── Footer ──────────────────────────────────────────────────────
	var footer string
	switch m.op {
	case opConfirmDelete:
		name := m.activePane().selectedName()
		prompt := fmt.Sprintf("Delete '%s'? (y/n)  ", name)
		footer = styleFooterBar.Width(m.width).Render(styleError.Render(prompt))
	case opRename:
		prompt := "Rename to: " + m.input + "█"
		footer = styleFooterBar.Width(m.width).Render(prompt)
	case opNewDir:
		prompt := "New dir: " + m.input + "█"
		footer = styleFooterBar.Width(m.width).Render(prompt)
	default:
		if m.status != "" {
			// Show transient status alongside key hints
			status := m.status + "  "
			hints := renderFooter(m.width,
				"tab", "switch",
				"enter", "open",
				"h", "back",
				"c", "copy",
				"m", "move",
				"d", "del",
				"r", "rename",
				"n", "mkdir",
				"q", "quit",
			)
			_ = hints
			footer = styleFooterBar.Width(m.width).Render(status)
		} else {
			footer = renderFooter(m.width,
				"tab", "switch",
				"enter", "open",
				"h", "back",
				"c", "copy",
				"m", "move",
				"d", "del",
				"r", "rename",
				"n", "mkdir",
				"q", "quit",
			)
		}
	}

	return header + "\n" + body.String() + footer
}

// ensureWidth pads or truncates a styled string to exactly w visual columns.
func ensureWidth(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis < w {
		return s + strings.Repeat(" ", w-vis)
	}
	if vis > w {
		// Strip and rebuild — simple fallback: just return as-is
		// (lipgloss can't easily truncate styled strings)
		return s
	}
	return s
}
