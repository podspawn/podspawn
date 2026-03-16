package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping <server>",
	Short: "Check if a podspawn server is reachable",
	Long: `Tests SSH connectivity to a server and reports the podspawn version running there.

  podspawn ping devbox.company.com
  podspawn ping work.pod`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, err := resolvePodHost(args[0])
		if err != nil {
			return err
		}

		// TCP probe
		fmt.Printf("pinging %s (port 22)...\n", host)
		addr := net.JoinHostPort(host, "22")
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		latency := time.Since(start)

		if err != nil {
			return fmt.Errorf("unreachable: %w", err)
		}
		_ = conn.Close()
		fmt.Printf("  tcp connect: %dms\n", latency.Milliseconds())

		// Try SSH to get podspawn version
		sshBin, sshErr := exec.LookPath("ssh")
		if sshErr == nil {
			sshStart := time.Now()
			out, err := exec.Command(sshBin,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=5",
				"-o", "BatchMode=yes",
				// Use a dummy user; auth will fail but we can still measure
				fmt.Sprintf("probe@%s", host),
				"podspawn version",
			).CombinedOutput()
			sshLatency := time.Since(sshStart)

			output := strings.TrimSpace(string(out))
			if err == nil && strings.Contains(output, "podspawn") {
				fmt.Printf("  ssh round-trip: %dms\n", sshLatency.Milliseconds())
				fmt.Printf("  remote version: %s\n", output)
			} else {
				fmt.Fprintf(os.Stderr, "  ssh probe failed (auth required; TCP is reachable)\n")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
