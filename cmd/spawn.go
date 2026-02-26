package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/spf13/cobra"
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "ForceCommand handler, routes session I/O to containers",
	Run: func(cmd *cobra.Command, args []string) {
		user, _ := cmd.Flags().GetString("user")

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			slog.Error("failed to connect to docker", "error", err)
			os.Exit(1)
		}

		sess := &spawn.Session{
			Username: user,
			Runtime:  rt,
		}

		exitCode := sess.RunAndCleanup(context.Background())
		os.Exit(exitCode)
	},
}

func init() {
	spawnCmd.Flags().String("user", "", "username for the session")
	_ = spawnCmd.MarkFlagRequired("user")
	rootCmd.AddCommand(spawnCmd)
}
