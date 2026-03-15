package wc

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var showLines, showWords, showChars bool
	c := &cobra.Command{
		Use:   "wc <file...>",
		Short: "Count lines, words, and characters in files",
		Long: `Count lines, words, and characters in files.

  babi wc main.go           # lines  words  chars  filename
  babi wc -l main.go        # lines only
  babi wc -w main.go        # words only
  babi wc -c main.go        # chars only
  babi wc file1 file2       # multiple files + total`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(args, showLines, showWords, showChars)
		},
	}
	c.Flags().BoolVarP(&showLines, "lines", "l", false, "print line count only")
	c.Flags().BoolVarP(&showWords, "words", "w", false, "print word count only")
	c.Flags().BoolVarP(&showChars, "chars", "c", false, "print character count only")
	return c
}

type counts struct{ lines, words, chars int }

func countFile(path string) (counts, error) {
	f, err := os.Open(path)
	if err != nil {
		return counts{}, err
	}
	defer f.Close()

	var c counts
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		c.lines++
		c.words += len(strings.Fields(line))
		c.chars += len([]rune(line)) + 1 // +1 for newline
	}
	return c, scanner.Err()
}

func run(args []string, lines, words, chars bool) error {
	if !lines && !words && !chars {
		lines, words, chars = true, true, true
	}

	var totals counts
	for _, path := range args {
		c, err := countFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wc: %s: %v\n", path, err)
			continue
		}
		totals.lines += c.lines
		totals.words += c.words
		totals.chars += c.chars
		printRow(c, path, lines, words, chars)
	}
	if len(args) > 1 {
		printRow(totals, "total", lines, words, chars)
	}
	return nil
}

func printRow(c counts, label string, lines, words, chars bool) {
	var parts []string
	if lines {
		parts = append(parts, fmt.Sprintf("%7d", c.lines))
	}
	if words {
		parts = append(parts, fmt.Sprintf("%7d", c.words))
	}
	if chars {
		parts = append(parts, fmt.Sprintf("%7d", c.chars))
	}
	fmt.Printf("%s  %s\n", strings.Join(parts, ""), label)
}
