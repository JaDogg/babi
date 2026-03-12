package git

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type appScreen int

const (
	screenFiles appScreen = iota
	screenCommit
	screenResult
)

// AppModel is the root bubbletea model for babi git.
type AppModel struct {
	screen  appScreen
	repoDir string
	width   int
	height  int

	files  filesModel
	commit commitModel
	result resultModel
}

// NewAppModel creates the root model.
func NewAppModel(repoDir string) AppModel {
	return AppModel{
		screen:  screenFiles,
		repoDir: repoDir,
	}
}

func (m AppModel) Init() tea.Cmd {
	return func() tea.Msg {
		items, err := loadItems(m.repoDir)
		return gitLoadedMsg{items: items, err: err}
	}
}

type gitLoadedMsg struct {
	items []DisplayItem
	err   error
}

func loadItems(repoDir string) ([]DisplayItem, error) {
	files, err := GetStatus(repoDir)
	if err != nil {
		return nil, err
	}
	return BuildItems(files), nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.files, cmd = m.files.Update(msg)
		cmds = append(cmds, cmd)
		m.commit, cmd = m.commit.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case gitLoadedMsg:
		if msg.err != nil {
			m.result = newResultModel("", msg.err, m.width, m.height)
			m.screen = screenResult
			return m, nil
		}
		m.files = newFilesModel(msg.items, m.repoDir, m.width, m.height)
		m.screen = screenFiles
		return m, nil

	case navigateToFilesMsg:
		if msg.reload {
			return m, func() tea.Msg {
				items, err := loadItems(m.repoDir)
				return gitLoadedMsg{items: items, err: err}
			}
		}
		m.screen = screenFiles
		return m, nil

	case navigateToCommitMsg:
		m.commit = newCommitModel(msg.files, m.repoDir, m.width, m.height)
		m.screen = screenCommit
		return m, m.commit.Init()

	case navigateToResultMsg:
		m.result = newResultModel(msg.output, msg.err, m.width, m.height)
		m.screen = screenResult
		return m, nil
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenFiles:
		m.files, cmd = m.files.Update(msg)
	case screenCommit:
		m.commit, cmd = m.commit.Update(msg)
	case screenResult:
		m.result, cmd = m.result.Update(msg)
	}
	return m, cmd
}

func (m AppModel) View() string {
	switch m.screen {
	case screenCommit:
		return m.commit.View()
	case screenResult:
		return m.result.View()
	default:
		return m.files.View()
	}
}

// resultModel shows the git commit output.
type resultModel struct {
	output  string
	success bool
	width   int
	height  int
}

func newResultModel(output string, err error, width, height int) resultModel {
	return resultModel{
		output:  formatResult(output, err),
		success: err == nil,
		width:   width,
		height:  height,
	}
}

func formatResult(output string, err error) string {
	if err != nil {
		return fmt.Sprintf("Error: %v\n\n%s", err, output)
	}
	return output
}

func (m resultModel) Update(msg tea.Msg) (resultModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "enter", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m resultModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	title := "babi committed"
	right := styleSuccess.Render("success")
	if !m.success {
		title = "babi commit — error"
		right = styleError.Render("failed")
	}
	header := renderHeader(w, title, right)
	footer := renderFooter(w, "q / enter", "quit")
	contentH := h - 2

	outputStr := m.output
	if m.success {
		outputStr = styleSuccess.Render(strings.TrimRight(m.output, "\n"))
	} else {
		outputStr = styleError.Render(strings.TrimRight(m.output, "\n"))
	}

	content := lipgloss.NewStyle().
		Width(w).Height(contentH).
		Align(lipgloss.Center, lipgloss.Center).
		Padding(0, 4).
		Render(outputStr)

	return header + "\n" + content + "\n" + footer
}
