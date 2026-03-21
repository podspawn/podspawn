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

		type serverInfo struct {
			Host string
			Mode string
		}
		servers := make(map[string]serverInfo)
		if clientCfg.Servers.Default != "" {
			servers["(default)"] = serverInfo{Host: clientCfg.Servers.Default}
		}
		for host, entry := range clientCfg.Servers.Mappings {
			servers[host] = serverInfo{Host: entry.Host, Mode: entry.Mode}
		}

		servers["localhost.pod"] = serverInfo{Host: "127.0.0.1"}

		type probeResult struct {
			Label   string
			Host    string
			Mode    string
			Latency time.Duration
			Err     error
		}

		results := make([]probeResult, 0, len(servers))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for label, info := range servers {
			wg.Add(1)
			go func(label string, info serverInfo) {
				defer wg.Done()
				addr := net.JoinHostPort(info.Host, "22")
				start := time.Now()
				conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
				latency := time.Since(start)
				if err == nil {
					_ = conn.Close()
				}
				mu.Lock()
				results = append(results, probeResult{label, info.Host, info.Mode, latency, err})
				mu.Unlock()
			}(label, info)
		}
		wg.Wait()

		sort.Slice(results, func(i, j int) bool {
			return results[i].Label < results[j].Label
		})

		for _, r := range results {
			modeStr := ""
			if r.Mode != "" {
				modeStr = fmt.Sprintf("  %-12s", r.Mode)
			} else {
				modeStr = fmt.Sprintf("  %-12s", "")
			}
			if r.Err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "  %-20s %-25s%s  unreachable\n", r.Label, r.Host, modeStr)
			} else {
				_, _ = fmt.Fprintf(os.Stdout, "  %-20s %-25s%s  ok (%dms)\n", r.Label, r.Host, modeStr, r.Latency.Milliseconds())
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serversCmd)
}
