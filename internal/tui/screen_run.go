package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	syncer "github.com/jadogg/babi/internal/sync"
	"github.com/jadogg/babi/internal/config"
)

type runModel struct {
	spinner  spinner.Model
	viewport viewport.Model
	lines    []string
	results  []syncer.Result
	done     bool
	width    int
	height   int

	// channels kept alive for streaming progress
	progressCh <-chan syncer.ProgressMsg
	resultsCh  <-chan []syncer.Result
}

func newRunModel(width, height int) runModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	vp := viewport.New(width-6, height-4)

	return runModel{
		spinner:  sp,
		viewport: vp,
		width:    width,
		height:   height,
	}
}

// runSyncStreamCmd starts the sync goroutine and returns the first listen command.
// It also returns the channels so they can be stored in the model.
func runSyncStreamCmd(cfg *config.Config) (tea.Cmd, <-chan syncer.ProgressMsg, <-chan []syncer.Result) {
	progress := make(chan syncer.ProgressMsg, 256)
	results := make(chan []syncer.Result, 1)

	go func() {
		r, _ := syncer.RunAll(cfg, progress)
		results <- r
		close(progress)
	}()

	return listenProgress(progress, results), progress, results
}

func listenProgress(ch <-chan syncer.ProgressMsg, done <-chan []syncer.Result) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-ch:
			if ok {
				return syncProgressMsg(msg)
			}
			r := <-done
			return syncCompleteMsg{results: r}
		case r := <-done:
			return syncCompleteMsg{results: r}
		}
	}
}

func (m runModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m runModel) Update(msg tea.Msg) (runModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 6
		m.viewport.Height = msg.Height - 4

	case spinner.TickMsg:
		if !m.done {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case syncProgressMsg:
		p := syncer.ProgressMsg(msg)
		if p.Err != nil {
			m.lines = append(m.lines, styleError.Render(fmt.Sprintf("  ✗  [%s] %s: %v", p.EntryName, p.FilePath, p.Err)))
		} else if p.Done {
			m.lines = append(m.lines, styleSuccess.Render(fmt.Sprintf("  ✓  [%s] complete", p.EntryName)))
		} else {
			m.lines = append(m.lines, styleSubtle.Render(fmt.Sprintf("  →  [%s] %s", p.EntryName, p.FilePath)))
		}
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()
		// Schedule next listen
		cmds = append(cmds, listenProgress(m.progressCh, m.resultsCh))

	case syncCompleteMsg:
		m.done = true
		m.results = msg.results

		m.lines = append(m.lines, "")
		m.lines = append(m.lines, styleLabel.Render("  Summary"))
		m.lines = append(m.lines, styleSubtle.Render("  "+strings.Repeat("─", 40)))
		for _, r := range msg.results {
			line := fmt.Sprintf("  %-20s  %d copied", r.Entry.Name, r.Copied)
			if len(r.Errors) > 0 {
				line += "  " + styleError.Render(fmt.Sprintf("%d error(s)", len(r.Errors)))
			} else {
				line += "  " + styleSuccess.Render("ok")
			}
			m.lines = append(m.lines, line)
		}
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			if m.done {
				return m, func() tea.Msg { return navigateToListMsg{} }
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m runModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	// Header
	var rightInfo string
	if m.done {
		total, errs := 0, 0
		for _, r := range m.results {
			total += r.Copied
			errs += len(r.Errors)
		}
		if errs > 0 {
			rightInfo = styleError.Render(fmt.Sprintf("%d copied · %d errors", total, errs))
		} else {
			rightInfo = fmt.Sprintf("%d files copied", total)
		}
	} else {
		rightInfo = m.spinner.View() + " running"
	}

	headerTitle := "Running Syncs"
	if m.done {
		headerTitle = "Sync Complete"
	}
	header := renderHeader(w, headerTitle, rightInfo)

	var footer string
	if m.done {
		footer = renderFooter(w, "q / esc", "back to list", "↑/↓", "scroll")
	} else {
		footer = renderFooter(w, "↑/↓", "scroll")
	}

	// Padding(1,3): +2 vertical, +6 horizontal → viewport must be (h-4) × (w-6)
	// Width(w-6) on the wrapper so total width = (w-6) + 3 + 3 = w
	m.viewport.Width = w - 6
	m.viewport.Height = h - 4
	content := padToHeight(
		lipgloss.NewStyle().Width(w-6).Padding(1, 3).Render(m.viewport.View()),
		h-2,
	)

	return header + "\n" + content + "\n" + footer
}
