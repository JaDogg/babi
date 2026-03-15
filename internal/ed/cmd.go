package ed

import (
	"os"

	"github.com/spf13/cobra"
)

func SearchCommand() *cobra.Command {
	var (
		edType    string
		edGlob    string
		edNoCase  bool
		edHidden  bool
		edFiles   bool
		edBefore  int
		edAfter   int
		edContext int
	)

	cmd := &cobra.Command{
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
			opts := SearchOpts{
				FileType:      edType,
				Glob:          edGlob,
				IgnoreCase:    edNoCase,
				Hidden:        edHidden,
				FilesOnly:     edFiles,
				ContextBefore: ctx,
				ContextAfter:  edAfter,
			}
			return Search(os.Stdout, pattern, path, opts)
		},
	}
	cmd.Flags().StringVarP(&edType, "type", "t", "", "file type filter (go, py, js, ts, rs, …)")
	cmd.Flags().StringVarP(&edGlob, "glob", "g", "", "glob pattern for filenames (e.g. *.go)")
	cmd.Flags().BoolVarP(&edNoCase, "ignore-case", "i", false, "case-insensitive matching")
	cmd.Flags().BoolVar(&edHidden, "hidden", false, "include hidden files and directories")
	cmd.Flags().BoolVarP(&edFiles, "files", "l", false, "print only filenames with matches")
	cmd.Flags().IntVarP(&edBefore, "before", "B", 0, "lines of context before match")
	cmd.Flags().IntVarP(&edAfter, "after", "A", 0, "lines of context after match")
	cmd.Flags().IntVarP(&edContext, "context", "C", 0, "lines of context before and after")
	return cmd
}

func ReplaceCommand() *cobra.Command {
	var (
		edType    string
		edGlob    string
		edNoCase  bool
		edHidden  bool
		edDryRun  bool
		edLiteral bool
	)

	cmd := &cobra.Command{
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
			opts := ReplaceOpts{
				FileType:   edType,
				Glob:       edGlob,
				IgnoreCase: edNoCase,
				Hidden:     edHidden,
				DryRun:     edDryRun,
				Literal:    edLiteral,
			}
			return Replace(os.Stdout, pattern, replacement, path, opts)
		},
	}
	cmd.Flags().StringVarP(&edType, "type", "t", "", "file type filter (go, py, js, ts, rs, …)")
	cmd.Flags().StringVarP(&edGlob, "glob", "g", "", "glob pattern for filenames (e.g. *.go)")
	cmd.Flags().BoolVarP(&edNoCase, "ignore-case", "i", false, "case-insensitive matching")
	cmd.Flags().BoolVar(&edHidden, "hidden", false, "include hidden files and directories")
	cmd.Flags().BoolVarP(&edDryRun, "dry-run", "n", false, "show changes without writing files")
	cmd.Flags().BoolVarP(&edLiteral, "literal", "F", false, "treat pattern as literal string, not regex")
	return cmd
}
