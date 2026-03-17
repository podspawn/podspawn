package cmd

import (
	"context"
	"fmt"
	"time"

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

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		containerName, err := ls.Session.Ensure(ctx)
		if err != nil {
			return fmt.Errorf("creating machine %q: %w", name, err)
		}

		fmt.Printf("created machine %q (%s)\n", name, containerName)
		return nil
	},
}

func init() {
	createCmd.Flags().String("image", "", "base image (default from config)")
	rootCmd.AddCommand(createCmd)
}
