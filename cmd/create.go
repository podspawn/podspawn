package cmd

import (
	"context"
	"time"

	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a named machine",
	Long: `Create a persistent container that you can attach to later with podspawn shell.

  podspawn create mydev                    # default image
  podspawn create mydev --image alpine:3.20  # custom image`,
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

		spin := ui.NewSpinner("Creating %s (%s)", name, ls.Session.Image)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		_, err = ls.Session.Ensure(ctx)
		if err != nil {
			spin.Fail()
			return err
		}

		spin.Stop()
		return nil
	},
}

func init() {
	createCmd.Flags().String("image", "", "base image (default from config)")
	rootCmd.AddCommand(createCmd)
}
