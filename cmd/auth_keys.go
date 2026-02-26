package cmd

import (
	"log/slog"
	"os"

	"github.com/podspawn/podspawn/internal/authkeys"
	"github.com/spf13/cobra"
)

const defaultKeyDir = "/etc/podspawn/keys"

var authKeysCmd = &cobra.Command{
	Use:   "auth-keys <username> [key-type] [key-data]",
	Short: "AuthorizedKeysCommand handler for sshd",
	Args:  cobra.MinimumNArgs(1),
	// sshd expects exit 0 regardless of outcome. A non-zero exit
	// or crash just means "no keys found, fall through."
	Run: func(cmd *cobra.Command, args []string) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("auth-keys panic", "error", r)
			}
		}()

		username := args[0]
		binPath, err := os.Executable()
		if err != nil {
			binPath = "/usr/local/bin/podspawn"
		}

		keyDir, _ := cmd.Flags().GetString("key-dir")
		n, err := authkeys.Lookup(username, keyDir, binPath, os.Stdout)
		if err != nil {
			slog.Error("auth-keys lookup failed", "user", username, "error", err)
			return
		}
		slog.Debug("auth-keys", "user", username, "keys", n)
	},
}

func init() {
	authKeysCmd.Flags().String("key-dir", defaultKeyDir, "directory containing per-user key files")
	rootCmd.AddCommand(authKeysCmd)
}
