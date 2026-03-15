package ip

import (
	"fmt"
	"net"

	cc "github.com/jadogg/babi/internal/clicolor"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var ipAllFlag bool

	cmd := &cobra.Command{
		Use:   "ip",
		Short: "Show local IP address for the internet-facing interface",
		Long: `Show the local IP of the network interface used for internet access.
Use --all to also list every non-loopback interface.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := net.Dial("udp", "8.8.8.8:80")
			if err == nil {
				defer conn.Close()
				primary := conn.LocalAddr().(*net.UDPAddr).IP.String()

				ifaceName := ""
				if ifaces, e := net.Interfaces(); e == nil {
				outer:
					for _, iface := range ifaces {
						addrs, _ := iface.Addrs()
						for _, addr := range addrs {
							var ip net.IP
							switch v := addr.(type) {
							case *net.IPNet:
								ip = v.IP
							case *net.IPAddr:
								ip = v.IP
							}
							if ip != nil && ip.String() == primary {
								ifaceName = iface.Name
								break outer
							}
						}
					}
				}

				label := "IP"
				if ifaceName != "" {
					label = fmt.Sprintf("IP (%s)", ifaceName)
				}
				fmt.Printf("%-24s %s\n", cc.Dim(label+":"), cc.BoldGreen(primary))
			} else {
				fmt.Printf("%s could not determine internet-facing interface\n", cc.BoldYellow("Warning:"))
			}

			if !ipAllFlag {
				return nil
			}

			fmt.Println()
			fmt.Println(cc.Bold("All interfaces:"))
			ifaces, err := net.Interfaces()
			if err != nil {
				return err
			}
			for _, iface := range ifaces {
				if iface.Flags&net.FlagLoopback != 0 {
					continue
				}
				addrs, _ := iface.Addrs()
				for _, addr := range addrs {
					var ip net.IP
					var mask net.IPMask
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
						mask = v.Mask
					case *net.IPAddr:
						ip = v.IP
					}
					if ip == nil || ip.IsLoopback() {
						continue
					}
					ipStr := ip.String()
					if mask != nil {
						ones, _ := mask.Size()
						ipStr = fmt.Sprintf("%s/%d", ipStr, ones)
					}
					fmt.Printf("  %-14s %s\n", cc.Dim(iface.Name), cc.Cyan(ipStr))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&ipAllFlag, "all", "a", false, "list all non-loopback interfaces")
	return cmd
}
