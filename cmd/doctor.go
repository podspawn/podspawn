package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system prerequisites and configuration",
	Long:  `Runs diagnostic checks on Docker, sshd, permissions, and configuration. Use this to debug setup issues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		checkCfg := doctor.CheckConfig{
			SSHDConfigPath: "/etc/ssh/sshd_config",
			KeyDir:         cfg.Auth.KeyDir,
			StateDir:       cfg.State.DBPath[:len(cfg.State.DBPath)-len("/state.db")], // strip filename
			LockDir:        cfg.State.LockDir,
			DefaultImage:   cfg.Defaults.Image,
		}

		fmt.Println("podspawn doctor")
		fmt.Println()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		passed, warned, failed := doctor.RunAll(ctx, checkCfg, os.Stdout)

		fmt.Println()
		fmt.Printf("%d passed, %d warnings, %d failed\n", passed, warned, failed)

		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
