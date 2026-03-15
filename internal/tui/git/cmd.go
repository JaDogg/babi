package git

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

// GitCommand returns the "babi git" headless command group.
func GitCommand() *cobra.Command {
	g := &cobra.Command{
		Use:   "git",
		Short: "Headless git operations (status, log, config, revert)",
		Long:  "Headless git subcommands. See also: babi commit (TUI), babi log (TUI), babi stash (TUI).",
	}
	g.AddCommand(gitStatusCmd(), gitLogCmd(), gitConfigCmd(), gitRevertCmd())
	return g
}

func gitStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [dir]",
		Short: "Show working tree status",
		Long: `Show the working tree status of a git repository.

  babi git status
  babi git status ~/Projects/myrepo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			repoDir, err := FindRoot(dir)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			files, err := GetStatus(repoDir)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Println("nothing to commit, working tree clean")
				return nil
			}
			for _, f := range files {
				fmt.Printf("%s  %s\n", f.Label(), f.Path)
			}
			return nil
		},
	}
}

func gitLogCmd() *cobra.Command {
	var n int
	c := &cobra.Command{
		Use:   "log [dir]",
		Short: "Print commit log (headless)",
		Long: `Print recent commits as one line each.

  babi git log
  babi git log -n 10
  babi git log -n 5 ~/Projects/myrepo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			repoDir, err := FindRoot(dir)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			commits, err := GetLog(repoDir, n)
			if err != nil {
				return err
			}
			for _, c := range commits {
				fmt.Printf("%s  %s  %s\n", c.Short, c.RelTime, c.Subject)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&n, "number", "n", 20, "number of commits to show")
	return c
}

func gitConfigCmd() *cobra.Command {
	var local, global bool
	c := &cobra.Command{
		Use:   "config [key] [dir]",
		Short: "Show git config",
		Long: `Show git configuration values.

  babi git config                      # all config (local + global)
  babi git config --local              # local config only
  babi git config --global             # global config only
  babi git config user.email           # specific key
  babi git config user.email ~/myrepo  # specific key in a repo`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			dir := "."
			for _, a := range args {
				// If arg looks like a path (contains / or ~ or .), treat as dir
				if strings.ContainsAny(a, "/~") {
					dir = a
				} else if strings.Contains(a, ".") {
					key = a
				} else {
					dir = a
				}
			}
			repoDir, err := FindRoot(dir)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			var gitArgs []string
			if key != "" {
				gitArgs = []string{"config", "--get", key}
				if local {
					gitArgs = []string{"config", "--local", "--get", key}
				} else if global {
					gitArgs = []string{"config", "--global", "--get", key}
				}
			} else {
				gitArgs = []string{"config", "--list"}
				if local {
					gitArgs = append(gitArgs, "--local")
				} else if global {
					gitArgs = append(gitArgs, "--global")
				}
			}
			c := exec.Command("git", gitArgs...)
			c.Dir = repoDir
			out, err := c.CombinedOutput()
			if len(out) > 0 {
				fmt.Print(string(out))
			}
			return err
		},
	}
	c.Flags().BoolVar(&local, "local", false, "show local config only")
	c.Flags().BoolVar(&global, "global", false, "show global config only")
	return c
}

func gitRevertCmd() *cobra.Command {
	var n int
	c := &cobra.Command{
		Use:   "revert [dir]",
		Short: "Revert the last N commits",
		Long: `Revert the last N commits (HEAD by default).

  babi git revert
  babi git revert -n 3
  babi git revert ~/Projects/myrepo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			repoDir, err := FindRoot(dir)
			if err != nil {
				return fmt.Errorf("not a git repository: %w", err)
			}
			ref := "HEAD"
			if n > 1 {
				ref = "HEAD~" + strconv.Itoa(n-1) + "..HEAD"
			}
			gc := exec.Command("git", "revert", "--no-edit", ref)
			gc.Dir = repoDir
			gc.Stdout = os.Stdout
			gc.Stderr = os.Stderr
			return gc.Run()
		},
	}
	c.Flags().IntVarP(&n, "number", "n", 1, "number of commits to revert")
	return c
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
