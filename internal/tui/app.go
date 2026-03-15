package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jadogg/babi/internal/config"
)

type screen int

const (
	screenList screen = iota
	screenAdd
	screenRun
)

// AppModel is the root bubbletea model.
type AppModel struct {
	screen screen
	cfg    *config.Config
	width  int
	height int

	list listModel
	add  addModel
	run  runModel
}

// NewAppModel creates the root model.
func NewAppModel() AppModel {
	return AppModel{
		screen: screenList,
	}
}

func (m AppModel) Init() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load(config.Path())
		if err != nil {
			cfg = &config.Config{Version: 1}
		}
		return configLoadedMsg{cfg: cfg}
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
		m.add, cmd = m.add.Update(msg)
		cmds = append(cmds, cmd)
		m.run, cmd = m.run.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case configLoadedMsg:
		m.cfg = msg.cfg
		m.list = newListModel(m.cfg.Entries, m.width, m.height)
		return m, nil

	case navigateToListMsg:
		m.screen = screenList
		m.list = newListModel(m.cfg.Entries, m.width, m.height)
		return m, nil

	case navigateToAddMsg:
		m.screen = screenAdd
		var existing *config.SyncEntry
		if msg.editIndex >= 0 && msg.editIndex < len(m.cfg.Entries) {
			e := m.cfg.Entries[msg.editIndex]
			existing = &e
		}
		m.add = newAddModel(msg.editIndex, existing, m.width, m.height)
		return m, m.add.Init()

	case navigateToRunMsg:
		m.screen = screenRun
		m.run = newRunModel(m.width, m.height)
		listenCmd, progressCh, resultsCh := runSyncStreamCmd(m.cfg)
		m.run.progressCh = progressCh
		m.run.resultsCh = resultsCh
		return m, tea.Batch(m.run.Init(), listenCmd)

	case entryConfirmedMsg:
		if msg.index < 0 {
			m.cfg.Entries = append(m.cfg.Entries, msg.entry)
		} else if msg.index < len(m.cfg.Entries) {
			m.cfg.Entries[msg.index] = msg.entry
		}
		_ = config.Save(config.Path(), m.cfg)
		m.screen = screenList
		m.list = newListModel(m.cfg.Entries, m.width, m.height)
		return m, nil

	case entryDeletedMsg:
		if msg.index >= 0 && msg.index < len(m.cfg.Entries) {
			m.cfg.Entries = append(m.cfg.Entries[:msg.index], m.cfg.Entries[msg.index+1:]...)
		}
		_ = config.Save(config.Path(), m.cfg)
		if m.list.cursor >= len(m.cfg.Entries) && m.list.cursor > 0 {
			m.list.cursor--
		}
		m.list = newListModel(m.cfg.Entries, m.width, m.height)
		return m, nil
	}

	// Delegate to active screen
	var cmd tea.Cmd
	switch m.screen {
	case screenList:
		m.list, cmd = m.list.Update(msg)
	case screenAdd:
		m.add, cmd = m.add.Update(msg)
	case screenRun:
		m.run, cmd = m.run.Update(msg)
	}
	return m, cmd
}

func (m AppModel) View() string {
	switch m.screen {
	case screenAdd:
		return m.add.View()
	case screenRun:
		return m.run.View()
	default:
		return m.list.View()
	}
}
