package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Create and attach to an ephemeral machine",
	Long: `Create a container and open an interactive shell. Destroys on disconnect by default.

  podspawn run scratch                     # ephemeral, default image
  podspawn run scratch --image alpine:3.20   # ephemeral, custom image`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		image, _ := cmd.Flags().GetString("image")

		ls, err := buildLocalSession(name)
		if err != nil {
			return err
		}
		defer ls.Close()

		if image != "" {
			ls.Session.Image = image
		}

		ls.Session.Mode = "destroy-on-disconnect"
		ls.Session.GracePeriod = 0

		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")

		exitCode := ls.Session.RunAndCleanup(context.Background())
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func init() {
	runCmd.Flags().String("image", "", "base image (default from config)")
	rootCmd.AddCommand(runCmd)
}
