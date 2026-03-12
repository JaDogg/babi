package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jadogg/babi/internal/config"
)

type listModel struct {
	entries []config.SyncEntry
	cursor  int
	confirm bool
	width   int
	height  int
}

var listKeys = struct {
	up, down, add, edit, delete, run, confirm, cancel, quit key.Binding
}{
	up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	add:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
	edit:    key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/↵", "edit")),
	delete:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	run:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run all")),
	confirm: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
	cancel:  key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "no")),
	quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

func newListModel(entries []config.SyncEntry, width, height int) listModel {
	return listModel{entries: entries, width: width, height: height}
}

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirm {
			switch {
			case key.Matches(msg, listKeys.confirm):
				idx := m.cursor
				m.confirm = false
				return m, func() tea.Msg { return entryDeletedMsg{index: idx} }
			case key.Matches(msg, listKeys.cancel):
				m.confirm = false
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, listKeys.quit):
			return m, tea.Quit
		case key.Matches(msg, listKeys.up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, listKeys.down):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case key.Matches(msg, listKeys.add):
			return m, func() tea.Msg { return navigateToAddMsg{editIndex: -1} }
		case key.Matches(msg, listKeys.edit):
			if len(m.entries) > 0 {
				return m, func() tea.Msg { return navigateToAddMsg{editIndex: m.cursor} }
			}
		case key.Matches(msg, listKeys.delete):
			if len(m.entries) > 0 {
				m.confirm = true
			}
		case key.Matches(msg, listKeys.run):
			return m, func() tea.Msg { return navigateToRunMsg{} }
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m listModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	header := renderHeader(w, "babi sync", fmt.Sprintf("%d entries", len(m.entries)))

	var footer string
	if m.confirm {
		name := m.entries[m.cursor].Name
		// styleFooterBar has Padding(0,2), so Width(w-4) → total = w
		footer = styleFooterBar.Width(w-4).Render(
			styleError.Render(fmt.Sprintf("Delete %q?   ", name)) +
				styleFooterKey.Render("y") + "  yes   " +
				styleFooterKey.Render("n / esc") + "  no",
		)
	} else {
		footer = renderFooter(w,
			"↑/↓", "navigate",
			"a", "add",
			"e", "edit",
			"d", "delete",
			"r", "run all",
			"q", "quit",
		)
	}

	content := padToHeight(m.renderContent(w, h-2), h-2)

	return header + "\n" + content + "\n" + footer
}

func (m listModel) renderContent(w, h int) string {
	if len(m.entries) == 0 {
		return lipgloss.NewStyle().
			Width(w).Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(colorMuted).
			Render("No syncs configured.\n\nPress " + styleFooterKey.Render("a") + " to add one.")
	}

	const leftPad = "  " // 2-space left margin; cursor (`>`) sits in col 0, space in col 1
	nameW := 20
	typeW := 6  // "file" or "folder"
	srcW  := 24

	// prefix is always 2 chars: "> " (selected) or "  " (unselected)
	// line content starts right after prefix, columns aligned identically
	header := styleColHeader.Render(
		"  " + fmt.Sprintf("%-*s  %-*s  %-*s  %s", nameW, "NAME", typeW, "TYPE", srcW, "SOURCE", "TARGETS"),
	)
	divider := styleSubtle.Render("  " + strings.Repeat("─", w-3))

	var rows strings.Builder
	for i, e := range m.entries {
		srcType := "file"
		if pathIsDir(e.Source) {
			srcType = "folder"
		}

		name := truncate(e.Name, nameW)
		src  := truncate(filepath.Base(e.Source), srcW)

		var tgtNames []string
		for _, t := range e.Targets {
			tgtNames = append(tgtNames, filepath.Base(t))
		}
		targets := strings.Join(tgtNames, ", ")

		line := fmt.Sprintf("%-*s  %-*s  %-*s  %s", nameW, name, typeW, srcType, srcW, src, targets)

		if i == m.cursor {
			rows.WriteString(styleSelected.Render(">") + " " + line + "\n")
		} else {
			rows.WriteString("  " + line + "\n")
		}
	}

	return header + "\n" + divider + "\n" + rows.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max+1:]
}

func pathIsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
