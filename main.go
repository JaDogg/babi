package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jadogg/babi/internal/cf"
	"github.com/jadogg/babi/internal/check"
	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/jadogg/babi/internal/config"
	cv "github.com/jadogg/babi/internal/convert"
	"github.com/jadogg/babi/internal/dt"
	"github.com/jadogg/babi/internal/ed"
	"github.com/jadogg/babi/internal/meta"
	"github.com/jadogg/babi/internal/newproject"
	"github.com/jadogg/babi/internal/pack"
	"github.com/jadogg/babi/internal/serve"
	syncer "github.com/jadogg/babi/internal/sync"
	"github.com/jadogg/babi/internal/tag"
	"github.com/jadogg/babi/internal/tree"
	"github.com/jadogg/babi/internal/tui"
	"github.com/jadogg/babi/internal/tui/editor"
	"github.com/jadogg/babi/internal/tui/fm"
	gitui "github.com/jadogg/babi/internal/tui/git"
	tuihex "github.com/jadogg/babi/internal/tui/hex"
	"github.com/jadogg/babi/internal/tui/typer"
)

var version = "dev" // set by -ldflags "-X main.version=vX.Y.Z" at build time

var configPath string

// ─── root ────────────────────────────────────────────────────────────────────

var rootCmd = &cobra.Command{
	Use:     "babi",
	Short:   "babi — file sync & git TUI",
	Long:    "babi: file sync, commitizen commits, text editor, hex editor, and file manager.",
	Version: version,
}

// ─── babi sync ───────────────────────────────────────────────────────────────

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Open the file-sync TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(tui.NewAppModel(configPath), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

var syncRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Sync all enabled entries (no TUI)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if len(cfg.Entries) == 0 {
			fmt.Printf("%s No sync entries configured. Run %s to add some.\n",
				cc.Dim("[babi]"), cc.Cyan("'babi sync'"))
			return nil
		}

		progress := make(chan syncer.ProgressMsg, 256)
		var results []syncer.Result
		done := make(chan error, 1)

		go func() {
			var runErr error
			results, runErr = syncer.RunAll(cfg, progress)
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

var syncAddCmd = &cobra.Command{
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
		cfg, err := config.Load(configPath)
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
		if err := config.Save(configPath, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("%s %s %s: %s %s %s\n",
			cc.Dim("[babi]"), cc.BoldGreen("Added"),
			cc.BoldCyan(fmt.Sprintf("%q", name)),
			source, cc.Dim("->"), strings.Join(targets, ", "))
		return nil
	},
}

var syncListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sync entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
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

var syncRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a sync entry by name",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := config.Load(configPath)
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
		if err := config.Save(configPath, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("%s %s %s\n",
			cc.Dim("[babi]"), cc.BoldYellow("Removed"), cc.Cyan(fmt.Sprintf("%q", name)))
		return nil
	},
}

// ─── babi commit ─────────────────────────────────────────────────────────────

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Open the commitizen commit TUI",
	Long:  "Select files to stage and commit following commitizen conventions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoDir, err := gitui.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf("not a git repository: %w", err)
		}
		p := tea.NewProgram(gitui.NewAppModel(repoDir), tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

var (
	commitRunType  string
	commitRunScope string
	commitRunAll   bool
)

var commitRunCmd = &cobra.Command{
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
		repoDir, err := gitui.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf("not a git repository: %w", err)
		}

		fmt.Printf("[babi] repo root: %s\n", repoDir)

		// Collect files to stage
		if commitRunAll || len(files) == 0 {
			statuses, err := gitui.GetStatus(repoDir)
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
		if err := gitui.StageFiles(repoDir, files); err != nil {
			return fmt.Errorf("git add failed: %w", err)
		}
		fmt.Printf("[babi] staged OK\n")

		// Build commit message
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

		out, err := gitui.CommitWithMessage(repoDir, message)
		fmt.Printf("[babi] git commit output:\n%s\n", out)
		if err != nil {
			return fmt.Errorf("git commit failed: %w", err)
		}
		fmt.Printf("[babi] commit OK\n")
		return nil
	},
}

// ─── babi search / babi replace ──────────────────────────────────────────────

var (
	edType    string
	edGlob    string
	edNoCase  bool
	edHidden  bool
	edFiles   bool
	edBefore  int
	edAfter   int
	edContext int
	edDryRun  bool
	edLiteral bool
)

var searchCmd = &cobra.Command{
	Use:   "search <pattern> [path]",
	Short: "Search for a pattern across files",
	Example: `  babi search "func main"
  babi search "TODO" --type go
  babi search "error" ./internal -C 2
  babi search "import" --glob "*.go" -l`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern := args[0]
		path := "."
		if len(args) == 2 {
			path = args[1]
		}
		ctx := edBefore
		if edContext > 0 {
			ctx = edContext
			edAfter = edContext
		}
		opts := ed.SearchOpts{
			FileType:      edType,
			Glob:          edGlob,
			IgnoreCase:    edNoCase,
			Hidden:        edHidden,
			FilesOnly:     edFiles,
			ContextBefore: ctx,
			ContextAfter:  edAfter,
		}
		return ed.Search(os.Stdout, pattern, path, opts)
	},
}

var replaceCmd = &cobra.Command{
	Use:   "replace <pattern> <replacement> [path]",
	Short: "Find and replace across files",
	Example: `  babi replace "OldName" "NewName" ./internal
  babi replace "bsy" "babi" . --type go --dry-run
  babi replace "foo" "bar" main.go --literal`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern := args[0]
		replacement := args[1]
		path := "."
		if len(args) == 3 {
			path = args[2]
		}
		opts := ed.ReplaceOpts{
			FileType:   edType,
			Glob:       edGlob,
			IgnoreCase: edNoCase,
			Hidden:     edHidden,
			DryRun:     edDryRun,
			Literal:    edLiteral,
		}
		return ed.Replace(os.Stdout, pattern, replacement, path, opts)
	},
}

func initEdFlags() {
	for _, c := range []*cobra.Command{searchCmd, replaceCmd} {
		c.Flags().StringVarP(&edType, "type", "t", "", "file type filter (go, py, js, ts, rs, …)")
		c.Flags().StringVarP(&edGlob, "glob", "g", "", "glob pattern for filenames (e.g. *.go)")
		c.Flags().BoolVarP(&edNoCase, "ignore-case", "i", false, "case-insensitive matching")
		c.Flags().BoolVar(&edHidden, "hidden", false, "include hidden files and directories")
	}
	searchCmd.Flags().BoolVarP(&edFiles, "files", "l", false, "print only filenames with matches")
	searchCmd.Flags().IntVarP(&edBefore, "before", "B", 0, "lines of context before match")
	searchCmd.Flags().IntVarP(&edAfter, "after", "A", 0, "lines of context after match")
	searchCmd.Flags().IntVarP(&edContext, "context", "C", 0, "lines of context before and after")
	replaceCmd.Flags().BoolVarP(&edDryRun, "dry-run", "n", false, "show changes without writing files")
	replaceCmd.Flags().BoolVarP(&edLiteral, "literal", "F", false, "treat pattern as literal string, not regex")
}

// ─── babi edit ───────────────────────────────────────────────────────────────

var editCmd = &cobra.Command{
	Use:   "edit [file]",
	Short: "Open the text editor",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var m editor.Model
		var err error
		if len(args) == 1 {
			m, err = editor.New(args[0])
			if err != nil {
				return err
			}
		} else {
			m = editor.NewEmpty()
		}
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// ─── babi hex ────────────────────────────────────────────────────────────────

var hexCmd = &cobra.Command{
	Use:   "hex <file>",
	Short: "Open the hex editor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := tuihex.New(args[0])
		if err != nil {
			return err
		}
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// ─── babi fm ─────────────────────────────────────────────────────────────────

var fmCmd = &cobra.Command{
	Use:   "fm [dir]",
	Short: "Open the two-pane file manager",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		m, err := fm.New(dir)
		if err != nil {
			return err
		}
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// ─── babi dt ─────────────────────────────────────────────────────────────────

const dtFmt = "2006-01-02 15:04:05 MST"
const dtFmtUTC = "2006-01-02 15:04:05"

var dtCmd = &cobra.Command{
	Use:   "dt",
	Short: "Date and time utilities",
	Long:  "Date/time calculations, age, timezone conversion, and NTP sync.",
	RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		fmt.Printf("%s %s\n", cc.Dim("Now: "), cc.BrightWhite(now.Format(dtFmt)))
		fmt.Printf("%s %s\n", cc.Dim("UTC: "), cc.Dim(now.UTC().Format(dtFmtUTC)))
		fmt.Printf("%s %s\n", cc.Dim("Unix:"), cc.Yellow(strconv.FormatInt(now.Unix(), 10)))
		fmt.Println()

		offsets := []string{"+1h", "+1d", "+1w", "+1mo", "+1y", "-1h", "-1d", "-1w", "-1mo", "-1y"}
		fmt.Println(cc.Bold("Offsets from now:"))
		for _, s := range offsets {
			d, _ := dt.ParseCalDuration(s)
			t := d.AddTo(now)
			timeStr := t.Format(dtFmt)
			if d.Neg {
				timeStr = cc.Dim(timeStr)
			}
			fmt.Printf("  %s  %s\n", cc.Cyan(fmt.Sprintf("%-6s", s)), timeStr)
		}
		return nil
	},
}

var dtInCmd = &cobra.Command{
	Use:                "in <duration>",
	Short:              "Time after/before a duration from now (e.g. 1h, -2d, 1mo, 1y2mo3d)",
	Example:            "  babi dt in 1h\n  babi dt in -2d\n  babi dt in 1y2mo3d",
	DisableFlagParsing: true, // prevents cobra treating -2d as a flag
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return cmd.Help()
		}
		if len(args) != 1 {
			return fmt.Errorf("expected exactly 1 argument (e.g. 1h, -2d, 1mo)")
		}
		d, err := dt.ParseCalDuration(args[0])
		if err != nil {
			return err
		}
		now := time.Now()
		result := d.AddTo(now)

		var label, resultStr string
		if d.Neg {
			label = cc.BoldYellow("Before " + d.String()[1:] + ":")
			resultStr = cc.BoldYellow(result.Format(dtFmt))
		} else {
			label = cc.BoldGreen("After " + d.String() + ":")
			resultStr = cc.BoldGreen(result.Format(dtFmt))
		}

		fmt.Printf("%s %s\n", cc.Dim("Now:   "), cc.BrightWhite(now.Format(dtFmt)))
		fmt.Printf("%-30s %s\n", label, resultStr)
		fmt.Printf("%s %s\n", cc.Dim("UTC:   "), cc.Dim(result.UTC().Format(dtFmtUTC)))
		fmt.Printf("%s %s\n", cc.Dim("Unix:  "), cc.Yellow(strconv.FormatInt(result.Unix(), 10)))
		return nil
	},
}

var dtAgeCmd = &cobra.Command{
	Use:     "age <date>",
	Short:   "Calculate age from a birth date",
	Example: "  babi dt age 1990-01-15\n  babi dt age \"Jan 15, 1990\"",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		birth, err := dt.ParseDate(args[0])
		if err != nil {
			return err
		}
		if birth.After(time.Now()) {
			return fmt.Errorf("date %s is in the future", args[0])
		}
		age := dt.CalcAge(birth)
		fmt.Printf("%s %s\n",
			cc.Dim("Born:       "),
			cc.BrightWhite(age.Birth.Format("2006-01-02")))
		fmt.Printf("%s %s\n",
			cc.Dim("Age:        "),
			cc.BoldGreen(fmt.Sprintf("%d years, %d months, %d days", age.Years, age.Months, age.Days)))
		fmt.Printf("%s %s\n",
			cc.Dim("Total days: "),
			cc.Cyan(formatInt(age.TotalDays)))
		fmt.Printf("%s %s\n",
			cc.Dim("Total hours:"),
			cc.Cyan(formatInt(age.TotalHours)))
		return nil
	},
}

var dtTZCmd = &cobra.Command{
	Use:     "tz <timezone> [time]",
	Short:   "Show current time (or a given time) in a timezone",
	Example: "  babi dt tz America/New_York\n  babi dt tz UTC\n  babi dt tz Europe/London \"2024-06-01 12:00:00\"",
	Args:    cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		loc, err := time.LoadLocation(args[0])
		if err != nil {
			return fmt.Errorf("unknown timezone %q: %w", args[0], err)
		}
		var base time.Time
		if len(args) == 2 {
			base, err = time.ParseInLocation("2006-01-02 15:04:05", args[1], time.Local)
			if err != nil {
				base, err = time.Parse(dtFmt, args[1])
				if err != nil {
					return fmt.Errorf("unrecognised time %q (use \"2006-01-02 15:04:05\")", args[1])
				}
			}
		} else {
			base = time.Now()
		}
		fmt.Printf("%s %s\n",
			cc.Dim(fmt.Sprintf("%-22s", fmt.Sprintf("Local (%s):", base.Location()))),
			cc.BrightWhite(base.Format(dtFmt)))
		fmt.Printf("%s %s\n",
			cc.BoldCyan(fmt.Sprintf("%-22s", args[0]+":")),
			cc.BrightWhite(base.In(loc).Format(dtFmt)))
		fmt.Printf("%s %s\n",
			cc.Dim(fmt.Sprintf("%-22s", "UTC:")),
			cc.Dim(base.UTC().Format(dtFmtUTC)))
		return nil
	},
}

var dtNTPCmd = &cobra.Command{
	Use:   "ntp",
	Short: "Query NTP servers and report clock offset",
	Long:  "Queries pool.ntp.org, time.google.com, time.cloudflare.com, and time.apple.com in parallel.\nUse --sync to apply the correction (requires root/sudo).",
	RunE: func(cmd *cobra.Command, args []string) error {
		doSync, _ := cmd.Flags().GetBool("sync")
		fmt.Println(cc.Dim("Querying NTP servers..."))
		results := dt.QueryAll()

		fmt.Println()
		for _, r := range results {
			if r.Err != nil {
				fmt.Printf("  %-22s  %s %v\n",
					cc.Cyan(r.Server), cc.BoldRed("ERROR:"), r.Err)
				continue
			}
			off := r.Offset
			sign := "+"
			if off < 0 {
				sign = "-"
				off = -off
			}
			offsetStr := sign + off.Round(time.Microsecond).String()
			offsetColored := ntpOffsetColor(r.Offset, offsetStr)
			fmt.Printf("  %-22s  %s  %s %s  %s %s\n",
				cc.Cyan(r.Server),
				r.Time.Format(dtFmtUTC+" UTC"),
				cc.Dim("offset:"), offsetColored,
				cc.Dim("rtt:"), cc.Dim(r.RTT.Round(time.Millisecond).String()),
			)
		}

		avg, ok := dt.AverageOffset(results)
		if !ok {
			return fmt.Errorf("all NTP queries failed")
		}
		dispAvg := avg
		sign := "+"
		if dispAvg < 0 {
			sign = "-"
			dispAvg = -dispAvg
		}
		avgStr := sign + dispAvg.Round(time.Microsecond).String()
		fmt.Printf("\n%s %s\n", cc.Bold("Average offset:"), ntpOffsetColor(avg, avgStr))

		if doSync {
			ntpBin, ntpArgs := ntpSyncCmd(results)
			if ntpBin == "" {
				fmt.Printf("\n%s no suitable NTP sync tool found (install ntpdate or sntp).\n",
					cc.BoldYellow("Sync:"))
				return nil
			}
			var c *exec.Cmd
			if runtime.GOOS == "windows" {
				fmt.Printf("\n%s %s %s\n",
					cc.Dim("Running:"), ntpBin, strings.Join(ntpArgs, " "))
				c = exec.Command(ntpBin, ntpArgs...)
			} else {
				fmt.Printf("\n%s sudo %s %s\n",
					cc.Dim("Running:"), ntpBin, strings.Join(ntpArgs, " "))
				syncArgs := append([]string{ntpBin}, ntpArgs...)
				c = exec.Command("sudo", syncArgs...)
			}
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		}

		absAvg := avg
		if absAvg < 0 {
			absAvg = -absAvg
		}
		if absAvg > time.Second {
			hint := "'babi dt ntp --sync'"
			if runtime.GOOS == "windows" {
				hint = "'babi dt ntp --sync' (requires admin)"
			}
			fmt.Printf("\n%s clock is off by more than 1s — run %s to correct it.\n",
				cc.BoldYellow("Hint:"), cc.Cyan(hint))
		}
		return nil
	},
}

// ntpOffsetColor colors an offset string: green <100ms, yellow <1s, red ≥1s.
func ntpOffsetColor(offset time.Duration, s string) string {
	abs := offset
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= time.Second:
		return cc.BoldRed(s)
	case abs >= 100*time.Millisecond:
		return cc.BoldYellow(s)
	default:
		return cc.BoldGreen(s)
	}
}

func ntpSyncCmd(results []dt.NTPResult) (string, []string) {
	if runtime.GOOS == "windows" {
		// w32tm is built into Windows Vista+; /resync applies correction, /force skips the guard interval
		return "w32tm", []string{"/resync", "/force"}
	}
	server := "pool.ntp.org"
	for _, r := range results {
		if r.Err == nil {
			server = r.Server
			break
		}
	}
	if _, err := exec.LookPath("sntp"); err == nil {
		return "sntp", []string{"-sS", server}
	}
	if _, err := exec.LookPath("ntpdate"); err == nil {
		return "ntpdate", []string{server}
	}
	return "", nil
}

func formatInt(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// ─── babi convert ─────────────────────────────────────────────────────────────

var (
	cvFPS   int
	cvScale int
)

var convertCmd = &cobra.Command{
	Use:   "convert <input> <output>",
	Short: "Convert files between formats (images, video, audio, docs)",
	Long: `Convert files between formats using ffmpeg, ImageMagick, and pandoc.
The right tool is chosen automatically based on file extensions.

` + "  babi convert photo.heic       photo.jpg        # image format\n" +
		"  babi convert clip.mov        clip.mp4         # video format\n" +
		"  babi convert video.mp4       audio.mp3        # video → audio\n" +
		"  babi convert video.mp4       animation.gif    # video → gif\n" +
		"  babi convert animation.gif   video.mp4        # gif → video\n" +
		"  babi convert recording.m4a   recording.mp3   # audio format\n" +
		"  babi convert notes.md        notes.pdf        # document (pandoc)\n\n" +
		"Use subcommands for more control:\n" +
		"  crop      Crop image or video to a size\n" +
		"  trim      Cut video/audio by time range\n" +
		"  compress  Reduce file size\n" +
		"  frames    Extract frames from video or gif\n" +
		"  slideshow Create video or gif from an image directory",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Help()
		}
		return cv.AutoConvert(args[0], args[1], cvFPS, cvScale)
	},
}

var (
	cvCropSize string
	cvCropPos  string
)

var convertCropCmd = &cobra.Command{
	Use:   "crop <input> <output> --size WxH [--pos X,Y]",
	Short: "Crop an image or video to a given size",
	Long: `Crop an image or video frame to the specified dimensions.
--size is required. --pos sets the top-left corner (default 0,0).

  babi convert crop photo.jpg  out.jpg  --size 800x600
  babi convert crop photo.jpg  out.jpg  --size 800x600 --pos 100,50
  babi convert crop video.mp4  out.mp4  --size 1280x720
  babi convert crop video.mp4  out.mp4  --size 1280x720 --pos 320,180`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cvCropSize == "" {
			return fmt.Errorf("--size is required (e.g. --size 1280x720)")
		}
		input, output := args[0], args[1]
		t := cv.Classify(input)
		fmt.Printf("[babi] crop %s %s → %s  size=%s pos=%s\n",
			t, input, output, cvCropSize, cvCropPos)
		switch t {
		case cv.TypeImage, cv.TypeGIF:
			return cv.CropImage(input, output, cvCropSize, cvCropPos)
		case cv.TypeVideo:
			return cv.CropVideo(input, output, cvCropSize, cvCropPos)
		default:
			return fmt.Errorf("crop not supported for %s files", filepath.Ext(input))
		}
	},
}

var (
	cvTrimStart    string
	cvTrimEnd      string
	cvTrimDuration string
)

var convertTrimCmd = &cobra.Command{
	Use:   "trim <input> <output>",
	Short: "Cut video or audio by time range (stream copy, no re-encode)",
	Long: `Trim a video or audio file to a time range.
Uses stream copy — fast and lossless (no re-encoding).
Time format: HH:MM:SS, MM:SS, or plain seconds.

  babi convert trim video.mp4  out.mp4  --start 00:01:00 --end 00:02:30
  babi convert trim video.mp4  out.mp4  --start 30 --duration 60
  babi convert trim podcast.mp3 clip.mp3 --start 00:05:00 --end 00:10:00`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if cvTrimStart == "" && cvTrimEnd == "" && cvTrimDuration == "" {
			return fmt.Errorf("provide at least one of: --start, --end, --duration")
		}
		fmt.Printf("[babi] trim %s → %s\n", args[0], args[1])
		return cv.TrimVideo(args[0], args[1], cvTrimStart, cvTrimEnd, cvTrimDuration)
	},
}

var (
	cvCompressQuality int
	cvCompressCRF     int
	cvCompressBitrate string
)

var convertCompressCmd = &cobra.Command{
	Use:   "compress <input> <output>",
	Short: "Reduce file size (image quality, video CRF, audio bitrate)",
	Long: `Re-encode a file at lower quality to reduce its size.
Defaults: image quality=85, video CRF=28 (H.264), audio bitrate=128k.
CRF scale: 0=lossless, 23=default, 28=good compression, 51=worst.

  babi convert compress photo.jpg     small.jpg
  babi convert compress photo.jpg     small.jpg  --quality 70
  babi convert compress video.mp4     small.mp4
  babi convert compress video.mp4     small.mp4  --crf 32
  babi convert compress podcast.mp3   small.mp3
  babi convert compress podcast.mp3   small.mp3  --bitrate 96k`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		input, output := args[0], args[1]
		switch t := cv.Classify(input); t {
		case cv.TypeImage, cv.TypeGIF:
			q := cvCompressQuality
			if q == 0 {
				q = 85
			}
			fmt.Printf("[babi] compress image %s → %s  quality=%d\n", input, output, q)
			return cv.CompressImage(input, output, q)
		case cv.TypeVideo:
			crf := cvCompressCRF
			if crf == 0 {
				crf = 28
			}
			fmt.Printf("[babi] compress video %s → %s  crf=%d\n", input, output, crf)
			return cv.CompressVideo(input, output, crf)
		case cv.TypeAudio:
			br := cvCompressBitrate
			if br == "" {
				br = "128k"
			}
			fmt.Printf("[babi] compress audio %s → %s  bitrate=%s\n", input, output, br)
			return cv.CompressAudio(input, output, br)
		default:
			return fmt.Errorf("compress not supported for %s", filepath.Ext(input))
		}
	},
}

var cvFramesFPS int

var convertFramesCmd = &cobra.Command{
	Use:   "frames <input> <output-dir>",
	Short: "Extract frames from a video or GIF into a directory",
	Long: `Save every frame of a video or animated GIF as a PNG image.
By default all frames are extracted. Use --fps to sample fewer.

  babi convert frames animation.gif  ./frames/
  babi convert frames video.mp4      ./frames/
  babi convert frames video.mp4      ./frames/  --fps 1   # one per second`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("[babi] extracting frames %s → %s/\n", args[0], args[1])
		return cv.ExtractFrames(args[0], args[1], cvFramesFPS)
	},
}

var cvSlideshowFPS int

var convertSlideshowCmd = &cobra.Command{
	Use:   "slideshow <input-dir> <output>",
	Short: "Create a video or GIF from a directory of images",
	Long: `Build a video or animated GIF from all images in a directory.
Images are sorted alphabetically. Output format is determined by extension.
Defaults: 24 fps for video, 10 fps for GIF.

  babi convert slideshow ./photos/  timelapse.mp4
  babi convert slideshow ./photos/  timelapse.mp4  --fps 30
  babi convert slideshow ./frames/  animation.gif
  babi convert slideshow ./frames/  animation.gif  --fps 15`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		inputDir, output := args[0], args[1]
		outType := cv.Classify(output)
		fps := cvSlideshowFPS
		if fps == 0 {
			if outType == cv.TypeGIF {
				fps = 10
			} else {
				fps = 24
			}
		}
		fmt.Printf("[babi] slideshow %s/ → %s  fps=%d\n", inputDir, output, fps)
		if outType == cv.TypeGIF {
			return cv.DirToGif(inputDir, output, fps)
		}
		return cv.Slideshow(inputDir, output, fps)
	},
}

// ─── babi convert spritesheet ────────────────────────────────────────────────

var (
	cvSpriteCols    int
	cvSpriteTileW   int
	cvSpriteTileH   int
	cvSpritePadding int
)

var convertSpritesheetCmd = &cobra.Command{
	Use:   "spritesheet <output> <image-or-dir...>",
	Short: "Pack images into a spritesheet (ImageMagick)",
	Long: `Pack one or more images (or all images in a directory) into a single spritesheet.
Images are sorted alphabetically. Output format is determined by extension.

  babi convert spritesheet sheet.png  ./icons/
  babi convert spritesheet sheet.png  ./icons/  --cols 8
  babi convert spritesheet sheet.png  ./icons/  --cols 4 --tile 64x64
  babi convert spritesheet sheet.png  a.png b.png c.png  --tile 32x32 --padding 2`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		output := args[0]
		inputs := args[1:]

		// --tile WxH shorthand overrides --tile-w / --tile-h
		if tile, _ := cmd.Flags().GetString("tile"); tile != "" {
			parts := strings.SplitN(strings.ToLower(tile), "x", 2)
			if len(parts) == 2 {
				if w, err := strconv.Atoi(parts[0]); err == nil {
					cvSpriteTileW = w
				}
				if h, err := strconv.Atoi(parts[1]); err == nil {
					cvSpriteTileH = h
				}
			}
		}

		// If a single argument is a directory, expand it.
		if len(inputs) == 1 {
			info, err := os.Stat(inputs[0])
			if err != nil {
				return err
			}
			if info.IsDir() {
				fmt.Printf("[babi] spritesheet %s/ → %s\n", inputs[0], output)
				return cv.DirToSpritesheet(inputs[0], output, cvSpriteCols, cvSpriteTileW, cvSpriteTileH, cvSpritePadding)
			}
		}
		fmt.Printf("[babi] spritesheet %d images → %s\n", len(inputs), output)
		return cv.Spritesheet(inputs, output, cvSpriteCols, cvSpriteTileW, cvSpriteTileH, cvSpritePadding)
	},
}

// ─── babi convert merge ───────────────────────────────────────────────────────

var convertMergeCmd = &cobra.Command{
	Use:   "merge <output> <file1> <file2> [more...]",
	Short: "Concatenate audio/video files, or mux a video+audio pair",
	Long: `Merge multiple media files into one.

Concat mode  — all inputs are the same type (all audio or all video):
  babi convert merge out.mp3  part1.mp3 part2.mp3 part3.mp3
  babi convert merge full.mp4 clip1.mp4 clip2.mp4

Mux mode  — exactly one video and one audio input (combined into one file):
  babi convert merge out.mp4  video.mp4 audio.mp3
  babi convert merge out.mkv  video.mkv score.flac`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		output := args[0]
		inputs := args[1:]

		// Classify inputs to decide concat vs mux.
		var videos, audios []string
		for _, f := range inputs {
			switch cv.Classify(f) {
			case cv.TypeVideo, cv.TypeGIF:
				videos = append(videos, f)
			case cv.TypeAudio:
				audios = append(audios, f)
			default:
				return fmt.Errorf("unsupported file type for merge: %s", f)
			}
		}

		switch {
		case len(videos) == 1 && len(audios) == 1:
			fmt.Printf("[babi] mux %s + %s → %s\n", videos[0], audios[0], output)
			return cv.MuxVideoAudio(videos[0], audios[0], output)
		case len(audios) == 0 && len(videos) >= 2:
			fmt.Printf("[babi] merge %d video files → %s\n", len(videos), output)
			return cv.MergeMedia(videos, output)
		case len(videos) == 0 && len(audios) >= 2:
			fmt.Printf("[babi] merge %d audio files → %s\n", len(audios), output)
			return cv.MergeMedia(audios, output)
		default:
			return fmt.Errorf("ambiguous inputs: got %d video(s) and %d audio file(s)\n"+
				"for mux use exactly 1 video + 1 audio; for concat use files of the same type",
				len(videos), len(audios))
		}
	},
}

// ─── babi hash ────────────────────────────────────────────────────────────────

var hashAlgo string
var hashString string

var hashCmd = &cobra.Command{
	Use:   "hash [file...]",
	Short: "Hash files or strings",
	Long: `Hash files or strings using common algorithms.
Reads from stdin if no file is given.

  babi hash file.zip                    # sha256 (default)
  babi hash -a md5 file.zip             # md5
  babi hash -a sha1 -s "hello world"    # sha1 of a string
  babi hash *.go                        # hash multiple files`,
	RunE: func(cmd *cobra.Command, args []string) error {
		h, name, err := newHasher(hashAlgo)
		if err != nil {
			return err
		}
		// Hash a string
		if hashString != "" {
			h.Write([]byte(hashString))
			fmt.Printf("%s  %s (%s)\n", hex.EncodeToString(h.Sum(nil)), cc.Dim(`"`+hashString+`"`), name)
			return nil
		}
		// Hash files
		if len(args) == 0 {
			// stdin
			if _, err := io.Copy(h, os.Stdin); err != nil {
				return err
			}
			fmt.Printf("%s  -\n", hex.EncodeToString(h.Sum(nil)))
			return nil
		}
		for _, path := range args {
			hh, _, _ := newHasher(hashAlgo)
			f, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", cc.BoldRed("error"), err)
				continue
			}
			io.Copy(hh, f)
			f.Close()
			fmt.Printf("%s  %s\n", hex.EncodeToString(hh.Sum(nil)), path)
		}
		return nil
	},
}

func newHasher(algo string) (hash.Hash, string, error) {
	switch strings.ToLower(algo) {
	case "", "sha256":
		return sha256.New(), "sha256", nil
	case "sha512":
		return sha512.New(), "sha512", nil
	case "sha1":
		return sha1.New(), "sha1", nil
	case "sha224":
		return sha256.New224(), "sha224", nil
	case "sha384":
		return sha512.New384(), "sha384", nil
	case "md5":
		return md5.New(), "md5", nil
	}
	return nil, "", fmt.Errorf("unknown algorithm %q — use: md5, sha1, sha224, sha256, sha384, sha512", algo)
}

// ─── babi encode ──────────────────────────────────────────────────────────────

var encodeCmd = &cobra.Command{
	Use:   "encode",
	Short: "Encode and decode data (base64, hex, URL)",
	Long: `Encode or decode data. Reads from arguments or stdin.

  babi encode b64   "hello"           # base64 encode
  babi encode b64d  "aGVsbG8="        # base64 decode
  babi encode hex   "hello"           # hex encode
  babi encode hexd  "68656c6c6f"      # hex decode
  babi encode url   "hello world"     # URL encode
  babi encode urld  "hello%20world"   # URL decode
  echo "hello" | babi encode b64      # read from stdin`,
}

var encodeB64Cmd = &cobra.Command{
	Use: "b64 [data]", Short: "Encode to base64",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(data))
		return nil
	},
}

var encodeB64DCmd = &cobra.Command{
	Use: "b64d [data]", Short: "Decode from base64",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		out, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			// try URL encoding variant
			out, err = base64.URLEncoding.DecodeString(strings.TrimSpace(string(data)))
		}
		if err != nil {
			return fmt.Errorf("invalid base64: %w", err)
		}
		fmt.Print(string(out))
		return nil
	},
}

var encodeHexCmd = &cobra.Command{
	Use: "hex [data]", Short: "Encode to hex",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		fmt.Println(hex.EncodeToString(data))
		return nil
	},
}

var encodeHexDCmd = &cobra.Command{
	Use: "hexd [data]", Short: "Decode from hex",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		out, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return fmt.Errorf("invalid hex: %w", err)
		}
		fmt.Print(string(out))
		return nil
	},
}

var encodeURLCmd = &cobra.Command{
	Use: "url [data]", Short: "URL-encode a string",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		fmt.Println(url.QueryEscape(string(data)))
		return nil
	},
}

var encodeURLDCmd = &cobra.Command{
	Use: "urld [data]", Short: "URL-decode a string",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readEncodeInput(args)
		if err != nil {
			return err
		}
		out, err := url.QueryUnescape(strings.TrimSpace(string(data)))
		if err != nil {
			return fmt.Errorf("invalid URL encoding: %w", err)
		}
		fmt.Println(out)
		return nil
	},
}

func readEncodeInput(args []string) ([]byte, error) {
	if len(args) > 0 {
		return []byte(strings.Join(args, " ")), nil
	}
	return io.ReadAll(os.Stdin)
}

// ─── babi gen ─────────────────────────────────────────────────────────────────

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate UUIDs, passwords, and random strings",
	Long: `Generate random values for common developer needs.

  babi gen uuid                      # random UUID v4
  babi gen uuid -n 5                 # generate 5 UUIDs
  babi gen pass                      # 20-char password
  babi gen pass -l 32 -s             # 32-char with symbols
  babi gen str                       # 16-char alphanumeric string
  babi gen str -l 32 -c hex          # 32-char hex string
  babi gen str -l 16 -c alpha        # 16-char alphabetic`,
}

var genCount int

var genUUIDCmd = &cobra.Command{
	Use: "uuid", Short: "Generate UUID v4",
	RunE: func(cmd *cobra.Command, args []string) error {
		n := genCount
		if n <= 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			u, err := newUUID()
			if err != nil {
				return err
			}
			fmt.Println(u)
		}
		return nil
	},
}

var (
	genPassLen     int
	genPassSymbols bool
)

var genPassCmd = &cobra.Command{
	Use: "pass", Short: "Generate a random password",
	RunE: func(cmd *cobra.Command, args []string) error {
		n := genCount
		if n <= 0 {
			n = 1
		}
		l := genPassLen
		if l <= 0 {
			l = 20
		}
		charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		if genPassSymbols {
			charset += "!@#$%^&*-_=+?"
		}
		for i := 0; i < n; i++ {
			s, err := randString(charset, l)
			if err != nil {
				return err
			}
			fmt.Println(s)
		}
		return nil
	},
}

var (
	genStrLen     int
	genStrCharset string
)

var genStrCmd = &cobra.Command{
	Use: "str", Short: "Generate a random string",
	RunE: func(cmd *cobra.Command, args []string) error {
		n := genCount
		if n <= 0 {
			n = 1
		}
		l := genStrLen
		if l <= 0 {
			l = 16
		}
		var charset string
		switch strings.ToLower(genStrCharset) {
		case "hex":
			charset = "0123456789abcdef"
		case "alpha":
			charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
		case "num", "numeric":
			charset = "0123456789"
		default:
			charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		}
		for i := 0; i < n; i++ {
			s, err := randString(charset, l)
			if err != nil {
				return err
			}
			fmt.Println(s)
		}
		return nil
	},
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func randString(charset string, length int) (string, error) {
	max := big.NewInt(int64(len(charset)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = charset[n.Int64()]
	}
	return string(out), nil
}

// ─── babi port ────────────────────────────────────────────────────────────────

var portKill bool

var portCmd = &cobra.Command{
	Use:   "port <number>",
	Short: "Show what process is using a port",
	Long: `Find which process is listening on a port, with option to kill it.

  babi port 3000             # show process on port 3000
  babi port 3000 --kill      # kill the process on port 3000
  babi port list             # list all listening ports`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port := args[0]
		if _, err := strconv.Atoi(port); err != nil {
			return fmt.Errorf("invalid port number: %s", port)
		}
		procs, err := portProcs(port)
		if err != nil {
			return err
		}
		if len(procs) == 0 {
			fmt.Printf("nothing is listening on port %s\n", cc.BoldCyan(port))
			return nil
		}
		for _, p := range procs {
			fmt.Printf("port %s  pid %s  %s\n",
				cc.BoldCyan(port), cc.BoldYellow(p.pid), p.name)
		}
		if portKill {
			for _, p := range procs {
				var killCmd *exec.Cmd
				if runtime.GOOS == "windows" {
					killCmd = exec.Command("taskkill", "/F", "/PID", p.pid)
				} else {
					killCmd = exec.Command("kill", "-9", p.pid)
				}
				if err := killCmd.Run(); err != nil {
					fmt.Printf("%s killing pid %s: %v\n", cc.BoldRed("error"), p.pid, err)
				} else {
					fmt.Printf("%s killed pid %s (%s)\n", cc.BoldGreen("killed"), p.pid, p.name)
				}
			}
		}
		return nil
	},
}

var portListCmd = &cobra.Command{
	Use: "list", Short: "List all listening ports",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS == "windows" {
			out, err := exec.Command("netstat", "-ano").Output()
			if err != nil {
				return fmt.Errorf("netstat not available: %w", err)
			}
			fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
				cc.BoldCyan("Proto"), cc.BoldCyan("Local Address"),
				cc.BoldCyan("Foreign Address"), cc.BoldCyan("State"), cc.BoldCyan("PID"))
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				// TCP: Proto Local Foreign State PID (5 fields)
				// UDP: Proto Local Foreign PID    (4 fields, no state)
				if len(fields) == 5 && strings.EqualFold(fields[3], "LISTENING") {
					fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
						fields[0], fields[1], fields[2], fields[3], fields[4])
				} else if len(fields) == 4 && strings.EqualFold(fields[0], "UDP") {
					fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
						fields[0], fields[1], fields[2], "", fields[3])
				}
			}
			return nil
		}
		out, err := exec.Command("lsof", "-iTCP", "-iUDP", "-n", "-P",
			"-sTCP:LISTEN").Output()
		if err != nil {
			// fallback: try ss (Linux)
			out, err = exec.Command("ss", "-tlnup").Output()
			if err != nil {
				return fmt.Errorf("lsof and ss not available: %w", err)
			}
			fmt.Print(string(out))
			return nil
		}
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			fmt.Println(cc.BoldCyan(lines[0])) // header
		}
		for _, l := range lines[1:] {
			if l != "" {
				fmt.Println(l)
			}
		}
		return nil
	},
}

type procInfo struct {
	pid  string
	name string
}

func portProcs(port string) ([]procInfo, error) {
	if runtime.GOOS == "windows" {
		return portProcsWindows(port)
	}
	out, err := exec.Command("lsof", "-i:"+port, "-n", "-P", "-F", "cpn").Output()
	if err != nil && len(out) == 0 {
		return nil, nil // nothing on that port
	}
	var procs []procInfo
	var cur procInfo
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			cur.pid = line[1:]
		case 'c':
			cur.name = line[1:]
		case 'n':
			if cur.pid != "" {
				procs = append(procs, cur)
				cur = procInfo{}
			}
		}
	}
	// deduplicate by pid
	seen := map[string]bool{}
	var unique []procInfo
	for _, p := range procs {
		if !seen[p.pid] {
			seen[p.pid] = true
			unique = append(unique, p)
		}
	}
	return unique, nil
}

// portProcsWindows parses `netstat -ano` output to find processes on a port.
func portProcsWindows(port string) ([]procInfo, error) {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var procs []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// TCP LISTENING: Proto Local Foreign State PID
		if len(fields) == 5 && strings.EqualFold(fields[3], "LISTENING") {
			if strings.HasSuffix(fields[1], ":"+port) {
				pid := strings.TrimSpace(fields[4])
				if !seen[pid] {
					seen[pid] = true
					procs = append(procs, procInfo{pid: pid, name: winProcName(pid)})
				}
			}
		}
		// UDP: Proto Local Foreign PID (no state)
		if len(fields) == 4 && strings.EqualFold(fields[0], "UDP") {
			if strings.HasSuffix(fields[1], ":"+port) {
				pid := strings.TrimSpace(fields[3])
				if !seen[pid] {
					seen[pid] = true
					procs = append(procs, procInfo{pid: pid, name: winProcName(pid)})
				}
			}
		}
	}
	return procs, nil
}

// winProcName looks up a process name by PID using tasklist.
func winProcName(pid string) string {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+pid, "/FO", "CSV", "/NH").Output()
	if err != nil {
		return "unknown"
	}
	line := strings.TrimSpace(string(out))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return "unknown"
	}
	// CSV: "process.exe","1234","Console","1","1,234 K"
	parts := strings.SplitN(line, ",", 2)
	return strings.Trim(parts[0], "\"")
}

// ─── babi log ─────────────────────────────────────────────────────────────────

var logCmd = &cobra.Command{
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
		repoDir, err := gitui.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf("not a git repository: %w", err)
		}
		p := tea.NewProgram(gitui.NewLogModel(repoDir), tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// ─── babi stash ───────────────────────────────────────────────────────────────

var stashCmd = &cobra.Command{
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
		repoDir, err := gitui.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf("not a git repository: %w", err)
		}
		p := tea.NewProgram(gitui.NewStashModel(repoDir), tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

// ─── babi pdf ─────────────────────────────────────────────────────────────────

var pdfCmd = &cobra.Command{
	Use:   "pdf",
	Short: "PDF utilities (merge, split)",
}

var pdfMergeCmd = &cobra.Command{
	Use:   "merge <output.pdf> <file1.pdf> <file2.pdf> [more...]",
	Short: "Merge two or more PDF files into one",
	Long: `Merge multiple PDF files into a single output PDF.

  babi pdf merge combined.pdf a.pdf b.pdf c.pdf
  babi pdf merge combined.pdf a.pdf b.pdf --divider`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		output := args[0]
		inputs := args[1:]
		divider, _ := cmd.Flags().GetBool("divider")
		fmt.Printf("[babi] pdf merge %d files → %s\n", len(inputs), output)
		return cv.MergePDF(inputs, output, divider)
	},
}

var pdfSplitCmd = &cobra.Command{
	Use:   "split <input.pdf> <output-dir>",
	Short: "Split a PDF into smaller files",
	Long: `Split a PDF by span (every N pages) or at explicit page boundaries.

  babi pdf split doc.pdf ./parts/                    # one file per page
  babi pdf split doc.pdf ./parts/ --span 5           # every 5 pages
  babi pdf split doc.pdf ./parts/ --pages 3,6,9      # split before pages 3, 6, 9`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		input, outDir := args[0], args[1]
		span, _ := cmd.Flags().GetInt("span")
		pagesStr, _ := cmd.Flags().GetString("pages")

		if pagesStr != "" {
			var pageNrs []int
			for _, p := range strings.Split(pagesStr, ",") {
				p = strings.TrimSpace(p)
				n, err := strconv.Atoi(p)
				if err != nil || n < 1 {
					return fmt.Errorf("invalid page number %q", p)
				}
				pageNrs = append(pageNrs, n)
			}
			fmt.Printf("[babi] pdf split %s at pages %v → %s/\n", input, pageNrs, outDir)
			return cv.SplitPDFAtPages(input, outDir, pageNrs)
		}

		if span == 0 {
			span = 1
		}
		fmt.Printf("[babi] pdf split %s  span=%d → %s/\n", input, span, outDir)
		return cv.SplitPDF(input, outDir, span)
	},
}

// ─── babi ip ─────────────────────────────────────────────────────────────────

var ipAllFlag bool

var ipCmd = &cobra.Command{
	Use:   "ip",
	Short: "Show local IP address for the internet-facing interface",
	Long: `Show the local IP of the network interface used for internet access.
Use --all to also list every non-loopback interface.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// UDP dial to a well-known external address determines routing without
		// sending any actual packets.
		conn, err := net.Dial("udp", "8.8.8.8:80")
		if err == nil {
			defer conn.Close()
			primary := conn.LocalAddr().(*net.UDPAddr).IP.String()

			// Find which interface owns this IP.
			ifaceName := ""
			if ifaces, e := net.Interfaces(); e == nil {
			outer:
				for _, iface := range ifaces {
					addrs, _ := iface.Addrs()
					for _, addr := range addrs {
						var ip net.IP
						switch v := addr.(type) {
						case *net.IPNet:
							ip = v.IP
						case *net.IPAddr:
							ip = v.IP
						}
						if ip != nil && ip.String() == primary {
							ifaceName = iface.Name
							break outer
						}
					}
				}
			}

			label := "IP"
			if ifaceName != "" {
				label = fmt.Sprintf("IP (%s)", ifaceName)
			}
			fmt.Printf("%-24s %s\n", cc.Dim(label+":"), cc.BoldGreen(primary))
		} else {
			fmt.Printf("%s could not determine internet-facing interface\n", cc.BoldYellow("Warning:"))
		}

		if !ipAllFlag {
			return nil
		}

		fmt.Println()
		fmt.Println(cc.Bold("All interfaces:"))
		ifaces, err := net.Interfaces()
		if err != nil {
			return err
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				var ip net.IP
				var mask net.IPMask
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
					mask = v.Mask
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() {
					continue
				}
				ipStr := ip.String()
				if mask != nil {
					ones, _ := mask.Size()
					ipStr = fmt.Sprintf("%s/%d", ipStr, ones)
				}
				fmt.Printf("  %-14s %s\n", cc.Dim(iface.Name), cc.Cyan(ipStr))
			}
		}
		return nil
	},
}

// ─── colored cobra help ───────────────────────────────────────────────────────

func initCobraColors() {
	cobra.AddTemplateFunc("cH", func(s string) string { return cc.BoldCyan(s) })
	cobra.AddTemplateFunc("cCmd", func(s string) string { return cc.BrightGreen(s) })
	cobra.AddTemplateFunc("cDim", func(s string) string { return cc.Dim(s) })

	const usageTmpl = `{{cH "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{cH "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{cH "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{cH "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cCmd (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{cH "Flags:"}}
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasAvailableInheritedFlags}}

{{cH "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

{{cH "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

{{cDim (printf "Use \"%s [command] --help\" for more information about a command." .CommandPath)}}{{end}}
`
	rootCmd.SetUsageTemplate(usageTmpl)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

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

func main() {
	defPath, _ := config.DefaultPath()

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defPath, "path to config.json")

	syncCmd.AddCommand(syncRunCmd, syncAddCmd, syncListCmd, syncRemoveCmd)
	commitRunCmd.Flags().StringVarP(&commitRunType, "type", "t", "chore", "commit type (feat, fix, docs, ...)")
	commitRunCmd.Flags().StringVarP(&commitRunScope, "scope", "s", "", "commit scope (optional)")
	commitRunCmd.Flags().BoolVarP(&commitRunAll, "all", "a", false, "stage all changed files")
	commitCmd.AddCommand(commitRunCmd)
	initEdFlags()
	dtNTPCmd.Flags().Bool("sync", false, "apply NTP correction to system clock (requires sudo)")
	dtCmd.AddCommand(dtInCmd, dtAgeCmd, dtTZCmd, dtNTPCmd)
	convertCmd.Flags().IntVar(&cvFPS, "fps", 0, "frames per second for gif output (default 10)")
	convertCmd.Flags().IntVar(&cvScale, "scale", 0, "output width in pixels for gif (default 480)")
	convertCropCmd.Flags().StringVar(&cvCropSize, "size", "", "crop size as WxH (e.g. 1280x720)")
	convertCropCmd.Flags().StringVar(&cvCropPos, "pos", "", "top-left corner as X,Y (default 0,0)")
	convertTrimCmd.Flags().StringVar(&cvTrimStart, "start", "", "start time (HH:MM:SS or seconds)")
	convertTrimCmd.Flags().StringVar(&cvTrimEnd, "end", "", "end time (HH:MM:SS or seconds)")
	convertTrimCmd.Flags().StringVar(&cvTrimDuration, "duration", "", "duration (HH:MM:SS or seconds)")
	convertCompressCmd.Flags().IntVar(&cvCompressQuality, "quality", 0, "image quality 0-100 (default 85)")
	convertCompressCmd.Flags().IntVar(&cvCompressCRF, "crf", 0, "video CRF 0-51, lower=better (default 28)")
	convertCompressCmd.Flags().StringVar(&cvCompressBitrate, "bitrate", "", "audio bitrate (default 128k)")
	convertFramesCmd.Flags().IntVar(&cvFramesFPS, "fps", 0, "frames per second to extract (default: all frames)")
	convertSlideshowCmd.Flags().IntVar(&cvSlideshowFPS, "fps", 0, "fps (default 24 for video, 10 for gif)")
	convertSpritesheetCmd.Flags().IntVar(&cvSpriteCols, "cols", 0, "number of columns (default: auto-square)")
	convertSpritesheetCmd.Flags().IntVar(&cvSpriteTileW, "tile-w", 0, "cell width in px (default: natural size)")
	convertSpritesheetCmd.Flags().IntVar(&cvSpriteTileH, "tile-h", 0, "cell height in px (default: natural size)")
	convertSpritesheetCmd.Flags().IntVar(&cvSpritePadding, "padding", 0, "gap between cells in px (default 0)")
	convertSpritesheetCmd.Flags().StringP("tile", "t", "", "shorthand for --tile-w and --tile-h as WxH (e.g. 64x64)")
	convertCmd.AddCommand(convertCropCmd, convertTrimCmd, convertCompressCmd, convertFramesCmd, convertSlideshowCmd, convertSpritesheetCmd, convertMergeCmd)

	// hash
	hashCmd.Flags().StringVarP(&hashAlgo, "algo", "a", "sha256", "algorithm: md5, sha1, sha224, sha256, sha384, sha512")
	hashCmd.Flags().StringVarP(&hashString, "string", "s", "", "hash a string instead of a file")

	// encode
	encodeCmd.AddCommand(encodeB64Cmd, encodeB64DCmd, encodeHexCmd, encodeHexDCmd, encodeURLCmd, encodeURLDCmd)

	// gen
	genUUIDCmd.Flags().IntVarP(&genCount, "count", "n", 1, "number of values to generate")
	genPassCmd.Flags().IntVarP(&genPassLen, "length", "l", 20, "password length")
	genPassCmd.Flags().BoolVarP(&genPassSymbols, "symbols", "s", false, "include symbols (!@#$%^&*-_=+?)")
	genPassCmd.Flags().IntVarP(&genCount, "count", "n", 1, "number of passwords to generate")
	genStrCmd.Flags().IntVarP(&genStrLen, "length", "l", 16, "string length")
	genStrCmd.Flags().StringVarP(&genStrCharset, "charset", "c", "alphanum", "charset: alphanum, alpha, hex, num")
	genStrCmd.Flags().IntVarP(&genCount, "count", "n", 1, "number of strings to generate")
	genCmd.AddCommand(genUUIDCmd, genPassCmd, genStrCmd)

	// port
	portCmd.Flags().BoolVar(&portKill, "kill", false, "kill the process on the port")
	portCmd.AddCommand(portListCmd)

	// ip
	ipCmd.Flags().BoolVarP(&ipAllFlag, "all", "a", false, "list all non-loopback interfaces")

	pdfMergeCmd.Flags().Bool("divider", false, "insert a blank page between each merged PDF")
	pdfSplitCmd.Flags().Int("span", 0, "pages per output file (default 1)")
	pdfSplitCmd.Flags().String("pages", "", "split before these page numbers, comma-separated (e.g. 3,6,9)")
	pdfCmd.AddCommand(pdfMergeCmd, pdfSplitCmd)

	rootCmd.AddCommand(syncCmd, commitCmd, searchCmd, replaceCmd, editCmd, hexCmd, fmCmd, dtCmd, convertCmd, hashCmd, encodeCmd, genCmd, portCmd, ipCmd, logCmd, stashCmd, pdfCmd, meta.Command(), cf.Command(), tree.Command(), tag.Command(), check.Command(), serve.Command(), pack.PackCommand(), pack.UnpackCommand(), typer.Command(), newproject.Command())
	initCobraColors()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
