package git

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type filesModel struct {
	items   []DisplayItem
	cursor  int
	offset  int // scroll offset
	width   int
	height  int
	repoDir string
	err     string
}

func newFilesModel(items []DisplayItem, repoDir string, width, height int) filesModel {
	return filesModel{
		items:   items,
		repoDir: repoDir,
		width:   width,
		height:  height,
	}
}

func (m filesModel) visibleHeight() int {
	h := m.height - 2 // header + footer
	if h < 1 {
		h = 1
	}
	return h
}

func (m filesModel) Update(msg tea.Msg) (filesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.err = ""
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				vh := m.visibleHeight()
				if m.cursor >= m.offset+vh {
					m.offset = m.cursor - vh + 1
				}
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
		"c", "commit",
		"q", "quit",
	)

	contentH := h - 2
	content := m.renderList(w, contentH)

	return header + "\n" + padToHeight(content, contentH) + "\n" + footer
}

func (m filesModel) renderList(w, h int) string {
	if len(m.items) == 0 {
		return lipgloss.NewStyle().
			Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).
			Render("Nothing to commit — working tree clean.")
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

	// File row
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

	// Right-align status
	left := cursor + indent + cb + " " + nameStr
	leftW := lipgloss.Width(left)
	statusW := lipgloss.Width(statusStyled)
	gap := w - leftW - statusW - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + statusStyled
}
