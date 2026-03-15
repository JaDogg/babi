package port

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

type procInfo struct {
	pid  string
	name string
}

func Command() *cobra.Command {
	var portKill bool

	portCmd := &cobra.Command{
		Use:   "port <number>",
		Short: "Show what process is using a port",
		Long: `Find which process is listening on a port, with option to kill it.

  babi port 3000             # show process on port 3000
  babi port 3000 --kill      # kill the process on port 3000
  babi port list             # list all listening ports`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := args[0]
			if _, err := strconv.Atoi(p); err != nil {
				return fmt.Errorf("invalid port number: %s", p)
			}
			procs, err := portProcs(p)
			if err != nil {
				return err
			}
			if len(procs) == 0 {
				fmt.Printf("nothing is listening on port %s\n", cc.BoldCyan(p))
				return nil
			}
			for _, proc := range procs {
				fmt.Printf("port %s  pid %s  %s\n",
					cc.BoldCyan(p), cc.BoldYellow(proc.pid), proc.name)
			}
			if portKill {
				for _, proc := range procs {
					var killCmd *exec.Cmd
					if runtime.GOOS == "windows" {
						killCmd = exec.Command("taskkill", "/F", "/PID", proc.pid)
					} else {
						killCmd = exec.Command("kill", "-9", proc.pid)
					}
					if err := killCmd.Run(); err != nil {
						fmt.Printf("%s killing pid %s: %v\n", cc.BoldRed("error"), proc.pid, err)
					} else {
						fmt.Printf("%s killed pid %s (%s)\n", cc.BoldGreen("killed"), proc.pid, proc.name)
					}
				}
			}
			return nil
		},
	}
	portCmd.Flags().BoolVar(&portKill, "kill", false, "kill the process on the port")

	listCmd := &cobra.Command{
		Use: "list", Short: "List all listening ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS == "windows" {
				out, err := exec.Command("netstat", "-ano").Output()
				if err != nil {
					return fmt.Errorf("netstat not available: %w", err)
				}
				fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
					cc.BoldCyan("Proto"), cc.BoldCyan("Local Address"),
					cc.BoldCyan("Foreign Address"), cc.BoldCyan("State"), cc.BoldCyan("PID"))
				for _, line := range strings.Split(string(out), "\n") {
					fields := strings.Fields(line)
					if len(fields) == 5 && strings.EqualFold(fields[3], "LISTENING") {
						fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
							fields[0], fields[1], fields[2], fields[3], fields[4])
					} else if len(fields) == 4 && strings.EqualFold(fields[0], "UDP") {
						fmt.Printf("%-5s %-45s %-45s %-12s %s\n",
							fields[0], fields[1], fields[2], "", fields[3])
					}
				}
				return nil
			}
			out, err := exec.Command("lsof", "-iTCP", "-iUDP", "-n", "-P", "-sTCP:LISTEN").Output()
			if err != nil {
				out, err = exec.Command("ss", "-tlnup").Output()
				if err != nil {
					return fmt.Errorf("lsof and ss not available: %w", err)
				}
				fmt.Print(string(out))
				return nil
			}
			lines := strings.Split(string(out), "\n")
			if len(lines) > 0 {
				fmt.Println(cc.BoldCyan(lines[0]))
			}
			for _, l := range lines[1:] {
				if l != "" {
					fmt.Println(l)
				}
			}
			return nil
		},
	}

	portCmd.AddCommand(listCmd)
	return portCmd
}

func portProcs(p string) ([]procInfo, error) {
	if runtime.GOOS == "windows" {
		return portProcsWindows(p)
	}
	out, err := exec.Command("lsof", "-i:"+p, "-n", "-P", "-F", "cpn").Output()
	if err != nil && len(out) == 0 {
		return nil, nil
	}
	var procs []procInfo
	var cur procInfo
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			cur.pid = line[1:]
		case 'c':
			cur.name = line[1:]
		case 'n':
			if cur.pid != "" {
				procs = append(procs, cur)
				cur = procInfo{}
			}
		}
	}
	seen := map[string]bool{}
	var unique []procInfo
	for _, proc := range procs {
		if !seen[proc.pid] {
			seen[proc.pid] = true
			unique = append(unique, proc)
		}
	}
	return unique, nil
}

func portProcsWindows(p string) ([]procInfo, error) {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var procs []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && strings.EqualFold(fields[3], "LISTENING") {
			if strings.HasSuffix(fields[1], ":"+p) {
				pid := strings.TrimSpace(fields[4])
				if !seen[pid] {
					seen[pid] = true
					procs = append(procs, procInfo{pid: pid, name: winProcName(pid)})
				}
			}
		}
		if len(fields) == 4 && strings.EqualFold(fields[0], "UDP") {
			if strings.HasSuffix(fields[1], ":"+p) {
				pid := strings.TrimSpace(fields[3])
				if !seen[pid] {
					seen[pid] = true
					procs = append(procs, procInfo{pid: pid, name: winProcName(pid)})
				}
			}
		}
	}
	return procs, nil
}

func winProcName(pid string) string {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+pid, "/FO", "CSV", "/NH").Output()
	if err != nil {
		return "unknown"
	}
	line := strings.TrimSpace(string(out))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return "unknown"
	}
	parts := strings.SplitN(line, ",", 2)
	return strings.Trim(parts[0], "\"")
}
