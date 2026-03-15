package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/jadogg/babi/internal/config"
	"github.com/spf13/cobra"
)

// Command returns the "babi sync" cobra command tree.
// configPath must be the same *string registered as a persistent flag on root
// so that its value is populated before any RunE executes.
// newAppModel is the tui.NewAppModel factory — passed in to avoid an import cycle
// (internal/tui imports internal/sync, so internal/sync cannot import internal/tui).
func Command(configPath *string, newAppModel func(string) tea.Model) *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Open the file-sync TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := tea.NewProgram(newAppModel(*configPath), tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	syncRunCmd := &cobra.Command{
		Use:   "run",
		Short: "Sync all enabled entries (no TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.Entries) == 0 {
				fmt.Printf("%s No sync entries configured. Run %s to add some.\n",
					cc.Dim("[babi]"), cc.Cyan("'babi sync'"))
				return nil
			}

			progress := make(chan ProgressMsg, 256)
			var results []Result
			done := make(chan error, 1)

			go func() {
				var runErr error
				results, runErr = RunAll(cfg, progress)
				close(progress)
				done <- runErr
			}()

			for p := range progress {
				if p.Done {
					continue
				}
				if p.Err != nil {
					fmt.Fprintf(os.Stderr, "%s %s [%s] %s: %v\n",
						cc.Dim("[babi]"), cc.BoldRed("ERROR"),
						cc.Cyan(p.EntryName), p.FilePath, p.Err)
				} else {
					fmt.Printf("%s [%s] %s\n",
						cc.Dim("[babi]"), cc.Cyan(p.EntryName), p.FilePath)
				}
			}
			if err := <-done; err != nil {
				return err
			}

			fmt.Println()
			for _, r := range results {
				errStr := ""
				if len(r.Errors) > 0 {
					errStr = cc.BoldRed(fmt.Sprintf(", %d error(s)", len(r.Errors)))
				}
				fmt.Printf("%s %-20s  %s%s\n",
					cc.Dim("[babi]"),
					cc.BoldCyan(r.Entry.Name),
					cc.BoldGreen(fmt.Sprintf("%d file(s) copied", r.Copied)),
					errStr)
			}
			fmt.Printf("%s Done. %s\n",
				cc.Dim("[babi]"),
				cc.Bold(fmt.Sprintf("%d entries synced.", len(results))))
			return nil
		},
	}

	syncAddCmd := &cobra.Command{
		Use:   "add <name> <source> <target> [target2 ...]",
		Short: "Add a new sync entry",
		Args:  cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			source := expandHome(args[1])
			targets := make([]string, len(args)-2)
			for i, t := range args[2:] {
				targets[i] = expandHome(t)
			}
			if _, err := os.Stat(source); err != nil {
				return fmt.Errorf("source %q: %w", source, err)
			}
			cfg, err := config.Load(*configPath)
			if err != nil {
				cfg = &config.Config{Version: 1}
			}
			for _, e := range cfg.Entries {
				if e.Name == name {
					return fmt.Errorf("entry %q already exists; use 'babi sync' to edit it", name)
				}
			}
			cfg.Entries = append(cfg.Entries, config.SyncEntry{
				Name:    name,
				Source:  source,
				Targets: targets,
				Enabled: true,
			})
			if err := config.Save(*configPath, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("%s %s %s: %s %s %s\n",
				cc.Dim("[babi]"), cc.BoldGreen("Added"),
				cc.BoldCyan(fmt.Sprintf("%q", name)),
				source, cc.Dim("->"), strings.Join(targets, ", "))
			return nil
		},
	}

	syncListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all sync entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.Entries) == 0 {
				fmt.Printf("%s No sync entries configured.\n", cc.Dim("[babi]"))
				return nil
			}
			for i, e := range cfg.Entries {
				var statusStr string
				if e.Enabled {
					statusStr = cc.BoldGreen("enabled")
				} else {
					statusStr = cc.BoldRed("disabled")
				}
				fmt.Printf("%s %s  [%s]\n",
					cc.Dim(fmt.Sprintf("%d.", i+1)),
					cc.BoldCyan(fmt.Sprintf("%-20s", e.Name)),
					statusStr)
				fmt.Printf("   %s  %s\n", cc.Dim("source:"), e.Source)
				for _, t := range e.Targets {
					fmt.Printf("   %s  %s\n", cc.Dim("target:"), t)
				}
			}
			return nil
		},
	}

	syncRemoveCmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a sync entry by name",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			idx := -1
			for i, e := range cfg.Entries {
				if e.Name == name {
					idx = i
					break
				}
			}
			if idx < 0 {
				return fmt.Errorf("no entry named %q", name)
			}
			cfg.Entries = append(cfg.Entries[:idx], cfg.Entries[idx+1:]...)
			if err := config.Save(*configPath, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("%s %s %s\n",
				cc.Dim("[babi]"), cc.BoldYellow("Removed"), cc.Cyan(fmt.Sprintf("%q", name)))
			return nil
		},
	}

	syncCmd.AddCommand(syncRunCmd, syncAddCmd, syncListCmd, syncRemoveCmd)
	return syncCmd
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") && path != "~" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
