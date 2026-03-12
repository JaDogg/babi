package cf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// Command returns the "babi cf" cobra command.
func Command() *cobra.Command {
	c := &cobra.Command{
		Use:   "cf <delimiter> <format> [file]",
		Short: "Cut and format text fields from each line",
		Long: `Split each line of input on a delimiter and format the result.
Reads from [file] or stdin. Requires Python (python3, python, or py).

  echo "alice 30" | babi cf ' ' '{part[0]} is {part[1]} years old'
  babi cf ',' '{part[0]}: {part[1]}' data.csv
  babi cf --maxsplit=1 ':' '[{part[0]}] {part[1]}' /etc/passwd
  babi cf -E '\s+' '{part[0]}/{part[1]}'
  babi cf --setup='total=0' --foreach='total+=float(part[0])' ',' '{total:.2f}'

Variables available in format/setup/foreach:
  n       line index (0-based, from enumerate)
  line    raw line (trailing newline stripped)
  part    list of split fields`,
		Args: cobra.RangeArgs(2, 3),
		RunE: run,
	}
	c.Flags().IntP("maxsplit", "m", 0, "max splits per line (0 = unlimited)")
	c.Flags().BoolP("extended", "E", false, "use re.split (regex) instead of str.split")
	c.Flags().StringP("setup", "s", "", "Python code to run once before the loop")
	c.Flags().StringP("foreach", "f", "", "Python code to run inside the loop, before formatting")
	c.Flags().Bool("debug", false, "print the generated Python script to stderr instead of running it")
	return c
}

func run(cmd *cobra.Command, args []string) error {
	delim := args[0]
	format := args[1]

	maxsplit, _ := cmd.Flags().GetInt("maxsplit")
	extended, _ := cmd.Flags().GetBool("extended")
	setup, _ := cmd.Flags().GetString("setup")
	foreach, _ := cmd.Flags().GetString("foreach")
	debug, _ := cmd.Flags().GetBool("debug")

	script := buildScript(delim, format, maxsplit, extended, setup, foreach)

	if debug {
		fmt.Fprintf(os.Stderr, "# generated Python script\n%s\n", script)
		return nil
	}

	python, err := FindPython()
	if err != nil {
		return err
	}

	// Write temp file.
	tmp, err := os.CreateTemp("", "babi_cf_*.py")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	// Open input source.
	var input *os.File
	if len(args) == 3 {
		input, err = os.Open(args[2])
		if err != nil {
			return fmt.Errorf("open %s: %w", args[2], err)
		}
		defer input.Close()
	} else {
		input = os.Stdin
	}

	c := exec.Command(python, tmpPath)
	c.Stdin = input
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// buildScript generates the Python script that will be executed.
func buildScript(delim, format string, maxsplit int, extended bool, setup, foreach string) string {
	var sb strings.Builder

	sb.WriteString("import sys\n")
	if extended {
		sb.WriteString("import re\n")
	}
	sb.WriteString("\n")

	if setup != "" {
		sb.WriteString(setup)
		if !strings.HasSuffix(setup, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("for n, line in enumerate(sys.stdin):\n")
	sb.WriteString("    line = line.rstrip('\\n')\n")

	if foreach != "" {
		for _, l := range strings.Split(foreach, "\n") {
			sb.WriteString("    ")
			sb.WriteString(l)
			sb.WriteString("\n")
		}
	}

	// Delimiter as a Python triple-double-quoted string.
	// Only """ itself needs escaping inside this string type.
	safeDelim := strings.ReplaceAll(delim, `"""`, `\"\"\"`)
	if extended {
		if maxsplit <= 0 {
			fmt.Fprintf(&sb, "    part = re.split(\"\"\"%s\"\"\", line)\n", safeDelim)
		} else {
			fmt.Fprintf(&sb, "    part = re.split(\"\"\"%s\"\"\", line, maxsplit=%d)\n", safeDelim, maxsplit)
		}
	} else {
		if maxsplit <= 0 {
			fmt.Fprintf(&sb, "    part = line.split(\"\"\"%s\"\"\")\n", safeDelim)
		} else {
			fmt.Fprintf(&sb, "    part = line.split(\"\"\"%s\"\"\", %d)\n", safeDelim, maxsplit)
		}
	}

	// Format string as a Python f-string with triple-double-quote delimiters.
	safeFormat := strings.ReplaceAll(format, `"""`, `\"\"\"`)
	fmt.Fprintf(&sb, "    print(f\"\"\"%s\"\"\")\n", safeFormat)

	return sb.String()
}

// FindPython returns the first Python interpreter found on PATH.
// Exported so babi check can discover what cf would use.
func FindPython() (string, error) {
	for _, bin := range []string{"python3", "python", "py"} {
		if _, err := exec.LookPath(bin); err == nil {
			return bin, nil
		}
	}
	return "", fmt.Errorf(
		"Python not found — install it:\n  macOS:  brew install python\n  Debian: apt install python3\n  Arch:   pacman -S python",
	)
}
