package git

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func CommitCommand() *cobra.Command {
	var commitRunType string
	var commitRunScope string
	var commitRunAll bool

	commitRunCmd := &cobra.Command{
		Use:   "run <description> [files...]",
		Short: "Headless commitizen commit (no TUI)",
		Long:  "Stage files and commit with a commitizen message. If no files given, stages all changed files.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := args[0]
			files := args[1:]

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoDir, err := FindRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}

			fmt.Printf("[babi] repo root: %s\n", repoDir)

			if commitRunAll || len(files) == 0 {
				statuses, err := GetStatus(repoDir)
				if err != nil {
					return fmt.Errorf("git status failed: %w", err)
				}
				fmt.Printf("[babi] git status --porcelain output (%d files):\n", len(statuses))
				for _, s := range statuses {
					fmt.Printf("  X=%q Y=%q path=%q\n", string(s.X), string(s.Y), s.Path)
				}
				files = make([]string, 0, len(statuses))
				for _, s := range statuses {
					files = append(files, s.Path)
				}
			}

			if len(files) == 0 {
				return fmt.Errorf("nothing to commit — working tree clean")
			}

			fmt.Printf("[babi] staging %d file(s): %v\n", len(files), files)
			if err := StageFiles(repoDir, files); err != nil {
				return fmt.Errorf("git add failed: %w", err)
			}
			fmt.Printf("[babi] staged OK\n")

			t := commitRunType
			if t == "" {
				t = "chore"
			}
			var message string
			if commitRunScope != "" {
				message = fmt.Sprintf("%s(%s): %s", t, commitRunScope, desc)
			} else {
				message = fmt.Sprintf("%s: %s", t, desc)
			}
			fmt.Printf("[babi] commit message: %q\n", message)

			out, err := CommitWithMessage(repoDir, message)
			fmt.Printf("[babi] git commit output:\n%s\n", out)
			if err != nil {
				return fmt.Errorf("git commit failed: %w", err)
			}
			fmt.Printf("[babi] commit OK\n")
			return nil
		},
	}
	commitRunCmd.Flags().StringVarP(&commitRunType, "type", "t", "chore", "commit type (feat, fix, docs, ...)")
	commitRunCmd.Flags().StringVarP(&commitRunScope, "scope", "s", "", "commit scope (optional)")
	commitRunCmd.Flags().BoolVarP(&commitRunAll, "all", "a", false, "stage all changed files")

	commitCmd := &cobra.Command{
		Use:   "commit",
		Short: "Open the commitizen commit TUI",
		Long:  "Select files to stage and commit following commitizen conventions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoDir, err := FindRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			p := tea.NewProgram(NewAppModel(repoDir), tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
	commitCmd.AddCommand(commitRunCmd)
	return commitCmd
}

func LogCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "log",
		Short: "Interactive git log viewer",
		Long: `Browse git commit history with an interactive TUI.

  ↑/↓ or k/j   navigate commits
  tab / enter   switch focus to diff pane
  g / G         jump to top / bottom
  q             quit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoDir, err := FindRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			p := tea.NewProgram(NewLogModel(repoDir), tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}

func StashCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stash",
		Short: "Interactive git stash manager",
		Long: `View, apply, pop, drop, and create git stashes with an interactive TUI.

  ↑/↓ or k/j   navigate stashes
  a             apply selected stash (keep it)
  p             pop selected stash (apply and remove)
  d             drop selected stash (confirm with y)
  n             create a new stash
  tab / enter   switch focus to diff pane
  r             refresh list
  q             quit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoDir, err := FindRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			p := tea.NewProgram(NewStashModel(repoDir), tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
