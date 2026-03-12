package git

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commitStep int

const (
	stepType commitStep = iota
	stepScope
	stepDesc
)

type commitType struct {
	name string
	desc string
}

var commitTypes = []commitType{
	{"feat", "new feature"},
	{"fix", "bug fix"},
	{"docs", "documentation only"},
	{"style", "formatting, whitespace"},
	{"refactor", "code restructuring"},
	{"perf", "performance improvement"},
	{"test", "add or fix tests"},
	{"build", "build system or deps"},
	{"ci", "CI configuration"},
	{"chore", "other maintenance"},
	{"revert", "revert a commit"},
}

type commitModel struct {
	step      commitStep
	typeCur   int
	scopeIn   textinput.Model
	descIn    textinput.Model
	files     []string
	repoDir   string
	err       string
	width     int
	height    int
}

func newCommitModel(files []string, repoDir string, width, height int) commitModel {
	scope := textinput.New()
	scope.Placeholder = "optional scope (e.g. parser)"
	scope.CharLimit = 48

	desc := textinput.New()
	desc.Placeholder = "short description"
	desc.CharLimit = 72

	return commitModel{
		step:    stepType,
		typeCur: 0,
		scopeIn: scope,
		descIn:  desc,
		files:   files,
		repoDir: repoDir,
		width:   width,
		height:  height,
	}
}

func (m commitModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m commitModel) buildMessage() string {
	t := commitTypes[m.typeCur].name
	scope := strings.TrimSpace(m.scopeIn.Value())
	desc := strings.TrimSpace(m.descIn.Value())
	if scope != "" {
		return fmt.Sprintf("%s(%s): %s", t, scope, desc)
	}
	return fmt.Sprintf("%s: %s", t, desc)
}

func (m commitModel) Update(msg tea.Msg) (commitModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.err = ""
		switch m.step {
		case stepType:
			switch msg.String() {
			case "up", "k":
				if m.typeCur > 0 {
					m.typeCur--
				}
			case "down", "j":
				if m.typeCur < len(commitTypes)-1 {
					m.typeCur++
				}
			case "enter":
				m.step = stepScope
				m.scopeIn.Focus()
				return m, textinput.Blink
			case "esc":
				return m, func() tea.Msg { return navigateToFilesMsg{reload: false} }
			case "q", "ctrl+c":
				return m, tea.Quit
			}

		case stepScope:
			switch msg.String() {
			case "enter":
				m.scopeIn.Blur()
				m.step = stepDesc
				m.descIn.Focus()
				return m, textinput.Blink
			case "esc":
				m.scopeIn.Blur()
				m.step = stepType
				return m, nil
			default:
				var cmd tea.Cmd
				m.scopeIn, cmd = m.scopeIn.Update(msg)
				return m, cmd
			}

		case stepDesc:
			switch msg.String() {
			case "enter":
				desc := strings.TrimSpace(m.descIn.Value())
				if desc == "" {
					m.err = "Description cannot be empty"
					return m, nil
				}
				// Run commit
				return m, m.doCommit()
			case "esc":
				m.descIn.Blur()
				m.step = stepScope
				m.scopeIn.Focus()
				return m, textinput.Blink
			default:
				var cmd tea.Cmd
				m.descIn, cmd = m.descIn.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m commitModel) doCommit() tea.Cmd {
	msg := m.buildMessage()
	selected := m.files
	repoDir := m.repoDir
	return func() tea.Msg {
		// Unstage any file that is currently staged but not in the selected list.
		staged, err := StagedFiles(repoDir)
		if err != nil {
			return navigateToResultMsg{err: err}
		}
		selectedSet := make(map[string]bool, len(selected))
		for _, p := range selected {
			selectedSet[p] = true
		}
		var toUnstage []string
		for _, p := range staged {
			if !selectedSet[p] {
				toUnstage = append(toUnstage, p)
			}
		}
		if err := UnstageFiles(repoDir, toUnstage); err != nil {
			return navigateToResultMsg{err: err}
		}
		if err := StageFiles(repoDir, selected); err != nil {
			return navigateToResultMsg{err: err}
		}
		out, err := CommitWithMessage(repoDir, msg)
		return navigateToResultMsg{output: out, err: err}
	}
}

func (m commitModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	stepLabels := []string{"type", "scope", "description"}
	right := fmt.Sprintf("step %d/3 — %s", m.step+1, stepLabels[m.step])
	header := renderHeader(w, "babi commit", right)
	footer := m.viewFooter(w)
	contentH := h - 2

	content := m.viewContent(w, contentH)

	return header + "\n" + padToHeight(content, contentH) + "\n" + footer
}

func (m commitModel) viewFooter(w int) string {
	switch m.step {
	case stepType:
		return renderFooter(w, "↑/↓", "navigate", "enter", "select", "esc", "back")
	case stepScope:
		return renderFooter(w, "enter", "next", "esc", "back")
	case stepDesc:
		if m.err != "" {
			return styleFooterBar.Width(w-4).Render(styleError.Render("✗  " + m.err))
		}
		return renderFooter(w, "enter", "commit", "esc", "back")
	}
	return ""
}

func (m commitModel) viewContent(w, h int) string {
	switch m.step {
	case stepType:
		return m.viewTypeList(w, h)
	case stepScope, stepDesc:
		return m.viewForm(w, h)
	}
	return ""
}

func (m commitModel) viewTypeList(w, h int) string {
	var sb strings.Builder

	sb.WriteString(styleLabel.Render("Commit type:") + "\n\n")

	for i, ct := range commitTypes {
		cursor := "  "
		if i == m.typeCur {
			cursor = styleTypeCursor.Render("> ")
		}
		nameStr := fmt.Sprintf("%-10s", ct.name)
		descStr := styleSubtle.Render(ct.desc)
		if i == m.typeCur {
			nameStr = styleTypeSelected.Render(fmt.Sprintf("%-10s", ct.name))
		}
		sb.WriteString(cursor + nameStr + "  " + descStr + "\n")
	}

	// Preview
	preview := styleSubtle.Render(fmt.Sprintf("  %s: <description>", commitTypes[m.typeCur].name))
	sb.WriteString("\n" + styleLabel.Render("Preview:") + "\n")
	sb.WriteString(preview + "\n")

	return lipgloss.NewStyle().
		Width(w).Height(h).
		Align(lipgloss.Left, lipgloss.Center).
		Padding(0, 4).
		Render(sb.String())
}

func (m commitModel) viewForm(w, h int) string {
	var sb strings.Builder

	// Scope field
	scopeLabel := styleLabel.Render("Scope")
	scopeHint := styleSubtle.Render("  (optional)")
	var scopeWidget string
	if m.step == stepScope {
		scopeWidget = styleActiveInput.Width(40).Render(m.scopeIn.View())
	} else {
		val := strings.TrimSpace(m.scopeIn.Value())
		if val == "" {
			val = styleSubtle.Render("(none)")
		} else {
			val = styleSuccess.Render(val)
		}
		scopeWidget = styleInactiveInput.Width(40).Render(val)
	}
	sb.WriteString(scopeLabel + scopeHint + "\n")
	sb.WriteString(scopeWidget + "\n\n")

	// Description field
	descLabel := styleLabel.Render("Description")
	var descWidget string
	if m.step == stepDesc {
		descWidget = styleActiveInput.Width(60).Render(m.descIn.View())
	} else {
		descWidget = styleInactiveInput.Width(60).Render(m.descIn.View())
	}
	sb.WriteString(descLabel + "\n")
	sb.WriteString(descWidget + "\n\n")

	// Preview
	desc := strings.TrimSpace(m.descIn.Value())
	if desc == "" {
		desc = "<description>"
	}
	preview := m.buildPreview(desc)
	sb.WriteString(styleLabel.Render("Preview:") + "\n")
	sb.WriteString(stylePreviewBox.Render(preview) + "\n")

	// File count
	sb.WriteString("\n" + styleSubtle.Render(fmt.Sprintf("%d file(s) to stage & commit", len(m.files))))

	return lipgloss.NewStyle().
		Width(w).Height(h).
		Align(lipgloss.Left, lipgloss.Center).
		Padding(0, 4).
		Render(sb.String())
}

func (m commitModel) buildPreview(desc string) string {
	t := commitTypes[m.typeCur].name
	scope := strings.TrimSpace(m.scopeIn.Value())
	if scope != "" {
		return styleSelected.Render(fmt.Sprintf("%s(%s): %s", t, scope, desc))
	}
	return styleSelected.Render(fmt.Sprintf("%s: %s", t, desc))
}
