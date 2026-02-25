package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "ForceCommand handler, routes session I/O to containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("spawn: no container runtime configured")
	},
}

func init() {
	spawnCmd.Flags().String("user", "", "Username for the session")
	_ = spawnCmd.MarkFlagRequired("user")
	rootCmd.AddCommand(spawnCmd)
}
