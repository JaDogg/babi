package dt

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

const dtFmt = "2006-01-02 15:04:05 MST"
const dtFmtUTC = "2006-01-02 15:04:05"

func Command() *cobra.Command {
	dtCmd := &cobra.Command{
		Use:   "dt",
		Short: "Date and time utilities",
		Long:  "Date/time calculations, age, timezone conversion, and NTP sync.",
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			fmt.Printf("%s %s\n", cc.Dim("Now: "), cc.BrightWhite(now.Format(dtFmt)))
			fmt.Printf("%s %s\n", cc.Dim("UTC: "), cc.Dim(now.UTC().Format(dtFmtUTC)))
			fmt.Printf("%s %s\n", cc.Dim("Unix:"), cc.Yellow(strconv.FormatInt(now.Unix(), 10)))
			fmt.Println()

			offsets := []string{"+1h", "+1d", "+1w", "+1mo", "+1y", "-1h", "-1d", "-1w", "-1mo", "-1y"}
			fmt.Println(cc.Bold("Offsets from now:"))
			for _, s := range offsets {
				d, _ := ParseCalDuration(s)
				t := d.AddTo(now)
				timeStr := t.Format(dtFmt)
				if d.Neg {
					timeStr = cc.Dim(timeStr)
				}
				fmt.Printf("  %s  %s\n", cc.Cyan(fmt.Sprintf("%-6s", s)), timeStr)
			}
			return nil
		},
	}

	dtInCmd := &cobra.Command{
		Use:                "in <duration> [--from <date>]",
		Short:              "Time after/before a duration from now (or a given date)",
		Example:            "  babi dt in 1h\n  babi dt in -2d\n  babi dt in 1y2mo3d\n  babi dt in 5w2d --from 03/02/2026",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}

			var durArg, fromStr string
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--from", "-from":
					if i+1 >= len(args) {
						return fmt.Errorf("--from requires a date argument")
					}
					fromStr = args[i+1]
					i++
				default:
					if durArg != "" {
						return fmt.Errorf("unexpected argument %q", args[i])
					}
					durArg = args[i]
				}
			}
			if durArg == "" {
				return fmt.Errorf("expected a duration argument (e.g. 1h, -2d, 1mo)")
			}

			d, err := ParseCalDuration(durArg)
			if err != nil {
				return err
			}

			base := time.Now()
			fromLabel := "Now:   "
			if fromStr != "" {
				base, err = ParseDate(fromStr)
				if err != nil {
					return fmt.Errorf("--from: %w", err)
				}
				fromLabel = "From:  "
			}
			result := d.AddTo(base)

			var label, resultStr string
			if d.Neg {
				label = cc.BoldYellow("Before " + d.String()[1:] + ":")
				resultStr = cc.BoldYellow(result.Format(dtFmt))
			} else {
				label = cc.BoldGreen("After " + d.String() + ":")
				resultStr = cc.BoldGreen(result.Format(dtFmt))
			}

			fmt.Printf("%s %s\n", cc.Dim(fromLabel), cc.BrightWhite(base.Format(dtFmt)))
			fmt.Printf("%-30s %s\n", label, resultStr)
			fmt.Printf("%s %s\n", cc.Dim("UTC:   "), cc.Dim(result.UTC().Format(dtFmtUTC)))
			fmt.Printf("%s %s\n", cc.Dim("Unix:  "), cc.Yellow(strconv.FormatInt(result.Unix(), 10)))
			return nil
		},
	}

	dtAgeCmd := &cobra.Command{
		Use:     "age <date>",
		Short:   "Calculate age from a birth date",
		Example: "  babi dt age 1990-01-15\n  babi dt age \"Jan 15, 1990\"",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			birth, err := ParseDate(args[0])
			if err != nil {
				return err
			}
			if birth.After(time.Now()) {
				return fmt.Errorf("date %s is in the future", args[0])
			}
			age := CalcAge(birth)
			fmt.Printf("%s %s\n",
				cc.Dim("Born:       "),
				cc.BrightWhite(age.Birth.Format("2006-01-02")))
			fmt.Printf("%s %s\n",
				cc.Dim("Age:        "),
				cc.BoldGreen(fmt.Sprintf("%d years, %d months, %d days", age.Years, age.Months, age.Days)))
			fmt.Printf("%s %s\n",
				cc.Dim("Total days: "),
				cc.Cyan(formatInt(age.TotalDays)))
			fmt.Printf("%s %s\n",
				cc.Dim("Total hours:"),
				cc.Cyan(formatInt(age.TotalHours)))
			return nil
		},
	}

	dtTZCmd := &cobra.Command{
		Use:     "tz <timezone> [time]",
		Short:   "Show current time (or a given time) in a timezone",
		Example: "  babi dt tz America/New_York\n  babi dt tz UTC\n  babi dt tz Europe/London \"2024-06-01 12:00:00\"",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			loc, err := time.LoadLocation(args[0])
			if err != nil {
				return fmt.Errorf("unknown timezone %q: %w", args[0], err)
			}
			var base time.Time
			if len(args) == 2 {
				base, err = time.ParseInLocation("2006-01-02 15:04:05", args[1], time.Local)
				if err != nil {
					base, err = time.Parse(dtFmt, args[1])
					if err != nil {
						return fmt.Errorf("unrecognised time %q (use \"2006-01-02 15:04:05\")", args[1])
					}
				}
			} else {
				base = time.Now()
			}
			fmt.Printf("%s %s\n",
				cc.Dim(fmt.Sprintf("%-22s", fmt.Sprintf("Local (%s):", base.Location()))),
				cc.BrightWhite(base.Format(dtFmt)))
			fmt.Printf("%s %s\n",
				cc.BoldCyan(fmt.Sprintf("%-22s", args[0]+":")),
				cc.BrightWhite(base.In(loc).Format(dtFmt)))
			fmt.Printf("%s %s\n",
				cc.Dim(fmt.Sprintf("%-22s", "UTC:")),
				cc.Dim(base.UTC().Format(dtFmtUTC)))
			return nil
		},
	}

	dtNTPCmd := &cobra.Command{
		Use:   "ntp",
		Short: "Query NTP servers and report clock offset",
		Long:  "Queries a random NTP server and reports clock offset.\nUse --sync to apply the correction (requires root/sudo).",
		RunE: func(cmd *cobra.Command, args []string) error {
			doSync, _ := cmd.Flags().GetBool("sync")

			r, err := queryNTPWithRetry()
			if err != nil {
				return err
			}

			if doSync {
				fmt.Printf("%s %s\n", cc.Dim("Syncing with"), cc.Cyan(r.Server))
				ntpBin, ntpArgs := ntpSyncCmdServer(r.Server)
				if ntpBin == "" {
					fmt.Printf("%s no suitable NTP sync tool found (install ntpdate or sntp).\n",
						cc.BoldYellow("Sync:"))
					return nil
				}
				var c *exec.Cmd
				if runtime.GOOS == "windows" {
					fmt.Printf("%s %s %s\n",
						cc.Dim("Running:"), ntpBin, strings.Join(ntpArgs, " "))
					c = exec.Command(ntpBin, ntpArgs...)
				} else {
					fmt.Printf("%s sudo %s %s\n",
						cc.Dim("Running:"), ntpBin, strings.Join(ntpArgs, " "))
					syncArgs := append([]string{ntpBin}, ntpArgs...)
					c = exec.Command("sudo", syncArgs...)
				}
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}
			off := r.Offset
			sign := "+"
			if off < 0 {
				sign = "-"
				off = -off
			}
			offsetStr := sign + off.Round(time.Microsecond).String()
			fmt.Printf("  %-22s  %s  %s %s  %s %s\n",
				cc.Cyan(r.Server),
				r.Time.Format(dtFmtUTC+" UTC"),
				cc.Dim("offset:"), ntpOffsetColor(r.Offset, offsetStr),
				cc.Dim("rtt:"), cc.Dim(r.RTT.Round(time.Millisecond).String()),
			)

			absOff := r.Offset
			if absOff < 0 {
				absOff = -absOff
			}
			if absOff > time.Second {
				hint := "'babi dt ntp --sync'"
				if runtime.GOOS == "windows" {
					hint = "'babi dt ntp --sync' (requires admin)"
				}
				fmt.Printf("\n%s clock is off by more than 1s — run %s to correct it.\n",
					cc.BoldYellow("Hint:"), cc.Cyan(hint))
			}
			return nil
		},
	}
	dtNTPCmd.Flags().Bool("sync", false, "apply NTP correction to system clock (requires sudo)")

	dtCmd.AddCommand(dtInCmd, dtAgeCmd, dtTZCmd, dtNTPCmd)
	return dtCmd
}

// queryNTPWithRetry picks a random server and retries with a different one on failure,
// trying each server at most once.
func queryNTPWithRetry() (NTPResult, error) {
	//nolint:gosec
	perm := rand.Perm(len(DefaultNTPServers))
	for _, i := range perm {
		server := DefaultNTPServers[i]
		r := QueryNTP(server)
		if r.Err == nil {
			return r, nil
		}
		fmt.Printf("%s %s failed (%v), retrying...\n", cc.Dim("NTP:"), cc.Cyan(server), r.Err)
	}
	return NTPResult{}, fmt.Errorf("all NTP servers failed")
}

func ntpOffsetColor(offset time.Duration, s string) string {
	abs := offset
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= time.Second:
		return cc.BoldRed(s)
	case abs >= 100*time.Millisecond:
		return cc.BoldYellow(s)
	default:
		return cc.BoldGreen(s)
	}
}

func ntpSyncCmdServer(server string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "w32tm", []string{"/resync", "/force"}
	}
	if _, err := exec.LookPath("sntp"); err == nil {
		return "sntp", []string{"-sS", server}
	}
	if _, err := exec.LookPath("ntpdate"); err == nil {
		return "ntpdate", []string{server}
	}
	return "", nil
}

func formatInt(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
