package editor

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [file]",
		Short: "Open the text editor",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var m Model
			var err error
			if len(args) == 1 {
				m, err = New(args[0])
				if err != nil {
					return err
				}
			} else {
				m = NewEmpty()
			}
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
