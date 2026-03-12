// Package clicolor provides simple ANSI color helpers for CLI output.
// Colors are automatically disabled when stdout is not a terminal.
package clicolor

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
)

// Enabled is true when stdout is a real terminal.
var Enabled = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

func c(s, code string) string {
	if !Enabled {
		return s
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, s)
}

func Bold(s string) string        { return c(s, "1") }
func Dim(s string) string         { return c(s, "2") }
func Red(s string) string         { return c(s, "31") }
func Green(s string) string       { return c(s, "32") }
func Yellow(s string) string      { return c(s, "33") }
func Cyan(s string) string        { return c(s, "36") }
func BrightGreen(s string) string { return c(s, "92") }
func BrightCyan(s string) string  { return c(s, "96") }
func BrightWhite(s string) string { return c(s, "97") }
func BoldCyan(s string) string    { return c(s, "1;36") }
func BoldGreen(s string) string   { return c(s, "1;32") }
func BoldRed(s string) string     { return c(s, "1;31") }
func BoldYellow(s string) string  { return c(s, "1;33") }
