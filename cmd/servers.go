package cmd

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
)

var serversCmd = &cobra.Command{
	Use:   "servers",
	Short: "List configured servers with connectivity status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := clientConfigPath()
		if err != nil {
			return err
		}

		clientCfg, err := config.LoadClient(cfgPath)
		if err != nil {
			fmt.Println("No client config found. Set one up with:")
			fmt.Println("  podspawn config set servers.default yourserver.com")
			return nil
		}

		servers := make(map[string]string)
		if clientCfg.Servers.Default != "" {
			servers["(default)"] = clientCfg.Servers.Default
		}
		for host, server := range clientCfg.Servers.Mappings {
			servers[host] = server
		}

		// localhost.pod is always available
		servers["localhost.pod"] = "127.0.0.1"

		if len(servers) == 0 {
			fmt.Println("No servers configured.")
			return nil
		}

		for label, host := range servers {
			addr := net.JoinHostPort(host, "22")
			start := time.Now()
			conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
			latency := time.Since(start)

			if err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "  %-25s %s  unreachable\n", label, host)
			} else {
				_ = conn.Close()
				_, _ = fmt.Fprintf(os.Stdout, "  %-25s %s  ok (%dms)\n", label, host, latency.Milliseconds())
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serversCmd)
}
