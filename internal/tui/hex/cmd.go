package hex

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "hex <file>",
		Short: "Open the hex editor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := New(args[0])
			if err != nil {
				return err
			}
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
