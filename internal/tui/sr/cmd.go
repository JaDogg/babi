package sr

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for smart-rename.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "sr [dir]",
		Short: "Smart rename / delete files in a directory",
		Long: `Smart rename (sr) lets you batch-rename or delete files and
directories in a directory using regular expressions.

  • Type a find pattern (regex) to see live matches
  • Press ctrl+r to rename matches with a replacement pattern
  • Press ctrl+d to delete all matches
  • Use $1, $2… in the replacement for regex capture groups
  • Use {#} or {#:3} for an auto-incrementing zero-padded counter
  • Confirm all changes before they are applied`,
		Args: cobra.MaximumNArgs(1),
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
