package ls

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var long, all, recursive bool
	c := &cobra.Command{
		Use:   "ls [path...]",
		Short: "List directory contents",
		Long: `List directory contents.

  babi ls                   # current directory
  babi ls internal/
  babi ls -l                # long format: perms  size  mtime  name
  babi ls -a                # include hidden (dot) files
  babi ls -r                # recursive, all files (like find -type f | sort)
  babi ls -la internal/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(args, long, all, recursive)
		},
	}
	c.Flags().BoolVarP(&long, "long", "l", false, "long format")
	c.Flags().BoolVarP(&all, "all", "a", false, "include hidden files")
	c.Flags().BoolVarP(&recursive, "recursive", "r", false, "recursive (files only)")
	return c
}

func run(args []string, long, all, recursive bool) error {
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	for i, p := range paths {
		if i > 0 {
			fmt.Println()
		}

		info, err := os.Stat(p)
		if err != nil {
			return err
		}

		if len(paths) > 1 {
			fmt.Println(cc.Bold(p) + ":")
		}

		if !info.IsDir() {
			if long {
				printLong(info, p)
			} else {
				fmt.Println(info.Name())
			}
			continue
		}

		if recursive {
			if err := listRecursive(p, all, long); err != nil {
				return err
			}
			continue
		}

		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if !all && strings.HasPrefix(name, ".") {
				continue
			}
			if long {
				fi, err := e.Info()
				if err != nil {
					continue
				}
				printLong(fi, filepath.Join(p, name))
			} else {
				if e.IsDir() {
					fmt.Println(cc.BoldCyan(name + "/"))
				} else {
					fmt.Println(name)
				}
			}
		}
	}
	return nil
}

func listRecursive(root string, all, long bool) error {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !all && strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !all && strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		if long {
			fi, err := os.Stat(f)
			if err != nil {
				continue
			}
			printLong(fi, f)
		} else {
			fmt.Println(f)
		}
	}
	return nil
}

func printLong(fi os.FileInfo, path string) {
	mode := fi.Mode().String()
	size := fi.Size()
	mtime := fi.ModTime().Format(time.DateTime)
	name := path
	if fi.IsDir() {
		name = cc.BoldCyan(path + "/")
	}
	fmt.Printf("%s  %8d  %s  %s\n", mode, size, mtime, name)
}
