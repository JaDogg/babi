package git

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type filesModel struct {
	items       []DisplayItem
	cursor      int
	offset      int // scroll offset for file list
	diffContent string
	diffOffset  int // scroll offset for diff panel
	diffPath    string
	width       int
	height      int
	repoDir     string
	err         string
}

func newFilesModel(items []DisplayItem, repoDir string, width, height int) filesModel {
	m := filesModel{
		items:   items,
		repoDir: repoDir,
		width:   width,
		height:  height,
	}
	return m
}

func (m filesModel) listWidth() int  { return (m.width * 2) / 5 }
func (m filesModel) diffWidth() int  { return m.width - m.listWidth() - 1 }
func (m filesModel) contentH() int {
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (m filesModel) visibleHeight() int {
	h := m.contentH()
	if h < 1 {
		h = 1
	}
	return h
}

func (m filesModel) loadDiffCmd() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	it := m.items[m.cursor]
	if it.IsDir {
		return nil
	}
	path := it.Path
	xy := it.Status
	repoDir := m.repoDir
	return func() tea.Msg {
		content, _ := GetFileDiff(repoDir, path, xy)
		return diffLoadedMsg{path: path, content: content}
	}
}

func (m filesModel) Update(msg tea.Msg) (filesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case diffLoadedMsg:
		if msg.path == m.currentFilePath() {
			m.diffContent = msg.content
			m.diffOffset = 0
		}

	case tea.KeyMsg:
		m.err = ""
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
				m.diffContent = ""
				m.diffOffset = 0
				return m, m.loadDiffCmd()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				vh := m.visibleHeight()
				if m.cursor >= m.offset+vh {
					m.offset = m.cursor - vh + 1
				}
				m.diffContent = ""
				m.diffOffset = 0
				return m, m.loadDiffCmd()
			}
		case " ":
			m.items = m.toggle(m.cursor)
		case "a":
			for i := range m.items {
				if !m.items[i].IsDir {
					m.items[i].Selected = true
				}
			}
		case "n":
			for i := range m.items {
				if !m.items[i].IsDir {
					m.items[i].Selected = false
				}
			}
		case "pgdown", "ctrl+d":
			diffLines := strings.Count(m.diffContent, "\n")
			maxOff := diffLines - m.contentH() + 1
			if maxOff < 0 {
				maxOff = 0
			}
			m.diffOffset += m.contentH() / 2
			if m.diffOffset > maxOff {
				m.diffOffset = maxOff
			}
		case "pgup", "ctrl+u":
			m.diffOffset -= m.contentH() / 2
			if m.diffOffset < 0 {
				m.diffOffset = 0
			}
		case "c", "enter":
			paths := SelectedPaths(m.items)
			if len(paths) == 0 {
				m.err = "No files selected"
				return m, nil
			}
			return m, func() tea.Msg { return navigateToCommitMsg{files: paths} }
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m filesModel) currentFilePath() string {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return ""
	}
	it := m.items[m.cursor]
	if it.IsDir {
		return ""
	}
	return it.Path
}

func (m filesModel) toggle(idx int) []DisplayItem {
	if idx < 0 || idx >= len(m.items) {
		return m.items
	}
	it := m.items[idx]
	if it.IsDir {
		return ToggleDir(m.items, it.Path)
	}
	result := make([]DisplayItem, len(m.items))
	copy(result, m.items)
	result[idx].Selected = !result[idx].Selected
	return result
}

// Init fires the initial diff load for cursor=0.
func (m filesModel) Init() tea.Cmd {
	return m.loadDiffCmd()
}

func (m filesModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	sel := len(SelectedPaths(m.items))
	total := 0
	for _, it := range m.items {
		if !it.IsDir {
			total++
		}
	}

	right := fmt.Sprintf("%d / %d selected", sel, total)
	if m.err != "" {
		right = styleError.Render(m.err)
	}
	header := renderHeader(w, "babi commit", right)
	footer := renderFooter(w,
		"↑/↓", "navigate",
		"space", "toggle",
		"a", "all",
		"n", "none",
		"pgdn/pgup", "scroll diff",
		"c", "commit",
		"q", "quit",
	)

	contentH := h - 2
	listW := m.listWidth()
	diffW := m.diffWidth()

	listPane := m.renderList(listW, contentH)
	diffPane := m.renderDiff(diffW, contentH)

	sep := lipgloss.NewStyle().
		Foreground(colorFaint).
		Render(strings.Repeat("│\n", contentH))

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		padToHeight(listPane, contentH),
		sep,
		padToHeight(diffPane, contentH),
	)

	return header + "\n" + body + "\n" + footer
}

func (m filesModel) renderList(w, h int) string {
	if len(m.items) == 0 {
		return lipgloss.NewStyle().
			Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).
			Render("Nothing to commit.")
	}

	vh := m.visibleHeight()
	start := m.offset
	end := start + vh
	if end > len(m.items) {
		end = len(m.items)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		it := m.items[i]
		line := m.renderItem(it, i == m.cursor, w)
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func (m filesModel) renderItem(it DisplayItem, selected bool, w int) string {
	indent := strings.Repeat("  ", it.Depth)

	if it.IsDir {
		state := DirSelected(m.items, it.Path)
		var cb string
		switch state {
		case 2:
			cb = styleCheckboxOn.Render("[x]")
		case 1:
			cb = styleCheckboxMix.Render("[-]")
		default:
			cb = styleCheckboxOff.Render("[ ]")
		}
		name := styleDirHeader.Render(it.Name)
		cursor := "  "
		if selected {
			cursor = styleSelected.Render("> ")
		}
		return cursor + indent + cb + " " + name
	}

	cb := styleCheckboxOff.Render("[ ]")
	if it.Selected {
		cb = styleCheckboxOn.Render("[x]")
	}

	statusStr := it.Status
	var statusStyled string
	switch {
	case strings.Contains(statusStr, "?"):
		statusStyled = styleSubtle.Render(statusStr)
	case statusStr[0] != ' ' && statusStr[0] != '?':
		statusStyled = styleSuccess.Render(statusStr)
	default:
		statusStyled = styleWarning.Render(statusStr)
	}

	nameStr := it.Name
	if selected {
		nameStr = styleSelected.Render(nameStr)
	}

	cursor := "  "
	if selected {
		cursor = styleSelected.Render("> ")
	}

	left := cursor + indent + cb + " " + nameStr
	leftW := lipgloss.Width(left)
	statusW := lipgloss.Width(statusStyled)
	gap := w - leftW - statusW - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + statusStyled
}

func (m filesModel) renderDiff(w, h int) string {
	if m.diffContent == "" {
		it := m.currentItem()
		var placeholder string
		if it == nil || it.IsDir {
			placeholder = "select a file to see diff"
		} else {
			placeholder = "loading…"
		}
		return lipgloss.NewStyle().
			Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).
			Render(placeholder)
	}

	lines := strings.Split(strings.TrimRight(m.diffContent, "\n"), "\n")

	// Apply vertical scroll
	if m.diffOffset > 0 && m.diffOffset < len(lines) {
		lines = lines[m.diffOffset:]
	}
	if len(lines) > h {
		lines = lines[:h]
	}

	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(m.colorDiffLine(line, w) + "\n")
	}
	return sb.String()
}

func (m filesModel) currentItem() *DisplayItem {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	it := m.items[m.cursor]
	return &it
}

func (m filesModel) colorDiffLine(line string, w int) string {
	// Truncate to panel width
	if lipgloss.Width(line) > w {
		line = line[:w]
	}
	if len(line) == 0 {
		return line
	}
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		return styleSubtle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0088CC", Dark: "#56B6C2"}).Render(line)
	case line[0] == '+':
		return styleDiffAdd.Render(line)
	case line[0] == '-':
		return styleDiffDel.Render(line)
	default:
		return line
	}
}
