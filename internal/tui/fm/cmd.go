package fm

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "fm [dir]",
		Short: "Open the two-pane file manager",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			m, err := New(dir)
			if err != nil {
				return err
			}
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
