package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	cc "github.com/jadogg/babi/internal/clicolor"
)

// Command returns the "babi tree" cobra command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "tree [dir]",
		Short: "Display directory contents as a tree",
		Long: `List the contents of a directory as an indented tree.

  babi tree
  babi tree ./internal
  babi tree -a              # include hidden (dot) files
  babi tree -d              # directories only
  babi tree -L 3            # limit to 3 levels deep
  babi tree -a -L 2 ./src`,
		Args: cobra.MaximumNArgs(1),
		RunE: run,
	}
	c.Flags().BoolP("all", "a", false, "include hidden files and directories (dot files)")
	c.Flags().BoolP("dirs-only", "d", false, "list directories only")
	c.Flags().IntP("level", "L", 0, "max depth (0 = unlimited)")
	return c
}

func run(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("%s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}

	showHidden, _ := cmd.Flags().GetBool("all")
	dirsOnly, _ := cmd.Flags().GetBool("dirs-only")
	maxLevel, _ := cmd.Flags().GetInt("level")

	// Print root
	fmt.Println(cc.BoldCyan(root))

	stats := &walkStats{}
	walk(root, "", showHidden, dirsOnly, maxLevel, 0, stats)

	fmt.Printf("\n%s, %s\n",
		cc.Bold(fmt.Sprintf("%d director%s", stats.dirs, pluralY(stats.dirs))),
		cc.Bold(fmt.Sprintf("%d file%s", stats.files, pluralS(stats.files))),
	)
	return nil
}

type walkStats struct {
	dirs  int
	files int
}

func walk(dir, prefix string, showHidden, dirsOnly bool, maxLevel, level int, stats *walkStats) {
	if maxLevel > 0 && level >= maxLevel {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Printf("%s%s %s\n", prefix, branchEnd, cc.BoldRed("[error reading dir]"))
		return
	}

	// Filter entries
	var visible []os.DirEntry
	for _, e := range entries {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if dirsOnly && !e.IsDir() {
			continue
		}
		visible = append(visible, e)
	}

	// Sort: dirs first, then files, both alphabetical
	sort.Slice(visible, func(i, j int) bool {
		di, dj := visible[i].IsDir(), visible[j].IsDir()
		if di != dj {
			return di // dirs before files
		}
		return strings.ToLower(visible[i].Name()) < strings.ToLower(visible[j].Name())
	})

	for i, e := range visible {
		last := i == len(visible)-1

		connector := branchMid
		childPrefix := prefix + prefixMid
		if last {
			connector = branchEnd
			childPrefix = prefix + prefixEmpty
		}

		name := e.Name()
		if e.IsDir() {
			stats.dirs++
			fmt.Printf("%s%s%s\n", prefix, connector, cc.BoldCyan(name+"/"))
			walk(filepath.Join(dir, name), childPrefix, showHidden, dirsOnly, maxLevel, level+1, stats)
		} else {
			stats.files++
			info, err := e.Info()
			sizeStr := ""
			if err == nil {
				sizeStr = cc.Dim(formatSize(info.Size()))
			}
			fmt.Printf("%s%s%s  %s\n", prefix, connector, name, sizeStr)
		}
	}
}

// Box-drawing constants for the tree lines.
const (
	branchMid   = "├── "
	branchEnd   = "└── "
	prefixMid   = "│   "
	prefixEmpty = "    "
)

func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
