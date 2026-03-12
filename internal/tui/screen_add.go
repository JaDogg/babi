package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jadogg/babi/internal/config"
)

type addStep int

const (
	stepName addStep = iota
	stepSourceMode
	stepSource
	stepTarget
	stepAnotherTarget
)

type addModel struct {
	editIndex      int
	step           addStep
	sourceIsFile   bool
	nameInput      textinput.Model
	fp             filepicker.Model
	entry          config.SyncEntry
	targetCursor   int // selected target in stepAnotherTarget
	err            string
	width          int
	height         int
}

func newAddModel(editIndex int, existing *config.SyncEntry, width, height int) addModel {
	ni := textinput.New()
	ni.Placeholder = "e.g. dotfiles"
	ni.CharLimit = 64
	ni.Focus()

	fp := filepicker.New()
	fp.ShowHidden = false
	if home, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = home
	}
	fp.Height = fpHeight(height)

	m := addModel{
		editIndex: editIndex,
		step:      stepName,
		nameInput: ni,
		fp:        fp,
		width:     width,
		height:    height,
	}

	if existing != nil {
		m.nameInput.SetValue(existing.Name)
		m.entry = *existing
	} else {
		m.entry = config.SyncEntry{Enabled: true}
	}
	return m
}

func fpHeight(totalH int) int {
	h := totalH - 2 // header (1) + footer (1)
	if h < 5 {
		h = 5
	}
	return h
}

func (m *addModel) setFPMode(fileAllowed, dirAllowed bool) {
	m.fp.FileAllowed = fileAllowed
	m.fp.DirAllowed = dirAllowed
}

func (m addModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.fp.Init())
}

func (m addModel) Update(msg tea.Msg) (addModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fp.Height = fpHeight(msg.Height)

	case tea.KeyMsg:
		m.err = ""
		switch m.step {
		case stepName:
			switch msg.String() {
			case "esc":
				return m, func() tea.Msg { return navigateToListMsg{} }
			case "enter":
				name := strings.TrimSpace(m.nameInput.Value())
				if name == "" {
					m.err = "Name cannot be empty"
					return m, nil
				}
				m.entry.Name = name
				m.step = stepSourceMode
				return m, nil
			default:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			}

		case stepSourceMode:
			switch msg.String() {
			case "f", "F":
				m.sourceIsFile = true
				m.setFPMode(true, false)
				m.step = stepSource
				var cmd tea.Cmd
				m.fp, cmd = m.fp.Update(msg)
				return m, cmd
			case "d", "D":
				m.sourceIsFile = false
				m.setFPMode(false, true)
				m.step = stepSource
				var cmd tea.Cmd
				m.fp, cmd = m.fp.Update(msg)
				return m, cmd
			case "esc":
				m.step = stepName
				m.nameInput.Focus()
				return m, textinput.Blink
			}

		case stepSource:
			if msg.String() == "esc" {
				m.step = stepSourceMode
				return m, nil
			}

		case stepTarget:
			if msg.String() == "esc" {
				m.step = stepSource
				return m, nil
			}

		case stepAnotherTarget:
			switch msg.String() {
			case "up", "k":
				if m.targetCursor > 0 {
					m.targetCursor--
				}
			case "down", "j":
				if m.targetCursor < len(m.entry.Targets)-1 {
					m.targetCursor++
				}
			case "d", "x", "backspace":
				if len(m.entry.Targets) > 0 {
					m.entry.Targets = append(
						m.entry.Targets[:m.targetCursor],
						m.entry.Targets[m.targetCursor+1:]...,
					)
					if m.targetCursor >= len(m.entry.Targets) && m.targetCursor > 0 {
						m.targetCursor--
					}
					// If all targets removed, go back to picker
					if len(m.entry.Targets) == 0 {
						m.step = stepTarget
					}
				}
			case "y", "Y":
				m.step = stepTarget
				return m, nil
			case "n", "N", "enter":
				if len(m.entry.Targets) > 0 {
					return m, func() tea.Msg {
						return entryConfirmedMsg{index: m.editIndex, entry: m.entry}
					}
				}
			case "esc":
				m.step = stepTarget
				return m, nil
			}
		}
	}

	// Always forward messages to the filepicker so the initial readDirMsg
	// is processed even while the user is still on earlier steps.
	{
		var fpCmd tea.Cmd
		m.fp, fpCmd = m.fp.Update(msg)
		cmds = append(cmds, fpCmd)
	}

	if m.step == stepSource || m.step == stepTarget {
		if didSelect, path := m.fp.DidSelectFile(msg); didSelect {
			if m.step == stepSource {
				m.entry.Source = path
				m.setFPMode(false, true) // targets are always directories
				m.step = stepTarget
			} else {
				m.entry.Targets = append(m.entry.Targets, path)
				m.targetCursor = len(m.entry.Targets) - 1
				m.step = stepAnotherTarget
			}
		}
		if didSelect, path := m.fp.DidSelectDisabledFile(msg); didSelect {
			m.err = fmt.Sprintf("Cannot select %q", path)
		}
	}


	return m, tea.Batch(cmds...)
}

func (m addModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	title := "Add Sync"
	if m.editIndex >= 0 {
		title = "Edit Sync"
	}

	stepLabel, stepTotal := m.stepInfo()
	rightInfo := fmt.Sprintf("step %s of %d", stepLabel, stepTotal)
	header := renderHeader(w, title, rightInfo)
	footer := m.renderFooter(w)
	contentH := h - 2

	var content string
	switch m.step {
	case stepName:
		content = m.renderCenteredForm(w, contentH)
	case stepSourceMode:
		content = m.renderCenteredChoice(w, contentH)
	case stepSource, stepTarget:
		content = m.renderSplitPicker(w, contentH)
	case stepAnotherTarget:
		content = m.renderAnotherTarget(w, contentH)
	}

	return header + "\n" + padToHeight(content, contentH) + "\n" + footer
}

func (m addModel) stepInfo() (string, int) {
	switch m.step {
	case stepName:
		return "1", 4
	case stepSourceMode:
		return "2", 4
	case stepSource:
		return "3", 4
	case stepTarget, stepAnotherTarget:
		return "4", 4
	}
	return "?", 4
}

func (m addModel) renderFooter(w int) string {
	switch m.step {
	case stepName:
		return renderFooter(w, "enter", "confirm", "esc", "cancel")
	case stepSourceMode:
		return renderFooter(w, "f", "file", "d", "folder", "esc", "back")
	case stepSource, stepTarget:
		return renderFooter(w, "enter", "select", "↑/↓", "navigate", "←/→", "open/back", "esc", "back")
	case stepAnotherTarget:
		return renderFooter(w, "y", "add target", "d / x", "remove selected", "↑/↓", "navigate", "n / enter", "done")
	}
	return ""
}

// renderCenteredForm shows the name input centered in the content area.
func (m addModel) renderCenteredForm(w, h int) string {
	label := styleLabel.Render("Name your sync:")
	input := styleActiveInput.Width(36).Render(m.nameInput.View())

	var body strings.Builder
	body.WriteString(label + "\n\n")
	body.WriteString(input + "\n")
	if m.err != "" {
		body.WriteString("\n" + styleError.Render("✗  "+m.err))
	}

	return lipgloss.NewStyle().
		Width(w).Height(h).
		Align(lipgloss.Center, lipgloss.Center).
		Render(body.String())
}

// renderCenteredChoice shows the file/folder mode selector.
func (m addModel) renderCenteredChoice(w, h int) string {
	label := styleLabel.Render(fmt.Sprintf("Source type for %q:", m.entry.Name))

	fOpt := styleFooterKey.Render("f") + "  File        — sync a single file"
	dOpt := styleFooterKey.Render("d") + "  Folder      — recursively sync all contents"

	body := label + "\n\n" + fOpt + "\n" + dOpt

	return lipgloss.NewStyle().
		Width(w).Height(h).
		Align(lipgloss.Center, lipgloss.Center).
		Render(body)
}

// renderSplitPicker shows sidebar + filepicker side by side.
func (m addModel) renderSplitPicker(w, h int) string {
	sidebar := m.renderSidebar(h)
	picker := m.fp.View()
	if m.err != "" {
		picker += "\n" + styleError.Render("✗  "+m.err)
	}
	return renderSplit(w, h, sidebar, picker)
}

// renderSidebar shows the step list with done/active/pending states.
func (m addModel) renderSidebar(h int) string {
	var b strings.Builder

	type sideStep struct {
		label  string
		value  string
		status addStep
	}

	srcKind := "folder"
	if m.sourceIsFile {
		srcKind = "file"
	}

	steps := []sideStep{
		{label: "Name", value: m.entry.Name, status: stepName},
		{label: "Source type", value: srcKind, status: stepSourceMode},
		{label: "Source", value: truncate(m.entry.Source, 18), status: stepSource},
		{label: "Target(s)", value: fmt.Sprintf("%d set", len(m.entry.Targets)), status: stepTarget},
	}

	for _, s := range steps {
		var labelStr, valueStr string
		switch {
		case s.status == m.step:
			labelStr = styleStepActive.Render("▶  " + s.label)
			valueStr = styleSubtle.Render("   " + s.value)
		case s.status < m.step:
			labelStr = styleStepDone.Render("✓  " + s.label)
			valueStr = styleSubtle.Render("   " + s.value)
		default:
			labelStr = styleStepPending.Render("○  " + s.label)
			valueStr = ""
		}
		b.WriteString(labelStr + "\n")
		if valueStr != "" {
			b.WriteString(valueStr + "\n")
		}
		b.WriteString("\n")
	}

	// Show accumulated targets if in target step
	if m.step == stepTarget && len(m.entry.Targets) > 0 {
		b.WriteString(styleStepDone.Render("  Targets added:") + "\n")
		for _, t := range m.entry.Targets {
			b.WriteString(styleSubtle.Render("  • "+truncate(t, 18)) + "\n")
		}
	}

	return b.String()
}

// renderAnotherTarget shows the target list with remove/add controls.
func (m addModel) renderAnotherTarget(w, h int) string {
	var b strings.Builder
	b.WriteString(styleLabel.Render("Source:") + "  " + styleSubtle.Render(m.entry.Source) + "\n\n")
	b.WriteString(styleLabel.Render("Targets:") + "\n")

	for i, t := range m.entry.Targets {
		if i == m.targetCursor {
			cursor := styleRowCursor.Render("▶")
			line := styleRowSelected.Width(w - 10).Render("  " + t)
			b.WriteString("  " + cursor + line + "\n")
		} else {
			b.WriteString("  " + styleSuccess.Render("✓") + "  " + styleSubtle.Render(t) + "\n")
		}
	}

	b.WriteString("\n" + styleSubtle.Render("y  add another   d  remove selected   n  done"))

	return lipgloss.NewStyle().
		Width(w).Height(h).
		Align(lipgloss.Center, lipgloss.Center).
		Render(b.String())
}
