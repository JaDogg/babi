package ed

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SearchOpts configures a search.
type SearchOpts struct {
	FileType      string
	Glob          string
	IgnoreCase    bool
	Hidden        bool
	FilesOnly     bool
	ContextBefore int
	ContextAfter  int
}

// ReplaceOpts configures a find-and-replace.
type ReplaceOpts struct {
	FileType   string
	Glob       string
	IgnoreCase bool
	Hidden     bool
	DryRun     bool
	Literal    bool
}

// typeExts maps rg-style type names to file extensions.
var typeExts = map[string][]string{
	"go":     {".go"},
	"py":     {".py"},
	"js":     {".js"},
	"ts":     {".ts"},
	"jsx":    {".jsx"},
	"tsx":    {".tsx"},
	"rs":     {".rs"},
	"c":      {".c", ".h"},
	"cpp":    {".cpp", ".cc", ".cxx", ".h", ".hpp"},
	"java":   {".java"},
	"rb":     {".rb"},
	"md":     {".md"},
	"json":   {".json"},
	"yaml":   {".yaml", ".yml"},
	"toml":   {".toml"},
	"html":   {".html", ".htm"},
	"css":    {".css"},
	"sh":     {".sh", ".bash", ".zsh"},
	"txt":    {".txt"},
	"swift":  {".swift"},
	"kotlin": {".kt"},
	"lua":    {".lua"},
	"zig":    {".zig"},
}

// HasRipgrep reports whether rg is on PATH.
func HasRipgrep() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

// hasGrep reports whether grep is on PATH.
func hasGrep() bool {
	_, err := exec.LookPath("grep")
	return err == nil
}

// Search runs a search using rg, grep, or a pure-Go fallback (in that order).
func Search(w io.Writer, pattern, path string, opts SearchOpts) error {
	if HasRipgrep() {
		cmd := buildRgCmd(pattern, path, opts)
		cmd.Stdout = w
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if ex, ok := err.(*exec.ExitError); ok && ex.ExitCode() == 1 {
				return nil
			}
			return err
		}
		return nil
	}
	if hasGrep() {
		fmt.Fprintln(w, "[babi ed] note: ripgrep not found, falling back to grep (--type and --hidden flags unavailable)")
		cmd := buildGrepCmd(pattern, path, opts)
		cmd.Stdout = w
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if ex, ok := err.(*exec.ExitError); ok && ex.ExitCode() == 1 {
				return nil
			}
			return err
		}
		return nil
	}
	fmt.Fprintln(w, "[babi ed] note: ripgrep and grep not found, using built-in search (--hidden flag unavailable)")
	return searchGo(w, pattern, path, opts)
}

func buildRgCmd(pattern, path string, opts SearchOpts) *exec.Cmd {
	args := []string{"--line-number", "--no-heading", "--color=never", "--with-filename"}
	if opts.IgnoreCase {
		args = append(args, "-i")
	}
	if opts.FileType != "" {
		args = append(args, "-t", opts.FileType)
	}
	if opts.Glob != "" {
		args = append(args, "-g", opts.Glob)
	}
	if opts.Hidden {
		args = append(args, "--hidden")
	}
	if opts.FilesOnly {
		args = append(args, "-l")
	}
	if opts.ContextBefore > 0 {
		args = append(args, fmt.Sprintf("-B%d", opts.ContextBefore))
	}
	if opts.ContextAfter > 0 {
		args = append(args, fmt.Sprintf("-A%d", opts.ContextAfter))
	}
	args = append(args, "--", pattern)
	if path != "" && path != "." {
		args = append(args, path)
	}
	return exec.Command("rg", args...)
}

func buildGrepCmd(pattern, path string, opts SearchOpts) *exec.Cmd {
	args := []string{"-rn", "--color=never"}
	if opts.IgnoreCase {
		args = append(args, "-i")
	}
	if opts.FilesOnly {
		args = append(args, "-l")
	}
	if opts.Glob != "" {
		args = append(args, "--include="+opts.Glob)
	}
	if opts.ContextBefore > 0 {
		args = append(args, fmt.Sprintf("-B%d", opts.ContextBefore))
	}
	if opts.ContextAfter > 0 {
		args = append(args, fmt.Sprintf("-A%d", opts.ContextAfter))
	}
	if path == "" {
		path = "."
	}
	args = append(args, "--", pattern, path)
	return exec.Command("grep", args...)
}

// Replace performs regex (or literal) find-and-replace across files under path.
func Replace(w io.Writer, oldPat, newStr, path string, opts ReplaceOpts) error {
	pat := oldPat
	if opts.Literal {
		pat = regexp.QuoteMeta(oldPat)
	}
	prefix := ""
	if opts.IgnoreCase {
		prefix = "(?i)"
	}
	re, err := regexp.Compile(prefix + pat)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", oldPat, err)
	}

	files, err := collectFiles(path, opts.FileType, opts.Glob, opts.Hidden)
	if err != nil {
		return err
	}

	totalFiles, totalReplacements := 0, 0
	for _, f := range files {
		n, err := processFile(w, f, re, newStr, opts.DryRun)
		if err != nil {
			fmt.Fprintf(w, "error: %s: %v\n", f, err)
			continue
		}
		if n > 0 {
			totalFiles++
			totalReplacements += n
			if !opts.DryRun {
				fmt.Fprintf(w, "%s: %d replacement(s)\n", f, n)
			}
		}
	}

	if totalReplacements == 0 {
		fmt.Fprintln(w, "no matches found")
		return nil
	}
	tag := "replaced"
	if opts.DryRun {
		tag = "[dry-run] would replace"
	}
	fmt.Fprintf(w, "\n%s %d occurrence(s) in %d file(s)\n", tag, totalReplacements, totalFiles)
	return nil
}

// processFile applies the regex replacement to a single file.
// Returns the number of lines changed.
func processFile(w io.Writer, path string, re *regexp.Regexp, newStr string, dryRun bool) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	// Skip binary files
	if isBinary(data) {
		return 0, nil
	}

	lines := strings.Split(string(data), "\n")
	changed := 0
	newLines := make([]string, len(lines))

	var diffs []string
	for i, line := range lines {
		after := re.ReplaceAllString(line, newStr)
		newLines[i] = after
		if after != line {
			changed++
			if dryRun {
				diffs = append(diffs, fmt.Sprintf("  line %d:\n  - %s\n  + %s", i+1, line, after))
			}
		}
	}

	if changed == 0 {
		return 0, nil
	}

	if dryRun {
		fmt.Fprintf(w, "%s\n", path)
		for _, d := range diffs {
			fmt.Fprintln(w, d)
		}
		return changed, nil
	}

	return changed, os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o644)
}

// collectFiles gathers files under root matching the given filters.
func collectFiles(root, fileType, glob string, hidden bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var exts []string
	if fileType != "" {
		exts = typeExts[fileType]
		if exts == nil {
			return nil, fmt.Errorf("unknown file type %q (use rg --type-list for valid names)", fileType)
		}
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if !hidden && strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !hidden && strings.HasPrefix(name, ".") {
			return nil
		}
		if len(exts) > 0 {
			ext := strings.ToLower(filepath.Ext(name))
			ok := false
			for _, e := range exts {
				if ext == e {
					ok = true
					break
				}
			}
			if !ok {
				return nil
			}
		}
		if glob != "" {
			matched, _ := filepath.Match(glob, name)
			if !matched {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// searchGo is a pure-Go fallback for Search used when neither rg nor grep is available.
func searchGo(w io.Writer, pattern, path string, opts SearchOpts) error {
	prefix := ""
	if opts.IgnoreCase {
		prefix = "(?i)"
	}
	re, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	if path == "" {
		path = "."
	}

	files, err := collectFiles(path, opts.FileType, opts.Glob, opts.Hidden)
	if err != nil {
		return err
	}

	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		var lines []string
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		fh.Close()

		if opts.FilesOnly {
			for _, line := range lines {
				if re.MatchString(line) {
					fmt.Fprintln(w, f)
					break
				}
			}
			continue
		}

		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			lineNo := i + 1
			start := i - opts.ContextBefore
			if start < 0 {
				start = 0
			}
			end := i + opts.ContextAfter
			if end >= len(lines) {
				end = len(lines) - 1
			}
			for j := start; j <= end; j++ {
				sep := ":"
				if j != i {
					sep = "-"
				}
				fmt.Fprintf(w, "%s:%d%s%s\n", f, lineNo+(j-i), sep, lines[j])
			}
		}
	}
	return nil
}

// isBinary returns true if data looks like a binary file.
func isBinary(data []byte) bool {
	const sampleSize = 8000
	sample := data
	if len(sample) > sampleSize {
		sample = sample[:sampleSize]
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return false
}
