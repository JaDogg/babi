package check

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	cc "github.com/jadogg/babi/internal/clicolor"
)

type depEntry struct {
	bin      string
	commands string // babi commands that use it
	brew     string // macOS install hint
	apt      string // Debian/Ubuntu install hint
	pacman   string // Arch install hint
	winget   string // Windows install hint
	note     string // shown when available instead of install hint
	required bool   // required vs optional
}

// depGroup is a named set of dependencies where finding ANY one satisfies the group.
type depGroup struct {
	name    string
	anyOne  bool // if true, only one entry needs to be present
	entries []depEntry
}

var deps = []depGroup{
	{
		name: "VERSION CONTROL",
		entries: []depEntry{
			{
				bin:      "git",
				commands: "babi commit, log, stash",
				note:     "install via your OS package manager",
				winget:   "winget install Git.Git",
				required: true,
			},
		},
	},
	{
		name: "SEARCH",
		entries: []depEntry{
			{
				bin:      "rg",
				commands: "babi search, replace  (grep used as fallback)",
				brew:     "brew install ripgrep",
				apt:      "apt install ripgrep",
				pacman:   "pacman -S ripgrep",
				winget:   "winget install BurntSushi.ripgrep.MSVC",
			},
		},
	},
	{
		name: "MEDIA CONVERSION",
		entries: []depEntry{
			{
				bin:      "ffmpeg",
				commands: "babi convert  (video, audio, gif, frames)",
				brew:     "brew install ffmpeg",
				apt:      "apt install ffmpeg",
				pacman:   "pacman -S ffmpeg",
				winget:   "winget install Gyan.FFmpeg",
			},
			{
				bin:      "magick",
				commands: "babi convert, meta ico, meta icns  (ImageMagick v7)",
				brew:     "brew install imagemagick",
				apt:      "apt install imagemagick",
				pacman:   "pacman -S imagemagick",
				winget:   "winget install ImageMagick.ImageMagick",
				note:     "ImageMagick v7+",
			},
		},
	},
	{
		name: "DOCUMENT CONVERSION",
		entries: []depEntry{
			{
				bin:      "pandoc",
				commands: "babi convert  (md, html, pdf, docx, epub …)",
				brew:     "brew install pandoc",
				apt:      "apt install pandoc",
				pacman:   "pacman -S pandoc",
				winget:   "winget install JohnMacFarlane.Pandoc",
			},
		},
	},
	{
		name:   "NTP TIME SYNC",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "sntp",
				commands: "babi dt ntp --sync",
				note:     "usually bundled with macOS / ntp package",
				brew:     "brew install ntp",
				apt:      "apt install ntp",
				pacman:   "pacman -S ntp",
				winget:   "use w32tm /resync (built-in)",
			},
			{
				bin:      "ntpdate",
				commands: "babi dt ntp --sync  (fallback)",
				brew:     "brew install ntp",
				apt:      "apt install ntpdate",
				pacman:   "pacman -S ntp",
				winget:   "use w32tm /resync (built-in)",
			},
		},
	},
	{
		name:   "PORT INSPECTION",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "lsof",
				commands: "babi port  (macOS / Linux primary)",
				note:     "usually pre-installed",
				brew:     "brew install lsof",
				apt:      "apt install lsof",
				pacman:   "pacman -S lsof",
				winget:   "use netstat -ano (built-in)",
			},
			{
				bin:      "ss",
				commands: "babi port  (Linux fallback)",
				note:     "part of iproute2",
				apt:      "apt install iproute2",
				pacman:   "pacman -S iproute2",
				winget:   "not available on Windows",
			},
		},
	},
	{
		name: "ARCHIVING",
		entries: []depEntry{
			{
				bin:      "tar",
				commands: "babi pack/unpack  (.tar.bz2 pack, .tar.xz, .tar.lzma, .tar.zst)",
				note:     "usually pre-installed  (Windows: .lzma/.zst fall back to 7z)",
				brew:     "brew install gnu-tar",
				apt:      "apt install tar",
				pacman:   "pacman -S tar",
				winget:   "built-in (Windows 10+)",
			},
		},
	},
	{
		name:   "7-ZIP",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "7z",
				commands: "babi pack/unpack  (.7z; Windows: also .tar.lzma/.tar.zst)",
				brew:     "brew install p7zip",
				apt:      "apt install p7zip-full",
				pacman:   "pacman -S p7zip",
				winget:   "winget install 7zip.7zip",
			},
			{
				bin:      "7za",
				commands: "babi pack/unpack  (.7z archives, fallback)",
				brew:     "brew install p7zip",
				apt:      "apt install p7zip",
				pacman:   "pacman -S p7zip",
				winget:   "winget install 7zip.7zip",
			},
			{
				bin:      "7zz",
				commands: "babi pack/unpack  (.7z archives, fallback)",
				brew:     "brew install sevenzip",
				apt:      "apt install 7zip",
				pacman:   "pacman -S 7zip",
				winget:   "winget install 7zip.7zip",
			},
		},
	},
	{
		name:   "ICON GENERATION",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "iconutil",
				commands: "babi meta icns  (macOS preferred)",
				note:     "pre-installed on macOS",
				brew:     "pre-installed on macOS (Xcode Command Line Tools)",
				apt:      "not available on Linux — ImageMagick used instead",
				pacman:   "not available on Linux — ImageMagick used instead",
				winget:   "not available on Windows — ImageMagick used instead",
			},
		},
	},
	{
		name:   "FILE OPEN (desktop)",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "open",
				commands: "babi fm  (macOS open file/dir)",
				note:     "pre-installed on macOS",
				brew:     "pre-installed on macOS",
				apt:      "not available — use xdg-open",
				pacman:   "not available — use xdg-open",
				winget:   "not available — use rundll32 (built-in)",
			},
			{
				bin:      "xdg-open",
				commands: "babi fm  (Linux open file/dir)",
				note:     "part of xdg-utils",
				brew:     "not available on macOS — use open",
				apt:      "apt install xdg-utils",
				pacman:   "pacman -S xdg-utils",
				winget:   "not available — use rundll32 (built-in)",
			},
		},
	},
	{
		name:   "PYTHON",
		anyOne: true,
		entries: []depEntry{
			{
				bin:      "python3",
				commands: "babi cf",
				brew:     "brew install python",
				apt:      "apt install python3",
				pacman:   "pacman -S python",
				winget:   "winget install Python.Python.3",
			},
			{
				bin:      "python",
				commands: "babi cf  (fallback)",
				brew:     "brew install python",
				apt:      "apt install python3",
				pacman:   "pacman -S python",
				winget:   "winget install Python.Python.3",
			},
			{
				bin:      "py",
				commands: "babi cf  (Windows launcher fallback)",
				note:     "Windows Python Launcher",
				winget:   "winget install Python.Python.3",
			},
		},
	},
}

func installHint(e depEntry) string {
	switch runtime.GOOS {
	case "darwin":
		if e.brew != "" {
			return e.brew
		}
	case "linux":
		if e.apt != "" {
			return e.apt
		}
	case "windows":
		if e.winget != "" {
			return e.winget
		}
	}
	if e.brew != "" {
		return e.brew
	}
	if e.apt != "" {
		return e.apt
	}
	return "see project docs"
}

// Command returns the cobra command for "babi check".
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check for required third-party binaries",
		Long:  "Checks which optional and required external tools are installed that babi uses.",
		RunE:  run,
	}
}

func run(cmd *cobra.Command, args []string) error {
	type result struct {
		entry depEntry
		path  string
		found bool
	}

	const binW = 10
	const cmdW = 48

	pad := func(s string, w int) string {
		r := []rune(s)
		if len(r) >= w {
			return string(r[:w])
		}
		return s + strings.Repeat(" ", w-len(r))
	}

	totalOptional := 0
	foundOptional := 0

	fmt.Println()
	fmt.Println(cc.Bold("babi check") + cc.Dim(" — third-party dependency status"))
	fmt.Println()

	for _, group := range deps {
		fmt.Println(cc.Dim("  " + group.name))

		groupResults := make([]result, len(group.entries))
		anyFound := false
		for i, e := range group.entries {
			p, err := exec.LookPath(e.bin)
			found := err == nil
			groupResults[i] = result{entry: e, path: p, found: found}
			if found {
				anyFound = true
			}
			if !e.required {
				totalOptional++
				if found {
					foundOptional++
				}
			}
		}

		for _, r := range groupResults {
			dimmed := group.anyOne && anyFound && !r.found

			binStr := pad(r.entry.bin, binW)
			cmdStr := pad(r.entry.commands, cmdW)

			var status, detail string
			if r.found {
				status = cc.BoldGreen("✓")
				if r.entry.note != "" {
					detail = cc.Dim(r.entry.note)
				} else {
					detail = cc.Dim(r.path)
				}
				fmt.Printf("  %s  %s  %s  %s\n", status, cc.Bold(binStr), cc.Dim(cmdStr), detail)
			} else if dimmed {
				status = cc.Dim("–")
				detail = cc.Dim(installHint(r.entry))
				fmt.Printf("  %s  %s  %s  %s\n", status, cc.Dim(binStr), cc.Dim(cmdStr), detail)
			} else {
				tag := ""
				if r.entry.required {
					tag = cc.BoldRed(" [required]")
				}
				status = cc.BoldRed("✗")
				detail = cc.Yellow(installHint(r.entry))
				fmt.Printf("  %s  %s  %s  %s%s\n", status, cc.Bold(binStr), cc.Dim(cmdStr), detail, tag)
			}
		}
		fmt.Println()
	}

	if totalOptional > 0 {
		summary := fmt.Sprintf("%d / %d optional dependencies available", foundOptional, totalOptional)
		if foundOptional == totalOptional {
			fmt.Println(cc.BoldGreen("  ✓ " + summary))
		} else {
			fmt.Println(cc.Dim("  " + summary))
		}
		fmt.Println()
	}

	return nil
}
