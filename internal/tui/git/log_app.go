package git

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── messages ────────────────────────────────────────────────────────────────

type logLoadedMsg struct {
	commits []CommitInfo
	err     error
}

type logDiffLoadedMsg struct {
	content string
	err     error
}

// ─── model ───────────────────────────────────────────────────────────────────

// LogModel is the bubbletea model for babi log.
type LogModel struct {
	repoDir string
	commits []CommitInfo
	cursor  int
	offset  int // list scroll offset
	vp      viewport.Model
	vpFocus bool // true = diff pane has focus
	loading bool
	err     string
	width   int
	height  int
}

// NewLogModel creates the log TUI root model.
func NewLogModel(repoDir string) LogModel {
	return LogModel{
		repoDir: repoDir,
		loading: true,
	}
}

func (m LogModel) Init() tea.Cmd {
	return func() tea.Msg {
		commits, err := GetLog(m.repoDir, 200)
		return logLoadedMsg{commits: commits, err: err}
	}
}

// ─── list sizing ─────────────────────────────────────────────────────────────

func (m LogModel) listHeight() int {
	contentH := m.height - 2
	if contentH < 4 {
		return 4
	}
	h := contentH * 2 / 5
	if h < 4 {
		h = 4
	}
	return h
}

func (m LogModel) diffHeight() int {
	return m.height - 2 - m.listHeight() - 1 // -1 for divider
}

// ─── update ──────────────────────────────────────────────────────────────────

func (m LogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width - 4
		m.vp.Height = m.diffHeight()
		return m, nil

	case logLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.commits = msg.commits
		if len(m.commits) > 0 {
			return m, m.loadDiff(0)
		}
		return m, nil

	case logDiffLoadedMsg:
		if msg.err == nil {
			m.vp.SetContent(colorDiff(msg.content))
			m.vp.GotoTop()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.vpFocus = !m.vpFocus
			return m, nil
		}

		if m.vpFocus {
			// diff pane focus: scroll viewport, esc to go back to list
			switch msg.String() {
			case "esc":
				m.vpFocus = false
				return m, nil
			}
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}

		// list focus
		prev := m.cursor
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.commits)-1 {
				m.cursor++
				lh := m.listHeight()
				if m.cursor >= m.offset+lh {
					m.offset = m.cursor - lh + 1
				}
			}
		case "enter":
			m.vpFocus = true
			return m, nil
		case "g":
			m.cursor = 0
			m.offset = 0
		case "G":
			m.cursor = len(m.commits) - 1
			lh := m.listHeight()
			if m.cursor >= lh {
				m.offset = m.cursor - lh + 1
			}
		}
		if m.cursor != prev && len(m.commits) > 0 {
			return m, m.loadDiff(m.cursor)
		}
	}
	return m, nil
}

func (m LogModel) loadDiff(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.commits) {
		return nil
	}
	hash := m.commits[idx].Hash
	repoDir := m.repoDir
	return func() tea.Msg {
		content, err := ShowCommit(repoDir, hash)
		return logDiffLoadedMsg{content: content, err: err}
	}
}

// ─── view ────────────────────────────────────────────────────────────────────

func (m LogModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	total := len(m.commits)
	right := fmt.Sprintf("%d commits", total)
	if m.loading {
		right = "loading…"
	}
	if m.err != "" {
		right = styleError.Render("error")
	}
	header := renderHeader(w, "babi log", right)

	var focusHint string
	if m.vpFocus {
		focusHint = styleSelected.Render("diff")
	} else {
		focusHint = styleSelected.Render("list")
	}
	footer := renderFooter(w,
		"↑/↓ k/j", "navigate",
		"tab/enter", "focus: "+focusHint,
		"g/G", "top/bottom",
		"q", "quit",
	)

	lh := m.listHeight()
	dh := m.diffHeight()

	listView := m.renderList(w, lh)
	divider := lipgloss.NewStyle().
		Width(w).
		Foreground(colorFaint).
		Render(strings.Repeat("─", w))
	m.vp.Width = w - 4
	m.vp.Height = dh
	diffView := lipgloss.NewStyle().
		Width(w).Height(dh).
		Padding(0, 2).
		Render(m.vp.View())

	content := listView + "\n" + divider + "\n" + diffView
	return header + "\n" + padToHeight(content, h-2) + "\n" + footer
}

func (m LogModel) renderList(w, h int) string {
	if m.loading {
		return lipgloss.NewStyle().Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).Render("Loading commits…")
	}
	if m.err != "" {
		return lipgloss.NewStyle().Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Render(styleError.Render(m.err))
	}
	if len(m.commits) == 0 {
		return lipgloss.NewStyle().Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).Render("No commits found.")
	}

	end := m.offset + h
	if end > len(m.commits) {
		end = len(m.commits)
	}

	var sb strings.Builder
	for i := m.offset; i < end; i++ {
		c := m.commits[i]
		cursor := "  "
		if i == m.cursor {
			cursor = styleSelected.Render("> ")
		}
		hash := styleSuccess.Render(c.Short)
		relTime := styleSubtle.Render(fmt.Sprintf("%-14s", c.RelTime))
		author := styleSubtle.Render(fmt.Sprintf("%-12s", truncate(c.Author, 12)))
		subject := c.Subject
		if i == m.cursor {
			subject = styleSelected.Render(subject)
		}
		// Available width after fixed columns
		fixedW := 2 + 7 + 1 + 14 + 1 + 12 + 1
		subjW := w - fixedW - 2
		if subjW < 10 {
			subjW = 10
		}
		subject = truncate(subject, subjW)
		sb.WriteString(fmt.Sprintf("%s%s %s %s %s\n", cursor, hash, relTime, author, subject))
	}
	return sb.String()
}

// colorDiff applies ANSI colors to a unified diff string.
func colorDiff(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			lines[i] = styleLabel.Render(line)
		case strings.HasPrefix(line, "+"):
			lines[i] = styleSuccess.Render(line)
		case strings.HasPrefix(line, "-"):
			lines[i] = styleError.Render(line)
		case strings.HasPrefix(line, "@@"):
			lines[i] = styleSubtle.Render(line)
		case strings.HasPrefix(line, "commit "), strings.HasPrefix(line, "diff --git"):
			lines[i] = styleSelected.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
