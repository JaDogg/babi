package fscmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "fs",
		Short: "File system operations (mkdir, rm, cp)",
		Long:  "File system operations: create directories, remove files, copy files.",
	}
	c.AddCommand(mkdirCmd(), rmCmd(), cpCmd())
	return c
}

func mkdirCmd() *cobra.Command {
	var parents bool
	c := &cobra.Command{
		Use:   "mkdir <dir...>",
		Short: "Create directories",
		Long: `Create directories.

  babi fs mkdir internal/mytool
  babi fs mkdir -p internal/a/b/c   # create parents as needed`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, dir := range args {
				var err error
				if parents {
					err = os.MkdirAll(dir, 0755)
				} else {
					err = os.Mkdir(dir, 0755)
				}
				if err != nil {
					return fmt.Errorf("mkdir %s: %w", dir, err)
				}
				fmt.Printf("created %s\n", dir)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&parents, "parents", "p", false, "create parent directories as needed")
	return c
}

func rmCmd() *cobra.Command {
	var recursive bool
	c := &cobra.Command{
		Use:   "rm <path...>",
		Short: "Remove files or directories",
		Long: `Remove files or directories.

  babi fs rm file.go
  babi fs rm cmd_a.go cmd_b.go
  babi fs rm -r mydir/    # remove directory recursively`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, path := range args {
				var err error
				if recursive {
					err = os.RemoveAll(path)
				} else {
					err = os.Remove(path)
				}
				if err != nil {
					return fmt.Errorf("rm %s: %w", path, err)
				}
				fmt.Printf("removed %s\n", path)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&recursive, "recursive", "r", false, "remove directory recursively")
	return c
}

func cpCmd() *cobra.Command {
	var recursive bool
	c := &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "Copy a file or directory",
		Long: `Copy a file or directory.

  babi fs cp babi ~/.local/bin/babi
  babi fs cp -r src/ dst/`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, dst := args[0], args[1]
			info, err := os.Stat(src)
			if err != nil {
				return err
			}
			if info.IsDir() {
				if !recursive {
					return fmt.Errorf("%s is a directory (use -r to copy recursively)", src)
				}
				return copyDir(src, dst)
			}
			return copyFile(src, dst)
		},
	}
	c.Flags().BoolVarP(&recursive, "recursive", "r", false, "copy directories recursively")
	return c
}

func copyFile(src, dst string) error {
	if info, err := os.Stat(dst); err == nil && info.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	fmt.Printf("copied %s → %s\n", src, dst)
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}
