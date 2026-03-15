package sep

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "sep [char] [width]",
		Short: "Print a separator line",
		Long: `Print a horizontal separator line to stdout.

  babi sep          # ────────────────────── (terminal width, box-drawing char)
  babi sep =        # ══════════════════════
  babi sep - 20     # --------------------`,
		Args: cobra.MaximumNArgs(2),
		RunE: run,
	}
}

func run(cmd *cobra.Command, args []string) error {
	char := "─"
	width := 0

	if len(args) >= 1 {
		char = args[0]
	}
	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("width must be an integer: %w", err)
		}
		width = n
	}

	if width <= 0 {
		w, _, err := term.GetSize(os.Stdout.Fd())
		if err != nil || w <= 0 {
			width = 80
		} else {
			width = w
		}
	}

	if len([]rune(char)) == 0 {
		char = "─"
	}

	fmt.Println(strings.Repeat(char, width))
	return nil
}
