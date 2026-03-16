package cmd

import (
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
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

		servers["localhost.pod"] = "127.0.0.1"

		type probeResult struct {
			Label   string
			Host    string
			Latency time.Duration
			Err     error
		}

		results := make([]probeResult, 0, len(servers))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for label, host := range servers {
			wg.Add(1)
			go func(label, host string) {
				defer wg.Done()
				addr := net.JoinHostPort(host, "22")
				start := time.Now()
				conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
				latency := time.Since(start)
				if err == nil {
					_ = conn.Close()
				}
				mu.Lock()
				results = append(results, probeResult{label, host, latency, err})
				mu.Unlock()
			}(label, host)
		}
		wg.Wait()

		sort.Slice(results, func(i, j int) bool {
			return results[i].Label < results[j].Label
		})

		for _, r := range results {
			if r.Err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "  %-25s %s  unreachable\n", r.Label, r.Host)
			} else {
				_, _ = fmt.Fprintf(os.Stdout, "  %-25s %s  ok (%dms)\n", r.Label, r.Host, r.Latency.Milliseconds())
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serversCmd)
}
