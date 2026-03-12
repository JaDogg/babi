package git

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── messages ────────────────────────────────────────────────────────────────

type stashLoadedMsg struct {
	stashes []StashInfo
	err     error
}

type stashDiffLoadedMsg struct {
	content string
	err     error
}

type stashOpDoneMsg struct {
	output string
	err    error
}

// ─── model ───────────────────────────────────────────────────────────────────

// StashModel is the bubbletea model for babi stash.
type StashModel struct {
	repoDir string
	stashes []StashInfo
	cursor  int
	offset  int
	vp      viewport.Model
	vpFocus bool
	confirm string       // "drop" when waiting for y/n confirmation
	newMode bool         // creating a new stash
	input   textinput.Model
	loading bool
	err     string
	status  string
	width   int
	height  int
}

// NewStashModel creates the stash TUI root model.
func NewStashModel(repoDir string) StashModel {
	inp := textinput.New()
	inp.Placeholder = "stash message (optional)"
	inp.CharLimit = 72
	return StashModel{
		repoDir: repoDir,
		loading: true,
		input:   inp,
	}
}

func (m StashModel) Init() tea.Cmd {
	return m.doLoad()
}

func (m StashModel) doLoad() tea.Cmd {
	repoDir := m.repoDir
	return func() tea.Msg {
		stashes, err := GetStashes(repoDir)
		return stashLoadedMsg{stashes: stashes, err: err}
	}
}

func (m StashModel) listHeight() int {
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

func (m StashModel) diffHeight() int {
	return m.height - 2 - m.listHeight() - 1
}

// ─── update ──────────────────────────────────────────────────────────────────

func (m StashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width - 4
		m.vp.Height = m.diffHeight()
		return m, nil

	case stashLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.stashes = msg.stashes
		m.cursor = 0
		m.offset = 0
		if len(m.stashes) > 0 {
			return m, m.loadDiff(0)
		}
		return m, nil

	case stashDiffLoadedMsg:
		if msg.err == nil {
			m.vp.SetContent(colorDiff(msg.content))
			m.vp.GotoTop()
		}
		return m, nil

	case stashOpDoneMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.status = strings.TrimSpace(msg.output)
		}
		m.confirm = ""
		m.loading = true
		return m, m.doLoad()

	case tea.KeyMsg:
		// New stash input mode
		if m.newMode {
			switch msg.String() {
			case "enter":
				message := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				m.newMode = false
				m.loading = true
				repoDir := m.repoDir
				return m, func() tea.Msg {
					out, err := CreateStash(repoDir, message)
					return stashOpDoneMsg{output: out, err: err}
				}
			case "esc":
				m.newMode = false
				m.input.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}

		// Confirm drop
		if m.confirm == "drop" {
			switch msg.String() {
			case "y", "Y":
				if m.cursor < len(m.stashes) {
					ref := m.stashes[m.cursor].Ref
					repoDir := m.repoDir
					m.loading = true
					m.confirm = ""
					return m, func() tea.Msg {
						out, err := DropStash(repoDir, ref)
						return stashOpDoneMsg{output: out, err: err}
					}
				}
				m.confirm = ""
			default:
				m.confirm = ""
			}
			return m, nil
		}

		m.err = ""
		m.status = ""

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.vpFocus = !m.vpFocus
			return m, nil
		}

		if m.vpFocus {
			switch msg.String() {
			case "esc":
				m.vpFocus = false
				return m, nil
			}
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}

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
			if m.cursor < len(m.stashes)-1 {
				m.cursor++
				lh := m.listHeight()
				if m.cursor >= m.offset+lh {
					m.offset = m.cursor - lh + 1
				}
			}
		case "enter":
			m.vpFocus = true
			return m, nil
		case "a":
			if m.cursor < len(m.stashes) {
				ref := m.stashes[m.cursor].Ref
				repoDir := m.repoDir
				m.loading = true
				return m, func() tea.Msg {
					out, err := ApplyStash(repoDir, ref)
					return stashOpDoneMsg{output: out, err: err}
				}
			}
		case "p":
			if m.cursor < len(m.stashes) {
				ref := m.stashes[m.cursor].Ref
				repoDir := m.repoDir
				m.loading = true
				return m, func() tea.Msg {
					out, err := PopStash(repoDir, ref)
					return stashOpDoneMsg{output: out, err: err}
				}
			}
		case "d":
			if len(m.stashes) > 0 {
				m.confirm = "drop"
			}
		case "n":
			m.newMode = true
			m.input.Focus()
			return m, textinput.Blink
		case "r":
			m.loading = true
			return m, m.doLoad()
		}

		if m.cursor != prev && len(m.stashes) > 0 {
			return m, m.loadDiff(m.cursor)
		}
	}
	return m, nil
}

func (m StashModel) loadDiff(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.stashes) {
		return nil
	}
	ref := m.stashes[idx].Ref
	repoDir := m.repoDir
	return func() tea.Msg {
		content, err := ShowStash(repoDir, ref)
		return stashDiffLoadedMsg{content: content, err: err}
	}
}

// ─── view ────────────────────────────────────────────────────────────────────

func (m StashModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	right := fmt.Sprintf("%d stashes", len(m.stashes))
	if m.loading {
		right = "loading…"
	}
	if m.err != "" {
		right = styleError.Render("error")
	}
	header := renderHeader(w, "babi stash", right)

	var footer string
	if m.newMode {
		footer = renderFooter(w, "enter", "save", "esc", "cancel")
	} else if m.confirm == "drop" {
		footer = styleFooterBar.Width(w - 4).Render(
			styleWarning.Render("Drop stash? ")+styleFooterKey.Render("y")+" yes  "+styleFooterKey.Render("n")+" cancel",
		)
	} else {
		focusLabel := styleSelected.Render("list")
		if m.vpFocus {
			focusLabel = styleSelected.Render("diff")
		}
		footer = renderFooter(w,
			"↑/↓", "navigate",
			"a", "apply",
			"p", "pop",
			"d", "drop",
			"n", "new",
			"tab", "focus: "+focusLabel,
			"q", "quit",
		)
	}

	lh := m.listHeight()
	dh := m.diffHeight()

	listView := m.renderList(w, lh)

	var inputBar string
	if m.newMode {
		inputBar = lipgloss.NewStyle().Width(w).Padding(0, 2).Render(
			styleLabel.Render("New stash: ") + m.input.View(),
		)
	}

	divider := lipgloss.NewStyle().
		Width(w).Foreground(colorFaint).
		Render(strings.Repeat("─", w))

	m.vp.Width = w - 4
	m.vp.Height = dh
	diffView := lipgloss.NewStyle().Width(w).Height(dh).Padding(0, 2).Render(m.vp.View())

	var statusLine string
	if m.status != "" {
		statusLine = "\n" + styleSuccess.Render("  "+m.status)
	} else if m.err != "" {
		statusLine = "\n" + styleError.Render("  "+m.err)
	}

	var content string
	if m.newMode {
		content = listView + "\n" + inputBar + "\n" + divider + "\n" + diffView
	} else {
		content = listView + statusLine + "\n" + divider + "\n" + diffView
	}

	return header + "\n" + padToHeight(content, h-2) + "\n" + footer
}

func (m StashModel) renderList(w, h int) string {
	if m.loading {
		return lipgloss.NewStyle().Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).Render("Loading stashes…")
	}
	if len(m.stashes) == 0 {
		return lipgloss.NewStyle().Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).Render("No stashes. Press n to create one.")
	}

	end := m.offset + h
	if end > len(m.stashes) {
		end = len(m.stashes)
	}

	var sb strings.Builder
	for i := m.offset; i < end; i++ {
		s := m.stashes[i]
		cursor := "  "
		if i == m.cursor {
			cursor = styleSelected.Render("> ")
		}
		ref := styleSuccess.Render(fmt.Sprintf("%-12s", s.Ref))
		relTime := styleSubtle.Render(fmt.Sprintf("%-14s", s.RelTime))
		msg := s.Message
		if i == m.cursor {
			msg = styleSelected.Render(msg)
		}
		fixedW := 2 + 12 + 1 + 14 + 1
		msgW := w - fixedW - 2
		if msgW < 10 {
			msgW = 10
		}
		sb.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, ref, relTime, truncate(msg, msgW)))
	}
	return sb.String()
}
