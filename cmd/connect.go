package cmd

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect <user> <host> <port>",
	Short: "ProxyCommand handler for .pod namespace routing",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, host, port := args[0], args[1], args[2]

		target, err := resolveTarget(host, port)
		if err != nil {
			return err
		}

		return relay(target, os.Stdin, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func resolveTarget(host, port string) (string, error) {
	if !strings.HasSuffix(host, ".pod") {
		return net.JoinHostPort(host, port), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	clientCfg, err := config.LoadClient(filepath.Join(home, ".podspawn", "config.yaml"))
	if err != nil {
		return "", err
	}

	resolved, err := clientCfg.ResolveHost(host)
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(resolved, port), nil
}

func relay(addr string, stdin io.Reader, stdout io.Writer) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	done := make(chan struct{})

	go func() {
		_, _ = io.Copy(stdout, conn)
		_ = conn.Close()
		close(done)
	}()

	go func() {
		_, _ = io.Copy(conn, stdin)
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	<-done
	return nil
}
