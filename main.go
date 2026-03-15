package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/jadogg/babi/internal/cf"
	"github.com/jadogg/babi/internal/check"
	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/jadogg/babi/internal/convert"
	"github.com/jadogg/babi/internal/dt"
	"github.com/jadogg/babi/internal/ed"
	"github.com/jadogg/babi/internal/encode"
	"github.com/jadogg/babi/internal/gen"
	"github.com/jadogg/babi/internal/hash"
	"github.com/jadogg/babi/internal/ip"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jadogg/babi/internal/meta"
	"github.com/jadogg/babi/internal/newproject"
	"github.com/jadogg/babi/internal/pack"
	"github.com/jadogg/babi/internal/port"
	"github.com/jadogg/babi/internal/serve"
	synccmd "github.com/jadogg/babi/internal/sync"
	"github.com/jadogg/babi/internal/tag"
	"github.com/jadogg/babi/internal/tree"
	"github.com/jadogg/babi/internal/tui"
	"github.com/jadogg/babi/internal/tui/editor"
	"github.com/jadogg/babi/internal/tui/fm"
	gitui "github.com/jadogg/babi/internal/tui/git"
	tuihex "github.com/jadogg/babi/internal/tui/hex"
	"github.com/jadogg/babi/internal/tui/typer"
)

var version = "dev" // set by -ldflags "-X main.version=vX.Y.Z" at build time

var rootCmd = &cobra.Command{
	Use:     "babi",
	Short:   "babi — file sync & git TUI",
	Long:    "babi: file sync, commitizen commits, text editor, hex editor, and file manager.",
	Version: version,
}

func initCobraColors() {
	cobra.AddTemplateFunc("cH", func(s string) string { return cc.BoldCyan(s) })
	cobra.AddTemplateFunc("cCmd", func(s string) string { return cc.BrightGreen(s) })
	cobra.AddTemplateFunc("cDim", func(s string) string { return cc.Dim(s) })

	const usageTmpl = `{{cH "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{cH "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{cH "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{cH "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cCmd (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{cH "Flags:"}}
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasAvailableInheritedFlags}}

{{cH "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

{{cH "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

{{cDim (printf "Use \"%s [command] --help\" for more information about a command." .CommandPath)}}{{end}}
`
	rootCmd.SetUsageTemplate(usageTmpl)
}

func main() {
	rootCmd.AddCommand(
		synccmd.Command(func() tea.Model { return tui.NewAppModel() }),
		gitui.CommitCommand(),
		ed.SearchCommand(),
		ed.ReplaceCommand(),
		editor.Command(),
		tuihex.Command(),
		fm.Command(),
		dt.Command(),
		convert.Command(),
		convert.PDFCommand(),
		hash.Command(),
		encode.Command(),
		gen.Command(),
		port.Command(),
		ip.Command(),
		gitui.LogCommand(),
		gitui.StashCommand(),
		meta.Command(),
		cf.Command(),
		tree.Command(),
		tag.Command(),
		check.Command(),
		serve.Command(),
		pack.PackCommand(),
		pack.UnpackCommand(),
		typer.Command(),
		newproject.Command(),
	)
	initCobraColors()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
