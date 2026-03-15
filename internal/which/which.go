package which

import (
	"fmt"
	"os/exec"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "which <cmd...>",
		Short: "Locate a command on PATH",
		Long: `Find the full path of a command on PATH.

  babi which babi
  babi which git rg python3`,
		Args: cobra.MinimumNArgs(1),
		RunE: run,
	}
}

func run(cmd *cobra.Command, args []string) error {
	anyMissing := false
	for _, name := range args {
		path, err := exec.LookPath(name)
		if err != nil {
			fmt.Printf("%s  %s\n", cc.BoldRed("✗"), name)
			anyMissing = true
		} else {
			fmt.Printf("%s  %s\n", cc.BoldGreen("✓"), path)
		}
	}
	if anyMissing {
		return fmt.Errorf("one or more commands not found")
	}
	return nil
}
