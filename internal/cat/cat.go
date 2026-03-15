package cat

import (
	"bufio"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var tail int
	c := &cobra.Command{
		Use:   "cat <file> [start [end]]",
		Short: "Print file contents, optionally a line range",
		Long: `Print file contents to stdout with optional line range (1-based, inclusive).

  babi cat main.go            # whole file
  babi cat main.go 64 240     # lines 64–240
  babi cat main.go 2          # single line 2
  babi cat main.go --tail 50  # last 50 lines`,
		Args: cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(args, tail)
		},
	}
	c.Flags().IntVar(&tail, "tail", 0, "print last N lines")
	return c
}

func run(args []string, tail int) error {
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	total := len(lines)

	if tail > 0 {
		start := total - tail
		if start < 0 {
			start = 0
		}
		for _, l := range lines[start:] {
			fmt.Println(l)
		}
		return nil
	}

	start, end := 1, total

	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("start must be an integer: %w", err)
		}
		start = n
		end = n // single line by default
	}
	if len(args) >= 3 {
		n, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("end must be an integer: %w", err)
		}
		end = n
	}

	if start < 1 {
		start = 1
	}
	if end > total {
		end = total
	}
	for i := start - 1; i < end && i < total; i++ {
		fmt.Println(lines[i])
	}
	return nil
}
