package tui

import (
	"github.com/jadogg/babi/internal/config"
	syncer "github.com/jadogg/babi/internal/sync"
)

// Navigation messages
type navigateToListMsg struct{}
type navigateToAddMsg struct{ editIndex int } // -1 = new entry
type navigateToRunMsg struct{}

// Config mutation messages
type entryConfirmedMsg struct {
	index int // -1 = new
	entry config.SyncEntry
}
type entryDeletedMsg struct{ index int }

// Sync messages
type syncCompleteMsg struct{ results []syncer.Result }
type syncProgressMsg syncer.ProgressMsg

// Config loaded
type configLoadedMsg struct{ cfg *config.Config }
